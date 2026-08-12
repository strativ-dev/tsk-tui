package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// KeyEnv wins over pass, so CI and one-off shells never touch the store.
	KeyEnv = "TSK_API_KEY"
	// DBEnv overrides the Odoo database name from the pass entry.
	DBEnv = "TSK_ODOO_DB"
	// PassEnv renames the pass entry.
	PassEnv = "TSK_PASS_NAME"

	defaultPassName = "tsk/api-key"
)

// ErrNoPass means the credential store is unavailable; nothing is written in
// plaintext as a fallback.
var ErrNoPass = errors.New("pass not found — install password-store, or export " + KeyEnv)

// KeyMsg carries the resolved credentials. The key goes to the model field and
// the Authorization header, never to tasks.json and never into an error string.
// DB is the Odoo database name, treated as a secret for the same reason: it is
// half of what an attacker needs to talk to the ERP over JSON-RPC, so it lives in
// the encrypted entry rather than in this repo.
type KeyMsg struct {
	Key string
	DB  string
	Err error
}

type KeySavedMsg struct{ Err error }

// PassName is the pass entry holding the key, e.g. "tsk/api-key".
func PassName() string {
	if v := strings.TrimSpace(os.Getenv(PassEnv)); v != "" {
		return v
	}
	return defaultPassName
}

// LoadKey resolves the credentials: $TSK_API_KEY first, then `pass show <entry>`.
// A missing entry is not an error — the model asks for a key.
func LoadKey() tea.Cmd {
	envDB := strings.TrimSpace(os.Getenv(DBEnv))

	if v := strings.TrimSpace(os.Getenv(KeyEnv)); v != "" {
		return func() tea.Msg { return KeyMsg{Key: v, DB: envDB} }
	}
	if _, err := exec.LookPath("pass"); err != nil {
		return func() tea.Msg { return KeyMsg{DB: envDB, Err: ErrNoPass} }
	}

	var out bytes.Buffer
	c := exec.Command("pass", "show", PassName())
	c.Stdout = &out
	// ExecProcess lends the terminal to gpg, so a tty pinentry can ask for the
	// passphrase instead of scribbling over the alt screen. stdout stays captured.
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return KeyMsg{DB: envDB} // no entry yet, or gpg declined: prompt for it
		}
		db := envDB
		if db == "" {
			db = passField(out.String(), "db")
		}
		return KeyMsg{Key: firstLine(out.String()), DB: db}
	})
}

// passField reads a `name: value` line from a pass entry, following pass's own
// convention: the secret on line one, metadata on the lines after it.
func passField(entry, name string) string {
	lines := strings.Split(entry, "\n")
	if len(lines) < 2 {
		return ""
	}
	want := strings.ToLower(name) + ":"
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), want) {
			return strings.TrimSpace(trimmed[len(want):])
		}
	}
	return ""
}

// SaveKey stores the key with `pass insert`, which encrypts it to your GPG key.
// A known db name is written back as a `db:` line so re-entering a key does not
// lose it.
func SaveKey(key, db string) tea.Cmd {
	return func() tea.Msg {
		key, db = strings.TrimSpace(key), strings.TrimSpace(db)
		if key == "" {
			return KeySavedMsg{Err: errors.New("empty API key")}
		}
		if _, err := exec.LookPath("pass"); err != nil {
			return KeySavedMsg{Err: ErrNoPass}
		}

		entry := key + "\n"
		if db != "" {
			entry += "db: " + db + "\n"
		}
		c := exec.Command("pass", "insert", "-m", "-f", PassName())
		c.Stdin = strings.NewReader(entry)
		var stderr bytes.Buffer
		c.Stderr = &stderr
		if err := c.Run(); err != nil {
			why := firstLine(stderr.String())
			if why == "" {
				why = err.Error()
			}
			return KeySavedMsg{Err: fmt.Errorf("pass insert %s: %s", PassName(), why)}
		}
		return KeySavedMsg{}
	}
}

// MaskKey renders a key for the screen: last four characters only.
func MaskKey(key string) string {
	key = strings.TrimSpace(key)
	switch {
	case key == "":
		return "not set"
	case len(key) <= 4:
		return "••••"
	default:
		return "••••" + key[len(key)-4:]
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// Merge keeps remote titles and task set, and keeps every local time entry.
// Tasks only known locally survive, so logged hours are never dropped.
func Merge(local, remote []Task) []Task {
	rows := make(map[int][]Entry, len(local))
	seen := make(map[int]bool, len(remote))
	for _, t := range local {
		rows[t.ID] = t.Rows
	}

	out := make([]Task, 0, len(local)+len(remote))
	for _, t := range remote {
		t.Rows = rows[t.ID]
		seen[t.ID] = true
		out = append(out, t)
	}
	for _, t := range local {
		if !seen[t.ID] && len(t.Rows) > 0 {
			out = append(out, t)
		}
	}
	return out
}
