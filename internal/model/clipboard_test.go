package model

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The platform's own helper gets the text on stdin: OSC 52 is ignored by macOS's own
// Terminal.app, so on a Mac `pbcopy` is what actually makes the key do something.
func TestClipboardHelperTakesTheText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip")
	old := clipboardHelpers
	clipboardHelpers = [][]string{
		{"tsk-no-such-helper"},   // not installed: skipped, not a failure
		{"false"},                // installed and refuses: the next one is tried
		{"tee", path},            // takes stdin, which is what pbcopy and wl-copy do
		{"tee", path + ".never"}, // never reached: the one before it took the text
	}
	t.Cleanup(func() { clipboardHelpers = old })

	out := clipboardOut
	clipboardOut = io.Discard
	t.Cleanup(func() { clipboardOut = out })

	msg, ok := copyToClipboard("tasnim@strativ.se")().(copiedMsg)
	if !ok || msg.Err != nil {
		t.Fatalf("copy answered %#v", msg)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the helper was not given the text: %v", err)
	}
	if got := strings.TrimSpace(string(b)); got != "tasnim@strativ.se" {
		t.Errorf("the helper got %q", got)
	}
	if _, err := os.Stat(path + ".never"); err == nil {
		t.Error("a second helper ran after one had taken it")
	}

	// No helper at all is not an error: the sequence above is the whole of the copy in a
	// terminal that implements it, which is most of them outside macOS.
	clipboardHelpers = nil
	if bare, _ := copyToClipboard("x")().(copiedMsg); bare.Err != nil {
		t.Errorf("no helper reported %v", bare.Err)
	}
}
