package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const (
	testValidationFailureMessage = "The request did not satisfy the runtime contract."
	testNotFoundFailureMessage   = "The requested resource was not found."
	testForbiddenFailureMessage  = "The request is not authorized."
	testConflictFailureMessage   = "The request conflicts with the current runtime state."
	testInternalFailureMessage   = "The runtime request could not be completed safely."
	testMethodFailureMessage     = "The HTTP method is not allowed for this endpoint."
)

func TestAuthorizedHonorsBearerTokenAndInsecureMode(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/info", nil)
	if Authorized(request, DefaultRuntimeToken) {
		t.Fatal("request without bearer token should not be authorized")
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	if !Authorized(request, DefaultRuntimeToken) {
		t.Fatal("request with bearer token should be authorized")
	}
	request.Header.Del("Authorization")
	if !Authorized(request, DefaultRuntimeToken, true) {
		t.Fatal("insecure mode should authorize request")
	}
}

func TestWriteJSONAndMethodNotAllowedShape(t *testing.T) {
	recorder := httptest.NewRecorder()
	MethodNotAllowed(recorder)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type: %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "method_not_allowed" || body["message"] != testMethodFailureMessage {
		t.Fatalf("unexpected response body: %#v", body)
	}
}

func TestIntQueryRejectsInvalidOrNegativeValues(t *testing.T) {
	values := url.Values{}
	values.Set("limit", "12")
	if got := IntQuery(values, "limit"); got != 12 {
		t.Fatalf("unexpected positive query value: %d", got)
	}
	values.Set("limit", "-1")
	if got := IntQuery(values, "limit"); got != 0 {
		t.Fatalf("negative query value should clamp to zero: %d", got)
	}
	values.Set("limit", "abc")
	if got := IntQuery(values, "limit"); got != 0 {
		t.Fatalf("invalid query value should clamp to zero: %d", got)
	}
}

func TestHealthAndRuntimeCapabilityResponseKeepAnalytixSurface(t *testing.T) {
	health := HealthResponse()
	if health["service"] != "analytix" || health["mode"] != "serve" {
		t.Fatalf("unexpected health response: %#v", health)
	}
	info := RuntimeInfoResponse("2026-07-02T00:00:00Z")
	if info.Capabilities.ContractVersion != 1 {
		t.Fatalf("unexpected capabilities: %#v", info.Capabilities)
	}
}

func TestAvailabilityStateHelpersKeepContractShape(t *testing.T) {
	available := AvailableState()
	if available["status"] != "available" || available["enabled"] != true || available["available"] != true {
		t.Fatalf("unexpected available state: %#v", available)
	}
	unavailable := UnavailableState("missing dependency")
	if unavailable["status"] != "unavailable" || unavailable["enabled"] != false || unavailable["reason"] != "missing dependency" {
		t.Fatalf("unexpected unavailable state: %#v", unavailable)
	}
	disabled := DisabledState("disabled by config")
	if disabled["status"] != "disabled" || disabled["available"] != false || disabled["reason"] != "disabled by config" {
		t.Fatalf("unexpected disabled state: %#v", disabled)
	}
}
