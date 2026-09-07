package folio_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	folio "github.com/hollis-labs/folio"
	"github.com/hollis-labs/folio/service"
)

// TestIntegration_PublishToGitHub_E2E creates a real throwaway GitHub repo,
// pushes the initial commit, then deletes it. Gated on FOLIO_GH_E2E=1
// because it requires:
//   - `gh` CLI installed and authenticated
//   - Network access to github.com
//   - The current `gh` user having repo-create + repo-delete permissions
//     on the configured owner (env FOLIO_GH_E2E_OWNER, default = the
//     authenticated user as `gh` reports it).
//
// The test always tries to delete the repo on teardown — even on test
// failure — so a flake doesn't leave debris.
func TestIntegration_PublishToGitHub_E2E(t *testing.T) {
	if os.Getenv("FOLIO_GH_E2E") != "1" {
		t.Skip("FOLIO_GH_E2E not set; skipping live GitHub round-trip")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not on PATH; skipping")
	}

	owner := os.Getenv("FOLIO_GH_E2E_OWNER")
	if owner == "" {
		// `gh api user --jq .login` resolves the authenticated user.
		out, err := exec.Command("gh", "api", "user", "--jq", ".login").Output()
		if err != nil {
			t.Fatalf("resolve owner via gh api user: %v", err)
		}
		owner = string(out)
		// Trim trailing newline.
		for len(owner) > 0 && (owner[len(owner)-1] == '\n' || owner[len(owner)-1] == '\r') {
			owner = owner[:len(owner)-1]
		}
	}
	if owner == "" {
		t.Fatal("could not resolve a GitHub owner; set FOLIO_GH_E2E_OWNER")
	}

	repoName := fmt.Sprintf("folio-e2e-%d", time.Now().UnixNano())
	target := filepath.Join(t.TempDir(), repoName)

	svc := service.New(service.Options{
		BundledFS:    folio.BundledPresets,
		BundledRoot:  "presets",
		FolioVersion: "0.2.0-e2e",
	})
	if _, err := svc.New(service.NewOptions{
		PresetID:  "base",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "folio_e2e",
			"github_owner": owner,
			"description":  "Folio E2E throwaway",
		},
	}); err != nil {
		t.Fatalf("render: %v", err)
	}

	// Always attempt cleanup, even on test failure mid-flight.
	cleanedUp := false
	defer func() {
		if cleanedUp {
			return
		}
		_ = exec.Command("gh", "repo", "delete",
			fmt.Sprintf("%s/%s", owner, repoName), "--yes").Run()
	}()

	res, err := svc.PublishToGitHub(context.Background(), service.PublishOptions{
		TargetDir:   target,
		Owner:       owner,
		Repo:        repoName,
		Visibility:  "private",
		Description: "Folio E2E throwaway — safe to delete",
		Branch:      "main",
		Push:        true,
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if res.URL == "" || res.URL != fmt.Sprintf("https://github.com/%s/%s", owner, repoName) {
		t.Errorf("URL = %q, want https://github.com/%s/%s", res.URL, owner, repoName)
	}
	if !res.Pushed {
		t.Error("expected Pushed=true")
	}

	// Verify the repo actually exists on GitHub.
	if err := exec.Command("gh", "repo", "view",
		fmt.Sprintf("%s/%s", owner, repoName)).Run(); err != nil {
		t.Errorf("gh repo view after publish: %v", err)
	}

	// Clean up explicitly — defer only runs if we didn't.
	if err := exec.Command("gh", "repo", "delete",
		fmt.Sprintf("%s/%s", owner, repoName), "--yes").Run(); err != nil {
		t.Logf("cleanup delete: %v (deferred fallback will retry)", err)
	} else {
		cleanedUp = true
	}
}
