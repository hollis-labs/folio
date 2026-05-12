package render

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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

// TreeOptions parameterises a RenderTree call. Source must be an fs.FS
// rooted at the preset's template tree (typically `<presetRoot>/files`).
// Pass os.DirFS for on-disk presets and a sub-FS of an embed.FS for
// bundled presets — the engine is unaware of which it received.
type TreeOptions struct {
	// Source is the filesystem to walk. It must be rooted at the template
	// tree, NOT at the preset directory itself.
	Source fs.FS
	// TemplateSuffix marks files that should be rendered (e.g. ".tmpl").
	// Files without this suffix are copied literally. Defaults to
	// DefaultTemplateSuffix when empty.
	TemplateSuffix string
	// Ignore is a list of glob patterns (path.Match syntax) applied to
	// each file's path relative to the FS root. Matches are skipped.
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
	if opts.Source == nil {
		return TreeResult{}, errors.New("render: TreeOptions.Source is required")
	}
	root, err := fs.Stat(opts.Source, ".")
	if err != nil {
		return TreeResult{}, fmt.Errorf("render: stat source: %w", err)
	}
	if !root.IsDir() {
		return TreeResult{}, fmt.Errorf("render: source root is not a directory")
	}

	suffix := opts.suffix()
	binaryExts := map[string]struct{}{}
	for _, e := range opts.BinaryExtensions {
		binaryExts[strings.ToLower(e)] = struct{}{}
	}

	var files []RenderedFile

	walkErr := fs.WalkDir(opts.Source, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == "." {
			return nil
		}

		// Ignore-glob match runs against the source-relative (pre-render)
		// path so authors author globs against the on-disk layout, not the
		// templated output.
		for _, pat := range opts.Ignore {
			matched, err := path.Match(pat, p)
			if err != nil {
				return fmt.Errorf("render: invalid ignore glob %q: %w", pat, err)
			}
			if matched {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}

		renderedRel, err := renderPath(p, ctx)
		if err != nil {
			return &Error{Phase: "path", File: p, Err: err}
		}

		if d.IsDir() {
			return nil
		}

		mode := fs.FileMode(0o644)
		if fi, err := d.Info(); err == nil {
			mode = fi.Mode().Perm()
		}

		ext := strings.ToLower(path.Ext(renderedRel))
		_, isBinary := binaryExts[ext]

		var (
			outPath    string
			content    []byte
			isTemplate bool
		)

		switch {
		case !isBinary && strings.HasSuffix(renderedRel, suffix):
			raw, err := fs.ReadFile(opts.Source, p)
			if err != nil {
				return fmt.Errorf("render: read template %s: %w", p, err)
			}
			rendered, err := RenderString(string(raw), ctx)
			if err != nil {
				if re, ok := err.(*Error); ok && re.File == "" {
					re.File = p
				}
				return err
			}
			outPath = strings.TrimSuffix(renderedRel, suffix)
			content = []byte(rendered)
			isTemplate = true
		default:
			raw, err := fs.ReadFile(opts.Source, p)
			if err != nil {
				return fmt.Errorf("render: read file %s: %w", p, err)
			}
			outPath = renderedRel
			content = raw
			isTemplate = false
		}

		files = append(files, RenderedFile{
			RelPath:    outPath,
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

// DirFSAt returns an fs.FS rooted at the given absolute directory. Tiny
// wrapper around os.DirFS so callers in this package don't have to import
// "os" just for the constructor.
func DirFSAt(dir string) fs.FS { return os.DirFS(dir) }

// renderPath renders any template directives inside path segments,
// preserving the separator structure. Empty path = empty result. Paths are
// forward-slash because fs.FS uses forward slashes.
func renderPath(rel string, ctx Context) (string, error) {
	if rel == "" {
		return "", nil
	}
	parts := strings.Split(rel, "/")
	for i, seg := range parts {
		if !strings.Contains(seg, "{{") {
			continue
		}
		out, err := RenderString(seg, ctx)
		if err != nil {
			return "", err
		}
		if strings.ContainsAny(out, "/\\") {
			return "", fmt.Errorf("path segment %q rendered to contain a path separator: %q", seg, out)
		}
		parts[i] = out
	}
	return strings.Join(parts, "/"), nil
}
