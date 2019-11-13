package main

import (
	"github.com/alecthomas/kingpin"
	"github.com/vearutop/plt/curl"
)

func main() {
	kingpin.CommandLine.Help = "Pocket load tester pushes to the limit"

	curl.AddCommand()

	kingpin.Parse()
}
