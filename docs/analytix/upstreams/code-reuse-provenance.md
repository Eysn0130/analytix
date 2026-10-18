# analytix upstream code reuse provenance ledger

Status: Operational admission and attribution ledger.
Applies to: copied, ported, vendored, generated, extracted, or substantially
adapted upstream code, tests, prompts, documentation, binaries, and assets.
Current as of: 2026-10-11 for scoped source-record reconciliation and the Schedule and Present candidates below;
other reviews retain their recorded dates and evidence bounds.
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

The DSH port follows the user-confirmed [migration rule](deepseek-harness-presentation-sync.md#confirmed-migration-rule).
Cross-product defect repairs require official implementation evidence and
recorded behavior changes, plus this ledger's existing file-level license,
provenance and admission checks; the rule grants no additional reuse rights.

## Current Review Queue

| Item | Admission scope | Source id | Evidence | Current disposition | Required closure |
| --- | --- | --- | --- | --- | --- |
| DeepSeek Harness user message CSS and shared calendar destinations | `artifact-entering` | `deepseek-harness` | [Existing pinned objects and updated destination map](#dsh-user-message-destination-update--2026-10-15-utc) | `candidate` | Preserve existing MIT notice and native editing/privacy contracts; combined regression and rendered acceptance remain pending. |
| DeepSeek Harness independent public trajectory v2 | `artifact-entering` | `deepseek-harness` | [Ten pinned presentation sources and v2 destinations](#deepseek-harness-independent-public-trajectory-source-intake--2026-10-15-utc) | `candidate` | Preserve existing MIT notice, public projection and protected Inspect ownership; candidate application and coordinated execution acceptance remain pending. |
| DeepSeek Harness Markdown, protected-local Shell/Read, central composition and inline process/work-details policy | `artifact-entering` | `deepseek-harness` | [Pinned source objects, retained MIT notice and scoped 2100397 integration](deepseek-harness-presentation-sync.md#2026-10-10--scoped-source-admission-at-2100397) | `approved` for the listed source objects and adaptations only | Source admission is checked at candidate `210039729892bf80fb8e0e9a7c909b70266ec6bc`; all listed source path/blob pairs match fixed pin `5badb150…`. Earlier `0ac4f84d…` and subsequent source freezes retain their historical test/environment bounds. Existing protected-local authority, explicit Footer Fork semantic adaptation, selected Browse three-path artwork and full MIT notice are preserved. Full DSH parity, native/installed behavior, current CI/main, packaged distribution and release remain OPEN. |
| DeepSeek Harness settled grep/glob Search leaves | `artifact-entering` | `deepseek-harness` | [Four pinned leaf objects and protected-local adaptations](#deepseek-harness-settled-search-candidate); [current source admission](deepseek-harness-presentation-sync.md#2026-10-10--scoped-source-admission-at-2100397) | `approved` for these four source leaves and adapters only | The four recorded source blobs and current destinations are verified in integrated candidate `210039729892bf80fb8e0e9a7c909b70266ec6bc`; historical checkpoint `155951ff…` is its ancestor. Prior freeze hashes and validation counts remain bound to their original inputs. Preserve fixed source maps, headers, complete MIT notice and existing protected-local authority; full Search/DSH parity, native, current CI/main, packaged distribution and release remain OPEN. |
| DeepSeek Harness capped process-group scroll adaptation | `artifact-entering` | `deepseek-harness` | [Two pinned scroll-source objects and existing renderer owner](deepseek-harness-presentation-sync.md#capped-process-group-scroll-source-delta-2026-10-11) | `candidate` | Exact pin/blob and MIT notice are recorded; focused process-scroll regressions pass, while the wider visual unit retains an unresolved Records counterexample. Independent delta review, canonical integration and runtime acceptance remain pending. Preserve existing display authority and the full notice; no outer transcript controller, upstream store or execution authority is imported. |
| DeepSeek Harness process glyphs and public searchable folds | `artifact-entering` | `deepseek-harness` | [Exact selected glyph/helper objects and public/private adaptations](deepseek-harness-presentation-sync.md#process-glyphs-and-public-searchable-folds-source-candidate-2026-10-11); [bounded integration evidence](deepseek-harness-presentation-sync.md#svgfind-and-width-source-integration-checkpoint-2026-10-11) | `candidate` | Fixed pin/blob and scoped MIT reads are verified; A17 is locally integrated with the byte-matched SVG/find and width validation. Preserve selected Plan/AgentPreset artwork, existing Question/Browse/Globe, full notice and private leaf retirement. Native Chromium/Electron find/focus/visibility/scroll and full admission/parity remain OPEN. |
| DeepSeek Harness main Conversation width controls | `artifact-entering` | `deepseek-harness` | [Exact pinned width/phase/style sources and existing renderer owners](deepseek-harness-presentation-sync.md#main-conversation-width-candidate-2026-10-11); [bounded integration evidence](deepseek-harness-presentation-sync.md#svgfind-and-width-source-integration-checkpoint-2026-10-11) | `candidate` | Pinned source objects and full MIT notice are retained; B8 is locally integrated with byte-matched grouped tests, types, affected lint and renderer build. Continuable parent and elected overlay producer seams, native UI, full admission and parity remain OPEN. No upstream store, runtime phase or execution authority is imported. |
| DeepSeek Harness generated Office deliveries and selected shared file artwork | `artifact-entering` | `deepseek-harness` | [Exact ten-object map, selected artwork and license bounds](deepseek-harness-presentation-sync.md#generated-office-deliveries-and-shared-file-artwork-source-candidate-2026-10-11); [bounded admission record](#deepseek-harness-generated-office-deliveries-and-selected-file-artwork-candidate) | `candidate` | The 11-file renderer unit is locally integrated with Plan7/Shell5 and bounded fixture/type/lint/renderer evidence. Real Office receipts, protected-local Inspect, existing preview authority and full MIT notice are retained; native and full admission remain OPEN. Selected W/P/MD/file/grid/DeliverDoc artwork is explicitly included; CodeFileIcon/simple-icons and unselected assets remain excluded. Full DSH declaration/action parity is OPEN. |
| DeepSeek Harness submitted readonly Plan history cards | `artifact-entering` | `deepseek-harness` | [Pinned PlanCard/CSS subsets and shared Markdown artwork](deepseek-harness-presentation-sync.md#generated-office-deliveries-and-shared-file-artwork-source-candidate-2026-10-11) | `candidate` | Plan7 is locally integrated with the byte-matched 23-file source combination. Exact signed-history selectors and fresh Host-authorized private reads remain the only Plan identity/display authority. History offers Open only; normal pending approval retains its existing implementation route. Native and full DSH parity remain OPEN. |
| DeepSeek Harness single-message feedback actions, optional modal and canonical-log semantics | `artifact-entering` | `deepseek-harness` | [Exact pinned feedback/selected-artwork source map and bounded candidate evidence](deepseek-harness-presentation-sync.md#single-message-feedback-source-candidate-2026-10-11) | `candidate` | Forty isolated source paths retain the b090 base and full MIT notice. Two P2 owner/portal deltas are repaired with 14 affected-case, web-type and five-file lint passes; byte-identical Go/unrelated evidence is reused. Existing canonical event-log and protected authority/content guards are preserved. Feedback commit `c5b8434765b76128a081462bee21c9f3aef1b464` is locally integrated into the coordinated branch at `90c2578e5521bbcd11e606d7e2588871ebcc1f66`; 38 blobs and both IPC hunk deltas are verified. Current combined validation, final admission, native/full parity, packaging, CI and release remain OPEN. No upstream store/runtime, log upload or third-party sending code is imported. |
| DeepSeek Harness Schedule creation tail and native right task-detail presentation | `artifact-entering` | `deepseek-harness` | [Exact pinned Schedule/artwork/token objects, MIT notice and local source binding](deepseek-harness-presentation-sync.md#schedule-presentation-source-candidate-2026-10-11) | `candidate` | UI37, Analytix-owned Backend20 and Markdown2 are locally integrated at `eff56c96d3975d6d3435bd1d667089ce43b0a96a`, tree `fd586ee76b6c68b0baec811b647e72b148df8712`; all 59 blobs/modes bind the green exact4a62 archive. Selected 328 tests, web/node/runtime types, affected lint and one renderer-only build pass; unchanged Backend 86 Go/133 TS evidence is reused separately. Fixed pin `5badb150…`, selected object map and complete MIT notice are retained. Fresh Host receipt/catalog authority and actual Workbench Sidebar are preserved. Native/installed visual, absent delivery/timing/session capabilities, full admission/parity, CI/main, packaging and release remain OPEN. |
| DeepSeek Harness Subagent Started rows and Todo failure/parallel-active presentation | `artifact-entering` | `deepseek-harness` | [Selected fixed objects, native adaptations and exact combined source binding](deepseek-harness-presentation-sync.md#subagent-and-todo-local-integration-2026-10-11) | `candidate` | Subagent15, Todo failure3 and Todo parallel18 are locally integrated at `41617a787278edeb0c413778524a0959a621727d`, tree `c3eab1b3a02e15bfd35055d04c9f831709867b78`. Fixed pin `5badb150…`, MIT and the complete notice apply. Typed Started receipt/native child controls, verbatim protected failed/stopped IN, concurrent active Todo contracts and authoritative store updates retain existing owners. Exact combined nine-file 222/222 and separate store 131/131 pass; types/lint and one renderer-only build are source-bound, without adding old owner counts. Native/installed UI, full DSH parity, current CI/main, packaging and release remain OPEN. |
| DeepSeek Harness Present declaration/process cards, grouped file deliveries and native file-action presentation | `artifact-entering` | `deepseek-harness` | [Pinned source map and local integration checkpoint](deepseek-harness-presentation-sync.md#present-declaration-file-actions-and-local-integration-2026-10-11); [bounded admission record](#deepseek-harness-present-declaration-and-file-actions-candidate) | `candidate` | TS/Host/UI44 `47989644c…` and Go20 `fe915a405…` are normally merged at `17ff805a7fe3ab9338bf2ca511947c01cd497d88`, tree `3292e9a10c981d6ed32968e91699f3939fb7588d`; all64 modes/blobs match their frozen sources. Existing 20 unique fixture suites / 966 cases use scoped reuse; Go V3 evidence is reported separately. One fresh-archive renderer-only build passes (644 files / 42,416,109 bytes). Three existing provenance/notice documents record the closure; original rejection and preflight diagnostic records remain preserved. Native/installed, Provider, CI/main, full admission/parity, packaging and release remain OPEN. |
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

| OpenAI Apps SDK UI renderer icon candidates | `artifact-entering` | `openai/apps-sdk-ui` | [Fixed official source, retained MIT license and registration](openai-apps-sdk-ui-sync.md); the 139-object approval/map remains below | `approved` for these vector sources only | All 139 fixed source tuples and actual destinations match; preserve pin, complete MIT notice and explicit semantic/brand/data exceptions. Marker remains `not-proven`/null. New ledger tracked-HEAD binding is required before artifact admission; detached/no-tracking freshness and default-root availability remain separate. Current-web identity, native, CI/main and distribution acceptance remain OPEN. |


## 2026-10-10 — scoped source-record reconciliation

The two DSH queue dispositions above approve bounded source admission at
candidate `210039729892bf80fb8e0e9a7c909b70266ec6bc`, tree
`2a59d76106a0e042a690f8a94281a7f7f7e64893`. Independent read-only review read
134 distinct source path/blob pairs (1,222,506 bytes) from the fixed DSH Git
checkout, verified the complete MIT license/notice and checked current
integration. The [presentation ledger](deepseek-harness-presentation-sync.md#2026-10-10--scoped-source-admission-at-2100397)
records the exact scope, historical checkpoint ancestry and retired shared CSS
mapping. Earlier candidate descriptions remain historical; only their source
admission and integration-open statements are superseded here. Their test,
compiler, build, native and behavior claims keep their original inputs.

The manifest's current `analytixCommit` is this scoped candidate. Its former
`196c6dd3f6d74c660224b88117963ba52508389f` object is available but is not an
ancestor of this candidate; the unchanged pre-rewrite commit/map retain the
previous review's history context. This rebinding does not merge private
history, refresh every other upstream review or establish release acceptance.

The initial four-path freeze left Apps SDK UI source registration OPEN. A
subsequently authorized normal retrieval from the official repository now
closes actual source readback at pin
`0f00143c7a639906f1621fe58e1b6be7b5bea46d`, tree
`5c2aabc9e419aa1c55d6ebb8d219e19287bbb494`: all 139 source path/blob/SHA-256
tuples (168,494 bytes), the existing approval table, actual destination bytes
and candidate 210 tracked objects match. The root MIT license is exactly
1054 bytes, blob `b8ac8dc0f960bbe0b0cc90cd06d58b672151773f`, SHA-256
`f26a46776c890b0d7b45d042cb8a78fdca9c4742a5c7fd97f0f53767411b1cc6`;
it matches the local LICENSE and complete NOTICE. Two read-only reviewers
found no nested license/notice exception, gitlink, linked asset or runtime
import. The historical 139-vector approval remains bounded to those objects.

The [independent Apps SDK ledger](openai-apps-sdk-ui-sync.md) binds the exact
source id/pin and `not-proven`/null marker. The registry records the actual
isolated checkout basename `apps-sdk-ui`, official origin and empty detached
branch; it does not manufacture branch tracking. The closed license catalogue
adds only the verified content hash. Unknown/altered license bytes and
source/pin/marker mismatch remain rejected. New ledger tracked-HEAD binding
is still required; a correct uncommitted ledger does not establish artifact
admission. Default-root availability and latest/tracking freshness are
separate; the Apps-only source root leaves the overall research inventory
UNVERIFIED. The exact source/marker and negative regressions are in the
existing upstream-source contract, without a generic source or MIT exemption.

The current NOTICE digest is
`3199c29b46b438513c7b578b618dae2e6f87df26ffabb40cf4ada850d3f97d94`.
Its only changes from fixed historic `bbae1caa` are the verified DSH scope and
three-path Browse artwork attribution; other sources and MIT terms are
byte-identical. The artifact audit's fixed digest is updated to those exact
reviewed bytes, retaining its fail-closed comparison. This record work is limited
to the existing direct upstream-source contract and JSON/link/diff checks; no
Go, compiler, build or native check is authorized. Full DSH/native, current
CI/main, packaged distribution and release remain OPEN; Kun remains blocked
on material-specific commercial authorization and the complete inherited-file
inventory.

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


### DeepSeek Harness typed-tool candidate

`artifact-entering` / `candidate`, reviewed official pin
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`; exact source/destination/blob and
adaptation obligations are recorded in the [typed-tool intake section](deepseek-harness-presentation-sync.md#typed-tool-model-and-leaf-candidate).
MIT terms and DeepSeek copyright remain in existing `THIRD_PARTY_NOTICES.md`.
No new asset, dependency, Cordis runtime, raw tool history, or default side effect
is admitted. Reviewer: typed-tool writer with read-only source reviewer. Five affected suites pass on the exact candidate; the additional shared chat
suite has two retained DOM-selector failures owned by parallel unit B. Compiler,
scoped lint, renderer build, B-owned locale combination and coordinator
integration remain pending; this row is not parity.


The subsequent [A+B configured-Mac validation](deepseek-harness-presentation-sync.md#typed-tool-and-turn-state-combined-source-validation)
uses the exact 07dd fresh archive and disjoint 39-path payload. Ten suites pass
351 cases; the missing-turnId test-only type repair passes the overlapping 117
mapper cases. Web/unchanged Node types and scoped affected lint pass with zero
lint errors/warnings; the renderer-only 640-file artifact passes with manifest
SHA-256 `35c29db857f6aa455838d384303091a5d292134af1d7aaab47d962b906fbb0e0`.
Exact failures, input revisions and unchanged-program proofs stay in private
receipts; stable public test/compiler/build methods are linked in that section. B-owned en/zh labels and shared
Bash selector are verified in the combination. This closes those local checks,
not the explicit missing producer fields, full parity or coordinator integration.
No dependency install, main/preload/Go/native/Provider/CI/release result is added.


### DeepSeek Harness settled Search candidate

`artifact-entering` / `candidate`, reviewed official repository
`deepseek-ai/deepseek-harness` at
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`. Reuse mode: `port-and-adapt`.
The four ordinary source blobs below have no separate file-level license notice
or asset dependency. SPDX `MIT`; copyright (c) 2026 DeepSeek. Root `LICENSE`
blob `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785` and the existing complete
DeepSeek MIT section in `THIRD_PARTY_NOTICES.md` cover this bounded intake.
The four destination headers retain the pin and copyright/NOTICE pointer.

| Fixed upstream path | Blob | Analytix destination | Frozen destination SHA-256 |
| --- | --- | --- | --- |
| `packages/client/ui-tool/src/client/tool/toolviews/search-row.tsx` | `a500d170eee7295b04767aee6d72233e6e693810` | `src/renderer/src/components/chat/presentation/chat/toolviews/search-row.tsx` | `2697109e7356446a7383f793f69184301f3d6ae6f05f946064762d6f83674be4` |
| `packages/client/ui-tool/src/client/tool/models/search-card-model.ts` | `cc4d25b6b51127002677562f04f4fde543671a9a` | `src/renderer/src/components/chat/presentation/chat/models/search-card-model.ts` | `3089bc9530518323f483a467ab6998c6790530cecaad68d731b3396173b506e2` |
| `packages/client/ui-primitives/src/SearchBlock.tsx` | `c2328d6a9d16ed29c1cc0a5b8e293cdf5aa6d559` | `src/renderer/src/components/chat/presentation/SearchBlock.tsx` | `7ac578cdfb483577416c5f88d4ed07ee391ea7f702ab1fcbc70354e28d5c915c` |
| `packages/client/ui-primitives/src/SearchBlock.module.css` | `9b95d72f3d98eafe2c3aa3c423db5d57bf45758d` | `src/renderer/src/components/chat/presentation/SearchBlock.module.css` | `a8556f29bdf46798b23b6c3f0e83a11b254f44bbf7433dc2a68545d1448c40c8` |

Modifications preserve DSH's exact settled root `grep`/`glob` specialization,
file grouping, real line numbers, file disclosure, eight-row Chat head/tail
preview and copy independent of UI folding. Analytix replaces raw public result
metadata with the existing authorized protected-local capture/CAS/display and
Host effect routes. Actual producer context rows and columns are retained;
unknown totals stay null. Producer truncation and display-capture truncation are
distinct. Snapshot identity/digest replaces DSH's raw full-result text locator;
it does not promise recovery of results omitted before capture. Paths are display
text without navigation targets. Nonspecialized authorized captures use private IN/OUT; unavailable or legacy
records retain their existing generic fallback. No Cordis/slot/session package or new dependency is copied.

The Go producer, closed DTO, transport propagation, public identity selector,
SearchRow export and fifteen en/zh `dshSearch.*` strings are Analytix-owned
adapters. Locale wording is authored for the local contract, including unknown
totals and redacted copying; upstream locale files are not admitted by this row.
Reviewer: Search dedicated owner and public wiring writer, with independent
read-only source/key review. Source is bound to baseline
`155951ff79d4134fd9099e67e6697da6e1f640e5`, tree
`2b204b95bf10bba6213c5052ce8d30641a024a75`, and the coordinator's 22-path
Search freeze (14 production paths and eight test paths), source fingerprint
`703a00f2e6afc3efd3a76c202605cbb82585019c670de26ffaabaebf334a09ad`.
The seven-path public wiring delta is separate and disjoint; it does not consume unfrozen Live fields.

Validation: pinned Git blobs, existing notice, current leaf keys and disjoint
owner paths checked by read-only inspection; source tests are prepared but
unexecuted. Combined producer/CAS/DTO/consumer tests, affected types/lint,
renderer build, native/GUI evidence and canonical integration remain OPEN.
No earlier Web or Question PASS is transferred to this candidate. See the
[settled Search source record](deepseek-harness-presentation-sync.md#settled-search-source-candidate)
for the bounded behavior and remaining seams. This row does not establish full
Search parity, Provider/CI or release acceptance.


### Historical Fetch Core item isolation follow-up

See the [complete DSH record](deepseek-harness-presentation-sync.md#historical-fetch-core-item-isolation-follow-up)
for the original candidate, evidence, results and remaining OPEN items.


### Search33 isolated validation checkpoint

See the [complete DSH record](deepseek-harness-presentation-sync.md#search33-isolated-validation-checkpoint)
for the exact source freeze, validation results and unresolved blockers.


### Shared DSH conversation chrome source checkpoint (unvalidated)

See the [complete DSH record](deepseek-harness-presentation-sync.md#shared-dsh-conversation-chrome-source-checkpoint-unvalidated)
for pinned sources, MIT/copyright and notice, Analytix adaptations,
admission limits and pending validation.

### Timed question desktop consumer candidate

Disposition: `candidate`; reuse mode: `port-and-adapt`; reviewer: delegated
DSH desktop consumer writer. Fixed source repository DeepSeek Harness, commit
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`. MIT, Copyright (c) 2026 DeepSeek;
license blob `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`. Full license and
attribution remain in `THIRD_PARTY_NOTICES.md`; destination source headers retain
this notice. No other artwork or upstream runtime/dependency is admitted.

| Source path (under `packages/client/`) | Blob | Destination under `src/renderer/src/components/chat/presentation/` |
| --- | --- | --- |
| `ui-primitives/src/icons/index.tsx` (selected Regular Question, Check, Close, Left, Right, Copy, Nowrap, Wrap; existing Edit/Up/Down) | `d22cbdd06d50289f15154996564b250b2af4163a` | `DshToolIcons.tsx`, `chat/QuestionComposer.tsx`, `chat/QuestionReplyView.tsx`, `chat/toolviews/ask-question-row.tsx` |
| `ui-primitives/src/Button.module.css` (36px medium primary/outline geometry, tokens and hover/disabled states) | `731c593f7adb7ced790f5cba2305c745816a66f2` | `chat/QuestionComposer.module.css` |
| `ui-primitives/src/markdown/render.tsx` (Question-seat URL glyph, image-only glyph suppression, normal image and failed-image fallback) | `cdb9afcbdad8f2e2d6f73834ee9ded97560bfc6d` | `markdown/render.tsx`: existing glyph/CSS and private native action span; no file-image delegate/lightbox |
| `ui-user-questions/src/client/QuestionComposer.tsx` | `33bbcbec0aa3b887e65c3636ba5e5215bfeb7cee` | `chat/QuestionComposer.tsx`, `chat/useUserInputTimedWait.ts`, `chat/useUserInputPrivateDraft.ts` |
| `ui-user-questions/src/client/draft-store.ts` (partial/page restoration behavior reference; upstream store not imported) | `4d04ec283382bcf48398e6c8f18415f53b1aa992` | `chat/QuestionComposer.tsx`, independent Host-owned `chat/useUserInputPrivateDraft.ts` |
| `ui-user-questions/src/client/QuestionComposer.module.css` | `5f2eb4701ba2222f2959110ed066469047626ad7` | `chat/QuestionComposer.module.css` |
| `ui-user-questions/src/client/QuestionReplyView.tsx` | `8cd59f5b4d831da7b1fe2d9a078c864bed7dbfb0` | `chat/QuestionReplyView.tsx` |
| `ui-user-questions/src/client/QuestionReplyView.module.css` | `651fe889dc26d392fe3179b73ad5fef915404124` | `chat/QuestionReplyView.module.css` |
| `ui-tool/src/client/tool/toolviews/ask-question-row.tsx` | `5bf38d3e25b9e66bfb7af72e94853a1327b5fa66` | `chat/toolviews/ask-question-row.tsx` |
| `ui-tool/src/client/tool/components/QuestionToolRow.tsx` | `2ce27821555e450170a53608ea8e9bb0988201bd` | `chat/toolviews/ask-question-row.tsx` |
| `ui-tool/src/client/tool/components/QuestionToolRow.module.css` | `20c82c56a7e4109932171343dfb3bd9ec855a599` | `chat/toolviews/ask-question-row.module.css` |
| `ui-tool/src/client/tool/components/AskQuestionCard.tsx` | `d704aea5013455090967c8b801bab625e6a67450` | `chat/toolviews/ask-question-row.tsx` |
| `ui-tool/src/client/tool/components/AskQuestionCard.module.css` | `ae969921c90f5baaabfcdfee87e759ba2629d18e` | `chat/toolviews/ask-question-row.module.css` |

The same licensed candidate admits the following PlanReview selection. Source
review verified the fixed blobs below, React/parse-only imports and absence of
file-level license exceptions; behavioral/native validation remains separate.
No runtime, browser draft storage, file editor, new dependency or brand asset
is imported. Adaptations keep raw plan text in freshly authorized native leaves
and retain only selector/opened metadata in the existing workspace-tab owner.

| Pinned source under `packages/client/` | Blob | Repository destination / selected behavior |
| --- | --- | --- |
| `ui-user-questions/src/client/contract/slots.ts` | `4d1e9fd0e983227c1e80a6e6dcb5291bea610c55` | `src/renderer/src/components/chat/presentation/chat/question-plan-review.ts`: narrow private intent / actual option identity |
| `ui-user-questions/src/client/PlanReviewPanel.tsx`, `PlanReviewPanel.module.css` | `b63a64c0e06bdbcb5aad8244d47b449079d50cfa`, `67b302e3b5efd5e9928616055a26cf2db47d1242` | Same-named desktop `chat/` panel/CSS: complete summary and existing Host decision route |
| `ui-primitives/src/markdown/plain-text.ts` | `6797fdec6e63b47e187972de435445663ef5bffa` | `src/shared/markdown/plain-text.ts`: same existing `parseGfm`, first visible line / semantic paragraph |
| `ui-primitives/src/Button.tsx`, `Button.module.css`; `StateDot.tsx`, `StateDot.module.css` | `31a6edd78f51e35b9b70129d857f403b2d568ba3`, `731c593f7adb7ced790f5cba2305c745816a66f2`; `ec480c69810919601e992b22a187178b3106833d`, `1a692c68924e317e2e044da9db7ac96d5ff49ef2` | Selected 36px decision button, warning dot and busy spinner in desktop PlanReview panel/CSS |
| `ui-plan/src/client/PlanCard.tsx`, `review-store.ts` | `e629b024c3a8e67991fa01d37a033c0d2d99bd1b`, `b35b5daa394639ef708a03be5be11e98345edf7b` | Existing `workspace-tabs-store.ts`: Session-memory auto-open once, manual reopen, selector-only tab |
| `ui-plan/src/client/PlanPreview.tsx`, `PlanPreview.module.css` | `fdee35e7b9423c448d71d9db618e1713251a5d32`, `6344ddeb79deca18d1f9a384c974d61d164c5387` | `src/renderer/src/components/workbench/QuestionPlanPreview.tsx` and CSS: complete private Markdown, unchanged dock/overlay owner |

The desktop source uses actual Go wait/continue/late-response contracts through
existing Main/preload/provider lifetime. Original local question/detail/answer
and draft bytes remain complete in their private owners; public events/model
projection retain their separate contracts. Machine association requires the
private top-level callId; Reply additionally requires actual accepted delivery
plus the real public user-item/client identity. No browser draft store, generic
model-history envelope or global timer engine is added.

The earlier v13 isolated composition used Core
69498d0649d6de73f0bc971df425d84ff8b827f5 plus private-draft commit
363c152d036d76c1cfaf5d960522801ab5f62325 and its recorded desktop patch.
Its scoped tests, original runtime/Web/Node types, lint and one renderer-only
build passed in their recorded scopes; failed receipts remain retained.
The independent module v3 repair freeze then passed 681 scoped fixture tests;
517 tests were skipped by explicit owner/name filtering. Original
`tsc --noEmit -p` runtime, Web and Node projects passed; affected-file lint had
0 errors and 3 warnings. The v3 input SHA256 is
`4db7d11dcba6a675266991154d629a90bee3961efa3000ec635f15326def3159`,
and its scoped receipt SHA256 is
`4270c2b613120236de9d6b906e44749b4476e7d0ed64532c3a64d2277f681dfd`.
It used a Core 363c152 archive, exact desktop blobs, existing verified
Node 22.22.1/ABI 127 dependencies and the byte-identical private DTO snapshot
`49a9fb8da5f3afe987c08e4620cbddb29e60aa77273b5596b95b7af4fad6ee2b`.
No dirty Go overlay, native window, build, installation or Provider ran.

The later v4 wait-consumer freeze passed 181 direct UI and 176 contract/config
fixtures (`vitest run`, one worker, file parallelism disabled). Its input SHA256
is `73c0901f1c7371feb30aa8c01eced37434ab1f0c1cfb09fd6fb8d1bfe03acf4d`;
receipt SHA256 is
`79d82e67091c86c1a71c177a982d7238b8064326efebb0d9f840e6c905827da7`.
**Those passes do not validate the newest PlanReview/sidebar source, the
subsequent continued-wait/private-only-timing regressions, or the current
replacement cleanup and AppShell route tests.** Their tests/types/lint await
the coordinated window. Failed earlier receipts remain retained.
Canonical integration, current CI, native acceptance and full parity are unproven.

Within the approved Question scope, the DSH replacement removes the old
`QuestionComposer.tsx` undefined-selector/public-raw question and public-answer
seed paths, the duplicate legacy/structured submit validator and label/value
generator, and the unused `chat-store-maintenance-actions.ts` /
`chat-store-types.ts` continue action. Normal production enters through the
actual private native selector; signed Host question/option IDs determine
answers. Private legacy submission display remains for real stored history,
and native/runtime wait continuation remains the real Host owner. Existing
paired/unpaired tool rows, secondary/retired seats, ordinary GUI plans and
ordinary Markdown still have actual consumers. Shared parser old paths remain
byte-identical reexports for their real imports. DSH button tokens and
hover/disabled states replace the old Question adapter; real remote anchors,
bottom Copy tooltip, empty-detail selector recognition and Request changes
without a late target use the fixed-source behavior. Actual native PlanReview
production still rejects empty plans; the helper fixture does not prove native
admission. Immediate Hide flushes the retained same-owner leaf and removes
the obsolete closing waiters without a global draft queue. The same metadata-only
global plan authority observer moves from Workbench to the persistent AppShell, so
ordinary Settings return reacquires a fresh native leaf without extra Retry;
true revocation during Settings still requires explicit fresh authorization.
Observer detachment only unsubscribes and cannot fabricate retirement during
StrictMode replay. Question anchors reuse the existing URL glyph; normal
remote images keep the fixed-source CSS and failed-image alt fallback, with
no Question file-image delegate or lightbox. The corresponding private-boundary and actual shell-route tests
are prepared, not executed for these replacement bytes.

Fixed-source inspection distinguishes the normal Question seat from the ChatView
Markdown delegate. DSH normal detail has full code with Wrap then Copy, without
Collapse/Expand/Download/table exports. HTTP(S) images are ordinary lazy/async/
no-referrer images without a click lightbox. Local images display alt and local
file links display text because this seat has no file delegate. Reply detail
remains plain text. Caller-scoped actions preserve ordinary Markdown defaults.
Main derives code/link/Reply bytes from fresh Host data and the same shared
grammar; Renderer effect IPC carries selector/view/node-position or actual
accepted-reply identity metadata. Modified and middle link activation use that
route; right activation is inert. Native inline URL eligibility matches the
renderer HTTP(S) parser and opens the normalized anchor destination. Authorized local selection is allowed;
authority loss still retires stale raw leaves and actions.

| Behavior | Current repair source | Remaining evidence/dependency |
| --- | --- | --- |
| Reply Q/A Copy and selection | Native source-bound Copy, exact DSH text/labels/SVG and ACK feedback; authorized local selection in all Question leaves. | v3 scoped fixture pass; newest combined source and native acceptance pending: Main/preload/Reply and actual clipboard/revocation. |
| Detail code, links and images | Full code with Copy/Wrap, no extra exports, outside disabled answer fieldset; native remote-link action; remote image rendering and local inert fallback. | v3 scoped fixture pass; newest combined source and native acceptance pending: caller/default Markdown and readonly actions. Existing Electron img-src excludes most remote hosts; actual remote-image parity remains open for coordinated route/policy and native evidence. |
| Full settled Review | Explicit review metadata mounts selected terminal card readonly; queued Review stays inline. | v3 scoped fixture pass; newest combined source and native acceptance pending: real row/store/Float path, pagination and close/reopen, nullable/unavailable owner. |
| Persistent Take-time | Frozen private DTO adds optional waiting/editing; the existing draft writer preserves it and the existing Host wait owner replays it only on fresh pending authority. Continued retains metadata without starting a wait; old drafts infer editing only from changed answers. | v4 scoped UI/contract pass; newer continued/private-only regressions and real save/read/restart await validation. Saving wait never extends a deadline. |
| Declared PlanReview | Frozen signed private intent selects exact summary/decision controls and actual approval option ID. The existing workspace shows the full plan in a fresh native leaf; automatic open is once per thread/call, ordinary GUI plan remains separate. Only metadata survives tab changes; true authority retirement covers inactive tabs and dock/overlay, with explicit fresh reopen. | SOURCE_PREPARED_UNTESTED: newest Plan consumer, dock/overlay/revoke, signed producer integration and native behavior. No prose/order inference or file-editor writes. |
| 16KiB nonadmission retry | Exact canonical POST/409/closed Host body maps to fixed desktop late-too-large; repeated facade/IPC projection preserves only its exact fixed envelope. Edited new UUID only after definite nonadmission, exact retries for unknown outcomes. | v3 scoped fixture pass; newest combined source and native acceptance pending: sanitizer/facade/consumer matrix. Actual queue/promotion/Provider carrier remains Core-owned. |
| Longer history | Arbitrary over-32 fallbacks removed, preserving existing bounded private scheduler/raw-seat/transient budgets. | v3 scoped fixture pass; newest combined source and native acceptance pending: 40th exact match, ambiguity, capacity/close/authority. |
| Foreground to continued | Real same-owner continued retires submit/cancel epoch and old ACK; pending-only resolving; writer retained during foreground wait/failure. | v3 scoped fixture pass; newest combined source and native acceptance pending: deferred continued/late-action/old ACK; loading-null must not retire current late attempts. |
| Semantics/legacy | Reply group/name/toggle/outcome, cancelled action removal, queued answered-count, original legacy value and trimmed structured presentation. | v3 scoped fixture pass; newest combined source and native acceptance pending: semantic/native keyboard evidence; real legacy late producer unverified. |
| Thread detach | Existing store subscription synchronously flushes the current native leaf on thread/route change. Only bounded selector/revision/unsent warning metadata survives unmount; fresh matching Host revision or current save ACK clears it. Live native revoke prevents same-batch cleanup from reinserting it; confirmed deletion clears only that thread. | v3 scoped fixture pass; newest combined source and native acceptance pending: old/new leaf, actual Float/store thread switching, unconfirmed/unsent warning, late ACK, native revoke and confirmed deletion. No browser raw persistence. |
| Immediate background/Hide | First edit prompt, later finite coalescing in one writer. DSH panel Hide flushes and dismisses immediately; its retained same-owner leaf keeps the latest child chain and actual save/error state. System background admits no new save, while previously visible-admitted saves may finish for the same document; true authority retires all. | SOURCE_PREPARED_UNTESTED: delayed first/latest child ACK, saved/capacity reveal, hung-save background and real revoke regressions; actual Float/native acceptance pending. No close-success gate, hidden native grant or browser raw queue. |

Local display/draft budgets remain distinct from late steering admission.
Source repairs are not acceptance exceptions. Native pixels/spacing, light-dark/
narrow layout, clipboard, keyboard/screen reader, focus/blur, thread switching,
reconnect and restart require the parent-assigned exact-candidate window.
Idle-agent late-answer admission and correlation into a new normal turn remain
a Core contract dependency; the UI must not fabricate a running turn or weaken
target ACK checks. Private wait/intent schemas are frozen for consumer source;
actual signed persistence/producer/model-history/recovery validation remains
Core-owned. The latest consumer and exact combined candidate remain unverified.


## Footer and normal performance/usage preference candidate (2026-10-09)

Reference/admission for the selected desktop port, pinned to DSH
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`. Reuse mode is
`port-and-adapt`; license is MIT, copyright (c) 2026 DeepSeek. The complete
notice remains in `THIRD_PARTY_NOTICES.md`, and adapted source headers point
to that notice and this fixed-pin ledger. The coordinating desktop writer is
the source reviewer; read-only helpers checked the fixed blobs and owner routes.

| Exact upstream source | Fixed Git blob | Actual local destination / selected scope |
| --- | --- | --- |
| `packages/client/ui-chat/src/client/chat/TurnUsagePanel.tsx` | `460d1092a2f64f0869ff81a649b6401bfd5a6572` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx: turn panel adapter |
| `packages/client/ui-chat/src/client/chat/TurnUsagePanel.module.css` | `dfdfca258b6aaa676c96cd3b528b49ffef14c72f` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.module.css: panel styles |
| `packages/client/ui-chat/src/client/chat/StatsPills.tsx` | `838effb768a02f6230c8bcbe6593a5bd0d1870c3` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx: SessionStatsPills and formatters |
| `packages/client/ui-chat/src/client/chat/StatsPills.module.css` | `44dab514307156a7a121604c18c1a7f04e83fa9c` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.module.css: pill styles |
| `packages/client/ui-chat/src/client/chat/MessageIconActions.tsx` | `fd2b674edd5323873eed1d7f97271b31038e79fb` | src/renderer/src/components/chat/presentation/chat/TurnTailView.tsx and src/renderer/src/components/chat/message-timeline-bubbles.tsx: selected actions / CopyFeedbackButton |
| `packages/client/ui-chat/src/client/chat/MessageIconActions.module.css` | `29c1c9e7137ae45886c1e535f24383c229162670` | src/renderer/src/components/chat/presentation/chat/turn-state/TurnState.module.css: selected actions |
| `packages/client/ui-chat/src/client/chat/TurnTailNodeView.tsx` | `a3d58a8cfe48d00b1cd922e315ef6d831ad572b2` | src/renderer/src/components/chat/presentation/chat/TurnTailView.tsx: selected tail and native owner adapter |
| `packages/client/ui-chat/src/client/chat/TurnTailNodeView.module.css` | `bfa386d31c983b35912cbd210ab7a36f5f37d3b0` | src/renderer/src/components/chat/presentation/chat/turn-state/TurnState.module.css: selected tail styles |
| `packages/client/ui-chat/src/client/chat/stat-dialog.ts` | `43ee6cd220bac3541153a56bcdd63124f31886b3` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx: useStatDialog |
| `packages/client/ui-chat/src/client/chat/stat-dialog.module.css` | `4c5de52c51153962cf3b627498f424f5526dec3b` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.module.css: dialog styles |
| `packages/client/ui-chat/src/client/chat/token-format.ts` | `20936ff2f14227b93d83dcc01e83ac7319c5b728` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx: token formatters |
| `packages/client/ui-chat/src/client/chat/message-chrome.ts` | `e078d28177066fbd4110fb12afbe3e81bcce7bd1` | src/renderer/src/components/chat/presentation/chat/TurnTailView.tsx: clock formatter |
| `packages/client/ui-chat/src/client/chat/use-calendar-day.ts` | `29e5311383fa5d6cba826b6b607d60cb1cc95d1b` | src/renderer/src/components/chat/presentation/chat/TurnTailView.tsx: calendar-day hook |
| `packages/client/ui-primitives/src/clipboard.ts` | `aa45f4c76d197532bac80a53a7aaaa518f2857ac` | src/renderer/src/components/chat/message-timeline-bubbles.tsx: CopyFeedbackButton fallback |
| `packages/client/ui-primitives/src/useAnchoredPosition.ts` | `f87e4d06975ced7bc10484d70741a6f8178814a6` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx: selected position adapter |
| `packages/client/ui-primitives/src/useDismissOnOutsidePointer.ts` | `2f16f43476d3821ac142edf86d53751d0942d72a` | src/renderer/src/components/chat/presentation/useDismissOnOutsidePointer.ts: existing source unchanged; new Settings consumer in src/renderer/src/components/settings-performance-usage.tsx; Footer selected local adapter in src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx |
| `packages/client/ui-primitives/src/overlay-top-margin.ts` | `02ed6792808a0ac4048f8cba7ccaa7a080f3288a` | src/renderer/src/components/chat/presentation/overlay-top-margin.ts: existing source unchanged; new Settings consumer in src/renderer/src/components/settings-performance-usage.tsx; Footer selected local adapter in src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx |
| `packages/client/ui-primitives/src/icons/index.tsx` | `d22cbdd06d50289f15154996564b250b2af4163a` | src/renderer/src/components/chat/presentation/DshToolIcons.tsx: Branch / Database / Gauge; existing ChevronDown / Check reused unchanged |
| `packages/client/ui-chat/src/client/locale.ts` | `4c4bfa7e12648a55a70eef435a490620938488c1` | src/renderer/src/locales/en/common.json, src/renderer/src/locales/zh/common.json, src/renderer/src/locales/en/settings.json and src/renderer/src/locales/zh/settings.json: selected labels/units |
| `packages/client/ui-chat/src/chat-settings.ts` | `c4c6ce3b954749f01e9b4c99cc4e40920b65a95c` | src/shared/app-settings-types.ts, src/shared/app-settings-normalize.ts, src/shared/app-settings-runtime.ts and src/main/ipc/app-ipc-schemas.ts: mode/default reference; native persistence adapter |
| `packages/client/ui-chat/src/client/performance-usage.ts` | `b0313649a7a5dd4595ccff6136dd4b47d0473563` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.tsx: process-local UI mode and normal accepted settings reconciliation |
| `packages/client/ui-chat/src/client/settings/PerformanceUsageRow.tsx` | `db05aaebb95ebb92e596acbd9123ad3e9f84a7a1` | src/renderer/src/components/settings-performance-usage.tsx: selected two-option row |
| `packages/client/ui-chat/src/client/settings/PreferenceRow.tsx` | `bfad7b5316d80f36c27df23e9a35060a2eafeb65` | src/renderer/src/components/settings-performance-usage.tsx: selected two-option row |
| `packages/client/ui-chat/src/client/settings/PreferenceRow.module.css` | `2042d54db3da566f5b397df5c1a3fd99ef951e6c` | src/renderer/src/components/settings-performance-usage.module.css: selected two-option row |
| `packages/client/ui-primitives/src/Menu.tsx` | `fd5b655d5c061c748377dd82a2b8002f5a0b30a3` | src/renderer/src/components/settings-performance-usage.tsx: actual two-option menu/material/focus/IME subset |
| `packages/client/ui-primitives/src/Menu.module.css` | `a6d6714783d8e46c17f19b6bb5dee26e6693ee3c` | src/renderer/src/components/settings-performance-usage.module.css: actual two-option menu/material/focus/IME subset |
| `packages/client/ui-primitives/src/MenuSurface.tsx` | `1bdefd0d6a8339e526ebbd37272455177edf95f7` | src/renderer/src/components/settings-performance-usage.tsx: actual two-option menu/material/focus/IME subset |
| `packages/client/ui-primitives/src/MenuSurface.module.css` | `57d930845524b4e88f42b83a3650bf92f7ac5bee` | src/renderer/src/components/settings-performance-usage.module.css: actual two-option menu/material/focus/IME subset |
| `packages/client/ui-primitives/src/focus.ts` | `bc7e5a372a89e2c1502b12b787bdf08884af0de3` | src/renderer/src/components/settings-performance-usage.tsx: actual two-option menu/material/focus/IME subset |
| `packages/client/ui-primitives/src/keyboard-composition.ts` | `6c21bbda9577fa6b821f1389ffd29f5c664a70c9` | src/renderer/src/components/settings-performance-usage.tsx: actual two-option menu/material/focus/IME subset |
| `packages/client/ui-theme/src/styles/base.css` | `23be07f678050d8f144199d0312697118b9f33bc` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.module.css and src/renderer/src/components/settings-performance-usage.module.css: selected local theme values |
| `packages/client/ui-theme/src/styles/design-platform.css` | `eb67869c92bf53162dbd443c8a0e4fe8baabb11b` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.module.css and src/renderer/src/components/settings-performance-usage.module.css: selected local theme values |
| `packages/client/ui-theme/src/styles/gradient-shadow-text.css` | `863776c7d3360a3363fdc588392fc525c6795666` | src/renderer/src/components/chat/presentation/chat/TurnUsagePanel.module.css and src/renderer/src/components/settings-performance-usage.module.css: selected local theme values |
| `packages/client/ui-settings/src/client/config-form.ts` | `cf77536a97779bab13bbbd59940cf54c88a57372` | src/renderer/src/components/SettingsView.tsx: selected performanceUsage failure-recovery port-and-adapt; native IPC/write identity adapter |

Destinations under `src/renderer/src/components/chat/presentation/chat/`
retain the prior historical destinations as separate evidence. New Settings
leaf destinations are under `src/renderer/src/components/`; locale destinations
are the existing `src/renderer/src/locales/{en,zh}/{common,settings}.json`.
The native settings save/readback, real thread owners, verified terminal-batch
receipts and real Host rewind are local adapters. No DSH Host/configuration
engine, modal registry, global statistics engine or new endpoint is imported.

Immediate UI preference uses a separate display-only notification. Only the
normal accepted settings save/readback publishes the full settings event; save
failure does not turn a UI choice into accepted persistence. Default is Detailed.
The selected failure-recovery behavior is prepared in the normal SettingsView
owner. Its detached existing settings:get IPC readback preserves the save queue;
only the current entry, write request, draft generation and exact failed-form
identity may recover performanceUsageMode and invalidate stale query cache.
Unrelated draft fields and the original save error remain. Unlike fixed DSH's
structured response.ok=false branch, native IPC rejection also enters recovery,
including Host commit followed by ACK failure. This adapter neither waits for a
full shared describe mirror nor imports namespace revisions or memory persistence.
Unavailable readback retains the pending UI choice/error; no successful full
settings event is fabricated. The real SettingsView fixture covers failure,
queue progress, late readback, external accepted state, unmount and cache recovery.
Body portals receive local fixed-pin material tokens because chat/Settings
ancestor tokens do not cross the portal boundary. Footer stat dialogs use
specific-menu with border-l1; Settings menus use MenuSurface material, dark
border-l3 and the macOS backing. Existing platform tagging and body isolation
are reused. The menu port is limited to the actual two options, keyboard/IME,
focus, positioning and dismissal behavior; no unused submenu engine is copied.

Candidate status: `BOUNDED_TS_VALIDATION_PASS_WITH_OPEN_TARGET_GAPS`,
`NOT_INTEGRATED`.
On 2026-10-09, the isolated Git363 + frozen Core52 composition (actual Core commit
`48ab34fbd7846df05f8e219623036851e6cc56e9`) + Desktop98 + Docs2 + Markdown4 +
Footer57 candidate passed the registered 23-file module's latest 1,361 cases.
That count selects the latest result per case after one complete run and affected
reruns; first failures remain evidence, and repeat executions are not added.
It includes 13 normal General settings cases and real bounded replay, concurrent
same-cursor commit, rewind and Inspector revoke regressions. Runtime/web/node
strict types, 48 affected TypeScript files' lint (zero errors or warnings), and
one renderer-only build passed with existing matching dependencies.
No dependency install, Main/preload/native build, GUI, live Provider or Git write
ran for this extension. These checks do not establish native layout, real
clipboard behavior, normal credential readiness or full DSH equivalence. Prior
Question and Markdown evidence remains bound to its own exact inputs.

P2 restores verified historical terminal receipts without manufacturing completion.
Normal whole-log composer accounting still requires a frozen public producer DTO.
Missing groups stay absent; LastResult, turn-map sums and old snapshots do not
substitute. Secondary Fork remains a consumer wiring gap: side/Inspector have real
thread and turn identities, but the current UI hides Fork and its store action
captures the main owner. Removing only the UI restriction would fork the wrong
thread; explicit source-owner wiring and actual Host proof are still needed.
A leading newline in public failure text also remains a target conflict: fixed DSH
keeps the empty first line, while the current local mapper selects the first
nonempty line. Its existing assertion describes as-built behavior; the other-owned
mapper has not been changed or declared equivalent.

Producer review limits: observation completeness alone does not prove all durable
started attempts settled; the turn footer's current physical-attempt fields need
that producer proof before full-turn billing acceptance. Composer may eventually
show reported partial totals independently of that stricter turn-footer contract.
LogicalCallHMAC includes OuterAttempt and cannot supply DSH steps; cacheMiss is
uncached plus created, not uncached alone. Tool timing requires real durable
ready item identity and matching thread/turn/callId finished events; old presence
and route identity remain unknown. This desktop candidate adds no producer DTO
or guessed defaults for these gaps.


## Exact Footer Fork event boundary candidate (2026-10-09)

Status: `artifact-entering`, `partial`; parent-assigned source candidate with
fresh bounded evidence below, on `codex/dsh-fork-event-boundary`, based on
`332731d18054bd777cb09a0c81275a08d53a3c85`. Reuse mode: `port-and-adapt`
(selected semantics, with Go implementation using existing Analytix owners).
Reviewer: coordinating Fork source writer; read-only helpers checked the fixed
DSH producer, prefix and repair paths. Source pin:
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`, tree
`a9d30743edbe97adac9c8cb0811b648a0d9f6660`.

| Exact upstream source | Fixed Git blob | Local destination / selected adaptation |
| --- | --- | --- |
| `packages/client/ui-chat/src/client/chat/TurnTailNodeView.tsx` | `a3d58a8cfe48d00b1cd922e315ef6d831ad572b2` | `src/renderer/src/store/chat-store-maintenance-actions.ts`: forward captured Footer endSeq |
| `packages/client/ui-chat/src/client/apply.ts` | `e83c851c3bf6cdd108bd5d856fcac4426ddc61d3` | `src/renderer/src/agent/analytix-runtime.ts`, `types.ts`, `packages/runtime/src/contracts/threads.ts`: optional exact boundary through the current provider/HTTP contract |
| `packages/api/session-controller/src/commands.ts` | `7e900b92aee6e6862d433ecfe03bc83399f75c0c` | `packages/runtime-go/internal/app/thread/service.go`, `internal/adapters/inbound/httpapi/thread.go`: selected explicit versus omitted boundary routing |
| `packages/core/session/src/fork.ts` | `8d5b2219768ce0a660b461166d50dcb80f3bcff2` | `packages/runtime-go/internal/app/thread/fork_event_prefix.go`, `internal/server/durable_threads.go`: inclusive prefix reconstruction and inert inherited seed in the existing event log |
| `packages/core/session/src/index.ts` | `c3b783bf38adb69a692de677072929177221b1c4` | same Go owners: actual continuous event sequence and current source authority validation; historical turns remain legal |
| `packages/core/session/src/repair.ts` | `bb32f01ca06af2c496fa4c68fed287297e1b7fce` | existing `internal/app/thread/fork_projection.go` remains the deauthority and closed-tail owner; no new auto continuation |

MIT root license blob: `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`;
copyright (c) 2026 DeepSeek. The full MIT notice is retained in
`THIRD_PARTY_NOTICES.md`. The selected sources are regular source blobs under
that root; no gitlink, vendored asset, private reasoning, DSH queue/store/runtime
or Provider authority is imported. Adaptations bind the current public projector,
Host identities, normal CAS/outbox and existing Go mutation mutex. CASE inherited
history remains on its existing signed private chain; this port does not enlarge
CASE disclosure or model egress.

The public DTO adds a nonnullable optional positive safe integer `endSeq`, paired
with `turnId`. Whole-thread and Side omissions retain their current meaning.
Turn-only compatibility resolves an actual terminal event. Missing or invalid
boundaries fail before identity allocation; old history is unavailable only when
its required bytes cannot be proved from existing records. The retained TS
server-test-support oracle explicitly rejects a turn boundary rather than
silently cloning current items. Source and native acceptance remain separate;
no model, full-access, Provider or native validation is claimed here.

The ordinary creation producer writes one atomic bundle in the existing log:
`thread_created` binds child, parent and captured source E; bounded private
`thread_fork_seed` parts close with `thread_fork_seed_completed`. Inherited E
is the complete bundle's last sequence. Each frame retains the existing byte
limit; decoded turns retain the existing closed public history validation.
Order, count, identities, byte length and SHA bind the bundle. Strict JSONL
numbers are compared as validated exact integers, including source E.
A marked primary is staged outside the thread inventory before rename; normal
sidecars, index/list visibility and publication wait for complete readback.
Missing or partial creation remains unusable after restart. A committed append
error requires the same complete bundle readback and never causes reappend.

Existing producer propagation is limited to admitted steering metadata, exact
background-completion history bytes and the committed compaction turn. Exact
late-reply replay compares the immutable admission metadata; verified promotion
and terminal state do not rewrite that event or append it again. Private
seed records remain outside public SSE; CASE retains its signed private chain
and workspace. No new history store, queue, execution grant or Provider path is
introduced.

Fresh evidence on macOS 26.5.2 arm64, Node 22.22.1, exact base above, and the
37-file uncommitted candidate: the synthetic production HTTP test
`TestForkEventBoundaryExcludesDurableLateCallbackBeforeDelivery` fails on the
base with “same captured E inherited the durably late callback” and passes on
the candidate. All eight `TestForkEventBoundary*` production seam tests pass
with normal command exit0, including real GENERAL CAS/outbox/job admission,
restart, creation cuts/readback and CASE inherited authority. The affected
Fork owner rerun passes 30 cases with exit0. The earlier six-owner matrix logs
234 PASS and no failures; its Go CLI was interrupted with exit143 during
post-test directory I/O, so it is package-level evidence rather than a normal
command-completion claim.

Existing production continuation compatibility also ran through the real
timer, late queue, signed promotion, terminal and restart duplicate response:
`TestTimedQuestionProductionTimeoutLateQueuePromotionAndRestart` passes with
normal exit0 after immutable admission metadata replay was corrected. The
earlier combined run had one pre-late-reply pending-result assertion mismatch.
The unchanged single-test rerun passed with fresh random Host IDs, so it does not
clear the failed input; that input's cause remains unproven and the failure is
retained as a compatibility evidence gap.
No privacy or model-egress owner was changed to make it pass. Other bounded
CASE, reservations, archive, protected-result and Host-ID compatibility tests
passed; a 150-second combined-command timeout is retained separately.

Four direct TS contract/provider/maintenance files pass 279 tests; the full
ChatView jsdom file passes 70 tests with `--pool=threads`. Default forks worker
startup failed twice before test-body execution and remains `blocked` on this
host route. Runtime and desktop typechecks and all eleven affected TS-file
lint checks pass. These are synthetic source tests, not native, installed-app,
live Provider or actual model/full-access acceptance. Integration awaits the
parent's Git window; this candidate has not been staged, committed or pushed.


P2 follow-up within the same 37-file candidate preserves exact raw JSON numeric
precision for `endSeq` before float conversion; fractional decimals that round
to an integer are rejected. The established strict numeric text bound and a
local exact rational check retain positive-safe-integer semantics without
changing other request fields. Genuine Question history is recognized only
through the trusted signed steering reader or a verified complete creation
seed. Its inert codec slot keeps the complete closed Question body across
Fork, restart and refork; plain JSON-shaped text cannot acquire that slot.
Ordinary and Question leaf budgets remain unchanged. A verification copy skips
legacy strings only and retains every turn and other entry for the existing
last-running-turn, signature and identity-conflict constraints.

The new bounded production RED (`p2-red-v12.jsonl`, exit1) reproduces both
rounded fractional HTTP acceptance and failure to Fork the legal three-answer
400,000-character-per-answer Question. The repaired targeted run
(`p2-green-v13.jsonl`, exit0) passes the raw HTTP cases, real Fork/restart/refork,
closed Question owner negatives and four admission replay compatibility cases:
legacy event without metadata, cancelled precise replay, conflicting content
and duplicate event rejection. `gates_routes.go` is unchanged from the prior
frozen candidate. A read-only review found the legacy-string combination;
its real signed-record collector RED is retained (`p2-legacy-red-v14.jsonl`,
exit1). Its identity-conflict negative fixture was strengthened before GREEN
to a real signed ordinary entry, rather than an invalid Question-shaped map.
The final two-target server run (`p2-server-green-v16.jsonl`, exit0) passes the
complete large Question Fork/restart/refork and legacy combination, including
invalid entry, pending not last and conflicting identity rejection. Exact
command arguments, input hashes, logs and exits are retained in the task-owned
receipt; no timeout was extended and no pending/Provider owner was changed.

The complete two-direct-owner run (`p2-owners-green-v15.jsonl`, exit1) has a
passing Fork owner package but one HTTP display test failure at
`user_input_local_display_test.go:99`. Its display DTO, HTTP handler and test
are byte-identical to base332 and outside this 37-file scope. This failure is
retained for parent coordination, not repaired or reported as an overall PASS.
The same shell sourced the cache helper; all new Go tests use the local offline
Go1.26.4 toolchain and 90-second limits. Earlier TS, timeout/latequeue and native
limitations above remain separate; these P2 source results do not establish
installed/live Provider or actual model/full-access acceptance.

### Goal/Todo, Skill and recorded Job presentation candidate

Disposition: `candidate`; reuse mode: `port-and-adapt`; reviewer: delegated
DSH specialized ToolView renderer writer. Source pin is
`5badb15009ae1756c3afe0ae0cef1faafc290ccc` (MIT, Copyright (c) 2026 DeepSeek).
The full existing `THIRD_PARTY_NOTICES.md` notice applies. Source files were
read at the exact pin; no file-level license exceptions, upstream execution
framework, dependency or brand asset is imported.

| Source under `packages/client/` | Git blob | Destination under `src/renderer/src/components/chat/presentation/` |
| --- | --- | --- |
| `ui-tool/src/client/tool/toolviews/details-row.tsx` | `df96a5ffd0b02ed4b31ab14ea8ab025ba9b3fc9a` | `chat/toolviews/recorded-tool-row.tsx` |
| `ui-tool/src/client/tool/models/details-card-model.ts` | `c7e8e4fdcc7eb64b8b55b4dbfc4a6afdaaa185f9` | `chat/models/details-card-model.ts`, `recorded-tool-card-model.ts` |
| `ui-tool/src/client/tool/toolviews/todo-row.tsx` | `9aafae3fbccde25db4608f53f4cb8b21771c29c2` | `chat/toolviews/todo-row.tsx` |
| `ui-tool/src/client/tool/toolviews/plan-summary.ts` | `df609e7b3fd37b69b7504a162281d49b696ef36a` | `chat/models/todo-card-model.ts` |
| `ui-tool/src/client/tool/models/todo-diff-model.ts` | `f8451b12d3d602aad994f364c37cc4c55e93ee35` | `chat/models/todo-card-model.ts` |
| `ui-tool/src/client/tool/models/todo-history.ts` (baseline semantics only) | `591bc05ca5ac416415c9d4a347675f1242bc9c56` | `chat/models/todo-card-model.ts`; native authorized predecessor owner supplies history |
| `ui-skill/src/client/SkillRow.tsx` | `cee91367cff91302bc6662534fca483f9b522769` | `chat/toolviews/skill-row.tsx` |
| `ui-skill/src/client/SkillRow.module.css` | `8c74cf67636d3be69b0ddc60d577d8a9326f3839` | `chat/toolviews/skill-row.module.css` |
| `ui-skill/src/client/locales.ts` (row labels only) | `1028a827b49f9a32c1c8d91317fc3e370a0d49e9` | existing English/Chinese `common.json` |
| `ui-tool/src/client/tool/models/control-details-model.ts` | `47d6edceb839505f36a6fbdbb1d6e7431276b7dc` | `chat/models/recorded-tool-card-model.ts` (recorded list/status/receipt layout only) |
| `ui-tool/src/client/tool/models/detail-model-shared.ts` (finite badge presentation only) | `46c6b6b886ef42ead647553a997eb2220a827000` | `chat/models/recorded-tool-card-model.ts` |
| `ui-tool/src/client/tool/components/ToolDetails.tsx` | `618d998ce383a126d662e3500debdd190fdc230e` | `chat/toolviews/ToolDetails.tsx` |
| `ui-tool/src/client/tool/components/ToolDetails.module.css` | `de9133c4677403afb7894248517bcde4535ec566` | `chat/toolviews/ToolDetails.module.css` |
| `ui-primitives/src/icons/index.tsx` (selected regular Goal, Checklist, Play and Skill artwork) | `d22cbdd06d50289f15154996564b250b2af4163a` | `DshToolIcons.tsx` |

These leaves consume presentation inputs supplied by an independently validated
native contract; the pure view models are not wire validators or new authority.
Goal rounds/activation, teammate wait timeout/noProgress and differentiated
cancellation receipts are not synthesized. Todo identity uses native stable IDs,
including duplicate content, content edits and failed/canceled statuses. Job
output retains its real withheld meaning. No raw result JSON parser, current
catalog/job lookup, default clipboard/save/file effect or new execution control
is imported. Native disclosure/shimmer, protected container and existing
child/diagnostics action owners remain authoritative.

This is an uncommitted renderer candidate based on `9e80fa321…`, with layouts,
selected artwork/styles, pure presentation models and exact mapper/dispatcher
hookup to Core DTO freeze v1 (`items.ts` blob `efce7b305a9e07d92f06ec52e934e6238f1e3e67`,
`tool-result-local-display.ts` blob `da6f5ea89c7ef384bd9daa94f3d00c462a9710b2`).
The independent contract files are not copied into this renderer worktree.
DSH Inspect opens the selected call trajectory. Analytix adapts that user purpose
to the existing same-window `ToolResultInspector.tsx` and its single current
selection. Goal/Todo/Skill retain the current protected snapshot/lease; recorded
Job consumes only its admitted public record. Actual call/result identity and,
where published, snapshot identity accompany Summary and typed Result. These
domains have no raw Payload, Schema, Timing, Copy/Save or new IPC. The entry is
a read-only call-detail adaptation, not original trajectory parity. Optional
`onInspect` was an observer at the v1 source freeze; production completion at
that freeze did not depend on it. The A increment below supersedes that wiring.
Prepared synthetic model/leaf regressions do not prove producer or native/UI
acceptance. No test, type/lint check, build, package or live Provider result is
claimed for this candidate yet; canonical integration remains open.

### Conversation records Inspect handoff increment

Disposition: `candidate`; reuse mode: `port-and-adapt`; reviewer: delegated
renderer owner with independent read-only source review. The same exact DSH
pin, MIT copyright and full `THIRD_PARTY_NOTICES.md` obligations above apply.
This increment preserves the separately frozen nine-key renderer v1 source
anchor; it does not replace that evidence with a new parity claim.

| Exact source under `packages/client/ui-trajectory/src/client/` | Git blob | Destination |
| --- | --- | --- |
| `TrajectoryTable.tsx` (exact call lookup, actual ancestor expansion, Overview and pending scroll purpose) | `9eeeb5c2abb0e62994ae0031be32f21a4d806c06` | `src/renderer/src/components/chat/MessageTimeline.tsx`, `presentation/chat/ConversationRecords.tsx` |
| `TrajectoryTable.module.css` (30px record, finite hover/selected state and row boundary) | `12b03b1c51ff6b7791a03c7abe7d1a44c2e647ad` | `src/renderer/src/components/chat/presentation/chat/ConversationRecords.module.css` |
| `trajectory-record.ts` (stable physical record identity purpose) | `bd37189fa499433ddd1291166d65b2dfa5464ad4` | `src/renderer/src/components/chat/presentation/chat/models/conversation-record-model.ts` |

The native view retains actual admitted thread/turn/call and separate physical
call/result item witnesses through hydration and live renderer projection. A
cross-turn or cross-thread merge cannot manufacture a valid pair. Records group
by native turn identity even when a late call appears after another User; the
original physical chat grouping does not establish call ancestry. Missing or
ambiguous targets stay pending; only a uniquely mounted actual row is applied.
The existing turn window, measured cache and scroll hook remain the sole
virtualization owner. No upstream virtualizer, session store or backend is
imported. New requests and thread/scope/hide/revoke transitions retire prior
selection. Production Inspect now hands off to this conversation-local mode;
standalone rows retain the existing B Inspector behavior without a callback.

The selected private details remain mounted inside `ProtectedToolResult` with
its original Host lease and copy/cut/drag containment. The stable details seat
is outside virtual rows but is not a portal or a public snapshot store. Public
Job receipts reuse the same Inspector; a call with unavailable details shows
only actual public identity Overview. No raw payload, invocation-time schema,
PTC ancestry, nearest-Assistant association, catalog substitute or UI clock is
invented. This is the minimum admitted renderer navigation loop, not full DSH
trajectory dataset or action parity. Synthetic regressions are prepared;
tests/type checks/build/UI acceptance and canonical integration remain pending
the coordinating owner's combined validation and Git windows.

### Counted Goal lifecycle and live activation candidate

Disposition: `candidate`; reuse mode: `port-and-adapt`; reviewer: delegated
DSH Core writer. Fixed source `5badb15009ae1756c3afe0ae0cef1faafc290ccc`
is MIT, Copyright (c) 2026 DeepSeek; the existing full notice in
`THIRD_PARTY_NOTICES.md` applies. No file exception, dependency, upstream agent,
store, execution engine or authority framework is imported.

| Exact source | Git blob | Analytix destination and adaptation |
| --- | --- | --- |
| `packages/goal/goal/src/types.ts` | `eb7cd602ed7e39c75c133351b413e56d0c140ed0` | `domain/goal/round.go`, public `contracts/goal.ts`: closed refs and real user-item round source |
| `packages/goal/goal/src/runtime.ts` | `2f5c40735aad3c6504ab7b6ace38a5ad8575535b` | `app/goal/lifecycle.go`: fresh identity, CAS lifecycle, cumulative budget; no legacy adoption |
| `packages/goal/goal/src/domain.ts` | `63f1573dc7dab6e8cdffa73b0037a8daea6cd5b1` | existing Go Goal store mutation and lifecycle validation |
| `packages/goal/goal/src/fold.ts` | `6396c7f75b50348f97c9aefd1ce1cad3696b6f8f` | `app/turnstart/goal_admission.go`, existing primary durable turn writer: count actual admitted user item atomically |
| `packages/goal/goal-round-driver/src/index.ts` | `c4ca27e5deb7a9099c4af7c14641315a3e227e91` | `app/goal/round_driver.go`: process-only activation feeding existing foreground reservation and SendTurn; cold restore disarmed |
| `packages/goal/goal-round-driver/src/prompt.ts` | `d98f0bb83a90710da0a8c45fbb80facc76f5652f` | same Go driver: bounded attributed continuation, retaining Analytix evidence/todo/self-check policy |
| `packages/client/ui-goal/src/client/activation-source.ts` | `f1e74c866873623065996baf1c80bac4650d3c62` | renderer Goal epoch/ref fences and metadata-only live state; immutable snapshot history |

All Go destinations are under `packages/runtime-go/internal/`. Lifecycle edits
preserve phase and rounds. Legacy absent facts remain unknown and do not arm;
explicit clear/create establishes a new counted identity. Model completion
retains the existing evidence ledger, todo and strict self-check gates. Blocking
reports for counted Goals require actual recorded Goal user items. Activation
has no durable seq, history, provider grant or automatic cold-start effect.
The existing public Goal-prose SSE refusal remains closed; Goal history is
read only through the independently validated thread snapshot. Native gate,
case authority and protected-local disclosure remain their original owners.

Candidate base: `3705debe5a2b9cc59573cab686ac786cc0b6417a`, branch
`codex/dsh-goal-lifecycle`. Direct Goal owner regressions and the isolated
synthetic two-round HTTP/SSE chain passed, including foreground revocation,
interruption after edit, derived-thread history and live-attach metadata.
The ordinary receipt readback publication regression passed separately.
Recorded test inputs retain their own bounds. The affected TypeScript
dependency tests, runtime and desktop type checks, and scoped lint passed.
Native acceptance and canonical integration remain open.

### DeepSeek Harness generated Office deliveries and selected file artwork candidate

Disposition: `candidate`; reuse mode: `port-and-adapt`. Task owner: delegated
Office/Deliverables renderer writer; coordinating read-only review found no
behavior defect in the frozen source. The fixed source is
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`. The ten Office and two submitted-Plan source/blob/subset/
destination tuples are recorded in the
[Office source map](deepseek-harness-presentation-sync.md#generated-office-deliveries-and-shared-file-artwork-source-candidate-2026-10-11).
This scope includes the selected file artwork now shared with submitted Plan
cards through `src/renderer/src/components/chat/presentation/DeliverableIcons.tsx`;
it does not admit other primitives from the imported source files.

The root `LICENSE` is blob
`c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`, SHA-256
`ebb4f09972aee8608be255debaf78451a68e95c290f55c240dec2ecfa16ea6be`:
MIT, Copyright (c) 2026 DeepSeek. At the same pin, the three package manifests
also declare MIT: `ui-deliverables/package.json` blob
`174fcf3f72dcfa26309e346aec2d34d0dd5963ad`,
`ui-primitives/package.json` blob `8d02970440e7a127270f834a2d3c3cbd740f9554`,
and `ui-theme/package.json` blob `2e43211b92b1f4a1adb6a99febf24996b13effac`.
The selected Plan subtree also declares MIT in `ui-plan/package.json`, blob
`efaa5d5869892e416da088f9eefa83cf6f27f42e`; its two selected sources are
ordinary `100644` blobs with no nearer license exception.
The selected source objects are ordinary `100644` blobs, not gitlinks. No nearer
LICENSE/COPYING/NOTICE or file-level license exception was found in the selected
source directories and ancestors at this pin. The existing full DeepSeek MIT
notice is retained once in `THIRD_PARTY_NOTICES.md`; selected source headers
retain the fixed pin, copyright and notice pointer.

The package `ui-primitives` declares devDependency `simple-icons` `16.31.0`.
The selected file contour/fold, spreadsheet grid and W/P/MD type marks are
literal inline SVG in `FileTypeIcon.tsx`; the DeliverDoc glyph is five literal
inline paths in `icons/index.tsx`. No separate artwork license or third-party
attribution was found for those selected paths. `CodeFileIcon.tsx`,
`code-file-icon-artwork.ts`, `CODE_FILE_ARTWORK`, code-file classification,
`simple-icons` artwork/imports and every unselected glyph or brand asset are
excluded. No conclusion about their separate licenses is made. This review
records the selected source and notices; it does not establish independent
original authorship, trademark rights or rights to every asset in the repository.

The candidate ports `PresentedFileCard`/`Deliverables` and selected `PresentRow`
presentation into existing Analytix owners. The actual
`generate_office_document` -> CreatedReceipt -> `artifact_status` ->
`generatedArtifact` -> turn delivery projection and protected existing Office
preview remain the only data/action authorities. The generation process branch
matches actual producer plus `toolKind === 'file_change'`; Inspect retains the
original call identities and lazy protected-local result lease. No upstream
runtime, session store, declaration-addressing service, IPC, permission or new
artifact framework is imported. The superseded Office list/card presentation
is replaced at `GeneratedFilesPanel`; media delivery remains its existing owner.

Submitted Plan history adapts only the ordered `PlanCards` card/exact-call
Open subset of `ui-plan/src/client/PlanCard.tsx` and card CSS from
`PlanPreview.module.css`. `SubmittedPlanCards` reads the original signed
Question selector through existing private Host leases; public tabs retain
only selector and verified callId. Raw title/Markdown stays in authorized
mounted leaves. No Build control, upstream auto-open/store, runtime engine or
new authority is imported. The separate live pending approval and independent
file-plan owners remain their existing routes. Four pinned neutral fallbacks,
actual `data-theme` selectors and existing motion preferences are local CSS
adaptations; shared Markdown artwork is covered by the Office map above.

Local authored integration is limited to precise ToolCallTree dispatch,
receipt grouping, en/zh wording and two focused regression files. The existing
strict receipt schema, settled-success selection, ID deduplication, thread/
workspace preview scope and existing native Office opener are preserved.
Theme selectors use actual Analytix `data-theme` and motion preferences;
selected source palette values stay local fallback values rather than changing
the shared theme. Actual opener false/rejection/error keeps failure visible;
only an actual successful opener starts the adapted success hold/fade.

The en/zh `reviewPlanCardHint` and `reviewPlanOpen` keys are retired only after
read-only combination projection with the frozen submitted-Plan
`MessageTimeline.tsx` and `message-timeline-cards.tsx` proves zero renderer
consumers. The independent `planBuild` key remains. This locale cleanup depends
on that combined Plan replacement; the isolated older base is not claimed to
have already retired its old Plan component.

Baseline remains `dab5b9b331ee49382793ae60c160e79d1fa21524`, tree
`f6ac5f0ffe79f6a09fcf17f2300c694ea6263c49`. Office v3 is commit
`59e49e3a26794195e5a35d86c864f3130d0fdec2`, fingerprint
`926a4acd30bca833d4cf53357ff5bfbc6ab42567700808b4ca1117252196afd3`,
patch SHA-256 `0402372a47e9f1c37a2e325db6ca937365aff7121607042f2ffdcce03c45e7a7`.
Only its two regression files changed from v2; the nine other source hashes
remain unchanged. Plan7 is commit `f827ddb2cbf516831f1ccf9df83d6b4f12a7f262`,
fingerprint `de2ba942ee5a3d6bae5b9bb537d9a73151a2b794311d01c9b208fe5ca8061a4e`.
Shell5 is commit `3d79f5c5a804ceda39ebe6763f98ab1f64537a9f`; its existing
terminal mappings cover the command adapter and command whitespace correction.
No additional upstream source or copyright block is introduced for Shell.

The coordinating branch locally integrated these 23 byte-matched source files
at `e50fb4c7b42fcb837004a5312547eddb64f17ad9`, tree
`83872d9e6ed4930f930e2f6aaa833ae0910c20e9`. The combined fingerprint is
`18518379da583eb64029c414442fcbd89afd1001724f0bf0ea2fcb783c1443ba`;
all other inherited source blobs/modes remain unchanged. The scoped fixture,
type/lint and latest CSS-only renderer evidence is bounded in the
[combined source map](deepseek-harness-presentation-sync.md#generated-office-deliveries-and-shared-file-artwork-source-candidate-2026-10-11).
The original queued hunk receipt, Office v2 four-test failure and skipped build
remain preserved as history. No native, Provider, CI, main, package, release or
full DSH acceptance is inferred from these local commits.

Arbitrary existing-file presentation producers, actual filename/path/description
fields absent from the public Office receipt, DSH seq/index addressing, native
reveal/application actions, specialized preparing-tool ownership beyond the
existing shared owner and full DSH/native parity remain OPEN. No inferred
filename/path, fabricated description/index or simulated action closes them.


## DeepSeek Harness Present declaration and file actions candidate

Disposition: `candidate`; reuse mode: `port-and-adapt`. The source is DeepSeek
Harness at `5badb15009ae1756c3afe0ae0cef1faafc290ccc`, MIT, Copyright (c) 2026
DeepSeek. The complete notice remains in [THIRD_PARTY_NOTICES.md](../../../THIRD_PARTY_NOTICES.md).
The root LICENSE blob is `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`;
the selected `ui-open-in-app/package.json` blob
`e1b15cdf7247fc31960b486860708fad806e9080` declares MIT. All newly listed
objects and their ordinary-file modes were checked at the fixed pin. Existing
selected file/type palette artwork and its scoped license bounds retain the
[Office admission record](#deepseek-harness-generated-office-deliveries-and-selected-file-artwork-candidate).
CodeFileIcon, simple-icons, unselected artwork, fonts and assets remain excluded;
no additional authorship or trademark-rights claim is made.

The [Present checkpoint](deepseek-harness-presentation-sync.md#present-declaration-file-actions-and-local-integration-2026-10-11)
records the exact source/blob/destination mapping, source commits, evidence
chain and actual local merge/build checkpoint. DSH source contributes selected card/group/row/menu
presentation, timing/ordering semantics, labels and FolderOpen artwork. Go
protected Present receipts/history, private DTO validation, sealed file identity,
Host lifecycle and native file authority are Analytix-owned compatibility work;
no upstream runtime/store/controller, public history-order API, execution
permission or application-path authority is copied.

The TS/Host/UI44 source was normally committed as
`47989644c9d55cecd045c1aa38d1f44f3b450218` on `codex/dsh-native-event-hookup`
from `68ed3d00549a50bfaa87b1b42e896a43306a6fa8`, tree
`3b4c3a366855c83e7d19a3237dec4111fbd9b80b`. All 29 source and 15 direct-test
blobs/modes match the reviewed frozen composition. The coordinating author
reports independent review complete, with the final pending-Tab P2 closed.
Local fixture evidence covers 966 distinct cases across 20 unique suites with
scoped unchanged-owner reuse; it is not a fresh full-suite execution. Go V3
HTTP/app/server evidence is separately reported by its author, not freshly run
in this checkpoint. The two initial normal-merge approval attempts were
rejected before execution: the first required exact Go authorization; the sole
same-tool/same-parameter retry treated the supplied transcript as untrusted
delegated evidence. At that stage no third attempt or alternate actor/route was
used. Those historical records remain preserved. A subsequent direct user
message explicitly authorized the exact Go commit and this closure. The same
exec_command reviewer then accepted the normally executed `--no-ff` merge.

Go20 `fe915a405ac59a9412ae29ae685aa3822009b442` is integrated at
`17ff805a7fe3ab9338bf2ca511947c01cd497d88`, tree `3292e9a10c981d6ed32968e91699f3939fb7588d`.
The merge parents are the exact TS44 and Go20 source commits; the 64 frozen
modes/blobs match, and its delta from TS44 consists only of the 20 Go paths.
One renderer-only build from a new exact Git archive passed (21.835s,
644 files / 42,416,109 bytes), using matching existing package versions and
unchanged cache helper in the same shell. A package-metadata preflight error
occurred before any Vite build; its script/log are retained, and the corrected
metadata lookup preceded the sole actual renderer build. Only the three
existing provenance/notice documents change after the source merge; prose is
not renderer input and no rebuild is performed. Fresh receipts are recorded
in the linked checkpoint. These source/test/build records do not close
native/installed, live Provider, current CI/main, full source admission/parity,
packaging or release acceptance. The earlier Office11 OPEN claims remain
historical and bound to that earlier candidate.

## Schedule rule/calendar adaptation preparation — 2026-10-11

Status: candidate; bounded rule validation passes; whole Schedule integration remains pending.
Source is DeepSeek Harness `5badb15009ae1756c3afe0ae0cef1faafc290ccc`,
`packages/schedule/schedule/src/domain.ts`, blob
`28748ace0aa02b61de09987d3543890761f0a47a`; package manifest blob
`7ea453734aca03f7796dff9258e27ba8dc241084` pins Temporal 0.5.1.
License object `LICENSE` is `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`,
MIT, Copyright (c) 2026 DeepSeek; the existing complete notice is retained.
The DSH desktop consumer writer ports/adapts only timezone/weekday validation,
cron parser/star-preserving canonical encoders and next-calendar-target functions
into `packages/runtime/src/contracts/schedule-rule.ts` and
`src/main/schedule-calendar.ts`, with their direct tests. No Session/store/framing,
Cordis package or general calendar framework is imported. Adaptations preserve
Analytix legacy four fields and HH:mm, detach pure validation from Temporal,
keep Temporal only in Main, and retain legacy daily/Claw behavior.
Root manifest/lock adopt the regular npm production dependency
`@js-temporal/polyfill` exactly 0.5.1. Source object/license identity was checked
before adaptation; source-bound checks, dependency license intake and coordinator
review are pending, without native/parity/package/release claims.

Rule/calendar natural boundary: the normal isolated npm ci/postinstall completed
with new manifest/lock and official root/runtime stamps current on Node 22.22.1,
ABI 127, npm 10.9.4. Only Temporal 0.5.1 and jsbi 4.3.2 lock entries were added;
existing entries are unchanged. Temporal's ISC notice is preserved above the
existing complete DSH MIT notice reference; jsbi retains Apache-2.0.
Nine owner files provide 46 selected fixture/regression cases: seven unchanged
owners passed 30 selected cases; the initial two Main suites failed transform
because an extracted function lacked a closing brace, then only those two owners
were rerun and passed 16 selected cases after repair. Unselected cases were not
run. Node, Web and runtime type checks and one 21-file affected lint pass.
These are fixture/type/source checks, not runtime/native/Provider or renderer
build acceptance. Origin, admission ACK/history and the Host source-authentication
seam are not implemented in this rules phase. The source remains uncommitted,
with no staging, push, main mutation or package build.

Schedule History source adaptation (implementation pending verification): selected
`packages/client/ui-schedule/src/client/DeliveryHistory.tsx` and
`TaskManagerPage.module.css` at the same fixed DSH pin are adapted into existing
`ScheduleTaskDetail.tsx`/CSS. Copyright (c) 2026 DeepSeek and the complete existing
MIT notice apply. Changes use the actual Main ACK page API and stable attempt
identity, preserve native task/owner retirement, and reuse the already admitted
clock/chevron SVGs and font/tokens. No remote/store/component framework is imported.
The Host original-thread navigation remains separate from last-run navigation.


### DeepSeek Harness elected Records composer overlay source intake (2026-10-12 UTC)

This scoped intake extends the existing main Conversation width candidate;
its earlier producer-OPEN statement remains bound to that earlier checkpoint.
Research and source admission are fixed to `5badb15009ae1756c3afe0ae0cef1faafc290ccc`.
Reuse mode is bounded adaptation, reviewed by the coordinating renderer owner.
The existing MIT license object `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`,
Copyright (c) 2026 DeepSeek, and complete `THIRD_PARTY_NOTICES.md` notice apply.
No assets, upstream store, slot framework or measurement controller are imported.

| Exact pinned source | Git blob | Selected destination and adaptation |
| --- | --- | --- |
| `packages/client/ui-trajectory/src/client/TrajectoryView.tsx` | `6c2cd98ec8de072f940a2d2fae627dbcd1ca409b` | `src/renderer/src/components/chat/MessageTimeline.tsx`: the elected `data-conversation-composer-overlay` marker is bound to the existing scoped Records mode, including Inspect handoff; no Trajectory state or ledger implementation is copied. |
| `packages/client/ui-conversation/src/client/skeleton/ConversationRoot.module.css` | `3aa0a9d1f933651c53851358d0be59f8bef8ca49` | Existing `Workbench.tsx`, `workbench-layout.ts` and `styles/base-shell.css`: full-bleed view scroller, floating composer seat and gutter compensation. Existing composer ResizeObserver measurement supplies bottom clearance; ordinary chat retains its existing width axis and symmetric gutter. |

The source window is based on Analytix `4428236a780c34655d09c4b8f0b7c5d881c73b85`.
Direct validation owners are the existing Records, workbench-layout and
ConversationWidthControls wiring tests. Tests are pending at intake; web types,
affected lint, renderer build and native/installed acceptance are not claimed.
No stage, commit, merge, push, main modification or live Provider use is admitted.


## Schedule original-session integration checkpoint — 2026-10-12 UTC

Status: locally integrated candidate with bounded renderer evidence. Normal merge
`4428236a780c34655d09c4b8f0b7c5d881c73b85`, tree
`1ee612b3fb983b00f0873312fc7e301eaaab0db3`, combines TS40
`6fc30573570713b5140ef44d3ebecedfe50f88fd` and Go23
`66023f7ebce6250f5109c1b3f86e296eb821f56a` without conflicts or path overlap.
Exact modes/blobs and unchanged out-of-scope entries bind this source checkpoint.
The selected History adaptation adds `packages/client/ui-schedule/src/client/DeliveryHistory.tsx`
(blob `2e6c1f3a3679fc4d25522efa26d26bc197a0d22f`) and the History subset of
`TaskManagerPage.module.css` (blob `25b573fcd4c84b938632d9ed90fb4a48ed2ca8b7`)
at the existing fixed DSH pin to the existing `ScheduleTaskDetail.tsx`/CSS owners.
The complete MIT notice remains. Earlier rules/history preparation and UI37 gaps
are retained as phase history; their current source integration is this checkpoint.

Exactly one renderer-only build from the merged Git archive passed, exit 0,
70.834 seconds, 644 files / 42,601,619 bytes. The official cache helper ran in
that shell; only `vite.build(resolved.config.renderer)` was invoked. No fresh
Go23 test PASS or full dependency byte-identity proof is added here. This build
covers the committed merge and excludes later Overlay, media and logical-usage
source preparation. See the [combination record](deepseek-harness-presentation-sync.md#schedule-original-session-combination-checkpoint-2026-10-12-utc).
Native, Provider, CI/main, full DSH parity, packaging and release remain OPEN.

## DSH Excel parser and viewer source intake — 2026-10-12 UTC

Currentness: source-intake preparation statements below predate the later [installation qualification](#excel-installation-qualification--2026-10-12-utc) and [finite implementation qualification](#excel-finite-implementation-qualification-2026-10-12-utc). Preserve them as phase history; the later evidence closes only its named implementation/standalone gaps. Browser/native viewer and complete Copy acceptance remain OPEN.

Disposition: candidate; reuse mode: port-and-adapt. Fixed DeepSeek Harness pin
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`, MIT, Copyright (c) 2026 DeepSeek;
root LICENSE blob `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785` and the existing
complete [notice](../../../THIRD_PARTY_NOTICES.md) apply to these selected
ordinary source objects. Coordinating source writer checked these exact objects
and bounded imports before preparation. Independent review and executable
validation are pending; this is intake, not a viewer or artifact PASS.

| Source at the fixed pin | Git blob | Exact destination | Current scope |
| --- | --- | --- | --- |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/ExcelBody.module.css` | `69f5a58c44ab20d872e11c5840519f8e8dd84625` | `src/renderer/src/components/file-preview/excel/ExcelBody.module.css` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/LazyExcelBody.tsx` | `1c4c657a24719c0f07a0ec1075f6e0b01e2d4df0` | `src/renderer/src/components/file-preview/excel/LazyExcelBody.tsx` | UI source candidate; execution pending |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/convert.ts` | `0ad1e5b9a7e8ad7ddf6cd3e6207bd3870434864b` | `src/shared/excel/convert.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/delimited.ts` | `8c2cc80d510dbca7afec1aeae7b0e063d4042719` | `src/shared/excel/delimited.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/error.ts` | `e3e6affd620d34eaca2285e93286271d1252825e` | `src/shared/excel/error.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/excel.tsx` | `ca3fcb8425588610d12e468a83497a37d3bb0175` | `src/renderer/src/components/file-preview/excel/ExcelBody.tsx` | UI source candidate; execution pending |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/format.ts` | `6385c9de841db59dc9df29348f3d8ce91c45fe1a` | `src/shared/excel/format.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/locales.ts` | `79b97690a44885b40925380b86eddc79dc6f2fad` | `src/renderer/src/components/file-preview/excel/locales.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/model.ts` | `51c2796f973ca40782991610f729e7dc5562bcd5` | `src/shared/excel/model.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/parse.ts` | `6e27de1b195075c6cfb07c9497b6f2c5f02463b4` | `src/renderer/src/components/file-preview/excel/parse.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/worker.ts` | `0fbea7a3535c4120dbc444689ba5aab2fedb0b09` | `src/renderer/src/components/file-preview/excel/worker.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/xls.ts` | `43a4eb12374a9075fe47c487c561d5ff70821a81` | `src/shared/excel/xls.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/xlsx-archive.ts` | `b8e5d3b7c7484cafcb1c73aa4ba2efc4cb177470` | `src/shared/excel/xlsx-archive.ts` | parser/style/locale source preparation |
| `packages/client/ui-sidebar-documentpreview/src/client/excel/xlsx.ts` | `7c4608cc04753d054006f8c0f67b4777208034e4` | `src/shared/excel/xlsx.ts` | parser/style/locale source preparation |

The parser keeps XLSX/BIFF/CSV/TSV saved values and limits, without formula
recalculation or external-link execution. Selected adaptations remove the
upstream raw-IIFE worker loader, TS import suffixes and Cordis locale declaration;
a same-origin Vite module Worker retains copied bytes, timeout, termination and
abort retirement. CSS selects warning/secondary fallback colors from
`ui-theme/src/styles/design-platform.css` (blob `eb67869c92bf53162dbd443c8a0e4fe8baabb11b`)
and radius8 from `ui-theme/src/styles/base.css` (blob
`23be07f678050d8f144199d0312697118b9f33bc`), under the real `data-theme`.
No new font, icon package, parser framework, RPC/store/plugin, CSP grant,
public Go event DTO or renderer path authority is included. `index.ts` and
`asset-imports.d.ts` are excluded. The two UI leaves now use the existing file
preview pane, Tooltip and locale system. Complete bytes enter through the
existing files/preload/Main route; protected Copy retains the original Present
index, selector, snapshot digest and native document lease. The eight pure
converter files have one shared implementation; no duplicate parser or thin
renderer re-export files remain. Execution qualification remains pending.

FortuneSheet's original complete MIT notice (Copyright (c) 2022 Suzhou Ruilisi
Technology Co., Ltd) is retained byte-for-byte from
`packages/client/ui-sidebar-documentpreview/licenses/FortuneSheet.txt`, blob
`065d1f9e7eade6f4765ffbc2413cd087b09c9a3d`, at
`src/renderer/src/components/file-preview/excel/licenses/FortuneSheet.txt`.
The fixed dependency/patch proposal retains Fortune core/react1.0.4 and
ExcelJS4.4.0 with all three DSH patches. The manifest, local patches and normal
postinstall owner are prepared; package-lock resolution and installation await
one coordinated normal npm window. Stock versions are not asserted equivalent. SheetJS0.20.3's Apache-2.0 payload/notice remains
subject to actual package admission. No dependency is installed by this intake.


The Regular warning triangle slice in
`packages/client/ui-primitives/src/icons/index.tsx`, blob
`d22cbdd06d50289f15154996564b250b2af4163a`, is ported to
`src/renderer/src/components/chat/presentation/DeliverableIcons.tsx`. Its two
paths, rounded joins/caps and Regular stroke width retain the same DSH MIT
notice; no icon package or other artwork is introduced.

| Fixed DSH patch source | Git blob | Local npm patch destination |
| --- | --- | --- |
| `patches/@fortune-sheet__core@1.0.4.patch` | `747433a75db13de814985a54d3284d077ad0f61b` | `patches/@fortune-sheet+core+1.0.4.patch` |
| `patches/@fortune-sheet__react@1.0.4.patch` | `4be77d9609e17c053985a97a29b395a2350430eb` | `patches/@fortune-sheet+react+1.0.4.patch` |
| `patches/exceljs@4.4.0.patch` | `87ee0151b2cd7d0c863e9f920bd1f59f3f3acae8` | `patches/exceljs+4.4.0.patch` |

The core/react adaptation adds a dedicated synchronous `protectedCopy` prop,
not a claimed upstream hook. Undefined retains ordinary behavior; null,
not-ready, throwing or asynchronous handlers cannot fall through to native
clipboard or session storage. The common Copy entry clones the current sheet
and bounded closed ranges; the original handleCopy prechecks remain, and protected
KeyC prevents its native default synchronously. This is source intent, pending
actual dispatch qualification and native UI evidence.

Official fixed package bytes were read without executing package code. All
13 modified postimages (core4, react2, ExcelJS7) were reconstructed as text and
frozen by SHA-256. The two original DSH core patch after-index fields are stale;
they are not postimage evidence. The local wrapper checks the exact three
ordinary patch files, versions and actual postimages; its bytes and patch bytes
enter the existing installation digest, with read-only Doctor qualification and
fail-closed postinstall stamping. Patch application has not yet run.

The renderer Worker and the fixed second codec entry import the same converter.
The second entry uses the existing codec builder/payload validator, existing
Mac Helper resolver and owned-process lifecycle; generation stdin/stdout stays
unchanged. The prepared entry closes external package/dynamic require and
network/process-launch builtin imports. It has no workbook path or raw-text
export authority. This is application-level isolation, not an OS network
sandbox. Real Node import, number-format and standalone closure qualification
remain pending.

Source budgets are16MiB compressed bytes,64MiB total ZIP expansion checked
before inflation,250000 rectangular cells and15000ms parsing. The shared converter checks a16MiB complete data-message budget before
structured-clone/JSON allocation, including repeated shared-string amplification. The Copy codec
has16MiB output and Host selected text has1MiB UTF-8; over-limit inputs fail
closed rather than truncating or silently omitting cells. Workbook formulas use
saved caches, including zero/false; absent caches remain missing/empty. The
Host owns selection serialization, ordinary export projection and the final
current-file/digest/lease checks. Pending test sources do not establish PASS. The
protected adapter now carries1–32 ranges and parsed number-format/date markers.
The Host admits same-axis nonoverlapping ranges within250000 cells, preserves
first-seen row/column order, gives date display `m` precedence, uses `v` for the
fixed W/w format exception and otherwise falls back from nil `m` to `v`; empty
`m` stays empty. Regression sources are prepared but not executed. The current
TSV sink is a controlled base route, not complete DSH Copy acceptance.

### Copy completion remains OPEN

Currentness: this matrix and missing-seam description preserve the pre-HTML candidate. See the [later finite qualification](#excel-finite-implementation-qualification-2026-10-12-utc) for implemented and executed subitems; browser/native full Copy remains OPEN.

The positions below refer to the official core1.0.4 `dist/index.esm.js` source;
its actual package bytes and the fixed DSH patch remain the source inputs.
Full-chain source review supersedes the withdrawn partial hidden-filter reading.
No hidden-filter hunk was applied.

| Behavior | Fixed native source behavior / position | Current candidate and gap |
| --- | --- | --- |
| Hidden rows/columns | `copy`62935–62975 filters only state calculations; `rangeValueToHtml`62612–62638 retains all selected indices. | Hidden coordinates retained; same source behavior, runtime comparison pending. |
| Entry/range semantics | `handleCopy`76760–76834 accepts same-axis nonoverlapping keyboard ranges and rejects partial merges; React ContextMenu44–62 rejects multi-selection. | Original prechecks retained; Host validates bounded ranges and original first-seen ordering. Source prepared; tests pending.32 ranges is an explicit candidate input limit. |
| W, dates, numbers and caches | `getCellValue`60399–60432 makes `ct.t=d` use `m` even for `v`;62670–62676 chooses W/w raw values;62863–62869 falls back only for nil `m`. | Parsed metadata now reaches Host; W/date/zero/false/empty/missing-cache cases prepared, not executed. |
| Merge |62681–62686 adds master rowspan/colspan;62829–62833 omits shadow td. | TSV master plus empty shadows does not carry HTML merge structure: OPEN. |
| Styles/dimensions |61258–61299 and62649–62862 carry style helpers, borders, column widths and row heights. | Current neutral Copy frame/text sink has no HTML styles or dimensions: OPEN. |
| Rich text/formula/link | Fixed selection chain has no active rich-run span branch, formula/raw-v dataattrs or hyperlink output. DSH patch uses `_.escape` at62869. | No invented rich-run framework is required; saved joined text and omission of f/links still need actual qualification. |
| Newline/tab/quotes | `clipboard.writeHtml`61435–61466 copies a DOM table; browser innerText/textContent determines plain text. | Quoted TSV is explicit; exact browser whitespace/delimiters and dual-format behavior: OPEN. |
| CF execution | `getStyleByCell`61258–61266 unconditionally calls getComputeMap/checkCF. | Future HTML must reject CF rules and use a checked empty-CF context; current TSV calls neither helper. HTML qualification OPEN. |

The minimal missing seam is within this same protected effect:
`ToolResultLocalDisplayHostV1` currently accepts `copy(text)` and registration
calls `clipboard.writeText`; `projectOrdinaryWriteExportContentV1` is text-only.
Core actually exports `rangeValueToHtml`, `getStyleByCell` and `getCellValue`.
Reusing those helpers requires bounded typed styles/config from the same
converter, ordinary text projection before HTML escaping, explicit no-CF
qualification and rejection of executable or remote-resource output. The same
Main sink then needs optional checked HTML alongside text while preserving
freshness, digest, authority and existing byte limits. No renderer HTML,
DOM/sessionStorage copier or new ordinary clipboard outlet is authorized by
this plan. This OPEN subitem does not block the normal dependency-install
window or read-only viewer qualification and does not establish Copy parity.

### Excel installation qualification — 2026-10-12 UTC

The configured Mac executed one script-enabled normal root `npm install`
(exit1,286.451s), retained its generated lock and failure, repaired only the
patch-package8.0.0 bin string/path normalization, then recovered the original
pipeline with `npm run postinstall` (exit0,31.110s). Runtime `npm ci`, clean,
TypeScript compilation and the existing native/stamp lifecycle actually ran.
The13 installed core/react/ExcelJS postimages match the reviewed SHA-256
manifest. Existing app/runtime stamp checks and the Electron in-memory SQLite
plus terminal module-load ABI check PASS (5.018s). Original shared dependency
link targets and canonical source remain unchanged; actual dependency/build
outputs belong to this task's worktree. No second root install, script skipping,
force/legacy-peer-deps, GUI, real document, Provider or Git write occurred.

The root lock now records the exact approved versions and official SheetJS
0.20.3 tarball/integrity; its SHA-256 is
`39351352979f91ccab9eb1b83320b5f0fca992ff448c199607146df4de13ef76`.
This establishes installed inputs and actual patch application, not complete
packaged-license admission, standalone closure, viewer or Copy acceptance.
The [installation checkpoint](deepseek-harness-presentation-sync.md#excel-dependency-installation-checkpoint-2026-10-12-utc)
records the bounded commands, elapsed times and remaining evidence.

A single Node probe loaded actual core `update` and ExcelJS with no DOM shim;
`update('0.00',1.234)` returned `1.23`. Its eight-file local CJS erasure
harness stopped at the import-only SheetJS codepage export with
`ERR_PACKAGE_PATH_NOT_EXPORTED` (exit1,22.350s), before CSV/XLSX conversion
samples. Real Node module/formatter load is observed; converter display-value
qualification is still pending. Production Vite receives ESM imports before
emitting CJS, so the probe's require-condition failure is not a production build
failure. No source import was changed to conceal this verification gap.


### Excel finite implementation qualification 2026-10-12 UTC

Currentness: the preceding source-intake and Copy OPEN matrix describe earlier
preparation. Their formatter, merge/style and selected-owner execution gaps are
superseded only by this bounded evidence; complete browser/native Copy and
viewer acceptance remain OPEN. The [current qualification record](deepseek-harness-presentation-sync.md#excel-finite-owner-and-paired-copy-qualification-2026-10-12-utc)
preserves the installation checkpoint, original CJS probe failure, subsequent
ESM qualification, finite owner selection and later combined web failure.

The Excel v5 validation snapshot has59 task-owned source/test/dependency/notice
and provenance paths, frozen at HEAD
`e4ac34c5d191ab3c5b3745f953fc6dbd03abfcc8` in
`/tmp/analytix-dsh-excel-html-source-freeze-v5.json`, SHA-256
`754327e749ac8864df083123a913e2d6ede63bf06665dbcc243caef0f2f677e8`.
No new upstream pin, parser, package version or notice is introduced by the
HTML adaptation. Installed Fortune's actual public formatter and value/merge
helpers run through the same fixed codec and shared converter with checked
styles/config and empty CF. Ordinary literal projection precedes HTML escaping;
Host admits only the fixed table grammar, matching projected literals and merge
coverage. The paired complete-text guard was repaired after horizontal/vertical
cross-cell privacy RED cases, and the affected green cases retain zero native
effects on refusal. One Main dual-format clipboard write follows the unchanged
original current-file/digest/lease checks; neither renderer content nor a raw
DOM/browser copier supplies authority.

Finite evidence is244 unique PASS:221 selected Vitest cases across13 owners,
19 patch-wrapper cases and4 standalone codec cases. The510 named-scope skips
remain skipped, and unchanged-input reuse is explicit in
`/tmp/analytix-dsh-excel-final-owner-results-v1.json`. Final v4 Node types,
affected lint and actual standalone build/closure exit0. The Node ESM v2
synthetic CSV/XLSX qualification passes with real npm import resolution and no
DOM shim; the earlier CJS resolver failure remains historical evidence.

The separate62-path isolated Excel/Plan combination has144/144 Plan cases and
three-file lint PASS, but web types are blocked: exit2, nine DOM/Node timer
errors plus missing browser-bridge `readExcel`. Original Plan worktree files
were unchanged. Seven source-only timer/browser-reader repairs have since been
integrated, bringing the owned candidate to66 paths; their new single case,
seven-file lint and combined web types are pending. The244 PASS remains the
earlier59-path Excel qualification, not a66-path or web type PASS. No renderer Worker build,
native GUI/keyboard/menu, browser native text MIME, PTY process, packaged,
current CI/release or full DSH acceptance is added. Existing installation and
license observations retain their original scope.

## Overlay media logical-usage and Settings candidate — 2026-10-12 UTC

Status: uncommitted candidate on `codex/dsh-native-event-hookup`, base
`4428236a780c34655d09c4b8f0b7c5d881c73b85`; the final affected Settings suites
passed31/31, including two departure/reload error regressions, and the current
bilingual Legacy owner retains its unchanged-input12/12 pass. The initial four Settings TS-file lint and later two-file affected lint exited0
with no warnings. The exact current batch has35 source/test paths plus three selected
provenance document deltas. The earlier Records source intake above supplies
its two fixed-pin source objects and existing MIT attribution; no new third-party
source object or dependency is admitted by media, logical-usage or Settings.

The coordinating sole TS/UI source writer reviewed the final scoped diff.
Records elects the real existing overlay marker and measured composer clearance,
including thread-scope retirement. Protected Present media reuses existing
ImagePreview/WritePdfViewer consumers through original selector/digest/lease
checks and complete bounded Host bytes. Logical-usage follows the closed wire
contract, including unknown counts independent of proven timing and complete
billing coverage. Settings retains local drafts on failed or stale departure,
with explicit retry; the Legacy card reports observed folder names and honestly
unavailable import/detection instead of exposing unimplemented actions.

The [bounded evidence record](deepseek-harness-presentation-sync.md#overlay-media-logical-usage-and-settings-candidate-2026-10-12-utc)
preserves initial failures, focused repairs and unchanged-input reuse separately.
The independent Settings Host-readback epoch guard passed the existing
late-recovery regression; departure, retry and draft-retirement checks also
passed. A late rejected reload cannot replace a newer draft's saving status; a
current failed departure shows a notice without presenting a false save failure. The earlier four-suite49/50 failure remains in its original receipt.
This is scoped repaired evidence, not a newly run complete50-case pass.
Final web noEmit passed for the exact35 source/test candidate after the two
Settings files changed. Node/runtime inputs exclude these renderer-only owners;
their individual passing commands are reused without repeating types. Excel and
renderer-build acceptance remain outside this evidence. The exact35 source/test
files and three selected document deltas are frozen in the bounded evidence
record, with Excel-only paragraphs excluded from the batch patch.
The15 untracked Excel files and their intake/notice paragraphs remain a separate
next batch. No install, combined renderer build, native/Provider execution,
stage, commit, merge, push or main change is included in this checkpoint.


## DSH running indicator source intake — 2026-10-13 UTC

Status: candidate; reuse mode: port-and-adapt. Source is DeepSeek Harness
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`, MIT, license object
`c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`; existing DeepSeek copyright
and MIT text remain in `THIRD_PARTY_NOTICES.md`. Reviewer: coordinating
DSH source owner. This adds no dependency or product authority.

| Source under `packages/client/ui-chat/src/client/chat/` | Git blob | Destination under `src/renderer/src/components/chat/presentation/chat/turn-state/` |
| --- | --- | --- |
| `RunningWhaleTail.tsx` | `d528c7502a0d51f50575ba1a152b488e39e493b9` | `RunningWhaleTail.tsx` |
| `running-whale@2x.png` | `daa680536c0a2b9044adc5b310a396deb0308a80` | `running-whale@2x.png` |
| `ChatView.module.css` (running indicator only) | `ff121998a6ba3dad6df5583200750642066abea9` | `TurnState.module.css` |

Adaptations: local CSS module, existing Analytix color/font tokens, and the
application reduced-motion setting in addition to the OS preference. The
APNG is copied without pixel changes. Local status text and clocks remain
bound to actual runtime facts. Source review is complete; affected tests,
bundle and native visual acceptance are pending for this candidate.


## DSH resident input and initial conversation candidate — 2026-10-13 UTC

Disposition: `candidate`; reuse mode: `port-and-adapt`. Source repository:
DeepSeek Harness, full commit `5badb15009ae1756c3afe0ae0cef1faafc290ccc`.
All source paths below are under
`packages/client/ui-conversation/src/client/skeleton/`; destinations are under
`src/renderer/src/`. Each raw byte stream's Git blob matches the official
fixed-commit object. The registered local checkout is unavailable; the bounded
intake uses the official public fixed-commit source rather than another branch.
Root MIT license object `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`,
Copyright (c) 2026 DeepSeek, is retained in full in `THIRD_PARTY_NOTICES.md`.
Ancestor listings, the selected package tree and selected file notices show no
nearer license exception. No separate proprietary authorization is required.

| Exact source leaf | Git blob | Destination and selected adaptation |
| --- | --- | --- |
| `InputBar.tsx` | `031413132b461b76c8632a06dfbdadd2d5f38dd9` | `components/chat/InputBar.tsx`: resident input/attachment/toolbar seats; existing Analytix editor, gestures and submission authority retained |
| `InputBar.module.css` | `4676f4cd7dd400937d00f531c5b0ecadc1056081` | `styles/base-shell.css`: shared card/editor/toolbar geometry for main, SDD and Inspector |
| `ConversationMainPanel.tsx` | `674b644a9fa50ee0997259288fc816966bb41812` | Existing `conversationPresentationPhase` through Workbench to the initial row and resident input attributes |
| `ConversationContent.tsx` | `15bfdd8732f577df9d7f1a7f58274e8eae362000` | Workbench's one permanent composer seat and phase-only visual transition |
| `EmptyHero.tsx` | `40df87269b5f9b434dbe561b03f3d368d0d53301` | `components/chat/message-timeline-empty.tsx`: separate hero chrome, existing Analytix brand and locale copy |
| `HeroShell.module.css` | `aac6a5cf616ca5b7a0d14a1b2bce9356624b92ec` | `styles/base-shell.css`: headline 26/32 weight500, mark34, gap10/12; no upstream fish artwork |
| `ConversationRoot.module.css` | `3aa0a9d1f933651c53851358d0be59f8bef8ca49` | Existing width/scroll/composer authority retained; hero positioning and settling visibility added without remounting input |

The coordinating input writer adapts composition and geometry to the existing
Analytix workspace/thread drafts, ProseMirror mention model, upload/currentness
checks and Host submission fences. Main, SDD and Inspector invoke their own
existing submission callbacks. Protected Gate overlays and their selectors,
leases and retained question seats keep their original owners. Fork, Claw,
workspace recovery and Write/native preparation boundaries remain in place.
No DSH Runtime/Session store, Cordis/slot framework, Lexical editor, dictionary,
fish asset or second draft state machine is imported. `SessionInputShell` is
a class in `packages/client/ui-conversation/src/client/input/facade.ts`
(blob `9f4a264e46e6f09451d3fb1c7118da8a34ebb63d`), reached through
`input/hub.ts` (blob `fb8d8a736f3722b058aee708add2c36628e38f56`).
Those two verified objects are reference-only: their state-machine/editor
implementation is not admitted or imported by this unit.

The old FloatingComposer/Island and no-op composer factories are retired after
all three real consumers switch. The unused old queue view and initial-only
heatmap are retired; real queued-message and measured session-usage leaves remain.
The new owner preserves the old feedback-toast DOM anchor as a compatibility
class, with styling owned by InputBar. Source review includes an independent
read-only boundary review. `git diff --check` passes for the static candidate;
direct regressions, types/lint, combined build and native acceptance are pending
the coordinator's exclusive execution window. This is source admission only,
not full DSH parity, current CI, distribution or release approval.

The hero brand amendment uses first-party formal Analytix assets and user-owned
product copy, not the upstream fish or dictionaries. Exact badge token/font
reference reuses previously admitted `packages/client/ui-theme/src/styles/design-platform.css`
(`eb67869c92bf53162dbd443c8a0e4fe8baabb11b`) and `base.css`
(`23be07f678050d8f144199d0312697118b9f33bc`); their raw bytes are rechecked at
the same fixed pin with the existing full MIT notice. Upstream's intentional
above-center bottom padding is adapted to symmetric padding for the user's
explicit available-chat-area centering. This remains a candidate pending the
new direct checks and coordinated rendered/native acceptance.

The amended input source subsequently passes its direct owner regressions
(188 interaction tests plus one selected real Footer fork adapter case), web/node
types and affected-file lint. Initial failure and fixed-pin baseline evidence
remain preserved; the local delayed-focus repair does not import additional
upstream behavior. Combined build and rendered/native acceptance remain pending.

## DSH user message destination update — 2026-10-15 UTC

Disposition: `candidate`; reuse mode: `port-and-adapt`; reviewer: delegated A
renderer/documentation owner with independent read-only source review. The
existing DeepSeek Harness source remains fixed to
`5badb15009ae1756c3afe0ae0cef1faafc290ccc`; MIT license object
`c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`, Copyright (c) 2026 DeepSeek.
The complete existing notice in [THIRD_PARTY_NOTICES.md](../../../THIRD_PARTY_NOTICES.md#deepseek-harness-presentation-leaves)
and adapted source headers are retained. This supplements the historical Footer
source map above with the extracted helper and user-only CSS destinations.

| Exact upstream source | Fixed Git blob | Actual local destination / selected scope |
| --- | --- | --- |
| `packages/client/ui-chat/src/client/chat/MessageItem.module.css` | `5d194833358d96278bbd1c31e69169f485bb2dbb` | `src/renderer/src/components/chat/UserMessage.module.css`: right-aligned user row/stack, width cap, bubble geometry and local token adaptation. |
| `packages/client/ui-chat/src/client/chat/message-chrome.ts` | `e078d28177066fbd4110fb12afbe3e81bcce7bd1` | `src/renderer/src/components/chat/presentation/chat/message-chrome.ts`: existing local clock formatter extracted from `TurnTailView.tsx`, shared by user and assistant consumers. |
| `packages/client/ui-chat/src/client/chat/use-calendar-day.ts` | `29e5311383fa5d6cba826b6b607d60cb1cc95d1b` | `src/renderer/src/components/chat/presentation/chat/message-chrome.ts`: existing local calendar-day hook extracted without changing its body. |

Existing action CSS and admitted Copy/Check/Edit SVGs remain in their recorded
owners. Analytix editing, public text projection and protected reply authority
are preserved. Direct regressions are prepared; tests, types/build and rendered
acceptance remain for the coordinating validation window. This records source
destinations, not a runtime, parity, canonical integration or release result.

### DeepSeek Harness independent public trajectory source intake — 2026-10-15 UTC

Disposition: `candidate`; reuse mode: `port-and-adapt`; reviewer: delegated
trajectory writer with independent read-only UI/wire review. Research/source identity is fixed to `5badb15009ae1756c3afe0ae0cef1faafc290ccc`.
The exact MIT license object is `c1f7a78e89e4e4dc7b86664c3b3c76eb5eee1785`,
Copyright (c) 2026 DeepSeek; the complete existing MIT notice in
`THIRD_PARTY_NOTICES.md` applies. Each adapted presentation file retains the pin,
copyright, and notice pointer. `pin/manifest.json` contains SHA-256 and Git blob
identities for every selected source below. No file exception, assets, upstream
store, execution engine, protected provider parser, slot framework, dependency or
PTC parser is imported.

Every short destination below is relative to `src/renderer/src/components/trajectory/`.

| Exact source under `packages/client/ui-trajectory/src/client/` | Git blob | Destination and bounded adaptation |
| --- | --- | --- |
| `TrajectoryView.tsx` | `6c2cd98ec8de072f940a2d2fae627dbcd1ca409b` | `src/renderer/src/components/trajectory/TrajectoryPanel.tsx`: independent conversation view, public record coordination, floating-composer marker, resident50/+50, selected record seat. |
| `views.module.css` | `687f4a465782f1fa87e7200a79ca3026bf5efcef` | `src/renderer/src/components/trajectory/TrajectoryPanel.module.css`: full-bleed flex shell and existing composer-height clearance. |
| `TrajectoryToolbar.tsx` | `c3861f03a95f96183ca22e2e67fe7f432f3c353f` | `TrajectoryPanel.tsx`: visible Duration, Turns/Calls bulk fold and AND public search. Hidden Actual time control remains unexposed. |
| `TrajectoryToolbar.module.css` | `ac89529bd5b50c96f7e48c194564d5aa8df17a7b` | `TrajectoryPanel.module.css`: selected 32px/20px toolbar, horizontal padding/gaps and flex search rules, existing Analytix tokens. |
| `TrajectoryTimeline.tsx` | `148de3bc018def403f5eb73a5245c8769a8e5b29` | `TrajectoryTimeline.tsx`: drag min3, wheel exp(deltaY*.0015), cursor anchor, right pan, range/reset, edge pan, equal lanes and actual point/span selection. |
| `TrajectoryTimeline.module.css` | `ec7d45db17eb4a0a5ed739d8a16520950887b8f3` | `TrajectoryPanel.module.css`: selected50px plot/44px labels/14px lanes/8px spans, range/hover/search/selection rules. |
| `timeline.ts` | `40cf1c72879f0681f00c7a4bdd084275e0fd1957` | `trajectory-timeline-model.ts`: equal sequence slots; Duration compresses idle gaps from real Host intervals; full-retained coordinates stabilize local prepend. Unknown ends stay points. |
| `trajectory-record.ts` | `bd37189fa499433ddd1291166d65b2dfa5464ad4` | `trajectory-adapter.ts`, `TrajectoryPanel.tsx`: record identity/navigation purpose; actual DTO projection is Analytix-specific closed public whitelist with stable turn/logical-step and call/item IDs. |
| `TrajectoryTable.tsx` | `9eeeb5c2abb0e62994ae0031be32f21a4d806c06` | `TrajectoryTable.tsx`, `TrajectoryInspector.tsx`, `TrajectoryPanel.tsx`: chronological turn/runs, stable group folding, selected-row reveal, viewport/tail/prepend anchor, public Inspector tabs/resizer. No model→tool parent is inferred. |
| `TrajectoryTable.module.css` | `12b03b1c51ff6b7791a03c7abe7d1a44c2e647ad` | `TrajectoryPanel.module.css`: selected 30px rows/header,122px/84px Event column, .5px boundaries, Inspector clamp(320px,38%,440px),42px header/34px tabs/94px overview and narrow overlay. |

Analytix changes source input and authority: only the existing validated finite
replay and accepted live dispatch fan out into an isolated public projection;
there is no second SSE, ACK owner, chat store/mapper mutation, raw-event archive,
provider-body capture or protected path persistence. Model start→assembled uses
the new Host assembly event, never close/HTTP counters; tool ready is separate
from actual execution. Retry folds by real logical-step UUID. Terminal public
assistant text remains its own ledger record because the wire has no step→text
item identity. PTC and inferred model/tool ancestry remain absent. Turns/Calls are
folds, not filters. Public facts determine available Inspector tabs. Tool Inspect
uses an explicit existing real-identity callback and ProtectedToolResult owner.

Memory is one current-thread display binding plus live/replay/view leases, with
old thread retirement. Stable IDs and source field sequence numbers deduplicate
replay/live without an unbounded event-ID set. Each retained projection is capped
at2000 records,16Ki UTF-16 preview units/record and128Ki total preview units. A
monotonic discarded seq floor prevents old replay/tombstones from resurrecting a
discarded prefix. Truncation is visible, completeThroughSeq becomes unknown, and
search is explicitly limited to retained previews; no server paging is invented.

Direct synthetic regression sources cover identity/time/privacy, replay watermark,
retirement/caps, resident+50, drag/zoom/pan/reset, stable prepend coordinates,
fold/search/reveal, tail/prepend/resize scroll, explicit protected intent retirement
and unchanged accepted dispatch→ACK→observer ordering. Static reviewer findings
were repaired. No tests, types, lint, builds, GUI/native or Provider acceptance ran
in this writer window. No pixel-equivalence, whole DSH parity, admission PASS,
canonical integration, commit, push, release or publication is claimed.
