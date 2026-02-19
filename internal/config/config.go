package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL      string
	NewRelicAccountID string
	NewRelicAPIKey   string
	NewRelicEntityGUID string
	AnthropicAPIKey  string
	GithubToken      string
	DuunitoriRepo    string
	DuunitoriBaseBranch string
}

func Load() *Config {
	return &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		NewRelicAccountID:  os.Getenv("NEW_RELIC_ACCOUNT_ID"),
		NewRelicAPIKey:     os.Getenv("NEW_RELIC_API_KEY"),
		NewRelicEntityGUID: getEnvDefault("NEW_RELIC_ENTITY_GUID", "NTIyMTg2fEFQTXxBUFBMSUNBVElPTnw1MjQyNTgxMjE"),
		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		GithubToken:        os.Getenv("GITHUB_TOKEN"),
		DuunitoriRepo:      os.Getenv("DUUNITORI_REPO"),
		DuunitoriBaseBranch: getEnvDefault("DUUNITORI_BASE_BRANCH", "testing"),
	}
}

func (c *Config) ValidateFetch() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.NewRelicAccountID == "" {
		return fmt.Errorf("NEW_RELIC_ACCOUNT_ID is required")
	}
	if c.NewRelicAPIKey == "" {
		return fmt.Errorf("NEW_RELIC_API_KEY is required")
	}
	return nil
}

func (c *Config) ValidateFix() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.AnthropicAPIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is required")
	}
	if c.GithubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}
	if c.DuunitoriRepo == "" {
		return fmt.Errorf("DUUNITORI_REPO is required")
	}
	return nil
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
