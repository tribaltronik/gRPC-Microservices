package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tiagoricardo/grpc-microservices/services/user/v1"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateUser inserts a new user and returns the created user with generated fields.
func (r *UserRepository) Create(ctx context.Context, name, email string) (*userv1.User, error) {
	var (
		id        string
		createdAt time.Time
		updatedAt time.Time
	)

	err := r.pool.QueryRow(ctx,
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

// GetUser retrieves a user by ID. Returns nil, nil if not found.
func (r *UserRepository) Get(ctx context.Context, id string) (*userv1.User, error) {
	var (
		name      string
		email     string
		createdAt time.Time
		updatedAt time.Time
	)

	err := r.pool.QueryRow(ctx,
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

// UpdateUser updates specific fields of a user based on the field mask.
// It sets updated_at automatically. Returns the updated user.
func (r *UserRepository) Update(ctx context.Context, id, name, email string, nameSet, emailSet bool) (*userv1.User, error) {
	// Build dynamic update query based on which fields are set
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
		// Nothing to update — fetch current state
		return r.Get(ctx, id)
	}

	// Always update updated_at
	setClauses += fmt.Sprintf("updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf(
		`UPDATE users SET %s WHERE id = $%d
		 RETURNING id, name, email, created_at, updated_at`,
		setClauses, argIdx,
	)

	var (
		outID       string
		outName     string
		outEmail    string
		createdAt   time.Time
		updatedAt   time.Time
	)

	err := r.pool.QueryRow(ctx, query, args...).Scan(&outID, &outName, &outEmail, &createdAt, &updatedAt)
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

// DeleteUser deletes a user by ID. Returns true if deleted, false if not found.
func (r *UserRepository) Delete(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete user: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
