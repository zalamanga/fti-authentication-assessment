package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + id
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = migrate(ctx, db); err != nil {
		t.Fatalf("migration must be idempotent: %v", err)
	}
	return &Store{DB: db, TTL: 7 * 24 * time.Hour}
}

func createTestUser(t *testing.T, store *Store, name string) User {
	t.Helper()
	u, err := store.createUser(context.Background(), name+"@example.com", name, "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestIntegrationSessionEvictionAndConcurrency(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	u := createTestUser(t, s, "rani")
	tokens := make([]string, 3)
	for i := range tokens {
		var err error
		tokens[i], _, err = s.issueSession(ctx, u.ID, fmt.Sprintf("device-%d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.authenticate(ctx, tokens[0]); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("third login must revoke oldest session")
	}
	for _, token := range tokens[1:] {
		if _, err := s.authenticate(ctx, token); err != nil {
			t.Fatal("two newest sessions must remain valid", err)
		}
	}
	const parallel = 12
	var wg sync.WaitGroup
	errorsCh := make(chan error, parallel)
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.issueSession(ctx, u.ID, "concurrent")
			errorsCh <- err
		}()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND expires_at>now()`, u.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("concurrent logins left %d sessions, err %v", count, err)
	}
	var storedHash string
	if err := s.DB.QueryRow(ctx, `SELECT token_hash FROM sessions WHERE user_id=$1 LIMIT 1`, u.ID).Scan(&storedHash); err != nil || len(storedHash) != 64 {
		t.Fatal("session must store SHA-256 hash only")
	}
}

func integrationRequest(app *App, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	return w
}

func TestIntegrationAuthorizationAndForceLogout(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	c := Config{AdminEmail: "admin@example.com", AdminUsername: "admin", AdminPassword: "admin-password-123", AppOrigin: "http://localhost:3000", SessionTTL: s.TTL}
	if err := s.bootstrapAdmin(ctx, c); err != nil {
		t.Fatal(err)
	}
	var admin User
	admin, err := scanUser(s.DB.QueryRow(ctx, `SELECT id,email,username,role,created_at FROM users WHERE role='admin'`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.bootstrapAdmin(ctx, c); err != nil {
		t.Fatal("bootstrap must be idempotent", err)
	}
	app, err := newApp(s, c)
	if err != nil {
		t.Fatal(err)
	}
	u := createTestUser(t, s, "member")
	first, _, _ := s.issueSession(ctx, u.ID, "first")
	second, _, _ := s.issueSession(ctx, u.ID, "second")
	adminToken, _, _ := s.issueSession(ctx, admin.ID, "admin")
	if w := integrationRequest(app, "GET", "/api/admin/users", first, ""); w.Code != 403 {
		t.Fatalf("user accessed admin listing: %d %s", w.Code, w.Body.String())
	}
	if w := integrationRequest(app, "POST", "/api/admin/users/"+u.ID+"/logout", first, ""); w.Code != 403 {
		t.Fatal("user must not force logout")
	}
	if w := integrationRequest(app, "POST", "/api/admin/users/"+admin.ID+"/logout", adminToken, ""); w.Code != 400 {
		t.Fatal("admin must not force logout self")
	}
	if w := integrationRequest(app, "POST", "/api/admin/users/not-a-real-id/logout", adminToken, ""); w.Code != 404 {
		t.Fatal("malformed user IDs must return 404")
	}
	if w := integrationRequest(app, "DELETE", "/api/sessions/not-a-real-id", first, ""); w.Code != 404 {
		t.Fatal("malformed session IDs must return 404")
	}
	if w := integrationRequest(app, "POST", "/api/admin/users/"+u.ID+"/logout", adminToken, ""); w.Code != 200 {
		t.Fatalf("force logout failed: %d %s", w.Code, w.Body.String())
	}
	for _, token := range []string{first, second} {
		if w := integrationRequest(app, "GET", "/api/auth/me", token, ""); w.Code != 401 {
			t.Fatal("force logout must invalidate every session on next request")
		}
	}
	if w := integrationRequest(app, "GET", "/api/auth/me", adminToken, ""); w.Code != 200 {
		t.Fatal("admin's session must remain valid")
	}
}

func TestIntegrationRegistrationRollsBackWhenSessionCreationFails(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(ctx, `ALTER TABLE sessions ADD CONSTRAINT reject_test_sessions CHECK (false)`); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := s.registerWithSession(ctx, "rollback@example.com", "rollback", "test-password-123", "test")
	if err == nil {
		t.Fatal("expected session insertion failure")
	}
	var count int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE username='rollback'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed registration must not leave an account: count=%d err=%v", count, err)
	}
}

func TestIntegrationRegistrationLoginOwnershipAndExpiry(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	app, err := newApp(s, Config{AppOrigin: "http://localhost:3000", SessionTTL: s.TTL})
	if err != nil {
		t.Fatal(err)
	}
	escalation := `{"email":"rani@example.com","username":"rani","password":"test-password-123","role":"admin"}`
	if w := integrationRequest(app, "POST", "/api/auth/register", "", escalation); w.Code != 400 {
		t.Fatal("role escalation was accepted")
	}
	w := integrationRequest(app, "POST", "/api/auth/register", "", `{"email":"Rani@Example.com","username":"Rani","password":"test-password-123"}`)
	if w.Code != 201 {
		t.Fatalf("registration failed: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		User User `json:"user"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.User.Role != "user" || response.User.Email != "rani@example.com" {
		t.Fatal("registered user role or normalization incorrect")
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("session cookie is not protected")
	}
	if w = integrationRequest(app, "POST", "/api/auth/register", "", `{"email":"other@example.com","username":"RANI","password":"test-password-123"}`); w.Code != 409 {
		t.Fatal("case-insensitive uniqueness failed")
	}
	for _, identifier := range []string{"rani", "RANI@EXAMPLE.COM"} {
		w = integrationRequest(app, "POST", "/api/auth/login", "", `{"identifier":"`+identifier+`","password":"test-password-123"}`)
		if w.Code != 200 {
			t.Fatalf("login by %s failed: %d %s", identifier, w.Code, w.Body.String())
		}
		cookie = w.Result().Cookies()[0]
	}
	if w = integrationRequest(app, "POST", "/api/auth/login", "", `{"identifier":"rani","password":"wrong-password"}`); w.Code != 401 {
		t.Fatal("incorrect password accepted")
	}
	other := createTestUser(t, s, "other")
	otherToken, _, _ := s.issueSession(ctx, other.ID, "other-device")
	otherAuth, _ := s.authenticate(ctx, otherToken)
	if w = integrationRequest(app, "DELETE", "/api/sessions/"+otherAuth.SessionID, cookie.Value, ""); w.Code != 404 {
		t.Fatal("session owner boundary violated")
	}
	if _, err = s.DB.Exec(ctx, `UPDATE sessions SET expires_at=now()-interval '1 second' WHERE user_id=$1`, response.User.ID); err != nil {
		t.Fatal(err)
	}
	if w = integrationRequest(app, "GET", "/api/auth/me", cookie.Value, ""); w.Code != 401 {
		t.Fatal("expired cookie must not authenticate")
	}
	if w = integrationRequest(app, "POST", "/api/auth/logout", otherToken, ""); w.Code != 200 {
		t.Fatal("logout failed")
	}
	if _, err = s.authenticate(ctx, otherToken); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("logout did not revoke server-side session")
	}
}
