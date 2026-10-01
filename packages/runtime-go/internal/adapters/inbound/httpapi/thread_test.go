package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func TestThreadHandlersListAndCreate(t *testing.T) {
	stub := &threadServiceStub{
		listResult:   []map[string]any{{"id": "thr_1"}},
		createResult: map[string]any{"id": "thr_new"},
	}
	handler := ThreadHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads?archived_only=true&include=side&limit=3&search=needle", nil)
	handler.HandleThreads(recorder, request)
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusOK || len(body["threads"].([]any)) != 1 {
		t.Fatalf("unexpected list response code=%d body=%#v", recorder.Code, body)
	}
	if !stub.listInput.ArchivedOnly || !stub.listInput.IncludeSide || stub.listInput.Limit != 3 || stub.listInput.Search != "needle" {
		t.Fatalf("unexpected list input: %#v", stub.listInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/threads", strings.NewReader(`{"title":"Draft"}`))
	handler.HandleThreads(recorder, request)
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusCreated || body["id"] != "thr_new" || stub.created["title"] != "Draft" {
		t.Fatalf("unexpected create response code=%d body=%#v created=%#v", recorder.Code, body, stub.created)
	}
}

func TestThreadHandlersRecordErrorMapping(t *testing.T) {
	stub := &threadServiceStub{thread: map[string]any{"id": "thr_1"}, patchResult: map[string]any{"title": "Renamed"}, deleteResult: true}
	handler := ThreadHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleRecord(recorder, request, "thr_1")
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusOK || body["id"] != "thr_1" {
		t.Fatalf("unexpected get response code=%d body=%#v", recorder.Code, body)
	}

	stub.getErr = threadapp.ErrPublicProjectionPending
	recorder = httptest.NewRecorder()
	handler.HandleRecord(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != "public_projection_pending" ||
		strings.Contains(string(recorder.Body.Bytes()), threadapp.ErrPublicProjectionPending.Error()) {
		t.Fatalf("pending public projection was not mapped to a safe retryable response: code=%d body=%#v", recorder.Code, body)
	}
	stub.getErr = errors.Join(threadapp.ErrPublicProjectionPending, errors.New("PRIVATE_RETAINED_MATERIAL_FAILURE"))
	recorder = httptest.NewRecorder()
	handler.HandleRecord(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != "public_projection_pending" || body["turns"] != nil ||
		strings.Contains(recorder.Body.String(), "PRIVATE_RETAINED_MATERIAL_FAILURE") {
		t.Fatal("retained authority failure leaked details or a partial batch")
	}
	stub.getErr = nil

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/ignored", strings.NewReader(`{`))
	handler.HandleRecord(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected patch validation code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/ignored", strings.NewReader(`{"executionPolicyVersion":2}`))
	handler.HandleRecord(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage || stub.patch != nil {
		t.Fatalf("runtime-owned marker patch should be rejected before service dispatch: code=%d body=%#v patch=%#v", recorder.Code, body, stub.patch)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/ignored", strings.NewReader(`{"status":"idle"}`))
	handler.HandleRecord(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage || stub.patch != nil {
		t.Fatalf("runtime-owned status patch should be rejected before service dispatch: code=%d body=%#v patch=%#v", recorder.Code, body, stub.patch)
	}

	stub.patchErr = os.ErrNotExist
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/ignored", strings.NewReader(`{"title":"Renamed"}`))
	handler.HandleRecord(recorder, request, "missing")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected patch not found code=%d body=%#v", recorder.Code, body)
	}
}

func TestThreadHandlersHydrateAcceptedFinalDeliveryOrFailClosed(t *testing.T) {
	stub := &threadServiceStub{thread: map[string]any{
		"id": "thr_hydration", "latestSeq": float64(7),
		"acceptedFinalDelivery":   map[string]any{"canary": "STALE_SINGLE_ALIAS"},
		"acceptedFinalDeliveries": []any{map[string]any{"canary": "STALE_ARRAY_ALIAS"}},
		"turns": []any{
			map[string]any{"acceptedFinalView": map[string]any{}},
			map[string]any{"acceptedFinalView": map[string]any{}},
		},
	}}
	first := map[string]any{
		"kind": "accepted_final_batch", "threadId": "thr_hydration", "turnId": "turn_first", "lastSeq": float64(3),
	}
	latest := map[string]any{
		"kind": "accepted_final_batch", "threadId": "thr_hydration", "lastSeq": float64(7),
	}
	handler := ThreadHandlers{
		Service: stub,
		HydrateAcceptedFinalDelivery: func(ctx context.Context, threadID string, thread map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
			if ctx == nil || threadID != "thr_hydration" || thread["latestSeq"] != float64(7) {
				t.Fatalf("hydration input is detached: threadID=%q thread=%#v", threadID, thread)
			}
			if thread["acceptedFinalDelivery"] != nil || thread["acceptedFinalDeliveries"] != nil {
				t.Fatalf("ambient delivery alias reached the hydration authority: %#v", thread)
			}
			return threadapp.AcceptedFinalHydrationProjectionV1{
				Latest: latest, Deliveries: []map[string]any{first, latest},
			}, nil
		},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleRecord(recorder, request, "thr_hydration")
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusOK {
		t.Fatalf("hydrated thread response failed: code=%d body=%#v", recorder.Code, body)
	}
	if _, present := body["acceptedFinalDelivery"]; present {
		t.Fatalf("multi-history response retained the singular legacy alias: %#v", body["acceptedFinalDelivery"])
	}
	deliveries, _ := body["acceptedFinalDeliveries"].([]any)
	if len(deliveries) != 2 || deliveries[0].(map[string]any)["turnId"] != "turn_first" ||
		deliveries[1].(map[string]any)["lastSeq"] != float64(7) {
		t.Fatalf("bounded per-turn deliveries were not attached exactly: %#v", deliveries)
	}
	if strings.Contains(recorder.Body.String(), "STALE_SINGLE_ALIAS") ||
		strings.Contains(recorder.Body.String(), "STALE_ARRAY_ALIAS") {
		t.Fatalf("ambient delivery alias survived exact hydration: %s", recorder.Body.String())
	}

	for _, hostile := range []struct {
		name       string
		projection threadapp.AcceptedFinalHydrationProjectionV1
		canary     string
	}{
		{
			name: "missing delivery",
			projection: threadapp.AcceptedFinalHydrationProjectionV1{
				Latest: first, Deliveries: []map[string]any{first},
			},
		},
		{
			name: "extra delivery",
			projection: threadapp.AcceptedFinalHydrationProjectionV1{
				Latest: map[string]any{"kind": "accepted_final_batch", "turnId": "EXTRA_DELIVERY_CANARY"},
				Deliveries: []map[string]any{
					first, latest, {"kind": "accepted_final_batch", "turnId": "EXTRA_DELIVERY_CANARY"},
				},
			},
			canary: "EXTRA_DELIVERY_CANARY",
		},
	} {
		t.Run(hostile.name, func(t *testing.T) {
			hostileHandler := handler
			hostileHandler.HydrateAcceptedFinalDelivery = func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
				return hostile.projection, nil
			}
			hostileRecorder := httptest.NewRecorder()
			hostileHandler.HandleRecord(hostileRecorder, request, "thr_hydration")
			hostileBody := decodeThreadBody(t, hostileRecorder)
			if hostileRecorder.Code != http.StatusServiceUnavailable ||
				hostileBody["acceptedFinalDelivery"] != nil || hostileBody["acceptedFinalDeliveries"] != nil ||
				(hostile.canary != "" && strings.Contains(hostileRecorder.Body.String(), hostile.canary)) {
				t.Fatalf("%s did not fail closed: code=%d body=%#v", hostile.name, hostileRecorder.Code, hostileBody)
			}
		})
	}

	singleStub := &threadServiceStub{thread: map[string]any{
		"id": "thr_hydration", "latestSeq": float64(3),
		"turns": []any{map[string]any{"acceptedFinalView": map[string]any{}}},
	}}
	singleHandler := ThreadHandlers{
		Service: singleStub,
		HydrateAcceptedFinalDelivery: func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
			return threadapp.AcceptedFinalHydrationProjectionV1{
				Latest: first, Deliveries: []map[string]any{first},
			}, nil
		},
	}
	recorder = httptest.NewRecorder()
	singleHandler.HandleRecord(recorder, request, "thr_hydration")
	singleBody := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusOK || !reflect.DeepEqual(singleBody["acceptedFinalDelivery"], first) {
		t.Fatalf("single-history response lost the exact legacy alias: code=%d body=%#v", recorder.Code, singleBody)
	}

	boundedTurns := make([]any, domainevent.AcceptedFinalDeliveryGroupLimitV1)
	boundedDeliveries := make([]map[string]any, domainevent.AcceptedFinalDeliveryGroupLimitV1)
	for index := range boundedTurns {
		boundedTurns[index] = map[string]any{"acceptedFinalView": map[string]any{}}
		boundedDeliveries[index] = map[string]any{
			"kind": "accepted_final_batch", "turnId": "turn-bounded-" + strconv.Itoa(index+1),
		}
	}
	boundedStub := &threadServiceStub{thread: map[string]any{
		"id": "thr_hydration", "latestSeq": float64(domainevent.AcceptedFinalDeliveryGroupLimitV1),
		"turns": boundedTurns,
	}}
	boundedHandler := ThreadHandlers{
		Service: boundedStub,
		HydrateAcceptedFinalDelivery: func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
			return threadapp.AcceptedFinalHydrationProjectionV1{
				Latest: boundedDeliveries[len(boundedDeliveries)-1], Deliveries: boundedDeliveries,
			}, nil
		},
	}
	recorder = httptest.NewRecorder()
	boundedHandler.HandleRecord(recorder, request, "thr_hydration")
	boundedBody := decodeThreadBody(t, recorder)
	boundedBodyDeliveries, _ := boundedBody["acceptedFinalDeliveries"].([]any)
	if recorder.Code != http.StatusOK || len(boundedBodyDeliveries) != domainevent.AcceptedFinalDeliveryGroupLimitV1 {
		t.Fatalf("256 accepted-final deliveries did not pass the HTTP boundary: code=%d count=%d", recorder.Code, len(boundedBodyDeliveries))
	}

	overflowDeliveries := append([]map[string]any{}, boundedDeliveries...)
	overflowDeliveries = append(overflowDeliveries, map[string]any{
		"kind": "accepted_final_batch", "turnId": "PRIVATE_OVERFLOW_CANARY",
	})
	boundedHandler.HydrateAcceptedFinalDelivery = func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
		return threadapp.AcceptedFinalHydrationProjectionV1{
			Latest: overflowDeliveries[len(overflowDeliveries)-1], Deliveries: overflowDeliveries,
		}, nil
	}
	recorder = httptest.NewRecorder()
	boundedHandler.HandleRecord(recorder, request, "thr_hydration")
	overflowBody := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable || overflowBody["code"] != "accepted_final_hydration_unavailable" ||
		overflowBody["acceptedFinalDelivery"] != nil || overflowBody["acceptedFinalDeliveries"] != nil ||
		strings.Contains(recorder.Body.String(), "PRIVATE_OVERFLOW_CANARY") {
		t.Fatalf("257 accepted-final deliveries did not fail closed at HTTP: code=%d body=%#v", recorder.Code, overflowBody)
	}

	handler.HydrateAcceptedFinalDelivery = func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
		return threadapp.AcceptedFinalHydrationProjectionV1{}, errors.New("PRIVATE_HYDRATION_FAILURE")
	}
	recorder = httptest.NewRecorder()
	handler.HandleRecord(recorder, request, "thr_hydration")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable ||
		body["code"] != "accepted_final_hydration_unavailable" ||
		strings.Contains(recorder.Body.String(), "PRIVATE_HYDRATION_FAILURE") ||
		body["acceptedFinalDelivery"] != nil || body["acceptedFinalDeliveries"] != nil {
		t.Fatalf("hydration failure did not fail closed: code=%d body=%#v", recorder.Code, body)
	}

	handler.HydrateAcceptedFinalDelivery = func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
		return threadapp.AcceptedFinalHydrationProjectionV1{
			Latest: latest,
			Deliveries: []map[string]any{{
				"kind": "accepted_final_batch", "threadId": "thr_foreign",
			}},
		}, nil
	}
	recorder = httptest.NewRecorder()
	handler.HandleRecord(recorder, request, "thr_hydration")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != "accepted_final_hydration_unavailable" {
		t.Fatalf("torn latest/per-turn projection did not fail closed: code=%d body=%#v", recorder.Code, body)
	}

	handler.HydrateAcceptedFinalDelivery = func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
		return threadapp.AcceptedFinalHydrationProjectionV1{}, nil
	}
	recorder = httptest.NewRecorder()
	handler.HandleRecord(recorder, request, "thr_hydration")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != "accepted_final_hydration_unavailable" {
		t.Fatalf("accepted-final reference without delivery authority did not fail closed: code=%d body=%#v", recorder.Code, body)
	}

	missingCallback := ThreadHandlers{Service: &threadServiceStub{thread: map[string]any{
		"id": "thr_missing_hydration", "turns": []any{map[string]any{"acceptedFinal": map[string]any{}}},
	}}}
	recorder = httptest.NewRecorder()
	missingCallback.HandleRecord(recorder, request, "thr_missing_hydration")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != "accepted_final_hydration_unavailable" {
		t.Fatalf("missing accepted-final hydration callback did not fail closed: code=%d body=%#v", recorder.Code, body)
	}
}

func TestThreadHandlersPrivateQAHydrationClassDoesNotExposeFailureText(t *testing.T) {
	const privateCanary = "PRIVATE_HYDRATION_CANARY"
	stub := &threadServiceStub{thread: map[string]any{
		"id": "thr_hydration_qa", "latestSeq": float64(1),
		"turns": []any{map[string]any{"acceptedFinalView": map[string]any{}}},
	}}
	handler := ThreadHandlers{
		Service: stub,
		HydrateAcceptedFinalDelivery: func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
			return threadapp.AcceptedFinalHydrationProjectionV1{}, errors.New(
				"accepted final hydration snapshot and event frontier are torn: " + privateCanary,
			)
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/thr_hydration_qa", nil)
	for _, qa := range []bool{false, true} {
		if qa {
			t.Setenv("ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK", "1")
		} else {
			t.Setenv("ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK", "")
		}
		recorder := httptest.NewRecorder()
		handler.HandleRecord(recorder, request, "thr_hydration_qa")
		body := decodeThreadBody(t, recorder)
		wantClass := ""
		if qa {
			wantClass = "frontier_torn"
		}
		if recorder.Code != http.StatusServiceUnavailable ||
			body["code"] != "accepted_final_hydration_unavailable" ||
			recorder.Header().Get("X-Analytix-QA-Hydration-Class") != wantClass ||
			strings.Contains(recorder.Body.String(), privateCanary) {
			t.Fatalf("unsafe hydration response: qa=%t code=%d class=%q", qa,
				recorder.Code, recorder.Header().Get("X-Analytix-QA-Hydration-Class"))
		}
	}
	if got := acceptedFinalHydrationQAClass(errors.New(privateCanary)); got != "other" {
		t.Fatalf("private error was promoted to a diagnostic category: %q", got)
	}
}

func TestThreadHandlersHydrationFrontierPendingDoesNotAdmitInvalidDelivery(t *testing.T) {
	stub := &threadServiceStub{thread: map[string]any{
		"id": "thr_hydration_pending", "latestSeq": float64(1),
		"turns": []any{map[string]any{"acceptedFinalView": map[string]any{}}},
	}}
	handler := ThreadHandlers{
		Service: stub,
		HydrateAcceptedFinalDelivery: func(context.Context, string, map[string]any) (threadapp.AcceptedFinalHydrationProjectionV1, error) {
			return threadapp.AcceptedFinalHydrationProjectionV1{}, errors.Join(
				threadapp.ErrPublicProjectionPending,
				errors.New("accepted final hydration snapshot and event frontier are torn"),
			)
		},
	}
	t.Setenv("ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SOAK", "1")
	recorder := httptest.NewRecorder()
	handler.HandleRecord(recorder, httptest.NewRequest(http.MethodGet, "/v1/threads/thr_hydration_pending", nil), "thr_hydration_pending")
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusServiceUnavailable || body["code"] != "public_projection_pending" ||
		recorder.Header().Get("X-Analytix-QA-Hydration-Class") != "" ||
		body["acceptedFinalDelivery"] != nil || body["acceptedFinalDeliveries"] != nil {
		t.Fatalf("torn frontier was admitted or misclassified: status=%d body=%#v", recorder.Code, body)
	}
}

func TestThreadHandlersRewindProjectsPublicResponse(t *testing.T) {
	// The committed mutation carries an internal authority turn. It must not
	// escape the public RewindThreadResponse contract after the durable cut.
	result := map[string]any{
		"threadId": "thr_1", "turnId": "turn_1", "removedTurns": 2,
		"remainingTurns": 1, "removedTurnIds": []any{"turn_1", "turn_2"},
		"authorityTurnId": "private_rewind_transition", "futurePrivateField": "private_canary",
	}
	handler := ThreadHandlers{Service: &threadServiceStub{rewindResult: result}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/threads/thr_1/rewind", strings.NewReader(`{"turnId":"turn_1"}`))
	handler.HandleRewind(recorder, request, "thr_1")
	want := map[string]any{
		"threadId": "thr_1", "turnId": "turn_1", "removedTurns": float64(2),
		"remainingTurns": float64(1), "removedTurnIds": []any{"turn_1", "turn_2"},
	}
	if body := decodeThreadBody(t, recorder); recorder.Code != http.StatusOK || !reflect.DeepEqual(body, want) {
		t.Fatalf("rewind public response mismatch: code=%d body=%#v", recorder.Code, body)
	}
	if result["authorityTurnId"] != "private_rewind_transition" || len(result) != 7 {
		t.Fatal("HTTP projection mutated the committed authority result")
	}
}

func TestThreadHandlersForkRewindAndCompactErrors(t *testing.T) {
	stub := &threadServiceStub{forkResult: map[string]any{"id": "fork_1"}, rewindResult: map[string]any{"ok": true}, compactResult: map[string]any{"ok": true}}
	handler := ThreadHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"title":"Fork"}`))
	handler.HandleFork(recorder, request, "thr_1")
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusCreated || body["id"] != "fork_1" || stub.forkRequest["title"] != "Fork" {
		t.Fatalf("unexpected fork response code=%d body=%#v request=%#v", recorder.Code, body, stub.forkRequest)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{}`))
	handler.HandleRewind(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected rewind validation code=%d body=%#v", recorder.Code, body)
	}

	stub.rewindErr = threadapp.ErrThreadRunning
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"turnId":"turn_1"}`))
	handler.HandleRewind(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["code"] != "conflict" {
		t.Fatalf("unexpected rewind conflict code=%d body=%#v", recorder.Code, body)
	}

	stub.rewindErr = threadapp.ErrAcceptedFinalRewind
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"turnId":"turn_1"}`))
	handler.HandleRewind(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["message"] != testConflictFailureMessage {
		t.Fatalf("unexpected accepted-final rewind conflict code=%d body=%#v", recorder.Code, body)
	}

	stub.compactErr = threadapp.ErrThreadRunning
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"reason":" manual "}`))
	handler.HandleCompact(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["code"] != "thread_running" || stub.compactReason != "manual" {
		t.Fatalf("unexpected compact conflict code=%d body=%#v reason=%q", recorder.Code, body, stub.compactReason)
	}

	stub.compactErr = threadapp.ErrCaseCompactionRequiresTrustedArchive
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{}`))
	handler.HandleCompact(recorder, request, "thr_case")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["code"] != "case_compaction_archive_required" ||
		body["message"] != "Case compaction requires a verified publication archive." {
		t.Fatalf("unexpected case compaction boundary code=%d body=%#v", recorder.Code, body)
	}
}

func TestThreadHandlersRejectSafeRecordAliasFork(t *testing.T) {
	stub := &threadServiceStub{forkResult: map[string]any{"id": "must-not-exist"}}
	handler := ThreadHandlers{Service: stub}
	for _, threadID := range []string{"thr:durable_1", "thr/durable/1", " thr_durable_1"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/threads/ignored/fork", strings.NewReader(`{}`))
		handler.HandleFork(recorder, request, threadID)
		body := decodeThreadBody(t, recorder)
		if recorder.Code != http.StatusNotFound || body["code"] != "not_found" || stub.forkRequest != nil {
			t.Fatalf("storage alias reached fork service: id=%q code=%d body=%#v request=%#v", threadID, recorder.Code, body, stub.forkRequest)
		}
	}
}

func TestThreadHandlersGoalTodos(t *testing.T) {
	stub := &threadServiceStub{
		goal:        map[string]any{"status": "active"},
		setGoal:     map[string]any{"status": "complete"},
		todos:       map[string]any{"items": []any{}},
		setTodos:    map[string]any{"items": []any{"a"}},
		clearResult: true,
	}
	handler := ThreadHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleGoal(recorder, request, "thr_1")
	body := decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusOK || body["goal"].(map[string]any)["status"] != "active" {
		t.Fatalf("unexpected goal get code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(`{"todos":[{"id":"todo_1","content":"a","status":"pending"}]}`))
	handler.HandleTodos(recorder, request, "thr_1")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusOK || len(stub.todosItems) != 1 {
		t.Fatalf("unexpected todos set code=%d body=%#v items=%#v", recorder.Code, body, stub.todosItems)
	}

	stub.todosErr = os.ErrNotExist
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleTodos(recorder, request, "missing")
	body = decodeThreadBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected todos not found code=%d body=%#v", recorder.Code, body)
	}
}

func TestThreadHandlersRejectMalformedTodosBeforeServiceMutation(t *testing.T) {
	stub := &threadServiceStub{setTodos: map[string]any{"items": []any{}}}
	handler := ThreadHandlers{Service: stub}
	for _, body := range []string{
		`{}`,
		`{"todos":"not-an-array"}`,
		`{"todos":["not-an-object"]}`,
		`{"todos":[],"unknown":true}`,
		`{"todos":[],"todos":[]}`,
		`{"todos":[{"id":"same","content":"one","status":"pending"},{"id":"same","content":"two","status":"pending"}]}`,
		`{"todos":[{"content":"one","status":"pending","unknown":true}]}`,
		`{"todos":[{"content":"one","status":"in_progress"},{"content":"two","status":"in_progress"}]}`,
		`{"todos":[{"content":"one","status":"failed"}]}`,
		`{"todos":[{"content":"one","status":"canceled","statusReasonCode":"tool_failed"}]}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/ignored", strings.NewReader(body))
		handler.HandleTodos(recorder, request, "thr_1")
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("malformed body passed: body=%s code=%d response=%s", body, recorder.Code, recorder.Body.String())
		}
	}
	if stub.setTodosCalls != 0 || stub.todosItems != nil {
		t.Fatalf("malformed requests reached todo persistence: calls=%d items=%#v", stub.setTodosCalls, stub.todosItems)
	}
}

func decodeThreadBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type threadServiceStub struct {
	listInput     threadapp.ListInput
	listResult    []map[string]any
	listErr       error
	created       map[string]any
	createResult  map[string]any
	createErr     error
	thread        map[string]any
	getErr        error
	patch         map[string]any
	patchResult   map[string]any
	patchErr      error
	deleteResult  bool
	deleteErr     error
	forkRequest   map[string]any
	forkResult    map[string]any
	forkErr       error
	rewindTurnID  string
	rewindResult  map[string]any
	rewindErr     error
	goal          map[string]any
	goalErr       error
	setGoalPatch  map[string]any
	setGoal       map[string]any
	setGoalErr    error
	todos         map[string]any
	todosItems    []any
	setTodosCalls int
	setTodos      map[string]any
	todosErr      error
	clearResult   bool
	clearErr      error
	compactReason string
	compactResult map[string]any
	compactErr    error
}

func (s *threadServiceStub) List(input threadapp.ListInput) ([]map[string]any, error) {
	s.listInput = input
	return s.listResult, s.listErr
}

func (s *threadServiceStub) Create(request map[string]any) (map[string]any, error) {
	s.created = request
	return s.createResult, s.createErr
}

func (s *threadServiceStub) Get(string) (map[string]any, error) {
	return s.thread, s.getErr
}

func (s *threadServiceStub) Patch(_ context.Context, _ string, patch map[string]any) (map[string]any, error) {
	s.patch = patch
	return s.patchResult, s.patchErr
}

func (s *threadServiceStub) Delete(context.Context, string) (bool, error) {
	return s.deleteResult, s.deleteErr
}

func (s *threadServiceStub) Fork(_ string, request map[string]any) (map[string]any, error) {
	s.forkRequest = request
	return s.forkResult, s.forkErr
}

func (s *threadServiceStub) Rewind(_ context.Context, _ string, turnID string) (map[string]any, error) {
	s.rewindTurnID = turnID
	return s.rewindResult, s.rewindErr
}

func (s *threadServiceStub) GetGoal(string) (map[string]any, error) {
	return s.goal, s.goalErr
}

func (s *threadServiceStub) SetGoal(_ string, patch map[string]any) (map[string]any, error) {
	s.setGoalPatch = patch
	return s.setGoal, s.setGoalErr
}

func (s *threadServiceStub) ClearGoal(string) (bool, error) {
	return s.clearResult, s.clearErr
}

func (s *threadServiceStub) GetTodos(string) (map[string]any, error) {
	return s.todos, s.todosErr
}

func (s *threadServiceStub) SetTodos(_ string, items []any) (map[string]any, error) {
	s.setTodosCalls++
	s.todosItems = items
	return s.setTodos, s.todosErr
}

func (s *threadServiceStub) ClearTodos(string) (bool, error) {
	return s.clearResult, s.clearErr
}

func (s *threadServiceStub) Compact(_ context.Context, _ string, reason string) (map[string]any, error) {
	s.compactReason = reason
	return s.compactResult, s.compactErr
}

var _ ThreadService = (*threadServiceStub)(nil)
