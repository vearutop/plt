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

// Prepare sets conditional defaults.
func (lf *Flags) Prepare() {
	if lf.Number == 0 && lf.Duration == 0 {
		lf.Number = 1000
		lf.Duration = time.Minute
	}
}
