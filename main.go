package main

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type server struct {
	router           *httprouter.Router
	s3               *s3client
	localCacheRoot   string
	cloudCache       bool
	cloudCacheRoot   string
	cacheMaxAge      int
	notFoundFilename string
}

type HttpError struct {
	error
	statusCode int
}

func main() {
	routerMux := httprouter.New()
	s3Service := NewS3Client(
		GetEnv("S3_ENDPOINT", ""),
		GetEnv("S3_BUCKET", ""),
		GetEnv("S3_KEY", ""),
		GetEnv("S3_SECRET", ""),
		GetEnv("S3_REGION", ""),
	)

	s := &server{
		routerMux,
		s3Service,
		GetEnv("STORAGE_LOCAL_PREFIX", "./cache/"),
		GetEnvAsBool("STORAGE_CLOUD_ENABLED", true),
		GetEnv("STORAGE_CLOUD_PREFIX", ""),
		GetEnvAsInt("STORAGE_CLOUD_PREFIX", 7200),
		GetEnv("NOTFOUND_FILENAME", "./static/no_image.png"),
	}

	s.router.GET("/image/:template/*filepath", s.ServeFile)

	port := GetEnvAsInt("PORT", 3000)
	fmt.Printf("Listening on http://localhost:%d/\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), routerMux))
}

func (s *server) ServeFile(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	template := p.ByName("template")
	filename := p.ByName("filepath")

	originalFilename := ReplacePathExt(filename, r)
	originalExt := strings.ToLower(filepath.Ext(originalFilename))
	resizedExt := strings.ToLower(filepath.Ext(filename))

	if err := validateFilename(originalExt); err != nil {
		s.buildErrorResponse(w, err)
		return
	}

	if err := validateTemplate(template); err != nil {
		s.buildErrorResponse(w, err)
		return
	}

	resizedFile, modTime, err := s.getResizedImageFile(template + filename)
	if err == nil {
		s.buildResponse(w, r, filename, resizedFile, *modTime)
		return
	}

	originalFile, modTime, err := s.getOriginalImageFile(originalFilename)
	if err != nil {
		s.buildNotFoundResponse(w, r, template, originalExt)
		return
	}
	defer os.Remove(originalFile.Name())

	resizedFile, modTime, err = s.resizeImageFile(template, originalFile, modTime, s.getLocalResizedPath(template+filename), originalExt, resizedExt)
	if err != nil {
		s.buildErrorResponse(w, err)
		return
	}

	s.writeResizedImageFile(template+filename, resizedFile)

	s.buildResponse(w, r, filename, resizedFile, *modTime)
}

func (s *server) getLocalResizedPath(path string) string {
	return s.localCacheRoot + path
}

func (s *server) getCloudResizedPath(path string) string {
	return s.cloudCacheRoot + path
}

func (s *server) getResizedImageFile(path string) (f *os.File, t *time.Time, err error) {
	f, t, err = OpenFileWithModTime(s.getLocalResizedPath(path))

	if err == nil {
		return
	}

	f, t, err = s.s3.DownloadFileWithModTime(s.getCloudResizedPath(path), s.getLocalResizedPath(path))

	return
}

func (s *server) getOriginalImageFile(path string) (*os.File, *time.Time, error) {
	return s.s3.DownloadFileWithModTime(path, "")
}

func (s *server) resizeImageFile(template string, originalFile *os.File, modTime *time.Time, outputFilename string, originalExt string, resizedExt string) (*os.File, *time.Time, error) {
	if shouldNotResize(template, originalExt) {
		return originalFile, modTime, nil
	}

	defer originalFile.Close()
	resizedFile, err := ConvertFile(originalFile, outputFilename, NewTemplate(template, originalExt, resizedExt))
	if err != nil {
		return nil, nil, err
	}

	resizedModTime := time.Now()
	return resizedFile, &resizedModTime, nil
}

func (s *server) writeResizedImageFile(filename string, content io.ReadSeeker) {
	go WriteFileFromReader(s.getLocalResizedPath(filename), content)
	if s.cloudCache {
		go s.s3.UploadContentReader(s.getCloudResizedPath(filename), content)
	}
}

func (s *server) buildResponse(w http.ResponseWriter, r *http.Request, filename string, content io.ReadSeeker, modTime time.Time) {
	if f, ok := content.(io.Closer); ok {
		defer f.Close()
	}

	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d, public", s.cacheMaxAge))

	http.ServeContent(w, r, filename, modTime, content)
}

func (s *server) buildNotFoundResponse(w http.ResponseWriter, r *http.Request, template string, originalExt string) {
	resizedFilename := template + "/" + filepath.Base(s.notFoundFilename)
	resizedFile, _, err := s.getResizedImageFile(resizedFilename)

	if err != nil {
		notFoundFile, err := os.Open(s.notFoundFilename)

		if err != nil {
			s.buildErrorResponse(w, err)
			return
		}

		resizedExt := filepath.Ext(s.notFoundFilename)
		notFoundModTime := time.Now()

		resizedFile, _, err = s.resizeImageFile(template, notFoundFile, &notFoundModTime, s.getLocalResizedPath(resizedFilename), originalExt, resizedExt)
	}

	defer resizedFile.Close()

	w.Header().Set("Cache-Control", "max-age=60, public")
	w.WriteHeader(http.StatusNotFound)

	_, _ = io.Copy(w, resizedFile)
}

func (s *server) buildErrorResponse(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if err, ok := err.(HttpError); ok {
		code = err.statusCode
	}
	w.Header().Set("Cache-Control", "max-age=60, public")

	http.Error(w, err.Error(), code)
}
