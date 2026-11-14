package client

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestExtractMessageKeepsJSONMessages proves the gateway's own JSON error
// message survives verbatim, including the read-only message the 403 mapping
// matches on.
func TestExtractMessageKeepsJSONMessages(t *testing.T) {
	t.Parallel()

	got := extractMessage(403, []byte(`"UI is configured as read-only"`))
	if got != "UI is configured as read-only" {
		t.Fatalf("extractMessage() = %q, want the JSON message", got)
	}
}

// TestExtractMessageDoesNotEchoNonJSONBodies pins that a proxy HTML error page
// never reaches a diagnostic: the message names the status and carries at most
// maxErrorBody bytes of the body.
func TestExtractMessageDoesNotEchoNonJSONBodies(t *testing.T) {
	t.Parallel()

	t.Run("html page is replaced by a generic message", func(t *testing.T) {
		page := "<html><head><title>502 Bad Gateway</title></head><body>" +
			strings.Repeat("<p>upstream is unavailable</p>", 40) + "</body></html>"

		got := extractMessage(502, []byte(page))
		// A short snippet is allowed, but the whole page must never be echoed
		// and the message must say what happened and name the status.
		if strings.Contains(got, "</body></html>") {
			t.Errorf("extractMessage() = %q, want the body truncated", got)
		}
		if !strings.Contains(got, "non-JSON") {
			t.Errorf("extractMessage() = %q, want a generic non-JSON message", got)
		}
		if !strings.Contains(got, "502") {
			t.Errorf("extractMessage() = %q, want it to name the status", got)
		}
		if len(got) > maxErrorBody+100 {
			t.Errorf("extractMessage() is %d bytes, want a short message", len(got))
		}
	})

	t.Run("long body is truncated", func(t *testing.T) {
		body := strings.Repeat("x", maxErrorBody*3)

		got := extractMessage(500, []byte(body))
		if !strings.Contains(got, "…") {
			t.Errorf("extractMessage() = %q, want a truncation marker", got)
		}
		if len(got) > maxErrorBody+100 {
			t.Errorf("extractMessage() is %d bytes, want a short message", len(got))
		}
	})

	t.Run("multi-byte rune is not cut in half", func(t *testing.T) {
		// "é" is two bytes, so a byte-offset cut at maxErrorBody lands inside
		// a rune.
		body := strings.Repeat("é", maxErrorBody)

		got := extractMessage(500, []byte(body))
		if !strings.Contains(got, "…") {
			t.Errorf("extractMessage() = %q, want a truncation marker", got)
		}
		if !utf8.ValidString(got) {
			t.Errorf("extractMessage() produced invalid UTF-8: %q", got)
		}
	})

	t.Run("empty body names the status only", func(t *testing.T) {
		got := extractMessage(503, nil)
		if !strings.Contains(got, "503") || !strings.Contains(got, "empty") {
			t.Errorf("extractMessage() = %q, want an empty-response message naming 503", got)
		}
	})
}
