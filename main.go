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

	fmt.Print("Listening on http://localhost:3000/\n")
	log.Fatal(http.ListenAndServe(":3000", routerMux))
}

func (s *server) imageResizeHandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// get parameters
	template := p.ByName("template")
	filename := p.ByName("filepath")

	fmt.Printf("\n == REQ: %s == \n", template+filename)

	fileReader, modTime, err := s.getResizedImageContent(template + filename)

	// no error - file served
	if err == nil {
		buildResponse(w, r, fileReader, *modTime)
		return
	}

	fileContent, modTime, err := s.getOriginalImageContent(filename, r)

	if err != nil {
		buildError(w, fmt.Sprintf("Not found: %s", err), http.StatusNotFound)
		return
	}

	if isOriginalTemplate(template) {
		buildResponse(w, r, bytes.NewReader(fileContent), *modTime)
		return
	}

	if err := validateTemplate(template); err != nil {
		buildError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templateConfig, err := NewTemplate(template)

	if err != nil {
		buildError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resizedFileContent, err := resizeImage(fileContent, templateConfig)

	if err != nil {
		buildError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = s.writeFile(template+filename, resizedFileContent)

	if err != nil {
		buildError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buildResponse(w, r, bytes.NewReader(resizedFileContent), time.Now())
}

func (s *server) getLocalResizedPath(path string) string {
	return s.localCacheRoot + path
}

func (s *server) getCloudResizedPath(path string) string {
	return s.cloudCacheRoot + path
}

func (s *server) getResizedImageContent(path string) (io.ReadSeeker, *time.Time, error) {
	fmt.Printf("Try to get local resized file %s\n", s.getLocalResizedPath(path))

	// get file from local filesystem
	f, err := os.Open(s.getLocalResizedPath(path))
	if err == nil {

		d, err := f.Stat()
		if err != nil {
			return nil, nil, err
		}

		modTime := d.ModTime()

		return f, &modTime, nil
	}

	fmt.Printf("Try to get cloud resized file S3://%s\n", s.getCloudResizedPath(path))

	// get file from S3 filesystem
	b, t, err := s.s3.GetContentWithModTime(s.getCloudResizedPath(path))

	if err == nil {
		return bytes.NewReader(b), t, nil
	}

	return nil, nil, err
}

func (s *server) getOriginalImageContent(path string, r *http.Request) ([]byte, *time.Time, error) {
	path = replaceImagePathExt(path, r)

	fmt.Printf("Get original image %s\n", path)

	return s.s3.GetContentWithModTime(path)
}

func (s *server) writeFile(filename string, content []byte) error {
	localResizedPath := s.getLocalResizedPath(filename)
	cloudResizedPath := s.getCloudResizedPath(filename)

	if _, err := os.Stat(localResizedPath); os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Dir(localResizedPath), 0700)
		err = ioutil.WriteFile(localResizedPath, content, 0700)
		if err != nil {
			return errors.New(fmt.Sprintf("Error writing out resized image, %s\n", err))
		}
	}

	fmt.Printf("Saving image to local %s\n", localResizedPath)

	err := s.s3.UploadContent(cloudResizedPath, content)

	if err != nil {
		return errors.New(fmt.Sprintf("Error writing out resized image, %s\n", err))
	}

	fmt.Printf("Saving image to cloud S3://%s\n", cloudResizedPath)

	return nil
}

func replaceImagePathExt(filepath string, r *http.Request) string {
	keys, ok := r.URL.Query()["original"]
	var ext string

	if !ok || len(keys[0]) < 1 {
		ext = path.Ext(filepath)
	} else {
		ext = keys[0]
	}

	return filepath[0:len(filepath)-len(path.Ext(filepath))] + ext
}

func buildResponse(w http.ResponseWriter, r *http.Request, content io.ReadSeeker, modTime time.Time) {
	fmt.Printf(" == RESP == \n\n")

	http.ServeContent(w, r, "", modTime, content)
}

func buildError(w http.ResponseWriter, error string, code int) {
	fmt.Printf(" == ERROR == \n\n")

	http.Error(w, error, code)
}
