package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"
)

func GetEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func GetEnvAsInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		value, _ := strconv.Atoi(value)
		return value
	}

	return fallback
}

func GetEnvAsBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		value, _ := strconv.ParseBool(value)
		return value
	}

	return fallback
}

func GetReaderContentType(c io.ReadSeeker) (mime string, err error) {
	b := make([]byte, 512)
	_, err = c.Read(b)

	if err != nil {
		return
	}

	mime = http.DetectContentType(b)

	_, err = c.Seek(0, io.SeekStart)

	return
}

func OpenFileWithModTime(p string) (*os.File, *time.Time, error) {
	f, err := os.Open(p)

	if err != nil {
		return nil, nil, err
	}

	d, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}

	t := d.ModTime()

	return f, &t, nil
}

func WriteFileFromReader(p string, c io.Reader) (bool, error) {
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		return false, nil
	}

	_ = os.MkdirAll(filepath.Dir(p), 0777)

	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return false, err
	}

	_, err = io.Copy(f, c)
	_ = f.Close()
	if err != nil {
		return false, err
	}

	return true, nil
}

func CopyFile(src, dst string) (err error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return
	}
	defer srcFile.Close()

	_ = os.MkdirAll(filepath.Dir(dst), 0777)
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)

	return
}

func CreateTempFileFromReader(c io.Reader, ext string) (f *os.File, err error) {
	f, err = ioutil.TempFile("", "_resize_*"+ext)

	if err != nil {
		return
	}

	_, err = io.Copy(f, c)

	if err != nil {
		return
	}

	_, err = f.Seek(0, io.SeekStart)

	return
}

func ReplacePathExt(filepath string, r *http.Request) string {
	ext := r.URL.Query().Get("original")

	if ext == "" {
		return filepath
	}

	return filepath[0:len(filepath)-len(path.Ext(filepath))+1] + ext
}

var verbose = GetEnvAsBool("VERBOSE", true)

func logger(format string, a ...interface{}) {
	if verbose {
		fmt.Printf(format, a...)
	}
}
