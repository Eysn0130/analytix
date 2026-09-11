//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type childIdentityOwnerAccessFailureV1 struct {
	*privatecastest.AccessAuthority
	owner  string
	err    error
	once   bool
	failed bool
}

func (access *childIdentityOwnerAccessFailureV1) WithExistingPrivateCASAccess(ctx context.Context, root string, visit func(privatecasport.RootBinding) error) error {
	if root == access.owner && (!access.once || !access.failed) {
		access.failed = true
		return access.err
	}
	return access.AccessAuthority.WithExistingPrivateCASAccess(ctx, root, visit)
}

func TestRuntimeEmptyPendingProofCannotHideOwnerAccessFailure(t *testing.T) {
	for _, once := range []bool{false, true} {
		t.Run(map[bool]string{false: "persistent", true: "transient"}[once], func(t *testing.T) {
			roots, err := persistencefs.ResolveRootSet(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			owner := filepath.Join(roots.DataDir, "private", "pending-work")
			if err := os.MkdirAll(filepath.Join(owner, "receipts"), 0o700); err != nil {
				t.Fatal(err)
			}
			inner, err := privatecastest.NewAccessAuthority(filepath.Join(roots.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
			if err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("synthetic exact-owner access failure")
			access := &childIdentityOwnerAccessFailureV1{AccessAuthority: inner, owner: owner, err: sentinel, once: once}
			before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			prepared, err := prepareRuntimeChildIdentityStartupV1(context.Background(), roots, rootAuthority, access, nil)
			if prepared != nil || !errors.Is(err, sentinel) {
				t.Fatalf("empty proof hid original owner access failure: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
				t.Fatal("owner access rejection changed state")
			}
		})
	}
}

func TestRuntimeChildIdentityLegacyRefusalPrecedesStartupWriters(t *testing.T) {
	_, config := runtimeWitnessedRegistryConfigV2(t)
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	root := filepath.Join(config.DataDir, "child-runs")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "job-900.json"), []byte(`{"id":"job-900","kind":"subagent","status":"completed","parentThreadId":"thr_durable_1","parentTurnId":"turn_1","childThreadId":"thr_durable_fork_800","childTurnId":"turn_950"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, config.DataDir, config.ProductionDurableRoot)
	handler, err = NewRuntimeServerHandlerE(config)
	if handler != nil {
		shutdownOwnedRuntimeHandler(t, handler)
	}
	if !errors.Is(err, pendingworkapp.ErrChildProducerInventoryIncomplete) {
		t.Errorf("missing producer witness did not produce the pre-writer Core refusal: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, config.DataDir, config.ProductionDurableRoot)) {
		t.Fatal("legacy child witness refusal changed startup state")
	}
}

func TestRuntimeChildIdentityPreflightAllowsAuthenticatedEmptyOwnerRecovery(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	missing := filepath.Join(config.DataDir, "private", "pending-work", "dispositions")
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	handler, err = NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("empty partial pending owner was rejected before authenticated recovery: %v", err)
	}
	defer shutdownOwnedRuntimeHandler(t, handler)
	if info, err := os.Lstat(missing); err != nil || !info.IsDir() {
		t.Fatal("empty owner recovery did not restore its exact leaf")
	}
}

func runtimeWriteClosedChildFloorReceiptV1(t *testing.T, config Config) string {
	t.Helper()
	ctx := context.Background()
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC)
	frozen, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thr_durable_1", TurnID: "turn_2", WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	digest := func(label string) string { return domainsecurity.SHA256Hex([]byte("synthetic-floor-" + label)) }
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindSideEffectIntent, SecurityContext: frozen, GrantRegistrySequence: 1, GrantRegistryDigest: digest("registry"),
		GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: digest("grant"), RegistrySequence: 1, RegistryEntryDigest: digest("entry")}},
		PayloadHash:  digest("payload"), RouteHash: digest("route"), IssuedAt: now, ExpiresAt: now.Add(time.Minute), AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey(),
		ChildProducer: &domainpendingwork.ChildProducerV1{ParentBindingDigest: digest("binding"), Children: []domainpendingwork.ChildProducerTargetV1{{Ordinal: 1, JobID: "job-900", ChildThreadID: "thr_durable_700", ChildTurnID: "turn_950"}}},
	}, func(message []byte) ([]byte, error) { return key.Sign(ctx, message) })
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusCancelled, "tool_call_cancelled", now.Add(time.Minute), key.KeyID(), key.PublicKey(), func(message []byte) ([]byte, error) { return key.Sign(ctx, message) })
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	closedBody, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leaf := range []struct {
		name string
		body []byte
	}{{"receipts", body}, {"dispositions", closedBody}} {
		cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", "pending-work", leaf.name), 1<<20, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := cas.PutIfAbsent(ctx, receipt.WorkID, leaf.body); err != nil {
			_ = cas.Close()
			t.Fatal(err)
		}
		if err := cas.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(config.DataDir, "private", "pending-work", "receipts", receipt.WorkID[:2], receipt.WorkID+".json")
}

func TestRuntimeChildIdentityClosedMissingJobFloorsSurviveFreshRestart(t *testing.T) {
	for _, enrolled := range []bool{false, true} {
		name := "local_existing_key"
		if enrolled {
			name = "enrolled_key"
		}
		t.Run(name, func(t *testing.T) {
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
			if enrolled {
				_, config = runtimeWitnessedRegistryConfigV2(t)
			}
			handler, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			shutdownOwnedRuntimeHandler(t, handler)
			receiptPath := runtimeWriteClosedChildFloorReceiptV1(t, config)
			original, err := os.ReadFile(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			handler, err = NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			defer shutdownOwnedRuntimeHandler(t, handler)
			thread := runtimeStartupJSON(t, handler, http.MethodPost, "/v1/threads", map[string]any{"title": "synthetic", "workspace": t.TempDir()}, http.StatusCreated)
			if thread["id"] != "thr_durable_701" {
				t.Fatalf("fresh restart reused a signed missing-child identity: %v", thread["id"])
			}
			if _, err := os.Lstat(filepath.Join(config.DataDir, "child-runs", "job-900.json")); !os.IsNotExist(err) {
				t.Fatal("floor restoration recreated a missing child job")
			}
			current, err := os.ReadFile(receiptPath)
			if err != nil || !reflect.DeepEqual(original, current) {
				t.Fatal("floor restoration rewrote signed receipt")
			}
		})
	}
}

func TestRuntimeChildIdentitySnapshotReadsBothPrimaryRootsAndRevalidatesFullDenominator(t *testing.T) {
	for _, mode := range []string{"complete", "missing_primary", "current_root_file", "legacy_root_file", "duplicate_primary_id", "new_thread", "new_job"} {
		t.Run(mode, func(t *testing.T) {
			roots, err := persistencefs.ResolveRootSet(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Join(roots.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			writePrimary := func(relative, id, turn string) {
				dir := filepath.Join(roots.DurableDir, relative, "threads", id)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				body, err := json.Marshal(map[string]any{"id": id, "turns": []any{map[string]any{"id": turn}}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "thread.json"), body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "current_root_file" {
				if err := os.WriteFile(filepath.Join(roots.DurableDir, "threads"), []byte("synthetic"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				writePrimary("", "thr_durable_500", "turn_600")
			}
			if mode == "legacy_root_file" {
				if err := os.MkdirAll(filepath.Join(roots.DurableDir, "runtime-go"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(roots.DurableDir, "runtime-go", "threads"), []byte("synthetic"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				id := "thr_durable_fork_700"
				if mode == "duplicate_primary_id" {
					id = "thr_durable_500"
				}
				writePrimary("runtime-go", id, "turn_800")
			}
			if mode == "missing_primary" {
				if err := os.Remove(filepath.Join(roots.DurableDir, "runtime-go", "threads", "thr_durable_fork_700", "thread.json")); err != nil {
					t.Fatal(err)
				}
			}
			before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := prepareRuntimeChildIdentityStartupV1(context.Background(), roots, rootAuthority, access, nil)
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
				t.Fatal("identity inventory wrote source state")
			}
			if mode == "missing_primary" || mode == "current_root_file" || mode == "legacy_root_file" || mode == "duplicate_primary_id" {
				if !errors.Is(err, pendingworkapp.ErrChildProducerInventoryIncomplete) {
					t.Fatalf("missing primary not refused: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if prepared.floors.ThreadSequence != 500 || prepared.floors.ForkSequence != 700 || prepared.floors.TurnSequence != 800 {
				t.Fatal("legacy primary directory omitted from floors")
			}
			switch mode {
			case "new_thread":
				writePrimary("", "thr_durable_900", "turn_901")
			case "new_job":
				root := filepath.Join(roots.DataDir, "child-runs")
				if err := os.MkdirAll(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "job-990.json"), []byte(`{"id":"job-990"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err = prepared.revalidate(context.Background())
			if (mode == "complete") != (err == nil) {
				t.Fatalf("complete source manifest revalidation mismatch: %v", err)
			}
		})
	}
}
