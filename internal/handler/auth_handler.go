package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/observability"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
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
	start := time.Now()

	method, route := methodAndRoute(req)

	requestID := req.RequestContext.RequestID
	if requestID == "" {
		requestID = uuid.NewString()
	}

	logger := observability.FromContext(ctx).With(
		slog.String(observability.KeyRequestID, requestID),
		slog.String(observability.KeyRoute, route),
		slog.String(observability.KeyMethod, method),
	)
	ctx = observability.WithLogger(ctx, logger)

	resp, err := h.dispatch(ctx, req)

	logger.LogAttrs(ctx, accessLevel(resp.StatusCode), "request",
		slog.Int(observability.KeyStatus, resp.StatusCode),
		slog.Int(observability.KeyHTTPStatusCode, resp.StatusCode),
		slog.Float64(observability.KeyDurationMS, millisSince(start)),
	)

	return resp, err
}

func (h *AuthHandler) dispatch(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	switch req.RouteKey {
	case "POST /auth/login":
		return h.login(ctx, req)
	case "POST /auth/register":
		return h.register(ctx, req)
	default:
		return jsonResponse(ctx, http.StatusNotFound, map[string]string{"error": "route not found"})
	}
}

func (h *AuthHandler) login(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var body LoginRequest
	if err := decodeBody(req, &body); err != nil {
		return jsonResponse(ctx, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	token, err := h.svc.Login(ctx, body.Username, body.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return jsonResponse(ctx, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		}
		return jsonResponse(ctx, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return jsonResponse(ctx, http.StatusOK, map[string]string{"token": token})
}

func (h *AuthHandler) register(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	var body RegisterRequest
	if err := decodeBody(req, &body); err != nil {
		return jsonResponse(ctx, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	user, err := h.svc.Register(ctx, body.Username, body.Password, body.Role)
	if err != nil {
		var validation *service.ValidationError
		switch {
		case errors.As(err, &validation):
			return jsonResponse(ctx, http.StatusBadRequest, map[string]string{"error": validation.Message})
		case errors.Is(err, service.ErrUsernameTaken):
			return jsonResponse(ctx, http.StatusConflict, map[string]string{"error": "username already taken"})
		default:
			return jsonResponse(ctx, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	return jsonResponse(ctx, http.StatusCreated, user)
}

func methodAndRoute(req events.APIGatewayV2HTTPRequest) (method, route string) {
	method, route = req.RequestContext.HTTP.Method, req.RawPath

	if key := strings.TrimSpace(req.RouteKey); key != "" {
		if verb, path, ok := strings.Cut(key, " "); ok {
			return verb, path
		}
	}

	return method, route
}

func accessLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func millisSince(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
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

func jsonResponse(ctx context.Context, status int, payload any) (events.APIGatewayV2HTTPResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		observability.FromContext(ctx).ErrorContext(ctx, "marshal response", observability.Err(err))
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
