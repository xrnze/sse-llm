package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sse-streaming-chat/internal/domain"
)

// streamingService implements the core business logic for streaming chat
// It orchestrates interactions between clients, LLM providers, and infrastructure
type streamingService struct {
	// providers holds all configured LLM providers
	providers map[string]domain.LLMProvider

	// clientRepo manages client connections
	clientRepo domain.ClientRepository

	// metricsRepo tracks system metrics
	metricsRepo domain.MetricsRepository

	// logger for structured logging
	logger domain.Logger

	// validator for input validation
	validator domain.MessageValidator

	// rateLimiter for request rate limiting
	rateLimiter domain.RateLimiter

	// defaultProvider is the default LLM provider to use
	defaultProvider string

	// streamingDelay adds artificial delay between tokens (for demo)
	streamingDelay time.Duration

	// maxConnections limits concurrent connections
	maxConnections int

	// mutex protects concurrent access to service state
	mutex sync.RWMutex
}

// NewStreamingService creates a new streaming service instance
// This follows dependency injection principles for clean architecture
func NewStreamingService(
	providers map[string]domain.LLMProvider,
	clientRepo domain.ClientRepository,
	metricsRepo domain.MetricsRepository,
	logger domain.Logger,
	validator domain.MessageValidator,
	rateLimiter domain.RateLimiter,
	defaultProvider string,
	streamingDelay time.Duration,
	maxConnections int,
) domain.StreamingService {
	return &streamingService{
		providers:       providers,
		clientRepo:      clientRepo,
		metricsRepo:     metricsRepo,
		logger:          logger,
		validator:       validator,
		rateLimiter:     rateLimiter,
		defaultProvider: defaultProvider,
		streamingDelay:  streamingDelay,
		maxConnections:  maxConnections,
	}
}

// StartStreaming initiates a streaming chat session with the specified LLM provider
// This is the main entry point for processing streaming chat requests
func (s *streamingService) StartStreaming(ctx context.Context, request *domain.StreamingRequest, streamer domain.SSEStreamer) error {
	// Validate the incoming request
	if err := s.validator.ValidateRequest(request); err != nil {
		s.logger.Error("Request validation failed", err, "request", request)
		s.metricsRepo.IncrementErrors()
		return s.sendError(streamer, domain.ErrorCodeInvalidRequest, err.Error())
	}

	// Determine which provider to use
	providerName := request.Provider
	if providerName == "" {
		providerName = s.defaultProvider
	}

	// Get the specified provider
	provider, exists := s.providers[providerName]
	if !exists {
		s.logger.Error("Provider not found", nil, "provider", providerName)
		s.metricsRepo.IncrementErrors()
		return s.sendError(streamer, domain.ErrorCodeProviderNotFound, fmt.Sprintf("Provider '%s' not found", providerName))
	}

	// Check if provider is available
	if !provider.IsAvailable(ctx) {
		s.logger.Error("Provider unavailable", nil, "provider", providerName)
		s.metricsRepo.IncrementErrors()
		return s.sendError(streamer, domain.ErrorCodeProviderUnavailable, fmt.Sprintf("Provider '%s' is currently unavailable", providerName))
	}

	// Apply rate limiting
	clientKey := request.ClientID
	if clientKey == "" {
		clientKey = "anonymous"
	}

	if !s.rateLimiter.Allow(clientKey) {
		s.logger.Warn("Rate limit exceeded", "client", clientKey)
		s.metricsRepo.IncrementErrors()
		return s.sendError(streamer, domain.ErrorCodeProviderRateLimit, "Rate limit exceeded")
	}

	// Start the streaming process
	s.logger.Info("Starting streaming session", "provider", providerName, "client", request.ClientID)

	// Record the start time for metrics
	startTime := time.Now()

	// Stream the completion
	streamChan, err := provider.StreamCompletion(ctx, request)
	if err != nil {
		s.logger.Error("Failed to start streaming", err, "provider", providerName)
		s.metricsRepo.IncrementErrors()
		s.metricsRepo.RecordProviderMetrics(providerName, time.Since(startTime), false)
		return s.sendError(streamer, domain.ErrorCodeProviderUnavailable, err.Error())
	}

	// Process streaming responses
	return s.processStreamingResponses(ctx, streamChan, streamer, providerName, startTime)
}

// HandleClientConnection manages a new client SSE connection
// This sets up the client in the repository and sends welcome messages
func (s *streamingService) HandleClientConnection(ctx context.Context, clientID string, streamer domain.SSEStreamer) error {
	// Check connection limits
	if s.clientRepo.GetActiveCount() >= s.maxConnections {
		s.logger.Warn("Maximum connections reached", "clientID", clientID, "current", s.clientRepo.GetActiveCount(), "max", s.maxConnections)
		return s.sendError(streamer, domain.ErrorCodeMaxConnectionsReached, "Maximum connections reached")
	}

	// Create client entity
	client := &domain.Client{
		ID:           clientID,
		ConnectedAt:  time.Now(),
		LastActivity: time.Now(),
	}

	// Register the client
	if err := s.clientRepo.RegisterClient(client, streamer); err != nil {
		s.logger.Error("Failed to register client", err, "clientID", clientID)
		return err
	}

	// Update metrics
	s.metricsRepo.IncrementConnections()

	// Send welcome message
	welcomeMsg := domain.NewMessage(
		fmt.Sprintf("Connected as %s", clientID),
		domain.MessageTypeConnection,
	)
	welcomeMsg.Provider = "system"

	if err := streamer.WriteMessage(welcomeMsg); err != nil {
		s.logger.Error("Failed to send welcome message", err, "clientID", clientID)
		return err
	}

	s.logger.Info("Client connected successfully", "clientID", clientID)
	return nil
}

// BroadcastMessage sends a message to all connected clients or specific clients
func (s *streamingService) BroadcastMessage(ctx context.Context, message *domain.Message, clientIDs ...string) error {
	if len(clientIDs) == 0 {
		// Check if there are any clients before broadcasting
		if s.clientRepo.GetActiveCount() == 0 {
			s.logger.Debug("No clients connected, skipping broadcast")
			return nil
		}
		// Broadcast to all clients
		return s.clientRepo.BroadcastToAll(message)
	} else {
		// Broadcast to specific clients
		return s.clientRepo.BroadcastToClients(clientIDs, message)
	}
}

// GetAvailableProviders returns a list of configured and available LLM providers
func (s *streamingService) GetAvailableProviders(ctx context.Context) []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var available []string
	for name, provider := range s.providers {
		if provider.IsAvailable(ctx) {
			available = append(available, name)
		}
	}

	return available
}

// GetProviderModels returns available models for a specific provider
func (s *streamingService) GetProviderModels(ctx context.Context, providerName string) ([]string, error) {
	s.mutex.RLock()
	provider, exists := s.providers[providerName]
	s.mutex.RUnlock()

	if !exists {
		return nil, domain.NewError(domain.ErrorCodeProviderNotFound, fmt.Sprintf("Provider '%s' not found", providerName))
	}

	models, err := provider.GetModels(ctx)
	if err != nil {
		s.logger.Error("Failed to get models", err, "provider", providerName)
		return nil, err
	}

	return models, nil
}

// StopStreaming cancels an ongoing streaming session for a specific client
func (s *streamingService) StopStreaming(ctx context.Context, clientID string) error {
	// Get the client
	_, streamer, exists := s.clientRepo.GetClient(clientID)
	if !exists {
		return domain.NewError(domain.ErrorCodeConnectionClosed, "Client not found")
	}

	// Send stop message
	stopMsg := domain.NewMessage("Streaming stopped", domain.MessageTypeDone)
	if err := streamer.WriteMessage(stopMsg); err != nil {
		s.logger.Error("Failed to send stop message", err, "clientID", clientID)
	}

	// Unregister the client
	if err := s.clientRepo.UnregisterClient(clientID); err != nil {
		s.logger.Error("Failed to unregister client", err, "clientID", clientID)
		return err
	}

	s.metricsRepo.DecrementConnections()
	s.logger.Info("Client streaming stopped", "clientID", clientID)

	return nil
}

// processStreamingResponses handles the streaming response channel from LLM providers
func (s *streamingService) processStreamingResponses(
	ctx context.Context,
	streamChan <-chan *domain.StreamingResponse,
	streamer domain.SSEStreamer,
	providerName string,
	startTime time.Time,
) error {
	defer func() {
		// Record metrics when streaming completes
		duration := time.Since(startTime)
		s.metricsRepo.RecordResponseTime(duration)
		s.metricsRepo.RecordProviderMetrics(providerName, duration, true)
	}()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled mid-stream (e.g. upstream timeout, provider drop).
			// Send a terminal error frame using a background context so the browser
			// receives a complete final SSE chunk before the connection closes.
			s.logger.Info("Streaming cancelled due to context", "provider", providerName)

			interruptErr := domain.NewError(domain.ErrorCodeProviderUnavailable, "stream interrupted")
			// Intentionally ignore write error here — connection may already be gone.
			_ = streamer.WriteError(interruptErr)
			return nil

		case response, ok := <-streamChan:
			if !ok {
				// Channel closed, streaming complete.
				s.logger.Info("Streaming completed", "provider", providerName)

				doneMsg := domain.NewMessage("Response complete", domain.MessageTypeDone)
				doneMsg.Provider = providerName
				if err := streamer.WriteMessage(doneMsg); err != nil {
					s.logger.Error("Failed to write done message", err)
				}
				return nil
			}

			// Handle the response
			if err := s.handleStreamingResponse(response, streamer, providerName); err != nil {
				s.logger.Error("Failed to handle streaming response", err, "provider", providerName)
				return err
			}

			// Add artificial delay if configured (for demo purposes)
			if s.streamingDelay > 0 {
				time.Sleep(s.streamingDelay)
			}
		}
	}
}

// handleStreamingResponse processes a single streaming response chunk
func (s *streamingService) handleStreamingResponse(
	response *domain.StreamingResponse,
	streamer domain.SSEStreamer,
	providerName string,
) error {
	// Check for errors in the response
	if response.Error != "" {
		s.logger.Error("Provider returned error", nil, "provider", providerName, "error", response.Error)
		s.metricsRepo.IncrementErrors()
		return s.sendError(streamer, domain.ErrorCodeProviderUnavailable, response.Error)
	}

	// Update message with provider information
	response.Message.Provider = providerName

	// Send the message via SSE
	if err := streamer.WriteMessage(&response.Message); err != nil {
		s.logger.Error("Failed to send streaming message", err, "provider", providerName)
		return err
	}

	// Update metrics
	s.metricsRepo.IncrementMessages()

	// Record token usage if available
	if response.Usage != nil {
		s.metricsRepo.RecordTokenUsage(providerName, response.Usage)
	}

	return nil
}

// GetActiveClientCount returns the number of active clients
func (s *streamingService) GetActiveClientCount() int {
	return s.clientRepo.GetActiveCount()
}

// sendError sends an error message via SSE
func (s *streamingService) sendError(streamer domain.SSEStreamer, code, message string) error {
	domainErr := domain.NewError(code, message)
	return streamer.WriteError(domainErr)
}
