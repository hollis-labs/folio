# Nanite Install Readiness Plan

## Current Shape

Nanite is the reference shape for this effort.

Known existing install-friendly behavior:

- `nanite serve -db <path> -port <port>`
- `nanite mcp -db <path> [-session <id>]`
- HTTP server is already a primary surface.
- MCP server is already available over stdio.
- Plugins and MCP servers are already persisted/configured through Nanite
  systems.

Current risk:

- The default DB path is `./nanite.db`, which is correct for dev but unsafe for
  installed release mode.

## Target Installed Contract

Add or standardize:

```sh
nanite serve \
  --config "$ROOT/config/nanite.yaml" \
  --data-dir "$ROOT/nanite" \
  --db "$ROOT/nanite/nanite.db" \
  --addr "127.0.0.1:8090"

nanite mcp \
  --config "$ROOT/config/nanite.yaml" \
  --data-dir "$ROOT/nanite" \
  --db "$ROOT/nanite/nanite.db"
```

Keep `-db`/`-port` compatibility, but prefer long flags in docs and launchd.

## Required Work

1. Add `--data-dir` and `--addr` flags.
   - `--addr` should supersede `--port`.
   - `--port` remains as a compatibility alias.
2. Add `--config`.
   - Missing config should be acceptable.
   - Config should be able to declare DB path, data dir, auth, plugins dir, and
     MCP upstreams.
3. Add release-mode default path resolution.
   - If `NANITE_RELEASE=1` or an installed config is used, never default to
     `./nanite.db`.
   - Prefer `~/Library/Application Support/Hollis Labs/nanite/nanite.db`.
4. Add startup logging of resolved paths.
   - DB path
   - data dir
   - plugins dir
   - artifacts dir
   - listen address
5. Add guardrail test.
   - Release-mode serve without explicit DB must not resolve inside cwd.
6. Add health endpoint verification if not already stable.
   - `GET /api/health` is enough for Folio doctor.

## Tether MCP Catalog Entry

Installed Tether should render:

```yaml
id: nanite
transport: stdio
command: "${BIN_ROOT}/nanite"
args: ["mcp", "--db", "${ROOT}/nanite/nanite.db"]
tags: [chat, agents, plugins, nanite]
enabled: true
```

If Nanite gains `--config`, prefer:

```yaml
args: ["mcp", "--config", "${ROOT}/config/nanite.yaml"]
```

## Acceptance Criteria

- `nanite serve` installed via launchd uses only app-support paths.
- `nanite mcp` launched by Tether sees the same canonical DB as HTTP serve.
- Folio doctor can verify Nanite DB path is not repo-local.
- Existing dev commands still work.
- Tests cover default path behavior and flag precedence.

