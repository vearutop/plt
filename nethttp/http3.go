//go:build go1.21

package nethttp

import (
	"crypto/tls"
	"net/http"

	"github.com/quic-go/quic-go/http3"
)

// HTTP3Available guards HTTP3 library.
const HTTP3Available = true

func (j *JobProducer) makeTransport3() http.RoundTripper {
	return &http3.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Allow insecure mode in a dev tool.
		},
		DisableCompression: true,
	}
}
