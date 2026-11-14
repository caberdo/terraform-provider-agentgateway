package client

import (
	"net/http"
	"strings"
)

// RedactedHeaders returns a copy of h suitable for logging. Values that look
// like credentials are replaced with "[redacted]". Request and response bodies
// are never logged by the client.
func RedactedHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		lk := strings.ToLower(k)
		switch {
		case lk == "authorization",
			lk == "cookie",
			lk == "set-cookie",
			lk == "x-api-key",
			strings.Contains(lk, "token"),
			strings.Contains(lk, "api-key"),
			strings.Contains(lk, "apikey"):
			out[k] = "[redacted]"
		default:
			out[k] = strings.Join(v, ", ")
		}
	}
	return out
}
