package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/config"
	"github.com/tasnimAlam/tsk/internal/model"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--print-keys" {
		fmt.Print(model.KeysTOML())
		return
	}

	// The keymap is read here rather than in model.New: New runs in every test, and a
	// test must not depend on whatever is in the developer's config file. A bad file
	// stops the program — dropping into an alt screen you cannot drive is worse than a
	// message on stderr.
	binds, err := config.LoadKeys()
	if err == nil {
		err = model.ApplyKeys(binds)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(model.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
