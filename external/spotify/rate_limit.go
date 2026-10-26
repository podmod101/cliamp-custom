package spotify

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitError preserves Spotify's retry time instead of disguising an HTTP
// 429 as a request timeout or suggesting that a valid login must be reset.
type RateLimitError struct {
	Until        time.Time
	SharedClient bool
}

func (e *RateLimitError) Error() string {
	message := fmt.Sprintf("Spotify is rate-limiting requests. Retry after %s.", e.Until.Local().Format("Jan 2, 3:04:05 PM MST"))
	if e.SharedClient {
		message += " This uses the shared Spotify app quota; configure your own app client ID with cliamp setup."
	}
	return message
}

func retryDelay(value string, now time.Time, fallback time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 && seconds <= (1<<63-1)/int64(time.Second) {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil && date.After(now) {
		return date.Sub(now)
	}
	return fallback
}

func (p *SpotifyProvider) rateLimitError(now time.Time) *RateLimitError {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rateLimitedUntil.After(now) {
		return &RateLimitError{Until: p.rateLimitedUntil, SharedClient: p.clientID == DefaultClientID}
	}
	return nil
}

func (p *SpotifyProvider) rememberRateLimit(until time.Time) *RateLimitError {
	p.mu.Lock()
	defer p.mu.Unlock()
	if until.After(p.rateLimitedUntil) {
		p.rateLimitedUntil = until
	}
	return &RateLimitError{Until: p.rateLimitedUntil, SharedClient: p.clientID == DefaultClientID}
}
