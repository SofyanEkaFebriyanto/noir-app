// Package brain adalah interface "otak" Noir: provider LLM yang bisa diganti-ganti
// tanpa mengubah kode server. Default: API yang OpenAI-compatible (streaming).
package brain

import "context"

// Message adalah satu pesan dalam percakapan.
type Message struct {
	Role       string     `json:"role"`                   // "system" | "user" | "assistant" | "tool"
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // hanya untuk pesan assistant
	ToolCallID string     `json:"tool_call_id,omitempty"` // hanya untuk pesan tool
}

// FunctionCall adalah detail pemanggilan satu fungsi oleh LLM.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolCall adalah permintaan pemanggilan tool dari LLM (format OpenAI).
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// Tool adalah definisi tool bergaya OpenAI function calling.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON schema
}

// Provider mengalirkan token completion untuk sebuah percakapan.
// Channel tokens ditutup saat stream selesai atau ctx dibatalkan.
type Provider interface {
	StreamChat(ctx context.Context, messages []Message) (<-chan string, <-chan error)
}

// AgentProvider menambah chat non-streaming dengan tool calling (untuk agent loop).
type AgentProvider interface {
	Provider
	// Chat mengirim messages + definisi tools, mengembalikan pesan assistant
	// (Content dan/atau ToolCalls).
	Chat(ctx context.Context, messages []Message, tools []Tool) (Message, error)
}
