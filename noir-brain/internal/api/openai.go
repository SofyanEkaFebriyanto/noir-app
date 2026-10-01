package api

// Endpoint kompatibel OpenAI Chat Completions: POST /v1/chat/completions
//
// Tujuannya: client apa pun yang bisa bicara format OpenAI (termasuk Muse/Noir
// sendiri lewat HTTP) bisa ngobrol langsung dengan persona Noir tanpa WebSocket.
//
// Request:
//
//	{"model": "noir", "messages": [{"role": "user", "content": "halo"}],
//	 "stream": false}
//
// Response non-stream: format chat.completion standar OpenAI.
// Response stream=true: SSE text/event-stream dengan chunk
// chat.completion.chunk + "data: [DONE]" di akhir.
//
// Catatan: endpoint ini STATELESS terhadap memory store — tidak membaca atau
// menulis riwayat persisten, supaya sesi API tidak mencemari memori voice chat.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

// chatMessage adalah satu pesan format OpenAI.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionRequest adalah body POST /v1/chat/completions.
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// registerOpenAI mendaftarkan route OpenAI-compatible.
func (s *Server) registerOpenAI(r *gin.Engine) {
	r.POST("/v1/chat/completions", s.handleChatCompletions)
	r.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data": []gin.H{
				{"id": "noir", "object": "model", "created": time.Now().Unix(), "owned_by": "noir-brain"},
			},
		})
	})
}

// systemPrompt mengembalikan prompt persona (override config didahulukan).
func (s *Server) systemPrompt() string {
	if s.deps.Config.SystemPrompt != "" {
		return s.deps.Config.SystemPrompt
	}
	return s.deps.SystemPrompt
}

// toBrainMessages mengubah pesan OpenAI jadi pesan internal,
// dengan system prompt persona di depan.
func (s *Server) toBrainMessages(in []chatMessage) []brain.Message {
	messages := make([]brain.Message, 0, len(in)+1)
	messages = append(messages, brain.Message{Role: "system", Content: s.systemPrompt()})
	for _, m := range in {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "system" && role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, brain.Message{Role: role, Content: m.Content})
	}
	return messages
}

// streamAnswer menjalankan LLM dan mengembalikan channel token + error,
// dengan konteks yang bisa dibatalkan client.
func (s *Server) streamAnswer(c *gin.Context, messages []brain.Message) (<-chan string, <-chan error, context.CancelFunc) {
	ctx, cancel := context.WithCancel(c.Request.Context())
	tokens, errc := s.deps.Provider.StreamChat(ctx, messages)
	return tokens, errc, cancel
}

func openAIError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": gin.H{"message": msg, "type": "server_error"}})
}

// handleChatCompletions adalah POST /v1/chat/completions.
func (s *Server) handleChatCompletions(c *gin.Context) {
	var req chatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		openAIError(c, http.StatusBadRequest, "body JSON tidak valid: "+err.Error())
		return
	}
	if len(req.Messages) == 0 {
		openAIError(c, http.StatusBadRequest, "messages kosong")
		return
	}
	model := req.Model
	if model == "" {
		model = "noir"
	}

	messages := s.toBrainMessages(req.Messages)
	tokens, errc, cancel := s.streamAnswer(c, messages)
	defer cancel()

	id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	if req.Stream {
		s.handleChatCompletionsStream(c, id, created, model, tokens, errc)
		return
	}

	var full strings.Builder
	for {
		select {
		case <-c.Request.Context().Done():
			return // client disconnect
		case tok, ok := <-tokens:
			if !ok {
				goto collected
			}
			full.WriteString(tok)
		case err := <-errc:
			if err != nil {
				openAIError(c, http.StatusBadGateway, "LLM error: "+err.Error())
				return
			}
		}
	}
collected:
	answer := strings.TrimSpace(full.String())
	if answer == "" {
		answer = "Hmm, kosong. Coba ngomong lagi?"
	}
	c.JSON(http.StatusOK, gin.H{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []gin.H{
			{
				"index": 0,
				"message": gin.H{
					"role":    "assistant",
					"content": answer,
				},
				"finish_reason": "stop",
			},
		},
		"usage": gin.H{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	})
}

// handleChatCompletionsStream melayani stream=true via SSE.
func (s *Server) handleChatCompletionsStream(c *gin.Context, id string, created int64, model string, tokens <-chan string, errc <-chan error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	chunk := func(delta gin.H, finishReason any) gin.H {
		return gin.H{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []gin.H{
				{"index": 0, "delta": delta, "finish_reason": finishReason},
			},
		}
	}
	write := func(v any) bool {
		_, err := fmt.Fprintf(c.Writer, "data: %s\n\n", mustJSON(v))
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		}
		return err == nil
	}

	// Chunk pertama: role.
	if !write(chunk(gin.H{"role": "assistant"}, nil)) {
		return
	}
streamLoop:
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case tok, ok := <-tokens:
			if !ok {
				break streamLoop
			}
			if !write(chunk(gin.H{"content": tok}, nil)) {
				return
			}
		case err := <-errc:
			if err != nil {
				_ = write(gin.H{"error": gin.H{"message": "LLM error: " + err.Error()}})
				return
			}
		}
	}
	_ = write(chunk(gin.H{}, "stop"))
	_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
}

// mustJSON mengubah v jadi string JSON (fallback "{}" kalau gagal).
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
