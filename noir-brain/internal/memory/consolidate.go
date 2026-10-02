// Konsolidasi fakta berkala ala Hermes (F-16): gabung duplikat semantik,
// selesaikan kontradiksi (pilih yang lebih baru/spesifik), buang yang basi.
// Berjalan async, tidak pernah menyentuh voice path.
package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

const consolidatePrompt = `Kamu merapikan daftar FAKTA tentang Sofyan. Setiap fakta punya tanggal dibuat [YYYY-MM-DD].

Tugasmu:
1. GABUNG duplikat / duplikat semantik (makna sama, redaksi beda) menjadi SATU kalimat terbaik.
2. KONTRADIKSI: bila dua fakta bertentangan (mis. suka vs tidak suka hal yang sama), simpan yang LEBIH BARU atau lebih spesifik, buang yang lama. Kalau ragu, simpan keduanya.
3. BASI: buang fakta yang jelas kedaluwarsa (mis. rencana/event yang tanggalnya sudah lewat berminggu-minggu tanpa kelanjutan). JANGAN buang fakta preferensi yang timeless (suka/tidak suka, kebiasaan).
4. Jangan menambah fakta baru, jangan mengubah makna. Maksimal 200 fakta.

Jawab HANYA dengan JSON array of strings (tiap string satu fakta, sudut pandang orang ketiga "Sofyan ..."), tanpa teks lain. Contoh:
["Sofyan suka kopi tubruk", "Sofyan PKL November 2026 di bidang web"]

--- FAKTA ---
%s`

// minFactsForConsolidate: di bawah ini tidak worth satu panggilan LLM.
const minFactsForConsolidate = 10

// Consolidate merapikan seluruh fakta via LLM. Mengembalikan (sebelum, sesudah).
func Consolidate(ctx context.Context, p brain.AgentProvider, s *Store) (before, after int, err error) {
	facts, err := s.FactsFull()
	if err != nil {
		return 0, 0, err
	}
	before = len(facts)
	if before < minFactsForConsolidate {
		return before, before, nil // terlalu sedikit, lewati
	}

	var b strings.Builder
	for _, f := range facts {
		fmt.Fprintf(&b, "[%s] %s\n", f.CreatedAt.Format("2006-01-02"), f.Fact)
	}
	prompt := fmt.Sprintf(consolidatePrompt, b.String())

	tctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	msg, err := p.Chat(tctx, []brain.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return before, before, fmt.Errorf("consolidate llm: %w", err)
	}
	cleaned := parseFacts(msg.Content)
	if len(cleaned) == 0 {
		return before, before, fmt.Errorf("consolidate: LLM mengembalikan daftar kosong, dibatalkan demi keamanan")
	}
	if len(cleaned) > 200 {
		cleaned = cleaned[:200]
	}
	if err := s.ReplaceFacts(cleaned); err != nil {
		return before, before, err
	}
	return before, len(cleaned), nil
}
