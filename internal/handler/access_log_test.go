package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/observability"
	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func handleWithLog(t *testing.T, req events.APIGatewayV2HTTPRequest, svc *stubAuthService) map[string]any {
	t.Helper()
	t.Setenv("DD_ENV", "prod")

	var buf bytes.Buffer
	ctx := observability.WithLogger(context.Background(), observability.New(&buf))

	_, err := NewAuthHandler(svc).Handle(ctx, req)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &out))
	return out
}

func TestHandle_EmitsAccessLine(t *testing.T) {
	req := request("POST /auth/login", `{"username":"a","password":"b"}`)
	req.RequestContext.RequestID = "id-vindo-do-gateway"

	line := handleWithLog(t, req, &stubAuthService{token: "tok"})

	assert.Equal(t, "request", line["msg"])
	assert.Equal(t, "/auth/login", line[observability.KeyRoute])
	assert.Equal(t, "POST", line[observability.KeyMethod])
	assert.Equal(t, float64(200), line[observability.KeyStatus])
	assert.Equal(t, float64(200), line[observability.KeyHTTPStatusCode])
	assert.GreaterOrEqual(t, line[observability.KeyDurationMS], float64(0))
	assert.Equal(t, "INFO", line["level"])
}

func TestHandle_ReusesTheGatewayRequestID(t *testing.T) {
	req := request("POST /auth/login", `{"username":"a","password":"b"}`)
	req.RequestContext.RequestID = "id-vindo-do-gateway"

	line := handleWithLog(t, req, &stubAuthService{token: "tok"})

	assert.Equal(t, "id-vindo-do-gateway", line[observability.KeyRequestID])
}

func TestHandle_GeneratesARequestIDWhenTheEventHasNone(t *testing.T) {
	line := handleWithLog(t, request("POST /auth/login", `{"username":"a","password":"b"}`), &stubAuthService{token: "tok"})

	assert.NotEmpty(t, line[observability.KeyRequestID])
}

func TestHandle_AccessLineLevelFollowsTheStatus(t *testing.T) {
	line := handleWithLog(t, request("GET /rota-inexistente", ""), &stubAuthService{})

	assert.Equal(t, float64(404), line[observability.KeyStatus])
	assert.Equal(t, "/rota-inexistente", line[observability.KeyRoute])
	assert.Equal(t, "GET", line[observability.KeyMethod])
	assert.Equal(t, "WARN", line["level"])
}
