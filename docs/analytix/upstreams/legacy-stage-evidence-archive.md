# Legacy Stage Evidence Archive

Status: archive-only.

This document classifies older stage-numbered evidence files that remain in the
repository for audit traceability.

These files are not production runtime entrypoints.
These files are not product capabilities.
These files are not current Go-default authorization gates.
These files are not evidence that TypeScript is the default runtime.

Current formal replacements:

- Go runtime validation command: `scripts/runtime-go-validation-command.mjs`
- Go runtime preflight: `scripts/runtime-go-preflight.mjs`
- Go runtime product regression: `scripts/runtime-go-product-regression.mjs`
- Go runtime speed/cache gate: `scripts/runtime-go-speed-cache-gate.mjs`
- Go runtime cutover report: `scripts/runtime-go-cutover-report.mjs`
- Go runtime engine absorption report: `scripts/runtime-go-engine-absorption-report.mjs`
- Go runtime rollback-retirement report: `scripts/runtime-go-rollback-retirement-report.mjs`
- Go runtime live evidence: `docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json`
- Reasonix integration topology: `docs/analytix/upstreams/reasonix-integration-topology.md`
- Go runtime retirement checklist: `docs/analytix/upstreams/go-runtime-retirement-checklist.md`
- Go runtime candidate summary: `docs/analytix/upstreams/runtime-go-default-readiness/default-readiness-summary.md`
- Runtime retirement summaries: `docs/analytix/upstreams/runtime-go-retirement/`

Archive-only stage evidence retained for traceability:

| Archived path | Archive reason | Current replacement |
| --- | --- | --- |
| `docs/analytix/upstreams/d0248-go-runtime-g6-preflight-report.json` | historical deterministic preflight report | `scripts/runtime-go-preflight.mjs` and `runtime-go-live-evidence/live-evidence-report.json` |
| `docs/analytix/upstreams/d0248-go-runtime-g6-preflight-summary.md` | historical deterministic preflight summary | `runtime-go-preflight` JSON output |
| `docs/analytix/upstreams/d0248-go-runtime-readiness-gate-report.json` | historical readiness gate report | `scripts/runtime-go-preflight.mjs` |
| `docs/analytix/upstreams/d0248-go-runtime-readiness-gate-summary.md` | historical readiness gate summary | `scripts/runtime-go-preflight.mjs` |
| `docs/analytix/upstreams/d0248-packaged-go-runtime-qa-report.json` | historical packaged QA input | `scripts/runtime-go-packaged-qa.mjs` and live-evidence component digests |
| `docs/analytix/upstreams/d0249-d0250-go-runtime-retirement-checklist.md` | historical checklist snapshot | `docs/analytix/upstreams/go-runtime-retirement-checklist.md` |
| `docs/analytix/upstreams/d0249-reasonix-integration-topology.md` | historical topology snapshot | `docs/analytix/upstreams/reasonix-integration-topology.md` |
| `docs/analytix/upstreams/d0250b-go-runtime-cutover-report.json` | historical cutover intake report | `scripts/runtime-go-cutover-report.mjs` |
| `docs/analytix/upstreams/d0250b-go-runtime-cutover-summary.md` | historical cutover intake summary | `scripts/runtime-go-cutover-report.mjs` |
| `docs/analytix/upstreams/d0250c-go-runtime-code-stage-report.json` | historical code-stage report | `scripts/runtime-go-engine-absorption-report.mjs` |
| `docs/analytix/upstreams/d0250c-go-runtime-code-stage-summary.md` | historical code-stage summary | `scripts/runtime-go-engine-absorption-report.mjs` |
| `docs/analytix/upstreams/d0250c-reasonix-superiority-matrix.json` | historical matrix artifact | `packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go` |
| `docs/analytix/upstreams/d0251-live/` | historical live-evidence bundle | `docs/analytix/upstreams/runtime-go-live-evidence/` |
| `docs/analytix/upstreams/d0252-candidate/` | historical candidate bundle | `docs/analytix/upstreams/runtime-go-default-readiness/` |
| `docs/analytix/upstreams/d0253-retirement/` | historical fallback-retirement bundle | `docs/analytix/upstreams/runtime-go-retirement/` |

Operational rule:

- Product/runtime code, package scripts, public runtime metadata, and renderer UI
  must use the current formal replacements above.
- Historical paths may be cited only with archive wording.
- Historical paths must not be used as current production gates, product
  capabilities, package scripts, runtime routes, or renderer-visible surfaces.
