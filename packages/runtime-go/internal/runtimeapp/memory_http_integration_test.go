package runtimeapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
)

func TestMemoryHTTPPatchWithholdsExistingTamperedCaseRecord(t *testing.T) {
	dataDir := t.TempDir()
	store, err := filestore.NewPersistentMemoryStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{"content": "general manual preference"})
	if err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	authorityRef := "cer1_" + strings.Repeat("a", 64)
	if err := filestore.WriteJSONMapFile(filepath.Join(dataDir, "memory", "records", id+".json"), map[string]any{
		"id": id, "content": "raw case content", "scope": "workspace", "tags": []any{},
		"confidence": float64(1), "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
		"provenance": "manual-general", "captureMode": "manual", "modelInjection": false,
		"sourceThreadId": "private-thread", "authorityRef": authorityRef,
	}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	httpapi.MemoryHandlers{Store: store}.HandleRecordPath(
		recorder,
		httptest.NewRequest(http.MethodPatch, "/v1/memory/"+id, strings.NewReader(`{"disabled":true}`)),
	)
	var body map[string]any
	decodeErr := json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusBadRequest || decodeErr != nil ||
		body["code"] != "validation_error" ||
		body["message"] != "The request did not satisfy the runtime contract." {
		t.Fatalf("tampered patch did not fail closed: status=%d body=%s err=%v", recorder.Code, recorder.Body.String(), decodeErr)
	}
	for _, private := range []string{"raw case content", "private-thread", authorityRef, "sourceThreadId", "authorityRef"} {
		if strings.Contains(recorder.Body.String(), private) {
			t.Fatalf("tampered patch HTTP response exposed %q: %s", private, recorder.Body.String())
		}
	}
}
