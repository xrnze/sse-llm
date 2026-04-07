package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config represents application configuration
// This struct holds all configuration values needed by the application
// It follows clean architecture principles by separating configuration concerns
type Config struct {
	// Server configuration
	Server ServerConfig `mapstructure:"server"`

	// OpenAI API configuration
	OpenAI OpenAIConfig `mapstructure:"openai"`

	// Groq API configuration
	Groq LLMConfig `mapstructure:"groq"`

	// OpenRouter API configuration
	OpenRouter LLMConfig `mapstructure:"openrouter"`

	// Application behavior configuration
	App AppConfig `mapstructure:"app"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	// Port to run the server on
	Port string `mapstructure:"port"`

	// Host to bind the server to
	Host string `mapstructure:"host"`

	// ReadTimeout for HTTP requests in seconds
	ReadTimeout int `mapstructure:"read_timeout"`

	// WriteTimeout for HTTP responses in seconds
	WriteTimeout int `mapstructure:"write_timeout"`

	// IdleTimeout for HTTP connections in seconds
	IdleTimeout int `mapstructure:"idle_timeout"`
}

// OpenAIConfig holds OpenAI API configuration
type OpenAIConfig struct {
	// APIKey for authentication with OpenAI
	APIKey string `mapstructure:"api_key"`

	// Model to use for chat completions (e.g., "gpt-4", "gpt-3.5-turbo")
	Model string `mapstructure:"model"`

	// BaseURL for OpenAI API (useful for proxies or compatible APIs)
	BaseURL string `mapstructure:"base_url"`

	// MaxTokens limit for responses
	MaxTokens int `mapstructure:"max_tokens"`

	// Temperature for response randomness (0.0 to 2.0)
	Temperature float32 `mapstructure:"temperature"`
}

// LLM provider config holds Groq/X.AI API configuration
type LLMConfig struct {
	// APIKey for authentication with X.AI (Groq)
	APIKey string `mapstructure:"api_key"`

	// Model to use for Groq (e.g., "grok-beta")
	Model string `mapstructure:"model"`

	// BaseURL for Groq API
	BaseURL string `mapstructure:"base_url"`

	// MaxTokens limit for responses
	MaxTokens int `mapstructure:"max_tokens"`

	// Temperature for response randomness
	Temperature float32 `mapstructure:"temperature"`
}

// AppConfig holds application-specific configuration
type AppConfig struct {
	// DefaultProvider specifies which LLM to use by default ("openai" or "grok")
	DefaultProvider string `mapstructure:"default_provider"`

	// StreamingDelay adds artificial delay between tokens in milliseconds (for demo purposes)
	StreamingDelay int `mapstructure:"streaming_delay"`

	// MaxConnections limits concurrent SSE connections
	MaxConnections int `mapstructure:"max_connections"`

	// LogLevel for application logging ("debug", "info", "warn", "error")
	LogLevel string `mapstructure:"log_level"`
}

// Load reads configuration from files and environment variables
// It follows the 12-factor app methodology for configuration management
func Load() (*Config, error) {
	// Load .env file if it exists (useful for local development)
	_ = godotenv.Load()

	// Set default configuration values
	setDefaults()

	// Configure viper to read from environment variables
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SSE") // Environment variables will be prefixed with SSE_

	// Try to read from config file
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/sse-chat")

	// Read config file (it's optional)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		log.Println("No config file found, using defaults and environment variables")
	}

	// Unmarshal configuration into struct
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
// These can be overridden by config files or environment variables
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.host", "localhost")
	viper.SetDefault("server.read_timeout", 15)
	viper.SetDefault("server.write_timeout", 15)
	viper.SetDefault("server.idle_timeout", 60)

	// OpenAI defaults
	viper.SetDefault("openai.model", "gpt-3.5-turbo")
	viper.SetDefault("openai.base_url", "")
	viper.SetDefault("openai.max_tokens", 1000)
	viper.SetDefault("openai.temperature", 0.7)

	// Groq defaults
	viper.SetDefault("groq.model", "grok-beta")
	viper.SetDefault("groq.base_url", "https://api.x.ai/v1")
	viper.SetDefault("groq.max_tokens", 1000)
	viper.SetDefault("groq.temperature", 0.7)

	// App defaults
	viper.SetDefault("app.default_provider", "openai")
	viper.SetDefault("app.streaming_delay", 50)
	viper.SetDefault("app.max_connections", 100)
	viper.SetDefault("app.log_level", "info")
}

// validate checks if the configuration is valid
func validate(cfg *Config) error {
	// Validate server configuration
	if cfg.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	// Validate that at least one LLM provider is configured
	hasProvider := false

	if cfg.OpenAI.APIKey != "" {
		hasProvider = true
		if cfg.OpenAI.Model == "" {
			return fmt.Errorf("openai model cannot be empty when api_key is provided")
		}
	}

	if cfg.Groq.APIKey != "" {
		hasProvider = true
		if cfg.Groq.Model == "" {
			return fmt.Errorf("grok model cannot be empty when api_key is provided")
		}
	}

	if !hasProvider {
		return fmt.Errorf("at least one LLM provider (openai or grok) must be configured")
	}

	// Validate default provider
	if cfg.App.DefaultProvider != "openai" && cfg.App.DefaultProvider != "groq" {
		return fmt.Errorf("default_provider must be either 'openai' or 'groq'")
	}

	// Ensure the default provider is actually configured
	if cfg.App.DefaultProvider == "openai" && cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("default_provider is set to 'openai' but no OpenAI API key is configured")
	}

	if cfg.App.DefaultProvider == "groq" && cfg.Groq.APIKey == "" {
		return fmt.Errorf("default_provider is set to 'groq' but no Groq API key is configured")
	}

	return nil
}

// GetServerAddress returns the full server address
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// IsOpenAIEnabled returns true if OpenAI is configured
func (c *Config) IsOpenAIEnabled() bool {
	return c.OpenAI.APIKey != ""
}

// IsGroqEnabled returns true if Groq is configured
func (c *Config) IsGroqEnabled() bool {
	return c.Groq.APIKey != ""
}

// IsOpenRouterEnabled returns true if OpenRouter is configured
func (c *Config) IsOpenRouterEnabled() bool {
	return c.OpenRouter.APIKey != ""
}

// GetAPIKeyFromEnv is a helper function to safely get API keys from environment
func GetAPIKeyFromEnv(envName string) string {
	key := os.Getenv(envName)
	if key == "" {
		log.Printf("Warning: %s environment variable not set", envName)
	}
	return key
}
