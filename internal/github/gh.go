package github

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Preflight verifies that `gh` and `git` are installed and that `gh` is
// authenticated. Callers should run this BEFORE rendering so we fail fast
// instead of leaving the user with a generated tree but no GitHub repo.
func Preflight(ctx context.Context, r Runner) error {
	if err := ensureBinary(ctx, r, "gh"); err != nil {
		return err
	}
	if err := ensureBinary(ctx, r, "git"); err != nil {
		return err
	}
	out, err := r.Run(ctx, "", "gh", "auth", "status")
	if err != nil {
		return newErr(ErrGHUnauth, "gh auth status", out, err)
	}
	return nil
}

// ensureBinary tries `<name> --version`. A LookPath-style failure becomes
// the appropriate "missing" error; a non-zero exit (rare for --version) is
// surfaced as a publish-failed error so the caller still gets a code.
func ensureBinary(ctx context.Context, r Runner, name string) error {
	out, err := r.Run(ctx, "", name, "--version")
	if err == nil {
		return nil
	}
	if isMissingBinary(err) {
		code := ErrPublishFailed
		switch name {
		case "gh":
			code = ErrGHMissing
		case "git":
			code = ErrGitMissing
		}
		return newErr(code, name+" --version", out, err)
	}
	return newErr(ErrPublishFailed, name+" --version", out, err)
}

// isMissingBinary reports whether err is exec.ErrNotFound (binary not on
// PATH). exec.CommandContext wraps with exec.Error, which carries
// exec.ErrNotFound as its inner Err.
func isMissingBinary(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	// exec.Error{Name, Err: ErrNotFound} — checked above via errors.Is via
	// Unwrap. Some shells render the error string differently; fall back
	// to a defensive substring sniff that catches the common variants.
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file or directory")
}
