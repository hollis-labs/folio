// Package render implements folio's Go text/template render engine.
//
// The package exposes RenderString (mirroring Hadron's renderString shape)
// for single-string template evaluation and RenderTree for walking a preset
// template source directory and producing a flat list of rendered files
// ready for the service layer to write.
//
// The funcmap is a curated whitelist. Hadron-shared helpers (basename,
// dirname, ext, json) keep parity by name so templates portable between
// Hadron blueprints and folio presets continue to evaluate. folio EXCLUDES
// the env and readFile helpers Hadron exposes — folio's threat model
// includes third-party presets via git URL (v1.1+), and template-time access
// to environment variables / the filesystem is a secret-leak and
// reproducibility risk in that model.
package render

import "time"

// Context is the typed root passed to every template render. It is
// converted to a lowercase-keyed map[string]any before being handed to
// text/template, so templates reference .inputs.foo and not .Inputs.foo.
type Context struct {
	// Inputs are the user/agent-supplied input values, typed per the
	// preset's inputs schema.
	Inputs map[string]any
	// Computed values derived from Inputs.
	Computed map[string]any
	// Target is the absolute directory the preset is being rendered into.
	Target string
	// Preset identifies the preset producing this render.
	Preset PresetInfo
	// Folio identifies the folio binary version.
	Folio FolioInfo
	// Now is frozen at the start of the render call. All files generated in
	// a single RenderTree share this value, so multi-file timestamps agree.
	Now time.Time
}

// PresetInfo is what templates can reach via .preset.id / .preset.version.
type PresetInfo struct {
	ID      string
	Version string
}

// FolioInfo is what templates can reach via .folio.version.
type FolioInfo struct {
	Version string
}

// asMap converts a Context to the lowercase-keyed shape Go templates expect.
func (c Context) asMap() map[string]any {
	return map[string]any{
		"inputs":   nonNilMap(c.Inputs),
		"computed": nonNilMap(c.Computed),
		"target":   c.Target,
		"preset": map[string]any{
			"id":      c.Preset.ID,
			"version": c.Preset.Version,
		},
		"folio": map[string]any{
			"version": c.Folio.Version,
		},
		"now": c.Now,
	}
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
