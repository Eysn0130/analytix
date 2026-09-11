package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
)

type attachmentStoreStub struct {
	createdBody map[string]any
	metadata    map[string]any
	dataBase64  string
	found       bool
	err         error
	createErr   error
}

func (s *attachmentStoreStub) Create(body map[string]any) (map[string]any, error) {
	s.createdBody = body
	if s.createErr != nil {
		return nil, s.createErr
	}
	return map[string]any{"id": "att_created"}, nil
}

func (s *attachmentStoreStub) Diagnostics() map[string]any {
	return map[string]any{"enabled": true}
}

func (s *attachmentStoreStub) Metadata(id string) (map[string]any, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	if !s.found {
		return nil, false, nil
	}
	return s.metadata, true, nil
}

func (s *attachmentStoreStub) Content(id string) (map[string]any, string, bool, error) {
	if s.err != nil {
		return nil, "", false, s.err
	}
	if !s.found {
		return nil, "", false, nil
	}
	return s.metadata, s.dataBase64, true, nil
}

func TestAttachmentHandlersUploadDiagnosticsAndContentContract(t *testing.T) {
	store := &attachmentStoreStub{
		metadata:   map[string]any{"id": "att_1"},
		dataBase64: "aGVsbG8=",
		found:      true,
	}
	handlers := AttachmentHandlers{
		Store: store,
		Authorize: func(_ context.Context, metadata map[string]any, threadID string, workspace string) bool {
			return threadID == "thread_1"
		},
	}

	recorder := httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/attachments", strings.NewReader(`{"name":"note.txt","dataBase64":"aGVsbG8=","threadId":"thread_1","workspace":"/workspace"}`)))
	if recorder.Code != http.StatusCreated || store.createdBody["name"] != "note.txt" {
		t.Fatalf("upload contract mismatch: status=%d body=%#v response=%s", recorder.Code, store.createdBody, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandleDiagnostics(recorder, httptest.NewRequest(http.MethodGet, "/v1/attachments/diagnostics", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enabled":true`) {
		t.Fatalf("diagnostics contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandlePath(recorder, httptest.NewRequest(http.MethodGet, "/v1/attachments/att_1/content?thread_id=thread_1&workspace=%2Fworkspace", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"dataBase64":"aGVsbG8="`) {
		t.Fatalf("content contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAttachmentHandlersValidationForbiddenAndErrorContracts(t *testing.T) {
	store := &attachmentStoreStub{
		metadata:   map[string]any{"id": "att_1"},
		dataBase64: "aGVsbG8=",
		found:      true,
		createErr:  errors.New("bad upload"),
	}
	handlers := AttachmentHandlers{
		Store:     store,
		Authorize: func(context.Context, map[string]any, string, string) bool { return false },
	}

	recorder := httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/attachments", strings.NewReader(`{"name":"note.txt","dataBase64":"aGVsbG8=","threadId":"thread_1","workspace":"/workspace"}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), testValidationFailureMessage) || strings.Contains(recorder.Body.String(), "bad upload") {
		t.Fatalf("validation contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handlers.HandlePath(recorder, httptest.NewRequest(http.MethodGet, "/v1/attachments/att_1/content?thread_id=wrong&workspace=%2Fworkspace", nil))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), testForbiddenFailureMessage) {
		t.Fatalf("forbidden contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	store.found = false
	recorder = httptest.NewRecorder()
	handlers.HandlePath(recorder, httptest.NewRequest(http.MethodGet, "/v1/attachments/missing?thread_id=thread_1&workspace=%2Fworkspace", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("not found contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	store.err = errors.New("disk failed")
	store.found = true
	recorder = httptest.NewRecorder()
	handlers.HandlePath(recorder, httptest.NewRequest(http.MethodGet, "/v1/attachments/att_1?thread_id=thread_1&workspace=%2Fworkspace", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("internal error contract mismatch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAttachmentHandlersRejectInvalidJSONAndMethods(t *testing.T) {
	handlers := AttachmentHandlers{Store: &attachmentStoreStub{}}
	recorder := httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodGet, "/v1/attachments", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method contract mismatch: %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/attachments", strings.NewReader(`{`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON contract mismatch: %d", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "validation_error" {
		t.Fatalf("invalid JSON body mismatch: %#v", body)
	}
}

func TestAttachmentHandlersRequireClosedScopeBoundSchema(t *testing.T) {
	handlers := AttachmentHandlers{
		Store:     &attachmentStoreStub{metadata: map[string]any{"id": "att_1"}, found: true},
		Authorize: func(context.Context, map[string]any, string, string) bool { return true },
	}
	for name, body := range map[string]string{
		"missing thread":    `{"name":"note.txt","dataBase64":"aGVsbG8=","workspace":"/workspace"}`,
		"missing workspace": `{"name":"note.txt","dataBase64":"aGVsbG8=","threadId":"thread_1"}`,
		"unknown field":     `{"name":"note.txt","dataBase64":"aGVsbG8=","threadId":"thread_1","workspace":"/workspace","scope":"global"}`,
		"nested unknown":    `{"name":"note.txt","dataBase64":"aGVsbG8=","threadId":"thread_1","workspace":"/workspace","textFallback":{"dataBase64":"eA==","mimeType":"text/plain","byteSize":1,"owner":"caller"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/attachments", strings.NewReader(body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected closed schema rejection, got %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	for name, path := range map[string]string{
		"missing thread":    "/v1/attachments/att_1?workspace=%2Fworkspace",
		"missing workspace": "/v1/attachments/att_1?thread_id=thread_1",
		"unknown query":     "/v1/attachments/att_1?thread_id=thread_1&workspace=%2Fworkspace&case_id=case_1",
		"duplicate scope":   "/v1/attachments/att_1?thread_id=thread_1&thread_id=thread_2&workspace=%2Fworkspace",
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handlers.HandlePath(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected exact query rejection, got %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAttachmentUploadRequiresOwnerAuthorityCommitBeforePublicResponse(t *testing.T) {
	store := &attachmentStoreStub{}
	handlers := AttachmentHandlers{
		Store: store,
		CommitUpload: func(context.Context, map[string]any) error {
			return errors.New("PRIVATE_OWNER_STORE_FAILURE")
		},
	}
	recorder := httptest.NewRecorder()
	handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/attachments", strings.NewReader(
		`{"name":"note.txt","dataBase64":"aGVsbG8=","threadId":"thread_1","workspace":"/workspace"}`,
	)))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "attachment_authority_unavailable") {
		t.Fatalf("owner authority failure response mismatch: %d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "PRIVATE_OWNER_STORE_FAILURE") {
		t.Fatalf("private owner authority error leaked: %s", recorder.Body.String())
	}
}

func TestAttachmentTransactionUploadPreservesPublicErrorClassification(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "owner authority", err: appturn.ErrAttachmentNotAuthorized, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "unsafe binding", err: domainattachment.ErrUploadBindingUnsafe, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "invalid input", err: errors.Join(domainattachment.ErrUploadInputInvalid, errors.New("unsupported image MIME type")), wantStatus: http.StatusBadRequest, wantCode: "validation_error"},
		{name: "private transaction failure", err: errors.New("PRIVATE_ATTACHMENT_CAS_FAILURE"), wantStatus: http.StatusServiceUnavailable, wantCode: "attachment_upload_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handlers := AttachmentHandlers{
				Store: &attachmentStoreStub{},
				Upload: func(context.Context, map[string]any) (map[string]any, error) {
					return nil, test.err
				},
			}
			recorder := httptest.NewRecorder()
			handlers.HandleCollection(recorder, httptest.NewRequest(http.MethodPost, "/v1/attachments", strings.NewReader(
				`{"name":"note.txt","dataBase64":"aGVsbG8=","threadId":"thread_1","workspace":"/workspace"}`,
			)))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != test.wantCode {
				t.Fatalf("code = %#v, want %q: %#v", body["code"], test.wantCode, body)
			}
			if strings.Contains(recorder.Body.String(), "PRIVATE_ATTACHMENT_CAS_FAILURE") {
				t.Fatalf("private transaction error leaked: %s", recorder.Body.String())
			}
		})
	}
}
