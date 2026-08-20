package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts a config file in a temp dir and returns its path.
func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
		want map[string][]string
		err  string // a fragment the error has to mention
	}{
		{
			name: "overrides",
			body: "[keys]\nhalf_down = [\"ctrl+d\"]\ndown = [\"j\", \"down\", \"n\"]\n",
			want: map[string][]string{"half_down": {"ctrl+d"}, "down": {"j", "down", "n"}},
		},
		{
			// The whole point of the empty list: q no longer quits, ctrl+c still does.
			name: "unbind",
			body: "[keys]\nquit = []\n",
			want: map[string][]string{"quit": {}},
		},
		{
			name: "no keys table is not an error",
			body: "# nothing here yet\n",
			want: nil,
		},
		{
			name: "named keys and modifiers",
			body: "[keys]\naccept = [\"enter\", \"shift+tab\", \"pgdown\", \"alt+k\", \"/\"]\n",
			want: map[string][]string{"accept": {"enter", "shift+tab", "pgdown", "alt+k", "/"}},
		},
		{
			// A misspelled key would silently never match, which reads as a broken app.
			name: "misspelled key",
			body: "[keys]\nhalf_down = [\"ctrl-d\"]\n",
			err:  `"ctrl-d" is not a key`,
		},
		{
			name: "key that is a word, not a key",
			body: "[keys]\ndown = [\"jj\"]\n",
			err:  `"jj" is not a key`,
		},
		{
			name: "empty string is not a key",
			body: "[keys]\ndown = [\"\"]\n",
			err:  `is not a key`,
		},
		{
			name: "malformed toml",
			body: "[keys\ndown = j\n",
			err:  "config.toml",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := loadKeys(write(t, c.body))
			switch {
			case c.err != "" && err == nil:
				t.Fatalf("loadKeys(%q) = %v, want an error mentioning %q", c.body, got, c.err)
			case c.err != "":
				if !strings.Contains(err.Error(), c.err) {
					t.Fatalf("error = %q, want it to mention %q", err, c.err)
				}
				return
			case err != nil:
				t.Fatalf("loadKeys(%q): %v", c.body, err)
			}

			if len(got) != len(c.want) {
				t.Fatalf("keys = %v, want %v", got, c.want)
			}
			for action, want := range c.want {
				if strings.Join(got[action], ",") != strings.Join(want, ",") {
					t.Errorf("keys.%s = %v, want %v", action, got[action], want)
				}
			}
		})
	}
}

// A config file nobody wrote is the normal case, not a failure.
func TestMissingFileIsNotAnError(t *testing.T) {
	got, perTab, err := loadKeys(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil || got != nil || perTab != nil {
		t.Errorf("loadKeys(absent) = %v, %v, %v, want nil, nil, nil", got, perTab, err)
	}
}

// Path follows $XDG_CONFIG_HOME, which is what lets a test run — or a user try a
// keymap — without touching the real one.
func TestPathFollowsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/somewhere")
	if want := "/tmp/somewhere/tsk/config.toml"; Path() != want {
		t.Errorf("Path() = %q, want %q", Path(), want)
	}
}
