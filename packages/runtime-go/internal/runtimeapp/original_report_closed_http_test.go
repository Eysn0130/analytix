//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
)

func TestRuntimeClosedReportAllowsSameThreadOrdinaryHTTP(t *testing.T) {
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, false, "")
}

func TestRuntimeClosedReportAllowsSameThreadOrdinaryHTTPWitnessOutage(t *testing.T) {
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, true, "")
}

func TestRuntimeClosedCompletedReportAllowsSameThreadOrdinaryHTTPWitnessOutage(t *testing.T) {
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, true, publicationapp.RestartAttemptStageDispositionV1)
}

func TestRuntimeMixedClosedReportAllowsSameThreadOrdinaryHTTPWitnessOutage(t *testing.T) {
	closed, _ := newRuntimeOriginalMixedClosedOpenFixtureV1(t)
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, true, publicationapp.RestartAttemptStageDispositionV1, closed)
}

func testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t *testing.T, offline bool, cut publicationapp.RestartAttemptStateV1, supplied ...*runtimeOriginalReservedReportHistoryFixtureV1) {
	const marker = "R129_CLOSED_REPORT_ORDINARY_READ_OK"
	var calls atomic.Int64
	var observed atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if bytes.Contains(body, []byte(runtimeOptionalDomainPrivateCanary)) {
			t.Error("private report canary reached provider")
			return
		}
		var delta map[string]any
		finish := "stop"
		switch calls.Add(1) {
		case 1:
			finish = "tool_calls"
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "closed_report_ordinary_read", "type": "function", "function": map[string]any{"name": "read", "arguments": `{"path":"closed-report-ordinary.txt","limit":1}`}}}}
		case 2:
			var request struct {
				Messages []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Error(err)
				return
			}
			for _, message := range request.Messages {
				if message.Role == "tool" && bytes.Contains(message.Content, []byte(marker)) {
					observed.Store(true)
				}
			}
			delta = map[string]any{"content": marker}
		default:
			t.Error("closed report ordinary continuation made an unexpected provider call")
			delta = map[string]any{"content": "unexpected call"}
		}
		chunk, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
	}))
	defer provider.Close()
	options := runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, preWitnessDisposition: domainpending.StatusFailed, omitFailedResult: true}
	if cut != "" {
		options = runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: cut}
	}
	var fixture *runtimeOriginalReservedReportHistoryFixtureV1
	if len(supplied) > 1 {
		t.Fatal("ambiguous supplied report history")
	}
	if len(supplied) == 1 {
		fixture = supplied[0]
	} else {
		fixture = newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, options)
	}
	if offline && !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("fixture witness was not available to disable")
	}
	config := fixture.config
	config.BaseURL, config.ProviderID, config.Model = provider.URL+"/v1", "closed-report-ordinary", "closed-report-ordinary-model"
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
	body, err := os.ReadFile(fixture.primary)
	if err != nil {
		t.Fatal(err)
	}
	var primary map[string]any
	if err := json.Unmarshal(body, &primary); err != nil {
		t.Fatal(err)
	}
	frozen := primary["securityState"].(map[string]any)
	workspace := frozen["workspaceRealPath"].(string)
	if err := os.WriteFile(filepath.Join(workspace, "closed-report-ordinary.txt"), []byte(marker+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	primary["workspace"], primary["providerId"], primary["model"] = workspace, config.ProviderID, config.Model
	body, err = json.Marshal(primary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.primary, body, 0600); err != nil {
		t.Fatal(err)
	}
	fixture.refresh(t)
	history, err := prepareRuntimeOriginalReportHistoryV1(context.Background(), fixture.core, fixture.publication, fixture.advance, fixture.scope)
	if err != nil {
		t.Fatal(err)
	}
	workID := fixture.attempt.ReportStageWorkID
	pendingRoot := filepath.Join(config.DataDir, "private", "pending-work")
	protected := []string{filepath.Join(pendingRoot, "receipts", workID[:2], workID+".json"), filepath.Join(pendingRoot, "dispositions", workID[:2], workID+".json")}
	for _, owner := range runtimePublicationOwnersV1 {
		protected = append(protected, filepath.Join(config.DataDir, "private", owner))
	}
	if history.mixedClosed != nil && history.deferred != nil {
		for _, entry := range history.deferred.plan.Attempts {
			primary, err := fixture.core.primaries.ReadPrimaryThreadSnapshotV1(context.Background(), entry.Stage.Context.ThreadID)
			if err != nil || primary.ThreadFileSHA256 == "" {
				t.Fatal("mixed held primary unavailable", err)
			}
			path := fixture.core.primaries.entries[entry.Stage.Context.ThreadID].primary.Path
			if !strings.HasPrefix(path, "durable/") {
				t.Fatal("mixed held primary is outside durable roots")
			}
			protected = append(protected, filepath.Join(fixture.core.roots.DurableDir, filepath.FromSlash(strings.TrimPrefix(path, "durable/"))))
			id := entry.Stage.WorkID
			protected = append(protected, filepath.Join(pendingRoot, "receipts", id[:2], id+".json"))
			if entry.Disposition != nil {
				protected = append(protected, filepath.Join(pendingRoot, "dispositions", id[:2], id+".json"))
			}
		}
	}
	before := startupWholeTreeRecordMapForTest(t, protected...)
	sharedBefore, _ := fixture.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
	checkHistory := func() {
		if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, protected...)) {
			t.Fatal("ordinary same-thread work changed report or pending history")
		}
		body, err := os.ReadFile(fixture.primary)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := finalauthority.ParsePrimaryThreadSnapshotV1(context.Background(), fixture.attempt.ThreadID, body)
		if err != nil {
			t.Fatal(err)
		}
		if cut == "" && history.mixedClosed == nil {
			if err := history.validateClosedResultsV1(context.Background(), runtimeReportSemanticPrimariesV1{fixture.attempt.ThreadID: snapshot}); err != nil {
				t.Fatal(err)
			}
		} else {
			if history.semantic == nil || fixture.scope.OwnsThread(fixture.attempt.ThreadID) {
				t.Fatal("completed prefix must have historical protection without thread hold")
			}
			if err := history.semantic.withSemanticCandidateV1(context.Background(), nil, nil, "", nil); err != nil {
				t.Fatal(err)
			}
		}
		shared, _ := fixture.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
		if shared.AdvanceCalls != sharedBefore.AdvanceCalls || shared.ResolveCalls != sharedBefore.ResolveCalls || !reflect.DeepEqual(shared.Checkpoint, sharedBefore.Checkpoint) {
			t.Fatal("ordinary continuation advanced or resolved report authority")
		}
	}
	var firstTurn map[string]any
	turnID := ""
	afterFirstAttempts := 0
	for restart := 0; restart < 2; restart++ {
		handler, err := NewRuntimeServerHandlerE(config)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		func() {
			defer func() { server.Close(); shutdownOwnedRuntimeHandler(t, handler) }()
			client := &http.Client{Timeout: 30 * time.Second, Transport: runtimeOptionalPrivacyTransport{t: t}}
			checkHistory()
			if history.mixedClosed != nil && history.deferred != nil {
				for _, held := range fixture.scope.ThreadIDs() {
					status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodPost, "/v1/threads/"+held+"/turns", map[string]any{"prompt": "Read the local ordinary file.", "mode": "agent"})
					if status < 400 || status == http.StatusNotFound || contracts.StringField(result, "code") == "" {
						t.Fatalf("held execution was not explicitly refused: status=%d", status)
					}
					for _, path := range []string{"/v1/threads/" + held, "/v1/usage?thread_id=" + held} {
						status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodGet, path, nil)
						if status < 400 || status == http.StatusNotFound || contracts.StringField(result, "code") == "" {
							t.Fatalf("held observation returned empty success: path=%s status=%d", path, status)
						}
					}
				}
				for _, path := range []string{"/v1/threads"} {
					status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodGet, path, nil)
					found := false
					threads, _ := result["threads"].([]any)
					for _, raw := range threads {
						thread, _ := raw.(map[string]any)
						found = found || contracts.StringField(thread, "id") == fixture.attempt.ThreadID
						if contracts.StringField(thread, "id") == fixture.attempt.ThreadID && (contracts.StringField(thread, "workspace") != "" || contracts.StringField(thread, "historyAuthority") != threadapp.CaseBoundaryOnlyHistoryAuthority) {
							t.Fatal("closed case list lost its private-workspace boundary")
						}
					}
					if status != http.StatusOK || !found {
						t.Fatalf("independent thread disappeared from public list: path=%s status=%d", path, status)
					}
				}
				// Case public summaries intentionally omit private workspace.
				// A caller cannot rediscover membership through its private path.
				projectID := threadapp.CaseProjectIDForRoot(workspace)
				status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodGet, "/v1/case-projects/"+projectID+"/threads", nil)
				threads, _ := result["threads"].([]any)
				if status != http.StatusOK || len(threads) != 0 {
					t.Fatal("case project exposed private workspace membership")
				}
			}
			if restart == 0 {
				status, response := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodPost, "/v1/threads/"+fixture.attempt.ThreadID+"/turns", map[string]any{
					"prompt": "Read the local ordinary file closed-report-ordinary.txt with limit 1, then return its marker. Do not query, publish, or retry the previous report.",
					"mode":   "agent", "async": true, "approvalPolicy": "never", "sandboxMode": "read-only", "providerId": config.ProviderID, "model": config.Model,
				})
				turnID = contracts.StringField(response, "turnId")
				if status != http.StatusAccepted || turnID == "" {
					t.Fatalf("same-thread ordinary turn refused: status=%d code=%s", status, contracts.StringField(response, "code"))
				}
				firstTurn = packagedPlanThenProtectedWaitTurnV1(t, client, server.URL, fixture.attempt.ThreadID, turnID)
				packagedPlanThenProtectedValidateOrdinaryReadV1(t, firstTurn, marker)
				if calls.Load() != 2 || !observed.Load() {
					t.Fatalf("ordinary read was not executed: calls=%d resultObserved=%t", calls.Load(), observed.Load())
				}
			} else {
				turn := packagedPlanThenProtectedWaitTurnV1(t, client, server.URL, fixture.attempt.ThreadID, turnID)
				if !reflect.DeepEqual(firstTurn, turn) || calls.Load() != 2 {
					t.Fatal("ordinary same-thread history changed or replayed provider work on restart")
				}
			}
			checkHistory()
		}()
		checkHistory()
		if cut != "" {
			t.Logf("completed-prefix ordinary lifecycle restart=%d witnessRequests=%d", restart, fixture.witnessAttempts())
		}
		if restart == 0 {
			afterFirstAttempts = fixture.witnessAttempts()
		} else if offline && fixture.witnessAttempts() != afterFirstAttempts {
			t.Fatal("offline ordinary restart repeated a witness attempt")
		}
	}
}
