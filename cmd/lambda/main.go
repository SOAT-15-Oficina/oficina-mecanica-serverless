// Funcao de autenticacao da plataforma.
//
// Serve POST /auth/login e POST /auth/register atras do API Gateway. Le e
// escreve na tabela `users` do mesmo RDS que o oficina-mecanica-monolith usa --
// o schema e propriedade dele; esta funcao nao roda migration nenhuma.
package main

import (
	"context"
	"log"
	"time"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/config"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/handler"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/repository"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Tudo aqui roda uma vez por container, nao por invocacao: em container
	// reutilizado o pool e o segredo ja estao prontos.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	pool, err := newPool(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	authHandler := handler.NewAuthHandler(
		service.NewAuthService(repository.NewUserRepository(pool), cfg.JWTSecret),
	)

	lambda.Start(authHandler.Handle)
}

func newPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.Database.DSN())
	if err != nil {
		return nil, err
	}

	// Concorrencia de Lambda multiplica conexoes: cada container quente
	// mantem o proprio pool. Teto baixo por container evita esgotar o
	// max_connections do RDS. Se o volume de login crescer, o proximo passo e
	// RDS Proxy, nao aumentar este numero.
	poolCfg.MaxConns = cfg.Database.MaxConnections

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
