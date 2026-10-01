# PRD — Aplikasi Interaktif Noir

**Status:** Draf · **Tanggal:** 2026-10-01 · **Versi target:** v1.0 (MVP voice-only ala JARVIS)

> **Pivot 2026-10-01:** atas permintaan Sofyan, aplikasi menjadi **full voice-to-voice tanpa teks sama sekali di UI** — seperti JARVIS di Iron Man. Chat teks dihapus dari scope; STT yang tadinya fase 2 naik ke v1.0.

---

## 1. Latar belakang

Sofyan punya karakter AI pribadi — **Noir** — yang selama ini hidup di chat Muse: punya avatar (dari character sheet buatannya sendiri), animasi, dan suara TTS. PRD ini mendefinisikan langkah menjadikannya **aplikasi mandiri ala JARVIS**: full voice-to-voice, tanpa teks sama sekali di UI. Pengguna memanggil "Hey Noir", Noir mendengarkan, berpikir, dan menjawab dengan suara — avatar beranimasi mengikuti status. Satu otak terpusat yang nantinya juga dipakai robot fisik.

---

## 2. Pengguna

| Pengguna | Kebutuhan |
|---|---|
| **Sofyan** (pengguna utama & satu-satunya di v1) | Ngobrol dengan Noir kapan saja; Noir ingat konteks; tampilannya enak dilihat |
| **Noir (operator)** | Persona & memori bisa dikurasi; perilaku konsisten di semua wujud |

v1 single-user. Multi-user / login / akun = out of scope.

---

## 3. Tujuan produk

1. Noir bisa diajak ngobrol dari HP Android seperti JARVIS: full voice-to-voice, **tanpa teks di UI**. Avatar bereaksi (idle → listening → thinking → speaking) + indikator suara.
2. Satu backend ("otak") yang dipakai aplikasi sekarang dan robot fisik nanti.
3. Fondasi memori: Noir ingat percakapan sebelumnya (fase 3; v1 menyimpan riwayat di server walau tidak ditampilkan).

---

## 4. Scope MVP — v1.0

### 4.1 Fitur fungsional

| ID | Fitur | Deskripsi | Prioritas |
|---|---|---|---|
| F-01 | Layar utama avatar | Avatar Noir full-body + animasi loop sesuai status. **Nol teks di UI**: tidak ada bubble chat, tidak ada input teks, tidak ada riwayat visual | Must |
| F-02 | Status avatar + indikator suara | State machine: `idle` → `listening` → `thinking` → `speaking` → `idle`; tiap status ada animasinya + cincin waveform saat listening/speaking | Must |
| F-03 | Wake word | "Hey Noir" via Porcupine (on-device). Fallback: tap avatar untuk mulai bicara | Must |
| F-04 | Voice input (STT) | STT on-device streaming; endpointing otomatis (dianggap selesai saat pengguna diam ~0,8 dtk) → transkrip dikirim ke server | Must |
| F-05 | Respons suara (TTS) | Jawaban dibacakan **per kalimat** (streaming, tidak nunggu teks lengkap); status `speaking` sinkron dengan audio | Must |
| F-06 | Indikator koneksi | Visual tanpa teks: titik hijau = terhubung, merah = offline; tap untuk retry | Must |
| F-07 | Pengaturan (visual minimal) | Voice TTS on-device, kecepatan bicara, sensitivitas mic, varian avatar — tanpa teks panjang, pakai ikon + slider | Should |
| F-08 | Timeout & fallback suara | STT tidak menangkap suara 10 dtk → kembali idle. LLM timeout → Noir bilang "maaf, coba ulangi?" via TTS | Must |

### 4.2 Alur utama (happy path)

1. Buka app → avatar idle (animasi loop santai), titik hijau = terhubung.
2. "Hey Noir" (atau tap avatar) → status `listening`, cincin waveform menyala.
3. Pengguna ngomong → diam 0,8 dtk → status `thinking` → kalimat pertama jawaban mulai dibacakan (`speaking`) sementara sisanya masih di-generate → selesai → kembali `idle`.
4. Backend mati → titik merah + avatar meredup; tap untuk retry.

---

## 5. Fase berikutnya (bukan MVP)

**v1.1 — Full duplex (JARVIS beneran)** ✅ diimplementasi 2026-10-01
- F-09: Barge-in — **tap avatar** saat Noir bicara = interupsi utama (andal, tanpa AEC).
  Barge-in **suara** tersedia sebagai eksperimen (default mati, toggle di pengaturan):
  STT tetap jalan saat speaking, heuristik membedakan suara Noir sendiri
  (parsial cocok dengan teks dibacakan → abaikan) vs suara pengguna
  (2 parsial beruntun tidak cocok → interupsi). Jujur: tanpa echo cancellation
  hardware, barge-in suara tidak 100% andal.
- F-10: Continuous conversation — 30 dtk setelah Noir selesai bicara, langsung
  dengarkan lagi tanpa wake word ulang.
- F-11: Perintah cepat lokal tanpa LLM: "diam"/"stop"/"berhenti" hentikan total,
  "ulangi" baca ulang jawaban terakhir.
- F-08 diperketat: diam 12 dtk saat listening → kembali idle.

**v2.0 — Memori & kepribadian**
- F-12: Memori jangka panjang — Noir ingat fakta & preferensi antar sesi.
- F-13: Halaman "Tentang Noir" — lihat/edit persona (yang dikurasi bareng).
- F-14: Proactive ping — Noir nyapa duluan kalau ada hal relevan (butuh persetujuan pola notifikasi).

**Fase robot (butuh modal)**
- F-15: ESP32 + LCD menampilkan Noir pixel-art + status yang sama dengan app.
- F-16: Speaker + servo kepala; perintah suara sederhana.
- F-17: MQTT bridge — robot dan app ngobrol dengan otak yang sama.

---

## 6. Non-functional requirements

| ID | Aspek | Target v1 |
|---|---|---|
| N-01 | Latency suara pertama | Audio kalimat pertama keluar < 2 dtk setelah pengguna selesai bicara (di LAN) |
| N-02 | Ketersediaan | Backend jalan 24/7 di STB; app kasih tahu jelas saat offline |
| N-03 | Privasi | Chat tersimpan lokal di STB (SQLite). **Audio tidak pernah dikirim ke server** — STT on-device, yang dikirim hanya transkrip. Tidak ada analytics pihak ketiga. Kunci API LLM di server, tidak di app |
| N-04 | Ukuran APK | < 100 MB (aset video dikompres / dipilih subset) |
| N-05 | Baterai | WebSocket idle hemat; animasi pause saat app di-background; wake word on-device hemat daya |
| N-06 | Bahasa | Indonesia (primer), Inggris (sekunder) |
| N-07 | Robustness suara | Tidak salah dengar parah di ruangan normal; endpointing tidak kepotong saat pengguna jeda mikir (< 0,8 dtk) |

---

## 7. Metrik sukses (v1.0)

- App terinstal di HP Sofyan dan dipakai ngobrol ≥ 5 sesi dalam seminggu pertama.
- 0 crash saat demo fase 1.
- Latency token pertama memenuhi N-01 dalam 90% percobaan LAN.
- Sofyan bilang "ini berasa Noir" (ya, metriknya subjektif — dan itu yang penting).

---

## 8. Out of scope (v1)

- Login / multi-user / sinkronisasi antar perangkat.
- **UI teks apa pun** (bubble chat, input teks, riwayat chat visual) — by design, bukan keterbatasan.
- iOS (menyusul setelah Android stabil).
- Robot fisik (fase tersendiri, butuh modal).
- Publish ke Play Store (distribusi via APK langsung).
- Proactive notification / background service.

---

## 9. Asumsi & dependensi

1. STB H680P online 24/7 dan bisa dijangkau HP via LAN/Tailscale.
2. Ada kunci API LLM (atau keputusan provider) sebelum eksekusi fase 1.
3. Aset avatar final (webp + mp4 per status) tersedia — sebagian sudah ada dari sesi avatar.
4. Mode eksekusi (Noir bangun semua vs. pair-programming) diputuskan di gate.

---

*Dokumen pendamping: `blueprint.md` (visi & fase), `design.md` (desain teknis).*
