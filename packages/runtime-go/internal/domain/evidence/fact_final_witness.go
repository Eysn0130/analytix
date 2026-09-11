package evidence

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	FactFinalWitnessAdmissionSchemaVersionV1 = 1
	FactFinalWitnessAdmissionPurposeV1       = "analytix.fact-final-witness-admission/v1"
	FactFinalWitnessAdmissionSchemaVersionV2 = 2
	FactFinalWitnessAdmissionPurposeV2       = "analytix.fact-final-witness-admission/v2"

	// The unversioned aliases name the only version that may be newly issued.
	// V1 remains parseable for historical audit and exact byte verification.
	FactFinalWitnessAdmissionSchemaVersion = FactFinalWitnessAdmissionSchemaVersionV2
	FactFinalWitnessAdmissionPurpose       = FactFinalWitnessAdmissionPurposeV2
)

var factFinalWitnessReceiptSetDomainV1 = []byte("analytix.fact-final-witness-receipt-set/v1\x00")

// FactFinalWitnessAdmissionV1 retains its historical type name so signed V1
// bytes remain decodable. Newly issued schema V2 values bind one fact-bearing
// final to the exact current-run evidence-authority observation, selected
// registry, witnessed dataset root, selected DSV2 record/manifest, and
// canonical funds producer content. The compact binding is signed as part of
// AcceptedFinalRecord; live restart authority additionally requires replay
// against a newly observed witness head, so this value alone can never
// resurrect an old local snapshot.
type FactFinalWitnessAdmissionV1 struct {
	SchemaVersion                          int                               `json:"schemaVersion"`
	Purpose                                string                            `json:"purpose"`
	ContextDigest                          string                            `json:"contextDigest"`
	DatasetSnapshotID                      string                            `json:"datasetSnapshotId"`
	SourceManifestHash                     string                            `json:"sourceManifestHash"`
	EnvelopeDigest                         string                            `json:"envelopeDigest"`
	RenderedTextSHA256                     string                            `json:"renderedTextSha256"`
	PublicationSnapshotProofDigest         string                            `json:"publicationSnapshotProofDigest"`
	RegistrySequence                       uint64                            `json:"registrySequence"`
	RegistryStateDigest                    string                            `json:"registryStateDigest"`
	EvidenceReceiptIDsDigest               string                            `json:"evidenceReceiptIdsDigest"`
	EvidenceReceiptCount                   uint64                            `json:"evidenceReceiptCount"`
	EvidenceAuthorityBundleDigest          string                            `json:"evidenceAuthorityBundleDigest"`
	EvidenceAuthorityBundleGeneration      uint64                            `json:"evidenceAuthorityBundleGeneration"`
	EvidenceRegistryIndexDigest            string                            `json:"evidenceRegistryIndexDigest"`
	EvidenceRegistryCount                  uint64                            `json:"evidenceRegistryCount"`
	SelectedRegistryIndexDigest            string                            `json:"selectedRegistryIndexDigest"`
	SelectedRegistryIndexGeneration        uint64                            `json:"selectedRegistryIndexGeneration"`
	SelectedRegistryCapsuleDigest          string                            `json:"selectedRegistryCapsuleDigest"`
	DatasetSnapshotIndexDigest             string                            `json:"datasetSnapshotIndexDigest,omitempty"`
	DatasetSnapshotCount                   uint64                            `json:"datasetSnapshotCount,omitempty"`
	SelectedDatasetSnapshotIndexDigest     string                            `json:"selectedDatasetSnapshotIndexDigest,omitempty"`
	SelectedDatasetSnapshotIndexGeneration uint64                            `json:"selectedDatasetSnapshotIndexGeneration,omitempty"`
	SelectedDatasetSnapshotRecordDigest    string                            `json:"selectedDatasetSnapshotRecordDigest,omitempty"`
	DatasetSnapshotManifestDigest          string                            `json:"datasetSnapshotManifestDigest,omitempty"`
	FundsProducerContentID                 string                            `json:"fundsProducerContentId,omitempty"`
	FundsProducerContentManifestSHA256     string                            `json:"fundsProducerContentManifestSha256,omitempty"`
	WitnessBinding                         EvidenceAuthorityWitnessBindingV1 `json:"witnessBinding"`
	AdmittedAt                             string                            `json:"admittedAt"`
	AdmissionDigest                        string                            `json:"admissionDigest"`
}

type FactFinalWitnessAdmissionInputV1 struct {
	Context              domainsecurity.TurnSecurityContext
	Envelope             FinalAnswerEnvelope
	RenderedText         string
	PublicationProof     *PublicationSnapshotProof
	Registry             EvidenceReceiptRegistry
	Bundle               EvidenceAuthorityBundleV1
	ObserveRequest       domainsecurity.MonotonicHeadObserveRequestV1
	Observation          domainsecurity.MonotonicHeadObservationV1
	RootIndex            EvidenceRegistryAuthorityIndexV2
	SelectedIndex        EvidenceRegistryAuthorityIndexV2
	SelectedCapsule      EvidenceRegistryAuthorityCapsule
	RegistryIndexPath    []EvidenceRegistryAuthorityIndexV2
	DatasetRootIndex     domainsecurity.DatasetSnapshotIndexV1
	SelectedDatasetIndex domainsecurity.DatasetSnapshotIndexV1
	DatasetIndexPath     []domainsecurity.DatasetSnapshotIndexV1
	DatasetRecord        domainsecurity.DatasetSnapshotAuthorityRecordV2
	DatasetManifest      domainsecurity.DatasetSnapshotManifestV2
	FundsProducerContent domainsecurity.FundsProducerContentManifestV1
	BindingObservation   domainsecurity.CaseBindingObservationV1
	InstallationID       string
	EnrollmentID         string
	AuthorityKeyID       string
	AuthorityPublicKey   []byte
	WitnessKeyID         string
	WitnessPublicKey     []byte
	AdmittedAt           time.Time
}

func NewFactFinalWitnessAdmissionV1(input FactFinalWitnessAdmissionInputV1) (FactFinalWitnessAdmissionV1, error) {
	admittedAt := input.AdmittedAt.UTC()
	if admittedAt.IsZero() {
		admittedAt = time.Now().UTC()
	}
	input.AdmittedAt = admittedAt
	admission, err := newFactFinalWitnessAdmissionWithoutExactV1(
		input,
		FactFinalWitnessAdmissionSchemaVersionV2,
	)
	if err != nil {
		return FactFinalWitnessAdmissionV1{}, err
	}
	if err := ValidateFactFinalWitnessAdmissionExactV1(admission, input); err != nil {
		return FactFinalWitnessAdmissionV1{}, err
	}
	return admission, nil
}

func ValidateFactFinalWitnessAdmissionV1(admission FactFinalWitnessAdmissionV1) error {
	parsed, timeErr := time.Parse(time.RFC3339Nano, admission.AdmittedAt)
	if !validFactFinalWitnessAdmissionVersionV1(admission) ||
		!validSHA256(admission.ContextDigest) || strings.TrimSpace(admission.DatasetSnapshotID) == "" ||
		!validSHA256(admission.SourceManifestHash) || !validSHA256(admission.EnvelopeDigest) ||
		!validSHA256(admission.RenderedTextSHA256) || !validSHA256(admission.PublicationSnapshotProofDigest) ||
		admission.RegistrySequence == 0 || !validSHA256(admission.RegistryStateDigest) ||
		!validSHA256(admission.EvidenceReceiptIDsDigest) || admission.EvidenceReceiptCount == 0 ||
		!validSHA256(admission.EvidenceAuthorityBundleDigest) || admission.EvidenceAuthorityBundleGeneration == 0 ||
		!validSHA256(admission.EvidenceRegistryIndexDigest) || admission.EvidenceRegistryCount == 0 ||
		!validSHA256(admission.SelectedRegistryIndexDigest) || admission.SelectedRegistryIndexGeneration == 0 ||
		admission.SelectedRegistryIndexGeneration > admission.EvidenceRegistryCount ||
		!validSHA256(admission.SelectedRegistryCapsuleDigest) ||
		ValidateEvidenceAuthorityWitnessBindingV1(admission.WitnessBinding) != nil ||
		timeErr != nil || parsed.Location() != time.UTC || parsed.UTC().Format(time.RFC3339Nano) != admission.AdmittedAt ||
		!validSHA256(admission.AdmissionDigest) || admission.AdmissionDigest != factFinalWitnessAdmissionDigestV1(admission) {
		return errors.New("fact final witness admission is invalid")
	}
	if admission.WitnessBinding.BundleRecordDigest != admission.EvidenceAuthorityBundleDigest ||
		admission.WitnessBinding.BundleGeneration != admission.EvidenceAuthorityBundleGeneration {
		return errors.New("fact final witness admission bundle binding is invalid")
	}
	return nil
}

func ValidateFactFinalWitnessAdmissionExactV1(admission FactFinalWitnessAdmissionV1, input FactFinalWitnessAdmissionInputV1) error {
	if err := ValidateFactFinalWitnessAdmissionV1(admission); err != nil {
		return err
	}
	expected := input
	expected.AdmittedAt, _ = time.Parse(time.RFC3339Nano, admission.AdmittedAt)
	rebuilt, err := newFactFinalWitnessAdmissionWithoutExactV1(expected, admission.SchemaVersion)
	if err != nil || !reflect.DeepEqual(admission, rebuilt) {
		return errors.New("fact final witness admission does not match exact authority")
	}
	return nil
}

func newFactFinalWitnessAdmissionWithoutExactV1(
	input FactFinalWitnessAdmissionInputV1,
	schemaVersion int,
) (FactFinalWitnessAdmissionV1, error) {
	// Avoid recursive exact validation while retaining the constructor's full
	// contract. The public constructor performs the same checks before calling
	// this helper through the exact comparator.
	binding, err := NewEvidenceAuthorityWitnessBindingV1(
		input.Bundle, input.ObserveRequest, input.Observation,
		input.InstallationID, input.EnrollmentID, input.AuthorityKeyID, input.AuthorityPublicKey,
		input.WitnessKeyID, input.WitnessPublicKey,
	)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		ValidateFinalAnswerEnvelope(input.Envelope) != nil || !FinalAnswerRequiresPublicationSnapshotProof(input.Envelope) ||
		!EvidenceReceiptRegistryMatchesContext(input.Registry, input.Context) ||
		ValidatePublicationSnapshotProofAgainstRegistry(input.PublicationProof, input.Context, input.Envelope, input.Registry) != nil ||
		validateFactFinalRegistrySelectionV1(input) != nil {
		return FactFinalWitnessAdmissionV1{}, errors.New("fact final witness admission exact input is invalid")
	}
	switch schemaVersion {
	case FactFinalWitnessAdmissionSchemaVersionV1:
		if !factFinalWitnessDatasetInputIsEmptyV1(input) {
			return FactFinalWitnessAdmissionV1{}, errors.New("legacy fact final witness admission carries dataset authority")
		}
	case FactFinalWitnessAdmissionSchemaVersionV2:
		if validateFactFinalDatasetSelectionV2(input) != nil {
			return FactFinalWitnessAdmissionV1{}, errors.New("fact final witness dataset authority is invalid")
		}
	default:
		return FactFinalWitnessAdmissionV1{}, errors.New("fact final witness admission version is invalid")
	}
	rendered, renderErr := RenderFinalAnswer(input.Envelope)
	checkedAt, checkedErr := time.Parse(time.RFC3339Nano, input.PublicationProof.CheckedAt)
	admittedAt := input.AdmittedAt.UTC()
	if renderErr != nil || rendered != input.RenderedText || admittedAt.IsZero() || checkedErr != nil || admittedAt.Before(checkedAt) {
		return FactFinalWitnessAdmissionV1{}, errors.New("fact final witness admission exact body is invalid")
	}
	purpose := FactFinalWitnessAdmissionPurposeV1
	if schemaVersion == FactFinalWitnessAdmissionSchemaVersionV2 {
		purpose = FactFinalWitnessAdmissionPurposeV2
	}
	admission := FactFinalWitnessAdmissionV1{
		SchemaVersion: schemaVersion, Purpose: purpose,
		ContextDigest: input.Context.ContextDigest, DatasetSnapshotID: input.Context.DatasetSnapshotID,
		SourceManifestHash: input.Context.SourceManifestHash, EnvelopeDigest: input.Envelope.EnvelopeDigest,
		RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(input.RenderedText)), PublicationSnapshotProofDigest: input.PublicationProof.ProofDigest,
		RegistrySequence: input.Registry.Sequence, RegistryStateDigest: input.Registry.StateDigest,
		EvidenceReceiptIDsDigest:      factFinalWitnessReceiptIDsDigestV1(input.Envelope.EvidenceReceiptIDs),
		EvidenceReceiptCount:          uint64(len(input.Envelope.EvidenceReceiptIDs)),
		EvidenceAuthorityBundleDigest: input.Bundle.RecordDigest, EvidenceAuthorityBundleGeneration: input.Bundle.Generation,
		EvidenceRegistryIndexDigest: input.Bundle.EvidenceRegistryIndexDigest, EvidenceRegistryCount: input.Bundle.EvidenceRegistryCount,
		SelectedRegistryIndexDigest: input.SelectedIndex.IndexDigest, SelectedRegistryIndexGeneration: input.SelectedIndex.Generation,
		SelectedRegistryCapsuleDigest: input.SelectedCapsule.RecordDigest,
		WitnessBinding:                binding, AdmittedAt: admittedAt.Format(time.RFC3339Nano),
	}
	if schemaVersion == FactFinalWitnessAdmissionSchemaVersionV2 {
		producerSHA256, err := domainsecurity.FundsProducerContentManifestV1SHA256(input.FundsProducerContent)
		if err != nil {
			return FactFinalWitnessAdmissionV1{}, errors.New("fact final witness producer content is invalid")
		}
		admission.DatasetSnapshotIndexDigest = input.Bundle.DatasetSnapshotIndexDigest
		admission.DatasetSnapshotCount = input.Bundle.DatasetSnapshotCount
		admission.SelectedDatasetSnapshotIndexDigest = input.SelectedDatasetIndex.IndexDigest
		admission.SelectedDatasetSnapshotIndexGeneration = input.SelectedDatasetIndex.Generation
		admission.SelectedDatasetSnapshotRecordDigest = input.DatasetRecord.RecordDigest
		admission.DatasetSnapshotManifestDigest = input.DatasetManifest.ManifestDigest
		admission.FundsProducerContentID = domainsecurity.DeriveFundsProducerContentIDV1(input.FundsProducerContent)
		admission.FundsProducerContentManifestSHA256 = producerSHA256
	}
	admission.AdmissionDigest = factFinalWitnessAdmissionDigestV1(admission)
	return admission, ValidateFactFinalWitnessAdmissionV1(admission)
}

func ValidateFactFinalWitnessAdmissionForRecordV1(admission *FactFinalWitnessAdmissionV1, record AcceptedFinalRecord) error {
	if admission == nil || ValidateFactFinalWitnessAdmissionV1(*admission) != nil ||
		admission.ContextDigest != record.ContextDigest || admission.DatasetSnapshotID != record.DatasetSnapshotID ||
		admission.EnvelopeDigest != record.EnvelopeDigest || admission.RenderedTextSHA256 != record.RenderedTextSHA256 ||
		admission.PublicationSnapshotProofDigest != record.PublicationSnapshotProofDigest ||
		admission.RegistrySequence != record.RegistrySequence || admission.RegistryStateDigest != record.RegistryStateDigest ||
		admission.WitnessBinding.AuthorityKeyID != record.AuthorityKeyID {
		return errors.New("fact final witness admission does not match accepted final")
	}
	return nil
}

func validateFactFinalRegistrySelectionV1(input FactFinalWitnessAdmissionInputV1) error {
	path := input.RegistryIndexPath
	if ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(
		input.RootIndex, input.Bundle.EvidenceRegistryIndexDigest, input.Bundle.EvidenceRegistryCount,
	) != nil || len(path) == 0 || !reflect.DeepEqual(path[0], input.RootIndex) ||
		!reflect.DeepEqual(path[len(path)-1], input.SelectedIndex) ||
		!EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(input.SelectedIndex, input.SelectedCapsule) ||
		!reflect.DeepEqual(input.SelectedCapsule.SecurityContext, input.Context) ||
		!reflect.DeepEqual(input.SelectedCapsule.Registry, input.Registry) {
		return errors.New("fact final witness registry selection is invalid")
	}
	for index := range path {
		if ValidateEvidenceRegistryAuthorityIndexForInstallationV2(
			path[index], input.InstallationID, input.EnrollmentID, input.AuthorityKeyID, input.AuthorityPublicKey,
		) != nil {
			return errors.New("fact final witness registry selection authority is invalid")
		}
		if index > 0 && ValidateEvidenceRegistryAuthorityIndexTransitionV2(path[index], path[index-1]) != nil {
			return errors.New("fact final witness registry selection path is invalid")
		}
	}
	return nil
}

func validFactFinalWitnessAdmissionVersionV1(admission FactFinalWitnessAdmissionV1) bool {
	switch admission.SchemaVersion {
	case FactFinalWitnessAdmissionSchemaVersionV1:
		return admission.Purpose == FactFinalWitnessAdmissionPurposeV1 &&
			admission.DatasetSnapshotIndexDigest == "" && admission.DatasetSnapshotCount == 0 &&
			admission.SelectedDatasetSnapshotIndexDigest == "" && admission.SelectedDatasetSnapshotIndexGeneration == 0 &&
			admission.SelectedDatasetSnapshotRecordDigest == "" && admission.DatasetSnapshotManifestDigest == "" &&
			admission.FundsProducerContentID == "" && admission.FundsProducerContentManifestSHA256 == ""
	case FactFinalWitnessAdmissionSchemaVersionV2:
		return admission.Purpose == FactFinalWitnessAdmissionPurposeV2 &&
			domainsecurity.IsDatasetSnapshotIDV2Syntax(admission.DatasetSnapshotID) &&
			validSHA256(admission.DatasetSnapshotIndexDigest) && admission.DatasetSnapshotCount > 0 &&
			validSHA256(admission.SelectedDatasetSnapshotIndexDigest) && admission.SelectedDatasetSnapshotIndexGeneration > 0 &&
			admission.SelectedDatasetSnapshotIndexGeneration <= admission.DatasetSnapshotCount &&
			validSHA256(admission.SelectedDatasetSnapshotRecordDigest) &&
			validSHA256(admission.DatasetSnapshotManifestDigest) &&
			domainsecurity.IsFundsProducerContentIDV1Syntax(admission.FundsProducerContentID) &&
			validSHA256(admission.FundsProducerContentManifestSHA256)
	default:
		return false
	}
}

func factFinalWitnessDatasetInputIsEmptyV1(input FactFinalWitnessAdmissionInputV1) bool {
	return reflect.DeepEqual(input.DatasetRootIndex, domainsecurity.DatasetSnapshotIndexV1{}) &&
		reflect.DeepEqual(input.SelectedDatasetIndex, domainsecurity.DatasetSnapshotIndexV1{}) &&
		len(input.DatasetIndexPath) == 0 &&
		reflect.DeepEqual(input.DatasetRecord, domainsecurity.DatasetSnapshotAuthorityRecordV2{}) &&
		reflect.DeepEqual(input.DatasetManifest, domainsecurity.DatasetSnapshotManifestV2{}) &&
		reflect.DeepEqual(input.FundsProducerContent, domainsecurity.FundsProducerContentManifestV1{}) &&
		reflect.DeepEqual(input.BindingObservation, domainsecurity.CaseBindingObservationV1{})
}

func validateFactFinalDatasetSelectionV2(input FactFinalWitnessAdmissionInputV1) error {
	path := input.DatasetIndexPath
	if len(path) == 0 || uint64(len(path)) != input.Bundle.DatasetSnapshotCount ||
		!reflect.DeepEqual(path[0], input.DatasetRootIndex) ||
		domainsecurity.ValidateDatasetSnapshotIndexWitnessRootV1(
			input.DatasetRootIndex,
			input.Bundle.DatasetSnapshotIndexDigest,
			input.Bundle.DatasetSnapshotCount,
		) != nil ||
		input.SelectedDatasetIndex.Generation == 0 ||
		input.SelectedDatasetIndex.Generation > input.Bundle.DatasetSnapshotCount {
		return errors.New("fact final witness dataset root is invalid")
	}
	selectedOffset := input.Bundle.DatasetSnapshotCount - input.SelectedDatasetIndex.Generation
	if selectedOffset >= uint64(len(path)) ||
		!reflect.DeepEqual(path[selectedOffset], input.SelectedDatasetIndex) {
		return errors.New("fact final witness dataset selection is outside the witnessed path")
	}
	for index := range path {
		if domainsecurity.ValidateDatasetSnapshotIndexForInstallationV1(
			path[index],
			input.InstallationID,
			input.EnrollmentID,
			input.AuthorityKeyID,
			input.AuthorityPublicKey,
		) != nil {
			return errors.New("fact final witness dataset path authority is invalid")
		}
		if index > 0 &&
			domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(path[index], path[index-1]) != nil {
			return errors.New("fact final witness dataset path is invalid")
		}
	}
	genesis := path[len(path)-1]
	if genesis.Generation != 1 ||
		genesis.PreviousIndexDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		return errors.New("fact final witness dataset path is incomplete")
	}
	context := input.Context
	observation := input.BindingObservation
	if domainsecurity.ValidateDatasetSnapshotIndexNodeForManifestV2(
		input.SelectedDatasetIndex,
		input.DatasetRecord,
		input.DatasetManifest,
		input.FundsProducerContent,
		context.TenantID,
		context.UserID,
		observation,
		input.InstallationID,
		input.EnrollmentID,
		input.AuthorityKeyID,
		input.AuthorityPublicKey,
	) != nil {
		return errors.New("fact final witness selected dataset graph is invalid")
	}
	if observation.WorkspaceRealPath != context.WorkspaceRealPath ||
		observation.CaseID != context.CaseID ||
		observation.CaseBindingHash != context.CaseBindingHash ||
		observation.ObservationDigest != context.PublicationPolicy.BindingObservationDigest ||
		input.DatasetRecord.DatasetSnapshotID != context.DatasetSnapshotID ||
		domainsecurity.DeriveDatasetSnapshotIDV2(input.DatasetManifest) != context.DatasetSnapshotID ||
		input.DatasetRecord.SourceManifestHash != context.SourceManifestHash ||
		input.DatasetManifest.SourceManifestHash != context.SourceManifestHash ||
		input.FundsProducerContent.CaseID != context.CaseID {
		return errors.New("fact final witness dataset context binding is invalid")
	}
	return nil
}

func factFinalWitnessReceiptIDsDigestV1(receiptIDs []string) string {
	canonical := canonicalEvidenceStrings(receiptIDs)
	body, _ := json.Marshal(canonical)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), factFinalWitnessReceiptSetDomainV1...), body...))
}

func factFinalWitnessAdmissionDigestV1(admission FactFinalWitnessAdmissionV1) string {
	admission.AdmissionDigest = ""
	body, _ := json.Marshal(admission)
	return domainsecurity.SHA256Hex(body)
}

func cloneFactFinalWitnessAdmissionV1(admission *FactFinalWitnessAdmissionV1) *FactFinalWitnessAdmissionV1 {
	if admission == nil {
		return nil
	}
	clone := *admission
	return &clone
}
