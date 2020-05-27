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
	router         *httprouter.Router
	s3             *s3client
	localCacheRoot string
	cloudCache     bool
	cloudCacheRoot string
	cacheMaxAge    int
}

func (s *server) getLocalResizedPath(path string) string {
	return s.localCacheRoot + path
}

func (s *server) getCloudResizedPath(path string) string {
	return s.cloudCacheRoot + path
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
	}

	s.router.GET("/image/:template/*filepath", s.imageResizeHandler)

	fmt.Print("Listening on http://localhost:3000/\n")
	log.Fatal(http.ListenAndServe(":3000", routerMux))
}

func (s *server) imageResizeHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	template := p.ByName("template")
	filename := p.ByName("filepath")

	resizedFile, modTime, err := s.getResizedImageFile(template + filename)
	if err == nil {
		s.buildResponse(w, r, filename, resizedFile, *modTime)
		return
	}

	originalFilename := ReplacePathExt(filename, r)
	originalExt := strings.ToLower(filepath.Ext(originalFilename))
	resizedExt := strings.ToLower(filepath.Ext(filename))

	if err := validateFilename(originalExt); err != nil {
		s.buildError(w, err.Error(), http.StatusBadRequest)
		return
	}

	originalFile, modTime, err := s.getOriginalImageFile(originalFilename)
	if err != nil {
		// TODO: add no-image placeholder response
		s.buildError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	if isOriginalTemplate(template, originalExt) {
		s.buildResponse(w, r, filename, originalFile, *modTime)
		return
	}

	if err := validateTemplate(template); err != nil {
		s.buildError(w, err.Error(), http.StatusBadRequest)
		return
	}

	resizedFile, err = ResizeImage(originalFile, s.getLocalResizedPath(template+filename), NewTemplate(template, originalExt, resizedExt))
	if err != nil {
		s.buildError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeResizedImageFile(template+filename, resizedFile)

	s.buildResponse(w, r, filename, resizedFile, time.Now())
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

func (s *server) buildError(w http.ResponseWriter, err string, code int) {
	w.Header().Set("Cache-Control", "max-age=60, public")

	http.Error(w, err, code)
}
