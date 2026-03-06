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

type groqProvider struct {
	// client is the OpenAI-compatible client configured for Groq
	client *openai.Client
	// config holds Groq-specific configuration
	config config.GroqConfig
	// logger for structured logging
	logger domain.Logger
}

func NewGroqProvider(cfg config.GroqConfig, logger domain.Logger) domain.LLMProvider {
	// Create OpenAI client configuration for Groq
	clientConfig := openai.DefaultConfig(cfg.APIKey)

	// Set Groq base URL
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	} else {
		clientConfig.BaseURL = "https://api.groq.com/openai/v1"
	}

	// Create the client
	client := openai.NewClientWithConfig(clientConfig)

	return &groqProvider{
		client: client,
		config: cfg,
		logger: logger.With("provider", "groq"),
	}
}

// Name returns the provider name
func (p *groqProvider) Name() string {
	return "groq"
}

// IsAvailable checks if the Groq provider is properly configured and reachable
func (p *groqProvider) IsAvailable(ctx context.Context) bool {
	// Quick validation - check if API key is set
	if p.config.APIKey == "" {
		p.logger.Warn("Groq API key not configured")
		return false
	}

	// Create a context with timeout for the API call
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to make a simple API call to test connectivity
	// We'll use the models endpoint as it's lightweight
	_, err := p.client.ListModels(timeoutCtx)
	if err != nil {
		p.logger.Warn("Groq API availability check failed", "error", err.Error())
		return false
	}

	return true
}

// StreamCompletion sends a streaming chat completion request to Groq
func (p *groqProvider) StreamCompletion(ctx context.Context, request *domain.StreamingRequest) (<-chan *domain.StreamingResponse, error) {
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
			p.logger.Warn("Groq streaming failed, falling back to simulation", "error", err.Error())
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
					p.logger.Error("Groq streaming error", err)
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

// HealthCheck performs a health check on the Groq provider
func (p *groqProvider) HealthCheck(ctx context.Context) *domain.CheckResult {
	startTime := time.Now()

	// Try to list models as a health check
	_, err := p.client.ListModels(ctx)
	duration := time.Since(startTime)

	if err != nil {
		return &domain.CheckResult{
			Status:   "fail",
			Message:  fmt.Sprintf("Groq API error: %v", err),
			Duration: duration.Milliseconds(),
		}
	}

	return &domain.CheckResult{
		Status:   "pass",
		Message:  "Groq API is responding normally",
		Duration: duration.Milliseconds(),
	}
}

// GetModels returns available models for Groq
func (p *groqProvider) GetModels(ctx context.Context) ([]string, error) {
	// Create a context with timeout for the API call
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models, err := p.client.ListModels(timeoutCtx)
	if err != nil {
		p.logger.Error("Failed to fetch Groq models", err)
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
func (p *groqProvider) simulateStreaming(ctx context.Context, responseChan chan<- *domain.StreamingResponse, request *domain.StreamingRequest) {
	p.logger.Info("Simulating Groq streaming response")

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
			p.logger.Info("Streaming cancelled due to context", "provider", "groq")
			return
		default:
			// Try to send, but don't block if channel is closed
			select {
			case responseChan <- &domain.StreamingResponse{
				Message: *domain.NewMessage(word+" ", domain.MessageTypeData),
			}:
				// Successfully sent
			case <-ctx.Done():
				p.logger.Info("Streaming cancelled during send", "provider", "groq")
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

func (p *groqProvider) getModel(request *domain.StreamingRequest) string {
	if request.Model != "" {
		return request.Model
	}
	return p.config.Model
}

func (p *groqProvider) getTemperature(request *domain.StreamingRequest) float32 {
	if request.Temperature != nil {
		return *request.Temperature
	}
	return p.config.Temperature
}

func (p *groqProvider) getMaxTokens(request *domain.StreamingRequest) int {
	if request.MaxTokens != nil {
		return *request.MaxTokens
	}
	return p.config.MaxTokens
}

func (p *groqProvider) convertMessages(request *domain.StreamingRequest) []openai.ChatCompletionMessage {
	var messages []openai.ChatCompletionMessage

	// Add conversation history
	for _, msg := range request.ConversationHistory {
		role := "user"
		if msg.Type == domain.MessageTypeAssistant {
			role = "assistant"
		} else if msg.Type == domain.MessageTypeSystem {
			role = "system"
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
