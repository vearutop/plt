package main

import (
	"github.com/alecthomas/kingpin"
	"github.com/vearutop/plt/curl"
	"github.com/vearutop/plt/loadgen"
)

func main() {
	kingpin.CommandLine.Help = "Pocket load tester pushes to the limit"

	lf := loadgen.Flags{}
	kingpin.Flag("num", "Number of requests to run, 0 is infinite.").
		Default("1000").IntVar(&lf.Number)
	kingpin.Flag("cnc", "Number of requests to run concurrently.").
		Default("50").IntVar(&lf.Concurrency)
	kingpin.Flag("rl", "Rate limit, in requests per second, 0 disables limit (default).").
		Default("0").IntVar(&lf.RateLimit)
	kingpin.Flag("dur", "Max duration of load testing, 0 is infinite.").
		Default("1m").DurationVar(&lf.Duration)
	kingpin.Flag("slow", "Min duration of slow response.").
		Default("1s").DurationVar(&lf.SlowResponse)

	curl.AddCommand(&lf)

	kingpin.Parse()
}
