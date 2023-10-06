package imageresizer

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
	"os"
	"path/filepath"
	"time"
)

type s3client struct {
	s3     *s3.S3
	bucket string
}

func NewS3Client(endpoint string, bucket string, key string, secret string, region string, pathStyle bool) *s3client {
	s, err := session.NewSession(&aws.Config{
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(region),
		Credentials:      credentials.NewStaticCredentials(key, secret, ""),
		S3ForcePathStyle: aws.Bool(pathStyle),
	})

	if err != nil {
		fmt.Printf("Unable create New S3 s %v\n", err)
		os.Exit(1)
	}

	return &s3client{s3.New(s), bucket}
}

func (c *s3client) DownloadFile(path string, outputPath string) (f *os.File, err error) {
	f, _, err = c.DownloadFileWithModTime(path, outputPath)

	return
}

func (c *s3client) DownloadFileWithModTime(path string, outputPath string) (f *os.File, t *time.Time, err error) {
	resp, err := c.getContent(path)

	if err != nil {
		return nil, nil, err
	}

	t = resp.LastModified
	defer resp.Body.Close()

	if outputPath == "" {
		f, err = CreateTempFileFromReader(resp.Body, filepath.Ext(path))
	} else {
		_, err = WriteFileFromReader(outputPath, resp.Body)
		if err != nil {
			return
		}
		f, err = os.Open(outputPath)
	}

	return
}

func (c *s3client) UploadContentReaderIfNewer(path string, time time.Time, content io.ReadSeeker) (bool, error) {
	resp, err := c.headContent(path)

	if err == nil && resp.LastModified.After(time) {
		return false, nil
	}

	err = c.UploadContentReader(path, content)

	if err != nil {
		return false, err
	}

	return true, nil
}

func (c *s3client) UploadContentReader(path string, content io.ReadSeeker) error {
	mime, err := GetReaderContentType(content)
	if err != nil {
		return err
	}

	return c.putContent(path, content, mime)
}

func (c *s3client) putContent(path string, content io.ReadSeeker, mime string) (err error) {
	_, err = c.s3.PutObject(&s3.PutObjectInput{
		Key:                aws.String(path),
		Body:               content,
		Bucket:             aws.String(c.bucket),
		ContentType:        aws.String(mime),
		ContentDisposition: aws.String("attachment"),
	})

	return
}

func (c *s3client) getContent(path string) (resp *s3.GetObjectOutput, err error) {
	resp, err = c.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(path),
	})

	return
}

func (c *s3client) headContent(path string) (resp *s3.HeadObjectOutput, err error) {
	resp, err = c.s3.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(path),
	})

	return
}
