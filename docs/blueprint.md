# Blueprint — Aplikasi Interaktif Noir

**Status:** Draf perencanaan · **Tanggal:** 2026-10-01
**Pemilik ide:** Sofyan · **Arsitek:** Noir

---

## 1. Visi

Satu karakter AI — **Noir** — yang hidup di tiga wujud:

1. **Aplikasi mobile** (sekarang): avatar + animasi + chat teks/suara, interaktif penuh.
2. **Otak terpusat** (server): persona, memori, dan logika Noir yang konsisten di semua wujud.
3. **Robot fisik** (nanti, kalau ada modal): Arduino/ESP32 — Noir bisa disentuh.

Prinsip: **satu otak, banyak badan.** Apa pun wujudnya — HP, web, robot — yang ngomong tetap Noir yang sama, dengan ingatan yang sama.

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
Fase 0  Perencanaan          ← KITA DI SINI
         blueprint.md · prd.md · design.md → disetujui Sofyan → gate eksekusi
Fase 1  MVP voice-only ala JARVIS (v1.0)
         Wake word "Hey Noir" (tap avatar sebagai fallback) + STT on-device +
         TTS per kalimat. Half-duplex (mic mati saat Noir bicara). Tanpa teks di UI.
Fase 2  Full duplex (v1.1)
         Barge-in (motong omongan Noir), continuous conversation 30 dtk,
         perintah cepat lokal tanpa LLM.
Fase 3  Memori & kepribadian (v2.0)
         Memori jangka panjang (SQLite), Noir ingat percakapan lama, persona berkembang.
Fase 4  Robot fisik (butuh modal)
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
| Wake word | **Porcupine** (Picovoice) | SDK Flutter resmi, 100% on-device, gratis untuk personal; "Hey Noir" dilatih via Picovoice Console | openWakeWord (open-source penuh, tapi butuh porting ke mobile) |
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
