package osclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/observability"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const token = "service-token"

type captured struct {
	method    string
	path      string
	token     string
	requestID string
	body      map[string]string
}

func fakeOS(t *testing.T, status int, response string) (*Client, *captured) {
	t.Helper()
	seen := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.path = r.URL.Path
		seen.token = r.Header.Get(HeaderServiceToken)
		seen.requestID = r.Header.Get(HeaderRequestID)
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &seen.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", token, srv.Client()), seen
}

func TestVerifyCredentials_Success(t *testing.T) {
	client, seen := fakeOS(t, http.StatusOK, `{"id":"u-1","username":"alice","role":"admin"}`)
	ctx := observability.WithRequestID(context.Background(), "req-9")

	user, err := client.VerifyCredentials(ctx, "alice", "secret")

	require.NoError(t, err)
	assert.Equal(t, &domain.User{ID: "u-1", Username: "alice", Role: domain.UserRoleAdmin}, user)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/internal/credentials/verify", seen.path)
	assert.Equal(t, token, seen.token)
	assert.Equal(t, "req-9", seen.requestID)
	assert.Equal(t, map[string]string{"username": "alice", "password": "secret"}, seen.body)
}

func TestVerifyCredentials_Unauthorized(t *testing.T) {
	client, _ := fakeOS(t, http.StatusUnauthorized, `{"error":"invalid credentials"}`)

	user, err := client.VerifyCredentials(context.Background(), "alice", "wrong")

	assert.Nil(t, user)
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestVerifyCredentials_UnexpectedStatus(t *testing.T) {
	client, _ := fakeOS(t, http.StatusServiceUnavailable, strings.Repeat("x", 5000))

	_, err := client.VerifyCredentials(context.Background(), "alice", "pass")

	require.Error(t, err)
	assert.NotErrorIs(t, err, service.ErrInvalidCredentials)
	assert.Contains(t, err.Error(), "returned 503")
	assert.Less(t, len(err.Error()), 4300)
}

func TestVerifyCredentials_MalformedUser(t *testing.T) {
	for _, body := range []string{`nao-json`, `{"id":"u-1"}`} {
		client, _ := fakeOS(t, http.StatusOK, body)

		_, err := client.VerifyCredentials(context.Background(), "alice", "pass")

		assert.Error(t, err)
	}
}

func TestCreateUser_Success(t *testing.T) {
	client, seen := fakeOS(t, http.StatusCreated, `{"id":"u-2","username":"bob","role":"employee"}`)

	user, err := client.CreateUser(context.Background(), "bob", "pass", domain.UserRoleEmployee)

	require.NoError(t, err)
	assert.Equal(t, "u-2", user.ID)
	assert.Equal(t, "/internal/users", seen.path)
	assert.Equal(t, "", seen.requestID)
	assert.Equal(t, map[string]string{"username": "bob", "password": "pass", "role": "employee"}, seen.body)
}

func TestCreateUser_Conflict(t *testing.T) {
	client, _ := fakeOS(t, http.StatusConflict, `{"error":"duplicado"}`)

	_, err := client.CreateUser(context.Background(), "bob", "pass", domain.UserRoleEmployee)

	assert.ErrorIs(t, err, service.ErrUsernameTaken)
}

func TestCreateUser_BadRequestKeepsMessage(t *testing.T) {
	cases := map[string]string{
		`{"error":"password too short"}`:  "password too short",
		`{"message":"username invalido"}`: "username invalido",
		`sem corpo json`:                  "invalid user",
	}

	for body, want := range cases {
		client, _ := fakeOS(t, http.StatusBadRequest, body)

		_, err := client.CreateUser(context.Background(), "bob", "pass", domain.UserRoleEmployee)

		var validation *service.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Equal(t, want, validation.Message)
	}
}

func TestCreateUser_UnexpectedStatus(t *testing.T) {
	client, _ := fakeOS(t, http.StatusInternalServerError, `{"error":"boom"}`)

	_, err := client.CreateUser(context.Background(), "bob", "pass", domain.UserRoleEmployee)

	assert.ErrorContains(t, err, "returned 500")
}

func TestPost_TransportError(t *testing.T) {
	client := New("http://127.0.0.1:1", token, &http.Client{Timeout: time.Second})

	_, err := client.VerifyCredentials(context.Background(), "alice", "pass")

	assert.ErrorContains(t, err, "osclient: call")
}

func TestPost_InvalidBaseURL(t *testing.T) {
	client := New("http://[::1", token, nil)

	_, err := client.CreateUser(context.Background(), "bob", "pass", domain.UserRoleEmployee)

	assert.ErrorContains(t, err, "osclient: build")
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("reset by peer") }
func (failingBody) Close() error             { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPost_ReadError(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}, Header: http.Header{}}, nil
	})}
	client := New("http://os", token, httpClient)

	_, err := client.VerifyCredentials(context.Background(), "alice", "pass")

	assert.ErrorContains(t, err, "osclient: read")
}

func TestNew_DefaultsHTTPClient(t *testing.T) {
	client := New("http://os/", token, nil)

	assert.Equal(t, "http://os", client.baseURL)
	assert.Equal(t, DefaultTimeout, client.http.Timeout)
}
