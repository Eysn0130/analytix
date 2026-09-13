package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func runtimeRestorePreservedReportFixture(t *testing.T) (*runtimeServerHandler, string, string) {
	t.Helper()
	handler, threadID, gate, _ := installGateInventoryCrashCut(t, false, false)
	thread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	frozen := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_914", WorkspaceRealPath: stringField(thread, "workspace"),
		ContextEpoch: gate.SecurityContext.ContextEpoch + 1, IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(frozen)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": frozen.TurnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "securityContext": securityRecord, "items": []any{},
	}, "provider-a", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"report":"synthetic"}`)
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("preserved_report"), Name: pendingworkapp.ReportStageToolName, Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: frozen, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: call.Name,
		ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("preserved-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("preserved-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	})
	item, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{
		ThreadID: threadID, TurnID: frozen.TurnID, ItemID: domaintoolcall.ToolCallItemIDV1(frozen.TurnID, call.ID),
		CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "report", Context: frozen, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.store.AppendItemToTurn(threadID, frozen.TurnID, item); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.pendingWork.BeginReportStage(context.Background(), pendingworkapp.ReportStageRequest{
		PendingToolCall: appmodel.PendingToolCall{ThreadID: threadID, TurnID: frozen.TurnID, ProviderID: "provider-a", Call: call, SecurityContext: frozen, ExecutionGrant: grant},
		StageInputHash:  domainsecurity.SHA256Hex([]byte("preserved-report-input")), IssuedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return handler, threadID, gate.GateID
}

func runtimeRestoreFileDigestsV1(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = domainsecurity.SHA256Hex(body)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRuntimeRestorePreservesReportGraphBeforeGenericRecovery(t *testing.T) {
	for _, persistedUnknown := range []bool{false, true} {
		name := "original_open"
		if persistedUnknown {
			name = "original_unknown"
		}
		t.Run(name, func(t *testing.T) {
			handler, threadID, gateID := runtimeRestorePreservedReportFixture(t)
			if persistedUnknown {
				if _, err := handler.pendingWork.CloseAllOpenOnRestart(context.Background(), time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
			}
			reader, err := finalauthorityadapter.NewAcceptedFinalCASReader(handler.store.root)
			if err != nil {
				t.Fatal(err)
			}
			scope, err := handler.pendingWork.PlanReportRestartPreservationV1(context.Background(), reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := handler.pendingWork.PreserveReportRestartScopeV1(context.Background(), scope); err != nil {
				t.Fatal(err)
			}
			if len(scope.Contexts()) != 2 {
				t.Fatal("thread hold omitted an original frozen context")
			}
			beforeDurable := runtimeRestoreFileDigestsV1(t, handler.store.root)
			beforeData := runtimeRestoreFileDigestsV1(t, handler.dataDir)
			for run := 0; run < 2; run++ {
				if err := handler.restoreRuntimeState(); err != nil {
					t.Fatalf("preserved restore %d failed: %v", run, err)
				}
				if !reflect.DeepEqual(beforeDurable, runtimeRestoreFileDigestsV1(t, handler.store.root)) || !reflect.DeepEqual(beforeData, runtimeRestoreFileDigestsV1(t, handler.dataDir)) {
					t.Fatal("generic restart changed preserved primary/events/epoch/grant/gate or private authority bytes")
				}
			}
			if _, err := handler.continuations.VerifyOpen(context.Background(), gateID, time.Now().UTC()); err != nil {
				t.Fatalf("held gate was disposed: %v", err)
			}
			if !scope.OwnsThread(threadID) || handler.turnSeq < 914 {
				t.Fatal("held primary turn identity was lost from runtime sequence allocation")
			}
		})
	}
}

func TestRuntimeRestorePreservesReportWhileIndependentOrdinaryStateRecovers(t *testing.T) {
	handler, heldThreadID, _ := runtimeRestorePreservedReportFixture(t)
	reader, err := finalauthorityadapter.NewAcceptedFinalCASReader(handler.store.root)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := handler.pendingWork.PlanReportRestartPreservationV1(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.pendingWork.PreserveReportRestartScopeV1(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	ordinary, err := handler.store.CreateThread(map[string]any{"title": "independent ordinary recovery"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	ordinaryID := stringField(ordinary, "id")
	if scope.OwnsThread(ordinaryID) || ordinary["contextEpochState"] != nil {
		t.Fatal("ordinary control was already held or recovered")
	}
	beforeHeld := runtimeRestoreFileDigestsV1(t, handler.store.threadDir(heldThreadID))
	beforeData := runtimeRestoreFileDigestsV1(t, handler.dataDir)
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatal(err)
	}
	ordinary, err = handler.store.GetThread(ordinaryID)
	if err != nil || ordinary["contextEpochState"] == nil {
		t.Fatalf("independent ordinary recovery did not progress: %v", err)
	}
	if !reflect.DeepEqual(beforeHeld, runtimeRestoreFileDigestsV1(t, handler.store.threadDir(heldThreadID))) || !reflect.DeepEqual(beforeData, runtimeRestoreFileDigestsV1(t, handler.dataDir)) {
		t.Fatal("independent recovery changed held original graph")
	}
}
