package pro

import (
	"context"
	"fmt"
	"sse-streaming-chat/config"
	"sse-streaming-chat/internal/domain"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

type openRouterProvider struct {
	// client is the OpenAI-compatible client configured for Groq
	client *openai.Client
	// config holds Groq-specific configuration
	config config.LLMConfig
	// logger for structured logging
	logger domain.Logger
}

func NewOpenRouterProvider(cfg config.LLMConfig, logger domain.Logger) domain.LLMProvider {
	// Create OpenAI client configuration for OpenRouter
	clientConfig := openai.DefaultConfig(cfg.APIKey)

	// Set OpenRouter base URL
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	} else {
		clientConfig.BaseURL = "https://openrouter.ai/api/v1/"
	}

	// Create the client
	client := openai.NewClientWithConfig(clientConfig)

	return &openRouterProvider{
		client: client,
		config: cfg,
		logger: logger.With("provider", "openrouter"),
	}
}

// Name returns the provider name
func (p *openRouterProvider) Name() string {
	return "openrouter"
}

// IsAvailable checks if the OpenRouter provider is properly configured and reachable
func (p *openRouterProvider) IsAvailable(ctx context.Context) bool {
	// Quick validation - check if API key is set
	if p.config.APIKey == "" {
		p.logger.Warn("OpenRouter API key not configured")
		return false
	}

	// Create a context with timeout for the API call
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to make a simple API call to test connectivity
	// We'll use the models endpoint as it's lightweight
	_, err := p.client.ListModels(timeoutCtx)
	if err != nil {
		p.logger.Warn("OpenRouter API availability check failed", "error", err.Error())
		return false
	}

	return true
}

// StreamCompletion sends a streaming chat completion request to Groq
func (p *openRouterProvider) StreamCompletion(ctx context.Context, request *domain.StreamingRequest) (<-chan *domain.StreamingResponse, error) {
	// Create response channel
	responseChan := make(chan *domain.StreamingResponse, 10)

	// Create a new context that won't be canceled to avoid "context canceled" errors
	streamCtx := context.Background()

	go func() {
		defer close(responseChan)

		// Convert domain request to OpenAI request
		openaiReq := openai.ChatCompletionRequest{
			Model:       p.getModel(request),
			Temperature: p.getTemperature(request),
			MaxTokens:   p.getMaxTokens(request),
			Stream:      true,
			Messages:    p.convertMessages(request),
		}

		// Create streaming request with new context
		stream, err := p.client.CreateChatCompletionStream(streamCtx, openaiReq)
		if err != nil {
			p.logger.Warn("OpenRouter streaming failed, falling back to simulation", "error", err.Error())
			p.simulateStreaming(streamCtx, responseChan, request)
			return
		}
		defer stream.Close()

		// Process streaming response
		for {
			response, err := stream.Recv()
			if err != nil {
				if err.Error() == "EOF" {
					// End of stream
					responseChan <- &domain.StreamingResponse{
						Message:    *domain.NewMessage("", domain.MessageTypeDone),
						IsComplete: true,
					}
				} else {
					p.logger.Error("OpenRouter streaming error", err)
					responseChan <- &domain.StreamingResponse{
						Error: fmt.Sprintf("Streaming error: %v", err),
					}
				}
				return
			}

			// Extract content from response
			if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
				responseChan <- &domain.StreamingResponse{
					Message: *domain.NewMessage(response.Choices[0].Delta.Content, domain.MessageTypeData),
				}
			}
		}
	}()

	return responseChan, nil
}

// HealthCheck performs a health check on the OpenRouter provider
func (p *openRouterProvider) HealthCheck(ctx context.Context) *domain.CheckResult {
	startTime := time.Now()

	// Try to list models as a health check
	_, err := p.client.ListModels(ctx)
	duration := time.Since(startTime)

	if err != nil {
		return &domain.CheckResult{
			Status:   "fail",
			Message:  fmt.Sprintf("OpenRouter API error: %v", err),
			Duration: duration.Milliseconds(),
		}
	}

	return &domain.CheckResult{
		Status:   "pass",
		Message:  "OpenRouter API is responding normally",
		Duration: duration.Milliseconds(),
	}
}

// GetModels returns available models for OpenRouter
func (p *openRouterProvider) GetModels(ctx context.Context) ([]string, error) {
	// Create a context with timeout for the API call
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models, err := p.client.ListModels(timeoutCtx)
	if err != nil {
		p.logger.Error("Failed to fetch OpenRouter models", err)
		return []string{p.config.Model}, nil // Return default model
	}

	var modelNames []string
	for _, model := range models.Models {
		modelNames = append(modelNames, model.ID)
	}

	if len(modelNames) == 0 {
		return []string{p.config.Model}, nil
	}

	return modelNames, nil
}

// simulateStreaming provides fallback streaming simulation
func (p *openRouterProvider) simulateStreaming(ctx context.Context, responseChan chan<- *domain.StreamingResponse, request *domain.StreamingRequest) {
	p.logger.Info("Simulating OpenRouter streaming response")

	// Try non-streaming request first
	openaiReq := openai.ChatCompletionRequest{
		Model:       p.getModel(request),
		Temperature: p.getTemperature(request),
		MaxTokens:   p.getMaxTokens(request),
		Stream:      false,
		Messages:    p.convertMessages(request),
	}

	// Create a new context for non-streaming request with longer timeout for large responses
	reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	resp, err := p.client.CreateChatCompletion(reqCtx, openaiReq)
	if err != nil {
		p.logger.Error("Groq non-streaming request failed", err)
		responseChan <- &domain.StreamingResponse{
			Error: fmt.Sprintf("API request failed: %v", err),
		}
		return
	}

	if len(resp.Choices) == 0 {
		responseChan <- &domain.StreamingResponse{
			Error: "No response from Groq API",
		}
		return
	}

	// Simulate streaming by splitting response into words
	content := resp.Choices[0].Message.Content
	words := strings.Fields(content)

	for _, word := range words {
		// Check if context is cancelled or if channel is closed
		select {
		case <-ctx.Done():
			p.logger.Info("Streaming cancelled due to context", "provider", "openrouter")
			return
		default:
			// Try to send, but don't block if channel is closed
			select {
			case responseChan <- &domain.StreamingResponse{
				Message: *domain.NewMessage(word+" ", domain.MessageTypeData),
			}:
				// Successfully sent
			case <-ctx.Done():
				p.logger.Info("Streaming cancelled during send", "provider", "openrouter")
				return
			}

			// Add delay for streaming effect
			time.Sleep(time.Duration(50) * time.Millisecond)
		}
	}

	// Send completion signal
	responseChan <- &domain.StreamingResponse{
		Message:    *domain.NewMessage("", domain.MessageTypeDone),
		IsComplete: true,
	}
}

// Helper methods

func (p *openRouterProvider) getModel(request *domain.StreamingRequest) string {
	if request.Model != "" {
		return request.Model
	}
	return p.config.Model
}

func (p *openRouterProvider) getTemperature(request *domain.StreamingRequest) float32 {
	if request.Temperature != nil {
		return *request.Temperature
	}
	return p.config.Temperature
}

func (p *openRouterProvider) getMaxTokens(request *domain.StreamingRequest) int {
	if request.MaxTokens != nil {
		return *request.MaxTokens
	}
	return p.config.MaxTokens
}

func (p *openRouterProvider) convertMessages(request *domain.StreamingRequest) []openai.ChatCompletionMessage {
	var messages []openai.ChatCompletionMessage

	// Add conversation history
	for _, msg := range request.ConversationHistory {
		role := ""
		switch msg.Type {
		case domain.MessageTypeAssistant:
			role = "assistant"
		case domain.MessageTypeSystem:
			role = "system"
		default:
			role = "user"
		}

		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	// Add current prompt
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "user",
		Content: request.Prompt,
	})

	return messages
}
