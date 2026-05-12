package main

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/tiagoricardo/grpc-microservices/internal/tls"
	"github.com/tiagoricardo/grpc-microservices/services/user/v1"
)

type UserClient struct {
	client userv1.UserServiceClient
	conn   *grpc.ClientConn
}

func NewUserClient(addr, caFile, certFile, keyFile string) (*UserClient, error) {
	var opts []grpc.DialOption

	if certFile != "" && keyFile != "" && caFile != "" {
		creds, err := tls.LoadClientConfig(caFile, certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load client TLS: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial user service: %w", err)
	}

	return &UserClient{
		client: userv1.NewUserServiceClient(conn),
		conn:   conn,
	}, nil
}

func (c *UserClient) Close() error {
	return c.conn.Close()
}

// GetUser retrieves a user by ID. Returns nil, nil if user not found (404 case).
func (c *UserClient) GetUser(ctx context.Context, id string) (*userv1.User, error) {
	resp, err := c.client.GetUser(ctx, &userv1.GetUserRequest{Id: id})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			if st.Code().String() == "NotFound" {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return resp.User, nil
}
