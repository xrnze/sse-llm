package infrastructure

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"sse-streaming-chat/internal/domain"
)

// messageValidator implements domain.MessageValidator interface
type messageValidator struct {
	maxPromptLength  int
	maxMessageLength int
}

// NewMessageValidator creates a new message validator
func NewMessageValidator() domain.MessageValidator {
	return &messageValidator{
		maxPromptLength:  10000, // 10KB max prompt
		maxMessageLength: 50000, // 50KB max message
	}
}

// ValidateRequest checks if a streaming request is valid
func (v *messageValidator) ValidateRequest(request *domain.StreamingRequest) error {
	if request == nil {
		return fmt.Errorf("request cannot be nil")
	}

	// Validate prompt
	if strings.TrimSpace(request.Prompt) == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	// Check prompt length
	if len(request.Prompt) > v.maxPromptLength {
		return fmt.Errorf("prompt too long: %d characters (max %d)", len(request.Prompt), v.maxPromptLength)
	}

	// Validate UTF-8 encoding
	if !utf8.ValidString(request.Prompt) {
		return fmt.Errorf("prompt contains invalid UTF-8 characters")
	}

	// Validate provider if specified
	if request.Provider != "" {
		if request.Provider != "openai" && request.Provider != "groq" && request.Provider != "openrouter" {
			return fmt.Errorf("invalid provider: %s (must be 'openai' or 'groq' or 'openrouter')", request.Provider)
		}
	}

	// Validate temperature if specified
	if request.Temperature != nil {
		if *request.Temperature < 0 || *request.Temperature > 2 {
			return fmt.Errorf("temperature must be between 0 and 2, got %f", *request.Temperature)
		}
	}

	// Validate max tokens if specified
	if request.MaxTokens != nil {
		if *request.MaxTokens <= 0 || *request.MaxTokens > 100000 {
			return fmt.Errorf("max_tokens must be between 1 and 100000, got %d", *request.MaxTokens)
		}
	}

	// Validate conversation history
	if request.ConversationHistory != nil {
		for i, msg := range request.ConversationHistory {
			if err := v.ValidateMessage(&msg); err != nil {
				return fmt.Errorf("invalid message at history index %d: %w", i, err)
			}
		}
	}

	return nil
}

// ValidateMessage checks if a message is valid
func (v *messageValidator) ValidateMessage(message *domain.Message) error {
	if message == nil {
		return fmt.Errorf("message cannot be nil")
	}

	// Validate message ID
	if strings.TrimSpace(message.ID) == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	// Check content length
	if len(message.Content) > v.maxMessageLength {
		return fmt.Errorf("message content too long: %d characters (max %d)",
			len(message.Content), v.maxMessageLength)
	}

	// Validate UTF-8 encoding
	if !utf8.ValidString(message.Content) {
		return fmt.Errorf("message content contains invalid UTF-8 characters")
	}

	// Validate message type
	validTypes := map[domain.MessageType]bool{
		domain.MessageTypeUser:       true,
		domain.MessageTypeAssistant:  true,
		domain.MessageTypeSystem:     true,
		domain.MessageTypeError:      true,
		domain.MessageTypeConnection: true,
		domain.MessageTypeData:       true,
		domain.MessageTypeDone:       true,
	}

	if !validTypes[message.Type] {
		return fmt.Errorf("invalid message type: %s", message.Type)
	}

	// Validate timestamp
	if message.Timestamp.IsZero() {
		return fmt.Errorf("message timestamp cannot be zero")
	}

	return nil
}

// SanitizeContent cleans and sanitizes message content
func (v *messageValidator) SanitizeContent(content string) string {
	// Trim whitespace
	content = strings.TrimSpace(content)

	// Remove null bytes
	content = strings.ReplaceAll(content, "\x00", "")

	// Normalize line breaks
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// Remove excessive whitespace
	lines := strings.Split(content, "\n")
	var cleanLines []string

	for _, line := range lines {
		cleanLine := strings.TrimSpace(line)
		if cleanLine != "" {
			cleanLines = append(cleanLines, cleanLine)
		}
	}

	// Rejoin with single newlines
	content = strings.Join(cleanLines, "\n")

	// Truncate if too long
	if len(content) > v.maxMessageLength {
		content = content[:v.maxMessageLength]
		// Try to break at word boundary
		if lastSpace := strings.LastIndex(content, " "); lastSpace > v.maxMessageLength-100 {
			content = content[:lastSpace]
		}
		content += "..."
	}

	return content
}

// CheckContentPolicy verifies content against policies
func (v *messageValidator) CheckContentPolicy(content string) error {
	// Basic content policy checks

	// Check for suspicious patterns (basic implementation)
	lowerContent := strings.ToLower(content)

	// Check for potential prompt injection patterns
	suspiciousPatterns := []string{
		"ignore previous instructions",
		"disregard the above",
		"forget everything",
		"system:",
		"assistant:",
		"<script>",
		"javascript:",
		"data:",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerContent, pattern) {
			return fmt.Errorf("content contains potentially harmful pattern: %s", pattern)
		}
	}

	// Check for excessive repetition (potential spam)
	if v.hasExcessiveRepetition(content) {
		return fmt.Errorf("content contains excessive repetition")
	}

	// Check for extremely long lines (potential binary data)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if len(line) > 1000 {
			return fmt.Errorf("content contains excessively long line")
		}
	}

	return nil
}

// hasExcessiveRepetition checks if content has too much repetition
func (v *messageValidator) hasExcessiveRepetition(content string) bool {
	if len(content) < 50 {
		return false
	}

	// Check for repeated substrings
	words := strings.Fields(content)
	if len(words) < 10 {
		return false
	}

	// Count word frequency
	wordCount := make(map[string]int)
	for _, word := range words {
		wordCount[strings.ToLower(word)]++
	}

	// Check if any word appears more than 30% of the time
	totalWords := len(words)
	for _, count := range wordCount {
		if float64(count)/float64(totalWords) > 0.3 {
			return true
		}
	}

	return false
}
