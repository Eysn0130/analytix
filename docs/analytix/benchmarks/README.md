# analytix benchmark and scorecard workspace

Status: Reference index for benchmark definitions and dated result ledgers.
Source of truth for a current score: a pinned harness/input plus a fresh run
against the identified analytix and upstream commits.

This directory stores the repeatable evidence used to prove analytix keeps
getting stronger as it absorbs Kun and Reasonix upgrades.

Normative method and document governance:

```text
docs/analytix/README.md
docs/analytix/specs/09-agent-quality-product-benchmark.md
```

## Files

| File | Purpose |
| --- | --- |
| `upstream-scorecard.md` | Tracks comparative scores for Kun, Reasonix, and analytix target surfaces. |
| `benchmark-scenarios.md` | Defines product, agent, runtime, provider, thread, desktop, and safety scenarios. |
| `quality-gates.md` | Defines when a sync batch, Go runtime stage, or release can claim completion. |
| `streaming-ui-guardrails.md` | Defines smooth streaming hot-path boundaries, UI gates, and proxy/API buffering diagnosis. |

## Operating Rule

Conformance proves compatibility. Benchmarks prove strength.

Do not claim analytix is stronger than Kun or Reasonix unless the scorecard and
evidence support that claim. A historical row or a score of `5` does not stay
current after the code, harness, upstream baseline, or environment changes.
