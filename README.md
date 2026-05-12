# folio

Preset-driven project scaffolding for the hollis-labs portfolio.

`folio new <preset> <target-dir>` renders a typed preset (YAML manifest +
text/template tree) into a new project directory, with a `.folio.yaml`
breadcrumb recording exactly what was generated so the project can be
re-rendered later (and, in a future release, synced when the preset evolves).

## Status

v0.1.0 — vertical slice. Single bundled `base` preset, `folio new` /
`folio plan` / `folio preset validate`. Composition (`composes:`), sync,
post-render Hadron hooks, and MCP / HTTP surfaces are deliberately deferred;
see [`CHANGELOG.md`](./CHANGELOG.md) for the full deferred list.

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
