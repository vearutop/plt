// +build go1.12

package main

import (
	"runtime/debug"
)

//nolint:gochecknoinits
func init() {
	if info, available := debug.ReadBuildInfo(); available {
		if info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
}
