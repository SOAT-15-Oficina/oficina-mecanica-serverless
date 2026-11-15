package config

import (
	"context"
	"errors"
	"testing"
	"time"

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

func clearEnv(t *testing.T) {
	for _, key := range []string{"JWT_SECRET_ID", "JWT_SECRET_KEY", "INTERNAL_SERVICE_TOKEN_SECRET_ID", "INTERNAL_SERVICE_TOKEN", "OS_INTERNAL_BASE_URL", "OS_INTERNAL_TIMEOUT"} {
		t.Setenv(key, "")
	}
}

func localEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("JWT_SECRET_KEY", "local-secret")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "local-token")
	t.Setenv("OS_INTERNAL_BASE_URL", "http://os-service:8080/")
}

func TestLoad_FromSecretsManager(t *testing.T) {
	clearEnv(t)
	t.Setenv("JWT_SECRET_ID", "arn:jwt")
	t.Setenv("INTERNAL_SERVICE_TOKEN_SECRET_ID", "arn:token")
	t.Setenv("OS_INTERNAL_BASE_URL", "http://internal-alb.local")
	client := &stubSecrets{values: map[string]string{
		"arn:jwt":   "chave-de-assinatura",
		"arn:token": "token-de-servico",
	}}

	cfg, err := loadWith(context.Background(), client)

	require.NoError(t, err)
	assert.Equal(t, "chave-de-assinatura", cfg.JWTSecret)
	assert.Equal(t, "token-de-servico", cfg.InternalServiceToken)
	assert.Equal(t, "http://internal-alb.local", cfg.OSBaseURL)
	assert.Equal(t, 2, client.calls)
}

func TestLoad_LocalFallbackDoesNotCallAWS(t *testing.T) {
	localEnv(t)
	client := &stubSecrets{}

	cfg, err := loadWith(context.Background(), client)

	require.NoError(t, err)
	assert.Equal(t, "local-secret", cfg.JWTSecret)
	assert.Equal(t, "local-token", cfg.InternalServiceToken)
	assert.Equal(t, "http://os-service:8080", cfg.OSBaseURL)
	assert.Equal(t, 5*time.Second, cfg.OSTimeout)
	assert.Zero(t, client.calls)
}

func TestLoad_WithoutSecretIDsSkipsAWSClient(t *testing.T) {
	localEnv(t)

	cfg, err := Load(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "local-token", cfg.InternalServiceToken)
}

func TestLoad_SecretsManagerFailurePropagates(t *testing.T) {
	localEnv(t)
	t.Setenv("INTERNAL_SERVICE_TOKEN_SECRET_ID", "arn:token")
	awsErr := errors.New("AccessDeniedException")

	_, err := loadWith(context.Background(), &stubSecrets{err: awsErr})

	require.Error(t, err)
	assert.ErrorIs(t, err, awsErr)
}

func TestLoad_JWTSecretFailurePropagates(t *testing.T) {
	localEnv(t)
	t.Setenv("JWT_SECRET_ID", "arn:jwt")
	awsErr := errors.New("ResourceNotFoundException")

	_, err := loadWith(context.Background(), &stubSecrets{err: awsErr})

	assert.ErrorIs(t, err, awsErr)
}

func TestLoad_SecretWithoutStringValue(t *testing.T) {
	localEnv(t)
	t.Setenv("JWT_SECRET_ID", "arn:binary")

	_, err := loadWith(context.Background(), &stubSecrets{values: map[string]string{}})

	assert.ErrorContains(t, err, "has no string value")
}

func TestLoad_SecretIDWithoutClientFails(t *testing.T) {
	localEnv(t)
	t.Setenv("JWT_SECRET_ID", "arn:jwt")

	_, err := loadWith(context.Background(), nil)

	assert.ErrorContains(t, err, "no secrets manager client")
}

func TestLoad_MissingJWTSecretFails(t *testing.T) {
	localEnv(t)
	t.Setenv("JWT_SECRET_KEY", "")

	_, err := loadWith(context.Background(), &stubSecrets{})

	assert.ErrorContains(t, err, "jwt secret is not configured")
}

func TestLoad_MissingServiceTokenFails(t *testing.T) {
	localEnv(t)
	t.Setenv("INTERNAL_SERVICE_TOKEN", "")

	_, err := loadWith(context.Background(), &stubSecrets{})

	assert.ErrorContains(t, err, "internal service token is not configured")
}

func TestLoad_BaseURLValidation(t *testing.T) {
	cases := map[string]string{
		"vazia":          "",
		"sem esquema":    "os-service:8080",
		"esquema errado": "ftp://os-service",
		"sem host":       "http://",
		"nao e uma url":  "http://[::1",
		"so com espacos": "   ",
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			localEnv(t)
			t.Setenv("OS_INTERNAL_BASE_URL", value)

			_, err := loadWith(context.Background(), &stubSecrets{})

			assert.ErrorContains(t, err, "OS_INTERNAL_BASE_URL")
		})
	}
}

func TestLoad_Timeout(t *testing.T) {
	localEnv(t)
	t.Setenv("OS_INTERNAL_TIMEOUT", "2s")

	cfg, err := loadWith(context.Background(), &stubSecrets{})

	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, cfg.OSTimeout)
}

func TestLoad_InvalidTimeout(t *testing.T) {
	for _, value := range []string{"abc", "-1s", "0s"} {
		t.Run(value, func(t *testing.T) {
			localEnv(t)
			t.Setenv("OS_INTERNAL_TIMEOUT", value)

			_, err := loadWith(context.Background(), &stubSecrets{})

			assert.ErrorContains(t, err, "OS_INTERNAL_TIMEOUT")
		})
	}
}
