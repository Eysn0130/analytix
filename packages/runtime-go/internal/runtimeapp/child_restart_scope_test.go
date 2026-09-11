//go:build darwin || linux

package runtimeapp

import (
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"analytix.local/runtime-go/internal/jobs"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type childScopeVerificationFailureV1 struct {
	authorityport.Authority
	call, failAt int
	err          error
}

func (authority *childScopeVerificationFailureV1) VerifyTrusted(ctx context.Context, keyID string, publicKey, body, signature []byte) error {
	authority.call++
	if authority.call == authority.failAt {
		return authority.err
	}
	return authority.Authority.VerifyTrusted(ctx, keyID, publicKey, body, signature)
}

func TestRuntimeReportRestartChildScopePreservesVerificationErrors(t *testing.T) {
	core, _ := runtimeChildRestartScopeFixtureV1(t, false, false)
	scope, err := prepareRuntimeReportRestartScopeV1(context.Background(), core)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []error{errors.New("synthetic verification I/O failure"), context.Canceled} {
		for _, failAt := range []int{1, len(core.pendingInventory.Receipts) + 1} {
			authority := &childScopeVerificationFailureV1{Authority: core.verification, failAt: failAt, err: sentinel}
			_, err := pendingworkapp.ExtendReportRestartChildScopeV1(context.Background(), *scope, core.pendingInventory, core.jobRecords, authority, core.primaries)
			if !errors.Is(err, sentinel) {
				t.Errorf("child scope lost original verification error at %d: %v", failAt, err)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pendingworkapp.VerifyTrustedSnapshotV1(ctx, nil, nil, core.verification); !errors.Is(err, context.Canceled) {
		t.Errorf("snapshot lost initial cancellation: %v", err)
	}
}

func TestRuntimeReportRestartChildConstructorCarriesOriginalScope(t *testing.T) {
	for _, scenario := range []string{"semantic", "live", "physical_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, targets := runtimeChildRestartScopeFixtureV1(t, false, false)
			root := filepath.Join(core.roots.DataDir, "child-runs")
			heldArtifact := filepath.Join(root, targets[0].JobID+".log")
			if err := os.WriteFile(heldArtifact, []byte("synthetic held artifact"), 0o600); err != nil {
				t.Fatal(err)
			}
			if scenario == "semantic" {
				if err := os.WriteFile(filepath.Join(root, "job-1.log"), []byte("synthetic independent artifact"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			core, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			input, err := preserved.childConstructorInputV1(ctx)
			if err != nil || input == nil || len(input.JobIDs) != 2 || len(input.OriginalRecords) != 1 {
				t.Fatalf("root constructor dependency input is incomplete: %v", err)
			}
			if scenario == "physical_drift" {
				if err := os.Remove(heldArtifact); err != nil {
					t.Fatal(err)
				}
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			var manager *jobs.Manager
			if scenario == "live" {
				manager, err = jobs.NewManagerWithRestartPreservationV1(ctx, root, nil, input, core.floors)
			} else {
				manager, err = jobs.NewManagerForSemanticStartupWithRestartPreservationV1(ctx, root, root, nil, nil, input, core.floors)
			}
			if scenario == "physical_drift" {
				if err == nil || manager != nil {
					t.Error("root lost original physical constructor witness")
				}
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
					t.Fatal("refused constructor wrote state")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "semantic" {
				if _, err := os.Stat(filepath.Join(root, "job-1.log")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("root held scope blocked independent cleanup")
				}
			}
			if _, err := manager.UpdateChildRun(targets[0].JobID, jobs.UpdateRequest{Status: "interrupted"}); !errors.Is(err, jobs.ErrRestartPreserved) {
				t.Fatalf("root constructor failed to install live denial: %v", err)
			}
			ordinary, err := manager.StartChildRun(jobs.StartRequest{ParentGoalID: "goal-independent", ParentThreadID: "thread-independent", Kind: "subagent", Status: "running"})
			if err != nil || ordinary.ID != "job-902" {
				t.Fatalf("independent child allocation reused the original signed vector: id=%s err=%v", ordinary.ID, err)
			}
			after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for path, entry := range before {
				if scenario == "semantic" && path == "0:child-runs/job-1.log" {
					continue
				}
				if !semanticOriginalRecordUnchangedForTestV1(entry, after[path]) {
					t.Errorf("root constructor changed original state at %s", path)
				}
			}
		})
	}
}

func TestRuntimeSemanticChildRecordsRequireCompleteOperationalValidation(t *testing.T) {
	for _, scenario := range []string{"held_original_status", "independent_original_status", "candidate_status", "candidate_unbound_receipt", "independent_candidate"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, _ := runtimeChildRestartScopeFixtureV1(t, false, false)
			producerRoot := t.TempDir()
			producer, err := jobs.NewManager(producerRoot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := producer.StartChildRun(jobs.StartRequest{ParentGoalID: "goal-independent", ParentThreadID: "thread-independent", Kind: "subagent", Status: "running"}); err != nil {
				t.Fatal(err)
			}
			snapshot, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, producerRoot)
			if err != nil {
				t.Fatal(err)
			}
			record := snapshot.Records[0]
			original := scenario == "held_original_status" || scenario == "independent_original_status"
			if scenario == "held_original_status" {
				record = core.jobRecords[0]
			}
			if scenario != "independent_candidate" {
				record.Status = "invalid-status"
			}
			if scenario == "candidate_unbound_receipt" {
				record.Status = "completed"
				record.ChildCompletionReceipt = &domainjob.ChildCompletionReceiptV1{}
			}
			body, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if original {
				if err := os.WriteFile(filepath.Join(core.roots.DataDir, "child-runs", record.ID+".json"), body, 0o600); err != nil {
					t.Fatal(err)
				}
				core, err = runtimeObservePendingCoreForTestV1(t, core)
				if err != nil {
					if scenario == "independent_original_status" && errors.Is(err, pendingworkapp.ErrChildProducerInventoryIncomplete) {
						return
					}
					t.Fatal(err)
				}
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if !original && err != nil {
				t.Fatal(err)
			}
			if !original {
				digest := domainsecurity.SHA256Hex([]byte("synthetic-child-operational-plan"))
				plan, planErr := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, []domainstartup.SemanticStartupOperationV1{{Kind: domainstartup.SemanticOperationInstallFile, Path: "data/child-runs/" + record.ID + ".json", Before: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}, After: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}}})
				if planErr != nil {
					t.Fatal(planErr)
				}
				err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }, "")
			}
			if scenario == "independent_candidate" {
				if err != nil {
					t.Fatalf("valid independent child was rejected: %v", err)
				}
			} else if err == nil {
				t.Error("identity-only child record bypassed complete operational validation")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("child operational observation wrote state")
			}
		})
	}
}

func TestRuntimeReportRestartChildScopeUsesWholeOriginalVectorAndIssuedGrant(t *testing.T) {
	for _, scenario := range []string{"current", "legacy", "wrong_issued_prefix"} {
		t.Run(scenario, func(t *testing.T) {
			core, targets := runtimeChildRestartScopeFixtureV1(t, scenario == "legacy", scenario == "wrong_issued_prefix")
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			scope, err := prepareRuntimeReportRestartScopeV1(context.Background(), core)
			if scenario == "wrong_issued_prefix" {
				if err == nil {
					t.Fatal("signed child vector bypassed the original issued registry prefix")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				for _, target := range targets {
					if !scope.OwnsThread(target.ChildThreadID) {
						t.Errorf("whole original child vector omitted %s", target.ChildThreadID)
					}
				}
				if len(scope.Contexts()) != 2 {
					t.Error("existing original child context was not preserved")
				}
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("child scope observation wrote original state or reminted a reserved identity")
			}
		})
	}
}

func TestRuntimeSemanticChildScopeRejectsHeldJobAndAbsentPrimaryEffects(t *testing.T) {
	for _, scenario := range []string{"remove_job", "change_job", "remint_job", "create_current_primary", "create_legacy_primary"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, targets := runtimeChildRestartScopeFixtureV1(t, false, false)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(filepath.Join(core.roots.DataDir, "child-runs", targets[0].JobID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var record domainjob.Record
			if err := json.Unmarshal(original, &record); err != nil {
				t.Fatal(err)
			}
			state := func(body []byte) domainstartup.SemanticEntryStateV1 {
				return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			}
			absent := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
			path := "data/child-runs/" + record.ID + ".json"
			operation := domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationRemoveFile, Path: path, Before: state(original), After: absent}
			var body []byte
			var operations []domainstartup.SemanticStartupOperationV1
			switch scenario {
			case "change_job", "remint_job":
				if scenario == "change_job" {
					record.Status = "failed"
				} else {
					record.ID, record.ChildThreadID, record.ChildTurnID, record.ParallelIndex = targets[1].JobID, targets[1].ChildThreadID, targets[1].ChildTurnID, 2
					operation.Path, operation.Before = "data/child-runs/"+record.ID+".json", absent
				}
				body, err = json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				operation.Kind, operation.After = domainstartup.SemanticOperationInstallFile, state(body)
			case "create_current_primary", "create_legacy_primary":
				family := "durable/threads/"
				if scenario == "create_legacy_primary" {
					family = "durable/runtime-go/threads/"
				}
				path := family + targets[1].ChildThreadID
				operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationCreateDirectory, Path: path, Before: absent, After: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeDirectory, Mode: 0o700}})
				body, err = json.Marshal(map[string]any{"id": targets[1].ChildThreadID, "turns": []any{}})
				if err != nil {
					t.Fatal(err)
				}
				operation = domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationInstallFile, Path: path + "/thread.json", Before: absent, After: state(body)}
			}
			operations = append(operations, operation)
			digest := domainsecurity.SHA256Hex([]byte("synthetic-held-child-effects"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, operations)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }, "")
			if err == nil {
				t.Fatal("semantic program bypassed held child identity")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("held child semantic observation changed state")
			}
		})
	}
}

func TestRuntimeSemanticChildScopeRejectsAppliedRemovalWithoutReclassifyingAbsence(t *testing.T) {
	ctx := context.Background()
	core, targets := runtimeChildRestartScopeFixtureV1(t, false, false)
	installed, absent := "child-runs/job-1.log", "child-runs/"+targets[0].JobID+".json"
	const tail = "private/z-child-removal-tail.bin"
	if err := os.WriteFile(filepath.Join(core.roots.DataDir, tail), []byte("original tail"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.WriteFile(filepath.Join(stage.DataDir, installed), []byte("synthetic independent prefix"), 0o600); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(stage.DataDir, absent)); err != nil {
			return err
		}
		return os.Remove(filepath.Join(stage.DataDir, tail))
	}, installed, absent)
	fresh, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	_, err = prepareRuntimeReportRestartPreservationV1(ctx, fresh)
	if !errors.Is(err, pendingworkapp.ErrRestartPreserved) {
		t.Fatalf("applied removal became an original absent reservation: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("child applied-removal refusal changed state")
	}
}

func TestRuntimeSemanticChildScopeRejectsAppliedPrimaryCreationAsOriginal(t *testing.T) {
	ctx := context.Background()
	core, targets := runtimeChildRestartScopeFixtureV1(t, false, false)
	tailRoot := filepath.Join(core.roots.DurableDir, "threads", "zz-independent")
	if err := os.MkdirAll(tailRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tailRoot, "thread.json"), []byte(`{"id":"zz-independent","turns":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := filepath.Rel(core.roots.DataDir, filepath.Join(core.roots.DurableDir, "threads", targets[1].ChildThreadID, "thread.json"))
	if err != nil {
		t.Fatal(err)
	}
	absent, err := filepath.Rel(core.roots.DataDir, filepath.Join(tailRoot, "z-tail.bin"))
	if err != nil {
		t.Fatal(err)
	}
	runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		path := filepath.Join(stage.DurableDir, "threads", targets[1].ChildThreadID, "thread.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		body, err := json.Marshal(map[string]any{"id": targets[1].ChildThreadID, "turns": []any{}})
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DurableDir, "threads", "zz-independent", "z-tail.bin"), []byte("independent tail"), 0o600)
	}, installed, absent)
	fresh, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	_, err = prepareRuntimeReportRestartPreservationV1(ctx, fresh)
	if !errors.Is(err, pendingworkapp.ErrRestartPreserved) {
		t.Fatalf("signed future child primary became original authority: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("future child primary refusal changed state")
	}
}
