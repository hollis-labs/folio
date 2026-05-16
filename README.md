# folio

Preset-driven project scaffolding for the hollis-labs portfolio.

`folio new <preset> <target-dir>` renders a typed preset (YAML manifest +
text/template tree) into a new project directory, with a `.folio.yaml`
breadcrumb recording exactly what was generated so the project can be
re-rendered later (and, in a future release, synced when the preset evolves).

## Status

v0.2 — composition slice. Bundled presets:

| Preset | Scaffolds |
|---|---|
| `base` | A minimal Go module |
| `go-package` | An `internal/<pkg>/` library layered on `base` |
| `nanite-plugin` | A Nanite subprocess plugin (Go binary + `plugin.yaml` + optional UI) |
| `sysop-ui` | A Sysop UI app — a `@hollis-labs/sysop-ui` React frontend served by a Go binary via `go-webui` |

`folio new` / `folio plan` / `folio preset validate` all understand
`composes:`. Sync, post-render Hadron hooks, federated git-URL preset
sources, and MCP / HTTP surfaces are deliberately deferred; see
[`CHANGELOG.md`](./CHANGELOG.md) for the full deferred list.

## Quickstart

```sh
go install github.com/hollis-labs/folio/cmd/folio@latest

folio new base /tmp/folio-smoke \
  --input project_name=smoke_test \
  --input github_owner=chrispian \
  --input description="folio v0 smoke" \
  --non-interactive

cd /tmp/folio-smoke
go vet ./...
go test ./...
```

Drop `--non-interactive` to be prompted for any inputs you didn't supply on
the command line. Drop the `--input` flags entirely and folio will prompt
for everything required.

## Commands

| Command | What it does |
|---|---|
| `folio new <preset> <dir>` | Render a preset into `<dir>`. Prompts for missing inputs unless `--non-interactive`. |
| `folio plan <preset> <dir>` | Dry-run — print resolved inputs + computed values + planned file list. No writes. |
| `folio preset validate <preset-dir>` | Run the v0 validation rule set against `<preset-dir>/preset.yaml`. |
| `folio sync` / `folio inspect` / `folio preset list` / `folio preset show` | Reserved — print "not yet implemented in v0" and exit 1. |

## Composing presets

A preset can layer on top of other presets by declaring `composes:`. Each
entry names a composed preset by id, pins its version via a semver
constraint, and points at its directory. The composing preset's files then
overlay the composed preset's tree in declared order — later layers
overwrite earlier ones on the same relative path.

```yaml
# presets/go-package/preset.yaml
folio_version: "0.1"
id: go-package
version: 1.0.0

composes:
  - id: base
    version: ">=0.1,<1.0"
    source: local
    path: ../base

inputs:
  - name: package_name
    type: string
    required: true
    pattern: "^[a-z][a-z0-9_]*$"
```

The bundled `go-package` preset does exactly this — it layers on `base`
to add `internal/<package_name>/` library code and overwrites the base
`README.md` + `cmd/<project>/main.go` with a library-first stub.

```sh
folio new go-package /tmp/folio-compose \
  --input project_name=smoke_compose \
  --input github_owner=chrispian \
  --input package_name=greeter \
  --non-interactive

cd /tmp/folio-compose
go vet ./...
go test ./...
```

The resulting `.folio.yaml` records both layers in apply order under
`presets:`, and each file's `preset:` field names the layer that produced
it (the last writer on overwrites).

**Constraint syntax.** npm-style operators are supported: `>=`, `<=`, `>`,
`<`, exact, `*`, tilde (`~1.2.3` → `>=1.2.3,<1.3.0`), caret (`^1.2.3` →
`>=1.2.3,<2.0.0`), AND-via-comma (`>=1.0,<2.0`), OR-via-pipe (`^1.0 || ^2.0`).
Partial versions canonicalize (`1.0` → `1.0.0`).

**Var scoping.** A composed preset inherits the caller's `.inputs.*` by
default. To override a per-key value for the composed layer, declare it in
the entry's `vars:` block — the value is a Go template rendered against
the *caller's* render context (so `{{.inputs.foo}}` inside a `vars:` value
resolves to the caller's input named `foo`). Templates inside the composed
preset's own files render against the layer's own resolved inputs +
computed values, with prior layers' values visible.

**Limits.** v0.2 supports `source: local` only (git URL sources land in
v1.1+). Compose DAG depth is capped at 8; direct or transitive cycles
produce a path-bearing error.

## Inputs resolution

For each input declared by a preset, folio resolves a value in this order
(higher beats lower):

1. CLI flag — `--input key=value` (repeatable).
2. `--inputs-file <path>` — YAML or JSON file of `key: value` pairs.
3. Environment variable — `FOLIO_INPUT_<UPPER>` (hyphens → underscores).
4. Preset-declared default.
5. Interactive prompt (charmbracelet/huh), unless `--non-interactive`.
6. Error — exit 2 with the list of missing required inputs.

## Template helpers

folio templates use Go `text/template` with a curated funcmap. Helper names
match Hadron where shared (`basename`, `dirname`, `ext`, `json`, `default`,
`ternary`, `upper`, `lower`, `trim`, `replace`, `split`, `join`) so
templates portable between the two tools evaluate identically.

folio additions cover case conversion (`kebabCase`, `snakeCase`,
`camelCase`, `pascalCase`), quoting (`quote`, `squote`, `shellQuote`,
`jsonEscape`), encoding (`jsonIndent`, `toYAML`, `b64encode`, `b64decode`),
date/time (`date`, `dateISO`), lists/dicts (`list`, `first`, `last`,
`slice`, `dict`, `get`, `hasKey`), random (`uuid`, `randAlphaNum`), and the
folio-specific `licenseHeader`, `gomodPath`, `gitUser`, `spdxId`.

**Excluded from the funcmap**: `env`, `readFile`, `getHostByName`, `httpGet`,
`exec`/`shell`. These exist in Hadron's funcmap but are deliberately not
in folio's — folio's threat model includes third-party presets via git URL
(v1.1+), and template-time access to environment variables / the
filesystem is a secret-leak and reproducibility risk under that model.

## Development

```sh
make test       # go test -race ./...
make vet        # go vet ./...
make lint       # golangci-lint run
make vuln       # govulncheck ./...
make build      # build the folio binary
make install    # go install ./cmd/folio
```

CI runs `go test -race`, `go vet`, `golangci-lint`, and `govulncheck` on
push and pull requests to `main`.

## License

MIT — see [LICENSE](./LICENSE).
