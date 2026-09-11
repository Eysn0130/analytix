package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	threadapp "analytix.local/runtime-go/internal/app/thread"
)

type caseProjectServiceStub struct {
	projects      []map[string]any
	indexStatus   string
	threads       []map[string]any
	detail        map[string]any
	projectsErr   error
	threadsErr    error
	detailErr     error
	lastProjectID string
	lastLimit     int
}

func (s *caseProjectServiceStub) ListCaseProjectSummaries(limit int) ([]map[string]any, string, error) {
	s.lastLimit = limit
	return s.projects, s.indexStatus, s.projectsErr
}

func (s *caseProjectServiceStub) ListCaseProjectThreads(caseProjectID string, limit int) ([]map[string]any, error) {
	s.lastProjectID, s.lastLimit = caseProjectID, limit
	return s.threads, s.threadsErr
}

func (s *caseProjectServiceStub) GetCaseProjectDetail(caseProjectID string, limit int) (map[string]any, error) {
	s.lastProjectID, s.lastLimit = caseProjectID, limit
	return s.detail, s.detailErr
}

func TestCaseProjectHandlersRouteApplicationService(t *testing.T) {
	service := &caseProjectServiceStub{
		projects:    []map[string]any{{"id": "case-1"}},
		indexStatus: "ready",
		threads:     []map[string]any{{"id": "thread-1"}},
		detail:      map[string]any{"id": "case-1"},
	}
	handler := CaseProjectHandlers{Service: service}
	for _, test := range []struct {
		name string
		path string
		key  string
	}{
		{name: "summaries", path: "/v1/case-projects?limit=7", key: "caseProjects"},
		{name: "threads", path: "/v1/case-projects/case-1/threads?limit=8", key: "threads"},
		{name: "detail", path: "/v1/case-projects/case-1/detail?limit=9", key: "id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.Handle(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			body := decodeCaseProjectBody(t, recorder)
			if recorder.Code != http.StatusOK || body[test.key] == nil {
				t.Fatalf("unexpected case-project response code=%d body=%#v", recorder.Code, body)
			}
		})
	}
	if service.lastProjectID != "case-1" || service.lastLimit != 9 {
		t.Fatalf("route identity was not projected to application service: id=%q limit=%d", service.lastProjectID, service.lastLimit)
	}
}

func TestCaseProjectHandlersMapBoundaryFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler CaseProjectHandlers
		method  string
		path    string
		status  int
		code    string
	}{
		{name: "method", handler: CaseProjectHandlers{}, method: http.MethodPost, path: "/v1/case-projects", status: http.StatusMethodNotAllowed, code: "method_not_allowed"},
		{name: "missing service", handler: CaseProjectHandlers{}, method: http.MethodGet, path: "/v1/case-projects", status: http.StatusInternalServerError, code: "internal_error"},
		{name: "unknown route", handler: CaseProjectHandlers{Service: &caseProjectServiceStub{}}, method: http.MethodGet, path: "/v1/case-projects/case-1/unknown", status: http.StatusNotFound, code: "not_found"},
		{name: "missing detail", handler: CaseProjectHandlers{Service: &caseProjectServiceStub{detailErr: threadapp.ErrCaseProjectNotFound}}, method: http.MethodGet, path: "/v1/case-projects/case-1/detail", status: http.StatusNotFound, code: "not_found"},
		{name: "service error", handler: CaseProjectHandlers{Service: &caseProjectServiceStub{threadsErr: errors.New("index unavailable")}}, method: http.MethodGet, path: "/v1/case-projects/case-1/threads", status: http.StatusInternalServerError, code: "internal_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			test.handler.Handle(recorder, httptest.NewRequest(test.method, test.path, nil))
			body := decodeCaseProjectBody(t, recorder)
			if recorder.Code != test.status || body["code"] != test.code {
				t.Fatalf("unexpected failure projection code=%d body=%#v", recorder.Code, body)
			}
		})
	}
}

func decodeCaseProjectBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode case-project response: %v body=%q", err, recorder.Body.String())
	}
	return body
}
