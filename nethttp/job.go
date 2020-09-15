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
)

// JobProducer sends HTTP requests.
type JobProducer struct {
	start time.Time

	dnsHist  *dynhist.Collector
	connHist *dynhist.Collector
	tlsHist  *dynhist.Collector

	upstreamHist        *dynhist.Collector
	upstreamHistPrecise *dynhist.Collector

	mu       sync.Mutex
	respCode map[int]int
	respBody map[int][]byte

	bytesWritten int64
	bytesRead    int64

	f Flags

	tr *http.Transport
}

// RequestCounts returns distribution by status code.
func (j *JobProducer) RequestCounts() map[string]int {
	j.mu.Lock()
	defer j.mu.Unlock()

	res := make(map[string]int, len(j.respCode))
	for code, cnt := range j.respCode {
		res[strconv.Itoa(code)] = cnt
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
	j.upstreamHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.upstreamHistPrecise = &dynhist.Collector{BucketsLimit: 100, WeightFunc: dynhist.LatencyWidth}
	j.respCode = make(map[int]int, 5)
	j.respBody = make(map[int][]byte, 5)
	j.f = f

	if _, ok := f.HeaderMap["User-Agent"]; !ok {
		f.HeaderMap["User-Agent"] = "plt"
	}

	return &j
}

// Print prints results.
func (j *JobProducer) Print() {
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

	fmt.Println("Connection latency distribution in ms:")
	fmt.Println(j.connHist.String())

	fmt.Println("Responses by status code")
	j.mu.Lock()
	codes := ""
	resps := ""

	for code, cnt := range j.respCode {
		codes += fmt.Sprintf("[%d] %d\n", code, cnt)
		resps += fmt.Sprintf("[%d]\n%s\n", code, string(j.respBody[code]))
	}
	j.mu.Unlock()
	fmt.Println(codes)

	fmt.Println("Bytes read", atomic.LoadInt64(&j.bytesRead))
	fmt.Println("Bytes written", atomic.LoadInt64(&j.bytesWritten))

	fmt.Println(resps)
}

// Job runs single item of load.
func (j *JobProducer) Job(_ int) (time.Duration, error) {
	start := time.Now()

	var dnsStart, connStart, tlsStart time.Time

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
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	tr := j.tr
	// Keep alive flag here.
	if j.f.NoKeepalive {
		tr = j.makeTransport()
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		return 0, err
	}

	si := time.Since(start)

	j.mu.Lock()
	j.respCode[resp.StatusCode]++

	if envoyUpstreamMS := resp.Header.Get("X-Envoy-Upstream-Service-Time"); envoyUpstreamMS != "" {
		ms, err := strconv.Atoi(envoyUpstreamMS)
		if err == nil {
			j.upstreamHist.Add(float64(ms))
			j.upstreamHistPrecise.Add(float64(ms))
		}
	}

	if j.respCode[resp.StatusCode] == 1 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return si, err
		}

		switch {
		case resp.Header.Get("Content-Encoding") != "":
			j.respBody[resp.StatusCode] = []byte("<" + resp.Header.Get("Content-Encoding") + "-encoded-content>")
		case len(body) > 1000:
			j.respBody[resp.StatusCode] = append(body[0:1000], '.', '.', '.')
		default:
			j.respBody[resp.StatusCode] = body
		}
	}

	j.mu.Unlock()

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		return si, err
	}

	err = resp.Body.Close()
	if err != nil {
		return si, err
	}

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
