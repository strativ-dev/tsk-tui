package model

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// copiedMsg answers a copy: what was put on the clipboard, or why it could not be written.
type copiedMsg struct {
	Text string
	Err  error
}

// copyToClipboard puts one line on the system clipboard, both ways there are, because neither
// one covers every terminal this app runs in:
//
//   - **OSC 52**, the terminal's own copy sequence, written to stdout. It costs no dependency
//     and no process, and it is the only one that works **over ssh** — the terminal in front of
//     you answers it, not the machine the app is on. But macOS's own Terminal.app does not
//     implement it at all, and iTerm2 asks to have it switched on, so on a Mac it is often
//     written into a terminal that quietly ignores it.
//   - **The platform's own helper** — `pbcopy` on macOS, `wl-copy`, `xclip` or `xsel` on Linux,
//     whichever is installed and works. This is what makes the key do something on a default
//     macOS terminal. It reaches the clipboard of the machine the app is running on, which is
//     the wrong one over ssh, and needs a program that may not be there.
//
// So both are attempted and neither is required. The helper's own failure is not reported: it
// is the fallback for the case the sequence already covered, and a Linux box with no `xclip`
// in a terminal that copied the address perfectly well should not be told the copy failed.
//
// It is a tea.Cmd, so the writes happen off the Update loop like every other side effect here.
// The sequence deliberately does not go through View: a string rendered every frame would
// re-copy on every frame.
func copyToClipboard(s string) tea.Cmd {
	return func() tea.Msg {
		_, err := fmt.Fprintf(clipboardOut, "\x1b]52;c;%s\a",
			base64.StdEncoding.EncodeToString([]byte(s)))
		runClipboardHelper(s)
		return copiedMsg{Text: s, Err: err}
	}
}

// clipboardOut is where the sequence goes: the terminal Bubble Tea is drawing on. A package
// var so a test can read what was written instead of setting the clipboard of whoever is
// running the suite.
var clipboardOut io.Writer = os.Stdout

// clipboardHelpers is the programs to offer the text to, in order, until one takes it. A
// package var for the same reason as clipboardOut — a test sets its own, so the suite forks
// nothing and copies nothing.
var clipboardHelpers = defaultClipboardHelpers()

// defaultClipboardHelpers is what to try on this platform. The Linux list is tried in order
// rather than chosen by inspecting `$WAYLAND_DISPLAY`: `wl-copy` on an X11 session fails
// immediately and `xclip` is right behind it, which is the same answer with nothing to keep
// in step with how a desktop reports itself.
func defaultClipboardHelpers() [][]string {
	if runtime.GOOS == "darwin" {
		return [][]string{{"pbcopy"}}
	}
	return [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
}

// runClipboardHelper hands the text to the first helper that is installed and exits cleanly.
// Best effort: no helper, or every one of them refusing, is not an error here.
func runClipboardHelper(s string) {
	for _, argv := range clipboardHelpers {
		path, err := exec.LookPath(argv[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, argv[1:]...)
		cmd.Stdin = strings.NewReader(s)
		// Nothing of theirs may reach the alt screen: a helper that printed a warning would
		// draw it over the calendar.
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		if cmd.Run() == nil {
			return
		}
	}
}
