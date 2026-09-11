package piiauthorization

import (
	"context"
	"testing"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
)

func TestStoredControlledPublicationBindingsPreserveCompleteGraph(t *testing.T) {
	for _, scenario := range []string{"healthy", "missing grant", "artifact binding", "missing commit"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newControlledAccessInventoryFixtureV1(t)
			access := fixture.accessReceipt
			ctx := context.Background()
			receipt, err := fixture.receipts.Resolve(ctx, access.PublicationReceiptDigest)
			if err != nil {
				t.Fatal(err)
			}
			commit, err := fixture.commits.Resolve(ctx, access.PublicationCommitDigest)
			if err != nil {
				t.Fatal(err)
			}
			index, err := fixture.indexes.Resolve(ctx, commit.PublicationIndexDigest)
			if err != nil {
				t.Fatal(err)
			}
			grant, err := fixture.grants.ResolveGrant(ctx, access.PIIAuthorizationDigest)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := fixture.ledgers.Resolve(ctx, access.ClaimLedgerDigest)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := fixture.projections.Resolve(ctx, access.PIIProjectionDigest)
			if err != nil {
				t.Fatal(err)
			}
			inspection, err := fixture.inspections.Resolve(ctx, receipt.RenderInspectionDigest)
			if err != nil {
				t.Fatal(err)
			}
			materials := StoredControlledPublicationMaterialsV1{
				Grant: grant, Receipt: receipt, Commit: commit, Index: index, Ledger: ledger,
				Projection: projection, Inspection: inspection, Artifact: fixture.artifacts.metadata,
			}
			switch scenario {
			case "missing grant":
				materials.Grant = domainpii.PIIProjectionGrantV1{}
			case "artifact binding":
				materials.Artifact.ClaimLedgerDigest = ""
			case "missing commit":
				materials.Commit = domainpublication.PublicationCommitReceiptV1{}
			}
			candidateErr := ValidateStoredControlledPublicationV1(materials)
			if (candidateErr == nil) != (scenario == "healthy" || scenario == "missing commit") {
				t.Fatalf("controlled candidate binding classification: %v", candidateErr)
			}
			accessErr := ValidateStoredControlledAccessPublicationV1(access, materials)
			if (accessErr == nil) != (scenario == "healthy") {
				t.Fatalf("controlled access binding classification: %v", accessErr)
			}
		})
	}
}
