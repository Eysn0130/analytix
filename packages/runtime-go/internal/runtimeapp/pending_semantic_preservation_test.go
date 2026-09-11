//go:build darwin || linux

package runtimeapp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestRuntimeSemanticPendingPreservationChecksOriginalAndCompleteCandidateInventory(t *testing.T) {
	for _, scenario := range []string{"remove_held_pair", "replace_held_disposition", "held_mode", "independent_closed_pair", "orphan_disposition", "wrong_address"} {
		t.Run(scenario, func(t *testing.T) {
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
			held := core.pendingInventory.Receipts[0]
			independent, _ := runtimeReservedReportAttemptFixtureV1(t, authority)
			if independent.Context.ThreadID == held.Context.ThreadID {
				t.Fatal("independent fixture reused held thread")
			}
			pathFor := func(leaf, id string) string {
				return "data/private/pending-work/" + leaf + "/" + id[:2] + "/" + id + ".json"
			}
			bodies := map[string][]byte{}
			operations := []domainstartup.SemanticStartupOperationV1{}
			state := func(body []byte) domainstartup.SemanticEntryStateV1 {
				return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			}
			add := func(kind, path string, after []byte) {
				t.Helper()
				before := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				body, err := os.ReadFile(filepath.Join(core.roots.DataDir, path[len("data/"):]))
				if err == nil {
					before = state(body)
				} else if !os.IsNotExist(err) {
					t.Fatal(err)
				}
				next := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				if after != nil {
					next = state(after)
					bodies[path] = after
				}
				if kind == domainstartup.SemanticOperationSetMode {
					next = before
					next.Mode = 0o400
				}
				operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: kind, Path: path, Before: before, After: next})
			}
			failedBody := func(receipt domainpendingwork.PendingWorkReceiptV1) []byte {
				t.Helper()
				at, err := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusFailed, "report_stage_failed", at.Add(time.Second), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
				if err != nil {
					t.Fatal(err)
				}
				return body
			}
			switch scenario {
			case "remove_held_pair":
				add(domainstartup.SemanticOperationRemoveFile, pathFor("receipts", held.WorkID), nil)
				add(domainstartup.SemanticOperationRemoveFile, pathFor("dispositions", held.WorkID), nil)
			case "replace_held_disposition":
				add(domainstartup.SemanticOperationInstallFile, pathFor("dispositions", held.WorkID), failedBody(held))
			case "held_mode":
				add(domainstartup.SemanticOperationSetMode, pathFor("receipts", held.WorkID), nil)
			case "independent_closed_pair", "wrong_address":
				body, err := domainpendingwork.PendingWorkReceiptV1Bytes(independent)
				if err != nil {
					t.Fatal(err)
				}
				id := independent.WorkID
				if scenario == "wrong_address" {
					id = domainsecurity.SHA256Hex([]byte("wrong-address"))
				}
				add(domainstartup.SemanticOperationInstallFile, pathFor("receipts", id), body)
				add(domainstartup.SemanticOperationInstallFile, pathFor("dispositions", independent.WorkID), failedBody(independent))
			case "orphan_disposition":
				add(domainstartup.SemanticOperationInstallFile, pathFor("dispositions", independent.WorkID), failedBody(independent))
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-semantic-binding"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, operations)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return append([]byte(nil), bodies[operation.Path]...), nil
			}, "")
			if scenario == "independent_closed_pair" {
				if err != nil {
					t.Fatalf("independent complete signed pair was denied: %v", err)
				}
			} else if err == nil {
				t.Fatal("semantic private pending program bypassed original hold or complete graph validation")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("pending semantic observation wrote durable state")
			}
		})
	}
}

func TestRuntimeSemanticPendingPreservationRejectsApplyBeforeIndependentPrefix(t *testing.T) {
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-pending-semantic-configuration"))
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
	held := core.pendingInventory.Receipts[0]
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "a-semantic-independent.bin"), []byte("independent-after"), 0o600); err != nil {
			return err
		}
		return os.Remove(filepath.Join(stage.DataDir, "private", "pending-work", "dispositions", held.WorkID[:2], held.WorkID+".json"))
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	plan := prepared.Plan()
	if len(plan.Operations) != 2 || plan.Operations[0].Path != "data/private/a-semantic-independent.bin" {
		t.Fatal("fixture does not put independent write before held pending change")
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if err := prepared.Apply(ctx); err == nil {
		t.Fatal("root-bound semantic Apply accepted a held pending mutation")
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("root-bound semantic Apply wrote its independent prefix before refusal")
	}
}
