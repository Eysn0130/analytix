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
	"strings"
	"testing"
	"unicode/utf16"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

// Synthetic trusted composition for this private HTTP seam only. Production
// must inject the real thread/purpose and protected-field authority.
type proposalHTTPIdentity struct {
	objectTestIdentity
	revoked bool
}

func (i *proposalHTTPIdentity) ValidateCurrent(ctx context.Context, p identitydomain.PrincipalV1) error {
	if i.revoked {
		return errors.New("private revoked identity")
	}
	return i.objectTestIdentity.ValidateCurrent(ctx, p)
}

type proposalHTTPProjector struct{}

func (proposalHTTPProjector) ValidateCurrent(_ context.Context, a editingapp.ScopeAuthority) error {
	if a.ThreadID != "thread-1" {
		return errors.New("private authority cause")
	}
	return nil
}
func (p proposalHTTPProjector) AuthorizeAndProject(ctx context.Context, in editingapp.ProjectionInput) ([]editingapp.ProtectedRange, error) {
	if err := p.ValidateCurrent(ctx, in.ScopeAuthority); err != nil {
		return nil, err
	}
	if at := strings.Index(in.Text, "Alice"); at >= 0 {
		return []editingapp.ProtectedRange{{StartByte: at, EndByte: at + 5}}, nil
	}
	return nil, nil
}

func TestObjectProposalPrivateHTTPDraftThenExplicitCAS(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "document.md")
	original := "Hello Alice 😀"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
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
	authority := &proposalHTTPIdentity{objectTestIdentity: objectTestIdentity{principal}}
	service := editingapp.NewWithProjector(authority, files, proposalHTTPProjector{})
	handler := LocalDisplayMuxV1{RuntimeToken: "synthetic-token", LocalDisplay: LocalDisplayHandlerV1{ObjectEditing: ObjectEditingHandler{Service: service}}}
	call := func(input map[string]any, authorized bool) (int, map[string]any) {
		t.Helper()
		data, _ := json.Marshal(input)
		r := httptest.NewRequest(http.MethodPost, ObjectEditingPath, bytes.NewReader(data))
		if authorized {
			r.Header.Set("Authorization", "Bearer synthetic-token")
			r.Header.Set(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid JSON response")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private response cacheable")
		}
		return w.Code, result
	}
	good := func(input map[string]any) map[string]any {
		t.Helper()
		code, result := call(input, true)
		if code != 200 {
			t.Fatalf("%s: code=%d error=%v", input["action"], code, result["code"])
		}
		return result
	}
	disk := func(want string) {
		t.Helper()
		value, err := os.ReadFile(path)
		if err != nil || string(value) != want {
			t.Fatal("unexpected disk write")
		}
	}
	opened := good(map[string]any{"action": "open", "workspace": workspace, "path": path})["document"].(map[string]any)
	sid := opened["sessionId"]
	base := opened["revision"]
	draft := good(map[string]any{"action": "draft-update", "sessionId": sid, "baseRevision": base, "expectedVersion": "", "content": original})["draft"].(map[string]any)
	capture := func(version any) map[string]any {
		return good(map[string]any{"action": "scope-capture", "sessionId": sid, "draftVersion": version, "threadId": "thread-1", "purpose": "edit", "range": map[string]any{"start": 0, "end": len(utf16.Encode([]rune(draft["content"].(string))))}})["scope"].(map[string]any)
	}
	if code, result := call(map[string]any{"action": "scope-capture", "sessionId": sid, "draftVersion": draft["version"], "threadId": "thread-1", "purpose": "edit", "range": map[string]any{"start": 13, "end": 14}}, true); code != 400 || result["code"] != "invalid_request" {
		t.Fatal("surrogate-splitting range accepted")
	}
	scope := capture(draft["version"])
	binding := func(action string) map[string]any {
		return map[string]any{"action": action, "sessionId": sid, "scopeId": scope["scopeId"], "threadId": "thread-1", "purpose": "edit", "draftVersion": scope["draftVersion"]}
	}
	parts := scope["parts"].([]any)
	if len(parts) != 3 {
		t.Fatal("protected projection missing")
	}
	encoded, _ := json.Marshal(scope)
	if strings.Contains(string(encoded), "Alice") {
		t.Fatal("scope leaked protected span")
	}
	parts[0] = map[string]any{"kind": "literal", "text": "Dear "}
	propose := binding("proposal-create")
	propose["operationId"] = "propose_0001"
	propose["parts"] = parts
	if code, _ := call(propose, false); code == 200 {
		t.Fatal("unauthorized proposal")
	}
	proposal := good(propose)["proposal"].(map[string]any)
	if replay := good(propose)["proposal"].(map[string]any); replay["proposalId"] != proposal["proposalId"] {
		t.Fatal("proposal replay changed identity")
	}
	altered := binding("proposal-create")
	altered["operationId"] = "propose_0001"
	altered["parts"] = []any{map[string]any{"kind": "literal", "text": "different"}}
	if code, result := call(altered, true); code != 409 || result["code"] != "operation_mismatch" {
		t.Fatal("operation payload mismatch accepted")
	}
	altered["operationId"] = "propose_badspan"
	if code, result := call(altered, true); code != 400 || result["code"] != "protected_span_invalid" {
		t.Fatal("protected span removal accepted")
	}

	reject := binding("proposal-reject")
	reject["operationId"] = "reject_0001"
	reject["proposalId"] = proposal["proposalId"]
	good(reject)
	good(reject)
	disk(original)
	if got := good(map[string]any{"action": "draft-read", "sessionId": sid})["draft"].(map[string]any)["content"]; got != original {
		t.Fatal("reject changed draft")
	}
	propose["operationId"] = "propose_0002"
	proposal = good(propose)["proposal"].(map[string]any)
	accept := binding("proposal-accept")
	accept["operationId"] = "accept_0001"
	accept["proposalId"] = proposal["proposalId"]
	decision := good(accept)["decision"].(map[string]any)
	replay := good(accept)["decision"].(map[string]any)
	if decision["draftVersion"] != replay["draftVersion"] {
		t.Fatal("accept replay changed version")
	}
	disk(original)
	draft = good(map[string]any{"action": "draft-read", "sessionId": sid})["draft"].(map[string]any)
	if draft["content"] != "Dear Alice 😀" || draft["baseRevision"] != base {
		t.Fatal("accept failed to keep protected span and disk base")
	}
	stale := binding("proposal-create")
	stale["operationId"] = "propose_stale"
	stale["parts"] = parts
	if code, _ := call(stale, true); code == 200 {
		t.Fatal("stale scope permitted edit")
	}
	scope = capture(draft["version"])
	wrong := binding("scope-read")
	wrong["threadId"] = "thread-2"
	if code, result := call(wrong, true); code == 200 || strings.Contains(result["message"].(string), "private") {
		t.Fatal("cross-thread authority bypass or cause leak")
	}
	good(binding("scope-revoke"))
	if code, _ := call(binding("scope-read"), true); code == 200 {
		t.Fatal("revoked scope readable")
	}
	good(map[string]any{"action": "commit", "sessionId": sid, "operationId": "save_000001", "baseRevision": draft["baseRevision"], "content": draft["content"]})
	disk("Dear Alice 😀")
	authority.revoked = true
	if code, result := call(map[string]any{"action": "draft-read", "sessionId": sid}, true); code == 200 || result["draft"] != nil {
		t.Fatal("revoked identity read draft")
	}
}

func TestObjectProposalHTTPStrictJSON(t *testing.T) {
	handler := ObjectEditingHandler{Service: editingapp.New(nil, nil)}
	token := strings.Repeat("a", 48)
	hash := strings.Repeat("b", 64)
	valid := []string{
		`{"action":"draft-read","sessionId":"` + token + `"}`,
		`{"action":"draft-update","sessionId":"` + token + `","baseRevision":"` + hash + `","expectedVersion":"","content":"x"}`,
		`{"action":"scope-capture","sessionId":"` + token + `","draftVersion":"` + token + `","threadId":"thread-1","purpose":"edit","range":{"start":0,"end":1}}`,
		`{"action":"proposal-create","sessionId":"` + token + `","scopeId":"` + token + `","draftVersion":"` + token + `","threadId":"thread-1","purpose":"edit","operationId":"propose_001","parts":[{"kind":"literal","text":"x"}]}`,
	}
	check := func(body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ObjectEditingPath, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("strict validation status=%d want=%d", w.Code, want)
		}
	}
	for _, body := range valid {
		check(body, 503)
		var fields map[string]json.RawMessage
		_ = json.Unmarshal([]byte(body), &fields)
		for key := range fields {
			changed := map[string]json.RawMessage{}
			for k, v := range fields {
				changed[k] = v
			}
			delete(changed, key)
			data, _ := json.Marshal(changed)
			check(string(data), 400)
			changed[key] = json.RawMessage(`null`)
			data, _ = json.Marshal(changed)
			check(string(data), 400)
			delete(changed, key)
			changed[strings.ToUpper(key)] = fields[key]
			data, _ = json.Marshal(changed)
			check(string(data), 400)
		}
		check(strings.TrimSuffix(body, "}")+`,"principal":"forged"}`, 400)
		check(strings.TrimSuffix(body, "}")+`,"action":"draft-read"}`, 400)
	}
	for _, replacement := range []string{`{"start":null,"end":1}`, `{"Start":0,"end":1}`, `{"start":0,"end":1,"end":2}`, `{"start":0.5,"end":1}`, `{"start":0,"end":1572865}`, `{"start":1,"end":0}`, `{"start":0,"end":{}}`} {
		check(strings.Replace(valid[2], `{"start":0,"end":1}`, replacement, 1), 400)
	}
	for _, replacement := range []string{`[{"kind":"literal","text":null}]`, `[{"kind":"literal","text":"x","protectedRef":"x"}]`, `[{"kind":"protected","protectedRef":"x"}]`, `[{"kind":"Literal","text":"x"}]`, `null`, `[{"kind":"literal","text":"\ud800"}]`} {
		check(strings.Replace(valid[3], `[{"kind":"literal","text":"x"}]`, replacement, 1), 400)
	}
}

func TestObjectProposalHTTPClosedErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		code   string
		status int
	}{
		{editingapp.ErrDraftStale, "draft_stale", 409}, {editingapp.ErrScope, "scope_invalid", 409},
		{editingapp.ErrProposal, "proposal_invalid", 409}, {editingapp.ErrProjection, "projection_unavailable", 503},
		{editingapp.ErrProtected, "protected_span_invalid", 400},
	} {
		w := httptest.NewRecorder()
		writeObjectEditingError(w, errors.Join(test.err, errors.New("private body /private/path")), fileport.Receipt{})
		var result map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != test.status || result["code"] != test.code || len(result) != 3 || strings.Contains(w.Body.String(), "private") {
			t.Fatal("error vocabulary or projection changed")
		}
	}
}
