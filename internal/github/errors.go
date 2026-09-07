package github

import "fmt"

// ErrorCode mirrors service.ErrorCode — short stable strings the CLI and
// future MCP/HTTP surfaces pattern-match without parsing English.
type ErrorCode string

const (
	// ErrGHMissing — `gh` is not installed or not on PATH.
	ErrGHMissing ErrorCode = "gh_missing"
	// ErrGHUnauth — `gh auth status` reported the user is not logged in.
	ErrGHUnauth ErrorCode = "gh_unauthenticated"
	// ErrGitMissing — `git` is not installed or not on PATH.
	ErrGitMissing ErrorCode = "git_missing"
	// ErrRepoExists — `gh repo create` rejected the name; one already exists.
	ErrRepoExists ErrorCode = "repo_exists"
	// ErrPublishFailed — generic catch-all for an unclassified gh/git failure.
	ErrPublishFailed ErrorCode = "publish_failed"
)

// Error is the typed error returned by this package. Carries a stable
// ErrorCode plus the underlying tool's combined output so the CLI can
// surface gh/git's own message verbatim.
type Error struct {
	Code   ErrorCode
	Op     string // "preflight", "git init", "gh repo create", "push", ...
	Output string // combined stdout+stderr from the failing tool
	Err    error  // underlying exec error
}

// Error formats the error in a "code (op): message" shape with the tool's
// output trailing.
func (e *Error) Error() string {
	out := e.Output
	if out == "" && e.Err != nil {
		out = e.Err.Error()
	}
	if e.Op == "" {
		return fmt.Sprintf("%s: %s", e.Code, out)
	}
	return fmt.Sprintf("%s (%s): %s", e.Code, e.Op, out)
}

// Unwrap supports errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Err }

func newErr(code ErrorCode, op string, output []byte, err error) *Error {
	return &Error{Code: code, Op: op, Output: string(output), Err: err}
}
