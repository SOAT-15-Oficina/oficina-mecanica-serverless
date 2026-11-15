package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/auth"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/observability"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUsernameTaken      = errors.New("username already taken")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func NewValidationError(message string) error { return &ValidationError{Message: message} }

type UserDirectory interface {
	VerifyCredentials(ctx context.Context, username, password string) (*domain.User, error)
	CreateUser(ctx context.Context, username, password string, role domain.UserRole) (*domain.User, error)
}

type AuthService interface {
	Register(ctx context.Context, username, password string, role domain.UserRole) (*domain.User, error)
	Login(ctx context.Context, username, password string) (string, error)
}

type authService struct {
	users        UserDirectory
	jwtSecretKey string
}

func NewAuthService(users UserDirectory, jwtSecretKey string) AuthService {
	return &authService{users: users, jwtSecretKey: jwtSecretKey}
}

func (s *authService) Register(ctx context.Context, username, password string, role domain.UserRole) (*domain.User, error) {
	if username == "" {
		return nil, NewValidationError("username is required")
	}
	if password == "" {
		return nil, NewValidationError("password is required")
	}
	if role != domain.UserRoleAdmin && role != domain.UserRoleEmployee {
		return nil, NewValidationError("invalid role: must be 'admin' or 'employee'")
	}

	logger := observability.FromContext(ctx)

	user, err := s.users.CreateUser(ctx, username, password, role)
	if err != nil {
		var validation *ValidationError
		if !errors.Is(err, ErrUsernameTaken) && !errors.As(err, &validation) {
			logger.ErrorContext(ctx, "create user",
				observability.Integration(observability.IntegrationOSService),
				observability.Err(err))
		}
		return nil, err
	}

	logger.InfoContext(ctx, "user registered",
		slog.String(observability.KeyUser, user.Username),
		slog.String(observability.KeyRole, string(user.Role)))

	return user, nil
}

func (s *authService) Login(ctx context.Context, username, password string) (string, error) {
	logger := observability.FromContext(ctx)

	if username == "" || password == "" {
		logger.WarnContext(ctx, "login failed: missing credentials",
			observability.Event(observability.EventLoginFailed),
			slog.String(observability.KeyUser, username))
		return "", ErrInvalidCredentials
	}

	user, err := s.users.VerifyCredentials(ctx, username, password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			logger.WarnContext(ctx, "login failed: invalid credentials",
				observability.Event(observability.EventLoginFailed),
				slog.String(observability.KeyUser, username))
			return "", ErrInvalidCredentials
		}

		logger.ErrorContext(ctx, "verify credentials",
			observability.Integration(observability.IntegrationOSService),
			observability.Err(err))
		return "", err
	}

	token, err := auth.GenerateToken(user.Username, string(user.Role), s.jwtSecretKey)
	if err != nil {
		logger.ErrorContext(ctx, "generate token", observability.Err(err))
		return "", err
	}

	logger.InfoContext(ctx, "login succeeded",
		slog.String(observability.KeyUser, user.Username),
		slog.String(observability.KeyRole, string(user.Role)))

	return token, nil
}
