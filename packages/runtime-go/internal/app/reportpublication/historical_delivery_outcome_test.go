package reportpublication

import (
	"context"
	"errors"
	"testing"

	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

func TestHistoricalDeliveryOutcomeRequiresCompleteGraphForBothTerminals(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		name := "projected"
		if rejected {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			for _, scenario := range []string{"complete", "missing-completion", "missing-result", "missing-grant-settlement", "missing-witness", "missing-selection", "changed-artifact", "wrong-context", "changed-outcome"} {
				t.Run(scenario, func(t *testing.T) {
					authority, current, selector, fixture := historicalDeliveryOutcomeFixtureV1(t, rejected)
					switch scenario {
					case "missing-completion":
						fixture.stageCompletions.records = map[string]domainpublication.ReportStageCompletionV1{}
					case "missing-result":
						fixture.stages.trustedResultItem = nil
					case "missing-grant-settlement":
						fixture.grantSettlements.records = map[string]domainpublication.ReportGrantSettlementV1{}
					case "missing-witness":
						fixture.observations.records = map[string]evidenceauthorityport.ObservationBundle{}
					case "missing-selection":
						fixture.selections.records = map[string]domainpublication.PublicationCommitSelectionV1{}
					case "changed-artifact":
						fixture.artifacts.records[fixture.input.TargetIdentityDigest] = []byte("synthetic changed historical report")
					case "wrong-context":
						selector.ContextDigest = domainsecurity.SHA256Hex([]byte("another historical context"))
					case "changed-outcome":
						authority.config.DeliveryOutcomes = &sequencedProjectedOutcomeResolverV1{
							first: fixture.delivery.outcome, secondErr: publicationport.ErrNotFound,
						}
					}
					err := authority.VerifyTrustedHistoricalDeliveryOutcomeV1(context.Background(), selector)
					if scenario == "complete" {
						if err != nil {
							t.Fatalf("complete historical terminal failed audit: %v", err)
						}
						if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), current); !errors.Is(err, ErrProjectedDeliveryUnavailable) {
							t.Fatalf("historical audit gained current release authority: %v", err)
						}
					} else if !errors.Is(err, ErrProjectedDeliveryIntegrity) {
						t.Fatalf("incomplete historical terminal was accepted or misclassified: %v", err)
					}
				})
			}
		})
	}
}

func TestHistoricalDeliveryOutcomePreservesCancellation(t *testing.T) {
	authority, _, selector, _ := historicalDeliveryOutcomeFixtureV1(t, true)
	authority.config.DeliveryOutcomes = projectedOutcomeFailureStubV1{err: context.Canceled}
	if err := authority.VerifyTrustedHistoricalDeliveryOutcomeV1(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryCancelled) || !errors.Is(err, context.Canceled) || errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("historical cancellation lost its cause or classification: %v", err)
	}
}

func TestHistoricalDeliveryOutcomePreservesMixedReadFailures(t *testing.T) {
	physical := errors.New("synthetic physical inventory changed")
	for _, cancelled := range []bool{false, true} {
		authority, _, selector, _ := historicalDeliveryOutcomeFixtureV1(t, true)
		failure := errors.Join(publicationport.ErrNotFound, physical)
		if cancelled {
			failure = errors.Join(failure, context.Canceled)
		}
		authority.config.DeliveryOutcomes = projectedOutcomeFailureStubV1{err: failure}
		err := authority.VerifyTrustedHistoricalDeliveryOutcomeV1(context.Background(), selector)
		if !errors.Is(err, physical) || !errors.Is(err, publicationport.ErrNotFound) {
			t.Fatalf("mixed physical/missing failure lost a cause: %v", err)
		}
		if cancelled {
			if !errors.Is(err, ErrProjectedDeliveryCancelled) || !errors.Is(err, context.Canceled) || errors.Is(err, ErrProjectedDeliveryNotFound) {
				t.Fatalf("mixed cancellation was classified as only missing: %v", err)
			}
		} else if !errors.Is(err, ErrProjectedDeliveryNotFound) {
			t.Fatalf("non-cancelled missing failure lost its class: %v", err)
		}
	}
}

func historicalDeliveryOutcomeFixtureV1(t *testing.T, rejected bool) (*ProjectedDeliveryAuthorityV1, publicationport.ProjectedDeliverySelectorV1, publicationport.HistoricalProjectedDeliverySelectorV1, *reportPublicationFixture) {
	t.Helper()
	currentAuthority, current, _, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	if rejected {
		entry := fixtureProjectedEntryV1(t, fixture)
		rejection, err := domainpublication.NewReportDeliveryRejectionV1(
			domainpublication.ReportDeliveryRejectionInputV1{
				Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
				StageReceipt: entry.Stage, StageDisposition: *entry.Disposition, StageCompletion: *entry.StageCompletion,
				ReasonCode:                    domainpublication.ReportDeliveryRejectionEvidenceChangedV1,
				WitnessObservationDigest:      domainsecurity.SHA256Hex([]byte("historical rejection changed witness")),
				EvidenceAuthorityBundleDigest: domainsecurity.SHA256Hex([]byte("historical rejection changed bundle")),
				EvidenceRegistryIndexDigest:   domainsecurity.SHA256Hex([]byte("historical rejection changed index")),
				EvidenceRegistrySequence:      entry.Decision.EvidenceRegistrySequence + 1,
				EvidenceRegistryStateDigest:   domainsecurity.SHA256Hex([]byte("historical rejection changed state")),
				AuthorityKeyID:                fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
			},
			func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
		)
		if err != nil {
			t.Fatal(err)
		}
		fixture.delivery.outcome, err = domainpublication.RejectedReportDeliveryOutcomeV1(rejection)
		if err != nil {
			t.Fatal(err)
		}
		fixture.delivery.projection = domainpublication.ReportDeliveryProjectionV1{}
		current.OutcomeRecordDigest = rejection.RecordDigest
	}
	authority, err := NewHistoricalProjectedDeliveryAuthorityV1(currentAuthority.config)
	if err != nil {
		t.Fatal(err)
	}
	return authority, current, historicalProjectedSelectorFromCurrentV1(current), fixture
}
