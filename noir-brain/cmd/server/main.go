// Command server menjalankan Noir Brain: HTTP API + WebSocket untuk aplikasi Noir.
package main

import (
	"log"
	"os"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/api"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/config"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/memory"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/persona"
)

func main() {
	cfg := config.Load()

	if cfg.LLMAPIKey == "" {
		log.Println("PERINGATAN: LLM_API_KEY kosong — chat akan gagal sampai diisi (lihat .env.example)")
	}

	store, err := memory.Open(cfg.DataDir + "/noir.db")
	if err != nil {
		log.Fatalf("memory: %v", err)
		os.Exit(1)
	}
	defer store.Close()

	provider := brain.NewOpenAICompat(brain.OpenAICompatConfig{
		BaseURL: cfg.LLMBaseURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
	})

	srv := api.New(api.Deps{
		Config:       cfg,
		Provider:     provider,
		Store:        store,
		SystemPrompt: persona.SystemPrompt(),
	})

	log.Printf("noir-brain listening on :%s (model=%s)", cfg.Port, cfg.LLMModel)
	if err := srv.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}
