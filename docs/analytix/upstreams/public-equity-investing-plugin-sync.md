# Public Equity Investing installed-plugin absorption ledger

Status: Pinned white-box audit complete; the installed focused regression
sample is not green, and implementation/benchmark remain open.

## Current installed source

- Package: `public-equity-investing` `0.1.31`
- Local root:
  `/Users/sun/.codex/plugins/cache/openai-curated-remote/public-equity-investing/0.1.31`
- Manifest SHA-256:
  `e7936773d3dc8aacf53ec38f60e627380521da8528ed1c62bb2c5ec3d12cf96d`
- Router SHA-256:
  `63a63b89e5a65de1f4b5ef23f8a60ad01655dfbbb43c07018f4da29a1d32943d`
- License posture: manifest `Proprietary`; no package-root LICENSE/NOTICE was
  found. `skills/earnings-preview/LICENSE.txt` is separately proprietary and
  hashes to
  `10d31cfac9580486d77c1ff0d9fdc30e132a9bb9ad8d2c3c863bb8f2fb719d8e`.
  Goal authorization must be preserved as exact-file release provenance.

## As-built review

The strongest capability is analytical state classification and disciplined
product routing:

- one implicit router selects a focused owner while financial-source-of-truth,
  cleaning, sector overlays, rendering, and style support stay internal;
- workflows distinguish source-derived fact, provider value, deterministic
  derived value, management statement, assumption, inference, stale,
  contradicted, missing, and unknown states;
- public price, estimates, catalysts, and event timing carry as-of/freshness
  posture, and connector declarations are not treated as current access;
- outputs explicitly separate what is priced in, variant perception,
  disconfirmers, conflicts, action thresholds, and evidence needed to upgrade;
- hero/support artifact hierarchy and deck/report QC prevent raw JSON/CSV/logs
  from masquerading as the client-facing deliverable.

These are primarily instruction and local-validator contracts. A model label
such as `fact`, `contradicted`, or `source-derived` is not host authority. The
QC scripts can validate packaging and repeated-number consistency without
proving same-case receipt membership or semantic citation support. A complete
HTML/DOCX/XLSX artifact is not a PublicationReceipt.

Fresh focused validation used stdlib tests with bytecode writes disabled:

```text
PYTHONDONTWRITEBYTECODE=1 python -m unittest \
  tests.test_core_pm_skill_sharpening \
  tests.test_pm_judgment_language \
  tests.test_stale_public_markets_language \
  tests.test_deliverable_framework \
  tests.test_artifact_packaging_and_qc
78 run: 48 passed, 30 failed
```

The failures are present package/test drift, concentrated in older exact-text
contracts for deliverable intake, router saved-context/path wording, and
related workflow language. Passing artifact tests use temporary directories;
they do not establish factual publication. The installed package therefore
cannot be treated as a fully green or evidence-authoritative baseline.

## Admission decisions

| Capability | Decision | Analytix landing | Deterministic proof required |
| --- | --- | --- | --- |
| Fact/assumption/inference/conflict/as-of taxonomy | adapt as typed values | ClaimRecord support, wording, counterevidence, and review fields | Model labels cannot upgrade state; transitions require host verifier receipts. |
| Priced-in/disconfirmers/action thresholds | adapt | Investigative-lead and analysis-inference renderers | Wording is bounded by exact scope and counterevidence. |
| Router/internal support separation | adapt | Funds owner-skill bundles | Only the owner is user-facing; support remains non-advertised. |
| Citation readiness and artifact QC | adapt | Claim validation and render inspection | Fake/mismatched/stale citations and unsupported cells block publication. |
| Skill-declared fact status | reject as authority | Host registry and source capability matrix | Same thread/turn/case/epoch/snapshot and exact field support are mandatory. |
| Artifact completeness as truth | reject | PublicationReceipt registry | Renderable but unsupported artifacts remain staging-only. |

No row is reached or exceeded from this audit. OpenSpec tasks 8.16, 8.19,
8.20, and the P2 claim/report tasks remain open.
