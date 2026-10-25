# OpenAI Apps SDK UI selected renderer icons

Status: Reference / bounded source-admission record, reviewed 2026-10-10.
Scope: the existing 139 selected vector modules and Analytix-owned renderer
adapter. No SDK runtime/dependency, unrelated brand/data asset or current-web
pixel identity is admitted.

<!-- analytix-upstream-review-v1 {"schemaVersion":1,"sourceId":"openai/apps-sdk-ui","reviewedCommit":"0f00143c7a639906f1621fe58e1b6be7b5bea46d","parity":"not-proven","capabilityBenchmarkV1":null} -->

## Fixed source and admission

Official repository: `https://github.com/openai/apps-sdk-ui.git`; commit
`0f00143c7a639906f1621fe58e1b6be7b5bea46d`, tree
`5c2aabc9e419aa1c55d6ebb8d219e19287bbb494`.
Root package `@openai/apps-sdk-ui@0.2.2` declares MIT. Root `LICENSE` blob
`b8ac8dc0f960bbe0b0cc90cd06d58b672151773f`, SHA-256
`f26a46776c890b0d7b45d042cb8a78fdca9c4742a5c7fd97f0f53767411b1cc6`,
1054 bytes; Copyright 2025 OpenAI. The exact full license remains in
[renderer LICENSE](../../../src/renderer/src/design/openai-icons/LICENSE) and
[THIRD_PARTY_NOTICES.md](../../../THIRD_PARTY_NOTICES.md).

Reuse mode: byte-identical vendor. The [existing approval and object table](code-reuse-provenance.md#openai-apps-sdk-ui--complete-semantic-renderer-icon-selection-2026-10-03)
and [renderer provenance](../../../src/renderer/src/design/openai-icons/provenance.json)
remain the file-level map; this ledger does not duplicate its 139 rows.
Normal official Git retrieval and two independent read-only reviews match all
139 source path/blob/SHA-256 tuples (168,494 bytes) to the actual destination
bytes and candidate `210039729892bf80fb8e0e9a7c909b70266ec6bc` tracked objects.
The selected paths and their ancestors have only the root license, no nested
LICENSE/NOTICE exception or gitlink, no linked/external asset, and only React
`SVGProps` type imports. The full license matches the retained NOTICE bytes.
Disposition: `approved` for these mapped objects and the existing bounded
adapter; no new vector or consumer is added by this registration.

## Evidence and limits

The task-owned source root contains an actual `apps-sdk-ui/.git` checkout at
that exact pin. It is detached (`branch` is empty), with official origin and
no upstream tracking; no latest-branch or synchronization PASS is claimed.
The source audit supports an explicit `ANALYTIX_UPSTREAM_ROOT` pointing at
that verified source root. Default-root availability is separate. With only
this checkout, other registered directories are unavailable and the overall
research projection is UNVERIFIED; Apps tracking/synchronization remain
freshness problems. No remote ref or tracking configuration is manufactured.

The license catalogue admits only the exact verified content hash; altered
bytes and marker/source/pin mismatch remain rejected. The new ledger also
requires a tracked HEAD blob before artifact admission can pass. Pre-commit
source/marker correctness does not bypass that binding. Direct contract and
JSON/link/diff checks are the permitted verification scope; no Go, build,
native, Provider, package or distribution acceptance follows. Parity remains
`not-proven` with a null benchmark. Kun's actual source inventory/license
blocker remains independent of Analytix's current product direction.
