// Package config reads ~/.config/tsk/config.toml. Pure apart from the one file read:
// no tea, no model, so the parsing rules unit-test on their own.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Path is the config file: $XDG_CONFIG_HOME/tsk/config.toml, alongside tasks.json.
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tsk", "config.toml")
}

// file is the whole config. Only [keys] so far; a later [theme] goes here rather than
// in a second file.
type file struct {
	Keys map[string][]string `toml:"keys"`
}

// LoadKeys reads the [keys] table. No file is the normal case, not an error: it means
// the compiled-in defaults stand.
func LoadKeys() (map[string][]string, error) {
	return loadKeys(Path())
}

func loadKeys(path string) (map[string][]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var f file
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for action, keys := range f.Keys {
		for _, k := range keys {
			if !validKey(k) {
				// Worth refusing rather than passing through: a misspelled key never
				// matches anything, which reads as "rebinding is broken" instead of
				// "that key does not exist".
				return nil, fmt.Errorf("%s: keys.%s: %q is not a key — try a single "+
					"character, ctrl+/alt+/shift+ and one, or one of %s",
					path, action, k, strings.Join(named, ", "))
			}
		}
	}
	return f.Keys, nil
}

// named is bubbletea's own spelling of the keys that are not single characters
// (key.go, keyNames) — the ones worth writing in a config file.
var named = []string{
	"enter", "esc", "tab", "shift+tab", "space", "backspace", "delete",
	"up", "down", "left", "right", "home", "end", "pgup", "pgdown",
}

func validKey(k string) bool {
	if k == "" {
		return false
	}
	for _, n := range named {
		if k == n {
			return true
		}
	}
	// A modifier and one more key: ctrl+f, alt+enter, shift+tab.
	for _, mod := range []string{"ctrl+", "alt+", "shift+"} {
		if rest, ok := strings.CutPrefix(k, mod); ok {
			return validKey(rest)
		}
	}
	return len([]rune(k)) == 1
}
