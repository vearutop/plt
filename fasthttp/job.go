// Package fasthttp implements http load generator with fasthttp transport.
package fasthttp

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/nethttp"
	"github.com/vearutop/plt/report"
)

// JobProducer sends HTTP requests.
type JobProducer struct {
	PrepareRequest func(i int, req *fasthttp.Request) error

	bytesWritten int64
	bytesRead    int64

	mu       sync.Mutex
	respCode map[int]int
	respBody map[int][]byte

	body   []byte
	f      nethttp.Flags
	client *fasthttp.Client

	log string
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

// NewJobProducer creates load generator.
func NewJobProducer(f nethttp.Flags, options ...func(lf *loadgen.Flags, f *nethttp.Flags, j loadgen.JobProducer)) (*JobProducer, error) {
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

	j.log += fmt.Sprintln("Host resolved:", strings.Join(addrs, ","))

	j.respCode = make(map[int]int, 5)
	j.respBody = make(map[int][]byte, 5)

	if f.Body != "" {
		j.body = []byte(f.Body)
	}

	j.client = &fasthttp.Client{}
	j.client.Dial = func(addr string) (net.Conn, error) {
		c, err := fasthttp.Dial(addr)
		if err != nil {
			return c, err
		}

		return countingConn{
			j:    &j,
			Conn: c,
		}, nil
	}
	j.client.MaxConnsPerHost = 10000

	for _, o := range options {
		o(nil, &f, &j)
	}

	if _, ok := f.HeaderMap["User-Agent"]; !ok {
		j.client.Name = "plt"
	}

	return &j, nil
}

// String reports results.
func (j *JobProducer) String() string {
	j.mu.Lock()
	defer j.mu.Unlock()

	codes := ""
	resps := ""
	res := j.log

	for code, cnt := range j.respCode {
		codes += fmt.Sprintf("[%d] %d\n", code, cnt)
		resps += fmt.Sprintf("[%d]\n%s\n", code, string(j.respBody[code]))
	}

	if codes == "" {
		return ""
	}

	res += fmt.Sprintln(codes)

	res += fmt.Sprintln("Responses by status code")
	res += fmt.Sprintln(codes)

	res += fmt.Sprintln("Bytes read", report.ByteSize(atomic.LoadInt64(&j.bytesRead)))
	res += fmt.Sprintln("Bytes written", report.ByteSize(atomic.LoadInt64(&j.bytesWritten)))

	res += fmt.Sprintln(resps)

	return res
}

// Job sends a single http request.
func (j *JobProducer) Job(i int) (time.Duration, error) {
	start := time.Now()

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()

	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	if j.body != nil {
		req.SetBody(j.body)
	}

	req.Header.SetMethod(j.f.Method)
	req.SetRequestURI(j.f.URL)

	for k, v := range j.f.HeaderMap {
		req.Header.Set(k, v)
	}

	if j.PrepareRequest != nil {
		if err := j.PrepareRequest(i, req); err != nil {
			return 0, err
		}
	}

	err := j.client.Do(req, resp)
	if err != nil {
		return 0, err
	}

	si := time.Since(start)

	j.mu.Lock()
	j.respCode[resp.StatusCode()]++

	if j.respCode[resp.StatusCode()] == 1 {
		body := resp.Body()

		if len(resp.Header.Peek("Content-Encoding")) > 0 {
			j.respBody[resp.StatusCode()] = []byte("<" + string(resp.Header.Peek("Content-Encoding")) + "-encoded-content>")
		} else {
			j.respBody[resp.StatusCode()] = report.PeekBody(body, 1000)
		}
	}
	j.mu.Unlock()

	return si, nil
}
