package manifest_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio/internal/manifest"
)

func TestDigest_StableForKnownContent(t *testing.T) {
	content := []byte("hello world\n")
	want := "sha256:a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
	if got := manifest.Digest(content); got != want {
		t.Errorf("digest = %s, want %s", got, want)
	}
}

func TestDigest_LFNormalization(t *testing.T) {
	lf := []byte("line1\nline2\nline3\n")
	crlf := []byte("line1\r\nline2\r\nline3\r\n")
	cr := []byte("line1\rline2\rline3\r")
	want := manifest.Digest(lf)
	if got := manifest.Digest(crlf); got != want {
		t.Errorf("CRLF digest = %s, want %s (LF baseline)", got, want)
	}
	if got := manifest.Digest(cr); got != want {
		t.Errorf("CR digest = %s, want %s (LF baseline)", got, want)
	}
}

func TestDigest_FastPathNoCR(t *testing.T) {
	// Repeated calls on identical LF-only input return identical results
	// (this also exercises the no-CR fast path inside NormalizeLineEndings).
	in := []byte("alpha beta gamma")
	if a, b := manifest.Digest(in), manifest.Digest(in); a != b {
		t.Errorf("digest not deterministic: %s vs %s", a, b)
	}
}

func sampleManifest() manifest.Manifest {
	return manifest.Manifest{
		FolioVersion: "0.1",
		GeneratedAt:  time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
		Generator:    "folio/0.1.0",
		Presets: []manifest.PresetRef{{
			ID:           "base",
			Version:      "1.0.0",
			Source:       "local",
			ResolvedPath: "/Users/chrispian/.folio/presets/local/base@1.0.0",
			Digest:       "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		}},
		Inputs: map[string]any{
			"project_name": "smoke_test",
			"github_owner": "chrispian",
			"description":  "folio v0 smoke",
		},
		Computed: map[string]any{
			"module_path": "github.com/chrispian/smoke_test",
			"year":        2026,
		},
		Files: map[string]manifest.FileRecord{
			"README.md": {Preset: "base", DigestAtGen: manifest.Digest([]byte("# Smoke\n"))},
			"go.mod":    {Preset: "base", DigestAtGen: manifest.Digest([]byte("module github.com/chrispian/smoke_test\n"))},
			"Makefile":  {Preset: "base", DigestAtGen: manifest.Digest([]byte(".PHONY: build\nbuild:\n\tgo build ./...\n"))},
		},
		SyncHistory: []manifest.SyncEvent{{
			At:        time.Date(2026, 5, 12, 15, 30, 0, 0, time.UTC),
			Operation: "init",
			Presets:   []manifest.PresetRef{{ID: "base", Version: "1.0.0"}},
		}},
	}
}

func TestWrite_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := sampleManifest()
	if err := manifest.Write(dir, m); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := manifest.Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.FolioVersion != m.FolioVersion {
		t.Errorf("folio_version = %q, want %q", got.FolioVersion, m.FolioVersion)
	}
	if got.Generator != m.Generator {
		t.Errorf("generator = %q, want %q", got.Generator, m.Generator)
	}
	if !got.GeneratedAt.Equal(m.GeneratedAt) {
		t.Errorf("generated_at = %v, want %v", got.GeneratedAt, m.GeneratedAt)
	}
	if len(got.Presets) != 1 || got.Presets[0].ID != "base" {
		t.Errorf("presets mismatch: %+v", got.Presets)
	}
	if got.Inputs["project_name"] != "smoke_test" {
		t.Errorf("inputs.project_name = %v, want smoke_test", got.Inputs["project_name"])
	}
	if len(got.Files) != 3 {
		t.Errorf("files len = %d, want 3", len(got.Files))
	}
	if got.Files["README.md"].DigestAtGen != m.Files["README.md"].DigestAtGen {
		t.Errorf("README.md digest_at_gen drift after round-trip")
	}
}

func TestWrite_Idempotent(t *testing.T) {
	dir := t.TempDir()
	m := sampleManifest()

	first, err := manifest.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal first: %v", err)
	}
	second, err := manifest.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal second: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("Marshal is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	if err := manifest.Write(dir, m); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if err := manifest.Write(dir, m); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
}

func TestWrite_DeterministicMapOrder(t *testing.T) {
	dir := t.TempDir()
	m := sampleManifest()
	if err := manifest.Write(dir, m); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifest.ManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	// Map keys are alphabetised by yaml.v3 — README.md must come before
	// go.mod in the files block. (capital R < lowercase g in ASCII).
	body := string(data)
	rIdx := strings.Index(body, "README.md")
	mfIdx := strings.Index(body, "Makefile")
	gIdx := strings.Index(body, "go.mod")
	if rIdx < 0 || mfIdx < 0 || gIdx < 0 {
		t.Fatalf("expected all three filenames in output; got:\n%s", body)
	}
	if mfIdx >= rIdx || rIdx >= gIdx {
		t.Errorf("file map keys not in sorted order; Makefile@%d README.md@%d go.mod@%d", mfIdx, rIdx, gIdx)
	}
}

func TestRead_Missing(t *testing.T) {
	dir := t.TempDir()
	if _, err := manifest.Read(dir); err == nil {
		t.Fatal("expected error for missing manifest")
	}
}
