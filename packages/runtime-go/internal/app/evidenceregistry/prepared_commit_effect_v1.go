package evidenceregistry

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// A context carries only this callback's private operation identity. Completion
// can be written only by Service.CommitPrepared after its own CAS and readback.
// Neither a caller-supplied head nor a successful arbitrary callback is proof.
type preparedCommitEffectKeyV1 struct{}

type preparedCommitEffectV1 struct {
	mu          sync.Mutex
	active      atomic.Bool
	ctx         context.Context
	coordinator evidenceauthorityport.RegistryCoordinator
	before      evidenceauthorityport.FreshHead
	prepared    domainevidence.PreparedEvidenceSettlement
	attempted   bool
	failed      bool
	completed   *registryCommitReadbackV1
}

type registryCommitReadbackV1 struct {
	before  evidenceauthorityport.FreshHead
	after   evidenceauthorityport.FreshHead
	index   domainevidence.EvidenceRegistryAuthorityIndexV2
	capsule domainevidence.EvidenceRegistryAuthorityCapsule
}

// WithPreparedCommitEffectV1 observes exactly one real registry commit by the
// same Coordinator owner. The returned head is evidence for this call only;
// the dataset capability must freshly witness it before accepting the effect.
// It does not advance or replace any read capability.
func WithPreparedCommitEffectV1(
	ctx context.Context,
	coordinator evidenceauthorityport.RegistryCoordinator,
	before evidenceauthorityport.FreshHead,
	prepared domainevidence.PreparedEvidenceSettlement,
	marker domainevidence.HostEvidenceSettlementMarker,
	use func(context.Context) error,
) (evidenceauthorityport.FreshHead, error) {
	if ctx == nil || ctx.Err() != nil || use == nil || !before.HasBundle ||
		!sameCoordinatorOwnerV1(coordinator, coordinator) || ctx.Value(preparedCommitEffectKeyV1{}) != nil {
		return evidenceauthorityport.FreshHead{}, errors.New("prepared registry effect authority is unavailable")
	}
	expected, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil || expected != marker || prepared.HostAuthority == nil {
		return evidenceauthorityport.FreshHead{}, errors.New("prepared registry effect marker is invalid")
	}
	// Canonical parsing freezes slices and pointers independently of the caller.
	body, err := domainevidence.PreparedEvidenceSettlementBytes(prepared)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	frozen, err := domainevidence.ParsePreparedEvidenceSettlement(body)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	lease, cancel := context.WithCancel(ctx)
	defer cancel()
	effect := &preparedCommitEffectV1{ctx: lease, coordinator: coordinator, before: before, prepared: frozen}
	effect.active.Store(true)
	defer effect.active.Store(false)
	useErr := use(context.WithValue(lease, preparedCommitEffectKeyV1{}, effect))
	// Close before inspecting completion, then drain an in-flight call. Work
	// finishing after callback closure cannot produce an accepted handoff.
	effect.active.Store(false)
	cancel()
	effect.mu.Lock()
	defer effect.mu.Unlock()
	if useErr != nil || ctx.Err() != nil || effect.failed || !effect.attempted || effect.completed == nil {
		return evidenceauthorityport.FreshHead{}, errors.Join(useErr, ctx.Err(), errors.New("prepared registry effect did not complete exactly"))
	}
	return effect.completed.after, nil
}

func sameCoordinatorOwnerV1(left, right evidenceauthorityport.RegistryCoordinator) bool {
	l, r := reflect.ValueOf(left), reflect.ValueOf(right)
	return l.IsValid() && r.IsValid() && l.Kind() == reflect.Pointer && r.Kind() == reflect.Pointer &&
		!l.IsNil() && !r.IsNil() && l.Type() == r.Type() && l.Pointer() == r.Pointer()
}

// Called while Service.mu is held. The returned release function is held
// through the actual CAS and receipt verification, including all error exits.
func (service *Service) beginPreparedCommitEffectV1(ctx context.Context, input registryport.CommitPreparedInput) (*preparedCommitEffectV1, func(), error) {
	effect, present := ctx.Value(preparedCommitEffectKeyV1{}).(*preparedCommitEffectV1)
	if !present {
		return nil, func() {}, nil
	}
	effect.mu.Lock()
	release := func() {
		if effect.completed == nil {
			effect.failed = true
		}
		effect.mu.Unlock()
	}
	if !effect.active.Load() || effect.ctx.Err() != nil || effect.attempted || effect.failed ||
		!sameCoordinatorOwnerV1(effect.coordinator, service.coordinator) ||
		input.Context != effect.prepared.SecurityContext ||
		!reflect.DeepEqual(input.Draft, effect.prepared.ReceiptDraft) ||
		!reflect.DeepEqual(input.CanonicalEvidence, effect.prepared.CanonicalEvidence) ||
		input.SettlementProof != (domainevidence.EvidenceSettlementProof{
			SettlementID: effect.prepared.SettlementID, PreparedRecordDigest: effect.prepared.RecordDigest,
		}) {
		effect.failed = true
		release()
		return nil, func() {}, errors.New("prepared registry effect operation is not exact")
	}
	effect.attempted = true
	keyID, publicKey, signature, err := domainevidence.EvidenceSettlementAuthorityMaterial(effect.prepared)
	if err != nil || service.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainevidence.EvidenceSettlementSigningBytes(effect.prepared), signature) != nil {
		effect.failed = true
		release()
		return nil, func() {}, errors.New("prepared registry effect signature is untrusted")
	}
	return effect, release, nil
}

func (effect *preparedCommitEffectV1) finish(readback *registryCommitReadbackV1, receipt domainevidence.EvidenceReceipt) error {
	if !effect.active.Load() || effect.ctx.Err() != nil || effect.failed || readback == nil ||
		readback.before.Bundle != effect.before.Bundle ||
		domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(readback.before.Bundle, readback.after.Bundle) != nil ||
		readback.index.PreviousIndexDigest != effect.before.Bundle.EvidenceRegistryIndexDigest ||
		readback.index.Generation != effect.before.Bundle.EvidenceRegistryCount+1 ||
		readback.after.Bundle.EvidenceRegistryIndexDigest != readback.index.IndexDigest ||
		readback.index.Entry.CapsuleRecordDigest != readback.capsule.RecordDigest {
		return errors.New("prepared registry effect CAS readback is not exact")
	}
	resolved, found, err := domainevidence.ResolvePreparedSettlementIssue(readback.capsule.Registry, effect.prepared)
	if err != nil || !found || !reflect.DeepEqual(resolved, receipt) {
		return errors.New("prepared registry effect receipt readback is not exact")
	}
	effect.completed = readback
	return nil
}
