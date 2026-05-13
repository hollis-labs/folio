// Package cli wires the Cobra command tree for the folio binary. It lives
// under cmd/folio/internal/ so the API surface stays unexported and tests
// can drive Run(...) directly without spawning subprocesses.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/hollis-labs/folio/service"
)

// Exit codes follow design doc cli-prompt-flow-v0.md §9.
const (
	ExitOK        = 0
	ExitGeneric   = 1
	ExitUsage     = 2
	ExitCancelled = 130
)

// Run constructs the root command tree, executes it against args, and
// returns the process exit code. bundledFS supplies the read-only
// filesystem holding the bundled presets (typically folio.BundledPresets).
// Pass nil to disable bundled-preset lookup (tests).
func Run(args []string, bundledFS fs.FS, version string) int {
	root := NewRootCmd(bundledFS, version)
	root.SetArgs(args)
	root.SilenceErrors = true
	root.SilenceUsage = true

	err := root.ExecuteContext(context.Background())
	if err == nil {
		return ExitOK
	}
	// SilenceErrors keeps cobra quiet so the CLI controls its own
	// presentation; we still need to surface the error to the user.
	var ce *cancelledError
	if !errors.As(err, &ce) {
		fmt.Fprintln(os.Stderr, "folio: "+err.Error())
	}
	return exitCodeFor(err)
}

// NewRootCmd builds the `folio` Cobra root with all subcommands wired in.
// Exposed for tests that need to invoke commands directly (e.g. in-process
// rather than via Run + os.Args).
func NewRootCmd(bundledFS fs.FS, version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "folio",
		Short: "Preset-driven project scaffolding",
		Long: `folio scaffolds new projects from preset manifests.

A preset bundles a typed inputs schema, computed variables, and a
text/template-rendered file tree. ` + "`folio new`" + ` renders a preset into a target
directory; ` + "`folio plan`" + ` previews the same render without writing.`,
		Version:           version,
		DisableAutoGenTag: true,
	}

	root.AddCommand(newCmd(bundledFS, version))
	root.AddCommand(makeCmd(bundledFS, version))
	root.AddCommand(planCmd(bundledFS, version))
	root.AddCommand(presetCmd(bundledFS, version))

	// Reserved subcommands. These announce themselves as "not yet
	// implemented" so users get a clear signal that the surface is planned
	// rather than missing or buggy.
	for _, name := range []string{"sync", "inspect"} {
		root.AddCommand(stubCmd(name))
	}

	return root
}

func stubCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("%s (not yet implemented in v0)", name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return &service.Error{Code: service.ErrInternal, Message: fmt.Sprintf("%s: not yet implemented in v0", name)}
		},
	}
}

// exitCodeFor maps service errors and Cobra usage errors to the documented
// exit-code table.
func exitCodeFor(err error) int {
	var ce *cancelledError
	if errors.As(err, &ce) {
		return ExitCancelled
	}
	var se *service.Error
	if errors.As(err, &se) {
		switch se.Code {
		case service.ErrInputMissing, service.ErrInputInvalid:
			return ExitUsage
		default:
			return ExitGeneric
		}
	}
	// Cobra surfaces flag/usage errors via the cobra.ErrSubCommandRequired
	// family — they typically embed a UsageError. We treat any non-service
	// error from Execute as a generic failure.
	return ExitGeneric
}

// fprintf writes a formatted line to the command's err stream. Helper so
// CLI code doesn't sprinkle direct os.Stderr writes.
func fprintln(cmd *cobra.Command, msg string) {
	if cmd != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}
