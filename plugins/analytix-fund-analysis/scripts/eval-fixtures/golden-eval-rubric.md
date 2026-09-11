# Golden Eval Rubric

Use this rubric for DeepSeek/OpenAI smoke reviews and manual comparison against old free-exploration threads. The goal is not to make the answer identical to an old thread; the goal is to prove the plugin produces stronger, lower-context, more auditable investigation output.

For live GPT5.5 A/B reviews, run `scripts/model-ab-eval.mjs` to print the prompt suite, fetch deterministic fact anchors, and score saved model answers. Fixed expected facts are eval-only and must stay outside production skills, MCP resources, and references.

## Score: 100 Points

| Dimension | Points | Passing Standard |
| --- | ---: | --- |
| Correct route and gates | 15 | Starts with case resolution, source/data-quality audit, reconciliation/coverage, plan/lab when complex, and no raw DuckDB/SQL. |
| Deterministic facts | 20 | Amounts, counts, Top rankings, holder/account scopes, and date ranges come from MCP aggregate/profile/rank tools; bounded rows are evidence only. |
| Source and cleaning skepticism | 15 | Distinguishes import scope, normalized detail scope, analysis index scope, holder tree scope, unindexed sources, same-fact duplicates, same-holder duplicate families, and card-replacement candidates; rejects full-case blind dedupe. |
| Old-thread-style discovery | 20 | Surfaces high-value irregularities such as financial-product misclassification, named continuation leads, duplicate transaction ids, missing-counterparty rows, amount concentrations, and keyword links. |
| Claim discipline | 15 | Uses labels `数据事实` / `统计特征` / `可疑特征` / `线索` / `需复核` / `不可判断`; no unsupported legal or ownership conclusions. |
| Tool efficiency | 5 | Ordinary answers use the smallest sufficient semantic fact tools, avoid all-tools sweeps, and reserve `funds_investigate` for fuzzy navigation; report wording may add one validation call. |
| Output usefulness | 10 | Provides prioritized next investigative actions, concrete transaction/fact context, and clear limits that investigators can act on; does not expose opaque `q_xxx/audit_ref/artifact_id` ids. |

## Semantic Fact Toolbox Requirement

For natural-language tasks with a clear analytical intent, the preferred route is to call the target semantic fact tool directly: current-case/scope tools for case context, rank tools for rankings, profile tools for holder or account analysis, trace tools for continuation and fund-flow questions, `hypothesis_probe` for investigative hypotheses, and quality/validation tools for data or report checks. `funds_investigate` is a navigator and lightweight route hint for fuzzy or ambiguous tasks; it must not monopolize ranking, profile, tracing, hypothesis, quality, or report validation work.

- Default MCP text output must stay compact: target under `6000` chars for broad tasks, with human-readable answer card, key facts, quality boundaries, and continuation options as the primary model context. Do not feed large JSON to the model by default.
- Opaque ids (`q_xxx`, `audit_ref`, `detail_ref`, `artifact_id`, `evidence_refs`) must not appear in model-visible text or default `structuredContent`. `include_debug=true` is disabled by default in production.
- A model answer is downgraded if it reads the root `SKILL.md` through shell, scans the local plugin files for facts, or lets large JSON replace investigation planning.

## Bad-Thread Replay

Replay these tasks against the case under review, especially for the previously poor thread pattern `019e5fdc`:

1. `对合成主体甲名下账户进行分析` -> use `analyze_holder_full(holder_name="合成主体甲")`, then targeted rank/trace tools only if the answer needs them.
2. `资金转给谁了` -> use `trace_subject_top_outflows(holder_name="合成主体甲")` or `rank_counterparties` when the requested grain is a ranking.
3. `转给合成主体乙后，合成主体乙又将钱给了谁` -> use `trace_fund_next_hop(holder_name="合成主体乙")` with the prior supported flow as context.
4. `画出资金流向图` -> use `build_fund_flow_graph(holder_name="合成主体甲", via_holder_name=...)` and draw only returned supported transaction edges.

Passing requires: case identity is locked, text output is compact, every amount/path comes from deterministic facts or supported transaction edges, `42M aggregate vs transaction/account-scope ambiguity` is not flattened when present, and no unsupported graph arrows are invented.

## Automatic Fail

- Amounts, paths, rankings, or report claims written from weak sources, unverified calculations, raw/source rows, or missing delivery state.
- Final all-case report when a blocking gate is partial/error or report validation fails.
- Candidate accounts written as confirmed ownership.
- Top/ranking claims inferred from samples.
- Omission of mandatory investigation-lab cards in full-case answers.
- Mermaid or flow diagrams containing an edge not returned by `build_fund_flow_graph.flow_graph.edges`.
- Tool-visible text dumps full nested JSON for a broad task when compact output would suffice.

## Minimum Commercial Bar

- `90+`: high-quality diagnostic result, but not sufficient for release by itself.
- `80-89`: usable internal analysis; must list residual risk.
- `70-79`: investigation memo only; not report-grade.
- `<70`: fail; rerun with corrected route and anti-pattern review.

Scores are diagnostic bands. Release readiness still requires current-version
Agent UI/runtime A/B evidence, runtime cache preflight, real frontdoor turns,
source-backed user-visible answers, and no automatic fail.

## Comparison To Old Threads

The plugin beats an old free-exploration thread only when it preserves these old-thread strengths and makes them safer:

- Codex still forms hypotheses and follows surprising leads.
- Deterministic tools reduce hallucinated totals and unit mistakes.
- Source, cleaning, duplicate, and holder-scope risks are explicit before final wording.
- Same-fact duplicate filtering is scoped to same holder/person or explicit account sets, with candidate deltas kept separate from confirmed totals.
- The answer reaches old-thread-level details with fewer context tokens and a reproducible evidence trail.

## A/B Pass Rule

Run the required focused modes with the same model and reasoning effort:

- `plugin_disabled`: no plugin, baseline old-thread-style freedom with direct data-source verification required.
- `full_plugin`: focused skill product surface plus MCP semantic fact toolbox, casegraph context, evidence language, and report claim review only for explicit report tasks.

Optional diagnostic modes may be added, but they do not prove release quality:

- `skill_only`: no MCP, only the short Analytix investigation workflow discipline; direct data-source verification is still required.
- `skill_mcp`: short skill plus the compact MCP fact engine; source, validation, and delivery boundaries must remain explicit.
- `plugin_enabled_passive`: plugin enabled but not commanded; passive non-funds tasks must not call Analytix tools.

The plugin passes only when `full_plugin` has no automatic fail, scores at least 80, and beats `plugin_disabled` by at least 10 points on each focused product task: account dossier, subject dossier, full-case analysis, continue-one-hop, report builder, claim review, and passive nonintervention. If `full_plugin` is weaker than no-plugin Codex on free exploration, the current route must be rebuilt or disabled for that task class.
