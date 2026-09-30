package service_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/service"
)

// presetTree is a throwaway preset directory the tests can edit between
// generating a project and inspecting it, which is how a preset "updates".
type presetTree struct {
	t    *testing.T
	root string
}

func newPresetTree(t *testing.T, id, version, extraYAML string, files map[string]string) *presetTree {
	t.Helper()
	pt := &presetTree{t: t, root: t.TempDir()}
	pt.write(id+"/preset.yaml", "folio_version: \"0.1\"\nid: "+id+"\nversion: "+version+"\ndescription: inspect test\nauthor: t\n"+extraYAML+"files:\n  source: ./files\n  template_suffix: .tmpl\n")
	for p, body := range files {
		pt.write(id+"/files/"+p, body)
	}
	return pt
}

func (pt *presetTree) write(rel, body string) {
	pt.t.Helper()
	full := filepath.Join(pt.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		pt.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		pt.t.Fatal(err)
	}
}

func (pt *presetTree) remove(rel string) {
	pt.t.Helper()
	if err := os.Remove(filepath.Join(pt.root, filepath.FromSlash(rel))); err != nil {
		pt.t.Fatal(err)
	}
}

func (pt *presetTree) service(now time.Time, folioVersion string) *service.Service {
	return service.New(service.Options{
		BundledFS:    os.DirFS(pt.root),
		BundledRoot:  ".",
		UserDir:      pt.t.TempDir(),
		FolioVersion: folioVersion,
		Now:          now,
	})
}

func generate(t *testing.T, svc *service.Service, preset string, inputs map[string]any) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "proj")
	if _, err := svc.New(service.NewOptions{PresetID: preset, TargetDir: target, Inputs: inputs}); err != nil {
		t.Fatalf("New: %v", err)
	}
	return target
}

func inspect(t *testing.T, svc *service.Service, target string) service.InspectResult {
	t.Helper()
	res, err := svc.Inspect(service.InspectOptions{TargetDir: target})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	return res
}

func statusOf(t *testing.T, res service.InspectResult, path string) service.FileDrift {
	t.Helper()
	for _, f := range res.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no row for %s in %+v", path, res.Files)
	return service.FileDrift{}
}

func wantStatus(t *testing.T, res service.InspectResult, path string, want service.FileDriftStatus) {
	t.Helper()
	if got := statusOf(t, res, path).Status; got != want {
		t.Errorf("%s status = %s, want %s", path, got, want)
	}
}

var tenAM = time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

func basicTree(t *testing.T) *presetTree {
	return newPresetTree(t, "pp", "1.0.0", "inputs:\n  - name: name\n    type: string\n    default: demo\n", map[string]string{
		"README.md.tmpl": "# {{ .inputs.name }}\n",
		"go.mod.tmpl":    "module {{ .inputs.name }}\n",
		"NOTES.txt":      "static\n",
	})
}

func TestInspect_UnchangedProject_AllUnchanged(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})

	res := inspect(t, svc, target)
	if res.RootPreset != "pp" || res.RootVersion != "1.0.0" {
		t.Errorf("root = %s@%s", res.RootPreset, res.RootVersion)
	}
	if len(res.Files) != 3 {
		t.Fatalf("files = %+v", res.Files)
	}
	for _, f := range res.Files {
		if f.Status != service.DriftUnchanged || f.Unstable {
			t.Errorf("%+v", f)
		}
	}
	if len(res.Warnings) != 0 {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

func TestInspect_LocallyModifiedFile_Detected(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("# edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := inspect(t, svc, target)
	wantStatus(t, res, "README.md", service.DriftLocallyModified)
	wantStatus(t, res, "go.mod", service.DriftUnchanged)
}

func TestInspect_PresetUpdatedAndConflict(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})

	pt.write("pp/files/README.md.tmpl", "# {{ .inputs.name }} v2\n")
	pt.write("pp/files/go.mod.tmpl", "module {{ .inputs.name }}\n\ngo 1.26\n")
	if err := os.WriteFile(filepath.Join(target, "go.mod"), []byte("module mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := inspect(t, svc, target)
	wantStatus(t, res, "README.md", service.DriftPresetUpdated)
	wantStatus(t, res, "go.mod", service.DriftConflict)
	wantStatus(t, res, "NOTES.txt", service.DriftUnchanged)
}

func TestInspect_AddedAndRemovedUpstream(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})

	pt.remove("pp/files/NOTES.txt")
	pt.write("pp/files/CONTRIBUTING.md.tmpl", "hi\n")

	res := inspect(t, svc, target)
	wantStatus(t, res, "NOTES.txt", service.DriftRemovedUpstream)
	wantStatus(t, res, "CONTRIBUTING.md", service.DriftAddedUpstream)
	wantStatus(t, res, "README.md", service.DriftUnchanged)
}

func TestInspect_MissingTrackedFile_ReportsMissingLocally(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})
	if err := os.Remove(filepath.Join(target, "go.mod")); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, inspect(t, svc, target), "go.mod", service.DriftMissingLocally)
}

func TestInspect_CRLFOnDisk_IsNotLocalModification(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte("# demo\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, inspect(t, svc, target), "README.md", service.DriftUnchanged)
}

func TestInspect_ManifestProblems(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")

	t.Run("missing manifest", func(t *testing.T) {
		_, err := svc.Inspect(service.InspectOptions{TargetDir: t.TempDir()})
		assertCode(t, err, service.ErrManifestNotFound)
	})
	t.Run("no target", func(t *testing.T) {
		_, err := svc.Inspect(service.InspectOptions{})
		assertCode(t, err, service.ErrInputInvalid)
	})
	t.Run("malformed manifest", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, manifest.ManifestFilename), []byte("presets: [unterminated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := svc.Inspect(service.InspectOptions{TargetDir: dir})
		assertCode(t, err, service.ErrManifestInvalid)
	})
	t.Run("no presets recorded", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, manifest.ManifestFilename), []byte("folio_version: \"0.1\"\npresets: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := svc.Inspect(service.InspectOptions{TargetDir: dir})
		assertCode(t, err, service.ErrManifestInvalid)
	})
	t.Run("preset no longer shipped", func(t *testing.T) {
		target := generate(t, svc, "pp", map[string]any{"name": "demo"})
		gone := newPresetTree(t, "other", "1.0.0", "", map[string]string{"x.txt": "x\n"})
		_, err := gone.service(tenAM, "1.0.0").Inspect(service.InspectOptions{TargetDir: target})
		assertCode(t, err, service.ErrPresetNotFound)
	})
}

func assertCode(t *testing.T, err error, want service.ErrorCode) {
	t.Helper()
	var se *service.Error
	if !errors.As(err, &se) || se.Code != want {
		t.Fatalf("err = %v, want code %s", err, want)
	}
}

func TestInspect_NewRequiredInputSincePreset_ReturnsClearError(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})

	pt.write("pp/preset.yaml", "folio_version: \"0.1\"\nid: pp\nversion: 1.1.0\ndescription: d\nauthor: t\ninputs:\n  - name: name\n    type: string\n    default: demo\n  - name: owner\n    type: string\n    required: true\nfiles:\n  source: ./files\n  template_suffix: .tmpl\n")
	_, err := svc.Inspect(service.InspectOptions{TargetDir: target})
	assertCode(t, err, service.ErrDriftUnverifiable)
	if !strings.Contains(err.Error(), "cannot verify") || !strings.Contains(err.Error(), "owner") {
		t.Errorf("error should say what cannot be verified and why: %v", err)
	}
}

func TestInspect_PresetVersionChange_Warns(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})
	pt.write("pp/preset.yaml", strings.Replace(readFile(t, filepath.Join(pt.root, "pp/preset.yaml")), "version: 1.0.0", "version: 1.2.0", 1))

	res := inspect(t, svc, target)
	if res.RootVersion != "1.0.0" {
		t.Errorf("RootVersion should be the generated version, got %s", res.RootVersion)
	}
	if !hasWarning(res, "generated at 1.0.0", "ships 1.2.0") {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

func hasWarning(res service.InspectResult, parts ...string) bool {
next:
	for _, w := range res.Warnings {
		for _, p := range parts {
			if !strings.Contains(w, p) {
				continue next
			}
		}
		return true
	}
	return false
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const yearComputed = "computed:\n  year: \"{{ .now.Year }}\"\n"

// The calendar-year trap: LICENSE reads .now.Year through a computed value.
// Rendering with today's clock would call LICENSE preset_updated the day the
// year turns; Inspect replays the clock recorded at generation instead.
func TestInspect_YearRollover_NoFalseDrift(t *testing.T) {
	pt := newPresetTree(t, "pp", "1.0.0", yearComputed, map[string]string{
		"LICENSE.tmpl": "Copyright (c) {{ .computed.year }}\n",
		"NOTES.txt":    "static\n",
	})
	gen := pt.service(time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC), "1.0.0")
	target := generate(t, gen, "pp", nil)

	later := pt.service(time.Date(2027, 2, 3, 4, 5, 6, 0, time.UTC), "1.0.0")
	res := inspect(t, later, target)
	wantStatus(t, res, "LICENSE", service.DriftUnchanged)
	if len(res.Warnings) != 0 {
		t.Errorf("no warning expected: %v", res.Warnings)
	}
}

// The recorded generation time keeps its UTC offset, so the year it replays is
// the one the LICENSE was written with even when that differs from UTC's.
func TestInspect_YearRollover_RespectsRecordedOffset(t *testing.T) {
	pt := newPresetTree(t, "pp", "1.0.0", yearComputed, map[string]string{
		"LICENSE.tmpl": "Copyright (c) {{ .computed.year }}\n",
	})
	west := time.FixedZone("west", -7*3600)
	// 2025-12-31 20:00 in the west zone is already 2026-01-01 in UTC.
	gen := pt.service(time.Date(2025, 12, 31, 20, 0, 0, 0, west), "1.0.0")
	target := generate(t, gen, "pp", nil)
	if got := readFile(t, filepath.Join(target, "LICENSE")); !strings.Contains(got, "2025") {
		t.Fatalf("setup: LICENSE = %q", got)
	}
	res := inspect(t, pt.service(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), "1.0.0"), target)
	wantStatus(t, res, "LICENSE", service.DriftUnchanged)
}

// A template that prints the folio version must not report drift when the
// binary was upgraded after the project was generated.
func TestInspect_FolioVersionReplayed(t *testing.T) {
	pt := newPresetTree(t, "pp", "1.0.0", "", map[string]string{
		"STAMP.txt.tmpl": "made with folio {{ .folio.version }}\n",
	})
	target := generate(t, pt.service(tenAM, "0.3.0"), "pp", nil)
	wantStatus(t, inspect(t, pt.service(tenAM, "9.9.9"), target), "STAMP.txt", service.DriftUnchanged)
}

// Genuinely random output cannot be replayed. Such a file is flagged, and its
// preset side is not compared, so it is never reported as preset_updated.
func TestInspect_NonDeterministicFile_IsFlaggedNotFalselyDrifted(t *testing.T) {
	pt := newPresetTree(t, "pp", "1.0.0", "", map[string]string{
		"ID.txt.tmpl": "{{ uuid }}\n",
		"NOTES.txt":   "static\n",
	})
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", nil)

	res := inspect(t, svc, target)
	row := statusOf(t, res, "ID.txt")
	if !row.Unstable || row.Status != service.DriftUnchanged {
		t.Errorf("ID.txt = %+v, want unstable and unchanged", row)
	}
	if statusOf(t, res, "NOTES.txt").Unstable {
		t.Error("static file must not be flagged")
	}
	if !hasWarning(res, "ID.txt", "every run") {
		t.Errorf("warnings = %v", res.Warnings)
	}

	if err := os.WriteFile(filepath.Join(target, "ID.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, inspect(t, svc, target), "ID.txt", service.DriftLocallyModified)
}

func TestInspect_ComposedPreset_AttributesFilesToOwningLayer(t *testing.T) {
	svc := newTestService(t)
	target := filepath.Join(t.TempDir(), "out")
	if _, err := svc.New(service.NewOptions{
		PresetID:  "composer",
		TargetDir: target,
		Inputs: map[string]any{
			"project_name": "smoke_compose",
			"github_owner": "chrispian",
			"description":  "compose smoke",
			"package_name": "greeter",
		},
	}); err != nil {
		t.Fatal(err)
	}
	res := inspect(t, svc, target)
	if res.RootPreset != "composer" {
		t.Errorf("root = %s", res.RootPreset)
	}
	owners := map[string]string{
		"go.mod":                      "sample",
		"README.md":                   "composer", // the composer overwrites sample's
		"internal/greeter/greeter.go": "composer",
	}
	for path, owner := range owners {
		row := statusOf(t, res, path)
		if row.Preset != owner {
			t.Errorf("%s owned by %q, want %q", path, row.Preset, owner)
		}
		if row.Status != service.DriftUnchanged {
			t.Errorf("%s = %s, want unchanged", path, row.Status)
		}
	}
}

func TestInspect_TrackedPathThatIsNowADirectory_ReportsReadFailure(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})
	if err := os.Remove(filepath.Join(target, "NOTES.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(target, "NOTES.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Inspect(service.InspectOptions{TargetDir: target})
	assertCode(t, err, service.ErrReadFailed)
}

// Inspect is read-only: not one byte or entry of the project changes,
// including .folio.yaml.
func TestInspect_WritesNothing(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})
	pt.write("pp/files/README.md.tmpl", "changed\n")
	if err := os.WriteFile(filepath.Join(target, "go.mod"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := snapshot(t, target)
	inspect(t, svc, target)
	after := snapshot(t, target)
	if before != after {
		t.Fatalf("Inspect changed the project:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		sum := ""
		if !d.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum = fmt.Sprintf("%x", sha256.Sum256(b))
		}
		lines = append(lines, fmt.Sprintf("%s %v %d %s %s", rel, d.IsDir(), info.Size(), info.ModTime().Format(time.RFC3339Nano), sum))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// .folio.yaml is written by whoever owns the project. Paths in it that leave
// the project are skipped with a warning and never read; a symlink inside the
// project that points out is refused too. The outside file below has exactly
// the content (and so the digest) the manifest claims, so following the link
// would be visible as an "unchanged" row.
func TestInspect_ManifestPathsCannotEscapeTheProject(t *testing.T) {
	pt := basicTree(t)
	svc := pt.service(tenAM, "1.0.0")
	target := generate(t, svc, "pp", map[string]any{"name": "demo"})

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(target, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(target, "README.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	m, err := manifest.Read(target)
	if err != nil {
		t.Fatal(err)
	}
	digest := m.Files["README.md"].DigestAtGen
	for _, bad := range []string{"../outside.txt", "/etc/hostname", "a/../../x", "", ".", "sub//x"} {
		m.Files[bad] = manifest.FileRecord{Preset: "pp", DigestAtGen: digest}
	}
	if err := manifest.Write(target, m); err != nil {
		t.Fatal(err)
	}

	res := inspect(t, svc, target)
	for _, f := range res.Files {
		switch f.Path {
		case "README.md":
			t.Errorf("README.md is a symlink out of the project and must not be classified: %+v", f)
		case "../outside.txt", "/etc/hostname", "a/../../x", "", ".", "sub//x":
			t.Errorf("escaping path %q got a row: %+v", f.Path, f)
		}
	}
	if !hasWarning(res, "symlink that leaves the project", "README.md") {
		t.Errorf("no symlink warning: %v", res.Warnings)
	}
	for _, bad := range []string{"../outside.txt", "/etc/hostname", "a/../../x", "sub//x"} {
		if !hasWarning(res, "not a relative path inside the project", bad) {
			t.Errorf("no warning for %q: %v", bad, res.Warnings)
		}
	}
	// the rest of the project is still reported
	wantStatus(t, res, "go.mod", service.DriftUnchanged)
}
