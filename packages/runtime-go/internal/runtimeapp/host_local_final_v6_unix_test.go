//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	"analytix.local/runtime-go/internal/server"
	fixturev2 "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
)

// The source probe is synthetic. Its lease delegates to the actual selected
// dataset capability; this is file-owner/CAS evidence, not native or GUI QA.
type runtimeHostLocalFinalSourceV6 struct {
	ctx             context.Context
	securityContext domainsecurity.TurnSecurityContext
	probe           domainsecurity.VerifiedSourceProbe
	selection       datasetsnapshotport.CurrentSelectionV2
	capability      datasetsnapshotport.CurrentSelectionCapabilityV2
}

func (source *runtimeHostLocalFinalSourceV6) DatasetSelection() (datasetsnapshotport.CurrentSelectionV2, error) {
	if source.ctx.Err() != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, source.ctx.Err()
	}
	return source.selection, nil
}
func (source *runtimeHostLocalFinalSourceV6) UseExact(securityContext domainsecurity.TurnSecurityContext, probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2, use func(context.Context) error) error {
	if !reflect.DeepEqual(source.securityContext, securityContext) || !reflect.DeepEqual(source.probe, probe) ||
		!reflect.DeepEqual(source.selection, selection) {
		return errors.New("synthetic source lease mismatch")
	}
	return source.capability.UseExact(selection, securityContext, use)
}

func runtimeHostLocalFinalRequestV6(t *testing.T, owner *runtimeHostLocalEvidenceOwnersV1,
	securityContext domainsecurity.TurnSecurityContext, resolve datasetsnapshotport.ResolveInputV2,
	prepared domainevidence.PreparedEvidenceSettlement, receipt domainevidence.EvidenceReceipt) registryport.FactFinalWitnessRequest {
	t.Helper()
	payload := domainevidence.NormalizedClaimPayload{SubjectID: "dataset:analysis_txn_detail_idx", EntityID: "dataset:analysis_txn_detail_idx", Count: "3", Granularity: "dataset_table_rows"}
	proposal := domainevidence.ClaimProposal{SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-host-local-count",
		ClaimType: domainevidence.ClaimCount, NormalizedPayload: payload, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{}}
	scope := receipt.QueryRange
	verifiedAt, err := time.Parse(time.RFC3339Nano, prepared.SourceProbe.CheckedAt)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: "claim-host-local-count", Proposal: proposal, SupportState: domainevidence.ClaimVerified,
		EvidenceIDs: []string{receipt.ReceiptID}, CounterEvidenceIDs: []string{}, SupportedScope: &scope,
		AllowedWording: []string{"exact_verified_fact"}, ProhibitedUpgrades: []string{"legal_characterization_without_review"},
		VerifierReceiptID:  domainevidence.VerifierReceiptDigest("claim-host-local-count", proposal.ClaimType, payload, []string{receipt.ReceiptID}, nil, domainevidence.ClaimVerified),
		VerificationReason: "synthetic canonical count", VerifiedAt: verifiedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.EvidenceBackedAnswer, Context: securityContext, TerminalReason: "success", Claims: []domainevidence.ClaimRecord{claim},
		EvidenceReceiptIDs: []string{receipt.ReceiptID}, CheckedScope: &scope, IssuedAt: verifiedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := owner.Replay(context.Background(), securityContext)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	probe := prepared.SourceProbe
	proof, err := domainevidence.NewPublicationSnapshotProof(domainevidence.PublicationSnapshotProofInput{
		Context: securityContext, RegistryHead: head, EvidenceReceiptIDs: envelope.EvidenceReceiptIDs,
		Sources: []domainevidence.PublicationSourceSnapshot{{ReceiptID: receipt.ReceiptID, ServerID: probe.ServerID, ServerIdentity: probe.ServerIdentity,
			ServerVersion: receipt.ServerVersion, ConnectionEpoch: probe.ConnectionEpoch, ToolName: receipt.ToolName,
			DatasetSnapshotID: probe.DatasetSnapshotID, CatalogFingerprint: probe.CatalogFingerprint, SpecFingerprint: probe.SpecFingerprint,
			ProbeDigest: probe.ProbeDigest, CheckedAt: probe.CheckedAt}}, CheckedAt: verifiedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: verifiedAt.Format(time.RFC3339Nano), TerminalStatus: "completed"}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	return registryport.FactFinalWitnessRequest{Context: securityContext, Envelope: envelope, RenderedText: rendered,
		PublicationProof: &proof, PublicationIntent: intent, BindingObservation: resolve.Observation, SourceProbes: []domainsecurity.VerifiedSourceProbe{probe}}
}

func runtimeHostLocalIssueAndPersistFinalV6(t *testing.T, ctx context.Context, owner *runtimeHostLocalEvidenceOwnersV1,
	access finalauthority.SecurePrivateCASAccessAuthority, privateRoot string, securityContext domainsecurity.TurnSecurityContext,
	resolve datasetsnapshotport.ResolveInputV2, prepared domainevidence.PreparedEvidenceSettlement, receipt domainevidence.EvidenceReceipt,
	config Config, key finalauthorityport.Authority, durable *server.DurableEventSessionStore) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	request := runtimeHostLocalFinalRequestV6(t, owner, securityContext, resolve, prepared, receipt)
	var original domainevidence.PrivateAcceptedFinalRecord
	var escaped registryport.FactFinalWitnessCapability
	err := owner.WithCurrentSelectionV2(ctx, resolve, securityContext, func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
		request.DatasetSelection = selection
		request.HostEvidenceCapability = &runtimeHostLocalFinalSourceV6{ctx: ctx, securityContext: securityContext, probe: prepared.SourceProbe, selection: selection, capability: capability}
		return owner.WithFactFinalWitnessAuthority(ctx, request, func(final registryport.FactFinalWitnessCapability) error {
			escaped = final
			var err error
			original, err = final.PrivateFinal()
			if err != nil {
				return err
			}
			if original.SchemaVersion != 6 || original.AcceptedFinal.SchemaVersion != 6 || original.AcceptedFinal.FactFinalWitnessAdmission != nil || original.AcceptedFinal.FactFinalHostLocalAdmission == nil {
				t.Fatal("host-local Final borrowed witnessed or boundary grammar")
			}
			if _, err := json.Marshal(final); err == nil {
				t.Fatal("Final capability serialized")
			}
			calls := 0
			bad := original
			bad.RenderedText += "tampered"
			if err := final.UseExact(bad, func() error { calls++; return nil }); err == nil || calls != 0 {
				t.Fatal("modified Final entered persistence")
			}
			// Exercise nested uses before the separate production finalizer below.
			return final.UseExact(original, func() error {
				return final.UseExact(original, func() error { return nil })
			})
		})
	})
	if err != nil {
		t.Fatalf("actual host-local Final issuance failed: %v", err)
	}
	if _, err := escaped.PrivateFinal(); err == nil {
		t.Fatal("escaped Final remained active")
	}
	if err := escaped.UseExact(original, func() error { t.Fatal("escaped Final ran callback"); return nil }); err == nil {
		t.Fatal("escaped Final was accepted")
	}
	original = runtimeHostLocalPublishFinalDeliveryV6(t, ctx, config, owner, key, access, durable, securityContext, resolve, prepared.SourceProbe)
	runtimeHostLocalFinalLeaseChecksV6(t, ctx, owner, original)
	return original
}

func runtimeHostLocalFinalLeaseChecksV6(t *testing.T, ctx context.Context, owner *runtimeHostLocalEvidenceOwnersV1, original domainevidence.PrivateAcceptedFinalRecord) {
	t.Helper()
	before, found, err := owner.heads.Current(ctx)
	if err != nil || !found {
		t.Fatal(err)
	}
	expectedError := errors.New("synthetic Final callback failure")
	if err := owner.WithRecoveredFactFinalWitness(ctx, original, func(cap registryport.FactFinalWitnessCapability) error {
		return cap.UseExact(original, func() error { return expectedError })
	}); !errors.Is(err, expectedError) {
		t.Fatalf("Final callback error was lost: %v", err)
	}
	// Model the session-lock inversion: an active Final owns the head serializer;
	// another recovery must fail before taking a callback or waiting for a lock.
	var session sync.Mutex
	session.Lock()
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- owner.WithRecoveredFactFinalWitness(ctx, original, func(cap registryport.FactFinalWitnessCapability) error {
			close(entered)
			session.Lock()
			defer session.Unlock()
			return cap.UseExact(original, func() error { return nil })
		})
	}()
	select {
	case <-entered:
	case err := <-done:
		session.Unlock()
		t.Fatalf("Final guard did not enter: %v", err)
	case <-ctx.Done():
		session.Unlock()
		t.Fatal(ctx.Err())
	}
	busyDone := make(chan error, 1)
	go func() {
		busyDone <- owner.WithRecoveredFactFinalWitness(ctx, original, func(registryport.FactFinalWitnessCapability) error {
			t.Error("busy recovery entered callback")
			return nil
		})
	}()
	var busy error
	var blocked bool
	select {
	case busy = <-busyDone:
	case <-time.After(3 * time.Second):
		blocked = true
	}
	session.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Final/session lock order deadlocked")
	}
	if blocked {
		select {
		case <-busyDone:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		t.Fatal("busy Final read guard waited for the session lock")
	}
	if busy == nil {
		t.Fatal("busy Final read guard was accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	called := false
	err = owner.WithRecoveredFactFinalWitness(cancelled, original, func(cap registryport.FactFinalWitnessCapability) error {
		cancel()
		return cap.UseExact(original, func() error { called = true; return nil })
	})
	cancel()
	if err == nil || called {
		t.Fatal("cancelled Final lease entered callback")
	}
	after, found, err := owner.heads.Current(ctx)
	if err != nil || !found || before != after {
		t.Fatalf("read-only/busy/cancelled Final changed head: %v", err)
	}
	runtimeHostLocalFinalCloseWaitsForUseV6(t, ctx, owner, original)
}

func runtimeHostLocalFinalCloseWaitsForUseV6(t *testing.T, ctx context.Context, owner *runtimeHostLocalEvidenceOwnersV1, original domainevidence.PrivateAcceptedFinalRecord) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	outerDone, useDone := make(chan error, 1), make(chan error, 1)
	var escaped registryport.FactFinalWitnessCapability
	go func() {
		outerDone <- owner.WithRecoveredFactFinalWitness(ctx, original, func(cap registryport.FactFinalWitnessCapability) error {
			escaped = cap
			go func() { useDone <- cap.UseExact(original, func() error { close(entered); <-release; return nil }) }()
			select {
			case <-entered:
				return nil
			case err := <-useDone:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case err := <-outerDone:
		t.Fatalf("concurrent Final use did not enter: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := escaped.PrivateFinal(); err != nil {
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("closing Final accepted new use")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	select {
	case err := <-outerDone:
		t.Fatalf("Final closed before active use drained: %v", err)
	default:
	}
	if err := escaped.UseExact(original, func() error { t.Error("closing Final entered new use"); return nil }); err == nil {
		t.Fatal("closing Final admitted new use")
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-useDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-outerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Final close did not drain")
	}
}

func runtimeHostLocalRecoverFinalAfterSuccessorV6(t *testing.T, ctx context.Context, owner *runtimeHostLocalEvidenceOwnersV1,
	access finalauthority.SecurePrivateCASAccessAuthority, privateRoot string, first datasetsnapshotport.ResolvedSnapshotV2,
	resolve datasetsnapshotport.ResolveInputV2, original domainevidence.PrivateAcceptedFinalRecord, restoreDelivery func()) {
	t.Helper()
	store, err := finalauthority.NewPrivateStore(filepath.Join(privateRoot, "accepted-finals"), access)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Resolve(ctx, original.AcceptedFinal.RecordDigest)
	originalBytes, _ := domainevidence.PrivateAcceptedFinalRecordBytes(original)
	loadedBytes, _ := domainevidence.PrivateAcceptedFinalRecordBytes(loaded)
	if err != nil || !bytes.Equal(originalBytes, loadedBytes) {
		t.Fatalf("disk reopen changed original Final: %v", err)
	}
	verify := func() {
		t.Helper()
		if err := owner.WithRecoveredFactFinalWitness(ctx, loaded, func(cap registryport.FactFinalWitnessCapability) error {
			value, err := cap.PrivateFinal()
			body, _ := domainevidence.PrivateAcceptedFinalRecordBytes(value)
			if err != nil || !bytes.Equal(body, originalBytes) {
				return errors.New("recovery reissued original Final")
			}
			return cap.UseExact(loaded, func() error { return nil })
		}); err != nil {
			t.Fatalf("original Final recovery failed: %v", err)
		}
	}
	verify()
	restoreDelivery()
	// Synthetic successor material exercises original A vs selected B. It does
	// not claim a cleaning operation or an Electron/Go process restart.
	input, err := fixturev2.CloneManifestInputV2(first)
	if err != nil {
		t.Fatal(err)
	}
	input.AnalyticalDuckDB = first.Manifest.AnalyticalDuckDB
	input.FundsProducerContentManifest.SourceRevision++
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	producerBytes, err := domainsecurity.FundsProducerContentManifestV1Bytes(input.FundsProducerContentManifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := domainsecurity.DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(privateRoot, "dataset-snapshot-authority", "materials"), 16*1024*1024, access)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{producerBytes, manifestBytes} {
		if err := cas.PutIfAbsent(ctx, domainsecurity.SHA256Hex(body), body); err != nil && !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if err := cas.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := owner.AdmitAfterExactV2(ctx, datasetsnapshotapp.AdmitAfterInputV2{
		ExpectedCurrentDatasetSnapshotID: first.Record.DatasetSnapshotID,
		AdmitInputV2: datasetsnapshotapp.AdmitInputV2{TenantID: resolve.TenantID, UserID: resolve.UserID, Observation: resolve.Observation,
			ManifestReference:      datasetsnapshotport.ExactMaterialReferenceV2{Address: domainsecurity.SHA256Hex(manifestBytes), SHA256: domainsecurity.SHA256Hex(manifestBytes), ByteLength: uint64(len(manifestBytes))},
			FundsProducerReference: datasetsnapshotport.ExactMaterialReferenceV2{Address: domainsecurity.SHA256Hex(producerBytes), SHA256: domainsecurity.SHA256Hex(producerBytes), ByteLength: uint64(len(producerBytes))}, AcceptedAt: time.Now().UTC()},
	})
	if err != nil || second.Record.DatasetSnapshotID == first.Record.DatasetSnapshotID {
		t.Fatalf("Final successor admission failed: %v", err)
	}
	resolve.ExpectedDatasetSnapshotID = second.Record.DatasetSnapshotID
	current, err := owner.ResolveWitnessedV2(ctx, resolve)
	if err != nil || current.Record != second.Record {
		t.Fatalf("current query did not select B: %v", err)
	}
	oldCurrent := resolve
	oldCurrent.ExpectedDatasetSnapshotID = first.Record.DatasetSnapshotID
	if _, err := owner.ResolveWitnessedV2(ctx, oldCurrent); !errors.Is(err, datasetsnapshotport.ErrStale) {
		t.Fatalf("historical A was accepted as current: %v", err)
	}
	verify()
	restoreDelivery()
	// A callback that changes case binding must fail even though head stays fixed.
	bindingPath := filepath.Join(original.SecurityContext.WorkspaceRealPath, ".analytix", "case-project.json")
	bindingBytes, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatal(err)
	}
	var innerDriftErr error
	err = owner.WithRecoveredFactFinalWitness(ctx, loaded, func(cap registryport.FactFinalWitnessCapability) error {
		innerDriftErr = cap.UseExact(loaded, func() error { return os.WriteFile(bindingPath, []byte(`{"caseId":"changed"}`), 0o600) })
		return innerDriftErr
	})
	if restoreErr := os.WriteFile(bindingPath, bindingBytes, 0o600); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if err == nil || innerDriftErr == nil {
		t.Fatal("Final callback binding drift retained authority")
	}
	verify()
	if err := owner.Revoke(ctx, registryport.RevokeInput{Context: original.SecurityContext, ReceiptID: original.Envelope.EvidenceReceiptIDs[0], ReasonCode: "source_retracted", RevokedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := owner.WithRecoveredFactFinalWitness(ctx, loaded, func(registryport.FactFinalWitnessCapability) error { t.Fatal("revoked Final ran callback"); return nil }); err == nil {
		t.Fatal("revoked original Final was recovered")
	}
	retained, err := store.Resolve(ctx, original.AcceptedFinal.RecordDigest)
	retainedBytes, _ := domainevidence.PrivateAcceptedFinalRecordBytes(retained)
	if err != nil || !bytes.Equal(originalBytes, retainedBytes) {
		t.Fatal("successor/revocation rewrote original Final")
	}
}
