# Test Strategy

Status: Normative verification attachment for this change.

## Test Layers

1. Pure Go domain tests for canonical hashes, context binding, grants, receipts, claims, source capabilities, answer unions, publication values, and invariants.
2. Go app/adapter tests for source probes, MCP protocol, execution, terminal finalization, persistence/restart, SSE, provider history, compaction, jobs, approvals/input, and reports.
3. Funds plugin contract/smoke/oracle/red-team tests for schemas, cache isolation, citations, data/PII, SQL policy, report staging, and router/tool bundles.
4. Shared/main/preload/renderer contract tests for accepted events, no draft/reasoning, history, reports, exports, and case switching.
5. Provider-family fake/local contract tests plus credentialed/live gates when configured.
6. Real Electron UI validation and current-host packaging; supported CI/approved-host validation for other platforms.

Source-level funds contract tests SHALL inspect current plugin/runtime source and schemas only. They SHALL NOT silently skip checks for missing historical extracted desktop bundles. Renderer/preload/main behavior is verified from current source in the desktop contract layer, while packaged ASAR/binary behavior is verified from a freshly built artifact in the P4 package gate; neither layer may substitute an absent legacy `analytixagent/src/*/_asar` snapshot for current-product evidence.

## Required Incident And Hostile Cases

- No tool: model directly emits amount, account, MAC, family relationship, bid quote, and legal conclusion.
- No tool: model forges a plausible namespaced tool call and evidence id.
- MCP disconnected with cached catalog.
- Spoofed `analytix_funds` identity.
- Every advertised funds fact tool with missing, partial, expired, wrong-argument, wrong-tool, wrong-connection, or cross-project host authority; execution count must remain zero.
- Same case id with a different thread, turn, binding, epoch, or dataset snapshot attempting to read process-local workbench history.
- Every evidence/query/artifact identity uses a complete lowercase SHA-256; truncated digests are rejected as authority.
- HTTP/JSON-RPC success with semantic failure or MCP `isError`.
- MCP operation before initialize/initialized, duplicate initialize, ping in every live phase, malformed/duplicate-key/oversized frames, bounded request queue, cancellation, EOF drain, and close.
- Cancellation while a future Go-authorized DuckDB worker is active; the process tree terminates and writes zero workbench-history/artifact/report/evidence bytes. Under P0 quarantine the same scenario proves zero process starts.
- Empty result with incomplete and complete scope variants.
- Partial coverage for one company/account/month/source.
- Fake receipt id and real-but-mismatched receipt.
- Exact mismatches for amount, account, direction, date, subject, relationship, quote, and device id.
- Authorized full-account reply uses an unexpired same-context `PIIProjectionGrantV1` and copies the exact verified account field after the Final Evidence Gate; missing/stale/cross-case/mismatched grants block without returning a truncated answer, and full identifiers produce zero bytes in model input, ordinary SSE/log/history/compaction/report/export.
- Controlled full-PII access proves the exact verified account bytes reach only a synchronous trusted host sink after a durable one-use access receipt. Fake handle/principal, stale renderer/backend generation, missing commit/receipt, revoked PII authority, cross-case context, duplicate use slot, cancellation after reservation, and artifact/hash mismatch release zero bytes. Any partial/error/ambiguous post-sink result closes as `release_indeterminate` with the complete artifact length, and receipt/result/disposition records contain neither the account nor the raw opaque tokens.
- Electron controlled-sink protocol tests prove exact loopback-only origin validation, a distinct per-launch secret, reserved environment scrubbing, disabled redirects/proxies, strict duplicate/unknown-field rejection, bounded metadata/body lengths, exact frame parsing, an invocation-frozen installation public key, independent Ed25519 receipt/record-digest verification, HMAC acknowledgement binding, host-generation and renderer-navigation revocation, one-use slot consumption, no-follow/atomic export, protected display staging, and zero raw token/path/PII bytes in logs, ordinary runtime responses, preload, renderer state, or IPC results. Wrong secret, non-loopback origin, redirect, truncated/extra body, malformed signature, valid self-signature from a different key, mismatched receipt/hash/length/action/slot, stale generation, duplicate release, timeout, viewer error, connection abort, and host restart all fail closed; every post-call ambiguity settles as `release_indeterminate` and is never automatically retried.
- Case A query finishes after case B binding/epoch is accepted.
- Signed case-risk tail deletion, whole-thread deletion, complete old private-state restore, old-index/old-policy composite rollback, witness equivocation/replay/unavailability, and every policy/index/intent/witness/receipt/projection crash cut.
- Frozen Authority Advance V1 grammar/wire goldens; true V2 genesis/successor signature and digest domains; domain/store byte-bound equality; V1-terminal-to-V2 activation with a late V1 writer; indeterminate recovery that performs fresh Observe before replay; winner/loser committed-range and superseded settlement validation; mutation-id conflicts; and missing/corrupt candidate or settlement quarantine.
- Host-private authority storage root/shard/record/temp owner and ACL/DACL checks, hardlink/symlink/reparse/ADS rejection, inode/128-bit FileID name-swap and same-size in-place mutation, bounded inventory/aggregate bytes, stable multi-read/final-name verification, and independent Store/process no-replace races on every claimed platform.
- Semantic-startup V3 final/temp migration, retirement V1 authentication/domain separation, Unix convergence, Windows typed zero-write blockers, exact 32/24-lowercase-hex record temporary names, deterministic staged-directory create names, repeatable directory inventory, and later-parent/later-page/later-shard/later-owner corruption causing zero earlier cleanup. Cover all 11 recovery groups and 33 physical CAS roots, including attachment owner/use/upload partitions, controlled-artifact access, report artifacts, missing-root no-create behavior, case-thread temp rejection in ordinary snapshots, exact empty create residue before the journal, canonical orphan topology only after the journal, and recovery of an older authenticated journal whose exact remove operation accounts for its residue before owner recovery.
- Startup boundedness at 257 directory entries, 100,001 snapshot entries, depth 65, 64 MiB + 1 managed files, 8 MiB + 1 journals, 10,001 operations, cancellation within one 256-entry/1 MiB checkpoint, no `WalkDir`/`Glob`/`ReadDir(-1)`/whole-file copy, and no planning bytes under shared OS temp. Repeated Unix and Windows directory inventories use independently reopened handles rather than shared cursors. A deterministic CLI test cancels during preparation and proves zero listener activation/readiness; architecture tests enforce empty-create-residue -> authenticated journal -> orphan topology -> global owner recovery -> baseline order and require every runtime-composed CAS constructor package to register two-phase recovery. Native Windows tests additionally cover protected DACL/ADS/reparse/case aliases, recursive empty partial owners, final-window rename/replacement, delete-share guarding, handle closure, and restart convergence; cross-compilation alone is not a pass.
- Accepted-final terminal-prefix restart with duplicate, gapped, physically backward, or foreign-thread raw event records fails in the managed persistence snapshot before a public handler, provider call, terminal disposition, or event repair can exist. The event bytes and complete managed tree remain unchanged, while a positive contiguous compacted sequence base remains valid. Together with durable-prefix recovery, private-registry corruption, signed case compaction, and accepted-final replay tamper tests, this closes task 5.10 only; semantic-startup planning and journal migration remain separately open in 5.10a and 5.10b.
- Case-risk steer or context-changing attachment/file reference targets an already frozen general turn; provider/effect invocation remains zero and a new V2 turn is required.
- Attachment upload with no host thread, caller-widened/global/unknown scope, cross-thread owner use, cross-case binding, owner-id alias, missing id, modified blob, byte-size mismatch, modified provider projection, or legacy ownerless metadata fails closed. Vision and primary provider call counts remain zero, private text/fallback/raw bytes do not enter SSE/history, and terminal case handling uses the Final Evidence Gate.
- Equal blob bytes uploaded under two owners/cases produce distinct owner ids and never merge path, extraction, fallback, or authority metadata. The later `AttachmentUseReceiptV1` matrix covers exact turn/case/epoch/snapshot/context binding, case-switch races, every private-CAS crash cut, restart-indeterminate use, and idempotent recovery.
- Upload transaction tests prove intent-before-files, exact file/metadata hashes, live binding recheck, atomic open-intent owner commit, disposition-before-201, cancellation settlement through a bounded non-caller-cancelled context, exact complete/current restart promotion, owner-before-response settlement, partial/corrupt/stale quarantine, quarantine-to-owner rejection, repeated-start convergence, and complete plan revalidation before the first recovery mutation. The remaining platform-specific file-write and private-CAS fault hooks stay required by task 3.18a2.
- Attachment upload, metadata, and content require exact explicit thread/workspace scope at Renderer, main IPC, Go HTTP, and owner authorization boundaries. Unknown fields or query keys fail closed. Extracted document text, preview bytes, blob hashes, caller paths, fallbacks, and owner material are rejected from public attachment responses and remain absent from optimistic blocks, queues, user messages, SSE, history, compaction, logs, and ordinary export.
- Approval then case switch; restart with stale pending grant.
- Restart with one or many dangling host/provider grants; reconciliation preflights the complete set before mutation, closes each exact grant before the restart boundary, emits no tool event, rejects duplicate/colliding call identities with zero partial writes, and is idempotent on repeated recovery.
- Provider-originated local, MCP, and read-only batch execution has one mechanically inventoried dispatcher. An AST test discovers every production `execute*` function that accepts a pending provider call and rejects any reference outside the single effect-authority gateway and its exact loop/approval-resume/required-plan owners. A dynamic hostile-scope test proves current context/catalog/grant rejection occurs before even an injected host override can escape. Both tests run in ordinary and `analytix_prod` builds; the dispatcher test also runs under `-race`.
- Every durable pause/effect record carries private host authority for both the frozen context and its exact grant set: approval and user-input V3 receipts persist the full signed `TurnSecurityContext` plus `ExecutionGrant`; task jobs persist `SecurityBindingV2` with parent context digest and grant id; tool batches, provider continuations, and report staging persist signed pending-work context bindings plus ordered grant members. Restart tests reopen the real private CAS, revalidate signatures/current authority, reject tampering/staleness, and never reconstruct missing authority from public thread/events. Public approval/input events retain only an opaque receipt id; a legacy job without `SecurityBindingV2` remains audit-readable but cannot execute, resume, or publish. This closes persistence task 4.7 only; production report-publication composition remains gated by 7.1.
- Native data-engine health uses one current-turn host grant under the same effect gate, never runs twice, never exposes its raw registry digest/error/process metadata, returns ready only after durable settlement readback, and remains absent from case public history, provider history, SSE, and sidecars.
- Prompt injection in CSV, database fields, MCP output, and background job output.
- Hostile tool result with account/MAC/PII, arbitrary root fields and result code, raw text, citations, `dataUrl`, remote/localhost preview URL, local/absolute path, generated files, diagnostics, and prompt injection. Disk/thread/event/provider-history/compaction plus HTTP/SSE/main/preload/renderer/UI/action/export surfaces contain zero sentinel bytes and accept no evidence/artifact authority.
- Tool-result case status with mismatched outer tool/call/context/epoch/grant/lifecycle, forged JSON-RPC code/class, or unknown schema/code; every replay boundary fails closed. Tool-originated media requires a current owner/use receipt and effect lease, rejects cross-case/stale/restart use, and recovers every crash cut idempotently.
- Tool-call arguments containing credentials, PII, write bodies, inline media, and prompt injection use a separate closed durable projection and cannot be resurrected from history when result output is withheld.
- Stream abort, cancel, timeout, provider failure, recovery, step-limit, approval, user input, resume, restart, and report fallback. A retryable physical attempt that emitted only private reasoning must be discarded and retried under freshly revalidated attempt authority; its reasoning/signature may not reach the successful attempt, public callback, event, persistence, history, compaction, report, or export.
- Backend report failure and zero-write `write_report=false`.
- Reasoning on success/failure/tool/recovery/history/compaction/report/export.
- DeepSeek, OpenAI chat, OpenAI responses, Anthropic messages, and custom endpoint structure/repair/final paths.
- Truncated, non-object, and duplicate-key streamed tool arguments across every provider family; the host never repairs an executable call, issues no grant/ready event, performs zero tool side effects, and sends only the terminal through the final evidence gate.
- Skill manifest entry traversal, candidate/entry/reference/script symlinks, path-component replacement races, post-discovery mutation, unknown package digest, digest drift on continue/fork, write/shell/MCP self-advertisement, and an empty host-authority intersection.
- Report-publication scanning with shared but acyclic JSON references and trusted host `sourceManifestHash`; neither may be mistaken for a report artifact, while a real cycle or publication material remains fail-closed.
- Verbatim “合成样例事件甲” incident input and fabricated output golden.

## Mandatory Named Tests

```text
CaseFundUnavailableReplacesFabricatedFinal
GlobalAttachmentCannotCrossCaseBinding
SameBlobHasDistinctCaseScopedAttachmentIDs
AttachmentContentHashMismatchBlocksBeforeProvider
AttachmentProjectionHashMismatchBlocksBeforeProvider
AttachmentUploadRequiresThreadAndWorkspaceAtEveryBoundary
AttachmentContentRequiresExactThreadAndWorkspace
AttachmentPublicMetadataRejectsPrivateFields
AttachmentUploadResponsePrivateFieldsBlockedByMain
AttachmentPreviewBytesNeverEnterBlockQueueEventOrHistory
DocumentAttachmentTextNeverEntersUserMessageSSEHistoryCompactionOrExport
AttachmentLocalPathNeverEntersPublicProjection
PublicToolResultRejectsArbitraryPayload
LegacyToolResultOutputWithheldEverywhere
ToolResultDataUrlNeverReachesPublicSurface
ToolResultReasoningNeverPersistsOrReplays
ToolResultRootFieldsCannotBypassProjection
ToolResultCodeCannotExfiltratePII
ToolResultBindingMismatchFailsClosed
ToolMediaRequiresCurrentEffectLease
ToolMediaNeverCrossesCaseBinding
ToolMediaCrashCutsRecoverIdempotently
UploadCrashBeforeOwnerCommitRemainsNonExecutable
UploadCrashAfterOwnerCommitRecoversExactly
ToolCallArgumentsUseClosedPublicProjection
ConcreteClaimRequiresEvidenceReceipt
RejectUnadvertisedToolInAgentMode
FundsSentinelRejectsReadGrepAndMemoryMCP
TruncatedProviderToolArgumentsNeverExecute
RuntimeServerTruncatedProviderToolArgumentsNeverReceiveGrantOrExecute
PairAmountCacheNeverCrossesCaseBinding
EveryFactToolRequiresCompleteFrozenHostContext
PendingExecutionGrantNeverReachesMCP
ProviderToolExecutionHasSingleGrantGatedDispatcher
ProviderToolEffectGatewayReauthorizesBeforeOverride
ApprovalAndUserInputReceiptsPersistExactContextAndGrantAcrossRestart
JobSecurityBindingPersistsAcrossManagerRestart
ToolBatchIssueVerifyAndCompleteRequiresEveryMemberSettled
ProviderContinuationHighLevelAPIBindsExpectedPayloadRouteAndReferences
ReportStageLeasePersistsExactContextGrantAndRevalidatesBeforeSideEffect
MCPExactRawObservationPreservedEndToEnd
HTTPProtocolViolationRevokesServerIdentity
StandardsCompliantMCPJSONSchemaKeywordsEnforcedOffline
BooleanChildSchemasCannotCrashInputOrOutputValidation
SchemaInstanceProductCannotExhaustRuntime
EnumCardinalityParticipatesInSchemaInstanceBudget
SchemaDiagnosticsAreByteStableAcrossRuns
MalformedBase64MCPContentRejected
InvalidStructuredContentCannotEraseRemoteIsError
InvalidOutputSchemaRetainsQuarantinedRawHashWithoutEvidenceAuthority
MCPProtocolVersionSingleSourceOfTruth
FundsNegotiatesSupportedMCPVersion
ParseToolsPreservesTaskSupport
CacheBindsTaskSupportAndRejectsInvalidValue
TaskRequiredToolCannotEnterSynchronousAdmission
MCPToolNameGrammarAcceptsUppercaseDotAnd128Characters
ParseCallToolResultAcceptsStandardMetaAndLastModified
ParseCallToolResultAcceptsResourceLinkIcons
CallToolResultRequiresContent
CallToolErrorMayOmitStructuredContent
MCPToolErrorWithoutStructuredContentIsSemanticFailure
ToolSchemaHashBindsHostOnlyMCPTaskSupport
ToolSchemaHashBindsCanonicalDraft7Dependencies
StreamProviderWithRetryDiscardsFailedAttemptReasoningBeforeRetry
ToolsListPaginationRequiresCompleteBoundedCatalog
HTTPToolsListPaginationCollectsCompleteCatalog
StdioToolsListPaginationCollectsCompleteCatalog
ExecutionGrantBindsNegotiatedMCPProtocolVersion
LegacyMCPWithoutStructuredOutputCannotBecomeFactSource
LegacyMCPIdentityCannotRemainFactAuthority
SubsequentHTTPRequestsCarryNegotiatedProtocolVersion
Session404RevokesIdentityAndRequiresInitialize
HTTPClientCloseTerminatesSessionExactlyOnce
HTTPClientCloseAccepts405AndClearsAuthority
HTTPClientCallAfterClosePerformsNoNetworkIO
NativeProbeFatalFailureRevokesIdentityAndAllProbes
NativeEvidenceFatalFailureRevokesIdentityAndAllProbes
NullIDConnectionErrorInvalidatesIdentityWhilePreservingClass
SSEMatchingResponseReturnsBeforeStreamClose
MismatchedStdioResponseCannotPrecedeAcceptedResponse
ToolsListChangedRevokesCatalogBeforeNextExecutionGrant
ServerCancelledRequestCannotPublishLateResult
FundsMcpRejectsPreInitializeOperation
FundsMcpRequiresInitializedNotification
FundsMcpPingWorksInEveryPhase
FundsMcpStrictJsonRpcEnvelope
FundsMcpUnknownMethodIs32601
FundsMcpUnknownOrHiddenToolIs32602
FundsMcpOversizedFrameIsBounded
FundsMcpConcurrencyAndQueueAreBounded
FundsMcpCancellationSuppressesLateResponse
CancelledRequestIdCannotBeReusedUntilOriginalSettles
FundsMcpCancellationReachesNestedFrontdoorAndDuckdbPython
FundsMcpCancellationSuppressesNestedLateResponse
DuckdbCancellationTerminatesPythonRunner
DuckdbCancellationDoesNotWriteWorkbenchHistory
DuckdbSqlCommentLiteralCannotHideExternalAccess
DuckdbSqlFunctionAllowlistRejectsGlobReadBlobAndExtensionScan
DuckdbSqlResultByteLimit
DuckdbSqlExecutionRequiresFrozenSnapshot
DuckdbCanonicalAccountAndMoneyPreserveExactBytes
DuckdbNumericAccountIdentifierFailsClosed
DuckdbSnapshotBindsAcceptedAndRejectedRows
DuckdbVersionedMaterializationAuthority
DuckdbIdentifierAggregateDoesNotMasqueradeAsIdentifier
DuckdbScalarSubqueryGrammarIsNotFunctionAuthority
SkillManifestEntryMustRemainInsidePackageSnapshot
SkillSnapshotRejectsSymlinkEntryAndReference
SkillSnapshotSwapRaceFailsClosed
SkillSnapshotIsImmutableAfterDiscovery
SkillPackageSnapshotDigestBindsEveryConsumedByte
SkillAllowedToolsCannotElevateChildPolicy
SkillWithNoAuthorizedAllowedToolsFailsClosed
SkillContinuationRejectsPackageDigestDrift
RuntimeServerMaliciousSkillCannotAdvertiseOrExecuteWriteTool
ManagedStartupSnapshotAndBaselineRejectTampering
ManagedStartupSnapshotDistinguishesAbsentAndEmptyDirectory
StrictSnapshotBindsManagedDirectoryAbsenceAndMode
StrictSnapshotIncludesAttachmentsAndMCPSchemaCache
StartupPlanSnapshotDriftRejectsBeforeJournal
ReadOnlyStartupBaselineBindsStableSnapshotAndConfiguration
RuntimeStartupCompletesTerminalPrefixAndSecondRestartIsStable
AcceptedFinalRestartRejectsCorruptEventSequence
PrivateEvidenceRegistryCorruptionFailsClosed
CaseCompactionPreservesSignedAuthorityAndAcceptedHistory
AcceptedFinalEventReplayRejectsTamperMissingAndUnknownCaseText
JobInventoryRejectsExternalArtifactPathBeforeAnyMutation
JobInventoryRejectsFilenameIdentityMismatchAndDuplicateJSONKeys
CopyMissingDirectoryLosslessRejectsDivergentConflictWithoutMutation
DurableEventStoreRejectsDivergentNonPlaceholderLegacyThreadWithoutDeletingSource
DurableInventoryReadersDoNotImportLegacyAfterConstruction
EvidenceRegistryAuthorityCapsuleBindsContextRegistryAndCanonicalLedger
EvidenceRegistryAuthorityIndexBindsExactSingleStepLineage
EvidenceRegistryAuthorityIndexRejectsForeignCapsuleKey
EvidenceRegistryAuthorityIndexRejectsSequenceJumpAndForeignPrefix
EvidenceRegistryAuthorityRecordsRejectDuplicateKeysAndNonCanonicalBytes
ValidRegistryPrefixTailDeletionPreservesTrustedCapsuleAuthority
RegistryProjectionExactPrefixRepairBoundaryAndInstallationTrust
CapsulePairRollbackCannotResurrectRevokedReceipt
OlderRootIndexCannotHideNewerInstalledCapsuleBlob
AuthorityIndexCommitSurvivesMissingDerivedProjections
SignedCapsuleProjectionWithoutRootIndexIsNotSilentlyAdopted
CommitPreparedHasNoHiddenCommittedError
AuthorityCommitFailureBeforeReplaceLeavesNoMembershipAndIsRetryable
CapsuleInstallFailureBeforeIndexReplaceCleansAndRetries
RevokeHasNoHiddenCommittedError
AuthorityIndexPredecessorCASRejectsLateReplacement
RootIndexWriteRejectsCorruptReferencedOtherTurnCapsule
CapsuleLinkUnlinkCrashResidueIsRepairableButNotAuthority
CurrentCapsuleGCPreventsQuadraticHistoryBlobGrowth
RegistryLockHardlinkDoesNotMutateExternalTarget
RegistryLockSymlinkCannotEscapeRoot
ColdRegistryRootRejectsSymlinkAncestorBeforeMutation
NormalToolRejectsRuntimeCaseProjectMismatch
WorkbenchHistoryNeverCrossesThreadEpochOrSnapshot
FullSha256RequiredForEvidenceIdentity
FakeCitationRejected
MismatchedCitationRejected
EmptyResultIsNotZero
PartialCoverageCannotBecomeWholeCaseConclusion
SourceEpochMismatchBlocksPublish
StaleApprovalGrantRejected
CallerCannotOverrideRiskOrPublicationPolicy
CaseBindingObserverDistinguishesMissingInvalidUnreadableUnstable
CorruptCaseProjectProducesSignedHostBoundary
CorruptCaseProjectDoesNotRequireProviderConfiguration
CaseBoundaryInvokesNoProviderMCPAttachmentVisionResearchEffect
UnboundHighRiskCaseRequestHasAcceptedFinalAuthority
BoundaryPolicyCannotIssueExecutionGrantEvidenceReceiptJobOrContinuation
CaseBindingRepairBumpsSharedContextEpoch
UnresolvedSnapshotCannotAuthorizeExecution
VerifiedProbeCannotUpgradeFrozenContext
LiveProbeSnapshotMustEqualFrozenHostSnapshot
V1RejectedAtEveryExecutionBoundary
BoundaryEveryTerminalPathUsesFinalEvidenceGate
BoundaryReasoningAndDraftNeverPersistedOrStreamed
BoundaryCompactionBlocked
ForkResumeInheritsCaseRisk
StrictStartRequestRejectsUnknownDuplicateAndWrongType
HighRiskSteerCannotEnterGeneralTurn
ThreadRiskTailDeletionCannotDowngradeCase
ThreadRiskWholeThreadDeletionCannotRebootstrapGeneral
ThreadRiskOldHeadAndOldPolicyCompositeRollbackRejected
ThreadRiskCurrentHeadPolicyDeletionQuarantines
HistoricalGeneralPolicyCannotAuthorizeAfterCaseRaise
ThreadRiskHeadChangeInvalidatesPendingApproval
ThreadRiskHeadChangeInvalidatesResumeAndBackgroundJob
ThreadRiskHeadChangeBumpsContextEpoch
ThreadRiskWitnessNonceReplayRejected
ThreadRiskWitnessWrongInstallationRejected
ThreadRiskWitnessSameGenerationEquivocationRejected
ThreadRiskWitnessUnavailableFailsClosed
ConcurrentRuntimeRiskRaiseHasSingleWitnessWinner
ThreadRiskCommitIndeterminatePoisonsUntilReconcile
SemanticStartupRepairsProjectionOnlyFromWitness
SemanticStartupNeverPromotesMaximumLocalGeneration
ReasoningNotPersistedOrExported
ReportRequiresPublicationReceipt
WriteReportFalseProducesNoFile
EveryTerminalPathUsesFinalEvidenceGate
```

Test names may use language-appropriate prefixes, but release tooling must map each required id to exactly one or more executable tests and report pass/fail/skip.

## Terminal Coverage Contract

A table-driven test enumerates every internal terminal reason. Production finalization code registers each reason in one closed set; the test fails if a reason has no final-gate mapping or a terminal persistence function can accept raw provider text. Required rows include normal success, unavailable, blocked tool, provider error, tool error, semantic failure, empty/partial, cancel, timeout, stream abort, recovery, step limit, approval allowed/denied/expired/stale, user-input resolved/cancelled/stale, resume, restart abort, background completion, report failure, and publish failure.

## Absence Assertions

High-risk rejection tests search all observable outputs, not only the final string:

```text
live SSE
events.jsonl and messages/history persistence
thread/session/fork/search/resume projections
provider-history reconstruction
compaction input/output
renderer store and timeline
logs/traces
report staging/final files
exported thread/report artifacts
```

Forbidden draft facts and reasoning must have zero bytes in every applicable surface.

## Required Commands

Run the smallest focused tests during each task, then before release run all applicable commands from the user request and root `AGENTS.md`, including TypeScript checks/builds, Go ordinary/prod/race tests, runtime release gate, plugin contract/smoke/oracle/red-team suites, `git diff --check`, real dev/UI verification, and current-host package validation. Results record exact command, exit status, duration, pass/fail/skip counts, commit/worktree state, environment, and artifact path without secrets or case data.

No skip is allowed for P0/P1 scenarios. External credentials/platforms are
separate truthful rows and cannot be silently counted as pass.
