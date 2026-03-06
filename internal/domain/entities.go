package domain

import (
	"fmt"
	"time"
)

// Message represents a chat message in the domain
// This is the core entity that flows through the application
type Message struct {
	// ID uniquely identifies the message
	ID string `json:"id"`

	// Content is the actual message text
	Content string `json:"content"`

	// Type indicates the message type (user, assistant, system, error)
	Type MessageType `json:"type"`

	// Provider indicates which LLM generated this message
	Provider string `json:"provider,omitempty"`

	// TokenIndex tracks the position in streaming sequence
	TokenIndex int `json:"token_index,omitempty"`

	// Timestamp when the message was created
	Timestamp time.Time `json:"timestamp"`

	// Metadata for additional information
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// MessageType represents different types of messages
type MessageType string

const (
	// MessageTypeUser represents messages from users
	MessageTypeUser MessageType = "user"

	// MessageTypeAssistant represents responses from LLMs
	MessageTypeAssistant MessageType = "assistant"

	// MessageTypeSystem represents system messages
	MessageTypeSystem MessageType = "system"

	// MessageTypeError represents error messages
	MessageTypeError MessageType = "error"

	// MessageTypeConnection represents connection status messages
	MessageTypeConnection MessageType = "connection"

	// MessageTypeData represents streaming data chunks
	MessageTypeData MessageType = "data"

	// MessageTypeDone represents completion of streaming
	MessageTypeDone MessageType = "done"
)

// StreamingRequest represents a request for streaming chat completion
type StreamingRequest struct {
	// Prompt is the user's input message
	Prompt string `json:"prompt"`

	// Provider specifies which LLM to use ("openai", "grok", or empty for default)
	Provider string `json:"provider,omitempty"`

	// Model overrides the default model for the provider
	Model string `json:"model,omitempty"`

	// Temperature controls randomness in responses
	Temperature *float32 `json:"temperature,omitempty"`

	// MaxTokens limits the response length
	MaxTokens *int `json:"max_tokens,omitempty"`

	// ConversationHistory for context-aware responses
	ConversationHistory []Message `json:"conversation_history,omitempty"`

	// ClientID identifies the requesting client
	ClientID string `json:"client_id,omitempty"`
}

// StreamingResponse represents a response chunk during streaming
type StreamingResponse struct {
	// Message contains the actual response data
	Message Message `json:"message"`

	// IsComplete indicates if this is the final chunk
	IsComplete bool `json:"is_complete"`

	// Error contains error information if applicable
	Error string `json:"error,omitempty"`

	// Usage contains token usage information
	Usage *TokenUsage `json:"usage,omitempty"`
}

// TokenUsage tracks token consumption for billing and monitoring
type TokenUsage struct {
	// PromptTokens used for the input
	PromptTokens int `json:"prompt_tokens"`

	// CompletionTokens used for the output
	CompletionTokens int `json:"completion_tokens"`

	// TotalTokens is the sum of prompt and completion tokens
	TotalTokens int `json:"total_tokens"`
}

// Client represents a connected SSE client
type Client struct {
	// ID uniquely identifies the client
	ID string `json:"id"`

	// ConnectedAt tracks when the client connected
	ConnectedAt time.Time `json:"connected_at"`

	// LastActivity tracks the last interaction time
	LastActivity time.Time `json:"last_activity"`

	// UserAgent from the HTTP request
	UserAgent string `json:"user_agent,omitempty"`

	// RemoteAddr is the client's IP address
	RemoteAddr string `json:"remote_addr,omitempty"`
}

// ServerMetrics represents server performance metrics
type ServerMetrics struct {
	// ActiveConnections counts current SSE connections
	ActiveConnections int64 `json:"active_connections"`

	// TotalConnections counts all connections since startup
	TotalConnections int64 `json:"total_connections"`

	// TotalMessages counts all messages sent
	TotalMessages int64 `json:"total_messages"`

	// TotalTokensUsed tracks token consumption across all providers
	TotalTokensUsed int64 `json:"total_tokens_used"`

	// AverageResponseTime in milliseconds
	AverageResponseTime float64 `json:"average_response_time_ms"`

	// Uptime in seconds since server start
	Uptime int64 `json:"uptime_seconds"`

	// ErrorCount tracks total errors
	ErrorCount int64 `json:"error_count"`

	// ProviderStats tracks usage by LLM provider
	ProviderStats map[string]*ProviderMetrics `json:"provider_stats"`
}

// ProviderMetrics tracks metrics for a specific LLM provider
type ProviderMetrics struct {
	// RequestCount tracks total requests to this provider
	RequestCount int64 `json:"request_count"`

	// TokensUsed tracks total tokens consumed
	TokensUsed int64 `json:"tokens_used"`

	// AverageResponseTime for this provider
	AverageResponseTime float64 `json:"average_response_time_ms"`

	// ErrorCount for this provider
	ErrorCount int64 `json:"error_count"`

	// LastUsed timestamp
	LastUsed time.Time `json:"last_used"`
}

// HealthStatus represents the health of the application
type HealthStatus struct {
	// Status is the overall health ("healthy", "unhealthy", "degraded")
	Status string `json:"status"`

	// Timestamp when health was checked
	Timestamp time.Time `json:"timestamp"`

	// Version of the application
	Version string `json:"version"`

	// Uptime in seconds
	Uptime int64 `json:"uptime"`

	// Checks contains detailed health check results
	Checks map[string]CheckResult `json:"checks"`
}

// CheckResult represents the result of a specific health check
type CheckResult struct {
	// Status of this check ("pass", "fail", "warn")
	Status string `json:"status"`

	// Message describing the check result
	Message string `json:"message,omitempty"`

	// Duration of the check in milliseconds
	Duration int64 `json:"duration_ms"`
}

// Error represents domain-specific errors
type Error struct {
	// Code is a machine-readable error code
	Code string `json:"code"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// Details provides additional error context
	Details map[string]interface{} `json:"details,omitempty"`

	// Timestamp when the error occurred
	Timestamp time.Time `json:"timestamp"`
}

// Error codes for common domain errors
const (
	// Provider-related errors
	ErrorCodeProviderNotFound    = "PROVIDER_NOT_FOUND"
	ErrorCodeProviderUnavailable = "PROVIDER_UNAVAILABLE"
	ErrorCodeProviderRateLimit   = "PROVIDER_RATE_LIMIT"
	ErrorCodeProviderInvalidKey  = "PROVIDER_INVALID_KEY"

	// Request-related errors
	ErrorCodeInvalidRequest  = "INVALID_REQUEST"
	ErrorCodeRequestTooLarge = "REQUEST_TOO_LARGE"
	ErrorCodeInvalidModel    = "INVALID_MODEL"

	// Connection-related errors
	ErrorCodeConnectionClosed      = "CONNECTION_CLOSED"
	ErrorCodeMaxConnectionsReached = "MAX_CONNECTIONS_REACHED"

	// Internal errors
	ErrorCodeInternalError      = "INTERNAL_ERROR"
	ErrorCodeConfigurationError = "CONFIGURATION_ERROR"
)

// NewError creates a new domain error
func NewError(code, message string) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// WithDetails adds details to a domain error
func (e *Error) WithDetails(details map[string]interface{}) *Error {
	e.Details = details
	return e
}

// Error implements the error interface
func (e *Error) Error() string {
	return e.Message
}

// NewMessage creates a new message with the given parameters
func NewMessage(content string, msgType MessageType) *Message {
	return &Message{
		ID:        generateID(),
		Content:   content,
		Type:      msgType,
		Timestamp: time.Now(),
	}
}

// NewStreamingRequest creates a new streaming request
func NewStreamingRequest(prompt string) *StreamingRequest {
	return &StreamingRequest{
		Prompt: prompt,
	}
}

// WithProvider sets the provider for the streaming request
func (sr *StreamingRequest) WithProvider(provider string) *StreamingRequest {
	sr.Provider = provider
	return sr
}

// WithModel sets the model for the streaming request
func (sr *StreamingRequest) WithModel(model string) *StreamingRequest {
	sr.Model = model
	return sr
}

// WithTemperature sets the temperature for the streaming request
func (sr *StreamingRequest) WithTemperature(temperature float32) *StreamingRequest {
	sr.Temperature = &temperature
	return sr
}

// WithMaxTokens sets the max tokens for the streaming request
func (sr *StreamingRequest) WithMaxTokens(maxTokens int) *StreamingRequest {
	sr.MaxTokens = &maxTokens
	return sr
}

// WithConversationHistory sets the conversation history
func (sr *StreamingRequest) WithConversationHistory(history []Message) *StreamingRequest {
	sr.ConversationHistory = history
	return sr
}

// generateID generates a unique ID for messages
func generateID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}
