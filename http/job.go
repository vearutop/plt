package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/vearutop/dynhist-go"
	"github.com/vearutop/plt/loadgen"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"time"
)

type JobProducer struct {
	start time.Time

	dnsHist  *dynhist.Collector
	connHist *dynhist.Collector
	tlsHist  *dynhist.Collector
	mu       sync.Mutex
	respCode map[int]int

	bytesWritten int64
	bytesRead    int64

	f Flags

	tr *http.Transport
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
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
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

func NewJobProducer(f Flags, lf loadgen.Flags) *JobProducer {
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
	j.respCode = make(map[int]int, 5)
	j.f = f

	if _, ok := f.HeaderMap["User-Agent"]; !ok {
		f.HeaderMap["User-Agent"] = "plt"
	}

	return &j
}

func (j *JobProducer) Print() {
	println("DNS latency distribution in ms:")
	println(j.dnsHist.String())
	println("TLS handshake latency distribution in ms:")
	println(j.tlsHist.String())

	println("Connection latency distribution in ms:")
	println(j.connHist.String())

	println("Responses by status code")
	j.mu.Lock()
	codes := ""
	for code, cnt := range j.respCode {
		codes += fmt.Sprintf("[%d] %d\n", code, cnt)
	}
	j.mu.Unlock()
	println(codes)

	println("Bytes read", atomic.LoadInt64(&j.bytesRead))
	println("Bytes written", atomic.LoadInt64(&j.bytesWritten))
}

func (j *JobProducer) Job(i int) (time.Duration, error) {
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

	_, err = io.Copy(ioutil.Discard, resp.Body)
	if err != nil {
		println(err.Error())
	}
	err = resp.Body.Close()
	if err != nil {
		println(err.Error())
	}
	j.mu.Lock()
	j.respCode[resp.StatusCode]++
	j.mu.Unlock()

	return si, nil
}

type Flags struct {
	HeaderMap   map[string]string
	URL         string
	Body        string
	Method      string
	NoKeepalive bool
}
