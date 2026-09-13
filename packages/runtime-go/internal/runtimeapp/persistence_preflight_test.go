package runtimeapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	continuationstore "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	cachetelemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
	"analytix.local/runtime-go/internal/server"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestStartupCorruptSnapshotProducesZeroDataMutation(t *testing.T) {
	durableRoot := t.TempDir()
	dataDir := t.TempDir()
	threadDir := filepath.Join(durableRoot, "threads", "thr_corrupt")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(threadDir, "thread.json")
	corrupt := []byte(`{"id":"thr_corrupt","nested":{"claim":1,"claim":2}}`)
	if err := os.WriteFile(corruptPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(dataDir, "startup-sentinel")
	sentinel := []byte("unchanged")
	if err := os.WriteFile(sentinelPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	beforeCorrupt := sha256.Sum256(corrupt)
	beforeSentinel := sha256.Sum256(sentinel)

	_, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: durableRoot,
		DataDir:        dataDir,
	})
	var integrityErr persistencefs.IntegrityError
	if !errors.As(err, &integrityErr) || integrityErr.Code != "invalid_json" {
		t.Fatalf("expected strict startup integrity failure, got %v", err)
	}
	afterCorrupt, readErr := os.ReadFile(corruptPath)
	if readErr != nil || sha256.Sum256(afterCorrupt) != beforeCorrupt {
		t.Fatalf("corrupt source changed before startup rejection: err=%v", readErr)
	}
	afterSentinel, readErr := os.ReadFile(sentinelPath)
	if readErr != nil || sha256.Sum256(afterSentinel) != beforeSentinel {
		t.Fatalf("data sentinel changed before startup rejection: err=%v", readErr)
	}
	for _, forbidden := range []string{
		filepath.Join(durableRoot, "durable-meta.json"),
		filepath.Join(dataDir, "private"),
		filepath.Join(dataDir, "memory"),
		filepath.Join(dataDir, "child-runs"),
	} {
		if _, statErr := os.Lstat(forbidden); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("startup integrity rejection mutated %s: %v", forbidden, statErr)
		}
	}
}

func TestStartupCorruptManagedDataBlocksLiveCheckpointTerminalWrite(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := t.TempDir()
	target := filepath.Join(workspace, "account-evidence.txt")
	writeOperationFixture(t, target, "before")
	checkpointRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	access, err := privatecastest.NewAccessAuthority(checkpointRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := checkpointauthority.NewStoreContext(context.Background(), checkpointRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	authority := checkpointapp.SnapshotAuthority{Store: store}
	service := checkpointapp.OperationService{Authority: authority, Observer: filestore.CheckpointOperationObserver{}}
	now := time.Unix(1_700_900_000, 0).UTC()
	arguments := []byte(`{"path":"account-evidence.txt","content":"after"}`)
	securityContext, grant := operationRecoverySecurity(t, now, workspace, "write_file", arguments)
	if _, err := service.Begin(context.Background(), checkpointapp.BeginOperationInput{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("corrupt-managed-preflight"), SourceWorkspaceCheckpointID: "corrupt-managed-preflight",
		Workspace: workspace, ToolName: "write_file", ArgumentsJSON: arguments,
		Paths: []checkpointapp.OperationPathRequest{{
			ResolvedPath: target, ArgumentKey: "path", RequestedPath: "account-evidence.txt", Role: "target",
			ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash("after"),
		}},
		CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	threadDir := filepath.Join(durableRoot, "threads", "thr_corrupt_before_checkpoint_recovery")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(threadDir, "thread.json"),
		[]byte(`{"id":"thr_corrupt_before_checkpoint_recovery","nested":{"claim":1,"claim":2}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeDigest(t, dataDir, durableRoot)
	_, err = NewRuntimeServerHandlerE(Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot,
	})
	var integrityErr persistencefs.IntegrityError
	if !errors.As(err, &integrityErr) || integrityErr.Code != "invalid_json" {
		t.Fatalf("expected strict managed preflight failure, got %v", err)
	}
	if after := startupWholeTreeDigest(t, dataDir, durableRoot); after != before {
		t.Fatalf("corrupt managed preflight allowed a live terminal write: before=%s after=%s", before, after)
	}
	open, err := authority.OpenOperationGroups(context.Background())
	if err != nil || len(open) != 1 {
		t.Fatalf("corrupt managed preflight settled the open operation: open=%#v err=%v", open, err)
	}
}

func TestColdPersistenceRootsRemainValidWithNoCreateRecoveryProbe(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "cold-data")
	durableRoot := filepath.Join(base, "cold-durable")
	handler, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot,
	})
	if err != nil {
		t.Fatalf("valid cold persistence roots failed startup: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	for _, root := range []string{dataDir, durableRoot} {
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() {
			t.Fatalf("cold persistence root was not promoted by semantic startup: root=%s err=%v", root, err)
		}
	}
}

func TestStartupDoesNotRecreateMissingAuthorityKeyForAnyEvidenceRegistryState(t *testing.T) {
	for _, testCase := range []struct {
		name string
		seed func(*testing.T, string)
	}{
		{name: "empty-root", seed: func(t *testing.T, root string) { t.Helper(); mustMkdirAllRuntimePreflightV2(t, root) }},
		{name: "indexes-only", seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirAllRuntimePreflightV2(t, filepath.Join(root, "indexes"))
		}},
		{name: "capsules-only", seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirAllRuntimePreflightV2(t, filepath.Join(root, "capsules"))
		}},
		{name: "empty-pair", seed: func(t *testing.T, root string) {
			t.Helper()
			mustMkdirAllRuntimePreflightV2(t, filepath.Join(root, "indexes"))
			mustMkdirAllRuntimePreflightV2(t, filepath.Join(root, "capsules"))
		}},
		{name: "zero-lock", seed: func(t *testing.T, root string) {
			t.Helper()
			mustWriteRuntimePreflightV2(t, filepath.Join(root, ".registry.lock"), nil)
		}},
		{name: "nonempty-lock", seed: func(t *testing.T, root string) {
			t.Helper()
			mustWriteRuntimePreflightV2(t, filepath.Join(root, ".registry.lock"), []byte("held"))
		}},
		{name: "empty-unknown-safe-residue", seed: func(t *testing.T, root string) {
			t.Helper()
			mustWriteRuntimePreflightV2(t, filepath.Join(root, ".registry-authority-index-empty.tmp"), nil)
		}},
		{name: "nonempty-unknown-safe-residue", seed: func(t *testing.T, root string) {
			t.Helper()
			mustWriteRuntimePreflightV2(t, filepath.Join(root, ".registry-authority-index-deadbeef.tmp"), []byte("frozen"))
		}},
		{name: "nested-legacy-residue", seed: func(t *testing.T, root string) {
			t.Helper()
			mustWriteRuntimePreflightV2(t, filepath.Join(root, "thread-a", "turn-a.jsonl"), nil)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dataDir := t.TempDir()
			durableRoot := t.TempDir()
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot}
			initial, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatalf("create initial installation authority: %v", err)
			}
			shutdownOwnedRuntimeHandler(t, initial)
			authorityPath := filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json")
			registryRoot := filepath.Join(dataDir, "private", "evidence-registry")
			testCase.seed(t, registryRoot)
			if err := os.Remove(authorityPath); err != nil {
				t.Fatal(err)
			}
			if _, err := NewRuntimeServerHandlerE(config); err == nil ||
				!strings.Contains(err.Error(), "authority key is missing while authority records exist") {
				t.Fatalf("registry state did not retain privateAuthorityRequired.EvidenceRegistry: %v", err)
			}
			if _, err := os.Lstat(authorityPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("startup recreated an authority key over registry state: %v", err)
			}
		})
	}
}

func mustMkdirAllRuntimePreflightV2(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWriteRuntimePreflightV2(t *testing.T, path string, body []byte) {
	t.Helper()
	mustMkdirAllRuntimePreflightV2(t, filepath.Dir(path))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStartupDoesNotRecreateMissingAuthorityKeyWhenPendingSteeringExists(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	authorityPath := filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json")
	authority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	store, err := server.NewProductionDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	store.SetSteeringAuthority(steeringauthorityapp.NewService(authority))
	thread, err := store.CreateThread(map[string]any{"title": "pending steering authority", "workspace": "/workspace"}, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	turnID := "turn-pending-steering-authority"
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Unix(1_700_000_000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start",
		"steering": []any{}, "items": []any{}, "createdAt": securityContext.IssuedAt, "startedAt": securityContext.IssuedAt,
		"securityContext": record,
	}, "", map[string]any{"securityState": record}); err != nil {
		t.Fatal(err)
	}
	clientID := "client-pending-steering-authority"
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, map[string]any{
		"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
		"text": "signed pending guidance", "admittedAt": securityContext.IssuedAt, "delivery": "steer",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(authorityPath); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntimeServerHandlerE(config); err == nil || !strings.Contains(err.Error(), "authority key is missing while authority records exist") {
		t.Fatalf("pending steering authority state did not prevent key recreation: %v", err)
	}
	if _, err := os.Lstat(authorityPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup recreated an authority key over pending steering state: %v", err)
	}
}

func TestStartupRejectsForeignContinuationAndPendingWorkAuthority(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
		seed func(*testing.T, string, *finalauthority.FileAuthority)
	}{
		{
			name: "continuation",
			want: "continuation receipt lacks current installation authority",
			seed: func(t *testing.T, dataDir string, foreign *finalauthority.FileAuthority) {
				t.Helper()
				root := filepath.Join(dataDir, "private", "gate-continuations")
				access, err := privatecastest.NewAccessAuthority(root)
				if err != nil {
					t.Fatal(err)
				}
				store, err := continuationstore.NewStore(root, access)
				if err != nil {
					t.Fatal(err)
				}
				receipt := foreignContinuationReceipt(t, foreign)
				if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "pending-work",
			want: "pending work receipt lacks current installation authority",
			seed: func(t *testing.T, dataDir string, foreign *finalauthority.FileAuthority) {
				t.Helper()
				root := filepath.Join(dataDir, "private", "pending-work")
				access, err := privatecastest.NewAccessAuthority(root)
				if err != nil {
					t.Fatal(err)
				}
				store, err := pendingworkstore.NewStore(root, access)
				if err != nil {
					t.Fatal(err)
				}
				receipt := foreignPendingWorkReceipt(t, foreign)
				if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			durableRoot := t.TempDir()
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot}
			initial, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatalf("create current installation: %v", err)
			}
			shutdownOwnedRuntimeHandler(t, initial)
			foreign, err := finalauthority.OpenOrCreateFileAuthority(
				filepath.Join(t.TempDir(), "authority", "foreign.json"), false,
			)
			if err != nil {
				t.Fatal(err)
			}
			test.seed(t, dataDir, foreign)

			if _, err := NewRuntimeServerHandlerE(config); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("foreign private authority was not rejected before activation: %v", err)
			}
		})
	}
}

func TestStartupMigratesLegacyContinuationAuthorityThroughSemanticJournal(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, initial)

	authority, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json"), true,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt := foreignContinuationReceipt(t, authority)
	disposition, err := domaincontinuation.NewDisposition(
		receipt, domaincontinuation.StatusDenied, "approval_denied",
		time.Date(2026, 7, 16, 11, 2, 0, 0, time.UTC),
		authority.KeyID(), authority.PublicKey(), func(message []byte) ([]byte, error) {
			return authority.Sign(context.Background(), message)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dataDir, "private", "gate-continuations")
	for _, partition := range []string{"receipts-v2", "dispositions-v2"} {
		if err := os.RemoveAll(filepath.Join(root, partition)); err != nil {
			t.Fatal(err)
		}
	}
	legacyReceiptPath := writeRuntimeLegacyContinuationRecord(t, root, "receipts", receipt.Payload.GateID, "receipt.json", mustContinuationReceiptBytes(t, receipt))
	legacyDispositionPath := writeRuntimeLegacyContinuationRecord(t, root, "dispositions", receipt.Payload.GateID, "disposition.json", mustContinuationDispositionBytes(t, disposition))
	legacyReceiptBefore, err := os.ReadFile(legacyReceiptPath)
	if err != nil || !bytes.Contains(legacyReceiptBefore, []byte("6222020202020202020")) {
		t.Fatalf("legacy controlled continuation fixture lost exact private account: %v", err)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("semantic startup did not publish continuation migration: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	for _, path := range []string{legacyReceiptPath, legacyDispositionPath, filepath.Join(root, "receipts"), filepath.Join(root, "dispositions")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy continuation path survived journal publication: %s err=%v", path, err)
		}
	}
	digest := strings.TrimPrefix(strings.TrimPrefix(receipt.Payload.GateID, "appr_"), "input_")
	v2ReceiptPath := filepath.Join(root, "receipts-v2", digest[:2], digest+".json")
	v2DispositionPath := filepath.Join(root, "dispositions-v2", digest[:2], digest+".json")
	v2Receipt, err := os.ReadFile(v2ReceiptPath)
	if err != nil || !bytes.Equal(v2Receipt, mustContinuationReceiptBytes(t, receipt)) || !bytes.Contains(v2Receipt, []byte("6222020202020202020")) {
		t.Fatalf("journal-published continuation receipt changed controlled authority bytes: %v", err)
	}
	v2Disposition, err := os.ReadFile(v2DispositionPath)
	if err != nil || !bytes.Equal(v2Disposition, mustContinuationDispositionBytes(t, disposition)) {
		t.Fatalf("journal-published continuation disposition changed authority bytes: %v", err)
	}

	stableRestart, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("migrated continuation authority was not restart-stable: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, stableRestart)
}

func writeRuntimeLegacyContinuationRecord(
	t *testing.T,
	root string,
	partition string,
	gateID string,
	name string,
	body []byte,
) string {
	t.Helper()
	digest := strings.TrimPrefix(strings.TrimPrefix(gateID, "appr_"), "input_")
	path := filepath.Join(root, partition, digest[:2], gateID, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for current := filepath.Dir(path); current != filepath.Dir(root); current = filepath.Dir(current) {
		if err := os.Chmod(current, 0o700); err != nil {
			t.Fatal(err)
		}
		if current == root {
			break
		}
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustContinuationReceiptBytes(t *testing.T, receipt domaincontinuation.Receipt) []byte {
	t.Helper()
	body, err := domaincontinuation.ReceiptBytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustContinuationDispositionBytes(t *testing.T, disposition domaincontinuation.Disposition) []byte {
	t.Helper()
	body, err := domaincontinuation.DispositionBytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func foreignContinuationReceipt(t *testing.T, authority *finalauthority.FileAuthority) domaincontinuation.Receipt {
	t.Helper()
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-foreign-continuation", TurnID: "turn-foreign-continuation", WorkspaceRealPath: "/workspace",
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"path":"case-a.txt","account":"6222020202020202020"}`)
	toolScope := []string{"write_file"}
	toolScopeBody, _ := json.Marshal(toolScope)
	entropy := sha256.Sum256([]byte("runtimeapp-foreign-continuation-tool-call"))
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-foreign", ServerIdentity: "host:builtin", ToolName: "write_file", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(toolScopeBody), ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	receipt, err := domaincontinuation.NewReceipt(domaincontinuation.Payload{
		Kind: domaincontinuation.KindApproval, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		CallID: grant.ToolCallID, ToolName: grant.ToolName,
		ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, grant.ToolCallID), Arguments: arguments,
		ProviderID: grant.Provider, Model: "model-foreign", ProviderRouteHash: domainsecurity.SHA256Hex([]byte("route")),
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", ToolScope: toolScope,
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		ProviderStepPromptSHA256: domaincontinuation.CanonicalProviderStepPromptSHA256("continue exact provider step"),
		ProviderNamespace:        domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		SecurityContext:          securityContext, ExecutionGrant: grant, IssuedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	}, authority.KeyID(), authority.PublicKey(), func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func foreignPendingWorkReceipt(t *testing.T, authority *finalauthority.FileAuthority) domainpendingwork.PendingWorkReceiptV1 {
	t.Helper()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-foreign-pending", TurnID: "turn-foreign-pending", WorkspaceRealPath: "/workspace",
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindToolBatch, SecurityContext: securityContext, GrantRegistrySequence: 1,
		GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("registry")), GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("grant")), RegistrySequence: 1,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("payload")), RouteHash: domainsecurity.SHA256Hex([]byte("route")),
		IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestSemanticStartupLateAuthorityFailureLeavesLegacyAndManagedBytesUntouched(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("create initial runtime state: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	legacyThreadID := "thr_semantic_late_failure"
	legacyThreadDir := filepath.Join(durableRoot, "runtime-go", "threads", legacyThreadID)
	if err := os.MkdirAll(legacyThreadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyThread := []byte(`{"id":"thr_semantic_late_failure","title":"quarantined legacy","workspace":"/workspace","model":"deepseek-chat","mode":"agent","status":"idle","approvalPolicy":"on-request","sandboxMode":"workspace-write","relation":"primary","createdAt":"2026-07-11T00:00:00Z","updatedAt":"2026-07-11T00:00:00Z","turns":[]}`)
	legacyPath := filepath.Join(legacyThreadDir, "thread.json")
	if err := os.WriteFile(legacyPath, legacyThread, 0o600); err != nil {
		t.Fatal(err)
	}
	authorityPath := filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json")
	authority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	seedRuntimeLegacyEvidenceRegistryV1(
		t, filepath.Join(dataDir, "private", "evidence-registry"), authority,
	)
	if err := os.Remove(authorityPath); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(dataDir, durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	before, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	beforeAuthorityWholeTree := startupWholeTreeDigest(t, dataDir, durableRoot)

	if _, err := NewRuntimeServerHandlerE(config); err == nil || !strings.Contains(err.Error(), "authority key is missing while authority records exist") {
		t.Fatalf("expected late authority failure after semantic simulation, got %v", err)
	}
	after, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if before.SHA256 != after.SHA256 {
		t.Fatalf("late semantic failure mutated managed persistence: before=%s after=%s", before.SHA256, after.SHA256)
	}
	if afterWholeTree := startupWholeTreeDigest(t, dataDir, durableRoot); afterWholeTree != beforeAuthorityWholeTree {
		t.Fatalf("late authority failure mutated a path outside the managed allowlist: before=%s after=%s", beforeAuthorityWholeTree, afterWholeTree)
	}
	if body, err := os.ReadFile(legacyPath); err != nil || string(body) != string(legacyThread) {
		t.Fatalf("late semantic failure changed legacy source: body=%q err=%v", body, err)
	}
	if _, err := os.Lstat(filepath.Join(durableRoot, "threads", legacyThreadID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("late semantic failure activated a partial legacy target: %v", err)
	}
}

func TestSemanticStartupPendingWorkLateFailureLeavesLiveRootUntouched(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := workspacetest.New(t)
	durable, err := server.NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := durable.CreateThread(map[string]any{"title": "late pending-work recovery", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	turnID := "turn_corrupt_pending_recovery"
	now := time.Now().UTC()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	contextBody, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{
		Thread: map[string]any{}, SecurityContext: securityContext, At: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := durable.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"contextEpochSnapshot": contextepochapp.PublicSnapshot(epoch.State.AcceptedSnapshot), "items": []any{},
	}, "", map[string]any{"securityState": contextRecord, "contextEpochState": contextepochapp.PublicState(epoch.State)}); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	pendingRoot := filepath.Join(dataDir, "private", "pending-work")
	pendingMutation, err := privatecastest.NewAccessAuthority(pendingRoot)
	if err != nil {
		t.Fatal(err)
	}
	pendingStore, err := pendingworkstore.NewStore(pendingRoot, pendingMutation)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindToolBatch, SecurityContext: securityContext, GrantRegistrySequence: 1,
		GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("registry")), GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("grant")), RegistrySequence: 1,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("payload")), RouteHash: domainsecurity.SHA256Hex([]byte("route")),
		IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(10 * time.Minute), AuthorityKeyID: authority.KeyID(),
		AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil || pendingStore.PutReceiptIfAbsent(context.Background(), receipt) != nil {
		t.Fatalf("seed open pending work: %v", err)
	}
	childRoot := filepath.Join(dataDir, "child-runs")
	if err := os.MkdirAll(childRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	// Identity collection now precedes every recovery consumer. Keep that
	// inventory valid and reject the lifecycle only in the later child-run
	// migration, after staged pending-work reconciliation has run.
	if err := os.WriteFile(filepath.Join(childRoot, "job-1.json"), []byte(`{"id":"job-1","status":"invalid-lifecycle"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(dataDir, durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	before, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	beforeWholeTree := startupWholeTreeDigest(t, dataDir, durableRoot)
	_, err = NewRuntimeServerHandlerE(Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot})
	if err == nil || !strings.Contains(err.Error(), "semantic startup phase child-run-migration:") ||
		!strings.Contains(err.Error(), "job status is outside the closed lifecycle allowlist") {
		t.Fatalf("expected failure after staged pending-work reconciliation, got %v", err)
	}
	after, err := persistencefs.CaptureStrict(roots)
	if err != nil || after.SHA256 != before.SHA256 {
		t.Fatalf("late pending-work failure mutated live roots: before=%s after=%s err=%v", before.SHA256, after.SHA256, err)
	}
	if afterWholeTree := startupWholeTreeDigest(t, dataDir, durableRoot); afterWholeTree != beforeWholeTree {
		t.Fatalf("late pending-work failure mutated a path outside the managed allowlist: before=%s after=%s", beforeWholeTree, afterWholeTree)
	}
	if _, err := pendingStore.ReadDisposition(context.Background(), receipt.WorkID); !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		t.Fatalf("staged restart-invalid disposition leaked into live authority: %v", err)
	}
}

func TestSemanticStartupLateAuthorityFailureKeepsReasoningAndMemoryMigrationsStaged(t *testing.T) {
	for _, fixture := range []struct {
		name string
		seed func(*testing.T, string, string) string
	}{
		{
			name: "reasoning-migration",
			seed: func(t *testing.T, _ string, durableRoot string) string {
				t.Helper()
				path := filepath.Join(durableRoot, "threads", "thr_staged_reasoning", "thread.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				body := `{"id":"thr_staged_reasoning","turns":[{"id":"turn_1","items":[{"id":"reasoning","turnId":"turn_1","kind":"assistant_reasoning","text":"PRIVATE_REASONING_STAGED_SENTINEL"},{"id":"answer","turnId":"turn_1","kind":"assistant_text","text":"public answer"}]}]}`
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
		{
			name: "memory-tombstone-migration",
			seed: func(t *testing.T, dataDir string, _ string) string {
				t.Helper()
				path := filepath.Join(dataDir, "memory", "records", "mem_go_1.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				body := `{"id":"mem_go_1","content":"legacy private memory","scope":"project","createdAt":"2026-07-11T00:00:00Z","updatedAt":"2026-07-11T00:00:00Z","deletedAt":"2026-07-11T00:00:00Z"}`
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
		{
			name: "child-run-reasoning-migration",
			seed: func(t *testing.T, dataDir string, _ string) string {
				t.Helper()
				path := filepath.Join(dataDir, "child-runs", "job-1.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				body := `{"id":"job-1","parentGoalId":"goal","parentThreadId":"thread","kind":"background-shell","name":"worker<think>PRIVATE_CHILD_NAME</think>","status":"completed","output":"ordinary child diagnostic<think>PRIVATE_CHILD_REASONING</think>","toolInvocations":0}`
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			dataDir := t.TempDir()
			durableRoot := t.TempDir()
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot}
			initial, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatalf("create initial runtime state: %v", err)
			}
			shutdownOwnedRuntimeHandler(t, initial)
			fixturePath := fixture.seed(t, dataDir, durableRoot)
			authorityPath := filepath.Join(dataDir, "private", "authority", "final-answer-ed25519-v1.json")
			authority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, true)
			if err != nil {
				t.Fatal(err)
			}
			seedRuntimeLegacyEvidenceRegistryV1(
				t, filepath.Join(dataDir, "private", "evidence-registry"), authority,
			)
			if err := os.Remove(authorityPath); err != nil {
				t.Fatal(err)
			}
			beforeFixture, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			beforeTree := startupWholeTreeDigest(t, dataDir, durableRoot)

			if _, err := NewRuntimeServerHandlerE(config); err == nil || !strings.Contains(err.Error(), "authority key is missing while authority records exist") {
				t.Fatalf("expected late authority failure after %s planning: %v", fixture.name, err)
			}
			afterFixture, err := os.ReadFile(fixturePath)
			if err != nil || string(afterFixture) != string(beforeFixture) {
				t.Fatalf("%s escaped the semantic stage: before=%q after=%q err=%v", fixture.name, beforeFixture, afterFixture, err)
			}
			if afterTree := startupWholeTreeDigest(t, dataDir, durableRoot); afterTree != beforeTree {
				t.Fatalf("late %s failure mutated live persistence: before=%s after=%s", fixture.name, beforeTree, afterTree)
			}
		})
	}
}

func TestSemanticStartupMigratesLegacyChildRunReasoningBeforeActivation(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("initialize runtime state: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, initial)

	path := filepath.Join(dataDir, "child-runs", "job-1.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "id":"job-1",
  "parentGoalId":"goal",
  "parentThreadId":"thread",
  "kind":"background-shell",
  "name":"worker<think>PRIVATE_CHILD_NAME</think>",
  "label":"ordinary child diagnostic",
  "prompt":"{\"reasoning_content\":\"PRIVATE_CHILD_PROMPT\"}",
  "profileDescription":"reviewer<think>PRIVATE_CHILD_PROFILE</think>",
  "diffSummary":"1 file<think>PRIVATE_CHILD_DIFF</think>",
  "status":"completed",
  "output":"ordinary child output<think>PRIVATE_CHILD_OUTPUT</think> account 6222020202020202020",
  "error":"ordinary child error<think>PRIVATE_CHILD_ERROR</think>",
  "autoContinueError":"PRIVATE_CHILD_AUTO",
  "completionDeliveryError":"PRIVATE_CHILD_DELIVERY",
  "toolInvocations":0
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("semantic child-run migration blocked activation: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(body))
	for _, forbidden := range []string{"private_child", "reasoning_content", "<think", "6222020202020202020"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("activated child-run storage retained %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(string(body), "background shell") ||
		!strings.Contains(string(body), "ordinary child output") || !strings.Contains(string(body), "[ACCOUNT]") {
		t.Fatalf("semantic migration removed valid public diagnostics: %s", body)
	}
}

func shutdownOwnedRuntimeHandler(t *testing.T, handler any) {
	t.Helper()
	lifecycle, ok := handler.(interface{ Shutdown(context.Context) error })
	if !ok {
		t.Fatal("runtime handler does not own a shutdown lifecycle")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := lifecycle.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown owned runtime handler: %v", err)
	}
}

func TestSemanticStartupFinalEventRepairLateFailureLeavesLiveRootUntouched(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	store, err := server.NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "staged final event repair"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	turnID := "turn_staged_final_event_repair"
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-staged-final-event-repair",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-staged-final-event-repair"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Unix(10, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{
		Thread: map[string]any{}, SecurityContext: securityContext, At: time.Unix(10, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"contextEpochSnapshot": contextepochapp.PublicSnapshot(epoch.State.AcceptedSnapshot), "items": []any{},
	}, "deepseek", map[string]any{
		"securityState": contextRecord, "contextEpochState": contextepochapp.PublicState(epoch.State),
	}); err != nil {
		t.Fatal(err)
	}
	beforeReplay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(beforeReplay.Diagnostics) != 0 {
		t.Fatalf("read pre-publication events: diagnostics=%#v err=%v", beforeReplay.Diagnostics, err)
	}
	privateRoot := filepath.Join(dataDir, "private")
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := evidenceregistry.NewStore(filepath.Join(privateRoot, "evidence-registry"), authority)
	if err != nil {
		t.Fatal(err)
	}
	privateStoreRoot := filepath.Join(privateRoot, "accepted-finals")
	privateAccess, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	privateStore, err := finalauthority.NewPrivateStore(privateStoreRoot, privateAccess)
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := startupFinalEventIOForTest(store, casReader)
	providerTelemetryRoot := filepath.Join(privateRoot, "provider-cache-telemetry")
	providerTelemetryStore, err := cachetelemetrystore.NewStore(providerTelemetryRoot, privateAccess)
	if err != nil {
		t.Fatal(err)
	}
	providerTelemetry, err := cachetelemetryapp.NewDurableService(authority, providerTelemetryStore)
	if err != nil {
		t.Fatal(err)
	}
	terminalRoot := filepath.Join(privateRoot, "turn-terminal-authority")
	terminalStore, err := turnterminalstore.NewStore(terminalRoot, privateAccess)
	if err != nil {
		t.Fatal(err)
	}
	terminalCoordinator, err := turnterminalapp.NewCoordinator(authority, privateStore, terminalStore, providerTelemetry)
	if err != nil {
		t.Fatal(err)
	}
	finalizer := evidenceapp.NewCasePublicationFinalizerWithAuthority(registry, registry, authority, privateStore, eventIO, terminalCoordinator)
	if _, err := finalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Store: store, Context: securityContext, TerminalReason: evidenceapp.TerminalProviderFailure,
		ThreadID: threadID, TurnID: turnID, AcceptedAt: time.Unix(11, 0),
	}); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(durableRoot, "threads", threadID, "events.jsonl")
	writeStartupEvents(t, eventPath, beforeReplay.Events)
	restartedStore, err := server.NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO = startupFinalEventIOForTest(restartedStore, casReader)
	inventory, err := evidenceapp.PreflightFinalAuthorityInventory(context.Background(), restartedStore, casReader, registry, authority, privateStore)
	if err != nil || len(inventory.Committed) != 1 {
		t.Fatalf("seed final authority inventory: committed=%d err=%v", len(inventory.Committed), err)
	}
	plans, err := evidenceapp.PreflightAcceptedFinalEventReconciliations(context.Background(), eventIO, restartedStore, restartedStore, inventory.Committed)
	if err != nil || len(plans) != 1 {
		t.Fatalf("missing accepted-final event repair was not detected: plans=%#v err=%v", plans, err)
	}
	childRoot := filepath.Join(dataDir, "child-runs")
	if err := os.MkdirAll(childRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	// Valid identity reaches child-run migration after staged final-event repair.
	if err := os.WriteFile(filepath.Join(childRoot, "job-1.json"), []byte(`{"id":"job-1","status":"invalid-lifecycle"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(dataDir, durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	beforeTree := startupWholeTreeDigest(t, dataDir, durableRoot)
	beforeEvents, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewRuntimeServerHandlerE(Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot})
	if err == nil || !strings.Contains(err.Error(), "semantic startup phase child-run-migration:") ||
		!strings.Contains(err.Error(), "job status is outside the closed lifecycle allowlist") {
		t.Fatalf("expected post-event-repair child-run failure, got %v", err)
	}
	afterSnapshot, captureErr := persistencefs.CaptureStrict(roots)
	if captureErr != nil || afterSnapshot.SHA256 != beforeSnapshot.SHA256 {
		t.Fatalf("late final-event failure mutated managed roots: before=%s after=%s err=%v", beforeSnapshot.SHA256, afterSnapshot.SHA256, captureErr)
	}
	afterEvents, readErr := os.ReadFile(eventPath)
	if readErr != nil || !bytes.Equal(afterEvents, beforeEvents) {
		t.Fatalf("staged accepted-final repair leaked into live events: before=%q after=%q err=%v", beforeEvents, afterEvents, readErr)
	}
	if afterTree := startupWholeTreeDigest(t, dataDir, durableRoot); afterTree != beforeTree {
		t.Fatalf("late final-event failure mutated live persistence: before=%s after=%s", beforeTree, afterTree)
	}
}

func startupFinalEventIOForTest(store *server.DurableEventSessionStore, casReader *finalauthority.AcceptedFinalCASReader) evidenceapp.FinalPublicationEventIO {
	delivery := store.AcceptedFinalEventDelivery()
	return evidenceapp.FinalPublicationEventIO{
		ReadThread: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			return store.GetThread(privateRecord.SecurityContext.ThreadID)
		},
		ReadCASObservation: func(ctx context.Context, _ appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (domainevidence.AcceptedFinalCASObservationV1, error) {
			return casReader.ReadAcceptedFinalCASObservation(ctx, privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID)
		},
		LoadEvents: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, threadID string) ([]map[string]any, error) {
			result, err := store.LoadEventsSince(threadID, 0)
			if err != nil || len(result.Diagnostics) != 0 {
				return nil, errors.New("startup final-event fixture log is invalid")
			}
			return result.Events, nil
		},
		AppendEvents: func(ctx context.Context, _ appturn.AcceptedFinalCompletionStore, events []map[string]any) ([]map[string]any, error) {
			return delivery.Stage(ctx, events)
		},
		Readback: delivery,
		WithEventReservation: func(
			ctx context.Context,
			_ appturn.AcceptedFinalCompletionStore,
			threadID, commitID string,
			work evidenceapp.AcceptedFinalEventReservationWorkV1,
		) error {
			return delivery.WithReservation(ctx, threadID, commitID, work)
		},
		ActivateAndPublishEvents: func(
			ctx context.Context,
			_ appturn.AcceptedFinalCompletionStore,
			events []map[string]any,
			seal domainevent.AcceptedFinalDeliverySealV1,
			activate func() error,
		) error {
			return delivery.ActivateAndPublish(ctx, events, seal, activate)
		},
	}
}

func writeStartupEvents(t *testing.T, path string, events []map[string]any) {
	t.Helper()
	var body bytes.Buffer
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		body.Write(encoded)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(path, body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func startupWholeTreeDigest(t *testing.T, roots ...string) string {
	t.Helper()
	records := []string{}
	for rootIndex, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			record := fmt.Sprintf("%d:%s:%d:%d", rootIndex, filepath.ToSlash(relative), uint32(info.Mode()), info.Size())
			if info.Mode().IsRegular() {
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				record += fmt.Sprintf(":%x", sha256.Sum256(body))
			}
			records = append(records, record)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(records)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(records, "\n"))))
}

func TestLeasedStartupRejectsPersistenceRootRetarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
	}
	base := t.TempDir()
	realA := filepath.Join(base, "real-a")
	realB := filepath.Join(base, "real-b")
	for _, root := range []string{realA, realB} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(realA, alias); err != nil {
		t.Fatal(err)
	}
	config := Config{
		RuntimeToken: DefaultRuntimeToken,
		DataDir:      filepath.Join(alias, "data"), DurableTempDir: filepath.Join(alias, "durable"),
	}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatalf("acquire frozen roots: %v", err)
	}
	defer lease.Close()
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realB, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease); err == nil {
		t.Fatal("retargeted persistence alias reused a lease for different roots")
	}
	for _, root := range []string{realA, realB} {
		for _, name := range []string{"data", "durable"} {
			if _, err := os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("retarget rejection mutated %s/%s: %v", root, name, err)
			}
		}
	}
}

func TestStartupInvalidConfigDoesNotApplyPersistencePlan(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "data")
	durableRoot := filepath.Join(base, "durable")
	_, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot,
		MCPConfigJSON: `{"mcpServers":{"funds":`,
	})
	if err == nil {
		t.Fatal("invalid current-run config passed startup planning")
	}
	for _, path := range []string{dataDir, durableRoot} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("invalid config mutated persistence root %s: %v", path, statErr)
		}
	}
}

func TestStartupDoesNotSpawnDetachedManagedWriters(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	if _, err := NewRuntimeServerHandlerE(Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot,
	}); err != nil {
		t.Fatal(err)
	}
	roots, err := persistencefs.ResolveRootSet(dataDir, durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	before, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1700 * time.Millisecond)
	after, err := persistencefs.CaptureStrict(roots)
	if err != nil {
		t.Fatal(err)
	}
	if before.SHA256 != after.SHA256 {
		t.Fatalf("startup spawned an unjournaled managed writer: before=%s after=%s", before.SHA256, after.SHA256)
	}
}
