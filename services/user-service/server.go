package main

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tiagoricardo/grpc-microservices/services/user/v1"
)

type UserServiceServer struct {
	userv1.UnimplementedUserServiceServer
	repo *UserRepository
	log  *zap.Logger
}

func NewUserServiceServer(repo *UserRepository, log *zap.Logger) *UserServiceServer {
	return &UserServiceServer{repo: repo, log: log}
}

func (s *UserServiceServer) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)

	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		return nil, status.Error(codes.InvalidArgument, "valid email is required")
	}

	user, err := s.repo.Create(ctx, req.Name, req.Email)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, status.Error(codes.AlreadyExists, "email already in use")
		}
		s.log.Error("failed to create user", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create user")
	}

	s.log.Info("user created", zap.String("id", user.Id), zap.String("email", user.Email))
	return &userv1.CreateUserResponse{User: user}, nil
}

func (s *UserServiceServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	user, err := s.repo.Get(ctx, req.Id)
	if err != nil {
		s.log.Error("failed to get user", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return &userv1.GetUserResponse{User: user}, nil
}

func (s *UserServiceServer) UpdateUser(ctx context.Context, req *userv1.UpdateUserRequest) (*userv1.UpdateUserResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.UpdateMask == nil || len(req.UpdateMask.Paths) == 0 {
		return nil, status.Error(codes.InvalidArgument, "update_mask is required")
	}

	var name, email string
	nameSet, emailSet := false, false

	for _, path := range req.UpdateMask.Paths {
		switch path {
		case "name":
			name = strings.TrimSpace(req.User.GetName())
			if name == "" {
				return nil, status.Error(codes.InvalidArgument, "name cannot be empty")
			}
			nameSet = true
		case "email":
			email = strings.TrimSpace(req.User.GetEmail())
			if email == "" || !strings.Contains(email, "@") {
				return nil, status.Error(codes.InvalidArgument, "valid email is required")
			}
			emailSet = true
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unknown field: %s", path)
		}
	}

	user, err := s.repo.Update(ctx, req.Id, name, email, nameSet, emailSet)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, status.Error(codes.AlreadyExists, "email already in use")
		}
		s.log.Error("failed to update user", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to update user")
	}
	if user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	s.log.Info("user updated", zap.String("id", user.Id))
	return &userv1.UpdateUserResponse{User: user}, nil
}

func (s *UserServiceServer) DeleteUser(ctx context.Context, req *userv1.DeleteUserRequest) (*userv1.DeleteUserResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	found, err := s.repo.Delete(ctx, req.Id)
	if err != nil {
		s.log.Error("failed to delete user", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to delete user")
	}
	if !found {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	s.log.Info("user deleted", zap.String("id", req.Id))
	return &userv1.DeleteUserResponse{Message: "user deleted"}, nil
}

func (s *UserServiceServer) mustEmbedUnimplementedUserServiceServer() {}
