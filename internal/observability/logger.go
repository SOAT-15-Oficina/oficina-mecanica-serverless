package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
)

const DefaultService = "auth-lambda"

var version = "dev"

const (
	KeyService        = "service"
	KeyEnv            = "env"
	KeyVersion        = "version"
	KeyRequestID      = "request_id"
	KeyRoute          = "route"
	KeyMethod         = "method"
	KeyStatus         = "status"
	KeyDurationMS     = "duration_ms"
	KeyEvent          = "event"
	KeyIntegration    = "integration"
	KeyError          = "error"
	KeyUser           = "user"
	KeyRole           = "role"
	KeyHTTPStatusCode = "http.status_code"
)

const (
	EventLoginFailed = "auth.login_failed"
)

const (
	IntegrationRDS = "rds"
)

func Setup() *slog.Logger {
	logger := New(os.Stdout)
	slog.SetDefault(logger)
	return logger
}

func New(w io.Writer) *slog.Logger {
	env := envOr("DD_ENV", "local")
	service := envOr("DD_SERVICE", DefaultService)

	var handler slog.Handler
	if env == "local" {
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	}

	return slog.New(handler).With(
		slog.String(KeyService, service),
		slog.String(KeyEnv, env),
		slog.String(KeyVersion, version),
	)
}

func Version() string { return version }

type loggerKey struct{}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

func Event(name string) slog.Attr { return slog.String(KeyEvent, name) }

func Integration(name string) slog.Attr { return slog.String(KeyIntegration, name) }

func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String(KeyError, err.Error())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
