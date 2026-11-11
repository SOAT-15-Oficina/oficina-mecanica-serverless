package osclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/observability"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
)

const (
	HeaderServiceToken = "X-Service-Token"
	HeaderRequestID    = "X-Request-Id"

	verifyPath = "/internal/credentials/verify"
	usersPath  = "/internal/users"

	DefaultTimeout = 5 * time.Second
	maxErrorBody   = 4 << 10
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    httpClient,
	}
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createUserRequest struct {
	Username string          `json:"username"`
	Password string          `json:"password"`
	Role     domain.UserRole `json:"role"`
}

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (c *Client) VerifyCredentials(ctx context.Context, username, password string) (*domain.User, error) {
	status, body, err := c.post(ctx, verifyPath, credentialsRequest{Username: username, Password: password})
	if err != nil {
		return nil, err
	}

	switch status {
	case http.StatusOK:
		return decodeUser(body)
	case http.StatusUnauthorized:
		return nil, service.ErrInvalidCredentials
	default:
		return nil, unexpected(verifyPath, status, body)
	}
}

func (c *Client) CreateUser(ctx context.Context, username, password string, role domain.UserRole) (*domain.User, error) {
	status, body, err := c.post(ctx, usersPath, createUserRequest{Username: username, Password: password, Role: role})
	if err != nil {
		return nil, err
	}

	switch status {
	case http.StatusCreated, http.StatusOK:
		return decodeUser(body)
	case http.StatusConflict:
		return nil, service.ErrUsernameTaken
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil, service.NewValidationError(errorMessage(body, "invalid user"))
	default:
		return nil, unexpected(usersPath, status, body)
	}
}

func (c *Client) post(ctx context.Context, path string, payload any) (int, []byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("osclient: encode %s: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, fmt.Errorf("osclient: build %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HeaderServiceToken, c.token)
	if id := observability.RequestID(ctx); id != "" {
		req.Header.Set(HeaderRequestID, id)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("osclient: call %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("osclient: read %s: %w", path, err)
	}

	return resp.StatusCode, body, nil
}

func decodeUser(body []byte) (*domain.User, error) {
	var user domain.User
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("osclient: decode user: %w", err)
	}
	if user.Username == "" || user.Role == "" {
		return nil, errors.New("osclient: user response without username or role")
	}
	return &user, nil
}

func errorMessage(body []byte, fallback string) string {
	var parsed errorBody
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Error != "" {
			return parsed.Error
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	return fallback
}

func unexpected(path string, status int, body []byte) error {
	if len(body) > maxErrorBody {
		body = body[:maxErrorBody]
	}
	return fmt.Errorf("osclient: %s returned %d: %s", path, status, strings.TrimSpace(string(body)))
}
