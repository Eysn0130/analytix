package objectediting

import (
	"context"
	"errors"
	"reflect"
	"testing"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type workbookManualTestFiles struct {
	*nativeRecoveryTestFiles
	eligibility fileport.NativeWorkbookEditingInput
	validation  fileport.NativeWorkbookManualValidationInput
	manual      fileport.NativeWorkbookManualInput
}

func (f *workbookManualTestFiles) NativeWorkbookEditing(_ context.Context, input fileport.NativeWorkbookEditingInput) error {
	f.eligibility = input
	f.observed()
	return nil
}
func (f *workbookManualTestFiles) ValidateNativeWorkbookManual(_ context.Context, input fileport.NativeWorkbookManualValidationInput) (*office.WorkbookReview, error) {
	f.validation = input
	f.observed()
	return &office.WorkbookReview{Before: input.Selection}, nil
}
func (f *workbookManualTestFiles) PrepareNativeWorkbookManual(_ context.Context, input fileport.NativeWorkbookManualInput) (fileport.NativeChangeStatus, error) {
	f.manual = input
	f.observed()
	return fileport.NativeChangeStatus{Version: 3, Origin: "manual", Workbook: input.Draft.Workbook}, nil
}

func TestWorkbookManualServiceUsesOnlyLiveSessionFileAuthority(t *testing.T) {
	s, _, base, opened := nativeRecoveryServiceFixture(t)
	files := &workbookManualTestFiles{nativeRecoveryTestFiles: base}
	s.files = files
	ctx := context.Background()
	target := office.WorkbookWriteTarget{SheetName: "Sheet1", Row: 2}
	edit := office.WorkbookManualEdit{Type: "formula", Formula: "SUM(A1:A2)"}
	selected := office.WorkbookSelection{SheetName: "Sheet1", EndRow: 2}
	if err := s.NativeWorkbookEditing(ctx, opened.SessionID, opened.Revision); err != nil {
		t.Fatal(err)
	}
	if files.eligibility != (fileport.NativeWorkbookEditingInput{Workspace: "/workspace", Path: opened.Path, ObjectIdentity: opened.ObjectID, BaseRevision: opened.Revision}) {
		t.Fatal("eligibility changed authority", files.eligibility)
	}
	if _, err := s.ValidateNativeWorkbookManual(ctx, opened.SessionID, opened.Revision, selected, target, edit); err != nil {
		t.Fatal(err)
	}
	want := fileport.NativeWorkbookManualValidationInput{Workspace: "/workspace", Path: opened.Path, ObjectIdentity: opened.ObjectID, BaseRevision: opened.Revision, Selection: selected, WriteTarget: target, Edit: edit}
	if !reflect.DeepEqual(files.validation, want) {
		t.Fatal("validation changed authority", files.validation)
	}
	draft := fileport.NativeWorkbookManualDraft{WriteTarget: target, Edit: edit, Workbook: &office.WorkbookReview{Before: selected}}
	if _, err := s.PrepareNativeWorkbookManual(ctx, opened.SessionID, draft); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files.manual, fileport.NativeWorkbookManualInput{Workspace: "/workspace", Path: opened.Path, ObjectIdentity: opened.ObjectID, Draft: draft}) {
		t.Fatal("manual prepare changed authority", files.manual)
	}
	if files.commits != 0 {
		t.Fatal("typed preparation wrote via generic commit")
	}
}

func TestWorkbookManualServiceDoesNotExposeResultsAfterIdentityLoss(t *testing.T) {
	for _, operation := range []string{"eligibility", "validation", "prepare"} {
		t.Run(operation, func(t *testing.T) {
			s, identity, base, opened := nativeRecoveryServiceFixture(t)
			files := &workbookManualTestFiles{nativeRecoveryTestFiles: base}
			s.files = files
			files.onNative = func() { identity.unavailable = true }
			ctx := context.Background()
			switch operation {
			case "eligibility":
				if err := s.NativeWorkbookEditing(ctx, opened.SessionID, opened.Revision); !errors.Is(err, ErrUnavailable) {
					t.Fatal(err)
				}
			case "validation":
				value, err := s.ValidateNativeWorkbookManual(ctx, opened.SessionID, opened.Revision, office.WorkbookSelection{}, office.WorkbookWriteTarget{}, office.WorkbookManualEdit{})
				if !errors.Is(err, ErrUnavailable) || value != nil {
					t.Fatal("review escaped revoked identity", value, err)
				}
			case "prepare":
				value, err := s.PrepareNativeWorkbookManual(ctx, opened.SessionID, fileport.NativeWorkbookManualDraft{Workbook: &office.WorkbookReview{}})
				if !errors.Is(err, ErrUnavailable) || value != (fileport.NativeChangeStatus{}) {
					t.Fatal("private manual authority escaped revoked identity", value, err)
				}
			}
		})
	}
}
