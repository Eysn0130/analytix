package evidence

// TerminalEmissionClassV1 distinguishes a turn terminal from later lifecycle
// or continuity input. Only turn_terminal reasons may enter the case terminal
// emitter; the other rows remain in the closed vocabulary for compatibility
// and must not finalize an already-terminal parent turn.
type TerminalEmissionClassV1 string

const (
	TerminalEmissionTurnV1                TerminalEmissionClassV1 = "turn_terminal"
	TerminalEmissionContinuityInputV1     TerminalEmissionClassV1 = "continuity_input"
	TerminalEmissionBackgroundLifecycleV1 TerminalEmissionClassV1 = "background_lifecycle"
	TerminalEmissionNoProductionEmitterV1 TerminalEmissionClassV1 = "no_production_emitter"
)

type TerminalProductionOwnerV1 string

const (
	TerminalOwnerForegroundCompletionV1 TerminalProductionOwnerV1 = "completeStartedRuntimeTurn"
	TerminalOwnerHostBoundaryV1         TerminalProductionOwnerV1 = "completeHostBoundaryTurn"
	TerminalOwnerRuntimeFailureV1       TerminalProductionOwnerV1 = "persistRuntimeTurnFailureForInput"
	TerminalOwnerGateContinuationV1     TerminalProductionOwnerV1 = "finalizeRuntimeTurnAfterLoop"
	TerminalOwnerInterruptV1            TerminalProductionOwnerV1 = "InterruptActiveTurn"
	TerminalOwnerRestartV1              TerminalProductionOwnerV1 = "abortStaleRuntimeTurnAfterRestart"
	TerminalOwnerCurrentAuthorityV1     TerminalProductionOwnerV1 = "currentPublicationAuthorityFinalizer.PersistBoundary"
)

type TerminalReasonOwnershipV1 struct {
	Reason TerminalReason
	Class  TerminalEmissionClassV1
	Owners []TerminalProductionOwnerV1
}

// ProductionTerminalReasonOwnershipV1 is the closed as-built reason/owner
// table. It intentionally records dormant vocabulary without manufacturing a
// production emitter for it.
func ProductionTerminalReasonOwnershipV1() []TerminalReasonOwnershipV1 {
	return []TerminalReasonOwnershipV1{
		{Reason: TerminalSuccess, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerForegroundCompletionV1, TerminalOwnerHostBoundaryV1, TerminalOwnerGateContinuationV1}},
		{Reason: TerminalSourceUnavailable, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerForegroundCompletionV1, TerminalOwnerHostBoundaryV1, TerminalOwnerRuntimeFailureV1, TerminalOwnerGateContinuationV1}},
		{Reason: TerminalSemanticFailure, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerForegroundCompletionV1, TerminalOwnerRuntimeFailureV1, TerminalOwnerGateContinuationV1}},
		{Reason: TerminalProviderFailure, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerHostBoundaryV1, TerminalOwnerRuntimeFailureV1, TerminalOwnerCurrentAuthorityV1}},
		{Reason: TerminalCancel, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerRuntimeFailureV1, TerminalOwnerGateContinuationV1, TerminalOwnerInterruptV1}},
		{Reason: TerminalTimeout, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerRuntimeFailureV1}},
		{Reason: TerminalStreamAbort, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerRuntimeFailureV1}},
		{Reason: TerminalRecovery, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerForegroundCompletionV1, TerminalOwnerGateContinuationV1}},
		{Reason: TerminalApproval, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerGateContinuationV1}},
		{Reason: TerminalUserInput, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerGateContinuationV1}},
		{Reason: TerminalResume, Class: TerminalEmissionContinuityInputV1},
		{Reason: TerminalRestart, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerRestartV1}},
		{Reason: TerminalReportFallback, Class: TerminalEmissionNoProductionEmitterV1},
		{Reason: TerminalStepLimit, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerForegroundCompletionV1, TerminalOwnerRuntimeFailureV1, TerminalOwnerGateContinuationV1}},
		{Reason: TerminalBackgroundCompletion, Class: TerminalEmissionBackgroundLifecycleV1},
		{Reason: TerminalToolFailure, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerRuntimeFailureV1}},
		{Reason: TerminalApprovalDenied, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerGateContinuationV1}},
		{Reason: TerminalInputCancelled, Class: TerminalEmissionTurnV1, Owners: []TerminalProductionOwnerV1{TerminalOwnerGateContinuationV1}},
	}
}

func productionTurnTerminalReasonV1(reason TerminalReason) bool {
	for _, row := range ProductionTerminalReasonOwnershipV1() {
		if row.Reason == reason {
			return row.Class == TerminalEmissionTurnV1 && len(row.Owners) > 0
		}
	}
	return false
}
