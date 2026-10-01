package evidence

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	FactFinalHostLocalAdmissionVersionV1  = 1
	FactFinalHostLocalAdmissionPurposeV1  = "analytix.fact-final-host-local-admission/v1"
	HostLocalAcceptedFinalRecordVersionV6 = 6
)

// This signed-record value binds host-local authority, never an independent
// witness. Live use must additionally challenge the selected head and complete
// retained material chain; it does not prove whole-profile rollback resistance.
type FactFinalHostLocalAdmissionV1 struct {
	SchemaVersion                          int               `json:"schemaVersion"`
	Purpose                                string            `json:"purpose"`
	ContextDigest                          string            `json:"contextDigest"`
	DatasetSnapshotID                      string            `json:"datasetSnapshotId"`
	SourceManifestHash                     string            `json:"sourceManifestHash"`
	EnvelopeDigest                         string            `json:"envelopeDigest"`
	RenderedTextSHA256                     string            `json:"renderedTextSha256"`
	PublicationSnapshotProofDigest         string            `json:"publicationSnapshotProofDigest"`
	RegistrySequence                       uint64            `json:"registrySequence"`
	RegistryStateDigest                    string            `json:"registryStateDigest"`
	EvidenceReceiptIDsDigest               string            `json:"evidenceReceiptIdsDigest"`
	EvidenceReceiptCount                   uint64            `json:"evidenceReceiptCount"`
	HostLocalHead                          domainhost.HeadV1 `json:"hostLocalHead"`
	ModeCommitmentDigest                   string            `json:"modeCommitmentDigest"`
	SelectedRegistryIndexDigest            string            `json:"selectedRegistryIndexDigest"`
	SelectedRegistryIndexGeneration        uint64            `json:"selectedRegistryIndexGeneration"`
	SelectedRegistryCapsuleDigest          string            `json:"selectedRegistryCapsuleDigest"`
	SelectedDatasetSnapshotIndexDigest     string            `json:"selectedDatasetSnapshotIndexDigest"`
	SelectedDatasetSnapshotIndexGeneration uint64            `json:"selectedDatasetSnapshotIndexGeneration"`
	SelectedDatasetSnapshotRecordDigest    string            `json:"selectedDatasetSnapshotRecordDigest"`
	DatasetSnapshotManifestDigest          string            `json:"datasetSnapshotManifestDigest"`
	FundsProducerContentID                 string            `json:"fundsProducerContentId"`
	FundsProducerContentManifestSHA256     string            `json:"fundsProducerContentManifestSha256"`
	AdmittedAt                             string            `json:"admittedAt"`
	AdmissionDigest                        string            `json:"admissionDigest"`
}

type FactFinalHostLocalAdmissionInputV1 struct {
	Context              domainsecurity.TurnSecurityContext
	Envelope             FinalAnswerEnvelope
	RenderedText         string
	PublicationProof     *PublicationSnapshotProof
	Registry             EvidenceReceiptRegistry
	Head                 domainhost.HeadV1
	ModeCommitment       domainhost.HeadV1
	RegistryIndexPath    []EvidenceRegistryAuthorityIndexV2
	SelectedCapsule      EvidenceRegistryAuthorityCapsule
	DatasetIndexPath     []domainsecurity.DatasetSnapshotIndexV1
	SelectedDatasetIndex domainsecurity.DatasetSnapshotIndexV1
	DatasetRecord        domainsecurity.DatasetSnapshotAuthorityRecordV2
	DatasetManifest      domainsecurity.DatasetSnapshotManifestV2
	FundsProducerContent domainsecurity.FundsProducerContentManifestV1
	BindingObservation   domainsecurity.CaseBindingObservationV1
	InstallationID       string
	RootBindingDigest    string
	AuthorityKeyID       string
	AuthorityPublicKey   []byte
	AdmittedAt           time.Time
}

func NewFactFinalHostLocalAdmissionV1(input FactFinalHostLocalAdmissionInputV1) (FactFinalHostLocalAdmissionV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		ValidateFinalAnswerEnvelope(input.Envelope) != nil || !FinalAnswerRequiresPublicationSnapshotProof(input.Envelope) ||
		!EvidenceReceiptRegistryMatchesContext(input.Registry, input.Context) ||
		ValidatePublicationSnapshotProofAgainstRegistry(input.PublicationProof, input.Context, input.Envelope, input.Registry) != nil ||
		domainhost.ValidateHeadForInstallationV1(input.Head, input.InstallationID, input.RootBindingDigest, input.AuthorityKeyID, input.AuthorityPublicKey) != nil ||
		domainhost.ValidateHeadForInstallationV1(input.ModeCommitment, input.InstallationID, input.RootBindingDigest, input.AuthorityKeyID, input.AuthorityPublicKey) != nil ||
		input.ModeCommitment.Generation != 0 || validateFactFinalHostLocalRegistryV1(input) != nil ||
		validateFactFinalHostLocalDatasetV1(input) != nil {
		return FactFinalHostLocalAdmissionV1{}, errors.New("fact final host-local admission exact authority is invalid")
	}
	rendered, renderErr := RenderFinalAnswer(input.Envelope)
	checkedAt, checkedErr := time.Parse(time.RFC3339Nano, input.PublicationProof.CheckedAt)
	admittedAt := input.AdmittedAt.UTC()
	if renderErr != nil || rendered != input.RenderedText || checkedErr != nil || admittedAt.IsZero() || admittedAt.Before(checkedAt) {
		return FactFinalHostLocalAdmissionV1{}, errors.New("fact final host-local admission body is invalid")
	}
	producerSHA, err := domainsecurity.FundsProducerContentManifestV1SHA256(input.FundsProducerContent)
	if err != nil {
		return FactFinalHostLocalAdmissionV1{}, err
	}
	selectedRegistry := input.RegistryIndexPath[len(input.RegistryIndexPath)-1]
	admission := FactFinalHostLocalAdmissionV1{
		SchemaVersion: FactFinalHostLocalAdmissionVersionV1, Purpose: FactFinalHostLocalAdmissionPurposeV1,
		ContextDigest: input.Context.ContextDigest, DatasetSnapshotID: input.Context.DatasetSnapshotID,
		SourceManifestHash: input.Context.SourceManifestHash, EnvelopeDigest: input.Envelope.EnvelopeDigest,
		RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(input.RenderedText)), PublicationSnapshotProofDigest: input.PublicationProof.ProofDigest,
		RegistrySequence: input.Registry.Sequence, RegistryStateDigest: input.Registry.StateDigest,
		EvidenceReceiptIDsDigest: factFinalWitnessReceiptIDsDigestV1(input.Envelope.EvidenceReceiptIDs), EvidenceReceiptCount: uint64(len(input.Envelope.EvidenceReceiptIDs)),
		HostLocalHead: input.Head, ModeCommitmentDigest: input.ModeCommitment.RecordDigest,
		SelectedRegistryIndexDigest: selectedRegistry.IndexDigest, SelectedRegistryIndexGeneration: selectedRegistry.Generation,
		SelectedRegistryCapsuleDigest:          input.SelectedCapsule.RecordDigest,
		SelectedDatasetSnapshotIndexDigest:     input.SelectedDatasetIndex.IndexDigest,
		SelectedDatasetSnapshotIndexGeneration: input.SelectedDatasetIndex.Generation,
		SelectedDatasetSnapshotRecordDigest:    input.DatasetRecord.RecordDigest, DatasetSnapshotManifestDigest: input.DatasetManifest.ManifestDigest,
		FundsProducerContentID: domainsecurity.DeriveFundsProducerContentIDV1(input.FundsProducerContent), FundsProducerContentManifestSHA256: producerSHA,
		AdmittedAt: admittedAt.Format(time.RFC3339Nano),
	}
	admission.AdmissionDigest = factFinalHostLocalAdmissionDigestV1(admission)
	return admission, ValidateFactFinalHostLocalAdmissionV1(admission)
}

func validateFactFinalHostLocalRegistryV1(input FactFinalHostLocalAdmissionInputV1) error {
	path := input.RegistryIndexPath
	if len(path) == 0 || ValidateEvidenceRegistryAuthorityIndexHostLocalRootV3(path[0], input.Head.EvidenceRegistryIndexDigest, input.Head.EvidenceRegistryCount) != nil ||
		!EvidenceRegistryAuthorityIndexEntryMatchesCapsuleHostLocalV3(path[len(path)-1], input.SelectedCapsule) ||
		!reflect.DeepEqual(input.SelectedCapsule.SecurityContext, input.Context) || !reflect.DeepEqual(input.SelectedCapsule.Registry, input.Registry) {
		return errors.New("fact final host-local registry selection is invalid")
	}
	for offset, index := range path {
		if ValidateEvidenceRegistryAuthorityIndexForHostLocalV3(index, input.InstallationID, input.ModeCommitment.RecordDigest, input.AuthorityKeyID, input.AuthorityPublicKey) != nil ||
			(offset > 0 && ValidateEvidenceRegistryAuthorityIndexHostLocalTransitionV3(index, path[offset-1]) != nil) {
			return errors.New("fact final host-local registry path is invalid")
		}
	}
	return nil
}

func validateFactFinalHostLocalDatasetV1(input FactFinalHostLocalAdmissionInputV1) error {
	path := input.DatasetIndexPath
	if len(path) == 0 || uint64(len(path)) != input.Head.DatasetSnapshotCount ||
		domainsecurity.ValidateDatasetSnapshotIndexHostLocalRootV2(path[0], input.Head.DatasetSnapshotIndexDigest, input.Head.DatasetSnapshotCount) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexNodeForHostLocalManifestV2(input.SelectedDatasetIndex, input.DatasetRecord, input.DatasetManifest, input.FundsProducerContent,
			input.Context.TenantID, input.Context.UserID, input.BindingObservation, input.InstallationID, input.ModeCommitment.RecordDigest, input.AuthorityKeyID, input.AuthorityPublicKey) != nil {
		return errors.New("fact final host-local dataset graph is invalid")
	}
	selected := 0
	for offset, index := range path {
		if domainsecurity.ValidateDatasetSnapshotIndexForHostLocalV2(index, input.InstallationID, input.ModeCommitment.RecordDigest, input.AuthorityKeyID, input.AuthorityPublicKey) != nil ||
			(offset > 0 && domainsecurity.ValidateDatasetSnapshotIndexHostLocalTransitionV2(index, path[offset-1]) != nil) {
			return errors.New("fact final host-local dataset path is invalid")
		}
		if index == input.SelectedDatasetIndex {
			selected++
		}
	}
	binding := input.BindingObservation
	if selected != 1 || path[len(path)-1].Generation != 1 || path[len(path)-1].PreviousIndexDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() ||
		binding.WorkspaceRealPath != input.Context.WorkspaceRealPath || binding.CaseID != input.Context.CaseID || binding.CaseBindingHash != input.Context.CaseBindingHash ||
		binding.ObservationDigest != input.Context.PublicationPolicy.BindingObservationDigest || input.DatasetRecord.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
		domainsecurity.DeriveDatasetSnapshotIDV2(input.DatasetManifest) != input.Context.DatasetSnapshotID || input.DatasetRecord.SourceManifestHash != input.Context.SourceManifestHash ||
		input.DatasetManifest.SourceManifestHash != input.Context.SourceManifestHash || input.FundsProducerContent.CaseID != input.Context.CaseID {
		return errors.New("fact final host-local dataset context is invalid")
	}
	return nil
}

func ValidateFactFinalHostLocalAdmissionV1(admission FactFinalHostLocalAdmissionV1) error {
	if admission.SchemaVersion != FactFinalHostLocalAdmissionVersionV1 || admission.Purpose != FactFinalHostLocalAdmissionPurposeV1 ||
		domainhost.ValidateHeadV1(admission.HostLocalHead) != nil || !domainsecurity.IsDatasetSnapshotIDV2Syntax(admission.DatasetSnapshotID) ||
		admission.RegistrySequence == 0 || admission.EvidenceReceiptCount == 0 || admission.SelectedRegistryIndexGeneration == 0 ||
		admission.SelectedRegistryIndexGeneration > admission.HostLocalHead.EvidenceRegistryCount || admission.SelectedDatasetSnapshotIndexGeneration == 0 ||
		admission.SelectedDatasetSnapshotIndexGeneration > admission.HostLocalHead.DatasetSnapshotCount ||
		admission.DatasetSnapshotID != "dsv2_"+admission.DatasetSnapshotManifestDigest ||
		!domainsecurity.IsFundsProducerContentIDV1Syntax(admission.FundsProducerContentID) {
		return errors.New("fact final host-local admission is incomplete")
	}
	for _, digest := range []string{admission.ContextDigest, admission.SourceManifestHash, admission.EnvelopeDigest, admission.RenderedTextSHA256,
		admission.PublicationSnapshotProofDigest, admission.RegistryStateDigest, admission.EvidenceReceiptIDsDigest, admission.ModeCommitmentDigest,
		admission.SelectedRegistryIndexDigest, admission.SelectedRegistryCapsuleDigest, admission.SelectedDatasetSnapshotIndexDigest,
		admission.SelectedDatasetSnapshotRecordDigest, admission.DatasetSnapshotManifestDigest, admission.FundsProducerContentManifestSHA256, admission.AdmissionDigest} {
		if !validSHA256(digest) {
			return errors.New("fact final host-local admission digest is invalid")
		}
	}
	at, err := time.Parse(time.RFC3339Nano, admission.AdmittedAt)
	if err != nil || at.Location() != time.UTC || at.UTC().Format(time.RFC3339Nano) != admission.AdmittedAt || admission.AdmissionDigest != factFinalHostLocalAdmissionDigestV1(admission) {
		return errors.New("fact final host-local admission integrity is invalid")
	}
	return nil
}

func ValidateFactFinalHostLocalAdmissionExactV1(admission FactFinalHostLocalAdmissionV1, input FactFinalHostLocalAdmissionInputV1) error {
	expected, err := NewFactFinalHostLocalAdmissionV1(input)
	if err != nil || !reflect.DeepEqual(admission, expected) {
		return errors.Join(errors.New("fact final host-local admission is not exact"), err)
	}
	return nil
}

func ValidateFactFinalHostLocalAdmissionForRecordV1(admission *FactFinalHostLocalAdmissionV1, record AcceptedFinalRecord) error {
	if admission == nil || record.PublicView == nil {
		return errors.New("fact final host-local admission is absent")
	}
	acceptedAt, acceptedErr := time.Parse(time.RFC3339Nano, record.AcceptedAt)
	admittedAt, admittedErr := time.Parse(time.RFC3339Nano, admission.AdmittedAt)
	if ValidateFactFinalHostLocalAdmissionV1(*admission) != nil || admission.ContextDigest != record.ContextDigest ||
		admission.DatasetSnapshotID != record.DatasetSnapshotID || admission.EnvelopeDigest != record.EnvelopeDigest || admission.RenderedTextSHA256 != record.RenderedTextSHA256 ||
		admission.PublicationSnapshotProofDigest != record.PublicationSnapshotProofDigest || admission.RegistrySequence != record.RegistrySequence ||
		admission.RegistryStateDigest != record.RegistryStateDigest || admission.HostLocalHead.AuthorityKeyID != record.AuthorityKeyID ||
		admission.HostLocalHead.AuthorityPublicKey != record.AuthorityPublicKey ||
		admission.EvidenceReceiptCount != uint64(record.PublicView.ReceiptMetadata.Count) ||
		acceptedErr != nil || admittedErr != nil || acceptedAt.Before(admittedAt) {
		return errors.New("fact final host-local admission does not match final authority")
	}
	return nil
}

func factFinalHostLocalAdmissionDigestV1(admission FactFinalHostLocalAdmissionV1) string {
	admission.AdmissionDigest = ""
	body, _ := json.Marshal(admission)
	return domainsecurity.SHA256Hex(body)
}

func cloneFactFinalHostLocalAdmissionV1(admission *FactFinalHostLocalAdmissionV1) *FactFinalHostLocalAdmissionV1 {
	if admission == nil {
		return nil
	}
	copy := *admission
	return &copy
}
