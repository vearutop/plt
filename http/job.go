package http

import (
	"bytes"
	"crypto/tls"
	"github.com/vearutop/dynhist-go"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"time"
)

type JobProducer struct {
	dnsHist  *dynhist.Collector
	connHist *dynhist.Collector
	tlsHist  *dynhist.Collector

	f Flags
}

func NewJobProducer(f Flags) *JobProducer {
	j := JobProducer{}

	j.dnsHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.connHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.tlsHist = &dynhist.Collector{BucketsLimit: 10, WeightFunc: dynhist.LatencyWidth}
	j.f = f

	return &j
}

func (j *JobProducer) Job(i int) (time.Duration, error) {
	start := time.Now()
	var dnsStart, connStart, tlsStart time.Time

	var body io.Reader
	if j.f.Body != "" {
		body = bytes.NewBufferString(j.f.Body)
	}
	req, _ := http.NewRequest(j.f.Method, j.f.URL, body)
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

	// Keep alive flag here.
	if j.f.NoKeepalive {
		tr = &http.Transport{}
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		println(err.Error())
	} else {
		si := time.Since(start)
		ms := si.Seconds() * 1000
		if si >= lj.f.SlowResponse {
			slow.Add(1)
		}
		roundTripHist.Add(ms)

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

	panic("implement me")
}

type Flags struct {
	HeaderMap   map[string]string
	URL         string
	Body        string
	Method      string
	NoKeepalive bool
}
