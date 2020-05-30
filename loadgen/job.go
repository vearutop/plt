package loadgen

import "time"

// JobProducer produces load items.
type JobProducer interface {
	Job(i int) (time.Duration, error)
	RequestCounts() map[string]int
}

// JobWithOtherMetrics exposes additional stats.
type JobWithOtherMetrics interface {
	Metrics() map[string]map[string]float64
}
