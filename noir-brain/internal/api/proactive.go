// F-14: sapaan proaktif, scope in-app only.
// Server mengecek berkala (PROACTIVE_INTERVAL_MIN, default 45 menit);
// bila LLM menilai ada hal layak disampaikan, sapaan di-broadcast ke
// semua client WS yang terhubung. Tidak ada push/browser notification.
package api

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
)

const proactivePrompt = `Kamu Noir, AI companion pribadi Sofyan. Tugasmu: tentukan apakah ada hal yang LAYAK disampaikan ke Sofyan SEKARANG sebagai sapaan proaktif.

Konteks waktu: {{NOW}}.

Hal yang layak: pengingat dari fakta (jadwal, rencana, deadline), sapaan waktu yang natural (pagi/siang/sore/malam) bila sudah lama tidak ngobrol, follow-up dari percakapan terakhir yang belum selesai, hal relevan dari fakta tentang Sofyan.

JANGAN: mengarang fakta/jadwal, mengulang sapaan terakhir, menyapa cuma basa-basi kosong, atau membahas hal sensitif.

Jawab dengan SATU baris pesan singkat (maks 200 karakter, Bahasa Indonesia kasual ala Noir) ATAU jawab persis: TIDAK ADA`

// startProactive menjalankan loop pengecekan proaktif. Berhenti saat ctx dibatalkan.
func (s *Server) startProactive(ctx context.Context) {
	if !s.deps.Config.ProactiveEnabled {
		return
	}
	interval := time.Duration(s.deps.Config.ProactiveInterval) * time.Minute
	if interval < 5*time.Minute {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	log.Printf("proactive: aktif, cek tiap %v (in-app only)", interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkProactive()
		}
	}
}

// checkProactive: satu putaran evaluasi sapaan.
func (s *Server) checkProactive() {
	if isQuietHour(s.deps.Config.ProactiveQuiet) {
		return
	}
	ap, ok := s.deps.Provider.(brain.AgentProvider)
	if !ok {
		return
	}

	now := time.Now()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	nowWIB := now.In(loc)

	facts, _ := s.deps.Store.Facts(30)
	hist, _ := s.deps.Store.Recent(10)
	lastPing, _ := s.deps.Store.LastPing()

	var b strings.Builder
	b.WriteString(strings.Replace(proactivePrompt, "{{NOW}}",
		nowWIB.Format("Monday, 2 January 2006, 15:04 WIB"), 1))
	if len(facts) > 0 {
		b.WriteString("\n\nFAKTA TENTANG SOFYAN:\n- " + strings.Join(facts, "\n- "))
	}
	if len(hist) > 0 {
		b.WriteString("\n\nPERCAKAPAN TERAKHIR:\n")
		for _, h := range hist {
			if h.Role == "user" || h.Role == "assistant" {
				b.WriteString(h.Role + ": " + h.Content + "\n")
			}
		}
	}
	if lastPing.Message != "" {
		b.WriteString("\nSAPAAN PROAKTIF TERAKHIR (" +
			lastPing.CreatedAt.Format("2 Jan 15:04") + "): " + lastPing.Message +
			"\nJangan ulangi/mirip dengan ini.")
	}

	tctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	msg, err := ap.Chat(tctx, []brain.Message{{Role: "user", Content: b.String()}}, nil)
	if err != nil {
		log.Printf("proactive: llm error: %v", err)
		return
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" || strings.EqualFold(text, "TIDAK ADA") {
		return
	}
	// ambil baris pertama saja, batasi panjang
	if i := strings.Index(text, "\n"); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	for _, r := range []string{"\r", "\""} {
		text = strings.ReplaceAll(text, r, "")
	}
	if len([]rune(text)) > 220 {
		text = string([]rune(text)[:217]) + "..."
	}
	if text == "" {
		return
	}

	id, err := s.deps.Store.SavePing(text)
	if err != nil {
		log.Printf("proactive: save: %v", err)
		return
	}
	log.Printf("proactive: ping #%d: %s", id, text)
	s.broadcast(Event{Type: "proactive_ping", Text: text})
	_ = s.deps.Store.MarkPingsDelivered([]int64{id})
}

// sendPendingPings mengirim sapaan yang belum terkirim ke client yang baru connect.
func (s *Server) sendPendingPings(conn *clientConn) {
	pings, err := s.deps.Store.PendingPings()
	if err != nil || len(pings) == 0 {
		return
	}
	var ids []int64
	for _, p := range pings {
		conn.send(Event{Type: "proactive_ping", Text: p.Message})
		ids = append(ids, p.ID)
	}
	_ = s.deps.Store.MarkPingsDelivered(ids)
}

// isQuietHour: true bila jam WIB sekarang masuk rentang sepi ("23-6").
func isQuietHour(spec string) bool {
	parts := strings.Split(strings.TrimSpace(spec), "-")
	if len(parts) != 2 {
		return false
	}
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return false
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	h := time.Now().In(loc).Hour()
	if start <= end {
		return h >= start && h < end
	}
	return h >= start || h < end // rentang melewati tengah malam
}

// lastPingMessage untuk test.
func parseProactiveReply(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || strings.EqualFold(t, "TIDAK ADA") {
		return ""
	}
	if i := strings.Index(t, "\n"); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	return t
}
