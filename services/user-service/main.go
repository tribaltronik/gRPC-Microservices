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

	"github.com/tiagoricardo/grpc-microservices/db/migrations"
	"github.com/tiagoricardo/grpc-microservices/internal/log"
	"github.com/tiagoricardo/grpc-microservices/internal/shutdown"
	"github.com/tiagoricardo/grpc-microservices/internal/tls"
	"github.com/tiagoricardo/grpc-microservices/internal/vault"
	"github.com/tiagoricardo/grpc-microservices/services/user/v1"
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

	logger.Info("starting user-service", zap.String("port", cfg.Port))

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
		databaseURL = fmt.Sprintf("postgres://%s:%s@user-db:5432/users?sslmode=disable",
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

	// Repository + Server
	repo := NewUserRepository(pool)
	svc := NewUserServiceServer(repo, logger)

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
	userv1.RegisterUserServiceServer(grpcServer, svc)

	// Health check
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("user.v1.UserService", healthpb.HealthCheckResponse_SERVING)

	// Reflection (for grpcurl)
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
	)
}
