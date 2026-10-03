# analytix upstream code reuse provenance ledger

Status: Operational admission and attribution ledger.
Applies to: copied, ported, vendored, generated, extracted, or substantially
adapted upstream code, tests, prompts, documentation, binaries, and assets.
Current as of: 2026-10-02.
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

| OpenAI Apps SDK UI renderer icon candidates | `artifact-entering` | `openai/apps-sdk-ui` | 139 exact public MIT source blobs, real consumers, full decisions, notice and bounded adapter recorded below | `approved` for these vector sources only | Preserve pinned vectors/notice and explicit semantic/brand/data exceptions; current-web pixel identity is not claimed. |

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

## 2026-09-22 — exact dependency legal materials

The existing artifact legal owner now owns the bounded
[`build/dependency-legal/manifest.json`](../../../build/dependency-legal/manifest.json)
catalog and its unmodified `materials/` files. This is license/notice supply for
already locked dependencies, not adoption of another runtime or framework.
Each source/destination record binds the exact npm archive integrity and file
digests, root/runtime lock instances, material digest and fixed Git source where
available. The supplied exif-parser0.1.12, khroma2.1.0, type-fest4.41.0 and
canvas0.1.100 evidence was independently compared with the downloaded archives;
the eight supplied Git blob hashes were recomputed successfully.

Reuse mode: unmodified license or complete license-bearing README; no upstream
implementation copied. Copyright and full terms remain in those materials.
Sixteen records select the proven MIT branch; type-fest retains CC0 as well.
Fifteen records remain unresolved, including canvas embedded-component
provenance/notices. Reviewer: Codex, 2026-09-22. Validation belongs to the existing
legal reader, exact-content/owner negative tests and the newly built artifact;
none of this changes signing, publication or source-egress admission.

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

### OpenAI Apps SDK UI — five bounded renderer icon candidates (2026-10-02)

- Source: official [openai/apps-sdk-ui](https://github.com/openai/apps-sdk-ui),
  commit `0f00143c7a639906f1621fe58e1b6be7b5bea46d`. This is the public SDK icon source;
  it is not a claim that ChatGPT web or Codex desktop GUI source is public.
- Mode: `vendor`. Only these non-brand vector modules enter the renderer:

| Source at the pinned commit | Git blob | Analytix destination |
| --- | --- | --- |
| `src/components/Icon/svg/Sidebar.tsx` | `5189967d00927d5b2b1753434bd21235b504a4b3` | `src/renderer/src/design/openai-icons/svg/Sidebar.tsx` |
| `src/components/Icon/svg/ComposeEditSquare.tsx` | `50cfcfd3301b4cd0fa8bed8f392f30bdb8b94d34` | `src/renderer/src/design/openai-icons/svg/ComposeEditSquare.tsx` |
| `src/components/Icon/svg/Search.tsx` | `6c7fcb265c8b023a73781231db0ef60de0f35301` | `src/renderer/src/design/openai-icons/svg/Search.tsx` |
| `src/components/Icon/svg/PlusComposer.tsx` | `938d0b97c4b5114b0379586557c3684f435e754a` | `src/renderer/src/design/openai-icons/svg/PlusComposer.tsx` |
| `src/components/Icon/svg/MicLgDictate.tsx` | `6d0ce3876ed704da790f07fe388b0ed6ade7d8d6` | `src/renderer/src/design/openai-icons/svg/MicLgDictate.tsx` |

- License: SPDX `MIT`, Copyright 2025 OpenAI. Root source `LICENSE`
  blob `b8ac8dc0f960bbe0b0cc90cd06d58b672151773f`; these five files have
  no separate notices, external imports, embedded brands or linked assets.
  No upstream `NOTICE` or gitlink enters this selection.
- Attribution: exact MIT text in
  `src/renderer/src/design/openai-icons/LICENSE` and the OpenAI icon section of
  `THIRD_PARTY_NOTICES.md`. The latter is already included by
  `electron-builder.config.cjs` `extraResources`; no packaging rules change.
- Object mapping and scope: the adjacent `provenance.json` records each
  source/destination, blob id and SHA-256. Vector files remain byte-identical.
  `OpenAiUiIcons.ts` is an Analytix-owned adapter for size, inherited color,
  accessibility, ref and existing event props; it does not rewrite vector
  geometry or apply Lucide stroke/fill conventions.
- Mapping: `Sidebar` → existing left-panel disclosure; `ComposeEditSquare` →
  new-chat actions; `Search` → existing thread search; `PlusComposer` →
  existing add controls; `MicLgDictate` → existing microphone controls.
  Existing data/document SVG, product brands and unmatched UI icons remain
  explicitly outside this five-source admission.
- Evidence boundary: a current-web screenshot comparison at equal display
  size supports `visual-verified official-near` for these five groups.
  Microphone weight/base differ. Conservative literal/normalized path
  matching of 19 visible controls against 755 official vectors proves no
  exact hit; it does not prove pixel dissimilarity. None is marked exact,
  and this record does not claim that every UI icon has been replaced.
- Authority/reviewer: the human's visual/icon request and coordinating parent
  task's explicit five-candidate integration decision; renderer writer
  `01a0fc03-b4cc-71e4-8f2c-ad2c14326032`, 2026-10-02, reviewed source blobs,
  MIT text, selected file boundaries and packaged notice consumer.
  MIT grants this bounded reuse without separate written commercial permission.
- Validation: source object hashes verified before copying and by the
  adapter test; renderer typecheck, scoped lint and renderer build pass.
  Nine focused owner files pass 61 tests, including the accepted chrome
  placement and an old-layout regression negative. Isolated renderer
  screenshots cover 1440/900 windows, light/dark, actual workspace-button
  clicks, search focus/filter, dock navigation/resize/collapse and tab-close
  focus; small/medium/large scales have no page overflow at 900. Native
  titlebar geometry is CSS-simulated here. No performance, native-window,
  package, provider or release claim is made.
- Current disposition: `approved` admission for these five licensed vectors
  and candidate adaptation. Functional verification and any final shipping
  decision remain separate; unrelated inherited-license blockers are unchanged.


### OpenAI Apps SDK UI — complete semantic renderer icon selection (2026-10-03)

- Authority: user accepted official MIT consistency with explicit semantic, brand and data exceptions. This extends the five-source admission above, without a current ChatGPT Web pixel-identity claim.
- Source: `openai/apps-sdk-ui@0f00143c7a639906f1621fe58e1b6be7b5bea46d`; MIT `LICENSE` blob `b8ac8dc0f960bbe0b0cc90cd06d58b672151773f`, independently read and byte-compared with the existing notice.
- Mode: byte-identical `vendor`; each selected module imports React types only, has no embedded brands, external assets, gitlinks or separate file notice. No SDK dependency or bulk icon directory is admitted.
- Exact admitted objects and destinations (the adjacent renderer manifest retains SHA-256, mappings and real consumers):

| Pinned source | Git blob | Destination |
| --- | --- | --- |
| `src/components/Icon/svg/Archive.tsx` | `a90b2977a4c15d6ea55f98e49faff4ff5129e1cf` | `src/renderer/src/design/openai-icons/svg/Archive.tsx` |
| `src/components/Icon/svg/ArrowCurvedLeft.tsx` | `278d61f44a3ce46f0e35b7143709863adf73e4ef` | `src/renderer/src/design/openai-icons/svg/ArrowCurvedLeft.tsx` |
| `src/components/Icon/svg/ArrowDown.tsx` | `e7d2856d057ad0ba00f11a8e41369e8e469a7c5b` | `src/renderer/src/design/openai-icons/svg/ArrowDown.tsx` |
| `src/components/Icon/svg/ArrowLeft.tsx` | `8ab2c229072f10edcf6808906d9258234b56227e` | `src/renderer/src/design/openai-icons/svg/ArrowLeft.tsx` |
| `src/components/Icon/svg/ArrowRight.tsx` | `4ae471daec7cbdb680a5c348954749fd993efb1a` | `src/renderer/src/design/openai-icons/svg/ArrowRight.tsx` |
| `src/components/Icon/svg/ArrowRotateCcw.tsx` | `a5442790b225e5ae54c521932290568749b0250c` | `src/renderer/src/design/openai-icons/svg/ArrowRotateCcw.tsx` |
| `src/components/Icon/svg/ArrowRotateCw.tsx` | `c9b5513658e0c5826c0ebd7b415ea1bf3b2f768b` | `src/renderer/src/design/openai-icons/svg/ArrowRotateCw.tsx` |
| `src/components/Icon/svg/ArrowUp.tsx` | `a36a99e762efc5faf2c76879c4b262e345b095e7` | `src/renderer/src/design/openai-icons/svg/ArrowUp.tsx` |
| `src/components/Icon/svg/AtSign.tsx` | `91fec096d238cd902e1a452cdce28cd548a6b6ec` | `src/renderer/src/design/openai-icons/svg/AtSign.tsx` |
| `src/components/Icon/svg/AvatarProfile.tsx` | `ca9c19998667187305c5e5d058adf84f549bc151` | `src/renderer/src/design/openai-icons/svg/AvatarProfile.tsx` |
| `src/components/Icon/svg/BarChart.tsx` | `2033d19f11950cf00782eea615c4659a63bab7c6` | `src/renderer/src/design/openai-icons/svg/BarChart.tsx` |
| `src/components/Icon/svg/Bell.tsx` | `863f17bfc1d621c009565c1de9ca8a33d8931404` | `src/renderer/src/design/openai-icons/svg/Bell.tsx` |
| `src/components/Icon/svg/BookOpen.tsx` | `e9a9412f92aa2bad6705a2629f4b7470fd214c45` | `src/renderer/src/design/openai-icons/svg/BookOpen.tsx` |
| `src/components/Icon/svg/Brain.tsx` | `f956af789495fdff78d0df54f4ceb76786ae9283` | `src/renderer/src/design/openai-icons/svg/Brain.tsx` |
| `src/components/Icon/svg/Branch.tsx` | `0550babedd1fa56b4a66bf641a7b8b845cfdc943` | `src/renderer/src/design/openai-icons/svg/Branch.tsx` |
| `src/components/Icon/svg/BranchAlt.tsx` | `ee2021d00b0c08e07c761dbeb25fb26c9a4ea3ae` | `src/renderer/src/design/openai-icons/svg/BranchAlt.tsx` |
| `src/components/Icon/svg/Bug.tsx` | `95757c7729306b8215681f4b13eb69ae05c5ef65` | `src/renderer/src/design/openai-icons/svg/Bug.tsx` |
| `src/components/Icon/svg/Cabinet.tsx` | `d1f81bfdbfd7d4171802cbce2e5456f906a2849f` | `src/renderer/src/design/openai-icons/svg/Cabinet.tsx` |
| `src/components/Icon/svg/Calendar.tsx` | `f7506ba4f0edd08915380a3d26252d2ad36e34e8` | `src/renderer/src/design/openai-icons/svg/Calendar.tsx` |
| `src/components/Icon/svg/Chat.tsx` | `57cefed17e6e7b542be4e880032107dc9e02f178` | `src/renderer/src/design/openai-icons/svg/Chat.tsx` |
| `src/components/Icon/svg/ChatCompose.tsx` | `5e20a5cdde58a419b070189a76143dd2bab3c673` | `src/renderer/src/design/openai-icons/svg/ChatCompose.tsx` |
| `src/components/Icon/svg/ChatTripleDots.tsx` | `b4a56987766691df1401fef73cc852600adbe55d` | `src/renderer/src/design/openai-icons/svg/ChatTripleDots.tsx` |
| `src/components/Icon/svg/Check.tsx` | `fb2189f81fbec6da6d36763cbf816cda166dec15` | `src/renderer/src/design/openai-icons/svg/Check.tsx` |
| `src/components/Icon/svg/CheckCircle.tsx` | `18360cc84af987666328e2595e6c0a21e02529bc` | `src/renderer/src/design/openai-icons/svg/CheckCircle.tsx` |
| `src/components/Icon/svg/ChevronDown.tsx` | `11d760b4f97e396274baa30335b29993bb8ceb98` | `src/renderer/src/design/openai-icons/svg/ChevronDown.tsx` |
| `src/components/Icon/svg/ChevronLeft.tsx` | `8af6616e21a7f19acc763f49fc9f05a2c573f489` | `src/renderer/src/design/openai-icons/svg/ChevronLeft.tsx` |
| `src/components/Icon/svg/ChevronRight.tsx` | `436f1142ccf7f08397c3a923fa70ab95c87aed39` | `src/renderer/src/design/openai-icons/svg/ChevronRight.tsx` |
| `src/components/Icon/svg/ChevronUp.tsx` | `914d65115a432ec1eb26f26217610f37872580d2` | `src/renderer/src/design/openai-icons/svg/ChevronUp.tsx` |
| `src/components/Icon/svg/CircleDashed.tsx` | `a48c9cb6bc150efb42bd81e9860f887e1b619080` | `src/renderer/src/design/openai-icons/svg/CircleDashed.tsx` |
| `src/components/Icon/svg/ClappingBoardClosed.tsx` | `ba366a24f6d19ed16b129e11cfeb7821f134df34` | `src/renderer/src/design/openai-icons/svg/ClappingBoardClosed.tsx` |
| `src/components/Icon/svg/Clock.tsx` | `855568424611b20420ead9317bba7194a9c470df` | `src/renderer/src/design/openai-icons/svg/Clock.tsx` |
| `src/components/Icon/svg/Code.tsx` | `1726b9b4ea5c272636d25b9eea05cd44cbc97696` | `src/renderer/src/design/openai-icons/svg/Code.tsx` |
| `src/components/Icon/svg/CollapseLarge.tsx` | `7aadb66da1e73297b573b35593fce4c416af685a` | `src/renderer/src/design/openai-icons/svg/CollapseLarge.tsx` |
| `src/components/Icon/svg/ColorTheme.tsx` | `6f7657ee1607e25d419d8831c2dda865f832e2f6` | `src/renderer/src/design/openai-icons/svg/ColorTheme.tsx` |
| `src/components/Icon/svg/Compare.tsx` | `47d964c18bb6d01a26fbd0cf8937f6d2cb811427` | `src/renderer/src/design/openai-icons/svg/Compare.tsx` |
| `src/components/Icon/svg/CompareArrows.tsx` | `7d031d76cbbbf0120adb41021d7b2e36f52c16fc` | `src/renderer/src/design/openai-icons/svg/CompareArrows.tsx` |
| `src/components/Icon/svg/ComposeEditSquare.tsx` | `50cfcfd3301b4cd0fa8bed8f392f30bdb8b94d34` | `src/renderer/src/design/openai-icons/svg/ComposeEditSquare.tsx` |
| `src/components/Icon/svg/Connect.tsx` | `2e1735cf737a6db472a0efc8030da61d0812472a` | `src/renderer/src/design/openai-icons/svg/Connect.tsx` |
| `src/components/Icon/svg/Copy.tsx` | `741d9c10ae7e59814b326f7189cbef0342629067` | `src/renderer/src/design/openai-icons/svg/Copy.tsx` |
| `src/components/Icon/svg/CreditCard.tsx` | `b23826c45c8a003366259823c484fcf1c0e86bcc` | `src/renderer/src/design/openai-icons/svg/CreditCard.tsx` |
| `src/components/Icon/svg/Cube.tsx` | `bac41122d00a30fa9fd61967129b5d2b2d6cbef4` | `src/renderer/src/design/openai-icons/svg/Cube.tsx` |
| `src/components/Icon/svg/Cursor.tsx` | `916c333bf1b7cc4ea7aee695675c05f1f527e6b9` | `src/renderer/src/design/openai-icons/svg/Cursor.tsx` |
| `src/components/Icon/svg/Desktop.tsx` | `92dce441208c8019013f24cbbb8a885575b6a16c` | `src/renderer/src/design/openai-icons/svg/Desktop.tsx` |
| `src/components/Icon/svg/Document.tsx` | `f5f826cc4fe471da70f7ca723213689d75900945` | `src/renderer/src/design/openai-icons/svg/Document.tsx` |
| `src/components/Icon/svg/DotsHorizontal.tsx` | `f14768d0a4e39f83694303688babbf8c2f15a005` | `src/renderer/src/design/openai-icons/svg/DotsHorizontal.tsx` |
| `src/components/Icon/svg/Download.tsx` | `5a30f3f3dc0cef64939d891bae98dcac95f71f4d` | `src/renderer/src/design/openai-icons/svg/Download.tsx` |
| `src/components/Icon/svg/EditPencil.tsx` | `d2dba782ffe07fc8e1c9d4bdeecae0c817452043` | `src/renderer/src/design/openai-icons/svg/EditPencil.tsx` |
| `src/components/Icon/svg/EmptyCircle.tsx` | `4e5c70eb61dda4a56c1e1a18b8fbe24b4556f0a1` | `src/renderer/src/design/openai-icons/svg/EmptyCircle.tsx` |
| `src/components/Icon/svg/Error.tsx` | `58924f360258317a463b2cd22aafa7602a2a0050` | `src/renderer/src/design/openai-icons/svg/Error.tsx` |
| `src/components/Icon/svg/ExclamationMarkCircle.tsx` | `2b4de2f363abdf78adee30e355330a4bf252afe5` | `src/renderer/src/design/openai-icons/svg/ExclamationMarkCircle.tsx` |
| `src/components/Icon/svg/ExitLogout.tsx` | `fd5a61eeb30944f58b743378d778aaea961b8268` | `src/renderer/src/design/openai-icons/svg/ExitLogout.tsx` |
| `src/components/Icon/svg/Expand.tsx` | `cb136fdcb2097472050bf73fc7869062eccaa389` | `src/renderer/src/design/openai-icons/svg/Expand.tsx` |
| `src/components/Icon/svg/ExpandLarge.tsx` | `3fe3b014d7b926e9b8f9651a5686d875436d2949` | `src/renderer/src/design/openai-icons/svg/ExpandLarge.tsx` |
| `src/components/Icon/svg/ExternalLink.tsx` | `5311db4f174a018863a3db80f0069ee82e6ddeba` | `src/renderer/src/design/openai-icons/svg/ExternalLink.tsx` |
| `src/components/Icon/svg/Eye.tsx` | `c885fbf284288432f912d681bce754aa42d82c5b` | `src/renderer/src/design/openai-icons/svg/Eye.tsx` |
| `src/components/Icon/svg/EyeOff.tsx` | `355e59cd46f81f503a9231e774504b9e0b976bb3` | `src/renderer/src/design/openai-icons/svg/EyeOff.tsx` |
| `src/components/Icon/svg/FileBlank.tsx` | `bb378c78fb6fe40726ed708c14db360794646258` | `src/renderer/src/design/openai-icons/svg/FileBlank.tsx` |
| `src/components/Icon/svg/FileCode.tsx` | `2f5fddbb4dce77919ef70b2a6a7d919955a9f704` | `src/renderer/src/design/openai-icons/svg/FileCode.tsx` |
| `src/components/Icon/svg/FileDocument.tsx` | `9a3ccdfedef885e77058262d66c0e9befa65469b` | `src/renderer/src/design/openai-icons/svg/FileDocument.tsx` |
| `src/components/Icon/svg/FilePresentation.tsx` | `f0aa9501c3e27060563845fe56c8ae62e474f0bc` | `src/renderer/src/design/openai-icons/svg/FilePresentation.tsx` |
| `src/components/Icon/svg/FileSpreadsheet.tsx` | `791a5fbf218f5f6537d2ec0fa51c86707e3cd51d` | `src/renderer/src/design/openai-icons/svg/FileSpreadsheet.tsx` |
| `src/components/Icon/svg/Filter.tsx` | `22b94d2476397a5e936294c8ae0e46d718031f19` | `src/renderer/src/design/openai-icons/svg/Filter.tsx` |
| `src/components/Icon/svg/Flask.tsx` | `5167def690d5f43dc6f72a5a855efcd4f3b5ddb1` | `src/renderer/src/design/openai-icons/svg/Flask.tsx` |
| `src/components/Icon/svg/Folder.tsx` | `8cd94c5392cf967f6d482a3d6e7779d4cc6ee895` | `src/renderer/src/design/openai-icons/svg/Folder.tsx` |
| `src/components/Icon/svg/FolderDocumentsFinder.tsx` | `0d5fc058e1d02ad5912671f5c5b2a13b493c9163` | `src/renderer/src/design/openai-icons/svg/FolderDocumentsFinder.tsx` |
| `src/components/Icon/svg/FolderOpen.tsx` | `b7a2401dca37ddab4932ef72923d3f09b617a02a` | `src/renderer/src/design/openai-icons/svg/FolderOpen.tsx` |
| `src/components/Icon/svg/FolderPlus.tsx` | `f3cb0961ed6b2b100bbd12820001075119b18e9d` | `src/renderer/src/design/openai-icons/svg/FolderPlus.tsx` |
| `src/components/Icon/svg/GenerateSuggestedEdits.tsx` | `a7f89416ab119a33cceefc24ba9e187fba4c2cea` | `src/renderer/src/design/openai-icons/svg/GenerateSuggestedEdits.tsx` |
| `src/components/Icon/svg/Globe.tsx` | `71b56fbd0e678718f138b8eddc49a4469af908a8` | `src/renderer/src/design/openai-icons/svg/Globe.tsx` |
| `src/components/Icon/svg/Grid.tsx` | `88c2a46d9063ee56476eed54de3766f0745c0fd8` | `src/renderer/src/design/openai-icons/svg/Grid.tsx` |
| `src/components/Icon/svg/HandRaised.tsx` | `6bc1d2e4cb35a637b6d2f97372100fe70fa4c033` | `src/renderer/src/design/openai-icons/svg/HandRaised.tsx` |
| `src/components/Icon/svg/Identity.tsx` | `2abbb035907110448a1662e313d2ebc4febe8b47` | `src/renderer/src/design/openai-icons/svg/Identity.tsx` |
| `src/components/Icon/svg/ImageSquare.tsx` | `aba0347ab7c0db230aba584aa7973c4b137e7a77` | `src/renderer/src/design/openai-icons/svg/ImageSquare.tsx` |
| `src/components/Icon/svg/InfoCircle.tsx` | `c31b2b255b0813b40fe8576d9fc337ad1eb474ac` | `src/renderer/src/design/openai-icons/svg/InfoCircle.tsx` |
| `src/components/Icon/svg/Invoice.tsx` | `1f003291d1270bbf801ff47610470dde2e978346` | `src/renderer/src/design/openai-icons/svg/Invoice.tsx` |
| `src/components/Icon/svg/Key.tsx` | `2db4e0b0149d54edfc5ce63cb78c81cb344163ee` | `src/renderer/src/design/openai-icons/svg/Key.tsx` |
| `src/components/Icon/svg/Keyboard.tsx` | `43fd263d32da0de83c4c1a507e497e848c2cbd0a` | `src/renderer/src/design/openai-icons/svg/Keyboard.tsx` |
| `src/components/Icon/svg/Lightbulb.tsx` | `b913340d97f10c220f669bc30d966d87c44020af` | `src/renderer/src/design/openai-icons/svg/Lightbulb.tsx` |
| `src/components/Icon/svg/Lock.tsx` | `1820b4adf95fe20f2da8c1f50635d57b4e258e47` | `src/renderer/src/design/openai-icons/svg/Lock.tsx` |
| `src/components/Icon/svg/LockKeyHole.tsx` | `4bc4da46a8b04d07c8c083cc265114f80860be57` | `src/renderer/src/design/openai-icons/svg/LockKeyHole.tsx` |
| `src/components/Icon/svg/MicLgDictate.tsx` | `6d0ce3876ed704da790f07fe388b0ed6ade7d8d6` | `src/renderer/src/design/openai-icons/svg/MicLgDictate.tsx` |
| `src/components/Icon/svg/Minus.tsx` | `25765caec99a36ce216eaa147d5a34ebd37e82da` | `src/renderer/src/design/openai-icons/svg/Minus.tsx` |
| `src/components/Icon/svg/Mobile.tsx` | `0fd08332c83ea8bcda7fd95dc24dfeb651b98650` | `src/renderer/src/design/openai-icons/svg/Mobile.tsx` |
| `src/components/Icon/svg/Moon.tsx` | `bc82c8ff8e3afb6828f9c2934c584bc1cfea4bf4` | `src/renderer/src/design/openai-icons/svg/Moon.tsx` |
| `src/components/Icon/svg/Music.tsx` | `d8454bd0d1510751814d14a2cb6d4d66e71ee59f` | `src/renderer/src/design/openai-icons/svg/Music.tsx` |
| `src/components/Icon/svg/Nodes.tsx` | `3c9f8354a16127ccbdfe3a695d43e28eb66dbc7a` | `src/renderer/src/design/openai-icons/svg/Nodes.tsx` |
| `src/components/Icon/svg/NotebookPencil.tsx` | `48f8735e3449d222e02dc2bfc7cad05985074182` | `src/renderer/src/design/openai-icons/svg/NotebookPencil.tsx` |
| `src/components/Icon/svg/On.tsx` | `bd74579b479a170388b97b881f4f4b76d1053e2b` | `src/renderer/src/design/openai-icons/svg/On.tsx` |
| `src/components/Icon/svg/PageBlank.tsx` | `0643890f0a215a3b56b8d526dbec655742b3aa50` | `src/renderer/src/design/openai-icons/svg/PageBlank.tsx` |
| `src/components/Icon/svg/Paperclip.tsx` | `afa2bb6651cdc5a8ac35e0e4db7255fa1ef75ab4` | `src/renderer/src/design/openai-icons/svg/Paperclip.tsx` |
| `src/components/Icon/svg/PauseOutline.tsx` | `ac0e78d6f9ae929afadeae667255ed6ca1c8b0df` | `src/renderer/src/design/openai-icons/svg/PauseOutline.tsx` |
| `src/components/Icon/svg/PauseSm.tsx` | `4a34bbe0fc8d155cecede5b0495de3efb7d5cc08` | `src/renderer/src/design/openai-icons/svg/PauseSm.tsx` |
| `src/components/Icon/svg/Pin.tsx` | `36fee74465a7d4929bc8552e760c8b1e58da0618` | `src/renderer/src/design/openai-icons/svg/Pin.tsx` |
| `src/components/Icon/svg/PinFilled.tsx` | `c79897d4c7c8151ee2f8dc0a243be24aa9518014` | `src/renderer/src/design/openai-icons/svg/PinFilled.tsx` |
| `src/components/Icon/svg/PlayOutline.tsx` | `458a3a2c82455dcfb39f876e221317c2724a626c` | `src/renderer/src/design/openai-icons/svg/PlayOutline.tsx` |
| `src/components/Icon/svg/PlaySm.tsx` | `41cac01834973d633b1a2c0ca694fcbb11617bef` | `src/renderer/src/design/openai-icons/svg/PlaySm.tsx` |
| `src/components/Icon/svg/Plugin.tsx` | `6f7b93891416ebeef48e02d33ccdb547d09eaa60` | `src/renderer/src/design/openai-icons/svg/Plugin.tsx` |
| `src/components/Icon/svg/PlusCircle.tsx` | `68c0ed8380429a6e2ae9a922419603b58263dcbe` | `src/renderer/src/design/openai-icons/svg/PlusCircle.tsx` |
| `src/components/Icon/svg/PlusComposer.tsx` | `938d0b97c4b5114b0379586557c3684f435e754a` | `src/renderer/src/design/openai-icons/svg/PlusComposer.tsx` |
| `src/components/Icon/svg/PopOutWindow.tsx` | `af7d880fe82d2815c09468743759a3885e751af2` | `src/renderer/src/design/openai-icons/svg/PopOutWindow.tsx` |
| `src/components/Icon/svg/PullRequestMerged.tsx` | `c15b4e22b91206792b1597bb219e0aaf3de50a2a` | `src/renderer/src/design/openai-icons/svg/PullRequestMerged.tsx` |
| `src/components/Icon/svg/Quote.tsx` | `2986f3e9b6b72d919feed8ae36fb3e1da365316e` | `src/renderer/src/design/openai-icons/svg/Quote.tsx` |
| `src/components/Icon/svg/QuoteReplyFilledQuoteXs.tsx` | `99c16953a50dbb85e1ae773b9a5e35e5d6852a71` | `src/renderer/src/design/openai-icons/svg/QuoteReplyFilledQuoteXs.tsx` |
| `src/components/Icon/svg/Reload.tsx` | `fc2677f9c3c9275645a7fba6b0e1f9eeaad8ccbb` | `src/renderer/src/design/openai-icons/svg/Reload.tsx` |
| `src/components/Icon/svg/RobotHead.tsx` | `d8d055500259ff04b9feea5e9c6674c870f2b27b` | `src/renderer/src/design/openai-icons/svg/RobotHead.tsx` |
| `src/components/Icon/svg/Search.tsx` | `6c7fcb265c8b023a73781231db0ef60de0f35301` | `src/renderer/src/design/openai-icons/svg/Search.tsx` |
| `src/components/Icon/svg/SettingsCog.tsx` | `02027f336b7f6d38cbee49c822eb88a5770ae281` | `src/renderer/src/design/openai-icons/svg/SettingsCog.tsx` |
| `src/components/Icon/svg/SettingsSlider.tsx` | `dee5036972bcbf9dac680f1959df1210ae4669ea` | `src/renderer/src/design/openai-icons/svg/SettingsSlider.tsx` |
| `src/components/Icon/svg/SettingsWrench.tsx` | `ce39336b1eb5c6def3953c9ed889c492f1d75129` | `src/renderer/src/design/openai-icons/svg/SettingsWrench.tsx` |
| `src/components/Icon/svg/Share.tsx` | `f305c3c7dd75b24ba07be538b144e49cfc9cf152` | `src/renderer/src/design/openai-icons/svg/Share.tsx` |
| `src/components/Icon/svg/ShieldCheck.tsx` | `93e4e3a661bf438108b954a464488a1201cd060a` | `src/renderer/src/design/openai-icons/svg/ShieldCheck.tsx` |
| `src/components/Icon/svg/Sidebar.tsx` | `5189967d00927d5b2b1753434bd21235b504a4b3` | `src/renderer/src/design/openai-icons/svg/Sidebar.tsx` |
| `src/components/Icon/svg/SidebarCollapseRight.tsx` | `54f9746564f02664957176797ab80e547d8086bc` | `src/renderer/src/design/openai-icons/svg/SidebarCollapseRight.tsx` |
| `src/components/Icon/svg/SidebarOpenRightAlt.tsx` | `9dd8090f8ee486d280cc963bee8796cee13e0b70` | `src/renderer/src/design/openai-icons/svg/SidebarOpenRightAlt.tsx` |
| `src/components/Icon/svg/SimpleSmile.tsx` | `fc753e5efacf7d5add6c2a9698567aa91d4b71e6` | `src/renderer/src/design/openai-icons/svg/SimpleSmile.tsx` |
| `src/components/Icon/svg/Sparkles.tsx` | `5c26a7d4e90ed404a0f6aa2e521aefa50cf0960c` | `src/renderer/src/design/openai-icons/svg/Sparkles.tsx` |
| `src/components/Icon/svg/Spelling.tsx` | `539d61df273243b87aa5d0d8d51c998c2cdc265e` | `src/renderer/src/design/openai-icons/svg/Spelling.tsx` |
| `src/components/Icon/svg/Stack.tsx` | `05b1df481a484a76fbb8c0034779d7f682428807` | `src/renderer/src/design/openai-icons/svg/Stack.tsx` |
| `src/components/Icon/svg/Stop.tsx` | `892e17e37213404810fa426990dc2a36dfb8fc06` | `src/renderer/src/design/openai-icons/svg/Stop.tsx` |
| `src/components/Icon/svg/Storage.tsx` | `a4b9ff66a34ea76d271a05db3058dec0e4861172` | `src/renderer/src/design/openai-icons/svg/Storage.tsx` |
| `src/components/Icon/svg/Sun.tsx` | `730f21239d8feea8fd0c3450a306d66a9541da89` | `src/renderer/src/design/openai-icons/svg/Sun.tsx` |
| `src/components/Icon/svg/Tasks.tsx` | `e4b75b8977ca07fdf19e344319157368b96cdc48` | `src/renderer/src/design/openai-icons/svg/Tasks.tsx` |
| `src/components/Icon/svg/Terminal.tsx` | `2fc3f026dfb4e00fc9004630448faf9d86c5ee96` | `src/renderer/src/design/openai-icons/svg/Terminal.tsx` |
| `src/components/Icon/svg/Text.tsx` | `b55740990c9b3f3d1cd69b94fabf7d767b77912e` | `src/renderer/src/design/openai-icons/svg/Text.tsx` |
| `src/components/Icon/svg/Timer.tsx` | `a3fb1213502458f0ef1a2f20c4437bd945e44a3a` | `src/renderer/src/design/openai-icons/svg/Timer.tsx` |
| `src/components/Icon/svg/Tools.tsx` | `abdaadf18c97a147dcdb181576df166136fb86fd` | `src/renderer/src/design/openai-icons/svg/Tools.tsx` |
| `src/components/Icon/svg/Trash.tsx` | `fb3f910b4846caaafea9c8b755d8791382cc7fa5` | `src/renderer/src/design/openai-icons/svg/Trash.tsx` |
| `src/components/Icon/svg/Unarchive.tsx` | `f04cbddf19f5db9aef176f00a5f5143b6ce8dacd` | `src/renderer/src/design/openai-icons/svg/Unarchive.tsx` |
| `src/components/Icon/svg/Undo.tsx` | `77bff2e4a6e6af0f34a69b95e8a406a6db0fa506` | `src/renderer/src/design/openai-icons/svg/Undo.tsx` |
| `src/components/Icon/svg/Upgrade.tsx` | `492ac156b8478af0e548b384c7e7cca2ea9b9ad5` | `src/renderer/src/design/openai-icons/svg/Upgrade.tsx` |
| `src/components/Icon/svg/Upscale.tsx` | `a321b7bcbafc449eb253af9d79487edd7972a293` | `src/renderer/src/design/openai-icons/svg/Upscale.tsx` |
| `src/components/Icon/svg/User.tsx` | `a96a6d6289cf0e6772db1e39b070afcadcd1111b` | `src/renderer/src/design/openai-icons/svg/User.tsx` |
| `src/components/Icon/svg/Users.tsx` | `f5d1f8bd80c466a9e8263b4334be4a680429c899` | `src/renderer/src/design/openai-icons/svg/Users.tsx` |
| `src/components/Icon/svg/Video.tsx` | `811e5a6df0ec477581018dc4ab1e6f82a20a83d4` | `src/renderer/src/design/openai-icons/svg/Video.tsx` |
| `src/components/Icon/svg/Voice5BarsSoundwave.tsx` | `80dfb7fd644f330650bb48203fea6cac37abc487` | `src/renderer/src/design/openai-icons/svg/Voice5BarsSoundwave.tsx` |
| `src/components/Icon/svg/Warning.tsx` | `77cc9a56793a009fb342da062b43d7cd8048adbe` | `src/renderer/src/design/openai-icons/svg/Warning.tsx` |
| `src/components/Icon/svg/Widget.tsx` | `3a27a6c5a2b3ce12a72554422308ee78ce477e7d` | `src/renderer/src/design/openai-icons/svg/Widget.tsx` |
| `src/components/Icon/svg/X.tsx` | `006367f21db11a86891db2bdec260d2e1f7c2db0` | `src/renderer/src/design/openai-icons/svg/X.tsx` |
| `src/components/Icon/svg/XCircle.tsx` | `8d7cae23baf73779f66951ee20e42ec4c5144651` | `src/renderer/src/design/openai-icons/svg/XCircle.tsx` |

- Reviewer: delegated UI coordinating writer, 2026-10-03; fixed source object, license, import/asset boundary and 20px contact sheet reviewed before copy. Attribution remains the exact MIT text in `openai-icons/LICENSE` and `THIRD_PARTY_NOTICES.md`; packaged NOTICE propagation remains mandatory.
- Validation: exact 139 vendor/manifest/adapter source sets and source/MIT hashes passed; the complete 203 export, 37 inline and 22 registry decisions passed owner checks. Focused renderer tests (159 distinct cases across the initial run and necessary repaired reruns), type checking, scoped lint, renderer build and packaged-NOTICE positive/negative selftests passed. Eight native Electron synthetic combinations covered light/dark, 900/1440 widths, 100/125% scale and 14–24px glyphs; actual sidebar menu, pin and Escape focus were checked. Independent read-only semantic/license/visual review found one stale five-source scope description, corrected before delivery. Existing reduced-motion spinner behavior remains outside this icon change. This evidence does not establish installed-product, Provider, performance, CI or release acceptance.
