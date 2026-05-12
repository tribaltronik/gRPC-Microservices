package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/tiagoricardo/grpc-microservices/db/migrations"
	"github.com/tiagoricardo/grpc-microservices/internal/log"
	"github.com/tiagoricardo/grpc-microservices/internal/shutdown"
	"github.com/tiagoricardo/grpc-microservices/internal/tls"
	"github.com/tiagoricardo/grpc-microservices/internal/vault"
	"github.com/tiagoricardo/grpc-microservices/services/order/v1"
)

func recoveryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered in gRPC handler",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
				)
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

func loggingInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)
		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
		}
		if err != nil {
			fields = append(fields, zap.String("code", status.Code(err).String()))
			logger.Warn("gRPC call", fields...)
		} else {
			logger.Info("gRPC call", fields...)
		}
		return resp, err
	}
}

func newPool(ctx context.Context, databaseURL string, cfg Config, logger *zap.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	poolCfg.MaxConns = cfg.DBPoolMaxConns
	poolCfg.MinConns = cfg.DBPoolMinConns
	poolCfg.MaxConnLifetime = cfg.DBPoolMaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.DBPoolMaxConnIdleTime
	poolCfg.HealthCheckPeriod = cfg.DBPoolHealthCheckPeriod

	logger.Info("database pool configured",
		zap.Int32("max_conns", cfg.DBPoolMaxConns),
		zap.Int32("min_conns", cfg.DBPoolMinConns),
		zap.Duration("max_lifetime", cfg.DBPoolMaxConnLifetime),
		zap.Duration("health_check", cfg.DBPoolHealthCheckPeriod),
	)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return pool, nil
}

func buildDatabaseURL(creds *vault.DBCreds) string {
	return fmt.Sprintf("postgres://%s:%s@order-db:5432/orders?sslmode=disable",
		creds.Username, creds.Password)
}

func monitorDBHealth(ctx context.Context, pool *pgxpool.Pool, healthServer *health.Server, logger *zap.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := pool.Ping(pingCtx)
			cancel()

			if err != nil {
				logger.Warn("database health check failed", zap.Error(err))
				healthServer.SetServingStatus("order.v1.OrderService", healthpb.HealthCheckResponse_NOT_SERVING)
			} else {
				healthServer.SetServingStatus("order.v1.OrderService", healthpb.HealthCheckResponse_SERVING)
			}
		}
	}
}

func main() {
	cfg := LoadConfig()

	logger, err := log.New(cfg.LogLevel)
	if err != nil {
		logger.Fatal("failed to init logger", zap.Error(err))
	}
	defer logger.Sync()

	logger.Info("starting order-service",
		zap.String("port", cfg.Port),
		zap.String("user_service_addr", cfg.UserServiceAddr),
	)

	ctx := context.Background()

	var vc *vault.Client
	databaseURL := cfg.DatabaseURL

	if cfg.UseVault() {
		logger.Info("fetching database credentials from Vault",
			zap.String("addr", cfg.VaultAddr),
			zap.String("role", cfg.VaultDBRole),
		)
		vc, err = vault.New(cfg.VaultAddr, cfg.VaultToken)
		if err != nil {
			logger.Fatal("failed to create vault client", zap.Error(err))
		}
		creds, err := vc.GetDBCreds(ctx, cfg.VaultDBRole)
		if err != nil {
			logger.Fatal("failed to get db creds from vault", zap.Error(err))
		}
		logger.Info("dynamic db credentials obtained",
			zap.String("username", creds.Username),
			zap.Duration("lease_duration", creds.LeaseDuration),
		)
		databaseURL = buildDatabaseURL(creds)
	}

	pool, err := newPool(ctx, databaseURL, cfg, logger)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}
	logger.Info("database migrations applied")

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Fatal("failed to parse redis URL", zap.Error(err))
	}
	rdb := redis.NewClient(redisOpts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("redis not available, continuing without cache", zap.Error(err))
	}

	userClient, err := NewUserClient(cfg.UserServiceAddr, cfg.CAFile, cfg.CertFile, cfg.KeyFile)
	if err != nil {
		logger.Fatal("failed to create user client", zap.Error(err))
	}
	defer userClient.Close()

	repo := NewOrderRepository(pool, rdb, logger)
	svc := NewOrderServiceServer(repo, userClient, logger)

	if vc != nil {
		vc.StartCredentialRefresher(ctx, cfg.VaultDBRole,
			func(creds *vault.DBCreds) {
				newURL := buildDatabaseURL(creds)
				logger.Info("vault credentials refreshed, rotating pool",
					zap.String("username", creds.Username),
				)
				newPool, err := newPool(ctx, newURL, cfg, logger)
				if err != nil {
					logger.Error("failed to create new pool after credential refresh", zap.Error(err))
					return
				}
				repo.SetPool(newPool)
			},
			func(msg string, args ...interface{}) {
				logger.Sugar().Warnw(msg, args...)
			},
		)
	}

	creds, err := tls.LoadServerConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	if err != nil {
		logger.Fatal("failed to load TLS config", zap.Error(err))
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(
			recoveryInterceptor(logger),
			loggingInterceptor(logger),
		),
	)
	orderv1.RegisterOrderServiceServer(grpcServer, svc)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("order.v1.OrderService", healthpb.HealthCheckResponse_SERVING)

	go monitorDBHealth(ctx, pool, healthServer, logger, 15*time.Second)

	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Fatal("failed to listen", zap.Error(err))
	}

	go func() {
		logger.Info("serving gRPC", zap.String("address", lis.Addr().String()))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatal("failed to serve", zap.Error(err))
		}
	}()

	shutdown.Graceful(ctx, 30*time.Second,
		shutdown.Callback{Name: "gRPC server", Func: func(ctx context.Context) error {
			grpcServer.GracefulStop()
			return nil
		}},
		shutdown.Callback{Name: "database pool", Func: func(ctx context.Context) error {
			pool.Close()
			return nil
		}},
		shutdown.Callback{Name: "redis client", Func: func(ctx context.Context) error {
			return rdb.Close()
		}},
	)
}
