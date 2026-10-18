package webfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestExecuteToolRejectsDisabledConfig(t *testing.T) {
	output, isError := ExecuteTool(ToolInput{
		Config: runtimeinfoapp.WebConfig{},
		Args:   map[string]any{"url": "https://example.com"},
	})
	if !isError {
		t.Fatalf("disabled web_fetch should fail: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["code"] != "provider_unavailable" || record["outcome"] != "disabled" {
		t.Fatalf("disabled output mismatch: %#v", output)
	}
	assertFetchMeasurementsAbsent(t, record)
}

func TestExecuteToolValidatesRequestBeforeFetching(t *testing.T) {
	output, isError := ExecuteTool(ToolInput{
		Config: runtimeinfoapp.WebConfig{Enabled: true, FetchEnabled: true},
		Args:   map[string]any{"url": "file:///tmp/nope"},
	})
	if !isError {
		t.Fatalf("invalid URL should fail: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["code"] != "policy_blocked" || record["outcome"] != "policy_blocked" {
		t.Fatalf("invalid URL output mismatch: %#v", output)
	}
	assertFetchMeasurementsAbsent(t, record)
}

func TestExecuteToolRejectedRequestsHaveNoHTTPMeasurements(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    map[string]any
		outcome string
	}{
		{name: "missing URL", args: map[string]any{}, outcome: "invalid_url"},
		{name: "relative URL", args: map[string]any{"url": "/synthetic/page"}, outcome: "invalid_url"},
		{name: "SSRF blocked", args: map[string]any{"url": "http://169.254.169.254/latest"}, outcome: "ssrf_blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, isError := ExecuteTool(ToolInput{
				Config: runtimeinfoapp.WebConfig{Enabled: true, FetchEnabled: true},
				Args:   tc.args,
			})
			record, ok := output.(map[string]any)
			if !isError || !ok || record["outcome"] != tc.outcome {
				t.Fatalf("request refusal mismatch: want=%s output=%#v", tc.outcome, output)
			}
			assertFetchMeasurementsAbsent(t, record)
		})
	}
}

func TestExecuteToolPreservesResponseMetadataAndModelText(t *testing.T) {
	const body = "<title>Tool source</title><p>existing model text</p>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	requestURL := server.URL + "/page"
	output, isError := ExecuteTool(ToolInput{
		Context: context.Background(),
		Config:  runtimeinfoapp.WebConfig{Enabled: true, FetchEnabled: true, MaxFetchBytes: 4096},
		Args:    map[string]any{"url": requestURL, "max_bytes": 4096.0, "timeout_ms": 1000.0},
	})
	record, ok := output.(map[string]any)
	if isError || !ok {
		t.Fatalf("local tool fetch should succeed: %#v", output)
	}
	assertFetchResponseMetadata(t, record, "success", http.StatusCreated, len(body), false)
	if stringField(record, "title") != "Tool source" || stringField(record, "text") != "Tool source existing model text" {
		t.Fatalf("new metadata must preserve the tool's existing model content: %#v", record)
	}
	sources, ok := record["sources"].([]any)
	if !ok || len(sources) != 1 {
		t.Fatalf("new metadata must preserve the tool's existing source: %#v", record["sources"])
	}
	source, ok := sources[0].(map[string]any)
	if !ok || stringField(source, "url") != requestURL || stringField(source, "title") != "Tool source" {
		t.Fatalf("tool source mismatch: %#v", sources[0])
	}
}

func TestExecuteToolCapturesDisabledAndInvalidInputWithoutHTTPV1(t *testing.T) {
	for _, input := range []ToolInput{
		{Config: runtimeinfoapp.WebConfig{}, Args: map[string]any{"url": "https://example.test/actual"}},
		{Config: runtimeinfoapp.WebConfig{Enabled: true, FetchEnabled: true}, Args: map[string]any{}},
	} {
		ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
		input.Context = ctx
		output, isError := ExecuteTool(input)
		record, _ := output.(map[string]any)
		capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
		if !isError || !ok || capture.Kind != "web_fetch" || capture.Status != "failed" || capture.Web.ResponseComplete || !capture.Web.ToolIsError || capture.Web.StatusCode != nil || capture.Web.FetchTruncated != nil || capture.Body != stringField(record, "error") || capture.Web.Error == nil || capture.WebTargets != nil {
			t.Fatal("pre-request failure lost capture or invented HTTP evidence")
		}
	}
}
