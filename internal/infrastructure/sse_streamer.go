package infrastructure

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"sse-streaming-chat/internal/domain"
)

// sseStreamer implements the SSEStreamer interface for Server-Sent Events
// It handles the SSE protocol details and provides a clean interface for sending messages
type sseStreamer struct {
	// writer is the HTTP response writer
	writer http.ResponseWriter

	// flusher enables immediate data transmission
	flusher http.Flusher

	// closed indicates if the connection is closed
	closed bool

	// mutex protects concurrent access to the streamer state
	mutex sync.RWMutex

	// logger for debugging and monitoring
	logger domain.Logger
}

// NewSSEStreamer creates a new SSE streamer instance
// It sets up the HTTP response writer for SSE communication
func NewSSEStreamer(w http.ResponseWriter, logger domain.Logger) (domain.SSEStreamer, error) {
	// Check if the writer supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}

	streamer := &sseStreamer{
		writer:  w,
		flusher: flusher,
		closed:  false,
		logger:  logger.With("component", "sse_streamer"),
	}

	// Set SSE headers
	streamer.SetHeaders()

	return streamer, nil
}

// SetHeaders sets the required SSE headers
// These headers are essential for proper SSE functionality
func (s *sseStreamer) SetHeaders() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return
	}

	// Set Content-Type for SSE
	s.writer.Header().Set("Content-Type", "text/event-stream")

	// Disable caching to ensure real-time delivery
	s.writer.Header().Set("Cache-Control", "no-cache")

	// Maintain persistent connection
	s.writer.Header().Set("Connection", "keep-alive")

	// Enable CORS for web clients
	s.writer.Header().Set("Access-Control-Allow-Origin", "*")
	s.writer.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
	s.writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

// WriteMessage sends a message via SSE
// It converts the domain message to SSE format and sends it immediately
func (s *sseStreamer) WriteMessage(message *domain.Message) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return fmt.Errorf("connection is closed")
	}

	// Check if writer is still valid
	if s.writer == nil {
		s.closed = true
		return fmt.Errorf("writer is nil, connection closed")
	}

	// Convert message to JSON
	jsonData, err := json.Marshal(message)
	if err != nil {
		s.logger.Error("Failed to marshal message", err, "message", message)
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Format as SSE message
	// SSE format: "data: <json>\n\n"
	sseMessage := fmt.Sprintf("data: %s\n\n", jsonData)

	// Write to the connection
	if _, err := s.writer.Write([]byte(sseMessage)); err != nil {
		s.logger.Error("Failed to write SSE message", err)
		s.closed = true
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Flush immediately for real-time delivery - use defer to catch panics
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic during flush, connection likely closed", nil, "panic", r)
			s.closed = true
		}
	}()

	if s.flusher != nil {
		s.flusher.Flush()
	}

	s.logger.Debug("SSE message sent", "type", message.Type, "content_length", len(message.Content))
	return nil
}

// WriteError sends an error message via SSE
// It formats domain errors as SSE messages with error type
func (s *sseStreamer) WriteError(err *domain.Error) error {
	// Convert domain error to message
	errorMessage := &domain.Message{
		ID:        fmt.Sprintf("error_%d", err.Timestamp.UnixNano()),
		Content:   err.Message,
		Type:      domain.MessageTypeError,
		Timestamp: err.Timestamp,
		Metadata: map[string]interface{}{
			"error_code": err.Code,
			"details":    err.Details,
		},
	}

	return s.WriteMessage(errorMessage)
}

// Flush ensures immediate delivery of buffered data
func (s *sseStreamer) Flush() error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.closed {
		return fmt.Errorf("connection is closed")
	}

	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

// Close terminates the SSE connection
// It marks the connection as closed and sends a final message if possible
func (s *sseStreamer) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return nil
	}

	// Send a close message before terminating
	closeMessage := &domain.Message{
		ID:        fmt.Sprintf("close_%d", time.Now().UnixNano()),
		Content:   "Connection closed",
		Type:      domain.MessageTypeSystem,
		Timestamp: time.Now(),
	}

	// Try to send close message (ignore errors since we're closing anyway)
	if jsonData, err := json.Marshal(closeMessage); err == nil {
		sseMessage := fmt.Sprintf("data: %s\n\n", jsonData)
		s.writer.Write([]byte(sseMessage))
		if s.flusher != nil {
			s.flusher.Flush()
		}
	}

	s.closed = true
	s.logger.Debug("SSE connection closed")
	return nil
}

// IsClosed returns true if the connection is closed
func (s *sseStreamer) IsClosed() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.closed
}

// SendKeepAlive sends a keep-alive message to maintain the connection
// This is useful for long-running connections to prevent timeouts
func (s *sseStreamer) SendKeepAlive() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return fmt.Errorf("connection is closed")
	}

	// Send SSE comment as keep-alive (comments don't appear in the event stream)
	keepAlive := ": keep-alive\n\n"

	if _, err := s.writer.Write([]byte(keepAlive)); err != nil {
		s.logger.Error("Failed to send keep-alive", err)
		s.closed = true
		return fmt.Errorf("failed to send keep-alive: %w", err)
	}

	if s.flusher != nil {
		s.flusher.Flush()
	}
	s.logger.Debug("Keep-alive sent")
	return nil
}

// SendEvent sends a custom SSE event with a specific event type
// This allows for different types of events beyond the standard data events
func (s *sseStreamer) SendEvent(eventType, data string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return fmt.Errorf("connection is closed")
	}

	// Format custom SSE event
	// Format: "event: <type>\ndata: <data>\n\n"
	eventMessage := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)

	if _, err := s.writer.Write([]byte(eventMessage)); err != nil {
		s.logger.Error("Failed to send custom event", err, "event_type", eventType)
		s.closed = true
		return fmt.Errorf("failed to send event: %w", err)
	}

	if s.flusher != nil {
		s.flusher.Flush()
	}
	s.logger.Debug("Custom SSE event sent", "event_type", eventType)
	return nil
}

// WriteRetry sets the retry interval for automatic reconnection
// This tells the client how long to wait before attempting to reconnect
func (s *sseStreamer) WriteRetry(milliseconds int) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return fmt.Errorf("connection is closed")
	}

	// Format retry directive
	retryMessage := fmt.Sprintf("retry: %d\n\n", milliseconds)

	if _, err := s.writer.Write([]byte(retryMessage)); err != nil {
		s.logger.Error("Failed to send retry directive", err)
		s.closed = true
		return fmt.Errorf("failed to send retry: %w", err)
	}

	if s.flusher != nil {
		s.flusher.Flush()
	}
	s.logger.Debug("SSE retry directive sent", "milliseconds", milliseconds)
	return nil
}
