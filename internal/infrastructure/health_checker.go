package infrastructure

import (
	"context"
	"runtime"
	"time"

	"sse-streaming-chat/internal/domain"
)

// healthChecker implements domain.HealthChecker interface
type healthChecker struct {
	providers   map[string]domain.LLMProvider
	metricsRepo domain.MetricsRepository
	logger      domain.Logger
	startTime   time.Time
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(
	providers map[string]domain.LLMProvider,
	metricsRepo domain.MetricsRepository,
	logger domain.Logger,
) domain.HealthChecker {
	return &healthChecker{
		providers:   providers,
		metricsRepo: metricsRepo,
		logger:      logger.With("component", "health_checker"),
		startTime:   time.Now(),
	}
}

// CheckHealth performs a comprehensive health check
func (h *healthChecker) CheckHealth(ctx context.Context) *domain.HealthStatus {
	h.logger.Debug("Performing health check")
	
	status := &domain.HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   "1.0.0",
		Uptime:    int64(time.Since(h.startTime).Seconds()),
		Checks:    make(map[string]domain.CheckResult),
	}
	
	// Check providers
	providerChecks := h.CheckProviders(ctx)
	for name, result := range providerChecks {
		status.Checks["provider_"+name] = *result
		if result.Status == "fail" {
			status.Status = "degraded"
		}
	}
	
	// Check connectivity
	connectivityResult := h.CheckConnectivity(ctx)
	status.Checks["connectivity"] = *connectivityResult
	if connectivityResult.Status == "fail" {
		status.Status = "unhealthy"
	}
	
	// Check resources
	resourcesResult := h.CheckResources(ctx)
	status.Checks["resources"] = *resourcesResult
	if resourcesResult.Status == "fail" {
		status.Status = "unhealthy"
	}
	
	h.logger.Info("Health check completed", "status", status.Status)
	return status
}

// CheckProviders checks the health of all LLM providers
func (h *healthChecker) CheckProviders(ctx context.Context) map[string]*domain.CheckResult {
	results := make(map[string]*domain.CheckResult)
	
	for name, provider := range h.providers {
		h.logger.Debug("Checking provider health", "provider", name)
		results[name] = provider.HealthCheck(ctx)
	}
	
	return results
}

// CheckConnectivity verifies external service connectivity
func (h *healthChecker) CheckConnectivity(ctx context.Context) *domain.CheckResult {
	startTime := time.Now()
	
	// Basic connectivity check - ensure we can resolve DNS and make HTTP requests
	// In a real implementation, you might want to ping specific endpoints
	
	// For now, we'll just check if we have any providers available
	availableProviders := 0
	for _, provider := range h.providers {
		if provider.IsAvailable(ctx) {
			availableProviders++
		}
	}
	
	duration := time.Since(startTime)
	
	if availableProviders == 0 {
		return &domain.CheckResult{
			Status:   "fail",
			Message:  "No LLM providers are available",
			Duration: duration.Milliseconds(),
		}
	}
	
	return &domain.CheckResult{
		Status:   "pass",
		Message:  "External connectivity is working",
		Duration: duration.Milliseconds(),
	}
}

// CheckResources monitors system resource usage
func (h *healthChecker) CheckResources(ctx context.Context) *domain.CheckResult {
	startTime := time.Now()
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Get current metrics
	metrics := h.metricsRepo.GetMetrics()
	
	// Check memory usage (simple threshold check)
	memoryMB := m.Alloc / 1024 / 1024
	maxMemoryMB := uint64(1024) // 1GB threshold
	
	// Check active connections
	maxConnections := int64(1000) // Max connections threshold
	
	duration := time.Since(startTime)
	
	if memoryMB > maxMemoryMB {
		return &domain.CheckResult{
			Status:   "fail",
			Message:  "High memory usage detected",
			Duration: duration.Milliseconds(),
		}
	}
	
	if metrics.ActiveConnections > maxConnections {
		return &domain.CheckResult{
			Status:   "fail",
			Message:  "Too many active connections",
			Duration: duration.Milliseconds(),
		}
	}
	
	return &domain.CheckResult{
		Status:   "pass",
		Message:  "System resources are within normal limits",
		Duration: duration.Milliseconds(),
	}
}