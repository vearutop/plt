package main

import (
	"net/http"
	"strconv"

	"github.com/alecthomas/kingpin/v2"
	"github.com/vearutop/plt/curl"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/nethttp"
)

// cplt is a custom pocket load tester with URL customization to allow high request cardinality.

func main() {
	lf := loadgen.Flags{}
	lf.Register()

	// Custom command line flags.
	var cardinality, group int

	kingpin.Flag("cardinality", "Number of different urls to send.").Default("1000").IntVar(&cardinality)
	kingpin.Flag("group", "Number of sequential requests to group in single URL.").Default("10").IntVar(&group)

	curl.AddCommand(&lf, func(lf *loadgen.Flags, f *nethttp.Flags, j loadgen.JobProducer) {
		if nj, ok := j.(*nethttp.JobProducer); ok {
			nj.PrepareRequest = func(i int, req *http.Request) error {
				if req.URL.Path == "/hello" {
					k := i / group
					req.URL.RawQuery = "locale=en-US&name=user" + strconv.Itoa(k%cardinality)
				}

				return nil
			}
		}
	})

	kingpin.Parse()
}
