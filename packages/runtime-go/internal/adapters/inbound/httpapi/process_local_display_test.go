package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	processapp "analytix.local/runtime-go/internal/app/processdisplay"
	continuation "analytix.local/runtime-go/internal/domain/continuation"
	identity "analytix.local/runtime-go/internal/domain/identity"
	model "analytix.local/runtime-go/internal/domain/model"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	security "analytix.local/runtime-go/internal/domain/security"
	processport "analytix.local/runtime-go/internal/ports/processdisplay"
)

const processHTTPTokenV1 = "ec3dfd08-f6a7-4bb4-9eb9-5ecafc4cb610"
const processHTTPDocumentV1 = "f47d0793-b684-4ae4-8ce9-40b610c58004"

// This is an HTTP/owner unit fixture. Production grant, principal and signed
// historical-origin validation is exercised separately by server seam tests.
type processHTTPAuthorityV1 struct{ acquired chan struct{} }

func (a *processHTTPAuthorityV1) AcquireCurrent(ctx context.Context, binding processport.BindingV1) (processport.AuthorizedV1, func(), error) {
	if ctx.Err() != nil {
		return processport.AuthorizedV1{}, nil, processport.ErrUnavailableV1
	}
	select {
	case a.acquired <- struct{}{}:
	default:
	}
	return processport.AuthorizedV1{Binding: binding}, func() {}, nil
}
func (a *processHTTPAuthorityV1) ValidateCurrent(ctx context.Context, _ processport.AuthorizedV1) error {
	return ctx.Err()
}
func (a *processHTTPAuthorityV1) ValidateAssistantDraftCurrent(ctx context.Context, _ processport.BindingV1) error {
	return a.ValidateCurrent(ctx, processport.AuthorizedV1{})
}
func (a *processHTTPAuthorityV1) ValidateToolCurrent(ctx context.Context, _ processport.BindingV1, _ domainprocess.ToolBindingV1) error {
	return processport.ErrUnavailableV1
}

func processHTTPFixtureV1(t *testing.T) (ProcessLocalDisplayHandlerV1, *processapp.ProducerV1, *processHTTPAuthorityV1) {
	t.Helper()
	principal, err := identity.NewPrincipalV1(strings.Repeat("1", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	frozen := security.NewTurnSecurityContext(security.TurnSecurityContextInput{ThreadID: "thread-http-process", TurnID: "turn-http-process", WorkspaceRealPath: t.TempDir(), TenantID: "local", UserID: "local"})
	authority := &processHTTPAuthorityV1{acquired: make(chan struct{}, 32)}
	owner := processapp.NewServiceV1(authority, processapp.DefaultLimitsV1())
	producer := owner.BeginV1(processport.BindingV1{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Principal: principal, Original: frozen})
	if producer == nil {
		t.Fatal("HTTP fixture binding invalid")
	}
	t.Cleanup(func() { _ = owner.Close() })
	return ProcessLocalDisplayHandlerV1{Service: owner}, producer, authority
}

func processHTTPPostV1(t *testing.T, handler ProcessLocalDisplayHandlerV1, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Pragma") != "no-cache" {
		t.Fatal("process HTTP response permits caching")
	}
	return w
}

func processHTTPDecodedV1(t *testing.T, response *httptest.ResponseRecorder) processapp.ResponseV1 {
	t.Helper()
	var decoded processapp.ResponseV1
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &decoded) != nil {
		t.Fatalf("process HTTP result status=%d", response.Code)
	}
	return decoded
}

func processHTTPAuthorizeV1(t *testing.T, handler ProcessLocalDisplayHandlerV1) processapp.ResponseV1 {
	t.Helper()
	return processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"authorize","viewerToken":"`+processHTTPTokenV1+`","documentNonce":"`+processHTTPDocumentV1+`","threadId":"thread-http-process","turnId":"turn-http-process"}`))
}

func TestProcessLocalHTTPStrictMalformedTypesFieldsAndBounds(t *testing.T) {
	base := `{"schemaVersion":2,"action":"read","viewerToken":"` + processHTTPTokenV1 + `"`
	tests := []struct{ name, path, body string }{
		{"web_target_url_injection", ProcessLocalDisplayTargetPathV1, `{"schemaVersion":2,"action":"open_web_link","viewerToken":"` + processHTTPTokenV1 + `","blockId":"block","inputDigest":"` + strings.Repeat("a", 64) + `","role":"request","targetUrl":"https://injected.invalid"}`},
		{"web_target_wrong_role", ProcessLocalDisplayTargetPathV1, `{"schemaVersion":2,"action":"open_web_link","viewerToken":"` + processHTTPTokenV1 + `","blockId":"block","inputDigest":"` + strings.Repeat("a", 64) + `","role":"result"}`},
		{"web_target_capture_alias", ProcessLocalDisplayTargetPathV1, `{"schemaVersion":2,"action":"open_web_link","viewerToken":"` + processHTTPTokenV1 + `","blockId":"block","captureDigest":"` + strings.Repeat("a", 64) + `","role":"request"}`},
		{"missing_version", ProcessLocalDisplayReadPathV1, `{"action":"read","viewerToken":"` + processHTTPTokenV1 + `"}`},
		{"v1_version", ProcessLocalDisplayReadPathV1, `{"schemaVersion":1,"action":"read","viewerToken":"` + processHTTPTokenV1 + `"}`},
		{"version_string", ProcessLocalDisplayReadPathV1, `{"schemaVersion":"2","action":"read","viewerToken":"` + processHTTPTokenV1 + `"}`},
		{"empty", ProcessLocalDisplayReadPathV1, ""},
		{"malformed", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":`},
		{"array_root", ProcessLocalDisplayReadPathV1, `[]`},
		{"null_root", ProcessLocalDisplayReadPathV1, `null`},
		{"trailing_document", ProcessLocalDisplayReadPathV1, base + `} {}`},
		{"duplicate_property", ProcessLocalDisplayReadPathV1, base + `,"action":"read"}`},
		{"escaped_duplicate_property", ProcessLocalDisplayReadPathV1, base + `,"\u0061ction":"read"}`},
		{"unknown_private_authority", ProcessLocalDisplayReadPathV1, base + `,"grant":"PRIVATE_GRANT_CANARY"}`},
		{"unknown_source_path", ProcessLocalDisplayReadPathV1, base + `,"path":"/private/HTTP_SOURCE_CANARY"}`},
		{"known_property_wrong_action", ProcessLocalDisplayReadPathV1, base + `,"documentNonce":"` + processHTTPDocumentV1 + `"}`},
		{"case_alias", ProcessLocalDisplayReadPathV1, `{"Action":"read","viewerToken":"` + processHTTPTokenV1 + `"}`},
		{"missing_token", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read"}`},
		{"token_object", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":{"private":"PRIVATE_CANARY"}}`},
		{"wait_negative", ProcessLocalDisplayReadPathV1, base + `,"waitMs":-1}`},
		{"wait_too_large", ProcessLocalDisplayReadPathV1, base + `,"waitMs":25001}`},
		{"wait_fraction", ProcessLocalDisplayReadPathV1, base + `,"waitMs":0.5}`},
		{"wait_overflow", ProcessLocalDisplayReadPathV1, base + `,"waitMs":1e100}`},
		{"wait_string", ProcessLocalDisplayReadPathV1, base + `,"waitMs":"0"}`},
		{"wait_null", ProcessLocalDisplayReadPathV1, base + `,"waitMs":null}`},
		{"revision_number", ProcessLocalDisplayReadPathV1, base + `,"afterRevision":0}`},
		{"revision_null", ProcessLocalDisplayReadPathV1, base + `,"afterRevision":null}`},
		{"expanded_object", ProcessLocalDisplayReadPathV1, base + `,"expandedBlockIds":{}}`},
		{"expanded_null", ProcessLocalDisplayReadPathV1, base + `,"expandedBlockIds":null}`},
		{"expanded_null_member", ProcessLocalDisplayReadPathV1, base + `,"expandedBlockIds":[null]}`},
		{"expanded_number_member", ProcessLocalDisplayReadPathV1, base + `,"expandedBlockIds":[1]}`},
		{"expanded_nested_depth", ProcessLocalDisplayReadPathV1, base + `,"expandedBlockIds":[[[["PRIVATE_CANARY"]]]]}`},
		{"max_string", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":"` + strings.Repeat("x", 257) + `"}`},
		{"max_body", ProcessLocalDisplayReadPathV1, base + `}` + strings.Repeat(" ", maxLocalDisplayRequestBytesV1)},
		{"authorize_missing_thread", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"authorize","viewerToken":"x","documentNonce":"y","turnId":"z"}`},
		{"authorize_null_document", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"authorize","viewerToken":"x","documentNonce":null,"threadId":"y","turnId":"z"}`},
		{"read_on_target", ProcessLocalDisplayTargetPathV1, base + `}`},
		{"open_on_read", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"open_capture","viewerToken":"x","blockId":"y","captureDigest":"` + strings.Repeat("a", 64) + `"}`},
		{"open_digest_missing", ProcessLocalDisplayTargetPathV1, `{"schemaVersion":2,"action":"open_capture","viewerToken":"x","blockId":"y"}`},
		{"open_digest_bad", ProcessLocalDisplayTargetPathV1, `{"schemaVersion":2,"action":"open_capture","viewerToken":"x","blockId":"y","captureDigest":"PRIVATE_CANARY"}`},
		{"unknown_action", ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"PRIVATE_ACTION_CANARY","viewerToken":"x"}`},
	}
	for _, count := range []int{129, 520} {
		ids := make([]string, count)
		for index := range ids {
			ids[index] = fmt.Sprintf("%032x", index)
		}
		encoded, _ := json.Marshal(ids)
		tests = append(tests, struct{ name, path, body string }{fmt.Sprintf("expanded_or_token_count_%d", count), ProcessLocalDisplayReadPathV1, base + `,"expandedBlockIds":` + string(encoded) + `}`})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := ProcessLocalDisplayHandlerV1{}
			w := processHTTPPostV1(t, handler, test.path, test.body)
			var failure map[string]string
			if w.Code != http.StatusBadRequest || json.Unmarshal(w.Body.Bytes(), &failure) != nil || len(failure) != 2 || failure["code"] != "validation_error" || failure["message"] == "" || strings.Contains(w.Body.String(), "PRIVATE_") || strings.Contains(w.Body.String(), "HTTP_SOURCE_CANARY") {
				t.Fatalf("malformed process request escaped the existing public failure projection: status=%d case=%s", w.Code, test.name)
			}
		})
	}
}

func TestProcessLocalHTTPUnavailableVariantsStayClosedAtAcceptedBoundaries(t *testing.T) {
	ids := make([]string, 128)
	for index := range ids {
		ids[index] = fmt.Sprintf("%032x", index)
	}
	encoded, _ := json.Marshal(ids)
	for _, test := range []struct{ path, body string }{
		{ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":"` + processHTTPTokenV1 + `","waitMs":25000,"afterRevision":"0","expandedBlockIds":` + string(encoded) + `}`},
		{ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"authorize","viewerToken":"` + processHTTPTokenV1 + `","documentNonce":"` + processHTTPDocumentV1 + `","threadId":"thread-private","turnId":"turn-private"}`},
		{ProcessLocalDisplayTargetPathV1, `{"schemaVersion":2,"action":"open_capture","viewerToken":"` + processHTTPTokenV1 + `","blockId":"` + strings.Repeat("a", 32) + `","captureDigest":"` + strings.Repeat("b", 64) + `"}`},
		{ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"revoke","viewerToken":"` + processHTTPTokenV1 + `"}`},
	} {
		w := processHTTPPostV1(t, ProcessLocalDisplayHandlerV1{}, test.path, test.body)
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"status":"unavailable"}` || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal("missing owner denial expanded into scope/body or rejected a documented boundary")
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		(ProcessLocalDisplayHandlerV1{}).ServeHTTP(w, httptest.NewRequest(method, ProcessLocalDisplayReadPathV1, nil))
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Pragma") != "no-cache" {
			t.Fatal("method boundary or cache prohibition changed")
		}
	}
}

func TestProcessLocalHTTPLiveNativeBodyUsesExistingExpansionAndRevokeBoundary(t *testing.T) {
	handler, producer, _ := processHTTPFixtureV1(t)
	namespace := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !producer.UnitV2(namespace, true) {
		t.Fatal("actual running provider namespace denied")
	}
	const body = "complete first line\nunclosed continuation "
	producer.FrameV1(model.ProcessFrameV1{Kind: "start", Attempt: 1, ProviderSequence: 7, NativeIndex: 8})
	producer.FrameV1(model.ProcessFrameV1{Kind: "delta", Attempt: 1, ProviderSequence: 7, NativeIndex: 8, Text: body})
	if processHTTPAuthorizeV1(t, handler).Status != "authorized" {
		t.Fatal("running native lease denied")
	}
	read := `{"schemaVersion":2,"action":"read","viewerToken":"` + processHTTPTokenV1 + `"}`
	collapsed := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, read))
	if collapsed.Status != "snapshot" || len(collapsed.Blocks) != 1 {
		t.Fatal("running native summary missing")
	}
	block := collapsed.Blocks[0]
	if block.State != "running" || block.Source.Kind != "native" || block.Preview != "complete first line" || block.Body != nil || block.CaptureDigest != "" || block.ExitCode != nil {
		t.Fatal("running HTTP summary invented settlement or released body")
	}
	expanded := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":"`+processHTTPTokenV1+`","expandedBlockIds":["`+block.BlockID+`"]}`))
	if expanded.Status != "snapshot" || len(expanded.Blocks) != 1 || expanded.Blocks[0].BlockID != block.BlockID || expanded.Blocks[0].Body == nil || expanded.Blocks[0].Body.Text != body || expanded.Blocks[0].CaptureDigest != "" {
		t.Fatal("expanded running HTTP copy lost the same live source")
	}
	revoked := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"revoke","viewerToken":"`+processHTTPTokenV1+`"}`))
	if revoked.Status != "revoked" {
		t.Fatal("live HTTP revoke failed")
	}
	producer.FrameV1(model.ProcessFrameV1{Kind: "delta", Attempt: 1, ProviderSequence: 7, NativeIndex: 8, Text: "LATE_LIVE_HTTP_BODY"})
	producer.FrameV1(model.ProcessFrameV1{Kind: "end", Attempt: 1, ProviderSequence: 7, NativeIndex: 8})
	late := processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, read)
	if strings.TrimSpace(late.Body.String()) != `{"status":"unavailable"}` || bytes.Contains(late.Body.Bytes(), []byte("LATE_LIVE_HTTP_BODY")) {
		t.Fatal("late running producer revived a revoked HTTP viewer")
	}
}

func TestProcessLocalHTTPOwnerBodyRequiresExpansionExactDigestAndLiveDocument(t *testing.T) {
	handler, producer, _ := processHTTPFixtureV1(t)
	const body = "private HTTP native body\r\n"
	producer.FrameV1(model.ProcessFrameV1{Kind: "start", Attempt: 1, NativeIndex: 8})
	producer.FrameV1(model.ProcessFrameV1{Kind: "delta", Attempt: 1, NativeIndex: 8, Text: body})
	producer.FrameV1(model.ProcessFrameV1{Kind: "end", Attempt: 1, NativeIndex: 8})
	producer.FinishV1()
	authorization := processHTTPAuthorizeV1(t, handler)
	if authorization.Status != "authorized" || authorization.Scope == nil || authorization.Blocks != nil {
		t.Fatal("authorization returned source body or failed unit owner binding")
	}
	read := `{"schemaVersion":2,"action":"read","viewerToken":"` + processHTTPTokenV1 + `"}`
	collapsed := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, read))
	if collapsed.Status != "snapshot" || len(collapsed.Blocks) != 1 || collapsed.Blocks[0].Body != nil {
		t.Fatal("collapsed HTTP copy released native body")
	}
	block := collapsed.Blocks[0]
	expanded := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":"`+processHTTPTokenV1+`","expandedBlockIds":["`+block.BlockID+`"]}`))
	if expanded.Status != "snapshot" || expanded.Blocks[0].Body == nil || expanded.Blocks[0].Body.Text != body {
		t.Fatal("expanded HTTP copy lost frozen source bytes")
	}
	for _, digest := range []string{block.CaptureDigest, strings.Repeat("f", 64)} {
		opened := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayTargetPathV1, `{"schemaVersion":2,"action":"open_capture","viewerToken":"`+processHTTPTokenV1+`","blockId":"`+block.BlockID+`","captureDigest":"`+digest+`"}`))
		if digest == block.CaptureDigest {
			if opened.Status != "capture" || opened.Block == nil || opened.Block.Body == nil || opened.Block.Body.Text != body {
				t.Fatal("matching capture digest did not open frozen bytes")
			}
		} else if opened.Status != "unavailable" || opened.Block != nil || opened.Scope != nil {
			t.Fatal("substituted digest released body or scope")
		}
	}
	revoked := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"revoke","viewerToken":"`+processHTTPTokenV1+`"}`))
	if revoked.Status != "revoked" || revoked.Scope != nil {
		t.Fatal("document revocation response expanded")
	}
	denied := processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, read)
	if strings.TrimSpace(denied.Body.String()) != `{"status":"unavailable"}` {
		t.Fatal("revoked document retained body or response authority")
	}
}

func TestProcessLocalHTTPOversizedEncodedResponseWithholdsEntireCopy(t *testing.T) {
	handler, producer, _ := processHTTPFixtureV1(t)
	// JSON escaping doubles these complete body bytes. They fit the owner's
	// bounded display budget but exceed the separate HTTP response envelope cap.
	for index := 0; index < 96; index++ {
		producer.FrameV1(model.ProcessFrameV1{Kind: "start", Attempt: 1, NativeIndex: index})
		producer.FrameV1(model.ProcessFrameV1{Kind: "delta", Attempt: 1, NativeIndex: index, Text: strings.Repeat("\"", 1500)})
		producer.FrameV1(model.ProcessFrameV1{Kind: "end", Attempt: 1, NativeIndex: index})
	}
	producer.FinishV1()
	if response := processHTTPAuthorizeV1(t, handler); response.Status != "authorized" {
		t.Fatal("response quota fixture not authorized")
	}
	collapsed := processHTTPDecodedV1(t, processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":"`+processHTTPTokenV1+`"}`))
	if collapsed.Status != "snapshot" || len(collapsed.Blocks) != 96 {
		t.Fatal("response quota fixture lost bounded source blocks")
	}
	ids := make([]string, len(collapsed.Blocks))
	for index, block := range collapsed.Blocks {
		ids[index] = block.BlockID
	}
	encoded, _ := json.Marshal(ids)
	w := processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":"`+processHTTPTokenV1+`","expandedBlockIds":`+string(encoded)+`}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"status":"unavailable"}` {
		t.Fatal("oversized encoded HTTP copy leaked partial blocks or scope")
	}
}

func TestProcessLocalHTTPShutdownWakesPollAndRetiresProducerAndViewer(t *testing.T) {
	handler, producer, authority := processHTTPFixtureV1(t)
	authorized := processHTTPAuthorizeV1(t, handler)
	if authorized.Status != "authorized" {
		t.Fatal("shutdown fixture not authorized")
	}
	for len(authority.acquired) > 0 {
		<-authority.acquired
	}
	completed := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ProcessLocalDisplayReadPathV1, strings.NewReader(`{"schemaVersion":2,"action":"read","viewerToken":"`+processHTTPTokenV1+`","afterRevision":"`+authorized.Revision+`","waitMs":25000}`)))
		completed <- w
	}()
	select {
	case <-authority.acquired:
	case <-time.After(time.Second):
		t.Fatal("HTTP long poll never entered authority owner")
	}
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-completed:
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"status":"unavailable"}` || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("shutdown poll returned scope/body or changed cache boundary")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not wake bounded HTTP long poll")
	}
	producer.FrameV1(model.ProcessFrameV1{Kind: "start", Attempt: 1, NativeIndex: 9})
	producer.FrameV1(model.ProcessFrameV1{Kind: "delta", Attempt: 1, NativeIndex: 9, Text: "LATE_PRIVATE_BODY"})
	producer.FrameV1(model.ProcessFrameV1{Kind: "end", Attempt: 1, NativeIndex: 9})
	producer.FinishV1()
	w := processHTTPPostV1(t, handler, ProcessLocalDisplayReadPathV1, `{"schemaVersion":2,"action":"read","viewerToken":"`+processHTTPTokenV1+`"}`)
	if bytes.Contains(w.Body.Bytes(), []byte("LATE_PRIVATE_BODY")) || strings.TrimSpace(w.Body.String()) != `{"status":"unavailable"}` {
		t.Fatal("late producer revived a shutdown HTTP viewer")
	}
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatal("second owner close failed")
	}
	if err := (ProcessLocalDisplayHandlerV1{}).Shutdown(context.Background()); err != nil {
		t.Fatal("nil owner close failed")
	}
}

func TestProcessWebTargetRequiresCurrentActiveToolAuthorityV1(t *testing.T) {
	handler, _, _ := processHTTPFixtureV1(t)
	if got := processHTTPAuthorizeV1(t, handler); got.Status != "authorized" {
		t.Fatal("native lease denied")
	}
	body := `{"schemaVersion":2,"action":"open_web_link","viewerToken":"` + processHTTPTokenV1 + `","blockId":"block","inputDigest":"` + strings.Repeat("a", 64) + `","role":"request"}`
	response := processHTTPPostV1(t, handler, ProcessLocalDisplayTargetPathV1, body)
	var result processapp.WebLinkResponseV1
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Status != "unavailable" || result.TargetURL != "" {
		t.Fatal("missing sealed input produced target")
	}
}
