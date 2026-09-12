package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/auth"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/observability"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUsernameTaken      = errors.New("username already taken")
)

// ValidationError separa "entrada malformada" (400) de "credencial errada"
// (401) e de "falha interna" (500) sem o handler precisar inspecionar strings.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func NewValidationError(message string) error { return &ValidationError{Message: message} }

// UserRepository e a porta de persistencia. Esta funcao so precisa de duas
// operacoes: criar usuario e buscar por username. O CRUD administrativo de
// `/users` ficou no oficina-mecanica-monolith.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
}

type AuthService interface {
	Register(ctx context.Context, username, password string, role domain.UserRole) (*domain.User, error)
	Login(ctx context.Context, username, password string) (string, error)
}

type authService struct {
	repo         UserRepository
	jwtSecretKey string
}

func NewAuthService(repo UserRepository, jwtSecretKey string) AuthService {
	return &authService{repo: repo, jwtSecretKey: jwtSecretKey}
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

	hash, err := hashPassword(password)
	if err != nil {
		logger.ErrorContext(ctx, "hash password", observability.Err(err))
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.Create(ctx, &domain.User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	})
	if err != nil {
		if !errors.Is(err, ErrUsernameTaken) {
			logger.ErrorContext(ctx, "create user",
				observability.Integration(observability.IntegrationRDS),
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

	user, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Mesma resposta de senha errada: nao revela quais usuarios existem.
			logger.WarnContext(ctx, "login failed: unknown user",
				observability.Event(observability.EventLoginFailed),
				slog.String(observability.KeyUser, username))
			return "", ErrInvalidCredentials
		}

		logger.ErrorContext(ctx, "find user by username",
			observability.Integration(observability.IntegrationRDS),
			observability.Err(err))
		return "", err
	}

	if err := verifyPassword(password, user.PasswordHash); err != nil {
		logger.WarnContext(ctx, "login failed: wrong password",
			observability.Event(observability.EventLoginFailed),
			slog.String(observability.KeyUser, username))
		return "", ErrInvalidCredentials
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
