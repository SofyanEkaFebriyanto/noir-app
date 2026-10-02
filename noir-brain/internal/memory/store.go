// Package memory menyimpan riwayat percakapan di SQLite lokal.
// v1: riwayat per sesi tunggal. Fase 3: memori jangka panjang.
package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// F-12: fakta jangka panjang tentang pengguna (hasil ekstraksi LLM).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS facts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		fact TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: schema facts: %w", err)
	}
	// F-13: pengaturan (mis. persona override).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: schema settings: %w", err)
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

// ---------- F-12: fakta jangka panjang ----------

// SaveFact menyimpan satu fakta (dedupe case-insensitive). Maks 200 fakta,
// yang terlama dihapus bila melebihi.
func (s *Store) SaveFact(fact string) error {
	fact = strings.TrimSpace(fact)
	if fact == "" {
		return nil
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM facts WHERE LOWER(fact) = LOWER(?)`, fact).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil // sudah ada
	}
	if _, err := s.db.Exec(`INSERT INTO facts (fact) VALUES (?)`, fact); err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM facts WHERE id NOT IN (SELECT id FROM facts ORDER BY id DESC LIMIT 200)`)
	return err
}

// Facts mengambil hingga n fakta terbaru, urutan kronologis.
func (s *Store) Facts(n int) ([]string, error) {
	rows, err := s.db.Query(`SELECT fact FROM facts ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// ---------- F-13: settings ----------

// GetSetting membaca pengaturan; "" bila tidak ada.
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetSetting menyimpan pengaturan.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// DelSetting menghapus pengaturan.
func (s *Store) DelSetting(key string) error {
	_, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	return err
}
