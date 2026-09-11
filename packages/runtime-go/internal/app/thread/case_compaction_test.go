package thread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestCaseCompactionNarrowProjectionAuthorityFailsClosedWithoutPanic(t *testing.T) {
	current, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-narrow-authority", TurnID: "turn-case-narrow-authority",
		WorkspaceRealPath: "/cases/narrow-authority", CaseID: "case-narrow-authority",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("narrow-authority-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("narrow-authority"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("narrow-authority-manifest")),
		ContextEpoch:       1, IssuedAt: time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &repositoryStub{thread: map[string]any{
		"id": current.ThreadID, "workspace": current.WorkspaceRealPath, "caseId": current.CaseID,
		"status": "idle", "turns": []any{},
	}}
	service := NewService(Dependencies{
		Repository: repo,
		CaseThreads: caseCompactionProjectionAuthorityStubV1{contexts: map[string]domainsecurity.TurnSecurityContext{
			current.ContextDigest: current,
		}},
	})
	if _, err := service.Compact(context.Background(), current.ThreadID, "manual"); !errors.Is(err, ErrCaseCompactionRequiresTrustedArchive) {
		t.Fatalf("narrow projection authority did not fail closed: %v", err)
	}
}

func TestPreparedCaseCompactionRetainsSignedAuthorityAndTrustedFinals(t *testing.T) {
	at := time.Date(2026, 7, 29, 2, 0, 0, 0, time.UTC)
	threadID := "thread-case-compaction-authorized"
	contexts := make([]domainsecurity.TurnSecurityContext, 0, 4)
	turns := make([]any, 0, 4)
	for index := 1; index <= 4; index++ {
		turnID := "turn-case-" + string(rune('0'+index))
		securityContext, err := securitycontexttest.CaseExecutionContextV2(
			domainsecurity.TurnSecurityContextInput{
				ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/compaction-authorized",
				CaseID: "case-compaction-authorized", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")),
				DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("case-compaction-authorized"),
				SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: uint64(index),
				IssuedAt: at.Add(time.Duration(index) * time.Second),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		contexts = append(contexts, securityContext)
		turn := map[string]any{
			"id": turnID, "threadId": threadID, "status": "completed",
			"securityContext": turnsecurityapp.PublicRecord(securityContext),
			"items": []any{map[string]any{
				"id": "user-" + turnID, "kind": "user_message", "role": "user", "text": "继续当前案件核验",
			}},
		}
		if index == 1 {
			turn["acceptedFinal"] = map[string]any{"recordDigest": strings.Repeat("a", 64)}
			turn["items"] = []any{map[string]any{
				"id": "accepted-safe", "kind": "assistant_text", "acceptedFinal": map[string]any{"recordDigest": strings.Repeat("a", 64)},
			}}
		}
		if index == 2 {
			turn["items"] = []any{map[string]any{
				"id": "draft-sensitive", "kind": "assistant_text", "text": "DRAFT_CASE_SENTINEL_MUST_DISAPPEAR",
			}}
		}
		turns = append(turns, turn)
	}
	current := contexts[len(contexts)-1]
	state, err := contextepochapp.BootstrapState(
		threadID, current.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": threadID, "workspace": current.WorkspaceRealPath, "status": "idle", "turns": turns,
		"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(state),
	}
	continuation := caseCompactionContinuationFixtureV1(t, current)
	authorityTurnIDs := []string{contexts[2].TurnID, contexts[0].TurnID, contexts[3].TurnID, contexts[1].TurnID}
	prepared, err := PrepareCaseCompaction(
		thread, threadID, "manual", at.Add(10*time.Second), false,
		CaseCompactionAuthorization{
			Continuation: continuation, SourceContextDigest: current.ContextDigest,
			AuthorityTurnIDs: authorityTurnIDs,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.CaseBound || prepared.Result.ReplacedTokens <= 0 || prepared.SecurityContext.ContextEpoch != current.ContextEpoch+1 {
		t.Fatalf("authorized case compaction authority mismatch: %#v", prepared)
	}
	nextTurns := prepared.Thread["turns"].([]any)
	last := nextTurns[len(nextTurns)-1].(map[string]any)
	if last["caseHistoryProjection"] != "compaction_authority_v1" || last["id"] != prepared.Result.TurnID {
		t.Fatalf("case compaction turn is not the durable high-water: %#v", last)
	}
	seen := map[string]map[string]any{}
	for _, raw := range nextTurns {
		turn := raw.(map[string]any)
		seen[stringField(turn, "id")] = turn
	}
	for index, frozen := range contexts {
		retained := seen[frozen.TurnID]
		retainedContext, parseErr := domainsecurity.ParseTurnSecurityContext(retained["securityContext"])
		if parseErr != nil || retainedContext != frozen {
			t.Fatalf("signed context %d was not retained exactly: turn=%#v err=%v", index, retained, parseErr)
		}
	}
	if !reflect.DeepEqual(seen[contexts[0].TurnID], turns[0]) {
		t.Fatal("accepted-final primary-CAS turn was rewritten")
	}
	if body := mustCompactionJSONV1(t, prepared.Thread); strings.Contains(body, "DRAFT_CASE_SENTINEL_MUST_DISAPPEAR") {
		t.Fatalf("case draft survived authority-only compaction: %s", body)
	}
	item := last["items"].([]any)[0].(map[string]any)
	parsed, parseErr := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
	if parseErr != nil || parsed.StateDigest != continuation.StateDigest ||
		stringField(item, "sourceContextDigest") != current.ContextDigest {
		t.Fatalf("case continuation was not bound to the source authority: continuation=%#v err=%v", parsed, parseErr)
	}
	binding, bindingErr := turnapp.ParseCaseCompactionOperationBindingV1(item["caseCompactionBinding"])
	bindingDigest, digestErr := turnapp.CaseCompactionOperationDigestV1(binding)
	if bindingErr != nil || digestErr != nil || bindingDigest != prepared.Result.SourceDigest ||
		binding.SourceContextDigest != current.ContextDigest ||
		binding.ContinuationDigest != continuation.StateDigest ||
		!reflect.DeepEqual(binding.AuthorityTurnIDs, canonicalCompactionAuthorityTurnIDs(authorityTurnIDs)) ||
		prepared.EpochState.AcceptedSnapshot.RecoveryDigest != bindingDigest {
		t.Fatalf("case compaction operation was not bound to signed recovery authority: binding=%#v bindingErr=%v digestErr=%v prepared=%#v", binding, bindingErr, digestErr, prepared)
	}
	committed, err := ApplyCompactionCommit(thread, prepared.CommitRequest())
	if err != nil || committed.SecurityContext != prepared.SecurityContext ||
		committed.EpochState.StateDigest != prepared.EpochState.StateDigest {
		t.Fatalf("case compaction CAS replay diverged: committed=%#v err=%v", committed, err)
	}

	tampered := prepared.CommitRequest()
	tampered.CaseAuthorityTurnIDs = tampered.CaseAuthorityTurnIDs[:3]
	if _, err := ApplyCompactionCommit(thread, tampered); err == nil {
		t.Fatal("incomplete case authority inventory passed compaction CAS replay")
	}

	projectionAuthority := caseCompactionProjectionAuthorityStubV1{
		contexts: map[string]domainsecurity.TurnSecurityContext{
			prepared.SecurityContext.ContextDigest: prepared.SecurityContext,
		},
		committed: map[string]casethreadapp.CommittedContext{
			prepared.SecurityContext.ThreadID + "\x00" + prepared.SecurityContext.TurnID: {
				SecurityContext: prepared.SecurityContext,
				EpochState:      prepared.EpochState,
			},
		},
	}
	projected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, last, projectionAuthority)
	projectedItems, _ := projected["items"].([]any)
	projectedMarker, _ := projectedItems[0].(map[string]any)
	if err != nil || !retained || len(projectedItems) != 1 || !validProjectedCaseCompactionItemV1(projectedMarker) {
		t.Fatalf("authorized compaction metadata did not project a safe marker: projected=%#v retained=%t err=%v", projected, retained, err)
	}
	var useNumberTurn map[string]any
	useNumberDecoder := json.NewDecoder(strings.NewReader(mustCompactionJSONV1(t, last)))
	useNumberDecoder.UseNumber()
	if err := useNumberDecoder.Decode(&useNumberTurn); err != nil {
		t.Fatal(err)
	}
	useNumberProjected, useNumberRetained, useNumberErr := projectCaseCompactionRetentionTurnV1(
		threadID, useNumberTurn, projectionAuthority,
	)
	if useNumberErr != nil || !useNumberRetained || !reflect.DeepEqual(useNumberProjected, projected) {
		t.Fatalf("UseNumber replay changed the public compaction marker: projected=%#v retained=%t err=%v", useNumberProjected, useNumberRetained, useNumberErr)
	}
	for _, forbidden := range []string{"taskContinuation", "sourceContextDigest", "caseCompactionBinding"} {
		if _, present := projectedMarker[forbidden]; present {
			t.Fatalf("case compaction public marker exposed %s: %#v", forbidden, projectedMarker)
		}
	}
	untrustedPresentation := contracts.CloneMap(last)
	untrustedItem := untrustedPresentation["items"].([]any)[0].(map[string]any)
	untrustedItem["pinnedConstraints"] = []any{"UNTRUSTED_CASE_COMPACTION_PRESENTATION"}
	untrustedItem["sourceItemIds"] = []any{"UNTRUSTED_CASE_COMPACTION_PRESENTATION"}
	untrustedItem["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(untrustedItem)
	untrustedProjected, retained, err := projectCaseCompactionRetentionTurnV1(
		threadID, untrustedPresentation, projectionAuthority,
	)
	if err == nil || retained || untrustedProjected != nil {
		t.Fatalf("untrusted compaction presentation entered public history: projected=%#v retained=%t err=%v", untrustedProjected, retained, err)
	}
	tamperedIdentity := contracts.CloneMap(last)
	tamperedIdentityItem := tamperedIdentity["items"].([]any)[0].(map[string]any)
	tamperedIdentityItem["turnId"] = "turn_detached_compaction"
	tamperedIdentityItem["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(tamperedIdentityItem)
	if projected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, tamperedIdentity, projectionAuthority); err == nil || retained || projected != nil {
		t.Fatalf("detached compaction identity entered public history: projected=%#v retained=%t err=%v", projected, retained, err)
	}
	tamperedTurn := contracts.CloneMap(last)
	tamperedItem := tamperedTurn["items"].([]any)[0].(map[string]any)
	tamperedItem["sourceContextDigest"] = strings.Repeat("f", 64)
	if projected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, tamperedTurn, projectionAuthority); err == nil || retained || projected != nil {
		t.Fatalf("tampered compaction continuation entered public history: projected=%#v retained=%t err=%v", projected, retained, err)
	}
	tamperedSnapshot := contracts.CloneMap(last)
	tamperedSnapshot["contextEpochSnapshot"].(map[string]any)["recoveryDigest"] = strings.Repeat("e", 64)
	if projected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, tamperedSnapshot, projectionAuthority); err == nil || retained || projected != nil {
		t.Fatalf("compaction marker detached from signed recovery digest: projected=%#v retained=%t err=%v", projected, retained, err)
	}
	tamperedMode := contracts.CloneMap(last)
	tamperedModeItem := tamperedMode["items"].([]any)[0].(map[string]any)
	tamperedModeItem["auto"] = true
	tamperedModeItem["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(tamperedModeItem)
	if projected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, tamperedMode, projectionAuthority); err == nil || retained || projected != nil {
		t.Fatalf("compaction mode detached from signed epoch entered public history: projected=%#v retained=%t err=%v", projected, retained, err)
	}
	tamperedCount := contracts.CloneMap(last)
	tamperedCountItem := tamperedCount["items"].([]any)[0].(map[string]any)
	tamperedCountItem["replacedTokens"] = float64(prepared.Result.ReplacedTokens + 1)
	tamperedCountItem["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(tamperedCountItem)
	countProjected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, tamperedCount, projectionAuthority)
	countItems, _ := countProjected["items"].([]any)
	var countMarker map[string]any
	if len(countItems) == 1 {
		countMarker, _ = countItems[0].(map[string]any)
	}
	if err != nil || !retained || len(countItems) != 1 || countMarker["replacedTokens"] != nil ||
		!reflect.DeepEqual(countMarker, projectedMarker) {
		t.Fatalf("unbound compaction count affected the closed public marker: projected=%#v retained=%t err=%v", countProjected, retained, err)
	}
	coherentTamper := contracts.CloneMap(last)
	coherentItem := coherentTamper["items"].([]any)[0].(map[string]any)
	coherentBinding, err := turnapp.ParseCaseCompactionOperationBindingV1(coherentItem["caseCompactionBinding"])
	if err != nil {
		t.Fatal(err)
	}
	coherentBinding.OperationStamp = fmt.Sprintf("%d", at.Add(11*time.Second).UnixNano())
	coherentDigest, err := turnapp.CaseCompactionOperationDigestV1(coherentBinding)
	if err != nil {
		t.Fatal(err)
	}
	coherentItem["caseCompactionBinding"] = turnapp.CaseCompactionOperationBindingMapV1(coherentBinding)
	coherentItem["sourceDigest"] = coherentDigest
	coherentItem["digestMarker"] = "sha256:" + coherentDigest[:12]
	coherentItem["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(coherentItem)
	coherentTamper["contextEpochSnapshot"].(map[string]any)["recoveryDigest"] = coherentDigest
	if projected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, coherentTamper, projectionAuthority); err == nil || retained || projected != nil {
		t.Fatalf("self-consistent compaction detached from installation-signed epoch entered public history: projected=%#v retained=%t err=%v", projected, retained, err)
	}
	tamperedItemID := contracts.CloneMap(last)
	tamperedItemIDItem := tamperedItemID["items"].([]any)[0].(map[string]any)
	tamperedItemIDItem["id"] = "compaction_detached_identity"
	tamperedItemIDItem["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(tamperedItemIDItem)
	if projected, retained, err := projectCaseCompactionRetentionTurnV1(threadID, tamperedItemID, projectionAuthority); err == nil || retained || projected != nil {
		t.Fatalf("compaction item identity detached from signed operation entered public history: projected=%#v retained=%t err=%v", projected, retained, err)
	}

	secondAuthorityTurnIDs := append(append([]string(nil), authorityTurnIDs...), prepared.SecurityContext.TurnID)
	second, err := PrepareCaseCompaction(
		prepared.Thread, threadID, "manual", at.Add(20*time.Second), false,
		CaseCompactionAuthorization{
			Continuation:        caseCompactionContinuationFixtureV1(t, prepared.SecurityContext),
			SourceContextDigest: prepared.SecurityContext.ContextDigest,
			AuthorityTurnIDs:    secondAuthorityTurnIDs,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	secondTurns := second.Thread["turns"].([]any)
	var retainedPrior map[string]any
	for _, raw := range secondTurns {
		candidate := raw.(map[string]any)
		if stringField(candidate, "id") == prepared.SecurityContext.TurnID {
			retainedPrior = candidate
			break
		}
	}
	if !reflect.DeepEqual(retainedPrior, last) {
		t.Fatalf("second compaction did not retain the exact prior signed witness: retained=%#v prior=%#v", retainedPrior, last)
	}
	secondLast := secondTurns[len(secondTurns)-1].(map[string]any)
	secondItem := secondLast["items"].([]any)[0].(map[string]any)
	secondBinding, err := turnapp.ParseCaseCompactionOperationBindingV1(secondItem["caseCompactionBinding"])
	if err != nil || secondBinding.PreviousCompactionSourceDigest != prepared.Result.SourceDigest ||
		second.EpochState.AcceptedSnapshot.RecoveryDigest != second.Result.SourceDigest {
		t.Fatalf("second compaction lost signed ancestry: binding=%#v prepared=%#v err=%v", secondBinding, second, err)
	}
}

func TestPreparedBoundaryOnlyCaseCompactionReusesSignedCaseArchive(t *testing.T) {
	at := time.Date(2026, 8, 2, 2, 0, 0, 0, time.UTC)
	threadID := "thread-boundary-compaction-authorized"
	turns := make([]any, 0, 4)
	contexts := make([]domainsecurity.TurnSecurityContext, 0, 4)
	for index := 1; index <= 4; index++ {
		securityContext, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: fmt.Sprintf("turn-boundary-%d", index), WorkspaceRealPath: "/cases/boundary-compaction",
			ContextEpoch: uint64(index), IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		contexts = append(contexts, securityContext)
		turns = append(turns, map[string]any{
			"id": securityContext.TurnID, "threadId": threadID, "status": "completed",
			"securityContext": turnsecurityapp.PublicRecord(securityContext),
			"items": []any{map[string]any{
				"id": fmt.Sprintf("user-boundary-%d", index), "turnId": securityContext.TurnID,
				"threadId": threadID, "kind": "user_message", "role": "user", "status": "completed",
				"text": "continue ordinary work after the protected boundary",
			}},
		})
	}
	current := contexts[len(contexts)-1]
	state, err := contextepochapp.BootstrapState(
		threadID, current.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": threadID, "workspace": current.WorkspaceRealPath, "status": "idle", "turns": turns,
		"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(state),
	}
	continuation, err := turnapp.BuildTaskContinuationSnapshotV1(thread)
	if err != nil {
		t.Fatal(err)
	}
	authorityTurnIDs := make([]string, 0, len(contexts))
	for _, securityContext := range contexts {
		authorityTurnIDs = append(authorityTurnIDs, securityContext.TurnID)
	}
	prepared, err := PrepareCaseCompaction(
		thread, threadID, "manual", at.Add(time.Minute), false,
		CaseCompactionAuthorization{
			Continuation: continuation, SourceContextDigest: current.ContextDigest,
			AuthorityTurnIDs: authorityTurnIDs,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.CaseBound || prepared.Result.ReplacedTokens <= 0 ||
		!domainsecurity.TurnSecurityContextIsBoundaryOnly(prepared.SecurityContext) ||
		prepared.SecurityContext.PublicationPolicy != current.PublicationPolicy ||
		prepared.SecurityContext.ContextEpoch != current.ContextEpoch+1 {
		t.Fatalf("boundary compaction did not preserve the signed boundary: %#v", prepared)
	}
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(prepared.SecurityContext) == nil {
		t.Fatal("host-only boundary compaction upgraded a quarantined context into executable authority")
	}
	committed, err := ApplyCompactionCommit(thread, prepared.CommitRequest())
	if err != nil || committed.SecurityContext != prepared.SecurityContext ||
		committed.EpochState.StateDigest != prepared.EpochState.StateDigest {
		t.Fatalf("boundary compaction CAS replay diverged: committed=%#v err=%v", committed, err)
	}
}

type caseCompactionProjectionAuthorityStubV1 struct {
	contexts  map[string]domainsecurity.TurnSecurityContext
	committed map[string]casethreadapp.CommittedContext
}

func (stub caseCompactionProjectionAuthorityStubV1) IsCaseThread(threadID string) bool {
	for _, securityContext := range stub.contexts {
		if securityContext.ThreadID == threadID {
			return true
		}
	}
	return false
}

func (stub caseCompactionProjectionAuthorityStubV1) ContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	return stub.contexts[securityContext.ContextDigest] == securityContext
}

func (stub caseCompactionProjectionAuthorityStubV1) CommittedContext(
	threadID string,
	turnID string,
) (casethreadapp.CommittedContext, bool) {
	committed, found := stub.committed[threadID+"\x00"+turnID]
	return committed, found
}

func caseCompactionContinuationFixtureV1(
	t *testing.T,
	_ domainsecurity.TurnSecurityContext,
) threaddomain.TaskContinuationSnapshotV1 {
	t.Helper()
	continuation, err := threaddomain.SealTaskContinuationSnapshotV1(threaddomain.TaskContinuationSnapshotV1{
		SchemaVersion: threaddomain.TaskContinuationSchemaVersionV1,
		Todos:         []threaddomain.TaskContinuationTodoV1{}, LatestUserConstraints: []string{"继续当前案件核验"},
		EvidenceReferences: []threaddomain.TaskContinuationEvidenceReferenceV1{},
		EvidenceAuthority:  threaddomain.TaskContinuationEvidenceStateV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return continuation
}

func mustCompactionJSONV1(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func (caseCompactionProjectionAuthorityStubV1) RestartPreservesThreadV1(string) bool { return false }
