# analytix runtime and model configuration

Status: Operational as-built configuration guide.

Use `docs/analytix/README.md` for document authority and lifecycle. This file
describes the current Go-only production path; it does not promote every field
accepted by the TypeScript schema into an implemented Go feature.

## Configuration Layers

analytix has four related but distinct layers.

### 1. Local Provider Registry and protected Secret Store

Ordinary startup uses local Provider onboarding and Registry readiness. One
data-directory-scoped Registry owns Provider metadata and opaque credential
references; the protected Secret Store holds credential bytes. The existing
Electron main/preload/renderer API connects the UI to that authority, and the
Go runtime remains the Provider execution owner.

Settings, ordinary IPC snapshots, exports and diagnostics carry key-free
metadata. Normal credential entry is a dedicated input operation, not a secret
read API. Source review distinguishes component-local input and its dedicated
write request from key-free shared settings and read responses; it does not
establish complete credential-surface acceptance. The remaining contract and
input-lifetime coverage are recorded in
[`documentation-delivery-review-2026-09-09.md`](analytix/documentation-delivery-review-2026-09-09.md).
Do not treat this distinction as permission for shared/persistent renderer
credentials or a secret readback API.

Hub login, account refresh and gateway credentials are explicit lazy
compatibility inputs only. Ordinary startup must not load them or fall back to
a Hub-managed Provider. The accepted contract is
[`local-provider-credential-authority`](../openspec/specs/local-provider-credential-authority/spec.md).

### 2. Desktop settings

Electron-owned preferences are stored in `analytix-settings.json` under the
platform app-data directory. Runtime-owned fields live under top-level
`runtime`; model provider profiles live under top-level `provider`.

Typical settings locations:

- macOS: `~/Library/Application Support/analytix/analytix-settings.json`
- Windows: `%APPDATA%/analytix/analytix-settings.json`
- Linux: `~/.config/analytix/analytix-settings.json`

New saves must not write an old `agents.kun`, `agents.analytix`, or retired
runtime-backend tree.

### 3. Synchronized runtime config

Before Electron main starts the Go runtime, it synchronizes advanced values to:

```text
<runtime.dataDir>/config.json
```

The default runtime data directory is:

```text
~/.analytix/data
```

Electron then starts `packages/runtime-go/cmd/runtime-server` with explicit
provider/runtime arguments, managed environment, and the config path. The Go
composition root is `packages/runtime-go/internal/runtimeapp/app.go`.

### 4. Standalone `analytix serve`

The TypeScript launcher in `packages/runtime` supports:

```bash
analytix serve --config <path> --data-dir <path>
```

The launcher schema and standalone defaults are compatibility/API behavior;
they are not the packaged GUI first-run policy. Compatibility parser defaults
do not provide usable credential authority or replace normal protected local
Provider setup.

## Startup Flow

The packaged desktop flow is:

1. Main reads and normalizes key-free desktop settings and composes the local
   Provider API with the existing Go runtime boundary.
2. Local Registry readiness determines whether setup, recovery guidance or
   normal workspace entry is appropriate.
3. The user configures a connection through normal onboarding or Settings;
   credential-bearing changes use the protected Registry transaction.
4. Main synchronizes supported key-free advanced config to `<dataDir>/config.json`.
5. Authorized Go Provider consumers resolve committed credentials through the
   one Registry / Secret Store authority; stale or unavailable state fails
   closed without a settings or Hub fallback.

The launcher still parses config, supported environment variables and CLI
arguments. Their parser precedence does not authorize a second credential
store or make every compatibility field active in ordinary product execution.
Trace a particular launch or standalone path before claiming its behavior.

## Current Production-Consumed Inputs

The following categories are wired into the current Go production path. Trace
the exact field to code before expanding this table.

| Input | Current owner/consumer |
| --- | --- |
| host, port, data dir, runtime token, insecure | TypeScript launcher or Electron main -> Go runtime-server CLI -> `runtimeapp` |
| Provider metadata, model, endpoint format and routing | local Provider API / Registry -> Go provider configuration and outbound adapters; credentials use protected resolution |
| model/MCP proxy | runtime-server CLI -> Go provider/MCP clients |
| approval policy and sandbox mode | runtime-server CLI -> Go control/tool policy |
| `capabilities.mcp.servers` and MCP search | synchronized config -> Go MCP manager/runtime info |
| `capabilities.skills` | synchronized config -> Go skill catalog |
| `capabilities.subagents` | synchronized config -> Go subagent profile settings |
| `capabilities.web` | synchronized config -> Go web runtime configuration |
| `capabilities.visionBridge` | synchronized config -> Go vision bridge configuration |
| `runtime.streamIdleTimeoutMs` | synchronized config -> Go provider stream watchdog; positive values are milliseconds, `0` disables it, and an absent value keeps the 120-second default |
| `runtime.stepLimits` | synchronized config -> Go loop step-limit resolution |
| provider model-profile `contextWindowTokens` | provider JSON -> selected Go turn model and runtime-info model capability |
| write/protected roots | managed environment/config -> Go filesystem policy |

The desktop also consumes some settings before Go launch. For example,
Computer Use settings can cause Electron main to add a managed MCP server.
That is different from the Go core directly consuming the original field.

Current launcher and new Go thread defaults are `approvalPolicy: on-request`
and `sandboxMode: workspace-write`. Workspace-write is an Analytix
application-level tool/path policy, not an OS sandbox; it blocks foreground
and background host shell execution. Full host shell and unrestricted file
access require explicit `danger-full-access`. The exact unversioned legacy
`auto` plus `danger-full-access` pair migrates once, while other valid choices
and an explicit version-2 full-access opt-in are preserved. Unattended Connect
Phone and scheduled-task turns use `never` plus `workspace-write` so they do
not wait for operator approval.

## Minimal Advanced Config Example

This example shows currently wired capability shapes without secrets:

```json
{
  "runtime": {
    "streamIdleTimeoutMs": 45000,
    "stepLimits": {
      "defaultMaxModelSteps": 64,
      "plannerMaxModelSteps": 24,
      "headlessMaxModelSteps": 32
    }
  },
  "capabilities": {
    "mcp": {
      "enabled": false,
      "servers": {},
      "search": {
        "enabled": false,
        "mode": "auto",
        "autoThresholdToolCount": 24,
        "topKDefault": 5,
        "topKMax": 10,
        "minScore": 0.15
      }
    },
    "web": {
      "enabled": false,
      "fetchEnabled": false,
      "searchEnabled": false,
      "allowDomains": [],
      "denyDomains": ["localhost", "127.0.0.1"],
      "maxFetchBytes": 1000000
    },
    "skills": {
      "enabled": false,
      "roots": ["~/.agents/skills", "./.agents/skills"],
      "legacySkillMd": true
    },
    "subagents": {
      "enabled": true,
      "maxParallel": 2,
      "maxChildRuns": 4,
      "defaultToolPolicy": "readOnly",
      "profiles": {}
    },
    "visionBridge": {
      "enabled": false,
      "mode": "auto",
      "endpointFormat": "chat_completions",
      "maxImageDimension": 1280,
      "maxImageBytes": 1500000,
      "maxScreenshotsPerTurn": 4,
      "observationCacheTtlMs": 120000,
      "injectPolicy": "observation_text",
      "fallbackWhenPrimaryImageUnsupported": true,
      "semanticProbeStatus": "unknown"
    }
  }
}
```

Do not put live Provider, Hub, MCP or bridge secrets in this file. Use the
normal protected Provider/account flow. A compatibility environment parser is
not an alternative ordinary credential authority.

The tracked standalone example at
[`packages/runtime/config.example.json`](../packages/runtime/config.example.json)
uses the same production-active field-family allowlist and omits credential
values. A schema regression test keeps compatibility-only families out of that
example; schema acceptance by itself still does not prove Go consumption.

## Parsed Or Synchronized Does Not Mean Active

The TypeScript schemas still accept, and Electron may still preserve or write,
several TypeScript-era settings. As of the current Go production composition,
the following must not be documented as active configurable behavior without a
new implementation trace and Go tests:

| Field family | Current status |
| --- | --- |
| `serve.storage.backend` / `sqlitePath` | Compatibility schema only; the Go durable store does not select the old TypeScript hybrid/SQLite index through these fields. |
| `hooks` and `quality` | TypeScript schema/test-support remains; the Go production agent loop does not execute the documented command-hook/design-quality pipeline. |
| `serve.tokenEconomy*` | Parsed/forwarded compatibility surface; not wired into the current `runtimeapp` production behavior. |
| `contextCompaction` and top-level `models.profiles` compaction thresholds | Retained TypeScript configuration model; do not claim model/heuristic compaction tuning is active in Go from these keys. Provider profile/model routing is a separate current path. |
| `runtime.toolStorm` / `toolArgumentRepair` | Written/parsed compatibility settings; current Go guards are not configured through these fields. |
| memory enablement and media-generation capability blocks | Compatibility schema only. Memory is currently a manual/store-only registry; Write image generation and speech-to-text are Electron-owned surfaces; Go TTS/music/video generation is unavailable. |

This table records an as-built documentation gap, not a decision to remove the
features permanently. Implementing one requires a scoped spec/change, Go
ownership, contract propagation, and focused tests; removing a stale field
requires migration analysis.

## Runtime Diagnostics

Use live runtime endpoints rather than config presence to determine actual
availability:

```text
GET /health
GET /v1/runtime/info
GET /v1/runtime/tools
```

- `disabled` means policy/config keeps a capability off.
- `unavailable` means it was requested or expected but its backing component is
  absent or failed.
- Provider 404s require checking selected provider, model, Base URL, Endpoint
  format, and the sanitized final request URL separately.
- If a setting appears ignored, trace it through
  `src/main/analytix-process.ts`, `src/main/runtime/analytix-adapter.ts`,
  `packages/runtime-go/internal/runtimeapp`, and the relevant Go app/adapter.

## Validation

For configuration changes, choose the smallest applicable checks below.

On the configured macOS host, source `./scripts/use-analytix-cache.sh` in the
same shell before commands that use build storage. These are a menu, not a
required full-suite sequence.

```bash
npm run typecheck
npm run test -- src/main/analytix-process.test.ts src/main/runtime/analytix-adapter.test.ts --run
npm --prefix packages/runtime run test
(cd packages/runtime-go && go test ./...)
git diff --check
```

Do not claim a configuration feature works because JSON parsing succeeded.
Verify its observable Go behavior and runtime diagnostics.
