package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"crumbd/internal/auth"
	"crumbd/internal/store"
)

type contextKey int

const sessionContextKey contextKey = iota

// sessionFromContext returns the resolved session for the current request,
// set by requireSession.
func sessionFromContext(ctx context.Context) *store.Session {
	sess, _ := ctx.Value(sessionContextKey).(*store.Session)
	return sess
}

// requireSession resolves the Authorization: Bearer <token> header to a
// (vault, device) session, rejecting the request with 401 if missing,
// malformed, or expired/unknown.
func requireSession(st *store.Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed Authorization header")
			return
		}

		sess, err := st.LookupSession(auth.HashToken(token))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired session")
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey, sess)
		next(w, r.WithContext(ctx))
	}
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic handling request", "error", err, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

// ipRateLimiter is a simple per-key token-bucket limiter with lazy eviction
// of stale keys, used to blunt invite/challenge brute-forcing and registration spam.
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	burst    int
}

func newRateLimiter(perMinute int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        rate.Limit(float64(perMinute) / 60.0),
		burst:    perMinute,
	}
}

func (l *ipRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[key]
	if !ok {
		lim = rate.NewLimiter(l.r, l.burst)
		l.limiters[key] = lim
	}
	return lim.Allow()
}

func (l *ipRateLimiter) middleware(keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(keyFunc(r)) {
				writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// limitFunc is like middleware, but composes directly with an
// http.HandlerFunc — used to rate-limit *inside* requireSession, once
// keyFunc (e.g. sessionDeviceKey) can actually see the resolved session in
// the request context.
func (l *ipRateLimiter) limitFunc(keyFunc func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(keyFunc(r)) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		next(w, r)
	}
}

func clientIP(r *http.Request) string {
	// Deliberately uses RemoteAddr directly rather than X-Forwarded-For:
	// crumbd is meant to sit behind a reverse proxy on localhost, and a
	// simpler deployment (no proxy) must not be spoofable via a header.
	// Operators fronting crumbd with a proxy that sets a trustworthy
	// X-Forwarded-For can extend this if needed.
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}
