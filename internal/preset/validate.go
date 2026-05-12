package preset

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Severity classifies a ValidationError.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// ValidationError describes one rule violation found by Validate.
//
// File/Line/Column are populated when the error can be tied to a specific
// YAML node; they may be zero for cross-cutting errors. Hint is optional.
type ValidationError struct {
	Severity Severity
	File     string
	Line     int
	Column   int
	Path     string // dotted field path, e.g. "inputs[0].pattern"
	Message  string
	Hint     string
}

// Error formats the diagnostic as `file:line:col: message`.
func (e ValidationError) Error() string {
	loc := e.File
	if loc == "" {
		loc = "<preset>"
	}
	if e.Line > 0 {
		loc = fmt.Sprintf("%s:%d:%d", loc, e.Line, e.Column)
	}
	out := fmt.Sprintf("%s: %s: %s", loc, e.Severity, e.Message)
	if e.Path != "" {
		out += " (" + e.Path + ")"
	}
	if e.Hint != "" {
		out += "\n  hint: " + e.Hint
	}
	return out
}

// Result aggregates validation findings. A Result is "ok" when Errors is
// empty; warnings do not invalidate the preset.
type Result struct {
	Errors   []ValidationError
	Warnings []ValidationError
}

// OK reports whether the result contains no errors.
func (r Result) OK() bool { return len(r.Errors) == 0 }

// Regexes used by validation rules. Compiled once at package init.
var (
	idPattern       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	identPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	semverPattern   = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	folioVerPattern = regexp.MustCompile(`^0\.\d+$`)
)

// reservedInputNames collides with template context root keys; using one as
// an input name shadows the root and breaks template lookups.
var reservedInputNames = map[string]struct{}{
	"target":   {},
	"preset":   {},
	"folio":    {},
	"now":      {},
	"computed": {},
	"inputs":   {},
}

// supportedInputTypes is the v0 enum for input.type.
var supportedInputTypes = map[string]struct{}{
	"string":       {},
	"bool":         {},
	"number":       {},
	"enum":         {},
	"list[string]": {},
}

// Validate runs the v0 validation rule set against the parsed preset.
//
// Rules covered (per plan §2 P1):
//   - folio_version, id, version, files are required.
//   - id matches ^[a-z][a-z0-9-]*$ (2..64 chars).
//   - version is semver (MAJOR.MINOR.PATCH[-pre][+meta]).
//   - folio_version is 0.<minor>.
//   - inputs[*].name is identifier-shaped, unique, not reserved.
//   - inputs[*].type is one of supportedInputTypes.
//   - inputs[*].pattern compiles (string type only).
//   - inputs[*].default is type-compatible with inputs[*].type.
//   - inputs[*].values is non-empty when type is enum, and default (if set)
//     is one of values.
//   - computed keys are identifier-shaped, unique, do not collide with input
//     names.
//   - files.source is set; path-safe relative to preset root (no .. escape).
//   - files.template_suffix starts with '.'.
//   - composes is empty (v0 error if non-empty).
//   - post_render present emits a warning.
//   - sync is parsed but not validated beyond enum membership of policies.
func (p *Preset) Validate() Result {
	var r Result

	p.validateTopLevel(&r)
	p.validateInputs(&r)
	p.validateComputed(&r)
	p.validateFiles(&r)
	p.validateComposes(&r)
	p.validatePostRender(&r)
	p.validateSync(&r)

	return r
}

func (p *Preset) addErr(r *Result, path, msg, hint string) {
	r.Errors = append(r.Errors, p.makeErr(SeverityError, path, msg, hint))
}

func (p *Preset) addWarn(r *Result, path, msg, hint string) {
	r.Warnings = append(r.Warnings, p.makeErr(SeverityWarning, path, msg, hint))
}

func (p *Preset) makeErr(sev Severity, path, msg, hint string) ValidationError {
	e := ValidationError{
		Severity: sev,
		File:     p.sourceFile,
		Path:     path,
		Message:  msg,
		Hint:     hint,
	}
	if n := p.nodeAt(yamlPath(path)); n != nil {
		e.Line = n.Line
		e.Column = n.Column
	}
	return e
}

// yamlPath converts a dotted field path like "inputs[2].pattern" to the
// slash form nodeAt understands: "inputs/2/pattern".
func yamlPath(p string) string {
	out := strings.Builder{}
	for _, r := range p {
		switch r {
		case '.':
			out.WriteByte('/')
		case '[':
			out.WriteByte('/')
		case ']':
			// drop
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func (p *Preset) validateTopLevel(r *Result) {
	if p.FolioVersion == "" {
		p.addErr(r, "folio_version", "missing required field", `set folio_version: "0.1"`)
	} else if !folioVerPattern.MatchString(p.FolioVersion) {
		p.addErr(r, "folio_version", fmt.Sprintf("unsupported folio_version %q", p.FolioVersion), "v0 supports 0.<minor>; bump folio or downgrade the preset")
	}

	if p.ID == "" {
		p.addErr(r, "id", "missing required field", `id must match ^[a-z][a-z0-9-]*$ (2..64 chars)`)
	} else {
		if !idPattern.MatchString(p.ID) {
			p.addErr(r, "id", fmt.Sprintf("invalid id %q", p.ID), `must match ^[a-z][a-z0-9-]*$ (lowercase letters, digits, hyphens; start with a letter)`)
		}
		if l := len(p.ID); l < 2 || l > 64 {
			p.addErr(r, "id", fmt.Sprintf("id length %d out of range", l), "id must be 2..64 characters")
		}
	}

	if p.Version == "" {
		p.addErr(r, "version", "missing required field", "version must be MAJOR.MINOR.PATCH semver")
	} else if !semverPattern.MatchString(p.Version) {
		p.addErr(r, "version", fmt.Sprintf("invalid semver %q", p.Version), "expected MAJOR.MINOR.PATCH[-pre][+meta]")
	}

	if p.Files.Source == "" {
		p.addErr(r, "files", "files.source is required", "set files.source to the directory containing template files (relative to preset.yaml)")
	}
}

func (p *Preset) validateInputs(r *Result) {
	seen := map[string]int{}
	for i, in := range p.Inputs {
		base := fmt.Sprintf("inputs[%d]", i)

		if in.Name == "" {
			p.addErr(r, base+".name", "input name is required", "")
			continue
		}
		if !identPattern.MatchString(in.Name) {
			p.addErr(r, base+".name", fmt.Sprintf("invalid input name %q", in.Name), `must match ^[a-z][a-z0-9_]*$`)
		}
		if _, reserved := reservedInputNames[in.Name]; reserved {
			p.addErr(r, base+".name", fmt.Sprintf("input name %q is reserved (collides with template context root)", in.Name), "rename the input")
		}
		if prev, dup := seen[in.Name]; dup {
			p.addErr(r, base+".name", fmt.Sprintf("input name %q is not unique (previous declaration at inputs[%d])", in.Name, prev), "")
		} else {
			seen[in.Name] = i
		}

		if in.Type == "" {
			p.addErr(r, base+".type", "input type is required", "supported types: string, bool, number, enum, list[string]")
		} else if _, ok := supportedInputTypes[in.Type]; !ok {
			p.addErr(r, base+".type", fmt.Sprintf("unsupported input type %q", in.Type), "supported types: string, bool, number, enum, list[string]")
		}

		if in.Pattern != "" {
			if in.Type != "string" {
				p.addErr(r, base+".pattern", "pattern is only valid for type: string", "")
			}
			if _, err := regexp.Compile(in.Pattern); err != nil {
				p.addErr(r, base+".pattern", fmt.Sprintf("invalid regex: %v", err), "Go regexp/syntax")
			}
		}

		if in.Type == "enum" {
			if len(in.Values) == 0 {
				p.addErr(r, base+".values", "enum input requires non-empty values list", "")
			} else if in.Default != nil {
				if s, ok := in.Default.(string); ok {
					found := false
					for _, v := range in.Values {
						if v == s {
							found = true
							break
						}
					}
					if !found {
						p.addErr(r, base+".default", fmt.Sprintf("default %q is not one of values %v", s, in.Values), "")
					}
				} else {
					p.addErr(r, base+".default", "enum default must be a string from values", "")
				}
			}
		}

		if in.Default != nil {
			if err := checkDefaultType(in.Type, in.Default); err != nil {
				p.addErr(r, base+".default", err.Error(), "")
			}
		}

		if in.MinLength != nil || in.MaxLength != nil {
			if in.Type != "string" {
				p.addErr(r, base+".min_length", "min_length/max_length only valid for type: string", "")
			}
		}
		if in.Min != nil || in.Max != nil {
			if in.Type != "number" {
				p.addErr(r, base+".min", "min/max only valid for type: number", "")
			}
		}
	}
}

func checkDefaultType(t string, v any) error {
	switch t {
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("default value %v is not a string", v)
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("default value %v is not a bool", v)
		}
	case "number":
		switch v.(type) {
		case int, int64, float64:
			// ok
		default:
			return fmt.Errorf("default value %v is not a number", v)
		}
	case "list[string]":
		seq, ok := v.([]any)
		if !ok {
			return fmt.Errorf("default value %v is not a list", v)
		}
		for i, item := range seq {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("default list element %d (%v) is not a string", i, item)
			}
		}
	}
	return nil
}

func (p *Preset) validateComputed(r *Result) {
	inputNames := map[string]struct{}{}
	for _, in := range p.Inputs {
		inputNames[in.Name] = struct{}{}
	}
	for key := range p.Computed {
		path := "computed." + key
		if !identPattern.MatchString(key) {
			p.addErr(r, path, fmt.Sprintf("invalid computed key %q", key), `must match ^[a-z][a-z0-9_]*$`)
		}
		if _, dup := inputNames[key]; dup {
			p.addErr(r, path, fmt.Sprintf("computed key %q collides with an input name", key), "rename one of them")
		}
	}
}

func (p *Preset) validateFiles(r *Result) {
	if p.Files.Source == "" {
		return // already reported by validateTopLevel
	}
	if p.Files.TemplateSuffix != "" && !strings.HasPrefix(p.Files.TemplateSuffix, ".") {
		p.addErr(r, "files.template_suffix", fmt.Sprintf("template_suffix %q must start with a dot", p.Files.TemplateSuffix), "")
	}
	if p.sourceFile != "" {
		root := p.PresetRoot()
		abs := p.Files.Source
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, p.Files.Source)
		}
		clean := filepath.Clean(abs)
		rel, err := filepath.Rel(root, clean)
		if err != nil || strings.HasPrefix(rel, "..") {
			p.addErr(r, "files.source", fmt.Sprintf("files.source %q escapes the preset root", p.Files.Source), "use a path under the preset directory")
		}
	}
}

func (p *Preset) validateComposes(r *Result) {
	if len(p.Composes) > 0 {
		p.addErr(r, "composes",
			"composes is not yet implemented in v0",
			"remove the composes block or upgrade folio when composition lands in v0.2")
	}
}

func (p *Preset) validatePostRender(r *Result) {
	if p.PostRender != nil && p.PostRender.Blueprint != "" {
		p.addWarn(r, "post_render",
			"post_render is not yet implemented in v0; the hook will be ignored at generation time",
			"keep the field for forward-compatibility, but expect no Hadron invocation in v0")
	}
}

// supportedSyncPolicies caps the sync.default / sync.rules[].policy enum.
var supportedSyncPolicies = map[string]struct{}{
	"prompt":    {},
	"overwrite": {},
	"skip":      {},
	"three-way": {},
}

func (p *Preset) validateSync(r *Result) {
	if p.Sync == nil {
		return
	}
	if p.Sync.Default != "" {
		if _, ok := supportedSyncPolicies[p.Sync.Default]; !ok {
			p.addErr(r, "sync.default", fmt.Sprintf("unsupported sync policy %q", p.Sync.Default), "supported: prompt, overwrite, skip, three-way")
		}
	}
	for i, rule := range p.Sync.Rules {
		base := fmt.Sprintf("sync.rules[%d]", i)
		if rule.Glob == "" {
			p.addErr(r, base+".glob", "rule glob is required", "")
		} else if _, err := filepath.Match(rule.Glob, ""); err != nil {
			p.addErr(r, base+".glob", fmt.Sprintf("invalid glob: %v", err), "")
		}
		if rule.Policy == "" {
			p.addErr(r, base+".policy", "rule policy is required", "supported: prompt, overwrite, skip, three-way")
		} else if _, ok := supportedSyncPolicies[rule.Policy]; !ok {
			p.addErr(r, base+".policy", fmt.Sprintf("unsupported sync policy %q", rule.Policy), "supported: prompt, overwrite, skip, three-way")
		}
	}
}
