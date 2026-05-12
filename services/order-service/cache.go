package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/tiagoricardo/grpc-microservices/services/order/v1"
)

const orderCacheTTL = 5 * time.Minute
const orderCachePrefix = "order:"

type OrderCache struct {
	rdb *redis.Client
	log *zap.Logger
}

func NewOrderCache(rdb *redis.Client, log *zap.Logger) *OrderCache {
	return &OrderCache{rdb: rdb, log: log}
}

func (c *OrderCache) key(id string) string {
	return orderCachePrefix + id
}

func (c *OrderCache) Get(ctx context.Context, id string) (*orderv1.Order, error) {
	data, err := c.rdb.Get(ctx, c.key(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache get: %w", err)
	}

	var order orderv1.Order
	if err := protojson.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("cache unmarshal: %w", err)
	}
	return &order, nil
}

func (c *OrderCache) Set(ctx context.Context, order *orderv1.Order) error {
	data, err := protojson.Marshal(order)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}

	return c.rdb.Set(ctx, c.key(order.Id), data, orderCacheTTL).Err()
}

func (c *OrderCache) Invalidate(ctx context.Context, id string) {
	if err := c.rdb.Del(ctx, c.key(id)).Err(); err != nil {
		c.log.Warn("failed to invalidate cache", zap.String("id", id), zap.Error(err))
	}
}
