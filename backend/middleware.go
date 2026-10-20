package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

func errorType(err error) string { return fmt.Sprintf("%T", err) }

func (a *App) requireAuth(admin bool, next func(http.ResponseWriter, *http.Request, Auth)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Silakan masuk terlebih dahulu")
			return
		}
		auth, err := a.Store.authenticate(r.Context(), cookie.Value)
		if errors.Is(err, pgx.ErrNoRows) {
			a.clearCookie(w)
			writeError(w, http.StatusUnauthorized, "SESSION_EXPIRED", "Sesi telah berakhir. Silakan masuk kembali")
			return
		}
		if err != nil {
			serverError(w, err)
			return
		}
		if admin && auth.User.Role != "admin" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "Halaman ini hanya dapat diakses admin")
			return
		}
		next(w, r, auth)
	})
}

func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		origin := r.Header.Get("Origin")
		if origin != "" && origin != a.Config.AppOrigin {
			writeError(w, http.StatusForbidden, "ORIGIN_REJECTED", "Origin permintaan tidak diizinkan")
			return
		}
		if origin == a.Config.AppOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeError(w, http.StatusForbidden, "ORIGIN_REJECTED", "Permintaan lintas situs tidak diizinkan")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type attemptWindow struct {
	start time.Time
	count int
}

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]attemptWindow
}

func newRateLimiter() *rateLimiter { return &rateLimiter{entries: make(map[string]attemptWindow)} }

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for ip, window := range l.entries {
		if now.Sub(window.start) >= time.Minute {
			delete(l.entries, ip)
		}
	}
	window, exists := l.entries[key]
	if !exists {
		if len(l.entries) >= 10000 {
			return false
		}
		window.start = now
	}
	if window.count >= 20 {
		return false
	}
	window.count++
	l.entries[key] = window
	return true
}

func (a *App) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !a.limiter.allow(host, time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan. Coba lagi dalam satu menit")
			return
		}
		next.ServeHTTP(w, r)
	})
}
