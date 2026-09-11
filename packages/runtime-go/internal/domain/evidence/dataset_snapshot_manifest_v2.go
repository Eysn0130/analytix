package evidence

import (
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SourceRowSnapshotAcquisitionMethodFundsV2 = "host_case_import/v1"
	SourceRowSnapshotTimezoneUnresolvedV2     = "unresolved"
	SourceRowSnapshotCurrencyUnresolvedV2     = "unresolved"
)

type sourceRowDatasetSnapshotManifestInputV2 struct {
	Root                         SourceRowLedgerRootV1
	Pages                        []SourceRowLedgerPageV1
	FundsProducerContentManifest domainsecurity.FundsProducerContentManifestV1

	AcquiredAt                    time.Time
	AcquisitionActorDigest        string
	RawArtifactManifestDigest     string
	RawArtifactManifestSHA256     string
	RawArtifactManifestByteLength uint64
	RawArtifactCount              uint64

	ParsedReceipt ParsedGenerationReceiptV1
}

// newSourceRowDatasetSnapshotManifestV2 is package-private structural test
// material. Production construction must use a bounded streaming admission
// service that exact-reads raw, generation, classification, root, and page CAS
// objects before any authority record can be signed.
func newSourceRowDatasetSnapshotManifestV2(
	input sourceRowDatasetSnapshotManifestInputV2,
) (domainsecurity.DatasetSnapshotManifestV2, error) {
	policy, ok := ResolveSourceRowProducerPolicyV1(input.Root.PolicyID)
	if !ok || validateSourceRowLedgerRootWithPagesV1(policy, input.Root, input.Pages) != nil {
		return domainsecurity.DatasetSnapshotManifestV2{}, errors.New("source row snapshot root is invalid")
	}
	rootBody, err := SourceRowLedgerRootV1Bytes(input.Root)
	if err != nil {
		return domainsecurity.DatasetSnapshotManifestV2{}, errors.New("source row snapshot root bytes are invalid")
	}
	receiptBody, err := ParsedGenerationReceiptV1Bytes(input.ParsedReceipt)
	if err != nil || input.ParsedReceipt.Identity.PolicyID != policy.PolicyID ||
		input.ParsedReceipt.Identity.PolicyDigest != policy.PolicyDigest ||
		input.ParsedReceipt.Identity.BindingKeyDigest != input.Root.Binding.BindingKeyDigest ||
		input.ParsedReceipt.Identity.RawArtifactManifestDigest != strings.TrimSpace(input.RawArtifactManifestDigest) ||
		input.ParsedReceipt.Identity.RawArtifactManifestSHA256 != strings.TrimSpace(input.RawArtifactManifestSHA256) ||
		input.ParsedReceipt.Identity.RawArtifactManifestLength != input.RawArtifactManifestByteLength ||
		input.ParsedReceipt.AcceptedCount != input.Root.RecordCount {
		return domainsecurity.DatasetSnapshotManifestV2{}, errors.New("source row snapshot parsed receipt is invalid")
	}
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(domainsecurity.DatasetSnapshotManifestInputV2{
		Binding:                           input.Root.Binding,
		FundsProducerContentManifest:      input.FundsProducerContentManifest,
		AcquisitionMethod:                 SourceRowSnapshotAcquisitionMethodFundsV2,
		AcquiredAt:                        input.AcquiredAt,
		AcquisitionActorDigest:            strings.TrimSpace(input.AcquisitionActorDigest),
		RawArtifactManifestDigest:         strings.TrimSpace(input.RawArtifactManifestDigest),
		RawArtifactManifestSHA256:         strings.TrimSpace(input.RawArtifactManifestSHA256),
		RawArtifactManifestByteLength:     input.RawArtifactManifestByteLength,
		RawArtifactCount:                  input.RawArtifactCount,
		SourceType:                        policy.SourceType,
		ProducerPolicyID:                  policy.PolicyID,
		ProducerPolicyDigest:              policy.PolicyDigest,
		ProducerComponentID:               policy.ProducerComponentID,
		ProducerComponentVersion:          policy.ProducerComponentVersion,
		ProducerOperation:                 policy.Operation,
		ProducerOperationSchemaHash:       policy.OperationSchemaHash,
		ParserID:                          policy.ParserID,
		ParserVersion:                     policy.ParserVersion,
		ParsedGenerationReceiptDigest:     input.ParsedReceipt.ReceiptDigest,
		ParsedGenerationReceiptSHA256:     domainsecurity.SHA256Hex(receiptBody),
		ParsedGenerationReceiptByteLength: uint64(len(receiptBody)),
		ClassificationLedgerDigest:        input.ParsedReceipt.ReceiptDigest,
		ClassificationLedgerSHA256:        domainsecurity.SHA256Hex(receiptBody),
		ClassificationLedgerByteLength:    uint64(len(receiptBody)),
		TimezoneSemantics:                 input.ParsedReceipt.TimezoneSemantics,
		CurrencySemantics:                 input.ParsedReceipt.CurrencySemantics,
		SourceRowLedgerRootDigest:         input.Root.RootDigest,
		SourceRowLedgerRootSHA256:         domainsecurity.SHA256Hex(rootBody),
		SourceRowLedgerRootByteLength:     uint64(len(rootBody)),
		SourceRowLedgerPageCount:          input.Root.PageCount,
		SourceRecordCount:                 input.ParsedReceipt.OutcomeCount,
		AcceptedRecordCount:               input.ParsedReceipt.AcceptedCount,
		RejectedRecordCount:               input.ParsedReceipt.RejectedCount,
		DuplicateRecordCount:              input.ParsedReceipt.DuplicateCount,
	})
	if err != nil {
		return domainsecurity.DatasetSnapshotManifestV2{}, err
	}
	if err := domainsecurity.ValidateDatasetSnapshotManifestV2FundsProducerContentV1(
		manifest,
		input.FundsProducerContentManifest,
	); err != nil {
		return domainsecurity.DatasetSnapshotManifestV2{}, err
	}
	if err := ValidateDatasetSnapshotManifestV2RowRootStructureV1(manifest, input.Root); err != nil {
		return domainsecurity.DatasetSnapshotManifestV2{}, err
	}
	if err := ValidateDatasetSnapshotManifestV2ParsedGenerationV1(manifest, input.ParsedReceipt); err != nil {
		return domainsecurity.DatasetSnapshotManifestV2{}, err
	}
	return manifest, nil
}

// ValidateDatasetSnapshotManifestV2RowRootStructureV1 checks only the closed
// policy and canonical root reference. It does not read private CAS objects,
// validate every page, prove currentness, or grant factual authority.
func ValidateDatasetSnapshotManifestV2RowRootStructureV1(
	manifest domainsecurity.DatasetSnapshotManifestV2,
	root SourceRowLedgerRootV1,
) error {
	if domainsecurity.ValidateDatasetSnapshotManifestV2(manifest) != nil {
		return errors.New("dataset snapshot manifest v2 is structurally invalid")
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok || ValidateSourceRowLedgerRootV1(policy, root) != nil {
		return errors.New("dataset snapshot row ledger root is invalid")
	}
	rootBody, err := SourceRowLedgerRootV1Bytes(root)
	if err != nil {
		return errors.New("dataset snapshot row ledger root bytes are invalid")
	}
	if manifest.Binding != root.Binding || manifest.AcquisitionMethod != SourceRowSnapshotAcquisitionMethodFundsV2 ||
		manifest.SourceType != policy.SourceType || manifest.ProducerPolicyID != policy.PolicyID ||
		manifest.ProducerPolicyDigest != policy.PolicyDigest || manifest.ProducerComponentID != policy.ProducerComponentID ||
		manifest.ProducerComponentVersion != policy.ProducerComponentVersion || manifest.ProducerOperation != policy.Operation ||
		manifest.ProducerOperationSchemaHash != policy.OperationSchemaHash || manifest.ParserID != policy.ParserID ||
		manifest.ParserVersion != policy.ParserVersion || manifest.TimezoneSemantics != SourceRowSnapshotTimezoneUnresolvedV2 ||
		manifest.CurrencySemantics != SourceRowSnapshotCurrencyUnresolvedV2 ||
		manifest.SourceRowLedgerRootDigest != root.RootDigest || manifest.SourceRowLedgerRootSHA256 != domainsecurity.SHA256Hex(rootBody) ||
		manifest.SourceRowLedgerRootByteLength != uint64(len(rootBody)) || manifest.SourceRowLedgerPageCount != root.PageCount ||
		manifest.AcceptedRecordCount != root.RecordCount {
		return errors.New("dataset snapshot manifest v2 does not match the registered row ledger")
	}
	return nil
}

// ValidateDatasetSnapshotManifestV2ParsedGenerationV1 binds occurrence
// counts and the V1 classification-ledger alias to one exact typed parsed
// generation receipt. It does not establish storage membership or a fresh
// current witness.
func ValidateDatasetSnapshotManifestV2ParsedGenerationV1(
	manifest domainsecurity.DatasetSnapshotManifestV2,
	receipt ParsedGenerationReceiptV1,
) error {
	if domainsecurity.ValidateDatasetSnapshotManifestV2(manifest) != nil ||
		ValidateParsedGenerationReceiptV1(receipt) != nil {
		return errors.New("dataset snapshot parsed generation is invalid")
	}
	body, err := ParsedGenerationReceiptV1Bytes(receipt)
	if err != nil {
		return errors.New("dataset snapshot parsed generation bytes are invalid")
	}
	identity := receipt.Identity
	if manifest.Binding.BindingKeyDigest != identity.BindingKeyDigest ||
		manifest.ProducerPolicyID != identity.PolicyID || manifest.ProducerPolicyDigest != identity.PolicyDigest ||
		manifest.ParserID != identity.ParserID || manifest.ParserVersion != identity.ParserVersion ||
		manifest.RawArtifactManifestDigest != identity.RawArtifactManifestDigest ||
		manifest.RawArtifactManifestSHA256 != identity.RawArtifactManifestSHA256 ||
		manifest.RawArtifactManifestByteLength != identity.RawArtifactManifestLength ||
		manifest.ParsedGenerationReceiptDigest != receipt.ReceiptDigest ||
		manifest.ParsedGenerationReceiptSHA256 != domainsecurity.SHA256Hex(body) ||
		manifest.ParsedGenerationReceiptByteLength != uint64(len(body)) ||
		manifest.ClassificationLedgerDigest != receipt.ReceiptDigest ||
		manifest.ClassificationLedgerSHA256 != domainsecurity.SHA256Hex(body) ||
		manifest.ClassificationLedgerByteLength != uint64(len(body)) ||
		manifest.TimezoneSemantics != receipt.TimezoneSemantics ||
		manifest.CurrencySemantics != receipt.CurrencySemantics ||
		manifest.SourceRecordCount != receipt.OutcomeCount ||
		manifest.AcceptedRecordCount != receipt.AcceptedCount ||
		manifest.RejectedRecordCount != receipt.RejectedCount ||
		manifest.DuplicateRecordCount != receipt.DuplicateCount {
		return errors.New("dataset snapshot does not match its exact parsed generation receipt")
	}
	return nil
}

// ValidateDatasetSnapshotManifestV2RawArtifactHierarchyV1 binds DSV2 raw
// completeness to the exact intent and every canonical manifest page. It is
// structural comparison only; current authority still requires witnessed
// private-CAS readback in the app admission callback.
func ValidateDatasetSnapshotManifestV2RawArtifactHierarchyV1(
	manifest domainsecurity.DatasetSnapshotManifestV2,
	intent RawArtifactAcquisitionIntentV1,
	rawManifest RawArtifactManifestV1,
	rawPages []RawArtifactManifestPageV1,
) error {
	if domainsecurity.ValidateDatasetSnapshotManifestV2(manifest) != nil ||
		ValidateRawArtifactManifestHierarchyV1(intent, rawManifest, rawPages) != nil {
		return errors.New("dataset snapshot raw artifact hierarchy is invalid")
	}
	body, err := RawArtifactManifestV1Bytes(rawManifest)
	if err != nil || manifest.Binding.BindingKeyDigest != rawManifest.BindingKeyDigest ||
		manifest.AcquisitionMethod != rawManifest.AcquisitionMethod || manifest.AcquiredAt != rawManifest.AcquiredAt ||
		manifest.AcquisitionActorDigest != rawManifest.AcquisitionActorDigest ||
		manifest.RawArtifactManifestDigest != rawManifest.ManifestDigest ||
		manifest.RawArtifactManifestSHA256 != domainsecurity.SHA256Hex(body) ||
		manifest.RawArtifactManifestByteLength != uint64(len(body)) ||
		manifest.RawArtifactCount != rawManifest.ArtifactCount || manifest.RawArtifactCount != intent.ExpectedArtifactCount {
		return errors.New("dataset snapshot manifest does not match its exact raw artifact hierarchy")
	}
	return nil
}

// DatasetSnapshotManifestV2CanAuthorizeClaims is intentionally false for the
// first funds row policy. The manifest proves a target data contract only; the
// native producer, private CAS, generation completeness, account/entity
// resolver, current witness, and claim-specific callback are not composed.
func DatasetSnapshotManifestV2CanAuthorizeClaims(
	manifest domainsecurity.DatasetSnapshotManifestV2,
	root SourceRowLedgerRootV1,
) bool {
	_ = manifest
	_ = root
	return false
}
