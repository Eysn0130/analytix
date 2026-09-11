//go:build darwin || linux

package runtimeapp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

func runtimeAcceptedFinalSemanticFixtureV1(t *testing.T) (*runtimeChildIdentityStartupV1, terminaltest.FixtureV1, terminaltest.FixtureV1, terminaltest.FixtureV1) {
	t.Helper()
	core, held, independent := runtimeTerminalSemanticFixtureV1(t)
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, held.PrivateFinal.AcceptedFinal.AcceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	second, err := terminaltest.NewFixtureWithAuthorityV1(held.Context, at.Add(time.Minute), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(context.Background(), body) })
	if err != nil || second.PrivateFinal.AcceptedFinal.RecordDigest == held.PrivateFinal.AcceptedFinal.RecordDigest {
		t.Fatalf("fixture lacks a distinct same-context candidate: %v", err)
	}
	store, err := finalauthority.NewPrivateStore(filepath.Join(core.roots.DataDir, "private", "accepted-finals"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []domainevidence.PrivateAcceptedFinalRecord{held.PrivateFinal, second.PrivateFinal} {
		if err := store.PutIfAbsent(context.Background(), record); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PutDispositionIfAbsent(context.Background(), held.AcceptedDisposition); err != nil {
		t.Fatal(err)
	}
	return core, held, second, independent
}

func TestRuntimeSemanticAcceptedFinalPreservationChecksOriginalAndCompleteCandidate(t *testing.T) {
	for _, scenario := range []string{"remove_held_pair", "remove_second_same_context", "held_mode", "fill_held_disposition", "new_held_candidate", "independent_pair", "orphan_disposition", "foreign_key_pair", "wrong_address", "noncanonical"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, held, second, independent := runtimeAcceptedFinalSemanticFixtureV1(t)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			pathFor := func(leaf, id string) string {
				return "data/private/accepted-finals/" + leaf + "/" + id[:2] + "/" + id + ".json"
			}
			bodies := map[string][]byte{}
			operations := []domainstartup.SemanticStartupOperationV1{}
			state := func(body []byte) domainstartup.SemanticEntryStateV1 {
				return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			}
			add := func(kind, leaf, id string, body []byte) {
				t.Helper()
				path := pathFor(leaf, id)
				before := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				if original, err := os.ReadFile(filepath.Join(core.roots.DataDir, strings.TrimPrefix(path, "data/"))); err == nil {
					before = state(original)
				} else if !os.IsNotExist(err) {
					t.Fatal(err)
				}
				after := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				if body != nil {
					after = state(body)
					bodies[path] = body
				}
				if kind == domainstartup.SemanticOperationSetMode {
					after = before
					after.Mode = 0o400
				}
				operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: kind, Path: path, Before: before, After: after})
			}
			addPair := func(fixture terminaltest.FixtureV1, address string, noncanonical bool) {
				t.Helper()
				recordBody, err := domainevidence.PrivateAcceptedFinalRecordBytes(fixture.PrivateFinal)
				if err != nil {
					t.Fatal(err)
				}
				dispositionBody, err := domainevidence.AcceptedFinalDispositionRecordBytes(fixture.AcceptedDisposition)
				if err != nil {
					t.Fatal(err)
				}
				if noncanonical {
					recordBody = append(recordBody, ' ')
				}
				add(domainstartup.SemanticOperationInstallFile, "records", address, recordBody)
				add(domainstartup.SemanticOperationInstallFile, "dispositions", fixture.PrivateFinal.AcceptedFinal.RecordDigest, dispositionBody)
			}
			switch scenario {
			case "remove_held_pair":
				for _, leaf := range []string{"records", "dispositions"} {
					add(domainstartup.SemanticOperationRemoveFile, leaf, held.PrivateFinal.AcceptedFinal.RecordDigest, nil)
				}
			case "remove_second_same_context":
				add(domainstartup.SemanticOperationRemoveFile, "records", second.PrivateFinal.AcceptedFinal.RecordDigest, nil)
			case "held_mode":
				add(domainstartup.SemanticOperationSetMode, "records", second.PrivateFinal.AcceptedFinal.RecordDigest, nil)
			case "fill_held_disposition":
				body, err := domainevidence.AcceptedFinalDispositionRecordBytes(second.AcceptedDisposition)
				if err != nil {
					t.Fatal(err)
				}
				add(domainstartup.SemanticOperationInstallFile, "dispositions", second.PrivateFinal.AcceptedFinal.RecordDigest, body)
			case "new_held_candidate":
				key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
				if err != nil {
					t.Fatal(err)
				}
				at, err := time.Parse(time.RFC3339Nano, second.PrivateFinal.AcceptedFinal.AcceptedAt)
				if err != nil {
					t.Fatal(err)
				}
				third, err := terminaltest.NewFixtureWithAuthorityV1(held.Context, at.Add(time.Minute), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				addPair(third, third.PrivateFinal.AcceptedFinal.RecordDigest, false)
			case "independent_pair", "wrong_address", "noncanonical":
				id := independent.PrivateFinal.AcceptedFinal.RecordDigest
				if scenario == "wrong_address" {
					id = domainsecurity.SHA256Hex([]byte("wrong-accepted-address"))
				}
				addPair(independent, id, scenario == "noncanonical")
			case "orphan_disposition":
				body, err := domainevidence.AcceptedFinalDispositionRecordBytes(independent.AcceptedDisposition)
				if err != nil {
					t.Fatal(err)
				}
				add(domainstartup.SemanticOperationInstallFile, "dispositions", independent.PrivateFinal.AcceptedFinal.RecordDigest, body)
			case "foreign_key_pair":
				foreign, err := terminaltest.NewFixtureV1()
				if err != nil {
					t.Fatal(err)
				}
				addPair(foreign, foreign.PrivateFinal.AcceptedFinal.RecordDigest, false)
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-accepted-plan"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, operations)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return bodies[operation.Path], nil
			}, "")
			if scenario == "independent_pair" {
				if err != nil {
					t.Fatalf("independent accepted-final pair rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("accepted-final semantic program bypassed original hold or complete current-key graph")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("accepted-final observation changed original files")
			}
		})
	}
}

func runtimeWriteAcceptedSemanticRecordForTestV1(t *testing.T, data, leaf, id string, body []byte) {
	t.Helper()
	path := filepath.Join(data, "private", "accepted-finals", leaf, id[:2], id+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeSemanticAcceptedFinalPreservationAppliesIndependentAndRejectsHeldBeforeEffects(t *testing.T) {
	for _, heldProgram := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent", true: "held_refusal"}[heldProgram], func(t *testing.T) {
			ctx := context.Background()
			core, held, _, independent := runtimeAcceptedFinalSemanticFixtureV1(t)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-accepted-real-apply"))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			root, err := persistencefs.FreezeRootAuthority(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			recordBody, err := domainevidence.PrivateAcceptedFinalRecordBytes(independent.PrivateFinal)
			if err != nil {
				t.Fatal(err)
			}
			dispositionBody, err := domainevidence.AcceptedFinalDispositionRecordBytes(independent.AcceptedDisposition)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "a-accepted-prefix.bin"), []byte("independent"), 0o600); err != nil {
					return err
				}
				if heldProgram {
					id := held.PrivateFinal.AcceptedFinal.RecordDigest
					for _, leaf := range []string{"records", "dispositions"} {
						if err := os.Remove(filepath.Join(stage.DataDir, "private", "accepted-finals", leaf, id[:2], id+".json")); err != nil {
							return err
						}
					}
					return nil
				}
				id := independent.PrivateFinal.AcceptedFinal.RecordDigest
				runtimeWriteAcceptedSemanticRecordForTestV1(t, stage.DataDir, "records", id, recordBody)
				runtimeWriteAcceptedSemanticRecordForTestV1(t, stage.DataDir, "dispositions", id, dispositionBody)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = prepared.Apply(ctx)
			after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			if heldProgram {
				if err == nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("held accepted-final program crossed an effect: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for leaf, expected := range map[string][]byte{"records": recordBody, "dispositions": dispositionBody} {
				id := independent.PrivateFinal.AcceptedFinal.RecordDigest
				body, err := os.ReadFile(filepath.Join(core.roots.DataDir, "private", "accepted-finals", leaf, id[:2], id+".json"))
				if err != nil || !reflect.DeepEqual(body, expected) {
					t.Fatal("independent accepted-final pair was not installed exactly")
				}
			}
			for path, original := range before {
				if !semanticOriginalRecordUnchangedForTestV1(original, after[path]) {
					t.Fatalf("independent accepted-final apply changed an original file or mode at %s", path)
				}
			}
		})
	}
}

func TestRuntimeSemanticAcceptedFinalPreservationSeparatesOriginalAndFutureWinnerGraphs(t *testing.T) {
	for _, scenario := range []string{"new_independent_losing_graph", "preexisting_losing_orphan", "held_orphan_future_repair"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, _, second, independent := runtimeAcceptedFinalSemanticFixtureV1(t)
			key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			sign := func(body []byte) ([]byte, error) { return key.Sign(ctx, body) }
			at, err := time.Parse(time.RFC3339Nano, independent.PrivateFinal.AcceptedFinal.AcceptedAt)
			if err != nil {
				t.Fatal(err)
			}
			winner, err := terminaltest.NewFixtureWithAuthorityV1(independent.Context, at.Add(time.Minute), key.PublicKey(), sign)
			if err != nil {
				t.Fatal(err)
			}
			loser := independent
			if loser.PrivateFinal.AcceptedFinal.RecordDigest > winner.PrivateFinal.AcceptedFinal.RecordDigest {
				loser, winner = winner, loser
			}
			observation, err := domainevidence.NewAcceptedFinalCASObservationV1(domainevidence.AcceptedFinalCASObservationV1{ThreadID: loser.Context.ThreadID, TurnID: loser.Context.TurnID, Status: "completed", FrozenContext: loser.Context, CurrentContext: loser.Context, HasWinner: true, Winner: winner.PrivateFinal.AcceptedFinal, ThreadFileSHA256: domainsecurity.SHA256Hex([]byte("synthetic-original-thread")), TurnProjectionSHA256: domainsecurity.SHA256Hex([]byte("synthetic-original-projection"))})
			if err != nil {
				t.Fatal(err)
			}
			losing, err := domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{AcceptedFinal: loser.PrivateFinal.AcceptedFinal, State: domainevidence.AcceptedFinalExplicitlyNotCommitted, EventManifestDigest: loser.EventManifestDigest, DecidedAt: at.Add(3 * time.Minute), AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey()}, observation, domainevidence.AcceptedFinalDecisionDifferentPublicWinner, sign)
			if err != nil {
				t.Fatal(err)
			}
			records := []domainevidence.PrivateAcceptedFinalRecord{loser.PrivateFinal, winner.PrivateFinal}
			disposition := losing
			if scenario == "held_orphan_future_repair" {
				records = []domainevidence.PrivateAcceptedFinalRecord{second.PrivateFinal}
				disposition = second.AcceptedDisposition
				id := second.PrivateFinal.AcceptedFinal.RecordDigest
				if err := os.Remove(filepath.Join(core.roots.DataDir, "private", "accepted-finals", "records", id[:2], id+".json")); err != nil {
					t.Fatal(err)
				}
			}
			dispositionBody, err := domainevidence.AcceptedFinalDispositionRecordBytes(disposition)
			if err != nil {
				t.Fatal(err)
			}
			pathFor := func(leaf, id string) string {
				return filepath.Join("private", "accepted-finals", leaf, id[:2], id+".json")
			}
			installed, absent := pathFor("dispositions", disposition.AcceptedFinalDigest), pathFor("records", records[0].AcceptedFinal.RecordDigest)
			if scenario != "new_independent_losing_graph" {
				// Explicit malformed pre-transaction fixture; never a live store mutation.
				runtimeWriteAcceptedSemanticRecordForTestV1(t, core.roots.DataDir, "dispositions", disposition.AcceptedFinalDigest, dispositionBody)
				if scenario == "preexisting_losing_orphan" {
					installed, absent = pathFor("records", loser.PrivateFinal.AcceptedFinal.RecordDigest), pathFor("records", winner.PrivateFinal.AcceptedFinal.RecordDigest)
				} else {
					installed = "private/a-held-repair-prefix.bin"
				}
			}
			runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				for _, record := range records {
					body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
					if err != nil {
						return err
					}
					runtimeWriteAcceptedSemanticRecordForTestV1(t, stage.DataDir, "records", record.AcceptedFinal.RecordDigest, body)
				}
				if scenario == "new_independent_losing_graph" {
					runtimeWriteAcceptedSemanticRecordForTestV1(t, stage.DataDir, "dispositions", disposition.AcceptedFinalDigest, dispositionBody)
				}
				if scenario == "held_orphan_future_repair" {
					return os.WriteFile(filepath.Join(stage.DataDir, installed), []byte("applied"), 0o600)
				}
				return nil
			}, installed, absent)
			freshCore, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, freshCore)
			if scenario != "new_independent_losing_graph" {
				if err == nil || !strings.Contains(err.Error(), "exact private accepted-final authority") || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
					t.Fatalf("signed future records washed away an original orphan: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("authenticated independent losing graph rejected: %v", err)
			}
			root, err := persistencefs.FreezeRootAuthority(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved).RecoverAuthenticatedExisting(ctx); err != nil {
				t.Fatal(err)
			}
			after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for path, original := range before {
				if !semanticOriginalRecordUnchangedForTestV1(original, after[path]) {
					t.Fatal("accepted-final recovery changed an original file or mode")
				}
			}
			for _, record := range records {
				expected, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := os.ReadFile(filepath.Join(core.roots.DataDir, pathFor("records", record.AcceptedFinal.RecordDigest)))
				if err != nil || !reflect.DeepEqual(expected, actual) {
					t.Fatal("independent losing/winner graph was not recovered exactly")
				}
			}
		})
	}
}
