---
# DESIGN.md frontmatter -- machine-readable design context for design agents.
# Values point at the live codebase instead of duplicating every token here.

schema_version: 2
project: analytix
product_name: Analytix
product_category: Agent Platform
brand_line: Analytix — Agents for sensitive work.
architecture: Go Agent Harness + Plugins + Privacy Layer
runtime_core: packages/runtime-go
runtime_launcher_contracts: packages/runtime
runtime_cli: analytix serve
preload_bridge: window.analytix
settings_schema:
  runtime: runtime-owned desktop settings
  provider: model provider profiles
themes: [light, dark, system]
brand_assets:
  root: src/asset/brand
  provenance: src/asset/brand/provenance.json
  app_icon: src/asset/brand/analytix-app-icon-512.png
  splash: src/asset/brand/analytix-splash.png
visual_tokens:
  css: src/renderer/src/styles
  tokens: src/renderer/src/design/analytix-visual-tokens.ts
  icon_registry: src/renderer/src/design/AnalytixIconRegistry.ts
thread_runtime:
  projection: src/renderer/src/thread/projection
  virtualizer: src/renderer/src/thread/virtualizer
  streaming: src/renderer/src/thread/streaming
  tracing: src/renderer/src/thread/tracing
---

# Analytix Design Guide

**Analytix — Agents for sensitive work.** Analytix is an Agent Platform for
everyday and sensitive work. Design work should preserve the existing coding,
writing, planning, review, automation, and professional-plugin surfaces while
aligning product identity, runtime architecture, brand assets, privacy, and chat
performance with the registered specs in `docs/analytix/specs/`. Start at
`docs/analytix/README.md` to distinguish accepted targets, current behavior,
and dated evidence.

## Product Shape

- The user-visible brand is **Analytix** and the product category is **Agent
  Platform**. Lowercase `analytix` remains the machine identity for package,
  executable, CLI, protocol, app, and environment contracts where specified.
- The architecture expression is **Go Agent Harness + Plugins + Privacy
  Layer**. These are responsibilities inside one Go runtime, not separate
  runtimes.
- The only production agent core is the Go runtime in `packages/runtime-go`.
- `packages/runtime` contains the TypeScript launcher, public contracts, config,
  and telemetry. Its `analytix serve` CLI starts the Go `runtime-server`.
- Renderer code talks to the preload bridge through `window.analytix`.
- Runtime-owned desktop settings use top-level `runtime`; model provider profiles
  use top-level `provider`.
- Runtime-owned environment variables use the `ANALYTIX_*` prefix.
- Ordinary startup uses local Provider onboarding and Registry readiness.
  Settings manages Provider connections through the protected Secret Store;
  Hub is an explicit lazy compatibility surface, not a startup dependency.
- Official Standard Windows uses the localized display name `Analytix灵鉴` and
  `analytix-standard-${version}-x64.exe`. Its executable, app id, CLI, and
  environment-variable identity remain `analytix`, `com.analytix.desktop`,
  `analytix serve`, and `ANALYTIX_*`.
- Connect Phone is the user-visible name for phone/IM workflows.
- Funds is the first flagship professional plugin, not the primary brand or
  Core. Knowledge, Legal, Research, Writing, Coding, and later professional
  capabilities follow the same plugin direction.
- Unmasked sensitive values use the Core-owned protected local presentation
  surface. Generic renderer state and arbitrary plugin UI receive only their
  admitted public/model-safe projections.
- Retained mascot behavior should use mascot/cameo naming in current source and
  UI.

## UX Principles

- Preserve existing user flows, entry points, and display semantics.
- Prefer quiet, dense, work-focused surfaces over marketing-style composition in
  the core app.
- Do not hide features while changing identity, assets, runtime contracts, or
  chat virtualization.
- Use icons for compact commands, segmented controls for modes, toggles for
  binary settings, sliders/inputs for numeric settings, and menus for option
  sets.
- Repeated cards should keep radii at 8px or less unless an existing component
  explicitly requires a larger value.
- Avoid heavy blur, expensive shadows, and layout-triggering animation in the
  chat streaming path.

## Visual System

The live visual source of truth is code:

- `src/renderer/src/styles/*.css` for shell, component, and theme variables.
- `src/renderer/src/design/analytix-visual-tokens.ts` for surface, elevation,
  border, radius, and motion semantics.
- `src/renderer/src/design/AnalytixIconRegistry.ts` for the app icon registry.
- `src/asset/brand/provenance.json` for brand asset provenance.
- `build/icon.icns` and `build/icon.ico` for packaged app icons.

Brand assets should remain under `src/asset/brand` and should not expose
reference-product identity. New visual assets should include provenance and be
checked in only when they are part of the product source, not local build output.

## Workbench Layout

The desktop shell is an Electron main process plus React renderer:

```text
Electron main
  -> preload bridge: window.analytix
  -> React workbench
  -> local HTTP/SSE runtime: packages/runtime-go (Go runtime-server)

analytix serve: packages/runtime (TypeScript launcher/contracts)
  -> packages/runtime-go (Go runtime-server)
```

Code, Write, SDD, Preview, Dev Browser, Terminal, right panel, bottom panel, and
Connect Phone are workbench islands. They should subscribe to the smallest
state needed for their own surface and must not amplify token-by-token chat
streaming into broad app rerenders.

## Chat Timeline

The production chat path is:

```text
runtime event
  -> thread projection
  -> stable row model
  -> Analytix-derived ThreadVirtualizer
  -> row renderer
```

Guidelines:

- Do not derive turn sections repeatedly in the render hot path.
- Streaming text deltas enter a buffer and flush at most once per animation
  frame.
- Structural events may flush promptly so tool calls, approvals, and process
  state remain responsive.
- Measurement should use a row cache and batched `ResizeObserver`.
- Keep bottom distance stable when the user is near the bottom.
- Do not pull the user back to the bottom while they are reading history.
- History prepend must preserve the first visible row's visual position.
- Streaming markdown/code uses a lightweight text surface; finalized messages
  may render rich markdown and syntax highlighting.

## Runtime And Data

Default local data locations:

```text
~/.analytix/data
~/.analytix/write_workspace
```

First-stage legacy data behavior is explicit import/migration only. Current UI,
runtime launch, packaging, assets, release metadata, and settings should not use
old product identity.

SDD requirements live under
`.analytixsdd/requirements/<uuid>/requirement.md`; GUI plans live directly under
`.analytixsdd/plan/`.

## Validation

For closure-level design or architecture work, run:

```bash
npm run test
npm run typecheck
npm run build
npm run lint
git diff --check
```

Also run the naming scans from
`docs/analytix/specs/06-implementation-closure-and-acceptance.md` and classify
any remaining hits as current-product bug, legacy-kun migration/import/test
fixture allowed, provider/model name allowed, historical doc isolated, or
third-party/vendor path allowed.

Go runtime changes require a Go 1.22+ compatible toolchain, `gofmt`, and focused
coverage such as:

```bash
(cd packages/runtime-go && go test ./...)
```
