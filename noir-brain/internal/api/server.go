// Package api adalah HTTP API + WebSocket server noir-brain.
//
// Protokol WebSocket (JSON):
//   Client → server:
//     {"type":"hello","client":"noir-mobile/1.0"}
//     {"type":"chat","text":"<transkrip STT>"}     <- teks dari STT, bukan ketikan
//     {"type":"tts_done"}                          <- app selesai membacakan
//     {"type":"interrupt"}                         <- user memotong (barge-in)
//     {"type":"history","limit":50}
//   Server → client:
//     {"type":"avatar_state","state":"idle|listening|thinking|speaking","text":"..."}
//     {"type":"token","text":"..."}                <- token LLM streaming
//     {"type":"done"}
//     {"type":"history","messages":[{"role":"...","content":"..."}]}
//     {"type":"error","error":"..."}
//   HTTP API:
//     POST /v1/chat/completions  <- kompatibel OpenAI (lihat openai.go)
//     GET  /v1/models
package api

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/agent"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/config"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/memory"
)

// webFS adalah web UI noir (di-embed ke binary, diserve di "/").
//
//go:embed web
var webFS embed.FS

// Deps adalah dependensi server.
type Deps struct {
	Config       config.Config
	Provider     brain.Provider
	Store        *memory.Store
	SystemPrompt string
	Agent        *agent.Agent // nil = mode chat biasa (tanpa tools)
}

// Event adalah satu pesan WebSocket.
type Event struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	State    string         `json:"state,omitempty"`
	Client   string         `json:"client,omitempty"`
	Error    string         `json:"error,omitempty"`
	Limit    int            `json:"limit,omitempty"`
	Messages []brain.Message `json:"messages,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // LAN/Tailscale, tanpa browser
}

// Server adalah HTTP + WS server.
type Server struct {
	deps Deps
}

// New membuat Server.
func New(deps Deps) *Server { return &Server{deps: deps} }

// Run menjalankan server di addr (mis. ":8080").
func (s *Server) Run(addr string) error {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "noir-brain"})
	})
	r.GET("/ws", s.handleWS)
	s.registerOpenAI(r)
	s.registerPersona(r)
	// Web UI — via NoRoute supaya route API/WS tetap menang.
	webSub, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	r.NoRoute(gin.WrapH(http.FileServer(http.FS(webSub))))
	return r.Run(addr)
}

// clientConn membungkus satu koneksi WebSocket + pembatalan stream aktif.
type clientConn struct {
	ws     *websocket.Conn
	mu     sync.Mutex // serialisasi write
	cancel context.CancelFunc
}

func (c *clientConn) send(ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.ws.WriteJSON(ev)
}

// cancelStream membatalkan stream LLM yang sedang jalan di koneksi ini.
func (s *Server) cancelStream(conn *clientConn) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if conn.cancel != nil {
		conn.cancel()
		conn.cancel = nil
	}
}

func (s *Server) handleWS(c *gin.Context) {
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	defer ws.Close()

	conn := &clientConn{ws: ws}
	log.Printf("ws: client terhubung dari %s", ws.RemoteAddr())

	for {
		var ev Event
		if err := ws.ReadJSON(&ev); err != nil {
			break // client pergi
		}
		switch ev.Type {
		case "hello":
			conn.send(Event{Type: "avatar_state", State: "idle"})
		case "chat":
			s.handleChat(conn, strings.TrimSpace(ev.Text))
		case "tts_done":
			conn.send(Event{Type: "avatar_state", State: "idle"})
		case "interrupt":
			// Barge-in: user memotong → batalkan stream, kembali idle.
			s.cancelStream(conn)
			conn.send(Event{Type: "avatar_state", State: "idle"})
		case "history":
			s.handleHistory(conn, ev.Limit)
		}
	}

	s.cancelStream(conn)
	log.Printf("ws: client %s pergi", ws.RemoteAddr())
}

// handleChat memproses satu transkrip suara: simpan → thinking → stream → speaking.
func (s *Server) handleChat(conn *clientConn, text string) {
	if text == "" {
		return
	}

	// Batalkan stream sebelumnya kalau user ngomong lagi (half-duplex guard).
	s.cancelStream(conn)
	ctx, cancel := context.WithCancel(context.Background())
	conn.mu.Lock()
	conn.cancel = cancel
	conn.mu.Unlock()

	if err := s.deps.Store.Append("user", text); err != nil {
		log.Printf("memory append user: %v", err)
	}

	hist, err := s.deps.Store.Recent(20)
	if err != nil {
		log.Printf("memory recent: %v", err)
	}

	messages := make([]brain.Message, 0, len(hist)+2)
	messages = append(messages, brain.Message{Role: "system", Content: s.systemPrompt()})
	for _, h := range hist {
		messages = append(messages, brain.Message{Role: h.Role, Content: h.Content})
	}

	conn.send(Event{Type: "avatar_state", State: "thinking"})

	var answer string
	if s.deps.Agent != nil {
		answer = s.handleAgentChat(ctx, conn, messages)
	} else {
		answer = s.handleStreamChat(ctx, conn, messages)
	}

	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = "Hmm, kosong. Coba ngomong lagi?"
	}
	if err := s.deps.Store.Append("assistant", answer); err != nil {
		log.Printf("memory append assistant: %v", err)
	}

	// F-12: ekstraksi fakta jangka panjang, async (tidak memblokir suara).
	if ap, ok := s.deps.Provider.(brain.AgentProvider); ok {
		conv := append(append([]memory.Entry{}, hist...),
			memory.Entry{Role: "user", Content: text},
			memory.Entry{Role: "assistant", Content: answer})
		go func() {
			for _, f := range memory.ExtractFacts(context.Background(), ap, conv) {
				if err := s.deps.Store.SaveFact(f); err != nil {
					log.Printf("memory save fact: %v", err)
				}
			}
		}()
	}

	conn.send(Event{Type: "done"})
	// App yang membacakan via TTS per kalimat, lalu kirim tts_done.
	conn.send(Event{Type: "avatar_state", State: "speaking", Text: answer})
}

// baseSystemPrompt: persona override (F-13) > env SYSTEM_PROMPT > default.
// Dipakai endpoint /v1 (stateless terhadap memory).
func (s *Server) baseSystemPrompt() string {
	if v, err := s.deps.Store.GetSetting("persona_override"); err == nil && strings.TrimSpace(v) != "" {
		return v
	}
	if s.deps.Config.SystemPrompt != "" {
		return s.deps.Config.SystemPrompt
	}
	return s.deps.SystemPrompt
}

// systemPrompt menyusun system prompt efektif untuk voice WS:
// base + blok fakta jangka panjang (F-12).
func (s *Server) systemPrompt() string {
	system := s.baseSystemPrompt()
	if facts, err := s.deps.Store.Facts(30); err == nil {
		system += memory.FactsBlock(facts)
	}
	return system
}

// handleAgentChat menjalankan agent loop (LLM + tools). Mengembalikan jawaban
// final, atau "" bila dibatalkan. Progress tool dikirim sebagai status thinking.
func (s *Server) handleAgentChat(ctx context.Context, conn *clientConn, messages []brain.Message) string {
	answer, err := s.deps.Agent.Run(ctx, messages, func(step int, toolName string) {
		conn.send(Event{Type: "avatar_state", State: "thinking", Text: toolProgressText(toolName)})
	})
	if err != nil {
		if ctx.Err() != nil {
			return "" // dibatalkan: user mulai bicara lagi / disconnect
		}
		log.Printf("agent: %v", err)
		return "Maaf, otaknya lagi error. Coba ulangi?"
	}
	return answer
}

func toolProgressText(name string) string {
	switch name {
	case "exec":
		return "jalanin perintah…"
	case "read_file", "list_dir":
		return "baca file…"
	case "write_file":
		return "nulis file…"
	case "service_status":
		return "cek service…"
	case "service_restart":
		return "restart service…"
	case "sysinfo":
		return "cek sistem…"
	case "waktu":
		return "cek jam…"
	default:
		return "kerja…"
	}
}

// handleStreamChat adalah jalur chat biasa (streaming token, tanpa tools).
// Mengembalikan jawaban final, atau "" bila dibatalkan.
func (s *Server) handleStreamChat(ctx context.Context, conn *clientConn, messages []brain.Message) string {
	tokens, errc := s.deps.Provider.StreamChat(ctx, messages)

	var full strings.Builder
streamLoop:
	for {
		select {
		case <-ctx.Done():
			return "" // dibatalkan: user mulai bicara lagi / disconnect
		case tok, ok := <-tokens:
			if !ok {
				break streamLoop
			}
			full.WriteString(tok)
			conn.send(Event{Type: "token", Text: tok})
		case err := <-errc:
			if err != nil {
				log.Printf("llm stream: %v", err)
				// Fallback suara: app akan membacakan ini via TTS.
				return "Maaf, otaknya lagi error. Coba ulangi?"
			}
		}
	}

	return full.String()
}

// handleHistory mengembalikan riwayat percakapan.
func (s *Server) handleHistory(conn *clientConn, limit int) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	hist, err := s.deps.Store.Recent(limit)
	if err != nil {
		conn.send(Event{Type: "error", Error: "gagal baca memori"})
		return
	}
	msgs := make([]brain.Message, 0, len(hist))
	for _, h := range hist {
		msgs = append(msgs, brain.Message{Role: h.Role, Content: h.Content})
	}
	conn.send(Event{Type: "history", Messages: msgs})
}
