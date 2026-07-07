package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	restore := chdir(t, dir)
	defer restore()

	clearAllSimonEnv(t)

	s := Load()

	if s.OllamaHost != "http://localhost:11434" {
		t.Errorf("OllamaHost = %q, want default", s.OllamaHost)
	}
	if s.DefaultModel != "auto" {
		t.Errorf("DefaultModel = %q, want %q", s.DefaultModel, "auto")
	}
	if s.SimonMaxRetries != 2 {
		t.Errorf("SimonMaxRetries = %d, want 2", s.SimonMaxRetries)
	}
	if !s.EnableDirDocuments || !s.EnableDirDownloads {
		t.Error("expected documents/downloads dirs enabled by default")
	}
	if s.EnableDirPictures || s.EnableDirDesktop {
		t.Error("expected pictures/desktop dirs disabled by default")
	}
}

func TestLoadReadsDotEnvWithoutOverridingRealEnv(t *testing.T) {
	dir := t.TempDir()
	restore := chdir(t, dir)
	defer restore()

	clearAllSimonEnv(t)

	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("OPENAI_MODEL=from-dotenv\nANTHROPIC_MODEL=also-dotenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Setenv("ANTHROPIC_MODEL", "from-real-env")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_MODEL") })

	s := Load()

	if s.OpenAIModel != "from-dotenv" {
		t.Errorf("OpenAIModel = %q, want value from .env", s.OpenAIModel)
	}
	if s.AnthropicModel != "from-real-env" {
		t.Errorf("AnthropicModel = %q, want real env to win over .env", s.AnthropicModel)
	}
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { os.Chdir(old) }
}

func clearAllSimonEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"OPENAI_API_KEY", "OPENAI_MODEL", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL",
		"OLLAMA_HOST", "OLLAMA_MODEL", "DEFAULT_MODEL", "KNOWLEDGE_STORE_PATH",
		"EMBEDDING_PROVIDER", "EMBEDDING_MODEL", "ENABLE_DIR_DOCUMENTS",
		"ENABLE_DIR_DOWNLOADS", "ENABLE_DIR_PICTURES", "ENABLE_DIR_DESKTOP",
		"SIMON_LOGGING_ENABLED", "SIMON_LOG_LEVEL", "SIMON_LOG_DIR",
		"SIMON_MAX_RETRIES", "SIMON_REQUEST_TIMEOUT", "SIMON_RETRY_BASE_DELAY",
		"SIMON_STRUCTURED_RETRIES", "SIMON_ACTIVITY_STORE_PATH", "SIMON_SENSOR_POLL_INTERVAL",
	}
	for _, key := range keys {
		old, existed := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if existed {
				os.Setenv(key, old)
			}
		})
	}
}
