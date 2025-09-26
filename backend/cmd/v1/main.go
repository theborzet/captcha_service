package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/theborzet/captcha_service/internal/app"
	"github.com/theborzet/captcha_service/internal/config"
	"github.com/theborzet/captcha_service/pkg/utils"
)

func main() {
	cfg := config.MustLoad()

	log := utils.SetupLogger(cfg.Logging.Level)

	application := app.New(log, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-stop
		log.Info("Received shutdown signal")
		cancel()
	}()

	if err := application.Run(ctx); err != nil {
		log.Error("Application stopped with error", slog.Any("error", err))
		os.Exit(1)
	}

	log.Info("Gracefully stopped")
}
