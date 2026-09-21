package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// This is the real local-display HTTP -> Service -> filestore/projector ->
// native tool owner seam. Identity and all image/note bytes are synthetic.
// No Provider, native image inspector or GUI is invoked.
type imageProductFixture struct {
	t                             *testing.T
	p                             runtimeObjectProjector
	thread, workspace, path, root string
	handler                       http.Handler
	service                       *editingapp.Service
	opened                        editingapp.OpenedImage
	annotation                    editing.ImageAnnotation
}

func newImageProductFixture(t *testing.T) *imageProductFixture {
	t.Helper()
	p, _, authority := newSelectionProjectorFixture(t)
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal(err)
	}
	p.handler.turnSecurity.RiskAuthority = risk
	f := &imageProductFixture{t: t, p: p, thread: authority.ThreadID, workspace: authority.Workspace, root: t.TempDir()}
	f.path = filepath.Join(f.workspace, "private-image.png")
	var body bytes.Buffer
	pic := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	pic.Pix[0] = 77
	if err := png.Encode(&body, pic); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.path, body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	f.restart()
	f.open()
	return f
}
func (f *imageProductFixture) restart() {
	f.t.Helper()
	files, err := filestore.NewObjectEditingFiles(f.root, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.service = editingapp.New(f.p.handler.turnSecurity.Identity, files)
	if err := f.service.BindHost(f.p, func(context.Context, string, string, func() error) (func(), error) {
		f.t.Fatal("image acquired managed edit lease")
		return nil, nil
	}); err != nil {
		f.t.Fatal(err)
	}
	f.p.handler.objectEditing = f.service
	f.handler = httpapi.LocalDisplayMuxV1{RuntimeToken: "synthetic-image-token", LocalDisplay: httpapi.LocalDisplayHandlerV1{ObjectEditing: httpapi.ObjectEditingHandler{Service: f.service}}}
}
func (f *imageProductFixture) request(body any, authorized bool) *httptest.ResponseRecorder {
	f.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		f.t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, httpapi.ObjectEditingPath, bytes.NewReader(raw))
	if authorized {
		r.Header.Set("Authorization", "Bearer synthetic-image-token")
		r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" {
		f.t.Fatal("image cached")
	}
	return w
}
func (f *imageProductFixture) open() {
	f.t.Helper()
	w := f.request(map[string]any{"action": "image-open", "threadId": f.thread, "path": f.path}, true)
	var result struct {
		OK    bool                   `json:"ok"`
		Image editingapp.OpenedImage `json:"image"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.OK || result.Image.DataBase64 == "" {
		f.t.Fatalf("open image: %d %s", w.Code, w.Body.String())
	}
	f.opened = result.Image
}
func (f *imageProductFixture) save(note string) {
	f.t.Helper()
	f.saveRegions([]editing.ImageAnnotationRegion{{RegionID: strings.Repeat("a", 48), Region: editing.ImageRegion{X: 1, Y: 2, Width: 3, Height: 2}, Note: note}})
}
func (f *imageProductFixture) saveRegions(regions []editing.ImageAnnotationRegion) {
	f.t.Helper()
	w := f.request(map[string]any{"action": "image-annotation-write", "sessionId": f.opened.SessionID, "threadId": f.thread, "sourceRevision": f.opened.SourceRevision, "expectedAnnotationRevision": f.annotation.AnnotationRevision, "regions": regions}, true)
	var result struct {
		Annotation editing.ImageAnnotation `json:"annotation"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.Annotation.Current {
		f.t.Fatalf("save image: %d %s", w.Code, w.Body.String())
	}
	f.annotation = result.Annotation
}
func (f *imageProductFixture) capture() editingapp.ImageScope {
	f.t.Helper()
	return f.captureRegion(strings.Repeat("a", 48))
}
func (f *imageProductFixture) captureRegion(regionID string) editingapp.ImageScope {
	f.t.Helper()
	w := f.request(map[string]any{"action": "image-scope-capture", "sessionId": f.opened.SessionID, "threadId": f.thread, "sourceRevision": f.opened.SourceRevision, "annotationRevision": f.annotation.AnnotationRevision, "regionId": regionID}, true)
	var result struct {
		Scope editingapp.ImageScope `json:"scope"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.Scope.Current {
		f.t.Fatalf("capture image: %d %s", w.Code, w.Body.String())
	}
	return result.Scope
}
func (f *imageProductFixture) model(thread, name, id string) (string, bool) {
	f.t.Helper()
	value, failed := f.p.handler.executeNativeSelectionTool(context.Background(), runtimePendingToolCall{ThreadID: thread, Call: domainmodel.ToolCall{Name: name}}, map[string]any{"scopeId": id})
	raw, err := json.Marshal(value)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(raw), failed
}
func TestImageProductLocalDisplayPersistReopenAndProjectedModelReference(t *testing.T) {
	f := newImageProductFixture(t)
	before, err := f.p.handler.store.GetThread(f.thread)
	if err != nil {
		t.Fatal(err)
	}
	f.save("Permitted remark. Contact alice@example.com")
	scope := f.capture()
	if scope.Kind != "image-region" || scope.Editable || scope.Purpose != "discuss" || scope.Region.Width != 3 {
		t.Fatal("invalid scope")
	}
	raw, failed := f.model(f.thread, "native_selection_read", scope.ScopeID)
	if failed || !strings.Contains(raw, "Permitted remark.") || !strings.Contains(raw, `"kind":"protected"`) || !strings.Contains(raw, `"imageObservation":"unavailable"`) || !strings.Contains(raw, `"width":3`) {
		t.Fatal("projected model reference", raw)
	}
	for _, private := range []string{"alice@example.com", f.path, f.workspace, f.opened.DataBase64, f.opened.SourceRevision, f.opened.ObjectID} {
		if strings.Contains(raw, private) {
			t.Fatal("private image data entered model")
		}
	}
	if _, failed := f.model(f.thread, "native_selection_propose", scope.ScopeID); !failed {
		t.Fatal("readonly region accepted proposal")
	}
	if _, failed := f.model("foreign-thread", "native_selection_read", scope.ScopeID); !failed {
		t.Fatal("foreign thread read scope")
	}
	if _, failed := f.model(f.thread, "native_selection_read", scope.ScopeID); failed {
		t.Fatal("foreign request revoked original")
	}
	after, err := f.p.handler.store.GetThread(f.thread)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("image flow manufactured history")
	}
	saved := f.annotation
	old := f.opened
	f.restart()
	if _, failed := f.model(f.thread, "native_selection_read", scope.ScopeID); !failed {
		t.Fatal("restart recovered capability")
	}
	if _, err := f.service.ReadImageAnnotation(context.Background(), old.SessionID, f.thread); err == nil {
		t.Fatal("restart retained session")
	}
	f.open()
	if f.opened.ObjectID != old.ObjectID || f.opened.SessionID == old.SessionID {
		t.Fatal("reopen identity/session")
	}
	w := f.request(map[string]any{"action": "image-annotation-read", "sessionId": f.opened.SessionID, "threadId": f.thread}, true)
	var result struct {
		Annotation editing.ImageAnnotation `json:"annotation"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !reflect.DeepEqual(saved, result.Annotation) {
		t.Fatal("persistent rectangle did not reopen", w.Body.String())
	}
	f.annotation = result.Annotation
	_ = f.capture()
}
func TestImageProductRevocationVersionPermissionAndTextIsolation(t *testing.T) {
	for _, mode := range []string{"close", "revoke", "annotation", "source", "permission"} {
		t.Run(mode, func(t *testing.T) {
			f := newImageProductFixture(t)
			f.save("allowed note")
			scope := f.capture()
			ctx := context.Background()
			switch mode {
			case "close":
				if err := f.service.Close(ctx, f.opened.SessionID); err != nil {
					t.Fatal(err)
				}
			case "revoke":
				if err := f.service.RevokeImageScope(ctx, editingapp.ImageScopeBinding{SessionID: f.opened.SessionID, ThreadID: f.thread, ScopeID: scope.ScopeID}); err != nil {
					t.Fatal(err)
				}
			case "annotation":
				f.save("changed note")
			case "source":
				var body bytes.Buffer
				_ = png.Encode(&body, image.NewNRGBA(image.Rect(0, 0, 8, 6)))
				if err := os.WriteFile(f.path, body.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			case "permission":
				f.p.handler.turnSecurity.RiskAuthority = nil
			}
			if raw, failed := f.model(f.thread, "native_selection_read", scope.ScopeID); !failed || strings.Contains(raw, "allowed note") {
				t.Fatal("stale scope survived", raw)
			}
			if _, err := f.service.ReadImageScope(ctx, editingapp.ImageScopeBinding{SessionID: f.opened.SessionID, ThreadID: f.thread, ScopeID: scope.ScopeID}); err == nil {
				t.Fatal("local scope survived invalidation")
			}
		})
	}
	f := newImageProductFixture(t)
	ctx := context.Background()
	if _, err := f.service.Commit(ctx, f.opened.SessionID, "operation_1234", f.opened.SourceRevision, "overwrite"); err == nil {
		t.Fatal("image text commit")
	}
	if _, err := f.service.UpdateDraft(ctx, editingapp.UpdateDraftInput{SessionID: f.opened.SessionID, BaseRevision: f.opened.SourceRevision, Content: "overwrite"}); err == nil {
		t.Fatal("image text draft")
	}
	if _, err := f.service.WriteAnnotationDraft(ctx, f.opened.SessionID, f.thread, "", "text note", f.opened.SourceRevision); err == nil {
		t.Fatal("image untyped annotation")
	}
	if _, err := f.service.CommitNativeChange(ctx, f.opened.SessionID, f.thread, "change", "operation_1234", f.opened.SourceRevision, "overwrite"); err == nil {
		t.Fatal("image native text commit")
	}
	open := map[string]any{"action": "image-open", "threadId": f.thread, "path": f.path}
	if w := f.request(open, false); w.Code == 200 || strings.Contains(w.Body.String(), f.opened.DataBase64) {
		t.Fatal("unprotected image")
	}
	open["threadId"] = "foreign-thread"
	if w := f.request(open, true); w.Code == 200 || strings.Contains(w.Body.String(), f.opened.DataBase64) {
		t.Fatal("unauthorized thread image")
	}
	open["threadId"] = f.thread
	open["workspace"] = f.workspace
	if w := f.request(open, true); w.Code != 400 {
		t.Fatal("caller workspace accepted")
	}
}

func TestImageScopeSurvivesNormalSameThreadTurnAuthority(t *testing.T) {
	f := newImageProductFixture(t)
	f.save("permitted region note")
	scope := f.capture()
	thread, err := f.p.handler.store.GetThread(f.thread)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(), Authority: f.p.handler.turnSecurity, Thread: thread, ThreadID: f.thread, TurnID: "image-normal-turn", Workspace: f.workspace, Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	record := turnsecurityapp.PublicRecord(frozen)
	if err := f.p.handler.store.AppendTurnToThread(f.thread, map[string]any{"id": frozen.TurnID, "threadId": f.thread, "status": "completed", "items": []any{}, "securityContext": record}, "", map[string]any{"securityState": record}); err != nil {
		t.Fatal(err)
	}
	if raw, failed := f.model(f.thread, "native_selection_read", scope.ScopeID); failed || !strings.Contains(raw, "permitted region note") {
		t.Fatal("same primary turn lost image reference", raw)
	}
}

type imageCaptureRaceProjector struct {
	runtimeObjectProjector
	afterProject func()
}

func (p *imageCaptureRaceProjector) AuthorizeAndProject(ctx context.Context, input editingapp.ProjectionInput) ([]editingapp.ProtectedRange, error) {
	ranges, err := p.runtimeObjectProjector.AuthorizeAndProject(ctx, input)
	if err == nil && p.afterProject != nil {
		p.afterProject()
	}
	return ranges, err
}
func TestImageCaptureRechecksSourceAfterProjection(t *testing.T) {
	f := newImageProductFixture(t)
	files, err := filestore.NewObjectEditingFiles(f.root, nil)
	if err != nil {
		t.Fatal(err)
	}
	projector := &imageCaptureRaceProjector{runtimeObjectProjector: f.p}
	f.service = editingapp.NewWithProjector(f.p.handler.turnSecurity.Identity, files, projector)
	f.p.handler.objectEditing = f.service
	f.handler = httpapi.LocalDisplayMuxV1{RuntimeToken: "synthetic-image-token", LocalDisplay: httpapi.LocalDisplayHandlerV1{ObjectEditing: httpapi.ObjectEditingHandler{Service: f.service}}}
	f.open()
	f.save("allowed note")
	projector.afterProject = func() {
		var body bytes.Buffer
		_ = png.Encode(&body, image.NewNRGBA(image.Rect(0, 0, 8, 6)))
		if err := os.WriteFile(f.path, body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w := f.request(map[string]any{"action": "image-scope-capture", "sessionId": f.opened.SessionID, "threadId": f.thread, "sourceRevision": f.opened.SourceRevision, "annotationRevision": f.annotation.AnnotationRevision, "regionId": strings.Repeat("a", 48)}, true)
	if w.Code == 200 || strings.Contains(w.Body.String(), "allowed note") || f.service.HasImageScopes() {
		t.Fatal("capture returned stale authority", w.Body.String())
	}
}

func TestImageProductMultipleRegionsCaptureSelectedNotesAndCollectionRevocation(t *testing.T) {
	f := newImageProductFixture(t)
	regions := []editing.ImageAnnotationRegion{
		{RegionID: strings.Repeat("a", 48), Region: editing.ImageRegion{X: 1, Y: 1, Width: 2, Height: 2}, Note: "first area annotation"},
		{RegionID: strings.Repeat("b", 48), Region: editing.ImageRegion{X: 5, Y: 1, Width: 2, Height: 2}, Note: "second area annotation"},
	}
	f.saveRegions(regions)
	scopes := []editingapp.ImageScope{f.captureRegion(regions[0].RegionID), f.captureRegion(regions[1].RegionID)}
	for i, scope := range scopes {
		if scope.RegionID != regions[i].RegionID || scope.Region != regions[i].Region {
			t.Fatal("wrong region captured")
		}
		raw, failed := f.model(f.thread, "native_selection_read", scope.ScopeID)
		if failed || !strings.Contains(raw, regions[i].Note) || strings.Contains(raw, regions[1-i].Note) {
			t.Fatal("capture projected other region note", raw)
		}
	}
	for _, field := range []string{"regionId", "annotationRevision", "sourceRevision"} {
		input := map[string]any{"action": "image-scope-capture", "sessionId": f.opened.SessionID, "threadId": f.thread, "sourceRevision": f.opened.SourceRevision, "annotationRevision": f.annotation.AnnotationRevision, "regionId": regions[0].RegionID}
		input[field] = strings.Repeat("c", 64)
		if field == "regionId" {
			input[field] = strings.Repeat("c", 48)
		}
		if w := f.request(input, true); w.Code == 200 {
			t.Fatal("wrong capture binding accepted", field)
		}
	}
	regions[1].Note = "second area edited"
	f.saveRegions(regions)
	for _, scope := range scopes {
		if _, failed := f.model(f.thread, "native_selection_read", scope.ScopeID); !failed {
			t.Fatal("collection update retained old scope")
		}
	}
	saved := f.annotation
	f.restart()
	f.open()
	annotation, err := f.service.ReadImageAnnotation(context.Background(), f.opened.SessionID, f.thread)
	if err != nil || !reflect.DeepEqual(annotation, saved) {
		t.Fatal("collection restart", err)
	}
	f.annotation = annotation
	scope := f.captureRegion(regions[1].RegionID)
	if raw, failed := f.model(f.thread, "native_selection_read", scope.ScopeID); failed || !strings.Contains(raw, regions[1].Note) || strings.Contains(raw, regions[0].Note) {
		t.Fatal("reopened selected note", raw)
	}
	f.saveRegions([]editing.ImageAnnotationRegion{})
	if f.annotation.Regions == nil || len(f.annotation.Regions) != 0 || f.annotation.AnnotationRevision == "" {
		t.Fatal("clear lost tombstone")
	}
	if _, failed := f.model(f.thread, "native_selection_read", scope.ScopeID); !failed {
		t.Fatal("clear retained scope")
	}
}
