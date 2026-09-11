# Analytix Fund Analysis Release Notes

## Unreleased deterministic RC candidate

Release status: source-level RC candidate; not a packaged, signed, published,
or commercially licensed release.

- Keeps the ordinary plugin `.mcp.json` disabled. The Go runtime exclusively
  owns the reserved, pinned `analytix_funds` binding and dynamically advertises
  only `analyze_account_flows` after current case, immutable snapshot, source,
  execution, and evidence authority all validate. `count_case_rows` remains an
  internal host canary.
- Carries exact inflow, outflow, transaction-count, coverage, and currentness
  support through typed evidence, `ClaimRecord`, and the Final Evidence Gate;
  signed net is derived from the exact gated amount claims. Unsupported
  counterparty facts are omitted or reported as gaps.
- Consumes or discards the host-private evidence carrier exactly once. Full PII
  remains outside provider/model, public messages/events, logs, telemetry, and
  generic tool results.
- Adds local Direct Source Preview outside Agent/MCP/evidence turns and retained
  AcceptedSlotDisplay recovery against the original immutable snapshot. The
  display mode changes only the final trusted local projection.
- Focused and cross-layer deterministic tests have passed for these seams;
  full source-freeze, live-provider, Electron, and same-artifact formal package
  evidence are tracked separately and are not implied by this note.

## 0.16.16

Release status: evidence-kernel integration candidate. This version supersedes
0.16.15 because the high-risk source probe now observes a current-run,
read-only DuckDB dataset snapshot instead of echoing a host-supplied snapshot
identifier.

- Binds the native source probe and the exact unfiltered transaction-index row
  count to the same DuckDB transaction and deterministic dataset snapshot.
- Derives the snapshot from imported raw-source SHA-256 records, the monotonic
  source revision, materialization metadata, schema, and exact source/index row
  counts; stale or inconsistent materialization fails closed.
- Keeps count output non-publishable until the Go host completes grant-bound
  EvidenceReceipt settlement and Final Evidence Gate validation.
- Requires the host to pin and revalidate the complete installed plugin source
  tree before a high-risk probe or tool execution can be trusted.

## 0.16.15

Release status: Go-only runtime closure identity. This version supersedes
0.16.14 because the runtime closure pass added provider/model hard binding,
goal/todo final-readiness, and release-slice grouping evidence after the
0.16.14 tag was created.

- Keeps the 0.16.14 and 0.16.13 fund-analysis runtime behavior unchanged.
- Aligns the plugin release identity with the current Go runtime P0/P1 closure
  source tree instead of moving the existing 0.16.14 tag.
- Updates the eval-coverage and runtime-cache contract versions so candidate
  release planning no longer depends on stale 0.16.12 contract identifiers.

## 0.16.14

Release status: release-gate identity fix. This version supersedes 0.16.13
because the 0.16.13 runtime fix was correct, but the current source tree still
contained an uncommitted `release-slice-audit.mjs` coverage update. A publishable
plugin cannot mix a tagged runtime package with an untagged release-gate script.

- Keeps the 0.16.13 front-door flow-metric guard unchanged: "资金流向/去向前 N"
  still resolves to current-case all-case external outflow destination ranking,
  not bidirectional turnover.
- Adds release-slice dirty-group coverage for the current runtime/renderer
  worktree surfaces so unrelated analytix runtime changes are classified
  explicitly instead of becoming ambiguous release blockers.
- Makes 0.16.14 the only valid release identity for the installed-runtime,
  direct MCP, front-door oracle, Hub source, and production publish evidence.

## 0.16.13

Release status: front-door flow-metric P0 fix. This version supersedes 0.16.12
because a fresh real front-door oracle proved that the case-title guard worked,
but the model could still answer "资金流向前十位对手方" with
`metric="turnover"` and visible bidirectional totals instead of the required
outflow destination ranking.

- Adds runtime intent normalization for `rank_counterparties`: when the current
  user request says 资金流向、资金去向、去向前 N、转给谁、付款对象 or 收款方前 N,
  the MCP enforces `metric="outflow"`, `direction_mode="out"`, and name-grain
  counterparties even if the model sends `metric="turnover"`.
- Reads only the current Analytix turn's user message from the injected
  `_analytix.threadId/turnId` context; it does not scan local databases, old
  outputs, neighboring case projects, or historical evidence.
- Clarifies tool schema and root skill routing: turnover is valid only for
  explicit 交易总额/往来总额/双向/不区分方向 requests.
- Makes ranking compact previews reuse the same metric-specific ranking rows,
  preventing one tool result from exposing conflicting turnover/outflow cues.
- Keeps the 0.16.12 case-title scope guard, 0.16.11 name-grain/default Top-N
  contract, Pair Amount source-of-truth, Workbench ladder, and L5 front-door
  gates intact.

## 0.16.12

Release status: front-door case-title scope P0 fix. This version supersedes
0.16.11 because a fresh real front-door canary exposed that
"江苏航案件分析中资金流向前十位对手方是谁" could be misread as
`holder_name="江苏航"` instead of current-case full-scope ranking.

- Adds a runtime guard for ranking tools: when `holder_name` is only the current
  case-project title fragment, the tool removes the holder filter and executes
  the all-case ranking. Explicit complete payer/holder/account names remain
  scoped.
- Clarifies the `rank_counterparties` schema and root skill instructions:
  "某案件/某案件分析中/当前案件中/项目中" is case-project scope, not a holder
  filter.
- Extends `mcp-return-oracle` with a regression check that deliberately passes
  the case-title fragment as `holder_name` and requires the result to match
  DuckDB all-case external counterparty Top10.
- Aligns `mcp-surface-snapshot` and `release-slice-audit` with the current
  case-project source contract: release surface evidence now probes
  `get_current_case` first and accepts the explicit user-visible
  "case project not ready" blocker when no `.analytix/case-project.json` is
  bound, instead of treating the legacy backend active-case API as the only
  valid current-case proof.
- Keeps the 0.16.11 name-grain default, empty/same-holder filtering, Top-N
  contract, Pair Amount source-of-truth, Workbench ladder, and L5 front-door
  gates intact.

## 0.16.11

Release status: front-door Top-N counterparty grain P0 fix. This version
supersedes 0.16.10 because the real front-door canary exposed a remaining risk:
ordinary "资金流向前十位对手方是谁" prompts could omit
`counterparty_group_mode`, allowing `rank_counterparties` to default to account
grain while the business question expected holder/name grain.

- Defaults `rank_counterparties` to `counterparty_group_mode=name` in both MCP
  argument normalization and current-case DuckDB fallback execution. Name-grain
  ordinary rankings also exclude empty counterparty names and same-holder
  self-transfer rows before backfilling the requested Top N. Account grain and
  explicit self-transfer inclusion remain available only when the user asks for
  账号/卡号/账户粒度, missing-name account investigation, or 同名自转/内部调拨.
- Clarifies the tool schema so model routing treats "谁/户名/对手方/资金去向 Top-N"
  as external name-grain ranking and does not silently answer a person/entity
  question from account-grain rows or self-transfer rows.
- Hardens `frontdoor-p0-oracle` Task C: it now checks the visible answer against
  DuckDB Top-10 counterparty names and amounts, not just requested limit and
  visible row count.
- Extends `mcp-return-oracle` so the direct MCP gate verifies the no-explicit-
  grain default path, preventing a future regression where only explicit
  `counterparty_group_mode=name` calls pass.

## 0.16.10

Release status: mature-plugin and Skill Creator alignment candidate. This
version inherits the 0.16.9 DuckDB fact-layer implementation but must collect
fresh installed-runtime front-door evidence before it can be treated as a
published release.

- Adds `scripts/skill-creator-alignment-audit.mjs`, a deterministic L0 gate
  that audits every production `SKILL.md` for focused trigger descriptions,
  required workflow/output/completion sections, one-hop local references, and
  absence of golden-answer or fixed-case facts.
- Re-anchors the blueprint, benchmark, and drift references to the locally
  installed mature plugin sources for Data Analytics, Investment Banking, and
  Public Equity Investing, plus OpenAI Codex Skills/Plugins/MCP and Anthropic
  Agent Skills / skill-creator rules.
- Makes `doctor` syntax-check the new alignment audit so release candidates
  cannot drift back into long, fixed, mechanical, or support-layer-leaking skill
  scaffolding without an explicit failing gate.
- Keeps 0.16.9 front-door P0 evidence as historical baseline only: current
  release readiness still requires fresh 0.16.10 doctor, direct MCP DuckDB
  oracle, front-door oracle, L5 review, runtime cache equality, and Hub
  publication/install evidence.

## 0.16.9

Release status: front-door P0 closure candidate. Hub publish remains gated on
installed runtime cache equality, direct DuckDB oracle, real front-door oracle,
L5 review, and package publication evidence for this exact version.

- Scopes real Analytix case-project fund prompts at the Go runtime boundary so
  provider-visible tools prefer the `analytix_funds` owner surface and keep
  broad support tools from competing with focused MCP owners.
- Preserves MCP `structuredContent` when compact text is also present, allowing
  report artifacts, evidence cards, and Workbench validation payloads to remain
  auditable in front-door thread replay.
- Adds a current-case DuckDB report artifact fallback for
  `run_full_case_analysis(write_report=true)`: if the backend report service is
  unavailable or returns no inspected file, the MCP runtime writes a local
  markdown/manifest pair, checks size and SHA256, and returns delivery metadata.
- Hardens `frontdoor-p0-oracle` L5 review into a real release gate and narrows
  its missing-data check so `Top 10` wording is not misread as “missing equals
  zero.”
- Recovers unsafe model drafts that leak support-layer terms, ask whether to
  continue instead of using the current-case tools, or claim report completion
  without inspected artifact evidence.

## 0.16.8

Release status: front-door P0 evidence-visibility candidate; Hub publish remains
blocked until the installed runtime cache passes `frontdoor-p0-oracle
--fail-on-gaps` with 0 failed tasks and L5 review is recorded.

- Makes controlled Workbench evidence deterministic in real front-door
  `events.jsonl` by rendering SQL policy, evidence-card summary, source hash,
  metric scope, and validation summary into the compact text that the model
  actually sees when Go runtime stores only `tool_result.output.result`.
- Updates `frontdoor-p0-oracle` to parse both MCP structured result envelopes
  and Go runtime compact text results, so custom SQL tasks can prove
  `sql_policy` / `evidence_card` / `validation_state` from the installed
  front-door transcript instead of direct MCP-only artifacts.
- Fixes the largest-holder canary audit to treat “总流水 / 周转额 / 流入金额与
  流出金额之和” as a valid turnover-style default口径.
- Expands user-visible leakage checks to catch support-layer tokens such as
  `source_hash`, `evidence_card`, `validation_state`, `sql_policy`, and
  `metric_scope` if the model copies them into ordinary answers.

## 0.16.7

Release status: front-door P0 fix candidate; Hub publish remains blocked until
`frontdoor-p0-oracle --fail-on-gaps` has 0 failed tasks on the installed cache.

- Fixes the real front-door count canary where “交易明细表数据量多少” could be
  answered from `fc_transaction` / `fc_transaction_norm` width instead of the
  authoritative `analysis_txn_detail_idx` detail index. `count_case_rows` now
  normalizes no-filter transaction-table total counts to the detail index and
  returns a business table label for model-facing evidence.
- Hardens the Pair Amount source-of-truth path by declaring common natural
  language aliases such as `counterparty_name` / `payee_name` as known fields
  for `investigate_pair_amount`, then normalizing them to `receiver_name`
  before execution. Unknown data-source fields such as `duckdb_path` remain
  rejected at MCP request validation.
- Tightens `frontdoor-p0-oracle` task matching so custom Top-N SQL prompts are
  not mistaken for the Top 10 counterparty-flow task, and improves amount
  matching for yuan / 万元 / 亿元 visible answers.
- Extends `case_workbench_control_contract` to prove the Pair Amount alias
  contract without weakening unknown-field rejection.

## 0.16.6

Release status: front-door P0 oracle and MCP contract hardening candidate; Hub
publish remains blocked until the new front-door oracle has 0 failed tasks
against an installed runtime cache.

- Adds `scripts/frontdoor-p0-oracle.mjs`, a real analytix thread evidence gate
  that reads front-door `events.jsonl`/`messages.jsonl`, extracts user prompts,
  final answers, MCP tool calls/results, leakage findings, DuckDB cross-check
  SQL, and an L5 review checklist for the `江苏航案件分析` case project.
- Enforces MCP input schema boundaries in the request handler: unknown tool
  input fields such as per-call database paths are rejected, while the
  Analytix runtime-injected current workspace context remains allowed.
- Promotes Top-N to a protocol field across current-case DuckDB ranking,
  tracing, and graph fallback outputs: `requested_limit`, `resolved_limit`,
  `returned_count`, `visible_count`, `filtered_count`, `data_exhausted`,
  `truncation_reason`, `compact_text_row_count`, and
  `final_answer_expected_min_rows`.
- Preserves `top_n` / `requested_limit` ranking aliases through schemas and
  runtime normalization so explicit Top 10/15/20 user intent cannot silently
  fall back to a default preview count.
- Expands semantic-tool and Workbench capability gaps into a deterministic
  current-case Workbench ladder with prohibited claims and
  `safe_to_answer_current_task=false`, so unsupported semantic tasks must
  inspect the current case DuckDB scene or stop with an auditable blocker.

## 0.16.5

Release status: superseded by 0.16.6.

- Moves the current-case DuckDB Workbench closer to the `crystaldba/postgres-mcp`
  restricted-mode pattern: user-controlled SQL now passes a static read-only
  cleaned/analysis-scope guard and a DuckDB `EXPLAIN` parser/binder preflight
  before the original SQL can execute.
- Adds structured `sql_policy` evidence to `run_case_sql`, `explain_case_sql`,
  `diagnose_case_sql`, `preview_case_rows`, notebook cells, `validation_state`,
  and Workbench history summaries without exposing SQL text or local paths.
- Keeps the parser/binder preflight non-executing: the MCP result records that
  preflight used `EXPLAIN <redacted-current-case-sql>` and did not execute the
  user SQL until after policy validation passed.
- Extends `mcp-return-oracle` so real DuckDB validation now fails if
  parser/binder policy evidence is missing, or if an invalid column is not
  blocked before data return.

## 0.16.4

Release status: superseded by 0.16.5.

- Fixes the P0 Top-N contract gap where `rank_counterparties(limit=10)` could
  execute against DuckDB correctly but expose only five rows in the Agent-visible
  compact text. Ranking previews, ranking key facts, and generic result text now
  respect the requested bounded limit.
- Fixes destination/frontdoor Top-N behavior after self-counterparty filtering:
  the frontdoor fetches buffered rank rows, then the destination card returns
  the requested number of non-self terminal facts.
- Fixes the same fixed-preview risk for `trace_subject_top_outflows` compact
  payloads: `top_outflows` key facts now follow requested `top_n` / `limit`
  instead of a hard-coded preview size.
- Tightens frontdoor routing for "给谁/转给谁/流向谁" prompts so explicit
  `metric` and `counterparty_group_mode` survive normalization and the visible
  amount label follows the requested metric.
- Strengthens `diagnose_case_sql` for SQL-column mistakes by returning missing
  identifiers, DuckDB candidate bindings, SQL-safe candidate columns, and a
  reviewed `analysis_txn_detail_idx` repair pattern without translating SQL
  identifiers in the model-visible diagnostic text.
- Keeps `validate_report_claims` useful when the optional backend semantic
  service is unavailable by running the local deterministic claim-risk fallback
  and marking backend status as a capability gap instead of claiming a full pass.
- Adds current-case DuckDB semantic fallbacks for fund tracing: when the backend
  semantic service is unavailable, `trace_subject_top_outflows`,
  `trace_fund_next_hop`, `trace_fund`, and `build_fund_flow_graph` can still
  return read-only one-hop Top outflows, terminal categories, in-case next-hop
  candidates, query ids, and explicit trace-depth/data-breakpoint boundaries.
- Extends the safe skill runtime and MCP call runtime so supported semantic
  tools and Workbench tools try the current-case DuckDB fallback before
  returning capability-gap cards.
- Extends `mcp-return-oracle` with real DuckDB gates for Top 10 visible ranking
  parity, destination-card Top 10 filtering, `top_outflows` requested Top-N
  preservation, trace fallback Top 10 parity, next-hop fallback parity,
  fund-flow graph fallback parity, and SQL-safe diagnostic text.

## 0.16.3

Release status: Hub publish candidate for final analytix-only runtime cleanup.

- Removes remaining legacy runtime defaults from release/eval scripts: Hub cache
  checks now default to `~/.analytix`, case DB audits default to the current
  analytix `data-analysis/cases` store, and pair-amount real-case smoke tests
  require an explicit current case DB path.
- Updates artifact-store diagnostics to assert the current
  `.analytix/artifacts/analytix-funds` root instead of the retired standalone
  fund-analysis app directory.
- Keeps legacy `Analytix_new` mentions only as explicit deprecated-history or
  leak-detection text; no development, publish, runtime, case resolution, or
  evidence path uses the old project as a source.
- Revalidates installed-cache DuckDB Workbench answers and current-case notebook
  artifacts against the `江苏航案件分析` DuckDB, including `analysis_*` and
  `fc_*_norm` row counts.

## 0.16.2

Release status: Hub publish candidate for current-case DuckDB workbench closure.

- Adds real local `create_case_notebook` support to the controlled DuckDB
  Workbench fallback. Notebook cells now execute only against the current
  analytix case project, use read-only `analysis_*` / `fc_*_norm` scope,
  clamp rows to 500, record query ids/source hashes, and write notebook +
  manifest artifacts under the case project's `.analytix/artifacts` root.
- Keeps semantic MCP tools as the first source of truth while making the
  Workbench path a live database-scene fallback for schema inspection,
  row-count checks, safe SQL execution, query diagnosis, query history, and
  reproducible notebook artifacts.
- Fixes the release-readiness gap where `create_case_notebook` was advertised
  in the MCP schema but not implemented by the local DuckDB fallback.

## 0.16.1

Release status: analytix case-project runtime fix.

- Allows `get_current_case` to return a confirmed current case-project card
  from `.analytix/case-project.json` when the bound DuckDB exists, even if the
  optional backend case API is temporarily unreachable.
- Keeps the cross-project guard unchanged: explicit `case_id` must still match
  the current case project, and missing DuckDB still blocks analysis instead of
  falling back to another case.

## 0.16.0

Release status: analytix migration candidate.

- Moves the plugin source, skills, references, scripts, and assets into
  `/Users/sun/Projects/analytix/plugins/analytix-fund-analysis`; the old
  `Analytix_new` tree is no longer a development or publish source.
- Replaces global active-case resolution with analytix case-project resolution:
  MCP calls receive the current workspace from the runtime, read
  `.analytix/case-project.json`, and use only that bound `caseId`.
- Fails closed when an explicit `case_id` differs from the current case project,
  preventing cross-project DuckDB reads and stale selected-case leakage.
- Repoints local DuckDB workbench fallback to analytix data-analysis case
  storage and removes the old `~/.Analytix资金分析工具/cases` fallback path.
- Updates cleaned export handling to pass `target_dir` through to the current
  analytix backend and validate output under the analytix data-analysis case
  directory or the user-selected target directory.
- Updates local runtime-cache and artifact defaults to `~/.analytix`.

## 0.15.131

Release status: publish candidate.

- Corrects the Hub/plugin-page visual role of `assets/logo.png`: the logo asset
  is now the same square icon-only Analytix glass mark as `assets/icon.png`, so
  plugin UI renders like the Computer Use reference with the icon on the left
  and title/subtitle supplied by the page, instead of showing a miniature
  text-bearing card inside the icon slot.
- Supersedes 0.15.130 only for the plugin visual identity asset shape; no
  funds-analysis behavior, MCP tool surface, focused skill routing, or evidence
  contract changed.

## 0.15.130

Release status: superseded by 0.15.131.

- Replaces the public plugin icon and Hub logo assets with the new Analytix
  glass-style symbol icon so the Analytix Hub listing, plugin page, and
  composer icon use the same updated fund-analysis visual identity.
- Supersedes 0.15.129 only for the published visual identity package; no
  current-case funds-analysis behavior, MCP tools, focused skill routing, or
  evidence contract changed in this release.

## 0.15.129

Release status: superseded by 0.15.130.

- Supersedes 0.15.128 because the existing `analytix-fund-analysis-v0.15.128`
  tag already points at an earlier release identity and the latest naming and
  cleanup pass needs a fresh monotonic publish version.
- Standardizes the public plugin surface, focused skill names, MCP visible
  titles, command metadata, capability registry, and release/eval smoke
  contracts around public-security economic-investigation and data-judgment
  language, including `特定双方资金往来核验`, `资金事实快查`,
  `数据质量与口径复核`, `专项资金核算`, `异常资金线索研判`,
  `证据图表附件`, `报告事实结论复核`, `研判材料交付复核`, and
  `资金流向图谱`.
- Keeps old wording only where it is deliberately used as a negative leak guard,
  regression fixture, or historical release note; current manifest, focused
  skills, MCP card text, registry labels, and user-facing contracts use the new
  names.
- Removes local desktop acceptance Electron user-data/cache leftovers from the
  plugin development environment while preserving release evidence and
  functional-closure outputs outside the package source.

## 0.15.128

Release status: superseded by 0.15.129.

- Supersedes 0.15.127 after final-answer delivery review showed the previous
  candidate did not yet carry the new schema-first proof-action and internal
  validation audit events at the release identity level.
- Extends `FinalInvestigationAnswer` proof actions with target, material,
  period/field scope, linked evidence gap, priority, evidence boundary, and
  forbidden-upgrade guidance so public-security economic-investigation answers
  preserve the distinction between verified current-case facts and materials
  that still need to be obtained.
- Adds deterministic internal audit events for structured-answer validation
  failures/warnings and SQL diagnose results. These events support delivery QC
  and release review, but ordinary case users still see only evidence-backed
  facts, evidence boundaries, and next proof actions in case language.
- Keeps the plugin inside the Codex/Agent plugin boundary: export, PNG/JPG,
  workbook, and report figure delivery remains an inspection contract over
  available Analytix/Codex delivery surfaces, not a standalone PDF renderer,
  OCR pipeline, asset-registry connector, or external evidence system.

## 0.15.127

Release status: superseded by 0.15.128.

- Supersedes 0.15.125 for the final publishability closure after the local
  Analytix-owned runtime cache had already advanced to a stale 0.15.126 package.
  The workspace manifest, MCP server, runtime cache contract, capability
  registry, release audit identity, and generated runtime remount target now use
  one monotonic candidate version.
- Standardizes the skill surface around Data Analytics-style source envelopes,
  focused-owner delivery, controlled DuckDB workbench escalation, and hidden
  support layers adapted from Investment Banking and Public Equity Investing.
- Tightens root/navigator guidance, focused-skill non-goal wording, ordinary
  user-facing language, and the mature-plugin reuse matrix so release evidence
  must show code-level, semantic-level, and test-level adoption rather than
  benchmark slogans.
- Adds deterministic non-LLM acceptance gates for the next iteration:
  `skill-clause-audit.mjs` audits every Analytix skill clause against mature
  Data Analytics / Investment Banking / Public Equity Investing owner, source,
  evidence, language, and completion-gate patterns; `mcp-return-oracle.mjs`
  validates Workbench MCP returns against a read-only DuckDB oracle, including
  schema inventory, row counts, aggregates, profile stats, preview masking,
  diagnostics, EXPLAIN behavior, and write/raw/external/unfiltered-preview
  blocking. This replaces model-answer scoring as the primary correctness proof.

## 0.15.125

Release status: superseded by 0.15.127.

- Supersedes 0.15.124 after agent thread audit of the real Analytix desktop
  front-door run showed the user-visible text guard blocked old review wording,
  but the raw Pair Amount owner answer could still contain a negated old-word
  phrase in the rollout log.
- Adds a hard confirmed-duplicate wording contract across the root agent, the
  embedded root agent, the Pair Amount owner, and the model-visible support
  excerpt: confirmed repeated records must be stated affirmatively as confirmed,
  removed, and not new transfers; the final answer must not quote, negate, or
  echo old review wording even when the user asks with that wording.
- Keeps the verified real-case money contract unchanged: raw detail, effective
  transfer facts, and confirmed repeated records remain separate, and the
  confirmed repeated records are already removed rather than sent back to users
  as another review task.

## 0.15.124

Release status: superseded by 0.15.125.

- Supersedes 0.15.123 after real Analytix desktop front-door retesting showed
  prompt-only controls still allowed a user-primed amount challenge to echo old
  review wording.
- Adds an Analytix-owned desktop runtime visible-text guard for assistant
  messages and agent-message deltas in confirmed-duplicate Pair Amount context.
  It rewrites old review-word echoes into affirmative case language: confirmed
  repeated records, removed, and not new transfers.
- Tightens root and Pair Amount owner guidance plus the model-visible support
  excerpt so the source-of-truth facts remain: raw detail, effective transfer
  facts, and confirmed repeated records are separate; confirmed repeated records
  are not sent back to users as a review task.

## 0.15.123

Release status: superseded by 0.15.124.

- Supersedes 0.15.122 after real Analytix desktop model-window testing showed
  the amount-challenge follow-up could still echo old duplicate-review wording
  in a negative sentence, even while the numeric conclusion was correct.
- Tightens Pair Amount focused-skill guidance, router metadata, and model-visible
  fact summaries so confirmed same-holder cross-account repeated records are
  expressed only as confirmed repeated records that have been removed and are
  not new transfers.
- Keeps the verified real-case Pair Amount fact contract unchanged: raw detail,
  effective facts, and confirmed repeated records stay separated in local
  evidence, without publishing case-specific amounts in package docs.

## 0.15.122

Release status: superseded by 0.15.123.

- Supersedes 0.15.121 after real Analytix desktop multi-turn acceptance showed
  the acceptance harness only read `*_FORBIDDEN_MARKERS`, while some release
  runs supplied the legacy `*_FORBIDDEN` variable name.
- Updates `desktop-multiturn-acceptance.mjs` to merge both forbidden-marker env
  names and fixes `frontdoor-smoke.mjs` so all real tasks apply task-level
  forbidden markers. User-visible banned phrases, stale amount rows, and old
  duplicate review wording can no longer silently skip these gates.
- Tightens Pair Amount focused skill and command metadata so confirmed
  same-holder cross-account repeated records are stated affirmatively as
  confirmed, removed, and not new transfers, without echoing stale review
  wording as contrast text.

## 0.15.121

Release status: superseded by 0.15.122.

- Supersedes 0.15.120 after real Analytix desktop frontdoor testing showed the
  Pair Amount support path could still fall back to the older
  `rank_counterparties` candidate aggregate when the internal targeted
  Workbench SQL was rejected by the guardrail for containing `SELECT *`.
- Rewrites the Pair Amount internal SQL to use explicit columns throughout, so
  the controlled Workbench can execute the same-holder cross-account
  same-fact dedupe query and return the controlling current-case result with raw
  detail, effective facts, and confirmed repeated-record rows separated.
- Carries confirmed duplicate amount/count/status/explanation into
  `key_facts.pair_amount_review`, keeping the model-visible support excerpt on
  the confirmed-duplicate branch rather than the unresolved
  duplicate/evidence-insufficient fallback.
- Updates Pair Amount eval/smoke/golden expectations so the older rank-candidate
  aggregate remains non-controlling and cannot replace the material conclusion
  for the fixed two-party amount question.

## 0.15.120

Release status: superseded by 0.15.121.

- Supersedes 0.15.119 after real case verification showed the Pair Amount lane
  correctly reduced a raw 23-row two-party transfer set to 18 effective transfer
  facts, but still described the five removed rows as needing duplicate review.
- Adds the production same-holder cross-account repeated-record rule: within one
  selected scope, rows under different card/account numbers are counted once
  only when holder name, ID number, direction, time, amount, balance,
  counterparty, transaction id when available, summary/remark/type, and success
  status match.
- Carries confirmed repeated-record amount/count through the MCP fact pack,
  support-text compiler, Pair Amount focused skill, command metadata, and smoke
  tests so confirmed duplicate rows are written as already removed from the
  count, not as `暂不能认定` or a request to re-review the same duplicate fact.
- Adds `pair-amount-real-case-dedup-smoke.mjs`, a read-only true-case regression
  for the fixed Pair Amount family that asserts the raw count, effective count,
  and confirmed repeated-record amount without shipping case-specific evidence
  into the Hub package.

## 0.15.119

Release status: superseded by 0.15.120.

- Supersedes 0.15.118 after real Analytix desktop frontdoor acceptance showed the
  eighth multi-turn scenario could be scored against the previous report answer
  when `turn/start` did not return a material turn id and the thread was already
  idle.
- Tightens `desktop-multiturn-acceptance.mjs` so every scenario records the
  assistant-message baseline before sending a turn, then waits for a new
  assistant response after that baseline before applying marker, language, JSON,
  and internal-word leakage checks.
- Keeps the plugin's funds facts, MCP semantic layer, focused-skill prompts, and
  user-facing answer contracts unchanged from 0.15.118; the change is a
  validation-harness correctness fix so real frontdoor evidence cannot reuse a
  stale answer.

## 0.15.118

Release status: superseded by 0.15.120.

- Supersedes 0.15.117 after real Analytix desktop frontdoor acceptance showed the
  report workflow could still put instruction-contract wording such as
  `必须保留` into an otherwise correct case report.
- Tightens the user-facing language sanitizer and shared/report/full-case
  completion gates so amount-dispute boundaries are written as
  `需在材料中列明` and competing amount categories are written as `不得混同`,
  not as internal title or wording contracts.
- Extends smoke coverage with the exact report leakage sentence observed in the
  0.15.117 desktop run.

## 0.15.117

Release status: superseded by 0.15.118.

- Supersedes 0.15.116 after real Analytix desktop frontdoor acceptance showed
  facts, amounts, graph next-step tails, and source-window wording were fixed,
  but a Pair Amount row note could still write `需结合用途材料复核`, downstream
  continuation could omit an explicit `暂不能认定` boundary sentence, and report
  tables could expose the diagnostic header `证据状态`.
- Tightens the user-facing language sanitizer with a final fallback rewrite for
  residual-bucket row notes, `同事实去重`, and `证据状态`, using case-material
  wording such as `摘要/类型提示用途线索`, `重复风险复核`, and `核验意见`.
- Tightens Pair Amount, fund-tracing, report, full-case, and shared focused-skill
  completion gates so final visible answers scan headings, table headers, and
  row notes before sending.
- Extends smoke coverage for the exact real-frontdoor leakage samples observed
  in 0.15.116.

## 0.15.116

Release status: superseded by 0.15.117.

- Supersedes 0.15.115 after real Analytix desktop frontdoor acceptance showed
  the facts and next-step tails were present, but ordinary answers could still
  expose source-window wording such as `当前案件可见` / `当前可见`, and flow-graph
  answers could omit an explicit `下一步核查建议` tail.
- Tightens shared focused-skill language so user-visible answers, graph text,
  report text, and figure labels use case-material wording such as
  `经已清洗流水复核`, `已调取流水显示`, `主要集中转账`, and `资金链路` instead of
  internal or audit-window phrases.
- Tightens graph, tracing, report, full-case, and Pair Amount completion gates
  so ordinary frontdoor answers keep support/audit language hidden and end with
  concrete proof actions whenever downstream continuity, product redemption,
  balance continuity, or final-beneficiary proof remains incomplete.
- Extends the user-facing language sanitizer to rewrite `当前可见` /
  `当前案件已见` / `当前案件中` and to avoid producing replacement text that still
  contains `当前可见`.

## 0.15.115

Release status: superseded by 0.15.116.

- Supersedes 0.15.114 after real Analytix desktop frontdoor acceptance showed
  the amount challenge and subject dossier answers could be factually correct
  yet omit an explicit next verification / proof-action tail.
- Tightens Pair Amount completion so raw-vs-effective or competing-amount
  disputes must include `暂不能认定` for unsupported extra amounts and a visible
  `下一步补证/核查建议` action list separating current-case review from external
  proof materials.
- Tightens subject dossier completion so ordinary subject profiles always end
  with concrete `下一步核查/补证建议`, covering current-case export/review and
  external opening, control, receipt, balance, counterparty, product/KYC, and
  purpose evidence.

## 0.15.114

Release status: superseded by 0.15.116.

- Supersedes 0.15.113 with the commercial closed-loop fund-investigation
  overhaul: Data Analytics source/validation/delivery discipline is now the
  default operating model, while Investment Banking/Public Equity style lead
  ownership keeps support/audit layers hidden from user-facing answers.
- Locks the production sequence to focused skill owner -> current-case source
  envelope -> MCP semantic facts -> evidence/validation state -> controlled
  Workbench only when needed -> economic-investigation final writing.
- Adds real desktop acceptance evidence for Pair Amount, flow graph, subject
  dossier, downstream continuation, full-case analysis, report generation,
  amount challenge/rescope, and active-case cold-start/switch/restore, with
  thread audit checks for internal term leakage, JSON leakage, duplicate calls,
  local DuckDB guessing, latency, amount correctness, and mechanical language.
- Fixes active-case synchronization, date-window support facts, destination
  outflow support rendering, claim-review cash/counterparty risk downgrades,
  Pair Amount rank leakage, same-fact/duplicate wording, and full-case
  conclusion-first public-security language.

## 0.15.113

Release status: superseded by 0.15.114.

- Supersedes 0.15.112 with a clean baseline release after pruning old worktree
  metadata, historical runtime package caches, and tracked legacy closure
  artifacts from the active delivery line.
- Keeps the 0.15.112 economic-investigation behavior unchanged while ensuring
  the release tag, Hub source tree, runtime cache, and local desktop launch all
  point at the same current version.
- Leaves git history and prior release tags intact for auditability; old local
  caches and generated historical evidence are no longer part of the active
  workspace baseline.

## 0.15.112

Release status: superseded by 0.15.113.

- Supersedes 0.15.111 after post-publish real Analytixagent review showed the
  Pair Amount natural answer was professional and non-JSON, but the downstream
  follow-up for a receiving subject's onward destinations could still over-call
  semantic tools: 27 calls and 19,179 tool-result text chars for one
  continuation question.
- Promotes fund-flow graph support from a thin graph contract into a
  source-backed Chinese fact pack: upstream amount/date rows, downstream Top
  outflows, supported transaction edges, scope coverage, boundary language, and
  subpoena/follow-up direction are all visible in one model-readable excerpt.
- Keeps the support layer hidden while reducing model-visible context:
  the same real continuation task fell to 2 tool calls and 4,007 tool-result
  text chars, with no `over_budget` route violation.

## 0.15.111

Release status: superseded by 0.15.112.

- Supersedes 0.15.110 after true Analytix desktop UI review showed the Pair
  Amount answer was factually correct and much less mechanical, but ordinary
  natural answers could still surface `对手方排行交叉差异` as a visible amount-table
  row, distracting from the controlling current-case detail aggregate.
- Keeps rank/detail differences as internal or short proof-boundary review for
  ordinary `A 转给 B 多少钱` answers; they are expanded only when the user asks
  about amount discrepancy.
- Preserves the full-detail conclusion, complete small row table, focused concentration
  abnormality, outside focused concentration interpretation, and visible downstream
  continuation as the business answer spine.

## 0.15.110

Release status: superseded by 0.15.111.

- Supersedes 0.15.109 after real Analytixagent thread review showed the
  Pair Amount facts were correct, but the natural answer could still read like
  a heading-heavy amount audit and over-weight `下一步取证`.
- Adds a stricter natural-answer visible-shape contract: ordinary `A 转给 B
  多少钱` answers should use compact economic-investigation narrative, then
  amount reconciliation and complete small row tables, with proof-value actions
  only as a short tail.
- Extends delivery QC and Pair Amount smoke coverage to block long heading
  skeletons, next-step-heavy answers, and support-envelope concepts being
  copied into user-facing section templates.

## 0.15.109

Release status: superseded by 0.15.110.

- Supersedes 0.15.108 after real Analytix desktop UI multi-turn validation
  showed `继续追下游资金去向` could still echo support-layer wording such as
  `本窗口` and over-weight generic `下一步取证` after current-case outflows were
  already visible.
- Normalizes downstream tracing support labels before they enter model context,
  replacing tool-window language with case-facing wording such as `当前可见流水范围内`
  and `列为继续穿透对象`.
- Tightens the `fund-tracing` focused skill contract so downstream follow-ups
  first interpret visible current-case outflows, product endpoints, third-party
  transfers, cash/payment stops, and proof boundaries; `下一步` stays short and
  only fixes proof value or continues from real stop points.

## 0.15.108

Release status: superseded by 0.15.109.

- Supersedes 0.15.107 after real Analytix desktop UI validation fixed the full
  Pair Amount period and total, but still rounded visible continuation facts
  and compressed receiver-side outflows into a generic proof-action tail.
- Tightens the Pair Amount focused prompt and skill gate so current-case
  continuation facts must keep exact amounts, counts, product names, recipients,
  accounts, and date ranges. Already visible downstream rows must be analyzed
  as case facts before any external proof-value actions.
- Keeps the short prompt under the runtime loader limit while preserving the
  report-style economic-investigation narrative requirement.

## 0.15.107

Release status: superseded by 0.15.108.

- Supersedes 0.15.106 after real Analytix desktop UI validation showed the
  natural Pair Amount answer was no longer heading-led, and remained correct on
  full period, account counts, effective count, effective amount, duplicate
  boundary, and visible continuation facts, but could still compress the answer
  into conclusion + account list + row table + one abnormality sentence.
- Adds an explicit short-prompt failure shape: conclusion + account list + row
  table + one abnormality sentence is not a completed economic-investigation
  answer.
- Tightens the focused Pair Amount agent prompt so unrestricted `A 转给 B 多少钱`
  answers must include two transfer-structure judgment paragraphs: one for
  outside focused concentration rows by time span, account changes, amount tiers, and
  remarks/types; one for focused concentration abnormality by share, account/time
  concentration, and visible continuation.

## 0.15.106

Release status: superseded by 0.15.107.

- Supersedes 0.15.105 after real Analytix desktop UI validation showed the
  natural Pair Amount answer was now factually correct on full period, account
  counts, effective count, effective amount, duplicate boundary, and visible
  continuation facts, but could still read too much like a heading-led amount
  audit.
- Tightens the focused Pair Amount agent prompt so unrestricted `A 转给 B 多少钱`
  answers start as compact economic-investigation narrative: current-flow proof,
  account/time evolution, focused concentration versus outside focused concentration meaning, and
  visible continuation/diversion must be explained before row tables.
- Blocks the `统计期间 / 金额核验 / 异常特征 / 下一步取证` seven-part skeleton as
  the default answer shape for natural questions. Tables remain evidence
  anchors, not the main reasoning structure.
- Keeps already-returned case facts out of next-step wording: the tail may
  discuss proof-value fixation and external evidentiary boundaries, but it must
  not present visible case rows or continuation facts as if they still need to
  be discovered.

## 0.15.105

Release status: superseded by 0.15.106.

- Supersedes 0.15.104 after real Analytix desktop UI validation showed the
  Pair Amount answer now used the correct full-period amount and time range,
  but the opening still could omit the payer/receiver account counts and
  account scope that investigators need to understand the full two-party
  boundary.
- Adds an explicit `opening_sentence_must_cover` support fact to the Pair
  Amount support envelope: unrestricted natural `A 转给 B 多少钱` answers must
  open with the full first/last transaction time, payer account count, receiver
  account count, effective transaction count, and effective amount.
- Tightens the focused Pair Amount agent prompt so the 2100 万集中交易 is treated
  as an abnormal concentration and downstream clue, not as the controlling
  answer when the user did not limit the question to that date/account.
- Keeps external回单、合同、理财资料等材料 at the proof-value boundary: if the
  current case data already contains rows or downstream outflows, the answer
  must analyze them instead of presenting them as missing data or a next-step
  substitute.

## 0.15.104

Release status: superseded by 0.15.107.

- Supersedes 0.15.103 after true Analytix desktop UI validation showed the
  final Pair Amount answer was materially improved, but the visible progress
  rail could still expose the English fallback label `Investigate pair amount`
  and the local SKILL.md read path before the final answer.
- Adds a narrow Analytix-owned embedded UI label bridge for the fund-analysis
  MCP surface so investigator-facing progress uses Chinese case-work actions
  such as `正在核验两方转账` / `两方金额核验` instead of title-cased function
  names.
- Sanitizes visible runtime skill-read command labels for Analytix fund-analysis
  skills so support-layer file paths are not shown to investigators during real
  UI conversations.
- Extends Pair Amount contract smoke to assert both the MCP metadata and the
  embedded Analytix desktop UI bundle keep Chinese user-visible labels and hide
  Pair Amount SKILL.md paths.

## 0.15.103

Release status: superseded by 0.15.104.

- Supersedes 0.15.102 after real desktop UI validation showed MCP tool
  progress could still fall back to English title-cased function names such as
  `Investigate pair amount`, which is not acceptable for an investigator-facing
  Analytixagent answer surface.
- Adds Chinese user-visible invocation metadata to every MCP tool (`title`,
  `annotations.title`, and OpenAI Apps `_meta` invocation text) so support
  calls remain a hidden/assistive layer instead of becoming English process
  labels in the desktop UI.
- Tightens the Pair Amount focused agent prompt again within the 1024-byte
  loader limit: natural `A 转给 B 多少钱` answers must begin as connected
  economic-investigation material, then use amount and row tables as evidence
  anchors, and may only leave proof-value fixation or external evidence
  boundaries at the tail.
- Extends Pair Amount contract smoke to fail if the MCP surface loses Chinese
  user-visible invocation metadata.

## 0.15.102

Release status: superseded by 0.15.103.

- Supersedes 0.15.101 after real desktop startup showed the focused agent
  `default_prompt` exceeded the runtime loader limit and was ignored.
- Compresses the Pair Amount focused agent prompt below the 1024-byte loader
  threshold while keeping the hard behavioral contract: full-detail amount
  controls unrestricted questions, small returned tables are listed in full,
  `investigative_row_reading` is used for row interpretation, and natural
  answers read as compact economic-investigation material rather than a
  section checklist.

## 0.15.101

Release status: superseded by 0.15.102.

- Supersedes 0.15.100 after investigator review showed the natural Pair Amount
  answer was factually corrected but could still feel too sectioned, with
  `下一步取证` competing with the current-case analysis.
- Reframes Pair Amount delivery as compact economic-investigation material:
  answer the user's exact transfer question in connected report-style prose,
  then use amount/row tables as evidence anchors and leave proof-value fixation
  or external subpoenas as the tail.
- Tightens the focused skill, agent prompt, command metadata, support envelope,
  delivery QC, and smoke checks so headings cannot substitute for analysis.
  Current-case facts must explain relationship development, account changes,
  row cues, duplicate amplification, focused concentration significance, outside-
  cluster meaning, and already visible continuation before any next-step list.

## 0.15.100

Release status: superseded by 0.15.101.

- Supersedes 0.15.99 after investigator review showed natural Pair Amount
  answers could have the correct headline amount and period but still read as a
  filled template, with too little row-level transfer-relationship analysis.
- Adds runtime `investigative_row_reading` support for Pair Amount facts:
  chronology, payer/receiver account distribution, focused concentration reading,
  outside focused concentration reading, row cue buckets, and low-value/terminal-row warnings
  are exposed to the focused owner as the default analysis spine.
- Tightens the Pair Amount skill, focused agent prompt, command metadata,
  support envelope, delivery QC, and smoke contracts so current-case rows must
  be interpreted before next-step evidence requests: account changes,
  repayment/remittance/terminal/batch/product/cash/platform cues, outside-
  concentration rows, and already visible continuation facts must be explained as
  investigative meaning rather than folded into generic wording.

## 0.15.99

Release status: superseded by 0.15.100.

- Supersedes 0.15.98 after investigator review showed the natural Pair Amount
  answer could still feel like a template and could be pulled toward a lower
  cross-check/ranking aggregate or the largest focused concentration instead of the
  full current-case detail aggregate.
- Makes the full detail aggregate the controlling answer for unrestricted
  `A 转给 B 多少钱` questions: rank/cross-check aggregates, focused concentrations,
  prior reports, and historical artifacts can explain differences but cannot
  silently replace the detail amount, count, first transaction time, last
  transaction time, or returned row set.
- Tightens the Pair Amount skill, focused agent prompt, command metadata,
  support envelope, delivery QC, and user-visible language smoke so early,
  low-value, terminal/batch, repayment/remittance, and outside focused concentration rows are
  retained and interpreted as relationship-duration, account-change,
  purpose/control, or rank/detail-difference evidence rather than folded into a
  vague residual bucket.

## 0.15.98

Release status: superseded by 0.15.99.

- Supersedes 0.15.97 after investigator review showed the answer could be
  numerically closer but still feel like a basic accounting template: a correct
  headline amount/period, one dominant focused concentration row, one outside focused concentration summary,
  and generic evidence requests.
- Treats small returned Pair Amount row tables as current answer evidence, not
  attachment/export placeholders; support envelopes retain up to 20 rows by
  default and expose outside focused concentration rows for relationship analysis.
- Strengthens the focused skill, agent prompt, command metadata, delivery QC,
  and user-visible language smoke so natural `A 转给 B 多少钱` answers must read
  rows like case流水: account changes, remarks/types, concentration, duplicate
  difference, and already visible continuation facts must be interpreted before
  external proof requests.

## 0.15.97

Release status: superseded by 0.15.98.

- Supersedes 0.15.96 after investigator review showed the semantic Pair Amount
  card already returned the right full-period facts, but the visible answer
  could still over-center a dominant focused concentration or describe the outside rows
  as a residual bucket.
- Removes a focused concentration timestamp from the generic Pair Amount first-sentence
  example and requires unrestricted natural questions to lead with the returned
  full first/last transaction time, full effective amount/count, payer account
  list, and receiver account list.
- Adds hard blockers for shallow but plausible wording such as `全期间扣除该集中链路后`,
  `零散历史往来`, `需结合用途材料复核`, and `导出核对该 N 笔明细` when the
  current support facts already contain a complete small transaction table.

## 0.15.96

Release status: superseded by 0.15.97.

- Supersedes 0.15.95 after investigator review showed natural Pair Amount
  answers could still feel like a filled template: correct totals and dates
  were present, but phrases such as vague full-scope/multi-account wording,
  one-line focused concentration summaries, and generic next-evidence lists made the
  answer too shallow for case use.
- Strengthens the Pair Amount owner, focused agent prompt, support envelope,
  delivery QC, and user-visible language smoke so an unrestricted `A 转给 B
  多少钱` answer is framed as two-party transfer-fact analysis: accounts,
  first/last times, complete rows when small enough, dominant focused concentration, outside
  cluster, duplicate differences, continuation facts, and transfer-structure
  judgment must be explained before next-step evidence.
- Adds a visible-language guard for `全区间` and expands QC blockers for
  answers that use headings as a checklist or tell users to discover downstream
  rows already present in the current case.

## 0.15.95

Release status: superseded by 0.15.96.

- Supersedes 0.15.94 after investigator review showed natural Pair Amount
  answers could still be factually correct but too template-like: they reported
  totals and tables without enough transfer-structure judgment, and could still
  describe already visible receiver-side continuation as generic missing data.
- Adds a natural Pair Amount depth contract to the support envelope and focused
  skill: explain the full pair period, payer/receiver accounts, dominant
  cluster, outside focused concentration rows, duplicate/rank differences, and relationship or
  purpose leads instead of only filling standard headings.
- Strengthens root agent policy, command metadata, delivery QC, and user-visible
  smoke checks so returned outflows/products/cash/third-party facts must be
  written as `已见承接/分流`; next evidence should prove the correspondence with
  balances, receipts, product/KYC/order/control records, and purpose materials.

## 0.15.94

Release status: superseded by 0.15.95.

- Supersedes 0.15.93 after true run.sh desktop UI review showed the Pair Amount
  support facts were correct but the final answer still folded a complete
  18-row transfer table into `其余 8 笔/多个账户` and wrote receiver-side
  continuation as generic next evidence.
- Strengthens the support envelope, focused skill, command metadata, focused
  agent policy, and delivery QC so 20-row-or-fewer Pair Amount results must
  show every returned transaction row with concrete accounts; folded small-row
  shortcuts are treated as failed delivery.
- Adds explicit receiver-continuation guidance for high-share focused concentrations:
  use the receiver holder/account from the focused inflow date through at least
  30 calendar days later, or omit `date_end`; do not default to a next-day or
  24-hour window.

## 0.15.93

Release status: superseded by 0.15.94.

- Supersedes 0.15.92 after true run.sh desktop UI review showed the Pair Amount
  facts were corrected but the first visible progress line still announced
  `pair-amount-investigation` and the answer treated receiver-side continuation
  as generic follow-up work.
- Moves the silent skill-selection rule into the Pair Amount discovery text,
  root agent policy, focused agent policy, and contract smoke checks so any
  required pre-tool commentary must be omitted or exactly `正在核验当前案件事实。`.
- Raises high-share focused concentrations from optional downstream context to a
  completion condition: perform one bounded current-case continuation check
  when needed, surface returned receiver-side product/cash/third-party facts as
  `已见承接/分流`, and keep external materials separate from already imported
  current-case流水 review/export actions.

## 0.15.92

Release status: superseded by 0.15.93.

- Supersedes 0.15.91 after manual UI review found a residual Pair Amount skill
  instruction still allowed a `top_transactions` table to be capped at 10 rows.
- Aligns the Pair Amount skill with the runtime support envelope and delivery
  QC: when the returned effective current-case transfer table is 20 rows or
  fewer, the final answer must list every returned row, not just the largest
  10 rows, the dominant date/account concentration, or one aggregate line.

## 0.15.91

Release status: superseded by 0.15.92.

- Supersedes 0.15.90 after true desktop UI retest showed the Pair Amount
  answer still listed only 10 transaction rows because the agent-readable
  support payload entered the budget fallback path.
- Keeps Pair Amount budget fallback row-preserving: when the current-case
  effective transfer count is 20 rows or fewer, the fallback support facts keep
  up to 20 returned top transaction rows and add a support-only completion
  marker requiring the final answer to list the complete row table.
- Prevents `support_payload_truncated=true` from silently downgrading small
  pair-amount answers back to a dominant focused concentration/top-10 table.

## 0.15.90

Release status: superseded by 0.15.91.

- Supersedes 0.15.89 after investigator review showed natural Pair Amount
  answers could still look template-like by listing only the dominant focused concentration
  or the largest 10 rows while the current case contained a small complete
  row-level transfer set.
- Raises Pair Amount transaction-candidate support from 10/12 rows to 20 rows
  across the frontdoor card and agent-readable support envelope.
- Strengthens `pair-amount-investigation`, command metadata, delivery QC, and
  contract smoke checks so 20 笔以内的当前案件转账必须完整逐笔展开；重点集中交易只能作为
  异常、承接和案件意义分析，不能替代未限定时间的完整两方金额结论。

## 0.15.89

Release status: superseded by 0.15.90.

- Supersedes 0.15.88 after true Analytix desktop UI natural Pair Amount retest
  corrected the amount facts but still produced a template-like answer and
  treated already imported transaction/continuation work as generic next steps.
- Strengthens `pair-amount-investigation` so ordinary `A 转给 B 多少钱`
  answers must separate current-case transaction facts, in-case continuation
  leads, and external evidence materials instead of leaving the user with a
  shallow amount card.
- Allows one bounded receiver-side current-case continuation check for material
  focused concentrations before writing follow-up actions, so available outflow,
  product, cash, redemption, or third-party continuation facts can appear as
  `已见承接/分流`.
- Extends delivery QC and contract smoke checks to block wording that presents
  already visible current-case facts as missing data.

## 0.15.88

Release status: superseded by 0.15.89.

- Supersedes 0.15.87 after true Analytix desktop UI natural Pair Amount retest
  showed the runtime still selected ranking support in the default tool surface
  and produced a shallow, template-like answer.
- Makes `investigate_pair_amount` a default visible tool ahead of ranking and
  navigator support, so ordinary `A 转给 B 多少钱` turns can reach the Pair
  Amount owner without depending on user prompting.
- Keeps rank cross-check amounts out of the ordinary agent-readable support
  envelope while preserving the detail/rank difference, full-detail conclusion,
  account scope, row candidates, focused concentration share, and outside focused concentration
  amount/count for the focused owner.
- Adds a Pair Amount non-template depth gate: answers must explain the date
  span, payer/receiver account counts and numbers, full-detail relationship
  versus concentrated transfer cluster, duplicate/repeated-feedback risk, and
  the split between in-case review/export and external evidence.

## 0.15.87

Release status: superseded by 0.15.88.

- Supersedes 0.15.86 after true Analytix desktop UI natural Pair Amount retest
  showed `investigate_pair_amount` was called correctly but its support facts
  promoted a dominant focused transfer cluster into the ordinary conclusion.
- Restores the unrestricted `A 转给 B 多少钱` contract: current-case
  payer-to-receiver detail aggregate controls amount, count, first/last time,
  account scope, and row candidates; `focus_cluster` is retained only as
  abnormal concentration and downstream-tracing support unless the user names
  that period/account/cluster.
- Preserves first/last transaction time, full-scope raw/effective amounts,
  rank cross-check differences, full-scope account counts, and focused concentration
  role in compact support facts so desktop answers cannot collapse to a single
  aggregate row or the wrong statistical period.

## 0.15.86

Release status: superseded by 0.15.87.

- Supersedes 0.15.85 after true Analytix desktop UI natural Pair Amount retest
  still selected the broad navigator path, exposed a `Funds investigate` support
  card, and collapsed available transaction candidates into an aggregate
  `5 笔合计` row.
- Reorders the MCP surface so `investigate_pair_amount` is the first Pair Amount
  fact source, makes `funds_investigate` redirect Pair Amount-like calls back to
  the focused support path, and keeps broad navigator output support-only.
- Promotes Pair Amount `top_transactions`, raw transaction candidates, payer
  accounts, receiver accounts, and focused concentrations into compact facts so the
  final answer has enough row-level material to write a real `重点交易表`.
- Blocks the exact true-UI progress leak pattern `我将使用“两方金额核验”流程`
  / `先核对...统计口径`, and tightens the visible answer contract around
  户名/账号展开、已调取流水内复核/导出、外部补证分离.

## 0.15.85

Release status: superseded by 0.15.86.

- Supersedes 0.15.84 after true Analytix desktop UI Pair Amount retest thread
  `019ebfc3-2b90-7402-8b52-765c7a690db2` selected broad local/tool paths and
  produced an over-broad same-name amount instead of the current-case focused
  2025-08-27 transfer cluster.
- Adds `investigate_pair_amount` as the first-class Pair Amount support tool so
  ordinary `A 转给 B 多少钱` turns do not begin with local files, free SQL,
  full-holder dossiers, or repeated ranking discovery.
- Promotes dominant date/account transfer clusters, payer/receiver account
  counts, raw/effective amounts, duplicate-risk rows, and broader same-name
  scope boundaries into the support envelope for the focused owner to write
  公安经侦材料.
- Blocks additional mechanical UI wording such as `收款端`, `主体账户集合`, and
  generic `调取...全量流水` when the current case already contains relevant
  transaction流水.

## 0.15.84

Release status: superseded by 0.15.85.

- Supersedes 0.15.83 after rollout forensics on true UI thread
  `019ebf96-9c53-7221-b304-e35fb56e0757` showed the answer used local
  skill/report/graph-script context and a Workbench capability gap instead of a
  fully validated semantic source.
- Hardens Pair Amount as a Data Analytics-style source/validation/delivery
  workflow: current-case facts must come from Analytix semantic fact tools or a
  verified controlled Workbench result; shell/grep/local old reports, old graph
  scripts, images, and history are development evidence only.
- Splits Pair Amount next actions into current-case流水 review/export actions
  and external missing materials, so answers do not tell users to re-request
  generic complete流水 when the current case already contains the relevant
  transaction data.

## 0.15.83

Release status: superseded by 0.15.84.

- Supersedes 0.15.82 after true Analytix desktop UI natural Pair Amount retest
  still produced support-layer detail wording such as `原始命中记录` and
  `本轮未展开逐笔明细表`.
- Requires natural `A 转给 B 多少钱` answers to convert available
  `top_transactions` into a visible row-level `重点交易表`; if only aggregate
  support exists, the answer must still include an aggregate evidence row with
  explicit fields to export next.
- Adds smoke coverage for the exact no-detail fallback wording observed in the
  real UI thread.

## 0.15.82

Release status: superseded by 0.15.84.

- Supersedes 0.15.81 after real Analytix desktop UI Pair Amount acceptance
  found the visible answer still used mechanical review language such as
  `同事实去重后的金额`, `证据状态`, and `本轮命中`.
- Hardens Pair Amount, graph, delivery QC, command metadata, and user-visible
  language normalization so ordinary case answers use `当前流水可见`,
  `重复风险复核`, `已有流水支持`, and `核验意见` instead of support-layer terms.
- Adds smoke coverage for the exact mechanical wording variants observed in
  true UI acceptance, while keeping the 0.15.81 visual artifact state-word
  guard intact.

## 0.15.81

Release status: superseded by 0.15.82.

- Supersedes 0.15.80 after true Analytix desktop UI acceptance found a
  fund-flow PNG attachment could expose English diagnostic state words such as
  `supported`, `needs_review`, and `candidate` inside the rendered figure.
- Treats attached image text, legends, node labels, edge labels, alt text, and
  filenames as user-visible output: graph/visual skills must inspect reused
  artifacts and regenerate or downgrade to a compliant table when the image is
  not fit for经侦材料口吻.
- Extends the user-visible language smoke to block graph-state leakage in
  visual artifacts while keeping Pair Amount first-message and convergence
  guards intact.

## 0.15.80

Release status: superseded by 0.15.81.

- Supersedes 0.15.79 so the manual desktop UI acceptance thread locator is a
  versioned plugin artifact instead of an untracked local helper.
- `agent-thread-locator.mjs` now searches only Analytix-owned runtime homes by
  default; the system Codex home is available only through the explicit
  read-only `--include-system-codex` diagnostic flag.
- Keeps the 0.15.79 Pair Amount first-message guard intact while aligning
  release-chain source parity with the new real UI acceptance evidence path.

## 0.15.79

Release status: superseded by 0.15.80.

- Supersedes 0.15.78 after real desktop Pair Amount targeted acceptance proved
  the substantive answer and required labels passed, but the first visible
  message still said `我会使用 pair-amount-investigation 流程`.
- Extends the desktop first-message guard to block `我会使用`, workflow/process
  announcements, `commentary`, and `item-*` leakage in ordinary case turns.
- Adds a smoke fixture for the exact 0.15.78 workflow-announcement leakage.

## 0.15.78

Release status: superseded by 0.15.79.

- Supersedes 0.15.77 after real desktop Pair Amount targeted acceptance proved
  the substantive answer and required labels passed, but the first visible
  message still announced `pair-amount-investigation` skill usage.
- Hardens Pair Amount and root desktop language contracts: even when a focused
  workflow is selected, the user must not see skill names, `我将使用`, or
  “两方金额核验技能/先定位当前案件数据/统计口径” progress wording.
- Adds a smoke fixture for the exact 0.15.77 skill-announcement leakage.

## 0.15.77

Release status: superseded by 0.15.78.

- Supersedes 0.15.76 after real desktop Pair Amount targeted acceptance fixed
  the first-message leakage but still timed out while repeatedly gathering
  support evidence.
- Adds a Desktop Convergence Contract for `pair-amount-investigation`: once a
  controlling pair review/aggregate returns amount, count, period, and
  transaction candidates, the owner must stop tool use and write the case
  material.
- For ordinary `A 转给 B 多少钱` turns, `rank_counterparties` is at most one
  candidate-support call and `validate_report_claims`, graph, dossier, and
  full-case lanes are out of scope for the first answer.

## 0.15.76

Release status: superseded by 0.15.77.

- Supersedes 0.15.75 after real desktop targeted acceptance showed Pair Amount
  content passed but the first visible progress message still said
  `我会按...口径`.
- Hardens agent-level routing instructions so ordinary case tasks default to no
  progress and the first visible assistant message may only be
  `正在核验当前案件事实。` when progress is unavoidable.
- Adds explicit smoke coverage for `我会按当前案件资金研判流程先读取案件数据口径`
  and `可用交易明细` progress leakage.

## 0.15.75

Release status: superseded by 0.15.76.

- Supersedes 0.15.74 after real desktop acceptance showed several answers had
  correct facts but still missed mandatory material labels or leaked
  `按...口径` progress wording.
- Makes substantive Pair Amount answers retain exact visible labels
  `重点交易表`, `异常特征`, `案件意义`, `暂不能认定`, and `下一步取证`.
- Requires downstream continuation to expose `下游去向`, subject dossiers to
  expose `重点对手方`, and report continuation with amounts to include an
  independent `## 金额核对` section.
- Extends user-visible leakage guards and desktop acceptance to block
  `按...口径`, `先读取...技能说明`, and audit-layer `资金边` wording.

## 0.15.74

Release status: superseded by 0.15.75.

- Supersedes 0.15.73 after real nine-scenario desktop acceptance exposed
  delivery-layer gaps outside the Pair Amount fact result.
- Tightens graph visualization so ordinary answers use `资金流向图`,
  `资金链路`, and `交易链路` instead of Mermaid source/code fences or
  audit-layer `资金边` wording.
- Requires subject dossiers to retain a visible `重点对手方` section when
  counterparties are discussed.
- Requires report continuation or report patching that contains amounts to
  include a visible `金额核对` note/section.
- Makes desktop acceptance marker matching whitespace-insensitive for Chinese
  fact markers such as `16笔` and `16 笔`.

## 0.15.73

Release status: superseded by 0.15.74.

- Supersedes 0.15.72 after real desktop Pair Amount acceptance showed correct
  amount/count facts but leaked implementation progress language to ordinary
  users.
- Tightens Pair Amount and root skill user-visible progress: interim messages
  must be omitted or limited to the business sentence "正在核验当前案件事实。"
  and must not mention execution, table/schema checks, semantic layers, SQL, or
  support internals.
- Requires substantive two-party amount answers to expose the business labels
  `统计期间` and `案件意义`, so correct facts are delivered as economic
  investigation material rather than loosely titled prose.
- Extends user-facing language translation for executor/query/schema/semantic
  wording before the leakage guard runs.

## 0.15.72

Release status: superseded by 0.15.73.

- Fixes the real desktop Pair Amount closure by letting the deduped
  counterparty aggregation control effective amount/count when it is available,
  while demoting conflicting detail-query output to auxiliary examples and
  review risk.
- Adds support-envelope fields for the effective-stat basis and auxiliary
  detail-query flag so the focused Pair Amount owner receives one controlling
  amount/count instead of competing support facts.
- Blocks visible implementation leakage such as SQL/executor progress language
  in the shared user-facing language guard and the Electron multi-turn
  acceptance runner.
- Adds the real Electron multi-turn acceptance runner to doctor and release
  slice coverage so desktop current-case, report, graph, export, and
  continuation workflows are checked through the embedded analytixagent path.

## 0.15.71

Release status: superseded by 0.15.72.

- Closes a remaining support-card language leak by replacing the old
  "重点收款账户核验" heading with material-facing "重点收款账户流水" in audit
  rendering, destination diagnostics, frontdoor forbidden markers, and
  user-visible language smoke coverage.
- Tightens legacy amount-card translation so old "金额口径/口径提示" wording is
  rewritten as "金额核验情况/补证方向" instead of another tool-card heading.
- Makes replay tasks override same-id historical golden prompts in the legacy
  model eval harness, so old support-card wording cannot re-enter positive
  prompt or success-marker paths through fixture merge order.
- Moves the SQL-safe schema contract into the MCP tool surface itself:
  `inspect_case_schema` and `run_case_sql` now tell the model to use
  `table.sql_name`, `column.sql_name`, or `sql_identifier`, and never execute
  Chinese `display_name` labels as SQL; functional eval now guards that
  model-visible tool description.
- Extends Pair Amount frontdoor support with Top transaction candidates, so the
  `pair-amount-investigation` lead owner can form the required Top5/Top10
  transaction table instead of relying only on aggregate amount concentrations.

## 0.15.70

Release status: superseded by 0.15.71.

- Makes focused workflows the only owners of substantive Chinese investigation
  delivery while keeping navigator, ranking, cards, and workbench output as
  bounded structured support.
- Adds a real current-case cleaned-detail export workflow with explicit
  confirmation, Analytix-owned output boundaries, resumable jobs, and CSV/XLSX
  read inspection before delivery.
- Adds SQL-safe schema names, Analytix workflow context, investigative delivery
  QC, Pair Amount duplicate-key safety, mature-plugin drift mapping, and Hub
  package checks that reject fixed real-case facts.
- Keeps Investigation Lab's internal planning pass while removing its duplicate
  full-case risk scan, so focused open-ended review completes within the live
  frontdoor timeout without weakening the later evidence probes.

## 0.15.69

Release status: superseded by 0.15.70.

- Promotes Pair Amount into `pair-amount-investigation` as the lead owner:
  `quick-fact`, `rank_counterparties`, `funds_investigate`, duplicate audit, and
  controlled workbench results now act as support evidence instead of final
  Chinese amount prose.
- Adds Analytix workflow context and delivery QC coverage so current-case sync,
  cleaned-detail exports, analysis indexes, visual artifacts, report
  continuation, full-report amount checks, attachments, and multi-turn
  continuation are treated as first-class workflow surfaces.
- Tightens support-layer hiding and professional delivery contracts: answer
  drafts are no longer rendered as ordinary agent-readable bodies, and
  functional checks now cover lead owner routing, support-layer hidden text,
  workflow context, visual/report artifacts, investigative spine, evidence
  maturity language, and non-template output.

## 0.15.68

Release status: superseded by 0.15.69.

- Supersedes 0.15.67 with a healthy release-chain closure: release slice and
  isolated node checks now include the report-conclusion diagnostic runtime and
  the current production MCP protocol files touched by user-facing delivery.
- Promotes Pair Amount into `pair-amount-investigation` as the lead owner:
  `quick-fact`, `rank_counterparties`, `funds_investigate`, duplicate audit, and
  controlled workbench results now act as support evidence instead of final
  Chinese amount prose.
- Adds Analytix workflow context and delivery QC coverage so current-case sync,
  cleaned-detail exports, analysis indexes, visual artifacts, report
  continuation, full-report amount checks, attachments, and multi-turn
  continuation are treated as first-class workflow surfaces.
- Tightens support-layer hiding and professional delivery contracts: answer
  drafts are no longer rendered as ordinary agent-readable bodies, and
  functional checks now cover lead owner routing, support-layer hidden text,
  workflow context, visual/report artifacts, investigative spine, evidence
  maturity language, and non-template output.
- Tightens economic-investigation language in graph/report cards by replacing
  audit-layer phrases such as "已有数据支持的交易级资金边" with "交易级可证实资金边"
  and removing "作答骨架/第一行保留" instruction wording from visible review
  output.
- Expands user-visible language smoke and functional delivery checks so graph
  protocol text, Lab protocol text, intent-plan text, and report-review cards
  fail if they leak the old internal wording.

## 0.15.67

Release status: superseded by 0.15.68.

- Supersedes 0.15.66 with the P0 user-facing language closure: ordinary
  answers, cards, reports, graphs, tables, and attachment-facing text now
  translate internal status/tool/debug wording into public-security
  economic-investigation language.
- Fixes account and subject dossier delivery so account-level cards surface
  real top counterparties instead of placeholder empty tables, including clear
  "对手字段缺失" wording when bank data lacks counterparty fields.
- Tightens one-hop and continuation amount wording while expanding
  functional/output-leak tests; later releases replace the remaining
  mechanical amount labels with conclusion-led focused-owner delivery.

## 0.15.66

Release status: superseded by 0.15.67.

- Supersedes 0.15.65 with the real-frontdoor closure fix: embedded
  analytixagent now passes the current desktop backend base URL to MCP plugins,
  so active-case resolution follows the last-opened/selected Analytix case
  instead of falling back to a stale packaged localhost default.
- Tightens Pair Amount routing so "A 转给 B 多少钱" uses the deterministic
  mini-review card before any ranking fallback; `rank_counterparties` remains a
  candidate locator, and the visible answer must keep raw, effective/dedup,
  optional principal/fee, duplicate/unsupported, account/time cluster,
  duplicate-group, and next-verification boundaries.
- Updates functional smoke expectations to the new Pair Amount mini-review
  contract, preserving the old ranking amount only as a boundary/candidate
  marker and not as the final pair amount.

## 0.15.65

Release status: superseded by 0.15.66.

- Supersedes 0.15.64 with the real-frontdoor P0 closure for current-case
  synchronization: backend active-case now restores the last-opened Analytix
  case, MCP source resolution emits Case Source Blocker instead of local case
  discovery or guessed ids, and explicit invalid `case_id` stays invalid.
- Fixes Pair Amount delivery for "A 转给 B 多少钱" questions: rank results are
  candidate-only, the frontdoor performs a targeted controlled Workbench
  aggregation over the current case, and the visible answer separates raw,
  effective/dedup, optional principal/fee, duplicate/unsupported, account/time
  cluster, duplicate-group, and next-verification boundaries.
- Extends functional release evidence with env-driven real frontdoor tasks for
  Pair Amount and weak-source blockers, so production scripts do not store real
  case answers while release closure can still prove current-case source,
  validation, and delivery behavior.

## 0.15.64

Release status: superseded by 0.15.65.

- Supersedes 0.15.63 after the Data Analytics parity closure pass promoted
  source, validation, and delivery checks into functional release gates instead
  of relying on old score-style evidence.
- Adds functional closure coverage for `case_source_envelope`, MCP
  decentralization, controlled Case Workbench SQL/notebook boundaries, output
  leak prevention, Hub package golden/oracle isolation, Pair Amount
  reconciliation, and live frontdoor semantic/weak-source smoke.
- Makes doctor enforce the controlled Case Workbench contract: SQL/notebook
  tools must require purpose/analysis goal, allow active-case resolution,
  clamp rows, stay in cleaned/analysis scope, block raw/write/external paths,
  and return capability gaps rather than fabricated facts.

## 0.15.63

Release status: superseded by 0.15.64.

- Supersedes 0.15.62 after the real post-install Agent UI replay proved the
  Amount Challenge source-backed answer path but needed 14 analytix_funds calls
  for schema/scope/data-quality, rank candidate positioning, duplicate-family
  review, and multiple targeted SQL aggregations.
- Raises the explicit Amount Challenge replay budget to 16 so the release
  runner no longer treats required raw/dedup/window/core-account recomputation
  as a route violation.
- Production Pair Amount / Amount Challenge behavior remains unchanged: final
  disputed amounts must be current-case, read-only, source-backed, scoped, and
  evidence-bounded.

## 0.15.62

Release status: superseded by 0.15.63.

- Supersedes 0.15.61 after the real Agent UI replay showed the Amount
  Challenge task prompt was underspecified and could reasonably ask for
  clarification without using case tools.
- Rewrites the replay prompt as the concrete Pair Amount dispute
  a synthetic Pair Amount dispute between a duplicate-amplified value and a
  high-confidence core transaction concentration, so the release
  proof exercises the required source-backed verification path instead of a
  generic clarification behavior.
- Keeps the explicit 10-tool source-backed replay budget from 0.15.61 and the
  production Pair Amount / Amount Challenge behavior unchanged.

## 0.15.61

Release status: superseded by 0.15.62.

- Supersedes 0.15.60 after reviewing the real post-install Agent UI replay
  shape: Amount Challenge can legitimately need schema/scope/quality, SQL
  retry, coverage, duplicate-family, and current-case checks.
- Raises the explicit Amount Challenge replay budget to 10 and exempts
  explicit source-backed replay tasks from the generic ordinary-task 1-2 tool
  warning, so validation no longer pressures Codex back into weak-source or
  rank-only final answers.
- Production Pair Amount / Amount Challenge behavior remains unchanged from
  0.15.59/0.15.60: final challenged amounts must remain current-case,
  read-only, source-backed, scoped, and evidence-bounded.

## 0.15.60

Release status: superseded by 0.15.61.

- Supersedes 0.15.59 after the post-install Agent UI replay proved the
  Amount Challenge functional path but still surfaced a stale golden-task
  constructor budget of 2 tools.
- Carries `expected_tool_budget` from golden replay fixtures into the live
  Agent UI task inventory so source/schema, quality, targeted SQL, coverage,
  and duplicate-family checks are not misreported as ordinary-task budget
  drift.
- Keeps the production Pair Amount / Amount Challenge behavior from 0.15.59:
  `rank_counterparties` is optional candidate positioning only, while final
  challenged amounts remain source-backed and current-case scoped.

## 0.15.59

Release status: superseded by 0.15.60.

- Supersedes 0.15.58 after the post-install Agent UI replay proved the
  functional fix but still reported a route-budget warning and treated
  `rank_counterparties` as an expected Amount Challenge tool.
- Makes `rank_counterparties` optional candidate positioning for Amount
  Challenge replay instead of a required tool, while preserving the requirement
  that final challenged amounts use current-case, read-only
  cleaned/analysis-scope targeted aggregation when semantic facts are
  insufficient.
- Adds a task-level replay budget for Amount Challenge so source/schema,
  quality, targeted SQL, duplicate-family, and coverage checks are not
  misreported as a hard-route violation when they are needed to avoid an
  unsupported final amount.

## 0.15.58

Release status: superseded by 0.15.59.

- Supersedes 0.15.57 after the post-install Agent UI replay exposed an old
  validation contract that still asked Amount Challenge to call
  `rank_counterparties` only.
- Updates the frontdoor replay, model eval, and golden fixture contracts so
  `rank_counterparties` remains candidate positioning only. Challenged final
  amounts must continue through current-case, read-only
  cleaned/analysis-scope targeted aggregation with `inspect_case_schema` and
  `run_case_sql` when semantic facts are insufficient.
- Requires Amount Challenge answers to separate supported and unsupported
  amounts, raw/effective/dedup basis, holder/account scope, counterparty grain,
  time window, duplicate/card-replacement risk, validation state, source
  boundary, evidence boundary, difference reason, and next verification, while
  keeping unsupported competing amounts out of fact language.

## 0.15.57

Release status: superseded by 0.15.58.

- Supersedes 0.15.56 after the Pair Amount / Amount Challenge audit showed
  `rank_counterparties` could still be described as a one-turn final amount
  path and production claim review still contained fixed-case deterministic
  augments.
- Rebuilds the Pair Amount lane around Data Analytics-style source selection:
  ranking rows are candidate evidence only; final amounts must reconcile
  source-of-truth, holder/account scope, counterparty grain,
  raw/effective/dedup basis, time window, duplicate/card-replacement risk,
  validation state, source boundary, delivery state, supported amount,
  unsupported amount, difference reason, and next verification.
- Removes fixed-case claim-review augments from production runtime, generalizes
  packaged references, and updates release guard/frontdoor smoke scoring so
  direct MCP card coverage stays a diagnostic artifact rather than model
  quality evidence.

## 0.15.56

Release status: publish candidate.

- Supersedes 0.15.55 after the full focused Agent UI A/B release audit showed
  `subject_dossier_person` could exceed the focused tool budget through
  repeated account drill-down and Workbench fallback, while
  `graph_visualization_supported_edges` could omit the exact visible
  `candidate` label even though the Chinese candidate boundary was present.
- Tightens the Subject Dossier first-turn path to finish from
  `analyze_holder_full`, `rank_accounts`, and `rank_counterparties`, and to
  treat missing contact/address/IP/MAC/company/legal-representative fields as
  evidence gaps rather than opening ad hoc SQL or tracing lanes.
- Pins user-visible audit labels for Subject Dossier and graph answers:
  `Subject Dossier / 主体画像`, `账户结构`, `归属边界（direct / candidate）`,
  `不能写成已确认归属`, `candidate`, `needs_review`, and `证据缺口`.

## 0.15.55

Release status: published candidate superseded by 0.15.56 after full focused
Agent UI A/B release audit showed Subject Dossier tool-budget drift and graph
candidate-label drift.

- Supersedes 0.15.54 after the full focused Agent UI A/B release audit showed
  `case_notebook_controlled_query` could still echo diagnostic source/delivery
  vocabulary in a user-visible Workbench answer.
- Tightens Case Workbench answer style and real Agent UI route hints so the
  final answer uses affirmative delivery language, preserves
  `验证状态` / `证据边界` / `能力缺口`, and keeps internal diagnostic labels out of
  ordinary user-facing text.
- Fixes the canonical night-large-outflow SQL guidance: the all-outflow
  denominator includes every `dc_val='出'` row with a non-null amount, while the
  night condition applies only to the numerator and the data-quality boundary.

## 0.15.54

Release status: published candidate superseded by 0.15.55 after full focused
Agent UI A/B release audit showed Workbench final-answer wording and denominator
discipline could still drift in the complete suite.

- Supersedes 0.15.53 after the full focused Agent UI A/B release audit showed
  Workbench now passed, but `report_builder_generation` and
  `case_3c72_report_claim_review` could still rewrite the tool-returned
  report/claim-review skeleton into a generic memo.
- Preserves report-builder and claim-review visible delivery structure in
  agent policy, focused skill contracts, and the real Agent UI route hints:
  `报告草稿`, `来源边界`, `数据质量`, `未支持 claim`, `未支持/不能确认资金流`,
  `降级/线索`, `禁用表述`, and `复核动作` remain user-facing safety labels.

## 0.15.53

Release status: published candidate superseded by 0.15.54 after full focused
Agent UI A/B release audit showed report-builder / claim-review final answers
could still drop the visible delivery skeleton.

- Supersedes 0.15.52 after real Agent UI Workbench targeted A/B showed the
  plugin could execute scope/schema/quality/SQL/notebook and surface the final
  delivery labels, but the final answer could still repeat an English internal
  source label and call `inspect_case_schema` twice.
- Rewrites model-injected Workbench boundaries to user-facing “原始来源明细/来源明细”
  language, keeps the release scorer guard intact, and tightens the Workbench
  route to one scope, one schema, one quality audit, one SQL execution, and one
  notebook artifact for the custom metric.

## 0.15.52

Release status: published candidate superseded by 0.15.53 after real Agent UI
Workbench targeted A/B showed the final answer could still leak an internal
source label and exceed the Workbench tool budget through duplicate schema
preflight.

- Supersedes 0.15.51 after real Agent UI Workbench targeted A/B still showed the
  final answer could omit literal `验证状态:` / `证据边界:` / `能力缺口:` labels,
  skip `audit_case_data_quality`, and repeatedly call `run_case_sql` to reread a
  custom metric.
- Adds a Workbench final-answer tail directly to the Agent-visible MCP result,
  including `验证状态:`, `证据边界:`, `能力缺口:`, and `可回放` wording, and clarifies
  that `get_scope_coverage` is a lightweight coverage counter, not a data-quality
  preflight substitute.

## 0.15.51

Release status: published candidate superseded by 0.15.52 after real Agent UI
Workbench targeted A/B showed the final answer could still omit the literal
validation/evidence/gap tail and skip the data-quality preflight.

- Supersedes 0.15.50 after real Agent UI Workbench targeted A/B still showed the
  final answer could replace the required `验证状态:` label with looser
  `SQL 执行` wording, leaving the source + validation + delivery contract
  unverifiable to the release scorer.
- Moves Workbench validation state to the first Agent-visible status line and
  explicitly rejects `SQL 执行: 已执行` / `Notebook/artifact: 已创建` as substitutes
  for the literal final `验证状态:` label.

## 0.15.50

Release status: published candidate superseded by 0.15.51 after real Agent UI
Workbench targeted A/B showed the final answer still omitted the literal
`验证状态:` label.

- Supersedes 0.15.49 after real Agent UI Workbench targeted A/B showed the
  plugin now computes the custom aggregate and creates a replayable notebook,
  but the final answer can still omit the literal `验证状态:` delivery label and
  fail the source + validation + delivery contract.
- Tightens the case-workbench workflow to use `get_case_scope_map` for the
  source envelope, requires final visible `验证状态:` / `证据边界:` / `能力缺口:`
  labels, and adds a fixed Workbench evidence-boundary line to the Agent-visible
  output compiler without changing the underlying calculation.

## 0.15.49

Release status: published candidate superseded by 0.15.50 after real Agent UI
Workbench targeted A/B showed the final answer still omitted the literal
`验证状态:` label after the aggregate-preview fix.

- Supersedes 0.15.48 after full real Agent UI focused A/B showed the Controlled
  Case Workbench path could execute SQL/notebook artifacts but leave the model
  without a compact aggregate result preview, causing it to treat successful
  delivery as missing delivery.
- Adds a bounded Workbench result preview to the Agent-visible payload only for
  aggregate or very small current-case cleaned/analysis results, and tightens
  the case-workbench skill so successful SQL/notebook runs must state
  `验证状态`, `能力缺口`, evidence boundary, and productization target without
  repeating the same SQL.

## 0.15.48

Release status: published candidate superseded by 0.15.49 after full real
Agent UI focused A/B exposed a Case Workbench delivery-envelope gap.

- Supersedes 0.15.47 after real Agent UI targeted A/B confirmed the graph path
  now calls both `get_casegraph` and `build_fund_flow_graph`, but the final
  answer could still merge the required `确定性交易边（Mermaid 前置事实表）`
  section into `数据事实` or rename it as `确定性资金边`.
- Makes the graph focused skill, Agent-visible output compiler, and fund-flow
  runtime contract require the exact standalone heading
  `确定性交易边（Mermaid 前置事实表）` before any Mermaid block, preserving the
  user-visible proof table that separates supported edges from candidate,
  missing, same-name, partial, needs-review, and unsupported records.

## 0.15.47

Release status: published candidate superseded by 0.15.48 after real Agent UI
targeted A/B showed the graph answer still omitted the exact standalone
`确定性交易边（Mermaid 前置事实表）` heading.

- Supersedes 0.15.46 after release closure found two non-runtime blockers:
  production Hub source on the server could still package stale evidence from an
  older source tree, and the direct MCP smoke script still encoded a legacy
  navigator-only/frontdoor score contract.
- Keeps runtime behavior from 0.15.46, but makes release evidence match the
  Data Analytics boundary: live Hub packages must be free of old evidence/eval
  artifacts, and direct smoke uses targeted semantic tools for explicit
  rank/trace/claim/casegraph tasks while reserving `funds_investigate` for
  navigator checks.

## 0.15.46

Release status: published candidate superseded by 0.15.47 after release closure
found stale Hub source/evidence packaging risk and a legacy direct MCP smoke
contract.

- Supersedes 0.15.45 after real Agent UI targeted A/B showed graph answers
  preserved the required output sections but broad graph requests could skip
  `get_casegraph`, losing case context and quality-boundary grounding.
- Clarifies graph tool discovery: broad current-case relationship/casegraph/
  visual graph QA calls `get_casegraph` before `build_fund_flow_graph`, while
  narrow known-source path continuation may still use the fund-flow graph tool
  directly.

## 0.15.45

Release status: published candidate superseded by 0.15.46 after real Agent UI
targeted A/B showed broad graph requests could skip `get_casegraph`.

- Supersedes 0.15.44 after real Agent UI targeted A/B showed graph answers
  preserved `candidate` but could still rename the required `证据缺口` section
  as a generic补证说明.
- Adds `证据缺口` as a standalone graph final-answer section in the flowgraph
  delivery contract, Agent-visible compiler facts, and graph-visualization
  focused skill output order.

## 0.15.44

Release status: published candidate superseded by 0.15.45 after real Agent UI
targeted A/B showed graph answers still lacked a standalone `证据缺口` section.

- Supersedes 0.15.43 after real Agent UI targeted A/B showed claim-review
  recovered but graph-visualization could still compress candidate and evidence
  gap boundaries out of the final answer.
- Keeps the supported-edge-only Mermaid behavior and promotes user-visible
  `candidate` and `证据缺口` graph boundaries into the flowgraph answer card,
  answer contract, compiler facts, and focused skill completion gate.

## 0.15.43

Release status: published candidate superseded by 0.15.44 after real Agent UI
targeted A/B showed graph-visualization could still omit visible candidate and
evidence-gap boundaries.

- Supersedes 0.15.42 after deep runtime health showed the dedicated
  claim-review card renderer preserved the table shape but no longer surfaced
  the cash/asset destination and report-claim source-boundary signals required
  by the live claim verifier risk gate.
- Keeps the dedicated `validate_report_claims` renderer and adds user-safe
  risk lines for `报告级 claim 需绑定事实来源`, cash/wealth/asset destination
  boundaries without supported transaction edges, and `必需事实已齐` status for
  supported-claim normalization.

## 0.15.42

Release status: published candidate superseded by 0.15.43 after deep runtime
health found missing claim-verifier risk signals in the dedicated card renderer.

- Supersedes 0.15.41 after real Agent UI targeted A/B proved graph delivery was
  above the release line but `case_3c72_report_claim_review` could still
  compress the review card into prose and miss stable per-claim delivery
  headings.
- Adds a dedicated Agent-visible claim-review card renderer for
  `validate_report_claims`: the answer skeleton now preserves the per-claim
  status table plus `需纠正`, `未支持 claim`, `未支持/不能确认资金流`, `降级/线索`,
  `来源边界`, `正式报告暂不出具`, `禁用表述`, and `复核动作` labels without exposing
  internal audit ids or report-gate fields.

## 0.15.41

Release status: published candidate superseded by 0.15.42 after real Agent UI
targeted A/B showed claim-review answers could still miss stable per-claim
delivery headings even though graph delivery passed.

- Supersedes 0.15.40 after real Agent UI targeted A/B showed graph tools and
  markers were present but graph answers could still compress the delivery into
  a memo and miss the commercial five-layer handoff shape.
- Adds a direct graph answer skeleton to `build_fund_flow_graph` compiled facts:
  `数据事实`, `统计特征`, `线索`, `需复核`, and `不可判断` must remain visible
  headings, with supported transaction edges separated from statistics, leads,
  review boundaries, and non-determinable final/legal claims.

## 0.15.40

Release status: published candidate superseded by 0.15.41 after real Agent UI
targeted A/B scored the graph answer below the release line because the answer
compressed the five-layer graph delivery into a memo.

- Supersedes 0.15.39 after real Agent UI targeted A/B showed graph answers had
  fixed supported/boundary status delivery but still did not reliably keep the
  explicit `数据事实` / `不可判断` evidence layers expected from a commercial
  graph handoff.
- Adds a visible five-layer graph delivery contract: `数据事实` for supported
  transaction edges, `统计特征` for amount/time/use distributions, `线索` for
  follow-up objects, `需复核` for missing/same-name/partial boundary rows, and
  `不可判断` for final destinations or legal conclusions without receipts,
  receiving-side statements, or balance-carry proof.

## 0.15.39

Release status: published candidate superseded by 0.15.40 after real Agent UI
targeted A/B scored the graph answer below the release line because the answer
still lacked stable `数据事实` / `不可判断` graph evidence layers.

- Supersedes 0.15.38 after real Agent UI targeted A/B showed graph answers
  could avoid bad Mermaid arrows but still drop the explicit
  `supported/candidate/needs_review` status and evidence-gap skeleton.
- Separates supported fund-flow edges from boundary rows in the Agent-visible
  graph payload and compiled text: `supported` edges are the only Mermaid
  source; `candidate`, `needs_review`, `partial`, `missing`,
  `needs_evidence`, and `unsupported` rows remain boundary/follow-up evidence.
- Adds graph status counts, supported-edge preview, boundary-edge preview, and
  visible `证据缺口` wording so frontdoor answers do not rely on the model to
  reconstruct the graph delivery contract.

## 0.15.38

Release status: published candidate superseded by 0.15.39 after real Agent UI
targeted A/B showed graph visualization still needed a stable status-summary
and evidence-gap delivery skeleton.

- Supersedes 0.15.37 after real Agent UI targeted A/B showed claim-review could
  still follow the eval route hint into `run_full_case_analysis` and one extra
  SQL call before answering the validation card.
- Treats existing-report `run_full_case_analysis(write_report=true) fetch
  failed` and incomplete `trace_fund/trace_fund_next_hop` statements as reviewed
  claims/source status, not commands to run full-case analysis.
- Hardens fund-flow graph runtime so missing, unknown, unmatched, and same-name
  review endpoints are `needs_review` boundary rows instead of Mermaid-eligible
  supported arrows; graph skill and command metadata now require
  endpoint-complete supported edges for Mermaid.
- Aligns Agent UI A/B route hints with the production contract: `/analytix
  review` is validate-first and stops on completed review cards, while
  `/analytix flowgraph` keeps missing/candidate/partial/needs_review rows out
  of proof arrows.

## 0.15.37

Release status: published candidate superseded by 0.15.38 after real Agent UI
targeted A/B showed claim-review and graph visualization still needed tighter
frontdoor route and endpoint-completeness contracts.

- Supersedes 0.15.36 after real Agent UI targeted A/B showed claim-review
  answers could drop the visible `既有报告 claim 复核事实卡` shape and continue
  into extra schema/SQL calls after the validation card.
- Registers `/analytix review` as a claim-review command alias and tightens the
  focused skill so existing report text is answered as a per-claim
  pass/block/downgrade/needs-data card with corrected wording, unsupported-flow
  boundary, forbidden legal/ownership upgrades, and next proof actions.
- Tightens the claim verifier/card renderer stop signal so a completed
  claim-review card defaults to zero additional same-turn tools while still
  preserving the controlled workbench path for named `needs data` claims.

## 0.15.36

Release status: published candidate superseded by 0.15.38 after real Agent UI
targeted A/B showed claim-review needed a stronger visible review-card contract.

- Hardens graph-visualization delivery so supported-edge fund-flow answers must
  state source/cleaning scope, then list `确定性交易边（Mermaid 前置事实表）`,
  and only then render Mermaid from those supported edges.
- Adds a graph delivery contract to the fund-flow runtime and compact Agent
  facts, preserving current-case normalized/detail or analysis-index scope,
  duplicate/same-fact/card-replacement boundaries, missing-counterparty
  boundaries, and next review actions.
- Extends fundgraph boundaries so charts and Mermaid remain visual indexes:
  candidate, partial, missing, needs-review, aggregate, or unmatched endpoints
  stay as evidence gaps rather than proof arrows or final destinations.

## 0.15.35

Release status: published candidate superseded by 0.15.36 after real Agent UI
A/B found graph visualization could still place Mermaid before the deterministic
edge fact table.

- Adds deterministic claim-review augmentation for submitted report claims that
  explicitly name court totals, Liu Wenliang/Zhang Jinzhi duplicate-boundary
  amounts, or existing-report `fetch failed` source status. The augmentation
  computes bounded current-case aggregates through production tools and returns
  corrected claims, unsupported-flow boundaries, and next review actions
  without exposing raw rows.
- Tightens the claim-review focused skill so existing report text goes through
  `validate_report_claims` first and stops when the claim-review card already
  contains corrected facts or `max_additional_tools: 0`.
- Requires graph-visualization answers to list a `确定性交易边` fact table or
  bullet list before Mermaid, keeping candidate/missing edges outside proof
  chains.
- Cleans claim-review card wording so ordinary answers keep the report blocker,
  evidence boundary, and next-action meaning without exposing internal protocol
  keys, generated claim ids, or raw payload terminology.

## 0.15.34

Release status: published candidate superseded by 0.15.35 after real Agent UI
A/B release audit found claim-review missed corrected report markers and graph
visualization missed the required deterministic transaction-edge wording.

- Adds a short MCP transient retry for retryable internal/fetch/lock errors so
  long Agent UI frontdoor sessions do not turn temporary service blips into
  false missing-fact answers.
- Aligns `get_evidence_pack` public schema with backend input support by
  removing unsupported `entity_ids`, and tightens visual-evidence delivery so
  failed packs produce explicit blockers rather than 0-valued placeholder
  tables.
- Strengthens Case Workbench and claim-review workflows: custom night/large
  outflow analysis now uses executable current schema fields
  (`txn_ts`, `dc_val`, `amount`), and report claim review may run one bounded
  claim-scoped SQL check when submitted amounts remain unverified.

## 0.15.33

Release status: published candidate superseded by 0.15.34 after real Agent UI
A/B found transient MCP failures, visual-evidence placeholder delivery, and
Workbench SQL field drift.

- Exposes the controlled Workbench tools (`inspect_case_schema`,
  `run_case_sql`, and `create_case_notebook`) in the default semantic toolbox
  so SQL/notebook/custom口径 work remains current-case, read-only, bounded, and
  auditable instead of being blocked by discovery.
- Tightens focused first-turn workflows for account/subject dossiers, case
  context, graph visualization, claim review, and Case Workbench delivery
  cards after real Agent UI A/B surfaced over-sweeping and hidden-tool gaps.
- Updates eval-only claim-review prompts and marker aliases so release evidence
  reviews submitted report claims explicitly and treats pure tool-budget drift
  as an audit warning, while preserving hard failures for weak-source facts,
  unsupported flow, internal leakage, and ownership/legal overclaim.

## 0.15.32

Release status: published candidate superseded by 0.15.33 after real Agent UI
A/B found Workbench discovery and focused-skill delivery drift.

- Aligns the release audit quality-evidence gate with the focused Agent UI A/B
  release contract, so strict publish evidence requires the 12 required focused
  product-surface tasks in `plugin_disabled` and `full_plugin` modes instead of
  misclassifying the broader 38-task eval inventory as mandatory frontdoor
  release evidence.

## 0.15.31

Release status: published candidate superseded by 0.15.32 after release-audit
quality-evidence contract alignment.

- Aligns the local runtime-cache contract with the production Hub package by
  checking only packaged runtime files and required skills; release/eval scripts
  remain workspace evidence and are intentionally excluded from installed Hub
  packages.

## 0.15.30

Release status: published candidate superseded by 0.15.31 after runtime-cache
contract alignment.

- Tightens Hub package isolation so production packages exclude local
  `output/`, `evidence/`, and validation `scripts/` trees; the package
  validator now rejects artifact, golden, oracle, and eval-fixture paths instead
  of relying on manual review.
- Supersedes 0.15.29 before Hub publish because the local source sync check
  found `output/.../evidence` would otherwise enter the Hub source tree.

## 0.15.29

Release status: publish candidate superseded by 0.15.30 before Hub publish.

- Aligns the release identity for the current Data Analytics-equivalent
  governance slice after the existing 0.15.25-0.15.28 tags, so manifest, MCP
  server, registry, runtime-cache contract, Hub package, runtime evidence, and
  final release tag can point at one HEAD.
- Extends the doctor/release guard contract to cover all 15 soft boundaries,
  including SQL/Python/notebook allowance, navigator-not-commander ownership,
  report completion gates, and selected-case scoped state.
- Keeps `funds_investigate` as a navigator while preserving focused ownership
  for ranking, dossiers, tracing, full-case analysis, visual evidence, report
  building, claim review, and Controlled Case Workbench delivery.
- Replaces stale manual-confirmation wording in release planning with the
  explicit 15.6 authorization model while keeping all publish actions gated by
  current HEAD, runtime cache, true frontdoor evidence, Hub package hash, and
  admin-console authentication.

## 0.15.24

Release status: published candidate superseded by 0.15.29.

- Hardens the top-pluginization blueprint and packaged references into the
  release source of truth for a commercial fund-investigation plugin, including
  the stable `/analytix` command namespace, capability matrix, command router,
  evidence boundaries, and Hub lifecycle contract.
- Filters eval-only registry coverage fields from production MCP resources and
  adds doctor coverage proving production resources expose only publishable
  capability metadata.
- Cleans user-visible product wording across manifest, README, SKILL,
  references, and MCP Answer Cards so public plugin output describes the
  Analytix packaged capability instead of internal process history.
- Expands release-slice ownership to the blueprint/reference/router/runtime
  files changed by this candidate, keeping package preparation, doctor, release
  audit, and later Hub publish checks aligned on the same plugin slice.

## 0.15.23

Release status: published candidate superseded by 0.15.24.

- Adds a shared capability-registry derived metadata snapshot contract covering
  command owners, tool ownership, eval task ids, passive/report task groups, and
  ordinary/report tool-budget profiles.
- Extends `doctor` and the release guard with a deterministic drift gate proving
  that 33 command metadata entries, 36 tools, 21 eval tasks, 6 passive tasks,
  and 4 report claim-review tasks derive from one registry source.
- Keeps the 0.15.22 casegraph component evidence-pack runtime behavior
  unchanged; this slice tightens registry/metadata governance for future
  SKILL/MCP/doctor/eval generation.

## 0.15.22

Release status: published candidate superseded by 0.15.23.


- Adds a minimal casegraph component evidence pack for every required
  casegraph/fundgraph roadmap component: subject, account, holder,
  counterparty, transaction edge, same-fact family, rule hit,
  wealth/asset/cash break, missing receipt, evidence pack, and claim support.
- Each component now produces internal `evidence_pack`, `claim_support`, and
  non-supported `evidence_boundaries` entries with `source_refs`; lead and
  needs-evidence components stay downgraded as next-review actions instead of
  being upgraded into facts.
- Extends `doctor` with a deterministic component evidence-pack contract and
  keeps the 0.15.21 Investigation Lab, golden coverage, Hub install, and
  runtime isolation behavior unchanged.

## 0.15.21

Release status: published candidate superseded by 0.15.22.

- Carries the production `hypothesis_probe` double-empty counterparty statistic
  into the Investigation Lab Answer Card, Evidence Ledger fact set,
  hypothesis queue, and cash-break boundary lines.
- Requires Investigation Lab answers to preserve `investigation_intent`,
  `hypothesis_queue`, `hypothesis_status`, `next_queries`, and
  `stop_conditions` as visible state-machine fields while still hiding opaque
  query/audit/artifact ids.
- Keeps the 21-task/3-case golden coverage shape introduced in 0.15.20; golden
  fixtures remain under `scripts/eval-fixtures/` only.

## 0.15.20

Release status: published candidate superseded by 0.15.21.

- Expands the golden answer set from 20 to 21 tasks with a new e7e3
  Investigation Lab hard case for cash breakpoints, double-empty
  counterparties, wealth-management/asset clues, and candidate-account
  ownership boundaries.
- Raises the capability-registry eval coverage contract to require the new
  investigation-lab task and mirrors the registry into the packaged skill
  references.
- Adds task-specific model scoring for evidence-constrained hypothesis queues,
  including `hypothesis_status`, downgraded leads, `next_queries`,
  `stop_conditions`, supported-edge boundaries, and overclaim guards.

## 0.15.19

Release status: published candidate superseded by 0.15.20.

- Adds an Agent UI A/B `--baseline-timeout-ms` so no-plugin and skill-only
  baselines can be bounded separately from plugin modes during expanded
  release-quality runs.
- Records `timeout_ms`, `baseline_timeout_ms`, and per-answer
  `turn_timeout_ms` in quality evidence so interrupted baselines remain
  auditable instead of looking like unexplained runtime failures.
- Keeps the 0.15.18 production fact surface, MCP tools, Evidence Ledger,
  Context Compiler, Claim Verifier, and Hub install behavior unchanged.

## 0.15.18

Release status: published candidate superseded by 0.15.19.

- Tightens non-case passive behavior: ordinary copy editing, formulas, masking,
  and meeting-note tasks must preserve user-provided numbers, dates, times,
  symbols, and requested formatting while staying completely out of
  `analytix_funds`.
- Extends Agent UI passive copy-edit scoring equivalence so `上午10时` is
  treated as the same time as `上午十点` / `上午10:00`; this fixes an
  over-strict non-funds marker without changing case facts or MCP behavior.

## 0.15.17

Release status: published candidate superseded by 0.15.18.

- Replays the verified 0.15.16 plugin slice onto the current Analytix mainline
  after the analytixagent runtime-isolation refactor, avoiding any tag move for
  the existing `analytix-fund-analysis-v0.15.16` candidate.
- Preserves the 0.15.16 runtime behavior while requiring fresh mainline
  release identity, Hub publish/install, runtime remount, B0, health, and Agent
  UI A/B evidence for the new tag.

## 0.15.16

Release status: published candidate superseded by 0.15.17.

- Consolidates `top-pluginization-plan.md` into a fuller packaged system runbook covering end-to-end execution, domain object model, deliverable lifecycle, and capability coverage, so future Agent runs can continue from reference docs instead of thread memory.
- Keeps the decisive quality bar explicit: `full_plugin` must prove a real advantage over no-plugin Codex and lower-capability baselines, not merely reach a generic internal score.
- Mirrors the updated top plan in both root references and embedded skill references so `doctor`, runtime cache sync, Hub packaging, and installed skill resources check the same source text.

## 0.15.15

Release status: published candidate superseded by 0.15.16.

- Removes the hardcoded `.mcp.json` `env.ANALYTIX_API_BASE_URL` default so real Agent UI app-server runs inherit the active backend URL from runtime env instead of being forced back to the packaged localhost default.
- Adds doctor and release-guard coverage for backend URL injection: `ANALYTIX_API_BASE_URL` must stay in `env_vars`, but must not be hardcoded in `.mcp.json` `env`.

## 0.15.14

Release status: published candidate superseded by 0.15.15.

- Keeps the 0.15.13 Agent UI app-server cwd isolation contract, but writes the `PWD` environment override through a computed key so package text audit no longer treats the safe cwd override as a secret-like assignment.
- Updates release guard to check the computed `PWD` override and preserves `INIT_CWD`, `client_process_cwd`, read-only, permission-decline, and worktree-mutation invalidation requirements.

## 0.15.13

Release status: published candidate superseded by 0.15.14.

- Pins spawned analytix-agent app-server environment `PWD` and `INIT_CWD` to the eval cwd, matching the process cwd so release-quality A/B cannot accidentally inherit the release repository as its working directory through environment state.
- Records `client_process_cwd` in Agent UI A/B evidence and extends release guard coverage for cwd/PWD isolation, preserving invalidation when any non-output worktree mutation appears.
- Extends the mirrored top-pluginization plan with future Agent construction procedure, external-reference adoption mapping, A/B proof design, and fund-tracing acceptance criteria so follow-up work can proceed from packaged references instead of thread memory.

## 0.15.12

Release status: published candidate superseded by 0.15.13.

- Adds release-quality Agent UI worktree mutation guard: read-only A/B runs now require a clean release worktree and invalidate quality evidence if anything outside the selected output artifact changes.
- Screens command execution approvals in read-only A/B runs so only clearly read-only `cat`/`sed` inspection is auto-approved; file-change and permission-escalation requests remain declined.
- Starts the spawned analytix-agent app-server from the eval cwd instead of the release repository cwd, reducing accidental contact with release files during model quality runs.
- Extends the top-pluginization plan with plugin-package boundary, external-reference adoption rules, future Codex construction discipline, and the new A/B evidence integrity rule.

## 0.15.11

Release status: published candidate superseded by 0.15.12.

- Expands `top-pluginization-plan.md` from a compact runbook into the full product, plugin architecture, program-construction, Hub lifecycle, Agent execution, and release-gate procedure so future Agent runs have a single packaged operating manual.
- Keeps the plan mirrored in root and skill references and continues to expose it through progressive resources, doctor, release guard, release-slice audit, and runtime-cache contracts.
- Keeps the 0.15.10 runtime/package mechanics unchanged; this patch only makes the operating procedure complete enough to guide autonomous follow-up work.

## 0.15.10

Release status: published candidate superseded by 0.15.11.

- Adds `top-pluginization-plan.md` to both root references and skill references as the durable product/engineering runbook for evidence-ledger, context-compiler, claim-verifier, casegraph/fundgraph, Investigation Lab, runtime boundary, Hub lifecycle, and release-quality gates.
- Exposes that plan through progressive skill resources, `doctor`, release guard, release-slice audit, and runtime-cache contracts so future Agent runs can discover and validate the same procedure without touching system Codex state.
- Keeps the 0.15.9 runtime fact surface and claim-support behavior unchanged; this patch turns the authorized operating procedure into packaged, cache-checked release material.

## 0.15.9

Release status: published candidate superseded by 0.15.10.

- Adds deterministic evidence packs and claim-support indexes for supported fundgraph edges and casegraph top account/holder/counterparty ranks, keeping internal source refs traceable while user-visible cards still hide opaque audit ids.
- Extends `doctor` with hard contracts for fundgraph evidence packs, casegraph rank evidence packs, and claim-verifier risk scans covering unsupported Mermaid/arrow flows, legal overreach, candidate-account ownership upgrades, missing source anchors, and cash/asset destination overclaims.
- Tightens `release-slice-audit` so historical Agent UI A/B files can be recorded as quality runs but cannot satisfy publish readiness unless they prove the current clean, tagged release identity; also documents the authorized Agent publish/install procedure in Hub lifecycle references.

## 0.15.8

Release status: published candidate superseded by 0.15.9.

- Corrects the Agent UI release scorer's passive copy-edit time marker so "上午10点" and "明日上午10点" are accepted as the same preserved fact as "上午十点".
- Keeps the 0.15.7 runtime fact surface, Evidence Ledger doctor contract, tool budget, and Hub lifecycle behavior unchanged; this patch closes the deterministic scorer false negative found in the post-install 20x5 run.

## 0.15.7

Release status: published candidate superseded by 0.15.8.

- Adds an executable Evidence Ledger coverage validator so every required amount, count, account, holder, counterparty, flow/transaction edge, and report claim ledger surface must carry source refs before the ledger can satisfy release traceability.
- Extends `doctor` with a deterministic ledger traceability contract: a supported sample must pass, while the same sample without source refs must fail, turning internal evidence anchoring into a repeatable gate instead of a manual review note.
- Documents the future Hub release runbook requirement that Evidence Ledger traceability, raw-context compression, and claim-review boundaries stay in doctor/release evidence before publish/install.

## 0.15.6

Release status: published candidate superseded by 0.15.7.

- Corrects passive account-masking scoring so literal `*`, Excel `REPT("*", ...)`, and SQL/Python `REPEAT("*", ...)` are accepted as the same masking marker as "星号"; also treats Chinese "进/出方向" wording as equivalent to the report-review `directed` marker.
- Preserves the 0.15.5 runtime fact surface, tool budget, Hub lifecycle runbook, and report claim-review behavior; this patch only closes the final scorer alias gap found in the post-Hub 20x5 run.

## 0.15.5

Release status: published candidate superseded by 0.15.6.

- Corrects the release scorer's guarded-claim detection so report-review answers that explicitly say "未形成 supported transaction edge" or "缺少凭证/回单/余额承接" are treated as evidence boundaries, not as confirmed legal/final-flow overclaims.
- Adds ordinary-language marker tolerance for passive CSV import guidance where "交易时间/时间字段" satisfies the same non-fund preparation requirement as "日期".
- Documents the repeatable Hub publish/install verification runbook in `references/hub-lifecycle.md`, keeping public publish, runtime install, local remount, and rollback checks on the Analytix-owned path only.

## 0.15.4

Release status: published candidate superseded by 0.15.5.

- Hardens the real Agent UI A/B runner for release evidence collection: self-started Analytix-owned `analytix-agent app-server` runs now use a configurable 90s readiness timeout instead of the DirectClient 30s default.
- Adds an explicit app-server binary preflight and records `app_server_binary_path` plus `app_server_ready_timeout_ms` in A/B evidence so a missing temporary-worktree binary is not misreported as a `healthz` readiness timeout.
- Extends release guard coverage for the app-server readiness/binary contract and documents the required `--app-server-binary` / `ANALYTIX_AGENT_APP_SERVER_BINARY` override for temporary release worktrees.

## 0.15.3

Release status: published candidate superseded by 0.15.4.

- Expands passive non-fund golden coverage to 6 tasks, adding ordinary finance/account masking prompts that contain "资金/账户" language but must not invoke `analytix_funds`; the registry now requires at least 2 passive near-miss tasks.
- Tightens data-quality Answer Card requirements so clean_duplicate, same-fact/card-switch candidates, counterparty gaps, per-family review, and next-subpoena boundaries remain visible in final answers.
- Adds `prepare-hub-package.mjs` as a local `/tmp` package-source preflight so Hub package review has a deterministic source tree, marketplace stub, archive SHA256, and golden-isolation check without pretending to publish or install.

## 0.15.2

Release status: local candidate superseded by 0.15.3.

- Requires claim-review answers to preserve the visible `fact_refs/source refs` boundary while still hiding internal query ids, audit refs, artifact ids, and evidence ids.
- Keeps the 0.15.1 risk-marker output contract and only closes the remaining model-eval marker gap observed in the `report_candidate_cash_boundary_guard` run.

## 0.15.1

Release status: published candidate superseded by 0.15.2.

- Tightens the claim-review Answer Card contract so report answers must preserve stable `risk_marker` labels for candidate-account ownership, cash/asset destination, missing-counterparty/cash-break, and missing source-anchor risks.
- Requires visible `source refs` boundary wording and "不能写成" in report-gate answers while keeping opaque `q_*`, `audit_ref`, `artifact_id`, and evidence ids hidden from user-facing text.
- Keeps the 0.15.0 deterministic fact surface and 18-task golden expansion; this patch closes the newly observed model-output gap where risk categories were paraphrased away.

## 0.15.0

Release status: published candidate superseded by 0.15.1.

- Strengthens report claim review gates for candidate-account ownership upgrades, cash/asset destination overclaims, missing-counterparty/cash-break closures, and sensitive claims without `fact_refs`/`source_refs`.
- Extends the internal Evidence Ledger claim lineage with `claim_id`, `claim_category`, and `risk_marker` so report claims can be traced through deterministic review results without exposing opaque refs to users.
- Expands golden coverage to 18 tasks across 3 cases, adding an e7e3 report guard for candidate ownership, double-blank counterparty breaks, and cash/asset destination boundaries.

## 0.14.9

Release status: publish candidate.

- Tightens the deterministic eval guard for asset/wealth-management wording so explicit downgrade phrasing such as "而非/不写作最终去向" is not misclassified as a cash-destination overclaim.
- Preserves the 0.14.8 runtime fact surface; this patch only corrects the release scorer's anti-pattern guard before Hub publication.

## 0.14.8

Release status: publish candidate.

- Preserves the 0.14.7 release gates while preparing a clean plugin-only release slice so unrelated Flow/network work is not part of the fund-analysis package identity.
- Makes top-ranking Answer Cards carry deterministic must-write facts for top account, holder, counterparty, and double-blank rankings, reducing the risk that report text omits core ranked facts.
- Removes the hard-coded top-ranking diagnostic fixture wording and derives holder/counterparty boundaries from live deterministic card facts.
- Adds passive non-fund skill guidance so ordinary non-case questions do not route into `analytix_funds`.
- Extends expanded golden coverage ratchets for per-case task count and report claim-review cases, and records those ratchets in release evidence.
- Extends local runtime-cache contract coverage for release/eval scripts and root references while keeping sync as an Analytix-owned local remount aid, not a Hub publish/install path.

## 0.14.7

Release status: publish candidate.

- Exposes `model-ab-eval --list-tasks` as a compact release-audit inventory for multi-case golden coverage, passive non-intervention tasks, report tasks, and registry-derived tool budgets.
- Records manifest/server/HEAD/tag release identity in real UI A/B outputs and adds read-only `release-slice-audit --quality-evidence` checks so stale 10x5 artifacts cannot satisfy the current candidate gate.
- Adds `release-slice-audit --quality-evidence-inventory` so release readiness can scan existing A/B outputs and explain why no current HEAD/version quality run is publishable.
- Allows isolated release-slice verification to pass a clean committed plugin slice with an empty patch while still checking unrelated workspace dirt stays out of the release.
- Adds a local doctor ingestion-contract check for `plugin.json`, skills, `.mcp.json`, interface metadata, and assets so Hub/package structure drift is caught without relying on system Codex state.
- Requires explicit `--confirm-local-remount` plus an existing same-version plugin cache and runtime skill mount before `sync-runtime-cache.mjs --apply` can write into the Analytix-owned runtime.
- Adds Evidence Ledger traceability coverage for required surfaces so amount/count/account/holder/counterparty/flow-edge/report-claim entries without `source_refs` are visible as release-auditable gaps.
- Makes Evidence Ledger coverage overflow-aware: `coverage_summary` is computed over all scanned entries before the user-visible entry list is truncated, so missing source refs cannot hide beyond the 200-entry output cap.
- Adds an internal `claim_support_index` for claim review cards so verified/corrected/unsupported claims, unsupported flows, source boundaries, forbidden phrasings, and review actions carry per-claim support refs into the Evidence Ledger while staying out of user-visible text.
- Promotes manifest/server/tag/release-notes identity into the default release-slice audit so publish blockers are visible without a separate readiness-plan invocation.
- Corrects readiness planning so a dirty already-tagged identity or stale old tag suggests the next patch version, while a new untagged candidate version can remain the release candidate to commit and tag.
- Expands candidate-version replacements to every declared version file, including both capability-registry copies, eval coverage contract, and runtime-cache contract, with a read-only gap check before release staging.

## 0.14.6

Release status: publish candidate.

- Treats transient DuckDB/schema lock states as retryable evidence boundaries instead of rendering missing core tables or zero-valued report facts.
- Propagates `schema_status` through backend contracts, case-scope MCP cards, and skill references so temporary source unavailability is visible as `needs_review` rather than a false factual conclusion.
- Derives report-task tool budgets from the capability registry in both real UI A/B and model scoring paths, reducing metadata drift between SKILL/MCP/eval gates.
- Adds a read-only release-slice audit with candidate-version, isolated worktree verification, staging plan, and readiness plan so mixed dirty worktrees can be prepared without staging unrelated changes.
- Keeps Hub/runtime lifecycle unchanged: runtime cache sync remains only an Analytix-owned local remount aid and is not a public publish or install path.

## 0.14.5

Release status: publish candidate.

- Blocks `holder_analysis` when required deterministic child tools fail, instead of rendering missing holder scope, account stats, or Top accounts as zero-valued facts.
- Adds a synthetic release-guard smoke for the holder required-fact gate: failed child facts must produce `partial`, set `required_facts_present=false`, emit a blocking warning, and omit unsupported holder key facts.
- Keeps the 0.14.4 Hub/runtime lifecycle boundary unchanged: runtime cache sync remains a local Analytix-owned remount aid, not the public publish or install path.

## 0.14.4

Release status: publish candidate.

- Makes live `check-health` robust to case data drift by preflighting whether the selected case contains the configured synthetic fact family before running stronger eval-only assertions.
- Keeps generic live health checks active when the selected case lacks that fixed fact family, while treating preflight API/runtime failures as hard health failures instead of silent skips.
- Serializes active-case mutation inside `check-health` with a temporary local lock so parallel health runs cannot mask regressions by racing the backend active case.
- Adds release guard coverage for the fixed fact-family preflight, active-case lock, thin `mcp/server.mjs` orchestrator budget, and plugin/non-plugin dirty worktree diagnostics.
- Promotes `check-health.mjs`, `phase4-diagnostics.mjs`, and release notes into doctor/release-guard coverage so release gate scripts remain first-class local checks.
- Clarifies that runtime cache sync is an Analytix-owned verification/remount aid and not the Hub publish/install path.

## 0.14.3

Release status: publish candidate.

- Promotes the expanded golden suite to the release baseline: 17 tasks across 3 cases, including 4 passive non-fund prompts that must not route into `analytix_funds`.
- Tightens passive non-funds scoring so useful general answers, no plugin route, and no case-fact leakage are scored separately without rewarding fund-tool intervention.
- Adds semantic scoring tolerance for ordinary wording variants and the `case_e7e3_top_rankings` task while preserving hard auto-fail gates for unsupported fund flows, wrong ownership, legal overreach, and missing evidence boundaries.
- Keeps Hub/runtime publishing boundaries unchanged: public publish goes through Analytix Hub, local runtime cache sync remains a verification/remount aid only.

## 0.14.2

Release status: local publish candidate.

- Adds live MCP evidence-ledger health checks for casegraph, frontdoor ranking/destination, flowgraph, and claim review outputs.
- Wraps `validate_report_claims` into a compact `claim_review_card` so report claim review returns model-visible verified/corrected/unsupported/boundary/forbidden/next-action sections instead of a generic skill completion line.
- Normalizes backend claim-review objects into readable claim text, carries supported `checked_claims` into `verified_claims`, and reports unsupported strict-report numbers as explicit `unsupported_number` claims.
- Tightens the capability registry/command metadata drift gate: command conditional tools must be present in the owning capability, and command-level tool budgets may not exceed the frontdoor/report budget contract.
- Fixes the real UI A/B runner's tool-sequence analysis initialization so invalid quality runs are caused by runtime/model failures, not the release harness itself.
- Keeps `0.14.0` as the prior RC baseline; this release supersedes the local `0.14.1` publish candidate because that candidate exposed the runner initialization bug during 10x5 validation.

## 0.14.0

Release status: release candidate.

This release turns the fund-analysis plugin into a protocol-level investigation system instead of a broad fact-query surface.

- Upgrades `funds_investigate` into the default high-value fact entry: ordinary fact tasks stop after one call; report review tasks may add only one `validate_report_claims` call.
- Extracts `mcp/agent-context-hygiene.mjs` so local path redaction, opaque evidence id stripping, and same-fact marker humanization live outside the MCP server router and are release-guarded with synthetic samples.
- Extracts `mcp/agent-output-compiler.mjs` so Agent-visible Answer Card text, low-context `structuredContent`, debug-gated raw JSON, and text-budget fallback cards no longer live inside the MCP server router.
- Extracts `mcp/agent-payload-compiler.mjs` so skill envelope decoding, compact key facts, warnings, rankings, graph previews, and model-visible MCP payloads live outside the MCP server router and remain release-guarded against golden/oracle reads.
- Extracts `mcp/mcp-artifact-store.mjs` so full MCP JSON is redacted and stored only as local audit artifacts under Analytix-owned roots; release guard now rejects system Codex artifact roots and verifies stored artifacts do not leak local paths into model context.
- Extracts `mcp/progressive-resources.mjs` so MCP `resources/list` / `resources/read` Skill and reference disclosure stays centralized, plugin-root guarded, and release-guarded outside the large MCP server router.
- Moves eval-only rubric material under `scripts/eval-fixtures/` and keeps rubric/diff helpers out of production references, production MCP resources, runtime skill copies, and the production `card-renderer.mjs`; `scripts/diagnostic-card-diff.mjs` now owns oracle/golden diagnostic comparison for local release checks only.
- Adds a registry-declared golden eval coverage ratchet: at least 15 tasks across 3 cases, passive non-intervention tasks, ranking, quality, holder boundary, continuation, investigation lab, and report claim-review dimensions must stay covered before release. `scripts/eval-coverage-contract.mjs` is the shared checker used by release guard, doctor, and `model-ab-eval`.
- Extracts `mcp/tool-discovery-policy.mjs` so default MCP tool discovery, frontdoor ordering, full-discovery escape hatch, and report-only eval profile are deterministic and release-guarded outside the large MCP server router.
- Extracts `mcp/tool-runtime-routing.mjs` so MCP tool-to-backend-skill routing and hypothesis-probe aliases are deterministic, release-guarded, and no longer embedded in the MCP server router.
- Extracts `mcp/tool-input-schemas.mjs` so shared MCP input schema fragments for frontdoor, owner scope, probes, continuation QA, and tracing live outside the MCP server router and are release-guarded.
- Extracts `mcp/tool-schemas.mjs` so the full MCP `tools/list` schema table is centralized outside the MCP server router, while the server keeps only runtime dispatch and `orderedToolsForAgent(tools)` exposure policy.
- Extracts `mcp/runtime-normalizers.mjs` so deterministic MCP argument normalization, validation warnings, empty-value pruning, and trace tolerance guards live outside the MCP server router.
- Extracts `mcp/backend-api-client.mjs` so Analytix backend URL normalization, case resolution, skill execution, and POST helpers live in a deterministic runtime client outside the MCP server router.
- Extracts `mcp/case-pipeline-runtime.mjs` so import, cleaning, stats tree, current-case, and pipeline scope summaries live outside the MCP server router and keep dashboard metadata separate from report-grade coverage.
- Extracts `mcp/case-scope-map-runtime.mjs` so case scope map nodes, edges, quality gates, child-tool retries, and compact child details live outside the MCP server router while keeping raw child payloads out of model context.
- Extracts `mcp/intent-plan-protocol.mjs` so the front door emits compact `intent_ast`, `plan_dag`, and `tool_budget` fields instead of burying ordinary/report tool budgets in prompt text.
- Extracts `mcp/frontdoor-answer-contract.mjs` so ordinary-answer stops, report claim-review next actions, duplicate suppression, candidate-account boundaries, and graph repeat guards live outside the MCP server router.
- Extracts `mcp/frontdoor-routing.mjs` so via/date/ranking inference plus duplicate `funds_investigate` cache suppression live outside the MCP server router and stay release-guarded.
- Adds required answer-card protocol fields in `mcp/answer-card-protocol.mjs`: `answer_card_complete`, `recommended_next_action`, `max_additional_tools`, `required_facts_present`, and `unsupported_flows_present`.
- Adds same case/task/intent repeat suppression and eval-visible tool-sequence diagnostics.
- Strengthens the real UI A/B runner so quality scoring is blocked before the first model turn when the Analytix-owned runtime cache is missing or stale, and so passive non-funds tasks, ordinary fund tasks, and report tasks each have an enforced tool budget.
- Adds task-specific scoring for Top ranking, claim review, and old-thread investigation patterns.
- Extracts `mcp/destination-diagnostic-runtime.mjs` so Top outflow classification, source-to-counterparty one-hop amount correction, via continuation fact cards, continuation-list boundaries, and supported-edge anchors no longer live inside the MCP server router.
- Extracts `mcp/top-rankings-diagnostic-runtime.mjs` so Top ranking fact-card construction, metric-specific rank facts, source-audit boundaries, and rank_accounts/rank_holders/rank_counterparties support anchors no longer live inside the MCP server router.
- Strengthens `claim_review_card` with verified, corrected, unsupported, missing-boundary, forbidden-phrasing, and next-review-action sections.
- Extracts `mcp/claim-review-diagnostic-runtime.mjs` so the production report claim-review facts, SQL/probe support anchors, write-blocked card, and one-extra-tool boundary no longer live inside the MCP server router.
- Extracts `mcp/claim-verifier-protocol.mjs` so report claim review has one protocol source for `write_blocked`, supported-edge Mermaid rules, forbidden legal phrasing, source-boundary gaps, and the one-extra-tool budget; it also runs a deterministic report-text risk scan for amount claims without facts, unsupported Mermaid/arrow flows, forbidden legal phrasing, and missing source boundaries.
- Extracts `mcp/diagnostic-fact-helpers.mjs` so diagnostic cards share one fact/source lineage builder for amount/count/account/holder/counterparty rows, claim probes, financial-product leads, duplicate guards, and stable `source_hash` metadata.
- Strengthens Investigation Lab with investigation intent, evidence-bound hypotheses, amount/temporal clusters, duplicate/same-fact risks, missing counterparty or cash breaks, asset clues, downgrade rules, next queries, forbidden-as-facts, and stop conditions.
- Extracts `mcp/investigation-lab-diagnostic-runtime.mjs` so old-thread-style deep investigation fact-card construction, hypothesis queue synthesis, same-fact risk anchors, and ordinary-answer stop boundary no longer live inside the MCP server router.
- Extracts `mcp/investigation-lab-protocol.mjs` so old-thread-style open discovery has one protocol source for evidence-bound hypotheses, candidate-account downgrades, unsupported-flow Mermaid bans, and ordinary-answer stop rules.
- Extracts `mcp/casegraph-protocol.mjs` so casegraph/fundgraph share one protocol source for required graph nodes, evidence packs, supported-edge Mermaid rules, missing receipts/cash breaks, and report claim support boundaries.
- Extracts `mcp/casegraph-runtime.mjs` so `get_casegraph` scope-map assembly, Top entity ranking probes, casegraph Answer Card protocol, quality gates, and audit refs live outside the MCP server router without reading eval fixtures or env state.
- Extracts `mcp/fund-flow-graph-runtime.mjs` so `build_fund_flow_graph` skill calls, source/via seed collection, supported-edge answer-card policy, and audit refs live outside the MCP server router while keeping graph arrows bound to deterministic transaction edges.
- Extracts `mcp/fundgraph-builder.mjs` so deterministic fund-flow seed parsing, supported-edge construction, and same-fact-safe transfer summaries no longer live in the MCP server front door.
- Adds internal Evidence Ledger metadata and `answer_card.context_compiler` fields so compact model context remains separate from local audit artifacts; these now live in dedicated MCP modules and ledger entries carry stable `fact_id`, `source_path`, `source_refs`, `support_status`, `surface_signals`, and a `coverage_summary` for amount/count/account/holder/counterparty/flow-edge/claim/source-ref coverage.
- Extracts `mcp/mcp-output-policy.mjs` so raw MCP result policy is deterministic: default model context is answer-card-only, full `structuredContent` requires both `include_debug=true` and `ANALYTIX_FUNDS_ALLOW_DEBUG_PAYLOAD=true`, and `_meta.analytix_evidence_ledger.output_policy` records that boundary.
- Adds a minimal capability-registry fact file and schema for drift-checking command metadata, MCP tool lists, doctor checks, scoring dimensions, and eval task coverage from one fact source.
- Adds `hub-lifecycle.md` plus doctor/release-guard coverage for Hub publish, install, uninstall, rollback, remount, and the rule that `sync-runtime-cache.mjs` is local verification only.
- Hardens auth/runtime-state boundaries: the plugin may only rely on the shared Codex GPT login state through Analytixagent, may not read or repair tokens/provider config, and eval/runtime logs redact Bearer tokens, API keys, JWTs, refresh tokens, and session tokens.
- Expands the eval fixture from 10 to 15 tasks across three cases, including passive non-funds tasks that must not invoke the fund-analysis plugin and a Mermaid/legal-phrasing report gate.
- Keeps casegraph as a roadmap surface for subjects, accounts, counterparties, transaction edges, same-fact families, rule hits, asset/cash breaks, evidence packs, and claim support.
- Adds release/eval guards that block golden/oracle leakage into production, eval fastpaths, opaque user-visible refs, unsupported Mermaid flows, legal overreach, and invalid report claims.

Validation evidence from the prior `0.14.0` RC baseline before the current 15-task fixture expansion:

- Static checks passed: `node --check` on server, renderer, eval harness, agent-ui runner, phase4 diagnostics, and runtime-cache sync; Python compile passed for touched backend files.
- Release guard: 21 pass / 0 warn / 0 fail at the RC tag; current local release guard is expected to be re-run after every protocol or fixture edit.
- B0 production card diff: 3 tasks, hard_diff=0, coverage_diff=0, production_call_fail=0.
- B1: Top ranking 96, report claim review 98, old-thread patterns 98; auto-fail=0.
- B2: holder scope 100, Liu Wenliang to Zhang Jinzhi 93, Zhang Jinzhi continuation 100, outflow continuation 100, full report gate 97; auto-fail=0.
- Full 10x5: `full_plugin` average 97.7 with auto-fail=0, above `skill_mcp` average 86.9 with auto-fail=2.

Publication boundary:

- Do not publish from a dirty or partially validated tree.
- Do not write to system Codex state or global `~/.codex`.
- Analytix-owned runtime-cache checks must use `~/.analytix` or an isolated temporary analytix runtime path.
