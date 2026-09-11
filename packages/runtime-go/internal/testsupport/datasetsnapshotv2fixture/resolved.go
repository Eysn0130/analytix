package datasetsnapshotv2fixture

import (
	"errors"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

// ResolvedInput describes a test-only, caller-signed V2 dataset snapshot. The
// helper deliberately owns no signing material: callers must reuse the
// authority already present in their fixture.
type ResolvedInput struct {
	TenantID           string
	UserID             string
	Observation        domainsecurity.CaseBindingObservationV1
	Material           string
	InstallationID     string
	AcceptedAt         time.Time
	AuthorityKeyID     string
	AuthorityPublicKey []byte
	Sign               func([]byte) ([]byte, error)
}

func NewResolvedSnapshotV2(input ResolvedInput) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if input.Sign == nil || domainsecurity.ValidateCaseBindingObservationV1(input.Observation) != nil ||
		input.Observation.State != domainsecurity.CaseBindingStateValid {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.New("test dataset snapshot v2 input is invalid")
	}
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID:                 input.TenantID,
		UserID:                   input.UserID,
		WorkspaceRealPath:        input.Observation.WorkspaceRealPath,
		CaseID:                   input.Observation.CaseID,
		CaseBindingHash:          input.Observation.CaseBindingHash,
		BindingObservationDigest: input.Observation.ObservationDigest,
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	seed := input.Material
	rawManifestSHA256 := testDigest("raw-manifest-bytes", seed)
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(domainsecurity.FundsProducerContentManifestInputV1{
		CaseID: input.Observation.CaseID, SourceRevision: 1,
		RawManifestSHA256:       rawManifestSHA256,
		NormalizedContentSHA256: testDigest("funds-normalized", seed),
		DetailContentSHA256:     testDigest("funds-detail", seed),
		AggregateContentSHA256:  testDigest("funds-aggregate", seed),
		KeywordContentSHA256:    testDigest("funds-keyword", seed),
		AccountContentSHA256:    testDigest("funds-account", seed),
		NormalizedRowCount:      3,
		AcceptedRowCount:        2,
		RejectedRowCount:        1,
		DuplicateRowCount:       0,
		DetailRowCount:          2,
		AggregateRowCount:       1,
		KeywordRowCount:         1,
		AccountRowCount:         1,
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(domainsecurity.DatasetSnapshotManifestInputV2{
		Binding:                           binding,
		AcquisitionMethod:                 "host_test_fixture/v1",
		AcquiredAt:                        input.AcceptedAt.Add(-time.Minute),
		AcquisitionActorDigest:            testDigest("actor", seed),
		RawArtifactManifestDigest:         testDigest("raw-manifest-digest", seed),
		RawArtifactManifestSHA256:         rawManifestSHA256,
		RawArtifactManifestByteLength:     512,
		RawArtifactCount:                  2,
		FundsProducerContentManifest:      producer,
		SourceType:                        "transaction_source_occurrence_projection",
		ProducerPolicyID:                  "funds.transaction-source-row-ledger/v1",
		ProducerPolicyDigest:              testDigest("producer-policy", seed),
		ProducerComponentID:               "data-engine",
		ProducerComponentVersion:          "analytix.data-engine-source-row/v1",
		ProducerOperation:                 "transaction_source_row_page_v1",
		ProducerOperationSchemaHash:       testDigest("producer-schema", seed),
		ParserID:                          "analytix.funds.transaction-row-parser",
		ParserVersion:                     "analytix.funds.transaction-row-parser/v1",
		ParsedGenerationReceiptDigest:     testDigest("generation-digest", seed),
		ParsedGenerationReceiptSHA256:     testDigest("generation-bytes", seed),
		ParsedGenerationReceiptByteLength: 640,
		ClassificationLedgerDigest:        testDigest("classification-digest", seed),
		ClassificationLedgerSHA256:        testDigest("classification-bytes", seed),
		ClassificationLedgerByteLength:    384,
		TimezoneSemantics:                 "unresolved",
		CurrencySemantics:                 "unresolved",
		SourceRowLedgerRootDigest:         testDigest("row-root-digest", seed),
		SourceRowLedgerRootSHA256:         testDigest("row-root-bytes", seed),
		SourceRowLedgerRootByteLength:     1024,
		SourceRowLedgerPageCount:          1,
		SourceRecordCount:                 3,
		AcceptedRecordCount:               2,
		RejectedRecordCount:               1,
		DuplicateRecordCount:              0,
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	var record domainsecurity.DatasetSnapshotAuthorityRecordV2
	err = manifest.WithExactFundsProducerAuthorityAdmissionV2(producer, func(
		issuer domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2,
	) error {
		issued, issueErr := issuer.Issue(domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
			InstallationID:     input.InstallationID,
			AcceptedAt:         input.AcceptedAt,
			AuthorityKeyID:     input.AuthorityKeyID,
			AuthorityPublicKey: input.AuthorityPublicKey,
		}, input.Sign)
		record = issued
		return issueErr
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	return datasetsnapshotport.ResolvedSnapshotV2{
		Record: record, Manifest: manifest, FundsProducerContent: producer,
	}, nil
}

func CurrentSelectionForContextV2(
	securityContext domainsecurity.TurnSecurityContext,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
) (datasetsnapshotport.CurrentSelectionV2, error) {
	selection := datasetsnapshotport.CurrentSelectionV2{
		Snapshot: datasetsnapshotport.ResolvedSnapshotV2{
			Record: domainsecurity.DatasetSnapshotAuthorityRecordV2{
				DatasetSnapshotID: securityContext.DatasetSnapshotID,
				Binding:           binding,
			},
			Manifest: domainsecurity.DatasetSnapshotManifestV2{
				Binding: binding, SourceManifestHash: securityContext.SourceManifestHash,
			},
		},
	}
	digest, err := datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	selection.SelectionDigest = digest
	return selection, nil
}

// RetainedSelectionForContextsV2 builds test-only current and historical
// snapshot material with the exact shared case binding and source hashes.
func RetainedSelectionForContextsV2(
	active domainsecurity.TurnSecurityContext,
	historical domainsecurity.TurnSecurityContext,
) datasetsnapshotport.RetainedSelectionV2 {
	binding := domainsecurity.DatasetSnapshotBindingKeyV1{
		TenantID:                 active.TenantID,
		UserID:                   active.UserID,
		WorkspaceRealPath:        active.WorkspaceRealPath,
		CaseID:                   active.CaseID,
		CaseBindingHash:          active.CaseBindingHash,
		BindingObservationDigest: active.PublicationPolicy.BindingObservationDigest,
	}
	return datasetsnapshotport.RetainedSelectionV2{
		Current: datasetsnapshotport.CurrentSelectionV2{
			Snapshot: datasetsnapshotport.ResolvedSnapshotV2{
				Record: domainsecurity.DatasetSnapshotAuthorityRecordV2{
					DatasetSnapshotID:  active.DatasetSnapshotID,
					SourceManifestHash: active.SourceManifestHash,
					Binding:            binding,
				},
				Manifest: domainsecurity.DatasetSnapshotManifestV2{
					Binding:            binding,
					SourceManifestHash: active.SourceManifestHash,
				},
			},
		},
		Snapshot: datasetsnapshotport.ResolvedSnapshotV2{
			Record: domainsecurity.DatasetSnapshotAuthorityRecordV2{
				DatasetSnapshotID:  historical.DatasetSnapshotID,
				SourceManifestHash: historical.SourceManifestHash,
				Binding:            binding,
			},
			Manifest: domainsecurity.DatasetSnapshotManifestV2{
				Binding:            binding,
				SourceManifestHash: historical.SourceManifestHash,
			},
		},
	}
}

func testDigest(kind, material string) string {
	return domainsecurity.SHA256Hex([]byte(kind + ":\x00" + material))
}
