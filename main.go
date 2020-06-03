package main

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
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
	// TODO: multiple session by domain
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
		GetEnvAsInt("CACHE_MAX_AGE", 7200),
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

	logger("\n== REQ %s == %s%s?%s\n", time.Now().Format("15:04:05.000"), template, filename, r.URL.RawQuery)

	if err := validateTemplate(template); err != nil {
		s.buildErrorResponse(w, err)
		return
	}

	originalFilename := ReplacePathExt(filename, r)

	if err := validateFilename(originalFilename); err != nil {
		s.buildErrorResponse(w, err)
		return
	}

	resizedFilename := template + filename
	resizedFile, modTime, err := s.getResizedImageFile(resizedFilename)
	if err == nil {
		s.buildResponse(w, r, resizedFile, *modTime, resizedFilename)
		return
	}

	originalFile, err := s.getOriginalImageFile(originalFilename)
	if err != nil {
		s.buildNotFoundResponse(w, r, template)
		return
	}
	_ = originalFile.Close()
	defer os.Remove(originalFile.Name())

	resizedFile, err = s.resizeImageFile(originalFile.Name(), s.getLocalResizedPath(resizedFilename), template)
	if err != nil {
		s.buildErrorResponse(w, err)
		return
	}

	s.buildResponse(w, r, resizedFile, time.Now(), resizedFilename)
}

func (s *server) getLocalResizedPath(path string) string {
	return s.localCacheRoot + path
}

func (s *server) getCloudResizedPath(path string) string {
	return s.cloudCacheRoot + path
}

func (s *server) getResizedImageFile(path string) (f *os.File, t *time.Time, err error) {
	logger("Get local resized image %s\n", s.getLocalResizedPath(path))
	f, t, err = OpenFileWithModTime(s.getLocalResizedPath(path))

	if err != nil && s.cloudCache {
		logger("Get cloud resized image S3://%s\n", s.getCloudResizedPath(path))
		f, t, err = s.s3.DownloadFileWithModTime(s.getCloudResizedPath(path), s.getLocalResizedPath(path))
	}

	return
}

func (s *server) getOriginalImageFile(path string) (*os.File, error) {
	logger("Get original image S3:/%s\n", path)
	return s.s3.DownloadFile(path, "")
}

func (s *server) resizeImageFile(inputFilePath string, outputFilePath string, template string) (*os.File, error) {
	if shouldNotResize(template, inputFilePath) {
		logger("Return original\n")
		err := CopyFile(inputFilePath, outputFilePath)
		if err != nil {
			return nil, err
		}
	} else {
		logger("Return resized\n")
		w, h, err := GetImageDimensions(inputFilePath)
		if err != nil {
			return nil, err
		}

		err = ConvertFile(inputFilePath, w, h, outputFilePath, NewTemplate(template))
		if err != nil {
			return nil, err
		}
	}

	err := OptimizeFile(outputFilePath)
	if err != nil {
		return nil, err
	}

	return os.Open(outputFilePath)
}

func (s *server) writeResizedImageFile(filePath string, content io.ReadSeeker, wg *sync.WaitGroup) {
	logger("Write local resized image %s\n", s.getLocalResizedPath(filePath))
	wg.Add(1)
	go func() {
		defer wg.Done()
		changed, _ := WriteFileFromReader(s.getLocalResizedPath(filePath), content)
		if changed {
			logger(" - [async] Written local resized image %s\n", s.getLocalResizedPath(filePath))
		} else {
			logger(" - [async] Unchanged local resized image %s\n", s.getLocalResizedPath(filePath))
		}
	}()

	if s.cloudCache {
		logger("Write cloud resized image S3://%s\n", s.getCloudResizedPath(filePath))
		wg.Add(1)
		go func() {
			defer wg.Done()
			changed, _ := s.s3.UploadIfNewerContentReader(s.getCloudResizedPath(filePath), time.Now().Add(-1*time.Second*time.Duration(s.cacheMaxAge)), content)
			if changed {
				logger(" - [async] Written cloud resized image S3://%s\n", s.getCloudResizedPath(filePath))
			} else {
				logger(" - [async] Unchanged cloud resized image S3://%s\n", s.getCloudResizedPath(filePath))
			}
		}()
	}
}

func (s *server) buildResponse(w http.ResponseWriter, r *http.Request, content io.ReadSeeker, modTime time.Time, resizedFilePath string) {
	var wg sync.WaitGroup
	s.writeResizedImageFile(resizedFilePath, content, &wg)

	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d, public", s.cacheMaxAge))
	http.ServeContent(w, r, resizedFilePath, modTime, content)

	logger("== RESP %s == %d %s\n", time.Now().Format("15:04:05.000"), http.StatusOK, resizedFilePath)

	go func() {
		wg.Wait()
		if f, ok := content.(io.Closer); ok {
			_ = f.Close()
		}
		logger(" - [async] Finish response\n")
	}()
}

func (s *server) buildNotFoundResponse(w http.ResponseWriter, r *http.Request, template string) {
	logger("Return 404 response\n")
	resizedFilePath := template + "/" + filepath.Base(s.notFoundFilename)
	resizedFile, _, err := s.getResizedImageFile(resizedFilePath)

	if err != nil {
		resizedFile, err = s.resizeImageFile(s.notFoundFilename, s.getLocalResizedPath(resizedFilePath), template)

		if err != nil {
			s.buildErrorResponse(w, err)
			return
		}
	}

	// TODO: add cache precondition
	w.Header().Set("Cache-Control", "max-age=60, public")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.Copy(w, resizedFile)

	_ = resizedFile.Close()

	logger("== RESP %s == %d %s\n", time.Now().Format("15:04:05.000"), http.StatusNotFound, resizedFile.Name())
}

func (s *server) buildErrorResponse(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if err, ok := err.(HttpError); ok {
		code = err.statusCode
	}

	w.Header().Set("Cache-Control", "max-age=60, public")
	http.Error(w, err.Error(), code)

	logger("== ERR %s == %d %s\n", time.Now().Format("15:04:05.000"), code, err.Error())
}
