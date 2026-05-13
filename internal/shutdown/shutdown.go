package shutdown

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func Wait(logger *zap.Logger, stop func(context.Context) error) {
	quit := make(chan os.Signal, 1)

	signal.Notify(
		quit,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-quit

	logger.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := stop(ctx); err != nil {
		logger.Error("shutdown failed", zap.Error(err))
		return
	}

	logger.Info("server shutdown complete")
}
