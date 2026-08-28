package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerifyPassword_RoundTrip(t *testing.T) {
	hash, err := hashPassword("my-secret")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(hash, "$argon2id$"))
	assert.NoError(t, verifyPassword("my-secret", hash))
}

func TestHashPassword_SaltIsRandom(t *testing.T) {
	a, err := hashPassword("mesma-senha")
	require.NoError(t, err)
	b, err := hashPassword("mesma-senha")
	require.NoError(t, err)

	assert.NotEqual(t, a, b, "dois hashes da mesma senha nao podem coincidir")
	assert.NoError(t, verifyPassword("mesma-senha", a))
	assert.NoError(t, verifyPassword("mesma-senha", b))
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := hashPassword("correct")
	require.NoError(t, err)

	assert.ErrorIs(t, verifyPassword("wrong", hash), ErrInvalidCredentials)
}

func TestVerifyPassword_InvalidHashFormat(t *testing.T) {
	cases := map[string]string{
		"nao e um hash":        "not-a-valid-hash",
		"algoritmo errado":     "$bcrypt$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA",
		"versao malformada":    "$argon2id$vXX$m=65536,t=1,p=4$c2FsdA$aGFzaA",
		"parametros ilegiveis": "$argon2id$v=19$memoria$c2FsdA$aGFzaA",
		"salt nao e base64":    "$argon2id$v=19$m=65536,t=1,p=4$!!!$aGFzaA",
		"hash nao e base64":    "$argon2id$v=19$m=65536,t=1,p=4$c2FsdA$!!!",
	}

	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			err := verifyPassword("any", encoded)

			require.Error(t, err)
			// Formato invalido nao e "senha errada": nao pode ser confundido
			// com ErrInvalidCredentials, que o handler traduz em 401.
			assert.NotErrorIs(t, err, ErrInvalidCredentials)
		})
	}
}
