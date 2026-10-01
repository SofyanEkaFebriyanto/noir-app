// Package config memuat konfigurasi noir-brain dari environment variables.
package config

import "os"

// Config adalah seluruh konfigurasi server.
type Config struct {
	Port         string // port HTTP/WebSocket, default 8080
	LLMBaseURL   string // base URL API LLM (OpenAI-compatible), default https://api.openai.com/v1
	LLMAPIKey    string // kunci API LLM — JANGAN di-commit
	LLMModel     string // nama model, default gpt-4o-mini
	DataDir      string // direktori SQLite, default ./data
	SystemPrompt string // override system prompt (opsional)
}

// Load membaca konfigurasi dari environment.
func Load() Config {
	return Config{
		Port:         env("PORT", "8080"),
		LLMBaseURL:   env("LLM_BASE_URL", "https://api.openai.com/v1"),
		LLMAPIKey:    os.Getenv("LLM_API_KEY"),
		LLMModel:     env("LLM_MODEL", "gpt-4o-mini"),
		DataDir:      env("DATA_DIR", "./data"),
		SystemPrompt: os.Getenv("SYSTEM_PROMPT"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
