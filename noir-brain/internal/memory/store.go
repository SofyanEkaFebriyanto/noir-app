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
	// F-14: sapaan proaktif (in-app only).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS proactive_pings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		delivered_at DATETIME
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: schema pings: %w", err)
	}
	// F-16: catatan harian otomatis (ala Hermes daily note).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS daily_notes (
		date TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: schema daily_notes: %w", err)
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

// ---------- F-14: proactive pings ----------

// Ping adalah satu sapaan proaktif.
type Ping struct {
	ID        int64
	Message   string
	CreatedAt time.Time
}

// SavePing menyimpan sapaan proaktif baru, mengembalikan ID-nya.
func (s *Store) SavePing(message string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO proactive_pings (message) VALUES (?)`, message)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PendingPings mengambil sapaan yang belum terkirim ke client (maks 5).
func (s *Store) PendingPings() ([]Ping, error) {
	rows, err := s.db.Query(`SELECT id, message, created_at FROM proactive_pings
		WHERE delivered_at IS NULL ORDER BY id ASC LIMIT 5`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ping
	for rows.Next() {
		var p Ping
		if err := rows.Scan(&p.ID, &p.Message, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// MarkPingsDelivered menandai sapaan sudah terkirim.
func (s *Store) MarkPingsDelivered(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	q := `UPDATE proactive_pings SET delivered_at = CURRENT_TIMESTAMP WHERE id IN (`
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			q += ","
		}
		q += "?"
		args[i] = id
	}
	q += `)`
	_, err := s.db.Exec(q, args...)
	return err
}

// LastPing mengambil sapaan terakhir (untuk anti-spam/dedupe).
func (s *Store) LastPing() (Ping, error) {
	var p Ping
	err := s.db.QueryRow(`SELECT id, message, created_at FROM proactive_pings
		ORDER BY id DESC LIMIT 1`).Scan(&p.ID, &p.Message, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return Ping{}, nil
	}
	return p, err
}

// ---------- F-16: konsolidasi + daily note ----------

// FactFull adalah fakta beserta tanggal dibuatnya.
type FactFull struct {
	Fact      string
	CreatedAt time.Time
}

// FactsFull mengambil semua fakta + tanggal, urutan kronologis.
func (s *Store) FactsFull() ([]FactFull, error) {
	rows, err := s.db.Query(`SELECT fact, created_at FROM facts ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FactFull
	for rows.Next() {
		var f FactFull
		if err := rows.Scan(&f.Fact, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FactCount menghitung jumlah fakta tersimpan.
func (s *Store) FactCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM facts`).Scan(&n)
	return n, err
}

// ReplaceFacts mengganti seluruh tabel facts dengan daftar baru (hasil konsolidasi).
func (s *Store) ReplaceFacts(facts []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM facts`); err != nil {
		return err
	}
	for _, f := range facts {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO facts (fact) VALUES (?)`, f); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MessagesBetween mengambil pesan dalam rentang [start, end), kronologis.
// created_at SQLite = UTC (CURRENT_TIMESTAMP), jadi bounds dikonversi ke UTC.
func (s *Store) MessagesBetween(start, end time.Time) ([]Entry, error) {
	rows, err := s.db.Query(`SELECT role, content, created_at FROM messages
		WHERE datetime(created_at) >= datetime(?) AND datetime(created_at) < datetime(?)
		ORDER BY id ASC`, start.UTC().Format("2006-01-02 15:04:05"), end.UTC().Format("2006-01-02 15:04:05"))
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
	return out, rows.Err()
}

// FactsBetween mengambil fakta yang dibuat dalam rentang [start, end).
// created_at SQLite = UTC (CURRENT_TIMESTAMP), jadi bounds dikonversi ke UTC.
func (s *Store) FactsBetween(start, end time.Time) ([]string, error) {
	rows, err := s.db.Query(`SELECT fact FROM facts
		WHERE datetime(created_at) >= datetime(?) AND datetime(created_at) < datetime(?)
		ORDER BY id ASC`, start.UTC().Format("2006-01-02 15:04:05"), end.UTC().Format("2006-01-02 15:04:05"))
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
	return out, rows.Err()
}

// DailyNote adalah catatan harian.
type DailyNote struct {
	Date      string
	Content   string
	CreatedAt time.Time
}

// SaveDailyNote menyimpan/menimpa catatan untuk tanggal (format "2006-01-02").
func (s *Store) SaveDailyNote(date, content string) error {
	_, err := s.db.Exec(`INSERT INTO daily_notes (date, content) VALUES (?, ?)
		ON CONFLICT(date) DO UPDATE SET content = excluded.content`, date, content)
	return err
}

// GetDailyNote mengambil catatan tanggal; "", nil bila belum ada.
func (s *Store) GetDailyNote(date string) (string, error) {
	var c string
	err := s.db.QueryRow(`SELECT content FROM daily_notes WHERE date = ?`, date).Scan(&c)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return c, err
}

// ListDailyNotes mengambil semua catatan harian, urutan tanggal menanjak.
func (s *Store) ListDailyNotes() ([]DailyNote, error) {
	rows, err := s.db.Query(`SELECT date, content, created_at FROM daily_notes ORDER BY date ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyNote
	for rows.Next() {
		var d DailyNote
		if err := rows.Scan(&d.Date, &d.Content, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
