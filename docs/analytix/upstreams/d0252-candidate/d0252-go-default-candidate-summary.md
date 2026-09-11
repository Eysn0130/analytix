# D-0252 Go Default Candidate Report

Generated at: 2026-06-24T17:51:04.868Z
Status: blocked
Go default candidate ready: false
TypeScript default-path fallback retained: false
TypeScript rollback override retained: false

Current-state note (2026-06-25): this generated report gates post-cutover
candidate/soak evidence, not the default startup path. Go runtime default
delivery is active through `go-runtime-default`; TypeScript is retained only as
`ANALYTIX_RUNTIME_BACKEND=typescript` retired-backend diagnostic. The blockers below
prevent D-0252 candidate claims until live/provider/MCP/packaged/operator
evidence passes; they do not roll the default runtime back to TypeScript.

## Blockers

- d0251-live-evidence: blocked - D-0251 aggregate is not candidate-ready; missing/invalid: status, passed, missingExternalInputs:58:provider:deepseek|provider:openai-compatible|provider:anthropic-compatible|provider:custom-endpoint|mcp:credentialed-execution|packaged:packaged-app-startup|packaged:health|packaged:runtime-info|...(+50), componentStatus.provider:live_blocked, componentStatus.mcp:live_blocked, componentStatus.packaged:live_blocked, componentStatus.operator:live_blocked
- strict-g6-readiness: blocked - strict G6 readiness missing/invalid: hardGate:provider-matrix-credentialed, hardGate:mcp-matrix-credentialed, hardGate:packaged-qa, hardGate:explicit-env-gate, check:provider-matrix-credentialed, check:mcp-matrix-credentialed, check:packaged-qa, check:explicit-env-gate
- d0250c-live-cutover-ready: blocked - D-0250C live cutover evidence missing/invalid: strictG6Passed, liveCutoverStatus, missingLiveEvidence:provider-matrix-credentialed|mcp-matrix-credentialed|packaged-qa|explicit-env-gate|credentialed-provider-matrix-json|credentialed-mcp-execution-json|packaged-desktop-qa-json|ANALYTIX_GO_RUNTIME_G6_READY=1|...(+1), d0250bMissingEvidence:provider-matrix-credentialed|mcp-matrix-credentialed|packaged-qa|explicit-env-gate, d0250cLiveEvidenceBlockers:credentialed-provider-matrix-json|credentialed-mcp-execution-json|packaged-desktop-qa-json|ANALYTIX_GO_RUNTIME_G6_READY=1|operator-gate-json
- provider-matrix-json: blocked - provider matrix must be passed D-0251 evidence
- mcp-execution-json: blocked - MCP execution must be passed D-0251 evidence
- packaged-qa-json: blocked - packaged QA must be passed D-0251 d0243-packaged-go-runtime-qa evidence
- operator-gate-json: blocked - operator gate missing/invalid for D-0250B/D-0251/D-0252 gate: status, passed, explicitEnvGate, credentialedEvidenceReviewed, goDefaultCandidateApproved
- operator-candidate-approval: blocked - D-0252 requires explicit operator candidate approval or a passed D-0251 operator gate

## Rollback

- Do not use ANALYTIX_RUNTIME_BACKEND=typescript for rollback; the current adapter returns a retired_backend diagnostic.
