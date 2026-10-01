# noir-brain

Backend Go untuk aplikasi Noir — "satu otak" yang dipakai aplikasi mobile sekarang dan robot fisik nanti.

## Jalanin

```bash
cp .env.example .env   # isi LLM_API_KEY
go mod tidy
go run ./cmd/server    # http://localhost:8080
```

Cek: `curl localhost:8080/health` → `{"status":"ok","service":"noir-brain"}`

## API

| Endpoint | Deskripsi |
|---|---|
| `GET /health` | Health check |
| `GET /ws` | WebSocket (protokol di `internal/api/server.go`) |

## Deploy di STB (Armbian)

```bash
# build sekali di STB (atau cross-compile dari laptop)
GOOS=linux GOARCH=arm64 go build -o noir-brain ./cmd/server

# jalanin persisten via systemd
sudo cp deploy/noir-brain.service /etc/systemd/system/
sudo mkdir -p /opt/noir-brain && sudo cp noir-brain .env /opt/noir-brain/
sudo useradd -r -s /usr/sbin/nologin noir  # kalau belum ada
sudo chown -R noir:noir /opt/noir-brain
sudo systemctl enable --now noir-brain
```

HP terhubung via Tailscale ke IP STB, mis. `ws://100.x.y.z:8080/ws`.
Isi URL itu via long-press avatar → pengaturan di aplikasi.

## Ganti provider LLM

`internal/brain` adalah interface. Bikin struct baru yang implement `Provider`,
lalu ganti `brain.NewOpenAICompat(...)` di `cmd/server/main.go`. Yang lain nggak berubah.
