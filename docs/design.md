# Design Doc — Aplikasi Interaktif Noir

**Status:** Draf · **Tanggal:** 2026-10-01 · **Target:** v1.0 (MVP)

---

## 1. Arsitektur sistem

```
┌─────────────────────────────┐         WebSocket (wss)          ┌──────────────────────────────┐
│        APLIKASI (Flutter)   │◄────────────────────────────────►│      NOIR BRAIN SERVER (Go)  │
│  Android (v1), iOS (nanti)  │   chat stream + avatar events    │      STB H680P · Armbian     │
│                             │                                  │                              │
│  ┌───────────────────────┐  │         HTTPS (REST)             │  ┌────────────────────────┐  │
│  │ AvatarRenderer        │  │◄────────────────────────────────►│  │ API (Gin)              │  │
│  │ (webp/mp4 per state)  │  │   /tts, /history, /config        │  │  /ws, /tts, /history   │  │
│  ├───────────────────────┤  │                                  │  ├────────────────────────┤  │
│  │ VoiceUI (nol teks):   │  │                                  │  │ BrainProvider          │  │
│  │ avatar + waveform,    │  │                                  │  │  (interface, default:  │  │
│  │ wake word, STT, TTS   │  │                                  │  │   OpenAI-compatible)   │  │
│  └───────────────────────┘  │                                  │  ├────────────────────────┤  │
└─────────────────────────────┘                                  │  │ SQLite (chat, memory,  │  │
                                                                 │  │  persona config)       │  │
                                                                 └──────────┬─────────────────┘
                                                                            │ HTTPS (keluar)
                                                                            ▼
                                                                 ┌────────────────────────┐
                                                                 │ LLM Provider API       │
                                                                 │ (kunci di server saja) │
                                                                 └────────────────────────┘

Fase robot (nanti):
  ESP32 ──MQTT──► Brain Server (topik: noir/robot/+) ──► respons teks+audio──► speaker/LCD/servo
```

**Keputusan kunci:** satu koneksi WebSocket untuk semuanya (chat streaming + event status avatar). REST hanya untuk operasi non-realtime (riwayat, konfigurasi, TTS cache).

---

## 2. Komponen

### 2.1 Aplikasi — `app/` (Flutter)

| Modul | Isi |
|---|---|
| `VoiceScreen` | Satu-satunya layar: avatar full-body + cincin waveform + titik status koneksi. Nol teks |
| `AvatarRenderer` | Menampilkan aset per status (lihat §6). Preload semua aset saat splash agar transisi mulus |
| `WakeWordService` | Porcupine on-device, keyword "Hey Noir" → picu status `listening`. Tap avatar sebagai fallback |
| `SpeechPipeline` | STT streaming on-device (`speech_to_text`); endpointing: anggap selesai jika diam ~0,8 dtk; kirim transkrip via WS |
| `TtsService` | Bungkus `flutter_tts`: **antrean per kalimat** (kalimat 1 dibunyikan sambil kalimat 2 di-generate); callback mulai/selesai → status `speaking` |
| `NoirConnection` | WebSocket client: kirim transkrip, terima token stream + event status; auto-reconnect dengan backoff |
| `SettingsStore` | Preferensi: voice TTS, kecepatan, sensitivitas mic, varian avatar (`shared_preferences`) |

### 2.2 Server — `server/` (Go + Gin)

| Modul | Isi |
|---|---|
| `api/` | Handler: `GET /ws` (upgrade), `GET /tts`, `GET /history`, `GET /config` |
| `brain/` | `BrainProvider` interface + implementasi default; bangun system prompt dari persona + memori |
| `avatar/` | Penerbit event status avatar berdasarkan tahap pemrosesan |
| `store/` | SQLite via `modernc.org/sqlite` (pure Go — gampang cross-compile ke ARM STB) |
| `config/` | YAML: kunci API, model, persona path, batas rate |

### 2.3 BrainProvider (interface)

```go
type BrainProvider interface {
    // StreamChat mengirim riwayat + persona, mengembalikan channel token.
    StreamChat(ctx context.Context, messages []Message) (<-chan Token, error)
    Name() string
}
```

Implementasi v1: `OpenAICompatibleProvider` — POST ke `{baseURL}/chat/completions` dengan `stream: true`. Base URL & model dari config, jadi ganti provider tanpa ubah kode.

---

## 3. Kontrak API

### 3.1 WebSocket — `GET /ws`

**Client → Server**

```jsonc
{ "type": "chat", "id": "m-001", "text": "halo noir" }   // kirim pesan
{ "type": "ping" }                                        // keepalive
```

**Server → Client**

```jsonc
{ "type": "avatar_state", "state": "thinking" }            // idle|listening|thinking|speaking
{ "type": "token", "id": "m-001", "delta": "halo " }       // stream per token
{ "type": "token", "id": "m-001", "delta": "juga!" }
{ "type": "message_done", "id": "m-001", "full_text": "halo juga!" }
{ "type": "error", "code": "LLM_TIMEOUT", "message": "..." }
```

Urutan normal per pesan: `avatar_state(thinking)` → token… → `message_done` → `avatar_state(speaking)` → (app selesai TTS) → `avatar_state(idle)`.

> Catatan: `speaking` dipicu server saat teks lengkap, tapi app yang tahu kapan audio selesai. App mengirim `{ "type": "tts_done" }` agar server boleh kembalikan status ke `idle` — detail kecil yang mencegah avatar "ngomong" padahal suara sudah habis.

> Catatan voice-only: pesan `{ "type": "chat", "text": ... }` berisi **transkrip STT**, bukan ketikan — tidak ada input teks di app. Secara internal pipeline tetap teks (speech→teks→LLM→teks→speech), yang dihapus hanya teks di UI.

### 3.3 Pipeline suara v1.0 (half-duplex)

```
Mic ──► [Porcupine: "Hey Noir"] ──► listening ──► [STT streaming on-device]
  ──► endpointing (diam 0,8 dtk) ──► WS {type:chat, text: transkrip}
  ──► avatar_state(thinking) ──► LLM stream ──► pecah per kalimat
  ──► TTS antrean per kalimat ──► avatar_state(speaking)
  ──► audio habis ──► {type:tts_done} ──► avatar_state(idle)
```

- **Half-duplex di v1:** mic dimatikan selama `speaking`. Barge-in (motong omongan) butuh echo cancellation dan dijadwalkan v1.1 — jujur ini bagian tersulitnya.
- **Audio mentah tidak pernah ke server** — STT on-device, yang dikirim hanya transkrip.
- **Target latency:** kalimat pertama dibunyikan < 2 dtk setelah endpointing (di LAN). Triknya: TTS per kalimat, bukan nunggu respons lengkap.
- **Fallback:** LLM timeout → server kirim teks pendek, app bacakan "maaf, coba ulangi?" via TTS.

### 3.4 Full duplex v1.1 (2026-10-01)

- **Tap-to-interrupt** (jalur utama): tap avatar saat `speaking` → app hentikan TTS,
  kirim `{type:interrupt}` (server batalkan stream via `cancelStream`), langsung `listening`.
- **Voice barge-in** (eksperimental, default mati): `BargeInMonitor` — STT tetap aktif
  saat speaking dalam mode monitor (`autoFinish=false`); parsial yang cocok dengan
  teks yang dibacakan = suara sendiri (abaikan); 2 parsial beruntun yang tidak
  cocok = interupsi pengguna → `_beginListening`.
- **Continuous conversation**: `_conversationUntil` = 30 dtk setelah tiap ucapan
  pengguna; `_toIdle` dalam window → langsung `listening` tanpa wake word.
- **Perintah lokal** (tanpa LLM, regex di `_onUserSpeech`):
  `diam|stop|berhenti|udah` → henti total + akhiri sesi; `ulangi` → bacakan ulang `_lastResponse`.
- **Listen timeout**: 12 dtk tanpa suara saat `listening` → kembali `idle` (F-08).

### 3.2 REST

| Method & path | Fungsi |
|---|---|
| `GET /tts?text=...&voice=...` | Sintesis TTS server-side (cadangan; v1 utama on-device). Respons: mp3, cache di `Cache-Control` |
| `GET /history?session=...&limit=50` | Riwayat chat sesi |
| `GET /config` | Info publik: nama, versi persona, daftar voice tersedia |
| `GET /healthz` | Health check untuk monitoring STB |

---

## 4. Model data (SQLite)

```sql
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER
);

CREATE TABLE messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    role        TEXT NOT NULL,          -- 'user' | 'noir'
    text        TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);

-- Fase 3 (v2.0): memori jangka panjang. Dibuat sekarang agar skema siap.
CREATE TABLE memories (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,          -- 'fact' | 'preference' | 'event'
    content     TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

CREATE TABLE persona (
    key         TEXT PRIMARY KEY,       -- 'system_prompt', 'style_notes', ...
    value       TEXT NOT NULL,
    updated_at  INTEGER NOT NULL
);
```

System prompt Noir dirakit saat startup: `persona.system_prompt` + ringkasan `memories` (v2.0; v1 hanya system prompt + riwayat sesi berjalan).

---

## 5. Avatar state machine

```
        ┌──────────────────────────────────────────────────┐
        ▼                                                  │
┌──────────────┐  pesan masuk   ┌──────────────┐  LLM mulai  ┌──────────────┐
│     idle     │ ─────────────► │   thinking   │ ──────────► │   speaking   │
│ (loop santai)│                │ (anim mikir) │             │ (anim ngomong│
└──────────────┘                └──────────────┘             │  + TTS audio)│
        ▲                                                   └──────┬───────┘
        │                          ┌──────────────┐                │ tts_done
        └──────────────────────────│  listening   │◄──────────────┘
              (v1.1: mic aktif)     │ (anim denger)│
                                   └──────────────┘
```

Transisi dipicu event `avatar_state` dari server (kecuali `listening` yang dipicu app saat mic aktif di v1.1).

---

## 6. Aset avatar & pemetaan status

Aset yang sudah ada (dari sesi avatar, di `~/workspace/avatars/`):

| Status | Aset saat ini | Catatan |
|---|---|---|
| `idle` | `avatar-1790850701595703294-0.mp4` (loop) | ✅ ada |
| `thinking` / `working` | `...-working.mp4` | ✅ ada (dipakai untuk thinking) |
| `speaking` | — | ❌ **perlu dibuat — KRITIS untuk voice-only** (loop 3–5 dtk, mulut/ekspresi "berbicara"). Tanpa ini avatar kelihatan diem saat bersuara |
| `listening` | — | ❌ perlu dibuat (v1.1; bisa reuse idle dulu) |
| `milestone_level_up` | `...-milestone_level_up.mp4` | ✅ ada (momen spesial) |
| `making_something` | `...-making_something.mp4` | ✅ ada (cadangan) |

Varian gambar: `saved/noir-moody.webp` (alternatif), `saved/noir-pixelart.webp` (**dicadangkan untuk LCD robot** — resolusi rendah pas untuk pixel art).

**PR asset sebelum eksekusi fase 1:** generate 1 video `speaking` (±3–5 dtk loop). Kompres semua mp4 (target total < 30 MB).

---

## 7. Deployment

```
STB H680P (Armbian, 24/7)
├── /opt/noir/server            ← binary Go (cross-compile GOARCH=arm64)
├── /opt/noir/noir.db           ← SQLite
├── /opt/noir/config.yaml       ← kunci API (chmod 600)
└── systemd unit noir.service   ← auto-restart

Akses:
├── LAN:  ws://<ip-stb>:8080/ws            (langsung)
└── Remote: Tailscale (utama) · atau Cloudflare Tunnel dari STB (cadangan)
```

Build & deploy v1 manual dulu (scp + ssh); otomasi (script deploy) menyusul kalau ritmenya sudah stabil.

---

## 8. Keamanan & privasi

- Kunci API LLM **hanya di server** (`config.yaml`, chmod 600). App tidak pernah pegang secret.
- v1 single-user tanpa auth; saat remote dibuka via Tailscale (yang sudah ber-auth), itu lapisan keamanannya. Kalau pakai Cloudflare Tunnel publik → tambah token sederhana di header WS (dibahas saat dibutuhkan).
- Chat tersimpan di STB milik sendiri. Tidak ada analytics/SDK pihak ketiga di app.
- Rate limit sederhana di server: maks 60 pesan/jam (cegah loop tak sengaja menguras kuota API).

---

## 9. Fase robot — Arduino/ESP32 (nanti, butuh modal)

**Arsitektur:** robot = klien "bodoh", otak tetap server.

```
┌──────────────────┐   MQTT (mosquitto di STB)   ┌──────────────────┐
│  ESP32 robot     │ ◄──────────────────────────► │  Brain Server    │
│  - LCD: Noir     │   noir/robot/cmd  (tampil,   │  + mqtt bridge   │
│    pixel-art     │     gerak servo, bunyi)      │                  │
│  - Speaker: TTS  │   noir/robot/event (tombol, │                  │
│  - Servo: kepala │     "user mendekat")         │                  │
│  - Mic (opsional │                              │                  │
│    v2 robot)     │                              │                  │
└──────────────────┘                              └──────────────────┘
```

| Komponen | Estimasi peran |
|---|---|
| ESP32 DevKit | WiFi + MQTT + kontrol |
| LCD TFT 2.4" / OLED | Tampilkan `noir-pixelart.webp` (dikonversi ke format LCD) + status |
| Speaker + modul DFPlayer/MAX98357 | Putar mp3 TTS dari server |
| 1–2 servo SG90 | Angguk / geleng kepala |
| Tombol fisik | "Ngomong" (picu listening) |

Firmware (Arduino/C++): loop MQTT, parser perintah JSON kecil, tidak ada logika AI di device. Semua kecerdasan tetap di server — ganti "badan" tanpa ganti "otak".

---

## 10. Rencana pengujian

| Level | Cara |
|---|---|
| Unit (Go) | `go test`: state machine avatar, perakit prompt, parser stream |
| Kontrak WS | Skrip uji: kirim `chat`, verifikasi urutan event `thinking → token* → message_done → speaking` |
| Manual app (voice) | Checklist: 10 percakapan suara di ruangan normal; uji wake word 20x (ukur false accept/reject); matikan server di tengah speaking (indikator merah); STT diam 10 dtk (kembali idle); background/foreground; uji di dekat TV menyala (noise) |
| Demo gate | Rekam layar 2 menit: buka app → ngobrol → TTS bunyi → offline → online lagi |

---

## 11. Struktur repo (rencana)

```
noir-app/
├── docs/               ← blueprint.md, prd.md, design.md (dari goal ini)
├── assets/
│   ├── avatar/         ← webp + mp4 per status (terkompres)
│   └── pixelart/       ← varian untuk LCD robot
├── app/                ← Flutter
│   └── lib/
│       ├── avatar/ chat/ connection/ tts/ settings/
├── server/             ← Go
│   ├── api/ brain/ avatar/ store/ config/
│   └── main.go
└── firmware/           ← (fase 4) Arduino/ESP32
```

---

*Dokumen pendamping: `blueprint.md` (visi & fase), `prd.md` (kebutuhan produk).*
