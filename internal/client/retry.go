package client

import (
	"context"
	"errors"
	"time"
)

// withRetries runs do up to three times. It retries only when do returns an
// error that is ErrTransient (or wraps it). The backoff schedule is 100ms,
// 500ms, 1s.
func withRetries(ctx context.Context, do func() (int, []byte, error)) (int, []byte, error) {
	backoffs := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second}

	var status int
	var body []byte
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		status, body, err = do()
		if err == nil {
			return status, body, nil
		}
		if !errors.Is(err, ErrTransient) {
			return status, body, err
		}
		if attempt == len(backoffs) {
			break
		}
		select {
		case <-ctx.Done():
			return status, body, ctx.Err()
		case <-time.After(backoffs[attempt]):
		}
	}
	return status, body, err
}
