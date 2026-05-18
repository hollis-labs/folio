# Torque Install Readiness Plan

## Current Shape

Torque already has most of the required runtime knobs, but they are primarily
environment-variable driven.

Useful existing env vars:

- `TORQUE_DB_PATH`
- `TORQUE_DATA_DIR`
- `TORQUE_QUEUE_DB_PATH`
- `TORQUE_HTTP_PORT`
- `TORQUE_PROFILES_PATH`
- `TORQUE_MUX_COMMAND`
- `TORQUE_MUX_ARGS`
- `TORQUE_GUI_DIR`
- `TORQUE_ADMIN_TOKEN`

Existing commands:

- `torque serve --addr <addr>`
- `torque mcp`

Current risk:

- Defaults still include repo-local-looking paths such as `torque.db`,
  `.torque`, and `queue.db`.
- The MCP command has no visible flags for DB/data paths, so installed use
  relies on env wiring.

## Target Installed Contract

Add explicit flags while preserving env support:

```sh
torque serve \
  --config "$ROOT/config/torque.yaml" \
  --db "$ROOT/torque/torque.db" \
  --data-dir "$ROOT/torque/data" \
  --queue-db "$ROOT/torque/queue.db" \
  --profiles "$ROOT/torque/profiles.yaml" \
  --addr "127.0.0.1:8990"

torque mcp \
  --config "$ROOT/config/torque.yaml" \
  --db "$ROOT/torque/torque.db" \
  --data-dir "$ROOT/torque/data" \
  --queue-db "$ROOT/torque/queue.db"
```

Flag precedence:

1. CLI flag
2. env var
3. config file
4. dev default

## Required Work

1. Add config file support or formalize existing env-only config.
   - Minimal first pass can load YAML and map to existing config struct.
2. Add CLI flags for DB/data/queue/profiles on `serve` and `mcp`.
3. Ensure `mcp` and `serve` share exactly the same config loading path.
4. Add installed default resolver.
   - In release/install mode, DB defaults to app-support path.
   - Dev mode may keep repo-local defaults.
5. Update scheduler/worktree defaults.
   - Workspaces and run logs should live under `TORQUE_DATA_DIR`.
   - Avoid implicit repo `.torque` writes in installed mode.
6. Add a health endpoint suitable for Folio doctor if current API health is not
   stable.
7. Add path guardrail tests.
   - Installed config cannot silently fall back to `./torque.db`.
   - MCP config and serve config resolve identical DB paths.

## Tether MCP Catalog Entry

Installed Tether should render:

```yaml
id: torque
transport: stdio
command: "${BIN_ROOT}/torque"
args: ["mcp", "--config", "${ROOT}/config/torque.yaml"]
env:
  TORQUE_DB_PATH: "${ROOT}/torque/torque.db"
  TORQUE_DATA_DIR: "${ROOT}/torque/data"
  TORQUE_QUEUE_DB_PATH: "${ROOT}/torque/queue.db"
  TORQUE_PROFILES_PATH: "${ROOT}/torque/profiles.yaml"
tags: [tasks, sprints, planning, orchestration, torque]
enabled: true
```

Keep env fields until the new flags are fully shipped.

## Acceptance Criteria

- `torque serve` and `torque mcp` can both be launched from outside the repo and
  use installed app-support state.
- No installed launch path writes `clockwork.db`, `torque.db`, `.torque`, or
  `queue.db` into the current working directory.
- Tether MCP proxy can list Torque tools from the installed binary.
- HTTP API responds on loopback.
- Tests cover env/flag/config precedence.

