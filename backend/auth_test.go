package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegistrationValidation(t *testing.T) {
	for _, test := range []struct {
		name, email, username, password string
		valid                           bool
	}{
		{"valid", "rani@example.com", "Rani_21", "a secure passphrase", true},
		{"display name", "Rani <rani@example.com>", "rani", "a secure passphrase", false},
		{"missing email", "", "rani", "a secure passphrase", false},
		{"local domain", "rani.mail@localhost", "rani", "a secure passphrase", false},
		{"email as username", "rani@example.com", "rani@example.com", "a secure passphrase", false},
		{"short username", "rani@example.com", "ab", "a secure passphrase", false},
		{"short password", "rani@example.com", "rani", "short", false},
		{"bcrypt byte limit", "rani@example.com", "rani", strings.Repeat("é", 37), false},
		{"bcrypt exact limit", "rani@example.com", "rani", strings.Repeat("a", 72), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validateRegistration(test.email, test.username, test.password) == nil; got != test.valid {
				t.Fatalf("valid = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestTokenHasEntropyAndOnlyHashIsPersisted(t *testing.T) {
	first, hash, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil || len(decoded) != 32 || len(hash) != 64 || first == hash || hashToken(first) != hash {
		t.Fatal("invalid token or hash format")
	}
	second, _, err := newToken()
	if err != nil || first == second {
		t.Fatal("tokens must be independent")
	}
}

func TestJSONRejectsPrivilegeEscalationAndExtraData(t *testing.T) {
	for _, body := range []string{`{"email":"rani@example.com","role":"admin"}`, `{"email":"rani@example.com"}{}`, strings.Repeat("x", 9000), `null true`} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		var input struct {
			Email string `json:"email"`
		}
		if decodeJSON(w, r, &input) || w.Code != http.StatusBadRequest {
			t.Fatalf("accepted invalid body: %s", body[:min(60, len(body))])
		}
	}
}

func TestOriginAndCookiePolicy(t *testing.T) {
	a := &App{Config: Config{AppOrigin: "http://localhost:3000", CookieSecure: true}}
	for _, test := range []struct {
		origin string
		status int
	}{
		{"http://localhost:3000", 200}, {"", 200}, {"http://localhost:3000.evil.example", 403}, {"null", 403},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		r.Header.Set("Origin", test.origin)
		a.security(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, r)
		if w.Code != test.status {
			t.Errorf("origin %q: status %d, want %d", test.origin, w.Code, test.status)
		}
	}
	w := httptest.NewRecorder()
	a.clearCookie(w)
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.MaxAge >= 0 {
		t.Fatalf("incorrect cookie: %+v", cookie)
	}
}

func TestRateLimitWindowAndIsolation(t *testing.T) {
	l := newRateLimiter()
	now := time.Now()
	for i := 0; i < 20; i++ {
		if !l.allow("first", now) {
			t.Fatal("blocked too soon")
		}
	}
	if l.allow("first", now) || !l.allow("second", now) || !l.allow("first", now.Add(time.Minute)) {
		t.Fatal("incorrect limit, client isolation, or expiry")
	}
}

func TestUnauthenticatedRoutesHaveJSONErrors(t *testing.T) {
	a := &App{Config: Config{AppOrigin: "http://localhost:3000"}, limiter: newRateLimiter()}
	for _, path := range []string{"/api/auth/me", "/api/sessions", "/api/admin/users"} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		var result map[string]string
		if w.Code != 401 || json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&result) != nil || result["code"] != "UNAUTHENTICATED" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
