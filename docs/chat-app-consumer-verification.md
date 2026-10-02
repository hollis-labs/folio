# Generated chat-app consumer verification

## Registry 0.3.0 verification

CW-20261002-0025, 2026-10-02. Both `app-dashboard` and `chat-app` default
`design_kit_version` to 0.3.0 and generate `^0.3.0` ranges. Fresh scaffolds
installed from `https://registry.npmjs.org/` using separate empty npm user and
global configs. No tarballs, workspace links or registry overrides were used.
Both passed typecheck, Biome/design ESLint, Vite build and Go compilation.

`npm ls --all` exited 0 for each scaffold. The JSON dependency trees reported
no problems, and physical installed-package paths proved exactly one copy of
each package below. Lockfile tarball URLs point to the public registry.

| Package | app-dashboard | chat-app |
| --- | --- | --- |
| design-tokens | one 0.3.0 | one 0.3.0 |
| design-components | one 0.3.0 | one 0.3.0 |
| design-app-runtime | one 0.3.0 | one 0.3.0 |
| eslint-config-design | one 0.3.0 | one 0.3.0 |
| kit-dashboard | one 0.3.0 | absent |
| kit-chat | absent | one 0.3.0 |
| design-bindings (unchanged transitive package) | absent | one 0.1.0 |

The browser check drove the actual generated Go server and embedded production
UI, sent messages to `/api/messages`, received real echo replies, confirmed the
cleared draft and reset the conversation. Node 24.21.0, npm 11.19.0,
Playwright 1.61.1 and Chromium build 1243 were used.

| Measured property | 1024 × 800 | 390 × 800 |
| --- | --- | --- |
| Starter Send controls | 1 | 1 |
| Starter Send radius / font | 6px / 13px | 6px / 13px |
| Bubble / composer font | 13px / 13px | 13px / 13px |
| Composer wrapper border / radius | 1px / 10px | 1px / 10px |
| Composer width | 768px | 358px |
| Isolated component fixture Stop radius / font | 6px / 13px | 6px / 13px |

The starter supplies `busy` but no `onStop`: it **never renders Stop**. No
starter cancellation behavior is claimed or added. Stop was measured in a
separate verification-only Vite page importing the registry-installed
`ChatInput`, with `busy` and a fixture `onStop`, using the generated starter's
CSS. Clicking Stop returned that fixture to Send. This proves the optional
component state's styling, not backend cancellation. Both browser checks had
no uncaught page errors; the real starter had no horizontal overflow.

The committed browser check now requires one Send, its 6px radius and 13px
font, plus 13px token-matching bubble/composer text. Existing generated apps are
not rewritten. Verification covers Chromium and the local echo starter, not
other browsers, agent backends, persistence, markdown or every kit state.

Biome emitted the existing schema/deprecated-field informational notices;
npm reported the existing ESLint deprecation and two audit advisories (one
moderate, one high). No peer-resolution warnings occurred. These notices are
outside this default-version change.

Folio `make test`, `make all` (golangci-lint 2.11.4, vet and race tests),
`make pins` (including self-test) and `make vuln` (govulncheck 1.8.0) passed.

Raw logs, dependency proof, browser JSON, screenshots and the isolated fixture
are retained under `~/.cache/design-kit-tmp/folio-CW-20261002-0025/`:
`dashboard-gates.log`, `chat-gates.log`, `*-npm-ls.{txt,json}`,
`registry-proof.json`, `starter-browser.json`, `stop-fixture-browser.json`,
`check-stop.cjs`, `chat/frontend/src/stop-fixture.tsx`, `make-test.log`,
`make-all-pinned.log` and `make-pins-vuln.log`.

## Historical registry 0.2.0 verification

CW-20261001-0669, 2026-10-01. At that point both `app-dashboard` and `chat-app` defaulted
`design_kit_version` to 0.2.0 and generate `^0.2.0` ranges. Fresh scaffolds
installed from the public npm registry with an empty userconfig, then passed
typecheck, Biome/design ESLint, Vite build and Go compile. Each `npm ls --all`
exited 0 with one design-tokens 0.2.0, no duplicate Hollis packages and no peer
warnings. All direct design-kit packages resolved to 0.2.0; chat's transitive
design-bindings remains registry 0.1.0. Both presets already declare Tailwind v4.

The updated browser check drove the embedded Go chat app, clicked its built-in
Send control, received the real local echo and reset the conversation at both
widths. Node 24.21.0, npm 11.19.0, Playwright 1.61.1 and cached Chromium build
1243 were used. No page errors or horizontal overflow occurred.

| Measured property | 1024 × 800 | 390 × 800 |
| --- | --- | --- |
| Send controls | 1 | 1 |
| Bubble / composer font | 13px / 13px | 13px / 13px |
| Composer wrapper border / radius | 1px / 10px | 1px / 10px |
| Composer width | 768px | 358px |

Composer chrome is measured on `[data-slot="chat-input"]`; the textarea supplies
the font measurement. That browser check required token-matching typography
and one Send control. Template markup already delegates Send to `ChatInput`;
no second control was added. Folio's full `make test` also passed.

Biome printed two informational notices (schema patch version and deprecated
`recommended` field), and npm printed an ESLint deprecation notice; these did
not fail checks and are outside this default bump. Existing generated apps are
not rewritten. Verification covers Chromium and the local echo starter, not
other browsers, agent backends, persistence, markdown or every card/state.

## Historical registry 0.1.0 verification

CW-20261001-0513, 2026-10-01. Rendered Folio's actual `chat-app` preset into a
fresh temporary directory, installed published npm dependencies, typechecked,
ran Biome plus design ESLint, built Vite output into the Go embed directory,
and compiled/ran the generated Go server on a temporary loopback port.
No workspace links, packed unreleased kits, registry overrides, Nanite services
or credentials were used.

Candidate defaults: `design_kit_version=0.1.0`, generated `^0.1.0` ranges;
registry kit-chat, design-components and design-tokens were still 0.1.0.
Node 24.21.0, npm 11.19.0, Playwright 1.63.0, cached Chromium build 1243.
The missing libasound library was supplied from a task-local extracted directory,
without changing the system. All build/test scratch used on-disk TMPDIR/GOTMPDIR.

## Browser evidence

The check drove the **embedded production app** at 1024 × 800 and 390 × 800,
sent a message through `/api/messages`, received the real echo response,
checked the cleared draft, and reset the transcript using New conversation.
Both widths produced no page errors or horizontal document overflow.

| Computed property | Desktop | Narrow |
| --- | --- | --- |
| User row alignment | flex-end | flex-end |
| User bubble radius | 10px (panel token) | 10px (panel token) |
| User bubble background | rgb(39, 39, 42) | rgb(39, 39, 42) |
| User bubble maximum width | 512px | 512px |
| Composer border / radius | 1px / 8px | 1px / 8px |
| Composer width | 768px | 358px |
| Transcript padding left / top | 16px / 24px | 16px / 24px |
| Transcript overflow-y | auto | auto |
| Emitted chat utilities | text-control, items-end, max-w-lg | same |

These are measured values of this candidate, not counts asserted against future
file contents. The browser assertions check actual layout/background/radius and
emitted source utilities, so a successful build alone cannot satisfy them.

## Published typography limitation

The source-registration and interaction checks pass, but **full typography
compliance is not claimed**. The control token is 13px. Published 0.1.0 bubbles
compute 16px; the desktop composer computes 14px, while narrow computes 13px.
The emitted `.text-control` utility is present. This reproduces the font-merge
and desktop override defects described in design-kit's
[kit-chat consumer verification](https://github.com/hollis-labs/design-kit/blob/main/packages/kit-chat/docs/consumer-verification.md).
The browser JSON reports control-token mismatches separately rather than hiding
them behind its source-emission/layout/interaction PASS.

At the time of this original run, merged kit-chat polish was not available from
the registry and the preset targeted 0.1.x. Published 0.2.0 included the fixes,
and CW-20261001-0669 changed the default to `^0.2.0`. The historical measurements
and screenshots above remain evidence of 0.1.0, not the current default.

## Artifacts and reproduction

- [Desktop screenshot](screenshots/chat-app-1024.png)
- [Narrow screenshot](screenshots/chat-app-390.png)
- [Computed styles and mismatch flags](screenshots/chat-app-computed-styles.json)

From the Folio root, provide an installed Playwright module and Chromium (see
README's Chat apps section), then run. For registry-only verification, provide
separate empty npm config files through `NPM_CONFIG_USERCONFIG` and
`NPM_CONFIG_GLOBALCONFIG`, and set `NPM_CONFIG_REGISTRY=https://registry.npmjs.org/`:

```sh
export TMPDIR=$HOME/.cache/design-kit-tmp GOTMPDIR=$HOME/.cache/design-kit-tmp
FOLIO_FRONTEND_E2E=1 FOLIO_BROWSER_E2E=1 \
  FOLIO_PLAYWRIGHT_MODULE=/path/to/browser-tools/node_modules/playwright \
  FOLIO_CHROMIUM_EXECUTABLE=/path/to/chromium \
  FOLIO_SCREENSHOT_DIR=/path/to/task-artifacts/screenshots \
  go test . -run TestIntegration_ChatAppPreset_Frontend -v -count=1
```

The normal offline render/discovery suite also covers custom mount paths.
Folio's `make all`, `make pins` (including selftest), and `make vuln` passed.
The initial npm tarball 404 was registry propagation lag and cleared on retry;
no override was added. This verification covers the starter echo UI, not an
agent backend, history persistence, optional markdown, or every kit card/state.
