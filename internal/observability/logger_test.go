package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	return out
}

func TestNew_EmitsFixedFieldsAsJSON(t *testing.T) {
	t.Setenv("DD_ENV", "homolog")
	t.Setenv("DD_SERVICE", "auth-lambda")

	var buf bytes.Buffer
	New(&buf).Info("hello")

	line := decodeLine(t, &buf)
	assert.Equal(t, "auth-lambda", line[KeyService])
	assert.Equal(t, "homolog", line[KeyEnv])
	assert.Equal(t, Version(), line[KeyVersion])
	assert.Equal(t, "hello", line["msg"])
	assert.Equal(t, "INFO", line["level"])
}

func TestNew_FallsBackToTheDefaultServiceName(t *testing.T) {
	t.Setenv("DD_ENV", "prod")
	t.Setenv("DD_SERVICE", "")

	var buf bytes.Buffer
	New(&buf).Info("hello")

	assert.Equal(t, DefaultService, decodeLine(t, &buf)[KeyService])
}

func TestNew_UsesTextHandlerLocally(t *testing.T) {
	t.Setenv("DD_ENV", "")

	var buf bytes.Buffer
	New(&buf).Info("hello")

	assert.NotContains(t, buf.String(), `"msg"`)
	assert.Contains(t, buf.String(), "env=local")
}

func TestEventAndIntegrationUseTheContractedKeys(t *testing.T) {
	t.Setenv("DD_ENV", "prod")

	var buf bytes.Buffer
	New(&buf).LogAttrs(context.Background(), slog.LevelError, "boom",
		Event(EventLoginFailed),
		Integration(IntegrationRDS),
		Err(errors.New("connection refused")))

	line := decodeLine(t, &buf)
	assert.Equal(t, "auth.login_failed", line[KeyEvent])
	assert.Equal(t, "rds", line[KeyIntegration])
	assert.Equal(t, "connection refused", line[KeyError])
	assert.Equal(t, "ERROR", line["level"])
}

func TestErr_OmitsTheFieldWhenThereIsNoError(t *testing.T) {
	t.Setenv("DD_ENV", "prod")

	var buf bytes.Buffer
	New(&buf).LogAttrs(context.Background(), slog.LevelInfo, "ok", Err(nil))

	assert.NotContains(t, decodeLine(t, &buf), KeyError)
}

func TestFromContext_ReturnsTheLoggerThatWasStored(t *testing.T) {
	t.Setenv("DD_ENV", "prod")

	var buf bytes.Buffer
	logger := New(&buf).With(slog.String(KeyRequestID, "abc-123"))

	FromContext(WithLogger(context.Background(), logger)).Info("hello")

	assert.Equal(t, "abc-123", decodeLine(t, &buf)[KeyRequestID])
}

func TestFromContext_FallsBackToTheDefault(t *testing.T) {
	assert.NotNil(t, FromContext(context.Background()))
	assert.Equal(t, slog.Default(), FromContext(context.Background()))
}
