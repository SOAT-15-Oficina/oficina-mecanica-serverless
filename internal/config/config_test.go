package config

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSecrets struct {
	values map[string]string
	err    error
	calls  int
}

func (s *stubSecrets) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	v, ok := s.values[*in.SecretId]
	if !ok {
		return &secretsmanager.GetSecretValueOutput{}, nil
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: &v}, nil
}

func baseEnv(t *testing.T) {
	t.Setenv("DATABASE_HOST", "db.internal")
	t.Setenv("DATABASE_NAME", "techchallenge-db")
}

func TestLoad_FromSecretsManager(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_SECRET_ID", "arn:db")
	t.Setenv("JWT_SECRET_ID", "arn:jwt")
	client := &stubSecrets{values: map[string]string{
		"arn:db":  `{"username":"tech","password":"s3cr3t"}`,
		"arn:jwt": "chave-de-assinatura",
	}}

	cfg, err := loadWith(context.Background(), client)

	require.NoError(t, err)
	assert.Equal(t, "tech", cfg.Database.User)
	assert.Equal(t, "s3cr3t", cfg.Database.Password)
	assert.Equal(t, "chave-de-assinatura", cfg.JWTSecret)
}

// Sem os IDs de secret configurados o codigo cai no ambiente -- e o caminho do
// docker-compose local com o Lambda RIE, onde nao ha AWS.
func TestLoad_LocalFallbackDoesNotCallAWS(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_USER", "local")
	t.Setenv("DATABASE_PASSWORD", "local")
	t.Setenv("JWT_SECRET_KEY", "local-secret")
	client := &stubSecrets{}

	cfg, err := loadWith(context.Background(), client)

	require.NoError(t, err)
	assert.Equal(t, "local", cfg.Database.User)
	assert.Equal(t, "local-secret", cfg.JWTSecret)
	assert.Zero(t, client.calls, "nao deve chamar o Secrets Manager sem os IDs")
}

func TestLoad_SecretsManagerFailurePropagates(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_SECRET_ID", "arn:db")
	awsErr := errors.New("AccessDeniedException")

	_, err := loadWith(context.Background(), &stubSecrets{err: awsErr})

	require.Error(t, err)
	assert.ErrorIs(t, err, awsErr)
}

func TestLoad_MalformedDatabaseSecret(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_SECRET_ID", "arn:db")
	client := &stubSecrets{values: map[string]string{"arn:db": "nao-e-json"}}

	_, err := loadWith(context.Background(), client)

	assert.ErrorContains(t, err, "not valid json")
}

// Sem JWT secret a funcao emitiria tokens que o monolito rejeita. Falhar no
// boot e melhor do que servir 100% de 401 depois.
func TestLoad_MissingJWTSecretFails(t *testing.T) {
	baseEnv(t)

	_, err := loadWith(context.Background(), &stubSecrets{})

	assert.ErrorContains(t, err, "jwt secret is not configured")
}

func TestLoad_MissingDatabaseCoordinatesFails(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "x")

	_, err := loadWith(context.Background(), &stubSecrets{})

	assert.ErrorContains(t, err, "DATABASE_HOST and DATABASE_NAME are required")
}

func TestDSN(t *testing.T) {
	db := Database{User: "u", Password: "p", Host: "h", Port: "5432", Name: "n"}

	assert.Equal(t, "postgres://u:p@h:5432/n", db.DSN())
}

func TestLoad_DefaultsPortAndPoolSize(t *testing.T) {
	baseEnv(t)
	t.Setenv("JWT_SECRET_KEY", "x")

	cfg, err := loadWith(context.Background(), &stubSecrets{})

	require.NoError(t, err)
	assert.Equal(t, "5432", cfg.Database.Port)
	assert.Equal(t, int32(2), cfg.Database.MaxConnections)
}
