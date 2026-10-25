package thread

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	controlapp "analytix.local/runtime-go/internal/app/control"
	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

func projectedCompactionHistoryForTest(ctx context.Context, thread map[string]any) ([]domainmodel.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil {
		return nil, err
	}
	request, err := privacyprojectionapp.ProjectProviderRequestForEffect(current, domainmodel.Request{Messages: appmodel.ProviderHistoryFromThread(thread)}, true)
	return request.Messages, err
}

func summaryForCompactionTest(context.Context, OrdinarySummaryInput) (string, error) {
	return "The user requested continuing the current task.", nil
}

func TestOrdinarySummaryFailureDoesNotReplacePrimaryHistory(t *testing.T) {
	for _, fault := range []string{"empty", "nonshrink", "provider", "cancel", "baseline", "revoked"} {
		t.Run(fault, func(t *testing.T) {
			repo, current := ordinarySummaryRepositoryForTest(t)
			before, _ := CompactionBaselineDigest(repo.thread)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controller := controlapp.NewController(nil)
			witness, release, err := controller.PrepareForegroundWithWitness(ctx, current.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			service := NewService(Dependencies{Repository: repo, WorkspaceReader: mutationWorkspaceReaderStub{}, ProjectCompactionHistory: projectedCompactionHistoryForTest,
				SummarizeCompaction: func(context.Context, OrdinarySummaryInput) (string, error) {
					switch fault {
					case "empty":
						return " ", nil
					case "nonshrink":
						return strings.Repeat("large summary ", 900), nil
					case "provider":
						return "", errors.New("synthetic provider failure")
					case "cancel":
						cancel()
					case "baseline":
						repo.thread["title"] = "concurrent change"
					case "revoked":
						release()
					}
					return "Short summary.", nil
				}, BeginTransition: func(_ context.Context, target domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
					return &compactionTransitionStub{target: target}, nil
				},
			})
			response, err := service.compact(ctx, current.ThreadID, "manual", fault == "revoked", witness)
			if err == nil || response != nil || repo.compactionCommits != 0 {
				t.Fatalf("failure replaced history: response=%#v err=%v commits=%d", response, err, repo.compactionCommits)
			}
			if fault != "baseline" {
				after, _ := CompactionBaselineDigest(repo.thread)
				if after != before {
					t.Fatal("failed summary mutated primary")
				}
			}
			if len(repo.events) != 2 || repo.events[0]["kind"] != "compaction_started" || repo.events[1]["kind"] != "compaction_failed" || repo.events[0]["compactionId"] != repo.events[1]["compactionId"] {
				t.Fatalf("failure lifecycle lost identity: %#v", repo.events)
			}
		})
	}
}

func TestOrdinarySummaryFactsCommitAndProjectionAreExact(t *testing.T) {
	repo, current := ordinarySummaryRepositoryForTest(t)
	service := NewService(Dependencies{Repository: repo, WorkspaceReader: mutationWorkspaceReaderStub{},
		SummarizeCompaction: summaryForCompactionTest, ProjectCompactionHistory: projectedCompactionHistoryForTest,
		BeginTransition: func(_ context.Context, target domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
			return &compactionTransitionStub{target: target}, nil
		},
	})
	response, err := service.Compact(context.Background(), current.ThreadID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	turn := listAny(repo.thread["turns"])[0].(map[string]any)
	item := listAny(turn["items"])[0].(map[string]any)
	if !domainevent.ValidGeneralCompactionProviderHistoryItemV5(item) || response["compactionId"] != item["id"] || response["shadowedItemCount"] != float64(2) || response["tokenCountKind"] != "estimated" {
		t.Fatalf("summary facts missing: %#v", response)
	}
	if turn["contextEpochSnapshot"].(map[string]any)["recoveryDigest"] != item["checkpointDigest"] {
		t.Fatal("epoch did not bind summary checkpoint")
	}
	projected, visible, err := ProjectPublicThreadEvent(current.ThreadID, repo.thread, repo.events[1])
	if err != nil || !visible || projected["summary"] != item["summary"] || projected["compactionId"] != item["id"] {
		t.Fatalf("summary projection mismatch: %#v visible=%t err=%v", projected, visible, err)
	}
	item["summary"] = "tampered summary"
	if domainevent.ValidGeneralCompactionProviderHistoryItemV5(item) {
		t.Fatal("tampered summary admitted")
	}
}

func ordinarySummaryRepositoryForTest(t *testing.T) (*repositoryStub, domainsecurity.TurnSecurityContext) {
	t.Helper()
	// Reuse the exact admitted general fixture and epoch owner used by the
	// automatic-compaction tests; source text is large enough to test shrink.
	current := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: "thr_summary", TurnID: "turn_4", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Now().UTC()})
	return ordinarySummaryRepositoryWithContextForTest(t, current), current
}

func ordinarySummaryRepositoryWithContextForTest(t *testing.T, current domainsecurity.TurnSecurityContext) *repositoryStub {
	t.Helper()
	at := time.Now().UTC()
	epoch, err := contextepochapp.BootstrapState(current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at)
	if err != nil {
		t.Fatal(err)
	}
	turns := []any{}
	for index := 1; index <= 4; index++ {
		turnID := "turn_" + string(rune('0'+index))
		frozen := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: current.ThreadID, TurnID: turnID, WorkspaceRealPath: current.WorkspaceRealPath, ContextEpoch: current.ContextEpoch, IssuedAt: at})
		if index == 4 {
			frozen = current
		}
		turns = append(turns, map[string]any{"id": turnID, "threadId": current.ThreadID, "status": "completed", "securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{"id": "item_" + turnID, "kind": "user_message", "text": strings.Repeat("Original safe task detail. ", 200)}}})
	}
	return &repositoryStub{thread: map[string]any{"id": current.ThreadID, "workspace": current.WorkspaceRealPath, "status": "idle", "turns": turns,
		"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(epoch)}}
}

func TestOrdinarySummaryNoHistoryNeedsNoProducerAuthority(t *testing.T) {
	repo := &repositoryStub{thread: map[string]any{"id": "thr_empty_summary", "status": "idle", "turns": []any{}}}
	service := NewService(Dependencies{Repository: repo})
	response, err := service.Compact(context.Background(), "thr_empty_summary", "automatic_context_threshold")
	if err != nil || response["ok"] != true || response["replacedTokens"] != float64(0) || repo.compactionCommits != 0 || len(repo.events) != 0 {
		t.Fatalf("empty history acquired a summary effect: response=%#v err=%v commits=%d events=%d", response, err, repo.compactionCommits, len(repo.events))
	}
}

func TestOrdinarySummaryAutomaticContinuationSourcesReadbackAndBarriers(t *testing.T) {
	repo, current := ordinarySummaryRepositoryForTest(t)
	// The unchanged continuation contract preserves recent user constraints.
	// Use a genuine large-source fixture so removing history exceeds that
	// existing bounded overhead; the production shrink guard stays unchanged.
	for _, raw := range listAny(repo.thread["turns"]) {
		turn := raw.(map[string]any)
		listAny(turn["items"])[0].(map[string]any)["text"] = strings.Repeat("Original safe task detail. ", 1000)
	}
	controller := controlapp.NewController(nil)
	service := NewService(Dependencies{Repository: repo, WorkspaceReader: mutationWorkspaceReaderStub{}, SummarizeCompaction: summaryForCompactionTest,
		ProjectCompactionHistory: projectedCompactionHistoryForTest, BeginTransition: func(_ context.Context, target domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
			return &compactionTransitionStub{target: target}, nil
		}})
	compact := func() {
		witness, release, err := controller.PrepareForegroundWithWitness(context.Background(), current.ThreadID)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if _, err := service.compact(context.Background(), current.ThreadID, "automatic_context_threshold", true, witness); err != nil {
			t.Fatal(err)
		}
	}
	compact()
	marker := listAny(listAny(repo.thread["turns"])[0].(map[string]any)["items"])[0].(map[string]any)
	snapshot, err := domainthread.ParseTaskContinuationSnapshotV1(marker["taskContinuation"])
	if err != nil || snapshot.UserHistory == nil || len(snapshot.UserHistory.Sources) != 4 || !appmodel.ContinuationSourceReferencesRequiredV1(repo.thread) {
		t.Fatalf("V5 source snapshot unusable: err=%v", err)
	}
	ref := snapshot.UserHistory.Sources[0].Reference
	current, err = domainsecurity.ParseTurnSecurityContext(repo.thread["securityState"])
	if err != nil {
		t.Fatal(err)
	}
	if page, err := turnapp.ReadTaskContinuationSourceV1(repo.thread, current, ref, 0, 80); err != nil || !strings.HasPrefix(page["text"].(string), "Original safe task") {
		t.Fatalf("source after first summary unavailable: err=%v", err)
	}
	for index := 5; index <= 6; index++ {
		turnID := "turn_" + string(rune('0'+index))
		current, err = turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{Context: context.Background(),
			Authority: turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: mutationWorkspaceReaderStub{}, RiskAuthority: &mutationServiceRiskAuthority{mutationRiskAuthority: newMutationRiskAuthority()}},
			Thread:    repo.thread, ThreadID: current.ThreadID, TurnID: turnID, Workspace: current.WorkspaceRealPath, Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		preparedStart, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{Thread: repo.thread, SecurityContext: current, At: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		current = preparedStart.SecurityContext
		repo.thread["contextEpochState"] = contextepochapp.PublicState(preparedStart.State)
		turn := map[string]any{"id": turnID, "threadId": current.ThreadID, "status": "completed", "securityContext": turnsecurityapp.PublicRecord(current), "items": []any{map[string]any{"id": "item_" + turnID, "kind": "user_message", "text": strings.Repeat("The latest correction remains part of the task. ", 200)}}}
		repo.thread["turns"] = append(listAny(repo.thread["turns"]), turn)
		repo.thread["securityState"] = turnsecurityapp.PublicRecord(current)
	}
	compact()
	marker = listAny(listAny(repo.thread["turns"])[0].(map[string]any)["items"])[0].(map[string]any)
	snapshot, err = domainthread.ParseTaskContinuationSnapshotV1(marker["taskContinuation"])
	if err != nil || snapshot.UserHistory == nil || len(snapshot.UserHistory.Sources) != 6 {
		t.Fatalf("repeated V5 lost original source ancestry: err=%v", err)
	}
	current, _ = domainsecurity.ParseTurnSecurityContext(repo.thread["securityState"])
	if _, err := turnapp.ReadTaskContinuationSourceV1(repo.thread, current, ref, 0, 80); err != nil {
		t.Fatal("original source after repeated summary unavailable", err)
	}
	// A latest V5 marker owns source ancestry. A stale or manual marker must
	// stop fallback to an older otherwise valid automatic snapshot.
	older := contracts.CloneValue(listAny(repo.thread["turns"])[0])
	for _, fault := range []string{"manual", "stale", "invalid"} {
		t.Run(fault, func(t *testing.T) {
			copy := contracts.CloneMap(repo.thread)
			latest := contracts.CloneMap(older.(map[string]any))
			copy["turns"] = []any{older, latest}
			item := listAny(latest["items"])[0].(map[string]any)
			switch fault {
			case "manual":
				item["auto"] = false
				delete(item, "taskContinuation")
				item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
			case "stale":
				delete(latest, "contextEpochSnapshot")
			case "invalid":
				item["summary"] = "tampered"
			}
			if appmodel.ContinuationSourceReferencesRequiredV1(copy) {
				t.Fatal("latest marker resurrected older reader route")
			}
			if _, err := turnapp.ReadTaskContinuationSourceV1(copy, current, ref, 0, 80); err == nil {
				t.Fatal("latest marker resurrected older source")
			}
		})
	}
}
