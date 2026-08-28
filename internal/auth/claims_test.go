package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-key"

func TestGenerateToken_Success(t *testing.T) {
	token, err := GenerateToken("alice", "admin", testSecret)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateToken_EmptySecret(t *testing.T) {
	token, err := GenerateToken("alice", "admin", "")
	assert.Error(t, err)
	assert.Empty(t, token)
}

func TestParseToken_Success(t *testing.T) {
	token, err := GenerateToken("alice", "admin", testSecret)
	require.NoError(t, err)

	claims, err := ParseToken(token, testSecret)
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.User)
	assert.Equal(t, "admin", claims.Role)
}

func TestParseToken_EmptySecret(t *testing.T) {
	token, err := GenerateToken("alice", "admin", testSecret)
	require.NoError(t, err)

	claims, err := ParseToken(token, "")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestParseToken_WrongSecret(t *testing.T) {
	token, err := GenerateToken("alice", "admin", testSecret)
	require.NoError(t, err)

	claims, err := ParseToken(token, "wrong-secret")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestParseToken_InvalidToken(t *testing.T) {
	claims, err := ParseToken("not.a.token", testSecret)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestParseToken_ExpiredToken(t *testing.T) {
	raw := AppClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
		Role: "admin",
		User: "alice",
	}
	t.Run("expired token rejected", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, raw)
		signed, err := tok.SignedString([]byte(testSecret))
		require.NoError(t, err)

		claims, err := ParseToken(signed, testSecret)
		assert.Error(t, err)
		assert.Nil(t, claims)
	})
}

func TestParseToken_MissingRole(t *testing.T) {
	raw := AppClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "",
		User: "alice",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, raw)
	signed, err := tok.SignedString([]byte(testSecret))
	require.NoError(t, err)

	claims, err := ParseToken(signed, testSecret)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestParseToken_MissingUser(t *testing.T) {
	raw := AppClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role: "admin",
		User: "",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, raw)
	signed, err := tok.SignedString([]byte(testSecret))
	require.NoError(t, err)

	claims, err := ParseToken(signed, testSecret)
	assert.Error(t, err)
	assert.Nil(t, claims)
}
