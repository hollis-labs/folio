// Package github wraps the user's local `gh` CLI and a small set of `git`
// commands so folio can publish a freshly-rendered project to GitHub.
//
// The package never touches OAuth tokens or the GitHub REST API directly —
// it shells out to `gh`, which the user is expected to have installed and
// authenticated (`gh auth login`). The Runner interface exists so tests can
// substitute a fake exec surface without touching the real binaries.
package github

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Runner abstracts command execution so unit tests can stub out `gh`/`git`
// invocations. The contract: Run returns combined stdout+stderr and the
// raw exec error; callers decide how to classify the failure (missing
// binary, non-zero exit, parsing failure, etc.).
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// ExecRunner is the default Runner — backed by os/exec.
type ExecRunner struct{}

// Run implements Runner.Run via exec.CommandContext, returning combined
// stdout+stderr so error messages from `gh` (which goes to stderr) are
// preserved for the caller.
func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return buf.Bytes(), fmt.Errorf("%s %v: %w", name, args, err)
	}
	return buf.Bytes(), nil
}
