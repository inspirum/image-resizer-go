package main

import (
	"fmt"
	imageresizer "github.com/inspirum/image-resizer-go"
	"github.com/julienschmidt/httprouter"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type server struct {
	router           *httprouter.Router
	storage          imageresizer.Storage
	localCache       bool
	localCacheRoot   string
	cloudCache       bool
	cloudCacheRoot   string
	cacheMaxAge      int
	notFoundFilename string
}

func main() {
	routerMux := httprouter.New()
	s3Service := imageresizer.NewS3Client(
		imageresizer.GetEnv("S3_ENDPOINT", ""),
		imageresizer.GetEnv("S3_BUCKET", ""),
		imageresizer.GetEnv("S3_KEY", ""),
		imageresizer.GetEnv("S3_SECRET", ""),
		imageresizer.GetEnv("S3_REGION", ""),
		imageresizer.GetEnvAsBool("S3_USE_PATH_STYLE", false),
	)

	s := &server{
		routerMux,
		s3Service,
		true,
		imageresizer.GetEnv("STORAGE_LOCAL_PREFIX", "./cache/"),
		imageresizer.GetEnvAsBool("STORAGE_CLOUD_ENABLED", true),
		imageresizer.GetEnv("STORAGE_CLOUD_PREFIX", ""),
		imageresizer.GetEnvAsInt("CACHE_MAX_AGE", 7200),
		imageresizer.GetEnv("NOTFOUND_FILENAME", "./static/no_image.png"),
	}

	s.router.GET("/image/:template/*filepath", s.ServeFile)

	port := imageresizer.GetEnvAsInt("PORT", 3000)
	fmt.Printf("Listening on http://localhost:%d/\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), routerMux))
}

func (s *server) ServeFile(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	template := p.ByName("template")
	filename := p.ByName("filepath")

	logger("\n== REQ %s == %s%s?%s\n", time.Now().Format("15:04:05.000"), template, filename, r.URL.RawQuery)

	if err := imageresizer.ValidateTemplate(template); err != nil {
		s.buildErrorResponse(w, err)
		return
	}

	originalFilename := imageresizer.ReplacePathExt(filename, r)
	if err := imageresizer.ValidateFilename(originalFilename); err != nil {
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
	vLogger("Get local resized image %s\n", s.getLocalResizedPath(path))
	f, t, err = imageresizer.OpenFileWithModTime(s.getLocalResizedPath(path))

	if err != nil && s.cloudCache {
		vLogger("Get cloud resized image cloud://%s\n", s.getCloudResizedPath(path))
		f, t, err = s.storage.DownloadFileWithModTime(s.getCloudResizedPath(path), s.getLocalResizedPath(path))
	}

	return
}

func (s *server) getOriginalImageFile(path string) (*os.File, error) {
	vLogger("Get original image cloud:/%s\n", path)
	return s.storage.DownloadFile(path, "")
}

func (s *server) resizeImageFile(inputFilePath string, outputFilePath string, template string) (*os.File, error) {
	if imageresizer.ShouldNotResize(template, inputFilePath) {
		vLogger("Return original\n")
		err := imageresizer.CopyFile(inputFilePath, outputFilePath)
		if err != nil {
			return nil, err
		}
	} else {
		vLogger("Return resized\n")
		w, h, err := imageresizer.GetImageDimensions(inputFilePath)
		if err != nil {
			return nil, err
		}

		err = imageresizer.ConvertFile(inputFilePath, w, h, outputFilePath, imageresizer.NewTemplate(template))
		if err != nil {
			return nil, err
		}
	}

	err := imageresizer.OptimizeFile(outputFilePath)
	if err != nil {
		return nil, err
	}

	return os.Open(outputFilePath)
}

func (s *server) writeResizedImageFile(filePath string, outputFilePath string, wg *sync.WaitGroup) {
	if s.localCache {
		localResizedPath := s.getLocalResizedPath(outputFilePath)
		vLogger("- [async] Write local resized image %s\n", localResizedPath)
		wg.Add(1)
		go func() {
			time.Sleep(time.Duration(1))
			defer wg.Done()
			content, err := os.Open(filePath)
			if err != nil {
				vLogger("- [await] Error local resized image %s: %v\n", localResizedPath, err)
			}
			changed, err := imageresizer.WriteFileFromReader(localResizedPath, content)
			_ = content.Close()
			if err != nil {
				vLogger("- [await] Error local resized image %s: %v\n", localResizedPath, err)
			} else if changed {
				vLogger("- [await] Written local resized image %s\n", localResizedPath)
			} else {
				vLogger("- [await] Unchanged local resized image %s\n", localResizedPath)
			}
		}()
	}

	if s.cloudCache {
		cloudResizedPath := s.getCloudResizedPath(outputFilePath)
		vLogger("- [async] Write cloud resized image cloud://%s\n", cloudResizedPath)
		wg.Add(1)
		go func() {
			defer wg.Done()
			content, err := os.Open(filePath)
			if err != nil {
				vLogger("- [await] Error cloud resized image %s: %v\n", cloudResizedPath, err)
			}
			changed, err := s.storage.UploadContentReaderIfNewer(cloudResizedPath, time.Now().Add(-1*time.Second*time.Duration(s.cacheMaxAge)), content)
			_ = content.Close()
			if err != nil {
				vLogger("- [await] Error cloud resized image %s: %v\n", cloudResizedPath, err)
			} else if changed {
				vLogger("- [await] Written cloud resized image cloud://%s\n", cloudResizedPath)
			} else {
				vLogger("- [await] Unchanged cloud resized image cloud://%s\n", cloudResizedPath)
			}
		}()
	}
}

func (s *server) buildResponse(w http.ResponseWriter, r *http.Request, content *os.File, modTime time.Time, resizedFilePath string) {
	var wg sync.WaitGroup

	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d, public", s.cacheMaxAge))
	http.ServeContent(w, r, resizedFilePath, modTime, content)
	_ = content.Close()

	s.writeResizedImageFile(content.Name(), resizedFilePath, &wg)

	logger("== RESP %s == %d %s\n", time.Now().Format("15:04:05.000"), http.StatusOK, resizedFilePath)

	go func() {
		wg.Wait()
		vLogger("- [await] Finish response\n")
	}()
}

func (s *server) buildNotFoundResponse(w http.ResponseWriter, r *http.Request, template string) {
	vLogger("Return 404 response\n")
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
	statusCode, _ := strconv.Atoi(r.URL.Query().Get("status"))
	if statusCode < 200 || statusCode >= 500 {
		statusCode = http.StatusNotFound
	}

	w.WriteHeader(statusCode)
	_, _ = io.Copy(w, resizedFile)

	_ = resizedFile.Close()

	logger("== RESP %s == %d %s\n", time.Now().Format("15:04:05.000"), statusCode, resizedFile.Name())
}

func (s *server) buildErrorResponse(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if err, ok := err.(imageresizer.HttpError); ok {
		code = err.StatusCode
	}

	w.Header().Set("Cache-Control", "max-age=60, public")
	http.Error(w, err.Error(), code)

	logger("== ERR %s == %d %s\n", time.Now().Format("15:04:05.000"), code, err.Error())
}

var verbose = imageresizer.GetEnvAsBool("VERBOSE", true)

func vLogger(format string, a ...interface{}) {
	if verbose {
		logger(format, a...)
	}
}

func logger(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}
