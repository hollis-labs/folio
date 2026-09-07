package github

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// fakeRunner records every call and replays scripted responses keyed by
// the argv joined with spaces. Missing keys return (nil, nil) — success.
type fakeRunner struct {
	calls  []fakeCall
	script map[string]fakeResponse
}

type fakeCall struct {
	dir  string
	name string
	args []string
}

type fakeResponse struct {
	out []byte
	err error
}

func (f *fakeRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{dir: dir, name: name, args: append([]string{}, args...)})
	parts := append([]string{name}, args...)
	key := strings.Join(parts, " ")
	if r, ok := f.script[key]; ok {
		return r.out, r.err
	}
	return nil, nil
}

func TestPreflight_MissingGH(t *testing.T) {
	r := &fakeRunner{script: map[string]fakeResponse{
		"gh --version": {err: &exec.Error{Name: "gh", Err: exec.ErrNotFound}},
	}}
	err := Preflight(context.Background(), r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var gerr *Error
	if !errors.As(err, &gerr) {
		t.Fatalf("expected *github.Error, got %T", err)
	}
	if gerr.Code != ErrGHMissing {
		t.Fatalf("expected code %q, got %q", ErrGHMissing, gerr.Code)
	}
}

func TestPreflight_MissingGit(t *testing.T) {
	r := &fakeRunner{script: map[string]fakeResponse{
		"git --version": {err: &exec.Error{Name: "git", Err: exec.ErrNotFound}},
	}}
	err := Preflight(context.Background(), r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var gerr *Error
	if !errors.As(err, &gerr) {
		t.Fatalf("expected *github.Error, got %T", err)
	}
	if gerr.Code != ErrGitMissing {
		t.Fatalf("expected code %q, got %q", ErrGitMissing, gerr.Code)
	}
}

func TestPreflight_NotAuthenticated(t *testing.T) {
	r := &fakeRunner{script: map[string]fakeResponse{
		"gh auth status": {
			out: []byte("You are not logged into any GitHub hosts. To log in, run: gh auth login"),
			err: errors.New("exit status 1"),
		},
	}}
	err := Preflight(context.Background(), r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var gerr *Error
	if !errors.As(err, &gerr) {
		t.Fatalf("expected *github.Error, got %T", err)
	}
	if gerr.Code != ErrGHUnauth {
		t.Fatalf("expected code %q, got %q", ErrGHUnauth, gerr.Code)
	}
}

func TestPreflight_OK(t *testing.T) {
	r := &fakeRunner{}
	if err := Preflight(context.Background(), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"gh --version", "git --version", "gh auth status"}
	if len(r.calls) != len(want) {
		t.Fatalf("expected %d calls, got %d", len(want), len(r.calls))
	}
	for i, c := range r.calls {
		parts := append([]string{c.name}, c.args...)
		got := strings.Join(parts, " ")
		if got != want[i] {
			t.Errorf("call %d: got %q, want %q", i, got, want[i])
		}
	}
}
