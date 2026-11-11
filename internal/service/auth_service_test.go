package service

import (
	"context"
	"errors"
	"testing"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/auth"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret"

type mockDirectory struct{ mock.Mock }

func (m *mockDirectory) VerifyCredentials(ctx context.Context, username, password string) (*domain.User, error) {
	args := m.Called(ctx, username, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockDirectory) CreateUser(ctx context.Context, username, password string, role domain.UserRole) (*domain.User, error) {
	args := m.Called(ctx, username, password, role)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func TestRegister_DelegatesToOSService(t *testing.T) {
	users := new(mockDirectory)
	created := &domain.User{ID: "u-1", Username: "alice", Role: domain.UserRoleAdmin}
	users.On("CreateUser", mock.Anything, "alice", "pass123", domain.UserRoleAdmin).Return(created, nil)

	got, err := NewAuthService(users, testSecret).Register(context.Background(), "alice", "pass123", domain.UserRoleAdmin)

	require.NoError(t, err)
	assert.Equal(t, created, got)
	users.AssertExpectations(t)
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
			users := new(mockDirectory)

			got, err := NewAuthService(users, testSecret).Register(context.Background(), tc.username, tc.password, tc.role)

			assert.Nil(t, got)
			var validation *ValidationError
			assert.ErrorAs(t, err, &validation)
			users.AssertNotCalled(t, "CreateUser", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestRegister_PropagatesDirectoryErrors(t *testing.T) {
	cases := map[string]error{
		"duplicado": ErrUsernameTaken,
		"invalido":  NewValidationError("password too short"),
		"falha":     errors.New("osclient: call /internal/users: connection refused"),
	}

	for name, dirErr := range cases {
		t.Run(name, func(t *testing.T) {
			users := new(mockDirectory)
			users.On("CreateUser", mock.Anything, "alice", "pass", domain.UserRoleEmployee).Return(nil, dirErr)

			got, err := NewAuthService(users, testSecret).Register(context.Background(), "alice", "pass", domain.UserRoleEmployee)

			assert.Nil(t, got)
			assert.ErrorIs(t, err, dirErr)
		})
	}
}

func TestLogin_ReturnsTokenTheServicesCanParse(t *testing.T) {
	users := new(mockDirectory)
	users.On("VerifyCredentials", mock.Anything, "alice", "correct-pass").
		Return(&domain.User{ID: "u-1", Username: "alice", Role: domain.UserRoleAdmin}, nil)

	token, err := NewAuthService(users, testSecret).Login(context.Background(), "alice", "correct-pass")
	require.NoError(t, err)

	claims, err := auth.ParseToken(token, testSecret)
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.User)
	assert.Equal(t, "admin", claims.Role)
}

func TestLogin_InvalidCredentials(t *testing.T) {
	users := new(mockDirectory)
	users.On("VerifyCredentials", mock.Anything, "alice", "errada").Return(nil, ErrInvalidCredentials)

	token, err := NewAuthService(users, testSecret).Login(context.Background(), "alice", "errada")

	assert.Empty(t, token)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_MissingCredentialsDoNotCallOSService(t *testing.T) {
	for _, pair := range [][2]string{{"", "pass"}, {"alice", ""}} {
		users := new(mockDirectory)

		_, err := NewAuthService(users, testSecret).Login(context.Background(), pair[0], pair[1])

		assert.ErrorIs(t, err, ErrInvalidCredentials)
		users.AssertNotCalled(t, "VerifyCredentials", mock.Anything, mock.Anything, mock.Anything)
	}
}

func TestLogin_DirectoryFailureIsNotCredentialError(t *testing.T) {
	users := new(mockDirectory)
	upstream := errors.New("osclient: /internal/credentials/verify returned 503")
	users.On("VerifyCredentials", mock.Anything, "alice", "pass").Return(nil, upstream)

	token, err := NewAuthService(users, testSecret).Login(context.Background(), "alice", "pass")

	assert.Empty(t, token)
	assert.NotErrorIs(t, err, ErrInvalidCredentials)
	assert.ErrorIs(t, err, upstream)
}

func TestLogin_EmptySecretFailsInsteadOfIssuingUnsignedToken(t *testing.T) {
	users := new(mockDirectory)
	users.On("VerifyCredentials", mock.Anything, "alice", "pass").
		Return(&domain.User{Username: "alice", Role: domain.UserRoleAdmin}, nil)

	token, err := NewAuthService(users, "").Login(context.Background(), "alice", "pass")

	assert.Empty(t, token)
	assert.Error(t, err)
}

func TestValidationErrorMessage(t *testing.T) {
	assert.Equal(t, "bad", NewValidationError("bad").Error())
}
