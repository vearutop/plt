// Package main provides pocket load tester application.
package main

import (
	"github.com/alecthomas/kingpin/v2"
	"github.com/vearutop/plt/curl"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/s3"
)

func main() {
	lf := loadgen.Flags{}
	lf.Register()

	curl.AddCommand(&lf)
	s3.AddCommand(&lf)

	kingpin.Parse()
}
