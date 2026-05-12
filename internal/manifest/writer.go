package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Write serialises m to <targetDir>/.folio.yaml. The encoder is configured
// with 2-space indent and the default yaml.v3 behaviour of sorting map
// keys alphabetically, so the same Manifest value produces byte-identical
// output across calls (the property the round-trip test in writer_test.go
// asserts).
//
// targetDir must already exist; Write does not mkdir on its caller's
// behalf because the service layer is responsible for the target directory
// lifecycle.
func Write(targetDir string, m Manifest) error {
	data, err := Marshal(m)
	if err != nil {
		return err
	}
	path := filepath.Join(targetDir, ManifestFilename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("manifest: write %s: %w", path, err)
	}
	return nil
}

// Marshal returns the canonical YAML byte form of m. Useful for callers
// that want to compute a digest of the manifest itself or write to a
// non-filesystem sink.
func Marshal(m Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&m); err != nil {
		_ = enc.Close()
		return nil, fmt.Errorf("manifest: marshal: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("manifest: close encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// Read parses <targetDir>/.folio.yaml into a Manifest value. Returns an
// error if the file is missing or malformed.
func Read(targetDir string) (Manifest, error) {
	path := filepath.Join(targetDir, ManifestFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest: read %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("manifest: parse %s: %w", path, err)
	}
	return m, nil
}
