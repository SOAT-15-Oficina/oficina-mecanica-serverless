package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/aws/aws-lambda-go/events"
)

type RegisterRequest struct {
	Username string          `json:"username"`
	Password string          `json:"password"`
	Role     domain.UserRole `json:"role"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Handle roteia pela RouteKey do API Gateway. Sao duas rotas numa unica funcao:
// duas Lambdas separadas dobrariam infraestrutura e cold starts sem ganho.
func (h *AuthHandler) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	switch req.RouteKey {
	case "POST /auth/login":
		return h.login(ctx, req)
	case "POST /auth/register":
		return h.register(ctx, req)
	default:
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "route not found"})
	}
}

func (h *AuthHandler) login(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var body LoginRequest
	if err := decodeBody(req, &body); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	token, err := h.svc.Login(ctx, body.Username, body.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		}
		log.Printf("login failed: %v", err)
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return jsonResponse(http.StatusOK, map[string]string{"token": token})
}

func (h *AuthHandler) register(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var body RegisterRequest
	if err := decodeBody(req, &body); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	user, err := h.svc.Register(ctx, body.Username, body.Password, body.Role)
	if err != nil {
		var validation *service.ValidationError
		switch {
		case errors.As(err, &validation):
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": validation.Message})
		case errors.Is(err, service.ErrUsernameTaken):
			return jsonResponse(http.StatusConflict, map[string]string{"error": "username already taken"})
		default:
			log.Printf("register failed: %v", err)
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	return jsonResponse(http.StatusCreated, user)
}

func decodeBody(req events.APIGatewayV2HTTPRequest, target any) error {
	body, err := rawBody(req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("invalid json body")
	}
	return nil
}

func jsonResponse(status int, payload any) (events.APIGatewayV2HTTPResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal response: %v", err)
		return events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusInternalServerError,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"internal server error"}`,
		}, nil
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(body),
	}, nil
}
