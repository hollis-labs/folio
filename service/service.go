package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hollis-labs/folio/internal/compose"
	"github.com/hollis-labs/folio/internal/manifest"
	"github.com/hollis-labs/folio/internal/preset"
	"github.com/hollis-labs/folio/internal/render"
)

// Options configure a Service. Both BundledFS and UserDir are optional —
// callers can ship folio with only one source, or both. v0's CLI passes a
// non-nil BundledFS so the base preset is always available; tests pass
// only an on-disk dir.
type Options struct {
	// BundledFS is the read-only filesystem containing built-in presets.
	// BundledRoot names the directory within that FS where presets live
	// (defaults to "presets"). Presets resolve to <BundledRoot>/<id>/.
	BundledFS   fs.FS
	BundledRoot string
	// UserDir is the on-disk directory where users add presets. Defaults
	// to ~/.folio/presets/local when empty.
	UserDir string
	// FolioVersion is recorded in the generated manifest.
	FolioVersion string
	// Now is used as the freeze point for .now during a render. Tests
	// override this to get deterministic timestamps; production callers
	// leave it zero and the service substitutes time.Now() per call.
	Now time.Time
}

// Service is the entry point for folio operations.
type Service struct {
	bundledFS    fs.FS
	bundledRoot  string
	userDir      string
	folioVersion string
	now          func() time.Time
}

// New constructs a Service from opts. Defaults are filled in for missing
// fields; New itself never errors.
func New(opts Options) *Service {
	root := opts.BundledRoot
	if root == "" {
		root = "presets"
	}
	userDir := opts.UserDir
	if userDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			userDir = filepath.Join(home, ".folio", "presets", "local")
		}
	}
	ver := opts.FolioVersion
	if ver == "" {
		ver = "0.0.0-dev"
	}
	nowFn := time.Now
	if !opts.Now.IsZero() {
		t := opts.Now
		nowFn = func() time.Time { return t }
	}
	return &Service{
		bundledFS:    opts.BundledFS,
		bundledRoot:  root,
		userDir:      userDir,
		folioVersion: ver,
		now:          nowFn,
	}
}

// NewOptions parameterises a Service.New (project generation) call.
type NewOptions struct {
	// PresetID is the id declared in preset.yaml — e.g., "base".
	PresetID string
	// TargetDir is the directory the project gets written into. v0
	// requires the directory to not already exist (or to be empty).
	TargetDir string
	// Inputs supplies values for the preset's declared inputs. Missing
	// required inputs without a default cause an ErrInputMissing.
	Inputs map[string]any
}

// NewResult summarizes a completed Service.New invocation. The manifest
// returned is the one that was written to <TargetDir>/.folio.yaml.
type NewResult struct {
	Files    []FileResult
	Manifest manifest.Manifest
	Warnings []string
}

// FileResult describes one file produced by the New call.
type FileResult struct {
	Path       string
	Bytes      int64
	Digest     string
	IsTemplate bool
}

// PlanResult summarizes a dry-run Service.Plan invocation. No writes occur;
// preview content is truncated at 2 KiB per file.
type PlanResult struct {
	Files    []PlanFile
	Inputs   map[string]any
	Computed map[string]any
	Warnings []string
}

// PlanFile is one entry in a PlanResult.
type PlanFile struct {
	Path       string
	Bytes      int64
	Preview    string
	IsTemplate bool
}

// PlanPreviewLimit caps the preview field of PlanFile at 2 KiB.
const PlanPreviewLimit = 2 * 1024

// LoadedPreset is the result of LoadPreset. It exposes both the parsed
// manifest and the filesystem rooted at the preset directory so the render
// engine can read template content via fs.FS.
type LoadedPreset struct {
	Preset       *preset.Preset
	FS           fs.FS
	Source       string // "bundled" or "local"
	ResolvedPath string // identifier suitable for manifest.PresetRef.ResolvedPath

	// Compose-time context. Used by composedLayers to resolve relative
	// `composes[].path` entries against the parent preset's directory.
	// sourceRootFS is the un-Sub'd root FS (the bundled embed.FS or
	// os.DirFS(userDir)); sourceRoot is the path prefix within that FS
	// where presets live ("presets" for the production bundled FS, "."
	// for user-dir or tests using bundledRoot="."). parentDir is this
	// preset's directory expressed in the sourceRootFS path space.
	sourceRootFS fs.FS
	sourceRoot   string
	parentDir    string
}

// New runs the full generate pipeline against the bundled or user preset
// matching opts.PresetID, writes the resulting tree into opts.TargetDir,
// and emits a .folio.yaml manifest alongside it. For composing presets
// (composes: declared), each layer renders in apply order; same-path
// writes overwrite earlier layers silently (last-writer-wins).
func (s *Service) New(opts NewOptions) (NewResult, error) {
	loaded, callerCtx, warnings, err := s.prepareRender(opts)
	if err != nil {
		return NewResult{}, err
	}
	now := callerCtx.Now

	layers, composeWarnings, err := s.composedLayers(loaded, callerCtx)
	if err != nil {
		return NewResult{}, err
	}
	warnings = append(warnings, composeWarnings...)

	rendered := map[string]renderedFile{}
	orderedPaths := []string{}
	for _, l := range layers {
		tree, err := s.renderTree(
			&LoadedPreset{Preset: l.Preset, FS: l.FS},
			l.Ctx,
		)
		if err != nil {
			return NewResult{}, err
		}
		for _, f := range tree.Files {
			if _, exists := rendered[f.RelPath]; !exists {
				orderedPaths = append(orderedPaths, f.RelPath)
			}
			rendered[f.RelPath] = renderedFile{File: f, PresetID: l.Preset.ID}
		}
	}

	if err := ensureTargetReady(opts.TargetDir); err != nil {
		return NewResult{}, err
	}
	if err := os.MkdirAll(opts.TargetDir, 0o755); err != nil {
		return NewResult{}, newErr(ErrWriteFailed, fmt.Sprintf("mkdir %s", opts.TargetDir), err)
	}

	var files []FileResult
	manifestFiles := map[string]manifest.FileRecord{}
	for _, rp := range orderedPaths {
		rf := rendered[rp]
		f := rf.File
		dst := filepath.Join(opts.TargetDir, filepath.FromSlash(f.RelPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return NewResult{}, newErr(ErrWriteFailed, fmt.Sprintf("mkdir %s", filepath.Dir(dst)), err)
		}
		var bytesOnDisk []byte
		if f.IsBinary {
			bytesOnDisk = f.Content
		} else {
			bytesOnDisk = manifest.NormalizeLineEndings(f.Content)
		}
		if err := os.WriteFile(dst, bytesOnDisk, modeFor(f)); err != nil {
			return NewResult{}, newErr(ErrWriteFailed, fmt.Sprintf("write %s", dst), err)
		}
		digest := manifest.Digest(f.Content)
		files = append(files, FileResult{
			Path:       f.RelPath,
			Bytes:      int64(len(bytesOnDisk)),
			Digest:     digest,
			IsTemplate: f.IsTemplate,
		})
		manifestFiles[f.RelPath] = manifest.FileRecord{
			Preset:      rf.PresetID,
			DigestAtGen: digest,
		}
	}

	presetRefs := make([]manifest.PresetRef, 0, len(layers))
	syncPresets := make([]manifest.PresetRef, 0, len(layers))
	for _, l := range layers {
		presetRefs = append(presetRefs, manifest.PresetRef{
			ID:           l.Preset.ID,
			Version:      l.Preset.Version,
			Source:       l.Source,
			ResolvedPath: l.ResolvedPath,
		})
		syncPresets = append(syncPresets, manifest.PresetRef{
			ID:      l.Preset.ID,
			Version: l.Preset.Version,
		})
	}

	// .folio.yaml records the TOP-LEVEL layer's inputs (the user-facing
	// config). Each composed layer derived its inputs from these via
	// compose.ScopeVarsForLayer at generation time; storing the derived
	// values would obscure what the user actually set.
	rootLayer := layers[len(layers)-1]

	m := manifest.Manifest{
		FolioVersion: "0.1",
		GeneratedAt:  now,
		Generator:    "folio/" + s.folioVersion,
		Presets:      presetRefs,
		Inputs:       rootLayer.Ctx.Inputs,
		Computed:     rootLayer.Ctx.Computed,
		Files:        manifestFiles,
		SyncHistory: []manifest.SyncEvent{{
			At:        now,
			Operation: "init",
			Presets:   syncPresets,
		}},
	}
	if err := manifest.Write(opts.TargetDir, m); err != nil {
		return NewResult{}, newErr(ErrWriteFailed, "write manifest", err)
	}

	return NewResult{Files: files, Manifest: m, Warnings: warnings}, nil
}

// Plan executes the same pipeline as New but writes nothing. Useful for
// `folio plan` (dry-run) and for agents that want to verify what would
// happen before committing. For composing presets, layers iterate in
// apply order with last-writer-wins on path collisions.
func (s *Service) Plan(opts NewOptions) (PlanResult, error) {
	loaded, callerCtx, warnings, err := s.prepareRender(opts)
	if err != nil {
		return PlanResult{}, err
	}

	layers, composeWarnings, err := s.composedLayers(loaded, callerCtx)
	if err != nil {
		return PlanResult{}, err
	}
	warnings = append(warnings, composeWarnings...)

	rendered := map[string]renderedFile{}
	orderedPaths := []string{}
	for _, l := range layers {
		tree, err := s.renderTree(
			&LoadedPreset{Preset: l.Preset, FS: l.FS},
			l.Ctx,
		)
		if err != nil {
			return PlanResult{}, err
		}
		for _, f := range tree.Files {
			if _, exists := rendered[f.RelPath]; !exists {
				orderedPaths = append(orderedPaths, f.RelPath)
			}
			rendered[f.RelPath] = renderedFile{File: f, PresetID: l.Preset.ID}
		}
	}

	var files []PlanFile
	for _, rp := range orderedPaths {
		f := rendered[rp].File
		preview := string(f.Content)
		if len(preview) > PlanPreviewLimit {
			preview = preview[:PlanPreviewLimit] + "\n... (truncated)"
		}
		files = append(files, PlanFile{
			Path:       f.RelPath,
			Bytes:      int64(len(f.Content)),
			Preview:    preview,
			IsTemplate: f.IsTemplate,
		})
	}

	rootLayer := layers[len(layers)-1]
	return PlanResult{
		Files:    files,
		Inputs:   rootLayer.Ctx.Inputs,
		Computed: rootLayer.Ctx.Computed,
		Warnings: warnings,
	}, nil
}

// ValidatePreset wraps preset.Parse + preset.Validate for the CLI command.
// The returned preset.Result includes both errors and warnings.
func (s *Service) ValidatePreset(path string) (preset.Result, *preset.Preset, error) {
	p, err := preset.Parse(path)
	if err != nil {
		return preset.Result{}, nil, newErr(ErrPresetInvalid, fmt.Sprintf("parse %s", path), err)
	}
	return p.Validate(), p, nil
}

// LoadPreset resolves a preset id against the configured sources. Bundled
// FS is searched first (per design doc §2 — bundled wins), then the user
// dir. The first match wins; user dir overrides require explicit selection
// via the file path API (deferred).
func (s *Service) LoadPreset(id string) (*LoadedPreset, error) {
	if id == "" {
		return nil, newErr(ErrPresetNotFound, "preset id is empty", nil)
	}
	if s.bundledFS != nil {
		sub := pathJoin(s.bundledRoot, id)
		if entry, err := fs.Stat(s.bundledFS, sub); err == nil && entry.IsDir() {
			lp, err := s.loadFromSubFS(s.bundledFS, sub, "bundled", "bundled:"+sub)
			if err != nil {
				return nil, err
			}
			lp.sourceRootFS = s.bundledFS
			lp.sourceRoot = s.bundledRoot
			lp.parentDir = sub
			return lp, nil
		}
	}
	if s.userDir != "" {
		entry, err := s.findUserPreset(id, nil)
		if err != nil {
			return nil, err
		}
		if entry != "" {
			full := filepath.Join(s.userDir, entry)
			lp, err := s.loadFromSubFS(os.DirFS(full), ".", "local", full)
			if err != nil {
				return nil, err
			}
			lp.sourceRootFS = os.DirFS(s.userDir)
			lp.sourceRoot = "."
			lp.parentDir = entry
			return lp, nil
		}
	}
	return nil, newErr(ErrPresetNotFound, fmt.Sprintf("no preset with id %q in bundled or user sources", id), nil)
}

func (s *Service) loadFromSubFS(parent fs.FS, sub, source, resolved string) (*LoadedPreset, error) {
	presetFS, err := fs.Sub(parent, sub)
	if err != nil {
		return nil, newErr(ErrInternal, "sub-fs", err)
	}
	if sub == "." {
		presetFS = parent
	}
	data, err := fs.ReadFile(presetFS, "preset.yaml")
	if err != nil {
		return nil, newErr(ErrPresetNotFound, fmt.Sprintf("preset.yaml missing at %s", resolved), err)
	}
	p, err := preset.ParseBytes(data)
	if err != nil {
		return nil, newErr(ErrPresetInvalid, fmt.Sprintf("parse preset.yaml at %s", resolved), err)
	}
	if res := p.Validate(); !res.OK() {
		return nil, newErr(ErrPresetInvalid, fmt.Sprintf("validation failed for %s: %d error(s)", resolved, len(res.Errors)), errors.New(res.Errors[0].Message))
	}
	return &LoadedPreset{Preset: p, FS: presetFS, Source: source, ResolvedPath: resolved}, nil
}

// findUserPreset scans s.userDir for "<id>@*" directories and returns the
// entry name (e.g., "base@1.2.0") of the highest matching version. When
// constraint is non-nil, only versions satisfying it are considered;
// otherwise the highest version overall wins. Returns "" if no directory
// matches the id at all, or no version satisfies a supplied constraint.
func (s *Service) findUserPreset(id string, constraint *compose.Constraint) (string, error) {
	entries, err := os.ReadDir(s.userDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", newErr(ErrInternal, fmt.Sprintf("read user dir %s", s.userDir), err)
	}
	prefix := id + "@"
	versions := make([]string, 0)
	entryByVersion := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		v := strings.TrimPrefix(name, prefix)
		versions = append(versions, v)
		entryByVersion[v] = name
	}
	if len(versions) == 0 {
		return "", nil
	}
	c := compose.MatchAny()
	if constraint != nil {
		c = *constraint
	}
	picked, err := compose.ResolveVersion(c, versions)
	if err != nil {
		if constraint == nil {
			// Defensive: MatchAny never fails for non-empty versions, but
			// keep the no-match-as-not-found sentinel for the top-level path.
			return "", nil
		}
		return "", newErr(ErrPresetNotFound,
			fmt.Sprintf("no user-dir version of %q satisfies constraint %s", id, constraint),
			err)
	}
	return entryByVersion[picked], nil
}

// prepareRender loads the preset and builds the bare render context — it
// does NOT resolve inputs or computed values, since for composing presets
// each layer needs its own resolution against its own declared schema.
// composedLayers handles per-layer resolveInputs + resolveComputed for both
// the composing and single-preset paths.
//
// The returned ctx.Inputs holds the user's raw, unfiltered input map (the
// caller-side `.inputs.*` perspective used by compose.ScopeVarsForLayer
// when overriding per-key for inner layers). ctx.Computed is empty.
func (s *Service) prepareRender(opts NewOptions) (*LoadedPreset, render.Context, []string, error) {
	if opts.TargetDir == "" {
		return nil, render.Context{}, nil, newErr(ErrInputInvalid, "target directory is required", nil)
	}
	abs, err := filepath.Abs(opts.TargetDir)
	if err != nil {
		return nil, render.Context{}, nil, newErr(ErrInputInvalid, fmt.Sprintf("resolve target %s", opts.TargetDir), err)
	}

	loaded, err := s.LoadPreset(opts.PresetID)
	if err != nil {
		return nil, render.Context{}, nil, err
	}

	now := s.now()

	ctx := render.Context{
		Inputs:   opts.Inputs,
		Computed: map[string]any{},
		Target:   abs,
		Preset:   render.PresetInfo{ID: loaded.Preset.ID, Version: loaded.Preset.Version},
		Folio:    render.FolioInfo{Version: s.folioVersion},
		Now:      now,
	}

	var warnings []string
	if loaded.Preset.PostRender != nil && loaded.Preset.PostRender.Blueprint != "" {
		warnings = append(warnings, "post_render is not implemented in v0; the hook will be skipped at generation time")
	}

	return loaded, ctx, warnings, nil
}

// renderTree builds the files-rooted sub-FS and delegates to render.RenderTree.
func (s *Service) renderTree(loaded *LoadedPreset, ctx render.Context) (render.TreeResult, error) {
	srcRel := strings.TrimPrefix(loaded.Preset.Files.Source, "./")
	srcRel = strings.TrimPrefix(srcRel, "/")
	if srcRel == "" {
		srcRel = "."
	}
	srcFS, err := fs.Sub(loaded.FS, srcRel)
	if err != nil {
		return render.TreeResult{}, newErr(ErrRenderFailed, fmt.Sprintf("sub-fs for files.source %q", loaded.Preset.Files.Source), err)
	}
	opts := render.TreeOptions{
		Source:           srcFS,
		TemplateSuffix:   loaded.Preset.Files.TemplateSuffixOrDefault(),
		Ignore:           loaded.Preset.Files.Ignore,
		BinaryExtensions: loaded.Preset.Files.BinaryExtensions,
	}
	tree, err := render.RenderTree(opts, ctx)
	if err != nil {
		return render.TreeResult{}, newErr(ErrRenderFailed, "render tree", err)
	}
	return tree, nil
}

// resolveInputs applies type-checked defaults and validates each declared
// input against its schema (type, pattern, range).
func resolveInputs(p *preset.Preset, user map[string]any) (map[string]any, []string, error) {
	out := map[string]any{}
	var warnings []string

	for _, in := range p.Inputs {
		val, present := user[in.Name]
		if !present {
			if in.Default != nil {
				val = in.Default
			} else if in.Required {
				return nil, warnings, newErr(ErrInputMissing, fmt.Sprintf("missing required input %q", in.Name), nil)
			} else {
				continue
			}
		}
		coerced, err := coerceInput(in, val)
		if err != nil {
			return nil, warnings, err
		}
		out[in.Name] = coerced
	}
	for k := range user {
		known := false
		for _, in := range p.Inputs {
			if in.Name == k {
				known = true
				break
			}
		}
		if !known {
			warnings = append(warnings, fmt.Sprintf("input %q is not declared by preset %q (ignored)", k, p.ID))
		}
	}
	return out, warnings, nil
}

// coerceInput type-checks and normalises a single user-supplied input
// against its declared schema. Returns a typed value usable by templates.
func coerceInput(in preset.Input, val any) (any, error) {
	switch in.Type {
	case "string":
		s, ok := val.(string)
		if !ok {
			return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q is not a string: %v", in.Name, val), nil)
		}
		if in.Pattern != "" {
			re, err := regexp.Compile(in.Pattern)
			if err != nil {
				return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q pattern compile failed", in.Name), err)
			}
			if !re.MatchString(s) {
				return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q value %q does not match pattern %q", in.Name, s, in.Pattern), nil)
			}
		}
		if in.MinLength != nil && len(s) < *in.MinLength {
			return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q length %d below min %d", in.Name, len(s), *in.MinLength), nil)
		}
		if in.MaxLength != nil && len(s) > *in.MaxLength {
			return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q length %d above max %d", in.Name, len(s), *in.MaxLength), nil)
		}
		return s, nil
	case "bool":
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q not parseable as bool: %v", in.Name, v), err)
			}
			return b, nil
		default:
			return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q is not a bool: %v", in.Name, val), nil)
		}
	case "number":
		switch v := val.(type) {
		case int:
			return float64(v), checkNumberBounds(in, float64(v))
		case int64:
			return float64(v), checkNumberBounds(in, float64(v))
		case float64:
			return v, checkNumberBounds(in, v)
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q not parseable as number: %v", in.Name, v), err)
			}
			return f, checkNumberBounds(in, f)
		default:
			return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q is not a number: %v", in.Name, val), nil)
		}
	case "enum":
		s, ok := val.(string)
		if !ok {
			return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q (enum) is not a string: %v", in.Name, val), nil)
		}
		for _, opt := range in.Values {
			if opt == s {
				return s, nil
			}
		}
		return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q value %q is not one of %v", in.Name, s, in.Values), nil)
	case "list[string]":
		switch v := val.(type) {
		case []any:
			out := make([]string, len(v))
			for i, it := range v {
				s, ok := it.(string)
				if !ok {
					return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q element %d is not a string: %v", in.Name, i, it), nil)
				}
				out[i] = s
			}
			return out, nil
		case []string:
			return v, nil
		default:
			return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q is not a list: %v", in.Name, val), nil)
		}
	default:
		return nil, newErr(ErrInputInvalid, fmt.Sprintf("input %q has unsupported type %q", in.Name, in.Type), nil)
	}
}

func checkNumberBounds(in preset.Input, v float64) error {
	if in.Min != nil && v < *in.Min {
		return newErr(ErrInputInvalid, fmt.Sprintf("input %q value %v below min %v", in.Name, v, *in.Min), nil)
	}
	if in.Max != nil && v > *in.Max {
		return newErr(ErrInputInvalid, fmt.Sprintf("input %q value %v above max %v", in.Name, v, *in.Max), nil)
	}
	return nil
}

// resolveComputed renders each computed[key] template against ctx (with
// the live computed map already wired so cross-references resolve in
// alphabetic key order). For v0 this is enough; topological dependency
// resolution can land later if a preset needs it.
// resolveComputed renders each computed[key] template against ctx in
// sorted-key order, returning a fresh map of all resolved values. The
// working map is seeded from ctx.Computed so prior values (e.g. from
// earlier composed layers) remain visible to templates in this layer.
// Within a layer, sorted-key order lets later keys reference earlier ones.
func resolveComputed(p *preset.Preset, ctx render.Context) (map[string]any, error) {
	out := make(map[string]any, len(ctx.Computed)+len(p.Computed))
	for k, v := range ctx.Computed {
		out[k] = v
	}
	if len(p.Computed) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(p.Computed))
	for k := range p.Computed {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ctx.Computed = out
	for _, k := range keys {
		tpl := p.Computed[k]
		v, err := render.RenderString(tpl, ctx)
		if err != nil {
			return nil, newErr(ErrComputeFailed, fmt.Sprintf("computed.%s", k), err)
		}
		out[k] = v
	}
	return out, nil
}

// ensureTargetReady checks that TargetDir doesn't already exist (or is
// empty); v0 refuses to overwrite a non-empty directory.
func ensureTargetReady(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return newErr(ErrWriteFailed, fmt.Sprintf("stat target %s", dir), err)
	}
	if len(entries) == 0 {
		return nil
	}
	return newErr(ErrTargetExists, fmt.Sprintf("target dir %s already exists and is not empty", dir), nil)
}

func modeFor(f render.RenderedFile) os.FileMode {
	if f.Mode == 0 {
		return 0o644
	}
	return f.Mode
}

// pathJoin uses forward-slash semantics regardless of OS — fs.FS paths are
// always forward-slash.
func pathJoin(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		clean = append(clean, p)
	}
	return path.Join(clean...)
}
