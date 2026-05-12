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

	// Database — use Vault for dynamic credentials if configured
	databaseURL := cfg.DatabaseURL
	if cfg.UseVault() {
		logger.Info("fetching database credentials from Vault",
			zap.String("addr", cfg.VaultAddr),
			zap.String("role", cfg.VaultDBRole),
		)
		vc, err := vault.New(cfg.VaultAddr, cfg.VaultToken)
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
		databaseURL = fmt.Sprintf("postgres://%s:%s@order-db:5432/orders?sslmode=disable",
			creds.Username, creds.Password)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	if err := migrations.Up(ctx, pool); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}
	logger.Info("database migrations applied")

	// Redis
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Fatal("failed to parse redis URL", zap.Error(err))
	}
	rdb := redis.NewClient(redisOpts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Warn("redis not available, continuing without cache", zap.Error(err))
	}

	// User Service client
	userClient, err := NewUserClient(cfg.UserServiceAddr, cfg.CAFile, cfg.CertFile, cfg.KeyFile)
	if err != nil {
		logger.Fatal("failed to create user client", zap.Error(err))
	}
	defer userClient.Close()

	// Repository + Server
	repo := NewOrderRepository(pool, rdb, logger)
	svc := NewOrderServiceServer(repo, userClient, logger)

	// mTLS
	creds, err := tls.LoadServerConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	if err != nil {
		logger.Fatal("failed to load TLS config", zap.Error(err))
	}

	// gRPC server
	grpcServer := grpc.NewServer(
		grpc.Creds(creds),
		grpc.UnaryInterceptor(loggingInterceptor(logger)),
	)
	orderv1.RegisterOrderServiceServer(grpcServer, svc)

	// Health check
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("order.v1.OrderService", healthpb.HealthCheckResponse_SERVING)

	// Reflection
	reflection.Register(grpcServer)

	// Listen
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

	// Graceful shutdown
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
