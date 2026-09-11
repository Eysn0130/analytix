# D-0251 Live Evidence Report

Generated at: 2026-06-24T17:51:00.863Z
Source commit: 69607b035a1000a24173abfbff2ad5b5c5e10846
Status: live_blocked
Default backend ready: true
Go default candidate allowed: true
TypeScript default-path fallback retained: false
TypeScript rollback override retained: false
Evidence digest algorithm: sha256:canonical-json-v1

Current-state note (2026-06-25): this generated report is post-cutover live
validation evidence only. Go runtime default delivery is active; TypeScript is
`ANALYTIX_RUNTIME_BACKEND=typescript` is retired and rejected by the current adapter. The
`live_blocked` rows below block live provider/MCP/packaged/operator claims and
future retirement authorization, not default Go startup.

## Components

- Provider matrix: live_blocked
- MCP execution: live_blocked
- Packaged desktop QA: live_blocked
- Operator gate: live_blocked

## Missing External Inputs

- provider:deepseek: skipped - missing env-gated credential/baseUrl/model; skipped without counting as pass
- provider:openai-compatible: skipped - missing env-gated credential/baseUrl/model; skipped without counting as pass
- provider:anthropic-compatible: skipped - missing env-gated credential/baseUrl/model; skipped without counting as pass
- provider:custom-endpoint: skipped - missing env-gated credential/baseUrl/model; skipped without counting as pass
- mcp:credentialed-execution: live_blocked - missing ANALYTIX_D0251_MCP_COMMAND/ANALYTIX_D0243_MCP_COMMAND or MCP URL
- packaged:packaged-app-startup: skipped - set ANALYTIX_D0251_PACKAGED_APP_PATH and ANALYTIX_D0251_PACKAGED_LAUNCH_COMMAND to run packaged launch QA
- packaged:health: skipped - set ANALYTIX_D0251_RUNTIME_URL to run live packaged runtime QA
- packaged:runtime-info: skipped - set ANALYTIX_D0251_RUNTIME_URL to run live packaged runtime QA
- packaged:thread-list: skipped - set ANALYTIX_D0251_RUNTIME_URL to run live packaged runtime QA
- packaged:turn-create: skipped - set ANALYTIX_D0251_RUNTIME_URL to run live packaged runtime QA
- packaged:sse-replay: skipped - set ANALYTIX_D0251_RUNTIME_URL to run live packaged runtime QA
- packaged:retired-backend-diagnostic: skipped - set ANALYTIX_D0251_ROLLBACK_PROOF_JSON after verifying `ANALYTIX_RUNTIME_BACKEND=typescript` returns retired_backend and does not start TS runtime
- packaged:proof:status: live_blocked - missing packaged QA proof field: status
- packaged:proof:passed: live_blocked - missing packaged QA proof field: passed
- packaged:proof:checks:packaged-app-startup:status: live_blocked - missing packaged QA proof field: checks:packaged-app-startup:status
- packaged:proof:checks:health:status: live_blocked - missing packaged QA proof field: checks:health:status
- packaged:proof:checks:runtime-info:status: live_blocked - missing packaged QA proof field: checks:runtime-info:status
- packaged:proof:checks:thread-list:status: live_blocked - missing packaged QA proof field: checks:thread-list:status
- packaged:proof:checks:turn-create:status: live_blocked - missing packaged QA proof field: checks:turn-create:status
- packaged:proof:checks:sse-replay:status: live_blocked - missing packaged QA proof field: checks:sse-replay:status
- packaged:proof:checks:retired-backend-diagnostic:status: live_blocked - missing packaged QA proof field: checks:retired-backend-diagnostic:status
- packaged:proof:startup.appPathExists: live_blocked - missing packaged QA proof field: startup.appPathExists
- packaged:proof:startup.launchCommandConfigured: live_blocked - missing packaged QA proof field: startup.launchCommandConfigured
- packaged:proof:startup.launchCommandReferencesAppPath: live_blocked - missing packaged QA proof field: startup.launchCommandReferencesAppPath
- packaged:proof:startup.exitCode: live_blocked - missing packaged QA proof field: startup.exitCode
- packaged:proof:startup.launchCommandHash: live_blocked - missing packaged QA proof field: startup.launchCommandHash
- packaged:proof:runtime.runtimeUrl: live_blocked - missing packaged QA proof field: runtime.runtimeUrl
- packaged:proof:runtime.runtimeTokenConfigured: live_blocked - missing packaged QA proof field: runtime.runtimeTokenConfigured
- packaged:proof:runtime.healthOK: live_blocked - missing packaged QA proof field: runtime.healthOK
- packaged:proof:runtime.runtimeInfo.candidateContract: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.candidateContract
- packaged:proof:runtime.runtimeInfo.hostLocalhost: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.hostLocalhost
- packaged:proof:runtime.runtimeInfo.dataDirConfigured: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.dataDirConfigured
- packaged:proof:runtime.runtimeInfo.dataDirHash: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.dataDirHash
- packaged:proof:runtime.runtimeInfo.mcpAvailable: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.mcpAvailable
- packaged:proof:runtime.runtimeInfo.subagentsAvailable: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.subagentsAvailable
- packaged:proof:runtime.runtimeInfo.reasonixUpstreamAbsorptionPresent: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.reasonixUpstreamAbsorptionPresent
- packaged:proof:runtime.runtimeInfo.reasonixCapabilityMatrixGreen: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.reasonixCapabilityMatrixGreen
- packaged:proof:runtime.runtimeInfo.kunAnalytixBaselineRetained: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.kunAnalytixBaselineRetained
- packaged:proof:runtime.runtimeInfo.reasonixSuperiorityCodeStageClosed: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.reasonixSuperiorityCodeStageClosed
- packaged:proof:runtime.runtimeInfo.defaultBackendReadinessBound: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.defaultBackendReadinessBound
- packaged:proof:runtime.runtimeInfo.readinessSemanticsBound: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.readinessSemanticsBound
- packaged:proof:runtime.runtimeInfo.rawValueRecorded:false: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.rawValueRecorded:false
- packaged:proof:runtime.runtimeInfo.infoShapeDigest: live_blocked - missing packaged QA proof field: runtime.runtimeInfo.infoShapeDigest
- packaged:proof:runtime.threadIdHash: live_blocked - missing packaged QA proof field: runtime.threadIdHash
- packaged:proof:runtime.turnIdHash: live_blocked - missing packaged QA proof field: runtime.turnIdHash
- packaged:proof:rollbackProof.activeBackendAfterRollback: live_blocked - missing packaged QA proof field: rollbackProof.activeBackendAfterRollback
- packaged:proof:rollbackProof.candidateStopped: live_blocked - missing packaged QA proof field: rollbackProof.candidateStopped
- packaged:proof:rollbackProof.retiredBackendRejected: live_blocked - missing packaged QA proof field: rollbackProof.retiredBackendRejected
- packaged:proof:rollbackProof.runtimeHealthAfterRollback: live_blocked - missing packaged QA proof field: rollbackProof.runtimeHealthAfterRollback
- packaged:proof:rollbackProof.candidateLaunchCommandHash: live_blocked - missing packaged QA proof field: rollbackProof.candidateLaunchCommandHash
- packaged:proof:rollbackProof.preRollbackRuntimeUrl:match: live_blocked - missing packaged QA proof field: rollbackProof.preRollbackRuntimeUrl:match
- packaged:proof:rollbackProof.postRollbackRuntimeUrl: live_blocked - missing packaged QA proof field: rollbackProof.postRollbackRuntimeUrl
- packaged:proof:rollbackProof.rollbackVerifiedAt: live_blocked - missing packaged QA proof field: rollbackProof.rollbackVerifiedAt
- operator:gate: live_blocked - provider matrix evidence is not passed
- operator:gate: live_blocked - MCP execution evidence is not passed
- operator:gate: live_blocked - packaged desktop QA evidence is not passed
- operator:gate: live_blocked - ANALYTIX_GO_RUNTIME_G6_READY is not set to 1
- operator:gate: live_blocked - operator post-cutover live-validation approval env is not set

## Next Evidence Commands

- `npm run runtime:go:d0251-live-evidence -- --json`
- `npm run runtime:go:preflight -- --json --gate`
- `ANALYTIX_GO_RUNTIME_G6_READY=1 ANALYTIX_D0252_OPERATOR_APPROVES_GO_DEFAULT_CANDIDATE=1 npm run runtime:go:d0252-candidate-report -- --gate --json`

This report is evidence-only and does not switch the default runtime.
