package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// stubPass puts a fake `pass` on PATH that records how it was called.
func stubPass(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + filepath.Join(dir, "args") +
		"\ncat > " + filepath.Join(dir, "stdin") + "\n"
	bin := filepath.Join(dir, "pass")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH")) // stub shadows the real pass
	return dir
}

func TestSaveKeyGoesToPass(t *testing.T) {
	dir := stubPass(t)
	t.Setenv(PassEnv, "work/tsk-key")

	msg := SaveKey("  odoo-key-1234  ")()
	if saved, ok := msg.(KeySavedMsg); !ok || saved.Err != nil {
		t.Fatalf("SaveKey = %+v", msg)
	}

	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "insert\n-m\n-f\nwork/tsk-key\n"; string(args) != want {
		t.Errorf("pass args = %q, want %q", args, want)
	}

	stdin, err := os.ReadFile(filepath.Join(dir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != "odoo-key-1234\n" {
		t.Errorf("key on stdin = %q, want it trimmed", stdin)
	}
}

func TestSaveKeyEmpty(t *testing.T) {
	stubPass(t)
	if got := SaveKey("   ")().(KeySavedMsg); got.Err == nil {
		t.Error("SaveKey(blank) = nil error, want a complaint")
	}
}

func TestKeyWithoutPass(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no pass on PATH
	t.Setenv(KeyEnv, "")

	if got := LoadKey()().(KeyMsg); !errors.Is(got.Err, ErrNoPass) {
		t.Errorf("LoadKey = %+v, want ErrNoPass", got)
	}
	if got := SaveKey("k")().(KeySavedMsg); !errors.Is(got.Err, ErrNoPass) {
		t.Errorf("SaveKey = %+v, want ErrNoPass", got)
	}
}

func TestKeyEnvWins(t *testing.T) {
	stubPass(t)
	t.Setenv(KeyEnv, "  from-env  ")
	if got := LoadKey()().(KeyMsg); got.Key != "from-env" {
		t.Errorf("LoadKey = %+v, want the trimmed env key", got)
	}
}

func TestPassName(t *testing.T) {
	t.Setenv(PassEnv, "")
	if got := PassName(); got != "tsk/api-key" {
		t.Errorf("PassName() = %q, want the default", got)
	}
	t.Setenv(PassEnv, " team/erp ")
	if got := PassName(); got != "team/erp" {
		t.Errorf("PassName() = %q, want team/erp", got)
	}
}

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"":              "not set",
		"abc":           "••••",
		"odoo-key-1234": "••••1234",
	}
	for in, want := range cases {
		if got := MaskKey(in); got != want {
			t.Errorf("MaskKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMerge(t *testing.T) {
	local := []Task{
		{ID: 1, Title: "old title", Tag: "old", Rows: []Entry{{ID: 1, Minutes: 60}}},
		{ID: 9, Title: "gone from API", Rows: []Entry{{ID: 1, Minutes: 30}}},
		{ID: 8, Title: "gone, no hours"},
	}
	remote := []Task{
		{ID: 1, Title: "new title", Tag: "ERP 360"},
		{ID: 2, Title: "brand new", Tag: "ui"},
	}

	got := Merge(local, remote)
	if len(got) != 3 {
		t.Fatalf("merged = %d tasks, want 3", len(got))
	}
	if got[0].Title != "new title" || got[0].Tag != "ERP 360" {
		t.Errorf("remote should win on title and tag: %+v", got[0])
	}
	if len(got[0].Rows) != 1 || got[0].Rows[0].Minutes != 60 {
		t.Errorf("local entries must survive a sync: %+v", got[0].Rows)
	}
	if len(got[1].Rows) != 0 {
		t.Errorf("new remote task should have no entries: %+v", got[1])
	}
	if got[2].ID != 9 {
		t.Errorf("task with local hours must not be dropped: %+v", got[2])
	}
}
