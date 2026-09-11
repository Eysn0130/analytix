# analytix quality gates

Status: Normative evidence discipline with historical Go cutover-stage wording.
Current as of: 2026-07-10 for this lifecycle note only.

Go is already the only production runtime. “Before Go becomes default” and
rollback requirements below describe the completed cutover stages; they must
not recreate a TypeScript fallback. After an RC source freeze,
`npm run runtime:go:rc-control-plane -- --json` may establish only the
source-bound validation control plane. Current Go product release-path changes
use `npm run runtime:go:release-gate`, which exits successfully only when its
final JSON has `passed: true`, plus the scoped deterministic/live/platform
evidence required by the change.

These gates define when analytix can claim that an upstream absorption batch,
Go runtime stage, or release made the product stronger.

## Sync Batch Gate

A Kun or Reasonix sync batch may close only when:

```text
1. Sync ledger is updated.
2. Meaningful upstream changes are classified.
3. Conflicts have decisions.
4. Impacted benchmark dimensions are listed.
5. Baseline and post-change scores are recorded.
6. Evidence is linked or summarized.
7. Regressions are accepted explicitly or fixed.
8. git diff --check passes.
```

## Go Runtime Stage Gate

A Go runtime stage may close only when:

```text
1. Stage scope matches spec 08.
2. Required conformance fixtures pass.
3. Backend selection remains internal to Electron main/runtime supervisor.
4. preload and renderer APIs do not change for Go.
5. Runtime benchmark score is at least parity for required surfaces.
6. At least one intended engine dimension improves before Go becomes default.
7. Rollback is documented.
```

## Release Strength Claim Gate

A release may claim "stronger than Kun and Reasonix" only when:

```text
1. Product workflow score is stronger than Kun baseline for target surfaces.
2. Engine/runtime score is at least Reasonix parity for absorbed engine areas.
3. Thread smoothness score is stronger than both upstreams for desktop use.
4. Safety score has no known regression.
5. Desktop QA evidence exists for the real Electron app.
6. Scorecard and release notes do not overstate unverified areas.
```

## Streaming Smoothness Gate

Any change that touches chat streaming, side conversations, runtime SSE,
provider stream parsing, markdown streaming, virtualizer measurement, or scroll
anchoring may close only when:

```text
1. Local fake-provider UI gate passes.
2. Long reasoning and long final-answer scenario is included.
3. Provider write cadence is recorded alongside UI paint cadence.
4. Real provider/proxy complaints are investigated with the raw cadence probe.
5. A batched upstream/proxy is documented as provider cadence evidence, not
   described as a renderer regression unless UI metrics also fail.
6. New streamed content types add a benchmark or explicit QA scenario before
   smoothness is claimed.
```

## Blocking Conditions

Do not close if:

```text
scorecard is missing
sync ledger is missing
contract changed without conformance evidence
UI regressed while engine improved
engine improved while desktop workflow broke
safety or trace privacy weakened
Go backend requires renderer-specific branches
release claim exceeds evidence
streaming smoothness is claimed without UI cadence and provider cadence evidence
```
