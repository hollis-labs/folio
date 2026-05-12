# Changelog

All notable changes to folio are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial v0 vertical slice: `folio new`, `folio plan`, `folio preset validate`
  against a bundled minimal `base` preset.
- `internal/preset` — preset.yaml parser + validator (v0 subset).
- `internal/render` — Go `text/template` render engine with a curated funcmap;
  Hadron-reconciled helper names (`basename`/`dirname`/`ext`, `json`); folio
  excludes `env` and `readFile` for security.
- `internal/manifest` — `.folio.yaml` writer/reader with SHA-256 per-file digests
  computed after LF newline normalization.
- `service/` — canonical Go API. CLI wraps it; future MCP/HTTP do the same.
- `cmd/folio` — Cobra CLI with `huh` interactive prompts, `--non-interactive`
  flag for agent/CI use, and reserved-stub commands (`sync`, `inspect`,
  `preset list`, `preset show`) that announce "not yet implemented in v0".
- `presets/base/` — bundled minimal Go-project preset, embedded via `embed.FS`.

[Unreleased]: https://github.com/hollis-labs/folio/compare/v0.1.0...HEAD
