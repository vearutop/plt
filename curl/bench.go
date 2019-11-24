package curl

import (
	"bytes"
	"context"
	"crypto/tls"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vearutop/dynhist-go"
	"github.com/vearutop/plt/loadgen"
	"golang.org/x/time/rate"
)

func run(lf *loadgen.Flags, f flags) {
	u, err := url.Parse(f.URL)
	if err != nil {
		log.Fatalf("failed to parse URL %q: %s", f.URL, err)
	}
	if _, err := net.LookupHost(u.Hostname()); err != nil {
		log.Fatalf("failed to resolve URL host: %s", err)
	}

	requestHist := dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth, RawValues: []float64{}}
	dnsHist := dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	connHist := dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	tlsHist := dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}

	mu := sync.Mutex{}
	respCode := map[int]int{}

	concurrencyLimit := lf.Concurrency // Number of simultaneous jobs.
	if concurrencyLimit <= 0 {
		concurrencyLimit = 50
	}

	tr := http.DefaultTransport.(*http.Transport)
	tr.MaxIdleConnsPerHost = concurrencyLimit

	limiter := make(chan struct{}, concurrencyLimit)

	start := time.Now()
	slow := expvar.Int{}

	n := lf.Number
	if n == 0 {
		n = math.MaxInt64
	}
	dur := lf.Duration
	if dur == 0 {
		dur = 1000 * time.Hour
	}

	var rl *rate.Limiter
	if lf.RateLimit > 0 {
		rl = rate.NewLimiter(rate.Limit(lf.RateLimit), concurrencyLimit)
	}

	exit := make(chan os.Signal)
	signal.Notify(exit, syscall.SIGTERM, os.Interrupt)
	done := int32(0)
	go func() {
		<-exit
		atomic.StoreInt32(&done, 1)
	}()

	println("Starting")
	for i := 0; i < n; i++ {
		if rl != nil {
			err = rl.Wait(context.Background())
			if err != nil {
				println(err.Error())
			}
		}
		limiter <- struct{}{} // Reserve limiter slot.
		go func() {
			defer func() {
				<-limiter // Free limiter slot.
			}()

			start := time.Now()
			var dnsStart, connStart, tlsStart time.Time

			var body io.Reader
			if f.Body != "" {
				body = bytes.NewBufferString(f.Body)
			}
			req, _ := http.NewRequest(f.Method, f.URL, body)
			for k, v := range f.HeaderMap {
				req.Header.Set(k, v)
			}
			trace := &httptrace.ClientTrace{
				DNSStart: func(info httptrace.DNSStartInfo) {
					dnsStart = time.Now()
				},
				DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
					dnsHist.Add(1000 * time.Since(dnsStart).Seconds())
				},

				ConnectStart: func(network, addr string) {
					connStart = time.Now()
				},
				ConnectDone: func(network, addr string, err error) {
					connHist.Add(1000 * time.Since(connStart).Seconds())
				},

				TLSHandshakeStart: func() {
					tlsStart = time.Now()
				},
				TLSHandshakeDone: func(tls.ConnectionState, error) {
					tlsHist.Add(1000 * time.Since(tlsStart).Seconds())
				},
			}
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

			// Keep alive flag here.
			if f.NoKeepalive {
				tr = &http.Transport{}
			}

			resp, err := tr.RoundTrip(req)
			if err != nil {
				println(err.Error())
			} else {
				si := time.Since(start)
				ms := si.Seconds() * 1000
				if si >= lf.SlowResponse {
					slow.Add(1)
				}
				requestHist.Add(ms)

				_, err = io.Copy(ioutil.Discard, resp.Body)
				if err != nil {
					println(err.Error())
				}
				err = resp.Body.Close()
				if err != nil {
					println(err.Error())
				}
				mu.Lock()
				respCode[resp.StatusCode]++
				mu.Unlock()
			}
		}()

		if time.Since(start) > dur || atomic.LoadInt32(&done) == 1 {
			break
		}
	}

	// Wait for goroutines to finish by filling full channel.
	for i := 0; i < cap(limiter); i++ {
		limiter <- struct{}{}
	}

	println("Requests per second:", fmt.Sprintf("%.2f", float64(requestHist.Count)/time.Since(start).Seconds()))
	println("Total requests:", requestHist.Count)
	println("Request latency distribution in ms:")
	println(requestHist.String())
	println("Requests with latency more than "+lf.SlowResponse.String()+":", slow.Value())

	println("DNS latency distribution in ms:")
	println(dnsHist.String())
	println("TLS handshake latency distribution in ms:")
	println(tlsHist.String())

	println("Connection latency distribution in ms:")
	println(connHist.String())

	println("Responses by status code")
	for code, cnt := range respCode {
		println("[", code, "]", ":", cnt)
	}
}
