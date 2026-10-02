package agent

import (
	"context"
	"testing"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

// mockProvider: panggilan pertama -> tool call sysinfo, kedua -> jawaban final.
type mockProvider struct{ n int }

func (m *mockProvider) StreamChat(ctx context.Context, messages []brain.Message) (<-chan string, <-chan error) {
	t, e := make(chan string), make(chan error)
	close(t); close(e)
	return t, e
}

func (m *mockProvider) Chat(ctx context.Context, messages []brain.Message, tools []brain.Tool) (brain.Message, error) {
	m.n++
	if m.n == 1 {
		// pastikan tool sysinfo ditawarkan
		found := false
		for _, t := range tools {
			if t.Name == "sysinfo" { found = true }
		}
		if !found { panic("sysinfo tidak ditawarkan") }
		return brain.Message{
			Role: "assistant",
			ToolCalls: []brain.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: brain.FunctionCall{Name: "sysinfo", Arguments: "{}"},
			}},
		}, nil
	}
	// pastikan hasil tool masuk sebagai pesan role=tool
	last := messages[len(messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call_1" {
		panic("hasil tool tidak diteruskan dengan benar")
	}
	return brain.Message{Role: "assistant", Content: "sistem oke bro"}, nil
}

func TestLoop(t *testing.T) {
	mp := &mockProvider{}
	ag := New(mp, DefaultTools(nil), 8, "")
	var progress []string
	ans, err := ag.Run(context.Background(), []brain.Message{{Role: "user", Content: "cek sistem"}}, func(step int, name string) {
		progress = append(progress, name)
	})
	if err != nil { t.Fatal(err) }
	if ans != "sistem oke bro" { t.Fatalf("jawaban salah: %q", ans) }
	if len(progress) != 1 || progress[0] != "sysinfo" {
		t.Fatalf("progress salah: %v", progress)
	}
}
