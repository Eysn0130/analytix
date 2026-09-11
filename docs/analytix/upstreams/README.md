# analytix upstream absorption ledger

Status: Reference / chronological decision and evidence ledger.
Current as of: 2026-08-26 for Agent Platform architecture research and
2026-08-23 for source and installed-plugin intake; dated capability reports remain
snapshots tied to their recorded commits.
Source of truth for current product behavior: current code/tests and fresh
evidence, not a historical ledger row.

This directory records how analytix studies and absorbs capability from the
twelve repositories currently registered in `upstream-sources.json`: Kun,
DeepSeek-Reasonix, OpenCode, CodexDesktop-Rebuild, Hermes Agent, Claude Code,
Claw Code, Gajae Code, LazyCodex, postgres-mcp, Instructor, and OpenClaw. It exists so analytix can keep learning
without becoming a patch pile, losing its product architecture, or copying
material without a verified license and provenance chain.

Normative governance and document authority:

```text
docs/analytix/README.md
docs/analytix/specs/08-upstream-absorption-and-go-runtime.md
docs/analytix/specs/09-agent-quality-product-benchmark.md
docs/analytix/specs/10-subagent-todo-goal-control-plane.md
docs/analytix/specs/11-agent-platform-brand-and-architecture.md
```

Current status as of 2026-06-29:

- Go runtime is the default deterministic core through `go-runtime-default`.
- `ANALYTIX_RUNTIME_BACKEND=typescript` is retired and rejected by the current
  desktop adapter.
- The historical D-0248/D-0250C scoped code stage and Go-default cutover are
  closed. That dated closure is not proof that the current P0-P4 Goal, current
  Reasonix pin, or any parity/superiority benchmark is complete.
- D-0251/D-0252/D-0253 provider, MCP, packaged, operator, soak, and fallback
  retirement evidence is post-cutover live validation. Missing external
  credentials or packaged/operator inputs must not be read as blockers for Go
  default startup.
- Historical ledger rows that say Go is shadow-only, TypeScript is current
  default, or live evidence blocks default cutover are retained as stage
  history and are superseded by the current code path and fresh gates. A report
  named `final` is still dated evidence, not permanent current authority.
- `npm run runtime:go:rc-control-plane -- --json` may pass only the
  source-bound validation control plane and never authorizes product packaging
  or release. Current release packaging must pass
  `npm run runtime:go:release-gate`, whose final JSON must have `passed: true`
  after artifact admission, the complete active OpenSpec `RC_REQUIRED`
  projection, preflight, and authorized formal product evidence close. Old
  stage-numbered scripts are archive context, not current production gates.

Current upstream intake as of 2026-08-23:

- `upstream-sources.json` pins all twelve current checkouts and their tracked
  license-object evidence/postures. The Goal-thread authorization declaration
  is recorded separately and does not rewrite those license classes.
- `npm run audit:upstreams` emits two independent projections without changing
  an upstream worktree. `artifactAdmission` reads only rows explicitly marked
  `artifact-entering` in `code-reuse-provenance.md` and fails closed on an
  unresolved disposition, registration, pin/object, license, or NOTICE gap.
  `researchFreshness` reports discovery, checkout/tracking-head/branch/remote
  drift, and unavailable or unregistered research checkouts as `STALE` or
  `UNVERIFIED`; it does not block an artifact that did not adopt those commits.
  Neither projection is capability or superiority proof, and neither may
  upgrade the other to PASS.
- Source-specific ledgers and `upstream-sources.json` record their reviewed
  pins, preserved dirty state, license objects, and deep-audit
  decisions. `upstream-currentness-delta-2026-07-18.md` and
  `upstream-capability-audit-2026-07-10.md` remain dated construction
  snapshots; neither document proves current upstream parity.
- Direct reuse is blocked until `code-reuse-provenance.md` records exact source
  and destination paths, reuse mode, license/NOTICE obligations, modifications,
  reviewer, and validation. A root license does not cover every gitlink,
  vendored subtree, extracted asset, or file-level exception.

## Files

| File | Purpose |
| --- | --- |
| `kun-sync.md` | Review and absorption ledger for Kun releases, commits, and product workflow changes. |
| `reasonix-sync.md` | Review and absorption ledger for Reasonix releases, commits, and agent/runtime changes. |
| `opencode-sync.md` | Review and comparison ledger for OpenCode agent, provider, task, sub-agent, TODO, and session-continuation ideas. |
| `codexdesktop-rebuild-sync.md` | Review and reference ledger for CodexDesktop-Rebuild desktop feel, responsiveness, virtualizer, worker, signal, and interaction evidence. |
| `hermes-agent-sync.md` | Review and comparison ledger for Hermes Agent learning loop, memory, skill evolution, gateway, scheduling, sub-agent, and trajectory ideas. |
| `claude-code-sync.md` | Behavior-only ledger for Claude Code daemon, background, enterprise policy, plugin, and safety ideas; the source is not open for direct copying. |
| `claw-code-sync.md` | Low-confidence comparison ledger for Claw Code task vocabulary, Rust experiments, and fixtures. |
| `gajae-code-sync.md` | Gajae Code orchestration, external-control, session, plugin, native-tool, and provenance ledger. |
| `lazycodex-sync.md` | LazyCodex committed plugin-component ledger for LSP/CodeGraph, team/worktree, bootstrap, continuation, and evidence verification. |
| `upstream-sources.json` | Machine-readable twelve-source commit, remote, ledger, and license evidence inventory. |
| `openclaw-sync.md` | OpenClaw v2026.5.18 annotated-tag, peeled-commit, license, and compatibility-shim routing record. |
| `upstream-currentness-delta-2026-07-18.md` | Current fast-forward/source-identity intake, preserved dirty boundary, preliminary delta routing, and explicit remaining gaps. |
| `postgres-mcp-sync.md` | Current postgres-mcp source, license, SQL-safety comparison role, intake decisions, and unresolved risks. |
| `instructor-sync.md` | Current Instructor source, license, schema/retry comparison role, intake decisions, and unresolved risks. |
| `data-analytics-plugin-sync.md` | Installed Data Analytics source/data-quality/reproducibility/artifact-QA audit and absorption boundary. |
| `investment-banking-plugin-sync.md` | Installed Investment Banking router/source-of-truth/manifest/QC audit, including current focused test drift. |
| `public-equity-investing-plugin-sync.md` | Installed Public Equity classification/citation-readiness/artifact audit, including current focused test drift. |
| `agent-platform-architecture-recheck-2026-08-25.md` | Dated official-source recheck of Codex, Claude Code, DeepSeek Harness, and OpenCode, including a 2026-08-26 latest-ref and plugin-boundary addendum supporting the accepted Agent Platform architecture target. |
| `upstream-capability-audit-2026-07-10.md` | Historical white-box capability, document-error, risk, reuse, and construction snapshot across all nine sources. |
| `code-reuse-provenance.md` | Operational provenance and attribution queue for copied, ported, vendored, extracted, or substantially adapted material. |
| `absorption-targets.md` | Concrete matrix of what to absorb from all upstream sources and how to prove it is better. |
| `continuous-absorption-strategy.md` | Standing strategy for keeping analytix on `main` while continuously absorbing upstream capability through specs, contracts, benchmarks, and ledgers. |
| `code-level-absorption-blueprint.md` | Historical nine-source reuse/admission map, landing boundaries, historical two-source decisions, conflict rules, and proof requirements. |
| `code-level-implementation-plan.md` | Historical stage plan. New construction uses the dated all-source audit, current absorption targets, and a scoped OpenSpec change. |
| `stage-closure-playbook.md` | Historical pre-cutover stage procedure. It is superseded for current construction and does not authorize a `/goal`, TypeScript oracle, Go shadow order, or old priority list. |
| `absorption-ledger.md` | Cross-upstream item tracker used when a batch spans multiple domains. |
| `absorption-admission-gate/` | Accepted OpenSpec-first admission gate for future upstream absorption rows, including Hard Gate Evidence and OpenSpec/Superpowers handoff rules. |
| `conflict-decisions.md` | Durable ADR-style record for conflicts and rulings. |
| `go-runtime-conformance.md` | Historical contract/fixture plan and stage inventory for introducing the Go runtime; current implementation ownership starts in `packages/runtime-go` and current claims require fresh gates. |

## Operating Rule

The twelve registered repositories are upstream capability sources. analytix is
the product and architecture owner.

Each source ledger contains exactly one `analytix-upstream-review-v1` marker.
Its `reviewedCommit` must equal both the manifest pin and the checkout tracking
HEAD. A `reached` or `exceeded` marker is rejected unless it binds a
SHA-256-verified `CapabilityBenchmarkV1` artifact, the exact Analytix and
upstream commits, the executed test ids and command, `passed: true`, and
`skipped: 0`. All current markers remain `not-proven`; source refresh and deep
intake do not establish parity.

Source roles are default research lenses, not exclusive capability boundaries.
No upstream owns a capability category. For every analytix capability, compare
all relevant upstreams and absorb the best proven design through analytix-owned
contracts.

Local Git rule:

```text
analytix mainline branch: main
analytix remotes: none by default for private local development
upstream research sources: /Users/sun/Projects/_upstreams/*
```

The upstream sources are research inputs, not Git remotes for analytix. Do not
compare the analytix working tree against an upstream remote as if analytix were
a fork of that project.

Kun product capabilities are a regression floor for inherited workflows. They
must keep the target Kun version's product position, entry layer, and trigger
path when claiming Kun parity. Later Kun current/main/master features, or ideas
from any other upstream, may still become analytix-native capabilities through
an explicit analytix spec, UX rationale, tests, and QA. They must not be
promoted into legacy baseline navigation by implication.

Every upgrade should answer:

```text
What changed upstream?
What does analytix want from it?
What conflicts with analytix?
How is the change adapted into analytix contracts?
How was it tested?
What was rejected or deferred?
Which source commit/path/license covers the material?
Where are required copyright, license, NOTICE, and modification records kept?
```

No project license or all-rights-reserved source defaults to behavior-only,
clean-room study. Permissive root licenses still require file-level review and
notice preservation. Written authorization must identify the covered material,
scope, and asset/source hashes; an undocumented statement that permission was
granted is not evidence.

## Batch Template

Use this shape in the relevant ledger:

```text
## YYYY-MM-DD - <upstream> <version-or-commit-range>

Source:
Reviewed by:
Related branch:

Summary:

Classification:
| Item | Area | Class | analytix decision | Follow-up |
| --- | --- | --- | --- | --- |

Conflicts:
| Conflict | Ruling | Decision id |
| --- | --- | --- |

Validation:

Remaining risks:
```
