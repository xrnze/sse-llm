package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sse-streaming-chat/internal/domain"
	"time"

	"github.com/gorilla/mux"
)

type httpRouter struct {
	streamingService domain.StreamingService
	healthChecker    domain.HealthChecker
	metricsRepo      domain.MetricsRepository
	logger           domain.Logger
	router           *mux.Router
}

func NewRouter(
	streamingService domain.StreamingService,
	healthChecker domain.HealthChecker,
	metricsRepo domain.MetricsRepository,
	logger domain.Logger,
) http.Handler {
	r := &httpRouter{
		streamingService: streamingService,
		healthChecker:    healthChecker,
		metricsRepo:      metricsRepo,
		logger:           logger,
		router:           mux.NewRouter(),
	}

	r.setupRoutes()
	return r.router
}

func (r *httpRouter) setupRoutes() {
	// All endpoints in here

	// Core SSE functionality
	r.router.HandleFunc("/stream", r.handleSSEConnection).Methods("GET")
	r.router.HandleFunc("/chat", r.handleChatRequest).Methods("POST", "OPTIONS")

	// Web interface for testing
	r.router.HandleFunc("/", r.handleWebClient).Methods("GET")

	// RESTful API endpoints
	r.router.HandleFunc("/api/providers", r.handleGetProviders).Methods("GET")
	r.router.HandleFunc("/api/providers/{provider}/models", r.handleGetProviderModels).Methods("GET")

	// Operational endpoints
	r.router.HandleFunc("/health", r.handleHealth).Methods("GET")
	r.router.HandleFunc("/metrics", r.handleMetrics).Methods("GET")

	// CORS preflight handling
	r.router.Methods("OPTIONS").HandlerFunc(r.handleCORS)

}

func (r *httpRouter) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cache-Control")
}

func (r *httpRouter) handleSSEConnection(w http.ResponseWriter, req *http.Request) {
	r.logger.Info("New SSE connection request")

	// 1. Resource Creation
	streamer, err := NewSSEStreamer(w, r.logger)
	if err != nil {
		r.logger.Error("Failed to create SSE streamer", err)
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 2. Client Registration
	clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())

	// 3. Business logic delegation
	if err := r.streamingService.HandleClientConnection(req.Context(), clientID, streamer); err != nil {
		r.logger.Error("Failed to handle client connection", err, "client_id", clientID)
		http.Error(w, "Handle client connection failed", http.StatusInternalServerError)
		return
	}

	// 4. Connection maintenance (blocks until cancellation)
	<-req.Context().Done()
	r.logger.Info("SSE connection closed", "client_id", clientID)
}

func (r *httpRouter) handleChatRequest(w http.ResponseWriter, req *http.Request) {
	// Handle CORS
	r.setCORSHeaders(w)

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.logger.Info("Chat request received")

	// Parse request body
	var requestData struct {
		Prompt      string           `json:"prompt"`
		Provider    string           `json:"provider,omitempty"`
		Model       string           `json:"model,omitempty"`
		Temperature *float32         `json:"temperature,omitempty"`
		MaxTokens   *int             `json:"max_tokens,omitempty"`
		History     []domain.Message `json:"history,omitempty"`
		ClientID    string           `json:"client_id,omitempty"`
	}

	if err := json.NewDecoder(req.Body).Decode(&requestData); err != nil {
		r.logger.Error("Failed to decode request", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create domain request
	streamingRequest := domain.NewStreamingRequest(requestData.Prompt).
		WithProvider(requestData.Provider).
		WithModel(requestData.Model).
		WithConversationHistory(requestData.History)

	if requestData.Temperature != nil {
		streamingRequest.WithTemperature(*requestData.Temperature)
	}

	if requestData.MaxTokens != nil {
		streamingRequest.WithMaxTokens(*requestData.MaxTokens)
	}

	streamingRequest.ClientID = requestData.ClientID

	// Get all connected clients to broadcast the response
	providers := r.streamingService.GetAvailableProviders(req.Context())
	if len(providers) == 0 {
		http.Error(w, "No providers available", http.StatusServiceUnavailable)
		return
	}

	// Start streaming to all connected clients
	// Note: In a real application, you might want to stream only to the requesting client
	go func() {
		// We'll broadcast a system message about the new request
		requestMsg := domain.NewMessage(
			fmt.Sprintf("Processing request: %s", requestData.Prompt),
			domain.MessageTypeSystem,
		)

		if err := r.streamingService.BroadcastMessage(context.Background(), requestMsg); err != nil {
			r.logger.Error("Failed to broadcast request message", err)
		}

		// Create context that can be cancelled but doesn't timeout
		streamCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create a dummy streamer for the response (we'll broadcast instead)
		dummyStreamer := &dummyStreamer{
			streamingService: r.streamingService,
			logger:           r.logger,
			cancel:           cancel, // Allow streamer to cancel if no clients
		}

		// Start streaming with timeout context
		if err := r.streamingService.StartStreaming(streamCtx, streamingRequest, dummyStreamer); err != nil {
			r.logger.Error("Failed to start streaming", err)
		}
	}()

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "streaming",
		"providers":  providers,
		"request_id": fmt.Sprintf("req_%d", time.Now().UnixNano()),
	})
}

func (r *httpRouter) handleWebClient(w http.ResponseWriter, req *http.Request) {
	htmlFile, err := os.Open(`internal/template/index.html`)
	if err != nil {
		r.logger.Error("Failed to open HTML file", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer htmlFile.Close()

	html, err := io.ReadAll(htmlFile)
	if err != nil {
		r.logger.Error("Failed to read HTML file", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(html)
}

// handleGetProviders returns available LLM providers
func (r *httpRouter) handleGetProviders(w http.ResponseWriter, req *http.Request) {
	r.setCORSHeaders(w)

	providers := r.streamingService.GetAvailableProviders(req.Context())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providers)
}

// handleGetProviderModels returns models for a specific provider
func (r *httpRouter) handleGetProviderModels(w http.ResponseWriter, req *http.Request) {
	r.setCORSHeaders(w)

	vars := mux.Vars(req)
	provider := vars["provider"]

	models, err := r.streamingService.GetProviderModels(req.Context(), provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

// handleCORS handles CORS preflight requests
func (r *httpRouter) handleCORS(w http.ResponseWriter, req *http.Request) {
	r.setCORSHeaders(w)
	w.WriteHeader(http.StatusOK)
}

func (r *httpRouter) handleHealth(w http.ResponseWriter, req *http.Request) {
	r.setCORSHeaders(w)
	health := r.healthChecker.CheckHealth(req.Context())

	status := http.StatusOK
	if health.Status == "unhealthy" {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(health)
}

// Metrics for monitoring and alerting
func (r *httpRouter) handleMetrics(w http.ResponseWriter, req *http.Request) {
	r.setCORSHeaders(w)
	metrics := r.metricsRepo.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// dummyStreamer implements SSEStreamer for broadcasting instead of direct streaming
type dummyStreamer struct {
	streamingService domain.StreamingService
	logger           domain.Logger
	cancel           context.CancelFunc
}

func (d *dummyStreamer) WriteMessage(message *domain.Message) error {
	// Check if there are any active clients before broadcasting
	if d.streamingService.GetActiveClientCount() == 0 {
		d.logger.Info("No active clients, cancelling streaming")
		if d.cancel != nil {
			d.cancel()
		}
		return nil
	}

	err := d.streamingService.BroadcastMessage(context.Background(), message)
	return err
}

func (d *dummyStreamer) WriteError(err *domain.Error) error {
	errorMsg := &domain.Message{
		ID:        fmt.Sprintf("error_%d", time.Now().UnixNano()),
		Content:   err.Message,
		Type:      domain.MessageTypeError,
		Timestamp: time.Now(),
	}
	return d.streamingService.BroadcastMessage(context.Background(), errorMsg)
}

func (d *dummyStreamer) Close() error   { return nil }
func (d *dummyStreamer) IsClosed() bool { return false }
func (d *dummyStreamer) SetHeaders()    {}
func (d *dummyStreamer) Flush() error   { return nil }
