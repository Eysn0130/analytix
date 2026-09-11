package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// FundsCanonicalCSVAdmissionMaterialKindV1 is the fixed material vocabulary
// consumed by the existing DSV2 exact-read store. It is not a registry and
// callers cannot add another material family.
type FundsCanonicalCSVAdmissionMaterialKindV1 string

const (
	FundsCanonicalCSVMaterialSnapshotManifestV1     FundsCanonicalCSVAdmissionMaterialKindV1 = "dataset-snapshot-manifest-v2"
	FundsCanonicalCSVMaterialProducerContentV1      FundsCanonicalCSVAdmissionMaterialKindV1 = "funds-producer-content-v1"
	FundsCanonicalCSVMaterialRawIntentV1            FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-acquisition-intent-v1"
	FundsCanonicalCSVMaterialRawManifestV1          FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-manifest-v1"
	FundsCanonicalCSVMaterialRawManifestPageV1      FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-manifest-page-v1"
	FundsCanonicalCSVMaterialRawEntryV1             FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-entry-v1"
	FundsCanonicalCSVMaterialRawSourceLocatorV1     FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-source-locator-v1"
	FundsCanonicalCSVMaterialRawContentRootV1       FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-content-root-v1"
	FundsCanonicalCSVMaterialRawContentIndexPageV1  FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-content-index-page-v1"
	FundsCanonicalCSVMaterialRawContentChunkV1      FundsCanonicalCSVAdmissionMaterialKindV1 = "raw-content-chunk-v1"
	FundsCanonicalCSVMaterialParsedConfigurationV1  FundsCanonicalCSVAdmissionMaterialKindV1 = "parsed-configuration-v1"
	FundsCanonicalCSVMaterialParsedIdentityV1       FundsCanonicalCSVAdmissionMaterialKindV1 = "parsed-identity-v1"
	FundsCanonicalCSVMaterialParsedReceiptV1        FundsCanonicalCSVAdmissionMaterialKindV1 = "parsed-receipt-v1"
	FundsCanonicalCSVMaterialClassificationLedgerV1 FundsCanonicalCSVAdmissionMaterialKindV1 = "classification-ledger-v1"
	FundsCanonicalCSVMaterialParsedIndexPageV1      FundsCanonicalCSVAdmissionMaterialKindV1 = "parsed-index-page-v1"
	FundsCanonicalCSVMaterialParsedPageV1           FundsCanonicalCSVAdmissionMaterialKindV1 = "parsed-page-v1"
	FundsCanonicalCSVMaterialSourceRowRootV1        FundsCanonicalCSVAdmissionMaterialKindV1 = "source-row-ledger-root-v1"
	FundsCanonicalCSVMaterialSourceRowIndexPageV1   FundsCanonicalCSVAdmissionMaterialKindV1 = "source-row-index-page-v1"
	FundsCanonicalCSVMaterialSourceRowPageV1        FundsCanonicalCSVAdmissionMaterialKindV1 = "source-row-page-v1"
	FundsCanonicalCSVMaterialSourceRowRecordV1      FundsCanonicalCSVAdmissionMaterialKindV1 = "source-row-record-v1"
	FundsCanonicalCSVMaterialSourceRowLineageV1     FundsCanonicalCSVAdmissionMaterialKindV1 = "source-row-lineage-v1"
)

var (
	fundsCanonicalCSVReaderRecordDomainV1 = []byte("analytix.funds-canonical-csv-reader-record/v1\x00")
	fundsCanonicalCSVDuplicateKeyDomainV1 = []byte("analytix.funds-canonical-csv-duplicate-key/v1\x00")
)

// FundsCanonicalCSVAdmissionMaterialV1 is callback-scoped inert CAS input.
// Body may contain source-exact private material and deliberately has no JSON
// representation. It grants no currentness, query, evidence or publication
// authority until SealedServiceV2 exact-reads the whole graph and advances the
// existing shared witness.
type FundsCanonicalCSVAdmissionMaterialV1 struct {
	kind    FundsCanonicalCSVAdmissionMaterialKindV1
	address string
	body    []byte
}

func (FundsCanonicalCSVAdmissionMaterialV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("funds canonical CSV admission material is host-private")
}

func (material FundsCanonicalCSVAdmissionMaterialV1) UseExactV1(
	consume func(FundsCanonicalCSVAdmissionMaterialKindV1, string, []byte) error,
) error {
	if consume == nil || !validFundsCanonicalCSVAdmissionMaterialKindV1(material.kind) ||
		!domainsecurity.IsSHA256Hex(material.address) || len(material.body) == 0 ||
		domainsecurity.SHA256Hex(material.body) != material.address {
		return errors.New("funds canonical CSV admission material is invalid")
	}
	body := append([]byte(nil), material.body...)
	defer clear(body)
	return consume(material.kind, material.address, body)
}

// FundsCanonicalCSVHostRowV1 is a trusted in-process carrier from the pinned
// native row projection to the fixed DSV2 graph composer. Complete values
// never receive a public JSON representation.
type FundsCanonicalCSVHostRowV1 struct {
	sourceFileID         string
	sourceRowNumber      uint64
	sourceArtifactSHA256 string
	canonicalRowSHA256   string
	canonicalTypedRow    []ParsedTypedFieldV1
}

// FundsCanonicalCSVHostTypedFieldV1 is the native-neutral input shape for the
// fixed host row constructor. It is immediately converted to the closed
// parsed evidence scalar and has no serialization or publication authority.
type FundsCanonicalCSVHostTypedFieldV1 struct {
	Name  string
	Kind  string
	Value string
}

func (FundsCanonicalCSVHostTypedFieldV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("funds canonical CSV host field is private")
}

func (FundsCanonicalCSVHostRowV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("funds canonical CSV host row is private")
}

func NewFundsCanonicalCSVHostRowV1(
	sourceFileID string,
	sourceRowNumber uint64,
	sourceArtifactSHA256 string,
	canonicalRowSHA256 string,
	canonicalTypedFields []FundsCanonicalCSVHostTypedFieldV1,
) (FundsCanonicalCSVHostRowV1, error) {
	canonicalTypedRow := make([]ParsedTypedFieldV1, len(canonicalTypedFields))
	for index, field := range canonicalTypedFields {
		canonicalTypedRow[index] = ParsedTypedFieldV1{
			Name:   field.Name,
			Scalar: ParsedTypedScalarV1{Kind: field.Kind, Value: field.Value},
		}
	}
	row := FundsCanonicalCSVHostRowV1{
		sourceFileID:         sourceFileID,
		sourceRowNumber:      sourceRowNumber,
		sourceArtifactSHA256: sourceArtifactSHA256,
		canonicalRowSHA256:   canonicalRowSHA256,
		canonicalTypedRow:    cloneParsedTypedFieldsV1(canonicalTypedRow),
	}
	if !validRawArtifactHostSourceFileIDV1(row.sourceFileID) || row.sourceRowNumber == 0 ||
		row.sourceRowNumber > maxSourceRowJSONIntegerV1 ||
		!validSourceRowSHA256V1(row.sourceArtifactSHA256) ||
		!validSourceRowSHA256V1(row.canonicalRowSHA256) ||
		validateParsedTypedRowV1(row.canonicalTypedRow) != nil ||
		row.canonicalRowSHA256 != parsedCanonicalRowSHA256V1(row.canonicalTypedRow) {
		clearFundsCanonicalCSVHostRowV1(&row)
		return FundsCanonicalCSVHostRowV1{}, errors.New("funds canonical CSV host row is invalid")
	}
	return row, nil
}

type FundsCanonicalCSVAdmissionInputV1 struct {
	Binding                  domainsecurity.DatasetSnapshotBindingKeyV1
	AcquiredAt               time.Time
	AcquisitionActorDigest   string
	IntentNonceDigest        string
	SourceArtifactSHA256     string
	SourceArtifactByteLength uint64
	SourceRowCount           uint64
}

// FundsCanonicalCSVNativeBuildContextV1 contains only composer-derived
// hashes, counts and the already-observed binding. The callback cannot choose
// raw or parsed identity and receives no source path.
type FundsCanonicalCSVNativeBuildContextV1 struct {
	Binding                        domainsecurity.DatasetSnapshotBindingKeyV1
	PrivateImportFileID            string
	RawArtifactManifestSHA256      string
	ParsedGenerationIdentitySHA256 string
	SourceArtifactSHA256           string
	SourceArtifactByteLength       uint64
	SourceRowCount                 uint64
}

// FundsCanonicalCSVNativeBuildResultV1 is an opaque, callback-scoped carrier
// for the pinned native builder result and its complete row projection.
type FundsCanonicalCSVNativeBuildResultV1 struct {
	materialization  domainsecurity.FundsMaterializationResultV1
	duckDBSHA256     string
	duckDBByteLength uint64
	rows             []FundsCanonicalCSVHostRowV1
}

func (FundsCanonicalCSVNativeBuildResultV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("funds canonical CSV native build result is host-private")
}

func NewFundsCanonicalCSVNativeBuildResultV1(
	materialization domainsecurity.FundsMaterializationResultV1,
	duckDBSHA256 string,
	duckDBByteLength uint64,
	rows []FundsCanonicalCSVHostRowV1,
) (FundsCanonicalCSVNativeBuildResultV1, error) {
	validated, err := revalidateFundsCanonicalCSVMaterializationV1(materialization)
	if err != nil || !validSourceRowSHA256V1(duckDBSHA256) || duckDBByteLength == 0 || len(rows) == 0 {
		return FundsCanonicalCSVNativeBuildResultV1{}, errors.New("funds canonical CSV native build result is invalid")
	}
	return FundsCanonicalCSVNativeBuildResultV1{
		materialization:  validated,
		duckDBSHA256:     duckDBSHA256,
		duckDBByteLength: duckDBByteLength,
		rows:             cloneFundsCanonicalCSVHostRowsV1(rows),
	}, nil
}

type FundsCanonicalCSVAdmissionSummaryV1 struct {
	ManifestSHA256       string
	ManifestByteLength   uint64
	ProducerSHA256       string
	ProducerByteLength   uint64
	SourceArtifactSHA256 string
	SourceRowCount       uint64
}

// ComposeFundsCanonicalCSVAdmissionV1 is the single fixed construction seam
// for the canonical direct-CSV funds slice. The caller must already own the
// authenticated Main-selected source and current case binding. The callback
// writes inert objects into the existing DSV2 material CAS; failure may leave
// unreferenced content-addressed objects but can never advance currentness.
func ComposeFundsCanonicalCSVAdmissionV1(
	ctx context.Context,
	input FundsCanonicalCSVAdmissionInputV1,
	source io.Reader,
	build func(context.Context, FundsCanonicalCSVNativeBuildContextV1) (FundsCanonicalCSVNativeBuildResultV1, error),
	emit func(FundsCanonicalCSVAdmissionMaterialV1) error,
) (FundsCanonicalCSVAdmissionSummaryV1, error) {
	if ctx == nil || source == nil || build == nil || emit == nil || ctx.Err() != nil ||
		domainsecurity.ValidateDatasetSnapshotBindingKeyV1(input.Binding) != nil ||
		input.AcquiredAt.IsZero() || !validSourceRowSHA256V1(input.AcquisitionActorDigest) ||
		!validSourceRowSHA256V1(input.IntentNonceDigest) ||
		!validSourceRowSHA256V1(input.SourceArtifactSHA256) ||
		input.SourceArtifactByteLength == 0 ||
		input.SourceArtifactByteLength > FundsTransactionCSVMaxArtifactBytesV1 ||
		input.SourceRowCount == 0 || input.SourceRowCount > FundsTransactionCSVMaxRowsV1 {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV admission input is invalid")
	}

	sourceBody, err := io.ReadAll(io.LimitReader(source, int64(input.SourceArtifactByteLength)+1))
	if err != nil || uint64(len(sourceBody)) != input.SourceArtifactByteLength ||
		domainsecurity.SHA256Hex(sourceBody) != input.SourceArtifactSHA256 {
		clear(sourceBody)
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV source identity changed")
	}
	defer clear(sourceBody)

	policy, ok := ResolveSourceRowProducerPolicyV1(FundsCanonicalTransactionSourceRowPolicyIDV1)
	if !ok || ValidateSourceRowProducerPolicyV1(policy) != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV row policy is unavailable")
	}
	emitter := fundsCanonicalCSVMaterialEmitterV1{ctx: ctx, emit: emit}
	privateImportFileID := domainsecurity.SHA256Hex(append(
		[]byte("analytix.funds-canonical-csv-private-file-id/v1\x00"),
		[]byte(input.SourceArtifactSHA256)...,
	))[:20]
	sourceFileID, err := DeriveFundsCanonicalCSVSourceFileIDV1(input.Binding.CaseID, privateImportFileID)
	if err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV source file identity is invalid")
	}
	intent, locator, contentRoot, rawEntry, rawPage, rawManifest, err :=
		composeFundsCanonicalCSVRawHierarchyV1(input, sourceFileID, sourceBody, &emitter)
	if err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}
	rawManifestBody, _ := RawArtifactManifestV1Bytes(rawManifest)

	configuration := NewFundsTransactionParsedConfigurationV1()
	configurationBody, err := FundsTransactionParsedConfigurationV1Bytes(configuration)
	if err != nil || emitter.put(FundsCanonicalCSVMaterialParsedConfigurationV1, configurationBody) != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV parser configuration is unavailable")
	}
	intentBody, _ := RawArtifactAcquisitionIntentV1Bytes(intent)
	identity, err := newParsedGenerationIdentityV1(parsedGenerationIdentityInputV1{
		Policy: policy, Binding: input.Binding,
		AcquisitionIntentDigest: intent.IntentDigest, AcquisitionIntentSHA256: domainsecurity.SHA256Hex(intentBody),
		AcquisitionIntentByteLength: uint64(len(intentBody)),
		RawArtifactManifestDigest:   rawManifest.ManifestDigest, RawArtifactManifestSHA256: domainsecurity.SHA256Hex(rawManifestBody),
		RawArtifactManifestLength: uint64(len(rawManifestBody)), MappingDigest: configuration.MappingDigest,
		ProjectionSchemaDigest: configuration.ProjectionSchemaDigest, ReaderModeDigest: configuration.ReaderModeDigest,
		ConfigurationDigest: configuration.ConfigurationDigest, ConfigurationSHA256: domainsecurity.SHA256Hex(configurationBody),
		ConfigurationByteLength: uint64(len(configurationBody)),
	})
	if err != nil || ValidateParsedGenerationIdentityForFundsTransactionConfigurationV1(identity, configuration) != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV parsed identity is invalid")
	}
	identityBody, _ := ParsedGenerationIdentityV1Bytes(identity)
	if err := emitter.put(FundsCanonicalCSVMaterialParsedIdentityV1, identityBody); err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}
	nativeResult, err := build(ctx, FundsCanonicalCSVNativeBuildContextV1{
		Binding:                        input.Binding,
		PrivateImportFileID:            privateImportFileID,
		RawArtifactManifestSHA256:      domainsecurity.SHA256Hex(rawManifestBody),
		ParsedGenerationIdentitySHA256: domainsecurity.SHA256Hex(identityBody),
		SourceArtifactSHA256:           input.SourceArtifactSHA256,
		SourceArtifactByteLength:       input.SourceArtifactByteLength,
		SourceRowCount:                 input.SourceRowCount,
	})
	if err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV native build failed")
	}
	defer clearFundsCanonicalCSVNativeBuildResultV1(&nativeResult)
	materialization, err := revalidateFundsCanonicalCSVMaterializationV1(nativeResult.materialization)
	if err != nil || materialization.CaseID != input.Binding.CaseID ||
		materialization.RawArtifactManifestSHA256 != domainsecurity.SHA256Hex(rawManifestBody) ||
		len(materialization.RawSourceManifest) != 1 ||
		materialization.RawSourceManifest[0].FileID != privateImportFileID ||
		materialization.RawSourceManifest[0].SHA256 != input.SourceArtifactSHA256 ||
		materialization.RawSourceManifest[0].RowsImportedNorm != input.SourceRowCount ||
		nativeResult.rows[0].sourceFileID != sourceFileID ||
		materialization.ProducerContentManifest.NormalizedRowCount != input.SourceRowCount ||
		materialization.ProducerContentManifest.AcceptedRowCount != input.SourceRowCount ||
		materialization.ProducerContentManifest.RejectedRowCount != 0 ||
		materialization.ProducerContentManifest.DuplicateRowCount != 0 ||
		uint64(len(nativeResult.rows)) != input.SourceRowCount ||
		!validSourceRowSHA256V1(nativeResult.duckDBSHA256) || nativeResult.duckDBByteLength == 0 {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV materialization is invalid")
	}
	if err := validateFundsCanonicalCSVHostRowsV1(input, nativeResult.rows, policy); err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}

	parsedPages, parsedIndexes, receipt, err := composeFundsCanonicalCSVParsedHierarchyV1(
		input, nativeResult.rows, policy, identity, rawEntry, &emitter,
	)
	if err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}
	receiptBody, _ := ParsedGenerationReceiptV1Bytes(receipt)
	if err := emitter.put(FundsCanonicalCSVMaterialParsedReceiptV1, receiptBody); err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}
	if err := emitter.put(FundsCanonicalCSVMaterialClassificationLedgerV1, receiptBody); err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}

	rowPages, rowIndexes, rowRoot, err := composeFundsCanonicalCSVRowHierarchyV1(
		input, nativeResult.rows, policy, receipt, parsedPages, parsedIndexes, intent, rawManifest, rawPage,
		rawEntry, locator, contentRoot, &emitter,
	)
	if err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}
	_ = rowPages
	_ = rowIndexes

	analyticalBinding, err := domainsecurity.NewDatasetSnapshotAnalyticalDuckDBBindingV2(
		domainsecurity.DatasetSnapshotAnalyticalDuckDBBindingInputV2{
			DuckDBSHA256: nativeResult.duckDBSHA256, DuckDBByteLength: nativeResult.duckDBByteLength,
			DuckDBContentSnapshotDigest:  materialization.DuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256: materialization.DuckDBSnapshotManifestSHA256,
			MaterializationIdentity:      materialization.MaterializationIdentity,
			SchemaDigest:                 materialization.SchemaDigest,
			QueryProfileDigest:           domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1(),
			DatasetUTCOffsetMinutes:      0, ExpectedCurrency: "CNY",
			MinorUnitScale: domainfundsquerysource.AccountFlowMinorUnitScaleV1,
		},
	)
	if err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV analytical binding is invalid")
	}
	rootBody, _ := SourceRowLedgerRootV1Bytes(rowRoot)
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(domainsecurity.DatasetSnapshotManifestInputV2{
		Binding: input.Binding, AcquisitionMethod: rawManifest.AcquisitionMethod,
		AcquiredAt: input.AcquiredAt.UTC(), AcquisitionActorDigest: input.AcquisitionActorDigest,
		RawArtifactManifestDigest: rawManifest.ManifestDigest, RawArtifactManifestSHA256: domainsecurity.SHA256Hex(rawManifestBody),
		RawArtifactManifestByteLength: uint64(len(rawManifestBody)), RawArtifactCount: rawManifest.ArtifactCount,
		FundsProducerContentManifest: materialization.ProducerContentManifest,
		SourceType:                   policy.SourceType, ProducerPolicyID: policy.PolicyID, ProducerPolicyDigest: policy.PolicyDigest,
		ProducerComponentID: policy.ProducerComponentID, ProducerComponentVersion: policy.ProducerComponentVersion,
		ProducerOperation: policy.Operation, ProducerOperationSchemaHash: policy.OperationSchemaHash,
		ParserID: policy.ParserID, ParserVersion: policy.ParserVersion,
		ParsedGenerationReceiptDigest: receipt.ReceiptDigest, ParsedGenerationReceiptSHA256: domainsecurity.SHA256Hex(receiptBody),
		ParsedGenerationReceiptByteLength: uint64(len(receiptBody)),
		ClassificationLedgerDigest:        receipt.ReceiptDigest, ClassificationLedgerSHA256: domainsecurity.SHA256Hex(receiptBody),
		ClassificationLedgerByteLength: uint64(len(receiptBody)), TimezoneSemantics: receipt.TimezoneSemantics,
		CurrencySemantics: receipt.CurrencySemantics, AnalyticalDuckDB: analyticalBinding,
		SourceRowLedgerRootDigest: rowRoot.RootDigest, SourceRowLedgerRootSHA256: domainsecurity.SHA256Hex(rootBody),
		SourceRowLedgerRootByteLength: uint64(len(rootBody)), SourceRowLedgerPageCount: rowRoot.PageCount,
		SourceRecordCount: receipt.OutcomeCount, AcceptedRecordCount: receipt.AcceptedCount,
		RejectedRecordCount: receipt.RejectedCount, DuplicateRecordCount: receipt.DuplicateCount,
	})
	if err != nil || ValidateDatasetSnapshotManifestV2RowRootStructureV1(manifest, rowRoot) != nil ||
		ValidateDatasetSnapshotManifestV2ParsedGenerationV1(manifest, receipt) != nil ||
		ValidateDatasetSnapshotManifestV2RawArtifactHierarchyV1(manifest, intent, rawManifest, []RawArtifactManifestPageV1{rawPage}) != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, errors.New("funds canonical CSV dataset snapshot manifest is invalid")
	}
	producerBody, _ := domainsecurity.FundsProducerContentManifestV1Bytes(materialization.ProducerContentManifest)
	manifestBody, _ := domainsecurity.DatasetSnapshotManifestV2Bytes(manifest)
	if err := emitter.put(FundsCanonicalCSVMaterialProducerContentV1, producerBody); err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}
	if err := emitter.put(FundsCanonicalCSVMaterialSnapshotManifestV1, manifestBody); err != nil {
		return FundsCanonicalCSVAdmissionSummaryV1{}, err
	}
	return FundsCanonicalCSVAdmissionSummaryV1{
		ManifestSHA256: domainsecurity.SHA256Hex(manifestBody), ManifestByteLength: uint64(len(manifestBody)),
		ProducerSHA256: domainsecurity.SHA256Hex(producerBody), ProducerByteLength: uint64(len(producerBody)),
		SourceArtifactSHA256: input.SourceArtifactSHA256, SourceRowCount: input.SourceRowCount,
	}, nil
}

type fundsCanonicalCSVMaterialEmitterV1 struct {
	ctx  context.Context
	emit func(FundsCanonicalCSVAdmissionMaterialV1) error
}

func (emitter *fundsCanonicalCSVMaterialEmitterV1) put(
	kind FundsCanonicalCSVAdmissionMaterialKindV1,
	body []byte,
) error {
	if emitter == nil || emitter.ctx == nil || emitter.emit == nil || emitter.ctx.Err() != nil ||
		!validFundsCanonicalCSVAdmissionMaterialKindV1(kind) || len(body) == 0 {
		return errors.New("funds canonical CSV material emitter is unavailable")
	}
	cloned := append([]byte(nil), body...)
	defer clear(cloned)
	return emitter.emit(FundsCanonicalCSVAdmissionMaterialV1{
		kind: kind, address: domainsecurity.SHA256Hex(cloned), body: cloned,
	})
}

func composeFundsCanonicalCSVRawHierarchyV1(
	input FundsCanonicalCSVAdmissionInputV1,
	privateImportFileID string,
	sourceBody []byte,
	emitter *fundsCanonicalCSVMaterialEmitterV1,
) (
	RawArtifactAcquisitionIntentV1,
	RawArtifactSourceLocatorV1,
	RawArtifactContentRootV1,
	RawArtifactEntryV1,
	RawArtifactManifestPageV1,
	RawArtifactManifestV1,
	error,
) {
	zeroIntent := RawArtifactAcquisitionIntentV1{}
	zeroLocator := RawArtifactSourceLocatorV1{}
	zeroRoot := RawArtifactContentRootV1{}
	zeroEntry := RawArtifactEntryV1{}
	zeroPage := RawArtifactManifestPageV1{}
	zeroManifest := RawArtifactManifestV1{}
	intent, err := newRawArtifactAcquisitionIntentV1(
		input.Binding, input.AcquiredAt.UTC(), input.AcquisitionActorDigest, 1, input.IntentNonceDigest,
	)
	if err != nil {
		return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, err
	}
	locator, err := newRawArtifactSourceLocatorV1(intent, 1, privateImportFileID)
	if err != nil {
		return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, err
	}

	chunkDescriptors := make([]RawArtifactContentChunkDescriptorV1, 0, (len(sourceBody)+int(RawArtifactContentChunkBytesV1)-1)/int(RawArtifactContentChunkBytesV1))
	for offset := 0; offset < len(sourceBody); offset += int(RawArtifactContentChunkBytesV1) {
		end := offset + int(RawArtifactContentChunkBytesV1)
		if end > len(sourceBody) {
			end = len(sourceBody)
		}
		chunk := sourceBody[offset:end]
		descriptor, descriptorErr := newRawArtifactContentChunkDescriptorV1(
			input.Binding.BindingKeyDigest, locator.LocatorDigest, uint64(len(chunkDescriptors)+1), uint64(offset), chunk,
		)
		if descriptorErr != nil || emitter.put(FundsCanonicalCSVMaterialRawContentChunkV1, chunk) != nil {
			return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest,
				errors.New("funds canonical CSV raw chunk is invalid")
		}
		chunkDescriptors = append(chunkDescriptors, descriptor)
	}
	indexPages := make([]RawArtifactContentIndexPageV1, 0, (len(chunkDescriptors)+maxRawArtifactChunksPerIndexPageV1-1)/maxRawArtifactChunksPerIndexPageV1)
	indexDescriptors := make([]RawArtifactContentIndexPageDescriptorV1, 0, cap(indexPages))
	for offset := 0; offset < len(chunkDescriptors); offset += maxRawArtifactChunksPerIndexPageV1 {
		end := offset + maxRawArtifactChunksPerIndexPageV1
		if end > len(chunkDescriptors) {
			end = len(chunkDescriptors)
		}
		page, pageErr := newRawArtifactContentIndexPageV1(
			input.Binding.BindingKeyDigest, locator.LocatorDigest, uint64(len(indexPages)+1), chunkDescriptors[offset:end],
		)
		if pageErr != nil {
			return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, pageErr
		}
		body, _ := RawArtifactContentIndexPageV1Bytes(page)
		if err := emitter.put(FundsCanonicalCSVMaterialRawContentIndexPageV1, body); err != nil {
			return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, err
		}
		descriptor, descriptorErr := newRawArtifactContentIndexPageDescriptorV1(page)
		if descriptorErr != nil {
			return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, descriptorErr
		}
		indexPages = append(indexPages, page)
		indexDescriptors = append(indexDescriptors, descriptor)
	}
	root, err := newRawArtifactContentRootV1(
		input.Binding.BindingKeyDigest, locator.LocatorDigest, input.SourceArtifactSHA256, indexDescriptors,
	)
	if err != nil || ValidateRawArtifactContentFullSHA256V1(root, bytes.NewReader(sourceBody)) != nil {
		return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest,
			errors.New("funds canonical CSV raw content root is invalid")
	}
	entry, err := newRawArtifactEntryV1(intent, locator, root)
	if err != nil {
		return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, err
	}
	page, err := newRawArtifactManifestPageV1(intent, 1, []RawArtifactEntryV1{entry})
	if err != nil {
		return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, err
	}
	pageDescriptor, err := newRawArtifactManifestPageDescriptorV1(page)
	if err != nil {
		return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, err
	}
	manifest, err := newRawArtifactManifestV1(intent, []RawArtifactManifestPageDescriptorV1{pageDescriptor})
	if err != nil {
		return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest, err
	}
	values := []struct {
		kind FundsCanonicalCSVAdmissionMaterialKindV1
		body []byte
	}{
		{FundsCanonicalCSVMaterialRawIntentV1, canonicalRawArtifactIntentBytesOrNilV1(intent)},
		{FundsCanonicalCSVMaterialRawSourceLocatorV1, canonicalRawArtifactSourceLocatorBytesOrNilV1(locator)},
		{FundsCanonicalCSVMaterialRawContentRootV1, canonicalRawArtifactContentRootBytesOrNilV1(root)},
		{FundsCanonicalCSVMaterialRawEntryV1, canonicalRawArtifactEntryBytesOrNilV1(entry)},
		{FundsCanonicalCSVMaterialRawManifestPageV1, canonicalRawArtifactManifestPageBytesOrNilV1(page)},
		{FundsCanonicalCSVMaterialRawManifestV1, canonicalRawArtifactManifestBytesOrNilV1(manifest)},
	}
	for _, value := range values {
		if len(value.body) == 0 || emitter.put(value.kind, value.body) != nil {
			return zeroIntent, zeroLocator, zeroRoot, zeroEntry, zeroPage, zeroManifest,
				errors.New("funds canonical CSV raw hierarchy persistence failed")
		}
	}
	return intent, locator, root, entry, page, manifest, nil
}

func composeFundsCanonicalCSVParsedHierarchyV1(
	input FundsCanonicalCSVAdmissionInputV1,
	rows []FundsCanonicalCSVHostRowV1,
	policy SourceRowProducerPolicyV1,
	identity ParsedGenerationIdentityV1,
	rawEntry RawArtifactEntryV1,
	emitter *fundsCanonicalCSVMaterialEmitterV1,
) ([]ParsedPageV1, []ParsedPageIndexV1, ParsedGenerationReceiptV1, error) {
	outcomes := make([]ParsedOutcomeV1, 0, len(rows))
	for index, row := range rows {
		locator, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{
			SourceFileID: row.sourceFileID, SourceRowNumber: row.sourceRowNumber,
		})
		if err != nil {
			return nil, nil, ParsedGenerationReceiptV1{}, err
		}
		recordID, err := DeriveSourceRowRecordIDV1(policy, input.Binding, row.sourceArtifactSHA256, locator)
		if err != nil {
			return nil, nil, ParsedGenerationReceiptV1{}, err
		}
		outcome := ParsedOutcomeV1{
			SchemaVersion: ParsedOutcomeSchemaVersionV1, Purpose: ParsedOutcomePurposeV1,
			Disposition: ParsedOutcomeAcceptedV1, OccurrenceOrdinal: uint64(index + 1),
			RawArtifactOrdinal: 1, RawArtifactEntryDigest: rawEntry.EntryDigest,
			SourceRecordID: recordID, Locator: locator, SourceArtifactSHA256: row.sourceArtifactSHA256,
			ReaderRecordSHA256: domainsecurity.SHA256Hex(append(
				append([]byte(nil), fundsCanonicalCSVReaderRecordDomainV1...), []byte(recordID)...,
			)),
			CanonicalTypedRow: cloneParsedTypedFieldsV1(row.canonicalTypedRow), CanonicalRowSHA256: row.canonicalRowSHA256,
			DuplicateKeyDigest: domainsecurity.SHA256Hex(append(
				append([]byte(nil), fundsCanonicalCSVDuplicateKeyDomainV1...), []byte(recordID)...,
			)),
		}
		if ValidateParsedOutcomeV1(identity, outcome) != nil {
			return nil, nil, ParsedGenerationReceiptV1{}, errors.New("funds canonical CSV parsed outcome is invalid")
		}
		outcomes = append(outcomes, outcome)
	}
	parsedPages := make([]ParsedPageV1, 0, (len(outcomes)+int(policy.MaxRowsPerPage)-1)/int(policy.MaxRowsPerPage))
	for offset := 0; offset < len(outcomes); offset += int(policy.MaxRowsPerPage) {
		end := offset + int(policy.MaxRowsPerPage)
		if end > len(outcomes) {
			end = len(outcomes)
		}
		page, err := newParsedPageV1(identity, uint64(len(parsedPages)+1), outcomes[offset:end])
		if err != nil {
			return nil, nil, ParsedGenerationReceiptV1{}, err
		}
		body, _ := ParsedPageV1Bytes(page)
		if err := emitter.put(FundsCanonicalCSVMaterialParsedPageV1, body); err != nil {
			return nil, nil, ParsedGenerationReceiptV1{}, err
		}
		parsedPages = append(parsedPages, page)
	}
	parsedIndexes := make([]ParsedPageIndexV1, 0, (len(parsedPages)+maxParsedPagesPerIndexV1-1)/maxParsedPagesPerIndexV1)
	for offset := 0; offset < len(parsedPages); offset += maxParsedPagesPerIndexV1 {
		end := offset + maxParsedPagesPerIndexV1
		if end > len(parsedPages) {
			end = len(parsedPages)
		}
		index, err := newParsedPageIndexV1(identity, uint64(len(parsedIndexes)+1), parsedPages[offset:end])
		if err != nil {
			return nil, nil, ParsedGenerationReceiptV1{}, err
		}
		body, _ := ParsedPageIndexV1Bytes(index)
		if err := emitter.put(FundsCanonicalCSVMaterialParsedIndexPageV1, body); err != nil {
			return nil, nil, ParsedGenerationReceiptV1{}, err
		}
		parsedIndexes = append(parsedIndexes, index)
	}
	receipt, err := newParsedGenerationReceiptV1(identity, parsedIndexes)
	if err != nil || ValidateParsedGenerationHierarchyV1(receipt, parsedIndexes, parsedPages) != nil {
		return nil, nil, ParsedGenerationReceiptV1{}, errors.New("funds canonical CSV parsed hierarchy is invalid")
	}
	return parsedPages, parsedIndexes, receipt, nil
}

func composeFundsCanonicalCSVRowHierarchyV1(
	input FundsCanonicalCSVAdmissionInputV1,
	rows []FundsCanonicalCSVHostRowV1,
	policy SourceRowProducerPolicyV1,
	receipt ParsedGenerationReceiptV1,
	parsedPages []ParsedPageV1,
	parsedIndexes []ParsedPageIndexV1,
	intent RawArtifactAcquisitionIntentV1,
	rawManifest RawArtifactManifestV1,
	rawPage RawArtifactManifestPageV1,
	rawEntry RawArtifactEntryV1,
	rawLocator RawArtifactSourceLocatorV1,
	contentRoot RawArtifactContentRootV1,
	emitter *fundsCanonicalCSVMaterialEmitterV1,
) ([]SourceRowLedgerPageV1, []SourceRowLedgerIndexPageV1, SourceRowLedgerRootV1, error) {
	receiptBody, _ := ParsedGenerationReceiptV1Bytes(receipt)
	rawManifestBody, _ := RawArtifactManifestV1Bytes(rawManifest)
	rawPageBody, _ := RawArtifactManifestPageV1Bytes(rawPage)
	rawEntryBody, _ := RawArtifactEntryV1Bytes(rawEntry)
	rawLocatorBody, _ := RawArtifactSourceLocatorV1Bytes(rawLocator)
	records := make([]SourceRowRecordV1, 0, len(rows))
	for index, row := range rows {
		pageIndex := index / int(policy.MaxRowsPerPage)
		page := parsedPages[pageIndex]
		pageBody, _ := ParsedPageV1Bytes(page)
		rowInPage := uint32(index % int(policy.MaxRowsPerPage))
		lineageInput := sourceRowRecordWithLineageInputV1{
			Policy: policy, Binding: input.Binding,
			Locator:                   SourceRowLocatorInputV1{SourceFileID: row.sourceFileID, SourceRowNumber: row.sourceRowNumber},
			RawArtifactManifestDigest: rawManifest.ManifestDigest, RawArtifactManifestSHA256: domainsecurity.SHA256Hex(rawManifestBody),
			RawArtifactManifestByteLength: uint64(len(rawManifestBody)), RawArtifactManifestPageDigest: rawPage.PageDigest,
			RawArtifactManifestPageSHA256: domainsecurity.SHA256Hex(rawPageBody), RawArtifactManifestPageByteLength: uint64(len(rawPageBody)),
			RawArtifactOrdinal: 1, RawArtifactEntryDigest: rawEntry.EntryDigest,
			RawArtifactEntrySHA256: domainsecurity.SHA256Hex(rawEntryBody), RawArtifactEntryByteLength: uint64(len(rawEntryBody)),
			RawSourceLocatorDigest: rawLocator.LocatorDigest, RawSourceLocatorSHA256: domainsecurity.SHA256Hex(rawLocatorBody),
			RawSourceLocatorByteLength: uint64(len(rawLocatorBody)), SourceArtifactSHA256: row.sourceArtifactSHA256,
			SourceArtifactByteLength: input.SourceArtifactByteLength, ParsedGenerationReceiptDigest: receipt.ReceiptDigest,
			ParsedGenerationReceiptSHA256: domainsecurity.SHA256Hex(receiptBody), ParsedGenerationReceiptByteLength: uint64(len(receiptBody)),
			ParsedPageDigest: page.PageDigest, ParsedPageSHA256: domainsecurity.SHA256Hex(pageBody), ParsedPageByteLength: uint64(len(pageBody)),
			ParsedRowOrdinal: rowInPage, MappingDigest: receipt.Identity.MappingDigest,
			ProjectionSchemaDigest: receipt.Identity.ProjectionSchemaDigest, ReaderModeDigest: receipt.Identity.ReaderModeDigest,
			CanonicalRowSHA256: row.canonicalRowSHA256,
		}
		record, lineage, err := newSourceRowRecordWithLineageV1(lineageInput)
		if err != nil || ValidateSourceRowLineageAgainstRawArtifactV1(
			lineage, intent, rawManifest, []RawArtifactManifestPageV1{rawPage}, rawEntry, rawLocator, contentRoot,
		) != nil || ValidateSourceRowLineageAgainstParsedGenerationV1(lineage, receipt, parsedIndexForPageV1(parsedIndexes, page.PageNumber), page) != nil {
			return nil, nil, SourceRowLedgerRootV1{}, errors.New("funds canonical CSV source row lineage is invalid")
		}
		lineageBody, _ := SourceRowLineageV1Bytes(lineage)
		recordBody, _ := json.Marshal(record)
		if emitter.put(FundsCanonicalCSVMaterialSourceRowLineageV1, lineageBody) != nil ||
			emitter.put(FundsCanonicalCSVMaterialSourceRowRecordV1, recordBody) != nil {
			return nil, nil, SourceRowLedgerRootV1{}, errors.New("funds canonical CSV source row persistence failed")
		}
		records = append(records, record)
	}
	rowPages := make([]SourceRowLedgerPageV1, 0, (len(records)+int(policy.MaxRowsPerPage)-1)/int(policy.MaxRowsPerPage))
	for offset := 0; offset < len(records); offset += int(policy.MaxRowsPerPage) {
		end := offset + int(policy.MaxRowsPerPage)
		if end > len(records) {
			end = len(records)
		}
		page, err := NewSourceRowLedgerPageV1(policy, input.Binding, uint64(len(rowPages)+1), records[offset:end])
		if err != nil {
			return nil, nil, SourceRowLedgerRootV1{}, err
		}
		body, _ := SourceRowLedgerPageV1Bytes(page)
		if err := emitter.put(FundsCanonicalCSVMaterialSourceRowPageV1, body); err != nil {
			return nil, nil, SourceRowLedgerRootV1{}, err
		}
		rowPages = append(rowPages, page)
	}
	rowIndexes, root, err := newSourceRowLedgerHierarchyV1FromPages(policy, input.Binding, rowPages)
	if err != nil {
		return nil, nil, SourceRowLedgerRootV1{}, err
	}
	for _, index := range rowIndexes {
		body, _ := SourceRowLedgerIndexPageV1Bytes(index)
		if err := emitter.put(FundsCanonicalCSVMaterialSourceRowIndexPageV1, body); err != nil {
			return nil, nil, SourceRowLedgerRootV1{}, err
		}
	}
	rootBody, _ := SourceRowLedgerRootV1Bytes(root)
	if err := emitter.put(FundsCanonicalCSVMaterialSourceRowRootV1, rootBody); err != nil {
		return nil, nil, SourceRowLedgerRootV1{}, err
	}
	return rowPages, rowIndexes, root, nil
}

func validateFundsCanonicalCSVHostRowsV1(
	input FundsCanonicalCSVAdmissionInputV1,
	rows []FundsCanonicalCSVHostRowV1,
	policy SourceRowProducerPolicyV1,
) error {
	previousFileDigest := ""
	previousRow := uint64(0)
	for _, row := range rows {
		if !validRawArtifactHostSourceFileIDV1(row.sourceFileID) ||
			row.sourceArtifactSHA256 != input.SourceArtifactSHA256 || row.sourceRowNumber == 0 ||
			validateParsedTypedRowV1(row.canonicalTypedRow) != nil ||
			row.canonicalRowSHA256 != parsedCanonicalRowSHA256V1(row.canonicalTypedRow) {
			return errors.New("funds canonical CSV host row changed")
		}
		locator, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{
			SourceFileID: row.sourceFileID, SourceRowNumber: row.sourceRowNumber,
		})
		if err != nil || locator.SourceFileIDDigest < previousFileDigest ||
			locator.SourceFileIDDigest == previousFileDigest && row.sourceRowNumber <= previousRow {
			return errors.New("funds canonical CSV host row order is invalid")
		}
		previousFileDigest = locator.SourceFileIDDigest
		previousRow = row.sourceRowNumber
	}
	if len(rows) == 0 || rows[0].sourceRowNumber != 1 ||
		rows[len(rows)-1].sourceRowNumber != input.SourceRowCount {
		return errors.New("funds canonical CSV host row coverage is incomplete")
	}
	return nil
}

func revalidateFundsCanonicalCSVMaterializationV1(
	value domainsecurity.FundsMaterializationResultV1,
) (domainsecurity.FundsMaterializationResultV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return domainsecurity.FundsMaterializationResultV1{}, err
	}
	return domainsecurity.ParseFundsMaterializationResultV1(body)
}

func parsedIndexForPageV1(indexes []ParsedPageIndexV1, pageNumber uint64) ParsedPageIndexV1 {
	for _, index := range indexes {
		for _, descriptor := range index.PageDescriptors {
			if descriptor.PageNumber == pageNumber {
				return index
			}
		}
	}
	return ParsedPageIndexV1{}
}

func cloneParsedTypedFieldsV1(fields []ParsedTypedFieldV1) []ParsedTypedFieldV1 {
	return append([]ParsedTypedFieldV1(nil), fields...)
}

func cloneFundsCanonicalCSVHostRowsV1(rows []FundsCanonicalCSVHostRowV1) []FundsCanonicalCSVHostRowV1 {
	cloned := make([]FundsCanonicalCSVHostRowV1, len(rows))
	for index, row := range rows {
		cloned[index] = row
		cloned[index].canonicalTypedRow = cloneParsedTypedFieldsV1(row.canonicalTypedRow)
	}
	return cloned
}

func clearFundsCanonicalCSVNativeBuildResultV1(result *FundsCanonicalCSVNativeBuildResultV1) {
	if result == nil {
		return
	}
	for index := range result.rows {
		clearFundsCanonicalCSVHostRowV1(&result.rows[index])
	}
	result.rows = nil
	result.materialization = domainsecurity.FundsMaterializationResultV1{}
	result.duckDBSHA256 = ""
	result.duckDBByteLength = 0
}

func clearFundsCanonicalCSVHostRowV1(row *FundsCanonicalCSVHostRowV1) {
	if row == nil {
		return
	}
	for index := range row.canonicalTypedRow {
		row.canonicalTypedRow[index].Name = ""
		row.canonicalTypedRow[index].Scalar.Kind = ""
		row.canonicalTypedRow[index].Scalar.Value = ""
	}
	row.canonicalTypedRow = nil
	row.sourceFileID = ""
	row.sourceArtifactSHA256 = ""
	row.canonicalRowSHA256 = ""
}

func validFundsCanonicalCSVAdmissionMaterialKindV1(kind FundsCanonicalCSVAdmissionMaterialKindV1) bool {
	switch kind {
	case FundsCanonicalCSVMaterialSnapshotManifestV1, FundsCanonicalCSVMaterialProducerContentV1,
		FundsCanonicalCSVMaterialRawIntentV1, FundsCanonicalCSVMaterialRawManifestV1,
		FundsCanonicalCSVMaterialRawManifestPageV1, FundsCanonicalCSVMaterialRawEntryV1,
		FundsCanonicalCSVMaterialRawSourceLocatorV1, FundsCanonicalCSVMaterialRawContentRootV1,
		FundsCanonicalCSVMaterialRawContentIndexPageV1, FundsCanonicalCSVMaterialRawContentChunkV1,
		FundsCanonicalCSVMaterialParsedConfigurationV1, FundsCanonicalCSVMaterialParsedIdentityV1,
		FundsCanonicalCSVMaterialParsedReceiptV1, FundsCanonicalCSVMaterialClassificationLedgerV1,
		FundsCanonicalCSVMaterialParsedIndexPageV1, FundsCanonicalCSVMaterialParsedPageV1,
		FundsCanonicalCSVMaterialSourceRowRootV1, FundsCanonicalCSVMaterialSourceRowIndexPageV1,
		FundsCanonicalCSVMaterialSourceRowPageV1, FundsCanonicalCSVMaterialSourceRowRecordV1,
		FundsCanonicalCSVMaterialSourceRowLineageV1:
		return true
	default:
		return false
	}
}

func canonicalRawArtifactIntentBytesOrNilV1(value RawArtifactAcquisitionIntentV1) []byte {
	body, _ := RawArtifactAcquisitionIntentV1Bytes(value)
	return body
}

func canonicalRawArtifactSourceLocatorBytesOrNilV1(value RawArtifactSourceLocatorV1) []byte {
	body, _ := RawArtifactSourceLocatorV1Bytes(value)
	return body
}

func canonicalRawArtifactContentRootBytesOrNilV1(value RawArtifactContentRootV1) []byte {
	body, _ := RawArtifactContentRootV1Bytes(value)
	return body
}

func canonicalRawArtifactEntryBytesOrNilV1(value RawArtifactEntryV1) []byte {
	body, _ := RawArtifactEntryV1Bytes(value)
	return body
}

func canonicalRawArtifactManifestPageBytesOrNilV1(value RawArtifactManifestPageV1) []byte {
	body, _ := RawArtifactManifestPageV1Bytes(value)
	return body
}

func canonicalRawArtifactManifestBytesOrNilV1(value RawArtifactManifestV1) []byte {
	body, _ := RawArtifactManifestV1Bytes(value)
	return body
}
