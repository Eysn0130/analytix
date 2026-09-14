package objectediting

import "context"

// NativeChangeDraft is supplied by Core's approved selection owner. It contains
// local review text, never model-facing metadata or caller-supplied file bytes.
type NativeChangeDraft struct {
	ChangeID     string
	ThreadID     string
	ProposalID   string
	BaseRevision string
	BeforeText   string
	AfterText    string
}

type NativeChangeInput struct {
	Workspace      string
	Path           string
	ObjectIdentity string
	Draft          NativeChangeDraft
}

// NativeChangeStatus belongs only to the protected-local display lane. Engine
// selection tokens are intentionally absent: reopening cannot revive a capture.
type NativeChangeStatus struct {
	ChangeID        string `json:"changeId"`
	ThreadID        string `json:"threadId"`
	ProposalID      string `json:"proposalId"`
	BaseRevision    string `json:"baseRevision"`
	Revision        string `json:"revision"`
	Status          string `json:"status"`
	BeforeText      string `json:"beforeText"`
	AfterText       string `json:"afterText"`
	SaveOperationID string `json:"saveOperationId"`
	UndoOperationID string `json:"undoOperationId"`
	CanUndo         bool   `json:"canUndo"`
	CanCancel       bool   `json:"canCancel"`
	CanRetryUndo    bool   `json:"canRetryUndo"`
	CanResume       bool   `json:"canResume"`
	CreatedAt       string `json:"createdAt"`
	SavedAt         string `json:"savedAt"`
}

type NativeRecovery struct {
	Current *NativeChangeStatus `json:"current"`
	Pending *NativeChangeStatus `json:"pending"`
}

type NativeCommitInput struct {
	CommitInput
	ThreadID string
	ChangeID string
}

type NativeUndoInput struct {
	Workspace      string
	Path           string
	ObjectIdentity string
	ThreadID       string
	ChangeID       string
	BaseRevision   string
}

// NativeRecoveryFiles is deliberately separate from ordinary text editing.
// Prepare reads the original through the existing file authority before the
// isolated engine changes. Commit/undo reuse the same CAS and operation journal.
type NativeRecoveryFiles interface {
	PrepareNativeChange(context.Context, NativeChangeInput) (NativeChangeStatus, error)
	NativeRecovery(context.Context, string, string, string, string) (NativeRecovery, error)
	CommitNativeChange(context.Context, NativeCommitInput) (Receipt, error)
	UndoNativeChange(context.Context, NativeUndoInput) (Receipt, error)
	// Cancel retires only a confirmed uncommitted pending change. BaseRevision
	// binds the explicit cancellation to the current on-disk revision.
	CancelNativeChange(context.Context, NativeUndoInput) (NativeRecovery, error)
	// Resume is a fresh explicit authorization to try the same pending native
	// save against its original base, using only Core's durable candidate bytes.
	ResumeNativeChange(context.Context, NativeUndoInput) (Receipt, error)
}
