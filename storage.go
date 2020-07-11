package imageresizer

import (
	"io"
	"os"
	"time"
)

type Storage interface {
	DownloadFile(path string, outputPath string) (f *os.File, err error)
	DownloadFileWithModTime(path string, outputPath string) (f *os.File, t *time.Time, err error)
	UploadContentReaderIfNewer(path string, time time.Time, content io.ReadSeeker) (bool, error)
	UploadContentReader(path string, content io.ReadSeeker) error
}
