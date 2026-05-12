// Package preset parses and validates folio preset manifests (preset.yaml).
//
// A preset declares: identity (id, version), the inputs schema agents/users
// fill, computed variables derived from inputs, the file tree to render, and
// optional composition / post-render / sync metadata. v0 implements parsing
// and a subset of validation; composition execution, post-render hooks, and
// sync are deferred.
package preset

import "gopkg.in/yaml.v3"

// Preset is the in-memory representation of a parsed preset.yaml.
//
// Field order mirrors the design doc §2 schema. composes is parsed but a
// non-empty value triggers a validation error in v0. post_render is parsed
// but ignored (validation emits a warning). sync is parsed and stored for
// forward-compatibility but not yet acted on.
type Preset struct {
	FolioVersion string            `yaml:"folio_version"`
	ID           string            `yaml:"id"`
	Version      string            `yaml:"version"`
	Description  string            `yaml:"description,omitempty"`
	Author       string            `yaml:"author,omitempty"`
	License      string            `yaml:"license,omitempty"`
	Composes     []ComposeEntry    `yaml:"composes,omitempty"`
	Inputs       []Input           `yaml:"inputs,omitempty"`
	Computed     map[string]string `yaml:"computed,omitempty"`
	Files        Files             `yaml:"files"`
	PostRender   *PostRender       `yaml:"post_render,omitempty"`
	Sync         *Sync             `yaml:"sync,omitempty"`

	// sourceFile is the absolute path the preset was loaded from. Empty when
	// parsed from an in-memory byte slice.
	sourceFile string

	// rootNode preserves the parsed YAML tree so validation errors can report
	// file:line:col. It is populated by Parse and ParseBytes.
	rootNode *yaml.Node
}

// SourceFile returns the absolute path the preset was loaded from, or the
// empty string if the preset was parsed from a byte slice without a path.
func (p *Preset) SourceFile() string { return p.sourceFile }

// Input declares one user-facing or agent-facing input the preset expects.
type Input struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`
	Required    bool     `yaml:"required,omitempty"`
	Default     any      `yaml:"default,omitempty"`
	Pattern     string   `yaml:"pattern,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Values      []string `yaml:"values,omitempty"`
	Min         *float64 `yaml:"min,omitempty"`
	Max         *float64 `yaml:"max,omitempty"`
	MinLength   *int     `yaml:"min_length,omitempty"`
	MaxLength   *int     `yaml:"max_length,omitempty"`
	Multiline   bool     `yaml:"multiline,omitempty"`
}

// Files describes how preset content maps to the rendered target tree.
type Files struct {
	Source            string   `yaml:"source"`
	TemplateSuffix    string   `yaml:"template_suffix,omitempty"`
	Ignore            []string `yaml:"ignore,omitempty"`
	BinaryExtensions  []string `yaml:"binary_extensions,omitempty"`
	LargeFilesAllowed bool     `yaml:"large_files_allowed,omitempty"`
}

// ComposeEntry references another preset to be applied as a layer.
//
// v0 parses but does not execute composition; a non-empty Composes slice
// triggers a validation error.
type ComposeEntry struct {
	ID      string            `yaml:"id"`
	Version string            `yaml:"version,omitempty"`
	Source  string            `yaml:"source,omitempty"`
	Path    string            `yaml:"path,omitempty"`
	Vars    map[string]string `yaml:"vars,omitempty"`
}

// PostRender declares a Hadron blueprint to invoke after rendering. v0
// validation emits a warning if present and the field is otherwise ignored.
type PostRender struct {
	Blueprint string            `yaml:"blueprint"`
	Inputs    map[string]string `yaml:"inputs,omitempty"`
}

// Sync declares the per-file sync policy applied by `folio sync` (deferred).
type Sync struct {
	Default string     `yaml:"default,omitempty"`
	Rules   []SyncRule `yaml:"rules,omitempty"`
}

// SyncRule applies a sync Policy to files matching Glob.
type SyncRule struct {
	Glob   string `yaml:"glob"`
	Policy string `yaml:"policy"`
}

// Default template suffix used when files.template_suffix is unset.
const DefaultTemplateSuffix = ".tmpl"

// TemplateSuffix returns the suffix used to mark templated files, falling
// back to DefaultTemplateSuffix when the preset doesn't override it.
func (f Files) TemplateSuffixOrDefault() string {
	if f.TemplateSuffix == "" {
		return DefaultTemplateSuffix
	}
	return f.TemplateSuffix
}
