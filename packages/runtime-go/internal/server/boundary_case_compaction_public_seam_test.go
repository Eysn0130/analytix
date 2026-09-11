package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const boundaryCompactionPrivateSourceV1 = "private boundary history must not reach the public seam"

func TestBoundaryOnlySignedCaseCompactionSurvivesHTTPDetailAndRestart(t *testing.T) {
	fixture := newBoundaryCaseCompactionPublicSeamFixtureV1(t)

	compact := boundaryCompactionHTTPRequestV1(
		t,
		fixture.handler,
		http.MethodPost,
		"/v1/threads/"+fixture.threadID+"/compact",
		map[string]any{"reason": "manual"},
	)
	if compact.Code != http.StatusOK {
		failure := boundaryCompactionDecodeResponseV1(t, compact)
		t.Fatalf(
			"boundary-only signed compaction status=%d want=%d code=%s message=%s",
			compact.Code,
			http.StatusOK,
			stringField(failure, "code"),
			stringField(failure, "message"),
		)
	}
	compactBody := boundaryCompactionDecodeResponseV1(t, compact)
	if compactBody["ok"] != true || stringField(compactBody, "threadId") != fixture.threadID ||
		stringField(compactBody, "turnId") == "" || stringField(compactBody, "itemId") == "" ||
		stringField(compactBody, "sourceDigest") == "" || compactBody["reasoningExcluded"] != true {
		t.Fatal("boundary-only signed compaction response is incomplete")
	}
	if replaced, _ := compactBody["replacedTokens"].(float64); replaced <= 0 {
		t.Fatal("boundary-only signed compaction did not replace history")
	}

	raw, err := fixture.store.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	target, err := domainsecurity.ParseTurnSecurityContext(raw["securityState"])
	if err != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(target) ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(target) != nil ||
		target.ContextEpoch != fixture.source.ContextEpoch+1 || target.TurnID != stringField(compactBody, "turnId") {
		t.Fatalf("compaction target did not preserve and advance boundary authority: epoch=%d err=%v", target.ContextEpoch, err)
	}
	state, ok, err := contextepochapp.StateFromThread(raw)
	if err != nil || !ok || state.AcceptedSnapshot.Epoch != target.ContextEpoch ||
		state.AcceptedSnapshot.RecoveryDigest != stringField(compactBody, "sourceDigest") {
		t.Fatalf("compaction target epoch binding is invalid: ok=%t epoch=%d err=%v", ok, state.AcceptedSnapshot.Epoch, err)
	}
	committed, found := fixture.authority.CommittedContext(fixture.threadID, target.TurnID)
	if !found || committed.SecurityContext != target || committed.EpochState.StateDigest != state.StateDigest {
		t.Fatal("compaction target lacks exact installation-signed readback")
	}
	assertBoundaryCompactionRawPrivateAuthorityV1(t, raw, stringField(compactBody, "itemId"))

	detail := boundaryCompactionHTTPRequestV1(
		t,
		fixture.handler,
		http.MethodGet,
		"/v1/threads/"+fixture.threadID,
		nil,
	)
	if detail.Code != http.StatusOK {
		t.Fatalf("boundary-only public detail status=%d want=%d", detail.Code, http.StatusOK)
	}
	detailBody := boundaryCompactionDecodeResponseV1(t, detail)
	marker := assertBoundaryCompactionPublicDetailV1(t, detailBody, compactBody)
	assertBoundaryCompactionPublicEventsV1(t, fixture.store, fixture.projector, raw, compactBody, marker)

	restartedStore, restartedAuthority := fixture.reopen(t)
	if err := threadapp.RecoverCommittedCaseCompactions(
		context.Background(), restartedAuthority, restartedStore,
	); err != nil {
		t.Fatalf("restart case compaction recovery failed: %v", err)
	}
	if err := casethreadapp.RepairCommittedContexts(restartedAuthority, restartedStore); err != nil {
		t.Fatalf("restart committed-context repair failed: %v", err)
	}
	inventory, err := casethreadapp.PreflightRestartInventory(restartedAuthority, restartedStore)
	if err != nil || inventory.Quarantined[fixture.threadID] != "" {
		t.Fatalf("restart inventory rejected safe boundary compaction: quarantined=%t err=%v", inventory.Quarantined[fixture.threadID] != "", err)
	}
	if err := casethreadapp.ApplyRestartInventory(restartedAuthority, inventory); err != nil ||
		!restartedAuthority.CanExecute(fixture.threadID) {
		t.Fatalf("restart inventory did not restore executable case lineage: err=%v", err)
	}

	restartedRaw, err := restartedStore.GetThreadForAuthorityRepair(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	restartedCurrent, err := domainsecurity.ParseTurnSecurityContext(restartedRaw["securityState"])
	if err != nil || restartedCurrent != target {
		t.Fatalf("restart recovered a different boundary target: err=%v", err)
	}
	restartedHandler := newBoundaryCompactionRestartHTTPHandlerV1(
		t,
		restartedStore,
		restartedAuthority,
		fixture.workspaceSecurity,
	)
	restartedDetail := boundaryCompactionHTTPRequestV1(
		t,
		restartedHandler,
		http.MethodGet,
		"/v1/threads/"+fixture.threadID,
		nil,
	)
	if restartedDetail.Code != http.StatusOK {
		t.Fatalf("restarted boundary-only public detail status=%d want=%d", restartedDetail.Code, http.StatusOK)
	}
	restartedDetailBody := boundaryCompactionDecodeResponseV1(t, restartedDetail)
	restartedMarker := assertBoundaryCompactionPublicDetailV1(t, restartedDetailBody, compactBody)
	if stringField(restartedMarker, "reasoningExclusionProof") != stringField(marker, "reasoningExclusionProof") {
		t.Fatal("restart changed the deterministic public compaction marker")
	}
	assertBoundaryCompactionPublicEventsV1(
		t, restartedStore, restartedHandler.publicProjector, restartedRaw, compactBody, restartedMarker,
	)
}

type boundaryCaseCompactionPublicSeamFixtureV1 struct {
	durableRoot       string
	authorityPath     string
	authorityStoreDir string
	threadID          string
	source            domainsecurity.TurnSecurityContext
	workspaceSecurity turnsecurityapp.WorkspaceSecurityAuthority
	store             *DurableEventSessionStore
	authority         *casethreadapp.Registry
	projector         threadapp.PublicProjector
	handler           *runtimeServerHandler
}

func newBoundaryCaseCompactionPublicSeamFixtureV1(t *testing.T) *boundaryCaseCompactionPublicSeamFixtureV1 {
	t.Helper()
	ctx := context.Background()
	durableRoot := t.TempDir()
	dataRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataRoot,
	}).(*runtimeServerHandler)
	t.Cleanup(func() { handler.runtimeSubagentState().CancelBackgroundJobsAndWait(time.Second) })

	authorityRoot := t.TempDir()
	if err := os.Chmod(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	authorityPath := filepath.Join(authorityRoot, "authority.json")
	signer, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	authorityStoreDir := filepath.Join(t.TempDir(), "case-thread-records")
	authorityStore, err := newServerTestCaseThreadStore(t, authorityStoreDir)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := casethreadapp.NewRegistry(ctx, signer, authorityStore)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseThreads = authority
	handler.store.SetCaseThreadAuthority(authority)

	workspace := writeThreadMutationCaseBinding(t)
	observer := filestore.CaseBindingReader{}
	workspaceSecurity := turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: newServerTestRiskAuthority(),
		RiskIntent: domainsecurity.RiskClassCase, TrustedCaseThread: true,
	}
	handler.turnSecurity = workspaceSecurity
	currentAuthority := threadapp.NewCurrentCaseThreadAuthorityValidator(authority, observer, nil)
	projector := threadapp.NewTrustedPublicProjectorWithCurrentCaseAuthority(nil, authority, currentAuthority)
	handler.publicProjector = projector
	handler.threads = nil

	thread, err := handler.store.CreateThread(
		map[string]any{"title": "Boundary case compaction", "workspace": workspace}, workspace,
	)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	at := time.Date(2026, 8, 2, 1, 0, 0, 0, time.UTC)
	var source domainsecurity.TurnSecurityContext
	for index := 1; index <= 4; index++ {
		currentThread, readErr := handler.store.GetThreadForAuthorityRepair(threadID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		frozen, freezeErr := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
			Context: ctx, Authority: workspaceSecurity, Thread: currentThread,
			ThreadID: threadID, TurnID: "turn_boundary_case_compaction_" + string(rune('0'+index)),
			Workspace: workspace, Principal: testIdentityPrincipal(), IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		if freezeErr != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(frozen) ||
			domainsecurity.ValidateTurnSecurityContextForCasePublication(frozen) != nil ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(frozen) == nil {
			t.Fatalf("boundary compaction fixture context is invalid at turn %d: err=%v", index, freezeErr)
		}
		state, stateErr := contextepochapp.BootstrapState(
			threadID,
			frozen.ContextEpoch,
			[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)},
			at.Add(time.Duration(index)*time.Second),
		)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if err := casethreadapp.RegisterRequired(ctx, authority, frozen); err != nil {
			t.Fatal(err)
		}
		prompt := "continue bounded host-boundary work"
		if index == 1 {
			prompt = boundaryCompactionPrivateSourceV1
		}
		if err := handler.store.AppendTurnToThread(threadID, map[string]any{
			"id": frozen.TurnID, "threadId": threadID, "status": "completed", "prompt": prompt,
			"securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{
				"id":     "item_boundary_case_compaction_" + string(rune('0'+index)),
				"turnId": frozen.TurnID, "threadId": threadID, "kind": "user_message", "role": "user",
				"status": "completed", "text": prompt,
			}},
		}, "deepseek", map[string]any{
			"securityState": turnsecurityapp.PublicRecord(frozen), "contextEpochState": contextepochapp.PublicState(state),
		}); err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.CommitRequired(
			ctx, authority, frozen, state, at.Add(time.Duration(index+10)*time.Second),
		); err != nil {
			t.Fatal(err)
		}
		source = frozen
	}
	if _, err := handler.store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	if err := handler.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(
		ctx, source, time.Second,
	); err != nil {
		t.Fatal(err)
	}

	return &boundaryCaseCompactionPublicSeamFixtureV1{
		durableRoot: durableRoot, authorityPath: authorityPath, authorityStoreDir: authorityStoreDir,
		threadID: threadID, source: source, workspaceSecurity: workspaceSecurity,
		store: handler.store, authority: authority, projector: projector, handler: handler,
	}
}

func (fixture *boundaryCaseCompactionPublicSeamFixtureV1) reopen(
	t *testing.T,
) (*DurableEventSessionStore, *casethreadapp.Registry) {
	t.Helper()
	signer, err := finalauthorityadapter.OpenOrCreateFileAuthority(fixture.authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	authorityStore, err := newServerTestCaseThreadStore(t, fixture.authorityStoreDir)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := casethreadapp.NewRegistry(context.Background(), signer, authorityStore)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewTempDurableEventSessionStore(fixture.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	store.SetCaseThreadAuthority(authority)
	return store, authority
}

func newBoundaryCompactionRestartHTTPHandlerV1(
	t *testing.T,
	store *DurableEventSessionStore,
	authority *casethreadapp.Registry,
	workspaceSecurity turnsecurityapp.WorkspaceSecurityAuthority,
) *runtimeServerHandler {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	t.Cleanup(func() { handler.runtimeSubagentState().CancelBackgroundJobsAndWait(time.Second) })
	handler.store = store
	handler.caseThreads = authority
	handler.turnSecurity = workspaceSecurity
	currentAuthority := threadapp.NewCurrentCaseThreadAuthorityValidator(
		authority, filestore.CaseBindingReader{}, nil,
	)
	handler.publicProjector = threadapp.NewTrustedPublicProjectorWithCurrentCaseAuthority(
		nil, authority, currentAuthority,
	)
	handler.threads = nil
	return handler
}

func boundaryCompactionHTTPRequestV1(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, payload)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func boundaryCompactionDecodeResponseV1(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode boundary compaction HTTP response: %v", err)
	}
	return body
}

func assertBoundaryCompactionRawPrivateAuthorityV1(t *testing.T, thread map[string]any, itemID string) {
	t.Helper()
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") != itemID {
				continue
			}
			if item["taskContinuation"] == nil || item["caseCompactionBinding"] == nil ||
				stringField(item, "sourceContextDigest") == "" ||
				stringField(turn, "caseHistoryProjection") != "compaction_authority_v1" ||
				turn["contextEpochSnapshot"] == nil {
				t.Fatal("raw compaction target lacks signed private continuation authority")
			}
			return
		}
	}
	t.Fatal("raw compaction target item is missing")
}

func assertBoundaryCompactionPublicDetailV1(
	t *testing.T,
	detail map[string]any,
	compact map[string]any,
) map[string]any {
	t.Helper()
	if stringField(detail, "id") != stringField(compact, "threadId") ||
		stringField(detail, "historyAuthority") != threadapp.CaseBoundaryOnlyHistoryAuthority {
		t.Fatal("public detail lost its case boundary authority")
	}
	var marker map[string]any
	for _, rawTurn := range listAny(detail["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") == stringField(compact, "itemId") {
				marker = item
			}
		}
	}
	if marker == nil || stringField(marker, "kind") != "compaction" ||
		stringField(marker, "summary") == "" || stringField(marker, "sourceDigest") != stringField(compact, "sourceDigest") ||
		marker["schemaVersion"] != float64(3) || marker["caseHistoryProjectionVersion"] != float64(2) ||
		marker["reasoningExcluded"] != true || marker["assistantProseExcluded"] != true ||
		marker["toolPayloadsExcluded"] != true || marker["caseFactsExcluded"] != true {
		t.Fatal("public detail lacks the deterministic safe compaction marker")
	}
	if _, present := marker["replacedTokens"]; present {
		t.Fatal("public detail exposed an unsigned case compaction token count")
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"taskContinuation", "caseCompactionBinding", "sourceContextDigest", boundaryCompactionPrivateSourceV1,
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public detail exposed forbidden private field category %q", forbidden)
		}
	}
	return marker
}

func assertBoundaryCompactionPublicEventsV1(
	t *testing.T,
	store *DurableEventSessionStore,
	projector threadapp.PublicProjector,
	thread map[string]any,
	compact map[string]any,
	marker map[string]any,
) {
	t.Helper()
	replay, err := store.LoadEventsSince(stringField(compact, "threadId"), 0)
	if err != nil {
		t.Fatal(err)
	}
	startedFound := false
	completedFound := false
	for _, event := range replay.Events {
		kind := stringField(event, "kind")
		if (kind != "compaction_started" && kind != "compaction_completed") ||
			stringField(event, "turnId") != stringField(compact, "turnId") {
			continue
		}
		if _, sanitizeErr := turnapp.SanitizeCaseEventPublication(thread, event); sanitizeErr != nil {
			t.Fatalf("durable compaction event failed idempotent case sanitation: %v", sanitizeErr)
		}
		projected, visible, projectErr := projector.ProjectEvent(
			stringField(compact, "threadId"), thread, event,
		)
		if projectErr != nil || !visible || projected == nil {
			t.Fatalf("public compaction event projection failed: visible=%t err=%v", visible, projectErr)
		}
		if _, present := projected["replacedTokens"]; present {
			t.Fatal("public compaction event exposed an unsigned case compaction token count")
		}
		if kind == "compaction_started" {
			if stringField(projected, "kind") != kind || projected["auto"] != nil || projected["itemId"] != nil {
				t.Fatal("public compaction start event was not reduced to fixed progress metadata")
			}
			startedFound = true
			continue
		}
		if stringField(projected, "sourceDigest") != stringField(marker, "sourceDigest") ||
			projected["reasoningExcluded"] != true || projected["schemaVersion"] != float64(2) {
			t.Fatal("public compaction completion event lost its closed marker binding")
		}
		encoded, marshalErr := json.Marshal(projected)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, forbidden := range []string{"taskContinuation", "caseCompactionBinding", "sourceContextDigest"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("public compaction event exposed forbidden private field category %q", forbidden)
			}
		}
		assertBoundaryCompactionOperationStampTamperFailsV1(t, thread, event)
		completedFound = true
	}
	if !startedFound || !completedFound {
		t.Fatalf("durable compaction event pair was incomplete: started=%t completed=%t", startedFound, completedFound)
	}
}

func assertBoundaryCompactionOperationStampTamperFailsV1(
	t *testing.T,
	thread map[string]any,
	event map[string]any,
) {
	t.Helper()
	tamperedThread := contracts.CloneMap(thread)
	turnID := stringField(event, "turnId")
	var target map[string]any
	for _, rawTurn := range listAny(tamperedThread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") == turnID {
			target = turn
			break
		}
	}
	items, _ := target["items"].([]any)
	item, _ := items[0].(map[string]any)
	binding, err := turnapp.ParseCaseCompactionOperationBindingV1(item["caseCompactionBinding"])
	stamp, stampErr := strconv.ParseInt(binding.OperationStamp, 10, 64)
	if err != nil || stampErr != nil {
		t.Fatal("compaction operation stamp fixture is invalid")
	}
	binding.OperationStamp = strconv.FormatInt(stamp+1, 10)
	digest, err := turnapp.CaseCompactionOperationDigestV1(binding)
	if err != nil {
		t.Fatal(err)
	}
	item["caseCompactionBinding"] = turnapp.CaseCompactionOperationBindingMapV1(binding)
	item["sourceDigest"] = digest
	item["digestMarker"] = "sha256:" + digest[:12]
	item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
	target["contextEpochSnapshot"].(map[string]any)["recoveryDigest"] = digest
	tamperedEvent := contracts.CloneMap(event)
	tamperedEvent["sourceDigest"] = digest
	tamperedEvent["digestMarker"] = "sha256:" + digest[:12]
	tamperedEvent["reasoningExclusionProof"] = item["reasoningExclusionProof"]
	if projected, sanitizeErr := turnapp.SanitizeCaseEventPublication(tamperedThread, tamperedEvent); sanitizeErr == nil || projected != nil {
		t.Fatal("self-consistent event with a detached operation stamp entered the durable case stream")
	}
}
