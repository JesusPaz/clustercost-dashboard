package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/api"
	"github.com/clustercost/clustercost-dashboard/internal/config"
	ccgrpc "github.com/clustercost/clustercost-dashboard/internal/grpc"
	"github.com/clustercost/clustercost-dashboard/internal/logging"
	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

func main() {
	logger := logging.New("dashboard")

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	vmClient, err := vm.NewClient(cfg)
	if err != nil {
		logger.Fatalf("victoria metrics query setup error: %v", err)
	}

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: api.NewRouter(vmClient),
	}

	vmIngestor, err := vm.NewIngestor(cfg, logger)
	if err != nil {
		logger.Fatalf("victoria metrics setup error: %v", err)
	}
	if vmIngestor != nil {
		defer vmIngestor.Stop()
		logger.Printf("victoria metrics ingest enabled")
	}

	grpcSrv := ccgrpc.NewServer(cfg, vmIngestor)
	go func() {
		logger.Printf("gRPC receiver initialized")
		logger.Printf("gRPC listening on %s", cfg.GrpcAddr)
		if err := grpcSrv.ListenAndServe(cfg.GrpcAddr); err != nil {
			logger.Fatalf("grpc server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		grpcSrv.Stop()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Printf("listening on %s", cfg.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
}
