# Changelog

All notable changes to folio are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-05-12

First public release. v0 vertical slice: prove the preset format + render
engine + manifest end-to-end against a bundled minimal `base` preset.

### Added

- **CLI commands**
  - `folio new <preset> <target-dir>` renders a preset into a new project,
    prompting for missing inputs (huh) or failing loudly under
    `--non-interactive`.
  - `folio plan <preset> <target-dir>` previews the same render with no
    writes; prints resolved inputs + computed values + planned file list.
  - `folio preset validate <preset-dir>` runs the v0 validation rule set
    over preset.yaml.
  - Reserved stubs (`folio sync`, `folio inspect`, `folio preset list`,
    `folio preset show`) print "not yet implemented in v0" and exit 1.
- **Preset format (v0 schema, folio_version: "0.1")**
  - Typed inputs: `string`, `bool`, `number`, `enum`, `list[string]`, with
    pattern/min_length/max_length/min/max validation hooks.
  - `computed:` block — Go templates evaluated against `.inputs.*` + `.now`
    + `.preset.*` + `.folio.*`.
  - `files.source` + `files.template_suffix` (default `.tmpl`) + `ignore`
    glob list + `binary_extensions`.
  - `composes:` (parsed but blocked — v0 errors on non-empty),
    `post_render:` (parsed, warns + ignored), `sync:` (parsed and stored).
- **Render engine** — Go `text/template` with a curated funcmap. Helper
  names match Hadron where shared (`basename`, `dirname`, `ext`, `json`,
  `default`, `ternary`) so templates portable between the two tools
  evaluate identically; folio adds case-conversion, quoting/escaping,
  encoding, date/time, lists/dicts, and the folio-specific `licenseHeader`,
  `gomodPath`, `gitUser`, `spdxId` helpers. `env` and `readFile` are
  deliberately excluded (security divergence from Hadron; documented).
- **Manifest** — `.folio.yaml` written into every generated project,
  recording preset identity, resolved inputs, computed values, and a
  SHA-256 digest per file (computed after LF newline normalisation so
  digests are platform-stable).
- **Bundled `base` preset** — minimal Go-project layout. Generates README,
  go.mod, Makefile, LICENSE (MIT), .gitignore, .github/workflows/ci.yml,
  and `cmd/{{project_name}}/main.go`. The resulting project passes
  `go vet`, `go test`, and `go build` out of the box.
- **Service layer** — `service.Service` is the canonical Go API; the CLI
  imports it directly. MCP / HTTP / ACP surfaces in future versions will
  wrap the same service. Typed `*service.Error` with stable codes
  (`preset_not_found`, `input_missing`, `target_exists`, …) so future
  envelopes can pattern-match without parsing English.

### Deferred to v0.x / v1.x

Captured in Vanta as follow-ups:

- `composes:` runtime — preset composition layering.
- `folio sync` + two-way diff UI; later three-way merge via `git merge-file`.
- Post-render Hadron blueprint invocation.
- MCP / HTTP / ACP wire surfaces.
- Federated preset sources via git URLs
  (`followups.folio.preset_sources.federated_git_urls`).
- Snapshot storage (`.folio/snapshots/`) for true three-way merge.
- `licenseHeader` for non-MIT SPDX values.
- `folio inspect` rich UI.
- Cross-platform CRLF support (LF-only documented for v0).

[0.1.0]: https://github.com/hollis-labs/folio/releases/tag/v0.1.0
