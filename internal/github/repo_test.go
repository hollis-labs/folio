package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateRepo_ValidatesOpts(t *testing.T) {
	td := t.TempDir()
	cases := []struct {
		name string
		opts CreateOpts
	}{
		{"missing target", CreateOpts{Owner: "o", Repo: "r", Visibility: VisibilityPrivate}},
		{"missing owner", CreateOpts{TargetDir: td, Repo: "r", Visibility: VisibilityPrivate}},
		{"missing repo", CreateOpts{TargetDir: td, Owner: "o", Visibility: VisibilityPrivate}},
		{"invalid visibility", CreateOpts{TargetDir: td, Owner: "o", Repo: "r", Visibility: ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CreateRepo(context.Background(), &fakeRunner{}, c.opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestCreateRepo_PublishesAndPushes(t *testing.T) {
	td := t.TempDir()
	if err := os.WriteFile(filepath.Join(td, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := &fakeRunner{script: map[string]fakeResponse{
		"git rev-parse HEAD": {err: errors.New("fatal: ambiguous argument 'HEAD'")},
	}}
	res, err := CreateRepo(context.Background(), r, CreateOpts{
		TargetDir:   td,
		Owner:       "hollis-labs",
		Repo:        "myproj",
		Visibility:  VisibilityPrivate,
		Description: "the project",
		Branch:      "main",
		Push:        true,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.URL != "https://github.com/hollis-labs/myproj" {
		t.Errorf("URL = %q", res.URL)
	}
	if !res.Pushed {
		t.Error("expected Pushed=true")
	}

	// Expected call order: git init, git rev-parse HEAD, git add, git commit, gh repo create
	wantPrefixes := []string{
		"git init -b main",
		"git rev-parse HEAD",
		"git add .",
		"git commit -m",
		"gh repo create hollis-labs/myproj --private --source=. --remote=origin --description the project --push",
	}
	if len(r.calls) != len(wantPrefixes) {
		t.Fatalf("call count = %d, want %d (calls: %v)", len(r.calls), len(wantPrefixes), callStrings(r.calls))
	}
	for i, want := range wantPrefixes {
		got := r.calls[i].name + " " + strings.Join(r.calls[i].args, " ")
		got = strings.TrimRight(got, " ")
		if !strings.HasPrefix(got, want) {
			t.Errorf("call %d: %q does not start with %q", i, got, want)
		}
	}
}

func TestCreateRepo_RepoExistsClassified(t *testing.T) {
	td := t.TempDir()
	r := &fakeRunner{script: map[string]fakeResponse{
		"gh repo create hollis-labs/myproj --private --source=. --remote=origin --push": {
			out: []byte("HTTP 422: Name already exists on this account"),
			err: errors.New("exit status 1"),
		},
		"git rev-parse HEAD": {err: errors.New("not a repo")},
	}}
	_, err := CreateRepo(context.Background(), r, CreateOpts{
		TargetDir:  td,
		Owner:      "hollis-labs",
		Repo:       "myproj",
		Visibility: VisibilityPrivate,
		Branch:     "main",
		Push:       true,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var gerr *Error
	if !errors.As(err, &gerr) {
		t.Fatalf("expected *github.Error, got %T", err)
	}
	if gerr.Code != ErrRepoExists {
		t.Fatalf("expected %q, got %q", ErrRepoExists, gerr.Code)
	}
}

func TestCreateRepo_NoPushOmitsFlag(t *testing.T) {
	td := t.TempDir()
	r := &fakeRunner{script: map[string]fakeResponse{
		"git rev-parse HEAD": {err: errors.New("not a repo")},
	}}
	res, err := CreateRepo(context.Background(), r, CreateOpts{
		TargetDir:  td,
		Owner:      "o",
		Repo:       "r",
		Visibility: VisibilityPublic,
		Branch:     "main",
		Push:       false,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.Pushed {
		t.Error("expected Pushed=false")
	}
	last := r.calls[len(r.calls)-1]
	for _, a := range last.args {
		if a == "--push" {
			t.Errorf("--push should not be present, got args %v", last.args)
		}
	}
}

func TestCreateRepo_SkipsGitInitWhenAlreadyRepo(t *testing.T) {
	td := t.TempDir()
	if err := os.Mkdir(filepath.Join(td, ".git"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := &fakeRunner{}
	_, err := CreateRepo(context.Background(), r, CreateOpts{
		TargetDir:  td,
		Owner:      "o",
		Repo:       "r",
		Visibility: VisibilityPrivate,
		Push:       false,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	for _, c := range r.calls {
		if c.name == "git" && len(c.args) > 0 && c.args[0] == "init" {
			t.Errorf("git init should be skipped when .git/ exists")
		}
	}
}

func TestCreateRepo_SkipsCommitWhenHistoryExists(t *testing.T) {
	td := t.TempDir()
	if err := os.Mkdir(filepath.Join(td, ".git"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := &fakeRunner{
		// git rev-parse HEAD succeeds → commits already exist
		script: map[string]fakeResponse{},
	}
	_, err := CreateRepo(context.Background(), r, CreateOpts{
		TargetDir:  td,
		Owner:      "o",
		Repo:       "r",
		Visibility: VisibilityPrivate,
		Push:       false,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	for _, c := range r.calls {
		if c.name == "git" && len(c.args) > 0 && (c.args[0] == "add" || c.args[0] == "commit") {
			t.Errorf("git %s should be skipped when HEAD exists, got call %v", c.args[0], c)
		}
	}
}

func TestVisibility_Valid(t *testing.T) {
	cases := map[Visibility]bool{
		VisibilityPublic:    true,
		VisibilityPrivate:   true,
		VisibilityInternal:  true,
		Visibility(""):      false,
		Visibility("bogus"): false,
	}
	for v, want := range cases {
		if got := v.Valid(); got != want {
			t.Errorf("%q.Valid() = %v, want %v", v, got, want)
		}
	}
}

func callStrings(cs []fakeCall) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.name + " " + strings.Join(c.args, " ")
	}
	return out
}
