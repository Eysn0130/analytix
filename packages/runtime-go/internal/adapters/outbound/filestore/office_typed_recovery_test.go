//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	officeapp "analytix.local/runtime-go/internal/app/officeediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	editingport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
	"github.com/xuri/excelize/v2"
)

// This exercises real XLSX packages and the production authorization, CAS and
// recovery chain. The candidate bytes represent an engine export; no UNO GUI
// execution is claimed by this fixture.
func TestOfficeTypedWorkbookApprovalSaveRestartUndo(t *testing.T) {
	for _, kind := range []string{"number", "formula", "range"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			book := excelize.NewFile()
			defer book.Close()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(book.SetCellFloat("Sheet1", "A1", 1, -1, 64))
			must(book.SetCellFloat("Sheet1", "A2", 2, -1, 64))
			must(book.SetCellStr("Sheet1", "D4", "outside"))
			originalBuffer, err := book.WriteToBuffer()
			must(err)
			original := append([]byte(nil), originalBuffer.Bytes()...)
			selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
			n := 4.0
			patch := office.WorkbookPatch{Kind: kind, Value: &n}
			switch kind {
			case "number":
				must(book.SetCellFloat("Sheet1", "A1", 4, -1, 64))
			case "formula":
				patch.Value = nil
				patch.Formula = "1+2"
				must(book.SetCellFormula("Sheet1", "A1", "1+2"))
			case "range":
				selected.EndRow = 2
				selected.Cells = append(selected.Cells, office.WorkbookSelectedCell{Row: 1, ValueType: "number", Value: 2, Text: "2", Formula: "2", RowVisible: true, ColumnVisible: true}, office.WorkbookSelectedCell{Row: 2, ValueType: "empty", RowVisible: true, ColumnVisible: true})
				patch.Value = nil
				patch.Cells = []office.WorkbookCellEdit{{Type: "number", Value: &n}, {RowOffset: 2, Type: "formula", Formula: "SUM(A1:A2)"}}
				must(book.SetCellFloat("Sheet1", "A1", 4, -1, 64))
				must(book.SetCellFormula("Sheet1", "A3", "SUM(A1:A2)"))
			}
			candidateBuffer, err := book.WriteToBuffer()
			must(err)
			candidate := append([]byte(nil), candidateBuffer.Bytes()...)
			files, workspace, path := officeEditingNativeFixture(t, "xlsx", original)
			principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
			must(err)
			identity := &officeTestIdentity{principal: principal}
			leases, writes := 0, 0
			bind := func() *officeapp.Adapter {
				files.replaceDocument = func(r atomicTextReplaceRequest) error {
					if leases != 1 {
						t.Fatal("CAS without capture", leases)
					}
					writes++
					return atomicReplaceText(r)
				}
				a := officeapp.New("xlsx", editingapp.New(identity, files), func(context.Context) bool { return true })
				must(a.BindSelectionHost(officeRecoveryTestProjector{identity}, func(_ context.Context, _ string, p string, read func() error) (func(), error) {
					if p != path {
						t.Fatal("scope path changed")
					}
					if e := read(); e != nil {
						return nil, e
					}
					leases++
					return func() { leases-- }, nil
				}))
				return a
			}
			adapter := bind()
			call := func(operation string, input map[string]any) map[string]any {
				t.Helper()
				body, e := json.Marshal(input)
				must(e)
				result, e := adapter.Invoke(ctx, adapterport.Call{Binding: adapterport.Binding{PackageID: "analytix-spreadsheets"}, Principal: identity.principal, ContributionID: "workspace-editor", Operation: operation, Input: body})
				must(e)
				var out map[string]any
				must(json.Unmarshal(result.Output, &out))
				return out
			}
			success := func(out map[string]any) map[string]any {
				t.Helper()
				if out["ok"] != true {
					t.Fatal(out)
				}
				return out
			}
			opened := success(call("open-object", map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}))["document"].(map[string]any)
			captureInput := map[string]any{"sessionId": opened["sessionId"], "threadId": "thread_main", "selectionToken": "typed_selection_01", "changeSequence": 0, "baseRevision": opened["revision"], "text": "1", "editable": true, "workbook": selected}
			scope := success(call("capture-selection", captureInput))["scope"].(map[string]any)
			proposalInput := map[string]any{"scopeId": scope["scopeId"], "threadId": "other_thread", "operationId": "typed_proposal_01", "workbook": patch}
			if out := call("model-selection-propose", proposalInput); out["ok"] == true {
				t.Fatal("cross-thread proposal accepted")
			}
			proposalInput["threadId"] = "thread_main"
			proposal := success(call("model-selection-propose", proposalInput))["result"].(map[string]any)
			if proposal["review"] != nil || proposal["results"] != nil || proposal["before"] != nil {
				t.Fatal("local typed diff entered model reply", proposal)
			}
			reviews := success(call("proposal-read", map[string]any{"sessionId": opened["sessionId"], "scopeId": scope["scopeId"]}))["localReviews"].([]any)
			if len(reviews) != 1 || reviews[0].(map[string]any)["workbook"] == nil {
				t.Fatal("typed local diff missing")
			}
			approved := success(call("proposal-accept", map[string]any{"sessionId": opened["sessionId"], "scopeId": scope["scopeId"], "proposalId": proposal["proposalId"], "operationId": "typed_approve_01", "selectionToken": scope["selectionToken"], "changeSequence": scope["changeSequence"], "baseRevision": scope["baseRevision"]}))["replacement"].(map[string]any)
			if approved["workbook"] == nil || writes != 0 || approved["saveOperationId"] != nativeSaveID(approved["changeId"].(string)) {
				t.Fatal("approval authority lost", approved, writes)
			}
			envelope := func(body []byte) map[string]any {
				return map[string]any{"encoding": "base64", "kind": "xlsx", "byteLength": len(body), "sha256": digestAtomicText(body), "data": base64.StdEncoding.EncodeToString(body)}
			}
			commit := map[string]any{"sessionId": opened["sessionId"], "threadId": "thread_main", "changeId": approved["changeId"], "operationId": approved["saveOperationId"], "baseRevision": opened["revision"], "content": envelope(original)}
			if out := call("commit-object", commit); out["ok"] == true || writes != 0 {
				t.Fatal("unapplied candidate saved", out, writes)
			}
			commit["content"] = envelope(candidate)
			success(call("commit-object", commit))
			if writes != 1 {
				t.Fatal("save count", writes)
			}
			officeEditingAssertBytes(t, path, candidate)
			reopenedBook, err := excelize.OpenReader(bytes.NewReader(candidate))
			must(err)
			defer reopenedBook.Close()
			outside, err := reopenedBook.GetCellValue("Sheet1", "D4")
			must(err)
			if outside != "outside" {
				t.Fatal("outside-range content changed")
			}
			switch kind {
			case "number":
				v, e := reopenedBook.GetCellValue("Sheet1", "A1")
				must(e)
				if v != "4" {
					t.Fatal(v)
				}
			case "formula":
				f, e := reopenedBook.GetCellFormula("Sheet1", "A1")
				must(e)
				v, e := reopenedBook.CalcCellValue("Sheet1", "A1")
				must(e)
				if f != "1+2" || v != "3" {
					t.Fatal(f, v)
				}
			case "range":
				f, e := reopenedBook.GetCellFormula("Sheet1", "A3")
				must(e)
				v, e := reopenedBook.CalcCellValue("Sheet1", "A3")
				must(e)
				if f != "SUM(A1:A2)" || v != "6" {
					t.Fatal(f, v)
				}
			}
			success(call("close-object", map[string]any{"sessionId": opened["sessionId"]}))
			if leases != 0 {
				t.Fatal("leaked lease")
			}
			files, err = NewOfficeObjectEditingFiles(files.receiptRoot, nil, "xlsx")
			must(err)
			adapter = bind()
			next := success(call("open-object", map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}))["document"].(map[string]any)
			recovery := success(call("object-recovery", map[string]any{"sessionId": next["sessionId"], "threadId": "thread_main"}))["recovery"].(map[string]any)["current"].(map[string]any)
			if recovery["workbook"] == nil || recovery["canUndo"] != true || recovery["changeId"] != approved["changeId"] {
				t.Fatal("typed recovery lost", recovery)
			}
			success(call("undo-change", map[string]any{"sessionId": next["sessionId"], "threadId": "thread_main", "changeId": approved["changeId"], "baseRevision": next["revision"]}))
			officeEditingAssertBytes(t, path, original)
			if writes != 2 || leases != 0 {
				t.Fatal("undo CAS/capture", writes, leases)
			}
		})
	}
}

func TestOfficeTypedWorkbookKeepsLegacyDraftCanonicalShape(t *testing.T) {
	d := editingport.NativeChangeDraft{ChangeID: strings.Repeat("a", 64), ThreadID: "thread_main", ProposalID: strings.Repeat("b", 48), BaseRevision: strings.Repeat("c", 64), BeforeText: "before", AfterText: "after"}
	old := struct{ ChangeID, ThreadID, ProposalID, BaseRevision, BeforeText, AfterText string }{d.ChangeID, d.ThreadID, d.ProposalID, d.BaseRevision, d.BeforeText, d.AfterText}
	body, _ := json.Marshal(old)
	current, _ := json.Marshal(d)
	if !bytes.Equal(body, current) || nativeDraftHash(d) != digestAtomicText(body) {
		t.Fatal("legacy native draft digest changed")
	}
	var decoded editingport.NativeChangeDraft
	if e := nativeDecode(body, &decoded, 1<<20); e != nil {
		t.Fatal("old draft rejected", e)
	}
	bad := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"workbook":null}`)...)
	if nativeDecode(bad, &decoded, 1<<20) == nil {
		t.Fatal("explicit null typed field admitted into historical shape")
	}
}
