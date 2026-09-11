# Post-881 Stage Closure Evidence - 2026-06-22

This file is a machine-scannable closure snapshot for the Reasonix
`881b2f2f..9ada14176629b1d59d7ed78446951b2bb5954904` absorption stream.
It summarizes the evidence already recorded in specs, ledgers, scorecards,
release evidence, and committed tests. It does not replace those sources.

## post881StageClosureCapabilityFloor

Analytix is at least fixture-level parity for the post-881 Reasonix surfaces
listed below, while keeping the public product boundary analytix-owned:

| Surface | Evidence | Current floor |
| --- | --- | --- |
| Provider cache accounting | D-0172 and D-0173 derive DeepSeek/OpenAI Responses/Anthropic cache accounting from raw fixture payloads; D-0180 keeps custom endpoint request-shape-only proof scan-visible. | Fixture/oracle parity floor; no live credential or independent custom cache telemetry claim. |
| Step/cancel/cache stability | D-0174 keeps combined route-cache reuse, next-turn reroute, cancel result pairing, and stable-prefix isolation focused in G5 proof. | Fixture/oracle parity floor; no live Go loop claim. |
| Plan step/cancel/cache G5 shadow | D-0229 proves explicit Plan mode advertises `create_plan` + `ls` but not `bash`, forces the follow-up to `create_plan`, keeps an aborted follow-up from advancing the cache baseline, reuses the original baseline on retry, and preserves DeepSeek `80/20` cache telemetry in Go G5 shadow. | Runtime/Go shadow floor; no public auto-plan setting, Reasonix controller protocol, or live Go loop claim. |
| Plan/auto-route state reset | D-0230 proves cancelled Plan mode does not leak `modeInstruction`, `requiredToolName`, or `create_plan` into later normal/auto turns, and the next `model: "auto"` turn reruns an isolated `_auto_router` request with current recommendation. | Runtime/Go shadow floor; no public auto-plan setting, Reasonix controller protocol, or live Go loop claim. |
| Approval/user-input gates | D-0175 proves denied no-execute, answer privacy, late action rejection, abort cleanup, and resume cleanup. | Fixture/oracle parity floor; no live Go gate manager claim. |
| AutoResearch direction tracking | D-0227 proves `record_research_direction` writes `directions_tried.json`, appends `direction_recorded` to `iteration_log.jsonl`, requires an active research goal, and replays the evidence in Go G5 shadow. | Runtime/Go shadow floor; no top-level AutoResearch entry or live Go research bridge claim. |
| Goal persistence off-lock | D-0181 proves TS `ThreadService` goal writes persist before goal events, warn/surface persistence failures, and avoid controller/status/approval lock coupling. | Fixture/source oracle parity floor; no live Go goal manager claim. |
| Tool result files/images | D-0182 proves inline image extraction/cap, evicted base64 omission, attachment `localFilePath`, text fallback `FilePath`, DeepSeek v4 fallback routing, and generated-file meta lifting. | Fixture/source oracle parity floor; no live Go file/image bridge claim. |
| Event JSONL replay/recovery | D-0183 proves newline-terminated `events.jsonl` append, replay filter/sort, highestSeq max, malformed line recovery, persist-before-publish, seq uniqueness, usage compaction carryover, and compaction-failure append-only recovery. | Fixture/source oracle parity floor; no live Go event store claim. |
| MCP malformed schema normalization | D-0184 proves malformed MCP `inputSchema` arrays default safely, invalid `properties` are dropped, mixed `required` keeps only strings, tool names remain advertised, and output-schema non-records are omitted. | Fixture/source oracle parity floor; no live Go MCP client claim. |
| Desktop runtime IPC bridge | D-0185 proves preload runtime request and SSE traffic stay on `window.analytix.runtime` plus `runtime:request` / `runtime:sse:*` IPC, with forbidden Reasonix/Kun/Go/workflow IPC channels absent. | Fixture/source oracle parity floor; no packaged desktop bridge walkthrough claim. |
| Desktop main IPC boundary | D-0186 proves main `runtime:request` rejects forbidden routes before runtime calls, and main `runtime:sse:*` preserves thread events route, reconnect cursor, matching stop id, and 100ms batching. | Fixture/source oracle parity floor; no packaged desktop bridge walkthrough claim. |
| Renderer route surface | D-0187 proves the top-level `AppRoute` union remains `chat/write/settings/plugins/claw/schedule`, forbidden Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entries are absent, dormant workflow code is quarantined, browser preview uses only `window.analytix`, and shell/plugin safe-area behavior remains preserved. | Fixture/source oracle parity floor; no packaged desktop route walkthrough claim. |
| Package/runtime CLI identity | D-0188 proves root/runtime package identity, electron-builder app/product/artifact/NSIS identity, AppUserModelID, bundled runtime `serve-entry.js`, public `analytix serve` usage, `ANALYTIX_READY`, afterPack validation, and `ANALYTIX_*` release env ownership stay analytix-owned. | Fixture/source oracle parity floor; no packaged artifact walkthrough claim. |
| Runtime HTTP/SSE route sovereignty | D-0189 proves the active `analytix serve` route table is exactly 44 entries, `/health` is the only unauthenticated route, all other routes are authenticated `/v1/*`, SSE remains `/v1/threads/:id/events`, thread lifecycle and approval/user-input routes are present, task-job routes stay internal, singular user-input is compatibility-only, and forbidden route tokens are absent. | Fixture/source oracle parity floor; no live Go HTTP server or packaged route walkthrough claim. |
| Runtime HTTP auth matrix | D-0190 dispatches every D-0189 route through the real TypeScript HTTP router without auth, proving `/health` returns 200 and all 43 `/v1/*` routes return structured 401, including SSE, task-job, approval, user-input, and resume-thread routes. | Runtime test parity floor; no live Go HTTP server or packaged route walkthrough claim. |
| Runtime HTTP auth matrix G5 shadow | D-0210 promotes the D-0190 matrix into `runtimeHttpRouteSovereignty.authMatrix` and Go G5 executable shadow output. | TS fixture/Go shadow floor; no live Go HTTP server or packaged route walkthrough claim. |
| Runtime forbidden route dispatch | D-0191 dispatches every D-0189 forbidden route token through the real TypeScript HTTP router with valid auth, proving each returns structured 404 rather than an authenticated hidden route. | Runtime test parity floor; no live Go HTTP server or packaged route walkthrough claim. |
| Runtime forbidden route dispatch G5 shadow | D-0211 promotes the D-0191 matrix into `runtimeHttpRouteSovereignty.forbiddenDispatchMatrix` and Go G5 executable shadow output. | TS fixture/Go shadow floor; no live Go HTTP server or packaged route walkthrough claim. |
| Go live-local sidecar prototype | D-0232 starts a real local Go `httptest.Server` over analytix-owned G1 health/runtime-info/tools and filtered G2 GET/SSE replay fixtures, then compares status, JSON bodies, and SSE frames against the TypeScript oracle while proving G5 rollback/default-backend guards stay false. | Test/conformance-only live-local prototype; no Electron connection, default backend, renderer-visible Go route, live provider/MCP/gate/file mutation, G6, or release claim. |
| Go live-local isolated mutating G2 lifecycle prototype | D-0233 extends the same test-only Go sidecar with an isolated in-memory G2 lifecycle store, executes archive/update/fork/resume mutating routes against the TS-owned oracle, records state in `LiveLocalSidecarSnapshot`, and proves a temp `events.jsonl` sentinel remains unchanged. | Test/conformance-only isolated mutating prototype; no durable Go store, Electron connection, default backend, renderer-visible Go route, live provider/MCP/gate/file mutation, G6, or release claim. |
| Go live-local G3 provider/cache streaming prototype | D-0234 extends the same test-only Go sidecar with a fixture-backed provider/cache harness for 5 usage cases, 7 request-shape cases, `item_delta -> usage -> turn_completed` SSE replay, cache accounting, cache drift, and bounded diagnostics privacy. | Test/conformance-only fixture-backed G3 prototype; no live external provider call, API-key read, credentialed provider matrix, Electron connection, default backend, renderer-visible Go route, G6, or release claim. |
| Shared endpoint builders | D-0192 proves shared endpoint builders URL-encode route ids, exported templates stay analytix-owned, canonical user-input remains plural `/v1/user-inputs/{id}`, and forbidden upstream/hidden endpoint tokens are absent. | Shared contract unit-test floor; no live Go HTTP server or packaged route walkthrough claim. |
| Shared endpoint builder G5 shadow | D-0212 promotes the D-0192 matrix into `runtimeHttpRouteSovereignty.sharedEndpointBuilderMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go HTTP server or packaged route walkthrough claim. |
| MCP call-time reconnect | D-0228 proves stale MCP transport failures reconnect and retry once while deterministic protocol errors return tool-result failures without reconnecting, then replays the classification in Go G5 shadow. | Runtime/Go shadow floor; no live Go MCP client or credentialed MCP matrix claim. |
| MCP search refresh drift | D-0231 makes existing `mcp_refresh_catalog`, `mcpSearchRefreshDrift`, `totalIndexed: 2`, and `catalogDrift: true` evidence scan-visible and stage-closure-visible. | Runtime/Go shadow floor; no public MCP-indexer route, live Go MCP client, or credentialed MCP matrix claim. |
| Renderer runtime endpoint builders | D-0193 proves renderer `AnalytixRuntimeProvider` root calls use shared endpoint constants and dynamic thread/turn/approval/user-input/session ids are encoded before bridge `runtimeRequest`. | Renderer unit-test floor; no packaged desktop route walkthrough claim. |
| Renderer runtime endpoint builder G5 shadow | D-0213 promotes the D-0193 renderer provider proof into `desktopSovereignty.rendererProviderEndpointMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go desktop bridge, default backend, or packaged route walkthrough claim. |
| Main IPC endpoint builder allow-list | D-0194 proves the main IPC `runtimeRequestPayloadSchema` accepts encoded shared builder output for thread/turn/checkpoint/approval/user-input/session/attachment/memory paths and rejects raw extra-segment plus singular user-input compatibility drift. | Main IPC unit-test floor; no packaged desktop route walkthrough claim. |
| Main IPC endpoint builder G5 shadow | D-0214 promotes the D-0194 main IPC schema proof into `desktopMainIpcBoundary.endpointBuilderAllowListMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go main IPC bridge, default backend, or packaged route walkthrough claim. |
| Main IPC runtime adapter handoff | D-0195 proves the registered `runtime:request` handler forwards encoded shared builder paths to `runtimeRequest` unchanged and rejects raw/singular compatibility drift before adapter invocation. | Main IPC handler unit-test floor; no packaged desktop route walkthrough claim. |
| Main IPC runtime adapter handoff G5 shadow | D-0215 promotes the D-0195 registered-handler proof into `desktopMainIpcBoundary.runtimeAdapterHandoffMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go main IPC bridge, default backend, or packaged route walkthrough claim. |
| Preload runtime request bridge | D-0196 proves the exposed `window.analytix` runtime and diagnostics facades pass encoded shared endpoint paths, method, and body unchanged into `ipcRenderer.invoke('runtime:request', ...)`. | Preload unit-test floor; no packaged desktop route walkthrough claim. |
| Preload runtime request bridge G5 shadow | D-0216 promotes the D-0196 preload bridge proof into `desktopSovereignty.preloadRuntimeRequestBridgeMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go preload bridge, default backend, or packaged route walkthrough claim. |
| Runtime host URL handoff | D-0197 proves `runtimeRequestViaHost` forwards encoded shared endpoint paths/query strings plus method/auth/header/content-type/body unchanged into the managed `analytix serve` HTTP host. | Runtime adapter unit-test floor; no packaged desktop route walkthrough claim. |
| Runtime host URL handoff G5 shadow | D-0217 promotes the D-0197 host handoff proof into `desktopMainIpcBoundary.runtimeHostHandoffMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go HTTP server, default backend, or packaged route walkthrough claim. |
| Preload SSE bridge | D-0198 proves the exposed `window.analytix` SSE facade preserves `startSse` / `stopSse` arguments and forwards event/end/error payloads without exposing Electron events. | Preload SSE unit-test floor; no packaged desktop SSE walkthrough claim. |
| Preload SSE bridge G5 shadow | D-0218 promotes the D-0198 preload SSE bridge proof into `desktopSovereignty.preloadSseBridgeMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go SSE server, default backend, or packaged SSE walkthrough claim. |
| Main SSE host URL encoding | D-0199 proves main `runtime:sse:start` encodes dangerous thread ids before fetching `/v1/threads/{id}/events` and preserves `since_seq`, `Last-Event-ID`, `Accept`, and auth. | Main SSE unit-test floor; no packaged desktop SSE walkthrough claim. |
| Main SSE host URL encoding G5 shadow | D-0219 promotes the D-0199 main SSE host encoding proof into `desktopMainIpcBoundary.mainSseHostEncodingMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go SSE server, default backend, or packaged SSE walkthrough claim. |
| Renderer runtime client bridge | D-0200 proves renderer runtime request and SSE facade calls go through `window.analytix.runtime` unchanged, while throwing `window.kun` / `window.reasonix` getters remain unread. | Renderer client unit-test floor; no packaged desktop runtime walkthrough claim. |
| Renderer runtime client bridge G5 shadow | D-0220 promotes the D-0200 renderer runtime client bridge proof into `desktopSovereignty.rendererRuntimeClientBridgeMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go desktop bridge, default backend, or packaged renderer walkthrough claim. |
| Renderer settings bridge | D-0201 proves renderer settings writes go through `window.analytix.settings`, preserve top-level `runtime` patch fields, update the returned settings cache, and keep throwing `window.kun` / `window.reasonix` getters unread. | Renderer client unit-test floor; no packaged desktop settings walkthrough claim. |
| Renderer settings bridge G5 shadow | D-0221 promotes the D-0201 renderer settings bridge proof into `desktopSovereignty.rendererSettingsBridgeMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go desktop bridge, default backend, or packaged settings walkthrough claim. |
| Renderer runtime provider alias guard | D-0202 proves `AnalytixRuntimeProvider` route, approval/user-input, fork/resume, and dynamic route-id encoding tests run with throwing `window.kun` / `window.reasonix` getters unread. | Renderer provider unit-test floor; no packaged desktop walkthrough claim. |
| Renderer provider alias guard G5 shadow | D-0222 promotes the D-0202 renderer provider alias guard proof into `desktopSovereignty.rendererProviderAliasGuardMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go desktop bridge, default backend, or packaged provider walkthrough claim. |
| Renderer provider runtime client facade seal | D-0203 routes archive/restore through `rendererRuntimeClient.runtimeRequest` and scans against direct `window.analytix.runtime.runtimeRequest` bypass inside `analytix-runtime.ts`. | Renderer provider source/unit-test floor; no packaged desktop walkthrough claim. |
| Renderer provider facade seal G5 shadow | D-0223 promotes the D-0203 provider facade seal proof into `desktopSovereignty.rendererProviderFacadeSealMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go desktop bridge, default backend, or packaged provider walkthrough claim. |
| Side conversation relation provider contract | D-0204 routes side promotion through `AgentProvider.updateThreadRelation`, keeps the relation PATCH inside `rendererRuntimeClient`, and scans against direct runtime bridge bypass in side-store source. | Renderer store/provider source/unit-test floor; no packaged side-conversation walkthrough claim. |
| Side conversation relation G5 shadow | D-0224 promotes the D-0204 side relation provider/store proof into `desktopSovereignty.sideConversationRelationContractMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go desktop bridge, default backend, or packaged side walkthrough claim. |
| Renderer usage runtime client facade seal | D-0205 routes thread/day/model usage, token economy, and LLM debug runtime HTTP requests through `rendererRuntimeClient` and forbids direct generic runtime request bridge calls in renderer production source. | Renderer usage/settings source/unit-test floor; no packaged usage dashboard walkthrough claim. |
| Renderer usage runtime client facade G5 shadow | D-0225 promotes the D-0205 usage/debug facade seal into `desktopSovereignty.rendererUsageRuntimeClientFacadeMatrix` and Go G5 executable shadow output. | TS source/unit/Go shadow floor; no live Go desktop bridge, default backend, or packaged usage walkthrough claim. |
| Renderer settings read facade seal | D-0206 routes keyboard shortcut, speech-to-text, and initial usage model-label settings reads through `rendererRuntimeClient.getSettings` and forbids direct settings reads in renderer production source. | Renderer settings-read source/unit-test floor; no packaged settings walkthrough claim. |
| Renderer settings read facade G5 shadow | D-0226 promotes the D-0206 settings-read facade seal into `desktopSovereignty.rendererSettingsReadFacadeMatrix` and Go G5 executable shadow output. | TS source/scan/Go shadow floor; no live Go desktop bridge, default backend, or packaged settings walkthrough claim. |
| Renderer named bridge API allow-list | D-0207 limits remaining direct `window.analytix.runtime.*` / `window.analytix.settings.*` production calls to explicit named preload contracts and keeps generic runtime/settings bypasses rejected. | Renderer bridge source-scan floor; no packaged bridge walkthrough claim. |
| Renderer optional bridge bypass seal | D-0208 makes optional-chain direct bridge access obey the same scans, routes Connect Phone dialog settings reads through `rendererRuntimeClient`, and removes plugin marketplace generic runtime bridge probing. | Renderer bridge source-scan floor; no packaged bridge walkthrough claim. |
| Renderer bridge allow-list G5 shadow | D-0209 promotes renderer bridge allow-list inventory, bypass counts, and optional-chain scan coverage into `desktopSovereignty` G5 executable shadow. | TS source/fixture/Go shadow floor; no live Go desktop bridge claim. |
| Thread/session routes and SSE replay | D-0176 proves archive/search, read/update, fork, resume-thread, SSE replay/caught-up, unauthorized auth, exact body hashes, and SSE hash. | Fixture/oracle parity floor; no live Go router claim. |
| Release evidence closure | D-0177 and D-0178 make final command gates D-0172 through D-0178 scan-visible. | Evidence freshness floor; not release readiness. |

## reasonixAbsorbedDeltas

The absorbed Reasonix values are classified as follows:

| Delta family | Classification | Analytix-owned result |
| --- | --- | --- |
| Auto-plan classifier/currentness drift | contract-reimplement | Internal auto-router and planner/currentness proofs remain behind analytix runtime contracts, not Reasonix config roots. |
| Planner gating | contract-reimplement | Plan mode and planner tool narrowing are proved through G5 control cases without adding public auto-plan settings. |
| Step/cancel/cache interaction | contract-reimplement | Combined G5 proof keeps dynamic state out of stable prefix and proves cancel aggregation. |
| Plan step/cancel/cache G5 shadow | code-port-and-adapt | D-0229 makes Plan-mode tool narrowing, forced plan follow-up, aborted-follow-up cache baseline preservation, retry baseline reuse, and DeepSeek cache telemetry executable in Go G5 shadow without exposing a public auto-plan setting or live Go loop. |
| Plan/auto-route state reset | code-port-and-adapt | D-0230 makes cancelled-Plan no-leak and post-cancel auto-router currentness executable in Go G5 shadow without exposing public auto-plan settings, Reasonix controller protocol, or a live Go loop. |
| Goal persistence off-lock | code-port-and-adapt | D-0181 converts Reasonix lock-split guidance into analytix-owned G5 source/fixture proof without adding TS controller locks. |
| Tool result file/image boundary | code-port-and-adapt | D-0182 converts file/image result preservation into analytix-owned G5 source/fixture proof without adding Reasonix file protocol. |
| Event JSONL replay boundary | contract-reimplement | D-0183 converts event replay/recovery into analytix-owned G5 source/fixture proof without adding Reasonix event protocol. |
| MCP malformed schema boundary | code-port-and-adapt | D-0184 converts MCP schema safety into analytix-owned G5 source/fixture proof without adding MCP-indexer public protocol. |
| Desktop runtime IPC bridge boundary | contract-reimplement | D-0185 converts Reasonix frontend/session boundary value into analytix-owned preload/runtime IPC proof without exposing SessionAPI or Go routes. |
| Desktop main IPC boundary | contract-reimplement | D-0186 converts Reasonix route/session boundary value into analytix-owned main IPC allow-list and SSE cursor proof. |
| Renderer route-surface sovereignty | contract-reimplement | D-0187 converts Reasonix frontend/session surface value into analytix-owned route-union, dormant quarantine, browser bridge, plugin safe-area, and shell safe-inset proof without exposing hidden navigation. |
| Package/runtime CLI identity | contract-reimplement | D-0188 converts runtime/packaging identity pressure into analytix-owned package, release, CLI, ready-handshake, afterPack, and release-env proof without exposing Reasonix CLI/session protocol. |
| Runtime HTTP/SSE route sovereignty | contract-reimplement | D-0189 converts Reasonix route/session pressure into analytix-owned HTTP route inventory, auth guard, SSE, task-job internal route, and forbidden-route proof without exposing Reasonix SessionAPI or public route protocol. |
| Runtime HTTP auth matrix | contract-reimplement | D-0190 converts the route-auth requirement into actual TypeScript dispatch proof for every registered route without adding a public control plane. |
| Runtime HTTP auth matrix G5 shadow | code-port-and-adapt | D-0210 makes the runtime auth matrix executable in Go G5 shadow without implementing a live Go HTTP server or default backend. |
| Runtime forbidden route dispatch | contract-reimplement | D-0191 converts forbidden route governance into actual TypeScript dispatch proof that upstream/hidden route tokens return structured not_found. |
| Runtime forbidden route dispatch G5 shadow | code-port-and-adapt | D-0211 makes the forbidden-route dispatch matrix executable in Go G5 shadow without implementing a live Go HTTP server or default backend. |
| Shared endpoint builder sovereignty | contract-reimplement | D-0192 converts endpoint path construction into unit proof for URL encoding, canonical plural user-input templates, and forbidden shared endpoint absence. |
| Shared endpoint builder G5 shadow | code-port-and-adapt | D-0212 makes shared endpoint builder encoding and template ownership executable in Go G5 shadow without implementing a live Go HTTP server or default backend. |
| Renderer runtime endpoint builder sovereignty | contract-reimplement | D-0193 converts GUI provider path construction into shared-constant/builder usage plus encoded dynamic id proof. |
| Renderer runtime endpoint builder G5 shadow | code-port-and-adapt | D-0213 makes renderer provider endpoint-builder ownership executable in Go G5 desktop sovereignty shadow without exposing a Go desktop bridge or default backend. |
| Main IPC endpoint builder allow-list | contract-reimplement | D-0194 converts the endpoint builder chain into main IPC schema proof that accepts encoded shared paths and rejects raw/singular compatibility drift. |
| Main IPC endpoint builder G5 shadow | code-port-and-adapt | D-0214 makes main IPC endpoint-builder allow-list and raw-route rejection executable in Go G5 desktop main IPC shadow without exposing a Go bridge or default backend. |
| Main IPC runtime adapter handoff | contract-reimplement | D-0195 converts the endpoint builder chain into registered handler proof that preserves encoded paths and blocks invalid paths before adapter calls. |
| Main IPC runtime adapter handoff G5 shadow | code-port-and-adapt | D-0215 makes main IPC adapter handoff preservation and reject-before-call ordering executable in Go G5 shadow without exposing a Go bridge or default backend. |
| Preload runtime request bridge | contract-reimplement | D-0196 converts the endpoint builder chain into executable preload facade proof on `window.analytix` without deprecated bridge aliases. |
| Preload runtime request bridge G5 shadow | code-port-and-adapt | D-0216 makes preload runtime/diagnostics request preservation executable in Go G5 desktop sovereignty shadow without exposing a Go bridge or default backend. |
| Runtime host URL handoff | contract-reimplement | D-0197 converts the endpoint builder chain into runtime adapter proof that encoded paths reach managed `analytix serve` unchanged. |
| Runtime host URL handoff G5 shadow | code-port-and-adapt | D-0217 makes runtime host path/query, method/header/body, and ensured-settings handoff executable in Go G5 desktop main IPC shadow without exposing a live Go HTTP server. |
| Preload SSE bridge | contract-reimplement | D-0198 converts SSE streaming bridge ownership into executable preload facade proof with payload-only listener delivery. |
| Preload SSE bridge G5 shadow | code-port-and-adapt | D-0218 makes preload SSE start/stop, payload-only delivery, and listener cleanup executable in Go G5 desktop sovereignty shadow without exposing a live Go SSE server. |
| Main SSE host URL encoding | contract-reimplement | D-0199 converts SSE event route construction into main IPC proof that dangerous thread ids are encoded and cursor headers preserved. |
| Main SSE host URL encoding G5 shadow | code-port-and-adapt | D-0219 makes main SSE encoded events path, cursor/header preservation, stream id error delivery, and forbidden route guard executable in Go G5 desktop main IPC shadow without exposing a live Go SSE server. |
| Renderer runtime client bridge | contract-reimplement | D-0200 converts renderer client bridge ownership into executable proof against legacy alias fallback. |
| Renderer runtime client bridge G5 shadow | code-port-and-adapt | D-0220 makes renderer request argument preservation, restart passthrough, SSE control/listener passthrough, and legacy alias unread proof executable in Go G5 desktop sovereignty shadow without exposing a live Go desktop bridge. |
| Renderer settings bridge | contract-reimplement | D-0201 converts renderer settings bridge ownership into executable proof for top-level `runtime` patch preservation through `window.analytix.settings`. |
| Renderer settings bridge G5 shadow | code-port-and-adapt | D-0221 makes settings read cache, setSettings refresh, top-level `runtime` patch preservation, and legacy alias unread proof executable in Go G5 desktop sovereignty shadow without exposing a live Go settings bridge. |
| Renderer runtime provider alias guard | contract-reimplement | D-0202 converts renderer provider bridge ownership into executable alias-guard proof across thread lifecycle, approval/user-input, fork/resume, and route encoding paths. |
| Renderer provider alias guard G5 shadow | code-port-and-adapt | D-0222 makes provider route ownership, lifecycle/gate coverage, fork/resume dynamic encoding, forbidden-route rejection, and legacy alias unread proof executable in Go G5 desktop sovereignty shadow without exposing a live Go provider bridge. |
| Renderer provider runtime client facade seal | code-port-and-adapt | D-0203 removes the remaining direct provider runtime request bypass and adds a product-sovereignty scan against reintroduction. |
| Renderer provider facade seal G5 shadow | code-port-and-adapt | D-0223 makes provider runtime-client use, direct bridge rejection, archive/restore coverage, relation PATCH coverage, scan guard presence, and unit-proof presence executable in Go G5 desktop sovereignty shadow without exposing a live Go provider bridge. |
| Side conversation relation provider contract | code-port-and-adapt | D-0204 removes side promotion's direct store runtime bridge bypass by adding an analytix provider relation contract. |
| Side conversation relation G5 shadow | code-port-and-adapt | D-0224 makes optional provider relation contract proof, provider promotion, refresh/close behavior, direct bridge rejection, scan guard presence, and unit-proof presence executable in Go G5 desktop sovereignty shadow without exposing a live Go side bridge. |
| Renderer usage runtime client facade seal | code-port-and-adapt | D-0205 removes direct renderer generic runtime HTTP request bypasses and makes the renderer-wide scan enforce the runtime client facade. |
| Renderer usage runtime client facade G5 shadow | code-port-and-adapt | D-0225 makes thread/day/model usage coverage, token economy and LLM debug diagnostics coverage, direct bridge rejection, scan guard presence, and unit-proof presence executable in Go G5 desktop sovereignty shadow without exposing a live Go usage bridge. |
| Renderer settings read facade seal | code-port-and-adapt | D-0206 removes direct renderer settings read bypasses and makes the renderer-wide scan enforce the settings read facade. |
| Renderer settings read facade G5 shadow | code-port-and-adapt | D-0226 makes keyboard shortcut, speech-to-text, and usage model-label settings-read coverage plus event-sync preservation, direct bridge rejection, and scan guard presence executable in Go G5 desktop sovereignty shadow without exposing a live Go settings bridge. |
| AutoResearch direction tracking G5 shadow | code-port-and-adapt | D-0227 makes `record_research_direction`, `directions_tried.json`, `iteration_log.jsonl`, and active research-goal gating executable in Go G5 shadow without exposing a top-level AutoResearch entry or live Go research bridge. |
| Renderer named bridge API allow-list | contract-reimplement | D-0207 makes the remaining direct renderer window APIs explicit named analytix preload contracts and fails scans on unlisted expansion. |
| Renderer optional bridge bypass seal | contract-reimplement | D-0208 treats optional-chain bridge access as direct access and removes optional-chain generic runtime/settings bypasses. |
| Renderer bridge allow-list G5 shadow | code-port-and-adapt | D-0209 makes the renderer bridge allow-list executable in Go G5 shadow without enabling a Go backend. |
| Provider cache accounting | code-port-and-adapt | DeepSeek/OpenAI Responses/Anthropic cache accounting derives from raw provider-like payloads while preserving endpoint/body semantics. |
| Custom provider request shape | contract-reimplement | Custom provider proof covers exact full-endpoint URL/body/header shape only, with D-0180 sealing custom telemetry ids to `[]`. |
| MCP lifecycle and tool approval | code-port-and-adapt | MCP lifecycle/search/approval annotation proofs remain internal runtime evidence, not MCP-indexer product routes. |
| MCP call-time reconnect G5 shadow | code-port-and-adapt | D-0228 makes stale transport retry and deterministic protocol no-retry classification executable in Go G5 shadow without exposing a live Go MCP client or MCP-indexer route. |
| MCP search refresh drift evidence closure | document-only | D-0231 makes existing `mcpSearchRefreshDrift`, `mcp_refresh_catalog`, `catalogDrift`, and `totalIndexed` evidence scan-visible without changing runtime behavior or exposing a public MCP-indexer route. |
| Go live-local sidecar prototype | code-port-and-adapt | D-0232 adds a test/conformance-only local Go HTTP sidecar harness for G1 health/runtime-info/tools plus G2 read-only route and SSE replay, rejects mutating route replay, and preserves rollback/default-backend guards. |
| Go live-local isolated mutating G2 lifecycle prototype | code-port-and-adapt | D-0233 admits the four G2 mutating lifecycle routes into an isolated in-memory sidecar harness while preserving TS oracle authority, forbidden route rejection, no real `events.jsonl` writes, and rollback/default-backend guards. |
| Go live-local G3 provider/cache streaming prototype | code-port-and-adapt | D-0234 admits fixture-backed provider/cache usage, request-shape, SSE, accounting, drift, and diagnostics replay into the local sidecar while preserving TS oracle authority, no real provider/API-key access, no Reasonix protocol, and rollback/default-backend guards. |
| Sub-agent/job orchestration | code-port-and-adapt | Task/job orchestration proofs remain internal runtime tools and Go shadow cases, not top-level Subagent routes. |
| Reasonix public protocol and config roots | reject | No SessionAPI, controller API, public route names, or Reasonix identity is exposed. |
| Live Go backend and packaged QA | defer | Live backend selection, packaged walkthroughs, and release readiness remain separate gates. |

## kunBaselinePreserved

Kun `0.2.13` -> `0.2.14` product sovereignty remains preserved:

```text
No top-level Workflow/Create Loop/Subagent/AutoResearch/MCP-indexer entry.
No Kun product identity or deprecated bridge alias.
No old runtime-shaped settings write path.
No default Go backend or renderer-visible Go route.
No Rust/Tauri rewrite path.
```

Kun-relevant retained surfaces remain Code, Write, Settings, Plugins, Connect
Phone, and Schedule, with Connect Phone naming retained for user-facing copy.

## analytixExceedsReasonixWhere

Analytix exceeds the direct Reasonix post-881 import path in these verified
ways:

| Area | Why analytix is stronger |
| --- | --- |
| Product sovereignty | The same engine values are absorbed without Reasonix public protocol, config roots, or route identity. |
| Multi-provider cache evidence | Cache accounting covers DeepSeek, OpenAI Responses, and Anthropic; OpenAI-compatible/custom endpoint families have request-shape coverage and unsupported cache telemetry stays unknown. |
| Desktop bridge safety | Renderer, preload, main IPC, browser preview, SSE bridge, and G5 shadow proofs are tied to `window.analytix`, optional-chain-aware named preload API allow-lists, renderer runtime/settings client G5 replay, renderer provider alias-guard/facade-seal G5 replay, side relation G5 replay, `runtime:request`, `runtime:sse:*`, strict main IPC schemas, and `/v1/*` analytix runtime routes. |
| Runtime/package identity | Package, release, CLI, ready-handshake, and afterPack proof keeps `analytix serve` machine-checkable instead of relying on prose. |
| Runtime HTTP surface | The active `analytix serve` route table is source-derived, counted, auth-checked, SSE-pinned, forbidden-token scanned, actual-dispatch 401 checked, auth-matrix G5 shadow checked, forbidden-token 404 checked, forbidden-dispatch G5 shadow checked, shared-builder unit and G5 shadow encoding checked, renderer-provider unit/G5 shadow checked, main IPC endpoint-builder unit/G5 shadow checked, main IPC adapter-handoff unit/G5 shadow checked, preload runtime request unit/G5 shadow checked, runtime host handoff unit/G5 shadow checked, preload SSE bridge unit/G5 shadow checked, and main SSE host encoding unit/G5 shadow checked instead of importing Reasonix route protocol. |
| Go sidecar proof | Analytix can start a real local Go HTTP sidecar against TS-owned G1/G2/G3 fixtures while machine-proving it remains test/conformance-only, now includes isolated mutating G2 lifecycle replay and fixture-backed G3 provider/cache streaming replay, not Electron-connected, not renderer-visible, and not the default backend. |
| Forbidden-surface governance | A single scan blocks top-level hidden entries, identity leaks, deprecated bridge/settings fallback, default Go backend, and Rust/Tauri activation. |
| Evidence closure | Release evidence final gates and post-881 proof tokens are machine-checkable, not only prose conventions. |

## remainingOpenGates

The following are deliberately not claimed complete:

```text
No live provider/cache superiority matrix.
No packaged desktop route/SSE/provider/MCP walkthrough.
No production Go thread/session router, provider client, gate manager, or Job
Manager.
No credentialed Go provider matrix or external model-call cache telemetry.
No Electron-connected or renderer-visible live Go HTTP server.
No G6 backend selection or rollback readiness.
No release readiness, signing, notarization, Windows installer QA, or live
distribution proof.
```

## Validation Pointers

The closure snapshot depends on these repeatable gates:

```text
git diff --check
npm --prefix packages/runtime test
npm run test
npm run typecheck
npm run build:runtime
cd packages/runtime-go && /tmp/analytix-go-toolchain/go/bin/go test -count=1 ./...
npm run scan:product-sovereignty
```
