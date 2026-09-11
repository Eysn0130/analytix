package reportpublication

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PublicationCommitSelectionSchemaVersion = 1
	PublicationCommitSelectionPurpose       = "analytix.publication-commit-selection/v1"
)

var (
	publicationCommitSelectionIDDomainV1        = []byte("analytix.publication-commit-selection/id/v1\x00")
	publicationCommitSelectionSignatureDomainV1 = []byte("analytix.publication-commit-selection/signature/v1\x00")
	publicationCommitSelectionRecordDomainV1    = []byte("analytix.publication-commit-selection/record/v1\x00")
)

// PublicationCommitSelectionV1 is the stable, no-replace choice of one exact
// unsigned commit for a report attempt. Fresh witness challenges may produce
// different otherwise-valid commits, so the selection is keyed only by the
// frozen attempt identity and must be durable before commit signing begins.
type PublicationCommitSelectionV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	SelectionID   string `json:"selectionId"`

	InstallationID string `json:"installationId"`
	EnrollmentID   string `json:"enrollmentId"`
	AttemptID      string `json:"attemptId"`

	CandidateRecordDigest            string                     `json:"candidateRecordDigest"`
	PublicationIndexDigest           string                     `json:"publicationIndexDigest"`
	AuthorityAdvanceMutationID       string                     `json:"authorityAdvanceMutationId"`
	AuthorityAdvanceIntentDigest     string                     `json:"authorityAdvanceIntentDigest"`
	AuthorityAdvanceSettlementDigest string                     `json:"authorityAdvanceSettlementDigest"`
	PreviousEvidenceBundleDigest     string                     `json:"previousEvidenceBundleDigest"`
	CommittedEvidenceBundleDigest    string                     `json:"committedEvidenceBundleDigest"`
	ObserveRequestDigest             string                     `json:"observeRequestDigest"`
	ObservationDigest                string                     `json:"observationDigest"`
	WitnessBindingDigest             string                     `json:"witnessBindingDigest"`
	CommitReceiptID                  string                     `json:"commitReceiptId"`
	CommitSigningDigest              string                     `json:"commitSigningDigest"`
	UnsignedCommit                   PublicationCommitReceiptV1 `json:"unsignedCommit"`

	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RecordDigest       string `json:"recordDigest"`
}

type PublicationCommitSelectionInputV1 struct {
	Attempt          PublicationAttemptV1
	SettlementDigest string
	CommitInput      PublicationCommitReceiptInputV1
}

func PublicationCommitSelectionIDV1(installationID, enrollmentID, attemptID string) string {
	body := append([]byte(nil), publicationCommitSelectionIDDomainV1...)
	body = append(body, strings.TrimSpace(installationID)...)
	body = append(body, 0)
	body = append(body, strings.TrimSpace(enrollmentID)...)
	body = append(body, 0)
	body = append(body, strings.TrimSpace(attemptID)...)
	return domainsecurity.SHA256Hex(body)
}

func NewPublicationCommitSelectionV1(
	input PublicationCommitSelectionInputV1,
	sign PublicationReceiptSignFuncV1,
) (PublicationCommitSelectionV1, error) {
	selection, err := newPublicationCommitSelectionV1WithoutSigning(input)
	if err != nil {
		return PublicationCommitSelectionV1{}, err
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(selection.AuthorityPublicKey)
	if err != nil || sign == nil || len(publicKey) != ed25519.PublicKeySize ||
		selection.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return PublicationCommitSelectionV1{}, errors.New("publication commit selection signing authority is invalid")
	}
	signature, err := sign(PublicationCommitSelectionSigningBytesV1(selection))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PublicationCommitSelectionV1{}, errors.New("publication commit selection signing failed")
	}
	selection.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	selection.RecordDigest = publicationCommitSelectionRecordDigestV1(selection)
	if err := ValidatePublicationCommitSelectionExactV1(selection, input); err != nil {
		return PublicationCommitSelectionV1{}, err
	}
	return selection, nil
}

func newPublicationCommitSelectionV1WithoutSigning(
	input PublicationCommitSelectionInputV1,
) (PublicationCommitSelectionV1, error) {
	attempt := input.Attempt
	commitInput := input.CommitInput
	binding, err := domainevidence.NewEvidenceAuthorityWitnessBindingV1(
		commitInput.CommittedBundle, commitInput.ObserveRequest, commitInput.Observation,
		commitInput.InstallationID, commitInput.EnrollmentID, commitInput.AuthorityKeyID, commitInput.AuthorityPublicKey,
		commitInput.WitnessKeyID, commitInput.WitnessPublicKey,
	)
	if err != nil {
		return PublicationCommitSelectionV1{}, err
	}
	unsigned, err := NewPublicationCommitReceiptV1WithoutSigning(commitInput, binding)
	if err != nil || ValidatePublicationCommitReceiptExactV1(unsigned, commitInput) != nil {
		return PublicationCommitSelectionV1{}, errors.New("publication commit selection exact unsigned commit is invalid")
	}
	publicKey := append([]byte(nil), commitInput.AuthorityPublicKey...)
	selection := PublicationCommitSelectionV1{
		SchemaVersion: PublicationCommitSelectionSchemaVersion,
		Purpose:       PublicationCommitSelectionPurpose,
		SelectionID: PublicationCommitSelectionIDV1(
			commitInput.InstallationID, commitInput.EnrollmentID, attempt.AttemptID,
		),
		InstallationID: strings.TrimSpace(commitInput.InstallationID), EnrollmentID: strings.TrimSpace(commitInput.EnrollmentID),
		AttemptID:                        attempt.AttemptID,
		CandidateRecordDigest:            commitInput.Candidate.RecordDigest,
		PublicationIndexDigest:           commitInput.Index.IndexDigest,
		AuthorityAdvanceMutationID:       attempt.AuthorityAdvanceMutationID,
		AuthorityAdvanceIntentDigest:     attempt.AuthorityAdvanceIntentDigest,
		AuthorityAdvanceSettlementDigest: strings.TrimSpace(input.SettlementDigest),
		PreviousEvidenceBundleDigest:     commitInput.PreviousBundle.RecordDigest,
		CommittedEvidenceBundleDigest:    commitInput.CommittedBundle.RecordDigest,
		ObserveRequestDigest:             commitInput.ObserveRequest.RequestDigest,
		ObservationDigest:                commitInput.Observation.ObservationDigest,
		WitnessBindingDigest:             binding.BindingDigest,
		CommitReceiptID:                  unsigned.CommitReceiptID,
		CommitSigningDigest:              domainsecurity.SHA256Hex(PublicationCommitReceiptSigningBytesV1(unsigned)),
		UnsignedCommit:                   unsigned,
		AuthorityAlgorithm:               PublicationReceiptAlgorithm,
		AuthorityKeyID:                   strings.TrimSpace(commitInput.AuthorityKeyID),
		AuthorityPublicKey:               base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if err := validatePublicationCommitSelectionInputV1(selection, input); err != nil {
		return PublicationCommitSelectionV1{}, err
	}
	return selection, nil
}

func ValidatePublicationCommitSelectionV1(selection PublicationCommitSelectionV1) error {
	if err := validatePublicationCommitSelectionUnsignedV1(selection); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(selection.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(selection.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		selection.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PublicationCommitSelectionSigningBytesV1(selection), signature) ||
		!domainsecurity.IsSHA256Hex(selection.RecordDigest) || selection.RecordDigest != publicationCommitSelectionRecordDigestV1(selection) {
		return errors.New("publication commit selection signature is invalid")
	}
	return nil
}

func ValidatePublicationCommitSelectionExactV1(
	selection PublicationCommitSelectionV1,
	input PublicationCommitSelectionInputV1,
) error {
	if selection.AuthoritySignature != "" || selection.RecordDigest != "" {
		if err := ValidatePublicationCommitSelectionV1(selection); err != nil {
			return err
		}
	} else if err := validatePublicationCommitSelectionUnsignedV1(selection); err != nil {
		return err
	}
	expected, err := newPublicationCommitSelectionV1WithoutSigning(input)
	if err != nil {
		return err
	}
	expected.AuthoritySignature = selection.AuthoritySignature
	expected.RecordDigest = selection.RecordDigest
	if !reflectPublicationCommitSelectionV1(expected, selection) {
		return errors.New("publication commit selection does not match exact attempt and witness materials")
	}
	return nil
}

func ValidatePublicationCommitSelectionMaterialsV1(
	selection PublicationCommitSelectionV1,
	attempt PublicationAttemptV1,
	candidate PublicationReceiptV1,
	index PublicationIndexV1,
	settlementDigest string,
) error {
	commit := selection.UnsignedCommit
	if ValidatePublicationCommitSelectionV1(selection) != nil || ValidatePublicationAttemptV1(attempt) != nil ||
		ValidatePublicationIndexReceiptV1(index, candidate) != nil || validatePublicationCommitReceiptUnsignedV1(commit) != nil ||
		selection.SelectionID != PublicationCommitSelectionIDV1(attempt.InstallationID, attempt.EnrollmentID, attempt.AttemptID) ||
		selection.InstallationID != attempt.InstallationID || selection.EnrollmentID != attempt.EnrollmentID || selection.AttemptID != attempt.AttemptID ||
		selection.CandidateRecordDigest != candidate.RecordDigest || selection.PublicationIndexDigest != index.IndexDigest ||
		selection.AuthorityAdvanceMutationID != attempt.AuthorityAdvanceMutationID ||
		selection.AuthorityAdvanceIntentDigest != attempt.AuthorityAdvanceIntentDigest ||
		selection.AuthorityAdvanceSettlementDigest != strings.TrimSpace(settlementDigest) ||
		selection.PreviousEvidenceBundleDigest != attempt.ExpectedEvidenceBundleDigest ||
		selection.CommittedEvidenceBundleDigest != attempt.NextEvidenceBundleDigest ||
		attempt.CandidateRecordDigest != candidate.RecordDigest || attempt.PublicationIndexDigest != index.IndexDigest ||
		commit.CandidateRecordDigest != candidate.RecordDigest || commit.PublicationIndexDigest != index.IndexDigest ||
		commit.PreviousEvidenceBundleDigest != selection.PreviousEvidenceBundleDigest ||
		commit.CommittedEvidenceBundleDigest != selection.CommittedEvidenceBundleDigest ||
		selection.ObserveRequestDigest != commit.WitnessBinding.ObserveRequestDigest ||
		selection.ObservationDigest != commit.WitnessBinding.ObservationDigest ||
		selection.WitnessBindingDigest != commit.WitnessBinding.BindingDigest ||
		selection.CommitReceiptID != commit.CommitReceiptID ||
		selection.CommitSigningDigest != domainsecurity.SHA256Hex(PublicationCommitReceiptSigningBytesV1(commit)) ||
		selection.AuthorityKeyID != attempt.AuthorityKeyID || selection.AuthorityPublicKey != attempt.AuthorityPublicKey ||
		commit.AuthorityKeyID != selection.AuthorityKeyID || commit.AuthorityPublicKey != selection.AuthorityPublicKey {
		return errors.New("publication commit selection material graph is invalid")
	}
	return nil
}

func ValidatePublicationCommitSelectionCommitV1(
	selection PublicationCommitSelectionV1,
	commit PublicationCommitReceiptV1,
) error {
	if ValidatePublicationCommitSelectionV1(selection) != nil || ValidatePublicationCommitReceiptV1(commit) != nil ||
		selection.CommitReceiptID != commit.CommitReceiptID ||
		selection.CommitSigningDigest != domainsecurity.SHA256Hex(PublicationCommitReceiptSigningBytesV1(commit)) {
		return errors.New("publication commit does not match its durable selection")
	}
	unsigned := commit
	unsigned.AuthoritySignature = ""
	unsigned.RecordDigest = ""
	if unsigned != selection.UnsignedCommit {
		return errors.New("publication commit differs from its selected unsigned commit")
	}
	return nil
}

func ParsePublicationCommitSelectionV1(body []byte) (PublicationCommitSelectionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 1 << 20, MaxDepth: 16, MaxTokens: 2048, MaxStringBytes: 128 << 10,
	}); err != nil {
		return PublicationCommitSelectionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var selection PublicationCommitSelectionV1
	if err := decoder.Decode(&selection); err != nil {
		return PublicationCommitSelectionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PublicationCommitSelectionV1{}, errors.New("publication commit selection contains trailing JSON")
	}
	canonical, err := json.Marshal(selection)
	if err != nil || !bytes.Equal(canonical, body) {
		return PublicationCommitSelectionV1{}, errors.New("publication commit selection is not canonically encoded")
	}
	return selection, ValidatePublicationCommitSelectionV1(selection)
}

func PublicationCommitSelectionV1Bytes(selection PublicationCommitSelectionV1) ([]byte, error) {
	if err := ValidatePublicationCommitSelectionV1(selection); err != nil {
		return nil, err
	}
	return json.Marshal(selection)
}

func PublicationCommitSelectionSigningBytesV1(selection PublicationCommitSelectionV1) []byte {
	selection.AuthoritySignature = ""
	selection.RecordDigest = ""
	body, _ := json.Marshal(selection)
	digest := sha256.Sum256(body)
	result := append([]byte(nil), publicationCommitSelectionSignatureDomainV1...)
	return append(result, digest[:]...)
}

func validatePublicationCommitSelectionInputV1(
	selection PublicationCommitSelectionV1,
	input PublicationCommitSelectionInputV1,
) error {
	attempt := input.Attempt
	commitInput := input.CommitInput
	if ValidatePublicationAttemptV1(attempt) != nil ||
		ValidatePublicationCommitReceiptExactV1(selection.UnsignedCommit, commitInput) != nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.SettlementDigest)) ||
		attempt.InstallationID != commitInput.InstallationID || attempt.EnrollmentID != commitInput.EnrollmentID ||
		attempt.CandidateRecordDigest != commitInput.Candidate.RecordDigest ||
		attempt.CandidatePublicationReceiptID != commitInput.Candidate.ReceiptID ||
		attempt.PublicationIndexDigest != commitInput.Index.IndexDigest ||
		attempt.ExpectedEvidenceBundleDigest != commitInput.PreviousBundle.RecordDigest ||
		attempt.NextEvidenceBundleDigest != commitInput.CommittedBundle.RecordDigest ||
		attempt.ThreadID != commitInput.Candidate.ThreadID || attempt.TurnID != commitInput.Candidate.TurnID ||
		attempt.ContextDigest != commitInput.Candidate.ContextDigest || attempt.CaseBindingHash != commitInput.Candidate.CaseBindingHash ||
		attempt.ContextEpoch != commitInput.Candidate.ContextEpoch || attempt.DatasetSnapshotID != commitInput.Candidate.DatasetSnapshotID ||
		attempt.SourceManifestHash != commitInput.Candidate.SourceManifestHash ||
		attempt.ClaimLedgerDigest != commitInput.Candidate.ClaimLedgerDigest ||
		attempt.PIIProjectionDigest != commitInput.Candidate.PIIProjectionDigest ||
		attempt.RenderInspectionDigest != commitInput.Candidate.RenderInspectionDigest ||
		attempt.ReportSHA256 != commitInput.Candidate.ReportSHA256 || attempt.ReportByteLength != commitInput.Candidate.ReportByteLength ||
		attempt.TargetIdentityDigest != commitInput.Candidate.TargetIdentityDigest ||
		attempt.AuthorityKeyID != commitInput.AuthorityKeyID || attempt.AuthorityPublicKey != selection.AuthorityPublicKey {
		return errors.New("publication commit selection attempt graph is invalid")
	}
	return nil
}

func validatePublicationCommitSelectionUnsignedV1(selection PublicationCommitSelectionV1) error {
	commit := selection.UnsignedCommit
	if selection.SchemaVersion != PublicationCommitSelectionSchemaVersion || selection.Purpose != PublicationCommitSelectionPurpose ||
		!domainsecurity.IsSHA256Hex(selection.SelectionID) ||
		selection.SelectionID != PublicationCommitSelectionIDV1(selection.InstallationID, selection.EnrollmentID, selection.AttemptID) ||
		!domainsecurity.IsSHA256Hex(selection.InstallationID) || !domainsecurity.IsSHA256Hex(selection.EnrollmentID) ||
		!domainsecurity.IsSHA256Hex(selection.AttemptID) || !domainsecurity.IsSHA256Hex(selection.CandidateRecordDigest) ||
		!domainsecurity.IsSHA256Hex(selection.PublicationIndexDigest) || !domainsecurity.IsSHA256Hex(selection.AuthorityAdvanceMutationID) ||
		!domainsecurity.IsSHA256Hex(selection.AuthorityAdvanceIntentDigest) ||
		!domainsecurity.IsSHA256Hex(selection.AuthorityAdvanceSettlementDigest) ||
		!domainsecurity.IsSHA256Hex(selection.PreviousEvidenceBundleDigest) ||
		!domainsecurity.IsSHA256Hex(selection.CommittedEvidenceBundleDigest) ||
		!domainsecurity.IsSHA256Hex(selection.ObserveRequestDigest) || !domainsecurity.IsSHA256Hex(selection.ObservationDigest) ||
		!domainsecurity.IsSHA256Hex(selection.WitnessBindingDigest) || !domainsecurity.IsSHA256Hex(selection.CommitReceiptID) ||
		!domainsecurity.IsSHA256Hex(selection.CommitSigningDigest) || selection.AuthorityAlgorithm != PublicationReceiptAlgorithm ||
		!domainsecurity.IsSHA256Hex(selection.AuthorityKeyID) ||
		commit.AuthoritySignature != "" || commit.RecordDigest != "" || validatePublicationCommitReceiptUnsignedV1(commit) != nil ||
		selection.CandidateRecordDigest != commit.CandidateRecordDigest || selection.PublicationIndexDigest != commit.PublicationIndexDigest ||
		selection.PreviousEvidenceBundleDigest != commit.PreviousEvidenceBundleDigest ||
		selection.CommittedEvidenceBundleDigest != commit.CommittedEvidenceBundleDigest ||
		selection.ObserveRequestDigest != commit.WitnessBinding.ObserveRequestDigest ||
		selection.ObservationDigest != commit.WitnessBinding.ObservationDigest ||
		selection.WitnessBindingDigest != commit.WitnessBinding.BindingDigest || selection.CommitReceiptID != commit.CommitReceiptID ||
		selection.CommitSigningDigest != domainsecurity.SHA256Hex(PublicationCommitReceiptSigningBytesV1(commit)) ||
		selection.AuthorityKeyID != commit.AuthorityKeyID || selection.AuthorityPublicKey != commit.AuthorityPublicKey {
		return errors.New("publication commit selection is invalid")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(selection.AuthorityPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize || base64.RawURLEncoding.EncodeToString(publicKey) != selection.AuthorityPublicKey ||
		selection.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return errors.New("publication commit selection authority is invalid")
	}
	return nil
}

func publicationCommitSelectionRecordDigestV1(selection PublicationCommitSelectionV1) string {
	selection.RecordDigest = ""
	body, _ := json.Marshal(selection)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), publicationCommitSelectionRecordDomainV1...), body...))
}

func reflectPublicationCommitSelectionV1(left, right PublicationCommitSelectionV1) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
