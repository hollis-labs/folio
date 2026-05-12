package preset

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Parse reads preset.yaml at path and returns the parsed Preset. It does NOT
// run validation; callers usually pair Parse with (*Preset).Validate.
//
// On YAML syntax errors, Parse wraps the underlying yaml.v3 error so callers
// see the file path alongside the diagnostic.
func Parse(path string) (*Preset, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("preset: resolve path %s: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("preset: read %s: %w", abs, err)
	}
	p, err := ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	p.sourceFile = abs
	return p, nil
}

// ParseBytes parses a preset.yaml from a byte slice. Useful for tests and for
// loading embedded presets via embed.FS.
func ParseBytes(data []byte) (*Preset, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("preset: yaml parse: %w", err)
	}
	var p Preset
	if err := root.Decode(&p); err != nil {
		return nil, fmt.Errorf("preset: yaml decode: %w", err)
	}
	p.rootNode = &root
	return &p, nil
}

// PresetRoot returns the directory containing the parsed preset.yaml file,
// or the empty string when the preset was loaded from a byte slice without
// a path. It is used by validators that need to resolve relative paths like
// files.source.
func (p *Preset) PresetRoot() string {
	if p.sourceFile == "" {
		return ""
	}
	return filepath.Dir(p.sourceFile)
}

// nodeAt walks the parsed YAML node tree following a slash-separated path of
// mapping keys (e.g. "inputs/0/name", "files/source") and returns the matching
// node, or nil if the path does not resolve. Used to attach file:line:col to
// validation errors.
func (p *Preset) nodeAt(path string) *yaml.Node {
	if p.rootNode == nil {
		return nil
	}
	// Document node wraps the actual root mapping.
	cur := p.rootNode
	if cur.Kind == yaml.DocumentNode && len(cur.Content) > 0 {
		cur = cur.Content[0]
	}
	if path == "" {
		return cur
	}
	for _, seg := range splitPath(path) {
		cur = childNode(cur, seg)
		if cur == nil {
			return nil
		}
	}
	return cur
}

func splitPath(path string) []string {
	out := []string{}
	cur := ""
	for _, r := range path {
		if r == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func childNode(parent *yaml.Node, seg string) *yaml.Node {
	if parent == nil {
		return nil
	}
	switch parent.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(parent.Content); i += 2 {
			k := parent.Content[i]
			v := parent.Content[i+1]
			if k.Value == seg {
				return v
			}
		}
		return nil
	case yaml.SequenceNode:
		idx := 0
		neg := false
		for _, r := range seg {
			if r == '-' {
				neg = true
				continue
			}
			if r < '0' || r > '9' {
				return nil
			}
			idx = idx*10 + int(r-'0')
		}
		if neg || idx >= len(parent.Content) {
			return nil
		}
		return parent.Content[idx]
	}
	return nil
}
