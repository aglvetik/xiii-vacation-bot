package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"xiii-vacation-bot/internal/bot"
	"xiii-vacation-bot/internal/config"
	"xiii-vacation-bot/internal/database"
	"xiii-vacation-bot/internal/logger"
)

func main() {
	cfg, err := config.Load()
	log := logger.New(logger.LevelFromString(os.Getenv("LOG_LEVEL")))
	if err != nil {
		log.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log = logger.New(logger.LevelFromString(cfg.LogLevel))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		log.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("failed to close database", slog.String("error", err.Error()))
		}
	}()

	client, err := bot.New(cfg, db, log)
	if err != nil {
		log.Error("failed to create bot", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err := client.Start(ctx); err != nil {
		log.Error("failed to start bot", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("bot started")
	<-ctx.Done()
	log.Info("shutting down")
	client.Stop()
}
