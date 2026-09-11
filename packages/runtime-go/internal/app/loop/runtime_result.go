package loop

import (
	"context"
	"errors"

	"analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
)

const RuntimeCandidateInputClassOrdinaryOnly = "ordinary_only"

type RuntimeAgentLoopResult struct {
	AssistantText        string
	LastResult           domainmodel.Result
	TerminalRecoveryKind RuntimeTerminalRecoveryKind
	// CaseSourceUnavailable is host-owned sticky state. It records that a
	// protected case-data step was unavailable even when independent ordinary
	// work was allowed to continue. The final evidence gate consumes it; it is
	// never provider output or a serialized public field.
	CaseSourceUnavailable   bool `json:"-"`
	Paused                  bool
	PendingKind             string
	PendingID               string
	ReportDeliveryCompleted bool `json:"-"`
	// CandidateUsesCaseData is the exact host-owned effect held by the
	// candidate terminal handoff. A case-bound thread may finish an ordinary
	// step without reintroducing a stale dataset as a global prerequisite.
	CandidateUsesCaseData bool `json:"-"`
	// CandidateOrdinaryWork and CandidateInputClass preserve the host-owned
	// provider-step/input provenance through terminal CAS handoff. Slot data
	// cannot assert these process-local facts about its own producer.
	CandidateOrdinaryWork bool   `json:"-"`
	CandidateInputClass   string `json:"-"`
	// OrdinaryResult is a projected, typed, explicitly non-evidentiary result
	// compiled only for an exact ordinary-work candidate step. The existing
	// terminal CAS or accepted-final authority must still bind it before any
	// public persistence.
	OrdinaryResult *domainordinaryresult.ResultSlotV1 `json:"-"`
	// CandidateTerminalContext and ReleaseCandidateTerminal are private host
	// capabilities. They carry the loop's final steering/terminal
	// linearization into the existing publication path and are never
	// serialized, persisted, or exposed to a provider.
	CandidateTerminalContext context.Context `json:"-"`
	ReleaseCandidateTerminal func()          `json:"-"`
}

// RuntimeTerminalRecoveryKind is sticky host state for a model loop that only
// reached a terminal response after a recovery intervention. It is not model
// output. The Final Evidence Gate uses it to prevent a recovered answer from
// being upgraded back to an evidence-bearing success/approval/resume path.
type RuntimeTerminalRecoveryKind = domaincontinuation.TerminalRecoveryKindV1

const (
	RuntimeTerminalRecoveryNone      = domaincontinuation.TerminalRecoveryNoneV1
	RuntimeTerminalRecoveryApplied   = domaincontinuation.TerminalRecoveryAppliedV1
	RuntimeTerminalRecoveryStepLimit = domaincontinuation.TerminalRecoveryStepLimitV1
)

func MergeRuntimeTerminalRecoveryKind(current, next RuntimeTerminalRecoveryKind) RuntimeTerminalRecoveryKind {
	if current == "" {
		current = RuntimeTerminalRecoveryNone
	}
	if next == "" {
		next = RuntimeTerminalRecoveryNone
	}
	if current == RuntimeTerminalRecoveryStepLimit || next == RuntimeTerminalRecoveryStepLimit {
		return RuntimeTerminalRecoveryStepLimit
	}
	if current == RuntimeTerminalRecoveryApplied || next == RuntimeTerminalRecoveryApplied {
		return RuntimeTerminalRecoveryApplied
	}
	if current != RuntimeTerminalRecoveryNone {
		return current
	}
	return next
}

func CommitRuntimeTerminalRecovery(result *RuntimeAgentLoopResult, sticky *RuntimeTerminalRecoveryKind) {
	if result == nil || sticky == nil {
		return
	}
	result.TerminalRecoveryKind = MergeRuntimeTerminalRecoveryKind(result.TerminalRecoveryKind, *sticky)
}

// SealRuntimeAgentLoopResultForReturn is the single return boundary for the
// runtime runner. Provider chunks can contain private reasoning, raw tool
// material, or malformed markup and therefore never survive success,
// failure, recovery, pause, cancellation, or a future early-return path.
// Bounded usage/cache diagnostics remain available separately on LastResult.
func SealRuntimeAgentLoopResultForReturn(result *RuntimeAgentLoopResult, sticky *RuntimeTerminalRecoveryKind) {
	if result == nil {
		return
	}
	result.LastResult.Chunks = nil
	CommitRuntimeTerminalRecovery(result, sticky)
}

type RuntimeAgentLoopFailure struct {
	Cause  error
	Result RuntimeAgentLoopResult
}

func RejectCancelledRuntimeResult(ctx context.Context, result RuntimeAgentLoopResult, err error) (RuntimeAgentLoopResult, error) {
	if ctx != nil && ctx.Err() != nil {
		result.AssistantText = ""
		result.OrdinaryResult = nil
		result.CandidateUsesCaseData = false
		result.CandidateOrdinaryWork = false
		result.CandidateInputClass = ""
		result.LastResult.Chunks = nil
		result.Paused = false
		result.PendingKind = ""
		result.PendingID = ""
		return result, ctx.Err()
	}
	return result, err
}

func RuntimeResultCarriesOrdinaryCandidate(result RuntimeAgentLoopResult) bool {
	return result.OrdinaryResult != nil && !result.CandidateUsesCaseData && result.CandidateOrdinaryWork &&
		result.CandidateInputClass == RuntimeCandidateInputClassOrdinaryOnly &&
		domainordinaryresult.ValidateResultSlotV1(*result.OrdinaryResult) == nil
}

func (err RuntimeAgentLoopFailure) Error() string {
	if err.Cause == nil {
		return "agent loop failed"
	}
	return err.Cause.Error()
}

func (err RuntimeAgentLoopFailure) Unwrap() error {
	return err.Cause
}

type NormalizedRuntimeFailure struct {
	Result   RuntimeAgentLoopResult
	Cause    error
	Public   domainfailure.Record
	Message  string
	Code     string
	Severity string
	Details  map[string]any
}

func NormalizeRuntimeFailure(cause error) NormalizedRuntimeFailure {
	normalized := NormalizedRuntimeFailure{Cause: cause, Severity: "error"}
	var loopFailure RuntimeAgentLoopFailure
	if errors.As(cause, &loopFailure) {
		normalized.Result = loopFailure.Result
		if loopFailure.Cause != nil {
			normalized.Cause = loopFailure.Cause
		}
	}
	normalized.Public = PublicFailureForError(normalized.Cause)
	normalized.Message = normalized.Public.Message()
	normalized.Code = normalized.Public.Code()
	normalized.Severity = normalized.Public.Severity()
	normalized.Details = contracts.CloneMap(normalized.Public.Details())
	return normalized
}
