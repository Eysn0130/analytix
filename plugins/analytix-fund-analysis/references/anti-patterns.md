# Anti-Patterns

Use this file when reviewing Agent output, `/analytix qa`, golden QA failures, or any task where the plugin may be making Codex worse instead of better.

## Blocking Anti-Patterns

- **P0 full-case/report bypass**: calling or simulating the hidden
  `run_full_case_analysis` entry, sweeping weaker tools to replace it, or
  producing a full-case/report draft instead of the fixed capability/evidence
  boundary.
- **Unverified source substitution**: treating weak sources, scratch
  calculations, unreviewed query output, bounded previews, or local artifacts as
  source-backed case facts. This breaks source guardrails, validation state, and
  the Analytix plugin isolation boundary.
- **Raw-table bypass**: answering ordinary investigation questions from `fc_*_raw`, source files, source detail rows, or uncleaned records instead of MCP-backed cleaned tables and analysis indexes.
- **Bounded-row totals**: summing `query_*_rows` samples and describing them as all-case totals, account totals, or holder totals.
- **Coverage language without coverage**: writing `全量`, `全部`, `覆盖`, `最大`, or `Top` without `get_scope_coverage`, rank tools, reconciliation, and source audit.
- **Dashboard metadata as coverage**: treating `get_current_case.case.case_dashboard_stats` counters as analysis account/holder/transaction coverage. Use `get_case_scope_map.coverage`, `get_scope_coverage`, or rank output for report-grade counts.
- **Candidate ownership upgrade**: treating `candidate_accounts` from owner resolution as confirmed person-owned accounts.
- **Partial source audit ignored**: saying the report is final when `audit_unindexed_sources.audit_status` is `needs_review` or transaction-looking non-pipeline tables are found; if `schema_status=temporarily_unavailable`, converting retry placeholders into missing-table or unindexed-source facts is also forbidden.
- **Claim validation skipped**: final report or appendix claims are not checked by `validate_report_claims` or `validate_continuation_list`.
- **Capability-gap over-hardening**: treating every semantic-tool gap as a
  reason to refuse analysis until a new MCP tool is built, even when the current
  task could be answered through audited Case Workbench.
- **Runtime self-upgrade fantasy**: implying the plugin can automatically add
  MCP tools, change backend schemas, publish a new version, or upgrade the
  installed runtime during an ordinary investigation answer.
- **Case-source guessing loop**: when current case-project resolution fails, searching
  local directories, `*.duckdb`, case folders, history, fixtures, or cached
  output for a `case_id`, then retrying rank/profile tools. Return a Case Source
  Blocker and recovery actions instead; an explicit invalid `case_id` is final
  for that turn.
- **Pair Amount owner bypass**: answering `A 转给 B 多少钱` or amount-basis
  disputes from `quick-fact`, `rank_counterparties`, `funds_investigate`
  answer drafts, or one workbench preview instead of `pair-amount-investigation`
  final reconciliation.
- **Support draft as final answer**: copying answer-card titles, `answer_draft`
  prose, workbench summaries, or audit checklists as the user-facing
  investigation answer instead of writing conclusion, evidence, abnormality,
  case significance, limits, and next evidence.

## High-Risk Interpretation Errors

- **Blank detail holder != unregistered account dimension**: transaction detail rows may have blank counterparty/holder fields; that is not the same as `analysis_account_dim.open_name` being empty.
- **Financial product mistaken as cash or ordinary transfer**: missing counterparty rows with financial-product keywords or known rules must pass through `classify_missing_counterparty_business`.
- **Duplicate cleaning over-trust**: `clean_duplicate=0` only means the configured exact duplicate rule did not remove rows; it does not disprove same-fact cross-account duplicates caused by card replacement or bank-system representation differences.
- **Card replacement collapse without proof**: same amount, balance, time, counterparty, and business text can indicate duplicate same-fact rows, but the plugin must mark it as a merge candidate until a deterministic merge policy or investigator confirmation exists.
- **Full-case blind dedupe**: deleting or deducting transactions across the whole case just because time, amount, balance, direction, counterparty, and summary match. Duplicate filtering must be at least scoped to the same holder/person account set or an explicit investigator-confirmed account set.
- **Unit drift**: never convert yuan to 万元 mentally in prose without preserving the original yuan amount. Use deterministic arithmetic: `万元 = 元 / 10000`, preserve the original exact amount in tables, and keep prose rounding separate from report-grade numbers. Concrete regression examples belong in eval fixtures, not production references.
- **Endpoint overclaim**: a Top outflow with no verified downstream is a supplementary investigation target, not a proven final destination.
- **Contact/address overclaim**: shared phone, shared address, employer, or legal representative is written as actual control, nominee holding, ownership, conspiracy, or fund-flow proof.
- **Device/IP/MAC overclaim**: shared IP, MAC, teller, branch, location, merchant, terminal, or receipt pattern is written as actual operator, controller, organized gang, co-offending, or same physical person.
- **Platform/virtual-asset overclaim**: payment-channel, merchant, wallet, OTC, or exchange-like clue is written as confirmed platform control, virtual-asset transfer, gambling settlement, laundering, or underground banking without platform/KYC/order/wallet evidence.
- **Total/classified mix-up**: all company-to-person payments are described as wages, labor, reimbursement, travel, or subsidy because some rows contain those keywords. Split total receipts, identifiable subset, and unclassified remainder.
- **Cost normality skipped**: project material, labor, logistics, tax, or loan-repayment flows are treated as suspicious without first separating ordinary cost appearance from later intersections with core persons, related companies, cash, assets, or return flows.
- **Task feedback ignored**: failed or partial bank feedback is ignored when making negative findings. Failed feedback is a coverage gap, not proof that no account or relationship exists.
- **Visual evidence overclaim**: Top tables, heatmaps, dashboards, or graph-like visuals imply a supported transaction path, final destination, actual control relation, asset ownership, or legal conclusion.
- **Visual scope missing**: a chart, table, appendix, or dashboard card lacks object scope, unit, time window, metric, direction, or evidence status, so the reader cannot audit what it actually proves.
- **Fact-correct but no investigation**: the amount, ranking, or table is
  technically correct, but the answer lacks abnormal features, case meaning,
  unsupported limits, and proof actions.
- **Required-source gap hidden**: a missing required source lane is treated as
  complete evidence, or an optional lane such as contact/address/IP/MAC/task
  feedback is missing but the answer says no lead exists.

## Tool-Sprawl Anti-Patterns

- **All-tools sweep**: running most MCP tools because the question is broad. Use `plan_case_analysis` to get lane budgets and allowed tools.
- **Ranking by profile tool**: using `get_account_stats`, `analyze_account_full`, or bounded rows to infer all-case Top accounts. Use `rank_accounts`, `rank_holders`, or `rank_counterparties`.
- **Lab without follow-up**: running `run_investigation_lab` but not following the highest-priority cards with trace/probe/validation.
- **Probe without hypothesis**: calling `hypothesis_probe` with vague parameters and no holder/account/date/keyword focus when the task supplies specific investigative intent.

## Corrective Pattern

For complex or open-ended investigative tasks, use:

```text
get_current_case
-> plan_case_analysis
-> audit_unindexed_sources
-> audit_case_data_quality
-> resolve_duplicate_families
-> run_investigation_lab
-> targeted rank/trace/probe/classify/validate from the strongest cards
-> validate_report_claims or validate_continuation_list
```

This preserves Codex autonomy while forcing every factual claim through the Analytix deterministic fact engine.
