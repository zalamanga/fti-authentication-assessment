package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func run() error {
	c, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	dbConfig, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		return errors.New("konfigurasi koneksi basis data tidak valid")
	}
	dbConfig.MaxConns = 20
	dbConfig.MinConns = 1
	dbConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	db, err := pgxpool.NewWithConfig(startup, dbConfig)
	if err != nil {
		return errors.New("gagal membuka pool basis data")
	}
	defer db.Close()
	for {
		if err = db.Ping(startup); err == nil {
			break
		}
		select {
		case <-startup.Done():
			return errors.New("basis data tidak siap dalam 60 detik")
		case <-time.After(time.Second):
		}
	}
	if err = migrate(startup, db); err != nil {
		return errors.New("migrasi basis data gagal")
	}
	store := &Store{DB: db, TTL: c.SessionTTL}
	if err = store.bootstrapAdmin(startup, c); err != nil {
		return err
	}
	app, err := newApp(store, c)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: ":" + c.Port, Handler: http.TimeoutHandler(app.routes(), 15*time.Second, `{"error":"Permintaan melewati batas waktu","code":"TIMEOUT"}`), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 10*time.Second)
				if err := store.cleanupExpired(cleanupCtx); err != nil {
					slog.Warn("expired session cleanup failed")
				}
				cleanupCancel()
			}
		}
	}()
	slog.Info("authentication API ready", "port", c.Port)
	select {
	case err = <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdown)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err.Error())
		os.Exit(1)
	}
}
