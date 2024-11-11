package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gizak/termui/v3/widgets"
	"github.com/sony/gobreaker"
	"github.com/vearutop/plt/curl"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/nethttp"
)

func main() {
	lf := loadgen.Flags{}
	lf.Register()

	var (
		// Response time window, normally distributed.
		minResp = int64(300 * time.Millisecond)
		maxResp = int64(510 * time.Millisecond)

		// Atomic counters.
		cbFailed int64
		cbPassed int64
		cbState  int64

		// Response time considered a timeout by HTTP client.
		timeout = 500 * time.Millisecond

		// ReadyToTrip params.
		requestsThreshold = uint32(100)
		errorThreshold    = 0.03

		mu             sync.Mutex
		readyToTripMsg string
	)

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

		// OnStateChange is called whenever the state of the CircuitBreaker changes.
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			atomic.StoreInt64(&cbState, int64(to))
		},

		// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state.
		// If ReadyToTrip returns true, the CircuitBreaker will be placed into the open state.
		// If ReadyToTrip is nil, default ReadyToTrip is used.
		// Default ReadyToTrip returns true when the number of consecutive failures is more than 5.
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			enoughRequests := counts.Requests > requestsThreshold
			errorRate := float64(counts.TotalFailures) / float64(counts.Requests)
			reachedFailureLvl := errorRate >= errorThreshold

			mu.Lock()
			defer mu.Unlock()

			readyToTripMsg = fmt.Sprintf("%d/%d failed (%.2f%%), %s",
				counts.TotalFailures, counts.Requests, 100*errorRate, time.Now().Format(time.TimeOnly))
			return enoughRequests && reachedFailureLvl
		},
	}

	// This handler returns 200 OK after a random delay between minResp and maxResp.
	h := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		r := rand.Float64()
		t := float64(atomic.LoadInt64(&minResp)) + float64(atomic.LoadInt64(&maxResp)-atomic.LoadInt64(&minResp))*r

		time.Sleep(time.Duration(t))
	})

	srv := httptest.NewServer(h)

	// Customizing Live UI.
	lf.PrepareLoadLimitsWidget = func(paragraph *widgets.Paragraph) {
		mu.Lock()
		defer mu.Unlock()

		paragraph.Title = "Response Time"
		paragraph.Text = fmt.Sprintf("Max resp: %s, <Up>/<Down>: ±5%%", time.Duration(atomic.LoadInt64(&maxResp)).Truncate(time.Millisecond).String())
		paragraph.Text += fmt.Sprintf("\nCB %s, f: %d, p: %d\nReady to trip: %s",
			gobreaker.State(atomic.LoadInt64(&cbState)).String(),
			atomic.LoadInt64(&cbFailed),
			atomic.LoadInt64(&cbPassed),
			readyToTripMsg,
		)
	}

	lf.KeyPressed["<Up>"] = func() {
		atomic.StoreInt64(&maxResp, int64(float64(atomic.LoadInt64(&maxResp))*1.05))
	}

	lf.KeyPressed["<Down>"] = func() {
		atomic.StoreInt64(&maxResp, int64(float64(atomic.LoadInt64(&maxResp))*0.95))
	}

	// Applying transport middleware.
	curl.AddCommand(&lf, func(lf *loadgen.Flags, f *nethttp.Flags, j loadgen.JobProducer) {
		if nj, ok := j.(*nethttp.JobProducer); ok {
			nj.PrepareRoundTripper = func(tr http.RoundTripper) http.RoundTripper {
				return CircuitBreakerMiddleware(cbSettings, &cbFailed)(
					roundTripperFunc(func(r *http.Request) (*http.Response, error) {
						ctx, cancel := context.WithTimeout(r.Context(), timeout)
						defer cancel()

						atomic.AddInt64(&cbPassed, 1)

						return tr.RoundTrip(r.WithContext(ctx))
					}),
				)
			}
		}
	})

	// Preparing command line arguments.
	os.Args = append(os.Args,
		"--live-ui",
		"--rate-limit=100",
		"--number=1000000",
		"curl", srv.URL)

	// Running the app.q
	kingpin.Parse()
}
