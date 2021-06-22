// Package report provides reporting utilities.
package report

import (
	"strconv"
	"strings"
	"unicode"
)

// PeekBody takes head of data for printing.
func PeekBody(body []byte, l int) []byte {
	tooLong := false
	if len(body) > l {
		tooLong = true
		body = body[0:l]
	}

	if !IsASCIIPrintable(string(body)) {
		return []byte("<non-printable-binary-data>")
	}

	if tooLong {
		return append(body, '.', '.', '.')
	}

	return body
}

// IsASCIIPrintable checks if s is ascii.
func IsASCIIPrintable(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}

	return true
}

// Bytes.
const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
	PETABYTE
	EXABYTE
)

// ByteSize returns a human-readable byte string of the form 10M, 12.5K, and so forth.
func ByteSize(bytes int64) string {
	var (
		unit  string
		value = float64(bytes)
	)

	switch {
	case bytes >= EXABYTE:
		unit = "EB"
		value /= EXABYTE
	case bytes >= PETABYTE:
		unit = "PB"
		value /= PETABYTE
	case bytes >= TERABYTE:
		unit = "TB"
		value /= TERABYTE
	case bytes >= GIGABYTE:
		unit = "GB"
		value /= GIGABYTE
	case bytes >= MEGABYTE:
		unit = "MB"
		value /= MEGABYTE
	case bytes >= KILOBYTE:
		unit = "KB"
		value /= KILOBYTE
	default:
		unit = "B"
	}

	result := strconv.FormatFloat(value, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")

	return result + unit
}
