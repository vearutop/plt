// Package redis implements redis-cli load tester command.
package redis

import (
	"github.com/alecthomas/kingpin"
	"github.com/vearutop/plt/loadgen"
)

// Flags describes redis-cli command parameters.
type Flags struct {
	Host     string
	Port     int
	Socket   string
	Password string
	URI      string
	DBNumber int
	RESP2    bool
	RESP3    bool
}

// AddCommand registers curl command into CLI app.
func AddCommand(lf *loadgen.Flags) {
	var f Flags

	s3 := kingpin.Command("s3", "S3 transfer")
	s3.Flag("access-key", "Access key/id (env AWS_ACCESS_KEY).").
		Envar("AWS_ACCESS_KEY").StringVar(&f.AccessKey)
	s3.Flag("secret-key", "Secret key (env AWS_SECRET_KEY).").
		Envar("AWS_SECRET_KEY").StringVar(&f.SecretKey)
	s3.Flag("session-token", "Session token (env AWS_SESSION_TOKEN).").
		Envar("AWS_SESSION_TOKEN").StringVar(&f.SessionToken)

	s3.Flag("region", "Region.").Default("eu-central-1").StringVar(&f.Region)
	s3.Flag("url", "Optional S3 URL (if not AWS).").StringVar(&f.URL)
	s3.Flag("bucket", "Bucket name.").Required().StringVar(&f.Bucket)
	s3.Flag("key", "Entry key.").Required().StringVar(&f.Key)
	s3.Flag("path-style", "To use path-style addressing, i.e., `http://s3.amazonaws.com/BUCKET/KEY`.").
		BoolVar(&f.PathStyle)

	s3.Flag("save", "Path to local file to save the entry.").StringVar(&f.Save)
	s3.Flag("upload", "Path to local file to upload to S3, enables upload load testing.").StringVar(&f.Upload)

	s3.Action(func(kp *kingpin.ParseContext) error {
		return run(*lf, f)
	})
}

func run(lf loadgen.Flags, f Flags) error {
	lf.Prepare()

	j, err := newJobProducer(f)
	if err != nil {
		return err
	}

	return loadgen.Run(lf, j)
}
