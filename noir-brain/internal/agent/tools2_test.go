package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ambil ToolDef dari DefaultTools berdasarkan nama.
func getTool(t *testing.T, name string) ToolDef {
	t.Helper()
	for _, td := range DefaultTools(nil) {
		if td.Name == name {
			return td
		}
	}
	t.Fatalf("tool %q tidak ketemu", name)
	return ToolDef{}
}

func TestWaktuIDHariBenar(t *testing.T) {
	// 2026-10-03 adalah Sabtu (bukan "Senin" literal seperti bug lama).
	out, err := getTool(t, "waktu").Exec(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "Sabtu, 3 Okt 2026") {
		t.Fatalf("nama hari salah: %q", out)
	}
}

func TestEditFile(t *testing.T) {
	td := getTool(t, "edit_file")
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("halo dunia\nhalo lagi\n"), 0644)

	out, err := td.Exec(context.Background(), `{"path":"`+p+`","old_string":"dunia","new_string":"bro"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1 kemunculan") {
		t.Fatalf("harusnya 1: %q", out)
	}
	d, _ := os.ReadFile(p)
	if !strings.Contains(string(d), "halo bro") {
		t.Fatalf("isi tidak keganti: %q", d)
	}

	// old_string tidak ketemu
	if _, err := td.Exec(context.Background(), `{"path":"`+p+`","old_string":"zzz","new_string":"x"}`); err == nil {
		t.Fatal("harus error kalau tidak ketemu")
	}
	// >1 tanpa replace_all
	if _, err := td.Exec(context.Background(), `{"path":"`+p+`","old_string":"halo","new_string":"x"}`); err == nil {
		t.Fatal("harus error kalau >1 tanpa replace_all")
	}
	// replace_all
	if _, err := td.Exec(context.Background(), `{"path":"`+p+`","old_string":"halo","new_string":"hai","replace_all":true}`); err != nil {
		t.Fatal(err)
	}
	// path kredensial ditolak
	if _, err := td.Exec(context.Background(), `{"path":"/root/.env","old_string":"a","new_string":"b"}`); err == nil {
		t.Fatal("path kredensial harus ditolak")
	}
}

func TestGrep(t *testing.T) {
	td := getTool(t, "grep")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc Halo() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("halo dunia\n"), 0644)

	out, err := td.Exec(context.Background(), `{"pattern":"func Halo","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.go:2:") {
		t.Fatalf("hasil grep salah: %q", out)
	}

	out, err = td.Exec(context.Background(), `{"pattern":"tidakada","path":"`+dir+`"}`)
	if err != nil || out != "(tidak ketemu)" {
		t.Fatalf("harus (tidak ketemu): %q %v", out, err)
	}

	if _, err := td.Exec(context.Background(), `{"pattern":"[","path":"`+dir+`"}`); err == nil {
		t.Fatal("regex invalid harus error")
	}
}

func TestGlob(t *testing.T) {
	td := getTool(t, "glob")
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "b.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "c.md"), []byte("x"), 0644)

	out, err := td.Exec(context.Background(), `{"pattern":"**/*.go","path":"`+dir+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") || strings.Contains(out, "c.md") {
		t.Fatalf("hasil glob salah: %q", out)
	}
}

func TestWebfetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>T</title><script>var x=1;</script></head><body><h1>Halo bro</h1><p>isi</p></body></html>`))
	}))
	defer srv.Close()

	td := getTool(t, "webfetch")
	out, err := td.Exec(context.Background(), `{"url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Halo bro") || strings.Contains(out, "var x=1") {
		t.Fatalf("HTML tidak bersih: %q", out)
	}

	// skema non-http ditolak
	if _, err := td.Exec(context.Background(), `{"url":"file:///etc/passwd"}`); err == nil {
		t.Fatal("file:// harus ditolak")
	}
}

func TestTodoWrite(t *testing.T) {
	td := getTool(t, "todo_write")
	out, err := td.Exec(context.Background(), `{"todos":[
		{"content":"a","status":"completed"},
		{"content":"b","status":"in_progress"},
		{"content":"c","status":"pending"}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "● a") || !strings.Contains(out, "◐ b") || !strings.Contains(out, "○ c") {
		t.Fatalf("render salah: %q", out)
	}

	if _, err := td.Exec(context.Background(), `{"todos":[{"content":"x","status":"ngaco"}]}`); err == nil {
		t.Fatal("status invalid harus error")
	}
}
