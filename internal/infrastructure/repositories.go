package infrastructure

import (
	"fmt"
	"sync"
	"time"

	"sse-streaming-chat/internal/domain"
)

// inMemoryClientRepository implements ClientRepository using in-memory storage
type inMemoryClientRepository struct {
	clients map[string]*clientConnection
	mutex   sync.RWMutex
	logger  domain.Logger
}

// clientConnection wraps a domain client with its SSE streamer
type clientConnection struct {
	client   *domain.Client
	streamer domain.SSEStreamer
}

// NewInMemoryClientRepository creates a new in-memory client repository
func NewInMemoryClientRepository(logger domain.Logger) domain.ClientRepository {
	return &inMemoryClientRepository{
		clients: make(map[string]*clientConnection),
		logger:  logger.With("component", "client_repository"),
	}
}

// RegisterClient adds a new client to the active connections
func (r *inMemoryClientRepository) RegisterClient(client *domain.Client, streamer domain.SSEStreamer) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.clients[client.ID] = &clientConnection{
		client:   client,
		streamer: streamer,
	}
	
	r.logger.Info("Client registered", "client_id", client.ID, "total_clients", len(r.clients))
	return nil
}

// UnregisterClient removes a client from active connections
func (r *inMemoryClientRepository) UnregisterClient(clientID string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if conn, exists := r.clients[clientID]; exists {
		// Close the streamer
		conn.streamer.Close()
		delete(r.clients, clientID)
		r.logger.Info("Client unregistered", "client_id", clientID, "total_clients", len(r.clients))
	}
	
	return nil
}

// GetClient retrieves a client by ID
func (r *inMemoryClientRepository) GetClient(clientID string) (*domain.Client, domain.SSEStreamer, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	if conn, exists := r.clients[clientID]; exists {
		return conn.client, conn.streamer, true
	}
	
	return nil, nil, false
}

// GetAllClients returns all active clients
func (r *inMemoryClientRepository) GetAllClients() map[string]*domain.Client {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	result := make(map[string]*domain.Client)
	for id, conn := range r.clients {
		result[id] = conn.client
	}
	
	return result
}

// GetActiveCount returns the number of active connections
func (r *inMemoryClientRepository) GetActiveCount() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	return len(r.clients)
}

// CleanupIdleClients removes clients that have been idle too long
func (r *inMemoryClientRepository) CleanupIdleClients(maxIdleTime time.Duration) int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	now := time.Now()
	var removedCount int
	
	for id, conn := range r.clients {
		if now.Sub(conn.client.LastActivity) > maxIdleTime {
			conn.streamer.Close()
			delete(r.clients, id)
			removedCount++
			r.logger.Debug("Idle client removed", "client_id", id)
		}
	}
	
	if removedCount > 0 {
		r.logger.Info("Cleaned up idle clients", "removed", removedCount, "remaining", len(r.clients))
	}
	
	return removedCount
}

// BroadcastToAll sends a message to all connected clients
func (r *inMemoryClientRepository) BroadcastToAll(message *domain.Message) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	var errors []error
	var disconnectedClients []string
	
	for id, conn := range r.clients {
		if err := conn.streamer.WriteMessage(message); err != nil {
			r.logger.Error("Failed to broadcast to client", err, "client_id", id)
			errors = append(errors, fmt.Errorf("client %s: %w", id, err))
			// Mark client for removal if the connection is closed
			if conn.streamer.IsClosed() {
				disconnectedClients = append(disconnectedClients, id)
			}
		}
	}
	
	// Remove disconnected clients
	for _, clientID := range disconnectedClients {
		delete(r.clients, clientID)
		r.logger.Info("Removed disconnected client", "client_id", clientID)
	}
	
	if len(errors) > 0 && len(disconnectedClients) == 0 {
		return fmt.Errorf("broadcast errors: %v", errors)
	}
	
	r.logger.Debug("Message broadcasted to all clients", "client_count", len(r.clients), "removed", len(disconnectedClients))
	return nil
}

// BroadcastToClients sends a message to specific clients
func (r *inMemoryClientRepository) BroadcastToClients(clientIDs []string, message *domain.Message) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	var errors []error
	var successCount int
	
	for _, id := range clientIDs {
		if conn, exists := r.clients[id]; exists {
			if err := conn.streamer.WriteMessage(message); err != nil {
				r.logger.Error("Failed to broadcast to specific client", err, "client_id", id)
				errors = append(errors, fmt.Errorf("client %s: %w", id, err))
			} else {
				successCount++
			}
		} else {
			errors = append(errors, fmt.Errorf("client %s: not found", id))
		}
	}
	
	r.logger.Debug("Message broadcasted to specific clients", 
		"target_count", len(clientIDs), 
		"success_count", successCount, 
		"error_count", len(errors))
	
	if len(errors) > 0 {
		return fmt.Errorf("broadcast errors: %v", errors)
	}
	
	return nil
}

// inMemoryMetricsRepository implements MetricsRepository using in-memory storage
type inMemoryMetricsRepository struct {
	metrics *domain.ServerMetrics
	mutex   sync.RWMutex
	startTime time.Time
}

// NewInMemoryMetricsRepository creates a new in-memory metrics repository
func NewInMemoryMetricsRepository() domain.MetricsRepository {
	return &inMemoryMetricsRepository{
		metrics: &domain.ServerMetrics{
			ProviderStats: make(map[string]*domain.ProviderMetrics),
		},
		startTime: time.Now(),
	}
}

// IncrementConnections tracks new connections
func (r *inMemoryMetricsRepository) IncrementConnections() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.metrics.ActiveConnections++
	r.metrics.TotalConnections++
}

// DecrementConnections tracks connection closures
func (r *inMemoryMetricsRepository) DecrementConnections() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if r.metrics.ActiveConnections > 0 {
		r.metrics.ActiveConnections--
	}
}

// IncrementMessages tracks messages sent
func (r *inMemoryMetricsRepository) IncrementMessages() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.metrics.TotalMessages++
}

// IncrementErrors tracks errors encountered
func (r *inMemoryMetricsRepository) IncrementErrors() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.metrics.ErrorCount++
}

// RecordTokenUsage tracks token consumption by provider
func (r *inMemoryMetricsRepository) RecordTokenUsage(provider string, usage *domain.TokenUsage) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if r.metrics.ProviderStats[provider] == nil {
		r.metrics.ProviderStats[provider] = &domain.ProviderMetrics{}
	}
	
	r.metrics.ProviderStats[provider].TokensUsed += int64(usage.TotalTokens)
	r.metrics.ProviderStats[provider].LastUsed = time.Now()
	r.metrics.TotalTokensUsed += int64(usage.TotalTokens)
}

// RecordResponseTime tracks response latency
func (r *inMemoryMetricsRepository) RecordResponseTime(duration time.Duration) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	// Simple moving average calculation
	if r.metrics.AverageResponseTime == 0 {
		r.metrics.AverageResponseTime = float64(duration.Milliseconds())
	} else {
		r.metrics.AverageResponseTime = (r.metrics.AverageResponseTime + float64(duration.Milliseconds())) / 2
	}
}

// RecordProviderMetrics tracks provider-specific metrics
func (r *inMemoryMetricsRepository) RecordProviderMetrics(provider string, duration time.Duration, success bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if r.metrics.ProviderStats[provider] == nil {
		r.metrics.ProviderStats[provider] = &domain.ProviderMetrics{}
	}
	
	stats := r.metrics.ProviderStats[provider]
	stats.RequestCount++
	stats.LastUsed = time.Now()
	
	// Update average response time
	if stats.AverageResponseTime == 0 {
		stats.AverageResponseTime = float64(duration.Milliseconds())
	} else {
		stats.AverageResponseTime = (stats.AverageResponseTime + float64(duration.Milliseconds())) / 2
	}
	
	if !success {
		stats.ErrorCount++
	}
}

// GetMetrics returns current server metrics
func (r *inMemoryMetricsRepository) GetMetrics() *domain.ServerMetrics {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	// Create a copy to avoid race conditions
	metricsCopy := *r.metrics
	metricsCopy.Uptime = int64(time.Since(r.startTime).Seconds())
	
	// Copy provider stats
	metricsCopy.ProviderStats = make(map[string]*domain.ProviderMetrics)
	for provider, stats := range r.metrics.ProviderStats {
		statsCopy := *stats
		metricsCopy.ProviderStats[provider] = &statsCopy
	}
	
	return &metricsCopy
}

// ResetMetrics clears all metrics (useful for testing)
func (r *inMemoryMetricsRepository) ResetMetrics() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.metrics = &domain.ServerMetrics{
		ProviderStats: make(map[string]*domain.ProviderMetrics),
	}
	r.startTime = time.Now()
}