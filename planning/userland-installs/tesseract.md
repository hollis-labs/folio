# Tesseract Install Readiness Plan

## Current Shape

Tesseract is already close to an installed service shape:

- `tesseract serve` exists.
- `tesseract mcp` exists.
- Default root resolves to `~/.tesseract`.
- Primary DB path is `data/index/context.db` under the root.
- Queue DB is under `data/queue.db`.
- HTTP API and embedded frontend are served together.

Current risks:

- CLI help/error output is rough compared with the other apps.
- Naming remains mixed in places (`contextd`, Conduit/Vanta env names and
  descriptions). **Resolved 2026-08-26:** the binary is `tesseract`, the
  `CONTEXTD_ROOT` env var is retired, and the MCP tool surface is
  `tesseract_*` / `context_*` / `memory_*` / `knowledge_*`.
- Token/bootstrap story needs to be installer-friendly.
- Folio needs a stable health endpoint and stable config path.

## Target Installed Contract

The installed binary is `tesseract` (renamed from `contextd` in v0.9.0, 2026-08-26).

```sh
tesseract serve \
  --config "$ROOT/config/tesseract.yaml" \
  --root "$ROOT/tesseract" \
  --db "$ROOT/tesseract/data/index/context.db" \
  --queue-db "$ROOT/tesseract/data/queue.db" \
  --addr "127.0.0.1:8080" \
  --static-token "$TOKEN"

tesseract mcp \
  --config "$ROOT/config/tesseract.yaml" \
  --root "$ROOT/tesseract" \
  --static-token "$TOKEN"
```

## Required Work

1. Modernize CLI help.
   - `tesseract --help`
   - `tesseract serve --help`
   - `tesseract mcp --help`
   - consistent exit codes
2. Add or confirm explicit flags:
   - `--config`
   - `--root`
   - `--db`
   - `--queue-db`
   - `--addr`
   - token/auth flags
3. Standardize names.
   - Prefer `TESSERACT_*` env vars.
   - Keep `CONDUIT_*` compatibility for one release if needed.
   - Remove Vanta wording from active tool descriptions/docs where practical.
4. Make serve and MCP use the same root/config resolver.
5. Add installed path guardrail tests.
   - No default should write into repo `data/index/context.db` in installed
     mode.
6. Add stable health endpoint for Folio doctor.
   - `GET /v1/health/readiness` or equivalent.
7. Document token handling.
   - Alpha can use generated static token in env/config.
   - Later move to Keychain or managed auth.

## Tether MCP Catalog Entry

Installed Tether should render:

```yaml
id: tesseract
transport: stdio
command: "${BIN_ROOT}/tesseract"
args: ["mcp", "--config", "${ROOT}/config/tesseract.yaml"]
env:
  TESSERACT_ROOT: "${ROOT}/tesseract"
tags: [memory, knowledge, context, tesseract]
enabled: true
```

If token cannot live in config yet, render it through env:

```yaml
env:
  TESSERACT_ROOT: "${ROOT}/tesseract"
  TESSERACT_TOKEN: "${TESSERACT_TOKEN}"
```

## Acceptance Criteria

- `tesseract serve` and `tesseract mcp` use the same installed store.
- Help text is clean enough for friend testers.
- Folio doctor can verify HTTP readiness and MCP tool discovery.
- Active install docs and configs use Tesseract naming.
- Compatibility env vars are documented if retained.

