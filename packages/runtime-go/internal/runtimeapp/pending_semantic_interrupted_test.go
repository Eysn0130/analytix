//go:build darwin || linux

package runtimeapp

import (
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRuntimeSemanticPendingPreservationResumesIndependentInterruptedClosedPair(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := runtimeReservedReportAttemptFixtureV1(t, authority)
	at, err := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusFailed, "report_stage_failed", at.Add(time.Second), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	receiptBody, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-pending-interrupted-configuration"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	rootAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journalAuthority, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, rootAuthority, journalAuthority, preserved)
	pathFor := func(data, leaf string) string {
		id := receipt.WorkID
		return filepath.Join(data, "private", "pending-work", leaf, id[:2], id+".json")
	}
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		for leaf, body := range map[string][]byte{"receipts": receiptBody, "dispositions": dispositionBody} {
			path := pathFor(stage.DataDir, leaf)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(path, body, 0o600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: pathFor(core.roots.DataDir, "dispositions"), intentPath: pathFor(core.roots.DataDir, "receipts")}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("pending pair did not reach signed physical interruption: %v", err)
	}
	if body, err := os.ReadFile(cut.dispositionPath); err != nil || !reflect.DeepEqual(body, dispositionBody) {
		t.Fatal("pending disposition did not reach physical After")
	}
	if _, err := os.Stat(cut.intentPath); !os.IsNotExist(err) {
		t.Fatal("pending fixture did not stop before receipt installation")
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	freshAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	freshCore, err := prepareRuntimeChildIdentityStartupV1(ctx, core.roots, freshAuthority, core.access, nil)
	if err != nil {
		t.Fatalf("fresh Core rejected an authenticated independent closed pending pair before replay: %v", err)
	}
	for _, original := range freshCore.pendingInventory.Receipts {
		if original.WorkID == receipt.WorkID {
			t.Fatal("unwritten projected receipt became original pending inventory")
		}
	}
	freshPreserved, err := prepareRuntimeReportRestartPreservationV1(ctx, freshCore)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(freshPreserved.report.ThreadIDs(), preserved.report.ThreadIDs()) {
		t.Fatal("independent pending repair changed original held scope")
	}
	freshJournal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	recovery := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, freshAuthority, freshJournal, freshPreserved)
	if err := recovery.RecoverAuthenticatedExisting(ctx); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(cut.intentPath); err != nil || !reflect.DeepEqual(body, receiptBody) {
		t.Fatal("independent pending receipt was not recovered exactly")
	}
	after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	for path, original := range before {
		if !semanticOriginalRecordUnchangedForTestV1(original, after[path]) {
			t.Fatalf("independent pending recovery changed an original file or mode at %s", path)
		}
	}
}

type cancelAfterPendingRemovalContextV1 struct {
	context.Context
	cancel        context.CancelFunc
	removedPaths  []string
	remainingPath string
}

func (ctx cancelAfterPendingRemovalContextV1) Err() error {
	allRemoved := true
	for _, path := range ctx.removedPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			allRemoved = false
		}
	}
	if allRemoved {
		if _, err := os.Stat(ctx.remainingPath); err == nil {
			ctx.cancel()
		}
	}
	return ctx.Context.Err()
}

func TestRuntimeSemanticPendingCoreRejectsLostBeforeInventoryAfterRemoveOnlyCut(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	id := core.pendingInventory.Receipts[0].WorkID
	pathFor := func(data, leaf string) string {
		return filepath.Join(data, "private", "pending-work", leaf, id[:2], id+".json")
	}
	remainingPath := func(data string) string { return filepath.Join(data, "private", "z-remove-after-pending.bin") }
	if err := os.WriteFile(remainingPath(core.roots.DataDir), []byte("remaining-original"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-pending-remove-only-cut"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	builder := persistencefs.NewSemanticPlanBuilder(core.roots)
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		for _, path := range []string{pathFor(stage.DataDir, "receipts"), pathFor(stage.DataDir, "dispositions"), remainingPath(stage.DataDir)} {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if len(prepared.Plan().Operations) != 3 {
		t.Fatal("remove-only fixture operation count differs")
	}
	for _, operation := range prepared.Plan().Operations {
		if operation.Kind != domainstartup.SemanticOperationRemoveFile {
			t.Fatal("fixture is not a remove-only signed program")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterPendingRemovalContextV1{Context: cancelled, cancel: cancel, removedPaths: []string{pathFor(core.roots.DataDir, "receipts"), pathFor(core.roots.DataDir, "dispositions")}, remainingPath: remainingPath(core.roots.DataDir)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("remove-only cut did not reach real cancellation: %v", err)
	}
	for _, path := range cut.removedPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("pending original file was not removed at the signed cut")
		}
	}
	if _, err := os.Stat(cut.remainingPath); err != nil {
		t.Fatal("fixture did not preserve the independent remaining operation")
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	freshAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	if next, err := prepareRuntimeChildIdentityStartupV1(ctx, core.roots, freshAuthority, core.access, nil); err == nil || next != nil || !strings.Contains(err.Error(), "original Before bytes are unavailable") {
		t.Fatalf("empty current inventory bypassed lost original pending denominator: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("lost-Before rejection changed durable state")
	}
}

// Each caller stops a real signed journal at an observed physical cut, then
// exercises fresh Core observation. Omitted preservation models an older
// producer; a supplied scope also verifies the current root-bound Apply.
func runtimePendingSemanticCutForTestV1(t *testing.T, core *runtimeChildIdentityStartupV1, mutation func(context.Context, startupport.PersistenceRootsV1) error, installed, absent string, preservation ...runtimeReportRestartPreservationV1) {
	t.Helper()
	ctx := context.Background()
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-pending-observation-cut"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	builder := persistencefs.NewSemanticPlanBuilder(core.roots)
	if len(preservation) > 1 {
		t.Fatal("fixture has conflicting preservation authorities")
	}
	if len(preservation) == 1 {
		root, err := persistencefs.FreezeRootAuthority(core.roots)
		if err != nil {
			t.Fatal(err)
		}
		journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
		if err != nil {
			t.Fatal(err)
		}
		builder = persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preservation[0])
	}
	prepared, err := builder.Prepare(ctx, baseline, digest, mutation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: filepath.Join(core.roots.DataDir, installed), intentPath: filepath.Join(core.roots.DataDir, absent)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("signed pending fixture did not reach its physical cut: %v", err)
	}
	if _, err := os.Stat(cut.dispositionPath); err != nil {
		t.Fatal("signed pending cut did not install its trigger")
	}
	if _, err := os.Stat(cut.intentPath); !os.IsNotExist(err) {
		t.Fatal("signed pending cut crossed its remaining operation")
	}
}

func TestRuntimeSemanticPendingCoreRejectsAppliedDispositionThatClosesOriginalHold(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, false, false)
	receipt := core.pendingInventory.Receipts[0]
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusFailed, "report_stage_failed", at.Add(time.Second), key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	const tail = "private/z-pending-tail.bin"
	dispositionPath := filepath.Join("private", "pending-work", "dispositions", receipt.WorkID[:2], receipt.WorkID+".json")
	runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		runtimeWritePendingSemanticBodyForTestV1(t, stage.DataDir, "dispositions", receipt.WorkID, body)
		return os.WriteFile(filepath.Join(stage.DataDir, tail), []byte("remaining"), 0o600)
	}, dispositionPath, tail)
	if next, err := runtimeObservePendingCoreForTestV1(t, core); next != nil || err == nil || !strings.Contains(err.Error(), "unresolved inventory changed") {
		t.Fatalf("physical closed pair erased its provable original open hold: %v", err)
	}
}

func TestRuntimeSemanticPendingCoreRejectsNewUnresolvedWorkFromEmptyOriginal(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "unknown"}[unknown], func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeReportPreservationFixtureV1(t, false, false, false)
			original := core.pendingInventory.Receipts[0]
			// Establish an actually empty synthetic inventory before the signed transaction.
			if err := os.Remove(filepath.Join(core.roots.DataDir, "private", "pending-work", "receipts", original.WorkID[:2], original.WorkID+".json")); err != nil {
				t.Fatal(err)
			}
			empty, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil || len(empty.pendingInventory.Receipts) != 0 {
				t.Fatalf("fixture original is not empty: %v", err)
			}
			key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			receipt, _ := runtimeReservedReportAttemptFixtureV1(t, key)
			body, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
			if err != nil {
				t.Fatal(err)
			}
			const prefix = "private/a-pending-prefix.bin"
			receiptPath := filepath.Join("private", "pending-work", "receipts", receipt.WorkID[:2], receipt.WorkID+".json")
			runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				runtimeWritePendingSemanticBodyForTestV1(t, stage.DataDir, "receipts", receipt.WorkID, body)
				if unknown {
					at, err := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
					if err != nil {
						return err
					}
					disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusOutcomeUnknown, "report_stage_outcome_unknown_after_restart", at.Add(time.Second), key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
					if err != nil {
						return err
					}
					closedBody, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
					if err != nil {
						return err
					}
					runtimeWritePendingSemanticBodyForTestV1(t, stage.DataDir, "dispositions", receipt.WorkID, closedBody)
				}
				return os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("applied"), 0o600)
			}, prefix, receiptPath)
			if next, err := runtimeObservePendingCoreForTestV1(t, core); next != nil || err == nil || !strings.Contains(err.Error(), "not independently closed") {
				t.Fatalf("empty original bypassed unresolved future receipt validation: %v", err)
			}
		})
	}
}

func TestRuntimeSemanticPendingCoreIncludesFutureChildVectorAndExactActualJob(t *testing.T) {
	for _, scenario := range []string{"missing_job", "exact_job", "wrong_job_turn", "wrong_job_binding", "wrong_job_ordinal"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
			key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
			frozen, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thr_durable_10", TurnID: "turn_11", WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			digest := func(label string) string { return domainsecurity.SHA256Hex([]byte("synthetic-future-child-" + label)) }
			entropy := sha256.Sum256([]byte("synthetic-future-child-call"))
			callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
			if err != nil {
				t.Fatal(err)
			}
			grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: frozen, Provider: "synthetic-provider", ServerIdentity: "host:builtin", ToolName: "parallel_tasks", ToolCallID: callID, ArgsHash: digest("args"), SchemaHash: digest("schema"), ScopeHash: digest("scope"), ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
			binding, err := domainjob.NewSecurityBinding(frozen, grant, callID)
			if err != nil {
				t.Fatal(err)
			}
			targets := []domainpendingwork.ChildProducerTargetV1{{Ordinal: 1, JobID: "job-900", ChildThreadID: "thr_durable_700", ChildTurnID: "turn_950"}, {Ordinal: 2, JobID: "job-901", ChildThreadID: "thr_durable_fork_801", ChildTurnID: "turn_951"}}
			sign := func(body []byte) ([]byte, error) { return key.Sign(ctx, body) }
			receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{Kind: domainpendingwork.KindSideEffectIntent, SecurityContext: frozen, GrantRegistrySequence: 1, GrantRegistryDigest: digest("registry"), GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: 1, RegistryEntryDigest: digest("entry")}}, PayloadHash: digest("payload"), RouteHash: digest("route"), IssuedAt: now, ExpiresAt: now.Add(time.Minute), AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey(), ChildProducer: &domainpendingwork.ChildProducerV1{ParentBindingDigest: binding.BindingDigest, Children: targets}}, sign)
			if err != nil {
				t.Fatal(err)
			}
			disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusCancelled, "tool_call_cancelled", now.Add(time.Minute), key.KeyID(), key.PublicKey(), sign)
			if err != nil {
				t.Fatal(err)
			}
			receiptBody, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
			if err != nil {
				t.Fatal(err)
			}
			dispositionBody, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "missing_job" {
				record := domainjob.Record{ID: targets[0].JobID, Kind: "subagent", Status: "completed", ParentThreadID: binding.ParentThreadID, ParentTurnID: binding.ParentTurnID, ParentToolCallID: binding.ParentToolCallID, SecurityBinding: binding, ChildThreadID: targets[0].ChildThreadID, ChildTurnID: targets[0].ChildTurnID}
				switch scenario {
				case "wrong_job_turn":
					record.ChildTurnID = "turn_999"
				case "wrong_job_binding":
					record.SecurityBinding.ParentContextEpoch++
				case "wrong_job_ordinal":
					record.ParallelIndex = 2
				}
				body, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(core.roots.DataDir, "child-runs", record.ID+".json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			const prefix = "private/a-child-prefix.bin"
			receiptPath := filepath.Join("private", "pending-work", "receipts", receipt.WorkID[:2], receipt.WorkID+".json")
			runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				runtimeWritePendingSemanticBodyForTestV1(t, stage.DataDir, "receipts", receipt.WorkID, receiptBody)
				runtimeWritePendingSemanticBodyForTestV1(t, stage.DataDir, "dispositions", receipt.WorkID, dispositionBody)
				return os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("applied"), 0o600)
			}, prefix, receiptPath)
			next, err := runtimeObservePendingCoreForTestV1(t, core)
			if strings.HasPrefix(scenario, "wrong_") {
				if next != nil || !errors.Is(err, pendingworkapp.ErrChildProducerInventoryIncomplete) {
					t.Fatalf("future signed vector accepted a cross-bound actual job: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if next.floors.JobSequence != 901 || next.floors.ThreadSequence != 700 || next.floors.ForkSequence != 801 || next.floors.TurnSequence != 951 {
				t.Fatalf("future closed vector lost reserved identities: %+v", next.floors)
			}
			for _, original := range next.pendingInventory.Receipts {
				if original.WorkID == receipt.WorkID {
					t.Fatal("future allocation receipt became original business inventory")
				}
			}
			if _, err := os.Stat(filepath.Join(core.roots.DataDir, "child-runs", targets[1].JobID+".json")); !os.IsNotExist(err) {
				t.Fatal("allocation observation recreated an absent reserved job")
			}
		})
	}
}
