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

	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/config"
	"github.com/ziyue67/tms/go-backend/internal/database"
	"github.com/ziyue67/tms/go-backend/internal/httpapi"
	"github.com/ziyue67/tms/go-backend/internal/maintenance"
	"github.com/ziyue67/tms/go-backend/internal/nodehub"
	"github.com/ziyue67/tms/go-backend/internal/payment"
	"github.com/ziyue67/tms/go-backend/internal/store"
	"github.com/ziyue67/tms/go-backend/internal/verification"
)

var (
	buildCommit = "dev"
	buildTime   = ""
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelStartup()

	db, dialect, err := database.Open(startupCtx, cfg.Database)
	if err != nil {
		logger.Error("database startup failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := database.Migrate(startupCtx, db, dialect); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	redisClient, err := httpapi.NewRedis(startupCtx, cfg.Redis)
	if err != nil {
		logger.Warn("redis is unavailable; email verification remains disabled", "error", err)
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	repository := store.New(db, dialect)
	tokens := auth.NewTokenService(cfg.JWTSecret, 90*24*time.Hour)
	verificationService := verification.New(cfg.Auth, redisClient, repository, nil)
	nodeHub := nodehub.New(repository, tokens, logger)
	paymentService := payment.New(repository)
	scheduler := maintenance.New(repository, nodeHub, logger)
	scheduler.Start(context.Background())
	defer scheduler.Close()
	handler := httpapi.New(httpapi.Dependencies{
		Config:       cfg,
		Store:        repository,
		Tokens:       tokens,
		Redis:        redisClient,
		Verification: verificationService,
		NodeHub:      nodeHub,
		Payments:     paymentService,
		Logger:       logger,
		BuildCommit:  buildCommit,
		BuildTime:    buildTime,
	})

	server := &http.Server{
		Addr:              cfg.ListenAddress(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server started", "address", server.Addr, "commit", buildCommit)
		serverErrors <- server.ListenAndServe()
	}()

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err = <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-shutdownSignal.Done():
		logger.Info("server shutdown requested")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}
