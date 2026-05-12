// Package manifest reads and writes .folio.yaml, the in-repo manifest folio
// generates alongside a rendered project. The manifest is the source of
// truth for re-render (folio sync, deferred) — it records which preset(s)
// produced the project, the resolved inputs + computed values, and a
// per-file SHA-256 digest computed AFTER LF newline normalization so the
// digests are stable across platforms.
package manifest

import "time"

// ManifestFilename is the canonical filename written into the target dir.
const ManifestFilename = ".folio.yaml"

// Manifest is the typed in-memory shape of .folio.yaml.
//
// Field order matches design doc §3 and is the order yaml.v3 emits when
// marshalling — relying on struct-field order rather than map iteration is
// what makes Write idempotent.
type Manifest struct {
	FolioVersion string                `yaml:"folio_version"`
	GeneratedAt  time.Time             `yaml:"generated_at"`
	Generator    string                `yaml:"generator"`
	Presets      []PresetRef           `yaml:"presets"`
	Inputs       map[string]any        `yaml:"inputs,omitempty"`
	Computed     map[string]any        `yaml:"computed,omitempty"`
	Files        map[string]FileRecord `yaml:"files,omitempty"`
	PostRender   *PostRenderRecord     `yaml:"post_render,omitempty"`
	SyncHistory  []SyncEvent           `yaml:"sync_history,omitempty"`
}

// PresetRef identifies one preset that contributed to the render. In v0,
// Presets always has length 1; composition (multiple layers) lands later.
type PresetRef struct {
	ID           string `yaml:"id"`
	Version      string `yaml:"version"`
	Source       string `yaml:"source,omitempty"`
	ResolvedPath string `yaml:"resolved_path,omitempty"`
	Digest       string `yaml:"digest,omitempty"`
}

// FileRecord captures per-file metadata used by `folio sync` (deferred) to
// decide whether the on-disk content has drifted from the rendered version.
//
// DigestAtGen is `sha256:<hex>` of the rendered content AFTER LF newline
// normalisation, matching the contract described in design doc §5.
type FileRecord struct {
	Preset          string `yaml:"preset,omitempty"`
	DigestAtGen     string `yaml:"digest_at_gen"`
	ModifiedLocally bool   `yaml:"modified_locally,omitempty"`
}

// PostRenderRecord records that a Hadron post-render hook ran (or did not).
// v0 leaves this nil; the field exists in the struct for v0.x compatibility
// so readers don't need to migrate the schema.
type PostRenderRecord struct {
	Blueprint   string    `yaml:"blueprint"`
	Preset      string    `yaml:"preset"`
	RanAt       time.Time `yaml:"ran_at"`
	HadronRunID string    `yaml:"hadron_run_id,omitempty"`
	Ran         bool      `yaml:"ran"`
}

// SyncEvent is one entry in the sync_history log. v0 always emits a single
// "init" event when the project is first generated.
type SyncEvent struct {
	At        time.Time   `yaml:"at"`
	Operation string      `yaml:"operation"`
	Presets   []PresetRef `yaml:"presets,omitempty"`
}
