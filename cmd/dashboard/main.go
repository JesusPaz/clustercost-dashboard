package main

import (
	"context"
	"flag"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/clustercost/clustercost-dashboard/internal/api"
	"github.com/clustercost/clustercost-dashboard/internal/auth"
	"github.com/clustercost/clustercost-dashboard/internal/config"
	"github.com/clustercost/clustercost-dashboard/internal/db"
	"github.com/clustercost/clustercost-dashboard/internal/finops"
	ccgrpc "github.com/clustercost/clustercost-dashboard/internal/grpc"
	"github.com/clustercost/clustercost-dashboard/internal/logging"
	"github.com/clustercost/clustercost-dashboard/internal/store"
	"github.com/clustercost/clustercost-dashboard/internal/vm"
)

func main() {
	logger := logging.New("dashboard")

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	logLevel := flag.String("log-level", "", "Set the logging level (info, debug)")
	flag.Parse()

	if *logLevel != "" {
		cfg.LogLevel = strings.ToLower(*logLevel)
	}
	logger.Printf("Configured Log Level: %s", cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	vmClient, err := vm.NewClient(cfg)
	if err != nil {
		logger.Fatalf("victoria metrics query setup error: %v", err)
	}

	sqlite, err := db.New(cfg.StoragePath)
	if err != nil {
		logger.Fatalf("sqlite setup error: %v", err)
	}
	defer func() { _ = sqlite.Close() }()

	// Initialize In-Memory Store
	st := store.New(cfg.Agents, cfg.RecommendedAgentVersion)

	// Initialize FinOps Engine
	finopsEngine := finops.NewEngine(vmClient, st.PricingCatalog())

	auth.SetSecret(cfg.JWTSecret)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewRouter(vmClient, sqlite, st, finopsEngine),
		ReadHeaderTimeout: 5 * time.Second,
	}

	vmIngestor, err := vm.NewIngestor(cfg, logger)
	if err != nil {
		logger.Fatalf("victoria metrics setup error: %v", err)
	}
	if vmIngestor != nil {
		defer vmIngestor.Stop()
		logger.Printf("victoria metrics ingest enabled")
	}

	// Pass store to gRPC server so it can receive reports
	grpcSrv := ccgrpc.NewServer(cfg, vmIngestor, st)
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
