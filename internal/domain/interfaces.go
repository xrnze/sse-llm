package domain

import (
	"context"
	"time"
)

// LLMProvider defines the interface for Language Model providers
// This abstraction allows us to support multiple LLM providers (OpenAI, Groq, etc.)
// while keeping the business logic provider-agnostic
type LLMProvider interface {
	// Name returns the provider name (e.g., "openai", "grok")
	Name() string

	// IsAvailable checks if the provider is properly configured and reachable
	IsAvailable(ctx context.Context) bool

	// StreamCompletion sends a request and returns a channel for streaming responses
	StreamCompletion(ctx context.Context, request *StreamingRequest) (<-chan *StreamingResponse, error)

	// GetModels returns available models for this provider
	GetModels(ctx context.Context) ([]string, error)

	// HealthCheck performs a health check on the provider
	HealthCheck(ctx context.Context) *CheckResult
}

// SSEStreamer defines the interface for Server-Sent Events streaming
// This abstraction separates the SSE protocol concerns from business logic
type SSEStreamer interface {
	// WriteMessage sends an SSE message to the client
	WriteMessage(message *Message) error

	// WriteError sends an error message via SSE
	WriteError(err *Error) error

	// Close terminates the SSE connection
	Close() error

	// IsClosed returns true if the connection is closed
	IsClosed() bool

	// SetHeaders sets the required SSE headers
	SetHeaders()

	// Flush ensures immediate delivery of buffered data
	Flush() error
}

// ClientRepository defines the interface for managing client connections
// This follows the Repository pattern for client lifecycle management
type ClientRepository interface {
	// RegisterClient adds a new client to the active connections
	RegisterClient(client *Client, streamer SSEStreamer) error

	// UnregisterClient removes a client from active connections
	UnregisterClient(clientID string) error

	// GetClient retrieves a client by ID
	GetClient(clientID string) (*Client, SSEStreamer, bool)

	// GetAllClients returns all active clients
	GetAllClients() map[string]*Client

	// GetActiveCount returns the number of active connections
	GetActiveCount() int

	// CleanupIdleClients removes clients that have been idle too long
	CleanupIdleClients(maxIdleTime time.Duration) int

	// BroadcastToAll sends a message to all connected clients
	BroadcastToAll(message *Message) error

	// BroadcastToClients sends a message to specific clients
	BroadcastToClients(clientIDs []string, message *Message) error
}

// MetricsRepository defines the interface for collecting and storing metrics
// This enables monitoring and observability of the streaming service
type MetricsRepository interface {
	// IncrementConnections tracks new connections
	IncrementConnections()

	// DecrementConnections tracks connection closures
	DecrementConnections()

	// IncrementMessages tracks messages sent
	IncrementMessages()

	// IncrementErrors tracks errors encountered
	IncrementErrors()

	// RecordTokenUsage tracks token consumption by provider
	RecordTokenUsage(provider string, usage *TokenUsage)

	// RecordResponseTime tracks response latency
	RecordResponseTime(duration time.Duration)

	// RecordProviderMetrics tracks provider-specific metrics
	RecordProviderMetrics(provider string, duration time.Duration, success bool)

	// GetMetrics returns current server metrics
	GetMetrics() *ServerMetrics

	// ResetMetrics clears all metrics (useful for testing)
	ResetMetrics()
}

// ConfigRepository defines the interface for configuration management
// This abstraction allows for dynamic configuration updates
type ConfigRepository interface {
	// GetConfig returns the current configuration
	GetConfig() interface{}

	// UpdateConfig updates configuration values
	UpdateConfig(updates map[string]interface{}) error

	// ReloadConfig reloads configuration from source
	ReloadConfig() error

	// WatchConfig notifies when configuration changes
	WatchConfig() <-chan interface{}
}

// HealthChecker defines the interface for health monitoring
// This enables comprehensive health checks across all system components
type HealthChecker interface {
	// CheckHealth performs a comprehensive health check
	CheckHealth(ctx context.Context) *HealthStatus

	// CheckProviders checks the health of all LLM providers
	CheckProviders(ctx context.Context) map[string]*CheckResult

	// CheckConnectivity verifies external service connectivity
	CheckConnectivity(ctx context.Context) *CheckResult

	// CheckResources monitors system resource usage
	CheckResources(ctx context.Context) *CheckResult
}

// Logger defines the interface for structured logging
// This abstraction allows for different logging implementations
type Logger interface {
	// Debug logs debug-level messages
	Debug(msg string, fields ...interface{})

	// Info logs info-level messages
	Info(msg string, fields ...interface{})

	// Warn logs warning-level messages
	Warn(msg string, fields ...interface{})

	// Error logs error-level messages
	Error(msg string, err error, fields ...interface{})

	// With returns a logger with additional context fields
	With(fields ...interface{}) Logger
}

// StreamingService defines the main business logic interface
// This is the core service that orchestrates streaming chat functionality
type StreamingService interface {
	// StartStreaming initiates a streaming chat session
	StartStreaming(ctx context.Context, request *StreamingRequest, streamer SSEStreamer) error

	// HandleClientConnection manages a new client connection
	HandleClientConnection(ctx context.Context, clientID string, streamer SSEStreamer) error

	// BroadcastMessage sends a message to all or specific clients
	BroadcastMessage(ctx context.Context, message *Message, clientIDs ...string) error

	// GetAvailableProviders returns configured and available LLM providers
	GetAvailableProviders(ctx context.Context) []string

	// GetProviderModels returns available models for a specific provider
	GetProviderModels(ctx context.Context, provider string) ([]string, error)

	// StopStreaming cancels an ongoing streaming session
	StopStreaming(ctx context.Context, clientID string) error
	
	// GetActiveClientCount returns the number of active clients
	GetActiveClientCount() int
}

// EventBus defines the interface for internal event communication
// This enables loose coupling between different parts of the system
type EventBus interface {
	// Publish sends an event to all subscribers
	Publish(event string, data interface{})

	// Subscribe registers a handler for specific events
	Subscribe(event string, handler func(data interface{}))

	// Unsubscribe removes a handler for specific events
	Unsubscribe(event string, handler func(data interface{}))
}

// RateLimiter defines the interface for rate limiting
// This helps prevent abuse and manage resource consumption
type RateLimiter interface {
	// Allow checks if a request is allowed for the given key
	Allow(key string) bool

	// GetLimit returns the current limit for a key
	GetLimit(key string) int

	// GetRemaining returns remaining requests for a key
	GetRemaining(key string) int

	// Reset resets the counter for a key
	Reset(key string)
}

// Cache defines the interface for caching frequently accessed data
// This improves performance by reducing expensive operations
type Cache interface {
	// Get retrieves a value from cache
	Get(key string) (interface{}, bool)

	// Set stores a value in cache with expiration
	Set(key string, value interface{}, expiration time.Duration)

	// Delete removes a value from cache
	Delete(key string)

	// Clear removes all values from cache
	Clear()

	// Stats returns cache statistics
	Stats() map[string]interface{}
}

// MessageValidator defines the interface for validating messages
// This ensures data integrity and security
type MessageValidator interface {
	// ValidateRequest checks if a streaming request is valid
	ValidateRequest(request *StreamingRequest) error

	// ValidateMessage checks if a message is valid
	ValidateMessage(message *Message) error

	// SanitizeContent cleans and sanitizes message content
	SanitizeContent(content string) string

	// CheckContentPolicy verifies content against policies
	CheckContentPolicy(content string) error
}
