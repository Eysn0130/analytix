export function observedSingleInstanceLockFailure(text: string): boolean
export function processProbeFailureKind(error: { code?: string; status?: number } | null, command: string): 'timeout' | 'no_match' | 'unknown'
export function exactTaskGroupMembers(processRows: string, groupPid: number, appContentsRoot: string): number[] | null
export function exactMainCommandMatches(commandRow: string, parentPid: number, mainPid: number, executable: string, debugPort: number): boolean
export function exactCoreRuntimeCommandMatches(command: string, executable: string, runtimeDataDir: string): boolean
export function normalQuitFailureCode(evidence: { quitRequestOk: boolean; targetClosed: boolean; mainExited: boolean; mainExitCode: number | null; mainSignal: string | null; fallbackUsed: boolean; residualProcessCount: number | null }): string
export function classifyExactTargetList(targets: unknown, capturedTargetId: string): 'target_present' | 'target_absent' | 'malformed'
export function residualMembersAlreadyPinned(currentPids: number[] | null, pinnedPids: Set<number>): boolean
export function parseRuntimeStartupNumericDiagnostic(line: string): { key: string; value: number } | null
export function parseRuntimeTransitionPhase(line: string): { phase: string; elapsedMs: number } | null
export function parseGoStartupPhase(line: string): { phase: string; elapsedMs: number } | null
export function parseGoOwnerPhase(line: string): { stage: string; durationMs: number } | null
export function isGoStartupAttemptMarker(line: string): boolean
export type FundsCSVActivationCodeV2 = 'native_owner_unavailable' | 'shared_evidence_enrollment_absent' | 'shared_evidence_credential_unavailable' | 'dataset_snapshot_unavailable' | 'ready' | 'other_unavailable'
export type FundsCSVAdmissionAttemptEvidenceV2 = {
  coreAttemptOrdinal: number
  status: 'complete' | 'incomplete'
  semanticPreparationCode: 'not_evaluated' | null
  code: FundsCSVActivationCodeV2 | null
  nativeOwner: 'available' | 'unavailable' | 'unknown'
}
export function parseFundsCSVAdmissionMarkerV2(line: string): { phase: 'semantic_preparation'; code: 'not_evaluated' } | { phase: 'activation'; code: FundsCSVActivationCodeV2 } | null
export function createFundsCSVAdmissionAttemptRecorderV2(enabled: boolean): {
  acceptLine(channel: string, line: string, complete?: boolean): void
  evidence(): FundsCSVAdmissionAttemptEvidenceV2[]
}
export function projectFundsCSVAdmissionDiagnosticV2(
  launchRecorders: ReturnType<typeof createFundsCSVAdmissionAttemptRecorderV2>[]
): { fundsCSVAdmissionDiagnostic?: (FundsCSVAdmissionAttemptEvidenceV2 & { launchOrdinal: number })[] }
