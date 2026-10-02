// Toolset v2 noir-brain (F-15): tool ala opencode.
//
//   - edit_file: edit bedah per string (bukan rewrite full).
//   - grep: cari pola regex di file/direktori.
//   - glob: cari file pakai pola glob (** didukung).
//   - webfetch: ambil URL -> teks (http/https saja, timeout + batas ukuran).
//   - todo_write: catat & lacak daftar kerja multi-step.
//
// Safety mengikuti pola v1: cleanPath + checkWritable untuk file,
// batas output, skip file kredensial & biner.
package agent

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxGrepHits  = 50
	maxGlobHits  = 100
	maxFetchSize = 32768 // 32KB
	fetchTimeout = 15 * time.Second
)

// ---------- edit_file ----------

func toolEditFile() ToolDef {
	return ToolDef{
		Name: "edit_file",
		Description: "Edit bedah isi file: ganti old_string menjadi new_string. " +
			"Gagal kalau old_string tidak ketemu, atau ketemu >1 kali tanpa replace_all=true. " +
			"Aturan tulis sama seperti write_file (direktori sistem & kredensial dilarang).",
		Parameters: objSchema(map[string]any{
			"path":        strProp("path file"),
			"old_string":  strProp("teks yang mau diganti (harus persis sama)"),
			"new_string":  strProp("teks pengganti"),
			"replace_all": map[string]any{"type": "boolean", "description": "ganti semua kemunculan (default false)"},
		}, "path", "old_string", "new_string"),
		Exec: func(ctx context.Context, argsJSON string) (string, error) {
			var a struct {
				Path       string `json:"path"`
				Old        string `json:"old_string"`
				New        string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if err := decodeArgs(argsJSON, &a); err != nil {
				return "", err
			}
			if a.Old == "" {
				return "", fmt.Errorf("old_string kosong")
			}
			abs, err := cleanPath(a.Path)
			if err != nil {
				return "", err
			}
			if err := checkWritable(abs); err != nil {
				return "", err
			}
			raw, err := os.ReadFile(abs)
			if err != nil {
				return "", err
			}
			content := string(raw)
			n := strings.Count(content, a.Old)
			if n == 0 {
				return "", fmt.Errorf("old_string tidak ketemu di %s", abs)
			}
			if n > 1 && !a.ReplaceAll {
				return "", fmt.Errorf("old_string ketemu %d kali; persempit atau pakai replace_all=true", n)
			}
			var updated string
			if a.ReplaceAll {
				updated = strings.ReplaceAll(content, a.Old, a.New)
			} else {
				updated = strings.Replace(content, a.Old, a.New, 1)
			}
			if err := os.WriteFile(abs, []byte(updated), 0644); err != nil {
				return "", err
			}
			return fmt.Sprintf("ok, diganti %d kemunculan di %s", n, abs), nil
		},
	}
}

// ---------- grep ----------

var skipDirNames = map[string]bool{
	".git": true, "node_modules": true, ".cache": true, "__pycache__": true,
}

func secretPathLower(lower string) bool {
	for _, s := range secretSubstr {
		if strings.Contains(lower, strings.ToLower(s)) {
			return true
		}
	}
	return false
}

func toolGrep() ToolDef {
	return ToolDef{
		Name: "grep",
		Description: "Cari pola regex di file atau direktori. Hasil format file:baris: isi (maks 50). " +
			"Direktori .git/node_modules/.cache dilewati; file kredensial & biner dilewati.",
		Parameters: objSchema(map[string]any{
			"pattern": strProp("pola regex, mis. \"func.*Chat\""),
			"path":    strProp("file atau direktori awal pencarian"),
			"include": strProp("opsional: filter glob nama file, mis. \"*.go\""),
		}, "pattern", "path"),
		Exec: func(ctx context.Context, argsJSON string) (string, error) {
			var a struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
				Include string `json:"include"`
			}
			if err := decodeArgs(argsJSON, &a); err != nil {
				return "", err
			}
			re, err := regexp.Compile(a.Pattern)
			if err != nil {
				return "", fmt.Errorf("regex tidak valid: %v", err)
			}
			abs, err := cleanPath(a.Path)
			if err != nil {
				return "", err
			}
			var files []string
			fi, err := os.Stat(abs)
			if err != nil {
				return "", err
			}
			if !fi.IsDir() {
				files = []string{abs}
			} else {
				count := 0
				walkErr := filepath.Walk(abs, func(p string, info os.FileInfo, err error) error {
					if err != nil || count > 2000 {
						return nil
					}
					if info.IsDir() {
						if skipDirNames[info.Name()] {
							return filepath.SkipDir
						}
						return nil
					}
					if secretPathLower(strings.ToLower(p)) {
						return nil
					}
					if a.Include != "" {
						ok, _ := filepath.Match(a.Include, info.Name())
						if !ok {
							return nil
						}
					}
					count++
					files = append(files, p)
					return nil
				})
				if walkErr != nil {
					return "", walkErr
				}
			}
			var b strings.Builder
			hits := 0
			for _, f := range files {
				if hits >= maxGrepHits {
					break
				}
				raw, err := os.ReadFile(f)
				if err != nil || len(raw) > 2*1024*1024 {
					continue
				}
				if strings.ContainsRune(string(raw[:min(len(raw), 8000)]), 0) {
					continue // biner
				}
				for i, line := range strings.Split(string(raw), "\n") {
					if re.MatchString(line) {
						fmt.Fprintf(&b, "%s:%d: %s\n", f, i+1, trunc(strings.TrimSpace(line), 200))
						hits++
						if hits >= maxGrepHits {
							break
						}
					}
				}
			}
			if hits == 0 {
				return "(tidak ketemu)", nil
			}
			out := b.String()
			if hits >= maxGrepHits {
				out += "…(dipotong, maks 50 hasil)\n"
			}
			return out, nil
		},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- glob ----------

// globToRegex mengubah pola glob (*, **, ?) menjadi regex.
func globToRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	i := 0
	for i < len(pattern) {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i += 2
				if i < len(pattern) && pattern[i] == '/' {
					i++
				}
			} else {
				b.WriteString("[^/]*")
				i++
			}
		case '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func toolGlob() ToolDef {
	return ToolDef{
		Name: "glob",
		Description: "Cari file pakai pola glob, mis. \"**/*.go\" atau \"*.md\". " +
			"Basis pencarian = path (default direktori kerja). Maks 100 hasil.",
		Parameters: objSchema(map[string]any{
			"pattern": strProp("pola glob, mis. \"**/*.go\""),
			"path":    strProp("opsional: direktori basis"),
		}, "pattern"),
		Exec: func(ctx context.Context, argsJSON string) (string, error) {
			var a struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
			}
			if err := decodeArgs(argsJSON, &a); err != nil {
				return "", err
			}
			base := a.Path
			if base == "" {
				base = "."
			}
			abs, err := cleanPath(base)
			if err != nil {
				return "", err
			}
			re, err := globToRegex(a.Pattern)
			if err != nil {
				return "", fmt.Errorf("pola tidak valid: %v", err)
			}
			var b strings.Builder
			hits := 0
			_ = filepath.Walk(abs, func(p string, info os.FileInfo, err error) error {
				if err != nil || hits >= maxGlobHits {
					return nil
				}
				if info.IsDir() {
					if skipDirNames[info.Name()] {
						return filepath.SkipDir
					}
					return nil
				}
				rel, err := filepath.Rel(abs, p)
				if err != nil {
					return nil
				}
				if re.MatchString(filepath.ToSlash(rel)) {
					b.WriteString(p + "\n")
					hits++
				}
				return nil
			})
			if hits == 0 {
				return "(tidak ketemu)", nil
			}
			return b.String(), nil
		},
	}
}

// ---------- webfetch ----------

var tagRe = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<!--.*?-->|<[^>]+>`)
var spaceRe = regexp.MustCompile(`[ \t]+`)

func htmlToText(raw string) string {
	t := tagRe.ReplaceAllString(raw, " ")
	t = html.UnescapeString(t)
	t = spaceRe.ReplaceAllString(t, " ")
	var lines []string
	for _, l := range strings.Split(t, "\n") {
		if s := strings.TrimSpace(l); s != "" {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}

func toolWebfetch() ToolDef {
	return ToolDef{
		Name: "webfetch",
		Description: "Ambil URL (http/https) dan kembalikan sebagai teks (tag HTML dibuang). " +
			"Timeout 15 detik, maks ~32KB. Hanya untuk baca; bukan untuk unduh file.",
		Parameters: objSchema(map[string]any{
			"url": strProp("URL http/https, mis. \"https://example.com\""),
		}, "url"),
		Exec: func(ctx context.Context, argsJSON string) (string, error) {
			var a struct {
				URL string `json:"url"`
			}
			if err := decodeArgs(argsJSON, &a); err != nil {
				return "", err
			}
			u := strings.TrimSpace(a.URL)
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				return "", fmt.Errorf("hanya http/https yang diizinkan")
			}
			tctx, cancel := context.WithTimeout(ctx, fetchTimeout)
			defer cancel()
			req, err := http.NewRequestWithContext(tctx, "GET", u, nil)
			if err != nil {
				return "", err
			}
			req.Header.Set("User-Agent", "noir-brain/1.0")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("fetch gagal: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return "", fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
			if err != nil {
				return "", err
			}
			text := htmlToText(string(body))
			if text == "" {
				return "(kosong / bukan teks)", nil
			}
			return trunc(text, 8192), nil
		},
	}
}

// ---------- todo_write ----------

type todoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

type todoList struct {
	mu    sync.Mutex
	items []todoItem
}

func (t *todoList) set(items []todoItem) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items = items
	return t.renderLocked()
}

func (t *todoList) renderLocked() string {
	if len(t.items) == 0 {
		return "(daftar kosong)"
	}
	var b strings.Builder
	for i, it := range t.items {
		mark := "○"
		switch it.Status {
		case "in_progress":
			mark = "◐"
		case "completed":
			mark = "●"
		}
		fmt.Fprintf(&b, "%d. %s %s [%s]\n", i+1, mark, it.Content, it.Status)
	}
	return b.String()
}

func toolTodoWrite() ToolDef {
	tl := &todoList{}
	return ToolDef{
		Name: "todo_write",
		Description: "Catat & lacak daftar kerja multi-step. Setiap ganti fase kerja, update list ini " +
			"agar progres tidak hilang. Status: pending | in_progress | completed.",
		Parameters: objSchema(map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string", "description": "isi tugas"},
						"status":  map[string]any{"type": "string", "description": "pending|in_progress|completed"},
					},
					"required": []string{"content", "status"},
				},
				"description": "daftar tugas lengkap (menggantikan yang lama)",
			},
		}, "todos"),
		Exec: func(ctx context.Context, argsJSON string) (string, error) {
			var a struct {
				Todos []todoItem `json:"todos"`
			}
			if err := decodeArgs(argsJSON, &a); err != nil {
				return "", err
			}
			for _, it := range a.Todos {
				switch it.Status {
				case "pending", "in_progress", "completed":
				default:
					return "", fmt.Errorf("status tidak valid: %q", it.Status)
				}
			}
			return tl.set(a.Todos), nil
		},
	}
}

// v2Tools mengembalikan toolset F-15 (ditambah ke DefaultTools).
func v2Tools() []ToolDef {
	return []ToolDef{
		toolEditFile(),
		toolGrep(),
		toolGlob(),
		toolWebfetch(),
		toolTodoWrite(),
	}
}
