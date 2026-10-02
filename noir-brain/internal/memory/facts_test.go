package memory

import (
	"path/filepath"
	"testing"
)

func TestFactsAndSettings(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "mem.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.SaveFact("Sofyan suka kopi tubruk"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFact("sofyan suka KOPI TUBRUK"); err != nil { // duplikat (case-insensitive)
		t.Fatal(err)
	}
	facts, err := s.Facts(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0] != "Sofyan suka kopi tubruk" {
		t.Fatalf("facts = %v", facts)
	}

	if v, _ := s.GetSetting("persona_override"); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
	if err := s.SetSetting("persona_override", "kamu noir"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting("persona_override"); v != "kamu noir" {
		t.Fatalf("got %q", v)
	}
	if err := s.DelSetting("persona_override"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting("persona_override"); v != "" {
		t.Fatalf("expected empty after delete, got %q", v)
	}
}

func TestParseFacts(t *testing.T) {
	in := "Intro text\n[\"fakta satu\", \"fakta dua\"]\ntrailing"
	got := parseFacts(in)
	if len(got) != 2 || got[0] != "fakta satu" {
		t.Fatalf("got %v", got)
	}
	if parseFacts("[]") != nil {
		t.Fatal("expected nil for empty array")
	}
	if parseFacts("bukan json") != nil {
		t.Fatal("expected nil for non-json")
	}
}
