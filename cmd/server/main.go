package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sse-streaming-chat/config"
	"sse-streaming-chat/internal/domain"
	"sse-streaming-chat/internal/infrastructure"
	p "sse-streaming-chat/internal/providers"
	"sse-streaming-chat/internal/service"
)

// main is the entry point for the SSE streaming chat server
// It demonstrates clean architecture principles by separating concerns
// and using dependency injection for component assembly
func main() {
	// Load configuration from files and environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger (for now using simple log, can be replaced with structured logger)
	logger := infrastructure.NewSimpleLogger()
	logger.Info("Starting SSE Streaming Chat Server", "version", "1.0.0")

	// Create dependency injection container
	container, err := buildContainer(cfg, logger)
	if err != nil {
		log.Fatalf("Failed to build dependency container: %v", err)
	}

	// Create HTTP server
	server := &http.Server{
		Addr:         cfg.GetServerAddress(),
		Handler:      container.Router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Server starting", "address", cfg.GetServerAddress())
		logger.Info("Web interface available at: http://localhost:" + cfg.Server.Port)
		logger.Info("SSE endpoint: /stream")
		logger.Info("Chat endpoint: /chat (POST)")
		logger.Info("Health check: /health")
		logger.Info("Metrics: /metrics")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Server shutting down...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server gracefully
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server stopped")
}

// DependencyContainer holds all application dependencies
// This implements the dependency injection pattern for clean architecture
type DependencyContainer struct {
	// Configuration
	Config *config.Config

	// Logger
	Logger domain.Logger

	// Repositories
	ClientRepo  domain.ClientRepository
	MetricsRepo domain.MetricsRepository

	// Services
	StreamingService domain.StreamingService
	HealthChecker    domain.HealthChecker

	// LLM Providers
	Providers map[string]domain.LLMProvider

	// Infrastructure
	Router      http.Handler
	Validator   domain.MessageValidator
	RateLimiter domain.RateLimiter
}

// buildContainer creates and wires all application dependencies
// This is where we implement the dependency injection pattern
func buildContainer(cfg *config.Config, logger domain.Logger) (*DependencyContainer, error) {
	// Create repositories
	clientRepo := infrastructure.NewInMemoryClientRepository(logger)
	metricsRepo := infrastructure.NewInMemoryMetricsRepository()

	// Create infrastructure components
	validator := infrastructure.NewMessageValidator()
	rateLimiter := infrastructure.NewInMemoryRateLimiter(100, time.Minute) // 100 requests per minute

	// Create LLM providers
	providers := make(map[string]domain.LLMProvider)

	// Initialize OpenAI provider if configured
	// if cfg.IsOpenAIEnabled() {
	// 	openaiProvider := p.NewOpenAIProvider(cfg.OpenAI, logger)
	// 	providers["openai"] = openaiProvider
	// 	logger.Info("OpenAI provider initialized", "model", cfg.OpenAI.Model)
	// }

	// Initialize Groq provider if configured
	if cfg.IsGroqEnabled() {
		groqProvider := p.NewGroqProvider(cfg.Groq, logger)
		providers["groq"] = groqProvider
		logger.Info("Groq provider initialized", "model", cfg.Groq.Model)
	}

	// Ensure at least one provider is available
	if len(providers) == 0 {
		return nil, fmt.Errorf("no LLM providers configured - please set API keys")
	}

	// Create streaming service
	streamingService := service.NewStreamingService(
		providers,
		clientRepo,
		metricsRepo,
		logger,
		validator,
		rateLimiter,
		cfg.App.DefaultProvider,
		time.Duration(cfg.App.StreamingDelay)*time.Millisecond,
		cfg.App.MaxConnections,
	)

	// Create health checker
	healthChecker := infrastructure.NewHealthChecker(providers, metricsRepo, logger)

	// Create HTTP router and handlers
	router := infrastructure.NewRouter(
		streamingService,
		healthChecker,
		metricsRepo,
		logger,
	)

	return &DependencyContainer{
		Config:           cfg,
		Logger:           logger,
		ClientRepo:       clientRepo,
		MetricsRepo:      metricsRepo,
		StreamingService: streamingService,
		HealthChecker:    healthChecker,
		Providers:        providers,
		Router:           router,
		Validator:        validator,
		RateLimiter:      rateLimiter,
	}, nil
}
