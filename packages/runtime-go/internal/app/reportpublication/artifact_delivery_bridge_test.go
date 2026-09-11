package reportpublication

import (
	"context"
	"errors"
	"testing"

	domainartifactdelivery "analytix.local/runtime-go/internal/domain/artifactdelivery"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	artifactdeliveryport "analytix.local/runtime-go/internal/ports/artifactdelivery"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

func TestArtifactDeliveryBridgeReturnsRestrictedWitnessFromExactPrivateGraph(t *testing.T) {
	projected, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureForProjectionV1(
		t, domainpublication.PIIProjectionControlledFull,
	)
	bridge, err := NewCurrentArtifactDeliveryBridgeV1(
		projected, fixture.commits, fixture.receipts, fixture.authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := bridge.ResolveCurrent(context.Background(), artifactDeliverySelectorForTestV1(selector))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainartifactdelivery.ContextBindingFromTurnSecurityContextV1(selector.SecurityContext)
	if err != nil {
		t.Fatal(err)
	}
	if domainartifactdelivery.ValidateVerifiedArtifactDeliveryV1(delivery) != nil || delivery.Context != binding ||
		delivery.DeliveryID != selector.DeliveryID || delivery.OutcomeRecordDigest != selector.OutcomeRecordDigest ||
		delivery.PublicationCommitDigest != selector.PublicationCommitDigest ||
		delivery.ExposureClass != domainartifactdelivery.ExposureClassRestrictedExactV1 ||
		!domainsecurity.IsSHA256Hex(delivery.PublicationReceiptDigest) {
		t.Fatalf("bridge returned an inexact controlled delivery witness: %#v", delivery)
	}
	called := false
	if err := bridge.WithCurrent(context.Background(), artifactDeliverySelectorForTestV1(selector),
		func(current domainartifactdelivery.VerifiedArtifactDeliveryV1) error {
			called = true
			if current != delivery {
				return errors.New("linearized delivery witness changed")
			}
			return nil
		}); err != nil || !called {
		t.Fatalf("bridge did not retain current report authority through use: called=%v err=%v", called, err)
	}
}

func TestArtifactDeliveryBridgeRejectsReceiptGraphMismatch(t *testing.T) {
	projected, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureForProjectionV1(
		t, domainpublication.PIIProjectionControlledFull,
	)
	bridge, err := NewCurrentArtifactDeliveryBridgeV1(
		projected, fixture.commits, fixture.receipts, fixture.authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.receipts.mu.Lock()
	for digest, receipt := range fixture.receipts.records {
		receipt.ReportSHA256 = domainsecurity.SHA256Hex([]byte("mismatched controlled report"))
		fixture.receipts.records[digest] = receipt
	}
	fixture.receipts.mu.Unlock()
	if _, err := bridge.ResolveCurrent(
		context.Background(), artifactDeliverySelectorForTestV1(selector),
	); !errors.Is(err, artifactdeliveryport.ErrIntegrity) {
		t.Fatalf("bridge accepted mismatched private receipt graph: %v", err)
	}
}

func TestHistoricalArtifactDeliveryBridgeCannotSatisfyCurrentAuthority(t *testing.T) {
	projected, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureForProjectionV1(
		t, domainpublication.PIIProjectionControlledFull,
	)
	bridge, err := NewHistoricalArtifactDeliveryBridgeV1(
		projected, fixture.commits, fixture.receipts, fixture.authority,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := any(bridge).(artifactdeliveryport.CurrentAuthority); ok {
		t.Fatal("historical artifact delivery bridge acquired live current authority")
	}
	binding, err := domainartifactdelivery.ContextBindingFromTurnSecurityContextV1(selector.SecurityContext)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := bridge.ResolveTrustedHistorical(context.Background(), artifactdeliveryport.HistoricalSelectorV1{
		Context: binding, DeliveryID: selector.DeliveryID, OutcomeRecordDigest: selector.OutcomeRecordDigest,
		PublicationCommitDigest: selector.PublicationCommitDigest,
	})
	if err != nil || delivery.Context != binding {
		t.Fatalf("historical bridge did not verify the exact audit graph: delivery=%#v err=%v", delivery, err)
	}
}

func artifactDeliverySelectorForTestV1(
	selector publicationport.ProjectedDeliverySelectorV1,
) artifactdeliveryport.SelectorV1 {
	return artifactdeliveryport.SelectorV1{
		SecurityContext: selector.SecurityContext, DeliveryID: selector.DeliveryID,
		OutcomeRecordDigest:     selector.OutcomeRecordDigest,
		PublicationCommitDigest: selector.PublicationCommitDigest,
	}
}
