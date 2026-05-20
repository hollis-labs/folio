package github

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Visibility is the GitHub repo visibility flag. The empty value is
// rejected by CreateRepo — callers must choose explicitly.
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityPrivate  Visibility = "private"
	VisibilityInternal Visibility = "internal"
)

// Valid reports whether v is one of the three accepted values.
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityPublic, VisibilityPrivate, VisibilityInternal:
		return true
	}
	return false
}

// CreateOpts parameterises a CreateRepo call. TargetDir must already
// contain the rendered project tree; CreateRepo will initialize git, make
// the initial commit, create the GitHub repo via `gh`, and (unless Push
// is false) push the default branch.
type CreateOpts struct {
	TargetDir   string
	Owner       string
	Repo        string
	Visibility  Visibility
	Description string
	Branch      string // default branch name, e.g. "main"
	Push        bool   // when false, repo + remote are created but nothing is pushed
	CommitMsg   string // optional override; defaults to "Initial commit (folio)"
}

// CreateResult describes what CreateRepo accomplished.
type CreateResult struct {
	URL    string
	Owner  string
	Repo   string
	Branch string
	Pushed bool
}

// CreateRepo runs the full publish flow against opts.TargetDir:
//
//  1. Initialize git (no-op if already a repo) on opts.Branch.
//  2. Stage everything and make the initial commit (skipped if the repo
//     already has commits).
//  3. `gh repo create <owner>/<repo>` with the right visibility +
//     description flags, attaching the local tree as the source.
//  4. Push the default branch unless opts.Push is false.
//
// On any failure, CreateRepo returns a typed *Error carrying the relevant
// code and the failing tool's combined output. The local working tree is
// left in whatever state the failure produced — callers should NOT delete
// the rendered project on a publish error; the user can re-run from the
// shell.
func CreateRepo(ctx context.Context, r Runner, opts CreateOpts) (CreateResult, error) {
	if err := validate(opts); err != nil {
		return CreateResult{}, err
	}
	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}
	commitMsg := opts.CommitMsg
	if commitMsg == "" {
		commitMsg = "Initial commit (folio)"
	}

	if err := ensureGitRepo(ctx, r, opts.TargetDir, branch); err != nil {
		return CreateResult{}, err
	}
	if err := ensureInitialCommit(ctx, r, opts.TargetDir, commitMsg); err != nil {
		return CreateResult{}, err
	}

	args := []string{
		"repo", "create",
		fmt.Sprintf("%s/%s", opts.Owner, opts.Repo),
		"--" + string(opts.Visibility),
		"--source=.",
		"--remote=origin",
	}
	if opts.Description != "" {
		args = append(args, "--description", opts.Description)
	}
	if opts.Push {
		args = append(args, "--push")
	}
	out, err := r.Run(ctx, opts.TargetDir, "gh", args...)
	if err != nil {
		code := ErrPublishFailed
		if strings.Contains(string(out), "Name already exists on this account") ||
			strings.Contains(string(out), "name already exists") {
			code = ErrRepoExists
		}
		return CreateResult{}, newErr(code, "gh repo create", out, err)
	}

	return CreateResult{
		URL:    fmt.Sprintf("https://github.com/%s/%s", opts.Owner, opts.Repo),
		Owner:  opts.Owner,
		Repo:   opts.Repo,
		Branch: branch,
		Pushed: opts.Push,
	}, nil
}

func validate(opts CreateOpts) error {
	if opts.TargetDir == "" {
		return newErr(ErrPublishFailed, "validate", nil, fmt.Errorf("target dir is empty"))
	}
	if _, err := os.Stat(opts.TargetDir); err != nil {
		return newErr(ErrPublishFailed, "validate", nil, fmt.Errorf("target dir: %w", err))
	}
	if opts.Owner == "" {
		return newErr(ErrPublishFailed, "validate", nil, fmt.Errorf("owner is empty"))
	}
	if opts.Repo == "" {
		return newErr(ErrPublishFailed, "validate", nil, fmt.Errorf("repo is empty"))
	}
	if !opts.Visibility.Valid() {
		return newErr(ErrPublishFailed, "validate", nil, fmt.Errorf("invalid visibility %q", opts.Visibility))
	}
	return nil
}

// ensureGitRepo runs `git init -b <branch>` if .git is absent; no-op if it
// already exists. We use -b to set the initial-branch name so we don't
// inherit whatever the user's `init.defaultBranch` config is.
func ensureGitRepo(ctx context.Context, r Runner, dir, branch string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	out, err := r.Run(ctx, dir, "git", "init", "-b", branch)
	if err != nil {
		return newErr(ErrPublishFailed, "git init", out, err)
	}
	return nil
}

// ensureInitialCommit checks `git rev-parse HEAD` — if it succeeds the
// repo already has commits and we're done. Otherwise we stage everything
// and commit. The commit is created with --allow-empty disabled (default);
// folio always writes at least one file, so this should never produce an
// empty-commit failure.
func ensureInitialCommit(ctx context.Context, r Runner, dir, msg string) error {
	if _, err := r.Run(ctx, dir, "git", "rev-parse", "HEAD"); err == nil {
		return nil
	}
	if out, err := r.Run(ctx, dir, "git", "add", "."); err != nil {
		return newErr(ErrPublishFailed, "git add", out, err)
	}
	if out, err := r.Run(ctx, dir, "git", "commit", "-m", msg); err != nil {
		return newErr(ErrPublishFailed, "git commit", out, err)
	}
	return nil
}
