# Runtime Boundary

This plugin is a commercial runtime package for analytix case-project fund investigation.

## Data Access

- Resolve the current case project through `get_current_case`; the resolver must
  use the runtime workspace and `.analytix/case-project.json`, not a global
  case state.
- Use `analytix_funds` MCP tools for case data. Semantic fact tools are the
  default path; Controlled Case Workbench handles custom SQL / notebook work
  when semantic tools cannot answer an explicit reproducible computation
  request.
- Do not treat weak sources, unverified calculations, full-row dumps, or
  bounded previews as source-backed case facts.
- Controlled workbench queries must remain current-case-project, read-only,
  cleaned/analysis-scope, row-limited, purpose-tagged, and backend-audited. They
  must not read `fc_*_raw`, source files, external files, network functions, or
  cross-case data.
- Treat the backend as owner of permissions, case existence, data cleaning state,
  concurrency, cache, and internal audit ids. Do not expose internal audit ids
  in user-facing answers.

## Owner And Query Order

Choose the focused owner first, then the smallest source path that satisfies the
owner's completion gate. Do not force all ordinary questions through
`funds_investigate`; it is a navigator/support layer.

Use this source order unless the selected owner has a smaller safe path:

1. Case-project resolution when status or source is unclear.
2. Scope, coverage, and casegraph context when downstream facts depend on them.
3. Aggregate/rank/profile/trace/hypothesis tools that directly answer the owner.
4. Fact/edge/evidence packs for visuals, reports, and proof tables.
5. Bounded transaction rows only when needed for a concrete example or appendix.
6. Controlled Case Workbench only for explicit custom SQL/notebook or
   reproducible-analysis gaps
7. Report, claim review, continuation-list validation, or delivery QC.

For Pair Amount, `pair-amount-investigation` owns final prose. MCP cards,
ranking rows, duplicate audit, and workbench aggregates are support evidence and
must not be copied as the final answer.

## Performance And Safety

- Prefer aggregated tools before row slices.
- Keep row queries bounded.
- Use one heavy full-case/report workflow at a time.
- If sub-agents are used, create one shared scope pack first and keep heavy detectors serialized or backend-queued.
- If a tool fails, report the failed tool and stop report-grade conclusions.
- Do not treat missing output as zero.
- Do not create hidden fallback paths that substitute weak or unverified sources
  for Analytix source-backed facts.
- If a semantic MCP tool is insufficient, first choose another semantic analysis
  path. Use Controlled Case Workbench only when the gap is real and explicit; if
  the workbench tool is unavailable, report the capability gap instead of
  fabricating results from weak sources, unverified calculations, or source detail rows.
- Controlled Case Workbench is allowed to satisfy the current bounded task when
  it returns an audited result. The follow-up recommendation to productize the
  custom口径 as a new semantic MCP tool is a release/backlog step, not a reason
  to block a safe current-case answer.
- When a required fact source is unavailable, stop report-grade claims. When an
  optional enrichment source is unavailable, continue with the strongest
  available evidence and label the gap.

## Auth And Runtime State

- The embedded Analytix agent may share only the common Codex GPT login/auth state needed to call the model.
- Auth sharing is read-only from the plugin's perspective: do not read, copy, mutate, refresh, export, or persist provider token files, OAuth browser state, account profiles, or model-provider config.
- Plugin files, MCP config, generated artifacts, evidence ledgers, eval outputs, local marketplaces, and runtime caches must stay under the Analytix-owned runtime home, not the system Codex home.
- Runtime-cache sync is a local verification aid only. It must refuse non-Analytix runtime homes, require an existing same-version plugin cache plus runtime skill mount, require explicit local-remount confirmation for writes, and must not be used as the Hub publication or customer installation path.
- Do not copy, print, export, or persist auth tokens in plugin artifacts, doctor output, eval output, or report text.
- Doctor, health, and eval harnesses may report an auth/runtime failure, but they must redact Bearer tokens, API keys, JWTs, refresh tokens, and session tokens before writing JSON output or logs.
- Login remediation belongs to the Analytix model-provider UI. The fund-analysis plugin must not implement alternate login, token repair, token sync, or provider-config migration paths.
- Publish, install, uninstall, rollback, and remount must follow `hub-lifecycle.md`.
- Plugin updates must flow through Analytix Hub/admin-console or a bundled
  Analytix release. The plugin must not self-upgrade, mutate backend schemas, or
  create MCP tools at runtime.

## Sub-Agent Boundary

Sub-agents are an execution strategy, not a data-access path. They may be used only after a shared scope has been resolved through MCP tools.

Use sub-agents for:

- full-case reports with multiple holders/accounts and multiple independent topics;
- concurrent specialist review of patterns, cash, payment, virtual assets, paths, assets/debts, control clues, fact cards, and QA;
- quality review after a draft report.

Do not use sub-agents for:

- independent fact-source creation outside the shared source envelope;
- raw row dumping;
- single-account quick statistics;
- filling gaps where backend tools are unavailable;
- legal/tax conclusion making.

Every sub-agent output must be a bounded card with route, scope, tool calls, figures, warnings, confidence, and the deterministic fact/edge fields needed to verify the claim. The main agent remains responsible for final integration and report wording.

## Provenance

For report-grade claims, keep internal provenance in the backend/tool layer, but user-facing answers should cite concrete fact fields rather than opaque ids:

- `case_id`
- parameters and warnings
- transaction id/time/amount/account/counterparty fields when the claim is a flow edge

If provenance is missing, label the statement as a preliminary feature or lead.
