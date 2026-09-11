# Investment Banking installed-plugin absorption ledger

Status: Pinned white-box audit complete; the installed focused regression
sample is not green, and implementation/benchmark remain open.

## Current installed source

- Package: `investment-banking` `0.1.29`
- Local root:
  `/Users/sun/.codex/plugins/cache/openai-curated-remote/investment-banking/0.1.29`
- Manifest SHA-256:
  `bbbbc9179ca0f7ab33aa72e0d9ba77b72ea6e2a589fdcf7f235f9182fbc9762c`
- Router SHA-256:
  `61af92a927494025125dafc5aa41eb49b0677173fe632e37bba5a74ea5cb6cd4`
- License posture: manifest `Proprietary`; no package-root LICENSE/NOTICE was
  found. Exact byte reuse requires the Goal authorization to be preserved as
  durable release provenance rather than inferred from package availability.

## As-built review

The plugin has useful product and artifact discipline:

- one implicit router chooses a focused owner and keeps evidence cleaning,
  provider guidance, dashboard rendering, and style adaptation internal;
- a configured connector is not treated as current availability. The selected
  workflow is instructed to use a smallest safe native read in the current run;
- source categories, `source_id`, as-of/freshness, source-of-truth ownership,
  conflicts, assumptions, and missing evidence are carried through handoffs;
- typed routing maps and handoff schemas preserve one hero artifact plus hidden
  support/audit artifacts;
- artifact manifests, model citation files, deterministic validators, and deck
  QC create a stronger client-ready packaging contract than an unstructured
  report fallback.

The authority remains skill/script-local. A string `source_id`, citation row,
manifest, successful native read, or QC status is not bound to an Analytix
thread/turn/case/epoch/snapshot receipt registry. The router may continue with
weaker user/public context when a source is missing, which is reasonable for
general banker work but cannot authorize high-risk case facts. Manifest/QC
creation also does not implement staging-to-claim-validation-to-atomic-publish
or `PublicationReceiptV1`.

Fresh focused validation used only stdlib tests with bytecode writes disabled:

```text
PYTHONDONTWRITEBYTECODE=1 python -m unittest \
  tests.test_plugin_routing_playbook \
  tests.test_banker_runtime_readiness \
  tests.test_artifact_manifest_policy \
  tests.test_model_citations_policy \
  tests.test_dashboard_citation_readiness_policy
50 run: 47 passed, 3 failed
```

The three failures are current package/test drift, not Analytix regressions:
two router tests expect older bundled-path/saved-context phrases, and one
manifest-policy test expects an older router phrase. They are retained as
upstream limitations; this package cannot be represented as a green baseline.

## Admission decisions

| Capability | Decision | Analytix landing | Deterministic proof required |
| --- | --- | --- | --- |
| Single router and focused owner | adapt | Funds top-level router with internal support hidden | Exactly one public router; hidden support cannot execute without a grant. |
| Current-run native read | adopt and strengthen | Go source live probe | Identity, connection epoch, case health, and snapshot equality are verified. |
| Source-of-truth/as-of/conflict taxonomy | adapt | Evidence/Claim source capability matrix | Each typed claim is supported only by an authorized source type and exact fields. |
| Typed handoff and artifact manifest | adapt | Closed report staging manifest | Unknown fields fail; hashes bind claims, snapshot, PII projection, and render output. |
| Client-ready QC | adapt | Render inspection before atomic publication | QC cannot upgrade evidence and publication still requires a host receipt. |
| Self-issued source/citation ids | reject as authority | Host Evidence Registry membership only | Fake and mismatched citation benchmarks fail closed. |

No row is reached or exceeded from this audit. OpenSpec tasks 8.15, 8.19,
8.20, and the P2 router/report tasks remain open.
