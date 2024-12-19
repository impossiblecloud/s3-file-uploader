package s3

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/adidenko/s3-file-uploader/internal/cfg"
	"github.com/adidenko/s3-file-uploader/internal/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// Docs: https://docs.aws.amazon.com/sdk-for-go/api/service/s3/

// Client stores pointers to configured remote endpoint writes/clients
type Client struct {
	Session  *session.Session
	Uploader *s3manager.Uploader
}

func getRealSourceFileName(config cfg.AppConfig, filename string) string {
	file := filepath.Base(filename)
	gzipFile := filepath.Join(config.GzipDir, file+".tgz")
	encFile := filepath.Join(config.EncryptDir, file+".tgz")
	realFile := filename

	if config.Gzip {
		realFile = gzipFile
	}

	if config.Encrypt {
		realFile = encFile
	}

	return realFile
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

// FakeUploadFile is used for testing
func FakeUploadFile(config cfg.AppConfig, filename string) (int64, error) {
	realFile := getRealSourceFileName(config, filename)

	fi, err := os.Stat(realFile)
	if err != nil {
		config.Applog.Error(err)
		return 0, err
	}

	config.Applog.Infof("FAKE UPLOAD TO S3: %q file, size %s", realFile, utils.HumanizeBytes(fi.Size(), false))
	return fi.Size(), nil
}

// CopyFile is used for testing, it copies to /var/tmp
func CopyFile(config cfg.AppConfig, filename string) (int64, error) {
	realFile := getRealSourceFileName(config, filename)

	fi, err := os.Stat(realFile)
	if err != nil {
		config.Applog.Error(err)
		return 0, err
	}

	dstFile := filepath.Join("/var/tmp", filepath.Base(realFile))
	_, err = copy(realFile, dstFile)
	if err != nil {
		config.Applog.Error(err)
		return 0, err
	}

	config.Applog.Infof("COPYING FILE: %q file, size %s, destination %q)",
		realFile, utils.HumanizeBytes(fi.Size(), false), dstFile)

	return fi.Size(), nil
}

// NewClient initializes a new s3 client
func NewClient(config cfg.AppConfig) (*Client, error) {

	// The session the S3 Uploader will use
	session := session.Must(session.NewSession())

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(session)

	client := Client{
		Session:  session,
		Uploader: uploader,
	}

	return &client, nil
}

// Close closes s3 client
func (client *Client) Close() {
	// Nothing to do here yet
}

// UploadFile uploads a file to s3
func (client *Client) UploadFile(config cfg.AppConfig, filename string) (int64, error) {
	realFile := getRealSourceFileName(config, filename)

	fi, err := os.Stat(realFile)
	if err != nil {
		config.Applog.Error(err)
		return 0, err
	}

	f, err := os.Open(realFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %q, %v", realFile, err)
	}
	defer f.Close()

	// Upload the file to S3.
	key := fmt.Sprintf("%s/%s", config.S3path, filepath.Base(realFile))
	result, err := client.Uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(config.S3bucket),
		Key:    aws.String(key),
		Body:   f,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to upload file, %v", err)
	}
	config.Applog.Infof("File uploaded to: %s\n", aws.StringValue(&result.Location))
	return fi.Size(), nil
}
