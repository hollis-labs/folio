package render

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// DefaultTemplateSuffix is the file suffix marking a template-rendered file.
// Mirrors preset.DefaultTemplateSuffix; redeclared here so this package has
// no dependency on internal/preset.
const DefaultTemplateSuffix = ".tmpl"

// RenderString evaluates a single Go text/template against ctx and returns
// the rendered string. Mirrors Hadron's renderString shape so templates
// written for either tool evaluate identically (within the shared funcmap
// subset).
//
// The "{{" fast-path is preserved: strings with no template directives are
// returned unchanged, matching Hadron's behavior at
// hadron/internal/blueprint/blueprint.go:1037-1050.
func RenderString(in string, ctx Context) (string, error) {
	if !strings.Contains(in, "{{") {
		return in, nil
	}
	tpl, err := template.New("folio").Option("missingkey=error").Funcs(FuncMap(ctx)).Parse(in)
	if err != nil {
		return "", &Error{Phase: "parse", Err: err}
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx.asMap()); err != nil {
		return "", &Error{Phase: "execute", Err: err}
	}
	return buf.String(), nil
}

// TreeOptions parameterises a RenderTree call. SourceDir must be the
// absolute path to the preset's template tree (typically `<presetRoot>/files`).
type TreeOptions struct {
	// SourceDir is the absolute path to the template source directory.
	SourceDir string
	// TemplateSuffix marks files that should be rendered (e.g. ".tmpl").
	// Files without this suffix are copied literally. Defaults to
	// DefaultTemplateSuffix when empty.
	TemplateSuffix string
	// Ignore is a list of glob patterns (filepath.Match syntax) applied to
	// each file's path relative to SourceDir. Matches are skipped entirely.
	Ignore []string
	// BinaryExtensions forces literal copy regardless of TemplateSuffix.
	// Entries should include the leading dot, e.g. ".png".
	BinaryExtensions []string
}

func (o TreeOptions) suffix() string {
	if o.TemplateSuffix == "" {
		return DefaultTemplateSuffix
	}
	return o.TemplateSuffix
}

// RenderedFile is a single file produced by RenderTree.
//
// RelPath is the path relative to the target directory after path-template
// resolution and TemplateSuffix stripping. Content is the rendered (or
// literal) file body. IsTemplate reports whether content was produced by
// template evaluation (false for literal copies and binaries).
type RenderedFile struct {
	RelPath    string
	Content    []byte
	IsTemplate bool
	Mode       os.FileMode
}

// TreeResult aggregates the files produced by a RenderTree call. Files is
// sorted by RelPath for deterministic downstream consumers (digest stability,
// manifest ordering, test assertions).
type TreeResult struct {
	Files []RenderedFile
}

// Error is returned by RenderString and RenderTree when template parsing or
// execution fails. It carries the source file (when known) and the phase
// the error originated in.
type Error struct {
	Phase   string
	File    string
	Err     error
}

func (e *Error) Error() string {
	switch {
	case e.File != "" && e.Phase != "":
		return fmt.Sprintf("render %s [%s]: %v", e.Phase, e.File, e.Err)
	case e.Phase != "":
		return fmt.Sprintf("render %s: %v", e.Phase, e.Err)
	default:
		return e.Err.Error()
	}
}

func (e *Error) Unwrap() error { return e.Err }

// RenderTree walks opts.SourceDir, applies path-template rendering to every
// directory and filename, then per file decides between ignore / literal
// copy / template render. The result is a sorted slice of RenderedFile
// values the service layer can write into a target directory.
//
// Two-pass shape (per design doc):
//  1. Render any {{ ... }} in the path (each directory + the filename) against
//     ctx, so `cmd/{{.inputs.project_name}}/main.go.tmpl` becomes
//     `cmd/foo/main.go.tmpl`.
//  2. Decide what to do with each resolved path:
//     - ignore glob match → skip
//     - binary extension or no template suffix → literal copy
//     - template suffix → render content, strip suffix
func RenderTree(opts TreeOptions, ctx Context) (TreeResult, error) {
	if opts.SourceDir == "" {
		return TreeResult{}, errors.New("render: TreeOptions.SourceDir is required")
	}
	info, err := os.Stat(opts.SourceDir)
	if err != nil {
		return TreeResult{}, fmt.Errorf("render: stat source dir: %w", err)
	}
	if !info.IsDir() {
		return TreeResult{}, fmt.Errorf("render: source %q is not a directory", opts.SourceDir)
	}

	suffix := opts.suffix()
	binaryExts := map[string]struct{}{}
	for _, e := range opts.BinaryExtensions {
		binaryExts[strings.ToLower(e)] = struct{}{}
	}

	var files []RenderedFile

	walkErr := filepath.WalkDir(opts.SourceDir, func(abs string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if abs == opts.SourceDir {
			return nil
		}

		relSource, err := filepath.Rel(opts.SourceDir, abs)
		if err != nil {
			return fmt.Errorf("render: rel %q: %w", abs, err)
		}

		// Apply path-template rendering segment by segment. We render whole
		// path; matching against ignore globs is done on the source-relative
		// (pre-render) path to keep glob authoring intuitive.
		for _, pat := range opts.Ignore {
			matched, err := filepath.Match(pat, relSource)
			if err != nil {
				return fmt.Errorf("render: invalid ignore glob %q: %w", pat, err)
			}
			if matched {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Refuse symlinks anywhere under source (security; matches the
		// validation rule documented in preset-yaml-validation-v0.md §5).
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("render: symlink not allowed under preset source: %s", relSource)
		}

		renderedRel, err := renderPath(relSource, ctx)
		if err != nil {
			return &Error{Phase: "path", File: relSource, Err: err}
		}

		if d.IsDir() {
			// We do not record directories as RenderedFile entries; the
			// writer will mkdir as needed from the file paths.
			return nil
		}

		mode := fs.FileMode(0o644)
		if fi, err := d.Info(); err == nil {
			mode = fi.Mode().Perm()
		}

		ext := strings.ToLower(filepath.Ext(renderedRel))
		_, isBinary := binaryExts[ext]

		var (
			outPath    string
			content    []byte
			isTemplate bool
		)

		switch {
		case !isBinary && strings.HasSuffix(renderedRel, suffix):
			raw, err := os.ReadFile(abs)
			if err != nil {
				return fmt.Errorf("render: read template %s: %w", relSource, err)
			}
			rendered, err := RenderString(string(raw), ctx)
			if err != nil {
				if re, ok := err.(*Error); ok && re.File == "" {
					re.File = relSource
				}
				return err
			}
			outPath = strings.TrimSuffix(renderedRel, suffix)
			content = []byte(rendered)
			isTemplate = true
		default:
			raw, err := os.ReadFile(abs)
			if err != nil {
				return fmt.Errorf("render: read file %s: %w", relSource, err)
			}
			outPath = renderedRel
			content = raw
			isTemplate = false
		}

		files = append(files, RenderedFile{
			RelPath:    filepath.ToSlash(outPath),
			Content:    content,
			IsTemplate: isTemplate,
			Mode:       mode,
		})
		return nil
	})
	if walkErr != nil {
		return TreeResult{}, walkErr
	}

	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return TreeResult{Files: files}, nil
}

// renderPath renders any template directives inside path segments,
// preserving the separator structure. Empty path = empty result.
func renderPath(rel string, ctx Context) (string, error) {
	if rel == "" {
		return "", nil
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		if !strings.Contains(p, "{{") {
			continue
		}
		out, err := RenderString(p, ctx)
		if err != nil {
			return "", err
		}
		if strings.ContainsAny(out, "/\\") {
			return "", fmt.Errorf("path segment %q rendered to contain a path separator: %q", p, out)
		}
		parts[i] = out
	}
	return filepath.FromSlash(strings.Join(parts, "/")), nil
}
