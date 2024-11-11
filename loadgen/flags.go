package loadgen

import (
	"io"
	"time"

	"github.com/gizak/termui/v3/widgets"
)

// Flags control load testing.
type Flags struct {
	Number       int
	Concurrency  int
	RateLimit    int
	Duration     time.Duration
	SlowResponse time.Duration
	LiveUI       bool

	//nolint:godox // Soon to be implemented.
	// Automated stress testing flags.
	// TODO implement support.
	// TargetLatency    time.Duration // When this latency is exceeded, request rate is reduced.
	// TargetPercentile float64       // Percentile value, e.g. 99.9 to check target latency against.
	// TargetErrorRate  float64       // When this percentage of errors is exceeded, request rate is reduced.
	// Step             time.Duration // Time between request rate increments.
	// Increment        float64       // Percentage of request rate increment on each step, e.g. 5.5 for 5.5%.

	Output io.Writer

	KeyPressed              map[string]func()
	PrepareLoadLimitsWidget func(paragraph *widgets.Paragraph)
}

// Prepare sets conditional defaults.
func (lf *Flags) Prepare() {
	if lf.Number == 0 && lf.Duration == 0 {
		lf.Number = 1000
		lf.Duration = time.Minute
	}
}
