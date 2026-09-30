package evidenceregistry

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type hostLocalRegistryHeadsV3 struct {
	mu         sync.Mutex
	mutationMu sync.Mutex
	genesis    domainhost.HeadV1
	current    domainhost.HeadV1
}

func (heads *hostLocalRegistryHeadsV3) Current(context.Context) (domainhost.HeadV1, bool, error) {
	heads.mu.Lock()
	defer heads.mu.Unlock()
	return heads.current, true, nil
}

func (heads *hostLocalRegistryHeadsV3) CurrentModeCommitment(context.Context) (domainhost.HeadV1, error) {
	return heads.genesis, nil
}

func (heads *hostLocalRegistryHeadsV3) WithProtectedMutation(ctx context.Context,
	use func(context.Context, evidenceauthorityport.HostLocalMutation) error) error {
	heads.mutationMu.Lock()
	defer heads.mutationMu.Unlock()
	return use(ctx, heads)
}

func (heads *hostLocalRegistryHeadsV3) AdvanceExact(_ context.Context,
	expected, next domainhost.HeadV1) error {
	heads.mu.Lock()
	defer heads.mu.Unlock()
	if heads.current != expected || domainhost.ValidateHeadTransitionV1(expected, next) != nil {
		return errors.New("host-local registry head CAS is not exact")
	}
	heads.current = next
	return nil
}

func TestHostLocalRegistryV3RejectsDirectCommitAndRevokesSelectedReceipt(t *testing.T) {
	ctx := context.Background()
	fixture := newWitnessedRegistryFixture(t)
	installationID := fixture.coordinator.bundle.InstallationID
	rootBinding := domainsecurity.SHA256Hex([]byte("host-local registry protected roots"))
	genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: installationID, RootBindingDigest: rootBinding,
		MutationID:                  domainsecurity.SHA256Hex([]byte("host-local registry genesis")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return fixture.authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	heads := &hostLocalRegistryHeadsV3{genesis: genesis, current: genesis}
	open := func() *HostLocalServiceV3 {
		t.Helper()
		service, err := NewHostLocalV3(ctx, HostLocalConfigV3{
			InstallationID: installationID, Authority: fixture.authority, Heads: heads,
			Indexes: fixture.indexes, Capsules: fixture.capsules, Now: func() time.Time { return fixture.now },
		})
		if err != nil {
			t.Fatal(err)
		}
		return service
	}
	service := open()
	input := witnessedRegistryIssueInput(t, fixture.securityContext, "host-local-registry")
	if receipt, err := service.CommitPrepared(ctx, input); err == nil || receipt.ReceiptID != "" ||
		len(fixture.indexes.records) != 0 || heads.current != genesis {
		t.Fatalf("direct host-local registry commit bypassed the scoped effect: receipt=%#v err=%v", receipt, err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(fixture.securityContext)
	if err != nil {
		t.Fatal(err)
	}
	registry, receipt, err := domainevidence.RegisterEvidenceReceipt(registry, input.Draft,
		input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt)
	if err != nil {
		t.Fatal(err)
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(fixture.securityContext,
		registry, fixture.authority.KeyID(), fixture.authority.PublicKey(),
		func(body []byte) ([]byte, error) { return fixture.authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainevidence.NewEvidenceRegistryAuthorityIndexHostLocalV3(
		domainevidence.EvidenceRegistryAuthorityIndexHostLocalInputV3{
			InstallationID: installationID, ModeCommitmentDigest: genesis.RecordDigest,
			Generation: 1, PreviousIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
			MutationID: domainsecurity.SHA256Hex([]byte("host-local registry first mutation")),
		}, capsule, fixture.authority.KeyID(), fixture.authority.PublicKey(),
		func(body []byte) ([]byte, error) { return fixture.authority.Sign(ctx, body) })
	if err != nil || fixture.capsules.PutIfAbsent(ctx, capsule) != nil || fixture.indexes.PutIfAbsent(ctx, index) != nil {
		t.Fatalf("host-local registry fixture was not installed: %v", err)
	}
	first, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: installationID, RootBindingDigest: rootBinding,
		Generation: 1, PreviousHeadDigest: genesis.RecordDigest,
		MutationID:                  index.MutationID,
		DatasetSnapshotIndexDigest:  genesis.DatasetSnapshotIndexDigest,
		EvidenceRegistryIndexDigest: index.IndexDigest, EvidenceRegistryCount: 1,
		PublicationIndexDigest: genesis.PublicationIndexDigest,
		AuthorityKeyID:         fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
	}, func(body []byte) ([]byte, error) { return fixture.authority.Sign(ctx, body) })
	if err != nil || heads.AdvanceExact(ctx, genesis, first) != nil {
		t.Fatalf("host-local registry fixture head was not selected: %v", err)
	}
	service = open()
	registered, err := service.Resolve(ctx, registryport.MembershipQuery{
		Context: fixture.securityContext, ReceiptID: receipt.ReceiptID,
	})
	if err != nil || registered.Revoked || !reflect.DeepEqual(registered.Receipt, receipt) {
		t.Fatalf("host-local registry restart lost exact receipt membership: %v", err)
	}
	if err := service.Revoke(ctx, registryport.RevokeInput{
		Context: fixture.securityContext, ReceiptID: receipt.ReceiptID,
		ReasonCode: "source_retracted", RevokedAt: fixture.now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if heads.current.EvidenceRegistryCount != 2 || heads.current.DatasetSnapshotCount != 0 ||
		heads.current.PublicationCount != 0 {
		t.Fatal("revocation did not advance only the selected registry child")
	}
	service = open()
	if _, err := service.Resolve(ctx, registryport.MembershipQuery{
		Context: fixture.securityContext, ReceiptID: receipt.ReceiptID,
	}); err == nil {
		t.Fatal("revoked host-local receipt remained current after restart")
	}
	if prefix, err := service.ReplayAt(ctx, fixture.securityContext, 1); err != nil || prefix.Sequence != 1 {
		t.Fatalf("host-local registry historical prefix was lost: %v", err)
	}
}
