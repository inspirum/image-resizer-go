package main

import (
	"bytes"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type s3client struct {
	s3     *s3.S3
	bucket string
}

func NewS3Client(endpoint string, bucket string, key string, secret string, region string) *s3client {
	session, err := session.NewSession(&aws.Config{
		Endpoint:    aws.String(endpoint),
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(key, secret, ""),
	})

	if err != nil {
		fmt.Printf("Unable create New S3 session %v\n", err)
		os.Exit(1)
	}

	return &s3client{s3.New(session), bucket}
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
		err = WriteFileFromReader(outputPath, resp.Body)
		if err != nil {
			return
		}
		f, err = os.Open(outputPath)
	}

	return
}

func (c *s3client) GetContentWithModTime(path string) ([]byte, *time.Time, error) {
	resp, err := c.getContent(path)

	if err != nil {
		return nil, nil, err
	}

	defer resp.Body.Close()

	buffer, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, nil, err
	}

	return buffer, resp.LastModified, nil
}

func (c *s3client) GetContent(path string) ([]byte, error) {
	resp, err := c.getContent(path)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	buffer, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	return buffer, nil
}

func (c *s3client) UploadContent(path string, content []byte) error {
	return c.UploadContentReader(path, bytes.NewReader(content))
}

func (c *s3client) UploadContentReader(path string, content io.ReadSeeker) (err error) {
	mime, err := GetReaderContentType(content)
	if err != nil {
		return
	}

	_, err = c.s3.PutObject(&s3.PutObjectInput{
		Key:                aws.String(path),
		Body:               content,
		Bucket:             aws.String(c.bucket),
		ContentType:        aws.String(mime),
		ContentDisposition: aws.String("attachment"),
	})

	return
}

func (c *s3client) getContent(path string) (*s3.GetObjectOutput, error) {
	resp, err := c.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(path),
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}
