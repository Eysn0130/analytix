# PR28 Round2 integration and interruption recovery

Status: Operational snapshot; not product or release acceptance.
Source of truth: fresh GitHub branch / PR / exact checks and applicable source.
Supersedes only the prior Round2 NOT_INTEGRATED status; privacy and historical evidence stay intact.

## Recovery anchor

- Branch: `codex/workbench-product-delivery-20260914`; PR #28 remains Draft, not merged.
- Start HEAD: `7465751b00532b629fb951896bbffa65d2ba7d9c`; base main: `ce96cf12581acfa0e19fae7c6aa9c709371012c8`.
- Implementation HEAD: `d7f542abbe15633376eeac3018144a862c85eeac`; previous batch: `21e6385d33c15b7fa9ac5b772204bdc5d260dc47`.
- This document is a subsequent documentation-only checkpoint. Resolve its containing commit and fresh branch HEAD from GitHub; do not infer current checks from its filename.
- Live interruption anchor: PR28 comment `5708659265`, updated at each meaningful batch with Branch / HEAD / Completed / Tests / Pending / Blockers / Next Action.
- Worktree: task-owned Linux reconstruction, locally committed and clean at the code checkpoint. Its local parent history was never pushed. Remote commits use the actual current remote parent and non-force updates.
- No owner Mac, Keychain, real profile, credential, case data or real Provider was accessed. No merge, release, tag or safety-gate waiver.

## Completed and code counts

The retained Round2 patch is now integrated, not merely archived: 17 logical files (one rename), +253/-78 against Start HEAD. This contains the previously delivered patch (+245/-76) and new Skill corrections (+8/-2). Do not count inherited lines as newly authored twice. Handover/index documents are reported separately.

- Required Ajv initialization fails before render/validate/inspect can produce output; help remains available. Validator dispatch uses a closed Map.
- Canvas data validation rejects inherited serialization hooks, sparse/extra array fields and noncanonical stylesheet-tag variants. It does not become an arbitrary SVG sanitizer or replace Core authority.
- Golden/layout/schema and missing-validator tests run in an additive step of the existing Source baseline job using the checked-in lockfile. No job, permission, timeout or dependency changed.
- Removed vacuous/degraded-validation guidance, preserving valid renderer behavior.
- Corrected Skill instructions to preserve complete facts and relations rather than delete edges for visual neatness, and removed a contradictory handwritten-SVG fallback. Standalone HTML and protected-local Canvas remain distinct outputs.
- Newer Go privacy-projection and knowledge-governance commits are preserved; their historical tests are not attributed to this round.

## Source identity and tests

Remote `plugins` tree `b86158c99f41b1508769bb4f7104f25fd899d727` and `.github` tree `3ae86d251f036d193c3fd21bf63c062660dddd95` match the locally tested candidate byte-for-byte, including modes. Changed subtrees were unchanged by the intervening remote privacy/docs commits before integration. The source archive's 6,341 blobs were checked before reconstruction.

Fresh local results: 48/48 Canvas/geometry/output/validator-unavailable tests, 8/8 existing CI-gate tests, 9/9 MJS syntax checks, whitespace check. YAML was compared against the baseline after removing exactly the added step; all other workflow structure is unchanged. This is not actionlint or full installed-plugin acceptance.

Baseline `7465751b` Development run `35177220324` completed successfully. Its separate CodeQL check `105061583808` still reports 37 high alerts. The old Rust failure is retained as historical non-deterministic evidence, not a current reproduced root cause.

Implementation `d7f542ab` Development run `35183963159` was queued at this checkpoint; full installed Ajv positives, application tests and all final mandatory checks remain pending. A CodeQL lookup for this implementation returned no check yet. Freshly query again; do not copy baseline results to the new candidate.

No local dependency install succeeded: terminal GitHub/npm DNS failed, and the approved download route did not provide the package. Tests with deliberately unavailable Ajv are negative evidence only, not a replacement validator. No GUI, IME, Office round-trip, installed-app or model-quality claim is made.

## Research decisions, not additional frameworks

- Codex App Server: reuse stable thread/turn/item boundaries and truthful UI events; do not add a second loop. Reference: https://openai.com/index/unlocking-the-codex-harness/
- Claude Code Skills: load procedures on use and distinguish invocation from permission. Reference: https://code.claude.com/docs/en/skills
- OpenCode permissions: action/resource decisions, not plugin-presence authorization; V1 and V2 configurations differ. Reference: https://opencode.ai/v2/docs/permissions
- DeepSeek Harness: fixed architecture revision `b150a551b8d465e31e418e1b2eaf5e79bbb7d28e`; borrow scoped/reversible contributions, not a replaceable privacy/permission core. No upstream implementation was copied.
- CaMeL (`arXiv:2503.18813v2`): control/data-flow separation and capability checks are useful design inputs, not proof that Analytix implements CaMeL or its guarantees. This round keeps input data inert and directs Skills through existing authority; it does not add the paper's multi-model machinery.
- AgentDojo (`arXiv:2406.13352`) and Lost in the Middle (`TACL 2024.tacl-1.9`): existing paired utility/safety and evidence-position checks are now committed. They test local facts/escaping, not LLM injection resistance, recall, token savings or reasoning accuracy.

## Pending / blockers / next action

1. Read current branch and exact CI. Obtain the new locked-plugin step's complete output: golden examples, positive layout, schema and missing-validator checks must all pass. Reproduce any failure and fix the actual cause; do not downgrade to schema-optional mode or retry until green.
2. Acquire the 37 alerts' exact traces through an authorized available route. Annotations fetch was rejected by this connector; review threads expose two unresolved filestore path findings (145/146), not all alert data. No alert was dismissed or marked fixed here.
3. Continue the agreed Canvas/DOCX selection/proposal product path, not another engine or renderer. Current WorkspaceToolSelector still has no Canvas entry and there is no completed renderer Canvas panel; a Main controller is not a GUI delivery. Preserve project-authorized object reuse but never reuse another thread's handle/draft/temporary permission.
4. Review original generator/installed-plugin composition and source/packaged resources in an independent qualified environment. A local or deterministic test is not native or installed acceptance. Do not revisit the owner's Mac.
5. Keep the current Task/PR as the sole integration line. Reread HEAD before each non-force update; preserve external changes and stop/reconcile on overlap. Update this index/PR anchor when durable facts change, not per file read.

Notion mirror: PENDING for this checkpoint; GitHub is sufficient to recover. No new mirror, global ledger or automatic background job was created. Prior Notion/Drive records are historical until explicitly synchronized.
