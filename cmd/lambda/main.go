package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/config"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/handler"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/observability"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/osclient"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	logger := observability.Setup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		fatal(ctx, logger, "failed to load configuration", err)
	}

	users := osclient.New(cfg.OSBaseURL, cfg.InternalServiceToken, &http.Client{Timeout: cfg.OSTimeout})

	authHandler := handler.NewAuthHandler(service.NewAuthService(users, cfg.JWTSecret))

	lambda.Start(authHandler.Handle)
}

func fatal(ctx context.Context, logger *slog.Logger, msg string, err error) {
	logger.LogAttrs(ctx, slog.LevelError, msg, observability.Err(err))
	os.Exit(1)
}
