package evidenceregistry

import (
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"errors"
	"io"
	"reflect"
	"sort"
	"sync"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type HostLocalConfigV3 struct {
	InstallationID   string
	Authority        finalauthorityport.Authority
	Heads            evidenceauthorityport.HostLocalHeadCoordinator
	Indexes          registryport.AuthorityIndexStore
	Capsules         registryport.AuthorityCapsuleStore
	DatasetAuthority datasetsnapshotport.CurrentAuthorityV2
	BindingObserver  casecontextport.Observer
	Random           io.Reader
	Now              func() time.Time
}

// HostLocalServiceV3 selects the existing receipt capsule graph only through
// the signed host-local child root. It cannot issue a witnessed snapshot.
type HostLocalServiceV3 struct {
	base                 *Service
	heads                evidenceauthorityport.HostLocalHeadCoordinator
	modeCommitmentDigest string
	rootBindingDigest    string
	mu                   sync.Mutex
}

var _ registryport.Registry = (*HostLocalServiceV3)(nil)
var _ registryport.LockedSnapshot = (*HostLocalServiceV3)(nil)
var _ registryport.HistoricalReplay = (*HostLocalServiceV3)(nil)
var _ registryport.Inventory = (*HostLocalServiceV3)(nil)

func NewHostLocalV3(ctx context.Context, config HostLocalConfigV3) (*HostLocalServiceV3, error) {
	if config.Random == nil {
		config.Random = cryptorand.Reader
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if ctx == nil || config.Heads == nil || config.Authority == nil || config.Indexes == nil ||
		config.Capsules == nil || !domainsecurity.IsSHA256Hex(config.InstallationID) ||
		len(config.Authority.PublicKey()) != ed25519.PublicKeySize ||
		config.Authority.KeyID() != domainsecurity.SHA256Hex(config.Authority.PublicKey()) {
		return nil, ErrAuthorityUnavailable
	}
	genesis, err := config.Heads.CurrentModeCommitment(ctx)
	if err != nil || domainhost.ValidateHeadV1(genesis) != nil || genesis.Generation != 0 || genesis.InstallationID != config.InstallationID ||
		genesis.AuthorityKeyID != config.Authority.KeyID() {
		return nil, errors.Join(ErrAuthorityUnavailable, err)
	}
	base := &Service{
		installationID: config.InstallationID, authority: config.Authority,
		keyID: config.Authority.KeyID(), publicKey: append([]byte(nil), config.Authority.PublicKey()...),
		indexes: config.Indexes, capsules: config.Capsules, datasetAuthority: config.DatasetAuthority,
		bindingObserver: config.BindingObserver,
		random:          config.Random, now: config.Now,
	}
	return &HostLocalServiceV3{base: base, heads: config.Heads,
		modeCommitmentDigest: genesis.RecordDigest, rootBindingDigest: genesis.RootBindingDigest}, nil
}

func (service *HostLocalServiceV3) currentLocked(ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext) (domainhost.HeadV1, registrySelectionV2, error) {
	if service == nil || service.base == nil || ctx == nil {
		return domainhost.HeadV1{}, registrySelectionV2{}, ErrAuthorityUnavailable
	}
	head, found, err := service.heads.Current(ctx)
	if err != nil || !found || domainhost.ValidateHeadV1(head) != nil ||
		head.RootBindingDigest != service.rootBindingDigest ||
		head.InstallationID != service.base.installationID ||
		head.AuthorityKeyID != service.base.keyID {
		return domainhost.HeadV1{}, registrySelectionV2{}, errors.Join(ErrAuthorityUnavailable, err)
	}
	selection, err := service.base.registrySelectionFromRootLocked(ctx,
		head.EvidenceRegistryIndexDigest, head.EvidenceRegistryCount, true,
		service.modeCommitmentDigest, securityContext)
	if err != nil {
		return domainhost.HeadV1{}, registrySelectionV2{}, err
	}
	if err := service.confirmHead(ctx, head); err != nil {
		return domainhost.HeadV1{}, registrySelectionV2{}, err
	}
	return head, selection, nil
}

func (service *HostLocalServiceV3) confirmHead(ctx context.Context, expected domainhost.HeadV1) error {
	current, found, err := service.heads.Current(ctx)
	if err != nil || !found || domainhost.ValidateHeadV1(current) != nil ||
		current.RootBindingDigest != service.rootBindingDigest || current != expected {
		return errors.Join(ErrAuthorityIntegrity, err)
	}
	return nil
}

func (service *HostLocalServiceV3) CommitPrepared(ctx context.Context,
	input registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	if service == nil || ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		!domainsecurity.IsHostToolCallIDV1(input.Draft.ToolCallID) {
		return domainevidence.EvidenceReceipt{}, ErrAuthorityUnavailable
	}
	effect, releaseEffect, err := service.beginPreparedEffectV3(ctx, input)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	defer releaseEffect()
	service.mu.Lock()
	defer service.mu.Unlock()
	head, selection, err := service.currentLocked(ctx, input.Context)
	if err != nil || head != effect.before {
		return domainevidence.EvidenceReceipt{}, errors.Join(ErrAuthorityIntegrity, err)
	}
	if _, matched, err := domainevidence.MatchEvidenceReceiptRegistration(selection.registry,
		input.Draft, input.CanonicalEvidence, input.SettlementProof); err != nil || matched {
		return domainevidence.EvidenceReceipt{}, errors.Join(ErrAuthorityIntegrity, err)
	}
	next, issued, err := domainevidence.RegisterEvidenceReceipt(selection.registry, input.Draft,
		input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	committed, after, err := service.commitLocked(ctx, effect.mutation, head, input.Context, next)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	registered, err := domainevidence.VerifyEvidenceReceiptMembership(committed, input.Context, issued.ReceiptID)
	if err != nil || registered.Revoked || !reflect.DeepEqual(registered.Receipt, issued) ||
		effect.finish(after, committed, issued) != nil {
		return domainevidence.EvidenceReceipt{}, ErrAuthorityIntegrity
	}
	return issued, nil
}

func (service *HostLocalServiceV3) Resolve(ctx context.Context,
	query registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(query.Context) != nil {
		return domainevidence.RegisteredEvidence{}, ErrAuthorityUnavailable
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	_, selection, err := service.currentLocked(ctx, query.Context)
	if err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	return domainevidence.VerifyEvidenceReceiptMembership(selection.registry, query.Context, query.ReceiptID)
}

func (service *HostLocalServiceV3) Revoke(ctx context.Context, input registryport.RevokeInput) error {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil {
		return ErrAuthorityUnavailable
	}
	return service.heads.WithProtectedMutation(ctx, func(lease context.Context,
		mutation evidenceauthorityport.HostLocalMutation) error {
		service.mu.Lock()
		defer service.mu.Unlock()
		head, selection, err := service.currentLocked(lease, input.Context)
		if err != nil {
			return err
		}
		if matched, err := domainevidence.MatchEvidenceReceiptRevocation(selection.registry,
			input.ReceiptID, input.ReasonCode); err != nil {
			return err
		} else if matched {
			return nil
		}
		next, err := domainevidence.RevokeEvidenceReceipt(selection.registry, input.ReceiptID,
			input.ReasonCode, input.RevokedAt)
		if err != nil {
			return err
		}
		committed, _, err := service.commitLocked(lease, mutation, head, input.Context, next)
		if err != nil {
			return err
		}
		if _, err := domainevidence.VerifyEvidenceReceiptMembership(committed, input.Context,
			input.ReceiptID); err == nil {
			return ErrAuthorityIntegrity
		}
		return nil
	})
}

func (service *HostLocalServiceV3) Replay(ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil {
		return domainevidence.EvidenceReceiptRegistry{}, ErrAuthorityUnavailable
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	_, selection, err := service.currentLocked(ctx, securityContext)
	return selection.registry, err
}

func (service *HostLocalServiceV3) ReplayAt(ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext, sequence uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	current, err := service.Replay(ctx, securityContext)
	if err != nil || sequence > current.Sequence {
		return domainevidence.EvidenceReceiptRegistry{}, errors.Join(ErrAuthorityUnavailable, err)
	}
	prefix, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	for _, entry := range current.Entries[:sequence] {
		prefix, err = domainevidence.ApplyEvidenceRegistryEntry(prefix, entry)
		if err != nil {
			return domainevidence.EvidenceReceiptRegistry{}, err
		}
	}
	return prefix, nil
}

func (service *HostLocalServiceV3) WithLockedSnapshot(ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(domainevidence.EvidenceReceiptRegistry) error) error {
	if service == nil || ctx == nil || callback == nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil {
		return ErrAuthorityUnavailable
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	head, selection, err := service.currentLocked(ctx, securityContext)
	if err != nil {
		return err
	}
	copy, err := domainevidence.ParseEvidenceReceiptRegistry(selection.registry)
	if err != nil {
		return err
	}
	callbackErr := callback(copy)
	return errors.Join(callbackErr, service.confirmHead(ctx, head))
}

func (service *HostLocalServiceV3) HasRecords(ctx context.Context) (bool, error) {
	if service == nil || ctx == nil {
		return false, ErrAuthorityUnavailable
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	head, found, err := service.heads.Current(ctx)
	if err != nil || !found || domainhost.ValidateHeadV1(head) != nil ||
		head.RootBindingDigest != service.rootBindingDigest ||
		head.InstallationID != service.base.installationID ||
		head.AuthorityKeyID != service.base.keyID {
		return false, errors.Join(ErrAuthorityUnavailable, err)
	}
	if head.EvidenceRegistryCount == 0 {
		if head.EvidenceRegistryIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
			return false, ErrAuthorityIntegrity
		}
		return false, nil
	}
	if head.EvidenceRegistryCount > maxRegistryAuthorityIndexDepthV2 {
		return false, ErrAuthorityIntegrity
	}
	root, err := service.base.indexes.Resolve(ctx, head.EvidenceRegistryIndexDigest)
	if err != nil || domainevidence.ValidateEvidenceRegistryAuthorityIndexForHostLocalV3(root,
		service.base.installationID, service.modeCommitmentDigest, service.base.keyID,
		service.base.publicKey) != nil ||
		domainevidence.ValidateEvidenceRegistryAuthorityIndexHostLocalRootV3(root,
			head.EvidenceRegistryIndexDigest, head.EvidenceRegistryCount) != nil {
		return false, errors.Join(ErrAuthorityIntegrity, err)
	}
	return true, service.confirmHead(ctx, head)
}

func (service *HostLocalServiceV3) ListRegistries(ctx context.Context,
	contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	if service == nil || ctx == nil {
		return nil, ErrAuthorityUnavailable
	}
	contextByIdentity := make(map[string]domainsecurity.TurnSecurityContext, len(contexts))
	for _, securityContext := range contexts {
		if domainsecurity.ValidateTurnSecurityContext(securityContext) != nil {
			return nil, ErrAuthorityIntegrity
		}
		identity := securityContext.ThreadID + "\x00" + securityContext.TurnID
		if _, duplicate := contextByIdentity[identity]; duplicate {
			return nil, ErrAuthorityIntegrity
		}
		contextByIdentity[identity] = securityContext
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	head, found, err := service.heads.Current(ctx)
	if err != nil || !found || domainhost.ValidateHeadV1(head) != nil ||
		head.RootBindingDigest != service.rootBindingDigest ||
		head.InstallationID != service.base.installationID ||
		head.AuthorityKeyID != service.base.keyID {
		return nil, errors.Join(ErrAuthorityUnavailable, err)
	}
	root, count := head.EvidenceRegistryIndexDigest, head.EvidenceRegistryCount
	if count == 0 {
		if root != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
			return nil, ErrAuthorityIntegrity
		}
		return []registryport.InventoryRecord{}, service.confirmHead(ctx, head)
	}
	if count > maxRegistryAuthorityIndexDepthV2 || root == domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
		return nil, ErrAuthorityIntegrity
	}
	type inventorySelection struct {
		entry   domainevidence.EvidenceRegistryAuthorityIndexEntry
		capsule domainevidence.EvidenceRegistryAuthorityCapsule
	}
	latest := make(map[string]domainevidence.EvidenceReceiptRegistry, len(contexts))
	lineages := make(map[string][]inventorySelection, len(contexts))
	currentDigest := root
	var newer *domainevidence.EvidenceRegistryAuthorityIndexV2
	for generation := count; generation > 0; generation-- {
		index, err := service.base.indexes.Resolve(ctx, currentDigest)
		if err != nil || domainevidence.ValidateEvidenceRegistryAuthorityIndexForHostLocalV3(index,
			service.base.installationID, service.modeCommitmentDigest,
			service.base.keyID, service.base.publicKey) != nil ||
			index.Generation != generation || index.IndexDigest != currentDigest {
			return nil, ErrAuthorityIntegrity
		}
		if generation == count && domainevidence.ValidateEvidenceRegistryAuthorityIndexHostLocalRootV3(index,
			root, count) != nil {
			return nil, ErrAuthorityIntegrity
		}
		if newer != nil && domainevidence.ValidateEvidenceRegistryAuthorityIndexHostLocalTransitionV3(index, *newer) != nil {
			return nil, ErrAuthorityIntegrity
		}
		identity := index.Entry.ThreadID + "\x00" + index.Entry.TurnID
		securityContext, present := contextByIdentity[identity]
		if !present || index.Entry.ContextDigest != securityContext.ContextDigest ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
			return nil, ErrAuthorityIntegrity
		}
		capsule, err := service.base.capsules.Resolve(ctx, index.Entry.CapsuleRecordDigest)
		if err != nil || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleHostLocalV3(index, capsule) ||
			!reflect.DeepEqual(capsule.SecurityContext, securityContext) {
			return nil, ErrAuthorityIntegrity
		}
		if _, selected := latest[identity]; !selected {
			latest[identity] = capsule.Registry
		}
		lineages[identity] = append(lineages[identity], inventorySelection{entry: index.Entry, capsule: capsule})
		if generation == 1 {
			if index.PreviousIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
				return nil, ErrAuthorityIntegrity
			}
			break
		}
		newer = &index
		currentDigest = index.PreviousIndexDigest
	}
	for _, lineage := range lineages {
		sort.Slice(lineage, func(i, j int) bool { return lineage[i].entry.RegistrySequence < lineage[j].entry.RegistrySequence })
		if lineage[0].entry.RegistrySequence != 1 {
			return nil, ErrAuthorityIntegrity
		}
		for position := 1; position < len(lineage); position++ {
			if domainevidence.ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(
				lineage[position-1].entry, lineage[position].entry, lineage[position].capsule) != nil {
				return nil, ErrAuthorityIntegrity
			}
		}
	}
	if err := service.confirmHead(ctx, head); err != nil {
		return nil, err
	}
	records := make([]registryport.InventoryRecord, 0, len(latest))
	for _, securityContext := range contexts {
		identity := securityContext.ThreadID + "\x00" + securityContext.TurnID
		if registry, selected := latest[identity]; selected {
			records = append(records, registryport.InventoryRecord{Context: securityContext, Registry: registry})
		}
	}
	return records, nil
}

func (service *HostLocalServiceV3) commitLocked(ctx context.Context,
	mutation evidenceauthorityport.HostLocalMutation, before domainhost.HeadV1,
	securityContext domainsecurity.TurnSecurityContext,
	nextRegistry domainevidence.EvidenceReceiptRegistry,
) (domainevidence.EvidenceReceiptRegistry, domainhost.HeadV1, error) {
	if mutation == nil || before.Generation == ^uint64(0) || before.EvidenceRegistryCount == ^uint64(0) ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainevidence.ValidateEvidenceReceiptRegistry(nextRegistry) != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, ErrAuthorityIntegrity
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(securityContext, nextRegistry,
		service.base.keyID, service.base.publicKey,
		func(message []byte) ([]byte, error) { return service.base.authority.Sign(ctx, message) })
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, err
	}
	mutationID, err := service.base.nextMutationIDLocked(ctx, before.RecordDigest, securityContext.ContextDigest)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, err
	}
	index, err := domainevidence.NewEvidenceRegistryAuthorityIndexHostLocalV3(
		domainevidence.EvidenceRegistryAuthorityIndexHostLocalInputV3{
			InstallationID: service.base.installationID, ModeCommitmentDigest: service.modeCommitmentDigest,
			Generation:          before.EvidenceRegistryCount + 1,
			PreviousIndexDigest: before.EvidenceRegistryIndexDigest, MutationID: mutationID,
		}, capsule, service.base.keyID, service.base.publicKey,
		func(message []byte) ([]byte, error) { return service.base.authority.Sign(ctx, message) })
	if err != nil || service.base.validateEntryAppendFromRootLocked(ctx, before.EvidenceRegistryIndexDigest,
		before.EvidenceRegistryCount, index, capsule) != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, errors.Join(ErrAuthorityIntegrity, err)
	}
	if err := service.base.capsules.PutIfAbsent(ctx, capsule); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, err
	}
	readCapsule, err := service.base.capsules.Resolve(ctx, capsule.RecordDigest)
	if err != nil || !reflect.DeepEqual(readCapsule, capsule) {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, ErrAuthorityIntegrity
	}
	if err := service.base.indexes.PutIfAbsent(ctx, index); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, err
	}
	readIndex, err := service.base.indexes.Resolve(ctx, index.IndexDigest)
	if err != nil || readIndex != index {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, ErrAuthorityIntegrity
	}
	nextHead, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: before.InstallationID, RootBindingDigest: before.RootBindingDigest,
		Generation: before.Generation + 1, PreviousHeadDigest: before.RecordDigest,
		MutationID: mutationID, DatasetSnapshotIndexDigest: before.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount:        before.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: index.IndexDigest,
		EvidenceRegistryCount:       before.EvidenceRegistryCount + 1,
		PublicationIndexDigest:      before.PublicationIndexDigest, PublicationCount: before.PublicationCount,
		AuthorityKeyID: service.base.keyID, AuthorityPublicKey: service.base.publicKey,
	}, func(message []byte) ([]byte, error) { return service.base.authority.Sign(ctx, message) })
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, err
	}
	if err := mutation.AdvanceExact(ctx, before, nextHead); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, err
	}
	selectedHead, selected, err := service.currentLocked(ctx, securityContext)
	if err != nil || selectedHead != nextHead || selected.registry.StateDigest != nextRegistry.StateDigest ||
		selected.registry.Sequence != nextRegistry.Sequence {
		return domainevidence.EvidenceReceiptRegistry{}, domainhost.HeadV1{}, errors.Join(ErrAuthorityIntegrity, err)
	}
	return selected.registry, nextHead, nil
}
