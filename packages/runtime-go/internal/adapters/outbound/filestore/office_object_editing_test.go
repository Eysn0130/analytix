//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	officeapp "analytix.local/runtime-go/internal/app/officeediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// Build native OOXML entirely in the test. These are structural fixtures, not
// Office engine/GUI acceptance evidence. Existing package helpers own ZIP/OPC.
func officeEditingNativeBytes(t *testing.T, kind, text string) []byte {
	t.Helper()
	parts := officeTestParts(kind)
	switch kind {
	case "docx":
		parts[2].body = []byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`)
	case "xlsx":
		parts[2].body = []byte(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`)
		parts = append(parts,
			officeTestPart{name: "xl/_rels/workbook.xml.rels", body: officeTestRels(officeTestRelation("rId1", "worksheet", "worksheets/sheet1.xml"))},
			officeTestPart{name: "xl/worksheets/sheet1.xml", body: []byte(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>` + text + `</t></is></c></row></sheetData></worksheet>`)},
		)
		officeTestOverride(parts, "xl/worksheets/sheet1.xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml")
	case "pptx":
		parts[2].body = []byte(`<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst></p:presentation>`)
		parts = append(parts,
			officeTestPart{name: "ppt/_rels/presentation.xml.rels", body: officeTestRels(officeTestRelation("rId1", "slide", "slides/slide1.xml"))},
			officeTestPart{name: "ppt/slides/slide1.xml", body: []byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`)},
		)
		officeTestOverride(parts, "ppt/slides/slide1.xml", "application/vnd.openxmlformats-officedocument.presentationml.slide+xml")
	default:
		t.Fatal("unknown fixture kind")
	}
	raw := officeTestZIP(t, parts)
	if err := InspectOfficePackage(raw, kind); err != nil {
		t.Fatalf("invalid native fixture: %v", err)
	}
	return raw
}

func officeEditingNativeFixture(t *testing.T, kind string, raw []byte) (*ObjectEditingFiles, string, string) {
	t.Helper()
	textStore, workspace, textPath := objectEditingFixture(t, raw)
	path := filepath.Join(workspace, "report."+kind)
	if err := os.Rename(textPath, path); err != nil {
		t.Fatal(err)
	}
	store, err := NewOfficeObjectEditingFiles(textStore.receiptRoot, nil, kind)
	if err != nil {
		t.Fatal(err)
	}
	return store, workspace, path
}

func officeEditingAssertBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("native bytes changed unexpectedly: %v", err)
	}
}

func TestOfficeObjectEditingSaveReopenReplayAndConflict(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			before := officeEditingNativeBytes(t, kind, "Original-office-marker")
			after := officeEditingNativeBytes(t, kind, "Saved-office-marker")
			store, workspace, path := officeEditingNativeFixture(t, kind, before)
			input := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
			infoBefore, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := store.Commit(ctx, input)
			if err != nil || receipt.Status != objectediting.StatusCommitted || receipt.Revision != digestAtomicText(after) || receipt.OperationID != input.OperationID || receipt.SavedAt == "" {
				t.Fatalf("save: %#v %v", receipt, err)
			}
			officeEditingAssertBytes(t, path, after)
			infoAfter, err := os.Stat(path)
			if err != nil || infoBefore.Mode() != infoAfter.Mode() {
				t.Fatalf("mode changed: %v", err)
			}
			restarted, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, current := range []*ObjectEditingFiles{store, restarted} {
				doc, err := current.Read(ctx, workspace, path)
				if err != nil || doc.Content != base64.StdEncoding.EncodeToString(after) || doc.Encoding != "office-base64" || doc.Revision != receipt.Revision {
					t.Fatalf("reopen: %#v %v", doc, err)
				}
				replay, err := current.Commit(ctx, input)
				if err != nil || replay != receipt {
					t.Fatalf("replay: %#v %v", replay, err)
				}
				status, err := current.Status(ctx, input.ObjectIdentity, input.OperationID, workspace, path)
				if err != nil || status != receipt {
					t.Fatalf("status: %#v %v", status, err)
				}
			}
			changed := input
			changed.Content = base64.StdEncoding.EncodeToString(before)
			if _, err := restarted.Commit(ctx, changed); !errors.Is(err, objectediting.ErrOperationMismatch) {
				t.Fatalf("changed replay: %v", err)
			}
			officeEditingAssertBytes(t, path, after)
			external := officeEditingNativeBytes(t, kind, "External-change")
			if err := os.WriteFile(path, external, 0o640); err != nil {
				t.Fatal(err)
			}
			conflict := input
			conflict.OperationID = "conflict_02"
			conflict.BaseRevision = receipt.Revision
			result, err := restarted.Commit(ctx, conflict)
			if !errors.Is(err, objectediting.ErrConflict) || result.Status != objectediting.StatusConflict {
				t.Fatalf("CAS: %#v %v", result, err)
			}
			officeEditingAssertBytes(t, path, external)
			status, err := restarted.Status(ctx, input.ObjectIdentity, conflict.OperationID, workspace, path)
			if err != nil || status.Status != objectediting.StatusConflict {
				t.Fatalf("conflict status: %#v %v", status, err)
			}
		})
	}
}

func TestOfficeObjectEditingCodecBoundsAndEncoding(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		store := &ObjectEditingFiles{officeKind: kind}
		content := base64.StdEncoding.EncodeToString(officeEditingNativeBytes(t, kind, "Saved"))
		for _, encoding := range []string{"utf-8", "unknown", ""} {
			if raw, err := store.encodeObject(content, encoding); !errors.Is(err, objectediting.ErrInvalidInput) || raw != nil {
				t.Fatalf("encoding admitted: %v", err)
			}
		}
		oversized := strings.Repeat("A", base64.StdEncoding.EncodedLen(MaxOfficeObjectBytes)+4)
		if _, err := store.encodeObject(oversized, "office-base64"); !errors.Is(err, objectediting.ErrTooLarge) {
			t.Fatalf("encoded limit: %v", err)
		}
		// Equal encoded lengths can represent either the last allowed byte count or
		// two bytes beyond it: decoding needs its independent raw-byte bound.
		oversized = base64.StdEncoding.EncodeToString(make([]byte, MaxOfficeObjectBytes+1))
		if _, err := store.encodeObject(oversized, "office-base64"); !errors.Is(err, objectediting.ErrTooLarge) {
			t.Fatalf("decoded limit: %v", err)
		}
	}
}

func TestOfficeObjectEditingRejectsInvalidPackagesWithoutMutation(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			before := officeEditingNativeBytes(t, kind, "Before")
			otherKind := map[string]string{"docx": "xlsx", "xlsx": "pptx", "pptx": "docx"}[kind]
			wrong := officeEditingNativeBytes(t, otherKind, "Wrong-kind")
			for _, test := range []struct {
				name, content string
				want          error
			}{
				{"not_zip", base64.StdEncoding.EncodeToString([]byte("not a ZIP archive")), objectediting.ErrNotText},
				{"truncated_zip", base64.StdEncoding.EncodeToString(before[:len(before)-1]), objectediting.ErrNotText},
				{"wrong_kind", base64.StdEncoding.EncodeToString(wrong), objectediting.ErrNotText},
				{"bad_base64", "invalid!", objectediting.ErrInvalidInput},
				{"noncanonical_base64", base64.StdEncoding.EncodeToString(before) + "\n", objectediting.ErrInvalidInput},
				{"oversized", strings.Repeat("A", base64.StdEncoding.EncodedLen(MaxOfficeObjectBytes)+4), objectediting.ErrTooLarge},
			} {
				t.Run(test.name, func(t *testing.T) {
					store, workspace, path := officeEditingNativeFixture(t, kind, before)
					input := objectEditingInput(t, store, workspace, path, test.content)
					if receipt, err := store.Commit(ctx, input); !errors.Is(err, test.want) || receipt != (objectediting.Receipt{}) {
						t.Fatalf("invalid save: %#v %v", receipt, err)
					}
					officeEditingAssertBytes(t, path, before)
					entries, err := os.ReadDir(store.receiptRoot)
					if err != nil || len(entries) != 0 {
						t.Fatalf("invalid input created journal entries: %v %v", entries, err)
					}
				})
			}
			for name, raw := range map[string][]byte{"not_zip": []byte("plain text"), "truncated_zip": before[:len(before)-1], "wrong_kind": wrong} {
				t.Run("open_"+name, func(t *testing.T) {
					store, workspace, path := officeEditingNativeFixture(t, kind, raw)
					if _, err := store.Read(ctx, workspace, path); !errors.Is(err, objectediting.ErrNotText) {
						t.Fatalf("invalid open: %v", err)
					}
					officeEditingAssertBytes(t, path, raw)
				})
			}
			store, workspace, path := officeEditingNativeFixture(t, kind, before)
			wrongExtension := filepath.Join(workspace, "renamed."+otherKind)
			if err := os.Rename(path, wrongExtension); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Read(ctx, workspace, wrongExtension); !errors.Is(err, objectediting.ErrNotText) {
				t.Fatalf("wrong extension accepted: %v", err)
			}
			officeEditingAssertBytes(t, wrongExtension, before)
			if _, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "doc"); !errors.Is(err, objectediting.ErrInvalidInput) {
				t.Fatalf("unsupported kind accepted: %v", err)
			}
		})
	}
}

func TestOfficeObjectEditingDoesNotChangeDefaultTextCodec(t *testing.T) {
	ctx := context.Background()
	for _, encoding := range []string{filetoolsapp.TextEncodingUTF8, filetoolsapp.TextEncodingUTF8BOM, filetoolsapp.TextEncodingUTF16LE, filetoolsapp.TextEncodingUTF16BE} {
		t.Run(encoding, func(t *testing.T) {
			before := filetoolsapp.EncodeTextBytes("Original text 中文\r\n", encoding)
			store, workspace, path := objectEditingFixture(t, before)
			if _, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, "docx"); err != nil {
				t.Fatal(err)
			}
			document, err := store.Read(ctx, workspace, path)
			if err != nil || document.Content != "Original text 中文\r\n" || document.Encoding != encoding {
				t.Fatalf("text open changed: %#v %v", document, err)
			}
			input := objectEditingInput(t, store, workspace, path, "Edited text 中文 😀\r\n")
			receipt, err := store.Commit(ctx, input)
			want := filetoolsapp.EncodeTextBytes(input.Content, encoding)
			if err != nil || receipt.Status != objectediting.StatusCommitted || receipt.Revision != digestAtomicText(want) {
				t.Fatalf("text save changed: %#v %v", receipt, err)
			}
			officeEditingAssertBytes(t, path, want)
			if store.maxObjectBytes() != objectediting.MaxTextBytes || store.maxContentBytes() != objectediting.MaxTextBytes {
				t.Fatal("default text limits changed")
			}
			if err := os.WriteFile(path, officeEditingNativeBytes(t, "docx", "Binary"), 0o640); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Read(ctx, workspace, path); !errors.Is(err, objectediting.ErrNotText) {
				t.Fatalf("default text codec accepted native ZIP: %v", err)
			}
		})
	}
}

// This crosses the actual adapter -> principal session -> Office codec -> CAS
// journal path. It does not invoke an Office engine or a real user profile.
type officeTestIdentity struct{ principal identitydomain.PrincipalV1 }

func (i *officeTestIdentity) ResolveCurrent(context.Context) (identitydomain.PrincipalV1, error) {
	return i.principal, nil
}
func (i *officeTestIdentity) ValidateCurrent(_ context.Context, principal identitydomain.PrincipalV1) error {
	if !identitydomain.SamePrincipalV1(i.principal, principal) {
		return identityport.ErrMismatch
	}
	return nil
}

type officeRecoveryTestProjector struct{ identity *officeTestIdentity }

func (p officeRecoveryTestProjector) ValidateCurrent(ctx context.Context, in editingapp.ScopeAuthority) error {
	if in.ThreadID != "thread_main" || p.identity.ValidateCurrent(ctx, in.Principal) != nil {
		return editingapp.ErrProjection
	}
	return nil
}
func (p officeRecoveryTestProjector) AuthorizeAndProject(ctx context.Context, in editingapp.ProjectionInput) ([]editingapp.ProtectedRange, error) {
	return nil, p.ValidateCurrent(ctx, in.ScopeAuthority)
}

func TestOfficeObjectEditingAdapterSessionSaveAndRestart(t *testing.T) {
	for kind, packageID := range map[string]string{"docx": "analytix-documents", "xlsx": "analytix-spreadsheets", "pptx": "analytix-presentations"} {
		t.Run(kind, func(t *testing.T) {
			before := officeEditingNativeBytes(t, kind, "Before")
			after := officeEditingNativeBytes(t, kind, "Actual-Core-save")
			files, workspace, path := officeEditingNativeFixture(t, kind, before)
			principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
			if err != nil {
				t.Fatal(err)
			}
			identity := &officeTestIdentity{principal: principal}
			captures, leases, writes := 0, 0, 0
			bind := func() *officeapp.Adapter {
				t.Helper()
				files.replaceDocument = func(request atomicTextReplaceRequest) error {
					if leases != 1 {
						t.Fatal("native mutation reached CAS without managed capture", leases)
					}
					writes++
					return atomicReplaceText(request)
				}
				adapter := officeapp.New(kind, editingapp.New(identity, files), func(context.Context) bool { return true })
				err := adapter.BindSelectionHost(officeRecoveryTestProjector{identity}, func(_ context.Context, _ string, capturedPath string, read func() error) (func(), error) {
					if capturedPath != path {
						t.Fatal("capture target changed")
					}
					captures++
					if err := read(); err != nil {
						return nil, err
					}
					leases++
					return func() { leases-- }, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return adapter
			}
			adapter := bind()
			call := func(operation string, input map[string]any) map[string]any {
				t.Helper()
				body, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				result, err := adapter.Invoke(context.Background(), adapterport.Call{Binding: adapterport.Binding{PackageID: packageID}, Principal: identity.principal, ContributionID: "workspace-editor", Operation: operation, Input: body})
				if err != nil {
					t.Fatal(err)
				}
				var out map[string]any
				if err := json.Unmarshal(result.Output, &out); err != nil {
					t.Fatal(err)
				}
				return out
			}
			openInput := map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}
			opened := call("open-object", openInput)
			if opened["ok"] != true {
				t.Fatal(opened)
			}
			doc := opened["document"].(map[string]any)
			unknownChange := strings.Repeat("e", 64)
			input := map[string]any{"sessionId": doc["sessionId"], "threadId": "thread_main", "changeId": unknownChange, "operationId": nativeSaveID(unknownChange), "baseRevision": doc["revision"], "content": map[string]any{"encoding": "base64", "kind": kind, "byteLength": len(after), "sha256": digestAtomicText(after), "data": base64.StdEncoding.EncodeToString(after)}}
			if out := call("commit-object", input); out["code"] != "operation_not_found" || writes != 0 {
				t.Fatal("unapproved native commit reached CAS", out, writes)
			}
			capture := call("capture-selection", map[string]any{"sessionId": doc["sessionId"], "threadId": "thread_main", "selectionToken": "native_selection_01", "changeSequence": 0, "baseRevision": doc["revision"], "text": "Before", "editable": true})
			if capture["ok"] != true {
				t.Fatal(capture)
			}
			scope := capture["scope"].(map[string]any)
			proposed := call("model-selection-propose", map[string]any{"scopeId": scope["scopeId"], "threadId": "thread_main", "operationId": "native_propose_01", "parts": []editingapp.PatchPart{{Kind: "literal", Text: "Actual-Core-save"}}})
			if proposed["ok"] != true {
				t.Fatal(proposed)
			}
			proposal := proposed["result"].(map[string]any)
			approved := call("proposal-accept", map[string]any{"sessionId": doc["sessionId"], "scopeId": scope["scopeId"], "proposalId": proposal["proposalId"], "operationId": "native_approve_01", "selectionToken": scope["selectionToken"], "changeSequence": scope["changeSequence"], "baseRevision": scope["baseRevision"]})
			if approved["ok"] != true {
				t.Fatal(approved)
			}
			replacement := approved["replacement"].(map[string]any)
			input["changeId"], input["operationId"] = replacement["changeId"], replacement["saveOperationId"]
			if input["operationId"] != nativeSaveID(input["changeId"].(string)) || writes != 0 || leases != 1 {
				t.Fatal("approval did not prepare a fixed operation without changing document", approved)
			}
			input["threadId"] = "other_thread"
			if out := call("commit-object", input); out["code"] != "projection_unavailable" || writes != 0 {
				t.Fatal("cross-thread commit", out)
			}
			input["threadId"] = "thread_main"
			saved := call("commit-object", input)
			if saved["ok"] != true || saved["receipt"].(map[string]any)["revision"] != digestAtomicText(after) || writes != 1 {
				t.Fatal("native save", saved, writes)
			}
			officeEditingAssertBytes(t, path, after)
			if out := call("close-object", map[string]any{"sessionId": doc["sessionId"]}); out["ok"] != true || leases != 0 {
				t.Fatal("close did not revoke capture", out)
			}
			if out := call("commit-object", input); out["code"] != "session_invalid" {
				t.Fatal("closed commit", out)
			}
			// Fresh file owner, Service and Adapter simulate reopening after process loss.
			files, err = NewOfficeObjectEditingFiles(files.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			adapter = bind()
			if out := call("object-status", map[string]any{"sessionId": doc["sessionId"], "operationId": input["operationId"]}); out["code"] != "session_invalid" {
				t.Fatal("old session after restart", out)
			}
			reopened := call("open-object", openInput)
			if reopened["ok"] != true {
				t.Fatal(reopened)
			}
			next := reopened["document"].(map[string]any)
			if next["content"] != base64.StdEncoding.EncodeToString(after) || next["objectId"] != doc["objectId"] || next["sessionId"] == doc["sessionId"] {
				t.Fatal("reopen binding/content changed")
			}
			input["sessionId"] = next["sessionId"]
			query := map[string]any{"sessionId": next["sessionId"], "threadId": "thread_main"}
			recovered := call("object-recovery", query)
			if recovered["ok"] != true {
				t.Fatal(recovered)
			}
			current := recovered["recovery"].(map[string]any)["current"].(map[string]any)
			if current["changeId"] != input["changeId"] || current["saveOperationId"] != input["operationId"] || current["canUndo"] != true || current["beforeText"] != "Before" || current["afterText"] != "Actual-Core-save" {
				t.Fatal("restart did not discover private recovery without operation ID", current)
			}
			identity.principal, _ = identitydomain.NewPrincipalV1(strings.Repeat("f", 64), "local", "local")
			if out := call("object-recovery", query); out["code"] != "session_invalid" {
				t.Fatal("previous principal recovered private change", out)
			}
			identity.principal = principal
			if out := call("commit-object", input); out["ok"] != true || writes != 1 {
				t.Fatal("native save replay wrote twice", out, writes)
			}
			if out := call("object-status", map[string]any{"sessionId": next["sessionId"], "operationId": input["operationId"]}); out["ok"] != true {
				t.Fatal(out)
			}
			undo := map[string]any{"sessionId": next["sessionId"], "threadId": "thread_main", "changeId": input["changeId"], "baseRevision": digestAtomicText(after)}
			capturesBefore := captures
			undone := call("undo-change", undo)
			if undone["ok"] != true || undone["receipt"].(map[string]any)["revision"] != digestAtomicText(before) || undone["receipt"].(map[string]any)["operationId"] != nativeUndoID(input["changeId"].(string)) || writes != 2 || captures != capturesBefore+1 || leases != 0 {
				t.Fatal("reopened undo lost Core bytes/capture", undone, writes, captures, leases)
			}
			officeEditingAssertBytes(t, path, before)
			if replay := call("undo-change", undo); replay["ok"] != true || writes != 2 || leases != 0 {
				t.Fatal("historical undo was rejected or rewritten", replay, writes)
			}
			external := officeEditingNativeBytes(t, kind, "External")
			if err := os.WriteFile(path, external, 0640); err != nil {
				t.Fatal(err)
			}
			if out := call("object-status", map[string]any{"sessionId": next["sessionId"], "operationId": input["operationId"]}); out["code"] != "conflict" {
				t.Fatal("stale save status", out)
			}
			if out := call("undo-change", undo); out["ok"] != false || writes != 2 {
				t.Fatal("undo overwrote external revision", out)
			}
			officeEditingAssertBytes(t, path, external)
		})
	}
}
