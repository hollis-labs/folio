# folio

folio renders a typed preset — a `preset.yaml` manifest plus a `text/template`
file tree — into a new project directory, leaving a `.folio.yaml` breadcrumb
that records what was rendered. It scaffolds once and walks away: it does not
build, install, run or update the projects it generates, and `folio sync` and
`folio inspect` are reserved stubs that error rather than doing anything.

## Start Here

- `README.md` — quickstart, command table, inputs resolution order and the
  template-helper catalog.
- `CHANGELOG.md` — `### Out of scope (still deferred)` is the live roadmap
  surface and is authoritative for what is not implemented.
- `service/service.go` is the canonical API. `cmd/folio/internal/cli/` is a
  thin cobra wrapper over it, and future MCP or HTTP surfaces wrap the same
  methods rather than reimplementing them.
- `folio.go` embeds `presets/` and holds the `Version` stamped into every
  generated `.folio.yaml`.
- `internal/compose/graph.go` owns compose order, cycle detection and the
  depth cap.
- `internal/render/funcmap.go` owns the template funcmap.
- `internal/manifest/digest.go` owns the breadcrumb's per-file digests.
- `presets/` holds the bundled presets; the first directory level under it is
  the preset id.

## Commands

```bash
go test ./internal/... ./service ./cmd/...
make test
make all
```

`make all` is tidy + vet + lint + test-race and matches CI, which gates on
`go vet`, `go test -race`, `golangci-lint` and `govulncheck`. The top-level
`integration_*_test.go` files render each bundled preset into a temp directory
and compile the result, so they are what catches a broken preset — run the full
`make test` when you touch `presets/`.

## Boundaries

The funcmap omits `env`, `readFile`, `getHostByName`, `httpGet` and
`exec`/`shell` deliberately: the threat model includes third-party presets
fetched by git URL, so template-time environment, filesystem and network access
is a secret-leak and reproducibility risk. `TestRenderString_ForbiddenHelpers`
guards it. Restoring one is a security decision, not a convenience.

`.folio.yaml` digests are hashed after LF normalisation so they are stable
across platforms. Every already-generated project carries digests computed that
way, so changing what goes into the hash invalidates all of them.

Rendering writes outside this repo, into a caller-supplied target directory,
and same-path writes across compose layers overwrite — last writer wins. Render
into temp directories; never point `folio new` at a tree you care about.
