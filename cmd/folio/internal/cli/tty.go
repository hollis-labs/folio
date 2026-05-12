package cli

import (
	"os"

	"golang.org/x/term"
)

// termIsTTY returns true when f looks like an interactive terminal.
// Wrapping the dependency in a tiny shim keeps the import noise out of
// new.go and makes it trivial to swap for a stub in tests.
//
// The uintptr → int conversion can only "overflow" on systems where file
// descriptors exceed math.MaxInt, which doesn't happen in practice (no
// modern OS allocates fds anywhere near that range). gosec G115 is too
// noisy to keep enabled portfolio-wide; we silence it locally.
func termIsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd())) //nolint:gosec // see comment
}
