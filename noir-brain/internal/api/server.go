// Package api adalah HTTP API + WebSocket server noir-brain.
//
// Protokol WebSocket (JSON):
//   Client → server:
//     {"type":"hello","client":"noir-mobile/1.0"}
//     {"type":"chat","text":"<transkrip STT>"}     <- teks dari STT, bukan ketikan
//     {"type":"tts_done"}                          <- app selesai membacakan
//     {"type":"history","limit":50}
//   Server → client:
//     {"type":"avatar_state","state":"idle|listening|thinking|speaking","text":"..."}
//     {"type":"token","text":"..."}                <- token LLM streaming
//     {"type":"done"}
//     {"type":"history","messages":[{"role":"...","content":"..."}]}
//     {"type":"error","error":"..."}
package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/brain"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/config"
	"github.com/SofyanEkaFebriyanto/noir-app/noir-brain/internal/memory"
)

// Deps adalah dependensi server.
type Deps struct {
	Config       config.Config
	Provider     brain.Provider
	Store        *memory.Store
	SystemPrompt string
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
		case "history":
			s.handleHistory(conn, ev.Limit)
		}
	}

	conn.mu.Lock()
	if conn.cancel != nil {
		conn.cancel()
	}
	conn.mu.Unlock()
	log.Printf("ws: client %s pergi", ws.RemoteAddr())
}

// handleChat memproses satu transkrip suara: simpan → thinking → stream → speaking.
func (s *Server) handleChat(conn *clientConn, text string) {
	if text == "" {
		return
	}

	// Batalkan stream sebelumnya kalau user ngomong lagi (half-duplex guard).
	conn.mu.Lock()
	if conn.cancel != nil {
		conn.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
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
	system := s.deps.SystemPrompt
	if s.deps.Config.SystemPrompt != "" {
		system = s.deps.Config.SystemPrompt
	}
	messages = append(messages, brain.Message{Role: "system", Content: system})
	for _, h := range hist {
		messages = append(messages, brain.Message{Role: h.Role, Content: h.Content})
	}

	conn.send(Event{Type: "avatar_state", State: "thinking"})

	tokens, errc := s.deps.Provider.StreamChat(ctx, messages)

	var full strings.Builder
streamLoop:
	for {
		select {
		case <-ctx.Done():
			return // dibatalkan: user mulai bicara lagi / disconnect
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
				conn.send(Event{Type: "avatar_state", State: "speaking", Text: "Maaf, otaknya lagi error. Coba ulangi?"})
				return
			}
		}
	}

	answer := strings.TrimSpace(full.String())
	if answer == "" {
		answer = "Hmm, kosong. Coba ngomong lagi?"
	}
	if err := s.deps.Store.Append("assistant", answer); err != nil {
		log.Printf("memory append assistant: %v", err)
	}

	conn.send(Event{Type: "done"})
	// App yang membacakan via TTS per kalimat, lalu kirim tts_done.
	conn.send(Event{Type: "avatar_state", State: "speaking", Text: answer})
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
