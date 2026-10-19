//go:build darwin || linux

package filestore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	officeapp "analytix.local/runtime-go/internal/app/officeediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
	"github.com/xuri/excelize/v2"
)

// Real OPC bytes exercise typed local authority, verifier, CAS and restart Undo.
// Candidate generation is a fixture; native worker/UI execution is separate QA.
func TestOfficeWorkbookManualTypedSaveRestartUndo(t *testing.T) {
	for _, name := range []string{"number", "formula", "text", "empty", "formula_to_number", "formula_to_text", "formula_to_empty"} {
		t.Run(name, func(t *testing.T) {
			kind := strings.TrimPrefix(name, "formula_to_")
			oldFormula := strings.HasPrefix(name, "formula_to_")
			ctx := context.Background()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			book := excelize.NewFile()
			defer book.Close()
			must(book.SetCellFloat("Sheet1", "A1", 1, -1, 64))
			must(book.SetCellFloat("Sheet1", "A2", 2, -1, 64))
			must(book.SetCellStr("Sheet1", "D4", "outside"))
			if oldFormula {
				must(book.SetCellFormula("Sheet1", "A3", "SUM(A1:A2)"))
			}
			buffer, err := book.WriteToBuffer()
			must(err)
			original := append([]byte(nil), buffer.Bytes()...)
			target := office.WorkbookWriteTarget{SheetName: "Sheet1"}
			selected := office.WorkbookSelection{SheetName: "Sheet1", Cells: []office.WorkbookSelectedCell{{ValueType: "number", Value: 1, Text: "1", Formula: "1", RowVisible: true, ColumnVisible: true}}}
			address := "A1"
			if oldFormula {
				target.Row, address = 2, "A3"
				selected.StartRow, selected.EndRow = 2, 2
				selected.Cells = []office.WorkbookSelectedCell{{Row: 2, ValueType: "formula", Value: 3, Text: "3", Formula: "=SUM(A1:A2)", RowVisible: true, ColumnVisible: true}}
			}
			edit := office.WorkbookManualEdit{Type: kind}
			switch kind {
			case "number":
				value := 4.0
				edit.Value = &value
				must(book.SetCellFloat("Sheet1", address, 4, -1, 64))
			case "formula":
				target.Row = 2
				edit.Formula = "SUM(A1:A2)"
				selected.EndRow = 2
				selected.Cells = append(selected.Cells, office.WorkbookSelectedCell{Row: 1, ValueType: "number", Value: 2, Text: "2", Formula: "2", RowVisible: true, ColumnVisible: true}, office.WorkbookSelectedCell{Row: 2, ValueType: "empty", RowVisible: true, ColumnVisible: true})
				must(book.SetCellFormula("Sheet1", "A3", edit.Formula))
			case "text":
				value := "PRIVATE local text"
				edit.Text = &value
				must(book.SetCellStr("Sheet1", address, value))
			case "empty":
				must(book.SetCellDefault("Sheet1", address, ""))
			}
			buffer, err = book.WriteToBuffer()
			must(err)
			candidate := append([]byte(nil), buffer.Bytes()...)
			must(book.SetCellStr("Sheet1", "D4", "changed outside"))
			buffer, err = book.WriteToBuffer()
			must(err)
			outsideCandidate := append([]byte(nil), buffer.Bytes()...)
			files, workspace, path := officeEditingNativeFixture(t, "xlsx", original)
			principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
			must(err)
			identity := &officeTestIdentity{principal: principal}
			leases, writes := 0, 0
			bind := func() *officeapp.Adapter {
				files.replaceDocument = func(r atomicTextReplaceRequest) error {
					if leases != 1 {
						t.Fatal("CAS without exact manual capture", leases)
					}
					writes++
					return atomicReplaceText(r)
				}
				a := officeapp.New("xlsx", editingapp.New(identity, files), func(context.Context) bool { return true })
				capture := func(_ context.Context, _ string, p string, read func() error) (func(), error) {
					if p != path {
						t.Fatal("changed path")
					}
					if err := read(); err != nil {
						return nil, err
					}
					leases++
					return func() { leases-- }, nil
				}
				must(a.BindSelectionHost(officeRecoveryTestProjector{identity}, capture))
				must(a.BindManualHost(capture))
				return a
			}
			adapter := bind()
			call := func(op string, input map[string]any) map[string]any {
				t.Helper()
				body, err := json.Marshal(input)
				must(err)
				result, err := adapter.Invoke(ctx, adapterport.Call{Admit: func() error { return nil }, Binding: adapterport.Binding{PackageID: "analytix-spreadsheets"}, Principal: principal, ContributionID: "workspace-editor", Operation: op, Input: body})
				must(err)
				var out map[string]any
				must(json.Unmarshal(result.Output, &out))
				return out
			}
			ok := func(out map[string]any) map[string]any {
				t.Helper()
				if out["ok"] != true {
					t.Fatal(out)
				}
				return out
			}
			opened := ok(call("open-object", map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}))
			if opened["workbookEditing"].(map[string]any)["editable"] != true {
				t.Fatal("simple workbook read only", opened["workbookEditing"])
			}
			doc := opened["document"].(map[string]any)
			input := map[string]any{"sessionId": doc["sessionId"], "threadId": "thread_main", "baseRevision": doc["revision"], "writeTarget": target, "edit": edit}
			planned := ok(call("plan-workbook-manual", input))["dependencyBounds"].(map[string]any)
			if planned["startRow"] != float64(selected.StartRow) || planned["endRow"] != float64(selected.EndRow) || writes != 0 || leases != 0 {
				t.Fatal("plan changed authority", planned, writes, leases)
			}
			input["operationId"] = "manual_typed_001"
			input["selectionToken"] = "native_manual_001"
			input["changeSequence"] = 0
			input["workbook"] = selected
			forged := selected
			forged.Cells = append([]office.WorkbookSelectedCell(nil), selected.Cells...)
			forged.Cells[0].Value = 999
			if oldFormula {
				forged.Cells[0].Formula = "=SUM(A1:A1)"
			}
			input["workbook"] = forged
			if out := call("prepare-workbook-manual", input); out["ok"] == true || writes != 0 || leases != 0 {
				t.Fatal("unverified capture granted manual authority", out, writes, leases)
			}
			input["workbook"] = selected
			manual := ok(call("prepare-workbook-manual", input))["workbookManual"].(map[string]any)
			if manual["origin"] != "manual" || manual["version"] != float64(3) || manual["proposalId"] != nil || writes != 0 || leases != 1 {
				t.Fatal("manual authority", manual, writes, leases)
			}
			envelope := func(body []byte) map[string]any {
				return map[string]any{"encoding": "base64", "kind": "xlsx", "byteLength": len(body), "sha256": digestAtomicText(body), "data": base64.StdEncoding.EncodeToString(body)}
			}
			commit := map[string]any{"sessionId": doc["sessionId"], "threadId": "thread_main", "changeId": manual["changeId"], "operationId": manual["saveOperationId"], "baseRevision": doc["revision"], "content": envelope(original)}
			if out := call("commit-object", commit); out["ok"] == true || writes != 0 {
				t.Fatal("unapplied export saved", out)
			}
			commit["content"] = envelope(outsideCandidate)
			if out := call("commit-object", commit); out["ok"] == true || writes != 0 {
				t.Fatal("manual export expanded write target", out)
			}
			commit["content"] = envelope(candidate)
			ok(call("commit-object", commit))
			officeEditingAssertBytes(t, path, candidate)
			if writes != 1 || leases != 0 {
				t.Fatal("save CAS/capture", writes, leases)
			}
			ok(call("close-object", map[string]any{"sessionId": doc["sessionId"]}))
			files, err = NewOfficeObjectEditingFiles(files.receiptRoot, nil, "xlsx")
			must(err)
			adapter = bind()
			next := ok(call("open-object", map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}))["document"].(map[string]any)
			recovery := ok(call("object-recovery", map[string]any{"sessionId": next["sessionId"], "threadId": "thread_main"}))["recovery"].(map[string]any)["current"].(map[string]any)
			if recovery["version"] != float64(3) || recovery["origin"] != "manual" || recovery["workbook"] == nil || recovery["proposalId"] != nil || recovery["beforeText"] != nil || recovery["canUndo"] != true {
				t.Fatal("recovery variant", recovery)
			}
			ok(call("undo-change", map[string]any{"sessionId": next["sessionId"], "threadId": "thread_main", "changeId": manual["changeId"], "baseRevision": next["revision"]}))
			officeEditingAssertBytes(t, path, original)
			if writes != 2 || leases != 0 {
				t.Fatal("undo CAS/capture", writes, leases)
			}
		})
	}
}

func TestWorkbookManualRecordV3DoesNotWidenV2OrCompactContent(t *testing.T) {
	manual := editing.NativeManualDraft{ChangeID: strings.Repeat("a", 64), ThreadID: "thread_main", BaseRevision: strings.Repeat("b", 64), BeginOperationID: "manual_variant_01"}
	v2 := nativeRecoveryRecord{Version: 2, Kind: "xlsx", Manual: &manual, Draft: editing.NativeChangeDraft{ChangeID: manual.ChangeID, ThreadID: manual.ThreadID, BaseRevision: manual.BaseRevision}}
	if nativeRecordVariantValid(v2, "xlsx") {
		t.Fatal("v2 manual widened to workbook")
	}
	text := strings.Repeat("\x01", 4096)
	draft := editing.NativeWorkbookManualDraft{NativeManualDraft: manual, WriteTarget: office.WorkbookWriteTarget{SheetName: "Sheet1"}, Edit: office.WorkbookManualEdit{Type: "text", Text: &text}}
	record := nativeRecoveryRecord{Version: 3, WorkbookManual: &draft, Draft: editing.NativeChangeDraft{ChangeID: manual.ChangeID, ThreadID: manual.ThreadID, BaseRevision: manual.BaseRevision}}
	compact := nativeCompact(record, "")
	body, err := json.Marshal(compact)
	if err != nil || len(body) > 4096 || compact.WorkbookManual.Edit.Text != nil || compact.WorkbookManual.Workbook != nil || !nativeRecordVariantValid(compact, "xlsx") {
		t.Fatal("compact leaked workbook content", len(body), err)
	}
}
