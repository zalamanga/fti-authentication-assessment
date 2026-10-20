package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const cookieName = "fti_session"

type App struct {
	Store     *Store
	Config    Config
	dummyHash []byte
	limiter   *rateLimiter
}

func newApp(store *Store, config Config) (*App, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte("unused-dummy-password-fti"), 12)
	if err != nil {
		return nil, err
	}
	return &App{Store: store, Config: config, dummyHash: hash, limiter: newRateLimiter()}, nil
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.health)
	mux.Handle("POST /api/auth/register", a.limit(http.HandlerFunc(a.register)))
	mux.Handle("POST /api/auth/login", a.limit(http.HandlerFunc(a.login)))
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.Handle("GET /api/auth/me", a.requireAuth(false, a.me))
	mux.Handle("GET /api/sessions", a.requireAuth(false, a.sessions))
	mux.Handle("DELETE /api/sessions/{id}", a.requireAuth(false, a.revokeSession))
	mux.Handle("GET /api/admin/users", a.requireAuth(true, a.users))
	mux.Handle("POST /api/admin/users/{id}/logout", a.requireAuth(true, a.forceLogout))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Endpoint tidak ditemukan")
	})
	return a.security(mux)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.Store.DB.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Basis data belum siap")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "CONTENT_TYPE", "Gunakan Content-Type application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Data JSON tidak valid atau memuat field yang tidak diizinkan")
		return false
	}
	if err = d.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Kirim tepat satu objek JSON")
		return false
	}
	return true
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Username = strings.TrimSpace(input.Username)
	if err := validateRegistration(input.Email, input.Username, input.Password); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION", err.Error())
		return
	}
	u, token, expires, err := a.Store.registerWithSession(r.Context(), input.Email, input.Username, input.Password, r.UserAgent())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "ALREADY_EXISTS", "Email atau username sudah digunakan")
		} else {
			serverError(w, err)
		}
		return
	}
	a.setSessionCookie(w, token, expires)
	writeJSON(w, http.StatusCreated, map[string]any{"user": u})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Identifier = strings.TrimSpace(input.Identifier)
	if len(input.Identifier) == 0 || len(input.Identifier) > 254 || len(input.Password) == 0 || len(input.Password) > 72 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "Email atau username dan kata sandi wajib diisi")
		return
	}
	u, valid, err := a.Store.checkPassword(r.Context(), input.Identifier, input.Password, a.dummyHash)
	if err != nil {
		serverError(w, err)
		return
	}
	if !valid {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email, username, atau kata sandi tidak sesuai")
		return
	}
	if !a.startSession(w, r, u.ID) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (a *App) startSession(w http.ResponseWriter, r *http.Request, userID string) bool {
	token, expires, err := a.Store.issueSession(r.Context(), userID, r.UserAgent())
	if err != nil {
		serverError(w, err)
		return false
	}
	a.setSessionCookie(w, token, expires)
	return true
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(a.Config.SessionTTL.Seconds())})
}

func (a *App) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Unix(1, 0), MaxAge: -1})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		if _, err = a.Store.DB.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, hashToken(cookie.Value)); err != nil {
			serverError(w, err)
			return
		}
	}
	a.clearCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Berhasil keluar"})
}

func (a *App) me(w http.ResponseWriter, r *http.Request, auth Auth) {
	writeJSON(w, http.StatusOK, map[string]any{"user": auth.User})
}

func (a *App) sessions(w http.ResponseWriter, r *http.Request, auth Auth) {
	sessions, err := a.Store.listSessions(r.Context(), auth)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (a *App) revokeSession(w http.ResponseWriter, r *http.Request, auth Auth) {
	id := r.PathValue("id")
	tag, err := a.Store.DB.Exec(r.Context(), `DELETE FROM sessions WHERE id=$1 AND user_id=$2`, id, auth.User.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Sesi tidak ditemukan")
		return
	}
	if id == auth.SessionID {
		a.clearCookie(w)
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Sesi berhasil diakhiri"})
}

type adminUser struct {
	User
	ActiveSessions int `json:"active_sessions"`
}

func (a *App) users(w http.ResponseWriter, r *http.Request, auth Auth) {
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 1000000 {
			writeError(w, http.StatusBadRequest, "VALIDATION", "Nomor halaman tidak valid")
			return
		}
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 254 {
		writeError(w, http.StatusBadRequest, "VALIDATION", "Pencarian terlalu panjang")
		return
	}
	const size = 20
	// Repeatable read makes the count and the page agree during registration.
	tx, err := a.Store.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var total int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE strpos(lower(email),lower($1))>0 OR strpos(lower(username),lower($1))>0`, q).Scan(&total); err != nil {
		serverError(w, err)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT u.id,u.email,u.username,u.role,u.created_at,(SELECT count(*) FROM sessions s WHERE s.user_id=u.id AND s.expires_at>now()) FROM users u WHERE strpos(lower(u.email),lower($1))>0 OR strpos(lower(u.username),lower($1))>0 ORDER BY u.created_at DESC,u.id DESC LIMIT $2 OFFSET $3`, q, size, (page-1)*size)
	if err != nil {
		serverError(w, err)
		return
	}
	users := make([]adminUser, 0)
	for rows.Next() {
		var u adminUser
		if err = rows.Scan(&u.ID, &u.Email, &u.Username, &u.Role, &u.CreatedAt, &u.ActiveSessions); err != nil {
			rows.Close()
			serverError(w, err)
			return
		}
		users = append(users, u)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "total": total, "page": page, "page_size": size})
}

func (a *App) forceLogout(w http.ResponseWriter, r *http.Request, auth Auth) {
	id := r.PathValue("id")
	if id == auth.User.ID {
		writeError(w, http.StatusBadRequest, "SELF_LOGOUT", "Gunakan menu keluar untuk mengakhiri sesi sendiri")
		return
	}
	err := a.Store.revokeAllSessions(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Pengguna tidak ditemukan")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Seluruh sesi pengguna telah diakhiri"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}

func serverError(w http.ResponseWriter, err error) {
	// SQL errors may contain submitted values, so log only their class.
	var dbErr *pgconn.PgError
	if errors.As(err, &dbErr) {
		slog.Error("database operation failed", "sqlstate", dbErr.Code)
	} else {
		slog.Error("request operation failed", "type", errorType(err))
	}
	writeError(w, http.StatusInternalServerError, "INTERNAL", "Terjadi kesalahan pada server. Silakan coba lagi")
}
