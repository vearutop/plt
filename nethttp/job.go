// Package nethttp implements HTTP load producer with net/http.
package nethttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
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
	"golang.org/x/net/http2"
)

// JobProducer sends HTTP requests.
type JobProducer struct {
	PrepareRequest      func(i int, req *http.Request) error
	PrepareRoundTripper func(tr http.RoundTripper) http.RoundTripper

	bytesWritten int64
	writeTime    int64
	bytesRead    int64
	readTime     int64
	total        int64
	respCode     [600]int64

	dnsHist  *dynhist.Collector
	connHist *dynhist.Collector
	tlsHist  *dynhist.Collector
	ttfbHist *dynhist.Collector

	upstreamHist        *dynhist.Collector
	upstreamHistPrecise *dynhist.Collector

	f  Flags
	lf loadgen.Flags

	tr http.RoundTripper

	mu         sync.Mutex
	respBody   map[int][]byte
	respHeader map[int]http.Header
	respProto  map[int]string

	log string
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

func (j *JobProducer) makeTransport() http.RoundTripper {
	var tr http.RoundTripper

	switch {
	case j.f.HTTP2:
		tr = j.makeTransport2()
	case j.f.HTTP3:
		tr = j.makeTransport3()
	default:
		tr = j.makeTransport1()
	}

	if j.PrepareRoundTripper != nil {
		tr = j.PrepareRoundTripper(tr)
	}

	return tr
}

func (j *JobProducer) makeTransport1() *http.Transport {
	t := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    true,
	}

	d := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
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

	concurrencyLimit := j.lf.Concurrency // Number of simultaneous jobs.
	if concurrencyLimit <= 0 {
		concurrencyLimit = 50
	}

	t.MaxIdleConnsPerHost = concurrencyLimit

	return t
}

func (j *JobProducer) makeTransport2() *http2.Transport {
	t := &http2.Transport{
		DisableCompression: true,
		AllowHTTP:          true,
	}

	t.DialTLSContext = func(_ context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
		c, err := tls.DialWithDialer(new(net.Dialer), network, addr, cfg)
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
func NewJobProducer(f Flags, lf loadgen.Flags, options ...func(lf *loadgen.Flags, f *Flags, j loadgen.JobProducer)) (*JobProducer, error) {
	u, err := url.Parse(f.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	addrs, err := net.LookupHost(u.Hostname())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve URL host: %w", err)
	}

	j := JobProducer{}
	j.f = f
	j.lf = lf

	j.log += fmt.Sprintln("Host resolved:", strings.Join(addrs, ","))

	j.dnsHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.connHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.tlsHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.ttfbHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.upstreamHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.upstreamHistPrecise = &dynhist.Collector{BucketsLimit: 100, WeightFunc: dynhist.LatencyWidth}
	j.respBody = make(map[int][]byte, 5)
	j.respHeader = make(map[int]http.Header, 5)
	j.respProto = make(map[int]string, 5)

	for _, o := range options {
		o(&lf, &f, &j)
	}

	j.tr = j.makeTransport()

	if _, ok := f.HeaderMap["User-Agent"]; !ok {
		f.HeaderMap["User-Agent"] = "plt"
	}

	return &j, nil
}

// String prints results.
func (j *JobProducer) String() string {
	j.mu.Lock()
	defer j.mu.Unlock()

	if len(j.respBody) == 0 {
		return ""
	}

	res := j.log

	bytesRead := atomic.LoadInt64(&j.bytesRead)
	readTime := atomic.LoadInt64(&j.readTime)
	dlSpeed := float64(bytesRead) / time.Duration(readTime).Seconds()

	bytesWritten := atomic.LoadInt64(&j.bytesWritten)
	writeTime := atomic.LoadInt64(&j.writeTime)
	ulSpeed := float64(bytesWritten) / time.Duration(writeTime).Seconds()

	if bytesRead > 0 && bytesWritten > 0 && atomic.LoadInt64(&j.total) > 0 {
		res += fmt.Sprintln("Bytes read", report.ByteSize(bytesRead), "total,",
			report.ByteSize(bytesRead/atomic.LoadInt64(&j.total)), "avg,", report.ByteSize(int64(dlSpeed))+"/s")
		res += fmt.Sprintln("Bytes written", report.ByteSize(bytesWritten), "total,",
			report.ByteSize(bytesWritten/atomic.LoadInt64(&j.total)), "avg,", report.ByteSize(int64(ulSpeed))+"/s")
		res += "\n"
	}

	if j.upstreamHist.Count > 0 {
		res += "Envoy upstream latency percentiles:\n"
		res += fmt.Sprintf("99%%: %.0fms\n", j.upstreamHistPrecise.Percentile(99))
		res += fmt.Sprintf("95%%: %.0fms\n", j.upstreamHistPrecise.Percentile(95))
		res += fmt.Sprintf("90%%: %.0fms\n", j.upstreamHistPrecise.Percentile(90))
		res += fmt.Sprintf("50%%: %.0fms\n\n", j.upstreamHistPrecise.Percentile(50))

		res += "Envoy upstream latency distribution in ms:\n"
		res += j.upstreamHist.String() + "\n"
	}

	if j.dnsHist.Count > 0 {
		res += "DNS latency distribution in ms:\n"
		res += j.dnsHist.String() + "\n"
	}

	if j.tlsHist.Count > 0 {
		res += "TLS handshake latency distribution in ms:\n"
		res += j.tlsHist.String() + "\n"
	}

	if j.ttfbHist.Count > 0 {
		res += "Time to first resp byte (TTFB) distribution in ms:\n"
		res += j.ttfbHist.String() + "\n"
	}

	if j.connHist.Count > 0 {
		res += "Connection latency distribution in ms:\n"
		res += j.connHist.String() + "\n"
	}

	res += "Responses by status code\n"

	codes := ""
	resps := ""

	for code := range j.respBody {
		codes += fmt.Sprintf("[%d] %d\n", code, atomic.LoadInt64(&j.respCode[code]))
		h := bytes.NewBuffer(nil)

		err := j.respHeader[code].Write(h)
		if err != nil {
			res += fmt.Sprintln("Failed to render headers:", err)
		}

		resps += fmt.Sprintf("[%s %d]\n%s\n%s\n", j.respProto[code], code, h.String(), string(j.respBody[code]))
	}

	res += codes + "\n"

	res += "Response samples (first by status code):\n"
	res += resps + "\n"

	return res
}

// SampleSize is maximum number of bytes to sample from response.
const SampleSize = 1000

// Job runs single item of load.
func (j *JobProducer) Job(i int) (time.Duration, error) {
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
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			j.dnsHist.Add(1000 * time.Since(dnsStart).Seconds())
		},

		ConnectStart: func(_, _ string) {
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
			dlStart = time.Now()

			atomic.AddInt64(&j.writeTime, int64(time.Since(start)))
		},

		GotFirstResponseByte: func() {
			dlStart = time.Now()

			j.ttfbHist.Add(1000 * time.Since(start).Seconds())
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	if j.PrepareRequest != nil {
		if err := j.PrepareRequest(i, req); err != nil {
			return 0, fmt.Errorf("failed to prepare request: %w", err)
		}
	}

	tr := j.tr
	// Keep alive flag here.
	if j.f.NoKeepalive {
		tr = j.makeTransport()
	}

	start = time.Now()
	dlStart = start

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
		if err != nil && err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, err
		}

		body = body[0:n]

		if resp.Header.Get("Content-Encoding") != "" {
			j.respBody[resp.StatusCode] = []byte("<" + resp.Header.Get("Content-Encoding") + "-encoded-content>")
		} else {
			j.respBody[resp.StatusCode] = report.PeekBody(body, SampleSize)
		}

		j.respHeader[resp.StatusCode] = resp.Header
		j.respProto[resp.StatusCode] = resp.Proto

		j.mu.Unlock()
	}

	if !j.f.IgnoreResponseBody {
		_, err = io.Copy(io.Discard, resp.Body)
		if err != nil {
			return 0, err
		}
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
	HeaderMap          map[string]string
	URL                string
	Body               string
	Method             string
	NoKeepalive        bool
	Compressed         bool
	Fast               bool
	IgnoreResponseBody bool
	HTTP2              bool
	HTTP3              bool
}
