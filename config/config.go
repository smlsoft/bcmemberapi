package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	Port              string
	MongoURI          string
	LineChannelSecret string
	LineChannelToken  string
	GeminiAPIKey      string
	LiffID            string
	LiffURL           string
	TableLiffID       string
	TableLiffURL      string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Try to load .env file (ignore error if not found)
	_ = godotenv.Load()

	liffID := getEnvOrDefault("LIFF_ID", "2008745223-8Ol0oVZk")
	tableLiffID := os.Getenv("TABLE_LIFF_ID")
	cfg := &Config{
		Port:              getEnvOrDefault("PORT", "8080"),
		MongoURI:          os.Getenv("MONGODB_URI"),
		LineChannelSecret: os.Getenv("LINE_CHANNEL_SECRET"),
		LineChannelToken:  os.Getenv("LINE_CHANNEL_TOKEN"),
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		LiffID:            liffID,
		LiffURL:           "https://liff.line.me/" + liffID,
		TableLiffID:       tableLiffID,
		TableLiffURL:      "https://liff.line.me/" + tableLiffID,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if all required configuration is present
func (c *Config) Validate() error {
	if c.MongoURI == "" {
		return errors.New("MONGODB_URI is required")
	}
	if c.LineChannelSecret == "" {
		return errors.New("LINE_CHANNEL_SECRET is required")
	}
	if c.LineChannelToken == "" {
		return errors.New("LINE_CHANNEL_TOKEN is required")
	}
	if c.GeminiAPIKey == "" {
		return errors.New("GEMINI_API_KEY is required")
	}
	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
