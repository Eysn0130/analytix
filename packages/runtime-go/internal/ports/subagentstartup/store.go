package subagentstartup

import domainjob "analytix.local/runtime-go/internal/domain/job"

type RecoveredParentToolSettlementInput struct {
	Record         domainjob.Record
	ToolCallItemID string
	CallID         string
	ToolName       string
	Status         string
	Timestamp      string
	ResultItem     map[string]any
}

type RecoveredParentToolSettlementResult string

const (
	RecoveredParentToolSettlementInserted      RecoveredParentToolSettlementResult = "inserted"
	RecoveredParentToolSettlementExistingExact RecoveredParentToolSettlementResult = "existing_exact"
	RecoveredParentToolSettlementConflict      RecoveredParentToolSettlementResult = "conflict"
)

type ThreadStore interface {
	GetThread(string) (map[string]any, error)
	EnsureRecoveredParentToolSettlementExact(RecoveredParentToolSettlementInput) (RecoveredParentToolSettlementResult, error)
}

type JobStore interface {
	AllRecords() []domainjob.Record
	LoadChildRun(string) (domainjob.Record, error)
	UpdateChildRun(string, domainjob.UpdateRequest) (domainjob.Record, error)
	// Closes restart-scope installation before returning. The result must
	// remain valid throughout recovery's later job and parent callbacks.
	RestartPreservesChildRunV1(domainjob.Record) (bool, error)
}

type SecurityAuthority interface {
	Blocker(domainjob.Record) string
	MarkDeadLetter(domainjob.Record, string) error
}
