// Package brain — implementasi OpenAI-compatible (streaming SSE).
package brain

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OpenAICompatConfig menunjuk ke API LLM mana pun yang OpenAI-compatible.
type OpenAICompatConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type openAICompat struct {
	cfg    OpenAICompatConfig
	client *http.Client
}

// NewOpenAICompat membuat Provider dari config.
func NewOpenAICompat(cfg OpenAICompatConfig) Provider {
	return &openAICompat{cfg: cfg, client: &http.Client{Timeout: 180 * time.Second}}
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (p *openAICompat) StreamChat(ctx context.Context, messages []Message) (<-chan string, <-chan error) {
	tokens := make(chan string, 64)
	errc := make(chan error, 1)

	go func() {
		defer close(tokens)
		defer close(errc)

		body, err := json.Marshal(map[string]any{
			"model":    p.cfg.Model,
			"messages": messages,
			"stream":   true,
		})
		if err != nil {
			errc <- err
			return
		}

		url := strings.TrimSuffix(p.cfg.BaseURL, "/") + "/chat/completions"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			errc <- err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if p.cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			errc <- err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			errc <- fmt.Errorf("llm: HTTP %d", resp.StatusCode)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}
			var chunk streamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue // abaikan baris yang bukan JSON valid
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content == "" {
					continue
				}
				select {
				case tokens <- choice.Delta.Content:
				case <-ctx.Done():
					errc <- ctx.Err()
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errc <- err
		}
	}()

	return tokens, errc
}
