package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// perIP holds a simple token-bucket state per remote IP.
type perIP struct {
	tokens    float64
	lastSeen  time.Time
}

// RateLimiter is a per-IP token-bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*perIP
	rate    float64 // tokens per second
	burst   float64

	// cleanup goroutine state
	stopCh chan struct{}
}

// NewRateLimiter creates a rate limiter that allows rate requests/s with burst capacity.
func NewRateLimiter(ratePerSec float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*perIP),
		rate:    ratePerSec,
		burst:   float64(burst),
		stopCh:  make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Allow returns true if the request from ip should be allowed.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	state, ok := rl.clients[ip]
	if !ok {
		rl.clients[ip] = &perIP{tokens: rl.burst - 1, lastSeen: now}
		return true
	}

	elapsed := now.Sub(state.lastSeen).Seconds()
	state.tokens = min(rl.burst, state.tokens+elapsed*rl.rate)
	state.lastSeen = now

	if state.tokens < 1 {
		return false
	}
	state.tokens--
	return true
}

// Stop shuts down the background cleanup goroutine.
func (rl *RateLimiter) Stop() { close(rl.stopCh) }

// cleanup evicts stale IP entries every minute.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-2 * time.Minute)
			for ip, state := range rl.clients {
				if state.lastSeen.Before(cutoff) {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Limit returns a middleware that enforces the rate limiter.
func Limit(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !rl.Allow(ip) {
				slog.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

