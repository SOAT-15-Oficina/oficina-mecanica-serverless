package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/repository"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercita o adaptador contra um Postgres de verdade. A tabela `users` e criada
// por tests/bootstrap.sql -- um recorte do schema cuja fonte da verdade e o
// oficina-mecanica-monolith.
func connectTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("teste de integracao: exige Postgres")
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL nao definida; pulando teste de integracao")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(context.Background()))

	t.Cleanup(pool.Close)

	return pool
}

func uniqueUsername(t *testing.T) string {
	t.Helper()
	return "user-" + uuid.NewString()[:8]
}

func TestIntegration_CreateAndFindByUsername(t *testing.T) {
	pool := connectTestDB(t)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()
	username := uniqueUsername(t)

	created, err := repo.Create(ctx, &domain.User{
		Username:     username,
		PasswordHash: "$argon2id$fake",
		Role:         domain.UserRoleEmployee,
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, created.ID)

	found, err := repo.FindByUsername(ctx, username)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "$argon2id$fake", found.PasswordHash)
	assert.Equal(t, domain.UserRoleEmployee, found.Role)
}

// A constraint users_username_key precisa virar ErrUsernameTaken, senao o
// handler devolve 500 no lugar de 409.
func TestIntegration_CreateDuplicateUsernameMapsToDomainError(t *testing.T) {
	pool := connectTestDB(t)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()
	username := uniqueUsername(t)

	user := func() *domain.User {
		return &domain.User{Username: username, PasswordHash: "$argon2id$fake", Role: domain.UserRoleAdmin}
	}

	_, err := repo.Create(ctx, user())
	require.NoError(t, err)

	_, err = repo.Create(ctx, user())

	assert.ErrorIs(t, err, service.ErrUsernameTaken)
}

// Usuario inexistente precisa chegar ao service como pgx.ErrNoRows, que e o que
// ele traduz em ErrInvalidCredentials.
func TestIntegration_FindByUsernameNotFound(t *testing.T) {
	pool := connectTestDB(t)
	repo := repository.NewUserRepository(pool)

	found, err := repo.FindByUsername(context.Background(), "nao-existe-"+uuid.NewString())

	assert.Nil(t, found)
	assert.ErrorIs(t, err, pgx.ErrNoRows)
}
