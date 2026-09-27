export function observedSingleInstanceLockFailure(text: string): boolean
export function processProbeFailureKind(error: { code?: string; status?: number } | null, command: string): 'timeout' | 'no_match' | 'unknown'
export function exactTaskGroupMembers(processRows: string, groupPid: number, appContentsRoot: string): number[] | null
export function exactMainCommandMatches(commandRow: string, parentPid: number, mainPid: number, executable: string, debugPort: number): boolean
export function normalQuitFailureCode(evidence: { quitRequestOk: boolean; targetClosed: boolean; mainExited: boolean; mainExitCode: number | null; mainSignal: string | null; fallbackUsed: boolean; residualProcessCount: number | null }): string
export function residualMembersAlreadyPinned(currentPids: number[] | null, pinnedPids: Set<number>): boolean
export function parseRuntimeStartupNumericDiagnostic(line: string): { key: string; value: number } | null
export function parseRuntimeTransitionPhase(line: string): { phase: string; elapsedMs: number } | null
export function parseGoStartupPhase(line: string): { phase: string; elapsedMs: number } | null
export function isGoStartupAttemptMarker(line: string): boolean
