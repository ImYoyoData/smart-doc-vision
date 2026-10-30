package api

import (
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	valid := []string{
		"user@example.com",
		"first.last+tag@sub.example.com",
	}
	for _, raw := range valid {
		got, err := normalizeEmail("  " + raw + "  ")
		if err != nil {
			t.Fatalf("valid email %q rejected: %v", raw, err)
		}
		if got != raw {
			t.Fatalf("normalizeEmail(%q) = %q, want %q", raw, got, raw)
		}
	}

	if got, err := normalizeEmail("   "); err != nil || got != "" {
		t.Fatalf("empty email should normalize to empty, got %q, %v", got, err)
	}

	invalid := []string{
		"not-an-email",
		"a@",
		"a b@example.com",
		"张三 <user@example.com>",
		strings.Repeat("a", 120) + "@example.com",
	}
	for _, raw := range invalid {
		if _, err := normalizeEmail(raw); err == nil {
			t.Fatalf("invalid email %q accepted", raw)
		}
	}
}
