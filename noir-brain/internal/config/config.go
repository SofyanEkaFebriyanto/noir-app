// Package config memuat konfigurasi noir-brain dari environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config adalah seluruh konfigurasi server.
type Config struct {
	Port         string // port HTTP/WebSocket, default 8080
	LLMBaseURL   string // base URL API LLM (OpenAI-compatible), default https://api.openai.com/v1
	LLMAPIKey    string // kunci API LLM — JANGAN di-commit
	LLMModel     string // nama model, default gpt-4o-mini
	DataDir      string // direktori SQLite, default ./data
	SystemPrompt string // override system prompt (opsional)

	AgentEnabled  bool     // AGENT_ENABLED, default true — agent loop + tools
	AgentMaxSteps int      // AGENT_MAX_STEPS, default 12
	AgentServices []string // AGENT_SERVICES, koma-dipisah, default ["noir-brain"]

	ProactiveEnabled  bool   // PROACTIVE_ENABLED, default true — sapaan proaktif (F-14, in-app only)
	ProactiveInterval int    // PROACTIVE_INTERVAL_MIN, default 45 — jeda antar cek (menit)
	ProactiveQuiet    string // PROACTIVE_QUIET_HOURS, default "23-6" — jam sepi WIB (format "23-6")

	MemoryJobsEnabled bool // MEMORY_JOBS_ENABLED, default true — konsolidasi + daily note (F-16)
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

		AgentEnabled:  envBool("AGENT_ENABLED", true),
		AgentMaxSteps: envInt("AGENT_MAX_STEPS", 12),
		AgentServices: envList("AGENT_SERVICES", []string{"noir-brain"}),

		ProactiveEnabled:  envBool("PROACTIVE_ENABLED", true),
		ProactiveInterval: envInt("PROACTIVE_INTERVAL_MIN", 45),
		ProactiveQuiet:    env("PROACTIVE_QUIET_HOURS", "23-6"),

		MemoryJobsEnabled: envBool("MEMORY_JOBS_ENABLED", true),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envList(key string, def []string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
