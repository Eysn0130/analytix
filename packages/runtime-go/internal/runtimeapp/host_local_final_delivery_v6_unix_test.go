//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	cachetelemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	"analytix.local/runtime-go/internal/server"
)

func runtimeHostLocalPrepareFinalTurnV6(t *testing.T, config Config, workspace string,
	observation domainsecurity.CaseBindingObservationV1, snapshot datasetsnapshotport.ResolvedSnapshotV2,
) (domainsecurity.TurnSecurityContext, *server.DurableEventSessionStore) {
	t.Helper()
	durable, err := server.NewTempDurableEventSessionStore(config.DurableTempDir)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := durable.CreateThread(map[string]any{"title": "host-local Final V6"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	securityContext := runtimeHostLocalFactContextV1(t, workspace, observation, snapshot, threadID, "host-local-turn")
	body, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	var contextRecord map[string]any
	if err := json.Unmarshal(body, &contextRecord); err != nil {
		t.Fatal(err)
	}
	at, _ := time.Parse(time.RFC3339Nano, securityContext.IssuedAt)
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{
		Thread: thread, SecurityContext: securityContext, At: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := durable.AppendTurnToThread(threadID, map[string]any{
		"id": securityContext.TurnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"contextEpochSnapshot": contextepochapp.PublicSnapshot(epoch.State.AcceptedSnapshot), "items": []any{},
	}, "synthetic-host-local", map[string]any{
		"securityState": contextRecord, "contextEpochState": contextepochapp.PublicState(epoch.State),
	}); err != nil {
		t.Fatal(err)
	}
	return securityContext, durable
}

// Only native source probing is synthetic; the exact dataset capability,
// Finalizer, terminal stores, public CAS and event delivery are production owners.
type runtimeHostLocalPublicationSourceV6 struct {
	owner   *runtimeHostLocalEvidenceOwnersV1
	resolve datasetsnapshotport.ResolveInputV2
	probe   domainsecurity.VerifiedSourceProbe
}

func (source runtimeHostLocalPublicationSourceV6) WithFreshPublicationSnapshotAuthority(ctx context.Context,
	input sourceprobeport.PublicationInput, use func([]domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	if len(input.Requirements) != 1 || !reflect.DeepEqual(input.Binding, source.resolve.Observation) {
		return errors.New("synthetic publication source scope mismatch")
	}
	return source.owner.WithCurrentSelectionV2(ctx, source.resolve, input.Context, func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
		return use([]domainsecurity.VerifiedSourceProbe{source.probe}, &runtimeHostLocalFinalSourceV6{
			ctx: ctx, securityContext: input.Context, probe: source.probe, selection: selection, capability: capability,
		})
	})
}

func runtimeHostLocalFinalDeliveryOwnersV6(t *testing.T, config Config, key finalauthorityport.Authority,
	access finalauthority.SecurePrivateCASAccessAuthority,
) (*finalauthority.PrivateStore, *turnterminalapp.Coordinator, *finalauthority.AcceptedFinalCASReader) {
	t.Helper()
	privateRoot := filepath.Join(config.DataDir, "private")
	private, err := finalauthority.NewPrivateStore(filepath.Join(privateRoot, "accepted-finals"), access)
	if err != nil {
		t.Fatal(err)
	}
	providerStore, err := cachetelemetrystore.NewStore(filepath.Join(privateRoot, "provider-cache-telemetry"), access)
	if err != nil {
		t.Fatal(err)
	}
	providerTelemetry, err := cachetelemetryapp.NewDurableService(key, providerStore)
	if err != nil {
		t.Fatal(err)
	}
	terminalStore, err := turnterminalstore.NewStore(filepath.Join(privateRoot, "turn-terminal-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(config.DurableTempDir)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := turnterminalapp.NewCoordinator(key, private, terminalStore, providerTelemetry)
	if err != nil {
		t.Fatal(err)
	}
	return private, coordinator, casReader
}

func runtimeHostLocalPublishFinalDeliveryV6(t *testing.T, ctx context.Context, config Config,
	owner *runtimeHostLocalEvidenceOwnersV1, key finalauthorityport.Authority, access finalauthority.SecurePrivateCASAccessAuthority,
	durable *server.DurableEventSessionStore, securityContext domainsecurity.TurnSecurityContext,
	resolve datasetsnapshotport.ResolveInputV2, probe domainsecurity.VerifiedSourceProbe,
) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	private, coordinator, casReader := runtimeHostLocalFinalDeliveryOwnersV6(t, config, key, access)
	eventIO := startupFinalEventIOForTest(durable, casReader)
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(key, eventIO.Readback)
	finalizer := evidenceapp.NewCasePublicationFinalizerWithHostEvidenceAuthority(owner, owner, key, private, eventIO, coordinator,
		runtimeHostLocalPublicationSourceV6{owner: owner, resolve: resolve, probe: probe}, filestore.CaseBindingReader{}, index)
	events, unsubscribe := durable.SubscribeEvents(securityContext.ThreadID)
	defer unsubscribe()
	result, err := finalizer.PersistBoundary(ctx, evidenceapp.PersistCaseBoundaryInput{
		Store: durable, Context: securityContext, TerminalReason: evidenceapp.TerminalSuccess,
		CaseSlotIntent: evidenceapp.CaseSlotRequestedV1,
		ThreadID:       securityContext.ThreadID, TurnID: securityContext.TurnID, AcceptedAt: time.Now().UTC(),
	})
	if err != nil || result.Boundary.Envelope.Variant != domainevidence.EvidenceBackedAnswer {
		t.Fatalf("production host-local Finalizer did not commit facts: variant=%q err=%v", result.Boundary.Envelope.Variant, err)
	}
	records, err := private.List(ctx)
	if err != nil || len(records) != 1 || records[0].SchemaVersion != 6 || records[0].AcceptedFinal.FactFinalHostLocalAdmission == nil {
		t.Fatalf("production Finalizer did not persist one host-local V6: count=%d err=%v", len(records), err)
	}
	original := records[0]
	projected, found := index.Resolve(securityContext.ThreadID, securityContext.TurnID)
	if !found || projected.AcceptedFinal.RecordDigest != original.AcceptedFinal.RecordDigest {
		t.Fatal("committed Final was not activated in the trusted projection")
	}
	select {
	case value := <-events:
		batch, err := domainevent.ParseAcceptedFinalDeliveryBatchV2(value)
		if err != nil || batch.ThreadID != securityContext.ThreadID || batch.TurnID != securityContext.TurnID ||
			domainevent.ContainsPrivateAcceptedFinalAuthority(value) {
			t.Fatalf("production Final delivery was not a closed public batch: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("committed Final did not publish its public batch")
	}
	return original
}

func runtimeHostLocalRestoreFinalDeliveryV6(t *testing.T, ctx context.Context, config Config,
	owner *runtimeHostLocalEvidenceOwnersV1, key finalauthorityport.Authority, access finalauthority.SecurePrivateCASAccessAuthority,
	original domainevidence.PrivateAcceptedFinalRecord,
) {
	t.Helper()
	before := startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir)
	durable, err := server.NewTempDurableEventSessionStore(config.DurableTempDir)
	if err != nil {
		t.Fatal(err)
	}
	private, coordinator, casReader := runtimeHostLocalFinalDeliveryOwnersV6(t, config, key, access)
	inventory, err := evidenceapp.PreflightFinalAuthorityInventory(ctx, durable, casReader, owner, key, private)
	if err != nil {
		t.Fatalf("reopened Final inventory rejected the original: %v", err)
	}
	recovery, err := coordinator.RecoverV1(ctx, turnterminalapp.RestartRecoveryInputV1{
		CompletionStore: durable, CASReader: casReader, AuditOnlyPrivateInventory: inventory.AuditOnlyPublicWinners,
	})
	if err != nil || len(recovery.FactCandidates) != 1 || len(recovery.Complete) != 0 {
		t.Fatalf("reopened terminal did not retain one fact candidate: count=%d err=%v", len(recovery.FactCandidates), err)
	}
	eventIO := startupFinalEventIOForTest(durable, casReader)
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(key, eventIO.Readback)
	// The real owner capability verifies the original head, retained dataset,
	// registry and binding. This fixture makes no live native-probe assertion.
	restoreRuntimeFactTerminalsV1(ctx, index, recovery.FactCandidates, owner,
		func(_ context.Context, record domainevidence.PrivateAcceptedFinalRecord) error {
			if !reflect.DeepEqual(record, original) {
				return errors.New("recovery replaced original Final")
			}
			return nil
		},
		func(lease context.Context, record domainevidence.PrivateAcceptedFinalRecord, cap appturn.FactFinalMutationAuthority) ([]map[string]any, error) {
			return evidenceapp.LoadVerifiedAcceptedFinalEventsWithAuthority(lease, eventIO, durable, record, cap)
		})
	projected, found := index.Resolve(original.SecurityContext.ThreadID, original.SecurityContext.TurnID)
	actual, _ := domainevidence.PrivateAcceptedFinalRecordBytes(projected)
	expected, _ := domainevidence.PrivateAcceptedFinalRecordBytes(original)
	if !found || !bytes.Equal(actual, expected) {
		t.Fatal("fresh trusted projection did not recover original V6 bytes")
	}
	if after := startupWholeTreeDigest(t, config.DataDir, config.DurableTempDir); after != before {
		t.Fatal("fact terminal recovery rewrote durable state")
	}
}
