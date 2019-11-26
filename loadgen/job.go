package loadgen

import "time"

type JobProducer interface {
	Job(i int) (time.Duration, error)
	Counts() map[string]int
}
