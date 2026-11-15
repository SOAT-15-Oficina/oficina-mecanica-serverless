package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type Config struct {
	JWTSecret            string
	OSBaseURL            string
	InternalServiceToken string
	OSTimeout            time.Duration
}

type secretsClient interface {
	GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

func Load(ctx context.Context) (*Config, error) {
	if os.Getenv("JWT_SECRET_ID") == "" && os.Getenv("INTERNAL_SERVICE_TOKEN_SECRET_ID") == "" {
		return loadWith(ctx, nil)
	}
	client, err := newSecretsClient(ctx)
	if err != nil {
		return nil, err
	}
	return loadWith(ctx, client)
}

func newSecretsClient(ctx context.Context) (secretsClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("config: load aws config: %w", err)
	}
	return secretsmanager.NewFromConfig(awsCfg), nil
}

func loadWith(ctx context.Context, client secretsClient) (*Config, error) {
	jwtSecret, err := secretOrEnv(ctx, client, "JWT_SECRET_ID", "JWT_SECRET_KEY")
	if err != nil {
		return nil, err
	}
	if jwtSecret == "" {
		return nil, fmt.Errorf("config: jwt secret is not configured")
	}

	token, err := secretOrEnv(ctx, client, "INTERNAL_SERVICE_TOKEN_SECRET_ID", "INTERNAL_SERVICE_TOKEN")
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("config: internal service token is not configured")
	}

	baseURL := strings.TrimSpace(os.Getenv("OS_INTERNAL_BASE_URL"))
	if baseURL == "" {
		return nil, fmt.Errorf("config: OS_INTERNAL_BASE_URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("config: OS_INTERNAL_BASE_URL must be an absolute http(s) url, got %q", baseURL)
	}

	timeout := 5 * time.Second
	if raw := os.Getenv("OS_INTERNAL_TIMEOUT"); raw != "" {
		parsedTimeout, err := time.ParseDuration(raw)
		if err != nil || parsedTimeout <= 0 {
			return nil, fmt.Errorf("config: OS_INTERNAL_TIMEOUT must be a positive duration, got %q", raw)
		}
		timeout = parsedTimeout
	}

	return &Config{
		JWTSecret:            jwtSecret,
		OSBaseURL:            strings.TrimRight(baseURL, "/"),
		InternalServiceToken: token,
		OSTimeout:            timeout,
	}, nil
}

func secretOrEnv(ctx context.Context, client secretsClient, idKey, valueKey string) (string, error) {
	id := os.Getenv(idKey)
	if id == "" {
		return os.Getenv(valueKey), nil
	}
	if client == nil {
		return "", fmt.Errorf("config: %s is set but there is no secrets manager client", idKey)
	}
	return fetchSecret(ctx, client, id)
}

func fetchSecret(ctx context.Context, client secretsClient, id string) (string, error) {
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &id})
	if err != nil {
		return "", fmt.Errorf("config: read secret %q: %w", id, err)
	}
	if out.SecretString == nil {
		return "", fmt.Errorf("config: secret %q has no string value", id)
	}
	return *out.SecretString, nil
}
