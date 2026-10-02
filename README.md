# folio

Preset-driven project scaffolding for the hollis-labs portfolio.

`folio new <preset> <target-dir>` renders a typed preset (YAML manifest +
text/template tree) into a new project directory, with a `.folio.yaml`
breadcrumb recording exactly what was generated so the project can be
re-rendered later (and, in a future release, synced when the preset evolves).

## Status

v0.3 — composition slice. Folio ships several bundled presets (Go modules,
libraries and service apps, a Nanite plugin, a design-kit dashboard app) plus one
compose-only layer, `go-baseline`, that is not meant to be rendered on its own.
Ask folio rather than this README which presets exist:

```sh
folio preset list           # bundled and user-dir presets, with version and description
folio preset show go-lib    # one preset's metadata, composes: entries and inputs
```

Add `--json` to either for machine-readable output. A preset marked `[layer]`
(`layer_only: true` in its `preset.yaml`) exists to be composed by other presets.

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
| `folio preset list [--json]` | List every bundled and user-dir preset (`~/.folio/presets/local/<id>@<version>/`) with its version, source and description. |
| `folio preset show <id> [--json]` | Show one preset's metadata, its own declared `composes:` entries (not a resolved chain) and its inputs. Exits 1 if the id is unknown. |
| `folio inspect <dir> [--json]` | Read-only drift report: re-render the preset recorded in `<dir>/.folio.yaml` with the recorded inputs and compare it, per file, with the project (`unchanged`, `preset_updated`, `locally_modified`, `conflict`, `added_upstream`, `removed_upstream`, `missing_locally`). Never writes; exits 0 whenever it can report (1 if `<dir>` has no `.folio.yaml` or the preset no longer accepts the recorded inputs). Replays the generation clock and folio version from the manifest, so a `LICENSE` reading `{{ .now.Year }}` does not drift when the year turns; a file that renders random bytes (`uuid`) is marked `*` and compared for local edits only. |
| `folio sync` | Reserved — prints "not yet implemented in v0" and exits 1. A real sync needs the original rendered bytes for a three-way merge; `.folio.yaml` stores only digests. |

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
make pins       # drift-check every version pin against platform-pins.yaml
make build      # build the folio binary
make install    # go install ./cmd/folio
```

CI runs `go test -race`, `go vet`, `golangci-lint`, `govulncheck` and the pins
drift check on push and pull requests to `main`.

`platform-pins.yaml` is the single canonical source for the portfolio's version
pins (Go floor, `actions/checkout`, `actions/setup-go`, golangci-lint action and
binary, govulncheck). It does not drive rendering yet: the presets still carry
the literals, and `scripts/check-platform-pins-drift.sh` fails when any of them —
preset `go_version` defaults, rendered workflow templates, folio's own CI, the
conformance script's floor, or (with `--checklist FILE`) the ratified lib
checklist — disagrees with the manifest. `--self-test` breaks one value at a time
in a scratch copy and requires exactly that check to fail.

## Dashboard apps and the former sysop-ui preset

```sh
folio new app-dashboard /tmp/my-dashboard \
  --input project_name=my_dashboard \
  --input github_owner=hollis-labs \
  --non-interactive
```

`app-dashboard` renders the design-kit foundation plus kit-dashboard, with
published npm packages (`design_kit_version`, default `0.3.0`). The generated
frontend uses the real `AppShell`, same-origin API client/context, the kit theme
and shell reset; its CI runs frontend typecheck, Biome/design-token lint and
build before the Go checks. The manifest-driven admin shell is deferred.

`sysop-ui` remains a deprecated bundled alias: `new`, `plan`, `preset show` and
`inspect` resolve it to `app-dashboard`. Discovery lists the canonical preset
once, and new breadcrumbs record `app-dashboard`. Existing breadcrumbs with
`sysop-ui` remain readable by `inspect`, which reports drift against the current
dashboard template without updating the app or its breadcrumb. The old
`sysop_ui_version` input is ignored with an undeclared-input warning; it cannot
pin the frozen kit anymore. Choose `design_kit_version` for new scaffolds.
There are no bundled `composes:` references to the old id.

For an existing app, render the new preset into a temporary directory and port
its imports, dependencies and CI intentionally. `folio sync` remains reserved;
this rename performs no automatic migration of existing apps.

The structural preset tests run offline. To verify a fresh generated consumer
against the published packages (requires npm/network), run:

```sh
FOLIO_FRONTEND_E2E=1 go test . -run TestIntegration_AppDashboardPreset_Frontend -v
```

This installs the frontend dependencies, typechecks, runs Biome and the design
ESLint gate, builds the embedded UI and compiles the generated Go application.
All generated output and tool homes are temporary.

## Chat apps

```sh
folio new chat-app /tmp/my-chat --input project_name=my_chat --non-interactive
```

The preset follows ops-chat's AppShell/sidebar and transcript/composer layout,
using published design-kit and kit-chat with explicit Tailwind source registration.
Its Go server provides a same-origin local echo endpoint so sending a message
works immediately; replace the endpoint/client with your application's transport.
Draft and transcript are app-owned, ephemeral state. No Nanite dependency,
credentials, persistence or optional markdown renderer is included.

`design_kit_version` is the single design-kit version input, defaulting to 0.3.0.
The generated `^0.3.0` ranges select the published 0.3.x packages, including the
kit-chat control-font and composer fixes plus the 6px small-control radius.
Existing generated apps keep their dependencies until intentionally updated.

The normal preset/discovery tests stay offline. To install published packages,
typecheck, lint, build the UI and compile the generated Go server:

```sh
FOLIO_FRONTEND_E2E=1 go test . -run TestIntegration_ChatAppPreset_Frontend -v
```

For computed-style and interaction verification against the actual embedded app,
install Playwright in a scratch tool directory and provide its module/browser:

```sh
npm install --prefix /path/to/browser-tools playwright
/path/to/browser-tools/node_modules/.bin/playwright install chromium
FOLIO_FRONTEND_E2E=1 FOLIO_BROWSER_E2E=1 \
  FOLIO_PLAYWRIGHT_MODULE=/path/to/browser-tools/node_modules/playwright \
  go test . -run TestIntegration_ChatAppPreset_Frontend -v
```

`FOLIO_CHROMIUM_EXECUTABLE` optionally selects an existing Chromium executable.
`FOLIO_SCREENSHOT_DIR` optionally saves desktop/narrow screenshots. The browser
check verifies user-row alignment, token radius, bubble background/width,
composer border/radius, scroll layout, emitted chat utilities, message send and
reset, one Send control with a 6px radius, and bubble/composer typography
matching the 13px control token at desktop and narrow widths, with no page errors. These fixes ship in the
registry 0.3.0 packages. The Go app runs on a temporary loopback port and tool
homes/output are temporary.

## License

MIT — see [LICENSE](./LICENSE).
