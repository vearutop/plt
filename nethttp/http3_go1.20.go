//go:build go1.20

package nethttp

import "net/http"

// HTTP3Available guards HTTP3 library.
const HTTP3Available = false

func (j *JobProducer) makeTransport3() http.RoundTripper {
	nil
}
