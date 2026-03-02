package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimitConfig holds the configuration for the rate limiter middleware.
type RateLimitConfig struct {
	// Rate is the number of tokens added per second.
	Rate float64
	// Burst is the maximum number of tokens (bucket size).
	Burst int
	// CleanupInterval is how often stale entries are removed.
	CleanupInterval time.Duration
	// StaleAfter is the duration after which an unused entry is considered stale.
	StaleAfter time.Duration
}

// DefaultRateLimitConfig returns a sensible default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Rate:            10,
		Burst:           20,
		CleanupInterval: 5 * time.Minute,
		StaleAfter:      10 * time.Minute,
	}
}

type tokenBucket struct {
	tokens    float64
	maxTokens int
	rate      float64
	lastTime  time.Time
}

func newTokenBucket(rate float64, burst int) *tokenBucket {
	return &tokenBucket{
		tokens:    float64(burst),
		maxTokens: burst,
		rate:      rate,
		lastTime:  time.Now(),
	}
}

// allow checks whether a request is permitted. It refills tokens based on
// elapsed time and returns true if at least one token is available.
func (tb *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()
	tb.lastTime = now

	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.maxTokens) {
		tb.tokens = float64(tb.maxTokens)
	}

	if tb.tokens < 1 {
		return false
	}

	tb.tokens--
	return true
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	cfg     RateLimitConfig
}

func newRateLimiter(cfg RateLimitConfig) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*tokenBucket),
		cfg:     cfg,
	}
	go rl.cleanup()
	return rl
}

// cleanup periodically removes stale entries from the rate limiter.
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, bucket := range rl.buckets {
			if now.Sub(bucket.lastTime) > rl.cfg.StaleAfter {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, ok := rl.buckets[ip]
	if !ok {
		bucket = newTokenBucket(rl.cfg.Rate, rl.cfg.Burst)
		rl.buckets[ip] = bucket
	}

	return bucket.allow()
}

// RateLimit returns an HTTP middleware that applies per-IP token bucket rate limiting.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	limiter := newRateLimiter(cfg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			if !limiter.allow(ip) {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractIP returns the client IP address from the request, stripping
// the port when present.
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For first for proxied requests.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain.
		if i := len(xff); i > 0 {
			for j := 0; j < len(xff); j++ {
				if xff[j] == ',' {
					return xff[:j]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP header.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
