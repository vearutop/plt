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

var (
	// Response time considered a timeout by HTTP client.
	requestTimeout = 500 * time.Millisecond

	// Response time window, normally distributed.
	minResp = int64(300 * time.Millisecond)
	maxResp = int64(510 * time.Millisecond)
)

func main() {
	lf := loadgen.Flags{}
	lf.Register()

	var (
		// Atomic counters.
		cbFailed int64
		cbPassed int64
		cbState  int64

		mu sync.Mutex
	)

	cbSettings, msg := delayedSettings()
	//cbSettings, msg := simpleSettings()

	cbSettings.OnStateChange = func(name string, from gobreaker.State, to gobreaker.State) {
		atomic.StoreInt64(&cbState, int64(to))
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
			msg(),
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
				return timeoutCB(cbSettings, false)(
					roundTripperFunc(func(r *http.Request) (*http.Response, error) {
						ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
