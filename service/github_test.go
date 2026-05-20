package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gh "github.com/hollis-labs/folio/internal/github"
)

// recRunner is a tiny gh.Runner that records calls and returns scripted
// (out, err) per fully-joined argv. Kept inline to avoid pulling in the
// internal-package test helper.
type recRunner struct {
	script map[string]struct {
		out []byte
		err error
	}
}

func (r *recRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	parts := append([]string{name}, args...)
	key := strings.Join(parts, " ")
	if r, ok := r.script[key]; ok {
		return r.out, r.err
	}
	return nil, nil
}

func TestPublishToGitHub_ValidatesOpts(t *testing.T) {
	svc := New(Options{})
	cases := []struct {
		name string
		opts PublishOptions
		want ErrorCode
	}{
		{"missing target", PublishOptions{Owner: "o", Repo: "r", Visibility: "private"}, ErrInputInvalid},
		{"missing owner", PublishOptions{TargetDir: "/tmp", Repo: "r", Visibility: "private"}, ErrInputMissing},
		{"missing repo", PublishOptions{TargetDir: "/tmp", Owner: "o", Visibility: "private"}, ErrInputMissing},
		{"missing visibility", PublishOptions{TargetDir: "/tmp", Owner: "o", Repo: "r"}, ErrInputInvalid},
		{"bad visibility", PublishOptions{TargetDir: "/tmp", Owner: "o", Repo: "r", Visibility: "bogus"}, ErrInputInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.PublishToGitHub(context.Background(), c.opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var sErr *Error
			if !errors.As(err, &sErr) {
				t.Fatalf("expected *service.Error, got %T", err)
			}
			if sErr.Code != c.want {
				t.Errorf("code = %q, want %q", sErr.Code, c.want)
			}
		})
	}
}

func TestPublishToGitHub_OK(t *testing.T) {
	td := t.TempDir()
	if err := os.WriteFile(filepath.Join(td, "README.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &recRunner{script: map[string]struct {
		out []byte
		err error
	}{
		"git rev-parse HEAD": {err: errors.New("not a repo yet")},
	}}
	svc := New(Options{})
	res, err := svc.PublishToGitHub(context.Background(), PublishOptions{
		TargetDir:   td,
		Owner:       "hollis-labs",
		Repo:        "glyph",
		Visibility:  "private",
		Description: "Glyph: content authoring",
		Branch:      "main",
		Push:        true,
		Runner:      r,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.URL != "https://github.com/hollis-labs/glyph" {
		t.Errorf("URL = %q", res.URL)
	}
	if !res.Pushed {
		t.Error("expected Pushed=true")
	}
}

func TestPublishToGitHub_PropagatesPreflightFailure(t *testing.T) {
	td := t.TempDir()
	r := &recRunner{script: map[string]struct {
		out []byte
		err error
	}{
		"gh auth status": {
			out: []byte("You are not logged into any GitHub hosts"),
			err: errors.New("exit status 1"),
		},
	}}
	svc := New(Options{})
	_, err := svc.PublishToGitHub(context.Background(), PublishOptions{
		TargetDir:  td,
		Owner:      "o",
		Repo:       "r",
		Visibility: "private",
		Runner:     r,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var sErr *Error
	if !errors.As(err, &sErr) {
		t.Fatalf("expected *service.Error, got %T", err)
	}
	if sErr.Code != ErrorCode(gh.ErrGHUnauth) {
		t.Errorf("code = %q, want %q", sErr.Code, gh.ErrGHUnauth)
	}
}

func TestPublishErrorCode(t *testing.T) {
	err := &Error{Code: ErrorCode(gh.ErrRepoExists), Message: "boom"}
	if got := PublishErrorCode(err); got != ErrorCode(gh.ErrRepoExists) {
		t.Errorf("got %q, want %q", got, gh.ErrRepoExists)
	}
	if got := PublishErrorCode(errors.New("plain")); got != "" {
		t.Errorf("plain err: got %q, want empty", got)
	}
}
