package memory

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAppendAndRecentOrder(t *testing.T) {
	s := openTestStore(t)

	msgs := []struct{ role, content string }{
		{"user", "halo noir"},
		{"assistant", "halo juga"},
		{"user", "lagi apa?"},
	}
	for _, m := range msgs {
		if err := s.Append(m.role, m.content); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, err := s.Recent(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("mau 3 pesan, dapat %d", len(got))
	}
	// harus kronologis: lama → baru
	for i, m := range msgs {
		if got[i].Role != m.role || got[i].Content != m.content {
			t.Errorf("pesan %d: mau (%s,%s), dapat (%s,%s)",
				i, m.role, m.content, got[i].Role, got[i].Content)
		}
	}
}

func TestRecentLimit(t *testing.T) {
	s := openTestStore(t)
	for i := 0; i < 5; i++ {
		if err := s.Append("user", "pesan"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	got, err := s.Recent(3)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("mau 3 pesan, dapat %d", len(got))
	}
}
