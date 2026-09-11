package sse

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appthread "analytix.local/runtime-go/internal/app/thread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestSetupErrorEventUsesTerminalContract(t *testing.T) {
	event := SetupErrorEvent(SetupErrorInput{
		ThreadID:  "thread_1",
		Seq:       -4,
		Phase:     "replay",
		Message:   "load failed",
		Timestamp: "2026-07-02T00:00:00Z",
	})
	if event["kind"] != "error" || event["code"] != "sse_setup_error" || event["terminal"] != true {
		t.Fatalf("unexpected setup error event: %#v", event)
	}
	if event["seq"] != float64(0) {
		t.Fatalf("expected seq clamp to 0, got %#v", event["seq"])
	}
	details, _ := event["details"].(map[string]any)
	if details["phase"] != "replay" {
		t.Fatalf("phase not preserved: %#v", event)
	}
}

func TestSnapshotRequiredEventUsesCursorContract(t *testing.T) {
	event := SnapshotRequiredEvent(SnapshotRequiredInput{
		ThreadID:         "thread_1",
		Seq:              100,
		SinceSeq:         5,
		HighestSeq:       100,
		ReplayEventCount: 1200,
		Timestamp:        "2026-07-02T00:00:00Z",
	})
	if event["kind"] != "snapshot_required" || event["seq"] != float64(100) || event["threadId"] != "thread_1" {
		t.Fatalf("unexpected snapshot event: %#v", event)
	}
	if event["sinceSeq"] != float64(5) || event["highestSeq"] != float64(100) || event["replayEventCount"] != float64(1200) {
		t.Fatalf("snapshot cursor fields not preserved: %#v", event)
	}
	if event["reason"] != "live_replay_backlog_exceeded" {
		t.Fatalf("snapshot reason not preserved: %#v", event)
	}
}

func TestSSETraceNeverMutatesAtomicTerminalBatch(t *testing.T) {
	t.Setenv("ANALYTIX_THREAD_TRACE", "1")
	handler := ThreadEventsHandler{Now: func() time.Time {
		return time.Date(2026, 7, 20, 3, 0, 0, 0, time.UTC)
	}}
	for _, kind := range []string{
		domainevent.AcceptedFinalDeliveryBatchKind,
		domainevent.GeneralTerminalDeliveryBatchKind,
	} {
		batch := map[string]any{"kind": kind, "seq": float64(3)}
		projected := handler.withSSETrace(batch)
		if _, present := projected["trace"]; present || len(projected) != len(batch) {
			t.Fatalf("atomic terminal batch %q was mutated by SSE trace: %#v", kind, projected)
		}
	}
	ordinary := handler.withSSETrace(map[string]any{"kind": "heartbeat", "seq": float64(4)})
	if _, present := ordinary["trace"]; !present {
		t.Fatalf("ordinary SSE event lost enabled trace metadata: %#v", ordinary)
	}
}

func TestSinceSeqUsesQueryBeforeLastEventID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=3", nil)
	request.Header.Set("Last-Event-ID", "7")
	if seq := SinceSeq(request); seq != 3 {
		t.Fatalf("expected query seq, got %d", seq)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events", nil)
	request.Header.Set("Last-Event-ID", "7")
	if seq := SinceSeq(request); seq != 7 {
		t.Fatalf("expected header seq, got %d", seq)
	}
}

func TestThreadEventsHandlerReplaysDurableEvents(t *testing.T) {
	handler := ThreadEventsHandler{
		Store: identityThreadEventStore(ThreadEventStore{
			HighestSeq: func(string) (int, error) { return 2, nil },
			LoadEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				if threadID != "t1" || afterSeq != 0 {
					t.Fatalf("unexpected replay input: threadID=%q afterSeq=%d", threadID, afterSeq)
				}
				return []map[string]any{{"kind": "message", "seq": float64(2), "threadId": threadID}}, nil
			},
		}),
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=1", nil)
	handler.ServeHTTP(recorder, request, "t1")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"seq":2`) {
		t.Fatalf("unexpected replay response code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCaseCheckpointAuditReplaysAndStreamsMetadataOnly(t *testing.T) {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-checkpoint-sse", TurnID: "turn-case-checkpoint-sse", WorkspaceRealPath: "/cases/checkpoint-sse",
		CaseID: "case-checkpoint-sse", DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("checkpoint-sse"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("checkpoint-sse-manifest")), ContextEpoch: 4,
		IssuedAt: time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	securityBody, _ := json.Marshal(securityContext)
	securityRecord := map[string]any{}
	if err := json.Unmarshal(securityBody, &securityRecord); err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": securityRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "securityContext": securityRecord, "items": []any{},
		}},
	}
	checkpointID := domaincheckpointref.RuntimeID("workspace/checkpoint:sse")
	planID := domaincheckpointref.PlanID(checkpointID)
	rescueID := domaincheckpointref.RescueID(checkpointID)
	applyID := domaincheckpointref.ApplyID(checkpointID)
	createdAt := "2026-07-15T03:00:01Z"
	raw := []map[string]any{
		{
			"kind": "checkpoint_captured", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"checkpoint": map[string]any{
				"schemaVersion": float64(1), "checkpointId": checkpointID, "threadId": securityContext.ThreadID,
				"turnId": securityContext.TurnID, "createdAt": createdAt, "status": "captured",
				"changedFileCount": float64(1), "snapshotStorage": "runtime_private_cas",
			},
		},
		{
			"kind": "checkpoint_rewind_rescue_created", "threadId": securityContext.ThreadID,
			"rescue": map[string]any{
				"schemaVersion": float64(1), "rescueId": rescueID, "planId": planID, "checkpointId": checkpointID,
				"threadId": securityContext.ThreadID, "createdAt": createdAt, "fileCount": float64(1), "storage": "runtime_private_sidecar",
			},
		},
		{
			"kind": "checkpoint_rewind_applied", "threadId": securityContext.ThreadID,
			"apply": map[string]any{
				"schemaVersion": float64(1), "applyId": applyID, "planId": planID, "checkpointId": checkpointID,
				"threadId": securityContext.ThreadID, "createdAt": createdAt, "scope": "code", "status": "applied",
				"destructive": true, "fileCount": float64(1), "conversationStatus": "not_requested",
				"summary": map[string]any{
					"fileAppliedCount": float64(1), "fileNoopCount": float64(0), "fileManualReviewCount": float64(0),
					"fileBlockedCount": float64(0), "fileFailedCount": float64(0),
				},
			},
		},
	}
	durable := make([]map[string]any, 0, len(raw))
	for index, event := range raw {
		projected, err := appturn.SanitizeCaseEventPublication(thread, event)
		if err != nil {
			t.Fatalf("sanitize case checkpoint event: %v", err)
		}
		projected["seq"] = float64(index + 1)
		projected["timestamp"] = createdAt
		durable = append(durable, projected)
	}
	projectPublic := func(threadID string, event map[string]any) (map[string]any, bool, string) {
		projected, visible, err := appthread.ProjectPublicThreadEvent(threadID, thread, event)
		if err != nil {
			return nil, false, casePublicAuthorityRejectedCode
		}
		return projected, visible, ""
	}
	assertBody := func(t *testing.T, body string) {
		t.Helper()
		for _, expected := range []string{
			"event: checkpoint_captured", "event: checkpoint_rewind_rescue_created", "event: checkpoint_rewind_applied",
			`"projectionKind":"checkpoint_status"`, `"disclosure":"metadata_only"`, `"factAnswerAllowed":false`, `"evidenceAuthority":false`,
		} {
			if !strings.Contains(body, expected) {
				t.Fatalf("case checkpoint SSE missing %q:\n%s", expected, body)
			}
		}
		for _, forbidden := range []string{
			checkpointID, planID, rescueID, applyID, "checkpointId", "planId", "rescueId", "applyId",
			"BANK_CARD_6222020202020202020", "relativePath", "beforeHash", "content", "workspace",
			"executionGrant", "publicationReceipt", "publicationAuthority",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("case checkpoint SSE leaked %q:\n%s", forbidden, body)
			}
		}
	}

	t.Run("replay", func(t *testing.T) {
		handler := ThreadEventsHandler{Store: ThreadEventStore{
			HighestSeq: func(string) (int, error) { return len(durable), nil },
			LoadEventsSince: func(string, int) ([]map[string]any, error) {
				return durable, nil
			},
			PreflightPublic: func(string) string { return "" },
			ProjectPublic:   projectPublic,
		}}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0", nil), securityContext.ThreadID)
		assertBody(t, recorder.Body.String())
	})

	t.Run("live", func(t *testing.T) {
		live := make(chan map[string]any, len(durable))
		for _, event := range durable {
			live <- event
		}
		close(live)
		handler := ThreadEventsHandler{Store: ThreadEventStore{
			HighestSeq:      func(string) (int, error) { return 0, nil },
			LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, nil },
			SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
			PreflightPublic: func(string) string { return "" },
			ProjectPublic:   projectPublic,
		}}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0&live=1", nil), securityContext.ThreadID)
		assertBody(t, recorder.Body.String())
	})
}

func TestThreadEventsHandlerDropsReasoningFromReplay(t *testing.T) {
	handler := ThreadEventsHandler{
		Store: identityThreadEventStore(ThreadEventStore{
			HighestSeq: func(string) (int, error) { return 4, nil },
			LoadEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				return []map[string]any{
					{"kind": "assistant_reasoning_delta", "seq": float64(2), "threadId": threadID, "text": "private scratchpad"},
					{"kind": "item_completed", "seq": float64(3), "threadId": threadID, "item": map[string]any{"kind": "assistant_reasoning", "text": "archived reasoning"}},
					{"kind": "pipeline_stage", "seq": float64(4), "threadId": threadID, "stage": "response_received", "label": "Public progress"},
				}, nil
			},
		}),
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=1", nil)
	handler.ServeHTTP(recorder, request, "t1")
	body := recorder.Body.String()
	if !strings.Contains(body, "Public progress") {
		t.Fatalf("public event missing: %s", body)
	}
	cursorIndex := strings.LastIndex(body, "event: cursor_advanced")
	publicIndex := strings.Index(body, "event: pipeline_stage")
	if cursorIndex < 0 || publicIndex < 0 || cursorIndex >= publicIndex ||
		!strings.Contains(body[cursorIndex:publicIndex], `"seq":3`) {
		t.Fatalf("restricted sequence gap was not advanced before the next public event: %s", body)
	}
	for _, forbidden := range []string{"assistant_reasoning", "private scratchpad", "archived reasoning"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("reasoning leaked through SSE replay %q: %s", forbidden, body)
		}
	}
}

func TestThreadEventsHandlerLiveAdvancesRestrictedSequenceBeforePublicEvent(t *testing.T) {
	live := make(chan map[string]any, 2)
	live <- map[string]any{
		"kind": "assistant_reasoning_delta", "seq": float64(4), "threadId": "t1", "text": "PRIVATE_REASONING_SENTINEL",
	}
	live <- map[string]any{
		"kind": "pipeline_stage", "seq": float64(5), "threadId": "t1", "turnId": "turn-1",
		"stage": "response_received", "label": "Response Received",
	}
	close(live)
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 3, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, nil },
		SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=3&live=1", nil), "t1")
	body := recorder.Body.String()
	cursorIndex := strings.Index(body, "event: cursor_advanced")
	publicIndex := strings.Index(body, "event: pipeline_stage")
	if cursorIndex < 0 || publicIndex < 0 || cursorIndex >= publicIndex ||
		!strings.Contains(body[cursorIndex:publicIndex], `"seq":4`) {
		t.Fatalf("live restricted sequence gap was not advanced before the next public event: %s", body)
	}
	if strings.Contains(body, "PRIVATE_REASONING_SENTINEL") || strings.Contains(body, "assistant_reasoning") {
		t.Fatalf("live restricted event leaked private content: %s", body)
	}
}

func TestThreadEventsHandlerAdvancesCursorPastRestrictedTail(t *testing.T) {
	handler := ThreadEventsHandler{
		Now: func() time.Time { return time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC) },
		Store: identityThreadEventStore(ThreadEventStore{
			HighestSeq: func(string) (int, error) { return 10, nil },
			LoadEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				return []map[string]any{{
					"kind":     "assistant_reasoning_delta",
					"seq":      float64(10),
					"threadId": threadID,
					"text":     "PRIVATE_REASONING_SENTINEL",
				}}, nil
			},
		}),
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=9", nil)
	handler.ServeHTTP(recorder, request, "t1")
	body := recorder.Body.String()
	for _, expected := range []string{"event: cursor_advanced", `"seq":10`, `"reason":"restricted_content_removed"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("restricted tail did not advance cursor with %q: %s", expected, body)
		}
	}
	if strings.Contains(body, "PRIVATE_REASONING_SENTINEL") || strings.Contains(body, "assistant_reasoning") {
		t.Fatalf("restricted tail leaked private content: %s", body)
	}
}

func TestThreadEventsHandlerLiveReplayDoesNotTruncateBacklog(t *testing.T) {
	replayEvents := make([]map[string]any, 0, 300)
	for seq := 1; seq <= 300; seq++ {
		replayEvents = append(replayEvents, map[string]any{
			"kind":     "pipeline_stage",
			"seq":      float64(seq),
			"threadId": "t1",
			"stage":    "response_received",
			"label":    "Public progress",
		})
	}
	handler := ThreadEventsHandler{
		Store: identityThreadEventStore(ThreadEventStore{
			HighestSeq: func(string) (int, error) { return 300, nil },
			LoadEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				if threadID != "t1" || afterSeq != 0 {
					t.Fatalf("unexpected replay input: threadID=%q afterSeq=%d", threadID, afterSeq)
				}
				return replayEvents, nil
			},
			SubscribeEvents: func(string) (<-chan map[string]any, func()) {
				return make(chan map[string]any), func() {}
			},
		}),
	}
	recorder := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=0&live=1", nil).WithContext(ctx)
	handler.ServeHTTP(recorder, request, "t1")

	body := recorder.Body.String()
	for _, needle := range []string{`"seq":1`, `"seq":44`, `"seq":300`} {
		if !strings.Contains(body, needle) {
			t.Fatalf("live replay backlog missing %q:\n%s", needle, body)
		}
	}
}

func TestThreadEventsHandlerLiveReplayUsesSnapshotRequiredForHugeBacklog(t *testing.T) {
	replayEvents := make([]map[string]any, 0, maxLiveReplayEvents+10)
	for seq := 1; seq <= maxLiveReplayEvents+10; seq++ {
		replayEvents = append(replayEvents, map[string]any{
			"kind":     "pipeline_stage",
			"seq":      float64(seq),
			"threadId": "t1",
			"stage":    "response_received",
			"label":    "Public progress",
		})
	}
	handler := ThreadEventsHandler{
		Now: func() time.Time { return time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC) },
		Store: identityThreadEventStore(ThreadEventStore{
			HighestSeq: func(string) (int, error) { return maxLiveReplayEvents + 10, nil },
			LoadEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				if threadID != "t1" || afterSeq != 0 {
					t.Fatalf("unexpected replay input: threadID=%q afterSeq=%d", threadID, afterSeq)
				}
				return replayEvents, nil
			},
			SubscribeEvents: func(string) (<-chan map[string]any, func()) {
				return make(chan map[string]any), func() {}
			},
		}),
	}
	recorder := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=0&live=1", nil).WithContext(ctx)
	handler.ServeHTTP(recorder, request, "t1")

	body := recorder.Body.String()
	for _, needle := range []string{
		"event: snapshot_required",
		`"kind":"snapshot_required"`,
		`"seq":1034`,
		`"highestSeq":1034`,
		`"replayEventCount":1034`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("snapshot replay missing %q:\n%s", needle, body)
		}
	}
	if strings.Contains(body, `"kind":"pipeline_stage"`) {
		t.Fatalf("huge live replay should use snapshot_required instead of partial deltas:\n%s", body)
	}
}

func TestThreadEventsHandlerWritesSetupErrorOnReplayFailure(t *testing.T) {
	handler := ThreadEventsHandler{
		Now: func() time.Time { return time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC) },
		Store: identityThreadEventStore(ThreadEventStore{
			HighestSeq:      func(string) (int, error) { return 2, nil },
			LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, errors.New("disk boom") },
		}),
		SanitizeError: func(error) string { return "redacted boom" },
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=1", nil)
	handler.ServeHTTP(recorder, request, "t1")
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"code":"sse_setup_error"`) || !strings.Contains(body, "redacted boom") {
		t.Fatalf("unexpected setup error response code=%d body=%s", recorder.Code, body)
	}
}

func TestThreadEventsHandlerSetupErrorFailsClosedWithoutSanitizer(t *testing.T) {
	const sentinel = "PRIVATE_REASONING_ACCOUNT_6222020202020202020"
	handler := ThreadEventsHandler{
		Now: func() time.Time { return time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC) },
		Store: identityThreadEventStore(ThreadEventStore{
			HighestSeq:      func(string) (int, error) { return 2, nil },
			LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, errors.New(sentinel) },
		}),
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events?since_seq=1", nil)
	handler.ServeHTTP(recorder, request, "t1")
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"code":"sse_setup_error"`) ||
		!strings.Contains(body, "SSE setup failed") || strings.Contains(body, sentinel) {
		t.Fatalf("nil sanitizer exposed setup failure: code=%d body=%s", recorder.Code, body)
	}
}

func TestThreadEventsHandlerFailsClosedWithoutPublicProjector(t *testing.T) {
	handler := ThreadEventsHandler{Store: ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 1, nil },
		LoadEventsSince: func(threadID string, _ int) ([]map[string]any, error) {
			return []map[string]any{{"kind": "message", "threadId": threadID, "seq": float64(1), "message": "CASE_FACT_SENTINEL"}}, nil
		},
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/t1/events", nil)
	handler.ServeHTTP(recorder, request, "t1")
	body := recorder.Body.String()
	if !strings.Contains(body, "public_projection_revoked") || !strings.Contains(body, casePublicAuthorityUnavailableCode) || strings.Contains(body, "CASE_FACT_SENTINEL") {
		t.Fatalf("missing public projector did not fail closed: %s", body)
	}
}

func TestCaseSSEReconnectAtHighestSeqStillPreflightsPrimaryCAS(t *testing.T) {
	preflightCalls := 0
	handler := ThreadEventsHandler{Store: ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 7, nil },
		PreflightPublic: func(string) string {
			preflightCalls++
			return casePublicAuthorityRejectedCode
		},
		ProjectPublic: func(string, map[string]any) (map[string]any, bool, string) {
			t.Fatal("event projection must not run after failed preflight")
			return nil, false, ""
		},
	}}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=7", nil), "thread-case")
	body := recorder.Body.String()
	if preflightCalls != 1 || !strings.Contains(body, `"kind":"public_projection_revoked"`) ||
		!strings.Contains(body, `"code":"case_public_authority_rejected"`) || strings.Contains(body, "id:") {
		t.Fatalf("highest-seq reconnect bypassed primary authority: calls=%d body=%s", preflightCalls, body)
	}
}

func TestCaseSSEProjectionFailureIsNotCursorAdvanced(t *testing.T) {
	handler := ThreadEventsHandler{Store: ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 1, nil },
		LoadEventsSince: func(threadID string, _ int) ([]map[string]any, error) {
			return []map[string]any{{"kind": "message", "threadId": threadID, "seq": float64(1)}}, nil
		},
		PreflightPublic: func(string) string { return "" },
		ProjectPublic: func(string, map[string]any) (map[string]any, bool, string) {
			return nil, false, casePublicAuthorityRejectedCode
		},
	}}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events", nil), "thread-case")
	body := recorder.Body.String()
	if !strings.Contains(body, "public_projection_revoked") || strings.Contains(body, "cursor_advanced") ||
		strings.Contains(body, `"seq"`) || strings.Contains(body, "id:") {
		t.Fatalf("authority failure advanced or acknowledged the cursor: %s", body)
	}
}

func TestCaseSSEAuthorityRevokedWithoutNewDurableEvent(t *testing.T) {
	preflightCalls := 0
	live := make(chan map[string]any)
	handler := ThreadEventsHandler{
		HeartbeatInterval: time.Millisecond,
		Store: ThreadEventStore{
			HighestSeq:      func(string) (int, error) { return 0, nil },
			SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
			PreflightPublic: func(string) string {
				preflightCalls++
				if preflightCalls == 1 {
					return ""
				}
				return casePublicAuthorityRejectedCode
			},
			ProjectPublic: func(_ string, event map[string]any) (map[string]any, bool, string) {
				return event, true, ""
			},
		},
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?live=1", nil), "thread-case")
	body := recorder.Body.String()
	if preflightCalls < 2 || !strings.Contains(body, "public_projection_revoked") || strings.Contains(body, "event: heartbeat") {
		t.Fatalf("quiet live stream retained revoked authority: calls=%d body=%s", preflightCalls, body)
	}
}

func TestPublicProjectionRevokedControlFrameHasClosedNonDurableShape(t *testing.T) {
	event := PublicProjectionRevokedEvent(" thread-case ", "UNTRUSTED_ERROR_WITH_PATH_/tmp/case")
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"seq", "timestamp", "/tmp/case", "title", "reasoning", "workspace"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("revocation control frame leaked %q: %s", forbidden, body)
		}
	}
	if event["code"] != casePublicAuthorityRejectedCode || event["action"] != "purge_case_projection" || event["terminal"] != true {
		t.Fatalf("unexpected revocation control frame: %#v", event)
	}
}

func identityThreadEventStore(store ThreadEventStore) ThreadEventStore {
	store.PreflightPublic = func(string) string { return "" }
	store.ProjectPublic = func(threadID string, event map[string]any) (map[string]any, bool, string) {
		value, _ := event["threadId"].(string)
		return event, strings.TrimSpace(value) == strings.TrimSpace(threadID), ""
	}
	if store.ValidateAcceptedFinalDelivery == nil {
		store.ValidateAcceptedFinalDelivery = func(domainevent.AcceptedFinalDeliveryBatchV2) error { return nil }
	}
	return store
}

func TestLegacyAcceptedFinalAssistantEventIsDisplayOnlyNotSSE(t *testing.T) {
	for _, value := range []any{
		map[string]any{"schemaVersion": float64(1)},
		map[string]any{"schemaVersion": float64(3)},
		map[string]any{"schemaVersion": float64(2.5)},
		"not-an-object",
	} {
		event := map[string]any{
			"kind": "item_completed", "threadId": "thread-a", "turnId": "turn-a", "seq": float64(1),
			"item": map[string]any{"kind": "assistant_text", "text": "legacy unverified case fact", "acceptedFinal": value},
		}
		if projected, visible := publicEvent(event); visible || projected != nil {
			t.Fatalf("invalid/legacy accepted final entered SSE: value=%#v projected=%#v", value, projected)
		}
	}
}

func TestCurrentAcceptedFinalAssistantEventRequiresClosedV3Identity(t *testing.T) {
	event := acceptedFinalSSEDeliveryBatchFixture(t).Events[0]
	projected, visible := publicEvent(event)
	if !visible || projected == nil {
		t.Fatalf("current closed V3 accepted final was removed from SSE: %#v", projected)
	}
	fullRecord := contracts.CloneMap(event)
	fullItem := fullRecord["item"].(map[string]any)
	delete(fullItem, "acceptedFinalView")
	privateRecord := acceptedFinalSSEFactV5V2Fixture(t)
	fullItem["acceptedFinal"] = privateRecord
	if leaked, allowed := publicEvent(fullRecord); allowed || leaked != nil {
		t.Fatalf("private full accepted final entered SSE through the low-level filter: %#v", leaked)
	}
	for _, key := range []string{"factFinalWitnessAdmission", "publicationSnapshotProofDigest"} {
		fragment := contracts.CloneMap(event)
		fragmentItem := fragment["item"].(map[string]any)
		fragmentItem[key] = contracts.CloneValue(privateRecord[key])
		if leaked, allowed := publicEvent(fragment); allowed || leaked != nil {
			t.Fatalf("detached private accepted-final field %q entered SSE: %#v", key, leaked)
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"view digest": func(candidate map[string]any) {
			candidate["item"].(map[string]any)["acceptedFinalView"].(map[string]any)["acceptedFinalDigest"] = acceptedFinalSSEHash("foreign")
		},
		"item thread": func(candidate map[string]any) {
			candidate["item"].(map[string]any)["threadId"] = "thread-foreign"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := contracts.CloneMap(event)
			mutate(candidate)
			if leaked, allowed := publicEvent(candidate); allowed || leaked != nil {
				t.Fatalf("identity-detached V3 entered SSE through the low-level filter: %#v", leaked)
			}
		})
	}
}

func TestStandaloneAcceptedFinalV3LiveEventCannotBypassSealedBatch(t *testing.T) {
	batch := acceptedFinalSSEDeliveryBatchFixture(t)
	live := make(chan map[string]any, 1)
	live <- contracts.CloneMap(batch.Events[0])
	close(live)
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 0, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, nil },
		SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0&live=1", nil), batch.ThreadID)
	body := recorder.Body.String()
	if !strings.Contains(body, `"kind":"public_projection_revoked"`) ||
		!strings.Contains(body, `"code":"case_public_authority_rejected"`) {
		t.Fatalf("standalone V3 live event did not revoke the generic stream: %s", body)
	}
	for _, forbidden := range []string{
		`event: item_completed`, `event: accepted_final_batch`, `"acceptedFinalView":`,
		domainevidence.CaseSourceUnavailableText,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("standalone V3 live event bypass leaked %q: %s", forbidden, body)
		}
	}
}

func TestPrivateFactFinalV5V2GenericReplayRevokesWithoutPublishing(t *testing.T) {
	batch := acceptedFinalSSEDeliveryBatchFixture(t)
	events := make([]map[string]any, 0, len(batch.Events))
	for _, event := range batch.Events {
		events = append(events, contracts.CloneMap(event))
	}
	assistantItem, _ := events[0]["item"].(map[string]any)
	delete(assistantItem, "acceptedFinalView")
	assistantItem["acceptedFinal"] = acceptedFinalSSEFactV5V2Fixture(t)

	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return batch.LastSeq, nil },
		LoadPublicEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
			if threadID != batch.ThreadID || afterSeq != 0 {
				t.Fatalf("unexpected private replay input: threadID=%q afterSeq=%d", threadID, afterSeq)
			}
			return events, nil
		},
		SealAcceptedFinalDelivery: func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			t.Fatal("private full record reached the generic delivery sealer")
			return domainevent.AcceptedFinalDeliverySealV1{}, nil
		},
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0", nil), batch.ThreadID)
	body := recorder.Body.String()
	if !strings.Contains(body, `"kind":"public_projection_revoked"`) ||
		!strings.Contains(body, `"code":"case_public_authority_rejected"`) {
		t.Fatalf("private V5/V2 replay did not revoke the generic stream: %s", body)
	}
	for _, forbidden := range []string{
		`"acceptedFinal":`, `"factFinalWitnessAdmission":`, `"publicationSnapshotProofDigest":`,
		`"acceptedFinalView":`, `"event: accepted_final_batch"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("private V5/V2 replay leaked %q into generic SSE: %s", forbidden, body)
		}
	}
}

func acceptedFinalSSEFactV5V2Fixture(t *testing.T) map[string]any {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "domain", "evidence", "testdata",
		"accepted-final-v5-fact-witness-admission-v2.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	parsed, err := domainevidence.ParseAcceptedFinalRecord(record)
	if err != nil || parsed.SchemaVersion != domainevidence.AcceptedFinalRecordVersion ||
		parsed.FactFinalWitnessAdmission == nil ||
		parsed.FactFinalWitnessAdmission.SchemaVersion != domainevidence.FactFinalWitnessAdmissionSchemaVersionV2 ||
		parsed.FactFinalWitnessAdmission.Purpose != domainevidence.FactFinalWitnessAdmissionPurposeV2 ||
		strings.TrimSpace(parsed.PublicationSnapshotProofDigest) == "" {
		t.Fatalf("fact-bearing V5/V2 replay canary is invalid: record=%#v err=%v", parsed, err)
	}
	return record
}

func TestAcceptedFinalReplayWritesOneClosedSSEBatch(t *testing.T) {
	batch := acceptedFinalSSEDeliveryBatchFixture(t)
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return batch.LastSeq, nil },
		LoadPublicEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
			if threadID != batch.ThreadID || afterSeq != 0 {
				t.Fatalf("unexpected replay input: threadID=%q afterSeq=%d", threadID, afterSeq)
			}
			return batch.Events, nil
		},
		SealAcceptedFinalDelivery: func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			return batch.PublicationAuthority, nil
		},
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0", nil), batch.ThreadID)
	body := recorder.Body.String()
	if strings.Count(body, "event: accepted_final_batch") != 1 ||
		!strings.Contains(body, fmt.Sprintf("id: %d", batch.LastSeq)) {
		t.Fatalf("accepted final was not emitted as one last-sequence batch:\n%s", body)
	}
	for _, forbidden := range []string{"event: item_completed", "event: usage", "event: turn_completed"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("accepted-final nested event escaped as a standalone SSE frame %q:\n%s", forbidden, body)
		}
	}
}

func TestAcceptedFinalReplayWritesEveryFinalAsItsOwnClosedSSEBatch(t *testing.T) {
	first := acceptedFinalSSEDeliveryBatchFixture(t)
	second := remapAcceptedFinalSSEDeliveryBatchFixture(t, first, "turn-sse-second", 4, 41)
	events := append(append([]map[string]any{}, first.Events...), second.Events...)
	sealCalls := 0
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return second.LastSeq, nil },
		LoadPublicEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
			if threadID != first.ThreadID || afterSeq != 0 {
				t.Fatalf("unexpected multi-final replay input: threadID=%q afterSeq=%d", threadID, afterSeq)
			}
			return events, nil
		},
		SealAcceptedFinalDelivery: func(group []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			sealCalls++
			turnID := contracts.StringField(group[0], "turnId")
			switch turnID {
			case first.TurnID:
				return first.PublicationAuthority, nil
			case second.TurnID:
				return second.PublicationAuthority, nil
			default:
				return domainevent.AcceptedFinalDeliverySealV1{}, fmt.Errorf("unexpected accepted-final turn %q", turnID)
			}
		},
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0", nil), first.ThreadID)
	body := recorder.Body.String()
	if sealCalls != 2 || strings.Count(body, "event: accepted_final_batch") != 2 ||
		!strings.Contains(body, fmt.Sprintf("id: %d", first.LastSeq)) ||
		!strings.Contains(body, fmt.Sprintf("id: %d", second.LastSeq)) ||
		strings.Index(body, fmt.Sprintf("id: %d", first.LastSeq)) >= strings.Index(body, fmt.Sprintf("id: %d", second.LastSeq)) {
		t.Fatalf("multi-final replay did not retain one ordered seal per final: calls=%d\n%s", sealCalls, body)
	}
	for _, forbidden := range []string{
		"event: item_completed", "event: usage", "event: turn_completed",
		`"acceptedFinal":`, `"factFinalWitnessAdmission":`, `"publicationSnapshotProof":`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("multi-final SSE leaked an unsealed/private payload %q:\n%s", forbidden, body)
		}
	}
}

func TestAcceptedFinalDeliveryUnitsCapsEveryGroupBeforeAnySeal(t *testing.T) {
	base := acceptedFinalSSEDeliveryBatchFixture(t)
	groupCount := domainevent.AcceptedFinalDeliveryGroupLimitV1 + 1
	events := make([]map[string]any, 0, groupCount*len(base.Events))
	seals := make(map[string]domainevent.AcceptedFinalDeliverySealV1, groupCount)
	for index := 0; index < groupCount; index++ {
		batch := base
		if index != 0 {
			batch = remapAcceptedFinalSSEDeliveryBatchFixture(
				t, base, fmt.Sprintf("turn-sse-cap-%d", index+1), len(events)+1, byte(index),
			)
		}
		seals[batch.TurnID] = batch.PublicationAuthority
		events = append(events, batch.Events...)
	}
	sealFor := func(calls *int) func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
		return func(group []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			(*calls)++
			seal, ok := seals[contracts.StringField(group[0], "turnId")]
			if !ok {
				return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("unexpected accepted-final group")
			}
			return seal, nil
		}
	}

	boundedEventCount := domainevent.AcceptedFinalDeliveryGroupLimitV1 * len(base.Events)
	sealCalls := 0
	units, err := acceptedFinalDeliveryUnits(base.ThreadID, events[:boundedEventCount], sealFor(&sealCalls))
	if err != nil || len(units) != domainevent.AcceptedFinalDeliveryGroupLimitV1 ||
		sealCalls != domainevent.AcceptedFinalDeliveryGroupLimitV1 {
		t.Fatalf("256 accepted-final SSE groups did not pass exactly: units=%d calls=%d err=%v", len(units), sealCalls, err)
	}

	sealCalls = 0
	units, err = acceptedFinalDeliveryUnits(base.ThreadID, events, sealFor(&sealCalls))
	if err == nil || units != nil || sealCalls != 0 {
		t.Fatalf("257 accepted-final SSE groups did not fail before sealing: units=%#v calls=%d err=%v", units, sealCalls, err)
	}

	sealCalls = 0
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return len(events), nil },
		LoadPublicEventsSince: func(string, int) ([]map[string]any, error) {
			return events, nil
		},
		SealAcceptedFinalDelivery: sealFor(&sealCalls),
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0", nil), base.ThreadID,
	)
	body := recorder.Body.String()
	if sealCalls != 0 || !strings.Contains(body, `"kind":"public_projection_revoked"`) ||
		!strings.Contains(body, `"code":"case_public_authority_rejected"`) {
		t.Fatalf("257 accepted-final SSE groups did not revoke before sealing: calls=%d body=%s", sealCalls, body)
	}
	for _, forbidden := range []string{"event: accepted_final_batch", "turn-sse-cap-257"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("257 accepted-final SSE groups leaked a public prefix %q: %s", forbidden, body)
		}
	}
}

func TestAcceptedFinalReplayRejectsWrongThreadLoaderBeforeAnySeal(t *testing.T) {
	batch := acceptedFinalSSEDeliveryBatchFixture(t)
	const requestedThreadID = "route-requested"
	sealCalls := 0
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return batch.LastSeq, nil },
		LoadPublicEventsSince: func(threadID string, _ int) ([]map[string]any, error) {
			if threadID != requestedThreadID {
				t.Fatalf("loader received thread %q, want %q", threadID, requestedThreadID)
			}
			return batch.Events, nil
		},
		SealAcceptedFinalDelivery: func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			sealCalls++
			return batch.PublicationAuthority, nil
		},
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0", nil), requestedThreadID,
	)
	body := recorder.Body.String()
	if sealCalls != 0 || !strings.Contains(body, `"kind":"public_projection_revoked"`) ||
		!strings.Contains(body, `"code":"case_public_authority_rejected"`) {
		t.Fatalf("wrong-thread replay did not revoke before sealing: calls=%d body=%s", sealCalls, body)
	}
	for _, forbidden := range []string{
		"event: accepted_final_batch", batch.ThreadID, batch.TurnID,
		domainevidence.CaseSourceUnavailableText,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("wrong-thread replay reflected hostile bytes %q: %s", forbidden, body)
		}
	}
}

func TestAcceptedFinalDeliveryUnitsRejectNonContiguousDuplicateCommit(t *testing.T) {
	first := acceptedFinalSSEDeliveryBatchFixture(t)
	second := remapAcceptedFinalSSEDeliveryBatchFixture(t, first, "turn-sse-second", 4, 41)
	events := make([]map[string]any, 0, len(first.Events)*2+len(second.Events))
	events = append(events, first.Events...)
	events = append(events, second.Events...)
	for _, event := range first.Events {
		cloned := contracts.CloneMap(event)
		seq, _ := contracts.NumericSeq(cloned["seq"])
		cloned["seq"] = float64(seq + second.LastSeq)
		events = append(events, cloned)
	}
	sealCalls := 0
	units, err := acceptedFinalDeliveryUnits(first.ThreadID, events, func(group []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
		sealCalls++
		switch contracts.StringField(group[0], "turnId") {
		case first.TurnID:
			return first.PublicationAuthority, nil
		case second.TurnID:
			return second.PublicationAuthority, nil
		default:
			return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("unexpected accepted-final group")
		}
	})
	if err == nil || units != nil || sealCalls != 0 {
		t.Fatalf("non-contiguous duplicate commit was not rejected before resealing: units=%#v calls=%d err=%v", units, sealCalls, err)
	}
}

func TestAcceptedFinalDeliveryUnitsRejectDuplicateTurnAndReversedFrontier(t *testing.T) {
	first := acceptedFinalSSEDeliveryBatchFixture(t)
	second := remapAcceptedFinalSSEDeliveryBatchFixture(t, first, "turn-sse-second", 4, 41)
	sealFor := func(group []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
		switch contracts.StringField(group[0], "turnId") {
		case first.TurnID:
			return first.PublicationAuthority, nil
		case second.TurnID:
			return second.PublicationAuthority, nil
		default:
			return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("unexpected accepted-final group")
		}
	}

	t.Run("same turn different commit", func(t *testing.T) {
		events := append([]map[string]any{}, first.Events...)
		for _, event := range second.Events {
			cloned := contracts.CloneMap(event)
			cloned["turnId"] = first.TurnID
			events = append(events, cloned)
		}
		sealCalls := 0
		units, err := acceptedFinalDeliveryUnits(first.ThreadID, events, func(group []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			sealCalls++
			return sealFor(group)
		})
		if err == nil || units != nil || sealCalls != 0 {
			t.Fatalf("duplicate accepted-final turn was not rejected before its second seal: units=%#v calls=%d err=%v", units, sealCalls, err)
		}
	})

	t.Run("reversed group frontier", func(t *testing.T) {
		events := append(append([]map[string]any{}, second.Events...), first.Events...)
		sealCalls := 0
		units, err := acceptedFinalDeliveryUnits(first.ThreadID, events, func(group []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			sealCalls++
			return sealFor(group)
		})
		if err == nil || units != nil || sealCalls != 0 {
			t.Fatalf("reversed accepted-final frontier was not rejected before its second seal: units=%#v calls=%d err=%v", units, sealCalls, err)
		}
	})
}

func TestAcceptedFinalReplayRejectsNonContiguousDuplicateCommitWithoutPrefix(t *testing.T) {
	first := acceptedFinalSSEDeliveryBatchFixture(t)
	second := remapAcceptedFinalSSEDeliveryBatchFixture(t, first, "turn-sse-second", 4, 41)
	events := make([]map[string]any, 0, len(first.Events)*2+len(second.Events))
	events = append(events, first.Events...)
	events = append(events, second.Events...)
	for _, event := range first.Events {
		cloned := contracts.CloneMap(event)
		seq, _ := contracts.NumericSeq(cloned["seq"])
		cloned["seq"] = float64(seq + second.LastSeq)
		events = append(events, cloned)
	}
	sealCalls := 0
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return len(events), nil },
		LoadPublicEventsSince: func(string, int) ([]map[string]any, error) {
			return events, nil
		},
		SealAcceptedFinalDelivery: func(group []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			sealCalls++
			if contracts.StringField(group[0], "turnId") == first.TurnID {
				return first.PublicationAuthority, nil
			}
			return second.PublicationAuthority, nil
		},
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0", nil), first.ThreadID)
	body := recorder.Body.String()
	if sealCalls != 0 || !strings.Contains(body, `"kind":"public_projection_revoked"`) ||
		!strings.Contains(body, `"code":"case_public_authority_rejected"`) {
		t.Fatalf("split commit did not revoke without a third seal: calls=%d body=%s", sealCalls, body)
	}
	for _, forbidden := range []string{"event: accepted_final_batch", domainevidence.CaseSourceUnavailableText} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("split accepted-final commit leaked a public prefix %q: %s", forbidden, body)
		}
	}
}

func TestMalformedAcceptedFinalLiveBatchRevokesWithoutPublishingPrefix(t *testing.T) {
	batch := domainevent.AcceptedFinalDeliveryBatchV2Map(acceptedFinalSSEDeliveryBatchFixture(t))
	delete(batch, "eventManifestDigest")
	live := make(chan map[string]any, 1)
	live <- batch
	close(live)
	handler := ThreadEventsHandler{Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 0, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, nil },
		SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=0&live=1", nil), "thread-sse")
	body := recorder.Body.String()
	if !strings.Contains(body, "public_projection_revoked") ||
		!strings.Contains(body, casePublicAuthorityRejectedCode) {
		t.Fatalf("malformed accepted-final batch did not revoke public projection:\n%s", body)
	}
	for _, forbidden := range []string{"event: accepted_final_batch", "event: item_completed", domainevidence.CaseSourceUnavailableText} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("malformed accepted-final batch leaked a prefix or final text %q:\n%s", forbidden, body)
		}
	}
}

func TestGeneralTerminalReplayWritesOneNonAuthoritativeAtomicBatch(t *testing.T) {
	events := generalTerminalSSEFixture(t)
	store := generalTerminalSSEStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 13, nil },
		LoadPublicEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
			if threadID != "thread-general-sse" || afterSeq != 9 {
				t.Fatalf("unexpected replay input: threadID=%q afterSeq=%d", threadID, afterSeq)
			}
			return events, nil
		},
	})
	if err := domainevent.ValidateGeneralTerminalDeliveryEventsV1(events); err != nil {
		t.Fatalf("fixture is not a valid durable terminal group: %v", err)
	}
	projected := make([]map[string]any, 0, len(events))
	for _, event := range events {
		public, visible := publicEvent(event)
		if !visible || public == nil {
			t.Fatalf("fixture did not pass the base public filter: %#v", event)
		}
		value, visible, code := store.ProjectPublic("thread-general-sse", public)
		if !visible || code != "" {
			t.Fatalf("fixture projection failed: visible=%v code=%q", visible, code)
		}
		projected = append(projected, value)
	}
	if _, err := domainevent.NewGeneralTerminalDeliveryBatchV1(events, projected); err != nil {
		t.Fatalf("fixture cannot form a public terminal batch: %v", err)
	}
	rewound, err := domainevent.AtomicTerminalReplayEventsAfter(events, 12)
	if err != nil || len(rewound) != len(events) {
		t.Fatalf("ordinary replay did not rewind full group: events=%#v err=%v", rewound, err)
	}
	handler := ThreadEventsHandler{Store: store}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=12", nil), "thread-general-sse")
	body := recorder.Body.String()
	for _, expected := range []string{
		"event: general_terminal_batch", "id: 13", `"firstSeq":11`, `"lastSeq":13`,
		`"transportAuthority":"host_batch_digest_v1"`, `"evidenceAuthority":false`,
		`"citationAuthority":false`, `"factAnswerAllowed":false`, domainevent.GeneralTerminalCompletedBoundaryTextV1,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("general terminal atomic replay missing %q:\n%s", expected, body)
		}
	}
	if strings.Count(body, "event: general_terminal_batch") != 1 {
		t.Fatalf("general terminal batch was not emitted exactly once:\n%s", body)
	}
	for _, forbidden := range []string{"event: item_completed", "event: usage", "event: turn_completed", "accepted_final_batch"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("general terminal prefix escaped or gained accepted authority %q:\n%s", forbidden, body)
		}
	}
}

func TestGeneralTerminalLiveBuffersUntilCompleteBatch(t *testing.T) {
	events := generalTerminalSSEFixture(t)
	live := make(chan map[string]any, len(events))
	for _, event := range events {
		live <- event
	}
	close(live)
	handler := ThreadEventsHandler{Store: generalTerminalSSEStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 10, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, nil },
		SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=10&live=1", nil), "thread-general-sse")
	body := recorder.Body.String()
	if strings.Count(body, "event: general_terminal_batch") != 1 || !strings.Contains(body, "id: 13") ||
		!strings.Contains(body, domainevent.GeneralTerminalCompletedBoundaryTextV1) {
		t.Fatalf("live general terminal group was not emitted atomically:\n%s", body)
	}
	for _, forbidden := range []string{"event: item_completed", "event: usage", "event: turn_completed"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("live general terminal prefix escaped as %q:\n%s", forbidden, body)
		}
	}
}

func TestGeneralTerminalLiveRecoversDurableTailAfterNotificationGap(t *testing.T) {
	events := generalTerminalSSEFixture(t)
	live := make(chan map[string]any, 1)
	live <- events[0]
	highestCalls := 0
	handler := ThreadEventsHandler{
		HeartbeatInterval: time.Millisecond,
		Store: generalTerminalSSEStore(ThreadEventStore{
			HighestSeq: func(string) (int, error) {
				highestCalls++
				if highestCalls == 1 {
					return 10, nil
				}
				return 13, nil
			},
			LoadPublicEventsSince: func(threadID string, afterSeq int) ([]map[string]any, error) {
				if threadID != "thread-general-sse" || afterSeq != 7 {
					t.Fatalf("unexpected catch-up input: threadID=%q afterSeq=%d", threadID, afterSeq)
				}
				return events, nil
			},
			SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
		}),
	}
	recorder := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/events?since_seq=10&live=1", nil).WithContext(ctx)

	handler.ServeHTTP(recorder, request, "thread-general-sse")

	body := recorder.Body.String()
	if strings.Count(body, "event: general_terminal_batch") != 1 ||
		!strings.Contains(body, "id: 13") || highestCalls < 2 {
		t.Fatalf("durable terminal tail was not recovered on the quiet live stream: highestCalls=%d body=%s", highestCalls, body)
	}
	for _, forbidden := range []string{"event: item_completed", "event: usage", "event: turn_completed"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("recovered terminal tail escaped as %q: %s", forbidden, body)
		}
	}
}

func TestMalformedGeneralTerminalLiveGroupRevokesWithoutPublishingPrefix(t *testing.T) {
	events := generalTerminalSSEFixture(t)
	live := make(chan map[string]any, 2)
	live <- events[0]
	live <- events[2]
	close(live)
	handler := ThreadEventsHandler{Store: generalTerminalSSEStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 10, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return nil, nil },
		SubscribeEvents: func(string) (<-chan map[string]any, func()) { return live, func() {} },
	})}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?since_seq=10&live=1", nil), "thread-general-sse")
	body := recorder.Body.String()
	if !strings.Contains(body, "public_projection_revoked") || !strings.Contains(body, casePublicAuthorityRejectedCode) {
		t.Fatalf("malformed live general terminal group did not revoke:\n%s", body)
	}
	for _, forbidden := range []string{"event: general_terminal_batch", "event: item_completed", domainevent.GeneralTerminalCompletedBoundaryTextV1} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("malformed live general terminal group leaked %q:\n%s", forbidden, body)
		}
	}
}

func generalTerminalSSEStore(store ThreadEventStore) ThreadEventStore {
	store.PreflightPublic = func(string) string { return "" }
	store.ProjectPublic = func(threadID string, event map[string]any) (map[string]any, bool, string) {
		if strings.TrimSpace(contracts.StringField(event, "threadId")) != strings.TrimSpace(threadID) {
			return nil, false, ""
		}
		projected := contracts.CloneMap(event)
		for _, marker := range []string{
			"generalTerminalCommitId", "generalTerminalEventId", "generalTerminalSlot",
			"generalTerminalPayloadDigest", "generalTerminalAuthorityKind", "generalTerminalAuthorityDigest",
		} {
			delete(projected, marker)
		}
		return projected, true, ""
	}
	store.ValidateAcceptedFinalDelivery = func(domainevent.AcceptedFinalDeliveryBatchV2) error { return nil }
	return store
}

func generalTerminalSSEFixture(t *testing.T) []map[string]any {
	t.Helper()
	commitID := domainsecurity.SHA256Hex([]byte("general-terminal-sse-commit"))
	authorityDigest := domainsecurity.SHA256Hex([]byte("general-terminal-sse-cas"))
	timestamp := "2026-07-20T03:00:00Z"
	events := []map[string]any{
		{
			"kind": "item_completed", "threadId": "thread-general-sse", "turnId": "turn-general-sse",
			"itemId": "item-general-sse", "seq": float64(11), "timestamp": timestamp,
			"item": map[string]any{
				"id": "item-general-sse", "threadId": "thread-general-sse", "turnId": "turn-general-sse",
				"role": "assistant", "kind": "assistant_text", "status": "completed", "createdAt": timestamp,
				"finishedAt": timestamp, "text": domainevent.GeneralTerminalCompletedBoundaryTextV1,
			},
		},
		{
			"kind": "usage", "threadId": "thread-general-sse", "turnId": "turn-general-sse",
			"seq": float64(12), "timestamp": timestamp, "model": "",
			"usage":            domainterminaltelemetry.ProviderUsageMap(domainmodel.Usage{}),
			"cacheDiagnostics": map[string]any{}, "usageFinalStatus": "completed",
		},
		{
			"kind": "turn_completed", "threadId": "thread-general-sse", "turnId": "turn-general-sse",
			"seq": float64(13), "timestamp": timestamp, "status": "completed", "terminalReason": "success",
		},
	}
	for index, slot := range []string{"terminal-item", "usage", "terminal"} {
		event := events[index]
		event["generalTerminalCommitId"] = commitID
		event["generalTerminalEventId"] = domainevent.GeneralTerminalDeliveryEventIDV1(commitID, slot)
		event["generalTerminalSlot"] = slot
		event["generalTerminalAuthorityKind"] = domainevent.GeneralTerminalCASAuthorityKind
		event["generalTerminalAuthorityDigest"] = authorityDigest
		event["generalTerminalPayloadDigest"] = domainevent.GeneralTerminalDeliveryPayloadDigestV1(event)
		if event["generalTerminalPayloadDigest"] != domainturnterminal.GeneralTerminalPublicationPayloadDigestV1(event) {
			t.Fatalf("SSE transport payload digest drifted from the durable outbox at slot %s", slot)
		}
	}
	return events
}

func acceptedFinalSSEDeliveryBatchFixture(t *testing.T) domainevent.AcceptedFinalDeliveryBatchV2 {
	t.Helper()
	acceptedFinal, privateKey := acceptedFinalSSEFixtureWithAuthority(t)
	commitID, _ := acceptedFinal["recordDigest"].(string)
	record, err := domainevidence.ParseAcceptedFinalRecord(acceptedFinal)
	if err != nil {
		t.Fatal(err)
	}
	view, err := domainevidence.NewAcceptedFinalPublicViewFromRecordV3(record)
	if err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{
		{
			"kind": "item_completed", "threadId": "thread-sse", "turnId": "turn-sse", "seq": float64(1),
			"itemId":    "item_turn-sse_assistant",
			"timestamp": "2026-07-11T00:00:00Z", "item": map[string]any{
				"id": "item_turn-sse_assistant", "threadId": "thread-sse", "turnId": "turn-sse", "role": "assistant",
				"kind": "assistant_text", "status": "completed", "createdAt": "2026-07-11T00:00:00Z",
				"finishedAt": "2026-07-11T00:00:00Z", "text": domainevidence.CaseSourceUnavailableText,
				"acceptedFinalView": domainevidence.AcceptedFinalPublicViewRecordV3(view),
			},
		},
		{
			"kind": "usage", "threadId": "thread-sse", "turnId": "turn-sse", "seq": float64(2),
			"timestamp": "2026-07-11T00:00:00Z", "usageFinalStatus": "completed",
		},
		{
			"kind": "turn_completed", "threadId": "thread-sse", "turnId": "turn-sse", "seq": float64(3),
			"timestamp": "2026-07-11T00:00:00Z", "status": "completed", "terminalReason": "source_unavailable",
		},
	}
	for index, slot := range []string{"assistant-final", "usage", "terminal"} {
		event := events[index]
		event["acceptedFinalDigest"] = commitID
		event["publicationCommitId"] = commitID
		event["publicationEventId"] = acceptedFinalSSEHash("analytix.accepted-final-event/v1\x00" + commitID + "\x00" + slot)
		event["publicationSlot"] = slot
		payload := make(map[string]any, len(event))
		for key, value := range event {
			if key != "seq" && key != "publicationPayloadDigest" {
				payload[key] = value
			}
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		event["publicationPayloadDigest"] = acceptedFinalSSEHashBytes(body)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	seal, err := domainevent.NewAcceptedFinalDeliverySealForEventsV2(
		events,
		acceptedFinalSSEHash("accepted-final-disposition-thread-sse"),
		acceptedFinalSSEHash("terminal-disposition-thread-sse"),
		acceptedFinalSSEHashBytes(publicKey),
		publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := domainevent.NewAcceptedFinalDeliveryBatchV2(events, seal)
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func remapAcceptedFinalSSEDeliveryBatchFixture(
	t *testing.T,
	base domainevent.AcceptedFinalDeliveryBatchV2,
	turnID string,
	firstSeq int,
	seedByte byte,
) domainevent.AcceptedFinalDeliveryBatchV2 {
	t.Helper()
	commitID := acceptedFinalSSEHash("accepted-final-sse/" + turnID)
	events := make([]map[string]any, len(base.Events))
	for index, raw := range base.Events {
		event := contracts.CloneMap(raw)
		event["turnId"] = turnID
		event["seq"] = float64(firstSeq + index)
		event["acceptedFinalDigest"] = commitID
		event["publicationCommitId"] = commitID
		slot := contracts.StringField(event, "publicationSlot")
		event["publicationEventId"] = acceptedFinalSSEHash("analytix.accepted-final-event/v1\x00" + commitID + "\x00" + slot)
		if item, ok := event["item"].(map[string]any); ok {
			item["turnId"] = turnID
			itemID := "item_" + turnID + "_assistant"
			item["id"] = itemID
			event["itemId"] = itemID
			if view, ok := item["acceptedFinalView"].(map[string]any); ok {
				view["acceptedFinalDigest"] = commitID
			}
		}
		delete(event, "publicationPayloadDigest")
		payload := make(map[string]any, len(event))
		for key, value := range event {
			if key != "seq" {
				payload[key] = value
			}
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		event["publicationPayloadDigest"] = acceptedFinalSSEHashBytes(body)
		events[index] = event
	}
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = seedByte
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	seal, err := domainevent.NewAcceptedFinalDeliverySealForEventsV2(
		events,
		acceptedFinalSSEHash("accepted-final-disposition-"+turnID),
		acceptedFinalSSEHash("terminal-disposition-"+turnID),
		acceptedFinalSSEHashBytes(publicKey),
		publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := domainevent.NewAcceptedFinalDeliveryBatchV2(events, seal)
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func acceptedFinalSSEHash(value string) string {
	return acceptedFinalSSEHashBytes([]byte(value))
}

func acceptedFinalSSEHashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("%x", digest[:])
}

func acceptedFinalSSEFixture(t *testing.T) map[string]any {
	t.Helper()
	record, _ := acceptedFinalSSEFixtureWithAuthority(t)
	return record
}

func acceptedFinalSSEFixtureWithAuthority(t *testing.T) (map[string]any, ed25519.PrivateKey) {
	t.Helper()
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-sse", TurnID: "turn-sse", WorkspaceRealPath: "/workspace", CaseID: "case-sse",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-sse")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-sse"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-sse")), ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: securityContext, TerminalReason: "source_unavailable",
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(securityContext, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	record, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return domainevidence.AcceptedFinalRecordMap(record), privateKey
}

func TestThreadEventReplayPropagatesCancelledRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	handler := ThreadEventsHandler{Store: ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 1, nil },
		PreflightPublic: func(string) string { return "" },
		ProjectPublic: func(_ string, event map[string]any) (map[string]any, bool, string) {
			return event, true, ""
		},
		LoadPublicEventsSinceContext: func(observed context.Context, _ string, _ int) ([]map[string]any, error) {
			called = true
			if !errors.Is(observed.Err(), context.Canceled) {
				t.Fatalf("loader received a live context after request cancellation: %v", observed.Err())
			}
			return nil, observed.Err()
		},
	}}
	_, setup, revocation := handler.replay(ctx, "thr_cancelled_replay", 0)
	if !called || revocation != "" || setup == nil || setup.phase != "replay" || !errors.Is(setup.err, context.Canceled) {
		t.Fatalf("cancelled replay was not propagated: called=%v setup=%#v revocation=%q", called, setup, revocation)
	}
}
