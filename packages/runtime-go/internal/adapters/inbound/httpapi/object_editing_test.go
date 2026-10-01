package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
)

type objectTestIdentity struct{ p identitydomain.PrincipalV1 }

func (i objectTestIdentity) ResolveCurrent(context.Context) (identitydomain.PrincipalV1, error) {
	return i.p, nil
}
func (i objectTestIdentity) ValidateCurrent(_ context.Context, p identitydomain.PrincipalV1) error {
	if !identitydomain.SamePrincipalV1(i.p, p) {
		return errors.New("mismatch")
	}
	return nil
}

func TestObjectEditingProtectedLaneFileCommitAndRestart(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("versioned replacement is not available on this platform")
	}
	workspace := t.TempDir()
	path := filepath.Join(workspace, "document.md")
	root := t.TempDir()
	if err := os.WriteFile(path, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	makeMux := func() http.Handler {
		files, err := filestore.NewObjectEditingFiles(root, nil)
		if err != nil {
			t.Fatal(err)
		}
		return LocalDisplayMuxV1{RuntimeToken: "synthetic-object-token", LocalDisplay: LocalDisplayHandlerV1{ObjectEditing: ObjectEditingHandler{Service: editingapp.New(objectTestIdentity{p}, files)}}}
	}
	handler := makeMux()
	request := func(body any, token, header bool) *httptest.ResponseRecorder {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, ObjectEditingPath, bytes.NewReader(data))
		if token {
			r.Header.Set("Authorization", "Bearer synthetic-object-token")
		}
		if header {
			r.Header.Set(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response became cacheable")
		}
		return w
	}
	open := map[string]any{"action": "open", "workspace": workspace, "path": path}
	for _, authority := range [][2]bool{{false, false}, {true, false}, {false, true}} {
		w := request(open, authority[0], authority[1])
		if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "original") {
			t.Fatal("untyped caller received original")
		}
	}
	w := request(open, true, true)
	var opened struct {
		OK       bool              `json:"ok"`
		Document editingapp.Opened `json:"document"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &opened) != nil || !opened.OK || opened.Document.Content != "original\n" {
		t.Fatalf("open failed: %d", w.Code)
	}
	commit := map[string]any{"action": "commit", "sessionId": opened.Document.SessionID, "operationId": "operation_1234", "baseRevision": opened.Document.Revision, "content": "saved draft\n"}
	w = request(commit, true, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"committed"`) {
		t.Fatalf("commit failed: %d %s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "saved draft\n" {
		t.Fatal("save response did not correspond to real bytes")
	}
	handler = makeMux()
	status := map[string]any{"action": "status", "sessionId": opened.Document.SessionID, "operationId": "operation_1234"}
	if w = request(status, true, true); w.Code != 409 {
		t.Fatal("pre-restart session retained authority")
	}
	w = request(open, true, true)
	var reopened struct {
		Document editingapp.Opened `json:"document"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &reopened) != nil || reopened.Document.Content != "saved draft\n" || reopened.Document.ObjectID != opened.Document.ObjectID {
		t.Fatal("disk reopen failed")
	}
	status["sessionId"] = reopened.Document.SessionID
	if w = request(status, true, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"committed"`) {
		t.Fatal("durable operation was not recovered")
	}
	if err := os.WriteFile(path, []byte("external editor\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if w = request(status, true, true); w.Code != 409 || !strings.Contains(w.Body.String(), `"status":"conflict"`) {
		t.Fatalf("historical receipt status mismatch: HTTP %d, body %s", w.Code, w.Body.String())
	}
	commit["sessionId"] = reopened.Document.SessionID
	if w = request(commit, true, true); w.Code != 409 {
		t.Fatal("replayed operation overwrote external edit")
	}
	data, _ = os.ReadFile(path)
	if string(data) != "external editor\n" {
		t.Fatal("replay rewrote the file")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("operation replay duplicated journal")
	}
	journal, _ := os.ReadFile(filepath.Join(root, entries[0].Name()))
	for _, private := range []string{workspace, path, "saved draft", "external editor"} {
		if bytes.Contains(journal, []byte(private)) {
			t.Fatal("receipt persisted private path or content")
		}
	}
}

func TestObjectEditingRejectsMalformedRequestsBeforeAuthorityRead(t *testing.T) {
	handler := ObjectEditingHandler{Service: editingapp.New(nil, nil)}
	for _, body := range []string{
		`{"action":"open","action":"close","workspace":"/tmp","path":"a"}`,
		`{"action":"commit","sessionId":"` + strings.Repeat("a", 48) + `","operationId":"save_1234","baseRevision":"` + strings.Repeat("b", 64) + `"}`,
		`{"action":"close","sessionId":"` + strings.Repeat("a", 48) + `","operationId":""}`,
		`{"action":"open","workspace":"/tmp","path":"a","principal":"caller"}`,
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ObjectEditingPath, strings.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("malformed input accepted: %d", w.Code)
		}
	}
}

type exportThreadAuthority struct{ workspace string }

func (p exportThreadAuthority) ValidateCurrent(_ context.Context, value editingapp.ScopeAuthority) error {
	if value.Workspace != p.workspace || value.ThreadID != "current-main" || value.Purpose != "discuss" {
		return editingapp.ErrProjection
	}
	return nil
}
func (p exportThreadAuthority) AuthorizeAndProject(context.Context, editingapp.ProjectionInput) ([]editingapp.ProtectedRange, error) {
	return nil, errors.New("export must not create a scope")
}

func TestObjectEditingExportSnapshotUsesTypedLaneAndCurrentFileDraft(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("versioned replacement unavailable")
	}
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "note.md")
	if err := os.WriteFile(path, []byte("disk original"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := filestore.NewObjectEditingFiles(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	identityWorkspace, ok := filestore.ResolveMutationIdentityPath(workspace, workspace)
	if !ok {
		t.Fatal("workspace identity unavailable")
	}
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("thread workspace spelling equals object workspace identity: %v", observation.WorkspaceRealPath == identityWorkspace)
	service := editingapp.NewWithProjector(objectTestIdentity{principal}, files, exportThreadAuthority{identityWorkspace})
	handler := LocalDisplayMuxV1{RuntimeToken: "synthetic-export-token", LocalDisplay: LocalDisplayHandlerV1{ObjectEditing: ObjectEditingHandler{Service: service}}}
	call := func(body any, authorized bool) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, ObjectEditingPath, bytes.NewReader(data))
		if authorized {
			r.Header.Set("Authorization", "Bearer synthetic-export-token")
			r.Header.Set(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	opened, err := service.Open(context.Background(), workspace, path)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := service.UpdateDraft(context.Background(), editingapp.UpdateDraftInput{SessionID: opened.SessionID, BaseRevision: opened.Revision, Content: "UNSAVED_EXPORT_SENTINEL"})
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"action": "export-snapshot", "sessionId": opened.SessionID, "objectId": opened.ObjectID, "threadId": "current-main", "baseRevision": opened.Revision, "draftVersion": draft.Version}
	if w := call(input, false); w.Code == http.StatusOK || strings.Contains(w.Body.String(), draft.Content) {
		t.Fatal("untyped export leaked")
	}
	w := call(input, true)
	var value struct {
		OK       bool                      `json:"ok"`
		Snapshot editingapp.ExportSnapshot `json:"snapshot"`
	}
	if json.Unmarshal(w.Body.Bytes(), &value) != nil || w.Code != http.StatusOK || !value.OK || value.Snapshot.Content != draft.Content || value.Snapshot.Workspace != identityWorkspace {
		t.Fatalf("snapshot: %d %s", w.Code, w.Body.String())
	}
	original, _ := os.ReadFile(path)
	if string(original) != "disk original" {
		t.Fatal("export saved the original")
	}
	for key, bad := range map[string]any{"threadId": "foreign-thread", "draftVersion": strings.Repeat("f", 48), "objectId": strings.Repeat("f", 64)} {
		previous := input[key]
		input[key] = bad
		if w := call(input, true); w.Code == http.StatusOK || strings.Contains(w.Body.String(), draft.Content) {
			t.Fatal("stale export leaked")
		}
		input[key] = previous
	}
	input["content"] = "forged"
	if w := call(input, true); w.Code != http.StatusBadRequest {
		t.Fatal("extra input accepted")
	}
	delete(input, "content")
	if err := os.WriteFile(path, []byte("external changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if w := call(input, true); w.Code != http.StatusConflict || strings.Contains(w.Body.String(), draft.Content) {
		t.Fatal("external change not rejected")
	}
}
