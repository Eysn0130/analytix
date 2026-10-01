# Round3: exact measures and trusted projection boundaries

Status: Operational checkpoint / component evidence; not product or release acceptance
Scope: PR #28; two Go privacyprojection files, this checkpoint and its index
Start remote HEAD: `d773f03a5230c8f51dde1f89fd18097fb5b0665d`
PR base: `ce96cf12581acfa0e19fae7c6aa9c709371012c8`
Branch: `codex/workbench-product-delivery-20260914`
Date: 2026-09-17 UTC / 2026-09-16 America/Los_Angeles
Authority: current user-approved continuation; existing accepted requirements and current code

## Implemented, not just proposed

1. `privacyprojection/value.go`: give `map[string]string` the same existing closed numeric-measure rule as `map[string]any`, in both projection and validation. Exact decimal strings, negative values, minor units and zero must not become PII placeholders merely because their Go container type differs. The allowed field list and numeric grammar are unchanged. Typed PII, unknown strings and nonnumeric content still take their existing conservative paths.
2. Restrict the host-generated question-ID preservation exception to a trusted parent. Provider-originated or prose-nested records cannot grant that exception by copying `kind`, `inputId` and `questions`. The existing trusted host protocol remains covered by positive tests.
3. Add `value_semantics_test.go`: three top-level regression tests covering map/list representations, typed PII precedence, input immutability, fixed-point projection, and both host-shaped record kinds in two untrusted contexts. Clarify existing comments instead of creating a new privacy engine.

This proves defects at the domain API, not an observed real-provider data leak or a closed CodeQL alert. The outer provider boundary has its own preparation, authorization and repeated validation. A numeric value under an allowed measure key is not, by itself, authority to disclose it.

## Evidence and its limits

The public source snapshot at `4c2907adfa4edb5fa3b942d8efb294b06041e3da` was recovered from an existing Actions artifact and all 6,341 snapshot blob hashes checked. Subsequent product-source changes were recovered from reviewed patches and compared against remote change metadata. The changed Go baseline was independently matched to the exact current remote blob `3b89d8c1a2368472035e9bbe4e2ddf3fa2084f57`; the current handover index was likewise matched before editing. The isolated local reconstruction is not the user's canonical macOS worktree and its synthetic Git ancestry must not be pushed.

| Check actually run | Result and boundary |
| --- | --- |
| Original privacy source + three new top-level regressions | 25 existing top-level tests pass, 3 new tests fail; exit 1 |
| Final source, privacyprojection / secretprojection / restrictedevidence | 28 + 17 + 12 = 57 top-level tests pass; no skips; exit 0 |
| Same three packages with race detection | Same 57 top-level tests pass; not 57 additional distinct tests |
| Required module-mode Go invocation | Blocked: repository requires Go >=1.25.0; available compiler is 1.23.2 |
| Component execution method | Isolated GOPATH source-only check with the unchanged standard-library-only package closure, GO111MODULE=off and GOTOOLCHAIN=local |
| Module/lockfile and CI configuration | Not modified by this delivery |
| Full Go application / Rust / TypeScript / GUI / installation | Not verified by these component checks |

Exact reproducible component commands and JSON test events are retained in the formal Round3 evidence export. The older-compiler source check is supplementary evidence, not a replacement for required module-mode CI. Re-run the affected packages and their provider/event callers with the repository's required toolchain before merge.

## Round2 remains NOT INTEGRATED

The archived patch SHA-256 is `06af7fbaf6f889853203525288fc3cd720e5f04f2bbecbaf6bab4a15855a5bd1`, based on `5a2db4e67c739b0fab41ed48cde6fe4d16f13de7`. The independent candidate `6678364c16c0e5142df652753761e31e9ac6d58f` is not remote ancestry.

This round recovered and checked that patch locally. Its focused 48 tests, existing CI-helper 8 tests, and syntax checks for 9 changed MJS files passed again. Removing its proposed one extra step leaves the original CI YAML structure unchanged. However, real `npm ci --ignore-scripts` failed with registry DNS `EAI_AGAIN`; the full plugin test attempt failed in the golden phase without Ajv (11 failed checks). A negative test proving failure without Ajv is not a positive validation test with real Ajv. Consequently the 17-file Round2 patch and its CI step are NOT included in this remote delivery and are not counted as new work.

## Direction preserved and researched next step

Keep one Go Harness and one privacy/permission authority. Existing caseentity aliases and verified local selectors already provide a starting point; do not build a global identity map or a second funds runtime. Follow the existing [case-provider structured-output requirements](../../../openspec/specs/case-provider-structured-output/spec.md): local real-data presentation, provider-safe input, public durable history and evidence receipts have distinct boundaries. A UI full/masked choice must not authorize new provider disclosure.

Next, extend tests at existing owners to cover case/epoch isolation, stable entity aliases, exact amounts/units/direction/time scope, omission/coverage markers, counterevidence, and UI-display/provider-request independence. Compute joins and amounts locally; send approved facts and evidence references rather than whole ledgers. Local re-retrieval must restore missing context under fresh authority. Schema validity does not prove truth or authorize disclosure. No real model request, token-saving percentage, accuracy improvement or full benchmark execution is claimed this round.

External mechanisms reviewed, not copied:
- [Codex App Server](https://openai.com/index/unlocking-the-codex-harness/): shared harness, typed lifecycle and real completion/approval events; no replacement of the Go runtime.
- [Anthropic context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents): bounded, high-signal context and just-in-time retrieval; never compress raw private data with an unapproved external service.
- [OpenCode V2 permissions](https://opencode.ai/v2/docs/permissions): action/resource policy clarity; Analytix delegation must remain within parent and case authority.
- [DeepSeek Harness preview](https://deepseek.com/harness/en/): plugin lifecycle and cleanup; do not make protected authority replaceable or copy raw reasoning/tool logs into durable history.
- [LLMLingua-2](https://aclanthology.org/2024.findings-acl.57/) and [Lost in the Middle](https://aclanthology.org/2024.tacl-1.9/): test compression and evidence-position effects separately from privacy; not zero-loss guarantees.
- [PrivacyLens](https://arxiv.org/abs/2409.00138), [inference privacy](https://arxiv.org/abs/2310.07298), [AgentDojo](https://arxiv.org/abs/2406.13352): assess recipients/purpose, linkability and untrusted-tool attacks together with legitimate task completion.
- [Anonymization performance preprint v2](https://arxiv.org/html/2609.11335v2): Watch only; task-dependent results and prose/table inconsistencies prevent treating it as proof of Analytix superiority.

## Exact continuation order

1. Fresh-read PR #28 and its head ref, current checks and applicable guides. Reuse this construction line; preserve unknown concurrent edits.
2. Validate these Go changes in required module mode, including affected provider/event callers. Keep trusted-host positive paths and PII negative paths together.
3. In a normal environment with real locked dependencies, review the still-pending Round2 hunks against fresh HEAD; run full plugin golden/schema/CLI tests before integrating. Do not merge reconstructed local history.
4. Inspect exact-candidate CI and CodeQL/review details. The earlier Rust/DuckDB failure is a hypothesis-driven diagnostic task, not solved by this Go change; no skipping or severity downgrades.
5. Implement the smallest existing-owner privacy/context vertical test journey described above. Only then expand to model/token ablations with explicitly authorized synthetic data and provider access.
6. Continue the accepted Canvas/DOCX journey and native/install acceptance through normal protected setup. No Keychain bypass, old-secret reuse, automatic merge, tag or release.

Read [the product checkpoint](2026-09-16-pr28-continuation.md) for wider product scope and [the governance supplement](2026-09-17-knowledge-acceptance.md) for original archives. GitHub is authoritative; the same existing Notion product item is the mirror. Formal reports/evidence go to Drive with checksums and readback. Post-commit remote HEAD and sync results belong in PR/Notion/report, not a self-referential commit loop.
