# noir-mobile

Aplikasi Flutter untuk Noir — **satu layar, full voice, nol teks**.

## Jalanin

```bash
flutter pub get
flutter run
```

## Konfigurasi (wajib sebelum dipakai)

**Long-press avatar** untuk buka pengaturan (tanpa teks panjang, sesuai desain):

| Key | Isi | Contoh |
|---|---|---|
| 🌐 | WebSocket ke noir-brain | `ws://100.64.0.5:8080/ws` (IP Tailscale STB) |
| 🎧 | Access key Picovoice (gratis) | dari console.picovoice.ai |
| 🎚️ | Slider kecepatan bicara TTS | 0.5 – 1.5 |

## Android

`android/app/src/main/AndroidManifest.xml` di repo ini adalah **referensi permission**
yang dibutuhkan (`INTERNET`, `RECORD_AUDIO`, `MODIFY_AUDIO_SETTINGS`).
Setelah `flutter create --platforms=android .`, pastikan permission itu ada di
manifest hasil generate.

## Wake word "Hey Noir"

1. Daftar gratis di [Picovoice Console](https://console.picovoice.ai)
2. Latih custom keyword **"Hey Noir"** → download `hey-noir.ppn`
3. Taruh di `assets/hey-noir.ppn`, daftarkan di `pubspec.yaml` → `assets:`
4. Isi `porcupine_key`. Tanpa ini, tap avatar jadi satu-satunya pemicu bicara.

## Aset avatar

`assets/avatar/` berisi video loop per status:

| File | Status |
|---|---|
| `idle.mp4` | Diam / standby |
| `listening.mp4` | Mendengarkan |
| `thinking.mp4` | Berpikir |
| `speaking.mp4` | Berbicara |

Kalau sebuah file belum ada, otomatis fallback ke `idle.mp4`.

## Arsitektur layar

`voice_screen.dart` = state machine `idle → listening → thinking → speaking → idle`:

1. Wake word / tap → `listening` (STT on-device mulai)
2. Diam 0,8 dtk → transkrip dikirim via WebSocket → `thinking`
3. Server stream token → kirim teks lengkap + status `speaking`
4. TTS bacakan **per kalimat** → selesai → `tts_done` → `idle`

v1 half-duplex: mic mati saat Noir bicara. Barge-in dijadwalkan v1.1.
