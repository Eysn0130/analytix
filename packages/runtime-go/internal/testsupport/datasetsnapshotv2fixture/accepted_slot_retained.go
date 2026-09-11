package datasetsnapshotv2fixture

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

type AcceptedSlotRetainedAdmissionInputV1 struct {
	Binding            domainsecurity.DatasetSnapshotBindingKeyV1
	OriginalExact      string
	Canonical          string
	Field              string
	InstallationID     string
	AcceptedAt         time.Time
	AuthorityKeyID     string
	AuthorityPublicKey []byte
	Sign               func([]byte) ([]byte, error)
}

type AcceptedSlotRetainedAdmissionV1 struct {
	Materials       map[datasetsnapshotport.MaterialKindV2]map[string][]byte
	Snapshot        datasetsnapshotport.ResolvedSnapshotV2
	SourceRecordID  string
	SourceFileID    string
	SourceRowNumber uint64
}

func NewAcceptedSlotRetainedAdmissionV1(
	input AcceptedSlotRetainedAdmissionInputV1,
) (AcceptedSlotRetainedAdmissionV1, error) {
	if input.OriginalExact == "" || input.Canonical == "" || input.Sign == nil ||
		!domainevidence.IsAcceptedSlotSourceFieldV1(input.Field) {
		return AcceptedSlotRetainedAdmissionV1{}, errors.New("accepted-slot retained admission input is invalid")
	}
	source := []byte(input.Field + ",amount\r\n" + input.OriginalExact + ",1.00\r\n")
	values := map[datasetsnapshotport.MaterialKindV2]map[string][]byte{}
	var manifestBody, producerBody []byte
	var sourceFileID string
	_, err := domainevidence.ComposeFundsCanonicalCSVAdmissionV1(
		context.Background(),
		domainevidence.FundsCanonicalCSVAdmissionInputV1{
			Binding: input.Binding, AcquiredAt: time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC),
			AcquisitionActorDigest: acceptedSlotDigestV1("accepted-slot-acquisition-actor"),
			IntentNonceDigest:      acceptedSlotDigestV1("accepted-slot-intent"),
			SourceArtifactSHA256:   domainsecurity.SHA256Hex(source), SourceArtifactByteLength: uint64(len(source)),
			SourceRowCount: 1,
		},
		bytes.NewReader(source),
		func(_ context.Context, build domainevidence.FundsCanonicalCSVNativeBuildContextV1) (domainevidence.FundsCanonicalCSVNativeBuildResultV1, error) {
			var deriveErr error
			sourceFileID, deriveErr = domainevidence.DeriveFundsCanonicalCSVSourceFileIDV1(
				build.Binding.CaseID,
				build.PrivateImportFileID,
			)
			if deriveErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, deriveErr
			}
			accountNorm := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			accountRaw := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			cardNorm := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			cardRaw := domainevidence.ParsedTypedScalarV1{Kind: "null"}
			if input.Field == domainevidence.AcceptedSlotSourceFieldAccountV1 {
				accountNorm = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: input.Canonical}
				accountRaw = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: input.OriginalExact}
			} else {
				cardNorm = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: input.Canonical}
				cardRaw = domainevidence.ParsedTypedScalarV1{Kind: "text", Value: input.OriginalExact}
			}
			fields := []domainevidence.ParsedTypedFieldV1{
				{Name: "norm.clean_acct_no", Scalar: accountNorm}, {Name: "norm.clean_card_no", Scalar: cardNorm},
				{Name: "raw.acct_no", Scalar: accountRaw}, {Name: "raw.card_no", Scalar: cardRaw},
			}
			row, rowErr := domainevidence.NewFundsCanonicalCSVHostRowV1(
				sourceFileID,
				1,
				build.SourceArtifactSHA256,
				acceptedSlotParsedRowDigestV1(fields),
				[]domainevidence.FundsCanonicalCSVHostTypedFieldV1{
					{Name: "norm.clean_acct_no", Kind: accountNorm.Kind, Value: accountNorm.Value},
					{Name: "norm.clean_card_no", Kind: cardNorm.Kind, Value: cardNorm.Value},
					{Name: "raw.acct_no", Kind: accountRaw.Kind, Value: accountRaw.Value},
					{Name: "raw.card_no", Kind: cardRaw.Kind, Value: cardRaw.Value},
				},
			)
			if rowErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, rowErr
			}
			materialization, buildErr := acceptedSlotMaterializationV1(build)
			if buildErr != nil {
				return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, buildErr
			}
			return domainevidence.NewFundsCanonicalCSVNativeBuildResultV1(
				materialization,
				acceptedSlotDigestV1("accepted-slot-duckdb"),
				4096,
				[]domainevidence.FundsCanonicalCSVHostRowV1{row},
			)
		},
		func(material domainevidence.FundsCanonicalCSVAdmissionMaterialV1) error {
			return material.UseExactV1(func(kind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1, address string, body []byte) error {
				materialKind := datasetsnapshotport.MaterialKindV2(kind)
				if values[materialKind] == nil {
					values[materialKind] = map[string][]byte{}
				}
				values[materialKind][address] = append([]byte(nil), body...)
				if materialKind == datasetsnapshotport.MaterialSnapshotManifestV2 {
					manifestBody = append([]byte(nil), body...)
				} else if materialKind == datasetsnapshotport.MaterialFundsProducerContentV1 {
					producerBody = append([]byte(nil), body...)
				}
				return nil
			})
		},
	)
	if err != nil {
		return AcceptedSlotRetainedAdmissionV1{}, err
	}
	manifest, err := domainsecurity.ParseDatasetSnapshotManifestV2(manifestBody)
	if err != nil {
		return AcceptedSlotRetainedAdmissionV1{}, err
	}
	producer, err := domainsecurity.ParseFundsProducerContentManifestV1(producerBody)
	if err != nil {
		return AcceptedSlotRetainedAdmissionV1{}, err
	}
	var authorityRecord domainsecurity.DatasetSnapshotAuthorityRecordV2
	err = manifest.WithExactFundsProducerAuthorityAdmissionV2(
		producer,
		func(issuer domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2) error {
			var issueErr error
			authorityRecord, issueErr = issuer.Issue(
				domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
					InstallationID:     input.InstallationID,
					AcceptedAt:         input.AcceptedAt,
					AuthorityKeyID:     input.AuthorityKeyID,
					AuthorityPublicKey: input.AuthorityPublicKey,
				},
				input.Sign,
			)
			return issueErr
		},
	)
	if err != nil {
		return AcceptedSlotRetainedAdmissionV1{}, err
	}
	var record domainevidence.SourceRowRecordV1
	for _, body := range values[datasetsnapshotport.MaterialSourceRowPageV1] {
		page, parseErr := domainevidence.ParseSourceRowLedgerPageV1(body)
		if parseErr != nil || len(page.Entries) != 1 {
			return AcceptedSlotRetainedAdmissionV1{}, errors.New("accepted-slot retained source ledger is invalid")
		}
		record = page.Entries[0].Record
	}
	if record.SourceRecordID == "" || sourceFileID == "" {
		return AcceptedSlotRetainedAdmissionV1{}, errors.New("accepted-slot retained source identity is unavailable")
	}
	return AcceptedSlotRetainedAdmissionV1{
		Materials: values,
		Snapshot: datasetsnapshotport.ResolvedSnapshotV2{
			Record: authorityRecord, Manifest: manifest, FundsProducerContent: producer,
		},
		SourceRecordID: record.SourceRecordID, SourceFileID: sourceFileID,
		SourceRowNumber: record.Locator.SourceRowNumber,
	}, nil
}

func acceptedSlotMaterializationV1(
	build domainevidence.FundsCanonicalCSVNativeBuildContextV1,
) (domainsecurity.FundsMaterializationResultV1, error) {
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(domainsecurity.FundsProducerContentManifestInputV1{
		CaseID: build.Binding.CaseID, SourceRevision: 1, RawManifestSHA256: build.RawArtifactManifestSHA256,
		NormalizedContentSHA256: acceptedSlotDigestV1("accepted-slot-normalized"), DetailContentSHA256: acceptedSlotDigestV1("accepted-slot-detail"),
		AggregateContentSHA256: acceptedSlotDigestV1("accepted-slot-aggregate"), KeywordContentSHA256: acceptedSlotDigestV1("accepted-slot-keyword"),
		AccountContentSHA256: acceptedSlotDigestV1("accepted-slot-account"), NormalizedRowCount: 1, AcceptedRowCount: 1,
		DetailRowCount: 1, AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
	})
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, err
	}
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, err
	}
	rawSources := domainsecurity.FundsRawSourceManifestV1{{
		CleanedStatus: "done", FileID: build.PrivateImportFileID, RowsImportedNorm: 1,
		SHA256: build.SourceArtifactSHA256, Status: "已完成",
	}}
	rawBody, err := json.Marshal(rawSources)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, err
	}
	value := domainsecurity.FundsMaterializationResultV1{
		CaseID: build.Binding.CaseID, RowCount: 1,
		AggregateName:                        domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		AggregateVersion:                     domainsecurity.FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		MaterializationIdentitySchemaVersion: domainsecurity.FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    domainsecurity.DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        domainsecurity.SHA256Hex(producerBody), ProducerContentManifestByteLength: uint64(len(producerBody)),
		RawArtifactManifestSHA256: build.RawArtifactManifestSHA256,
		RawSourceManifestBase64:   base64.StdEncoding.EncodeToString(rawBody), RawSourceManifestSHA256: domainsecurity.SHA256Hex(rawBody),
		RawSourceManifestByteLength: uint64(len(rawBody)), DuckDBContentSnapshotDigest: acceptedSlotDigestV1("accepted-slot-duckdb-content"),
		DuckDBSnapshotManifestSHA256: acceptedSlotDigestV1("accepted-slot-duckdb-manifest"), SchemaDigest: domainsecurity.FixedFundsAnalyticalSchemaDigestV1(),
	}
	body, err := json.Marshal(value)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, err
	}
	return domainsecurity.ParseFundsMaterializationResultV1(body)
}

func acceptedSlotParsedRowDigestV1(fields []domainevidence.ParsedTypedFieldV1) string {
	body, _ := json.Marshal(fields)
	return domainsecurity.SHA256Hex(append([]byte("analytix.parsed-canonical-row/digest/v1\x00"), body...))
}

func acceptedSlotDigestV1(value string) string {
	return domainsecurity.SHA256Hex([]byte(value))
}
