# analytix upstream code reuse provenance ledger

Status: Operational admission and attribution ledger.
Applies to: copied, ported, vendored, generated, extracted, or substantially
adapted upstream code, tests, prompts, documentation, binaries, and assets.
Current as of: 2026-08-23.
Source of truth: pinned Git objects, written authorization records, source
history, file-level licenses/notices, and reviewed Analytix diffs.

This ledger is an engineering control, not legal advice. A root repository
license never makes a gitlink, vendored subtree, extracted application asset,
or file with a different notice automatically reusable.

## Required Record

Every admitted reuse item must record:

```text
source repository and full commit
source path(s) and source blob hash(es)
Analytix destination path(s)
reuse mode: copy-with-rename | port-and-adapt | clean-room-reference | vendor
license/SPDX and copyright holders
NOTICE/attribution destination
written authorization record when required
modification summary and reviewer
tests, benchmark, security gate, and current disposition
```

Allowed dispositions are `candidate`, `unverified`, `blocked`, `approved`,
`rejected`, and `retired`. `unverified` is not approval to ship a copied or
substantially adapted implementation.

## Current Review Queue

| Item | Admission scope | Source id | Evidence | Current disposition | Required closure |
| --- | --- | --- | --- | --- | --- |
| Kun-derived product baseline | `artifact-entering` | `kun` | Exact matching historical trees prove the inherited baseline; representative file/blob mappings and both historical/current license blobs are recorded below | `blocked` | Owner must provide material-specific commercial authorization and approve a complete inherited-file attribution inventory; the team authorization declaration is not that evidence. |
| Reasonix-derived `packages/runtime-go/internal/netclient/netclient.go` | `artifact-entering` | `deepseek-reasonix` | Exact source, introduction, current destination, MIT license, notice, modifications, reviewer, and test mapping are recorded below | `approved` | Preserve the file header, `THIRD_PARTY_NOTICES.md`, exact object record, and focused contract tests. |
| Reasonix-derived `packages/runtime-go/internal/diff/diff.go` | `artifact-entering` | `deepseek-reasonix` | Exact source, introduction, current destination, MIT license, notice, modifications, reviewer, and test mapping are recorded below | `approved` | Preserve the file header, `THIRD_PARTY_NOTICES.md`, exact object record, and focused contract tests. |
| Reasonix-derived `packages/runtime-go/internal/provider/provider.go` | `artifact-entering` | `deepseek-reasonix` | Multi-source provider object map, destination evolution, MIT license, notice, modifications, reviewer, and test mapping are recorded below | `approved` | Preserve the provider-lineage header, `THIRD_PARTY_NOTICES.md`, exact object record, split-adapter attribution, and focused contract tests. |
| OpenClaw-derived `vendor/openclaw-shim` | `artifact-entering` | `openclaw` | Annotated tag object, peeled commit, source blobs/ranges, local destinations, MIT license, notice, modifications, reviewer, and exact package gates are recorded below | `approved` | Preserve the nested upstream MIT `LICENSE`, root `THIRD_PARTY_NOTICES.md`, exact package metadata/provenance, source registry pin, and mandatory exact-artifact checks. |
| Gajae implementation families | `research-only` | `gajae-code` | Root MIT conflicts with incomplete inherited copyright chain; no Analytix destination admitted | `blocked` | Separate clean-room/new files from pi-mono/oh-my-pi lineage and restore all required notices before considering direct reuse. |
| Analytix thread virtualizer lineage | `artifact-entering` | `codexdesktop-rebuild` | The historically ambiguous destination blob was retired and replaced from the Analytix-owned contract/tests; extracted reference hashes, absent tracked source/license, replacement blob, reviewer, and tests are recorded below | `retired` | Keep CodexDesktop extracts research-only and preserve the clean-room replacement contract tests. |
| CodexDesktop extracted assets and scripts | `research-only` | `codexdesktop-rebuild` | Reviewed commit has no project license; generated `src/` extraction is not tracked | `rejected` absent authorization | Clean-room behavior study only, or record separate written authorization with asset hashes and scope. |
| Claude Code code/prompts/plugin text | `research-only` | `claude-code` | `LICENSE.md` is all-rights-reserved under commercial terms | `rejected` absent authorization | Clean-room behavioral requirements/tests only. |
| OpenCode, Hermes, Lazy, and Claw candidate files | `research-only` | `multiple` | Root licenses are MIT, with file-level exceptions in Hermes/Lazy; no Analytix destination admitted | `candidate` | Record exact source/destination blobs and preserve applicable MIT/Apache/NOTICE material before copying. |

## File-level closure records

The records below classify the six artifact-entering lineages reviewed through
the 2026-08-23 RC control-plane freeze. Git blob ids refer to repository objects;
SHA-256 is used only where an extracted reference is not a tracked Git object.

### Kun-derived product baseline — Owner blocker

- Mode: exact inherited baseline, commercially blocked; not an independent
  implementation claim.
- Exact tree pairs: Kun `0c60469336ccc97eae89bc09c3500603df88bd16`
  and Analytix `957def684e325ca2801f4fd13200cebbdbf0bbb8` share tree
  `bcf312916d9e181ee04256ed64da496fcab0d05b`; Kun
  `0f7f6026c768d028b47d865d5b2b40f2e138ddf9` and Analytix
  `3ce92ee1c21c154884657af54cb9815d094e4f26` share tree
  `809f5570afa9cbe4aba79abf55eefcb0b5ae2efe`; Kun
  `5472bed3b878854d296851820834145f5fe1a353` and Analytix
  `926be338b903d91a6385dfa20819e07280b6fcc2` share tree
  `ac4af5a7abcfa7abf71f6c2fc7189b10d1d0098d`; Kun
  `2ba8decc2f56862e7f677fcf89bbc3d402ec3a23` and Analytix
  `3fe00bc28ca0a3660b0e594b3fa2addfc85e3f9b` share tree
  `1e0bfc18125e955277c5a9a37111932a5d976a60`.
- Representative exact file mappings at the 0.2.13 baseline include
  `kun/src/contracts/threads.ts` to
  `packages/runtime/src/contracts/threads.ts`, blob
  `0d01ecd140868856a61b201df1632d6ed8bea374`, and
  `src/asset/img/ikun.png` to `src/asset/img/mascot.png`, blob
  `4fd3ae0e95b5aa81e861193953a1d24b7b6727f5`.
- License evidence: historical Kun license blob
  `52c91ecbe223fed6161e599ccc9b704235e06dc4`; pinned Kun license blob
  `def8c00355e33337e1177ffe749c85d776e1c254`; current Analytix license blob
  before this RC change `f75c25aaccbb84fc8312f42878343e1627809424`.
  The Kun terms are PolyForm Noncommercial and the pinned commit has no
  `NOTICE` object.
- Blocker: no material-specific written commercial authorization or complete
  inherited-file notice inventory is present. The recorded team authorization
  declaration for other named projects does not extend to Kun. Only the Owner
  can close that commercial authorization decision.
- Reviewer: coordinating Analytix RC agent, 2026-08-18. This row intentionally
  remains `blocked`; it does not hide or waive the inherited lineage.

### Reasonix `netclient.go` — exact port-and-adapt

- Source: DeepSeek-Reasonix commit
  `5c026dfacea142909de6b67e567fc728993b9e35`, path
  `internal/netclient/netclient.go`, blob
  `a466fa9602114b236ab240592ebb2f1c3b705117`.
- Destination: introduced by Analytix
  `70e99c36421bc611317f59dd1949ed4fb430b2f5` as blob
  `0042a878bf2fe855bc2d2a9f29257d096ca47105`; current reviewed blob after
  the attribution header is
  `fcbb8dc369f6fb8be5815ad1540e2af808c4593a`.
- License/notice: source license blob
  `bc45a281d8050c59c9b833ea2d0b1fb6e02602c0`, MIT, Copyright (c) 2026
  Reasonix Contributors; full notice is in `THIRD_PARTY_NOTICES.md`, packaged
  as `resources/THIRD_PARTY_NOTICES.md`, and the destination carries an
  attribution header.
- Modifications: removed the Reasonix `httpproxy`/`sysproxy` dependency path;
  added injected proxy functions, direct-host handling, SOCKS alias validation,
  Analytix timeout options, and secret-redacted diagnostics.
- Review/tests: coordinating Analytix RC agent, 2026-08-18;
  `go test ./internal/netclient -count=1` covers proxy redaction, aliases,
  direct-host/no-proxy behavior, and transport timeouts.

### Reasonix `diff.go` — exact port-and-adapt

- Source: DeepSeek-Reasonix commit
  `7377f462391abdd9b538f5865fef73757dc6c914`, path
  `internal/diff/diff.go`, blob
  `57d52f77ec04d607aff271fd2d5aa3e4528dc697`.
- Destination: introduced by Analytix
  `70e99c36421bc611317f59dd1949ed4fb430b2f5` as blob
  `62de093ecf2523f29be3e36edd49f4282e8eeb46`; current reviewed blob after
  the attribution header is
  `70fbfa12a30d9785bbca71f3cd4c7753b35e9565`.
- License/notice: the same Reasonix MIT license blob and copyright above; full
  notice is in `THIRD_PARTY_NOTICES.md`, packaged as
  `resources/THIRD_PARTY_NOTICES.md`, and the destination carries an
  attribution header.
- Modifications: replaced the Reasonix old/new text result shape with the
  Analytix file-operation contract, retained a bounded Myers/unified-diff core,
  and added binary omission and large-rewrite truncation behavior.
- Review/tests: coordinating Analytix RC agent, 2026-08-18;
  `go test ./internal/diff -count=1` covers create/modify diffs, binary input,
  and bounded large rewrites.

### Reasonix `provider.go` — exact multi-source port-and-adapt

- Source object map: DeepSeek-Reasonix
  `5b81febdab450311d2f5d5928f2a9dcd820cf7ad:internal/provider/provider.go`
  blob `82817f9540cd0397149af654753f681d45601610`; and commit
  `91fe06db6177bb052fc8ae1a3081d60bfc104a4e` paths
  `internal/provider/openai/openai.go` blob
  `7a1de930b1e9c0afce6b1af450ae453b47042250`,
  `internal/provider/anthropic/anthropic.go` blob
  `dc6fa52a93901c63097917310ac66f976106d294`,
  `internal/provider/schema_canonicalize.go` blob
  `b492c190bcc62b201a502fba92e5614c88eb761f`, and
  `internal/agent/cache_shape.go` blob
  `ff4b982b34fa56cbe20a0d9b34fe35dedf7c8ae1`.
- Destination evolution: introduced by Analytix
  `992c1fbd113986602ffed595eb6fc4337e1da7eb` as blob
  `1c520bab88fbb6abc1c18afed8fb37e9bfc43ff1`; provider/cache/stream
  adaptation entered blob `7ecb074dd15750400ab266a1fd5c81921d1195c7` at
  `70e99c36421bc611317f59dd1949ed4fb430b2f5`; the implementation was split
  into outbound client/compat/stream/usage adapters by
  `2fd360b3da24bf71a334def4bba9028f10ab3c30` and
  `6b48caa713060dff8b75456784b7c08ca1eda5c2`; current reviewed facade blob
  after the attribution header is
  `980b359e2258643ec56dfcef98c53da6503b7d53`.
- License/notice: all mapped source commits carry Reasonix license blob
  `bc45a281d8050c59c9b833ea2d0b1fb6e02602c0`; the full MIT notice and split
  provider-adapter scope are in `THIRD_PARTY_NOTICES.md`, packaged as
  `resources/THIRD_PARTY_NOTICES.md`, and the facade carries an attribution
  header.
- Modifications: Analytix added multi-provider configuration, explicit
  provider/model validation, redaction, bounded reconnect/retry behavior,
  provider request/cache shapes, three endpoint-family streaming adapters, and
  later reduced this file to an Analytix port/domain facade.
- Review/tests: coordinating Analytix RC agent, 2026-08-18;
  `go test ./internal/provider -count=1` covers request/auth redaction,
  provider/model validation, retry and no-reconnect boundaries, SSE/tool-call
  parsing, cache/schema diagnostics, and interrupted history repair.

### OpenClaw compatibility shim — port-and-adapt plus API reference

<!-- analytix-openclaw-shim-provenance-v1 {"schemaVersion":1,"sourceId":"openclaw","upstreamTag":"v2026.5.18","upstreamTagObject":"0c5e335df4311f135a36f7f72fcd87784dd01c96","upstreamCommit":"50a2481652b6a62d573ece3cead60400dc77020d","licenseBlob":"f7b526698bb7ed2d26d96c49f2f32234c88f69bc","licenseSha256":"62316704df7426e5a79d2827ff8aca36e9abb3a73b8e68557030749ebefec667","destination":"vendor/openclaw-shim"} -->

- Source: `https://github.com/openclaw/openclaw`, annotated tag
  `v2026.5.18` object `0c5e335df4311f135a36f7f72fcd87784dd01c96`,
  peeled commit `50a2481652b6a62d573ece3cead60400dc77020d`.
- Substantial adaptation: upstream `src/routing/account-id.ts:4-70`, blob
  `ea77f01afc2573678f8d3680c02db52e463f8276`, to
  `vendor/openclaw-shim/plugin-sdk/account-id.js:1-74`, reviewed destination
  blob `c5d51f12ca4f9582bd75195f4fac19e53d2fb0b7`. Analytix retained the
  default/optional account normalization contract while replacing TypeScript
  helpers, regex/cache implementation, and prototype-key dependency with a
  dependency-free JavaScript compatibility implementation.
- Compatibility API references: upstream
  `src/infra/tmp-openclaw-dir.ts:5-175`, blob
  `8f6b3b93f89fd722f77def68967fffe382599312`, informs
  `vendor/openclaw-shim/plugin-sdk/infra-runtime.js:5-7`; upstream
  `src/plugin-sdk/config-runtime.ts:7-40,75-76,134-145`, blob
  `2d56e1afef6fa27d8682ce7fa7766e4aaadb893b`, informs
  `vendor/openclaw-shim/plugin-sdk/config-runtime.js:4-31`; and upstream
  `src/plugin-sdk/channel-config-schema.ts:1-20`, blob
  `82a0ace24894ddc20eebbdd502a3c7217b563af1`, informs
  `vendor/openclaw-shim/plugin-sdk/channel-config-schema.js:1-3`. These are
  narrow API-shape references implemented locally for the WeChat bridge, not
  claims that the full upstream modules were copied.
- License/notice: upstream `LICENSE` blob
  `f7b526698bb7ed2d26d96c49f2f32234c88f69bc`, MIT, Copyright (c) 2025
  Peter Steinberger. The exact upstream text is retained at
  `vendor/openclaw-shim/LICENSE`; the full notice, adaptation scope, and local
  destination are retained in `THIRD_PARTY_NOTICES.md`, packaged as
  `resources/THIRD_PARTY_NOTICES.md`.
- Maintenance identity: `vendor/openclaw-shim/package.json` identifies Guoqin
  He (GitHub: Eysn0130) as the current Analytix shim maintainer and separately
  records the OpenClaw contributor/upstream provenance. It does not identify
  Guoqin He as the original OpenClaw copyright holder.
- Review/tests: coordinating Analytix RC agent, 2026-08-23; legal audit
  self-tests and formal exact-artifact fixtures reject missing or variant shim
  author, upstream identity, MIT metadata, nested license bytes, and package
  absence. Packaging config tests bind the file dependency, lock metadata, and
  non-exclusion of `node_modules/openclaw/LICENSE`.

### Analytix thread virtualizer — clean-room replacement

- Historical destination: introduced by Analytix
  `8b214f738f0a9a602e1a69d01e0de12fa1ef5f40` as blob
  `89bdf76f3687411e371457bd6350a1728239526b`. Contemporaneous design text
  used port/reuse language, so structural differences alone were insufficient
  for an independent-implementation conclusion.
- Reference boundary: CodexDesktop-Rebuild pin
  `5b83c2a2a8503aaaf0f346010df179e038f59bbc` has no tracked project license
  and no tracked source object or source map for the extracted assets. The
  reviewed generated SHA-256 values are
  `3fbf2caa06068803c6bcff7ef4956c29bfe075ed45a473a680ca5d63524f7387`,
  `a4696f8a8bce3ab53bc459167f3c1a0da066bf1be22524dea404cdb5e6523ccc`, and
  `f35435799ef18b08ea0797babb90173b347c610dd95dcc5c41315af68120abf0`.
  They remain research-only and no extracted code or asset is admitted.
- Replacement: `src/renderer/src/thread/virtualizer/analytix-thread-virtualizer.ts`
  was rewritten from the Analytix-owned row/window contract and focused tests;
  the replacement blob is
  `93a201a43e941ff865bffa6deba6a71a700d6df6`.
- Modifications: the replacement computes an Analytix row layout, inclusive
  window intersections, exact spacers, and stable empty-window behavior without
  a CodexDesktop source input.
- Review/tests: coordinating Analytix RC agent, 2026-08-18;
  `npx vitest run src/renderer/src/thread/virtualizer/analytix-thread-virtualizer.test.ts`
  covers empty, full, overscanned, measured, boundary-touch, missed-window, and
  bottom-distance behavior. The historical ambiguous implementation is
  `retired`, not relicensed or reclassified as independent.

## Review Rule

Documentation that says an idea was "absorbed" proves a design relationship,
not the legal mode of reuse. The reviewer must distinguish independent
reimplementation from a substantial source adaptation using Git history and
the actual source/destination diff before adding a notice or declaring closure.

## 2026-09-07 — instruction-methodology research

- Source: obra/superpowers `b36e0829c6d0140e93cfef2ca599b1b07d4a7797`, manifest 6.3.0; MIT, Copyright (c) 2025 Jesse Vincent. Read 14 skill entrypoints plus README, LICENSE, manifest and hook declarations.
- Admission scope: `research-only`; reuse mode: `clean-room-reference`; disposition: `approved` for behavioral study, not installation or product artifact admission. No upstream source, prompt, script or asset was copied or substantially ported.
- Destination: Analytix-authored coordination guidance in `.agents/skills/analytix-rc-control/` and the [methodology review](instruction-methodology-review-2026-09-07.md). Existing OpenSpec admission remains authoritative; no new hook, dependency or tool schema.
- Modifications/reviewer: coordinating agent, 2026-09-07. Selected outcome grouping, focused hypothesis/evidence, bounded delegation and scoped review principles; rejected global mandatory workflow, fixed correction caps and model downgrades.
- Evidence: canonical Skill validation, scoped OpenSpec validation, independent forward scenarios and task-owned diff review. This entry does not establish runtime, product, parity or release acceptance.

## 2026-09-07 — Owner / Slice coordination follow-up

- Classification: `reference-only`; behavior research, no copied/adapted upstream code, prompt, skill body or asset enters the product.
- Sources: OpenAI Codex `21bd5d3cdcf6a6a22112a5a9571d00669af8ed05` (Apache-2.0; multi_agents_v2 followup_task/send_message/wait, agent status and app-server README); Claude Code `ab9b2cf7bb9e4f98ff264c07a22e46d83c29c558` (LICENSE.md; restricted source, behavior-only agent-development study); dated official model, subagent and scheduling docs linked in the [follow-up review](owner-slice-control-review-2026-09-07.md).
- Destination: Analytix-authored RC Owner orchestration, monitoring and lifecycle guidance, plus accepted agent-handoff-review governance requirements. No new hook, daemon, dependency, external tool schema or installed Superpowers workflow.
- Decision/reviewer: coordinating agent, 2026-09-07. Adopt actionable status, bounded follow-up and safe context transfer; reject continuous polling, fixed model limits, automatic scheduling/retired-task reuse and inferred publication authority.
- Validation: scoped skill/schema/link checks, independent behavioral scenarios and one read-only native wait snapshot; see the delivery record for results. Live scheduling and product restart are outside this maintenance evidence.

## 2026-09-14 — local native Office adapter

- Source: `https://github.com/allotropia/zetajs`, release 1.2.0, commit
  `57360bcb0e7726ffa0e66567c8041261b959f8dd`. Public API/example references:
  `examples/standalone/office_thread.js` (Git blob `415317d507fb84ad0d71628ddf14d2e2f85fe099`),
  `docs/start.md` (`0b78375805ff8a2fe04eaf342283d6a3b7a13814`),
  `LICENSE` (`d76b2f3a46f96e8a5d587c5c47361964f1432ab3`).
- Destination: `src/main/office/surface/office-worker.js`, `office-surface.js`,
  `office-surface.html`. Mode: `port-and-adapt` for the UNO/bootstrap patterns;
  first-party bounded read-only message protocol, selection projection and
  compact preview controls. Reviewer: canonical
  coordinating agent, 2026-09-14. Disposition: `approved` for this MIT-covered
  source adaptation; native binary distribution remains separately blocked.
- License: MIT, Copyright (c) 2024 allotropia software GmbH and contributors.
  The complete notice is retained at `src/main/office/surface/NOTICE.txt`.
  No upstream binary, font, Qt module or LibreOffice source is copied here.
- Historical experiment: isolated real DOCX/XLSX/PPTX roundtrips and 21
  structure/shortcut checks passed for the earlier editable adapter, whose
  source and evidence are retained outside the product tree. The owner's later
  preview-only decision supersedes that design: current source removes mutation,
  export and save acknowledgements, and rejects Office writes in Main and Core.
  Current bounded development GUI evidence is recorded in
  `docs/analytix/builtin-office-host.md`; it does not establish packaged admission.
- External runtime pin: LibreOffice build `efaf0670b4d055f838a2849becb10f08aa06a257`;
  exact engine and font dependency hashes live in
  `packages/runtime-go/internal/adapters/outbound/officeengineassets/manifest.json`.
  This is a non-distributable source experiment. Complete binary, linked Qt and
  font redistribution obligations are unresolved; the bridge's MIT grant does
  not relicense those assets. Packaged use remains denied.

- Native CJK font: Noto Sans CJK SC Regular, official `notofonts/noto-cjk`
  commit `f8d157532fbfaeda587e826d4cd5b21a49186f7c`,
  `Sans/OTF/SimplifiedChinese/NotoSansCJKsc-Regular.otf`, SHA-256
  `2c76254f6fc379fddfce0a7e84fb5385bb135d3e399294f6eeb6680d0365b74b`.
  SIL Open Font License 1.1; the unmodified font, OFL text and font notice are
  retained together in the external experiment asset directory. Source admission
  approves only the fixed loader/manifest: no font bytes enter this repository.
  The WASM fontconfig loader uses `/usr/share/fonts/analytix` before startup.
  Independent native Writer rendering and export/reopen passed; this does not
  establish emoji coverage or waive the engine distribution gap above.

### 2026-09-14 — Office preview design-method intake

- Scope: research/tooling-only; no third-party CSS, JS, font or asset enters the product.
- Taste `ccbc15639c97057cbfcf32ecebc38ef716e4bb37`: selected redesign/minimalist prompts loaded unchanged in task-local tooling; MIT copyright/license retained with the selected files. Exact source paths/blobs, tool destinations and dispositions are recorded in [the scoped research record](office-preview-ui-research-2026-09-14.md).
- transitions.dev `598d3d6ad89dabb4bdf742fd2e887ca53914a888`: public behavior/method study only. Tool MIT and transition usage terms are distinct; prompt redistribution is unverified, so no Skill or recipe is copied into Analytix.
- Reuse mode: clean-room-reference for product changes; original Analytix implementation of observed UI requirements. Reviewer: current primary integrator. Verification: pinned remote/tree and file-level source review; runtime acceptance remains in the separate candidate QA record. Disposition: approved for this bounded research use, not distribution admission.


### 2026-09-15 — Existing AtlasFlow renderer extraction

This is a bounded refactor of already tracked plugin code at Analytix commit
`1a543a45c`, not a new download or a claim of a newly verified upstream commit.
Source files under `plugins/atlasflow/skills/atlasflow/` are
`renderers/architecture/render-architecture.mjs` (Git blob
`c4d80940ea68214ae1aab9e71cb4ab13a5c579f6`) and
`renderers/shared/cli.mjs` (blob
`902768e3906f04c48f91a48147bb906a21067b25`). The existing MIT license is blob
`c79eedb2f1bf2da044ab85cf4b5fc5d2ceabb806`; both 2026 tt-a1i and 2025 Cocoon AI
copyright notices remain in place and accompany the extracted modules.

Reuse mode: extract a common pure renderer into
`renderers/architecture/render-plan.mjs` and
`renderers/shared/svg-attributes.mjs`, with new bounded Canvas adaptation in
`renderers/architecture/canvas-scene.mjs` and
`renderers/shared/local-document.mjs`. The CLI retains file IO, schema validation
and its original output. Canvas accepts validated data, retains every fact and
source reference in escaped SVG metadata, and emits no executable script or
external resource reference. These complete local outputs are private artifacts;
this refactor grants no publication, source-reference or model-egress authority.

Reviewer: primary product integrator, separate from the implementation agent.
Canonical verification: `npm --prefix plugins/atlasflow/skills/atlasflow test`
passed five existing byte-identical golden renders and 105 Node tests, including
six new pure-import, layout, complete-ID and hostile-input checks. GUI, packaged
Canvas admission and full provenance for external redistribution remain separate.
