package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type Database struct {
	User           string
	Password       string
	Host           string
	Port           string
	Name           string
	MaxConnections int32
}

type Config struct {
	Database  Database
	JWTSecret string
}

// secretsClient e a fatia da API do Secrets Manager que este pacote usa.
type secretsClient interface {
	GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// dbSecret e o formato que o Secrets Manager guarda para a credencial do RDS.
type dbSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Load monta a configuracao a partir do ambiente. Host, porta e nome do banco
// vem de variaveis (o Terraform as injeta a partir dos outputs do RDS);
// credencial e JWT secret vem do Secrets Manager, cujos ARNs chegam por
// DATABASE_SECRET_ID e JWT_SECRET_ID.
//
// A funcao roda na inicializacao do container, nao por invocacao: em container
// reutilizado o segredo ja esta em memoria e nao ha chamada extra.
func Load(ctx context.Context) (*Config, error) {
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
	cfg := &Config{
		Database: Database{
			Host:           os.Getenv("DATABASE_HOST"),
			Port:           envOr("DATABASE_PORT", "5432"),
			Name:           os.Getenv("DATABASE_NAME"),
			MaxConnections: int32(envIntOr("DATABASE_MAX_CONNECTIONS", 2)),
		},
	}

	// Escape hatch para desenvolvimento local (docker-compose com RIE): sem
	// AWS por perto, os valores vem direto do ambiente.
	if id := os.Getenv("DATABASE_SECRET_ID"); id != "" {
		raw, err := fetchSecret(ctx, client, id)
		if err != nil {
			return nil, err
		}
		var s dbSecret
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return nil, fmt.Errorf("config: database secret is not valid json: %w", err)
		}
		cfg.Database.User, cfg.Database.Password = s.Username, s.Password
	} else {
		cfg.Database.User = os.Getenv("DATABASE_USER")
		cfg.Database.Password = os.Getenv("DATABASE_PASSWORD")
	}

	if id := os.Getenv("JWT_SECRET_ID"); id != "" {
		raw, err := fetchSecret(ctx, client, id)
		if err != nil {
			return nil, err
		}
		cfg.JWTSecret = raw
	} else {
		cfg.JWTSecret = os.Getenv("JWT_SECRET_KEY")
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("config: jwt secret is not configured")
	}
	if cfg.Database.Host == "" || cfg.Database.Name == "" {
		return nil, fmt.Errorf("config: DATABASE_HOST and DATABASE_NAME are required")
	}

	return cfg, nil
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

func (d Database) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
