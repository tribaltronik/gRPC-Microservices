package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tiagoricardo/grpc-microservices/services/order/v1"
)

type OrderRepository struct {
	pool  *pgxpool.Pool
	rdb   *redis.Client
	log   *zap.Logger
	cache *OrderCache
}

type OrderItemRow struct {
	ID          string
	OrderID     string
	ProductID   string
	ProductName string
	Quantity    int32
	UnitPrice   float64
}

func NewOrderRepository(pool *pgxpool.Pool, rdb *redis.Client, log *zap.Logger) *OrderRepository {
	return &OrderRepository{
		pool:  pool,
		rdb:   rdb,
		log:   log,
		cache: NewOrderCache(rdb, log),
	}
}

func (r *OrderRepository) Create(ctx context.Context, userID string, items []*orderv1.OrderItem) (*orderv1.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	orderID := uuid.New().String()
	var total float64
	for _, item := range items {
		total += float64(item.Quantity) * item.UnitPrice
	}

	var createdAt time.Time
	err = tx.QueryRow(ctx,
		`INSERT INTO orders (id, user_id, total) VALUES ($1, $2, $3)
		 RETURNING created_at`,
		orderID, userID, total,
	).Scan(&createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert order: %w", err)
	}

	for _, item := range items {
		itemID := uuid.New().String()
		_, err = tx.Exec(ctx,
			`INSERT INTO order_items (id, order_id, product_id, product_name, quantity, unit_price)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			itemID, orderID, item.ProductId, item.ProductName, item.Quantity, item.UnitPrice,
		)
		if err != nil {
			return nil, fmt.Errorf("insert order_item: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	order := &orderv1.Order{
		Id:        orderID,
		UserId:    userID,
		Items:     items,
		Total:     total,
		Status:    orderv1.OrderStatus_ORDER_STATUS_PENDING,
		CreatedAt: timestamppb.New(createdAt),
	}

	return order, nil
}

func (r *OrderRepository) Get(ctx context.Context, id string) (*orderv1.Order, error) {
	// Try cache first
	if cached, err := r.cache.Get(ctx, id); err == nil && cached != nil {
		r.log.Debug("cache hit for order", zap.String("id", id))
		return cached, nil
	}

	order, err := r.getFromDB(ctx, id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, nil
	}

	// Write to cache (async, non-blocking)
	if err := r.cache.Set(ctx, order); err != nil {
		r.log.Warn("failed to cache order", zap.String("id", id), zap.Error(err))
	}

	return order, nil
}

func (r *OrderRepository) getFromDB(ctx context.Context, id string) (*orderv1.Order, error) {
	var userID, status string
	var total float64
	var createdAt time.Time

	err := r.pool.QueryRow(ctx,
		`SELECT user_id, status, total, created_at FROM orders WHERE id = $1`, id,
	).Scan(&userID, &status, &total, &createdAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get order: %w", err)
	}

	items, err := r.getItems(ctx, id)
	if err != nil {
		return nil, err
	}

	return &orderv1.Order{
		Id:        id,
		UserId:    userID,
		Items:     items,
		Total:     total,
		Status:    parseOrderStatus(status),
		CreatedAt: timestamppb.New(createdAt),
	}, nil
}

func (r *OrderRepository) getItems(ctx context.Context, orderID string) ([]*orderv1.OrderItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT product_id, product_name, quantity, unit_price
		 FROM order_items WHERE order_id = $1`, orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("get items: %w", err)
	}
	defer rows.Close()

	var items []*orderv1.OrderItem
	for rows.Next() {
		var item orderv1.OrderItem
		if err := rows.Scan(&item.ProductId, &item.ProductName, &item.Quantity, &item.UnitPrice); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		items = append(items, &item)
	}
	return items, nil
}

func (r *OrderRepository) List(ctx context.Context, userID string, page, pageSize int32) ([]*orderv1.Order, int32, error) {
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	// Count total
	var total int32
	countQuery := `SELECT COUNT(*) FROM orders`
	countArgs := []interface{}{}
	if userID != "" {
		countQuery += ` WHERE user_id = $1`
		countArgs = append(countArgs, userID)
	}
	if err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}

	// Fetch page
	listQuery := `SELECT id, user_id, status, total, created_at FROM orders`
	listArgs := []interface{}{}
	argIdx := 1
	if userID != "" {
		listQuery += fmt.Sprintf(` WHERE user_id = $%d`, argIdx)
		listArgs = append(listArgs, userID)
		argIdx++
	}
	listQuery += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprintf("%d", argIdx) + ` OFFSET $` + fmt.Sprintf("%d", argIdx+1)
	listArgs = append(listArgs, pageSize, offset)

	rows, err := r.pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var orders []*orderv1.Order
	for rows.Next() {
		var id, uid, status string
		var total float64
		var createdAt time.Time

		if err := rows.Scan(&id, &uid, &status, &total, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan order: %w", err)
		}

		orders = append(orders, &orderv1.Order{
			Id:        id,
			UserId:    uid,
			Total:     total,
			Status:    parseOrderStatus(status),
			CreatedAt: timestamppb.New(createdAt),
		})
	}

	return orders, total, nil
}

func (r *OrderRepository) Cancel(ctx context.Context, id string) (*orderv1.Order, error) {
	var status string
	err := r.pool.QueryRow(ctx,
		`UPDATE orders SET status = 'CANCELLED' WHERE id = $1
		 RETURNING status`, id,
	).Scan(&status)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("cancel order: %w", err)
	}

	// Invalidate cache
	r.cache.Invalidate(ctx, id)

	// Re-fetch full order
	return r.getFromDB(ctx, id)
}

func parseOrderStatus(s string) orderv1.OrderStatus {
	switch s {
	case "PENDING":
		return orderv1.OrderStatus_ORDER_STATUS_PENDING
	case "CONFIRMED":
		return orderv1.OrderStatus_ORDER_STATUS_CONFIRMED
	case "SHIPPED":
		return orderv1.OrderStatus_ORDER_STATUS_SHIPPED
	case "DELIVERED":
		return orderv1.OrderStatus_ORDER_STATUS_DELIVERED
	case "CANCELLED":
		return orderv1.OrderStatus_ORDER_STATUS_CANCELLED
	default:
		return orderv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}
