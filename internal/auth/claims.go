package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AppClaims struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
	User string `json:"user"`
}

func GenerateToken(username, role, secretKey string) (string, error) {
	if secretKey == "" {
		return "", fmt.Errorf("jwt secret is not configured")
	}

	claims := AppClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Role: role,
		User: username,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

func ParseToken(tokenString, secretKey string) (*AppClaims, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("jwt secret is not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &AppClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*AppClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	if claims.Role == "" {
		return nil, fmt.Errorf("missing or empty claim: role")
	}
	if claims.User == "" {
		return nil, fmt.Errorf("missing or empty claim: user")
	}

	return claims, nil
}
