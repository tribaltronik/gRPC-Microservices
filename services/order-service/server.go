package main

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/tiagoricardo/grpc-microservices/services/common/v1"
	"github.com/tiagoricardo/grpc-microservices/services/order/v1"
)

type OrderServiceServer struct {
	orderv1.UnimplementedOrderServiceServer
	repo   *OrderRepository
	client *UserClient
	log    *zap.Logger
}

func NewOrderServiceServer(repo *OrderRepository, client *UserClient, log *zap.Logger) *OrderServiceServer {
	return &OrderServiceServer{repo: repo, client: client, log: log}
}

func (s *OrderServiceServer) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (*orderv1.CreateOrderResponse, error) {
	req.UserId = strings.TrimSpace(req.UserId)

	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(req.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one item is required")
	}
	for i, item := range req.Items {
		if item.ProductId == "" {
			return nil, status.Errorf(codes.InvalidArgument, "items[%d].product_id is required", i)
		}
		if item.Quantity <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "items[%d].quantity must be positive", i)
		}
		if item.UnitPrice < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "items[%d].unit_price must be non-negative", i)
		}
	}

	// Verify user exists via User Service
	user, err := s.client.GetUser(ctx, req.UserId)
	if err != nil {
		s.log.Error("failed to verify user", zap.String("user_id", req.UserId), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to verify user")
	}
	if user == nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	order, err := s.repo.Create(ctx, req.UserId, req.Items)
	if err != nil {
		s.log.Error("failed to create order", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create order")
	}

	s.log.Info("order created", zap.String("id", order.Id), zap.String("user_id", order.UserId))
	return &orderv1.CreateOrderResponse{Order: order}, nil
}

func (s *OrderServiceServer) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest) (*orderv1.GetOrderResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	order, err := s.repo.Get(ctx, req.Id)
	if err != nil {
		s.log.Error("failed to get order", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get order")
	}
	if order == nil {
		return nil, status.Error(codes.NotFound, "order not found")
	}

	return &orderv1.GetOrderResponse{Order: order}, nil
}

func (s *OrderServiceServer) ListOrders(ctx context.Context, req *orderv1.ListOrdersRequest) (*orderv1.ListOrdersResponse, error) {
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}

	orders, total, err := s.repo.List(ctx, req.UserId, page, pageSize)
	if err != nil {
		s.log.Error("failed to list orders", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list orders")
	}

	hasMore := total > page*pageSize

	return &orderv1.ListOrdersResponse{
		Orders: orders,
		Pagination: &commonv1.PaginationResponse{
			TotalCount: total,
			HasMore:    hasMore,
		},
	}, nil
}

func (s *OrderServiceServer) CancelOrder(ctx context.Context, req *orderv1.CancelOrderRequest) (*orderv1.CancelOrderResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	order, err := s.repo.Cancel(ctx, req.Id)
	if err != nil {
		s.log.Error("failed to cancel order", zap.String("id", req.Id), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to cancel order")
	}
	if order == nil {
		return nil, status.Error(codes.NotFound, "order not found")
	}

	s.log.Info("order cancelled", zap.String("id", order.Id))
	return &orderv1.CancelOrderResponse{Order: order}, nil
}

func (s *OrderServiceServer) mustEmbedUnimplementedOrderServiceServer() {}
