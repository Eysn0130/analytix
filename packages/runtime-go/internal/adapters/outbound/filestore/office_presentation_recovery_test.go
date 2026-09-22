//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	officeapp "analytix.local/runtime-go/internal/app/officeediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
	editingport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func presentationRecoveryFixture(t *testing.T) ([]officeTestPart, office.PresentationSelection) {
	t.Helper()
	const pns = "http://schemas.openxmlformats.org/presentationml/2006/main"
	const ans = "http://schemas.openxmlformats.org/drawingml/2006/main"
	const rns = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	shape := func(id, name, text string, x int) string {
		return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%s" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="360000"/><a:ext cx="1440000" cy="720000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="ffffff"/></a:solidFill></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>%s</a:t></a:r></a:p></p:txBody></p:sp>`, id, name, x, text)
	}
	slide := func(s string) []byte {
		return []byte(`<p:sld xmlns:p="` + pns + `" xmlns:a="` + ans + `"><p:cSld><p:spTree><p:nvGrpSpPr/><p:grpSpPr/>` + s + `</p:spTree></p:cSld></p:sld>`)
	}
	parts := officeTestParts("pptx")
	parts[2].body = []byte(`<p:presentation xmlns:p="` + pns + `" xmlns:r="` + rns + `"><p:sldIdLst><p:sldId id="256" r:id="slide-b"/><p:sldId id="257" r:id="slide-a"/></p:sldIdLst><p:sldSz cx="10080000" cy="5670000"/></p:presentation>`)
	parts = append(parts,
		officeTestPart{name: "ppt/_rels/presentation.xml.rels", body: officeTestRels(officeTestRelation("slide-b", "slide", "slides/slide2.xml") + officeTestRelation("slide-a", "slide", "slides/slide1.xml"))},
		officeTestPart{name: "ppt/slides/slide2.xml", body: slide(shape("7", "Target", "Synthetic", 360000) + shape("9", "Neighbor", "Unchanged", 2160000))},
		officeTestPart{name: "ppt/slides/slide1.xml", body: slide(shape("1", "Other slide", "Unchanged slide", 360000))},
		officeTestPart{name: "ppt/notesSlides/notesSlide1.xml", body: []byte(`<p:notes xmlns:p="` + pns + `"><p:cSld name="Unchanged notes"/></p:notes>`)},
		officeTestPart{name: "ppt/theme/theme1.xml", body: []byte(`<a:theme xmlns:a="` + ans + `" name="Unchanged theme"><a:themeElements/></a:theme>`)},
		officeTestPart{name: "ppt/media/image.png", body: []byte("\x89PNG\r\n\x1a\nsynthetic")},
		officeTestPart{name: "ppt/slides/_rels/slide2.xml.rels", body: officeTestRels(officeTestRelation("notes", "notesSlide", "../notesSlides/notesSlide1.xml") + officeTestRelation("image", "image", "../media/image.png") + officeTestRelation("theme", "theme", "../theme/theme1.xml"))},
	)
	for _, n := range []string{"slide1.xml", "slide2.xml"} {
		officeTestOverride(parts, "ppt/slides/"+n, "application/vnd.openxmlformats-officedocument.presentationml.slide+xml")
	}
	officeTestOverride(parts, "ppt/media/image.png", "image/png")
	if err := InspectOfficePackage(officeTestZIP(t, parts), "pptx"); err != nil {
		t.Fatal(err)
	}
	s := office.PresentationSelection{PageWidth100thMM: 28000, PageHeight100thMM: 15750, Shapes: []office.PresentationShape{{Kind: "rectangle", Name: "Target", Text: "Synthetic", X100thMM: 1000, Y100thMM: 1000, Width100thMM: 4000, Height100thMM: 2000, FillRGB: "#ffffff"}, {ShapeIndex: 1, Kind: "rectangle", Name: "Neighbor", Text: "Unchanged", X100thMM: 6000, Y100thMM: 1000, Width100thMM: 4000, Height100thMM: 2000, FillRGB: "#ffffff"}}}
	return parts, s
}

func TestOfficePresentationApprovalCommitCASRestartUndo(t *testing.T) {
	for _, kind := range []string{"shape-fill", "shape-geometry"} {
		for _, conflict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/conflict=%v", kind, conflict), func(t *testing.T) {
				ctx := context.Background()
				parts, selected := presentationRecoveryFixture(t)
				original := officeTestZIP(t, parts)
				patch := office.PresentationPatch{Kind: kind, RGB: "#123456"}
				x, y, w, h := 2000, 3000, 5000, 2500
				body := string(parts[4].body)
				if kind == "shape-fill" {
					body = strings.Replace(body, `val="ffffff"`, `val="123456"`, 1)
				} else {
					patch = office.PresentationPatch{Kind: kind, X100thMM: &x, Y100thMM: &y, Width100thMM: &w, Height100thMM: &h}
					body = strings.Replace(body, `<a:off x="360000" y="360000"/><a:ext cx="1440000" cy="720000"/>`, `<a:off x="720000" y="1080000"/><a:ext cx="1800000" cy="900000"/>`, 1)
				}
				parts[4].body = []byte(body)
				candidate := officeTestZIP(t, parts)
				must := func(err error) {
					t.Helper()
					if err != nil {
						t.Fatal(err)
					}
				}
				files, workspace, path := officeEditingNativeFixture(t, "pptx", original)
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
					a := officeapp.New("pptx", editingapp.New(identity, files), func(context.Context) bool { return true })
					must(a.BindSelectionHost(officeRecoveryTestProjector{identity}, func(_ context.Context, _ string, p string, read func() error) (func(), error) {
						if p != path {
							t.Fatal("path changed")
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
				call := func(op string, input map[string]any) map[string]any {
					t.Helper()
					body, e := json.Marshal(input)
					must(e)
					r, e := adapter.Invoke(ctx, adapterport.Call{Admit: func() error { return nil }, Binding: adapterport.Binding{PackageID: "analytix-presentations"}, Principal: identity.principal, ContributionID: "workspace-editor", Operation: op, Input: body})
					must(e)
					var out map[string]any
					must(json.Unmarshal(r.Output, &out))
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
				capture := map[string]any{"sessionId": opened["sessionId"], "threadId": "thread_main", "selectionToken": "presentation_selection_01", "changeSequence": 0, "baseRevision": opened["revision"], "text": "Synthetic", "editable": true, "presentation": selected}
				bad := selected
				bad.Shapes = append([]office.PresentationShape{}, selected.Shapes...)
				bad.Shapes[1].Text = "Forged neighbor"
				capture["presentation"] = bad
				if call("capture-selection", capture)["ok"] == true {
					t.Fatal("forged ordered snapshot accepted")
				}
				capture["presentation"] = selected
				scope := success(call("capture-selection", capture))["scope"].(map[string]any)
				proposalInput := map[string]any{"scopeId": scope["scopeId"], "threadId": "thread_main", "operationId": "presentation_propose_01", "presentation": patch}
				proposal := success(call("model-selection-propose", proposalInput))["result"].(map[string]any)
				if proposal["before"] != nil || proposal["review"] != nil || proposal["beforeText"] != nil {
					t.Fatal("raw review escaped")
				}
				if success(call("model-selection-propose", proposalInput))["result"].(map[string]any)["proposalId"] != proposal["proposalId"] {
					t.Fatal("proposal replay changed identity")
				}
				reviews := success(call("proposal-read", map[string]any{"sessionId": opened["sessionId"], "scopeId": scope["scopeId"]}))["localReviews"].([]any)
				if len(reviews) != 1 || reviews[0].(map[string]any)["presentation"] == nil {
					t.Fatal("typed local diff missing")
				}
				approved := success(call("proposal-accept", map[string]any{"sessionId": opened["sessionId"], "scopeId": scope["scopeId"], "proposalId": proposal["proposalId"], "operationId": "presentation_approve_01", "selectionToken": scope["selectionToken"], "changeSequence": scope["changeSequence"], "baseRevision": scope["baseRevision"]}))["replacement"].(map[string]any)
				if approved["presentation"] == nil || writes != 0 {
					t.Fatal("prepare mutated source or lost review")
				}
				envelope := func(b []byte) map[string]any {
					return map[string]any{"encoding": "base64", "kind": "pptx", "byteLength": len(b), "sha256": digestAtomicText(b), "data": base64.StdEncoding.EncodeToString(b)}
				}
				commit := map[string]any{"sessionId": opened["sessionId"], "threadId": "thread_main", "changeId": approved["changeId"], "operationId": approved["saveOperationId"], "baseRevision": opened["revision"], "content": envelope(original)}
				if call("commit-object", commit)["ok"] == true || writes != 0 {
					t.Fatal("unchanged candidate saved")
				}
				for _, index := range []int{5, 6, 7, 8, 9} {
					old := parts[index].body
					parts[index].body = append(append([]byte{}, old...), []byte(" ")...)
					commit["content"] = envelope(officeTestZIP(t, parts))
					if call("commit-object", commit)["ok"] == true || writes != 0 {
						t.Fatal("outside part changed", index)
					}
					parts[index].body = old
				}
				parts[4].body = []byte(strings.Replace(body, "Unchanged", "Corrupted neighbor", 1))
				commit["content"] = envelope(officeTestZIP(t, parts))
				if call("commit-object", commit)["ok"] == true || writes != 0 {
					t.Fatal("neighbor changed")
				}
				parts[4].body = []byte(body)
				commit["content"] = envelope(candidate)
				if conflict {
					external := officeEditingNativeBytes(t, "pptx", "External change")
					must(os.WriteFile(path, external, 0600))
					if call("commit-object", commit)["ok"] == true || writes != 0 {
						t.Fatal("CAS overwrote external revision")
					}
					officeEditingAssertBytes(t, path, external)
					return
				}
				success(call("commit-object", commit))
				success(call("commit-object", commit))
				if writes != 1 {
					t.Fatal("commit replay wrote twice", writes)
				}
				officeEditingAssertBytes(t, path, candidate)
				success(call("close-object", map[string]any{"sessionId": opened["sessionId"]}))
				if leases != 0 {
					t.Fatal("lease leaked")
				}
				files, err = NewOfficeObjectEditingFiles(files.receiptRoot, nil, "pptx")
				must(err)
				adapter = bind()
				next := success(call("open-object", map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}))["document"].(map[string]any)
				if call("model-selection-read", map[string]any{"scopeId": scope["scopeId"], "threadId": "thread_main"})["ok"] == true {
					t.Fatal("restart revived native token")
				}
				recovery := success(call("object-recovery", map[string]any{"sessionId": next["sessionId"], "threadId": "thread_main"}))["recovery"].(map[string]any)["current"].(map[string]any)
				if recovery["presentation"] == nil || recovery["canUndo"] != true {
					t.Fatal("typed recovery lost")
				}
				undo := map[string]any{"sessionId": next["sessionId"], "threadId": "other_thread", "changeId": approved["changeId"], "baseRevision": next["revision"]}
				if call("undo-change", undo)["ok"] == true {
					t.Fatal("foreign thread undo")
				}
				undo["threadId"] = "thread_main"
				success(call("undo-change", undo))
				officeEditingAssertBytes(t, path, original)
				if writes != 2 || leases != 0 {
					t.Fatal("undo writes/lease", writes, leases)
				}
			})
		}
	}
}

func TestPresentationRecoveryLegacyCanonicalAndTypedExclusion(t *testing.T) {
	d := editingport.NativeChangeDraft{ChangeID: strings.Repeat("a", 64), ThreadID: "thread_main", ProposalID: strings.Repeat("b", 48), BaseRevision: strings.Repeat("c", 64), BeforeText: "before", AfterText: "after"}
	old := struct{ ChangeID, ThreadID, ProposalID, BaseRevision, BeforeText, AfterText string }{d.ChangeID, d.ThreadID, d.ProposalID, d.BaseRevision, d.BeforeText, d.AfterText}
	body, _ := json.Marshal(old)
	current, _ := json.Marshal(d)
	if !bytes.Equal(body, current) || nativeDraftHash(d) != digestAtomicText(body) {
		t.Fatal("old digest changed")
	}
	var decoded editingport.NativeChangeDraft
	if nativeDecode(body, &decoded, 1<<20) != nil {
		t.Fatal("old record rejected")
	}
	for _, key := range []string{"presentation", "workbook"} {
		bad := append(append([]byte{}, body[:len(body)-1]...), []byte(`,"`+key+`":null}`)...)
		if nativeDecode(bad, &decoded, 1<<20) == nil {
			t.Fatal("explicit null accepted", key)
		}
	}
	_, s := presentationRecoveryFixture(t)
	r, e := office.ApplyPresentationPatch(s, office.PresentationPatch{Kind: "shape-fill", RGB: "#123456"})
	if e != nil {
		t.Fatal(e)
	}
	d.Presentation = &r
	d.Workbook = &office.WorkbookReview{}
	if nativeDraftValid(d) {
		t.Fatal("mixed typed review accepted")
	}
}
