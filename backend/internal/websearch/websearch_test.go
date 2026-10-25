package websearch

import (
	"strings"
	"testing"
)

func TestCleanHTMLRemovesScriptsAndTags(t *testing.T) {
	raw := `<html><head><style>.x{color:red}</style></head><body>
	<h1>Redis Stream</h1><script>alert('x')</script>
	<p>XADD &amp; XREAD</p></body></html>`
	text := cleanHTML(raw)
	for _, unwanted := range []string{"color:red", "alert", "<h1>", "</p>"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("cleanHTML left %q in %q", unwanted, text)
		}
	}
	if !strings.Contains(text, "Redis Stream") || !strings.Contains(text, "XADD & XREAD") {
		t.Fatalf("cleanHTML lost content: %q", text)
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Fatalf("truncate = %q", got)
	}
	if got := truncate("中文", 3); got != "中文" {
		t.Fatalf("truncate short = %q", got)
	}
}
