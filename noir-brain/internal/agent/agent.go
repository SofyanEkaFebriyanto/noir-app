// Package agent adalah loop agent noir-brain: LLM + tool calling.
//
// LLM memutuskan kapan memakai tool; tool dieksekusi lokal di STB,
// hasilnya dikembalikan ke LLM, sampai ada jawaban final.
// Setiap pemanggilan tool dicatat ke operation log (ala Hermes).
package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

// ToolDef adalah satu tool yang bisa dipanggil LLM.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON schema
	// Exec menjalankan tool; argsJSON adalah JSON string argumen dari LLM.
	Exec func(ctx context.Context, argsJSON string) (string, error)
}

// Agent menjalankan loop LLM <-> tool.
type Agent struct {
	Provider brain.AgentProvider
	Tools    []ToolDef
	MaxSteps int
	mu      sync.Mutex
	logFile *os.File
}

// New membuat Agent. logPath boleh "" (tanpa operation log file).
func New(p brain.AgentProvider, tools []ToolDef, maxSteps int, logPath string) *Agent {
	a := &Agent{Provider: p, Tools: tools, MaxSteps: maxSteps}
	if a.MaxSteps <= 0 {
		a.MaxSteps = 8
	}
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			log.Printf("agent log: %v", err)
		} else {
			a.logFile = f
		}
	}
	return a
}

// Close menutup operation log.
func (a *Agent) Close() {
	if a.logFile != nil {
		a.logFile.Close()
	}
}

func (a *Agent) oplog(format string, args ...any) {
	if a.logFile == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	fmt.Fprintf(a.logFile, "[%s] %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

// Run menjalankan loop sampai ada jawaban final dari LLM.
// onProgress boleh nil; dipanggil tiap LLM meminta tool.
func (a *Agent) Run(ctx context.Context, messages []brain.Message, onProgress func(step int, toolName string)) (string, error) {
	btools := make([]brain.Tool, 0, len(a.Tools))
	for _, t := range a.Tools {
		btools = append(btools, brain.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}

	for step := 0; step < a.MaxSteps; step++ {
		msg, err := a.Provider.Chat(ctx, messages, btools)
		if err != nil {
			return "", err
		}
		messages = append(messages, msg)

		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}
		for _, tc := range msg.ToolCalls {
			if onProgress != nil {
				onProgress(step, tc.Function.Name)
			}
			out := a.execTool(ctx, tc)
			messages = append(messages, brain.Message{
				Role:       "tool",
				Content:    out,
				ToolCallID: tc.ID,
			})
		}
	}
	return "", fmt.Errorf("agent: max steps (%d) tercapai", a.MaxSteps)
}

func (a *Agent) execTool(ctx context.Context, tc brain.ToolCall) string {
	name := tc.Function.Name
	var def *ToolDef
	for i := range a.Tools {
		if a.Tools[i].Name == name {
			def = &a.Tools[i]
			break
		}
	}
	if def == nil {
		a.oplog("tool=%s status=unknown-tool", name)
		return "error: tool tidak dikenal: " + name
	}
	a.oplog("tool=%s args=%.500s", name, tc.Function.Arguments)
	out, err := def.Exec(ctx, tc.Function.Arguments)
	if err != nil {
		a.oplog("tool=%s status=error err=%.500s", name, err.Error())
		return "error: " + err.Error()
	}
	a.oplog("tool=%s status=ok out=%.500s", name, out)
	return out
}
