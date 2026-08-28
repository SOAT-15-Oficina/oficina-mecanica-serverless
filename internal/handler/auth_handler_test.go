package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAuthService struct {
	token    string
	user     *domain.User
	loginErr error
	regErr   error
}

func (s *stubAuthService) Register(context.Context, string, string, domain.UserRole) (*domain.User, error) {
	return s.user, s.regErr
}

func (s *stubAuthService) Login(context.Context, string, string) (string, error) {
	return s.token, s.loginErr
}

func request(routeKey, body string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{RouteKey: routeKey, Body: body}
}

func decode(t *testing.T, resp events.APIGatewayV2HTTPResponse) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Body), &out))
	return out
}

// --- roteamento ---

func TestHandle_UnknownRouteKey(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{})

	resp, err := h.Handle(context.Background(), request("GET /qualquer", ""))

	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestHandle_AlwaysReturnsJSONContentType(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{token: "tok"})

	resp, err := h.Handle(context.Background(), request("POST /auth/login", `{"username":"a","password":"b"}`))

	require.NoError(t, err)
	assert.Equal(t, "application/json", resp.Headers["Content-Type"])
}

// --- login ---

func TestLogin_Success(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{token: "jwt-token"})

	resp, err := h.Handle(context.Background(), request("POST /auth/login", `{"username":"a","password":"b"}`))

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "jwt-token", decode(t, resp)["token"])
}

func TestLogin_InvalidCredentialsIs401(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{loginErr: service.ErrInvalidCredentials})

	resp, err := h.Handle(context.Background(), request("POST /auth/login", `{"username":"a","password":"b"}`))

	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

// Falha interna nao pode virar 401, e a mensagem nao pode vazar detalhe do
// banco para quem chama.
func TestLogin_InternalErrorIs500AndDoesNotLeak(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{loginErr: errors.New("dial tcp 10.0.3.14:5432: connection refused")})

	resp, err := h.Handle(context.Background(), request("POST /auth/login", `{"username":"a","password":"b"}`))

	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	assert.NotContains(t, resp.Body, "10.0.3.14")
	assert.Equal(t, "internal server error", decode(t, resp)["error"])
}

func TestLogin_MalformedJSONIs400(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{token: "tok"})

	resp, err := h.Handle(context.Background(), request("POST /auth/login", `{"username":`))

	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

// --- register ---

func TestRegister_Success(t *testing.T) {
	id := uuid.New()
	h := NewAuthHandler(&stubAuthService{user: &domain.User{ID: id, Username: "alice", Role: domain.UserRoleAdmin}})

	resp, err := h.Handle(context.Background(), request("POST /auth/register", `{"username":"alice","password":"p","role":"admin"}`))

	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	body := decode(t, resp)
	assert.Equal(t, id.String(), body["id"])
	assert.Equal(t, "alice", body["username"])
}

// O hash nunca pode sair na resposta: domain.User marca o campo com json:"-".
func TestRegister_ResponseOmitsPasswordHash(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{user: &domain.User{
		ID: uuid.New(), Username: "alice", PasswordHash: "$argon2id$segredo", Role: domain.UserRoleAdmin,
	}})

	resp, err := h.Handle(context.Background(), request("POST /auth/register", `{"username":"alice","password":"p","role":"admin"}`))

	require.NoError(t, err)
	assert.NotContains(t, resp.Body, "argon2id")
	assert.NotContains(t, resp.Body, "password_hash")
}

func TestRegister_ValidationErrorIs400WithMessage(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{regErr: service.NewValidationError("invalid role: must be 'admin' or 'employee'")})

	resp, err := h.Handle(context.Background(), request("POST /auth/register", `{"username":"a","password":"p","role":"root"}`))

	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, "invalid role: must be 'admin' or 'employee'", decode(t, resp)["error"])
}

func TestRegister_UsernameTakenIs409(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{regErr: service.ErrUsernameTaken})

	resp, err := h.Handle(context.Background(), request("POST /auth/register", `{"username":"a","password":"p","role":"admin"}`))

	require.NoError(t, err)
	assert.Equal(t, 409, resp.StatusCode)
}

// --- corpo base64 ---

// O API Gateway entrega o payload em base64 quando classifica o content-type
// como binario. Assumir texto puro quebraria o login sem erro visivel.
func TestHandle_DecodesBase64Body(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{token: "jwt-token"})
	req := events.APIGatewayV2HTTPRequest{
		RouteKey:        "POST /auth/login",
		Body:            base64.StdEncoding.EncodeToString([]byte(`{"username":"a","password":"b"}`)),
		IsBase64Encoded: true,
	}

	resp, err := h.Handle(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "jwt-token", decode(t, resp)["token"])
}

func TestHandle_InvalidBase64BodyIs400(t *testing.T) {
	h := NewAuthHandler(&stubAuthService{token: "tok"})
	req := events.APIGatewayV2HTTPRequest{
		RouteKey: "POST /auth/login", Body: "!!!nao-e-base64!!!", IsBase64Encoded: true,
	}

	resp, err := h.Handle(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}
