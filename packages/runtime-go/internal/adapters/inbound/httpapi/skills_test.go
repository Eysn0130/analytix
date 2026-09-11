package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSkillHandlers(t *testing.T) {
	handler := SkillHandlers{Service: skillCatalogStub{response: map[string]any{"enabled": true}}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	handler.Handle(recorder, request)
	body := decodeSkillBody(t, recorder)
	if recorder.Code != http.StatusOK || body["enabled"] != true {
		t.Fatalf("unexpected skill response code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/skills", nil)
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d", recorder.Code)
	}

	handler = SkillHandlers{}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	handler.Handle(recorder, request)
	body = decodeSkillBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["message"] != testInternalFailureMessage {
		t.Fatalf("unexpected missing service response code=%d body=%#v", recorder.Code, body)
	}
}

func decodeSkillBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type skillCatalogStub struct {
	response map[string]any
}

func (s skillCatalogStub) Skills() map[string]any {
	return s.response
}

var _ SkillCatalogService = skillCatalogStub{}
