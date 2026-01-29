package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds application configuration values.
type Config struct {
	DatabaseURL string
	APIPort     string
}

// LoadConfig reads configuration variables or returns default values.
func LoadConfig() (*Config, error) {
	// Load .env file if available (optional - won't fail if missing)
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIPort:     os.Getenv("API_PORT"),
	}

	if cfg.APIPort == "" {
		cfg.APIPort = ":8082"
	}

	log.Printf("Config loaded successfully ...")

	return cfg, nil
}
