<p align="center">
  <img src="src/asset/brand/analytix-app-icon-512.png" width="104" alt="Analytix icon">
</p>

<h1 align="center">Analytix</h1>

<p align="center">
  <strong>Analytix — Agents for sensitive work.</strong><br>
  <em>Built for any work. Ready for sensitive work.</em>
</p>

<p align="center">
  <a href="./README.md">简体中文</a>
  &nbsp;·&nbsp;
  <strong>English</strong>
  &nbsp;·&nbsp;
  <a href="#documentation-map">Docs</a>
  &nbsp;·&nbsp;
  <a href="#run-from-source">Run from source</a>
</p>

<p align="center">
  <a href="./LICENSE"><img src="https://img.shields.io/badge/first--party%20license-Apache%202.0-blue" alt="First-party license: Apache 2.0; third-party terms also apply"></a>
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey" alt="Platform">
  <img src="https://img.shields.io/badge/Electron-41-47848F?logo=electron&logoColor=white" alt="Electron 41">
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black" alt="React 19">
</p>

Analytix is an **Agent Platform** for everyday and sensitive work. It handles code, requirements, plans, research, writing, and automation while providing built-in boundaries for sensitive data and professional workflows. It is not just a chat client or a CLI shell for programmers.

The core workflow starts from the requirement, then moves through design, plans, todos, agent coding, file changes, and acceptance. The normal desktop flow uses local Provider onboarding, with one Registry and protected Secret Store managing credentials. Sessions, logs, preferences, and runtime data stay local by default. Settings manages Providers, models, and connection recovery; ordinary startup does not require Hub sign-in.

The only production agent core is the Go runtime in `packages/runtime-go`. `packages/runtime` provides the public TypeScript contracts, config, telemetry, and `analytix serve` launcher; that launcher starts the Go `runtime-server` and is not a second TypeScript agent runtime. Code, Write, SDD, Connect Phone, and scheduled tasks share one local HTTP/SSE runtime boundary.

## Go Agent Harness + Plugins + Privacy Layer

- **Go Agent Harness** is the one stable execution core for the agent loop, sessions, tools, jobs, subagents, providers, permissions, recovery, and public protocol.
- **Plugins** add professional tools, workflows, skills, and provider/public-safe UI extensions through one capability and lifecycle contract. Funds is Analytix's **first flagship professional plugin**. Knowledge, Legal, Research, Writing, Coding, and later capabilities follow the same path instead of entering Core.
- **Privacy Layer** is built into that same Go Harness. It applies unbypassable projections across models, subagents, compaction, memory, persistence, logs, telemetry, and protected local display. It is neither a second runtime nor an optional plugin.

**Any capability. Any model. Private by design.** From everyday workflows to sensitive data, Analytix uses one general Agent rather than a separate branch for sensitive work.

This is the accepted product and architecture target, not a claim that every privacy, plugin-isolation, or formal release gate is already complete. See the [Analytix Agent Platform brand and architecture spec](docs/analytix/specs/11-agent-platform-brand-and-architecture.md) for the Core/Plugin boundary, data flows, current implementation, and gaps.

---

<p align="center">
  <a href="src/asset/img/code.mp4">
    <img src="src/asset/img/code.gif" width="410" alt="Analytix Code mode demo">
  </a>
  <a href="src/asset/img/write.mp4">
    <img src="src/asset/img/write.gif" width="410" alt="Analytix Write mode demo">
  </a>
</p>

## Requirement-First Coding

Analytix explores a next-generation programming workflow: **requirement -> design -> plan -> code -> verify**. It is not just a chat box attached to an IDE.

| Stage | Analytix approach |
| --- | --- |
| **Clarify** | Create requirement drafts in the GUI and ask AI to find missing questions, research options, and shape boundaries |
| **Document** | Save drafts as `.analytixsdd/requirements/<uuid>/requirement.md`, with structured requirement blocks, acceptance criteria, and history |
| **Design** | Generate UI design drafts, infographics, or interactive HTML prototypes from selected requirement content |
| **Plan** | Use `/plan` and `create_plan` to produce GUI-owned `.analytixsdd/plan/...` implementation plans linked back to requirements |
| **Code** | Move from plan into todos, file edits, command execution, and change review |
| **Verify** | Bring requirement blocks, acceptance criteria, plan state, and `/review` together to answer whether the original requirement is done |

## Core Features

- **Code workbench**: bind a real codebase, chat around project context, run shell commands, edit files, and review changes before committing.
- **Requirements, plans, and review**: create requirement drafts, plan work, manage todos, run `/goal`, review changes, compact threads, fork, and archive.
- **Write mode**: dedicated Markdown workspaces with a file tree, Live / Source / Split / Preview modes, completion, selection-based inline agent actions, image attachments, and `HTML / PDF / DOC / DOCX` export.
- **Connect Phone**: Feishu / Lark / WeChat entry points, local webhook / relay support, and one-time or recurring scheduled tasks.
- **Model providers**: the local Provider Registry configures DeepSeek, Xiaomi MiMo, MiniMax, OpenAI-compatible, self-hosted, and other custom services.
- **Multimodal and media capabilities**: image attachments, vision input, speech transcription, image generation, speech generation, music generation, and video generation when enabled by provider config.
- **MCP and Skills**: Model Context Protocol servers and project/global Skills give Analytix specialized tools and workflows for different tasks.
- **Local runtime**: the TypeScript `analytix serve` launcher starts the Go `runtime-server`, which provides the unified HTTP/SSE boundary, append-only event logs, usage tracking, and context management.

## More Demos

<p align="center">
  <a href="src/asset/img/pdf-research.mp4">
    <img src="src/asset/img/pdf-research.gif" width="680" alt="PDF research demo">
  </a>
</p>
<p align="center"><em>PDF research and source organization demo</em></p>

<p align="center">
  <a href="src/asset/img/sdd.mp4">
    <img src="src/asset/img/sdd.gif" width="680" alt="Requirement clarification and planning demo">
  </a>
</p>
<p align="center"><em>Requirement clarification, requirement documents, and planning demo</em></p>

<p align="center">
  <a href="src/asset/img/mascot-ui-plugin.mp4">
    <img src="src/asset/img/mascot-ui-plugin.gif" width="680" alt="Mascot UI plugin demo">
  </a>
</p>
<p align="center"><em>mascot / cameo UI plugin demo</em></p>

## Quick Start

### Download a Release

General release packages use `analytix-${version}-${os}-${arch}.${ext}`. The supported release channels are `stable` and `beta`. Official Standard Windows is an explicit localized exception: its installed display name is `Analytix灵鉴` and the current x64 artifact is `analytix-standard-${version}-x64.exe`; the executable `analytix`, app id `com.analytix.desktop`, CLI `analytix serve`, and `ANALYTIX_*` environment variables keep the canonical analytix identity.

| Platform | Package | Architecture |
| --- | --- | --- |
| macOS | `.dmg` or `.zip` | Intel / Apple Silicon |
| Windows | `.exe`, NSIS installer | x64 |
| Linux | `.AppImage` | x64 |

On first launch:

1. Choose a UI language.
2. Choose a service in local Provider onboarding and configure its Base URL, endpoint format, and model.
3. Complete the normal credential-entry flow; the protected Secret Store holds credentials while settings retain key-free metadata.
4. Open Code and bind a local project, or open Write and create a writing workspace. Manage later Provider changes in Settings.

### Run From Source

Requirements:

| Dependency | Version |
| --- | --- |
| Node.js | Verified baseline 22.22.1; see `.node-version` |
| npm | 10.9.4 with locked installation |
| Go | CI / current native baseline 1.26.4; `go.mod` 1.22 is the minimum language version |
| Native development | Rust 1.94.1, matching SDK / host build authority and runtime assets; see the development baseline |
| Model service | A usable local Provider connection for model tasks; ordinary startup does not require a Hub account |

```bash
cd /path/to/analytix
# On the configured Owner macOS host, source the cache helper in the same zsh first
npm run bootstrap
npm run verify:baseline
```

This verifies source development, not complete installer readiness. With native
resources and host prerequisites prepared, use `npm run doctor -- --native`
and `npm run dev` for the full development chain. See the
[development baseline](docs/analytix/development-baseline.md) for asset supply,
platform limits, CI and packaging. `dev:fast` is not full initialization.
`dev` / `dev:fast` retain their existing behavior and are not automatically
isolated from real user data. The explicit `dev:isolated` entrypoint additionally
requires a provisioned task Keychain; prepared directories are not launch readiness.

On the configured Owner macOS host only, source
`./scripts/use-analytix-cache.sh` before install/test/build commands. Other
public checkout hosts do not need that Owner-specific storage layout.

For slower network access in mainland China, use an npm mirror:

```bash
npm ci --registry=https://registry.npmmirror.com
```

## Common Commands

| Command | Description |
| --- | --- |
| `npm run bootstrap` | Install Git sync guards, refresh locked app/runtime dependencies and check inputs |
| `npm run verify:baseline` | Check inputs, sync regressions, types, source build and output smoke |
| `npm run dev` | Build native data tools and TS launcher/contracts, then start the Electron dev app |
| `npm run build:runtime` | Build the `packages/runtime` launcher/contracts; this does not validate the Go core |
| `(cd packages/runtime-go && go test ./...)` | Run the Go runtime tests |
| `npm run build` | Electron / TypeScript source build, not complete native package acceptance |
| `npm run typecheck` | TypeScript type checking |
| `npm run lint` | ESLint checks |
| `npm run test` | Vitest tests |
| `npm run dist:mac` | Build macOS `.dmg` and `.zip` |
| `npm run dist:win` | Build the Windows NSIS installer |
| `npm run dist:linux` | Build the Linux AppImage |

## Configuration and Data

- Default runtime dataDir: `~/.analytix/data`.
- Default Write workspace: `~/.analytix/write_workspace`.
- Runtime-owned desktop settings use top-level `runtime`, while model provider profiles use top-level `provider`; old data is read only through explicit legacy import / migration boundaries.
- The one Registry / Secret Store authority manages Provider credentials for authorized runtime consumers. Ordinary settings, exports, and logs do not carry credentials. Hub remains an explicit, lazy compatibility surface.
- Fresh interactive sessions use execution-policy version 2 with
  `approvalPolicy: on-request` and `sandboxMode: workspace-write`. The latter
  is an Analytix application-level tool/path policy, not an operating-system
  sandbox; it limits file tools and blocks host shell execution.
  `danger-full-access` remains an explicit opt-in.
- The exact unversioned legacy `auto` plus `danger-full-access` pair migrates
  once; other valid explicit combinations and a version-2 full-access opt-in
  are preserved.
- Unattended Connect Phone and scheduled-task turns default to `never` plus
  `workspace-write`, so they do not wait for an absent operator approval.

## Documentation Map

Entries route current work. Select the relevant scope without recursively loading historical evidence.

| Doc | Contents |
| --- | --- |
| [Documentation map](docs/analytix/README.md) | Current owners, sources of truth, validation and historical queries |
| [Spec registry](docs/analytix/specs/README.md) | Accepted targets/contracts and historical classification |
| [Handover entry](docs/analytix/handovers/README.md) | Fresh candidate, writer and evidence recovery |
| [Go runtime](packages/runtime-go/README.md) | The only production Agent core |
| [TypeScript runtime](packages/runtime/README.md) | Launcher/contracts/config |
| [Configuration](docs/ANALYTIX_CONFIG.md) | Local Provider and desktop settings |
| [Plugins](plugins/README.md) | Plugin sources and packaging boundaries |
| [Upstream admission](docs/analytix/upstreams/README.md) | Pinned versions, licenses and provenance |
| [Security disclosure](SECURITY.md) | Vulnerability reporting |

## Local Maintenance

Bug fixes, UI/UX improvements, documentation, localization, build/release work, and runtime integration changes are maintained locally.

Before committing, choose validation from the current task, applicable `AGENTS.md`, and the affected source and consumers. Use `node scripts/validation-burden.mjs --plan <task-owned-paths> --json` for mapped maintenance routes. Check diffs and links for documentation changes; select type checks, tests, builds, and other applicable gates for the actual impact of source, localization, or build changes. Unmapped scopes require the affected owners and existing CI matrix. This route does not replace CI or formal acceptance.

## License

Analytix-owned first-party code is licensed under the [Apache License 2.0](./LICENSE). Existing third-party material remains subject to its own terms. Public availability and Git synchronization do not mean that all code is Apache-2.0 or grant additional third-party commercial-use or redistribution rights.

Analytix's first-party copyright owner and Project Owner is Guoqin He,
publicly operating under the GitHub account [Eysn0130](https://github.com/Eysn0130).

License and attribution obligations for bundled third-party material are recorded separately in [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md); that file does not change Analytix's Apache-2.0 license.

## Source synchronization

This project and [GitHub main](https://github.com/Eysn0130/analytix) share the
public mainline. Development follows **Branch → PR → CI/acceptance → Merge main**.
Fast-forward a clean `main`, create a short-lived `codex/*` branch, and commit/push
that branch. Merge its PR only after CI and applicable acceptance pass; do not
push directly to `main`. Run `npm run git:setup` once in a new clone.
See the [Git workflow](docs/analytix/git-workflow.md)
for safe synchronization, private-history protection and resources not included
in the public source tree.
