package reportpublication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"testing"

	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestVerifyTrustedReportPublicationInventoryPinsInstallationAuthority(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil || result.Commit.RecordDigest == "" {
		t.Fatalf("publish fixture failed: result=%#v err=%v", result, err)
	}
	if err := VerifyTrustedInventoryV1(
		context.Background(), fixture.attempts, fixture.receipts, fixture.indexes,
		fixture.selections, fixture.commits, fixture.decisions, emptyReportGrantSettlementInventoryV1{}, emptyReportStageCompletionInventoryV1{}, emptyReportDeliveryProjectionInventoryV1{}, fixture.authority,
	); err != nil {
		t.Fatal(err)
	}

	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x6f}, ed25519.SeedSize))
	other := &reportPublicationAuthority{privateKey: privateKey, publicKey: privateKey.Public().(ed25519.PublicKey)}
	other.keyID = domainsecurity.SHA256Hex(other.publicKey)
	if err := VerifyTrustedInventoryV1(
		context.Background(), fixture.attempts, fixture.receipts, fixture.indexes,
		fixture.selections, fixture.commits, fixture.decisions, emptyReportGrantSettlementInventoryV1{}, emptyReportStageCompletionInventoryV1{}, emptyReportDeliveryProjectionInventoryV1{}, other,
	); err == nil {
		t.Fatal("self-signed report publication inventory crossed the installation authority")
	}
}

func TestVerifyTrustedReportPublicationInventoryRejectsMalformedOutcomeWithoutPanic(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil || result.Commit.RecordDigest == "" {
		t.Fatalf("publish fixture failed: result=%#v err=%v", result, err)
	}
	malformed := singleReportDeliveryOutcomeInventoryV1{outcome: domainpublication.ReportDeliveryOutcomeV1{
		Kind: domainpublication.ReportDeliveryOutcomeProjectedV1,
	}}
	if err := VerifyTrustedInventoryV1(
		context.Background(), fixture.attempts, fixture.receipts, fixture.indexes,
		fixture.selections, fixture.commits, fixture.decisions, emptyReportGrantSettlementInventoryV1{},
		emptyReportStageCompletionInventoryV1{}, malformed, fixture.authority,
	); err == nil {
		t.Fatal("malformed delivery outcome crossed trusted inventory")
	}
}

type emptyReportGrantSettlementInventoryV1 struct{}

func (emptyReportGrantSettlementInventoryV1) VisitGrantSettlements(
	context.Context,
	func(domainpublication.ReportGrantSettlementV1) error,
) error {
	return nil
}

type emptyReportStageCompletionInventoryV1 struct{}

func (emptyReportStageCompletionInventoryV1) VisitStageCompletions(
	context.Context,
	func(domainpublication.ReportStageCompletionV1) error,
) error {
	return nil
}

type emptyReportDeliveryProjectionInventoryV1 struct{}

func (emptyReportDeliveryProjectionInventoryV1) VisitDeliveryOutcomes(
	context.Context,
	func(domainpublication.ReportDeliveryOutcomeV1) error,
) error {
	return nil
}

type singleReportDeliveryOutcomeInventoryV1 struct {
	outcome domainpublication.ReportDeliveryOutcomeV1
}

func (inventory singleReportDeliveryOutcomeInventoryV1) VisitDeliveryOutcomes(
	_ context.Context,
	visit func(domainpublication.ReportDeliveryOutcomeV1) error,
) error {
	return visit(inventory.outcome)
}

func (store *memoryPublicationReceiptStore) VisitReceipts(_ context.Context, visit func(domainpublication.PublicationReceiptV1) error) error {
	for _, receipt := range store.records {
		if err := visit(receipt); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryPublicationIndexStore) VisitIndexes(_ context.Context, visit func(domainpublication.PublicationIndexV1) error) error {
	for _, index := range store.records {
		if err := visit(index); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryPublicationCommitStore) VisitCommitReceipts(_ context.Context, visit func(domainpublication.PublicationCommitReceiptV1) error) error {
	for _, receipt := range store.records {
		if err := visit(receipt); err != nil {
			return err
		}
	}
	return nil
}
