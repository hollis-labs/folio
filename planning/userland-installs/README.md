# Userland Installs Plan

Date: 2026-05-14

## Goal

Create a local, friend-testable install path for the Hollis Labs app suite where
installed apps use canonical user data, never repo-local development databases,
and can communicate through MCP at minimum with HTTP APIs available where the
app already supports them.

Initial scope:

- Nanite
- Torque
- Tesseract
- Tether

Nil and Hadron are already Wails apps and can join this installer contract
after the first CLI/service-oriented slice is stable.

## Decision

Put the builder/installer in `tools/folio`.

Rationale:

- Folio already owns portfolio-level generation/scaffolding.
- Install layout generation is a close sibling to project scaffolding: both are
  deterministic file trees from typed inputs.
- Keeping the suite installer outside any one app prevents Tether, Nanite, or
  Torque from becoming responsible for bootstrapping the entire portfolio.
- Folio can grow separate surfaces over the same service layer later: CLI first,
  then MCP/HTTP if useful.

## Installed Runtime Contract

Every app should support the same release-mode contract:

1. All writable state is rooted in an explicit app data directory.
2. Every SQLite path can be set explicitly by flag or env.
3. Every HTTP service can bind an explicit loopback address.
4. Every MCP server can run over stdio using the same canonical database as its
   HTTP service.
5. Installed defaults never point at `./*.db`, repo-local `.appname/`, or a dev
   checkout.
6. A `doctor` command can prove the process is using installed paths.

Preferred per-app flags:

```text
--config <path>
--data-dir <path>
--db <path>
--addr <host:port>
--token <token> or env-backed token
```

Env vars remain supported for launchd and MCP proxy use, but flags should be
the clear documented interface.

## Canonical User Layout

Use macOS Application Support for installed data:

```text
~/Library/Application Support/Hollis Labs/
  bin/
    nanite
    torque
    contextd
    mux
    folio
  config/
    suite.yaml
    env/
      nanite.env
      torque.env
      tesseract.env
      tether.env
  nanite/
    nanite.db
    plugins/
    artifacts/
  torque/
    torque.db
    queue.db
    data/
    profiles.yaml
  tesseract/
    config.yaml
    data/
      index/context.db
      queue.db
  tether/
    catalog/
    tether.db
    run/
      muxd.sock
      muxd.pid
```

Logs:

```text
~/Library/Logs/Hollis Labs/<app>/
```

Launch agents:

```text
~/Library/LaunchAgents/com.hollislabs.nanite.plist
~/Library/LaunchAgents/com.hollislabs.torque.plist
~/Library/LaunchAgents/com.hollislabs.tesseract.plist
~/Library/LaunchAgents/com.hollislabs.tether.plist
```

## Service Topology

Tether is the installed control plane and MCP gateway.

Installed MCP client config should point at one command:

```sh
"$APP_SUPPORT/bin/mux" --catalog "$APP_SUPPORT/tether/catalog" mcp --proxy
```

Tether's catalog owns upstream app entries:

- `nanite` via stdio MCP
- `torque` via stdio MCP
- `tesseract` via stdio MCP
- later: `hadron`, `nil`

HTTP should be loopback-only for the first userland release:

| App | Default installed bind |
|---|---|
| Nanite | `127.0.0.1:8090` |
| Tesseract | `127.0.0.1:8080` |
| Torque | `127.0.0.1:8990` |
| Tether daemon | Unix domain socket first; optional loopback later |

## Folio Installer Shape

Add a new folio command group:

```sh
folio suite plan
folio suite install
folio suite doctor
folio suite start
folio suite stop
folio suite uninstall
```

Suggested first slice:

- `suite plan`: print resolved paths, binaries, ports, launch agents, and MCP
  catalog entries. No writes.
- `suite install`: create directories, copy supplied binaries, render configs,
  render launchd plists, and render Tether catalog.
- `suite doctor`: verify installed paths, DB existence, launchd status, HTTP
  health, and Tether MCP proxy discovery.

Do not add auto-update or signing in the first slice.

## Acceptance Tests

Minimum portfolio-level checks:

1. `folio suite plan --profile local` produces no writes.
2. `folio suite install --profile local --from-source ~/dev/hollis-labs` creates
   app-support state and never writes databases into app repos.
3. `folio suite doctor` fails if any app resolves a repo-local DB.
4. Tether MCP proxy lists tools from Nanite, Torque, and Tesseract.
5. HTTP health endpoints respond on loopback for apps with HTTP services.
6. Stopping/restarting launch agents preserves user databases.

## Sequencing

1. Normalize Nanite as the reference app contract.
2. Bring Torque up to the same flag/config shape.
3. Bring Tesseract up to the same flag/config/help shape.
4. Bring Tether catalog/state defaults to installed Tether paths.
5. Implement Folio suite planner/installer/doctor.
6. Run end-to-end installed smoke on a clean macOS user account or temp HOME.

