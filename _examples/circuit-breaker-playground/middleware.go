package main

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/sony/gobreaker"
)

// CircuitBreakerMiddleware is a http.RoundTripper.
func CircuitBreakerMiddleware(
	settings gobreaker.Settings,
	cbFailed *int64,
) func(tripper http.RoundTripper) http.RoundTripper {
	cb := gobreaker.NewTwoStepCircuitBreaker(settings)
	return func(tripper http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			done, err := cb.Allow()
			// Error means circuit breaker is open and request should fail immediately.
			if err != nil {
				atomic.AddInt64(cbFailed, 1)

				return nil, fmt.Errorf("circuit breaker %s: %w", settings.Name, err)
			}
			resp, err := tripper.RoundTrip(r)

			// Done is nil if err is not.
			if done != nil {
				done(err == nil)
			}

			return resp, err
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
