package loadgen

import "time"

type JobProducer interface {
	Job(i int) (time.Duration, error)
	RequestCounts() map[string]int
}

type JobWithOtherMetrics interface {
	Metrics() map[string]map[string]float64
}
