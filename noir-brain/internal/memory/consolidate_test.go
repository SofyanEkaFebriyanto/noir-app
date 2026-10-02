package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

// mockProvider mengembalikan respons tetap untuk Chat.
type mockProvider struct{ reply string }

func (m *mockProvider) StreamChat(ctx context.Context, messages []brain.Message) (<-chan string, <-chan error) {
	tok := make(chan string)
	er := make(chan error)
	close(tok)
	close(er)
	return tok, er
}

func (m *mockProvider) Chat(ctx context.Context, messages []brain.Message, tools []brain.Tool) (brain.Message, error) {
	return brain.Message{Role: "assistant", Content: m.reply}, nil
}

func TestConsolidateMergesDupes(t *testing.T) {
	s := openTestStore(t)
	seeds := []string{
		"Sofyan suka kopi tubruk",
		"Sofyan sangat suka kopi tubruk",
		"Sofyan tidak suka kopi sachet",
		"Sofyan PKL November 2026",
		"Sofyan suka kopi tubruk", // duplikat persis (ditolak SaveFact)
		"fakta tambahan 1", "fakta tambahan 2", "fakta tambahan 3",
		"fakta tambahan 4", "fakta tambahan 5", "fakta tambahan 6",
	}
	for _, f := range seeds {
		_ = s.SaveFact(f)
	}
	// 11 unik (1 duplikat persis ditolak) — cukup untuk threshold 10
	mp := &mockProvider{reply: `["Sofyan suka kopi tubruk", "Sofyan tidak suka kopi sachet", "Sofyan PKL November 2026", "fakta tambahan 1", "fakta tambahan 2", "fakta tambahan 3", "fakta tambahan 4", "fakta tambahan 5", "fakta tambahan 6"]`}
	before, after, err := Consolidate(context.Background(), mp, s)
	if err != nil {
		t.Fatal(err)
	}
	if before != 10 || after != 9 {
		t.Fatalf("before=%d after=%d, mau 10→9", before, after)
	}
	facts, _ := s.Facts(50)
	for _, f := range facts {
		if strings.Contains(f, "sangat suka kopi tubruk") {
			t.Fatalf("duplikat semantik tidak kegabung: %v", facts)
		}
	}
}

func TestConsolidateSkipSedikit(t *testing.T) {
	s := openTestStore(t)
	_ = s.SaveFact("Sofyan suka kopi")
	mp := &mockProvider{reply: `[]`}
	before, after, err := Consolidate(context.Background(), mp, s)
	if err != nil || before != 1 || after != 1 {
		t.Fatalf("harusnya skip: %d→%d %v", before, after, err)
	}
}

func TestConsolidateTolakKosong(t *testing.T) {
	s := openTestStore(t)
	for i := 0; i < 10; i++ {
		_ = s.SaveFact(strings.Repeat("x", i+1) + " fakta")
	}
	mp := &mockProvider{reply: `[]`}
	if _, _, err := Consolidate(context.Background(), mp, s); err == nil {
		t.Fatal("LLM kosong harus ditolak demi keamanan")
	}
	n, _ := s.FactCount()
	if n != 10 {
		t.Fatalf("fakta harus utuh, tinggal %d", n)
	}
}

func TestGenerateDailyNote(t *testing.T) {
	s := openTestStore(t)
	_ = s.Append("user", "hari ini gw debug TTS")
	_ = s.Append("assistant", "sip, patch-nya udah dites")
	_ = s.SaveFact("Sofyan debug TTS hari ini")

	mp := &mockProvider{reply: "## Ringkasan\nDebug TTS beres.\n\n## Hal penting\n- patch dites\n\n## Terbuka / follow-up\n- tidak ada\n"}
	content, err := GenerateDailyNote(context.Background(), mp, s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "## Ringkasan") {
		t.Fatalf("format salah: %q", content)
	}
	dateStr := time.Now().In(wibLoc()).Format("2006-01-02")
	got, err := s.GetDailyNote(dateStr)
	if err != nil || got != content {
		t.Fatalf("tidak tersimpan: %v %q", err, got)
	}
	notes, err := s.ListDailyNotes()
	if err != nil || len(notes) != 1 || notes[0].Date != dateStr {
		t.Fatalf("list salah: %v %v", notes, err)
	}
}

func TestGenerateDailyNoteHariSepi(t *testing.T) {
	s := openTestStore(t)
	mp := &mockProvider{reply: "## Ringkasan\nx"}
	content, err := GenerateDailyNote(context.Background(), mp, s, time.Now())
	if err != nil || content != "" {
		t.Fatalf("hari sepi harus \"\": %q %v", content, err)
	}
}

func TestMessagesBetween(t *testing.T) {
	s := openTestStore(t)
	_ = s.Append("user", "pesan hari ini")
	now := time.Now().In(wibLoc())
	start, end := DayBounds(now)
	msgs, err := s.MessagesBetween(start, end)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("harusnya 1 pesan: %v %v", msgs, err)
	}
	// rentang kemarin tidak kena
	msgs, _ = s.MessagesBetween(start.Add(-24*time.Hour), start)
	if len(msgs) != 0 {
		t.Fatalf("harusnya 0: %v", msgs)
	}
}

func TestReplaceFacts(t *testing.T) {
	s := openTestStore(t)
	_ = s.SaveFact("lama 1")
	_ = s.SaveFact("lama 2")
	if err := s.ReplaceFacts([]string{"baru 1", "baru 2", "baru 3"}); err != nil {
		t.Fatal(err)
	}
	facts, _ := s.Facts(10)
	if len(facts) != 3 || facts[0] != "baru 1" {
		t.Fatalf("replace gagal: %v", facts)
	}
}
