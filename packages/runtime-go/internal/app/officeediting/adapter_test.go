package officeediting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type fakeEditing struct {
	calls    []string
	args     []string
	document editingapp.Opened
	receipt  fileport.Receipt
	err      error
}

func (f *fakeEditing) Open(_ context.Context, workspace, path string) (editingapp.Opened, error) {
	f.calls = append(f.calls, "open")
	f.args = []string{workspace, path}
	return f.document, f.err
}
func (f *fakeEditing) Commit(_ context.Context, id, operation, revision, content string) (fileport.Receipt, error) {
	f.calls = append(f.calls, "commit")
	f.args = []string{id, operation, revision, content}
	return f.receipt, f.err
}
func (f *fakeEditing) Status(_ context.Context, id, operation string) (fileport.Receipt, error) {
	f.calls = append(f.calls, "status")
	f.args = []string{id, operation}
	return f.receipt, f.err
}
func (f *fakeEditing) Close(_ context.Context, id string) error {
	f.calls = append(f.calls, "close")
	f.args = []string{id}
	return f.err
}

func validCall(t *testing.T, operation, input string) adapterport.Call {
	t.Helper()
	principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	return adapterport.Call{Binding: adapterport.Binding{PackageID: "analytix-documents"}, Principal: principal, ContributionID: "workspace-editor", Operation: operation, Input: json.RawMessage(input)}
}
func requestBody(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
func decoded(t *testing.T, result adapterport.Result, err error) map[string]any {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(result.Output, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func invoke(t *testing.T, adapter *Adapter, call adapterport.Call) map[string]any {
	t.Helper()
	result, err := adapter.Invoke(context.Background(), call)
	return decoded(t, result, err)
}
func commitFields() map[string]any {
	return map[string]any{"sessionId": strings.Repeat("b", 48), "operationId": "operation_01", "baseRevision": strings.Repeat("c", 64), "content": "UEsDBAoAAAA="}
}
func goodInput(t *testing.T, operation string) string {
	t.Helper()
	fields := commitFields()
	switch operation {
	case "open-object":
		return `{"object":{"workspace":"/workspace","path":"sample.docx"}}`
	case "object-status":
		delete(fields, "baseRevision")
		delete(fields, "content")
	case "close-object":
		delete(fields, "baseRevision")
		delete(fields, "content")
		delete(fields, "operationId")
	}
	return requestBody(t, fields)
}

func TestPreviewOperationsDelegateExactly(t *testing.T) {
	for _, operation := range []string{"open-object", "close-object"} {
		t.Run(operation, func(t *testing.T) {
			fields := commitFields()
			fake := &fakeEditing{document: editingapp.Opened{SessionID: fields["sessionId"].(string), ObjectID: strings.Repeat("d", 64), Path: "sample.docx", Content: fields["content"].(string), Revision: fields["baseRevision"].(string)}, receipt: fileport.Receipt{OperationID: fields["operationId"].(string), Revision: fields["baseRevision"].(string), Status: fileport.StatusCommitted, SavedAt: "2026-09-14T00:00:00Z"}}
			readyCalls := 0
			adapter := New("docx", fake, func(context.Context) bool { readyCalls++; return true })
			out := invoke(t, adapter, validCall(t, operation, goodInput(t, operation)))
			if out["ok"] != true || len(fake.calls) != 1 || readyCalls != 1 {
				t.Fatalf("not delegated exactly once: %v %v %d", out, fake.calls, readyCalls)
			}
			var wantCall string
			var wantArgs []string
			switch operation {
			case "open-object":
				wantCall, wantArgs = "open", []string{"/workspace", "sample.docx"}
				if len(out) != 2 || !reflect.DeepEqual(out["document"], map[string]any{"sessionId": fake.document.SessionID, "objectId": fake.document.ObjectID, "path": fake.document.Path, "content": fake.document.Content, "revision": fake.document.Revision}) {
					t.Fatalf("document projection: %v", out)
				}
			case "close-object":
				wantCall, wantArgs = "close", []string{fields["sessionId"].(string)}
				if len(out) != 2 || out["closed"] != true {
					t.Fatalf("close projection: %v", out)
				}
			}
			if fake.calls[0] != wantCall || !reflect.DeepEqual(fake.args, wantArgs) {
				t.Fatalf("delegation %v %v", fake.calls, fake.args)
			}

		})
	}
}

func TestReadinessAndFixedPackageBinding(t *testing.T) {
	for kind, id := range map[string]string{"docx": "analytix-documents", "xlsx": "analytix-spreadsheets", "pptx": "analytix-presentations"} {
		t.Run(kind, func(t *testing.T) {
			ready := true
			calls := 0
			fake := &fakeEditing{}
			adapter := New(kind, fake, func(context.Context) bool { calls++; return ready })
			binding := adapterport.Binding{PackageID: id}
			state, err := adapter.Readiness(context.Background(), binding)
			if err != nil || !state.Available || !reflect.DeepEqual(state.Operations, []string{"open-object", "close-object"}) || calls != 1 {
				t.Fatalf("readiness %v %v", state, err)
			}
			state.Operations[0] = "shell"
			ready = false
			state, err = adapter.Readiness(context.Background(), binding)
			if err != nil || state.Available || state.Operations[0] != "open-object" {
				t.Fatalf("disabled readiness %v %v", state, err)
			}
			call := validCall(t, "open-object", goodInput(t, "open-object"))
			call.Binding = binding
			if _, err := adapter.Invoke(context.Background(), call); !errors.Is(err, ErrUnavailable) || len(fake.calls) != 0 {
				t.Fatalf("disabled invoke %v", err)
			}
			binding.PackageID = "wrong-package"
			if _, err := adapter.Readiness(context.Background(), binding); !errors.Is(err, ErrBinding) {
				t.Fatalf("wrong readiness binding %v", err)
			}
			call.Binding = binding
			if _, err := adapter.Invoke(context.Background(), call); !errors.Is(err, ErrBinding) || len(fake.calls) != 0 {
				t.Fatalf("wrong invoke binding %v", err)
			}
		})
	}
	for _, adapter := range []*Adapter{nil, New("doc", &fakeEditing{}, func(context.Context) bool { return true })} {
		if _, err := adapter.Readiness(context.Background(), adapterport.Binding{PackageID: "analytix-documents"}); !errors.Is(err, ErrBinding) {
			t.Fatalf("unknown kind %v", err)
		}
	}
	for _, adapter := range []*Adapter{New("docx", nil, func(context.Context) bool { return true }), New("docx", &fakeEditing{}, nil)} {
		state, err := adapter.Readiness(context.Background(), adapterport.Binding{PackageID: "analytix-documents"})
		if err != nil || state.Available {
			t.Fatalf("missing dependency %v %v", state, err)
		}
	}
}

// Adapter validates Host-projected structure/digest, not current authority.
// Host ValidateCurrent and objectediting.Service own live principal matching.
func TestRejectsDamagedHostPrincipalAndCallerPrincipalFields(t *testing.T) {
	fake := &fakeEditing{}
	adapter := New("docx", fake, func(context.Context) bool { return true })
	call := validCall(t, "close-object", goodInput(t, "close-object"))
	call.Principal.UserID = "different-caller"
	if _, err := adapter.Invoke(context.Background(), call); !errors.Is(err, ErrBinding) {
		t.Fatalf("damaged principal %v", err)
	}
	for _, field := range []string{"principal", "identity", "Principal", "binding", "packageId"} {
		input := map[string]any{"sessionId": strings.Repeat("b", 48), field: map[string]any{"userId": "different-caller"}}
		out := invoke(t, adapter, validCall(t, "close-object", requestBody(t, input)))
		if out["code"] != "invalid_request" {
			t.Fatalf("caller authority accepted: %v", out)
		}
	}
	call = validCall(t, "close-object", goodInput(t, "close-object"))
	call.ContributionID = "arbitrary"
	if _, err := adapter.Invoke(context.Background(), call); !errors.Is(err, ErrBinding) {
		t.Fatalf("wrong contribution %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatal("invalid authority reached service")
	}
}

func TestRejectsStrictMalformedInputAndNoop(t *testing.T) {
	cases := []struct{ name, operation, input string }{
		{"duplicate", "open-object", `{"object":{"workspace":"/workspace","path":"a.docx","path":"b.docx"}}`},
		{"duplicate_outer", "open-object", `{"object":{},"object":{"workspace":"/workspace","path":"a.docx"}}`},
		{"unknown", "open-object", `{"object":{"workspace":"/workspace","path":"a.docx","executable":"sh"}}`},
		{"case", "open-object", `{"Object":{"workspace":"/workspace","path":"a.docx"}}`},
		{"nested_case", "open-object", `{"object":{"Workspace":"/workspace","path":"a.docx"}}`},
		{"null", "open-object", `{"object":null}`},
		{"null_string", "open-object", `{"object":{"workspace":"/workspace","path":null}}`},
		{"missing", "open-object", `{"object":{"workspace":"/workspace"}}`},
		{"relative", "open-object", `{"object":{"workspace":"relative","path":"a.docx"}}`},
		{"empty_path", "open-object", `{"object":{"workspace":"/workspace","path":""}}`},
		{"null_root", "close-object", `null`},
		{"array_root", "close-object", `[]`},
		{"trailing", "open-object", goodInput(t, "open-object") + ` {}`},
		{"noop", "noop", `{}`},
		{"shell", "shell", `{"command":"whoami"}`},
		{"unpaired_unicode", "open-object", `{"object":{"workspace":"/workspace","path":"\ud800"}}`},
		{"nul_path", "open-object", `{"object":{"workspace":"/workspace","path":"a\u0000.docx"}}`},
		{"long_path", "open-object", requestBody(t, map[string]any{"object": map[string]any{"workspace": "/workspace", "path": strings.Repeat("a", 4097)}})},
	}
	for _, value := range []any{nil, 42, "", "bad_session", strings.Repeat("a", 47), strings.Repeat("a", 49)} {
		cases = append(cases, struct{ name, operation, input string }{fmt.Sprintf("session_invalid_%v", value), "close-object", requestBody(t, map[string]any{"sessionId": value})})
	}
	cases = append(cases, struct{ name, operation, input string }{"close_missing", "close-object", `{}`})
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeEditing{}
			out := invoke(t, New("docx", fake, func(context.Context) bool { return true }), validCall(t, test.operation, test.input))
			if out["ok"] != false || out["code"] != "invalid_request" || len(out) != 3 || len(fake.calls) != 0 {
				t.Fatalf("malformed input delegated: %v %v", out, fake.calls)
			}
		})
	}
}

func TestPreviewCoreErrorsAndCausesRedacted(t *testing.T) {
	cases := []struct {
		err          error
		code, status string
	}{
		{fileport.ErrConflict, "conflict", fileport.StatusConflict},
		{fileport.ErrPersistence, "persistence_failure", fileport.StatusUnknown},
		{fileport.ErrOperationMismatch, "operation_mismatch", fileport.StatusUnknown},
		{fileport.ErrOperationNotFound, "operation_not_found", ""},
		{fileport.ErrInvalidInput, "invalid_request", ""},
		{fileport.ErrForbidden, "forbidden", ""},
		{fileport.ErrNotText, "not_text", ""},
		{fileport.ErrTooLarge, "too_large", ""},
		{editingapp.ErrSession, "session_invalid", ""},
		{editingapp.ErrCapacity, "capacity", ""},
		{editingapp.ErrUnavailable, "unavailable", ""},
		{errors.New("private-cause"), "unavailable", fileport.StatusUnknown},
	}
	for _, operation := range []string{"open-object", "close-object"} {
		for _, test := range cases {
			t.Run(operation+"_"+test.code, func(t *testing.T) {
				fake := &fakeEditing{err: fmt.Errorf("private-cause: %w", test.err)}
				if test.status != "" {
					fake.receipt = fileport.Receipt{OperationID: "operation_01", Revision: strings.Repeat("c", 64), Status: test.status, SavedAt: "2026-09-14T00:00:00Z"}
				}
				out := invoke(t, New("docx", fake, func(context.Context) bool { return true }), validCall(t, operation, goodInput(t, operation)))
				if out["ok"] != false || out["code"] != test.code || out["message"] != "The object operation could not be completed." {
					t.Fatalf("error projection %v", out)
				}
				if len(out) != 3 {
					t.Fatalf("unexpected receipt: %v", out)
				}
				if strings.Contains(requestBody(t, out), "private-cause") {
					t.Fatal("cause leaked")
				}
			})
		}
	}
	for _, operation := range []string{"open-object", "close-object"} {
		fake := &fakeEditing{err: fileport.ErrForbidden}
		out := invoke(t, New("docx", fake, func(context.Context) bool { return true }), validCall(t, operation, goodInput(t, operation)))
		if out["code"] != "forbidden" || len(out) != 3 {
			t.Fatalf("error projection %v", out)
		}
	}
}

func TestContextCancellationAndReadinessRecheck(t *testing.T) {
	fake := &fakeEditing{}
	ready := true
	adapter := New("docx", fake, func(context.Context) bool { return ready })
	call := validCall(t, "close-object", goodInput(t, "close-object"))
	if state, err := adapter.Readiness(context.Background(), call.Binding); err != nil || !state.Available {
		t.Fatal("initial readiness")
	}
	ready = false
	if _, err := adapter.Invoke(context.Background(), call); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("stale readiness %v", err)
	}
	ready = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.Invoke(ctx, call); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancelled %v", err)
	}
	if _, err := adapter.Invoke(nil, call); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil context %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatal("unavailable reached service")
	}
}

func TestWriteOperationsForbiddenWithoutDelegation(t *testing.T) {
	for kind, id := range map[string]string{"docx": "analytix-documents", "xlsx": "analytix-spreadsheets", "pptx": "analytix-presentations"} {
		for _, operation := range []string{"commit-object", "object-status"} {
			t.Run(kind+"_"+operation, func(t *testing.T) {
				fake := &fakeEditing{}
				adapter := New(kind, fake, func(context.Context) bool { return true })
				for _, input := range []string{goodInput(t, operation), `{}`, `not-json`} {
					call := validCall(t, operation, input)
					call.Binding.PackageID = id
					out := invoke(t, adapter, call)
					if out["ok"] != false || out["code"] != "forbidden" || len(out) != 3 || len(fake.calls) != 0 {
						t.Fatalf("preview delegated a write operation: %v %v", out, fake.calls)
					}
				}
			})
		}
	}
}
