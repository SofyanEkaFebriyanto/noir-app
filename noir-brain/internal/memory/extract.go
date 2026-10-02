// Ekstraksi fakta jangka panjang (F-12): setelah tiap percakapan selesai,
// LLM mengekstrak fakta tahan lama tentang pengguna untuk diingat sesi berikut.
package memory

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

const extractPrompt = `Dari percakapan berikut, ekstrak fakta-fakta TAHAN LAMA tentang pengguna (Sofyan) yang berguna diingat untuk sesi berikutnya: preferensi, proyek yang dikerjakan, nama orang, jadwal/rencana, kebiasaan, hal yang dia suka/tidak suka.

Abaikan: basa-basi, pertanyaan sesaat, dan fakta yang sudah jelas umum.
Tulis tiap fakta sebagai SATU kalimat pendek, sudut pandang orang ketiga ("Sofyan ...").

Jawab HANYA dengan JSON array of strings, tanpa teks lain. Contoh:
["Sofyan sedang PKL November 2026 di bidang web", "Sofyan tidak suka suara TTS yang terlalu formal"]
Kalau tidak ada fakta baru yang layak disimpan, jawab: []`

// ExtractFacts meminta LLM mengekstrak fakta dari riwayat terakhir.
// Berjalan async (tidak memblokir respons suara); error diabaikan diam-diam.
func ExtractFacts(ctx context.Context, p brain.AgentProvider, recent []Entry) []string {
	if len(recent) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(extractPrompt)
	b.WriteString("\n\n--- PERCAKAPAN ---\n")
	for _, e := range recent {
		if e.Role != "user" && e.Role != "assistant" {
			continue
		}
		b.WriteString(e.Role + ": " + e.Content + "\n")
	}

	tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	msg, err := p.Chat(tctx, []brain.Message{{Role: "user", Content: b.String()}}, nil)
	if err != nil {
		return nil
	}
	return parseFacts(msg.Content)
}

func parseFacts(s string) []string {
	s = strings.TrimSpace(s)
	// toleransi: ambil substring JSON array bila ada teks lain
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return nil
	}
	var clean []string
	for _, f := range out {
		if f = strings.TrimSpace(f); f != "" && len(f) < 500 {
			clean = append(clean, f)
		}
	}
	return clean
}

// FactsBlock memformat fakta untuk disuntik ke system prompt.
func FactsBlock(facts []string) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nFAKTA TENTANG SOFYAN (dari percakapan sebelumnya — ingat ini):\n")
	for _, f := range facts {
		b.WriteString("- " + f + "\n")
	}
	return b.String()
}
