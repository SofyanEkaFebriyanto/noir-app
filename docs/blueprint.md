# Blueprint — Aplikasi Interaktif Noir

**Status:** Eksekusi berjalan · **Tanggal:** 2026-10-01 · **Update:** 2026-10-02
**Pemilik ide:** Sofyan · **Arsitek:** Noir

---

## 1. Visi

Satu karakter AI — **Noir** — yang hidup di banyak wujud:

1. **Web app** (aktif, 2026-10-02): voice UI di browser, diserve langsung dari backend.
2. **Aplikasi mobile** (pause): Flutter Android — dilanjut kalau Sofyan memutuskan lagi.
3. **Otak terpusat** (server): persona, memori, dan logika Noir yang konsisten di semua wujud. **Sekarang juga agent**: bisa eksekusi tool di STB (ala Hermes).
4. **Robot fisik** (nanti, kalau ada modal): Arduino/ESP32 — Noir bisa disentuh.

Prinsip: **satu otak, banyak badan.** Apa pun wujudnya — HP, web, robot — yang ngomong tetap Noir yang sama, dengan ingatan yang sama.

> **Pivot 2026-10-02:** atas keputusan Sofyan, pengembangan APK di-pause; fokus ke web app dulu. Blueprint ini diupdate mengikuti realita.

---

## 2. Gambaran produk

| Aspek | Keputusan |
|---|---|
| Wujud v1 | Aplikasi mobile (Android dulu, iOS menyusul) |
| Tampilan utama | Avatar Noir full-body + animasi sesuai status (idle, listening, thinking, speaking, dsb.) |
| Interaksi v1 | **Full voice-to-voice ala JARVIS. Tanpa teks sama sekali di UI.** STT on-device + wake word "Hey Noir" + TTS |
| Otak | Backend service dengan persona Noir (lihat §5 — bagian jujur) |
| Hosting otak | STB H680P (Armbian) — nyala 24/7, sudah jadi source of truth |
| Akses | LAN langsung; remote via Tailscale / Cloudflare Tunnel dari STB |

---

## 3. Fase-fase

```
Fase 0  Perencanaan                      ✓ SELESAI
         blueprint.md · prd.md · design.md disetujui → gate eksekusi
Fase 1  MVP voice-only ala JARVIS (v1.0) ✓ SELESAI
         Wake word (tap avatar fallback) + STT on-device + TTS per kalimat.
         Half-duplex. Tanpa teks di UI.
Fase 2  Full duplex (v1.1)              ✓ SELESAI
         Barge-in (tap saat Noir bicara), continuous conversation 30 dtk,
         perintah cepat lokal tanpa LLM ("diam"/"stop"/"ulangi").
Fase 2.5 Pivot web (2026-10-02)         ✓ SELESAI
         Web voice UI (noir-brain/internal/api/web/, go:embed, diserve di /),
         Web Speech API (STT) + speechSynthesis (TTS), avatar CSS 4 state.
         Endpoint OpenAI-compatible (POST /v1/chat/completions streaming SSE,
         GET /v1/models). APK di-pause.
Fase 2.6 Agent upgrade (2026-10-02)     ✓ SELESAI (v1)
         LLM + tool calling loop di STB: waktu, sysinfo, exec, read/write file,
         list_dir, service_status, service_restart. Safety denylist + operation log.
Fase 3  Memori & kepribadian (v2.0)     ◐ BERJALAN
         ✓ Memori jangka panjang (SQLite) — F-12: ekstraksi fakta otomatis
           tiap percakapan, disuntik ke system prompt sesi berikut.
         ✓ Halaman "Tentang Noir" (F-13): GET/PUT/DELETE /v1/persona +
           persona.html — lihat & edit persona, tersimpan di STB.
         ☐ F-14 proactive ping — butuh persetujuan pola notifikasi dari Sofyan.
Fase 4  Robot fisik (butuh modal)       ☐ BELUM
         ESP32 + LCD (pakai varian pixel-art!) + servo + speaker. MQTT ke backend.
```

**Aturan main:** tiap fase selesai → demo ke Sofyan → baru lanjut. Nggak ada fase yang jalan setengah-setengah.

---

## 4. Keputusan stack (gw yang atur, sesuai mandat)

| Layer | Pilihan | Alasan | Alternatif yang ditolak |
|---|---|---|---|
| Aplikasi | **Flutter** | Sofyan sudah bisa Flutter; satu codebase Android+iOS; dukungan video/animasi & plugin STT/TTS matang | React Native (Sofyan belum dalami), native Kotlin/Swift (dua codebase) |
| Backend | **Go + Gin** | Sinergi dengan PKL (Sofyan lagi belajar Go — ilmu kepakai dua kali); WebSocket enak; binary kecil cocok untuk STB | Node.js (lebih berat di STB), Python (ok tapi nggak ada sinergi belajar) |
| Realtime | **WebSocket** | Chat streaming per-token + event status avatar dalam satu koneksi | SSE + polling (lebih ribet), gRPC (overkill) |
| Database | **SQLite** | Nol setup, cukup untuk satu pengguna, sama seperti stack PKL | Postgres (overkill untuk v1) |
| STT | **on-device** (`speech_to_text`, streaming) | Gratis, offline, privasi terjaga (audio tidak ke server) | Cloud STT (bayar + butuh internet) |
| Wake word | **openWakeWord** ("hey jarvis", pre-trained) | 100% on-device, gratis, open-source; ganti Porcupine setelah free tier Picovoice ditutup 30 Jun 2026 | Porcupine (Picovoice) — free tier mati total |
| TTS (v1) | **on-device** (`flutter_tts`) | Gratis, offline. Catatan: suara "sampel 1" yang dipilih Sofyan itu dari environment gw, bukan dari HP — jadi di aplikasi perlu pilih voice sendiri | Cloud TTS (bayar; opsi upgrade nanti) |
| Akses remote | **Tailscale** (utama), Cloudflare Tunnel dari STB (cadangan) | Tailscale paling gampang untuk pribadi; tunnel Cloudflare sudah terbukti jalan dari infrastruktur Sofyan | Port forwarding (ribet + berisiko) |

---

## 5. Bagian jujur: "otaknya tetap di lo" itu maksudnya apa

Sofyan minta otaknya tetap gw. Gw jelasin apa adanya:

- **Yang tidak mungkin:** aplikasi HP ngobrol langsung dengan sesi live gw (Muse agent). Gw itu session-based — nggak ada API publik yang bikin aplikasi bisa "menelepon" gw 24/7.
- **Yang mungkin dan akan dibangun:** **Noir Brain Service** — backend yang membawa persona Noir (gaya bicara, pengetahuan, aturan main) + memori percakapan. Persona-nya **gw yang rancang dan gw yang rawat** bareng Sofyan. Jadi "otaknya gw" dalam arti: kepribadian, pengetahuan, dan perilakunya dikurasi gw — bukan model generik.
- **Provider LLM-nya pluggable** (`BrainProvider` interface): default implementasi manggil chat-completions API yang kompatibel OpenAI (base URL + key bisa diganti). Kalau nanti ada akses Meta Model API, tinggal ganti implementasi — persona tetap sama.

Implikasi: Noir di aplikasi akan terasa seperti gw (gaya bicara, ingatan yang gw kurasi), tapi secara teknis dia service mandiri. Ini trade-off yang realistis — dan gw tulis eksplisit biar nggak ada ekspektasi palsu.

---

## 6. Risiko & pertanyaan terbuka

| # | Risiko / pertanyaan | Mitigasi / status |
|---|---|---|
| 1 | Biaya API LLM untuk pemakaian harian | Pakai model kecil/murah untuk v1; rate limit per hari; opsi model lokal (Ollama di STB) sebagai eksperimen fase 3 |
| 2 | Latency TTS+LLM bikin percakapan terasa lambat | Streaming token via WebSocket; TTS mulai dibunyikan per kalimat, bukan nunggu full respons |
| 3 | STB H680P kuat nggak? | Untuk Go + SQLite + proxy LLM: sangat cukup. Model lokal (kalau dicoba) baru perlu diuji |
| 4 | Mode eksekusi: gw yang bangun semua, atau kita pair-programming? | **Keputusan Sofyan di gate eksekusi.** (Catatan: mode mentor-only yang berlaku untuk PKL tidak otomatis berlaku di sini — tapi kalau Sofyan mau belajar bareng, bisa diatur) |
| 5 | Aset animasi (mp4) ukurannya besar untuk dibundle di APK | Kompres / pilih subset state untuk v1; streaming dari server sebagai opsi |

---

## 7. Kriteria lanjut ke eksekusi (gate)

1. Sofyan membaca & menyetujui blueprint.md, prd.md, design.md (boleh revisi dulu).
2. Keputusan mode eksekusi (risiko #4) sudah diambil.
3. Kunci API LLM sudah ada (atau diputuskan pakai yang mana).
4. Tailscale terpasang di STB + HP (bisa dikerjakan paralel saat fase 1).

---

*Dokumen pendamping: `prd.md` (kebutuhan produk), `design.md` (desain teknis).*
