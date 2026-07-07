// Package config loads environment-backed settings, mirroring Python's
// simon/config/settings.py (pydantic-settings, .env-backed, ~20 typed
// fields with defaults).
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Settings holds environment-backed configuration with sensible defaults,
// matching the field set and defaults of Python's Settings(BaseSettings).
type Settings struct {
	OpenAIAPIKey    string
	OpenAIModel     string
	AnthropicAPIKey string
	AnthropicModel  string
	OllamaHost      string
	OllamaModel     string
	DefaultModel    string

	KnowledgeStorePath string
	EmbeddingProvider  string
	EmbeddingModel     string

	EnableDirDocuments bool
	EnableDirDownloads bool
	EnableDirPictures  bool
	EnableDirDesktop   bool

	SimonLoggingEnabled    bool
	SimonLogLevel          string
	SimonLogDir            string
	SimonMaxRetries        int
	SimonRequestTimeout    float64
	SimonRetryBaseDelay    float64
	SimonStructuredRetries int

	SimonActivityStorePath  string
	SimonSensorPollInterval float64
}

// Load reads settings from the process environment, after first loading any
// key=value pairs from a ".env" file in the working directory (mirroring
// pydantic-settings' env_file=".env" behavior: real environment variables
// always take precedence over the file).
func Load() Settings {
	loadDotEnv(".env")

	s := Settings{
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:     os.Getenv("OPENAI_MODEL"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicModel:  os.Getenv("ANTHROPIC_MODEL"),
		OllamaHost:      envOrDefault("OLLAMA_HOST", "http://localhost:11434"),
		OllamaModel:     os.Getenv("OLLAMA_MODEL"),
		DefaultModel:    envOrDefault("DEFAULT_MODEL", "auto"),

		KnowledgeStorePath: os.Getenv("KNOWLEDGE_STORE_PATH"),
		EmbeddingProvider:  envOrDefault("EMBEDDING_PROVIDER", "OPENAI"),
		EmbeddingModel:     envOrDefault("EMBEDDING_MODEL", "text-embedding-3-small"),

		EnableDirDocuments: envBoolOrDefault("ENABLE_DIR_DOCUMENTS", true),
		EnableDirDownloads: envBoolOrDefault("ENABLE_DIR_DOWNLOADS", true),
		EnableDirPictures:  envBoolOrDefault("ENABLE_DIR_PICTURES", false),
		EnableDirDesktop:   envBoolOrDefault("ENABLE_DIR_DESKTOP", false),

		SimonLoggingEnabled:    envBoolOrDefault("SIMON_LOGGING_ENABLED", false),
		SimonLogLevel:          envOrDefault("SIMON_LOG_LEVEL", "INFO"),
		SimonLogDir:            envOrDefault("SIMON_LOG_DIR", "logs"),
		SimonMaxRetries:        envIntOrDefault("SIMON_MAX_RETRIES", 2),
		SimonRequestTimeout:    envFloatOrDefault("SIMON_REQUEST_TIMEOUT", 60.0),
		SimonRetryBaseDelay:    envFloatOrDefault("SIMON_RETRY_BASE_DELAY", 0.5),
		SimonStructuredRetries: envIntOrDefault("SIMON_STRUCTURED_RETRIES", 2),

		SimonActivityStorePath:  envOrDefault("SIMON_ACTIVITY_STORE_PATH", ".simon_activity/activity.db"),
		SimonSensorPollInterval: envFloatOrDefault("SIMON_SENSOR_POLL_INTERVAL", 1.0),
	}
	return s
}

// loadDotEnv sets process environment variables from a simple KEY=VALUE
// file, skipping blank lines and lines starting with '#', and never
// overwriting a variable already present in the environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBoolOrDefault(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envIntOrDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloatOrDefault(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}
