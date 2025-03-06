package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

func delayedSettings() (gobreaker.Settings, func() string) {
	var (
		mu                 sync.Mutex
		preOpenStart       time.Time
		readyToTripMessage string
		readyToTripCalled  = 0
	)

	requestsThreshold := uint32(100)
	errorThreshold := 0.01
	delayedOpenDuration := time.Second * 10

	circuitBreakerSettings := gobreaker.Settings{
		Name: "test",

		// MaxRequests is the maximum number of requests allowed to pass through
		// when the CircuitBreaker is half-open.
		// If MaxRequests is 0, the CircuitBreaker allows only 1 request.
		MaxRequests: 1000,

		// Interval is the cyclic period of the closed state
		// for the CircuitBreaker to clear the internal Counts.
		// If Interval is less than or equal to 0, the CircuitBreaker doesn't clear internal Counts during the closed state.
		Interval: 20 * time.Second,

		// Timeout is the period of the open state,
		// after which the state of the CircuitBreaker becomes half-open.
		// If Timeout is less than or equal to 0, the timeout value of the CircuitBreaker is set to 60 seconds.
		Timeout: 10 * time.Second,
	}

	// if this function returns true, then we will go to the open state
	circuitBreakerSettings.ReadyToTrip = func(counts gobreaker.Counts) bool {
		mu.Lock()
		defer mu.Unlock()

		enoughRequests := counts.Requests > requestsThreshold
		er := float64(counts.TotalFailures) / float64(counts.Requests)
		reachedFailureLvl := er >= errorThreshold

		readyToTripCalled++
		readyToTripMessage = fmt.Sprintf("%d. %d/%d er:%.2f%% %v %v, ", readyToTripCalled,
			counts.TotalFailures, counts.Requests, er, enoughRequests, reachedFailureLvl)

		if enoughRequests && reachedFailureLvl {
			if preOpenStart.IsZero() {
				readyToTripMessage += "start delay, "

				preOpenStart = time.Now()
			}
			if time.Since(preOpenStart) >= delayedOpenDuration {
				readyToTripMessage += "opening"
				return true
			}

			readyToTripMessage += fmt.Sprintf("keep closed (%s)", time.Since(preOpenStart).Truncate(100*time.Millisecond).String())

			return false
		} else {
			// Reset the timer if conditions have improved.
			if !preOpenStart.IsZero() {
				readyToTripMessage += "drop delay, "
				preOpenStart = time.Time{}
			}

			readyToTripMessage += "keep closed"

			return false
		}
	}

	return circuitBreakerSettings, func() string {
		mu.Lock()
		defer mu.Unlock()

		return readyToTripMessage
	}
}

// timeoutCB is a http.RoundTripper.
// timeoutCB will wrap transport with additional circuit breaker steps.
// If state is opened - will return auto rejected error
func timeoutCB(
	settings gobreaker.Settings,
	isDryRun bool,
) func(tripper http.RoundTripper) http.RoundTripper {
	cb := gobreaker.NewTwoStepCircuitBreaker(settings)
	return func(tripper http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			done, err := cb.Allow()
			// if we are receiving an error - it's mean that CB is open and we are generating auto-rejecting response.
			if err != nil {
				if !isDryRun {
					return nil, fmt.Errorf("circuit breaker %s: %w", settings.Name, err)
				}
			}
			resp, err := tripper.RoundTrip(r)
			// checking for nil due to the dry run
			if done != nil {
				// We only care about timeouts errors
				// if we receive code 5xx instead of timeout - it's still good for us
				done(err == nil)
			}
			return resp, err
		})
	}
}
