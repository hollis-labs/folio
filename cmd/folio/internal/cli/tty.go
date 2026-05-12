package cli

import (
	"os"

	"golang.org/x/term"
)

// termIsTTY returns true when f looks like an interactive terminal.
// Wrapping the dependency in a tiny shim keeps the import noise out of
// new.go and makes it trivial to swap for a stub in tests.
func termIsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
