# AGENTS.md — folio

## What is this, and why

**folio** is a preset-driven project scaffolding tool for the hollis-labs
portfolio. It is a single Go CLI binary (`github.com/hollis-labs/folio`).

`folio new <preset> <target-dir>` renders a typed preset — a YAML manifest
plus a `text/template` file tree — into a new project directory. Every
generated project carries a `.folio.yaml` breadcrumb recording exactly what
was rendered (preset identity, resolved inputs, computed values, per-file
SHA-256 digests) so it can be re-rendered later, and in a future release
synced when the preset evolves.

It exists so new portfolio projects start from a consistent, tested layout
(Go module scaffold, CI workflow, license, conventional directory shape)
instead of being hand-assembled and drifting apart. folio deliberately
shares helper-function names with Hadron's template funcmap where the two
overlap, so templates are portable between the tools.

Status: **v0.2 (unreleased)** — the "composition slice". The binary embeds
three bundled presets: `base` (minimal Go module), `go-package` (an
`internal/<pkg>/` library layered on `base`), and `nanite-plugin`. `folio
new` / `folio plan` / `folio preset validate` all understand `composes:`.

## Where to start

- **`README.md`** — quickstart, command table, composition model, inputs
  resolution order, template-helper catalog. Read this first.
- **`CHANGELOG.md`** — release-by-release detail; the v0.2 entry documents
  the composition runtime and the explicit "Out of scope / still deferred"
  list. Authoritative for what is and is not implemented.
- **`folio.go`** — package root; embeds `presets/` via `embed.FS` and holds
  the `Version` constant.
- **`cmd/folio/`** — CLI entry point (`main.go`) and the cobra command
  layer under `cmd/folio/internal/cli/`.
- **`service/`** — the canonical Go API (`service.Service`). The CLI imports
  it directly; future MCP / HTTP surfaces are expected to wrap the same
  service.
- **`internal/`** — `compose`, `manifest`, `preset`, `render` packages: the
  compose DAG walker, `.folio.yaml` manifest, preset parser/validator, and
  the `text/template` render engine.
- **`presets/`** — the three bundled presets, each a `preset.yaml` plus a
  `files/` template tree.
- **`planning/`** — design notes for in-flight work (untracked in git);
  `planning/userland-installs/folio-builder.md` describes the proposed
  `folio suite` installer expansion.

## Key domain concepts

- **Preset** — a `preset.yaml` manifest (typed inputs, `computed:` block,
  `files:` config, optional `composes:`) plus a `files/` template tree.
  Preset id = the first directory level under `presets/`.
- **Typed inputs** — `string`, `bool`, `number`, `enum`, `list[string]`,
  with pattern / min / max validation hooks.
- **Inputs resolution order** — CLI `--input` > `--inputs-file` >
  `FOLIO_INPUT_<UPPER>` env var > preset default > interactive prompt
  (charmbracelet/huh) > error. `--non-interactive` skips the prompt step.
- **Composition (`composes:`)** — a preset can layer on top of other
  presets, each pinned by a semver constraint. Layers render in topological
  order (deepest leaves first, root last); same-path writes overwrite —
  last writer wins. DAG depth capped at 8; cycles produce a path-bearing
  error. v0.2 supports `source: local` only.
- **Var scoping** — a composed preset inherits the caller's `.inputs.*` by
  default; the composing entry's `vars:` block can per-key override, with
  `vars:` templates evaluated against the *caller's* render context.
- **`.folio.yaml` manifest** — the breadcrumb written into every generated
  project; records all contributing layers in apply order and a per-file
  SHA-256 digest (computed after LF normalisation, so digests are
  platform-stable).
- **Template funcmap** — Go `text/template` with a curated funcmap. Names
  match Hadron where shared. `env`, `readFile`, `getHostByName`, `httpGet`,
  `exec`/`shell` are **deliberately excluded** — folio's threat model
  includes third-party git-URL presets (v1.1+), so template-time
  environment / filesystem access is a secret-leak and reproducibility
  risk.

## Common operations

Render a project from the minimal `base` preset:

```sh
folio new base /tmp/folio-smoke \
  --input project_name=smoke_test \
  --input github_owner=chrispian \
  --input description="folio v0 smoke" \
  --non-interactive
```

Render a composing preset (`go-package` layers on `base`):

```sh
folio new go-package /tmp/folio-compose \
  --input project_name=smoke_compose \
  --input github_owner=chrispian \
  --input package_name=greeter \
  --non-interactive
```

Dry-run a render (resolved inputs + computed values + planned file list, no
writes):

```sh
folio plan base /tmp/folio-smoke --input project_name=smoke_test --non-interactive
```

Validate a preset against the v0 rule set:

```sh
folio preset validate presets/base
```

Reserved / not-yet-implemented: `folio sync`, `folio inspect`,
`folio preset list`, `folio preset show` — these print a not-implemented
message and exit non-zero.

Development (see `Makefile` — matches CI):

```sh
make test        # go test ./...
make test-race   # go test -race ./...
make vet         # go vet ./...
make lint        # golangci-lint run
make vuln        # govulncheck ./...
make build       # build the folio binary into the repo root
make install     # go install ./cmd/folio
make all         # tidy + vet + lint + test-race (full pre-commit pipeline)
```

## Where to look for more

- **CI** — `.github/workflows/ci.yml`: three jobs (`go vet` + `go test
  -race`, `golangci-lint`, `govulncheck`) on push and PR to `main`,
  Go 1.25.
- **Lint config** — `.golangci.yml`.
- **Integration tests** — top-level `integration_*_test.go` files exercise
  the bundled presets end-to-end (render → `go vet` / `go test` the output).
- **Roadmap signal** — the `CHANGELOG.md` "Out of scope / still deferred"
  list (v0.2) and the v0.1 "Deferred to v0.x / v1.x" list are the live
  roadmap surface. `planning/userland-installs/folio-builder.md` proposes
  the `folio suite` installer direction (not yet implemented).
- **License** — MIT (`LICENSE`).
