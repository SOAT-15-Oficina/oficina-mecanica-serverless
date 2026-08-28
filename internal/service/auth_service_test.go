package service

import (
	"context"
	"errors"
	"testing"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/auth"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret"

type mockUserRepo struct{ mock.Mock }

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	args := m.Called(ctx, user)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockUserRepo) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

// --- Register ---

func TestRegister_Success(t *testing.T) {
	repo := new(mockUserRepo)
	svc := NewAuthService(repo, testSecret)
	created := &domain.User{ID: uuid.New(), Username: "alice", Role: domain.UserRoleAdmin}
	repo.On("Create", mock.Anything, mock.Anything).Return(created, nil)

	got, err := svc.Register(context.Background(), "alice", "pass123", domain.UserRoleAdmin)

	require.NoError(t, err)
	assert.Equal(t, "alice", got.Username)
	repo.AssertExpectations(t)
}

// A senha nunca pode chegar ao repositorio em texto claro.
func TestRegister_PersistsHashNotPlaintext(t *testing.T) {
	repo := new(mockUserRepo)
	svc := NewAuthService(repo, testSecret)

	var persisted *domain.User
	repo.On("Create", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { persisted = args.Get(1).(*domain.User) }).
		Return(&domain.User{Username: "alice"}, nil)

	_, err := svc.Register(context.Background(), "alice", "pass123", domain.UserRoleEmployee)

	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.NotEqual(t, "pass123", persisted.PasswordHash)
	assert.NoError(t, verifyPassword("pass123", persisted.PasswordHash))
}

func TestRegister_Validation(t *testing.T) {
	cases := []struct {
		name     string
		username string
		password string
		role     domain.UserRole
	}{
		{"username vazio", "", "pass", domain.UserRoleAdmin},
		{"password vazio", "alice", "", domain.UserRoleAdmin},
		{"role invalida", "alice", "pass", "superuser"},
		{"role vazia", "alice", "pass", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(mockUserRepo)
			svc := NewAuthService(repo, testSecret)

			got, err := svc.Register(context.Background(), tc.username, tc.password, tc.role)

			assert.Nil(t, got)
			var validation *ValidationError
			assert.ErrorAs(t, err, &validation)
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}
}

func TestRegister_UsernameTaken(t *testing.T) {
	repo := new(mockUserRepo)
	svc := NewAuthService(repo, testSecret)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil, ErrUsernameTaken)

	got, err := svc.Register(context.Background(), "alice", "pass", domain.UserRoleAdmin)

	assert.Nil(t, got)
	assert.ErrorIs(t, err, ErrUsernameTaken)
}

// --- Login ---

func TestLogin_ReturnsTokenTheMonolithCanParse(t *testing.T) {
	hash, err := hashPassword("correct-pass")
	require.NoError(t, err)

	repo := new(mockUserRepo)
	repo.On("FindByUsername", mock.Anything, "alice").
		Return(&domain.User{ID: uuid.New(), Username: "alice", PasswordHash: hash, Role: domain.UserRoleAdmin}, nil)
	svc := NewAuthService(repo, testSecret)

	token, err := svc.Login(context.Background(), "alice", "correct-pass")
	require.NoError(t, err)

	claims, err := auth.ParseToken(token, testSecret)
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.User)
	assert.Equal(t, "admin", claims.Role)
}

// Usuario inexistente e senha errada precisam ser indistinguiveis para quem
// chama, senao o endpoint vira um enumerador de usuarios.
func TestLogin_UnknownUserAndWrongPasswordAreIndistinguishable(t *testing.T) {
	missing := new(mockUserRepo)
	missing.On("FindByUsername", mock.Anything, "ghost").Return(nil, pgx.ErrNoRows)

	hash, err := hashPassword("correct")
	require.NoError(t, err)
	wrong := new(mockUserRepo)
	wrong.On("FindByUsername", mock.Anything, "alice").
		Return(&domain.User{Username: "alice", PasswordHash: hash, Role: domain.UserRoleAdmin}, nil)

	_, errMissing := NewAuthService(missing, testSecret).Login(context.Background(), "ghost", "any")
	_, errWrong := NewAuthService(wrong, testSecret).Login(context.Background(), "alice", "errada")

	assert.ErrorIs(t, errMissing, ErrInvalidCredentials)
	assert.ErrorIs(t, errWrong, ErrInvalidCredentials)
	assert.Equal(t, errMissing.Error(), errWrong.Error())
}

// Falha de banco nao pode ser reportada como credencial invalida: viraria 401
// no lugar de 500 e esconderia indisponibilidade.
func TestLogin_RepositoryErrorIsNotCredentialError(t *testing.T) {
	repo := new(mockUserRepo)
	dbErr := errors.New("connection refused")
	repo.On("FindByUsername", mock.Anything, "alice").Return(nil, dbErr)
	svc := NewAuthService(repo, testSecret)

	token, err := svc.Login(context.Background(), "alice", "pass")

	assert.Empty(t, token)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidCredentials)
	assert.ErrorIs(t, err, dbErr)
}

func TestLogin_EmptySecretFailsInsteadOfIssuingUnsignedToken(t *testing.T) {
	hash, err := hashPassword("pass")
	require.NoError(t, err)

	repo := new(mockUserRepo)
	repo.On("FindByUsername", mock.Anything, "alice").
		Return(&domain.User{Username: "alice", PasswordHash: hash, Role: domain.UserRoleAdmin}, nil)
	svc := NewAuthService(repo, "")

	token, err := svc.Login(context.Background(), "alice", "pass")

	assert.Empty(t, token)
	assert.Error(t, err)
}
