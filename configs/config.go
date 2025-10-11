package configs

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the application
type Config struct {
	Server struct {
		Port string `yaml:"port"`
	} `yaml:"server"`

	Storage struct {
		DataDir string `yaml:"data_dir"`
		Type    string `yaml:"type"` // "badger" or "memory"
	} `yaml:"storage"`

	Raft struct {
		Enabled   bool   `yaml:"enabled"`
		BindAddr  string `yaml:"bind_addr"`
		Bootstrap bool   `yaml:"bootstrap"`
	} `yaml:"raft"`

	JobQueue struct {
		WorkerCount  int           `yaml:"worker_count"`
		QueueSize    int           `yaml:"queue_size"`
		MaxRetries   int           `yaml:"max_retries"`
		RetryBackoff time.Duration `yaml:"retry_backoff"`
	} `yaml:"job_queue"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	config := &Config{}

	// Server defaults
	config.Server.Port = ":8080"

	// Storage defaults
	config.Storage.DataDir = "data"
	config.Storage.Type = "badger"

	// Raft defaults
	config.Raft.Enabled = false
	config.Raft.BindAddr = ":8081"
	config.Raft.Bootstrap = true

	// Job queue defaults
	config.JobQueue.WorkerCount = 5
	config.JobQueue.QueueSize = 100
	config.JobQueue.MaxRetries = 3
	config.JobQueue.RetryBackoff = 5 * time.Second

	return config
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filename string) (*Config, error) {
	config := DefaultConfig()

	if filename == "" {
		return config, nil
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %s: %w", filename, err)
	}
	defer func() {
		_ = file.Close()
	}()

	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filename, err)
	}

	return config, nil
}
