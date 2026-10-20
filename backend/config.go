package main

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL   string
	Port          string
	AdminEmail    string
	AdminUsername string
	AdminPassword string
	AppOrigin     string
	CookieSecure  bool
	SessionTTL    time.Duration
}

func loadConfig() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"), Port: envOr("PORT", "8080"),
		AdminEmail:    strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))),
		AdminUsername: strings.TrimSpace(os.Getenv("ADMIN_USERNAME")),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"), AppOrigin: envOr("APP_ORIGIN", "http://localhost:3000"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL wajib diisi")
	}
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return c, errors.New("PORT tidak valid")
	}
	u, err := url.Parse(c.AppOrigin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return c, errors.New("APP_ORIGIN harus berupa origin HTTP(S), tanpa path atau trailing slash")
	}
	c.CookieSecure, err = strconv.ParseBool(envOr("COOKIE_SECURE", "false"))
	if err != nil {
		return c, errors.New("COOKIE_SECURE harus true atau false")
	}
	hours, err := strconv.Atoi(envOr("SESSION_TTL_HOURS", "168"))
	if err != nil || hours < 1 || hours > 8760 {
		return c, errors.New("SESSION_TTL_HOURS harus antara 1 dan 8760")
	}
	c.SessionTTL = time.Duration(hours) * time.Hour
	if err := validateRegistration(c.AdminEmail, c.AdminUsername, c.AdminPassword); err != nil {
		return c, fmt.Errorf("konfigurasi admin: %w", err)
	}
	return c, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

func validateRegistration(email, username, password string) error {
	address, err := mail.ParseAddress(email)
	at := strings.LastIndex(email, "@")
	if err != nil || address.Address != email || len(email) > 254 || at < 1 || !strings.Contains(email[at+1:], ".") || strings.ContainsAny(email, "\r\n") {
		return errors.New("Gunakan alamat email yang valid")
	}
	if !usernamePattern.MatchString(username) {
		return errors.New("Username harus 3–32 karakter: huruf, angka, atau underscore")
	}
	if len(password) < 10 || len(password) > 72 {
		return errors.New("Kata sandi harus 10–72 byte (karakter ASCII dihitung 1 byte)")
	}
	return nil
}
