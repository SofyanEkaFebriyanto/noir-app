package api

import (
	"path/filepath"
	"testing"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/memory"
)

func TestParseProactiveReply(t *testing.T) {
	if got := parseProactiveReply("TIDAK ADA"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := parseProactiveReply("  tidak ada  "); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := parseProactiveReply("Bro, jangan lupa makan siang"); got != "Bro, jangan lupa makan siang" {
		t.Fatalf("got %q", got)
	}
	if got := parseProactiveReply("baris satu\nbaris dua"); got != "baris satu" {
		t.Fatalf("got %q", got)
	}
}

func TestIsQuietHour(t *testing.T) {
	// spec invalid → tidak quiet
	if isQuietHour("bogus") {
		t.Fatal("invalid spec should not be quiet")
	}
	if isQuietHour("") {
		t.Fatal("empty spec should not be quiet")
	}
	// spec "0-0" → tidak pernah quiet (h>=0 && h<0 mustahil)
	if isQuietHour("0-0") {
		t.Fatal("0-0 should never be quiet")
	}
}

func TestPingStore(t *testing.T) {
	s, err := memory.Open(filepath.Join(t.TempDir(), "mem.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	id, err := s.SavePing("halo bro")
	if err != nil || id == 0 {
		t.Fatalf("save: %v %d", err, id)
	}
	pending, err := s.PendingPings()
	if err != nil || len(pending) != 1 || pending[0].Message != "halo bro" {
		t.Fatalf("pending: %v %v", err, pending)
	}
	lp, err := s.LastPing()
	if err != nil || lp.Message != "halo bro" {
		t.Fatalf("last: %v %v", err, lp)
	}
	if err := s.MarkPingsDelivered([]int64{id}); err != nil {
		t.Fatal(err)
	}
	pending, err = s.PendingPings()
	if err != nil || len(pending) != 0 {
		t.Fatalf("expected none pending, got %v (err %v)", pending, err)
	}
	// last ping tetap ada (untuk dedupe)
	lp, err = s.LastPing()
	if err != nil || lp.Message != "halo bro" {
		t.Fatalf("last after deliver: %v %v", err, lp)
	}
}
