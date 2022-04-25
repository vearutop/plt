package s3

import (
	"fmt"
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

	dl := s3manager.NewDownloader(sess)

	return &jobProducer{
		f:     f,
		dl:    dl,
		start: time.Now(),
	}, nil
}

type jobProducer struct {
	f  Flags
	dl *s3manager.Downloader

	totBytes int64
	tot      int64
	start    time.Time
}

type nopWriterAt struct{}

func (nopWriterAt) WriteAt(p []byte, off int64) (n int, err error) {
	return len(p), nil
}

func (j *jobProducer) Job(_ int) (time.Duration, error) {
	start := time.Now()

	n, err := j.dl.Download(nopWriterAt{}, &s3.GetObjectInput{
		Bucket: aws.String(j.f.Bucket),
		Key:    aws.String(j.f.Key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to download file: %w", err)
	}

	atomic.AddInt64(&j.totBytes, n)
	atomic.AddInt64(&j.tot, 1)

	return time.Since(start), nil
}

func (j *jobProducer) RequestCounts() map[string]int {
	return nil
}

// Print prints additional stats.
func (j *jobProducer) Print() {
	elapsed := time.Since(j.start).Seconds()

	fmt.Println()
	fmt.Printf("Read: total %.2f MB, avg %.2f MB, %.2f MB/s\n",
		float64(j.totBytes)/(1024*1024),
		float64(j.totBytes)/float64(1024*1024*j.tot),
		float64(j.totBytes)/(1024*1024*elapsed))
}
