// F-16: memory ala Hermes — konsolidasi fakta berkala + daily note otomatis.
//
// Semua job berjalan async di background, diserialkan oleh satu mutex agar
// tidak tumpang tindih. Voice path tidak pernah menyentuh mutex ini.
package api

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/memory"
)

// memJobMu menserialkan job memory (konsolidasi & daily note).
var memJobMu sync.Mutex

const (
	consolidateEvery   = 24 * time.Hour
	consolidateMaxFacts = 150 // paksa konsolidasi bila fakta melebihi ini
	memTickEvery        = 30 * time.Minute
)

// startMemoryJobs menjalankan loop perawatan memory. Berhenti saat ctx dibatalkan.
func (s *Server) startMemoryJobs(ctx context.Context) {
	if !s.deps.Config.MemoryJobsEnabled {
		return
	}
	log.Printf("memory-jobs: aktif (konsolidasi tiap 24 jam / >%d fakta, daily note otomatis)", consolidateMaxFacts)
	// catch-up tak lama setelah start (mis. setelah downtime semalam)
	go func() {
		select {
		case <-time.After(2 * time.Minute):
			s.memoryTick()
		case <-ctx.Done():
		}
	}()
	t := time.NewTicker(memTickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.memoryTick()
		}
	}
}

// memoryTick: satu putaran perawatan (diserialkan, async dari voice).
func (s *Server) memoryTick() {
	go func() {
		memJobMu.Lock()
		defer memJobMu.Unlock()
		s.maybeConsolidate()
		s.maybeDailyNote()
	}()
}

// maybeConsolidate menjalankan konsolidasi bila sudah waktunya / fakta menumpuk.
func (s *Server) maybeConsolidate() {
	ap, ok := s.deps.Provider.(brain.AgentProvider)
	if !ok {
		return
	}
	lastStr, _ := s.deps.Store.GetSetting("memory_last_consolidate")
	due := true
	if lastStr != "" {
		if last, err := time.Parse(time.RFC3339, lastStr); err == nil {
			due = time.Since(last) >= consolidateEvery
		}
	}
	if !due {
		if n, err := s.deps.Store.FactCount(); err != nil || n < consolidateMaxFacts {
			return
		}
	}
	log.Printf("memory-jobs: konsolidasi fakta dimulai…")
	before, after, err := memory.Consolidate(context.Background(), ap, s.deps.Store)
	if err != nil {
		log.Printf("memory-jobs: konsolidasi gagal: %v", err)
		return
	}
	_ = s.deps.Store.SetSetting("memory_last_consolidate", time.Now().Format(time.RFC3339))
	log.Printf("memory-jobs: konsolidasi selesai (%d → %d fakta)", before, after)
}

// maybeDailyNote membuat catatan harian yang belum ada:
// kemarin (catch-up) + hari ini bila sudah lewat 23:30 WIB.
func (s *Server) maybeDailyNote() {
	ap, ok := s.deps.Provider.(brain.AgentProvider)
	if !ok {
		return
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)

	targets := []time.Time{today.AddDate(0, 0, -1)} // kemarin dulu
	if now.Hour() >= 23 && now.Minute() >= 30 {
		targets = append(targets, today)
	}
	for _, day := range targets {
		dateStr := day.Format("2006-01-02")
		existing, err := s.deps.Store.GetDailyNote(dateStr)
		if err != nil || existing != "" {
			continue
		}
		content, err := memory.GenerateDailyNote(context.Background(), ap, s.deps.Store, day)
		if err != nil {
			log.Printf("memory-jobs: daily note %s gagal: %v", dateStr, err)
			continue
		}
		if content != "" {
			log.Printf("memory-jobs: daily note %s tersimpan (%d karakter)", dateStr, len(content))
		}
	}
}

// registerMemory mendaftarkan endpoint memory F-16.
func (s *Server) registerMemory(r *gin.Engine) {
	// Export untuk sync ke Obsidian vault (dipakai cron di VM, bukan dari STB).
	r.GET("/v1/memory/export", func(c *gin.Context) {
		facts, err := s.deps.Store.FactsFull()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		notes, err := s.deps.Store.ListDailyNotes()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fj := make([]gin.H, 0, len(facts))
		for _, f := range facts {
			fj = append(fj, gin.H{"fact": f.Fact, "created_at": f.CreatedAt.Format(time.RFC3339)})
		}
		nj := make([]gin.H, 0, len(notes))
		for _, n := range notes {
			nj = append(nj, gin.H{"date": n.Date, "content": n.Content})
		}
		c.JSON(http.StatusOK, gin.H{
			"facts":       fj,
			"daily_notes": nj,
			"exported_at": time.Now().Format(time.RFC3339),
		})
	})
	// Trigger konsolidasi manual (sinkron, untuk testing/admin).
	r.POST("/v1/memory/consolidate", func(c *gin.Context) {
		ap, ok := s.deps.Provider.(brain.AgentProvider)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider tidak mendukung"})
			return
		}
		memJobMu.Lock()
		defer memJobMu.Unlock()
		tctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		before, after, err := memory.Consolidate(tctx, ap, s.deps.Store)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_ = s.deps.Store.SetSetting("memory_last_consolidate", time.Now().Format(time.RFC3339))
		c.JSON(http.StatusOK, gin.H{"before": before, "after": after})
	})
}
