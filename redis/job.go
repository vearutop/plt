package redis

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

func newJobProducer(f Flags) (*jobProducer, error) {
	if f.URL != "" && !strings.HasPrefix(f.URL, "http://") && !strings.HasPrefix(f.URL, "https://") {
		f.URL = "http://" + f.URL
	}

	if f.Region == "" {
		f.Region = "eu-central-1"
	}

	creds := credentials.NewStaticCredentials(
		f.AccessKey,
		f.SecretKey,
		f.SessionToken,
	)

	cfg := &aws.Config{
		Credentials: creds,
		Region:      aws.String(f.Region),
	}

	if f.URL != "" {
		cfg.Endpoint = &f.URL
	}

	if f.PathStyle {
		cfg.S3ForcePathStyle = &f.PathStyle
	}

	sess, err := session.NewSession(cfg)
	if err != nil {
		return nil, err
	}

	return &jobProducer{
		f:  f,
		dl: s3manager.NewDownloader(sess),
		ul: s3manager.NewUploader(sess),

		start: time.Now(),
	}, nil
}

type jobProducer struct {
	f  Flags
	dl *s3manager.Downloader
	ul *s3manager.Uploader

	totBytes int64
	tot      int64
	start    time.Time
}

type nopWriterAt struct{}

func (nopWriterAt) WriteAt(p []byte, off int64) (n int, err error) {
	return len(p), nil
}

func (j *jobProducer) Job(i int) (time.Duration, error) {
	var (
		start = time.Now()
		err   error
	)

	if j.f.Upload != "" {
		err = j.upload(i)
	} else {
		err = j.download(i)
	}

	if err != nil {
		return 0, err
	}

	atomic.AddInt64(&j.tot, 1)

	return time.Since(start), nil
}

func (j *jobProducer) download(i int) error {
	w := io.WriterAt(nopWriterAt{})

	if i == 0 && j.f.Save != "" {
		f, err := os.Create(j.f.Save)
		if err != nil {
			return fmt.Errorf("failed to create file to save S3 result: %w", err)
		}

		w = f

		defer func() {
			if err := f.Close(); err != nil {
				println("failed to close file:", err)
			}
		}()
	}

	n, err := j.dl.Download(w, &s3.GetObjectInput{
		Bucket: aws.String(j.f.Bucket),
		Key:    aws.String(j.f.Key),
	})
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	atomic.AddInt64(&j.totBytes, n)

	return nil
}

func (j *jobProducer) upload(_ int) error {
	f, err := os.Open(j.f.Upload)
	if err != nil {
		return fmt.Errorf("failed to open file to upload: %w", err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			println("failed to close file:", err)
		}
	}()

	_, err = j.ul.Upload(&s3manager.UploadInput{
		Bucket: aws.String(j.f.Bucket),
		Key:    aws.String(j.f.Key),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file to S3: %w", err)
	}

	return nil
}

func (j *jobProducer) RequestCounts() map[string]int {
	return nil
}

// Print prints additional stats.
func (j *jobProducer) String() string {
	if j.totBytes == 0 {
		return ""
	}

	elapsed := time.Since(j.start).Seconds()

	return fmt.Sprintf("\nRead: total %.2f MB, avg %.2f MB, %.2f MB/s\n",
		float64(j.totBytes)/(1024*1024),
		float64(j.totBytes)/float64(1024*1024*j.tot),
		float64(j.totBytes)/(1024*1024*elapsed))
}
