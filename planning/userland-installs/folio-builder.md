# Folio Builder Plan

## Objective

Extend Folio from project scaffolding into the portfolio suite installer for
local userland installs.

Folio should not become a process supervisor itself. It should generate and
manage the install layout, launchd plists, Tether catalog, and doctor checks.
launchd supervises long-running processes.

## Commands

Add:

```sh
folio suite plan
folio suite install
folio suite doctor
folio suite start
folio suite stop
folio suite uninstall
```

Initial command behavior:

- `plan`: resolves all paths, ports, binary sources, launch agents, and rendered
  files. Prints a deterministic summary and can emit JSON.
- `install`: creates directories, copies binaries, renders env/config files,
  renders Tether catalog, and optionally loads launch agents.
- `doctor`: checks installed paths, process status, HTTP health, MCP proxy
  discovery, and repo-local DB guardrails.
- `start`/`stop`: shells out to `launchctl bootstrap/bootout` or
  `launchctl kickstart`.
- `uninstall`: removes launch agents and installed binaries/config. User data is
  retained unless `--purge-data` is passed.

## Install Profile Schema

Create a suite config type that can later be rendered from YAML:

```yaml
suite_version: 1
root: "~/Library/Application Support/Hollis Labs"
logs_root: "~/Library/Logs/Hollis Labs"
bin_root: "${root}/bin"

apps:
  nanite:
    enabled: true
    binary: "${bin_root}/nanite"
    source_binary: "~/dev/hollis-labs/apps/nanite/nanite"
    data_dir: "${root}/nanite"
    db_path: "${root}/nanite/nanite.db"
    addr: "127.0.0.1:8090"
  torque:
    enabled: true
    binary: "${bin_root}/torque"
    source_binary: "~/dev/hollis-labs/apps/torque/torque"
    data_dir: "${root}/torque/data"
    db_path: "${root}/torque/torque.db"
    queue_db_path: "${root}/torque/queue.db"
    addr: "127.0.0.1:8990"
  tesseract:
    enabled: true
    binary: "${bin_root}/contextd"
    source_binary: "~/dev/hollis-labs/apps/tesseract/contextd"
    root_dir: "${root}/tesseract"
    db_path: "${root}/tesseract/data/index/context.db"
    queue_db_path: "${root}/tesseract/data/queue.db"
    addr: "127.0.0.1:8080"
  tether:
    enabled: true
    binary: "${bin_root}/mux"
    source_binary: "~/dev/hollis-labs/apps/tether/bin/mux"
    catalog_root: "${root}/tether/catalog"
    state_db: "${root}/tether/tether.db"
    socket: "unix:${root}/tether/run/muxd.sock"
```

## Rendered Files

Folio should render:

- `config/suite.yaml`
- `config/env/*.env`
- `tether/catalog/global.yaml`
- `tether/catalog/mcp-servers/*.yaml`
- `tether/catalog/projects/*.yaml` as needed for installed launch profiles
- launchd plists under `~/Library/LaunchAgents`
- optional MCP client snippets for Claude Desktop / Codex / Cursor

## Doctor Checks

Implement checks as typed results, not only printed text:

```go
type CheckResult struct {
    ID       string
    App      string
    Status   string // pass|warn|fail
    Message  string
    Evidence map[string]string
}
```

Required checks:

- Binary exists and is executable.
- Config/env files exist and do not contain repo-local DB paths.
- SQLite DB parent directories exist.
- launchd plist exists and matches rendered content digest.
- Service process is loaded/running when expected.
- HTTP health endpoint responds where available.
- Tether MCP proxy can list upstream tools.
- No installed MCP catalog entry points at a dev repo DB.

## Implementation Phases

1. Add suite config structs, path expansion, and `suite plan`.
2. Add renderers for env files, Tether catalog, and launchd plists.
3. Add `suite install` with dry-run parity against `suite plan`.
4. Add `suite doctor` with path and binary checks.
5. Add HTTP and MCP checks.
6. Add start/stop/uninstall.

## Open Questions

- Whether Folio should copy binaries from local build outputs or build them from
  source in the first slice.
- Whether friend-test packages should be zip-only or simple DMG.
- Whether secrets are stored in env files for alpha testing or Keychain from day
  one.

