package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tiagoricardo/grpc-microservices/services/user/v1"
)

const dbQueryTimeout = 10 * time.Second

type UserRepository struct {
	mu   sync.RWMutex
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) getPool() *pgxpool.Pool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pool
}

func (r *UserRepository) SetPool(pool *pgxpool.Pool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.pool
	r.pool = pool
	if old != nil {
		old.Close()
	}
}

func (r *UserRepository) queryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, dbQueryTimeout)
}

func (r *UserRepository) Create(ctx context.Context, name, email string) (*userv1.User, error) {
	qCtx, cancel := r.queryTimeout(ctx)
	defer cancel()

	var (
		id        string
		createdAt time.Time
		updatedAt time.Time
	)

	err := r.getPool().QueryRow(qCtx,
		`INSERT INTO users (name, email) VALUES ($1, $2)
		 RETURNING id, created_at, updated_at`,
		name, email,
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &userv1.User{
		Id:        id,
		Name:      name,
		Email:     email,
		CreatedAt: timestamppb.New(createdAt),
		UpdatedAt: timestamppb.New(updatedAt),
	}, nil
}

func (r *UserRepository) Get(ctx context.Context, id string) (*userv1.User, error) {
	qCtx, cancel := r.queryTimeout(ctx)
	defer cancel()

	var (
		name      string
		email     string
		createdAt time.Time
		updatedAt time.Time
	)

	err := r.getPool().QueryRow(qCtx,
		`SELECT name, email, created_at, updated_at FROM users WHERE id = $1`,
		id,
	).Scan(&name, &email, &createdAt, &updatedAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	return &userv1.User{
		Id:        id,
		Name:      name,
		Email:     email,
		CreatedAt: timestamppb.New(createdAt),
		UpdatedAt: timestamppb.New(updatedAt),
	}, nil
}

func (r *UserRepository) Update(ctx context.Context, id, name, email string, nameSet, emailSet bool) (*userv1.User, error) {
	setClauses := ""
	args := []interface{}{}
	argIdx := 1

	if nameSet {
		setClauses += fmt.Sprintf("name = $%d, ", argIdx)
		args = append(args, name)
		argIdx++
	}
	if emailSet {
		setClauses += fmt.Sprintf("email = $%d, ", argIdx)
		args = append(args, email)
		argIdx++
	}

	if setClauses == "" {
		return r.Get(ctx, id)
	}

	setClauses += fmt.Sprintf("updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf(
		`UPDATE users SET %s WHERE id = $%d
		 RETURNING id, name, email, created_at, updated_at`,
		setClauses, argIdx,
	)

	qCtx, cancel := r.queryTimeout(ctx)
	defer cancel()

	var (
		outID       string
		outName     string
		outEmail    string
		createdAt   time.Time
		updatedAt   time.Time
	)

	err := r.getPool().QueryRow(qCtx, query, args...).Scan(&outID, &outName, &outEmail, &createdAt, &updatedAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("update user: %w", err)
	}

	return &userv1.User{
		Id:        outID,
		Name:      outName,
		Email:     outEmail,
		CreatedAt: timestamppb.New(createdAt),
		UpdatedAt: timestamppb.New(updatedAt),
	}, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) (bool, error) {
	qCtx, cancel := r.queryTimeout(ctx)
	defer cancel()

	tag, err := r.getPool().Exec(qCtx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete user: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
