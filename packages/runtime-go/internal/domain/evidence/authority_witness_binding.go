package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	EvidenceAuthorityWitnessBindingSchemaVersion = 1
	EvidenceAuthorityWitnessBindingPurpose       = "analytix.evidence-authority-witness-binding/v1"
)

// EvidenceAuthorityWitnessBindingV1 is a compact reference to the exact
// current-run challenge and witness observation that selected one shared
// evidence authority bundle. Non-empty digests alone carry no authority;
// every consumer must validate the binding against the full signed contracts
// and its installation trust anchors.
type EvidenceAuthorityWitnessBindingV1 struct {
	SchemaVersion        int    `json:"schemaVersion"`
	Purpose              string `json:"purpose"`
	InstallationID       string `json:"installationId"`
	EnrollmentID         string `json:"enrollmentId"`
	Namespace            string `json:"namespace"`
	AuthorityKeyID       string `json:"authorityKeyId"`
	WitnessKeyID         string `json:"witnessKeyId"`
	BundleRecordDigest   string `json:"bundleRecordDigest"`
	BundleGeneration     uint64 `json:"bundleGeneration"`
	ObserveRequestDigest string `json:"observeRequestDigest"`
	CheckpointDigest     string `json:"checkpointDigest"`
	ObservationDigest    string `json:"observationDigest"`
	BindingDigest        string `json:"bindingDigest"`
}

func NewEvidenceAuthorityWitnessBindingV1(
	bundle EvidenceAuthorityBundleV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
	witnessKeyID string,
	witnessPublicKey []byte,
) (EvidenceAuthorityWitnessBindingV1, error) {
	binding := EvidenceAuthorityWitnessBindingV1{
		SchemaVersion:        EvidenceAuthorityWitnessBindingSchemaVersion,
		Purpose:              EvidenceAuthorityWitnessBindingPurpose,
		InstallationID:       strings.TrimSpace(installationID),
		EnrollmentID:         strings.TrimSpace(enrollmentID),
		Namespace:            EvidenceAuthorityBundleWitnessNamespaceV1,
		AuthorityKeyID:       strings.TrimSpace(authorityKeyID),
		WitnessKeyID:         strings.TrimSpace(witnessKeyID),
		BundleRecordDigest:   bundle.RecordDigest,
		BundleGeneration:     bundle.Generation,
		ObserveRequestDigest: request.RequestDigest,
		CheckpointDigest:     observation.Checkpoint.CheckpointDigest,
		ObservationDigest:    observation.ObservationDigest,
	}
	if err := validateEvidenceAuthorityWitnessContractsV1(
		bundle, request, observation,
		binding.InstallationID, binding.EnrollmentID, binding.AuthorityKeyID, authorityPublicKey,
		binding.WitnessKeyID, witnessPublicKey,
	); err != nil {
		return EvidenceAuthorityWitnessBindingV1{}, err
	}
	binding.BindingDigest = evidenceAuthorityWitnessBindingDigestV1(binding)
	if err := ValidateEvidenceAuthorityWitnessBindingExactV1(
		binding, bundle, request, observation,
		binding.InstallationID, binding.EnrollmentID, binding.AuthorityKeyID, authorityPublicKey,
		binding.WitnessKeyID, witnessPublicKey,
	); err != nil {
		return EvidenceAuthorityWitnessBindingV1{}, err
	}
	return binding, nil
}

func ValidateEvidenceAuthorityWitnessBindingV1(binding EvidenceAuthorityWitnessBindingV1) error {
	if binding.SchemaVersion != EvidenceAuthorityWitnessBindingSchemaVersion || binding.Purpose != EvidenceAuthorityWitnessBindingPurpose ||
		!domainsecurity.IsSHA256Hex(binding.InstallationID) || !domainsecurity.IsSHA256Hex(binding.EnrollmentID) ||
		binding.Namespace != EvidenceAuthorityBundleWitnessNamespaceV1 || !domainsecurity.IsSHA256Hex(binding.AuthorityKeyID) ||
		!domainsecurity.IsSHA256Hex(binding.WitnessKeyID) || !domainsecurity.IsSHA256Hex(binding.BundleRecordDigest) ||
		binding.BundleGeneration == 0 || !domainsecurity.IsSHA256Hex(binding.ObserveRequestDigest) ||
		!domainsecurity.IsSHA256Hex(binding.CheckpointDigest) || !domainsecurity.IsSHA256Hex(binding.ObservationDigest) ||
		!domainsecurity.IsSHA256Hex(binding.BindingDigest) || binding.BindingDigest != evidenceAuthorityWitnessBindingDigestV1(binding) {
		return errors.New("evidence authority witness binding is invalid")
	}
	return nil
}

func ValidateEvidenceAuthorityWitnessBindingExactV1(
	binding EvidenceAuthorityWitnessBindingV1,
	bundle EvidenceAuthorityBundleV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
	witnessKeyID string,
	witnessPublicKey []byte,
) error {
	if err := ValidateEvidenceAuthorityWitnessBindingV1(binding); err != nil {
		return err
	}
	if err := validateEvidenceAuthorityWitnessContractsV1(
		bundle, request, observation,
		installationID, enrollmentID, authorityKeyID, authorityPublicKey,
		witnessKeyID, witnessPublicKey,
	); err != nil {
		return err
	}
	if binding.InstallationID != strings.TrimSpace(installationID) || binding.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		binding.AuthorityKeyID != strings.TrimSpace(authorityKeyID) || binding.WitnessKeyID != strings.TrimSpace(witnessKeyID) ||
		binding.BundleRecordDigest != bundle.RecordDigest || binding.BundleGeneration != bundle.Generation ||
		binding.ObserveRequestDigest != request.RequestDigest || binding.CheckpointDigest != observation.Checkpoint.CheckpointDigest ||
		binding.ObservationDigest != observation.ObservationDigest {
		return errors.New("evidence authority witness binding does not match exact contracts")
	}
	return nil
}

func validateEvidenceAuthorityWitnessContractsV1(
	bundle EvidenceAuthorityBundleV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey []byte,
	witnessKeyID string,
	witnessPublicKey []byte,
) error {
	if err := ValidateEvidenceAuthorityBundleForInstallationV1(
		bundle, installationID, enrollmentID, authorityKeyID, authorityPublicKey,
	); err != nil {
		return errors.Join(errors.New("evidence authority bundle trust anchor is invalid"), err)
	}
	if request.Namespace != EvidenceAuthorityBundleWitnessNamespaceV1 ||
		observation.Checkpoint.Namespace != EvidenceAuthorityBundleWitnessNamespaceV1 {
		return errors.New("evidence authority witness namespace is invalid")
	}
	if err := domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
		observation, request, installationID, authorityKeyID, authorityPublicKey,
		enrollmentID, witnessKeyID, witnessPublicKey,
	); err != nil {
		return errors.Join(errors.New("evidence authority fresh observation is invalid"), err)
	}
	if err := ValidateEvidenceAuthorityBundleCheckpointV1(bundle, observation.Checkpoint); err != nil {
		return errors.Join(errors.New("evidence authority checkpoint does not select bundle"), err)
	}
	return nil
}

func ParseEvidenceAuthorityWitnessBindingV1(body []byte) (EvidenceAuthorityWitnessBindingV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 32 * 1024, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 4096,
	}); err != nil {
		return EvidenceAuthorityWitnessBindingV1{}, err
	}
	var binding EvidenceAuthorityWitnessBindingV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&binding); err != nil {
		return EvidenceAuthorityWitnessBindingV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return EvidenceAuthorityWitnessBindingV1{}, errors.New("evidence authority witness binding contains trailing JSON")
	}
	canonical, err := json.Marshal(binding)
	if err != nil || !bytes.Equal(canonical, body) {
		return EvidenceAuthorityWitnessBindingV1{}, errors.New("evidence authority witness binding is not canonical")
	}
	return binding, ValidateEvidenceAuthorityWitnessBindingV1(binding)
}

func EvidenceAuthorityWitnessBindingV1Bytes(binding EvidenceAuthorityWitnessBindingV1) ([]byte, error) {
	if err := ValidateEvidenceAuthorityWitnessBindingV1(binding); err != nil {
		return nil, err
	}
	return json.Marshal(binding)
}

func evidenceAuthorityWitnessBindingDigestV1(binding EvidenceAuthorityWitnessBindingV1) string {
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	return domainsecurity.SHA256Hex(body)
}
