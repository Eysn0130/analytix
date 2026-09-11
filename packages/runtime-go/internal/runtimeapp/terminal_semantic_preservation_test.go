//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	terminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

func runtimeTerminalSemanticFixtureV1(t *testing.T) (*runtimeChildIdentityStartupV1, terminaltest.FixtureV1, terminaltest.FixtureV1) {
	t.Helper()
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	heldContext := scope.Contexts()[0]
	at, err := time.Parse(time.RFC3339Nano, heldContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	independentContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thread-independent-terminal", TurnID: "turn-independent-terminal", WorkspaceRealPath: heldContext.WorkspaceRealPath, TenantID: heldContext.TenantID, UserID: heldContext.UserID, CaseID: heldContext.CaseID, CaseBindingHash: heldContext.CaseBindingHash, DatasetSnapshotID: heldContext.DatasetSnapshotID, SourceManifestHash: heldContext.SourceManifestHash, ContextEpoch: heldContext.ContextEpoch, IssuedAt: at, PublicationPolicy: heldContext.PublicationPolicy, RiskAuthorityBinding: heldContext.RiskAuthorityBinding})
	if err != nil {
		t.Fatal(err)
	}
	makeFixture := func(frozen domainsecurity.TurnSecurityContext) terminaltest.FixtureV1 {
		t.Helper()
		at, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
		if err != nil {
			t.Fatal(err)
		}
		fixture, err := terminaltest.NewFixtureWithAuthorityV1(frozen, at.Add(time.Second), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
		if err != nil {
			t.Fatal(err)
		}
		return fixture
	}
	held, independent := makeFixture(heldContext), makeFixture(independentContext)
	store, err := terminalstore.NewStore(filepath.Join(core.roots.DataDir, "private", "turn-terminal-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIntentIfAbsent(ctx, held.Intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(ctx, held.Disposition); err != nil {
		t.Fatal(err)
	}
	return core, held, independent
}

type cancelAfterTerminalDispositionContextV1 struct {
	context.Context
	cancel          context.CancelFunc
	dispositionPath string
	intentPath      string
}

func (ctx cancelAfterTerminalDispositionContextV1) Err() error {
	if _, err := os.Stat(ctx.dispositionPath); err == nil {
		if _, err := os.Stat(ctx.intentPath); os.IsNotExist(err) {
			ctx.cancel()
		}
	}
	return ctx.Context.Err()
}

func TestRuntimeSemanticTerminalPreservationResumesIndependentInterruptedPair(t *testing.T) {
	ctx := context.Background()
	core, _, independent := runtimeTerminalSemanticFixtureV1(t)
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-terminal-interrupted-configuration"))
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
	intentBody, err := domainturnterminal.TurnTerminalIntentV1Bytes(independent.Intent)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := domainturnterminal.TurnTerminalDispositionV1Bytes(independent.Disposition)
	if err != nil {
		t.Fatal(err)
	}
	pathFor := func(data, leaf string) string {
		id := independent.Context.ContextDigest
		return filepath.Join(data, "private", "turn-terminal-authority", leaf, id[:2], id+".json")
	}
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		for leaf, body := range map[string][]byte{"intents": intentBody, "dispositions": dispositionBody} {
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
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: pathFor(core.roots.DataDir, "dispositions"), intentPath: pathFor(core.roots.DataDir, "intents")}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a real cancellation after disposition install: %v", err)
	}
	if body, err := os.ReadFile(cut.dispositionPath); err != nil || !reflect.DeepEqual(body, dispositionBody) {
		t.Fatal("signed disposition operation did not reach its physical After state")
	}
	if _, err := os.Stat(cut.intentPath); !os.IsNotExist(err) {
		t.Fatal("fixture did not stop before independent intent installation")
	}
	beforeRecovery := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	freshAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	freshCore, err := prepareRuntimeChildIdentityStartupV1(ctx, core.roots, freshAuthority, core.access, nil)
	if err != nil {
		t.Fatal(err)
	}
	freshPreserved, err := prepareRuntimeReportRestartPreservationV1(ctx, freshCore)
	if err != nil {
		t.Fatalf("fresh root preservation rejected an authenticated independent interrupted pair before replay: %v", err)
	}
	freshJournalAuthority, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	recovery := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, freshAuthority, freshJournalAuthority, freshPreserved)
	if err := recovery.RecoverAuthenticatedExisting(ctx); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(cut.intentPath); err != nil || !reflect.DeepEqual(body, intentBody) {
		t.Fatal("independent terminal intent was not recovered exactly")
	}
	after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	for path, record := range beforeRecovery {
		if !semanticOriginalRecordUnchangedForTestV1(record, after[path]) {
			t.Fatalf("independent recovery changed an existing record at %s", path)
		}
	}
}

func TestRuntimeSemanticTerminalPreservationAppliesIndependentPairAndRejectsHeldProgramBeforeEffects(t *testing.T) {
	for _, independent := range []bool{false, true} {
		t.Run(map[bool]string{false: "held_removal", true: "independent_pair"}[independent], func(t *testing.T) {
			ctx := context.Background()
			core, held, other := runtimeTerminalSemanticFixtureV1(t)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-terminal-semantic-configuration"))
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
			intentBody, err := domainturnterminal.TurnTerminalIntentV1Bytes(other.Intent)
			if err != nil {
				t.Fatal(err)
			}
			dispositionBody, err := domainturnterminal.TurnTerminalDispositionV1Bytes(other.Disposition)
			if err != nil {
				t.Fatal(err)
			}
			privatePath := func(data, leaf, id string) string {
				return filepath.Join(data, "private", "turn-terminal-authority", leaf, id[:2], id+".json")
			}
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "a-semantic-independent.bin"), []byte("independent-after"), 0o600); err != nil {
					return err
				}
				if !independent {
					return os.Remove(privatePath(stage.DataDir, "dispositions", held.Context.ContextDigest))
				}
				for leaf, body := range map[string][]byte{"intents": intentBody, "dispositions": dispositionBody} {
					path := privatePath(stage.DataDir, leaf, other.Context.ContextDigest)
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
			plan := prepared.Plan()
			prefixIndex, terminalIndex := -1, -1
			for index, operation := range plan.Operations {
				if operation.Path == "data/private/a-semantic-independent.bin" {
					prefixIndex = index
				}
				if terminalIndex < 0 && operation.Kind != domainstartup.SemanticOperationCreateDirectory && strings.HasPrefix(operation.Path, "data/private/turn-terminal-authority/") {
					terminalIndex = index
				}
			}
			if prefixIndex < 0 || terminalIndex <= prefixIndex {
				t.Fatal("fixture lacks independent file prefix before terminal operations")
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = prepared.Apply(ctx)
			if !independent {
				if err == nil {
					t.Fatal("root-bound Apply accepted a held terminal mutation")
				}
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
					t.Fatal("held terminal refusal occurred after a durable write")
				}
				return
			}
			if err != nil {
				t.Fatalf("independent terminal pair Apply failed: %v", err)
			}
			for leaf, expected := range map[string][]byte{"intents": intentBody, "dispositions": dispositionBody} {
				body, err := os.ReadFile(privatePath(core.roots.DataDir, leaf, other.Context.ContextDigest))
				if err != nil || !reflect.DeepEqual(body, expected) {
					t.Fatal("independent terminal pair was not applied exactly")
				}
			}
			after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for path, record := range before {
				if !semanticOriginalRecordUnchangedForTestV1(record, after[path]) {
					t.Fatalf("independent transaction changed an original record at %s", path)
				}
			}
		})
	}
}

func TestRuntimeSemanticTerminalPreservationChecksOriginalAndCompleteCandidateInventory(t *testing.T) {
	for _, scenario := range []string{"remove_held_pair", "held_mode", "independent_pair", "orphan_disposition", "foreign_key_pair", "wrong_address", "new_held_context", "original_held_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, held, independent := runtimeTerminalSemanticFixtureV1(t)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			pathFor := func(leaf, id string) string {
				return "data/private/turn-terminal-authority/" + leaf + "/" + id[:2] + "/" + id + ".json"
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
			addPair := func(fixture terminaltest.FixtureV1, wrong bool) {
				t.Helper()
				intentBody, err := domainturnterminal.TurnTerminalIntentV1Bytes(fixture.Intent)
				if err != nil {
					t.Fatal(err)
				}
				dispositionBody, err := domainturnterminal.TurnTerminalDispositionV1Bytes(fixture.Disposition)
				if err != nil {
					t.Fatal(err)
				}
				id := fixture.Context.ContextDigest
				if wrong {
					id = domainsecurity.SHA256Hex([]byte("wrong-terminal-address"))
				}
				add(domainstartup.SemanticOperationInstallFile, pathFor("intents", id), intentBody)
				add(domainstartup.SemanticOperationInstallFile, pathFor("dispositions", fixture.Context.ContextDigest), dispositionBody)
			}
			switch scenario {
			case "remove_held_pair":
				add(domainstartup.SemanticOperationRemoveFile, pathFor("intents", held.Context.ContextDigest), nil)
				add(domainstartup.SemanticOperationRemoveFile, pathFor("dispositions", held.Context.ContextDigest), nil)
			case "held_mode":
				add(domainstartup.SemanticOperationSetMode, pathFor("intents", held.Context.ContextDigest), nil)
			case "independent_pair", "wrong_address":
				addPair(independent, scenario == "wrong_address")
			case "orphan_disposition":
				body, err := domainturnterminal.TurnTerminalDispositionV1Bytes(independent.Disposition)
				if err != nil {
					t.Fatal(err)
				}
				add(domainstartup.SemanticOperationInstallFile, pathFor("dispositions", independent.Context.ContextDigest), body)
			case "foreign_key_pair":
				foreign, err := terminaltest.NewFixtureV1()
				if err != nil {
					t.Fatal(err)
				}
				addPair(foreign, false)
			case "new_held_context":
				frozen := held.Context
				at, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				frozen, err = domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: frozen.ThreadID, TurnID: "turn-unobserved-held", WorkspaceRealPath: frozen.WorkspaceRealPath, TenantID: frozen.TenantID, UserID: frozen.UserID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, DatasetSnapshotID: frozen.DatasetSnapshotID, SourceManifestHash: frozen.SourceManifestHash, ContextEpoch: frozen.ContextEpoch, IssuedAt: at, PublicationPolicy: frozen.PublicationPolicy, RiskAuthorityBinding: frozen.RiskAuthorityBinding})
				if err != nil {
					t.Fatal(err)
				}
				authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
				if err != nil {
					t.Fatal(err)
				}
				fixture, err := terminaltest.NewFixtureWithAuthorityV1(frozen, at.Add(time.Second), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				addPair(fixture, false)
			case "original_held_drift":
				if err := os.Remove(filepath.Join(core.roots.DataDir, pathFor("dispositions", held.Context.ContextDigest)[len("data/"):])); err != nil {
					t.Fatal(err)
				}
				addPair(independent, false)
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-terminal-semantic-binding"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, operations)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return append([]byte(nil), bodies[operation.Path]...), nil
			}, "")
			if scenario == "independent_pair" {
				if err != nil {
					t.Fatalf("independent complete terminal pair denied: %v", err)
				}
			} else if err == nil {
				t.Fatal("semantic terminal program bypassed original hold or full current-key graph validation")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("terminal semantic observation wrote durable state")
			}
		})
	}
}

// Shared directory sizes change when an independent shard is added. Preserve
// their original modes and every original file's bytes, mode and size.
func semanticOriginalRecordUnchangedForTestV1(before, after string) bool {
	original := strings.Split(before, ":")
	current := strings.Split(after, ":")
	if len(original) == 2 {
		return len(current) == 2 && original[0] == current[0]
	}
	return before == after
}

func TestRuntimeSemanticTerminalPreservationRejectsPreexistingOrphanDespiteSignedFutureRepair(t *testing.T) {
	ctx := context.Background()
	core, _, independent := runtimeTerminalSemanticFixtureV1(t)
	intentBody, err := domainturnterminal.TurnTerminalIntentV1Bytes(independent.Intent)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := domainturnterminal.TurnTerminalDispositionV1Bytes(independent.Disposition)
	if err != nil {
		t.Fatal(err)
	}
	id := independent.Context.ContextDigest
	// Deliberately inject a canonical orphan into the isolated fixture. The
	// normal live store correctly refuses this graph and is not used to mint it.
	orphanPath := filepath.Join(core.roots.DataDir, "private", "turn-terminal-authority", "dispositions", id[:2], id+".json")
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, dispositionBody, 0o600); err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if _, err := prepareRuntimeReportRestartPreservationV1(ctx, core); err == nil {
		t.Fatal("original orphan without a signed journal was accepted")
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("unexplained orphan refusal changed state")
	}
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-unexplained-orphan-future-repair"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	// An older unguarded producer can sign a future repair. Its unrelated
	// prefix does not prove that this journal created the preexisting orphan.
	builder := persistencefs.NewSemanticPlanBuilder(core.roots)
	intentPath := func(data string) string {
		return filepath.Join(data, "private", "turn-terminal-authority", "intents", id[:2], id+".json")
	}
	prefixPath := func(data string) string { return filepath.Join(data, "private", "a-terminal-unrelated.bin") }
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		if err := os.WriteFile(prefixPath(stage.DataDir), []byte("unrelated-prefix"), 0o600); err != nil {
			return err
		}
		path := intentPath(stage.DataDir)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		return os.WriteFile(path, intentBody, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: prefixPath(core.roots.DataDir), intentPath: intentPath(core.roots.DataDir)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("unrelated signed prefix did not reach cancellation: %v", err)
	}
	if _, err := os.Stat(cut.dispositionPath); err != nil {
		t.Fatal("unrelated prefix did not land")
	}
	if _, err := os.Stat(cut.intentPath); !os.IsNotExist(err) {
		t.Fatal("future repair intent was already applied")
	}
	freshAuthority, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	freshCore, err := prepareRuntimeChildIdentityStartupV1(ctx, core.roots, freshAuthority, core.access, nil)
	if err != nil {
		t.Fatal(err)
	}
	before = startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if _, err := prepareRuntimeReportRestartPreservationV1(ctx, freshCore); err == nil || !strings.Contains(err.Error(), "not explained by an applied signed operation") {
		t.Fatalf("future journal repair laundered an original orphan: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("unexplained signed orphan refusal changed state")
	}
}
