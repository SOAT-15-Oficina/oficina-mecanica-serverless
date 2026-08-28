package auth

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contrato de token entre este servico (emite) e o oficina-mecanica-monolith
// (valida). Os dois repositorios sao independentes e cada um tem sua copia de
// AppClaims/GenerateToken/ParseToken.
//
// `testdata/token.golden` e byte a byte o mesmo arquivo nos dois repos e e
// produzido por `go run ./tools/gentoken`. Se alguem renomear um claim, trocar
// o algoritmo de assinatura ou mexer no formato, o build quebra aqui e no
// monolito, em vez de virar 401 em producao.
const (
	contractSecret = "contract-test-secret"
	contractUser   = "contract-user"
	contractRole   = "admin"
)

func readGoldenToken(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile("testdata/token.golden")
	require.NoError(t, err)

	return strings.TrimSpace(string(raw))
}

// Se este teste falhar, `go run ./tools/gentoken > internal/auth/testdata/token.golden`
// e copie o arquivo para o monolito no mesmo PR.
func TestContract_GoldenTokenIsAcceptedByOurOwnParser(t *testing.T) {
	claims, err := ParseToken(readGoldenToken(t), contractSecret)

	require.NoError(t, err)
	assert.Equal(t, contractUser, claims.User)
	assert.Equal(t, contractRole, claims.Role)
}

func TestContract_GeneratedTokenCarriesTheAgreedClaims(t *testing.T) {
	token, err := GenerateToken("alguem", "employee", contractSecret)
	require.NoError(t, err)

	claims, err := ParseToken(token, contractSecret)
	require.NoError(t, err)
	assert.Equal(t, "alguem", claims.User)
	assert.Equal(t, "employee", claims.Role)
}

func TestContract_SigningMethodIsHS256(t *testing.T) {
	parts := strings.Split(readGoldenToken(t), ".")
	require.Len(t, parts, 3)

	assert.Equal(t, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", parts[0],
		"o monolito valida HS256; trocar o algoritmo exige mudar os dois repos")
}
