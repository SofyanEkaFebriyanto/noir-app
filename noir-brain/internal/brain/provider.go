// Package brain adalah interface "otak" Noir: provider LLM yang bisa diganti-ganti
// tanpa mengubah kode server. Default: API yang OpenAI-compatible (streaming).
package brain

import "context"

// Message adalah satu pesan dalam percakapan.
type Message struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// Provider mengalirkan token completion untuk sebuah percakapan.
// Channel tokens ditutup saat stream selesai atau ctx dibatalkan.
type Provider interface {
	StreamChat(ctx context.Context, messages []Message) (<-chan string, <-chan error)
}
