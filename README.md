# Noir App

Aplikasi interaktif **Noir** — AI voice companion ala JARVIS: **full voice-to-voice, tanpa teks di UI**.
Panggil "Hey Noir", ngomong, dia jawab pakai suara. Avatar beranimasi mengikuti status.

```
┌──────────────┐   WebSocket    ┌──────────────┐
│  noir-mobile │ ◄────────────► │  noir-brain  │
│  (Flutter)   │  chat + events │  (Go, di STB)│
│              │                │  + LLM       │
│ wake word    │                │ persona Noir │
│ STT on-device│                │ memori SQLite│
│ TTS per kal. │                └──────────────┘
└──────────────┘
```

Satu otak (`noir-brain`), banyak badan: aplikasi mobile sekarang, robot fisik nanti.

## Struktur

| Path | Isi |
|---|---|
| `noir-brain/` | Backend Go (Gin + WebSocket). Jalan 24/7 di STB H680P/Armbian |
| `noir-mobile/` | Aplikasi Flutter (Android dulu). Voice UI, nol teks |
| `assets/avatar/` | Animasi avatar per status (`idle`, `listening`, `thinking`, `speaking`) |
| `docs/` | Blueprint, PRD, design doc (sudah disetujui 2026-10-01) |

## Status

- [x] Perencanaan — blueprint/PRD/design disetujui (2026-10-01)
- [x] **Fase 1 — MVP voice-only**: wake word + STT + TTS half-duplex
- [x] **Fase 1.5 — MVP runnable**: settings UI, AndroidManifest, aset avatar, systemd unit
- [ ] Fase 2 — Full duplex (tap-to-interrupt, continuous conversation, barge-in suara eksperimental)
- [ ] Fase 3 — Memori jangka panjang & kepribadian adaptif
- [ ] Fase 4 — Robot fisik ESP32 (butuh modal)

## Quickstart

Butuh: Go 1.22+, Flutter 3.22+, STB/komputer yang bisa dijangkau HP (Tailscale).

```bash
# 1. Backend
cd noir-brain
cp .env.example .env   # isi LLM_API_KEY
go run ./cmd/server    # :8080

# 2. App
cd noir-mobile
flutter pub get
flutter run            # sesuaikan WS_URL ke alamat STB
```

Detail tiap komponen ada di README masing-masing folder.

## Privasi

Audio **tidak pernah** dikirim ke server — STT jalan on-device di HP, yang dikirim hanya transkrip teks. Riwayat tersimpan lokal di STB (SQLite).
