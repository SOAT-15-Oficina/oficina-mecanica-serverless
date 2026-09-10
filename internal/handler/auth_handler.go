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
//
// E tambem o unico lugar onde a observabilidade da invocacao e montada: e o
// equivalente ao middleware do monolito (ADR-0011, secao 2), no formato que
// uma Lambda permite.
func (h *AuthHandler) Handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	start := time.Now()

	method, route := methodAndRoute(req)

	// O `requestId` do evento E o `$context.requestId` do access log do API
	// Gateway (ADR-0011, secao 5). Nao ha header a ler nem UUID a gerar: os dois
	// lados ja nascem com o mesmo valor, que e o que faz uma consulta por
	// `@request_id` devolver a linha da borda e a linha de dentro da funcao.
	//
	// O UUID e so para invocacao direta (o smoke check do CI, um `lambda
	// invoke` manual), onde nao ha gateway nenhum.
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

	// A linha de acesso: e dela que sai `oficina.http_request_duration`, o
	// painel de latencia por rota. Uma por invocacao, sempre -- inclusive nas
	// que falharam, que sao as que interessam.
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
		// O `error` e o `integration` desta falha ja sairam no servico, junto do
		// que ele sabe e este nivel nao sabe (se veio do banco ou do hash).
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

// methodAndRoute separa "POST /auth/login" em suas duas metades.
//
// O `route` guarda so o caminho, e nao a RouteKey inteira, para casar com o
// `@route` que o monolito emite -- la ele e o padrao da rota do Fiber. Um
// painel de latencia por rota que misturasse "POST /auth/login" com
// "/work-orders/:id" teria duas convencoes no mesmo eixo.
func methodAndRoute(req events.APIGatewayV2HTTPRequest) (method, route string) {
	method, route = req.RequestContext.HTTP.Method, req.RawPath

	if key := strings.TrimSpace(req.RouteKey); key != "" {
		if verb, path, ok := strings.Cut(key, " "); ok {
			return verb, path
		}
	}

	return method, route
}

// accessLevel traduz o status HTTP para o nivel da linha de acesso.
//
// E o que permite alertar sobre `status:error` sem alertar sobre todo trafego:
// uma credencial recusada (401) e um aviso, um erro interno (500) nao e.
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

// millisSince devolve a duracao em milissegundos com casas decimais. Inteiro
// arredondaria para 0 a maior parte das invocacoes em container quente, e uma
// distribuicao de zeros nao responde nada sobre latencia.
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
