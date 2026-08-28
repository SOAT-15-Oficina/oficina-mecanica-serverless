package repository

import (
	"context"
	"errors"

	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/domain"
	"github.com/SOAT-15-Oficina/oficina-mecanica-serverless/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgUniqueViolation e o SQLSTATE de violacao de constraint unica. A tabela
// `users` tem users_username_key.
const pgUniqueViolation = "23505"

type userRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) service.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	const query = `
		INSERT INTO users (id, username, password_hash, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, username, password_hash, role`

	var result domain.User
	err := r.db.QueryRow(ctx, query, user.ID, user.Username, user.PasswordHash, user.Role).
		Scan(&result.ID, &result.Username, &result.PasswordHash, &result.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return nil, service.ErrUsernameTaken
		}
		return nil, err
	}

	return &result, nil
}

func (r *userRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	const query = `SELECT id, username, password_hash, role FROM users WHERE username = $1`

	var result domain.User
	err := r.db.QueryRow(ctx, query, username).
		Scan(&result.ID, &result.Username, &result.PasswordHash, &result.Role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}

	return &result, nil
}
