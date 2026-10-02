// Toolset v1 noir-brain: aksi lokal di STB yang bisa dipanggil LLM.
//
// Safety model:
//   - exec: denylist pola destruktif, timeout 30 dtk, output dipotong.
//   - file: denylist path kredensial; write dilarang di direktori sistem.
//   - service_*: nama harus cocok regex dan ada di allowlist config.
//   - semua pemanggilan dicatat ke operation log oleh Agent.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxOutput = 4096   // potong output tool
	maxWrite  = 102400 // maks 100KB per write_file
	execTimeout = 30 * time.Second
)

// ---------- helpers ----------

func objSchema(props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n…(dipotong)"
	}
	return s
}

func decodeArgs(argsJSON string, v any) error {
	if argsJSON == "" {
		return fmt.Errorf("argumen kosong")
	}
	return json.Unmarshal([]byte(argsJSON), v)
}

// ---------- denylist ----------

// Pola perintah yang SELALU ditolak di exec (voice-triggered = risiko tinggi).
var execDeny = []string{
	"rm -rf /", "rm -rf /*", "rm -fr /",
	"mkfs", "dd if=", "dd of=/dev",
	"shutdown", "reboot", "poweroff", "halt",
	":(){", "fork bomb",
	"> /dev/sd", ">/dev/sd",
	"chmod -R 777 /", "chown -R",
	"|sh", "| sh", "|bash", "| bash", // remote code exec via pipe
	"curl", "wget", // unduh+eksekusi via suara = jangan
}

func execDenied(cmd string) string {
	lower := strings.ToLower(cmd)
	for _, p := range execDeny {
		if strings.Contains(lower, strings.ToLower(p)) {
			return p
		}
	}
	return ""
}

// Path yang tidak boleh dibaca/ditulis (kredensial & co).
var secretSubstr = []string{
	".env", ".ssh/", "id_rsa", "id_ed25519", "id_ecdsa",
	".gnupg/", "authorized_keys", "credentials", ".secret",
}

// Prefix direktori yang tidak boleh DITULIS.
var writeDenyPrefixes = []string{
	"/etc/", "/boot/", "/proc/", "/sys/", "/dev/",
	"/bin/", "/sbin/", "/usr/bin/", "/usr/sbin/", "/lib",
}

func cleanPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path kosong")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	lower := strings.ToLower(abs)
	for _, s := range secretSubstr {
		if strings.Contains(lower, strings.ToLower(s)) {
			return "", fmt.Errorf("path terlarang (kredensial): %s", p)
		}
	}
	return abs, nil
}

func checkWritable(abs string) error {
	for _, pre := range writeDenyPrefixes {
		if abs == strings.TrimSuffix(pre, "/") || strings.HasPrefix(abs, pre) {
			return fmt.Errorf("tidak boleh menulis ke %s", pre)
		}
	}
	return nil
}

var serviceNameRe = regexp.MustCompile(`^[a-zA-Z0-9@._:-]+$`)

// ---------- tools ----------

// DefaultTools mengembalikan toolset v1. serviceAllowlist = nama service yang
// boleh di-status/restart (mis. ["noir-brain"]).
func DefaultTools(serviceAllowlist []string) []ToolDef {
	allowed := map[string]bool{}
	for _, s := range serviceAllowlist {
		allowed[s] = true
	}

	return []ToolDef{
		{
			Name:        "waktu",
			Description: "Waktu sekarang di server STB (zona waktu lokal).",
			Parameters:  objSchema(map[string]any{}),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				return time.Now().Format("Senin, 2 Jan 2006 15:04:05 MST"), nil
			},
		},
		{
			Name:        "sysinfo",
			Description: "Info sistem STB: uptime, load average, memori, dan disk.",
			Parameters:  objSchema(map[string]any{}),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				var b strings.Builder
				if d, err := os.ReadFile("/proc/uptime"); err == nil {
					fmt.Fprintf(&b, "uptime: %s", strings.TrimSpace(string(d)))
				}
				if d, err := os.ReadFile("/proc/loadavg"); err == nil {
					fmt.Fprintf(&b, "\nloadavg: %s", strings.TrimSpace(string(d)))
				}
				if d, err := os.ReadFile("/proc/meminfo"); err == nil {
					for _, l := range strings.Split(string(d), "\n") {
						if strings.HasPrefix(l, "MemTotal:") || strings.HasPrefix(l, "MemAvailable:") {
							b.WriteString("\n" + strings.Join(strings.Fields(l), " "))
						}
					}
				}
				out, err := exec.CommandContext(ctx, "df", "-h", "/", "/opt").CombinedOutput()
				if err == nil {
					fmt.Fprintf(&b, "\ndisk:\n%s", trunc(string(out), 800))
				}
				return b.String(), nil
			},
		},
		{
			Name: "exec",
			Description: "Jalankan perintah shell di STB. Timeout 30 detik, output dipotong ~4KB. " +
				"Perintah destruktif (hapus sistem, format, shutdown, pipe ke shell, curl/wget) DITOLAK. " +
				"Gunakan untuk cek status, baca log, dan tugas non-destruktif.",
			Parameters: objSchema(map[string]any{
				"command": strProp("perintah shell, mis. \"systemctl is-active docker\""),
			}, "command"),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				var a struct {
					Command string `json:"command"`
				}
				if err := decodeArgs(argsJSON, &a); err != nil {
					return "", err
				}
				cmd := strings.TrimSpace(a.Command)
				if cmd == "" {
					return "", fmt.Errorf("command kosong")
				}
				if pat := execDenied(cmd); pat != "" {
					return "", fmt.Errorf("perintah ditolak (pola terlarang: %s)", pat)
				}
				tctx, cancel := context.WithTimeout(ctx, execTimeout)
				defer cancel()
				out, err := exec.CommandContext(tctx, "sh", "-c", cmd).CombinedOutput()
				res := trunc(string(out), maxOutput)
				if tctx.Err() == context.DeadlineExceeded {
					return res + "\n…(timeout 30 dtk)", nil
				}
				if err != nil {
					return res + fmt.Sprintf("\n(exit error: %v)", err), nil
				}
				return res, nil
			},
		},
		{
			Name:        "read_file",
			Description: "Baca isi file teks di STB (maks ~8KB). File kredensial (.env, key, ssh) ditolak.",
			Parameters: objSchema(map[string]any{
				"path": strProp("path file, mis. \"/var/log/syslog\""),
			}, "path"),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				var a struct {
					Path string `json:"path"`
				}
				if err := decodeArgs(argsJSON, &a); err != nil {
					return "", err
				}
				abs, err := cleanPath(a.Path)
				if err != nil {
					return "", err
				}
				d, err := os.ReadFile(abs)
				if err != nil {
					return "", err
				}
				return trunc(string(d), 8192), nil
			},
		},
		{
			Name: "write_file",
			Description: "Tulis/overwrite file teks di STB (maks 100KB). " +
				"Dilarang menulis ke direktori sistem (/etc, /boot, ...) dan file kredensial.",
			Parameters: objSchema(map[string]any{
				"path":    strProp("path file tujuan"),
				"content": strProp("isi file"),
			}, "path", "content"),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				var a struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				}
				if err := decodeArgs(argsJSON, &a); err != nil {
					return "", err
				}
				if len(a.Content) > maxWrite {
					return "", fmt.Errorf("content melebihi 100KB")
				}
				abs, err := cleanPath(a.Path)
				if err != nil {
					return "", err
				}
				if err := checkWritable(abs); err != nil {
					return "", err
				}
				if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
					return "", err
				}
				if err := os.WriteFile(abs, []byte(a.Content), 0644); err != nil {
					return "", err
				}
				return fmt.Sprintf("ok, tertulis %d byte ke %s", len(a.Content), abs), nil
			},
		},
		{
			Name:        "list_dir",
			Description: "Daftar isi direktori di STB.",
			Parameters: objSchema(map[string]any{
				"path": strProp("path direktori, mis. \"/opt\""),
			}, "path"),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				var a struct {
					Path string `json:"path"`
				}
				if err := decodeArgs(argsJSON, &a); err != nil {
					return "", err
				}
				abs, err := cleanPath(a.Path)
				if err != nil {
					return "", err
				}
				entries, err := os.ReadDir(abs)
				if err != nil {
					return "", err
				}
				var b strings.Builder
				for i, e := range entries {
					if i >= 100 {
						b.WriteString("…(dipotong, >100 entri)\n")
						break
					}
					mark := " "
					if e.IsDir() {
						mark = "/"
					}
					fmt.Fprintf(&b, "%s%s\n", e.Name(), mark)
				}
				return b.String(), nil
			},
		},
		{
			Name:        "service_status",
			Description: "Cek status systemd service (hanya service yang diizinkan).",
			Parameters: objSchema(map[string]any{
				"name": strProp("nama service, mis. \"noir-brain\""),
			}, "name"),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				name, err := checkService(a2s(argsJSON), allowed)
				if err != nil {
					return "", err
				}
				out, _ := exec.CommandContext(ctx, "systemctl", "is-active", name+".service").CombinedOutput()
				return fmt.Sprintf("%s: %s", name, strings.TrimSpace(string(out))), nil
			},
		},
		{
			Name: "service_restart",
			Description: "Restart systemd service (hanya service yang diizinkan, mis. noir-brain). " +
				"Pakai hanya kalau diminta eksplisit.",
			Parameters: objSchema(map[string]any{
				"name": strProp("nama service"),
			}, "name"),
			Exec: func(ctx context.Context, argsJSON string) (string, error) {
				name, err := checkService(a2s(argsJSON), allowed)
				if err != nil {
					return "", err
				}
				out, err := exec.CommandContext(ctx, "systemctl", "restart", name+".service").CombinedOutput()
				if err != nil {
					return "", fmt.Errorf("restart gagal: %s (%v)", strings.TrimSpace(string(out)), err)
				}
				return fmt.Sprintf("ok, %s di-restart", name), nil
			},
		},
	}
}

// a2s mengekstrak field "name" dari args JSON.
func a2s(argsJSON string) string {
	var a struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &a)
	return strings.TrimSpace(a.Name)
}

func checkService(name string, allowed map[string]bool) (string, error) {
	if name == "" {
		return "", fmt.Errorf("nama service kosong")
	}
	if !serviceNameRe.MatchString(name) {
		return "", fmt.Errorf("nama service tidak valid")
	}
	name = strings.TrimSuffix(name, ".service")
	if !allowed[name] {
		return "", fmt.Errorf("service %q tidak diizinkan (allowlist: AGENT_SERVICES)", name)
	}
	return name, nil
}
