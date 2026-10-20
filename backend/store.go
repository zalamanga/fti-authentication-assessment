package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed migrations/001_init.sql
var initialMigration string

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Current   bool      `json:"current"`
}

type Auth struct {
	User      User
	SessionID string
}

type Store struct {
	DB  *pgxpool.Pool
	TTL time.Duration
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(726104205)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=1)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, initialMigration); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES(1)`); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) bootstrapAdmin(ctx context.Context, c Config) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(c.AdminPassword), 12)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(726104206)`); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE role='admin')`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err = tx.Exec(ctx, `INSERT INTO users(id,email,username,password_hash,role) VALUES($1,$2,$3,$4,'admin')`, id, c.AdminEmail, c.AdminUsername, string(hash)); err != nil {
			return errors.New("gagal membuat admin pertama; periksa konflik email atau username")
		}
	}
	return tx.Commit(ctx)
}

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Role, &u.CreatedAt)
	return u, err
}

func (s *Store) createUser(ctx context.Context, email, username, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return User{}, err
	}
	id, err := randomID()
	if err != nil {
		return User{}, err
	}
	return scanUser(s.DB.QueryRow(ctx, `INSERT INTO users(id,email,username,password_hash) VALUES($1,$2,$3,$4) RETURNING id,email,username,role,created_at`, id, email, username, string(hash)))
}

func (s *Store) registerWithSession(ctx context.Context, email, username, password, agent string) (User, string, time.Time, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	id, err := randomID()
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	u, err := scanUser(tx.QueryRow(ctx, `INSERT INTO users(id,email,username,password_hash) VALUES($1,$2,$3,$4) RETURNING id,email,username,role,created_at`, id, email, username, string(hash)))
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	token, expires, err := s.issueSessionTx(ctx, tx, u.ID, agent)
	if err != nil {
		return User{}, "", time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, "", time.Time{}, err
	}
	return u, token, expires, nil
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func newToken() (string, string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b[:])
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// The user row is the common lock for login and administrative revocation.
// Keeping only the newest existing session before insertion enforces the cap
// even when different API instances process logins concurrently.
func (s *Store) issueSession(ctx context.Context, userID, agent string) (string, time.Time, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	token, expires, err := s.issueSessionTx(ctx, tx, userID, agent)
	if err != nil {
		return "", time.Time{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) issueSessionTx(ctx context.Context, tx pgx.Tx, userID, agent string) (string, time.Time, error) {
	token, hash, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	id, err := randomID()
	if err != nil {
		return "", time.Time{}, err
	}
	var lockedID string
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedID); err != nil {
		return "", time.Time{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND (expires_at <= clock_timestamp() OR id NOT IN (SELECT id FROM sessions WHERE user_id=$1 AND expires_at > clock_timestamp() ORDER BY created_at DESC,id DESC LIMIT 1))`, userID); err != nil {
		return "", time.Time{}, err
	}
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO sessions(id,user_id,token_hash,user_agent,expires_at) VALUES($1,$2,$3,$4,clock_timestamp()+($5::bigint * interval '1 second')) RETURNING expires_at`, id, userID, hash, truncateAgent(agent), int64(s.TTL.Seconds())).Scan(&expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func truncateAgent(agent string) string {
	runes := []rune(agent)
	if len(runes) > 300 {
		runes = runes[:300]
	}
	return string(runes)
}

func (s *Store) authenticate(ctx context.Context, token string) (Auth, error) {
	var a Auth
	if len(token) != 43 {
		return a, pgx.ErrNoRows
	}
	err := s.DB.QueryRow(ctx, `SELECT u.id,u.email,u.username,u.role,u.created_at,s.id FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at > clock_timestamp()`, hashToken(token)).Scan(&a.User.ID, &a.User.Email, &a.User.Username, &a.User.Role, &a.User.CreatedAt, &a.SessionID)
	return a, err
}

func (s *Store) revokeAllSessions(ctx context.Context, userID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) listSessions(ctx context.Context, a Auth) ([]Session, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,user_agent,created_at,expires_at FROM sessions WHERE user_id=$1 AND expires_at>clock_timestamp() ORDER BY created_at DESC,id DESC`, a.User.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]Session, 0, 2)
	for rows.Next() {
		var item Session
		if err = rows.Scan(&item.ID, &item.UserAgent, &item.CreatedAt, &item.ExpiresAt); err != nil {
			return nil, err
		}
		item.Current = item.ID == a.SessionID
		sessions = append(sessions, item)
	}
	return sessions, rows.Err()
}

func (s *Store) cleanupExpired(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= clock_timestamp()`)
	return err
}

func (s *Store) checkPassword(ctx context.Context, identifier, password string, dummyHash []byte) (User, bool, error) {
	var u User
	var hash string
	err := s.DB.QueryRow(ctx, `SELECT id,email,username,role,created_at,password_hash FROM users WHERE lower(email)=lower($1) OR lower(username)=lower($1)`, identifier).Scan(&u.ID, &u.Email, &u.Username, &u.Role, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return u, false, nil
	}
	if err != nil {
		return u, false, fmt.Errorf("lookup login: %w", err)
	}
	return u, bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, nil
}
