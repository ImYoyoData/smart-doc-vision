package api

import "testing"

func TestSixDigitCode(t *testing.T) {
	for _, code := range []string{"000000", "123456", "987654"} {
		if !sixDigitCode(code) {
			t.Fatalf("valid code rejected: %q", code)
		}
	}
	for _, code := range []string{"", "12345", "1234567", "12a456", " 123456", "12345 "} {
		if sixDigitCode(code) {
			t.Fatalf("invalid code accepted: %q", code)
		}
	}
}
