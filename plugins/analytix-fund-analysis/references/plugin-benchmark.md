# Plugin Benchmark Notes

This is an engineering reference for Analytix-owned plugin design. It records
mechanisms to borrow and mechanisms to reject; it must not be copied into system
Codex configuration, hooks, or global state.

Popularity signals such as stars, forks, and rankings change over time and are
not release evidence. Use official docs or repository READMEs only to re-check
mechanisms when a future release needs a fresh comparison.

## Official Baseline

- OpenAI Codex Skills: build a reusable workflow first; skills use progressive disclosure, so Codex sees name/description/path first and loads `SKILL.md` only when needed.
- OpenAI Plugins: use a plugin as the stable distribution unit with `.codex-plugin/plugin.json`, bundled skills, MCP servers, apps, hooks, and assets where appropriate.
- Anthropic Agent Skills: keep `SKILL.md` as the core activation and workflow surface, move detailed references/assets/scripts out of the default context, and forward-test skills against realistic tasks instead of trusting a hand-written prompt.
- Eval Skills: product-grade skills need systematic evals, not subjective “looks better” reviews.

Analytix translation: short focused skills + compact MCP fact layer + casegraph pre-index + direct DuckDB oracle + front-door oracle + claim/delivery review. Do not use long prompts, full JSON, or fixed case answers as the main capability.

Current local mature-plugin source evidence:

- Data Analytics `0.1.49-2470779139f2`: `index`, `analyze-data-quality`, `validate-data`, `jupyter-notebooks`, `metric-diagnostics`, `visualize-data`, and `build-report` were re-read as source/preflight/validation/delivery baselines.
- Investment Banking `0.1.27`: router, `user-context`, `memo-builder`, and `ib-deck-qc` were re-read as admission, soft preflight, lead workflow, hero artifact, and QC baselines.
- Public Equity Investing `0.1.29`: router, `user-context`, `memo-builder`, and `deck-report-qc` were re-read as owner-selection, PM judgment, source posture, and QC baselines.
- OpenAI Codex manual sections for Agent Skills, Build Plugins, Plugins, and MCP were re-read through the official Codex manual helper.
- Anthropic Agent Skills official docs and the `anthropics/skills` skill-creator source were re-read for progressive disclosure, scripts/references/assets boundaries, and forward-testing discipline.

Release candidates must now run `node scripts/skill-creator-alignment-audit.mjs --json --fail-on-gaps` to record these source-availability and skill-structure checks.

## Projects To Learn From

| Project | Useful mechanism | Analytix decision |
| --- | --- | --- |
| [OpenAI Codex Skills](https://developers.openai.com/codex/skills) | Skills are reusable workflows loaded through progressive disclosure; the `description` is the implicit trigger surface and must be concise, scoped, and front-loaded. | Keep focused `SKILL.md` files short, explicit, and owner-oriented; move domain depth to one-hop references; audit frontmatter, section gates, links, and production-skill contamination through `skill-creator-alignment-audit.mjs`. |
| [OpenAI Codex Plugins](https://developers.openai.com/codex/plugins/build) | Plugins are the installable distribution unit for skills, MCP config, app mappings, assets, and lifecycle metadata. | Keep plugin identity, MCP manifest, Hub lifecycle, runtime cache, and release evidence aligned; never treat local remount or old output as a customer release. |
| [Anthropic Agent Skills](https://docs.anthropic.com/en/docs/agents-and-tools/agent-skills/overview) | Skills package instructions, scripts, and resources; progressive disclosure and deterministic scripts reduce context and execution drift. | Keep mechanical checks in scripts, domain detail in references, and current-case facts in MCP/evidence artifacts; forward-test skill behavior on real front-door tasks rather than adding static prompt text. |
| [openai/plugins](https://github.com/openai/plugins) | Official plugin structure, marketplace, skills, MCP examples. | Follow plugin packaging discipline, manifest identity, MCP boundaries, assets, and release validation. |
| [openai/codex-plugin-cc](https://github.com/openai/codex-plugin-cc) | Read-only review, adversarial review, status/result/cancel, tests. | Convert into report claim review and adversarial fund-investigation review. Do not borrow cross-agent hook behavior. |
| [colbymchenry/codegraph](https://github.com/colbymchenry/codegraph) | Pre-indexed local graph for fewer tokens/tool calls and faster context discovery. | Build Analytix `casegraph/fundgraph`: accounts, holders, counterparties, transaction edges, duplicate families, gaps, evidence packs. |
| [pbakaus/impeccable](https://github.com/pbakaus/impeccable) | Domain vocabulary, anti-pattern catalog, deterministic critique, LLM critique pass, tests. | Build fund-investigation terminology, anti-pattern lint, analysis critique, visual/report claim review, and eval auto-fail rules. |
| [Yeachan-Heo/oh-my-codex](https://github.com/Yeachan-Heo/oh-my-codex) | Workflow layer, state, plan/log surfaces, hooks, teams, doctor/runtime readiness. | Borrow Analytix-owned case journal, doctor, runtime health, and evidence handoff. Do not borrow global hooks/config/scheduler changes. |
| [obra/superpowers](https://github.com/obra/superpowers) | Agentic skill methodology: brainstorm, plan, execute, test, review, finish. | Use for complex full-case/report work: plan DAG, specialist lanes, verification, and finish criteria. Do not force every quick fact into a heavy process. |
| [alirezarezvani/claude-skills](https://github.com/alirezarezvani/claude-skills) | Large skill taxonomy, references/scripts organization. | Borrow classification and reference organization only; avoid big context catalogs. |
| [nyldn/claude-octopus](https://github.com/nyldn/claude-octopus) | Multi-perspective blindspot/adversarial review. | Borrow optional second-view review for reports; avoid default multi-model dependency. |
| [study8677/antigravity-workspace-template](https://github.com/study8677/antigravity-workspace-template) | Scope management and workspace knowledge organization. | Map to case scope graph and investigation journal. |
| [GanyuanRan/Aegis](https://github.com/GanyuanRan/Aegis) | Baseline-first, evidence-verified, drift-checked workflow. | Use baseline locks for case identity, data coverage, and report claim drift. |
| [boshu2/agentops](https://github.com/boshu2/agentops) | Memory, validation, feedback loops. | Build local Analytix case journal and eval feedback, not third-party telemetry. |
| [crystaldba/postgres-mcp](https://github.com/crystaldba/postgres-mcp) | Production-grade database MCP patterns: tool-only API, schema/object inspection, configurable restricted/unrestricted access, AST-backed safe SQL, forced read-only execution, query timeout, `EXPLAIN` plan review, slow-query/workload analysis, database health checks, and deterministic index-analysis algorithms that complement LLM reasoning instead of relying on it. Licensed under MIT. | Borrow the database-site capability shape, not the Postgres-specific data exposure model: Analytix keeps current-case resolution, `analysis_*` / `fc_*_norm` scope, purpose gates, `count_case_rows`, privacy-projected preview, evidence ledger, local-only audit, no folder scanning or raw/source table access, plus DuckDB equivalents for schema/object detail, query plan, query diagnostics, tool/query-log health, graph/materialized-index health, and deterministic slow-path detection. Do not borrow unrestricted SQL, direct database credentials, extension installation, or database-health checks that have no DuckDB/current-case equivalent. |
| [567-labs/instructor](https://github.com/567-labs/instructor) | Schema-first structured outputs, response model validation, automatic retries for validation failures, streaming/partial objects, hooks, failed-attempt inspection, and provider-agnostic response-model handling. Licensed under MIT. | Borrow the structure and validation pattern, not a model-scoring loop: Analytix uses `FinalInvestigationAnswer` schema, deterministic JS validation, delivery-QC failures, local audit events, repair actions, and `validate_report_claims(final_investigation_answer)` integration. Missing structure may be repaired; missing facts must return to MCP/DuckDB or become evidence boundaries. Do not add a Python runtime dependency or use retry to hallucinate unsupported amounts, edges, ownership, legal conclusions, or proof actions. |

Secondary references: [hyhmrright/brooks-lint](https://github.com/hyhmrright/brooks-lint) for deterministic lint/risk labels, and [avivsinai/langfuse-mcp](https://github.com/avivsinai/langfuse-mcp) for observability patterns. For Analytix, observability must stay local/offline unless the user explicitly enables export.

## Non-Negotiable Filter

- Adopt: skill packaging, compact MCP facts, pre-indexing, deterministic lint, golden eval, local audit logs, optional adversarial review.
- Reject: global Codex hooks, global config rewrites, scheduler changes, third-party telemetry, long prompt catalogs, full JSON dumps, tool menus that make the Agent choose among dozens of similar commands.

## Writing-Style Borrowing Boundary

Public writing skills such as humanizer, Humanizer-zh, stop-slop, taste-skill,
ai-flavor-remover, shuorenhua, nuwa-skill, writing-agent, HC3-style
comparison/detection work, and De-AI prompt enhancers are useful for mechanism
borrowing, not for detector-evasion goals.

Analytix may borrow:

- phrase and structure anti-pattern catalogs;
- protected-span rules for numbers, names, accounts, dates, units, quotes, and
  responsibility attribution;
- scene/register routing so official material stays official and quick answers
  stay compact;
- expression-DNA checks such as sentence rhythm, paragraph density, and taboo
  terms;
- two-pass reread: preserve facts first, then remove remaining formulaic
  wording;
- final pre-flight checklists.

Analytix must not borrow:

- chatty humanizer tone, jokes, internet slang, rhetorical questions, or
  first-person performance for case material;
- detector-bypass scoring as a product promise;
- style changes that round amounts, merge accounts, hide rows, alter evidence
  maturity, or add unsourced authority;
- public-writing drama such as empty contrast, false ranges, or slogan endings.
