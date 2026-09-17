package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
)

type pluginHostHTTPFake struct {
	listCalls     int
	setCalls      int
	invokeCalls   int
	view          hostapp.PackageView
	setRequest    hostapp.SetDesiredStateRequest
	invokeRequest hostapp.InvokeRequest
	failure       error
	output        json.RawMessage
}

func (f *pluginHostHTTPFake) List(context.Context) ([]hostapp.PackageView, error) {
	f.listCalls++
	return []hostapp.PackageView{f.view}, f.failure
}
func (f *pluginHostHTTPFake) SetDesiredState(_ context.Context, r hostapp.SetDesiredStateRequest) (hostapp.PackageView, error) {
	f.setCalls++
	f.setRequest = r
	// Deliberately commit fake state before returning a failure, so HTTP tests do
	// not assume the absence of a durable transition from a failed response.
	f.view.ActivationState = "recorded"
	f.view.DesiredState = r.DesiredState
	f.view.ActivationRevision = r.ExpectedRevision + 1
	f.view.ActivationID = strings.Repeat("c", 64)
	return f.view, f.failure
}
func (f *pluginHostHTTPFake) Invoke(_ context.Context, r hostapp.InvokeRequest) (hostapp.InvokeResult, error) {
	f.invokeCalls++
	f.invokeRequest = r
	return hostapp.InvokeResult{Output: f.output}, f.failure
}
func pluginHostHTTPFixture() (*pluginHostHTTPFake, PluginPackageHostHandler) {
	fake := &pluginHostHTTPFake{view: hostapp.PackageView{PackageID: "analytix-documents", PackageVersion: "1.0.0", DisplayName: "Documents", Origin: "development-source", Materialized: true, GenerationID: strings.Repeat("a", 64), ActivationState: "unset", UnavailableReason: "activation_unset", Operations: []string{}}, output: json.RawMessage(`{"objectId":"synthetic-object"}`)}
	return fake, PluginPackageHostHandler{Service: fake}
}
func pluginHostHTTPBody(action string) string {
	switch action {
	case "setDesiredState":
		return `{"action":"setDesiredState","packageId":"analytix-documents","generationId":"` + strings.Repeat("a", 64) + `","expectedRevision":0,"desiredState":"enabled"}`
	case "invoke":
		return `{"action":"invoke","packageId":"analytix-documents","generationId":"` + strings.Repeat("a", 64) + `","expectedRevision":1,"contributionId":"workspace-editor","operation":"open","input":{"objectId":"synthetic-object"}}`
	default:
		return `{"action":"list"}`
	}
}
func pluginHostHTTPRequest(handler http.Handler, method, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, PluginPackageHostPath, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestPluginPackageHostHTTPTypedListSetAndInvoke(t *testing.T) {
	fake, handler := pluginHostHTTPFixture()
	w := pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody("list"))
	var listed pluginHostListResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &listed) != nil || !listed.OK || len(listed.Packages) != 1 || listed.Packages[0].ActivationState != "unset" || listed.Packages[0].DesiredState != "" {
		t.Fatal("typed unset list failed", w.Code, w.Body.String())
	}
	w = pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody("setDesiredState"))
	var changed pluginHostSetResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &changed) != nil || !changed.OK || changed.Package.ActivationRevision != 1 || changed.Package.DesiredState != domainplugin.DesiredEnabledV1 || fake.setRequest.ExpectedRevision != 0 || fake.setRequest.GenerationID != fake.view.GenerationID {
		t.Fatal("exact CAS did not reach Host", w.Code, w.Body.String())
	}
	w = pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody("invoke"))
	var invoked pluginHostInvokeResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &invoked) != nil || !invoked.OK || string(invoked.Output) != string(fake.output) || fake.invokeRequest.ContributionID != "workspace-editor" || fake.invokeRequest.ExpectedRevision != 1 {
		t.Fatal("typed contribution call failed", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Pragma") != "no-cache" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("protected result became cacheable")
	}
}

func TestPluginPackageHostHTTPRejectsAmbiguousUnknownAndUnboundedInputBeforeCore(t *testing.T) {
	fake, handler := pluginHostHTTPFixture()
	set := pluginHostHTTPBody("setDesiredState")
	invoke := pluginHostHTTPBody("invoke")
	invalid := []string{
		`{"action":"list","action":"invoke"}`, `{"action":"list","principal":"caller"}`, `{"Action":"list"}`, `{"action":"list","packageId":"analytix-documents"}`, `{"action":"register","path":"/private/source"}`,
		strings.Replace(set, `"expectedRevision":0,`, "", 1), strings.Replace(set, `"expectedRevision":0`, `"expectedRevision":null`, 1), strings.Replace(set, `"expectedRevision":0`, `"expectedRevision":-1`, 1), strings.Replace(set, `"expectedRevision":0`, `"expectedRevision":0.0`, 1), strings.Replace(set, `"expectedRevision":0`, `"expectedRevision":0e0`, 1), strings.Replace(set, `"expectedRevision":0`, `"expectedRevision":9007199254740992`, 1),
		strings.Replace(set, `"generationId"`, `"GenerationId"`, 1), strings.Replace(set, `"generationId":"`+strings.Repeat("a", 64)+`"`, `"generationId":"not-a-generation"`, 1), strings.Replace(set, `"desiredState":"enabled"`, `"desiredState":"unset"`, 1),
		strings.Replace(invoke, `"expectedRevision":1`, `"expectedRevision":0`, 1), strings.Replace(invoke, `"contributionId":"workspace-editor"`, `"contributionId":"editor-adapter"`, 1), strings.Replace(invoke, `"operation":"open"`, `"operation":"/private/script"`, 1),
		strings.Replace(invoke, `"input":{"objectId":"synthetic-object"}`, `"input":null`, 1), strings.Replace(invoke, `"input":{"objectId":"synthetic-object"}`, `"input":{"objectId":"a","objectId":"b"}`, 1), strings.Replace(invoke, `"input":{"objectId":"synthetic-object"}`, `"input":{"text":"`+strings.Repeat("x", hostapp.MaxInputBytes)+`"}`, 1),
		`[]`, `{"action":"list"} {"action":"invoke"}`,
	}
	for index, body := range invalid {
		w := pluginHostHTTPRequest(handler, http.MethodPost, body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid case %d returned %d", index, w.Code)
		}
		if !strings.Contains(w.Body.String(), `"code":"invalid_request"`) {
			t.Fatalf("case %d escaped error schema", index)
		}
	}
	if fake.listCalls+fake.setCalls+fake.invokeCalls != 0 {
		t.Fatal("malformed input reached Core")
	}
	w := pluginHostHTTPRequest(handler, http.MethodGet, pluginHostHTTPBody("list"))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "POST" {
		t.Fatal("non-POST operation accepted")
	}
	r := httptest.NewRequest(http.MethodPost, PluginPackageHostPath+"?root=/private", strings.NewReader(pluginHostHTTPBody("list")))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatal("query authority selector accepted")
	}
}

func TestPluginPackageHostHTTPClosedFailuresRequireRelist(t *testing.T) {
	cases := []struct {
		cause  error
		status int
		code   string
	}{
		{hostapp.ErrInvalid, 400, "invalid_request"}, {hostapp.ErrIdentity, 403, "identity_invalid"}, {hostapp.ErrNotFound, 404, "package_not_found"}, {hostapp.ErrConflict, 409, "conflict"}, {hostapp.ErrDisabled, 409, "disabled"}, {hostapp.ErrAdapterUnavailable, 503, "adapter_unavailable"}, {hostapp.ErrPersistence, 500, "persistence_failure"}, {hostapp.ErrUnavailable, 503, "unavailable"}, {errors.New("unknown internal cause"), 503, "unavailable"},
	}
	for _, test := range cases {
		fake, handler := pluginHostHTTPFixture()
		fake.failure = fmt.Errorf("private path /private/profile and principal-secret: %w", test.cause)
		for _, action := range []string{"setDesiredState", "invoke"} {
			w := pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody(action))
			var response pluginHostErrorResponse
			if w.Code != test.status || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.OK || response.Code != test.code || !response.Relist || !strings.Contains(response.Message, "Refresh the plugin list") {
				t.Fatal("unsafe failure projection", action, w.Code, w.Body.String())
			}
			for _, private := range []string{"/private", "principal-secret", "unknown internal cause"} {
				if strings.Contains(w.Body.String(), private) {
					t.Fatal("raw internal error leaked")
				}
			}
		}
	}
	fake, handler := pluginHostHTTPFixture()
	fake.failure = hostapp.ErrPersistence
	w := pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody("setDesiredState"))
	if w.Code != 500 {
		t.Fatal("persisted failure became success")
	}
	fake.failure = nil
	w = pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody("list"))
	var listed pluginHostListResponse
	if json.Unmarshal(w.Body.Bytes(), &listed) != nil || listed.Packages[0].DesiredState != domainplugin.DesiredEnabledV1 || listed.Packages[0].ActivationRevision != 1 {
		t.Fatal("failure response assumed state rollback")
	}
	w = pluginHostHTTPRequest(PluginPackageHostHandler{}, http.MethodPost, pluginHostHTTPBody("invoke"))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"relist":true`) {
		t.Fatal("absent Host did not fail closed")
	}
}

func TestPluginPackageHostHTTPUsesExistingProtectedLocalDisplayBoundary(t *testing.T) {
	fake, handler := pluginHostHTTPFixture()
	// New-route registration is intentionally outside this candidate. Exercise
	// the unchanged LocalDisplayMux authority using an already-classified local
	// display path with this handler injected as its downstream handler. This is
	// not evidence that PluginPackageHostPath has been registered in production.
	mux := LocalDisplayMuxV1{RuntimeToken: "synthetic-plugin-host-token", LocalDisplay: handler}
	cases := []struct {
		token   string
		headers []string
		status  int
	}{
		{"", nil, 401}, {"wrong", []string{LocalDisplayHeaderValueV1}, 401}, {"synthetic-plugin-host-token", nil, 403}, {"synthetic-plugin-host-token", []string{"ordinary"}, 403}, {"synthetic-plugin-host-token", []string{LocalDisplayHeaderValueV1, LocalDisplayHeaderValueV1}, 403}, {"synthetic-plugin-host-token", []string{LocalDisplayHeaderValueV1}, 200},
	}
	for _, test := range cases {
		r := httptest.NewRequest(http.MethodPost, LocalDisplayDirectPreviewPathV1, strings.NewReader(pluginHostHTTPBody("list")))
		if test.token != "" {
			r.Header.Set("Authorization", "Bearer "+test.token)
		}
		for _, header := range test.headers {
			r.Header.Add(LocalDisplayHeaderV1, header)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != test.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("protected-lane authority drift", w.Code)
		}
	}
	if fake.listCalls != 1 {
		t.Fatal("unqualified caller reached Host")
	}
}

func TestPluginPackageHostHTTPRejectsUnsafeJSONRevisionAndOutput(t *testing.T) {
	fake, handler := pluginHostHTTPFixture()
	body := strings.Replace(pluginHostHTTPBody("setDesiredState"), `"expectedRevision":0`, `"expectedRevision":9007199254740990`, 1)
	w := pluginHostHTTPRequest(handler, http.MethodPost, body)
	if w.Code != 200 || fake.setRequest.ExpectedRevision != 9007199254740990 {
		t.Fatal("JSON-safe revision was not transported exactly", w.Code)
	}
	fake.view.ActivationRevision = maxPluginHostJSONRevision + 1
	w = pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody("list"))
	if w.Code != 503 {
		t.Fatal("unsafe revision was rounded into public state")
	}
	fake.output = json.RawMessage(`{"objectId":"a","objectId":"b"}`)
	w = pluginHostHTTPRequest(handler, http.MethodPost, pluginHostHTTPBody("invoke"))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"relist":true`) {
		t.Fatal("ambiguous adapter response emitted as success")
	}
}
