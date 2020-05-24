package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type server struct {
	router         *httprouter.Router
	s3             *s3client
	localCacheRoot string
	cloudCacheRoot string
}

func main() {
	routerMux := httprouter.New()
	s3Service := NewS3Client(
		os.Getenv("S3_ENDPOINT"),
		os.Getenv("S3_BUCKET"),
		os.Getenv("S3_KEY"),
		os.Getenv("S3_SECRET"),
		os.Getenv("S3_REGION"),
	)

	s := &server{
		routerMux,
		s3Service,
		"./cache/",
		os.Getenv("STORAGE_PREFIX"),
	}

	s.router.GET("/image/:template/*filepath", s.imageResizeHandler)

	log.Fatal(http.ListenAndServe(":3000", routerMux))
}

func (s *server) imageResizeHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	template := p.ByName("template")
	filename := p.ByName("filepath")

	fileReader, modTime, err := s.getResizedImageContent(template + filename)

	if err == nil {
		buildResponse(w, r, filename, fileReader, *modTime)
		return
	}

	originalFilename := replaceImagePathExt(filename, r)
	fileContent, modTime, err := s.getOriginalImageContent(originalFilename)

	if err != nil {
		buildError(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	originalExt := strings.ToLower(filepath.Ext(originalFilename))
	resizedExt := strings.ToLower(filepath.Ext(filename))

	if err := validateFilename(originalExt); err != nil {
		buildError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if isOriginalTemplate(template, originalExt) {
		buildResponse(w, r, filename, bytes.NewReader(fileContent), *modTime)
		return
	}

	if err := validateTemplate(template); err != nil {
		buildError(w, err.Error(), http.StatusBadRequest)
		return
	}

	templateConfig := NewTemplate(template)
	templateConfig.inputExt = originalExt
	templateConfig.outputExt = resizedExt

	resizedFileContent, err := resizeImage(fileContent, templateConfig)

	if err != nil {
		buildError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content, _ := ioutil.ReadAll(resizedFileContent)
	err = s.writeFile(template+filename, content)

	if err != nil {
		buildError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buildResponse(w, r, filename, resizedFileContent, time.Now())
}

func (s *server) getLocalResizedPath(path string) string {
	return s.localCacheRoot + path
}

func (s *server) getCloudResizedPath(path string) string {
	return s.cloudCacheRoot + path
}

func (s *server) getResizedImageContent(path string) (io.ReadSeeker, *time.Time, error) {
	f, err := os.Open(s.getLocalResizedPath(path))
	if err == nil {
		d, err := f.Stat()
		if err != nil {
			return nil, nil, err
		}

		modTime := d.ModTime()

		return f, &modTime, nil
	}

	b, t, err := s.s3.GetContentWithModTime(s.getCloudResizedPath(path))

	if err == nil {
		return bytes.NewReader(b), t, nil
	}

	return nil, nil, err
}

func (s *server) getOriginalImageContent(path string) ([]byte, *time.Time, error) {
	return s.s3.GetContentWithModTime(path)
}

func (s *server) writeFile(filename string, content []byte) error {
	// TODO: async
	localResizedPath := s.getLocalResizedPath(filename)
	cloudResizedPath := s.getCloudResizedPath(filename)

	if _, err := os.Stat(localResizedPath); os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Dir(localResizedPath), 0700)
		err = ioutil.WriteFile(localResizedPath, content, 0700)
		if err != nil {
			return errors.New(fmt.Sprintf("Error writing out resized image, %s\n", err))
		}
	}

	err := s.s3.UploadContent(cloudResizedPath, content)

	if err != nil {
		return errors.New(fmt.Sprintf("Error writing out resized image, %s\n", err))
	}

	return nil
}

func replaceImagePathExt(filepath string, r *http.Request) string {
	keys, ok := r.URL.Query()["original"]
	var ext string

	if !ok || len(keys[0]) < 1 {
		ext = path.Ext(filepath)
	} else {
		ext = "." + keys[0]
	}

	return filepath[0:len(filepath)-len(path.Ext(filepath))] + ext
}

func buildResponse(w http.ResponseWriter, r *http.Request, filename string, content io.ReadSeeker, modTime time.Time) {
	// TODO: add etag
	http.ServeContent(w, r, filename, modTime, content)
}

func buildError(w http.ResponseWriter, error string, code int) {
	http.Error(w, error, code)
}
