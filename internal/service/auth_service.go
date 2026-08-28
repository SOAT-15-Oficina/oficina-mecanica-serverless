package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/auth"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
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

	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	return s.repo.Create(ctx, &domain.User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	})
}

func (s *authService) Login(ctx context.Context, username, password string) (string, error) {
	user, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Mesma resposta de senha errada: nao revela quais usuarios existem.
			return "", ErrInvalidCredentials
		}
		return "", err
	}

	if err := verifyPassword(password, user.PasswordHash); err != nil {
		return "", ErrInvalidCredentials
	}

	return auth.GenerateToken(user.Username, string(user.Role), s.jwtSecretKey)
}
