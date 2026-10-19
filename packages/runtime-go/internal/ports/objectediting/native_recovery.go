package objectediting

import (
	"context"
	"encoding/json"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
)

// NativeChangeDraft is supplied by Core's approved selection owner. It contains
// local review text, never model-facing metadata or caller-supplied file bytes.
type NativeChangeDraft struct {
	ChangeID     string
	ThreadID     string
	ProposalID   string
	BaseRevision string
	BeforeText   string
	AfterText    string
	Workbook     *office.WorkbookReview     `json:"workbook,omitempty"`
	Presentation *office.PresentationReview `json:"presentation,omitempty"`
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
	Version         int                        `json:"-"`
	Origin          string                     `json:"-"`
	ChangeID        string                     `json:"changeId"`
	ThreadID        string                     `json:"threadId"`
	ProposalID      string                     `json:"proposalId"`
	BaseRevision    string                     `json:"baseRevision"`
	Revision        string                     `json:"revision"`
	Status          string                     `json:"status"`
	BeforeText      string                     `json:"beforeText"`
	AfterText       string                     `json:"afterText"`
	Workbook        *office.WorkbookReview     `json:"workbook,omitempty"`
	Presentation    *office.PresentationReview `json:"presentation,omitempty"`
	SaveOperationID string                     `json:"saveOperationId"`
	UndoOperationID string                     `json:"undoOperationId"`
	CanUndo         bool                       `json:"canUndo"`
	CanCancel       bool                       `json:"canCancel"`
	CanRetryUndo    bool                       `json:"canRetryUndo"`
	CanResume       bool                       `json:"canResume"`
	CreatedAt       string                     `json:"createdAt"`
	SavedAt         string                     `json:"savedAt"`
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

// Candidate generation reads only Core's durable prepared workbook review and
// original backup. It neither authorizes a change nor starts a save journal.
type NativeWorkbookCandidateInput struct {
	Workspace, Path, ObjectIdentity               string
	ThreadID, ChangeID, OperationID, BaseRevision string
}

type NativeWorkbookCandidateContent struct {
	Encoding   string `json:"encoding"`
	Kind       string `json:"kind"`
	ByteLength int    `json:"byteLength"`
	SHA256     string `json:"sha256"`
	Data       string `json:"data"`
}

type NativeWorkbookCandidate struct {
	ChangeID        string                         `json:"changeId"`
	SaveOperationID string                         `json:"saveOperationId"`
	BaseRevision    string                         `json:"baseRevision"`
	Content         NativeWorkbookCandidateContent `json:"content"`
}

type NativeWorkbookCandidateFiles interface {
	BuildNativeWorkbookCandidate(context.Context, NativeWorkbookCandidateInput) (NativeWorkbookCandidate, error)
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
type NativeWorkbookInput struct {
	Workspace      string
	Path           string
	ObjectIdentity string
	BaseRevision   string
	Selection      office.WorkbookSelection
	Patch          *office.WorkbookPatch
}

type NativeRecoveryFiles interface {
	// ValidateNativeWorkbook reads the authorized original; nil Patch validates
	// capture only. It never writes an imported workbook or creates a receipt.
	ValidateNativeWorkbook(context.Context, NativeWorkbookInput) (*office.WorkbookReview, error)
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

// NativePresentationFiles extends the same session-backed native recovery owner.
// Capture validation does not grant mutation or return caller-selected paths.
type NativePresentationInput struct {
	Workspace      string
	Path           string
	ObjectIdentity string
	BaseRevision   string
	Selection      office.PresentationSelection
	Patch          *office.PresentationPatch
}
type NativePresentationFiles interface {
	ValidateNativePresentation(context.Context, NativePresentationInput) (*office.PresentationReview, error)
}

// Manual authority is an explicit variant; an empty proposal never grants edits.
type NativeManualDraft struct {
	ChangeID         string `json:"changeId"`
	ThreadID         string `json:"threadId"`
	BaseRevision     string `json:"baseRevision"`
	BeginOperationID string `json:"beginOperationId"`
}
type NativeManualInput struct {
	Workspace, Path, ObjectIdentity string
	Draft                           NativeManualDraft
}
type NativeManualFiles interface {
	PrepareNativeManual(context.Context, NativeManualInput) (NativeChangeStatus, error)
}

// Workbook manual authority is a distinct typed variant. The captured rectangle
// supplies read dependencies; WriteTarget remains the only writable cell.
type NativeWorkbookManualDraft struct {
	NativeManualDraft
	WriteTarget office.WorkbookWriteTarget `json:"writeTarget"`
	Edit        office.WorkbookManualEdit  `json:"edit"`
	Workbook    *office.WorkbookReview     `json:"workbook,omitempty"`
}
type NativeWorkbookManualInput struct {
	Workspace, Path, ObjectIdentity string
	Draft                           NativeWorkbookManualDraft
}
type NativeWorkbookManualValidationInput struct {
	Workspace, Path, ObjectIdentity, BaseRevision string
	Selection                                     office.WorkbookSelection
	WriteTarget                                   office.WorkbookWriteTarget
	Edit                                          office.WorkbookManualEdit
}
type NativeWorkbookEditingInput struct {
	Workspace, Path, ObjectIdentity, BaseRevision string
}
type NativeWorkbookManualFiles interface {
	NativeWorkbookEditing(context.Context, NativeWorkbookEditingInput) error
	ValidateNativeWorkbookManual(context.Context, NativeWorkbookManualValidationInput) (*office.WorkbookReview, error)
	PrepareNativeWorkbookManual(context.Context, NativeWorkbookManualInput) (NativeChangeStatus, error)
}

func (s NativeChangeStatus) MarshalJSON() ([]byte, error) {
	type legacy NativeChangeStatus
	if s.Version != 2 && s.Version != 3 {
		return json.Marshal(legacy(s))
	}
	if s.Version == 3 {
		return json.Marshal(struct {
			Version         int                    `json:"version"`
			Origin          string                 `json:"origin"`
			ChangeID        string                 `json:"changeId"`
			ThreadID        string                 `json:"threadId"`
			BaseRevision    string                 `json:"baseRevision"`
			Revision        string                 `json:"revision"`
			Status          string                 `json:"status"`
			Workbook        *office.WorkbookReview `json:"workbook,omitempty"`
			SaveOperationID string                 `json:"saveOperationId"`
			UndoOperationID string                 `json:"undoOperationId"`
			CanUndo         bool                   `json:"canUndo"`
			CanCancel       bool                   `json:"canCancel"`
			CanRetryUndo    bool                   `json:"canRetryUndo"`
			CanResume       bool                   `json:"canResume"`
			CreatedAt       string                 `json:"createdAt"`
			SavedAt         string                 `json:"savedAt"`
		}{3, "manual", s.ChangeID, s.ThreadID, s.BaseRevision, s.Revision, s.Status, s.Workbook, s.SaveOperationID, s.UndoOperationID, s.CanUndo, s.CanCancel, s.CanRetryUndo, s.CanResume, s.CreatedAt, s.SavedAt})
	}
	return json.Marshal(struct {
		Version         int    `json:"version"`
		Origin          string `json:"origin"`
		ChangeID        string `json:"changeId"`
		ThreadID        string `json:"threadId"`
		BaseRevision    string `json:"baseRevision"`
		Revision        string `json:"revision"`
		Status          string `json:"status"`
		SaveOperationID string `json:"saveOperationId"`
		UndoOperationID string `json:"undoOperationId"`
		CanUndo         bool   `json:"canUndo"`
		CanCancel       bool   `json:"canCancel"`
		CanRetryUndo    bool   `json:"canRetryUndo"`
		CanResume       bool   `json:"canResume"`
		CreatedAt       string `json:"createdAt"`
		SavedAt         string `json:"savedAt"`
	}{2, "manual", s.ChangeID, s.ThreadID, s.BaseRevision, s.Revision, s.Status,
		s.SaveOperationID, s.UndoOperationID, s.CanUndo, s.CanCancel, s.CanRetryUndo, s.CanResume, s.CreatedAt, s.SavedAt})
}
