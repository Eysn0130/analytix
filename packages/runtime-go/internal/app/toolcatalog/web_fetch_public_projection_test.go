package toolcatalog_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	webfetch "analytix.local/runtime-go/internal/adapters/outbound/webfetch"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	netclient "analytix.local/runtime-go/internal/netclient"
)

func TestWebFetchProducerToClosedPublicItem(t *testing.T) {
	const privatePage = "<title>PRIVATE_TITLE_SENTINEL</title><p>PRIVATE_BODY_SENTINEL</p>"
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		maxBytes  int
		outcome   string
		isError   bool
		truncated bool
	}{
		{name: "actual 201 success", status: 201, body: privatePage, maxBytes: 1024, outcome: "success"},
		{name: "actual 204 empty response", status: 204, maxBytes: 1024, outcome: "success"},
		{name: "actual 503 failure", status: 503, body: privatePage, maxBytes: 1024, outcome: "http_error", isError: true},
		{name: "measured truncation", status: 206, body: privatePage, maxBytes: 8, outcome: "success", truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			output, isError := publicWebFetchTestClientV1().Fetch(context.Background(), webfetch.Request{
				URL: server.URL + "/PRIVATE_URL_SENTINEL?secret=PRIVATE_QUERY_SENTINEL", MaxBytes: tc.maxBytes, Timeout: time.Second,
			})
			if isError != tc.isError {
				t.Fatalf("producer isError=%t want=%t", isError, tc.isError)
			}
			if tc.body != "" && !tc.truncated && !strings.Contains(output["text"].(string), "PRIVATE_BODY_SENTINEL") {
				t.Fatal("producer lost the existing model text")
			}
			projection := toolcatalogapp.BuildPublicToolResultProjectionV1("web_fetch", output, isError)
			wantCount := int64(len(tc.body))
			if tc.truncated {
				wantCount = int64(tc.maxBytes)
			}
			if projection.ProjectionKind != domaintoolresult.ProjectionWebFetchStatus || projection.Web == nil ||
				projection.Web.Outcome != tc.outcome || projection.Web.StatusCode == nil || *projection.Web.StatusCode != int64(tc.status) ||
				projection.Web.ByteCount == nil || *projection.Web.ByteCount != wantCount ||
				projection.Web.Truncated == nil || *projection.Web.Truncated != tc.truncated {
				t.Fatalf("producer response metadata did not survive public projection: %#v", projection)
			}
			assertPublicWebItemV1(t, projection, isError)
		})
	}
}

func TestWebFetchReadFailureAndBlockedRedirectPublicMetadata(t *testing.T) {
	t.Run("read failure keeps measured bytes without a false truncation claim", func(t *testing.T) {
		const partial = "PRIVATE_PARTIAL_BODY_SENTINEL"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(len(partial)+10))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(partial))
		}))
		defer server.Close()
		output, isError := publicWebFetchTestClientV1().Fetch(context.Background(), webfetch.Request{
			URL: server.URL + "/PRIVATE_URL_SENTINEL", MaxBytes: 1024, Timeout: time.Second,
		})
		projection := toolcatalogapp.BuildPublicToolResultProjectionV1("web_fetch", output, isError)
		if !isError || projection.Web == nil || projection.Web.Outcome != "read_error" ||
			projection.Web.StatusCode == nil || *projection.Web.StatusCode != 200 ||
			projection.Web.ByteCount == nil || *projection.Web.ByteCount != int64(len(partial)) || projection.Web.Truncated != nil {
			t.Fatalf("read failure metadata mismatch: %#v", projection)
		}
		assertPublicWebItemV1(t, projection, isError)
	})
	t.Run("blocked redirect keeps only the observed HTTP status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/PRIVATE_URL_SENTINEL", http.StatusFound)
		}))
		defer server.Close()
		output, isError := publicWebFetchTestClientV1().Fetch(context.Background(), webfetch.Request{
			URL: server.URL, MaxBytes: 1024, Timeout: time.Second,
		})
		projection := toolcatalogapp.BuildPublicToolResultProjectionV1("web_fetch", output, isError)
		if !isError || projection.Web == nil || projection.Web.Outcome != "ssrf_blocked" ||
			projection.Web.StatusCode == nil || *projection.Web.StatusCode != 302 || projection.Web.ByteCount != nil || projection.Web.Truncated != nil {
			t.Fatalf("blocked redirect metadata mismatch: %#v", projection)
		}
		assertPublicWebItemV1(t, projection, isError)
	})
}

func TestWebFetchPreResponseFailureDoesNotInventHTTPMetadata(t *testing.T) {
	for _, tc := range []struct {
		name    string
		url     string
		policy  webfetch.Policy
		outcome string
	}{
		{name: "invalid URL", url: "::", outcome: "invalid_url"},
		{name: "policy rejection", url: "http://127.0.0.1/PRIVATE_URL_SENTINEL", policy: webfetch.Policy{DenyDomains: []string{"127.0.0.1"}}, outcome: "policy_blocked"},
		{name: "SSRF rejection", url: "http://169.254.169.254/PRIVATE_URL_SENTINEL", outcome: "ssrf_blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := publicWebFetchTestClientV1()
			client.Policy = tc.policy
			output, isError := client.Fetch(context.Background(), webfetch.Request{URL: tc.url, Timeout: time.Second})
			projection := toolcatalogapp.BuildPublicToolResultProjectionV1("web_fetch", output, isError)
			if !isError || projection.Web == nil || projection.Web.Outcome != tc.outcome ||
				projection.Web.StatusCode != nil || projection.Web.ByteCount != nil || projection.Web.Truncated != nil {
				t.Fatalf("pre-response failure invented metadata: %#v", projection)
			}
			assertPublicWebItemV1(t, projection, isError)
		})
	}
}

func TestWebFetchMalformedAndLegacyResultsKeepSafeHostFallback(t *testing.T) {
	for name, mutation := range map[string]map[string]any{
		"missing legacy outcome":   {"outcome": nil},
		"private outcome":          {"outcome": "PRIVATE_OUTCOME_SENTINEL"},
		"missing HTTP status":      {"statusCode": nil},
		"missing byte count":       {"byteCount": nil},
		"missing truncation":       {"truncated": nil},
		"HTTP status text":         {"statusCode": "200"},
		"HTTP status fraction":     {"statusCode": 200.5},
		"HTTP status out of range": {"statusCode": 600},
		"negative count":           {"byteCount": -1},
		"unsafe count":             {"byteCount": 9007199254740992.0},
		"fractional count":         {"byteCount": 0.5},
		"NaN count":                {"byteCount": math.NaN()},
		"infinite count":           {"byteCount": math.Inf(1)},
		"wrong truncation type":    {"truncated": "false"},
		"failure with 2xx status":  {"outcome": "http_error"},
	} {
		t.Run(name, func(t *testing.T) {
			output := map[string]any{
				"outcome": "success", "statusCode": 200.0, "byteCount": 0.0, "truncated": false,
				"text": "PRIVATE_BODY_SENTINEL", "title": "PRIVATE_TITLE_SENTINEL", "url": "PRIVATE_URL_SENTINEL",
			}
			for key, value := range mutation {
				if value == nil {
					delete(output, key)
				} else {
					output[key] = value
				}
			}
			for _, isError := range []bool{false, true} {
				projection := toolcatalogapp.BuildPublicToolResultProjectionV1("web_fetch", output, isError)
				wantStatus := "completed"
				if isError {
					wantStatus = "failed"
				}
				if projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Web != nil || projection.Status != wantStatus {
					t.Fatalf("malformed result changed host isError lifecycle: %#v", projection)
				}
				assertPublicWebItemV1(t, projection, isError)
			}
		})
	}
	output := map[string]any{"outcome": "success", "statusCode": 206.0, "byteCount": 7.0, "truncated": false}
	if projection := toolcatalogapp.BuildPublicToolResultProjectionV1("web_fetch", output, true); projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Status != "failed" {
		t.Fatalf("successful Web metadata changed producer error truth: %#v", projection)
	}
	for _, toolName := range []string{"web_search", "fetch_url", "browse", " web_fetch", "web_fetch "} {
		projection := toolcatalogapp.BuildPublicToolResultProjectionV1(toolName, output, false)
		if projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Web != nil {
			t.Fatalf("unowned tool %q gained the Web fetch projection", toolName)
		}
	}
}

func TestWebFetchMalformedFallbackIgnoresConflictingCode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    string
		isError bool
	}{
		{name: "timeout code cannot turn success into cancellation", code: "tool_timeout"},
		{name: "completed code cannot turn error into success", code: "tool_completed", isError: true},
		{name: "cancelled code cannot turn success into cancellation", code: "tool_cancelled"},
		{name: "blocked code cannot turn success into blockage", code: "approval_policy_blocked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, legacy := range []bool{false, true} {
				output := map[string]any{
					"outcome": "success", "statusCode": 200.0, "byteCount": 0.0,
					"truncated": "false", "code": tc.code, "text": "PRIVATE_BODY_SENTINEL",
				}
				if legacy {
					delete(output, "outcome")
					output["truncated"] = false
				}
				projection := toolcatalogapp.BuildPublicToolResultProjectionV1("web_fetch", output, tc.isError)
				wantStatus, wantCode := "completed", "tool_completed"
				if tc.isError {
					wantStatus, wantCode = "failed", "tool_failed"
				}
				if projection.ProjectionKind != domaintoolresult.ProjectionHostStatus || projection.Web != nil ||
					projection.Status != wantStatus || projection.Code != wantCode || projection.MessageKey != wantCode {
					t.Fatalf("conflicting code changed producer isError: legacy=%t projection=%#v", legacy, projection)
				}
				assertPublicWebItemV1(t, projection, tc.isError)
			}
		})
	}
}

func publicWebFetchTestClientV1() webfetch.Client {
	return webfetch.Client{ProxySpec: netclient.ProxySpec{Mode: netclient.ModeOff}, DefaultMaxBytes: 1024, DefaultTimeout: time.Second}
}

func assertPublicWebItemV1(t *testing.T, projection domaintoolresult.PublicToolResultProjectionV1, isError bool) {
	t.Helper()
	entropy := sha256.Sum256([]byte("analytix.web-fetch-public-producer-test/v1"))
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID: "thread-web", TurnID: "turn-web", CreatedAt: "2026-10-08T00:00:00Z", FinishedAt: "2026-10-08T00:00:01Z",
		Call: domainmodel.ToolCall{ID: callID, Name: "web_fetch"}, Projection: projection, IsError: isError,
	})
	if err != nil {
		t.Fatalf("Web projection did not settle: %v", err)
	}
	projected := domaintoolresult.PublicToolResultItemRecordV1(settlement.ResultItem)
	if projected["isError"] != isError || projected["status"] != settlement.Status {
		t.Fatalf("public item changed producer lifecycle: %#v", projected)
	}
	parsed, err := domaintoolresult.ParsePublicToolResultProjectionV1(projected["output"])
	if err != nil || parsed.ProjectionKind != projection.ProjectionKind {
		t.Fatalf("public item lost the closed projection: %#v err=%v", parsed, err)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PRIVATE_", `"url":`, `"finalUrl":`, `"title":`, `"text":`, `"snippet":`, `"sources":`, `"citations":`, `"telemetry":`, `"error":`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private Web payload reached the public item via %q: %s", forbidden, encoded)
		}
	}
	if !parsed.PrivatePayloadWithheld || parsed.FactAnswerAllowed || parsed.EvidenceAuthority {
		t.Fatalf("Web projection acquired content or evidence authority: %#v", parsed)
	}
}
