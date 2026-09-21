package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	readapp "analytix.local/runtime-go/internal/app/workspaceread"
	readport "analytix.local/runtime-go/internal/ports/workspaceread"
)

type workspaceReadHTTPAuthority struct {
	calls  int
	denied bool
}

func (a *workspaceReadHTTPAuthority) Current(context.Context, string) (readapp.Scope, error) {
	a.calls++
	if a.denied {
		return readapp.Scope{}, readport.ErrUnavailable
	}
	return readapp.Scope{ThreadID: "thread-1", Workspace: "/workspace", Binding: "principal-policy"}, nil
}

type workspaceReadHTTPFiles struct{ inspects, scans int }

func (f *workspaceReadHTTPFiles) InspectRoot(context.Context, string) (readport.Root, error) {
	f.inspects++
	return readport.Root{Workspace: "/workspace", Identity: "device:inode", Policy: "protected-policy"}, nil
}
func (f *workspaceReadHTTPFiles) Scan(_ context.Context, _ readport.Root, _ bool, _ time.Duration, check func() error) ([]readport.File, error) {
	f.scans++
	if err := check(); err != nil {
		return nil, err
	}
	return []readport.File{{Path: "read.md", Kind: "text", Revision: strings.Repeat("a", 64), Content: []byte("fixture")}}, nil
}

func workspaceReadHTTPTestHandler(t *testing.T) (WorkspaceReadHandler, *workspaceReadHTTPAuthority, *workspaceReadHTTPFiles) {
	t.Helper()
	a := &workspaceReadHTTPAuthority{}
	f := &workspaceReadHTTPFiles{}
	s := &readapp.Service{Files: f}
	if err := s.BindAuthority(a); err != nil {
		t.Fatal(err)
	}
	return WorkspaceReadHandler{Service: s}, a, f
}

func workspaceReadHTTPCall(handler WorkspaceReadHandler, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, WorkspaceReadPath, strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestWorkspaceReadHTTPStrictActionShapes(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"action":null,"threadId":"thread-1"}`, `{"action":"authorize","threadId":null}`,
		`{"Action":"authorize","threadId":"thread-1"}`, `{"action":"authorize","ThreadId":"thread-1"}`,
		`{"action":"authorize","threadId":"thread-1","ThreadId":"thread-1"}`,
		`{"action":"authorize","threadId":"thread-1","unknown":false}`,
		`{"action":"authorize","threadId":"thread-1","binding":""}`,
		`{"action":"authorize","threadId":"thread-1","binding":null}`,
		`{"action":"authorize","threadId":"thread-1","includePdf":false}`,
		`{"action":"authorize","threadId":"thread-1","action":"authorize"}`,
		`{"action":"authorize","threadId":"thread-1"} {}`,
		`{"action":"authorize","threadId":"../thread"}`, `{"action":"authorize","threadId":" thread-1"}`,
		`{"action":"authorize","threadId":"` + strings.Repeat("t", 129) + `"}`,
		`{"action":"validate","threadId":"thread-1"}`,
		`{"action":"validate","threadId":"thread-1","binding":null}`,
		`{"action":"validate","threadId":"thread-1","Binding":"` + digest + `"}`,
		`{"action":"validate","threadId":"thread-1","binding":"` + digest + `","includePdf":false}`,
		`{"action":"validate","threadId":"thread-1","binding":"` + strings.Repeat("g", 64) + `"}`,
		`{"action":"validate","threadId":"thread-1","binding":"` + strings.Repeat("A", 64) + `"}`,
		`{"action":"validate","threadId":"thread-1","binding":" ` + digest + `"}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `"}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `","includePdf":null}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `","includePdf":"false"}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `","includePdf":0}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `","includePDF":false}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `","includePdf":false,"includePdf":true}`,
		`{"action":"unknown","threadId":"thread-1"}`,
		strings.Repeat(" ", 4097),
	} {
		h, a, f := workspaceReadHTTPTestHandler(t)
		response := workspaceReadHTTPCall(h, body)
		if response.Code != http.StatusBadRequest || a.calls != 0 || f.inspects != 0 || f.scans != 0 {
			t.Fatalf("malformed body was not rejected before authority/IO: %q status=%d", body, response.Code)
		}
		if strings.TrimSpace(response.Body.String()) != `{"code":"invalid_request","ok":false}` {
			t.Fatalf("unexpected failure projection: %s", response.Body.String())
		}
	}
}

func TestWorkspaceReadHTTPSuccessAndUnavailable(t *testing.T) {
	h, a, f := workspaceReadHTTPTestHandler(t)
	response := workspaceReadHTTPCall(h, `{"action":"authorize","threadId":"thread-1"}`)
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("local-display response is cacheable")
	}
	var result struct {
		OK       bool             `json:"ok"`
		Snapshot readapp.Snapshot `json:"snapshot"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.OK || len(result.Snapshot.Binding) != 64 {
		t.Fatalf("authorize failed: %s", response.Body.String())
	}
	digest := result.Snapshot.Binding
	for _, body := range []string{
		`{"action":"validate","threadId":"thread-1","binding":"` + digest + `"}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `","includePdf":false}`,
		`{"action":"scan","threadId":"thread-1","binding":"` + digest + `","includePdf":true}`,
	} {
		if got := workspaceReadHTTPCall(h, body); got.Code != http.StatusOK {
			t.Fatalf("valid request failed: %s", got.Body.String())
		}
	}
	if f.scans != 2 {
		t.Fatalf("scan count=%d", f.scans)
	}
	a.denied = true
	before := f.inspects
	response = workspaceReadHTTPCall(h, `{"action":"authorize","threadId":"thread-1"}`)
	if response.Code != http.StatusForbidden || f.inspects != before || strings.TrimSpace(response.Body.String()) != `{"code":"unavailable","ok":false}` {
		t.Fatalf("authority rejection leaked effects/details: %s", response.Body.String())
	}
	response = workspaceReadHTTPCall(WorkspaceReadHandler{}, `{"action":"authorize","threadId":"thread-1"}`)
	if response.Code != http.StatusForbidden {
		t.Fatal("missing service did not fail closed")
	}
}
