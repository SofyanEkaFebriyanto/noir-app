// Package memory menyimpan riwayat percakapan di SQLite lokal.
// v1: riwayat per sesi tunggal. Fase 3: memori jangka panjang.
package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Entry adalah satu pesan tersimpan.
type Entry struct {
	Role      string
	Content   string
	CreatedAt time.Time
}

// Store membungkus koneksi SQLite.
type Store struct {
	db *sql.DB
}

// Open membuka (atau membuat) database di path. Membuat direktori bila perlu.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("memory: mkdir: %w", err)
		}
	}
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("memory: open: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Append menyimpan satu pesan.
func (s *Store) Append(role, content string) error {
	_, err := s.db.Exec(`INSERT INTO messages (role, content) VALUES (?, ?)`, role, content)
	return err
}

// Recent mengambil n pesan terakhir, urutan kronologis (lama → baru).
func (s *Store) Recent(n int) ([]Entry, error) {
	rows, err := s.db.Query(`SELECT role, content, created_at FROM messages ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Role, &e.Content, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	// balik ke kronologis
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// Close menutup database.
func (s *Store) Close() error { return s.db.Close() }
