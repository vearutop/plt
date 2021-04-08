// Package nethttp implements HTTP load producer with net/http.
package nethttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vearutop/dynhist-go"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/report"
)

// JobProducer sends HTTP requests.
type JobProducer struct {
	start time.Time

	dnsHist  *dynhist.Collector
	connHist *dynhist.Collector
	tlsHist  *dynhist.Collector
	ttfbHist *dynhist.Collector

	upstreamHist        *dynhist.Collector
	upstreamHistPrecise *dynhist.Collector

	respCode     [600]int64
	bytesWritten int64
	writeTime    int64
	bytesRead    int64
	readTime     int64
	total        int64

	f Flags

	tr *http.Transport

	mu         sync.Mutex
	respBody   map[int][]byte
	respHeader map[int]http.Header
}

// RequestCounts returns distribution by status code.
func (j *JobProducer) RequestCounts() map[string]int {
	j.mu.Lock()
	defer j.mu.Unlock()

	res := make(map[string]int, len(j.respBody))
	for code := range j.respBody {
		res[strconv.Itoa(code)] = int(atomic.LoadInt64(&j.respCode[code]))
	}

	return res
}

// Metrics return additional stats.
func (j *JobProducer) Metrics() map[string]map[string]float64 {
	j.mu.Lock()
	defer j.mu.Unlock()

	elapsed := time.Since(j.start).Seconds()

	res := map[string]map[string]float64{
		"Bandwidth, MB/s": {
			"Read":  float64(j.bytesRead) / (1024 * 1024 * elapsed),
			"Write": float64(j.bytesWritten) / (1024 * 1024 * elapsed),
		},
	}

	return res
}

type countingConn struct {
	j *JobProducer
	net.Conn
}

// Read reads data from the connection.
// Read can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (c countingConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	atomic.AddInt64(&c.j.bytesRead, int64(n))

	return n, err
}

// Write writes data to the connection.
// Write can be made to time out and return an Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (c countingConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	atomic.AddInt64(&c.j.bytesWritten, int64(n))

	return n, err
}

func (j *JobProducer) makeTransport() *http.Transport {
	d := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	t := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}

	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := d.DialContext(ctx, network, addr)
		if err != nil {
			return c, err
		}

		return countingConn{
			j:    j,
			Conn: c,
		}, nil
	}

	return t
}

// NewJobProducer creates HTTP load generator.
func NewJobProducer(f Flags, lf loadgen.Flags) *JobProducer {
	u, err := url.Parse(f.URL)
	if err != nil {
		log.Fatalf("failed to parse URL: %s", err)
	}

	addrs, err := net.LookupHost(u.Hostname())
	if err != nil {
		log.Fatalf("failed to resolve URL host: %s", err)
	}

	fmt.Println("Host resolved:", strings.Join(addrs, ","))

	j := JobProducer{}

	concurrencyLimit := lf.Concurrency // Number of simultaneous jobs.
	if concurrencyLimit <= 0 {
		concurrencyLimit = 50
	}

	j.tr = j.makeTransport()
	j.tr.MaxIdleConnsPerHost = concurrencyLimit

	j.dnsHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.connHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.tlsHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.ttfbHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.upstreamHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.upstreamHistPrecise = &dynhist.Collector{BucketsLimit: 100, WeightFunc: dynhist.LatencyWidth}
	j.respBody = make(map[int][]byte, 5)
	j.respHeader = make(map[int]http.Header, 5)
	j.f = f

	if _, ok := f.HeaderMap["User-Agent"]; !ok {
		f.HeaderMap["User-Agent"] = "plt"
	}

	return &j
}

// Print prints results.
func (j *JobProducer) Print() {
	fmt.Println()

	bytesRead := atomic.LoadInt64(&j.bytesRead)
	readTime := atomic.LoadInt64(&j.readTime)
	dlSpeed := float64(bytesRead) / time.Duration(readTime).Seconds()

	bytesWritten := atomic.LoadInt64(&j.bytesWritten)
	writeTime := atomic.LoadInt64(&j.writeTime)
	ulSpeed := float64(bytesWritten) / time.Duration(writeTime).Seconds()

	fmt.Println("Bytes read", report.ByteSize(bytesRead), "total,",
		report.ByteSize(bytesRead/atomic.LoadInt64(&j.total)), "avg,", report.ByteSize(int64(dlSpeed))+"/s")
	fmt.Println("Bytes written", report.ByteSize(bytesWritten), "total,",
		report.ByteSize(bytesWritten/atomic.LoadInt64(&j.total)), "avg,", report.ByteSize(int64(ulSpeed))+"/s")
	fmt.Println()

	if j.upstreamHist.Count > 0 {
		fmt.Println("Envoy upstream latency percentiles:")
		fmt.Printf("99%%: %.0fms\n", j.upstreamHistPrecise.Percentile(99))
		fmt.Printf("95%%: %.0fms\n", j.upstreamHistPrecise.Percentile(95))
		fmt.Printf("90%%: %.0fms\n", j.upstreamHistPrecise.Percentile(90))
		fmt.Printf("50%%: %.0fms\n\n", j.upstreamHistPrecise.Percentile(50))

		fmt.Println("Envoy upstream latency distribution in ms:")
		fmt.Println(j.upstreamHist.String())
	}

	if j.dnsHist.Count > 0 {
		fmt.Println("DNS latency distribution in ms:")
		fmt.Println(j.dnsHist.String())
	}

	if j.tlsHist.Count > 0 {
		fmt.Println("TLS handshake latency distribution in ms:")
		fmt.Println(j.tlsHist.String())
	}

	if j.ttfbHist.Count > 0 {
		fmt.Println("Time to first resp byte (TTFB) distribution in ms:")
		fmt.Println(j.ttfbHist.String())
	}

	fmt.Println("Connection latency distribution in ms:")
	fmt.Println(j.connHist.String())

	fmt.Println("Responses by status code")
	j.mu.Lock()
	codes := ""
	resps := ""

	for code := range j.respBody {
		codes += fmt.Sprintf("[%d] %d\n", code, atomic.LoadInt64(&j.respCode[code]))
		h := bytes.NewBuffer(nil)

		err := j.respHeader[code].Write(h)
		if err != nil {
			fmt.Println("Failed to render headers:", err)
		}

		resps += fmt.Sprintf("[%d]\n%s\n%s\n", code, h.String(), string(j.respBody[code]))
	}
	j.mu.Unlock()
	fmt.Println(codes)

	fmt.Println("Response samples (first by status code):")
	fmt.Println(resps)
}

// SampleSize is maximum number of bytes to sample from response.
const SampleSize = 1000

// Job runs single item of load.
func (j *JobProducer) Job(_ int) (time.Duration, error) {
	var start, dnsStart, connStart, tlsStart, dlStart time.Time

	var body io.Reader
	if j.f.Body != "" {
		body = bytes.NewBufferString(j.f.Body)
	}

	req, err := http.NewRequest(j.f.Method, j.f.URL, body)
	if err != nil {
		return 0, err
	}

	for k, v := range j.f.HeaderMap {
		req.Header.Set(k, v)
	}

	trace := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			j.dnsHist.Add(1000 * time.Since(dnsStart).Seconds())
		},

		ConnectStart: func(network, addr string) {
			connStart = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			j.connHist.Add(1000 * time.Since(connStart).Seconds())
		},

		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			j.tlsHist.Add(1000 * time.Since(tlsStart).Seconds())
		},

		WroteRequest: func(_ httptrace.WroteRequestInfo) {
			atomic.AddInt64(&j.writeTime, int64(time.Since(start)))
		},

		GotFirstResponseByte: func() {
			dlStart = time.Now()
			j.ttfbHist.Add(1000 * time.Since(start).Seconds())
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	tr := j.tr
	// Keep alive flag here.
	if j.f.NoKeepalive {
		tr = j.makeTransport()
	}

	start = time.Now()

	resp, err := tr.RoundTrip(req)
	if err != nil {
		return 0, err
	}

	if envoyUpstreamMS := resp.Header.Get("X-Envoy-Upstream-Service-Time"); envoyUpstreamMS != "" {
		ms, err := strconv.Atoi(envoyUpstreamMS)
		if err == nil {
			j.upstreamHist.Add(float64(ms))
			j.upstreamHistPrecise.Add(float64(ms))
		}
	}

	cnt := atomic.AddInt64(&j.respCode[resp.StatusCode], 1)

	if cnt == 1 {
		j.mu.Lock()

		// Read a few bytes of response to save as sample.
		body := make([]byte, SampleSize+1)

		n, err := io.ReadAtLeast(resp.Body, body, SampleSize+1)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return 0, err
		}

		body = body[0:n]

		if resp.Header.Get("Content-Encoding") != "" {
			j.respBody[resp.StatusCode] = []byte("<" + resp.Header.Get("Content-Encoding") + "-encoded-content>")
		} else {
			j.respBody[resp.StatusCode] = report.PeekBody(body, SampleSize)
		}

		j.respHeader[resp.StatusCode] = resp.Header

		j.mu.Unlock()
	}

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return 0, err
	}

	err = resp.Body.Close()
	if err != nil {
		return 0, err
	}

	done := time.Now()

	atomic.AddInt64(&j.readTime, int64(done.Sub(dlStart)))
	si := done.Sub(start)

	atomic.AddInt64(&j.total, 1)

	return si, nil
}

// Flags control HTTP load setup.
type Flags struct {
	HeaderMap   map[string]string
	URL         string
	Body        string
	Method      string
	NoKeepalive bool
	Compressed  bool
	Fast        bool
}
