package piiauthorization

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

func TestVerifyTrustedPIIAuthorizationInventoryPinsInstallationAndExactLedger(t *testing.T) {
	fixture := newServiceFixture(t)
	grant, err := fixture.service.Issue(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.grants, fixture.ledgers, fixture.authority); err != nil {
		t.Fatal(err)
	}

	otherKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x27}, ed25519.SeedSize))
	otherAuthority := &testAuthority{privateKey: otherKey, publicKey: otherKey.Public().(ed25519.PublicKey)}
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.grants, fixture.ledgers, otherAuthority); err == nil {
		t.Fatal("self-signed PII grant crossed the installation trust anchor")
	}

	delete(fixture.ledgers.records, grant.ClaimLedgerDigest)
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.grants, fixture.ledgers, fixture.authority); err == nil {
		t.Fatal("PII grant with a missing cross-owner claim ledger passed startup verification")
	}
}

func TestVerifyTrustedPIIAuthorizationInventoryKeepsHistoricalExpiryNonAuthoritative(t *testing.T) {
	fixture := newServiceFixture(t)
	if _, err := fixture.service.Issue(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	fixture.clock = fixture.now.Add(24 * time.Hour)
	if err := VerifyTrustedInventoryV1(context.Background(), fixture.grants, fixture.ledgers, fixture.authority); err != nil {
		t.Fatalf("historical expiry corrupted append-only audit inventory: %v", err)
	}
	for _, grant := range fixture.grants.records {
		projection := fixture.controlledProjection(t, grant.RecordDigest, fixture.input.ProjectedContentSHA256)
		if fixture.service.ValidateCurrent(context.Background(), fixture.context, projection, fixture.input.TargetIdentityDigest) == nil {
			t.Fatal("historical inventory verification made an expired grant current")
		}
	}
}

func TestVerifyControlledPublicationInventoryRequiresExactGrantLedgerTargetAndGrantWindow(t *testing.T) {
	t.Run("exact controlled publication", func(t *testing.T) {
		fixture, authorized, receipt := controlledPublicationInventoryFixture(t, "", time.Time{})
		projections := &controlledProjectionInventoryStore{records: map[string]domainpublication.PIIProjectionV1{
			authorized.PIIProjection.ProjectionDigest: authorized.PIIProjection,
		}}
		if err := VerifyControlledPublicationInventoryV1(
			context.Background(), receiptInventoryStub{receipt}, projections, fixture.ledgers, fixture.grants, fixture.authority,
		); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("missing grant", func(t *testing.T) {
		fixture, authorized, receipt := controlledPublicationInventoryFixture(t, "", time.Time{})
		delete(fixture.grants.records, authorized.Grant.RecordDigest)
		projections := &controlledProjectionInventoryStore{records: map[string]domainpublication.PIIProjectionV1{
			authorized.PIIProjection.ProjectionDigest: authorized.PIIProjection,
		}}
		if err := VerifyControlledPublicationInventoryV1(
			context.Background(), receiptInventoryStub{receipt}, projections, fixture.ledgers, fixture.grants, fixture.authority,
		); err == nil {
			t.Fatal("controlled publication passed without its signed grant")
		}
	})

	t.Run("different target", func(t *testing.T) {
		otherTarget := domainsecurity.SHA256Hex([]byte("other-controlled-target"))
		fixture, authorized, receipt := controlledPublicationInventoryFixture(t, otherTarget, time.Time{})
		projections := &controlledProjectionInventoryStore{records: map[string]domainpublication.PIIProjectionV1{
			authorized.PIIProjection.ProjectionDigest: authorized.PIIProjection,
		}}
		if err := VerifyControlledPublicationInventoryV1(
			context.Background(), receiptInventoryStub{receipt}, projections, fixture.ledgers, fixture.grants, fixture.authority,
		); err == nil {
			t.Fatal("controlled publication crossed its approved target")
		}
	})

	t.Run("receipt after grant expiry", func(t *testing.T) {
		fixture, authorized, _ := controlledPublicationInventoryFixture(t, "", time.Time{})
		expiresAt, err := time.Parse(time.RFC3339Nano, authorized.Grant.ExpiresAt)
		if err != nil {
			t.Fatal(err)
		}
		receipt := controlledPublicationReceiptV1(t, fixture, authorized, authorized.Metadata.TargetIdentityDigest, expiresAt.Add(time.Nanosecond))
		projections := &controlledProjectionInventoryStore{records: map[string]domainpublication.PIIProjectionV1{
			authorized.PIIProjection.ProjectionDigest: authorized.PIIProjection,
		}}
		if err := VerifyControlledPublicationInventoryV1(
			context.Background(), receiptInventoryStub{receipt}, projections, fixture.ledgers, fixture.grants, fixture.authority,
		); err == nil {
			t.Fatal("controlled publication escaped its approval/grant time window")
		}
	})
}

func controlledPublicationInventoryFixture(
	t *testing.T,
	receiptTarget string,
	receiptIssuedAt time.Time,
) (*serviceFixture, AuthorizedControlledArtifactV1, domainpublication.PublicationReceiptV1) {
	t.Helper()
	fixture := newServiceFixture(t)
	prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(fixture))
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := fixture.service.AuthorizePreparedControlledArtifact(context.Background(), AuthorizePreparedControlledArtifactInputV1{
		SecurityContext: fixture.context, ClaimLedger: fixture.input.ClaimLedger, Prepared: prepared,
		ApprovalID: fixture.input.ApprovalID, ApprovalRecordDigest: fixture.input.ApprovalRecordDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receiptTarget == "" {
		receiptTarget = authorized.Metadata.TargetIdentityDigest
	}
	if receiptIssuedAt.IsZero() {
		receiptIssuedAt = fixture.clock.Add(time.Minute)
	}
	receipt := controlledPublicationReceiptV1(t, fixture, authorized, receiptTarget, receiptIssuedAt)
	return fixture, authorized, receipt
}

func controlledPublicationReceiptV1(
	t *testing.T,
	fixture *serviceFixture,
	authorized AuthorizedControlledArtifactV1,
	target string,
	issuedAt time.Time,
) domainpublication.PublicationReceiptV1 {
	t.Helper()
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer: "analytix-controlled-host", RendererVersion: "1.0.0", ReportSHA256: authorized.ProtectedSHA256,
		ReportByteLength: authorized.ProtectedByteLength, MediaType: domainpii.ControlledPIIArtifactMediaTypeV1,
		Passed: true, IssueCodes: []string{}, InspectedAt: fixture.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpublication.NewPublicationReceiptV1(domainpublication.PublicationReceiptInputV1{
		InstallationID: domainsecurity.SHA256Hex([]byte("controlled-publication-installation")),
		EnrollmentID:   domainsecurity.SHA256Hex([]byte("controlled-publication-enrollment")), Context: fixture.context,
		ReportVariant:                 domainpublication.EvidenceBackedReport,
		EvidenceAuthorityBundleDigest: domainsecurity.SHA256Hex([]byte("controlled-publication-bundle")),
		EvidenceRegistryIndexDigest:   domainsecurity.SHA256Hex([]byte("controlled-publication-registry-index")),
		EvidenceRegistryCount:         1, EvidenceRegistrySequence: 1,
		EvidenceRegistryStateDigest: domainsecurity.SHA256Hex([]byte("controlled-publication-registry-state")),
		ClaimLedger:                 fixture.input.ClaimLedger, ReportSHA256: authorized.ProtectedSHA256,
		ReportByteLength: authorized.ProtectedByteLength, MediaType: domainpii.ControlledPIIArtifactMediaTypeV1,
		PIIProjection: authorized.PIIProjection, RenderInspection: inspection,
		Publisher: "analytix-controlled-host", PublisherVersion: "1.0.0", TargetIdentityDigest: target, IssuedAt: issuedAt,
		AuthorityKeyID: fixture.authority.KeyID(), AuthorityPublicKey: fixture.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

type receiptInventoryStub []domainpublication.PublicationReceiptV1

func (inventory receiptInventoryStub) VisitReceipts(_ context.Context, visit func(domainpublication.PublicationReceiptV1) error) error {
	for _, receipt := range inventory {
		if err := visit(receipt); err != nil {
			return err
		}
	}
	return nil
}

type controlledProjectionInventoryStore struct {
	records map[string]domainpublication.PIIProjectionV1
}

func (store *controlledProjectionInventoryStore) PutIfAbsent(_ context.Context, projection domainpublication.PIIProjectionV1) error {
	if store.records == nil {
		store.records = map[string]domainpublication.PIIProjectionV1{}
	}
	store.records[projection.ProjectionDigest] = projection
	return nil
}

func (store *controlledProjectionInventoryStore) Resolve(_ context.Context, digest string) (domainpublication.PIIProjectionV1, error) {
	projection, ok := store.records[digest]
	if !ok {
		return domainpublication.PIIProjectionV1{}, errors.New("projection missing")
	}
	return projection, nil
}

var _ publicationport.PIIProjectionStore = (*controlledProjectionInventoryStore)(nil)

func (store *memoryGrantStore) VisitGrants(_ context.Context, visit func(domainpii.PIIProjectionGrantV1) error) error {
	for _, grant := range store.records {
		if err := visit(grant); err != nil {
			return err
		}
	}
	return nil
}
