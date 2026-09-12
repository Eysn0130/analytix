package server

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	evidenceregistryadapter "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

// The public-seam fixture retains its real signed historical registry. This
// callback capability proves that exact legacy commit and signed readback;
// it does not stand in for the production V2 shared-witness CAS capability.
type caseForegroundPublicSeamRegistryV1 struct {
	*evidenceregistryadapter.Store
	authority finalauthorityport.Verifier
}

type caseForegroundRegistryEffectKeyV1 struct{}
type caseForegroundRegistryEffectV1 struct {
	mu         sync.Mutex
	active     atomic.Bool
	owner      *caseForegroundPublicSeamRegistryV1
	capability *caseForegroundPublicSeamEvidenceCapabilityV1
	prepared   domainevidence.PreparedEvidenceSettlement
	attempted  bool
	failed     bool
	completed  *domainevidence.EvidenceReceiptRegistry
}

var _ sourceprobeport.HostEvidenceRegistryCommitCapabilityV1 = (*caseForegroundPublicSeamEvidenceCapabilityV1)(nil)

func (capability *caseForegroundPublicSeamEvidenceCapabilityV1) currentBinding(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
) error {
	if capability == nil {
		return errors.New("case foreground evidence capability is unavailable")
	}
	capability.mu.Lock()
	active := capability.active && capability.ctx != nil && capability.ctx.Err() == nil &&
		securityContext == capability.context && reflect.DeepEqual(probe, capability.probe) &&
		reflect.DeepEqual(selection, capability.selection) && capability.source != nil && capability.source.dataset != nil
	capability.mu.Unlock()
	if !active || datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil {
		return errors.New("case foreground evidence capability binding changed")
	}
	if err := capability.source.ValidateCurrentProbe(capability.ctx, probe.ServerID, securityContext); err != nil {
		return err
	}
	capability.source.mu.Lock()
	currentProbe := capability.source.probes[securityContext.ContextDigest]
	capability.source.mu.Unlock()
	if !reflect.DeepEqual(currentProbe, probe) || capability.source.dataset.validateCurrent(capability.ctx, securityContext) != nil {
		return errors.New("case foreground source or dataset is no longer current")
	}
	capability.source.dataset.mu.Lock()
	currentSelection := cloneServerCurrentSelectionV1(capability.source.dataset.selection)
	capability.source.dataset.mu.Unlock()
	if !reflect.DeepEqual(currentSelection, selection) {
		return errors.New("case foreground dataset selection changed")
	}
	return nil
}

func (capability *caseForegroundPublicSeamEvidenceCapabilityV1) UseExactRegistryCommit(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	prepared domainevidence.PreparedEvidenceSettlement,
	marker domainevidence.HostEvidenceSettlementMarker,
	use func(context.Context) error,
) error {
	if capability == nil || use == nil {
		return errors.New("case foreground registry capability is unavailable")
	}
	capability.mu.Lock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil ||
		capability.registryUsed || capability.registry == nil || capability.registry.authority == nil {
		capability.mu.Unlock()
		return errors.New("case foreground registry capability was consumed")
	}
	capability.registryUsed = true
	capability.mu.Unlock()
	if err := capability.currentBinding(securityContext, probe, selection); err != nil {
		return err
	}
	contentDigest, err := datasetsnapshotport.CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil || domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(prepared, securityContext, probe, selection.SelectionDigest, contentDigest) != nil {
		return errors.New("case foreground registry prepared binding is invalid")
	}
	expected, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil || expected != marker {
		return errors.New("case foreground registry marker is not exact")
	}
	keyID, publicKey, signature, err := domainevidence.EvidenceSettlementAuthorityMaterial(prepared)
	if err != nil || capability.registry.authority.VerifyTrusted(capability.ctx, keyID, publicKey, domainevidence.EvidenceSettlementSigningBytes(prepared), signature) != nil {
		return errors.New("case foreground registry prepared signature is untrusted")
	}
	body, err := domainevidence.PreparedEvidenceSettlementBytes(prepared)
	if err != nil {
		return err
	}
	frozen, err := domainevidence.ParsePreparedEvidenceSettlement(body)
	if err != nil {
		return err
	}
	lease, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	effect := &caseForegroundRegistryEffectV1{owner: capability.registry, capability: capability, prepared: frozen}
	effect.active.Store(true)
	useErr := use(context.WithValue(lease, caseForegroundRegistryEffectKeyV1{}, effect))
	effect.active.Store(false)
	cancel()
	effect.mu.Lock()
	defer effect.mu.Unlock()
	bindingErr := capability.currentBinding(securityContext, probe, selection)
	if useErr != nil || bindingErr != nil || effect.failed || !effect.attempted || effect.completed == nil {
		return errors.Join(useErr, bindingErr, errors.New("case foreground registry callback did not complete exactly"))
	}
	readback, err := capability.registry.Store.Replay(capability.ctx, securityContext)
	if err != nil || !reflect.DeepEqual(readback, *effect.completed) {
		return errors.New("case foreground registry signed readback changed")
	}
	return nil
}

func (registry *caseForegroundPublicSeamRegistryV1) CommitPrepared(ctx context.Context, input registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	if ctx == nil || ctx.Err() != nil {
		return domainevidence.EvidenceReceipt{}, errors.New("case foreground registry effect context is unavailable")
	}
	effect, ok := ctx.Value(caseForegroundRegistryEffectKeyV1{}).(*caseForegroundRegistryEffectV1)
	if !ok || effect == nil {
		return domainevidence.EvidenceReceipt{}, errors.New("case foreground registry effect was not issued")
	}
	effect.mu.Lock()
	defer effect.mu.Unlock()
	fail := func() (domainevidence.EvidenceReceipt, error) {
		effect.failed = true
		return domainevidence.EvidenceReceipt{}, errors.New("case foreground registry effect binding or readback is invalid")
	}
	prepared := effect.prepared
	if !effect.active.Load() || effect.owner != registry || effect.attempted || effect.failed ||
		input.Context != prepared.SecurityContext || !reflect.DeepEqual(input.Draft, prepared.ReceiptDraft) ||
		!reflect.DeepEqual(input.CanonicalEvidence, prepared.CanonicalEvidence) ||
		input.SettlementProof != (domainevidence.EvidenceSettlementProof{SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest}) ||
		effect.capability.currentBinding(input.Context, prepared.SourceProbe, effect.capability.selection) != nil {
		return fail()
	}
	effect.attempted = true
	before, err := registry.Store.Replay(ctx, input.Context)
	if err != nil {
		return fail()
	}
	receipt, err := registry.Store.CommitPrepared(ctx, input)
	if err != nil {
		return fail()
	}
	after, err := registry.Store.Replay(ctx, input.Context)
	if err != nil || len(after.Entries) != len(before.Entries)+1 || !reflect.DeepEqual(after.Entries[:len(before.Entries)], before.Entries) {
		return fail()
	}
	resolved, found, err := domainevidence.ResolvePreparedSettlementIssue(after, prepared)
	if err != nil || !found || !reflect.DeepEqual(resolved, receipt) || !effect.active.Load() || ctx.Err() != nil ||
		effect.capability.currentBinding(input.Context, prepared.SourceProbe, effect.capability.selection) != nil {
		return fail()
	}
	effect.completed = &after
	return receipt, nil
}

// These negative probes use the actual signed prepared settlement generated by
// each public-seam scenario before its first real commit. They never mutate the
// registry, so both existing end-to-end raw/private assertions remain intact.
func assertCaseForegroundRegistryCapabilityBoundaryV1(t *testing.T, source *caseForegroundPublicSeamMCPV1,
	securityContext domainsecurity.TurnSecurityContext, prepared domainevidence.PreparedEvidenceSettlement,
	marker domainevidence.HostEvidenceSettlementMarker,
) {
	t.Helper()
	issue := func(ctx context.Context, callback func(domainsecurity.VerifiedSourceProbe, *caseForegroundPublicSeamEvidenceCapabilityV1)) {
		t.Helper()
		err := source.WithCurrentProbeAuthority(ctx, sourceprobeport.CurrentInput{
			ServerID: prepared.SourceProbe.ServerID, Context: securityContext,
			Binding: prepared.HostAuthority.Binding, ConnectionEpoch: prepared.SourceProbe.ConnectionEpoch,
		}, func(probe domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
			callback(probe, capability.(*caseForegroundPublicSeamEvidenceCapabilityV1))
			return nil
		})
		if err != nil {
			t.Error(err)
		}
	}
	for _, mismatch := range []string{"context", "source", "selection", "prepared", "marker", "cancelled", "deactivated", "dataset revoked"} {
		issue(context.Background(), func(probe domainsecurity.VerifiedSourceProbe, capability *caseForegroundPublicSeamEvidenceCapabilityV1) {
			ctx, current, candidate, currentMarker := securityContext, cloneServerCurrentSelectionV1(capability.selection), prepared, marker
			switch mismatch {
			case "context":
				ctx.TurnID += "-other"
			case "source":
				probe.ConnectionEpoch++
			case "selection":
				current.SelectionDigest = domainsecurity.SHA256Hex([]byte("other-selection"))
			case "prepared":
				candidate.ResultItemID += "-other"
			case "marker":
				currentMarker.PreparedRecordDigest = domainsecurity.SHA256Hex([]byte("other-marker"))
			case "cancelled":
				cancelled, cancel := context.WithCancel(capability.ctx)
				cancel()
				capability.ctx = cancelled
			case "deactivated":
				capability.active = false
			case "dataset revoked":
				source.dataset.SetDatasetRevoked(true)
				defer source.dataset.SetDatasetRevoked(false)
			}
			called := false
			err := capability.UseExactRegistryCommit(ctx, probe, current, candidate, currentMarker, func(context.Context) error { called = true; return nil })
			if err == nil || called {
				t.Errorf("registry capability accepted %s before callback", mismatch)
			}
		})
	}
	var escaped *caseForegroundPublicSeamEvidenceCapabilityV1
	var escapedLease context.Context
	issue(context.Background(), func(probe domainsecurity.VerifiedSourceProbe, capability *caseForegroundPublicSeamEvidenceCapabilityV1) {
		escaped = capability
		called := false
		err := capability.UseExactRegistryCommit(securityContext, probe, capability.selection, prepared, marker, func(ctx context.Context) error { called = true; escapedLease = ctx; return nil })
		if err == nil || !called {
			t.Error("registry capability must reject a successful callback without a real commit")
		}
		called = false
		if err := capability.UseExactRegistryCommit(securityContext, probe, capability.selection, prepared, marker, func(context.Context) error { called = true; return nil }); err == nil || called {
			t.Error("registry capability allowed reuse")
		}
	})
	if escapedLease == nil || escapedLease.Err() == nil {
		t.Error("registry callback lease remained live after return")
	}
	if escaped == nil {
		t.Error("current source capability was not issued")
		return
	}
	if _, err := escaped.DatasetSelection(); err == nil {
		t.Error("registry capability survived its issuing callback")
	}
	if _, err := source.registry.CommitPrepared(escapedLease, registryport.CommitPreparedInput{}); err == nil {
		t.Error("registry accepted an escaped callback lease")
	}
}
