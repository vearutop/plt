package main

import (
	"time"

	"github.com/sony/gobreaker"
)

func simpleSettings() (gobreaker.Settings, func() string) {
	cbSettings := gobreaker.Settings{
		// Name is the name of the CircuitBreaker.
		Name: "acme",

		// MaxRequests is the maximum number of requests allowed to pass through
		// when the CircuitBreaker is half-open.
		// If MaxRequests is 0, the CircuitBreaker allows only 1 request.
		MaxRequests: 500,

		// Interval is the cyclic period of the closed state
		// for the CircuitBreaker to clear the internal Counts.
		// If Interval is less than or equal to 0, the CircuitBreaker doesn't clear internal Counts during the closed state.
		Interval: 2000 * time.Millisecond,

		// Timeout is the period of the open state,
		// after which the state of the CircuitBreaker becomes half-open.
		// If Timeout is less than or equal to 0, the timeout value of the CircuitBreaker is set to 60 seconds.
		Timeout: 3 * time.Second,
	}

	return cbSettings, func() string {
		return ""
	}
}
