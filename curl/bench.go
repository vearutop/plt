package curl

import (
	"bytes"
	"crypto/tls"
	"expvar"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync"
	"time"

	"github.com/vearutop/dynhist-go"
)

func run(f flags) {
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

	tr := http.DefaultTransport.(*http.Transport)
	tr.MaxIdleConnsPerHost = 50

	concurrencyLimit := 50 // Number of simultaneous jobs.
	limiter := make(chan struct{}, concurrencyLimit)

	start := time.Now()
	slow := expvar.Int{}

	for i := 0; i < 1000; i++ {
		limiter <- struct{}{} // Reserve limiter slot.
		go func() {
			defer func() {
				<-limiter // Free limiter slot.
			}()

			start := time.Now()
			var dnsStart, connStart, tlsStart time.Time

			var body io.Reader
			if f.Data != "" {
				body = bytes.NewBufferString(f.Data)
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
				ms := time.Since(start).Seconds() * 1000
				if ms > 1000 {
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

		if time.Since(start) > 10*time.Second {
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
	println("Requests with latency more than 1000ms:", slow.Value())

	println("DNS latency distribution in ms:")
	println(dnsHist.String())
	println("TLS handshake latency distribution in ms:")
	println(tlsHist.String())

	println("Connection latency distribution in ms:")
	println(connHist.String())

	for code, cnt := range respCode {
		println(code, ":", cnt)
	}
	//for _, v := range requestHist.RawValues {
	//	fmt.Printf("%.2f,", v)
	//}
}
