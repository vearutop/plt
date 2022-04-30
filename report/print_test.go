package report_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vearutop/plt/report"
)

func TestPeekBody(t *testing.T) {
	assert.Equal(t, []byte(nil), report.PeekBody(nil, 10))
	assert.Equal(t, "1234567890...", string(report.PeekBody([]byte("123456789012345"), 10)))
	assert.Equal(t, "1234567890", string(report.PeekBody([]byte("1234567890"), 10)))
	assert.Equal(t, "123456789", string(report.PeekBody([]byte("123456789"), 10)))
	assert.Equal(t, "<non-printable-binary-data>", string(report.PeekBody([]byte("\xed\xa0\x80\x80"), 10)))
}

func TestByteSize(t *testing.T) {
	assert.Equal(t, "0B", report.ByteSize(0))
	assert.Equal(t, "1KB", report.ByteSize(report.KILOBYTE))
	assert.Equal(t, "1MB", report.ByteSize(report.MEGABYTE))
	assert.Equal(t, "1GB", report.ByteSize(report.GIGABYTE))
	assert.Equal(t, "1TB", report.ByteSize(report.TERABYTE))
	assert.Equal(t, "1PB", report.ByteSize(report.PETABYTE))
	assert.Equal(t, "1EB", report.ByteSize(report.EXABYTE))
}
