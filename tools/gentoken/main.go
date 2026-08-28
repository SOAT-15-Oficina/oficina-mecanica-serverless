// Gera o fixture de contrato do token compartilhado entre este repositorio (que
// emite) e o oficina-mecanica-monolith (que valida).
//
//	go run ./tools/gentoken > internal/auth/testdata/token.golden
//
// Depois de regenerar, copie o arquivo para
// oficina-mecanica-monolith/internal/auth/testdata/token.golden NO MESMO PR --
// os testes de contrato dos dois repos leem o mesmo conteudo.
//
// A expiracao e fixa em 2099 para o fixture nao apodrecer.
package main

import (
	"fmt"
	"time"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/auth"
	"github.com/golang-jwt/jwt/v5"
)

const (
	contractSecret = "contract-test-secret"
	contractUser   = "contract-user"
	contractRole   = "admin"
)

func main() {
	claims := auth.AppClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)),
			IssuedAt:  jwt.NewNumericDate(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		Role: contractRole,
		User: contractUser,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(contractSecret))
	if err != nil {
		panic(err)
	}

	fmt.Print(signed)
}
