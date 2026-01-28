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
	// Load .env file if available
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Warning: .env file not found, using system environment variables")
	}

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIPort:     os.Getenv("API_PORT"),
	}

	// if cfg.DatabaseURL == "" {
	// 	cfg.DatabaseURL = "postgres://postgres:root@localhost:5432/jobsdb?sslmode=disable"
	// }

	// if cfg.APIPort == "" {
	// 	cfg.APIPort = ":8082"
	// }

	log.Printf("Config file connected successfully ...")

	return cfg, nil
}
