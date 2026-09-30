# Changelog

All notable changes to folio are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Added

- **`go-service-app` preset (0.1.0): a transport-boundary lint gate for new service apps.**
  Composed on `go-baseline`; renders `go.mod`, a `cmd/<repo_name>` composition root,
  `.golangci.transport.yml` (a standalone second golangci-lint config: forbidigo Rule A
  forbids store-handle method calls and Rule C raw `database/sql` calls inside the transport
  packages, with an optional commented depguard Rule B), `docs/transport-boundary.md`
  (portfolio baseline measured at recorded app commits, ratchet plan R0-R3 with a ceiling
  script, parity checklist, limits, CI requirements) and
  `.github/quality/transport-baseline.txt` (ceiling 0). Inputs: `repo_name`,
  `store_pkg_name`, `store_type` (required), `transport_dirs_regex`, `store_pkg_path`, plus
  go-baseline's (`go_version` defaults 1.26.6). Composed on `go-baseline` rather than `base`
  because `base` renders `ci.yml` with a literal `go-version` (the gate requires
  `go-version-file: go.mod`), a Makefile and a hello-world program; the cost is that this
  preset owns `go.mod` and one Go package, as `go-lib` does. The integration tests render it,
  run `golangci-lint config verify` on the result and run the rendered config against a
  synthetic module (positive control: `a.Store.Get(1)` in a transport is flagged, `store.Row`
  in a signature, the service layer and a path outside the regex are not). A missing
  golangci-lint is a visible skip, a failure under `FOLIO_REQUIRE_TOOLS=1`.

## [0.2.0] — 2026-09-29

Composition slice. `composes:` becomes a working layered render driving a
real semver constraint resolver, with the first bundled composing preset
(`go-package`) layering an internal library on top of `base`. Adds an
opt-in `--create-github-repo` flow that runs after a successful render
and publishes the generated tree to GitHub via the user's `gh` CLI.

### Added

- **`go-baseline` layer preset (0.1.0) and `go-lib` 0.3.0 composed on it.**
  `go-baseline` renders what every Go module should start with — `check.yml`,
  `.gitignore` (now ignoring `go.work` / `go.work.sum`), `.golangci.yml`,
  `lefthook.yml`, `LICENSE`, a `## Unreleased` `CHANGELOG.md`, `AGENTS.md` and a
  one-line `CLAUDE.md` — and holds the shared optional inputs once
  (`go_version`, `go_toolchain`, `copyright_holder`, `go_packages`, the
  `agents_*` bodies). It is a layer: compose it, do not render it directly.
  `go-lib` keeps only what is library-specific (`go.mod`, the package, an
  `ExampleHello`, `examples/hello`, README, `release.yml`) and redeclares its
  required inputs, because the CLI prompts only for the root preset's inputs.
  `github_owner` is now pinned to `hollis-labs` so the module path cannot be a
  typo away from the wrong org. New `go-lib` inputs: `stability`
  (`pre-1.0|stable`, shapes the README `## Compatibility` paragraph) and
  `package_layout` (`subdir|root`; the preset previously claimed a module-root
  package while rendering a `<package>/` directory).
- **A `go-lib` render is green in CI by construction.** The previous `check.yml`
  pinned golangci-lint v2.1.6 — built with go1.24, so it refuses any `go 1.26.x`
  module — used `go-version: stable`, `govulncheck@latest`, and never ran
  `go mod verify` / `tidy -diff`. The new workflow reads the Go pin from
  `go.mod` (`setup-go@v6`), pins golangci-lint v2.11.4 and govulncheck v1.8.0,
  verifies modules, and fails when `go list ./...` finds no packages. `go-lib`
  also renders a `release.yml` that refuses a tag with no CHANGELOG heading and
  publishes the section as the GitHub Release notes. The new workflows have not
  yet run in GitHub Actions; the first lib built from this preset is their
  proof.
- **`scripts/check-lib-conformance.sh <dir> [--release] [--report]`** reports a
  tree against the lib checklist (B1-B8, B10, B15-B18, C4, F1-F4; `--release` adds
  unresolved `TODO(author)` and a CHANGELOG heading per tag on HEAD). Exit 0 all
  pass, 1 any fail, 2 nothing examined, 3 a tool is missing; `--report` never
  exits 1. It is a bridge, not a permanent part of folio: remove it when the
  checklist is promoted and folio owns the conformance definition, or `folio`
  grows a conformance command.
- **`go-lib` integration tests that run the tools.** `gofmt`, `go build`,
  `go vet`, `go test -race`, `go mod verify`, `go mod tidy -diff` and
  `golangci-lint` (at the version parsed from the rendered `check.yml`, asserting
  it was built with a Go at least as new as the module's) run on a fresh render
  of each `package_layout` / `stability` / `go_toolchain` variant; `FOLIO_NET=1`
  adds govulncheck. A missing tool is a visible skip, and a failure under
  `FOLIO_REQUIRE_TOOLS=1`. Every conformance item has a positive control: a
  mutation of the tree that must turn exactly that item FAIL.

- **Shared portfolio hook configs are now folio's to own.** `lefthook.yml`
  and `.golangci.yml` ship from the `base`, `go-lib`, `nanite-plugin` and
  `sysop-ui` presets, replacing per-repo copies whose headers pointed at a
  `mentat` reference repo that no longer exists. With no live upstream the
  copies had silently forked: all four `.golangci.yml` in the portfolio
  differed, and so did the two `lefthook.yml`. The rendered header now names
  the folio preset as the canonical copy, and `sync` marks both files
  `overwrite` so a re-render restores them.
- **`go_version` and `go_packages` inputs.** `go_version` renders into
  `go.mod`, the CI workflow and golangci-lint's `go:` key from one value so
  the three cannot drift (the previous copies disagreed: `go: "1.25"` against
  a `go.mod` saying `1.23`). `go_packages` (default `./...`) parameterises the
  package pattern the `go-vet` / `go-test` hooks run against — the exact
  deviation that forked one repo's copy by hand.
- **A frontend lint hook that actually runs.** `sysop-ui` and
  `nanite-plugin` (under `include_ui`) render a `frontend-lint` command
  invoking the lockfile-installed `./node_modules/.bin/biome`, plus a
  `biome.json` and a pinned `@biomejs/biome` devDependency. The hook it
  replaces ran `npx biome`, which does not resolve `@biomejs/biome` — it
  fetches `biome`, an unrelated env-var tool last published in 2016 whose CLI
  exits 0 on every input, including invalid TypeScript. Four portfolio repos
  shipped that hook and none of them ever linted anything. `base` and
  `go-lib` deliberately carry no frontend hook at all.
- **GitHub publish flow (opt-in, CLI-driven).** `folio new` gains
  `--create-github-repo` plus `--github-owner`, `--github-repo`,
  `--github-visibility` (`public`/`private`/`internal`, default
  `private`), `--github-description`, `--github-branch` (default `main`),
  and `--github-no-push`. When the flag is set, folio preflights the
  user's `gh` and `git` binaries + `gh auth status` BEFORE rendering,
  runs the normal render, then `git init` + initial commit + `gh repo
  create --source=. --remote=origin [--push]`. On a publish failure the
  local tree is preserved and the CLI prints an explicit `gh repo
  create ...` retry command keyed to the resolved options. Defaults
  pull from `inputs.github_owner` / `inputs.description` so the
  bundled presets work without extra `--github-*` flags in the common
  case. Branch protection / repo settings (issues, wiki, default-branch
  rules) deferred to a follow-up — v0.2 ships repo creation + push
  only.
- **`internal/github/` package.** Wraps `gh` and a small `git` surface
  behind a `Runner` interface so unit tests can substitute a fake exec
  layer. Typed `*github.Error` carries stable codes (`gh_missing`,
  `gh_unauthenticated`, `git_missing`, `repo_exists`, `publish_failed`)
  that the CLI / future MCP envelopes pattern-match on.
- **`Service.PublishToGitHub`.** Canonical service-layer entry point
  for the publish flow. Symmetric with `Service.New` — `New` is
  render-only, `PublishToGitHub` operates on an already-rendered tree.
  Future MCP / HTTP surfaces wrap the same method without duplicating
  the publish pipeline. Validates owner/repo/visibility and lifts
  `internal/github` errors into the `service.ErrorCode` namespace.
- **`integration_github_test.go` (gated).** End-to-end round-trip that
  renders `base` and publishes to a real GitHub repo named
  `folio-e2e-<unixnano>`, then deletes it. Gated on
  `FOLIO_GH_E2E=1`; skipped in CI by default. `FOLIO_GH_E2E_OWNER`
  overrides the owner (defaults to the `gh`-authenticated user).


- **`composes:` runtime.** Presets can declare `composes: [{id, version,
  source: local, path, vars}]`; folio walks the compose DAG (cycle
  detection + depth cap at 8), resolves each entry's version constraint
  against the loaded preset, and renders layers in topological order
  (deepest leaves first, root last). Same-path writes across layers
  silently overwrite — last writer wins, paralleling how presets author
  composing overlays.
- **Semver constraint resolver.** npm-style operators (`>=`, `<=`, `>`,
  `<`, `~`, `^`, `*`, exact), AND-via-comma (`>=1.0,<2.0`), OR-via-pipe
  (`^1.0 || ^2.0`). Partial versions canonicalize (`1.0` → `1.0.0`).
  Replaces the v0.1 lexicographic-pick-highest in `service.findUserPreset`
  — superseding the `single_version_preset_storage_v0` limitation captured
  at v0.1 ship.
- **Cross-layer var scoping.** Per the design doc §7, a composed preset
  inherits caller `.inputs.*` by default; the composing preset can
  per-key override via the entry's `vars:` block. Templates inside `vars:`
  evaluate against the *caller's* render context (so `{{.inputs.foo}}`
  resolves to the caller's input named `foo`, not the composed preset's).
- **Cross-layer computed inheritance.** Later layers' `computed:` templates
  see earlier layers' resolved values via `.computed.*`. Cross-layer key
  collision is last-writer-wins, paralleling file overwrites.
- **Cross-layer input inheritance.** A layer's render context sees every
  prior layer's resolved/defaulted inputs in addition to its own declared
  keys, so composing presets needn't redeclare every input from a layer
  they compose.
- **`go-package` bundled preset.** First composing preset shipped in the
  binary. Layers on `base` to add `internal/<package_name>/` library code +
  test, overwrites `README.md` and the base `cmd/<project>/main.go` with a
  library-first stub. Exercises both the additive and overwrite directions
  of composition.
- **`go-lib` bundled preset.** `folio new go-lib <dir>` scaffolds an
  importable Go shared library — one package (no `cmd/`,
  no `internal/`, no Makefile), plus `CHANGELOG.md`, MIT `LICENSE`, an
  `examples/` placeholder, and a `check.yml` CI workflow (gofmt, vet,
  golangci-lint, `test -race`, govulncheck). Reproduces the hand-built
  `libs/go-providers` / `libs/go-agent-launch` layout so portfolio shared
  libraries stop being hand-assembled. Standalone (not composed on
  `base`): `base` is binary-oriented and its `project_name` input forbids
  the hyphen a `go-*` module name needs.
- **`sysop-ui` bundled preset.** `folio new sysop-ui <dir>` scaffolds a
  complete Sysop UI app in one command: a `@hollis-labs/sysop-ui` React
  frontend (Vite + Tailwind v4, nav-rail shell, a starter page, a
  same-origin API client) plus a Go binary that embeds the built frontend
  and serves it through the shared `github.com/hollis-labs/go-webui`
  harness. Ships `ui-build` / `ui-dev` Makefile targets; the frontend
  builds straight into the Go `//go:embed` directory. Standalone (not
  composed on `base`) — the Go-module-plus-frontend layout diverges too
  far from `base` for an overlay to pay off.
- **Multi-entry `.folio.yaml` `presets:`.** The array now carries every
  contributing layer in apply order; per-file `preset:` records the last
  layer that produced the file. Verified byte-identical round-trip.
- **CLI error visibility.** Non-zero exits now print `folio: <error>` to
  stderr (previously silenced by cobra `SilenceErrors`). Pre-existing v0.1
  papercut, fixed here because composition errors are richer and worth
  surfacing.
- **CLI input passthrough.** `--input` pairs are now forwarded verbatim to
  the service even when they aren't declared on the top-level preset —
  needed so composed-layer inputs (e.g., `--input project_name=...` for a
  `go-package` invocation, where `project_name` is declared on `base`)
  reach the right layer.

### Changed

- **Folio's own `go.mod` is `go 1.26.6`, and its CI reads the Go version from it**
  (`go-version-file: go.mod` in all three jobs, replacing `'1.25'`). govulncheck
  judges the stdlib by that line; at `go 1.26.1` it reported reachable stdlib
  advisories through `service.Service.New`.
- **`go_version` defaults to `1.26.6` in every bundled preset** (`base` was
  `1.23`; `go-lib`, `nanite-plugin`, `sysop-ui` were `1.26.1`). govulncheck
  judges the standard library by go.mod's `go` line, so a lower floor reports
  advisories fixed in go1.26.4-1.26.6 on a lib that touches `net/http`. The
  `sysop-ui` README now renders the input instead of a hard-coded "Go 1.26.1+".
  Optional `go_toolchain` (default none) renders a `toolchain` directive as an
  escape hatch for a later advisory.

- **`service.New`'s writer moves to the shared `go-materialize` engine**
  (CW-20260918-0036), replacing a direct `os.MkdirAll`/`os.WriteFile` loop
  with atomic staged writes and symlink-parent rejection at the target's
  ancestor chain — neither existed before. A path-traversal gap found
  during the crosswalk that produced this change is fixed at its source
  too: `internal/render.renderPath` now rejects a rendered path segment
  that is exactly `".."` (previously it only rejected a segment containing
  a path separator), so an unvalidated preset input used in a directory
  segment can no longer resolve outside the target directory — see
  `TestRenderTree_RejectsPathSegmentTraversal`. No change to what gets
  rendered or to `.folio.yaml`'s shape or digests.
- `service.findUserPreset` takes a `*compose.Constraint` parameter. `nil`
  picks the semver-highest version overall (replacing the v0.1 lexicographic
  shortcut); a non-nil constraint filters and surfaces a typed
  `ErrPresetNotFound` with the available-versions list when nothing matches.
- `service.LoadedPreset` gains unexported compose-context fields populated
  by `LoadPreset` so the compose loader can walk the source-root FS for
  relative `composes[].path` resolution.
- `internal/preset.validateComposes` (hard-error on any `composes:` entry)
  is replaced by `validateComposeEntries` running per-entry shape rules
  (id pattern, non-empty version constraint, source enum, required path,
  identifier-shaped vars keys). Cross-preset rules (vars key must name a
  declared input on the composed preset, cycle detection, depth cap) run
  in `internal/compose` at compose time.
- `service.resolveComputed` now seeds its output map from `ctx.Computed`
  instead of clobbering it, so cross-layer computed inheritance works.
  Single-preset path unchanged (caller passes empty `Computed`).
- `service.prepareRender` no longer pre-resolves the root preset's
  inputs/computed; that work moves to `service.composedLayers` so each
  layer (composed or single-preset) resolves uniformly against its own
  declared schema.
- **`sysop-ui` frontend sources are biome-clean.** `frontend/src/**` and
  `vite.config.ts` were reformatted to the shipped `biome.json` so a freshly
  rendered project passes its own pre-commit hook rather than failing on its
  first commit. `sysop-ui`'s `.github/workflows/ci.yml` became `ci.yml.tmpl`
  to carry `go_version`.

### Composition example

```sh
folio new go-package /tmp/folio-compose \
  --input project_name=smoke_compose \
  --input github_owner=chrispian \
  --input package_name=greeter \
  --non-interactive
```

Generates a project with `base`'s files (`go.mod`, `Makefile`, `.gitignore`,
`LICENSE`, `.github/workflows/ci.yml`) plus `go-package`'s additions
(`internal/greeter/greeter.go`, `internal/greeter/greeter_test.go`) and
overlays (`README.md` describing the library, `cmd/smoke_compose/main.go`
calling into the library). `.folio.yaml` records both layers in apply
order. The generated tree passes `go vet`, `go build`, `go test` clean.

### Fixed (review pass)

- **Read-only generated files** — The render engine copied each source
  template's permission bits verbatim. Bundled presets live in an
  `embed.FS`, which reports every file as `0o444`, so scaffolded projects
  landed read-only: `go mod tidy` and ordinary edits failed with
  "permission denied". `render` now OR-s `0o644` into the resolved mode —
  generated files are always owner-writable, while executable bits from a
  real-filesystem source preset are still preserved.
- **Diamond compose-entry binding** — In compose graphs where the same
  preset is reached via two parents, `LayerRef.ComposeEntry` now records
  the FIRST parent's entry (declared-order encounter, locked in
  BuildGraph). v0.2 does not support diamonds with conflicting `vars:`
  blocks under different parents; the first-parent-wins choice avoids
  the silently-arbitrary "last-stored" behavior the original rebuild
  produced.
- **`layerInputs` no longer leaks undeclared user keys** — The per-layer
  render context is now `mergedInputs ∪ declared` only; raw user keys
  that no layer declares (e.g., a typo like `--input bogous=value`) are
  dropped after the per-layer `resolveInputs` filter, preventing them
  from reaching templates or `.folio.yaml` `inputs:`.
- **`ResolveComposePath` tightened** — Rejects absolute entry paths
  (leading `/`) and any cleaned result equal to `..` or starting with
  `../`, regardless of root. Previously the root=`"."` short-circuit
  could let traversal escapes through to a downstream `fs.Sub` failure.
- **`coerceInput` accepts comma-strings for `list[string]`** — Now that
  the CLI passes undeclared `--input` pairs through verbatim, the
  service-side coercer needs to accept the same comma-separated string
  shape the CLI's `coerceForCLI` used to handle. Empty string → empty
  list; trims each split part.
- **Cross-layer "ignored input" warning noise suppressed** — When a
  user-supplied `--input` key IS declared on SOME layer in the compose
  chain, `resolveInputs`'s `"input X is not declared by preset Y
  (ignored)"` warning is suppressed for the layers that don't own it.
  Single-layer use is unaffected; genuine typos (key declared nowhere)
  still warn.

### Out of scope (still deferred)

- `base`, `nanite-plugin` and `sysop-ui` composing `go-baseline`; until then their
  `.golangci.yml` / `lefthook.yml` copies are still synced by hand.
- `go-app`, which composes `go-baseline` once the service-layer standard is ratified.
- Folio's own `.github/workflows/ci.yml` still uses `golangci-lint version: latest`
  and `govulncheck@latest` (its Go pin now comes from `go.mod`), and does not set
  `FOLIO_REQUIRE_TOOLS=1`.
- `folio sync` + diff UI.
- Federated git-URL preset sources (`source: git`).
- Multi-version bundled presets (`presets/<id>@<version>/`).
- Per-file `from_preset_chain` recording in `.folio.yaml` (a sync
  prerequisite).
- Cycle/depth error formatting beyond preset ids (no file:line yet).
- Topological dependency resolution within a single layer's `computed:`
  block (alphabetic ordering still serves).
- Diamond compose detection + warning when the same composed id appears
  under multiple parents with conflicting `vars:` blocks.

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
