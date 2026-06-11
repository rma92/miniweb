package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements per-IP token-bucket rate limiting.
// A new IP starts with burstTokens and refills at rate tokens/second.
// Stale entries are evicted lazily when the table grows too large.
type RateLimiter struct {
	mu          sync.Mutex
	clients     map[string]*rlClient
	rate        float64 // tokens per second
	burstTokens float64 // max tokens (burst capacity)
}

type rlClient struct {
	tokens   float64
	lastSeen time.Time
}

// NewRateLimiter creates a RateLimiter.
// rate = sustained requests/second per IP, burst = short-burst capacity.
func NewRateLimiter(rate, burst float64) *RateLimiter {
	return &RateLimiter{
		clients:     make(map[string]*rlClient),
		rate:        rate,
		burstTokens: burst,
	}
}

// Allow returns true if the given IP is allowed to proceed.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	c, ok := rl.clients[ip]
	if !ok {
		c = &rlClient{tokens: rl.burstTokens, lastSeen: now}
		rl.clients[ip] = c
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(c.lastSeen).Seconds()
	c.lastSeen = now
	c.tokens += elapsed * rl.rate
	if c.tokens > rl.burstTokens {
		c.tokens = rl.burstTokens
	}

	if c.tokens < 1 {
		return false
	}
	c.tokens--

	// Lazy eviction: if the table is large, purge IPs idle > 5 minutes.
	if len(rl.clients) > 10000 {
		cutoff := now.Add(-5 * time.Minute)
		for k, v := range rl.clients {
			if v.lastSeen.Before(cutoff) {
				delete(rl.clients, k)
			}
		}
	}
	return true
}

// Middleware returns an http.Handler that applies rate limiting.
// Requests that exceed the limit receive 429 Too Many Requests.
// When rl is nil the middleware is a no-op (rate limiting disabled).
func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if rl == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			if !rl.Allow(ip) {
				w.Header().Set("Retry-After", "1")
				writeError(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// realIP extracts the client IP from X-Real-IP, X-Forwarded-For, or RemoteAddr.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For may be a comma-separated list; take the first.
		for i := 0; i < len(ip); i++ {
			if ip[i] == ',' {
				return ip[:i]
			}
		}
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
