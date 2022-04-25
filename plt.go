package main

import (
	"github.com/alecthomas/kingpin"
	"github.com/bool64/dev/version"
	"github.com/vearutop/plt/curl"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/s3"
)

func main() {
	kingpin.CommandLine.Help = "Pocket load tester pushes to the limit"

	kingpin.Version(version.Info().Version)

	lf := loadgen.Flags{}
	kingpin.Flag("number", "Number of requests to run, 0 is infinite.").
		PlaceHolder("1000").IntVar(&lf.Number)
	kingpin.Flag("concurrency", "Number of requests to run concurrently.").
		Default("50").IntVar(&lf.Concurrency)
	kingpin.Flag("rate-limit", "Rate limit, in requests per second, 0 disables limit (default).").
		Default("0").IntVar(&lf.RateLimit)
	kingpin.Flag("duration", "Max duration of load testing, 0 is infinite.").
		PlaceHolder("1m").DurationVar(&lf.Duration)
	kingpin.Flag("slow", "Min duration of slow response.").
		Default("1s").DurationVar(&lf.SlowResponse)
	kingpin.Flag("live-ui", "Show live ui with statistics.").BoolVar(&lf.LiveUI)

	curl.AddCommand(&lf)
	s3.AddCommand(&lf)

	kingpin.Parse()
}
