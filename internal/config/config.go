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

// file is the whole config. [keys] holds the keymap; a later [theme] goes here rather than
// in a second file.
//
// The table is read as `map[string]any` because it holds two kinds of entry: an action, whose
// value is a list of keys, and a screen, whose value is a table of actions. TOML calls those
// a value and a sub-table, and one map cannot type both.
type file struct {
	Keys map[string]any `toml:"keys"`
}

// LoadKeys reads the [keys] table: the global keymap, and one map per screen from the
// `[keys.<tab>]` sub-tables. No file is the normal case, not an error — it means the
// compiled-in defaults stand.
func LoadKeys() (map[string][]string, map[string]map[string][]string, error) {
	return loadKeys(Path())
}

func loadKeys(path string) (map[string][]string, map[string]map[string][]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	var f file
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	global := map[string][]string{}
	perTab := map[string]map[string][]string{}
	for name, raw := range f.Keys {
		switch v := raw.(type) {
		case map[string]any:
			// A screen's own table: [keys.meal] and the actions under it.
			tab := map[string][]string{}
			for action, list := range v {
				binds, err := keyList(path, name+"."+action, list)
				if err != nil {
					return nil, nil, err
				}
				tab[action] = binds
			}
			perTab[name] = tab
		default:
			binds, err := keyList(path, name, raw)
			if err != nil {
				return nil, nil, err
			}
			global[name] = binds
		}
	}
	return global, perTab, nil
}

// keyList reads one action's value: a list of key spellings, each of which has to be one the
// runtime can actually match.
func keyList(path, action string, raw any) ([]string, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: keys.%s: expected a list of keys, like [\"x\"]", path, action)
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		k, ok := item.(string)
		if !ok || !validKey(k) {
			// Worth refusing rather than passing through: a misspelled key never matches
			// anything, which reads as "rebinding is broken" instead of "that key does not
			// exist".
			return nil, fmt.Errorf("%s: keys.%s: %q is not a key — try a single "+
				"character, ctrl+/alt+/shift+ and one, or one of %s",
				path, action, fmt.Sprint(item), strings.Join(named, ", "))
		}
		out = append(out, k)
	}
	return out, nil
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
