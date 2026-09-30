package evidenceregistry

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type hostLocalPreparedEffectKeyV3 struct{}

type hostLocalPreparedEffectV3 struct {
	mu        sync.Mutex
	active    atomic.Bool
	ctx       context.Context
	heads     evidenceauthorityport.HostLocalHeadCoordinator
	mutation  evidenceauthorityport.HostLocalMutation
	before    domainhost.HeadV1
	prepared  domainevidence.PreparedEvidenceSettlement
	attempted bool
	failed    bool
	after     *domainhost.HeadV1
}

// WithHostLocalPreparedCommitEffectV3 holds the common child-writer serializer
// through one concrete Registry.CommitPrepared call and exact signed-head
// readback. An arbitrary successful callback is not a receipt commit.
func WithHostLocalPreparedCommitEffectV3(ctx context.Context,
	heads evidenceauthorityport.HostLocalHeadCoordinator, before domainhost.HeadV1,
	prepared domainevidence.PreparedEvidenceSettlement,
	marker domainevidence.HostEvidenceSettlementMarker,
	use func(context.Context) error) (domainhost.HeadV1, error) {
	if ctx == nil || ctx.Err() != nil || heads == nil || use == nil ||
		domainhost.ValidateHeadV1(before) != nil || ctx.Value(hostLocalPreparedEffectKeyV3{}) != nil {
		return domainhost.HeadV1{}, ErrAuthorityUnavailable
	}
	expected, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil || expected != marker || prepared.HostAuthority == nil {
		return domainhost.HeadV1{}, ErrAuthorityUnavailable
	}
	body, err := domainevidence.PreparedEvidenceSettlementBytes(prepared)
	if err != nil {
		return domainhost.HeadV1{}, err
	}
	frozen, err := domainevidence.ParsePreparedEvidenceSettlement(body)
	if err != nil {
		return domainhost.HeadV1{}, err
	}
	var committed domainhost.HeadV1
	err = heads.WithProtectedMutation(ctx, func(lease context.Context,
		mutation evidenceauthorityport.HostLocalMutation) error {
		current, found, err := heads.Current(lease)
		if err != nil || !found || current != before {
			return errors.Join(ErrAuthorityIntegrity, err)
		}
		effectCtx, cancel := context.WithCancel(lease)
		defer cancel()
		effect := &hostLocalPreparedEffectV3{
			ctx: effectCtx, heads: heads, mutation: mutation, before: before, prepared: frozen,
		}
		effect.active.Store(true)
		useErr := use(context.WithValue(effectCtx, hostLocalPreparedEffectKeyV3{}, effect))
		effect.active.Store(false)
		cancel()
		effect.mu.Lock()
		defer effect.mu.Unlock()
		if useErr != nil || effect.failed || !effect.attempted || effect.after == nil {
			return errors.Join(useErr, errors.New("host-local prepared registry effect did not complete exactly"))
		}
		current, found, err = heads.Current(lease)
		if err != nil || !found || current != *effect.after {
			return errors.Join(ErrAuthorityIntegrity, err)
		}
		committed = current
		return nil
	})
	if err != nil {
		return domainhost.HeadV1{}, err
	}
	return committed, nil
}

func sameHostLocalHeadOwnerV3(left, right evidenceauthorityport.HostLocalHeadCoordinator) bool {
	l, r := reflect.ValueOf(left), reflect.ValueOf(right)
	return l.IsValid() && r.IsValid() && l.Kind() == reflect.Pointer && r.Kind() == reflect.Pointer &&
		!l.IsNil() && !r.IsNil() && l.Type() == r.Type() && l.Pointer() == r.Pointer()
}

func (service *HostLocalServiceV3) beginPreparedEffectV3(ctx context.Context,
	input registryport.CommitPreparedInput) (*hostLocalPreparedEffectV3, func(), error) {
	effect, present := ctx.Value(hostLocalPreparedEffectKeyV3{}).(*hostLocalPreparedEffectV3)
	if !present {
		return nil, nil, ErrAuthorityUnavailable
	}
	effect.mu.Lock()
	release := func() {
		if effect.after == nil {
			effect.failed = true
		}
		effect.mu.Unlock()
	}
	if !effect.active.Load() || effect.ctx.Err() != nil || effect.attempted || effect.failed ||
		!sameHostLocalHeadOwnerV3(effect.heads, service.heads) ||
		input.Context != effect.prepared.SecurityContext ||
		!reflect.DeepEqual(input.Draft, effect.prepared.ReceiptDraft) ||
		!reflect.DeepEqual(input.CanonicalEvidence, effect.prepared.CanonicalEvidence) ||
		input.SettlementProof != (domainevidence.EvidenceSettlementProof{
			SettlementID: effect.prepared.SettlementID, PreparedRecordDigest: effect.prepared.RecordDigest,
		}) {
		effect.failed = true
		release()
		return nil, nil, ErrAuthorityIntegrity
	}
	effect.attempted = true
	keyID, publicKey, signature, err := domainevidence.EvidenceSettlementAuthorityMaterial(effect.prepared)
	if err != nil || service.base.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainevidence.EvidenceSettlementSigningBytes(effect.prepared), signature) != nil {
		effect.failed = true
		release()
		return nil, nil, ErrAuthorityIntegrity
	}
	return effect, release, nil
}

func (effect *hostLocalPreparedEffectV3) finish(after domainhost.HeadV1,
	registry domainevidence.EvidenceReceiptRegistry, receipt domainevidence.EvidenceReceipt) error {
	if !effect.active.Load() || effect.ctx.Err() != nil || effect.failed ||
		domainhost.ValidateHeadTransitionV1(effect.before, after) != nil ||
		after.EvidenceRegistryCount != effect.before.EvidenceRegistryCount+1 ||
		after.DatasetSnapshotIndexDigest != effect.before.DatasetSnapshotIndexDigest ||
		after.DatasetSnapshotCount != effect.before.DatasetSnapshotCount ||
		after.PublicationIndexDigest != effect.before.PublicationIndexDigest ||
		after.PublicationCount != effect.before.PublicationCount {
		return ErrAuthorityIntegrity
	}
	resolved, found, err := domainevidence.ResolvePreparedSettlementIssue(registry, effect.prepared)
	if err != nil || !found || !reflect.DeepEqual(resolved, receipt) {
		return errors.Join(ErrAuthorityIntegrity, err)
	}
	effect.after = &after
	return nil
}
