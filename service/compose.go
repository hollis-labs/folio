package service

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/hollis-labs/folio/internal/compose"
	"github.com/hollis-labs/folio/internal/preset"
	"github.com/hollis-labs/folio/internal/render"
)

// ignoredInputDeclaredElsewhere reports whether a resolveInputs warning of
// the form `input "X" is not declared by preset "Y" (ignored)` names an
// input key X that IS declared somewhere in the compose chain. Those
// warnings are appropriate for single-preset use; for composition they
// just narrate the expected cross-layer flow and add noise.
func ignoredInputDeclaredElsewhere(warning string, declaredAnywhere map[string]struct{}) bool {
	prefix := `input "`
	if !strings.HasPrefix(warning, prefix) {
		return false
	}
	rest := warning[len(prefix):]
	endQuote := strings.IndexByte(rest, '"')
	if endQuote < 0 {
		return false
	}
	key := rest[:endQuote]
	_, ok := declaredAnywhere[key]
	return ok
}

// layer is one preset layer in apply order, paired with the per-layer
// render.Context. Produced by composedLayers; consumed by the New/Plan
// render loop.
type layer struct {
	Preset       *preset.Preset
	FS           fs.FS
	Source       string
	ResolvedPath string
	Ctx          render.Context
}

// renderedFile carries a tree file plus the preset id that produced it,
// used for last-writer-wins tracking across composed layers.
type renderedFile struct {
	File     render.RenderedFile
	PresetID string
}

// composeLoader implements compose.Loader for the service. It resolves
// each composes[] entry to a sub-FS within the source root (bundled FS or
// user-dir DirFS), enforcing path-safety + parsing the loaded preset.
// v0.2 supports source: local only.
type composeLoader struct {
	rootFS     fs.FS
	rootDir    string // ResolveComposePath root (e.g., "presets" for bundled, "." for user-dir)
	sourceTag  string // manifest source label ("bundled" | "local")
	parentDesc string // human-readable parent label for error context
}

// Load resolves a composes[] entry against parentDir and returns the
// loaded preset with its sub-FS. Path safety, source enum, and version
// constraint satisfaction are all enforced here.
func (l *composeLoader) Load(entry preset.ComposeEntry, parentDir string) (*compose.LoadResult, error) {
	source := entry.Source
	if source == "" {
		source = compose.DefaultSource
	}
	if source != "local" {
		return nil, fmt.Errorf("compose source %q not supported in v0.2 (only 'local')", entry.Source)
	}
	targetDir, err := compose.ResolveComposePath(parentDir, entry.Path, l.rootDir)
	if err != nil {
		return nil, err
	}
	subFS, err := fs.Sub(l.rootFS, targetDir)
	if err != nil {
		return nil, fmt.Errorf("sub-fs %q: %w", targetDir, err)
	}
	data, err := fs.ReadFile(subFS, "preset.yaml")
	if err != nil {
		return nil, fmt.Errorf("read preset.yaml at %s: %w", targetDir, err)
	}
	p, err := preset.ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parse preset.yaml at %s: %w", targetDir, err)
	}
	if res := p.Validate(); !res.OK() {
		return nil, fmt.Errorf("validation failed for composed preset at %s: %s",
			targetDir, res.Errors[0].Message)
	}
	if p.ID != entry.ID {
		return nil, fmt.Errorf("compose entry id %q points to preset at %s which declares id %q",
			entry.ID, targetDir, p.ID)
	}
	c, err := compose.ParseConstraint(entry.Version)
	if err != nil {
		return nil, fmt.Errorf("parse constraint %q for %s: %w", entry.Version, entry.ID, err)
	}
	if _, err := compose.ResolveVersion(c, []string{p.Version}); err != nil {
		return nil, fmt.Errorf("preset %s@%s does not satisfy constraint %s: %w",
			p.ID, p.Version, entry.Version, err)
	}
	return &compose.LoadResult{
		Preset:       p,
		FS:           subFS,
		Source:       l.sourceTag,
		ResolvedPath: l.sourceTag + ":" + targetDir,
		ParentDir:    targetDir,
	}, nil
}

// composedLayers builds the apply-order layer list and resolves inputs +
// computed per layer.
//
// For a single-preset load (no composes:) the result is a one-element
// slice [root]. For composing presets we walk the compose DAG, scope
// inputs per layer against the caller's render.Context (raw user
// inputs), and resolve per-layer computed values with cross-layer last-
// wins semantics — later layers' templates can reference earlier layers'
// computed values.
//
// Returns the layer slice in apply order, any warnings accumulated by
// resolveInputs across layers, and an error on the first failure.
func (s *Service) composedLayers(loaded *LoadedPreset, callerCtx render.Context) ([]layer, []string, error) {
	var layerRefs []compose.LayerRef
	if len(loaded.Preset.Composes) == 0 {
		layerRefs = []compose.LayerRef{{
			Preset:       loaded.Preset,
			Source:       loaded.Source,
			ResolvedPath: loaded.ResolvedPath,
			FS:           loaded.FS,
		}}
	} else {
		root := &compose.LoadResult{
			Preset:       loaded.Preset,
			FS:           loaded.FS,
			Source:       loaded.Source,
			ResolvedPath: loaded.ResolvedPath,
			ParentDir:    loaded.parentDir,
		}
		cl := &composeLoader{
			rootFS:     loaded.sourceRootFS,
			rootDir:    loaded.sourceRoot,
			sourceTag:  loaded.Source,
			parentDesc: loaded.Preset.ID,
		}
		g, err := compose.BuildGraph(root, cl)
		if err != nil {
			return nil, nil, newErr(ErrPresetInvalid, "compose graph", err)
		}
		layerRefs = g.LayerOrder()
	}

	// Build the union of every key declared by ANY layer in the graph. We
	// use this to suppress resolveInputs's "input X is not declared by
	// preset Y (ignored)" warnings when the key IS declared somewhere in
	// the chain — those warnings are appropriate noise for single-preset
	// use but spurious for composition (the user provides inputs spanning
	// multiple layers, and each layer's per-input check flags every key
	// it doesn't own).
	declaredAnywhere := make(map[string]struct{})
	for _, lr := range layerRefs {
		for _, in := range lr.Preset.Inputs {
			declaredAnywhere[in.Name] = struct{}{}
		}
	}

	out := make([]layer, 0, len(layerRefs))
	mergedComputed := map[string]any{}
	mergedInputs := map[string]any{}
	var allWarnings []string

	for _, lr := range layerRefs {
		var scopedInputs map[string]any
		if lr.Preset.ID == loaded.Preset.ID {
			scopedInputs = callerCtx.Inputs
		} else {
			// lr.ComposeEntry is the entry that introduced this layer
			// during BuildGraph's first-encounter visit. Diamond presets
			// reached via multiple parents bind to the first parent's
			// entry (declared order in the parent's composes:) — the
			// canonical choice locked at the graph level.
			v, err := compose.ScopeVarsForLayer(callerCtx, lr.ComposeEntry.Vars, lr.Preset)
			if err != nil {
				return nil, nil, newErr(ErrPresetInvalid,
					"scope vars for "+lr.Preset.ID, err)
			}
			scopedInputs = v
		}

		declared, warnings, err := resolveInputs(lr.Preset, scopedInputs)
		if err != nil {
			return nil, nil, err
		}
		for _, w := range warnings {
			if !ignoredInputDeclaredElsewhere(w, declaredAnywhere) {
				allWarnings = append(allWarnings, w)
			}
		}

		// Each layer's render context sees prior layers' resolved/defaulted
		// values plus this layer's own typed values. scopedInputs is the
		// INPUT to resolveInputs and intentionally NOT merged into the
		// final layerInputs — that prevents undeclared, unvalidated raw
		// user keys (e.g., a typo like --input bogous=value) from leaking
		// into templates and the .folio.yaml manifest.
		layerInputs := make(map[string]any, len(mergedInputs)+len(declared))
		for k, v := range mergedInputs {
			layerInputs[k] = v
		}
		for k, v := range declared {
			layerInputs[k] = v
		}
		mergedInputs = layerInputs

		lctx := render.Context{
			Inputs:   layerInputs,
			Computed: mergedComputed,
			Target:   callerCtx.Target,
			Preset:   render.PresetInfo{ID: lr.Preset.ID, Version: lr.Preset.Version},
			Folio:    callerCtx.Folio,
			Now:      callerCtx.Now,
		}
		computed, err := resolveComputed(lr.Preset, lctx)
		if err != nil {
			return nil, nil, newErr(ErrRenderFailed,
				"compute layer "+lr.Preset.ID, err)
		}
		// resolveComputed returns prior values seeded + this layer's new
		// entries (last-wins on collision). Carry forward for subsequent
		// layers and refresh the per-layer ctx so this layer's file
		// templates can reference its own computed values.
		mergedComputed = computed
		lctx.Computed = mergedComputed

		out = append(out, layer{
			Preset:       lr.Preset,
			FS:           lr.FS,
			Source:       lr.Source,
			ResolvedPath: lr.ResolvedPath,
			Ctx:          lctx,
		})
	}
	return out, allWarnings, nil
}
