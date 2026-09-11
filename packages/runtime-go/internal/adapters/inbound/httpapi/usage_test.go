package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	usageapp "analytix.local/runtime-go/internal/app/usage"
)

func TestUsageHandlersGroupsThreadUsage(t *testing.T) {
	records := []usageapp.Record{
		{
			ThreadID:    "thread-a",
			Model:       "deepseek-chat",
			Provider:    "deepseek",
			CompletedAt: "2026-05-01T10:00:00Z",
			Usage: usageapp.Snapshot{
				PromptTokens:     12,
				CompletionTokens: 3,
				TotalTokens:      15,
				Turns:            1,
			},
		},
	}
	seenThreadID := ""
	handler := UsageHandlers{
		Records: func(threadID string) ([]usageapp.Record, error) {
			seenThreadID = threadID
			return records, nil
		},
		RuntimeResponse: func(records []usageapp.Record) (map[string]any, error) {
			return usageapp.RuntimeResponse(records, []string{"thread-a"}), nil
		},
	}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=thread&thread_id=thread-a", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if seenThreadID != "thread-a" {
		t.Fatalf("thread_id was not forwarded: %q", seenThreadID)
	}
	body := decodeUsageBody(t, recorder)
	bucket := usageBucketByField(t, body, "thread_id", "thread-a")
	if bucket["total_tokens"] != float64(15) || bucket["provider"] != "deepseek" {
		t.Fatalf("thread bucket mismatch: %#v", bucket)
	}
}

func TestUsageHandlersRejectsInvalidWindowBeforeResponse(t *testing.T) {
	handler := UsageHandlers{
		Records: func(threadID string) ([]usageapp.Record, error) {
			return nil, nil
		},
		RuntimeResponse: func(records []usageapp.Record) (map[string]any, error) {
			return usageapp.RuntimeResponse(records, nil), nil
		},
	}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(http.MethodGet, "/v1/usage?group_by=day&from=2026-05-01", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeUsageBody(t, recorder)
	if body["code"] != "validation_error" || body["message"] != testValidationFailureMessage {
		t.Fatalf("validation error mismatch: %#v", body)
	}
}

func TestUsageHandlersRejectUnavailableRuntimeInventoryWithoutLeakingCause(t *testing.T) {
	const privateCause = "SYNTHETIC_PRIVATE_USAGE_READ_PATH"
	handler := UsageHandlers{
		Records: func(string) ([]usageapp.Record, error) { return nil, nil },
		RuntimeResponse: func([]usageapp.Record) (map[string]any, error) {
			return nil, errors.New(privateCause)
		},
	}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(http.MethodGet, "/v1/usage", nil))
	if recorder.Code < 400 || recorder.Code == http.StatusNotFound || strings.Contains(recorder.Body.String(), privateCause) {
		t.Fatalf("runtime inventory failure became success, missing or leaked: status=%d", recorder.Code)
	}
}

func decodeUsageBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func usageBucketByField(t *testing.T, response map[string]any, field string, value string) map[string]any {
	t.Helper()
	buckets, _ := response["buckets"].([]any)
	for _, raw := range buckets {
		bucket, _ := raw.(map[string]any)
		if bucket != nil && bucket[field] == value {
			return bucket
		}
	}
	t.Fatalf("usage bucket %s=%s missing in %#v", field, value, response)
	return nil
}
