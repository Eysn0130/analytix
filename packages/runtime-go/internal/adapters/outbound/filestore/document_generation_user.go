package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	docport "analytix.local/runtime-go/internal/ports/documentgeneration"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// Creation has no before-file. Its receipt is separate from edit reconciliation:
// an external identical blank file can never prove this operation committed.
type userDOCXRecord struct {
	Version     int    `json:"version"`
	Origin      string `json:"origin"`
	Subject     string `json:"subject"`
	Operation   string `json:"operation"`
	PathBinding string `json:"pathBinding"`
	RequestHash string `json:"requestHash"`
	Revision    string `json:"revision"`
	Status      string `json:"status"`
	ByteSize    int    `json:"byteSize"`
	SavedAt     string `json:"savedAt"`
}

func (s *ObjectEditingFiles) CreateUserDocument(ctx context.Context, input docport.UserCreateInput) (docport.UserCreation, error) {
	if !docport.ValidUserDOCXName(input.FileName) || s.officeKind != "docx" || !objectEditingHash(input.SubjectIdentity) || !objectEditingHash(input.RequestHash) || !objectEditingOperationID.MatchString(input.OperationID) || input.CheckCurrent == nil || input.Encode != nil && len(input.Content) != 0 {
		return docport.UserCreation{}, editing.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return docport.UserCreation{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(input.Workspace, input.FileName)
	if err != nil {
		return docport.UserCreation{}, err
	}
	journalPath := s.nativePath(input.SubjectIdentity, input.OperationID, ".user-docx.json")
	body, _, readErr := s.readNativePrivate(journalPath, 16384)
	if readErr == nil {
		var r userDOCXRecord
		if nativeDecode(body, &r, 16384) != nil || r.Version != 1 || r.Origin != "user-create" || r.Subject != input.SubjectIdentity || r.Operation != input.OperationID || !objectEditingHash(r.Revision) || r.ByteSize <= 0 || r.ByteSize > MaxOfficeObjectBytes || (r.Status != "unknown" && r.Status != "committed" && r.Status != "conflict") {
			return docport.UserCreation{}, editing.ErrPersistence
		}
		if r.PathBinding != target.binding || r.RequestHash != input.RequestHash {
			return docport.UserCreation{}, editing.ErrOperationMismatch
		}
		if input.CheckCurrent() != nil {
			return docport.UserCreation{}, editing.ErrForbidden
		}
		if r.Status == "conflict" {
			return docport.UserCreation{}, editing.ErrConflict
		}
		if r.Status != "committed" {
			return docport.UserCreation{}, &docport.UserCreationUnknown{OperationID: r.Operation, Revision: r.Revision}
		}
		if _, err := time.Parse(time.RFC3339Nano, r.SavedAt); err != nil {
			return docport.UserCreation{}, editing.ErrPersistence
		}
		return docport.UserCreation{OperationID: r.Operation, Workspace: target.workspace, Path: target.path, Revision: r.Revision, Kind: "docx", ByteSize: r.ByteSize, SavedAt: r.SavedAt}, nil
	}
	if !errors.Is(readErr, editing.ErrOperationNotFound) {
		return docport.UserCreation{}, readErr
	}
	if input.CheckCurrent() != nil {
		return docport.UserCreation{}, editing.ErrForbidden
	}
	if input.Encode != nil {
		input.Content, err = input.Encode(ctx)
		if err != nil {
			return docport.UserCreation{}, err
		}
	}
	if len(input.Content) == 0 || len(input.Content) > MaxOfficeObjectBytes || s.validateNativeObject(input.Content) != nil {
		return docport.UserCreation{}, editing.ErrNotText
	}
	if input.CheckCurrent() != nil {
		return docport.UserCreation{}, editing.ErrForbidden
	}
	r := userDOCXRecord{Version: 1, Origin: "user-create", Subject: input.SubjectIdentity, Operation: input.OperationID, PathBinding: target.binding, RequestHash: input.RequestHash, Revision: digestAtomicText(input.Content), Status: "unknown", ByteSize: len(input.Content)}
	body, _ = json.Marshal(r)
	if err = s.writeNativePrivate(journalPath, body, "", 16384); err != nil {
		return docport.UserCreation{}, err
	}
	conflict := func() (docport.UserCreation, error) {
		r.Status = "conflict"
		encoded, _ := json.Marshal(r)
		if s.writeNativePrivate(journalPath, encoded, digestAtomicText(body), 16384) != nil {
			return docport.UserCreation{}, editing.ErrPersistence
		}
		return docport.UserCreation{}, editing.ErrConflict
	}
	if _, err := os.Lstat(target.path); err == nil {
		return conflict()
	} else if !errors.Is(err, os.ErrNotExist) {
		return docport.UserCreation{}, editing.ErrForbidden
	}
	if input.CheckCurrent() != nil {
		return docport.UserCreation{}, editing.ErrForbidden
	}
	replace := s.replaceDocument
	if replace == nil {
		replace = atomicReplaceText
	}
	err = replace(atomicTextReplaceRequest{Path: target.path, Content: input.Content, MaxBytes: MaxOfficeObjectBytes, ReadPolicy: atomicTextReadPolicy{RequireSingleLink: true}, ExpectedExists: false, CreateParents: false, DefaultMode: 0600})
	if err != nil {
		if errors.Is(err, ErrAtomicTextBeforeDrift) || errors.Is(err, os.ErrExist) {
			return conflict()
		}
		return docport.UserCreation{}, &docport.UserCreationUnknown{OperationID: r.Operation, Revision: r.Revision}
	}
	r.Status, r.SavedAt = "committed", time.Now().UTC().Format(time.RFC3339Nano)
	committed, _ := json.Marshal(r)
	if s.writeNativePrivate(journalPath, committed, digestAtomicText(body), 16384) != nil {
		return docport.UserCreation{}, &docport.UserCreationUnknown{OperationID: r.Operation, Revision: r.Revision}
	}
	return docport.UserCreation{OperationID: r.Operation, Workspace: target.workspace, Path: target.path, Revision: r.Revision, Kind: "docx", ByteSize: r.ByteSize, SavedAt: r.SavedAt}, nil
}
