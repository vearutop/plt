package loadgen

import "time"

// Flags control load testing.
type Flags struct {
	Number       int
	Concurrency  int
	RateLimit    int
	Duration     time.Duration
	SlowResponse time.Duration
	LiveUI       bool
}
