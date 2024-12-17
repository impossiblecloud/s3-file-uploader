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

func getRealSourceFileName(config cfg.AppConfig, filename string) string {
	file := filepath.Base(filename)
	gzipFile := filepath.Join(config.GzipDir, file+".gz")
	encFile := filepath.Join(config.EncryptDir, file+".enc")
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
func FakeUploadFile(config cfg.AppConfig, filename string) error {
	realFile := getRealSourceFileName(config, filename)

	fi, err := os.Stat(realFile)
	if err != nil {
		config.Applog.Error(err)
		return err
	}

	config.Applog.Infof("FAKE UPLOAD TO S3: %q file, size %s", realFile, utils.HumanizeBytes(fi.Size(), false))

	return nil
}

// CopyFile is used for testing, it copies to /var/tmp
func CopyFile(config cfg.AppConfig, filename string) error {
	realFile := getRealSourceFileName(config, filename)

	fi, err := os.Stat(realFile)
	if err != nil {
		config.Applog.Error(err)
		return err
	}

	dstFile := filepath.Join("/var/tmp", filepath.Base(realFile))
	_, err = copy(realFile, dstFile)
	if err != nil {
		config.Applog.Error(err)
		return err
	}

	config.Applog.Infof("COPYING FILE: %q file, size %s, destination %q)",
		realFile, utils.HumanizeBytes(fi.Size(), false), dstFile)

	return nil
}

// UploadFile uploads a file to s3
func UploadFile(config cfg.AppConfig, filename string) error {
	realFile := getRealSourceFileName(config, filename)

	// The session the S3 Uploader will use
	sess := session.Must(session.NewSession())

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	f, err := os.Open(realFile)
	if err != nil {
		return fmt.Errorf("failed to open file %q, %v", realFile, err)
	}
	defer f.Close()

	// Upload the file to S3.
	key := fmt.Sprintf("%s/%s", config.S3path, filepath.Base(realFile))
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(config.S3bucket),
		Key:    aws.String(key),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file, %v", err)
	}
	config.Applog.Infof("File uploaded to: %s\n", aws.StringValue(&result.Location))
	return nil
}
