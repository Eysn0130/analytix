//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	office "analytix.local/runtime-go/internal/domain/officegeneration"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
	"github.com/xuri/excelize/v2"
)

func nativeWorkbookCandidateFixture(t *testing.T, manual bool) (*ObjectEditingFiles, editing.NativeWorkbookCandidateInput, []byte) {
	t.Helper()
	ctx := context.Background()
	original, err := base64.StdEncoding.Strict().DecodeString(nativeWorkbookCandidateOriginalBase64)
	if err != nil || len(original) != 5204 || digestAtomicText(original) != "888bbedffed2e064e935c43a3dd5748ee00d26d41808834338d761e164cdf6f8" {
		t.Fatal("registered original XLSX fixture identity changed", err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	files, workspace, path := officeEditingNativeFixture(t, "xlsx", original)
	commit := objectEditingInput(t, files, workspace, path, "")
	input := editing.NativeWorkbookCandidateInput{
		Workspace: workspace, Path: path, ObjectIdentity: commit.ObjectIdentity,
		ThreadID: "thread_candidate", ChangeID: strings.Repeat("c", 64), BaseRevision: commit.BaseRevision,
	}
	input.OperationID = nativeSaveID(input.ChangeID)
	address, column, row := "A1", 0, 0
	if manual {
		address, column, row = "C2", 2, 1
	}
	valueText, err := book.GetCellValue("Sheet1", address, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil || manual && value != 11 {
		t.Fatal("registered source selection is not the expected numeric cell", valueText, err)
	}
	selected := office.WorkbookSelection{SheetName: "Sheet1", StartColumn: column, EndColumn: column, StartRow: row, EndRow: row,
		Cells: []office.WorkbookSelectedCell{{Column: column, Row: row, ValueType: "number", Value: value, Text: valueText, Formula: valueText, RowVisible: true, ColumnVisible: true}}}
	var prepared editing.NativeChangeStatus
	if manual {
		text := "001"
		target := office.WorkbookWriteTarget{SheetName: "Sheet1", Column: column, Row: row}
		edit := office.WorkbookManualEdit{Type: "text", Text: &text}
		review, e := files.ValidateNativeWorkbookManual(ctx, editing.NativeWorkbookManualValidationInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, BaseRevision: input.BaseRevision, Selection: selected, WriteTarget: target, Edit: edit})
		if e != nil {
			t.Fatal(e)
		}
		prepared, err = files.PrepareNativeWorkbookManual(ctx, editing.NativeWorkbookManualInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, Draft: editing.NativeWorkbookManualDraft{
			NativeManualDraft: editing.NativeManualDraft{ChangeID: input.ChangeID, ThreadID: input.ThreadID, BaseRevision: input.BaseRevision, BeginOperationID: "candidate_begin_01"},
			WriteTarget:       target, Edit: edit, Workbook: review,
		}})
	} else {
		updated := 42.0
		review, e := files.ValidateNativeWorkbook(ctx, editing.NativeWorkbookInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, BaseRevision: input.BaseRevision, Selection: selected, Patch: &office.WorkbookPatch{Kind: "number", Value: &updated}})
		if e != nil {
			t.Fatal(e)
		}
		prepared, err = files.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, Draft: editing.NativeChangeDraft{
			ChangeID: input.ChangeID, ThreadID: input.ThreadID, ProposalID: strings.Repeat("a", 48), BaseRevision: input.BaseRevision,
			BeforeText: valueText, AfterText: "42", Workbook: review,
		}})
	}
	if err != nil || prepared.Status != "prepared" || prepared.Workbook == nil || prepared.SaveOperationID != input.OperationID {
		t.Fatal("prepare candidate fixture", prepared, err)
	}
	return files, input, original
}

type nativeWorkbookCandidateFile struct {
	Content string
	Mode    os.FileMode
}

func nativeWorkbookCandidateSnapshot(t *testing.T, files *ObjectEditingFiles, path string) map[string]nativeWorkbookCandidateFile {
	t.Helper()
	result := map[string]nativeWorkbookCandidateFile{}
	paths := []string{path}
	entries, err := os.ReadDir(files.receiptRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		paths = append(paths, filepath.Join(files.receiptRoot, entry.Name()))
	}
	for _, file := range paths {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		result[file] = nativeWorkbookCandidateFile{Content: string(body), Mode: info.Mode()}
	}
	return result
}

func nativeWorkbookCandidateBuildReadOnly(t *testing.T, files *ObjectEditingFiles, input editing.NativeWorkbookCandidateInput) (editing.NativeWorkbookCandidate, error) {
	t.Helper()
	before := nativeWorkbookCandidateSnapshot(t, files, input.Path)
	previousDocument, previousJournal := files.replaceDocument, files.replaceJournal
	refuse := func(atomicTextReplaceRequest) error {
		t.Fatal("candidate build attempted a write")
		return editing.ErrPersistence
	}
	files.replaceDocument, files.replaceJournal = refuse, refuse
	defer func() { files.replaceDocument, files.replaceJournal = previousDocument, previousJournal }()
	result, err := files.BuildNativeWorkbookCandidate(context.Background(), input)
	if !reflect.DeepEqual(before, nativeWorkbookCandidateSnapshot(t, files, input.Path)) {
		t.Fatal("candidate build changed target, metadata or recovery files")
	}
	return result, err
}

// These fixtures use real XLSX packages, the production verifier and CAS/undo.
// They do not establish native engine, Provider or desktop acceptance.
func TestNativeWorkbookCandidateBuildThenExplicitCommitAndUndo(t *testing.T) {
	for _, manual := range []bool{false, true} {
		name := "agent"
		if manual {
			name = "manual"
		}
		t.Run(name, func(t *testing.T) {
			files, input, original := nativeWorkbookCandidateFixture(t, manual)
			source, err := excelize.OpenReader(bytes.NewReader(original))
			if err != nil {
				t.Fatal(err)
			}
			outsideExpected, err := source.GetCellValue("Sheet1", "B1", excelize.Options{RawCellValue: true})
			source.Close()
			if err != nil {
				t.Fatal(err)
			}
			address := "A1"
			if manual {
				address = "C2"
			}
			var first editing.NativeWorkbookCandidate
			for attempt := 0; attempt < 2; attempt++ {
				candidate, err := nativeWorkbookCandidateBuildReadOnly(t, files, input)
				if err != nil || candidate.ChangeID != input.ChangeID || candidate.SaveOperationID != input.OperationID || candidate.BaseRevision != input.BaseRevision || candidate.Content.Encoding != "base64" || candidate.Content.Kind != "xlsx" {
					t.Fatal("build", candidate, err)
				}
				body, err := base64.StdEncoding.Strict().DecodeString(candidate.Content.Data)
				if err != nil || len(body) != candidate.Content.ByteLength || digestAtomicText(body) != candidate.Content.SHA256 {
					t.Fatal("candidate envelope does not match bytes", err)
				}
				book, err := excelize.OpenReader(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				value, valueErr := book.GetCellValue("Sheet1", address, excelize.Options{RawCellValue: true})
				typ, typeErr := book.GetCellType("Sheet1", address)
				outside, outsideErr := book.GetCellValue("Sheet1", "B1", excelize.Options{RawCellValue: true})
				book.Close()
				if valueErr != nil || typeErr != nil || outsideErr != nil || outside != outsideExpected || manual && (value != "001" || typ != excelize.CellTypeSharedString && typ != excelize.CellTypeInlineString) || !manual && value != "42" {
					t.Fatal("candidate did not apply the persisted typed review", value, typ, valueErr, typeErr, outsideErr)
				}
				if attempt == 0 {
					first = candidate
				}
			}
			if _, _, err := files.readRecord(input.ObjectIdentity, input.OperationID); !errors.Is(err, editing.ErrOperationNotFound) {
				t.Fatal("build created a save journal", err)
			}
			if _, err := os.Stat(files.nativePath(input.ObjectIdentity, input.ChangeID, ".after")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("build created a recovery candidate", err)
			}
			officeEditingAssertBytes(t, input.Path, original)
			receipt, err := files.CommitNativeChange(context.Background(), editing.NativeCommitInput{ThreadID: input.ThreadID, ChangeID: input.ChangeID, CommitInput: editing.CommitInput{
				Workspace: input.Workspace, Path: input.Path, ObjectIdentity: input.ObjectIdentity, BaseRevision: input.BaseRevision, OperationID: input.OperationID, Content: first.Content.Data,
			}})
			if err != nil || receipt.Status != editing.StatusCommitted || receipt.Revision != first.Content.SHA256 || receipt.SavedAt == "" {
				t.Fatal("explicit commit", receipt, err)
			}
			if _, err = nativeWorkbookCandidateBuildReadOnly(t, files, input); err == nil {
				t.Fatal("committed change produced another candidate")
			}
			restarted, err := NewOfficeObjectEditingFiles(files.receiptRoot, nil, "xlsx")
			if err != nil {
				t.Fatal(err)
			}
			undone, err := restarted.UndoNativeChange(context.Background(), editing.NativeUndoInput{Workspace: input.Workspace, Path: input.Path, ObjectIdentity: input.ObjectIdentity, ThreadID: input.ThreadID, ChangeID: input.ChangeID, BaseRevision: receipt.Revision})
			if err != nil || undone.Status != editing.StatusCommitted || undone.Revision != input.BaseRevision {
				t.Fatal("undo generated candidate", undone, err)
			}
			officeEditingAssertBytes(t, input.Path, original)
		})
	}
}

func TestNativeWorkbookCandidateBuildRetainsCancelability(t *testing.T) {
	for _, manual := range []bool{false, true} {
		files, input, original := nativeWorkbookCandidateFixture(t, manual)
		if _, err := nativeWorkbookCandidateBuildReadOnly(t, files, input); err != nil {
			t.Fatal(err)
		}
		recovery, err := files.NativeRecovery(context.Background(), input.ObjectIdentity, input.Workspace, input.Path, input.ThreadID)
		if err != nil || recovery.Pending == nil || recovery.Pending.Status != "prepared" || !recovery.Pending.CanCancel || recovery.Pending.CanResume {
			t.Fatal("build changed prepared recovery semantics", recovery, err)
		}
		cancelled, err := files.CancelNativeChange(context.Background(), editing.NativeUndoInput{Workspace: input.Workspace, Path: input.Path, ObjectIdentity: input.ObjectIdentity, ThreadID: input.ThreadID, ChangeID: input.ChangeID, BaseRevision: input.BaseRevision})
		if err != nil || cancelled.Pending != nil {
			t.Fatal("candidate build prevented cancellation", cancelled, err)
		}
		officeEditingAssertBytes(t, input.Path, original)
	}
}

func nativeWorkbookCandidateMutateRecord(t *testing.T, files *ObjectEditingFiles, input editing.NativeWorkbookCandidateInput, mutate func(*nativeRecoveryRecord)) {
	t.Helper()
	target, err := files.target(input.Workspace, input.Path)
	if err != nil {
		t.Fatal(err)
	}
	record, hash, err := files.nativeRecord(target, input.ObjectIdentity, input.ChangeID)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&record)
	if err = files.writeNativeRecord(record, hash); err != nil {
		t.Fatal(err)
	}
}

func TestNativeWorkbookCandidateRefusesForgedAndStartedChangesWithoutWrites(t *testing.T) {
	for _, name := range []string{"thread", "base", "identity", "change", "operation", "source", "journal", "after_hash", "undo", "retired", "untyped", "preparing", "retiring", "original"} {
		t.Run(name, func(t *testing.T) {
			files, input, _ := nativeWorkbookCandidateFixture(t, false)
			switch name {
			case "thread":
				input.ThreadID = "another_thread"
			case "base":
				input.BaseRevision = strings.Repeat("f", 64)
			case "identity":
				input.ObjectIdentity = strings.Repeat("f", 64)
			case "change":
				input.ChangeID = strings.Repeat("f", 64)
				input.OperationID = nativeSaveID(input.ChangeID)
			case "operation":
				input.OperationID = "candidate_forged_save"
			case "source":
				if err := os.WriteFile(input.Path, officeEditingNativeBytes(t, "xlsx", "external"), 0600); err != nil {
					t.Fatal(err)
				}
			case "journal":
				target, err := files.target(input.Workspace, input.Path)
				if err != nil {
					t.Fatal(err)
				}
				journal := objectEditingRecord{Version: 1, ObjectIdentity: input.ObjectIdentity, OperationID: input.OperationID, PathBinding: target.binding, RequestHash: strings.Repeat("d", 64), BeforeHash: input.BaseRevision, AfterHash: strings.Repeat("e", 64), Status: editing.StatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
				if err = files.writeRecord(journal, ""); err != nil {
					t.Fatal(err)
				}
			case "after_hash", "undo":
				nativeWorkbookCandidateMutateRecord(t, files, input, func(record *nativeRecoveryRecord) {
					record.AfterHash = strings.Repeat("f", 64)
					record.UndoStarted = name == "undo"
				})
			case "retired":
				nativeWorkbookCandidateMutateRecord(t, files, input, func(record *nativeRecoveryRecord) { *record = nativeCompact(*record, "cancelled") })
			case "untyped":
				nativeWorkbookCandidateMutateRecord(t, files, input, func(record *nativeRecoveryRecord) {
					record.Draft.Workbook = nil
					record.DraftHash = nativeDraftHash(record.Draft)
				})
			case "preparing", "retiring":
				target, err := files.target(input.Workspace, input.Path)
				if err != nil {
					t.Fatal(err)
				}
				index, hash, err := files.nativeIndex(target, input.ObjectIdentity)
				if err != nil {
					t.Fatal(err)
				}
				other := strings.Repeat("f", 64)
				if name == "preparing" {
					record, _, err := files.nativeRecord(target, input.ObjectIdentity, input.ChangeID)
					if err != nil {
						t.Fatal(err)
					}
					record.Draft.ChangeID, record.DraftHash = other, ""
					record = nativeCompact(record, "")
					index.Preparing = &record
				} else {
					index.Retiring = []string{other}
				}
				if err = files.writeNativeIndex(index, hash); err != nil {
					t.Fatal(err)
				}
			case "original":
				if err := os.WriteFile(files.nativePath(input.ObjectIdentity, input.ChangeID, ".before"), []byte("corrupt original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			candidate, err := nativeWorkbookCandidateBuildReadOnly(t, files, input)
			if err == nil || candidate != (editing.NativeWorkbookCandidate{}) {
				t.Fatal("invalid build returned a candidate", name, candidate, err)
			}
		})
	}
}

// Unmodified registered 5204-byte synthetic workbook; identity is asserted above.
const nativeWorkbookCandidateOriginalBase64 = "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" +
	"ZXNdLnhtbFBLBQYAAAAACgAKAIACAAC+EQAAAAA="
