package fasthttp_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	fh "github.com/vearutop/plt/fasthttp"
	"github.com/vearutop/plt/loadgen"
	"github.com/vearutop/plt/nethttp"
)

func TestNewJobProducer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/?foo=bar", r.URL.RequestURI())
	}))
	defer srv.Close()

	out := bytes.NewBuffer(nil)

	lf := loadgen.Flags{
		Number:       100,
		Concurrency:  5,
		RateLimit:    1000,
		Duration:     time.Minute,
		SlowResponse: time.Second,
		Output:       out,
	}
	f := nethttp.Flags{
		HeaderMap: map[string]string{
			"X-Foo": "bar",
		},
		URL:        srv.URL,
		Body:       "foo",
		Method:     http.MethodPost,
		Compressed: true,
	}
	j, err := fh.NewJobProducer(f)
	require.NoError(t, err)

	j.PrepareRequest = func(_ int, req *fasthttp.Request) error {
		req.SetRequestURI(srv.URL + "/?foo=bar")

		return nil
	}

	require.NoError(t, loadgen.Run(lf, j))
	assert.NotEmpty(t, out.String())

	assert.Equal(t, map[string]int{"200": 100}, j.RequestCounts())
}

func BenchmarkJobProducer_Job(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		require.Equal(b, "/?foo=bar", r.URL.RequestURI())
	}))
	defer srv.Close()

	out := bytes.NewBuffer(nil)

	lf := loadgen.Flags{
		Number:       b.N,
		Concurrency:  50,
		Duration:     time.Minute,
		SlowResponse: time.Second,
		Output:       out,
	}
	f := nethttp.Flags{
		HeaderMap: map[string]string{
			"X-Foo": "bar",
		},
		URL:        srv.URL,
		Body:       "foo",
		Method:     http.MethodPost,
		Compressed: true,
	}
	j, err := fh.NewJobProducer(f)
	require.NoError(b, err)

	j.PrepareRequest = func(_ int, req *fasthttp.Request) error {
		req.SetRequestURI(srv.URL + "/?foo=bar")

		return nil
	}

	b.ReportAllocs()
	require.NoError(b, loadgen.Run(lf, j))
}
