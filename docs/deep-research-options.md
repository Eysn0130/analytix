# Deep Research Options / 深度调研方案对比

This note compares open-source research-agent patterns that can inform
Analytix without adding a second runtime. The current migration adds a
low-risk `/research` brief on top of the existing desktop runtime, tools,
approval flow, workspace files, and scheduled-task system.

## Comparison / 对比

| Approach | Strengths | Trade-offs | Useful idea for Analytix |
| --- | --- | --- | --- |
| Agent Reach-style capability layers | Low-friction channel setup, health checks, and ordered fallback backends for web, search, GitHub, video, RSS, and authenticated platforms. | Capability and installation layer, not a research planner or report engine. Some channels depend on local login state or external CLIs. | Detect available tools before research, prefer zero-configuration sources, expose channel health, and keep authenticated access local. |
| LangChain Open Deep Research-style orchestration | Configurable search/MCP options, separate summarization/research/report stages, and benchmark support. | Heavier deployment, multiple model calls, and potentially high token/API cost. Adding it directly would create a second runtime beside Analytix. | Add explicit research stages, evidence compression, progress state, and later evaluation without importing another runtime. |
| Lightweight recursive deep-research agents | Small breadth/depth design, iterative query generation, follow-up directions, concurrency, and source-backed Markdown reports. | Usually rely on dedicated search/model services and have fewer desktop integration, authentication, and long-running task controls. | Give users visible breadth/depth expectations and iterate from evidence gaps instead of running one broad search. |
| Analytix research brief | Reuses the current Analytix runtime, workspace files, browser/MCP tools, approval flow, scheduled tasks, and output tools. No second process or provider configuration is required. | It is an orchestration prompt, not yet a dedicated research state machine. Results still depend on the tools currently installed and available. | Provides a low-risk entry point and a stable evidence/output contract while a full workflow is designed. |

## Phased Delivery / 分阶段实现

### Phase 1: Research brief / 调研任务模板

- Add `/research` and common aliases to the existing composer command system.
- Ask Analytix to inspect available search, browser, MCP, workspace, and paper tools.
- Require iterative breadth/depth planning, an evidence ledger, independent
  cross-checks, uncertainty labels, and report-ready Markdown.
- If ongoing monitoring is requested, hand off to the existing scheduled-task
  system instead of claiming that monitoring is already active.

### Phase 2: Research run state / 专用调研状态

- Persist the research plan, queries, evidence records, unresolved gaps, and
  generated artifacts with the thread.
- Stream research progress separately from ordinary assistant prose.
- Allow pause, resume, source inspection, and bounded parallel searches.

### Phase 3: Tool health and export / 工具健康与导出

- Add capability checks for configured web, browser, MCP, and authenticated
  channels without storing cookies in a new service.
- Add optional evaluation fixtures for citation coverage and claim support.
- Reuse existing document, PDF, slide, media, and scheduled-task tools for
  exports and recurring updates.

## Current Advantages / 当前优势

- Fits `analytix serve` and the existing approval boundary.
- Can combine local files with web and MCP sources in one thread.
- Does not force one search vendor or require a new API key.
- Keeps credentials in the tools or browser sessions that already own them.

## Current Limitations / 当前限制

- No dedicated research graph or persisted evidence schema yet.
- No built-in search backend health dashboard.
- No benchmark-backed quality score yet.
- Parallelism and export quality depend on the active model and installed tools.
