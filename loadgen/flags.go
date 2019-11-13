package loadgen

import "time"

type Flags struct {
	Number       int
	Concurrency  int
	RateLimit    int
	Duration     time.Duration
	SlowResponse time.Duration
}
