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

	telemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	telemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	domaintelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	telemetryport "analytix.local/runtime-go/internal/ports/cachetelemetry"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type runtimeTelemetrySemanticRecordsV1 struct {
	intent     domaintelemetry.ProviderAttemptIntentV1
	settlement domaintelemetry.ProviderAttemptSettlementV1
	closure    domaintelemetry.ProviderTurnClosureV1
}

func runtimeTelemetrySemanticRegistrationV1(frozen domainsecurity.TurnSecurityContext, at time.Time, logical uint64, attempt uint32) telemetryport.AttemptRegistrationInputV1 {
	return telemetryport.AttemptRegistrationInputV1{
		SecurityContext: frozen, OrdinaryEffect: !domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(frozen), UsageSource: domaintelemetry.ProviderUsageSourceTurn,
		Channel: domaintelemetry.ProviderChannelPrimary, LogicalSequence: logical, OuterAttempt: 1, PhysicalAttempt: attempt,
		ProviderFamily: domaintelemetry.ProviderFamilyDeepSeek, EndpointFormat: domaintelemetry.EndpointFormatChatCompletions,
		Model: []byte("synthetic-model"), Endpoint: []byte("https://provider.invalid/v1"), WireBody: []byte(`{"messages":[{"role":"user","content":"synthetic"}]}`),
		WireHeaders: []byte(`{"x-test":"synthetic"}`), CredentialScope: []byte("synthetic"), ProviderConfig: []byte(`{"model":"synthetic-model"}`), StartedAt: at,
	}
}

func runtimeTelemetrySemanticCompleteV1(t *testing.T, service *telemetryapp.DurableService, store *telemetrystore.Store, frozen domainsecurity.TurnSecurityContext, at time.Time) runtimeTelemetrySemanticRecordsV1 {
	t.Helper()
	ctx := context.Background()
	handle, err := service.BeginAttempt(ctx, runtimeTelemetrySemanticRegistrationV1(frozen, at, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SettleAttempt(ctx, telemetryport.AttemptSettlementInputV1{Handle: handle, DispatchState: domaintelemetry.ProviderDispatchStateSent, Status: domaintelemetry.ProviderCallStatusSucceeded, SafeReasonCode: "provider_succeeded", SettledAt: at.Add(time.Second), Usage: domaintelemetry.ProviderUsageV1{InputTokens: domaintelemetry.TokenCountV1{Known: true, Value: 1}, OutputTokens: domaintelemetry.TokenCountV1{Known: true, Value: 1}, CacheHitTokens: domaintelemetry.TokenCountV1{Known: true}, CacheMissTokens: domaintelemetry.TokenCountV1{Known: true, Value: 1}}}); err != nil {
		t.Fatal(err)
	}
	settlement, err := store.ReadSettlement(ctx, handle.Intent.IntentID)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := service.CloseTurn(ctx, frozen, domaintelemetry.ProviderTurnTerminalSuccessV1, at.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return runtimeTelemetrySemanticRecordsV1{intent: handle.Intent, settlement: settlement, closure: closure}
}

func runtimeTelemetrySemanticFixtureV1(t *testing.T) (*runtimeChildIdentityStartupV1, runtimeTelemetrySemanticRecordsV1, runtimeTelemetrySemanticRecordsV1) {
	t.Helper()
	ctx := context.Background()
	core, _ := runtimeReportPreservationFixtureV1(t, false, true, false)
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	store, err := telemetrystore.NewStore(filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	service, err := telemetryapp.NewDurableService(key, store)
	if err != nil {
		t.Fatal(err)
	}
	frozen := scope.Contexts()[0]
	at, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	held := runtimeTelemetrySemanticCompleteV1(t, service, store, frozen, at.Add(time.Second))
	observer, ok := core.verification.(interface {
		ObserveProviderTurnBindingHMACV1(context.Context, domainsecurity.TurnSecurityContext) (string, error)
	})
	if !ok {
		t.Fatal("fixture lacks the existing narrow HMAC observer")
	}
	binding, err := observer.ObserveProviderTurnBindingHMACV1(ctx, frozen)
	if err != nil || binding != held.intent.TurnBindingHMAC {
		t.Fatal("actual durable producer differs from original context HMAC observation")
	}
	// Establish an original open held attempt before any semantic transaction.
	for leaf, id := range map[string]string{"settlements": held.intent.IntentID, "turn-closures": held.closure.TurnBindingHMAC} {
		if err := os.Remove(filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry", leaf, id[:2], id+".json")); err != nil {
			t.Fatal(err)
		}
	}
	independentContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thread-independent-telemetry", TurnID: "turn-independent-telemetry", WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	tempPrivate := filepath.Join(t.TempDir(), "private")
	access, err := privatecastest.NewAccessAuthority(tempPrivate)
	if err != nil {
		t.Fatal(err)
	}
	independentStore, err := telemetrystore.NewStore(filepath.Join(tempPrivate, "provider-cache-telemetry"), access)
	if err != nil {
		t.Fatal(err)
	}
	independentService, err := telemetryapp.NewDurableService(key, independentStore)
	if err != nil {
		t.Fatal(err)
	}
	return core, held, runtimeTelemetrySemanticCompleteV1(t, independentService, independentStore, independentContext, at.Add(time.Second))
}

func runtimeTelemetrySemanticBodiesV1(t *testing.T, records runtimeTelemetrySemanticRecordsV1) map[string][]byte {
	t.Helper()
	attempt, err := domaintelemetry.ProviderAttemptIntentV1Bytes(records.intent)
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := domaintelemetry.ProviderAttemptSettlementV1Bytes(records.settlement)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := domaintelemetry.ProviderTurnClosureV1Bytes(records.closure)
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{"attempts": attempt, "settlements": settlement, "turn-closures": closure}
}

func TestRuntimeSemanticTelemetryPreservationChecksOriginalAndCompleteCandidate(t *testing.T) {
	for _, scenario := range []string{"remove_held_attempt", "held_mode", "settle_held", "close_held", "close_empty_held", "independent_closed_graph", "foreign_key", "orphan_settlement", "orphan_closure", "wrong_settlement_address", "wrong_closure_address", "noncanonical"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core, held, independent := runtimeTelemetrySemanticFixtureV1(t)
			if scenario == "foreign_key" || scenario == "close_empty_held" {
				private := filepath.Join(t.TempDir(), "private")
				access, err := privatecastest.NewAccessAuthority(private)
				if err != nil {
					t.Fatal(err)
				}
				keyPath := filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json")
				if scenario == "foreign_key" {
					keyPath = filepath.Join(private, "authority", "final-answer-ed25519-v1.json")
				}
				key, err := finalauthority.OpenOrCreateFileAuthority(keyPath, scenario != "foreign_key")
				if err != nil {
					t.Fatal(err)
				}
				store, err := telemetrystore.NewStore(filepath.Join(private, "provider-cache-telemetry"), access)
				if err != nil {
					t.Fatal(err)
				}
				service, err := telemetryapp.NewDurableService(key, store)
				if err != nil {
					t.Fatal(err)
				}
				scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
				if err != nil {
					t.Fatal(err)
				}
				frozen := scope.Contexts()[0]
				at, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "foreign_key" {
					independent = runtimeTelemetrySemanticCompleteV1(t, service, store, frozen, at.Add(time.Second))
				} else {
					held.closure, err = service.CloseTurn(ctx, frozen, domaintelemetry.ProviderTurnTerminalSuccessV1, at.Add(time.Second))
					if err != nil {
						t.Fatal(err)
					}
					// Establish a zero-attempt original before preparing the transaction.
					id := held.intent.IntentID
					if err := os.Remove(filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry", "attempts", id[:2], id+".json")); err != nil {
						t.Fatal(err)
					}
				}
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			bodies := map[string][]byte{}
			operations := []domainstartup.SemanticStartupOperationV1{}
			state := func(body []byte) domainstartup.SemanticEntryStateV1 {
				return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			}
			add := func(kind, leaf, id string, body []byte) {
				t.Helper()
				path := "data/private/provider-cache-telemetry/" + leaf + "/" + id[:2] + "/" + id + ".json"
				before := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				if original, err := os.ReadFile(filepath.Join(core.roots.DataDir, strings.TrimPrefix(path, "data/"))); err == nil {
					before = state(original)
				} else if !os.IsNotExist(err) {
					t.Fatal(err)
				}
				after := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
				if body != nil {
					after, bodies[path] = state(body), body
				}
				if kind == domainstartup.SemanticOperationSetMode {
					after = before
					after.Mode = 0o400
				}
				operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: kind, Path: path, Before: before, After: after})
			}
			switch scenario {
			case "remove_held_attempt":
				add(domainstartup.SemanticOperationRemoveFile, "attempts", held.intent.IntentID, nil)
			case "held_mode":
				add(domainstartup.SemanticOperationSetMode, "attempts", held.intent.IntentID, nil)
			case "close_empty_held":
				body, err := domaintelemetry.ProviderTurnClosureV1Bytes(held.closure)
				if err != nil {
					t.Fatal(err)
				}
				add(domainstartup.SemanticOperationInstallFile, "turn-closures", held.closure.TurnBindingHMAC, body)
			case "settle_held", "close_held":
				body := runtimeTelemetrySemanticBodiesV1(t, held)
				add(domainstartup.SemanticOperationInstallFile, "settlements", held.intent.IntentID, body["settlements"])
				if scenario == "close_held" {
					add(domainstartup.SemanticOperationInstallFile, "turn-closures", held.closure.TurnBindingHMAC, body["turn-closures"])
				}
			default:
				body := runtimeTelemetrySemanticBodiesV1(t, independent)
				for leaf, data := range body {
					if scenario == "orphan_settlement" && leaf != "settlements" || scenario == "orphan_closure" && leaf != "turn-closures" {
						continue
					}
					id := independent.intent.IntentID
					if leaf == "turn-closures" {
						id = independent.closure.TurnBindingHMAC
					}
					if scenario == "wrong_settlement_address" && leaf == "settlements" || scenario == "wrong_closure_address" && leaf == "turn-closures" {
						id = domainsecurity.SHA256Hex([]byte("wrong-telemetry-address"))
					}
					if scenario == "noncanonical" && leaf == "attempts" {
						data = append(data, ' ')
					}
					add(domainstartup.SemanticOperationInstallFile, leaf, id, data)
				}
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-telemetry-candidate"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, operations)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return bodies[operation.Path], nil
			}, "")
			if scenario == "independent_closed_graph" {
				if err != nil {
					t.Fatalf("independent complete telemetry graph rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("telemetry semantic program bypassed original hold or complete three-leaf graph")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("telemetry semantic observation changed original state")
			}
		})
	}
}

func TestRuntimeSemanticTelemetryPreservationAppliesIndependentAndRejectsHeldBeforeEffects(t *testing.T) {
	for _, heldProgram := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent", true: "held_refusal"}[heldProgram], func(t *testing.T) {
			ctx := context.Background()
			core, held, independent := runtimeTelemetrySemanticFixtureV1(t)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-telemetry-real-apply"))
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
			records := independent
			if heldProgram {
				records = held
			}
			bodies := runtimeTelemetrySemanticBodiesV1(t, records)
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if err := os.WriteFile(filepath.Join(stage.DataDir, "private", "a-telemetry-prefix.bin"), []byte("independent"), 0o600); err != nil {
					return err
				}
				for leaf, body := range bodies {
					if heldProgram && leaf != "settlements" {
						continue
					}
					id := records.intent.IntentID
					if leaf == "turn-closures" {
						id = records.closure.TurnBindingHMAC
					}
					runtimeWriteTelemetrySemanticBodyForTestV1(t, stage.DataDir, leaf, id, body)
				}
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
					t.Fatalf("held telemetry settlement crossed an effect: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for leaf, expected := range bodies {
				id := records.intent.IntentID
				if leaf == "turn-closures" {
					id = records.closure.TurnBindingHMAC
				}
				body, err := os.ReadFile(filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry", leaf, id[:2], id+".json"))
				if err != nil || !reflect.DeepEqual(body, expected) {
					t.Fatal("independent telemetry graph was not installed exactly")
				}
			}
			for path, original := range before {
				if !semanticOriginalRecordUnchangedForTestV1(original, after[path]) {
					t.Fatalf("independent telemetry apply changed an original file or mode at %s", path)
				}
			}
		})
	}
}

func runtimeWriteTelemetrySemanticBodyForTestV1(t *testing.T, data, leaf, id string, body []byte) {
	t.Helper()
	path := filepath.Join(data, "private", "provider-cache-telemetry", leaf, id[:2], id+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeSemanticTelemetryPreservationSeparatesSignedRetryCutFromOriginalGap(t *testing.T) {
	for _, preexistingGap := range []bool{false, true} {
		t.Run(map[bool]string{false: "signed_retry_cut", true: "preexisting_gap"}[preexistingGap], func(t *testing.T) {
			ctx := context.Background()
			core, _, _ := runtimeTelemetrySemanticFixtureV1(t)
			key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
			frozen, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "independent-retry-thread", TurnID: "independent-retry-turn", WorkspaceRealPath: t.TempDir(), ContextEpoch: 1, IssuedAt: at})
			if err != nil {
				t.Fatal(err)
			}
			private := filepath.Join(t.TempDir(), "private")
			access, err := privatecastest.NewAccessAuthority(private)
			if err != nil {
				t.Fatal(err)
			}
			store, err := telemetrystore.NewStore(filepath.Join(private, "provider-cache-telemetry"), access)
			if err != nil {
				t.Fatal(err)
			}
			service, err := telemetryapp.NewDurableService(key, store)
			if err != nil {
				t.Fatal(err)
			}
			first, err := service.BeginAttempt(ctx, runtimeTelemetrySemanticRegistrationV1(frozen, at, 1, 1))
			if err != nil {
				t.Fatal(err)
			}
			if err := service.SettleAttempt(ctx, telemetryport.AttemptSettlementInputV1{Handle: first, DispatchState: domaintelemetry.ProviderDispatchStateSent, Status: domaintelemetry.ProviderCallStatusSucceeded, SafeReasonCode: "provider_succeeded", SettledAt: at.Add(time.Second)}); err != nil {
				t.Fatal(err)
			}
			settlement, err := store.ReadSettlement(ctx, first.Intent.IntentID)
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.BeginAttempt(ctx, runtimeTelemetrySemanticRegistrationV1(frozen, at.Add(2*time.Second), 1, 2))
			if err != nil {
				t.Fatal(err)
			}
			firstBody, err := domaintelemetry.ProviderAttemptIntentV1Bytes(first.Intent)
			if err != nil {
				t.Fatal(err)
			}
			secondBody, err := domaintelemetry.ProviderAttemptIntentV1Bytes(second.Intent)
			if err != nil {
				t.Fatal(err)
			}
			settlementBody, err := domaintelemetry.ProviderAttemptSettlementV1Bytes(settlement)
			if err != nil {
				t.Fatal(err)
			}
			runtimeWriteTelemetrySemanticBodyForTestV1(t, core.roots.DataDir, "attempts", first.Intent.IntentID, firstBody)
			pathFor := func(leaf, id string) string {
				return filepath.Join("private", "provider-cache-telemetry", leaf, id[:2], id+".json")
			}
			installed, absent := pathFor("attempts", second.Intent.IntentID), pathFor("settlements", first.Intent.IntentID)
			preservation := []runtimeReportRestartPreservationV1{}
			if preexistingGap {
				runtimeWriteTelemetrySemanticBodyForTestV1(t, core.roots.DataDir, "attempts", second.Intent.IntentID, secondBody)
				installed = "private/a-telemetry-gap-prefix.bin"
			} else {
				preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
				if err != nil {
					t.Fatal(err)
				}
				preservation = append(preservation, preserved)
			}
			runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				runtimeWriteTelemetrySemanticBodyForTestV1(t, stage.DataDir, "attempts", second.Intent.IntentID, secondBody)
				runtimeWriteTelemetrySemanticBodyForTestV1(t, stage.DataDir, "settlements", first.Intent.IntentID, settlementBody)
				if preexistingGap {
					return os.WriteFile(filepath.Join(stage.DataDir, installed), []byte("applied"), 0o600)
				}
				return nil
			}, installed, absent, preservation...)
			// Both cuts physically violate the same complete ledger graph.
			physical, err := telemetrystore.PrepareRecoveryV1(ctx, filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry"), core.access)
			if err != nil {
				t.Fatal(err)
			}
			if err := physical.ValidateSemantics(ctx); err == nil {
				t.Fatal("fixture did not reach a real retry-before-settlement gap")
			}
			freshCore, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			freshPreserved, err := prepareRuntimeReportRestartPreservationV1(ctx, freshCore)
			if preexistingGap {
				if err == nil || !strings.Contains(err.Error(), "telemetry open inventory") || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
					t.Fatalf("future settlement washed away original retry gap: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("fresh scope rejected signed independent retry cut: %v", err)
			}
			root, err := persistencefs.FreezeRootAuthority(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, freshPreserved).RecoverAuthenticatedExisting(ctx); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(filepath.Join(core.roots.DataDir, absent))
			if err != nil || !reflect.DeepEqual(actual, settlementBody) {
				t.Fatal("signed retry settlement was not recovered exactly")
			}
			after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for path, original := range before {
				if !semanticOriginalRecordUnchangedForTestV1(original, after[path]) {
					t.Fatal("independent retry recovery changed original file or mode")
				}
			}
			final, err := telemetrystore.PrepareRecoveryV1(ctx, filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry"), core.access)
			if err != nil {
				t.Fatal(err)
			}
			if err := final.ValidateSemantics(ctx); err != nil {
				t.Fatalf("recovered retry ledger is incomplete: %v", err)
			}
		})
	}
}
