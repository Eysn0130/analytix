package evidenceregistry

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

type witnessedRegistryBindingObserver struct {
	workspaceRealPath string
	observation       domainsecurity.CaseBindingObservationV1
}

func (observer *witnessedRegistryBindingObserver) Observe(
	workspaceRealPath string,
) (domainsecurity.CaseBindingObservationV1, error) {
	if observer == nil || workspaceRealPath != observer.workspaceRealPath ||
		domainsecurity.ValidateCaseBindingObservationV1(observer.observation) != nil {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("test case binding observation is unavailable")
	}
	return observer.observation, nil
}

type witnessedRegistryDatasetAuthority struct {
	coordinator        *witnessedRegistryCoordinator
	selection          datasetsnapshotport.CurrentSelectionV2
	bindingObservation domainsecurity.CaseBindingObservationV1
}

var _ datasetsnapshotport.CurrentAuthorityV2 = (*witnessedRegistryDatasetAuthority)(nil)

func (authority *witnessedRegistryDatasetAuthority) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(
		datasetsnapshotport.CurrentSelectionV2,
		datasetsnapshotport.CurrentSelectionCapabilityV2,
	) error,
) error {
	if authority == nil || ctx == nil || ctx.Err() != nil || callback == nil ||
		authority.coordinator == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		input.TenantID != securityContext.TenantID || input.UserID != securityContext.UserID ||
		input.ExpectedDatasetSnapshotID != securityContext.DatasetSnapshotID ||
		!reflect.DeepEqual(input.Observation, authority.bindingObservation) ||
		input.Observation.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest {
		return errors.New("test current dataset authority input is invalid")
	}
	head, err := authority.coordinator.ObserveFresh(ctx)
	if err != nil {
		return err
	}
	selection, err := cloneWitnessedRegistryDatasetSelection(authority.selection)
	if err != nil {
		return err
	}
	selection.Head = head
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil || !selection.Head.HasBundle || len(selection.DatasetIndexPath) == 0 ||
		selection.Head.Bundle.DatasetSnapshotCount != uint64(len(selection.DatasetIndexPath)) ||
		selection.Head.Bundle.DatasetSnapshotIndexDigest != selection.DatasetIndexPath[0].IndexDigest ||
		selection.Snapshot.Record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		selection.Snapshot.Record.SourceManifestHash != securityContext.SourceManifestHash ||
		selection.Snapshot.Manifest.SourceManifestHash != securityContext.SourceManifestHash ||
		datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil {
		return errors.New("test current dataset selection is unavailable")
	}
	frozen, err := cloneWitnessedRegistryDatasetSelection(selection)
	if err != nil {
		return err
	}
	capability := &witnessedRegistryDatasetSelectionCapability{
		active: true, ctx: ctx, coordinator: authority.coordinator,
		securityContext: securityContext, selection: frozen,
	}
	defer capability.close()
	return callback(selection, capability)
}

type witnessedRegistryDatasetSelectionCapability struct {
	mu              sync.RWMutex
	active          bool
	ctx             context.Context
	coordinator     *witnessedRegistryCoordinator
	securityContext domainsecurity.TurnSecurityContext
	selection       datasetsnapshotport.CurrentSelectionV2
}

var _ datasetsnapshotport.CurrentSelectionCapabilityV2 = (*witnessedRegistryDatasetSelectionCapability)(nil)

func (capability *witnessedRegistryDatasetSelectionCapability) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	if capability == nil {
		return errors.New("test current dataset capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil ||
		capability.coordinator == nil || use == nil ||
		!reflect.DeepEqual(selection, capability.selection) ||
		!reflect.DeepEqual(securityContext, capability.securityContext) {
		return errors.New("test current dataset capability exact binding changed")
	}
	current, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !current.HasBundle || current.Bundle != selection.Head.Bundle {
		return errors.New("test current dataset capability shared head is stale")
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	if err := use(leaseContext); err != nil {
		return err
	}
	current, err = capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || leaseContext.Err() != nil || capability.ctx.Err() != nil ||
		!current.HasBundle || current.Bundle != selection.Head.Bundle ||
		!reflect.DeepEqual(selection, capability.selection) ||
		!reflect.DeepEqual(securityContext, capability.securityContext) {
		return errors.New("test current dataset capability changed during exact use")
	}
	return nil
}

func (capability *witnessedRegistryDatasetSelectionCapability) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	capability.mu.Unlock()
}

type witnessedRegistryHostEvidenceCapability struct {
	mu              sync.RWMutex
	active          bool
	ctx             context.Context
	cancel          context.CancelFunc
	coordinator     *witnessedRegistryCoordinator
	securityContext domainsecurity.TurnSecurityContext
	probe           domainsecurity.VerifiedSourceProbe
	selection       datasetsnapshotport.CurrentSelectionV2
}

var _ sourceprobeport.HostEvidenceCapability = (*witnessedRegistryHostEvidenceCapability)(nil)

func newWitnessedRegistryHostEvidenceCapability(
	t *testing.T,
	coordinator *witnessedRegistryCoordinator,
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
) *witnessedRegistryHostEvidenceCapability {
	t.Helper()
	cloned, err := cloneWitnessedRegistryDatasetSelection(selection)
	if err != nil || datasetsnapshotport.ValidateCurrentSelectionDigestV2(cloned) != nil ||
		!domainsecurity.SourceProbeEligibleForHostAuthorityV2(probe) ||
		probe.ThreadID != securityContext.ThreadID || probe.TurnID != securityContext.TurnID ||
		probe.ContextEpoch != securityContext.ContextEpoch ||
		probe.ProbeContextDigest != securityContext.ContextDigest ||
		probe.CaseID != securityContext.CaseID ||
		probe.CaseBindingHash != securityContext.CaseBindingHash ||
		probe.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		cloned.Snapshot.Record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		cloned.Snapshot.Record.SourceManifestHash != securityContext.SourceManifestHash {
		t.Fatal("test host evidence capability input is inconsistent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &witnessedRegistryHostEvidenceCapability{
		active: true, ctx: ctx, cancel: cancel, coordinator: coordinator,
		securityContext: securityContext, probe: probe, selection: cloned,
	}
}

func (capability *witnessedRegistryHostEvidenceCapability) DatasetSelection() (
	datasetsnapshotport.CurrentSelectionV2,
	error,
) {
	if capability == nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("test host evidence capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("test host evidence capability is inactive")
	}
	return cloneWitnessedRegistryDatasetSelection(capability.selection)
}

func (capability *witnessedRegistryHostEvidenceCapability) UseExact(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	use func(context.Context) error,
) error {
	if capability == nil {
		return errors.New("test host evidence capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil ||
		capability.coordinator == nil || use == nil ||
		!reflect.DeepEqual(securityContext, capability.securityContext) ||
		!reflect.DeepEqual(probe, capability.probe) ||
		!reflect.DeepEqual(selection, capability.selection) {
		return errors.New("test host evidence capability exact binding changed")
	}
	current, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !current.HasBundle || !selection.Head.HasBundle ||
		current.Bundle != selection.Head.Bundle {
		return errors.New("test host evidence capability shared head is stale")
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	if err := use(leaseContext); err != nil {
		return err
	}
	current, err = capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || leaseContext.Err() != nil || capability.ctx.Err() != nil ||
		!current.HasBundle || current.Bundle != selection.Head.Bundle ||
		!reflect.DeepEqual(securityContext, capability.securityContext) ||
		!reflect.DeepEqual(probe, capability.probe) ||
		!reflect.DeepEqual(selection, capability.selection) {
		return errors.New("test host evidence capability changed during exact use")
	}
	return nil
}

func (capability *witnessedRegistryHostEvidenceCapability) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	if capability.cancel != nil {
		capability.cancel()
	}
	capability.mu.Unlock()
}

func cloneWitnessedRegistryDatasetSelection(
	selection datasetsnapshotport.CurrentSelectionV2,
) (datasetsnapshotport.CurrentSelectionV2, error) {
	body, err := json.Marshal(selection)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	var cloned datasetsnapshotport.CurrentSelectionV2
	if err := json.Unmarshal(body, &cloned); err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	return cloned, nil
}

func (fixture *witnessedRegistryFixture) datasetSelectionForHead(
	t *testing.T,
	head evidenceauthorityport.FreshHead,
) datasetsnapshotport.CurrentSelectionV2 {
	t.Helper()
	selection, err := cloneWitnessedRegistryDatasetSelection(fixture.datasetSelection)
	if err != nil {
		t.Fatal(err)
	}
	selection.Head = head
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil || datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil {
		t.Fatal("test current dataset selection digest is invalid")
	}
	return selection
}

func (fixture *witnessedRegistryFixture) currentDatasetSelection(
	t *testing.T,
) datasetsnapshotport.CurrentSelectionV2 {
	t.Helper()
	head, err := fixture.coordinator.ObserveFresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return fixture.datasetSelectionForHead(t, head)
}

func (fixture *witnessedRegistryFixture) advanceDatasetChildForTest(t *testing.T) {
	t.Helper()
	fixture.coordinator.mu.Lock()
	defer fixture.coordinator.mu.Unlock()
	previous := fixture.coordinator.bundle
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID,
		Generation: previous.Generation + 1, PreviousBundleDigest: previous.RecordDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("fact-final-dataset-drift-mutation")),
		DatasetSnapshotIndexDigest:  domainsecurity.SHA256Hex([]byte("fact-final-dataset-drift-index")),
		DatasetSnapshotCount:        previous.DatasetSnapshotCount + 1,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       previous.EvidenceRegistryCount,
		PublicationIndexDigest:      previous.PublicationIndexDigest,
		PublicationCount:            previous.PublicationCount,
		AuthorityKeyID:              fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.coordinator.bundle = next
	fixture.coordinator.bundles[next.RecordDigest] = next
}

func witnessedRegistryFactProbe(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	source domainevidence.PublicationSourceSnapshot,
) domainsecurity.VerifiedSourceProbe {
	t.Helper()
	identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(source.ServerIdentity)
	if err != nil {
		t.Fatal(err)
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, source.CheckedAt)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: source.ServerID, ServerIdentity: source.ServerIdentity,
		ConnectionEpoch: source.ConnectionEpoch, CatalogFingerprint: source.CatalogFingerprint,
		SpecFingerprint: source.SpecFingerprint, ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		CheckedAt: checkedAt,
		Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: identity.ObservedName,
			ServerVersion: identity.ObservedVersion, CaseID: securityContext.CaseID,
			CaseBindingHash:   securityContext.CaseBindingHash,
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			Ready:             true, ReadOnly: true, CheckedAt: checkedAt.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return probe
}

func witnessedFactFinalRequest(
	t *testing.T,
	fixture *witnessedRegistryFixture,
	record domainevidence.PrivateAcceptedFinalRecord,
) registryport.FactFinalWitnessRequest {
	t.Helper()
	if record.PublicationSnapshotProof == nil || len(record.PublicationSnapshotProof.Sources) != 1 {
		t.Fatal("test fact final publication source is unavailable")
	}
	probe := witnessedRegistryFactProbe(t, record.SecurityContext, record.PublicationSnapshotProof.Sources[0])
	if probe.ProbeDigest != record.PublicationSnapshotProof.Sources[0].ProbeDigest {
		t.Fatal("test fact final source probe changed")
	}
	selection := fixture.currentDatasetSelection(t)
	capability := newWitnessedRegistryHostEvidenceCapability(
		t, fixture.coordinator, record.SecurityContext, probe, selection,
	)
	t.Cleanup(capability.close)
	return registryport.FactFinalWitnessRequest{
		Context: record.SecurityContext, Envelope: record.Envelope, RenderedText: record.RenderedText,
		PublicationProof: record.PublicationSnapshotProof, PublicationIntent: record.PublicationIntent,
		BindingObservation: fixture.bindingObservation,
		SourceProbes:       []domainsecurity.VerifiedSourceProbe{probe}, DatasetSelection: selection,
		HostEvidenceCapability: capability,
	}
}
