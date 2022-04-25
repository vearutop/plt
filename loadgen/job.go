package loadgen

import "time"

// JobProducer produces load items.
type JobProducer interface {
	Job(i int) (time.Duration, error)
	RequestCounts() map[string]int
}
