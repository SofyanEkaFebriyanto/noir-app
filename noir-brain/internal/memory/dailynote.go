// Daily note otomatis ala Hermes (F-16): ringkasan harian dari percakapan.
// Berjalan async via ticker, tidak pernah menyentuh voice path.
package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

const dailyNotePrompt = `Buatkan catatan harian singkat dari percakapan Noir dengan Sofyan hari ini.
Bahasa Indonesia, gaya Noir: santai, langsung, tanpa basa-basi.

Fakta-fakta baru tentang Sofyan hari ini:
%s

--- PERCAKAPAN HARI INI ---
%s

Tulis dalam markdown dengan format persis ini:

## Ringkasan
(satu-dua paragraf tentang hari ini)

## Hal penting
- keputusan / info penting (atau "- tidak ada" bila kosong)

## Terbuka / follow-up
- hal yang belum selesai / perlu ditindaklanjuti (atau "- tidak ada")

Kalau percakapan hari ini kosong atau tidak ada isinya sama sekali, jawab persis: TIDAK ADA`

// wibLoc adalah zona waktu server (STB = WIB).
func wibLoc() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.Local
}

// DayBounds mengembalikan [00:00, 00:00+1hari) WIB untuk tanggal date.
func DayBounds(date time.Time) (time.Time, time.Time) {
	loc := wibLoc()
	y, m, d := date.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	return start, start.Add(24 * time.Hour)
}

// GenerateDailyNote membuat catatan harian untuk tanggal date (WIB).
// Mengembalikan "" bila tidak ada konten layak catat. Menyimpan ke DB bila ada isi.
func GenerateDailyNote(ctx context.Context, p brain.AgentProvider, s *Store, date time.Time) (string, error) {
	start, end := DayBounds(date)
	msgs, err := s.MessagesBetween(start, end)
	if err != nil {
		return "", err
	}
	var conv []Entry
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			conv = append(conv, m)
		}
	}
	if len(conv) == 0 {
		return "", nil // hari sepi, tidak usah catat
	}
	newFacts, err := s.FactsBetween(start, end)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, m := range conv {
		// batasi panjang tiap pesan agar prompt tidak jebol
		c := m.Content
		if len(c) > 800 {
			c = c[:800] + "…"
		}
		b.WriteString(m.Role + ": " + c + "\n")
	}
	factsStr := "(tidak ada)"
	if len(newFacts) > 0 {
		factsStr = "- " + strings.Join(newFacts, "\n- ")
	}
	prompt := fmt.Sprintf(dailyNotePrompt, factsStr, b.String())

	tctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	msg, err := p.Chat(tctx, []brain.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return "", fmt.Errorf("dailynote llm: %w", err)
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" || strings.EqualFold(content, "TIDAK ADA") {
		return "", nil
	}
	dateStr := date.In(wibLoc()).Format("2006-01-02")
	if err := s.SaveDailyNote(dateStr, content); err != nil {
		return "", err
	}
	return content, nil
}
