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
	PublicationCommitReceiptSchemaVersion = 1
	PublicationCommitReceiptPurpose       = "analytix.publication-commit-receipt/v1"
)

var publicationCommitReceiptSignatureDomainV1 = []byte("analytix.publication-commit-receipt/signature/v1\x00")

// PublicationCommitReceiptV1 is issued only after the publication index is
// selected by a fresh shared-evidence witness observation. The earlier
// PublicationReceiptV1 remains a candidate and cannot authorize delivery.
type PublicationCommitReceiptV1 struct {
	SchemaVersion                     int                                              `json:"schemaVersion"`
	Purpose                           string                                           `json:"purpose"`
	CommitReceiptID                   string                                           `json:"commitReceiptId"`
	InstallationID                    string                                           `json:"installationId"`
	EnrollmentID                      string                                           `json:"enrollmentId"`
	ThreadID                          string                                           `json:"threadId"`
	TurnID                            string                                           `json:"turnId"`
	ContextDigest                     string                                           `json:"contextDigest"`
	CaseID                            string                                           `json:"caseId"`
	CaseBindingHash                   string                                           `json:"caseBindingHash"`
	ContextEpoch                      uint64                                           `json:"contextEpoch"`
	DatasetSnapshotID                 string                                           `json:"datasetSnapshotId"`
	SourceManifestHash                string                                           `json:"sourceManifestHash"`
	CandidateReceiptID                string                                           `json:"candidateReceiptId"`
	CandidateRecordDigest             string                                           `json:"candidateRecordDigest"`
	PublicationIndexDigest            string                                           `json:"publicationIndexDigest"`
	PublicationIndexGeneration        uint64                                           `json:"publicationIndexGeneration"`
	PublicationMutationID             string                                           `json:"publicationMutationId"`
	PreviousEvidenceBundleDigest      string                                           `json:"previousEvidenceBundleDigest"`
	CommittedEvidenceBundleDigest     string                                           `json:"committedEvidenceBundleDigest"`
	CommittedEvidenceBundleGeneration uint64                                           `json:"committedEvidenceBundleGeneration"`
	DatasetSnapshotIndexDigest        string                                           `json:"datasetSnapshotIndexDigest"`
	DatasetSnapshotCount              uint64                                           `json:"datasetSnapshotCount"`
	EvidenceRegistryIndexDigest       string                                           `json:"evidenceRegistryIndexDigest"`
	EvidenceRegistryCount             uint64                                           `json:"evidenceRegistryCount"`
	CommittedPublicationIndexDigest   string                                           `json:"committedPublicationIndexDigest"`
	CommittedPublicationCount         uint64                                           `json:"committedPublicationCount"`
	WitnessBinding                    domainevidence.EvidenceAuthorityWitnessBindingV1 `json:"witnessBinding"`
	EvidenceReceiptSetDigest          string                                           `json:"evidenceReceiptSetDigest"`
	EvidenceReceiptCount              uint64                                           `json:"evidenceReceiptCount"`
	ClaimLedgerDigest                 string                                           `json:"claimLedgerDigest"`
	ClaimCount                        uint64                                           `json:"claimCount"`
	ReportSHA256                      string                                           `json:"reportSha256"`
	ReportByteLength                  uint64                                           `json:"reportByteLength"`
	MediaType                         string                                           `json:"mediaType"`
	PIIProjectionDigest               string                                           `json:"piiProjectionDigest"`
	PIIProjectionClass                string                                           `json:"piiProjectionClass"`
	AuthorizationAuditDigest          string                                           `json:"authorizationAuditDigest"`
	RenderInspectionDigest            string                                           `json:"renderInspectionDigest"`
	TargetIdentityDigest              string                                           `json:"targetIdentityDigest"`
	AuthorityAlgorithm                string                                           `json:"authorityAlgorithm"`
	AuthorityKeyID                    string                                           `json:"authorityKeyId"`
	AuthorityPublicKey                string                                           `json:"authorityPublicKey"`
	AuthoritySignature                string                                           `json:"authoritySignature"`
	RecordDigest                      string                                           `json:"recordDigest"`
}

type PublicationCommitReceiptInputV1 struct {
	PreviousBundle     domainevidence.EvidenceAuthorityBundleV1
	CommittedBundle    domainevidence.EvidenceAuthorityBundleV1
	ObserveRequest     domainsecurity.MonotonicHeadObserveRequestV1
	Observation        domainsecurity.MonotonicHeadObservationV1
	Candidate          PublicationReceiptV1
	Index              PublicationIndexV1
	InstallationID     string
	EnrollmentID       string
	AuthorityKeyID     string
	AuthorityPublicKey []byte
	WitnessKeyID       string
	WitnessPublicKey   []byte
}

func NewPublicationCommitReceiptV1(input PublicationCommitReceiptInputV1, sign PublicationReceiptSignFuncV1) (PublicationCommitReceiptV1, error) {
	binding, err := domainevidence.NewEvidenceAuthorityWitnessBindingV1(
		input.CommittedBundle, input.ObserveRequest, input.Observation,
		input.InstallationID, input.EnrollmentID, input.AuthorityKeyID, input.AuthorityPublicKey,
		input.WitnessKeyID, input.WitnessPublicKey,
	)
	if err != nil {
		return PublicationCommitReceiptV1{}, err
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	candidate := input.Candidate
	index := input.Index
	committed := input.CommittedBundle
	receipt := PublicationCommitReceiptV1{
		SchemaVersion: PublicationCommitReceiptSchemaVersion, Purpose: PublicationCommitReceiptPurpose,
		InstallationID: strings.TrimSpace(input.InstallationID), EnrollmentID: strings.TrimSpace(input.EnrollmentID),
		ThreadID: candidate.ThreadID, TurnID: candidate.TurnID, ContextDigest: candidate.ContextDigest,
		CaseID: candidate.CaseID, CaseBindingHash: candidate.CaseBindingHash, ContextEpoch: candidate.ContextEpoch,
		DatasetSnapshotID: candidate.DatasetSnapshotID, SourceManifestHash: candidate.SourceManifestHash,
		CandidateReceiptID: candidate.ReceiptID, CandidateRecordDigest: candidate.RecordDigest,
		PublicationIndexDigest: index.IndexDigest, PublicationIndexGeneration: index.Generation, PublicationMutationID: index.MutationID,
		PreviousEvidenceBundleDigest:  input.PreviousBundle.RecordDigest,
		CommittedEvidenceBundleDigest: committed.RecordDigest, CommittedEvidenceBundleGeneration: committed.Generation,
		DatasetSnapshotIndexDigest: committed.DatasetSnapshotIndexDigest, DatasetSnapshotCount: committed.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: committed.EvidenceRegistryIndexDigest, EvidenceRegistryCount: committed.EvidenceRegistryCount,
		CommittedPublicationIndexDigest: committed.PublicationIndexDigest, CommittedPublicationCount: committed.PublicationCount,
		WitnessBinding: binding, EvidenceReceiptSetDigest: candidate.EvidenceReceiptSetDigest,
		EvidenceReceiptCount: candidate.EvidenceReceiptCount, ClaimLedgerDigest: candidate.ClaimLedgerDigest, ClaimCount: candidate.ClaimCount,
		ReportSHA256: candidate.ReportSHA256, ReportByteLength: candidate.ReportByteLength, MediaType: candidate.MediaType,
		PIIProjectionDigest: candidate.PIIProjectionDigest, PIIProjectionClass: candidate.PIIProjectionClass,
		AuthorizationAuditDigest: candidate.AuthorizationAuditDigest, RenderInspectionDigest: candidate.RenderInspectionDigest,
		TargetIdentityDigest: candidate.TargetIdentityDigest, AuthorityAlgorithm: PublicationReceiptAlgorithm,
		AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.CommitReceiptID = publicationCommitReceiptIDV1(receipt)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return PublicationCommitReceiptV1{}, errors.New("publication commit receipt signing authority is invalid")
	}
	if err := ValidatePublicationCommitReceiptExactV1(receipt, input); err != nil {
		return PublicationCommitReceiptV1{}, err
	}
	signature, err := sign(PublicationCommitReceiptSigningBytesV1(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PublicationCommitReceiptV1{}, errors.New("publication commit receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.RecordDigest = publicationCommitReceiptRecordDigestV1(receipt)
	if err := ValidatePublicationCommitReceiptExactV1(receipt, input); err != nil {
		return PublicationCommitReceiptV1{}, err
	}
	return receipt, nil
}

func ValidatePublicationCommitReceiptV1(receipt PublicationCommitReceiptV1) error {
	if err := validatePublicationCommitReceiptUnsignedV1(receipt); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PublicationCommitReceiptSigningBytesV1(receipt), signature) ||
		!domainsecurity.IsSHA256Hex(receipt.RecordDigest) || receipt.RecordDigest != publicationCommitReceiptRecordDigestV1(receipt) {
		return errors.New("publication commit receipt signature is invalid")
	}
	return nil
}

// ValidatePublicationCommitReceiptMaterialsV1 verifies the immutable local
// reference graph selected by a commit receipt. Witness freshness remains a
// separate runtime authority check; this function prevents a trusted commit
// from being paired with a different receipt or publication-index node.
func ValidatePublicationCommitReceiptMaterialsV1(
	commit PublicationCommitReceiptV1,
	receipt PublicationReceiptV1,
	index PublicationIndexV1,
) error {
	if ValidatePublicationCommitReceiptV1(commit) != nil ||
		ValidatePublicationIndexReceiptV1(index, receipt) != nil ||
		commit.InstallationID != receipt.InstallationID || commit.EnrollmentID != receipt.EnrollmentID ||
		commit.ThreadID != receipt.ThreadID || commit.TurnID != receipt.TurnID || commit.ContextDigest != receipt.ContextDigest ||
		commit.CaseID != receipt.CaseID || commit.CaseBindingHash != receipt.CaseBindingHash || commit.ContextEpoch != receipt.ContextEpoch ||
		commit.DatasetSnapshotID != receipt.DatasetSnapshotID || commit.SourceManifestHash != receipt.SourceManifestHash ||
		commit.CandidateReceiptID != receipt.ReceiptID || commit.CandidateRecordDigest != receipt.RecordDigest ||
		commit.PublicationIndexDigest != index.IndexDigest || commit.PublicationIndexGeneration != index.Generation ||
		commit.PublicationMutationID != index.MutationID || commit.CommittedPublicationIndexDigest != index.IndexDigest ||
		commit.EvidenceReceiptSetDigest != receipt.EvidenceReceiptSetDigest || commit.EvidenceReceiptCount != receipt.EvidenceReceiptCount ||
		commit.ClaimLedgerDigest != receipt.ClaimLedgerDigest || commit.ClaimCount != receipt.ClaimCount ||
		commit.ReportSHA256 != receipt.ReportSHA256 || commit.ReportByteLength != receipt.ReportByteLength ||
		commit.MediaType != receipt.MediaType || commit.PIIProjectionDigest != receipt.PIIProjectionDigest ||
		commit.PIIProjectionClass != receipt.PIIProjectionClass || commit.AuthorizationAuditDigest != receipt.AuthorizationAuditDigest ||
		commit.RenderInspectionDigest != receipt.RenderInspectionDigest || commit.TargetIdentityDigest != receipt.TargetIdentityDigest ||
		commit.AuthorityKeyID != receipt.AuthorityKeyID || commit.AuthorityPublicKey != receipt.AuthorityPublicKey {
		return errors.New("publication commit receipt does not reference the exact publication materials")
	}
	return nil
}

func ValidatePublicationCommitReceiptExactV1(receipt PublicationCommitReceiptV1, input PublicationCommitReceiptInputV1) error {
	if receipt.AuthoritySignature != "" || receipt.RecordDigest != "" {
		if err := ValidatePublicationCommitReceiptV1(receipt); err != nil {
			return err
		}
	} else if err := validatePublicationCommitReceiptUnsignedV1(receipt); err != nil {
		return err
	}
	if ValidatePublicationReceiptForInstallationV1(
		input.Candidate, input.InstallationID, input.EnrollmentID, input.AuthorityKeyID, input.AuthorityPublicKey,
	) != nil || ValidatePublicationIndexReceiptV1(input.Index, input.Candidate) != nil ||
		domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(input.PreviousBundle, input.CommittedBundle) != nil ||
		input.Candidate.EvidenceAuthorityBundleDigest != input.PreviousBundle.RecordDigest ||
		input.CommittedBundle.PublicationIndexDigest != input.Index.IndexDigest ||
		input.CommittedBundle.PublicationCount != input.PreviousBundle.PublicationCount+1 ||
		input.CommittedBundle.DatasetSnapshotIndexDigest != input.PreviousBundle.DatasetSnapshotIndexDigest ||
		input.CommittedBundle.DatasetSnapshotCount != input.PreviousBundle.DatasetSnapshotCount ||
		input.CommittedBundle.EvidenceRegistryIndexDigest != input.PreviousBundle.EvidenceRegistryIndexDigest ||
		input.CommittedBundle.EvidenceRegistryCount != input.PreviousBundle.EvidenceRegistryCount {
		return errors.New("publication commit receipt transition is invalid")
	}
	if err := domainevidence.ValidateEvidenceAuthorityWitnessBindingExactV1(
		receipt.WitnessBinding, input.CommittedBundle, input.ObserveRequest, input.Observation,
		input.InstallationID, input.EnrollmentID, input.AuthorityKeyID, input.AuthorityPublicKey,
		input.WitnessKeyID, input.WitnessPublicKey,
	); err != nil {
		return err
	}
	expected, err := NewPublicationCommitReceiptV1WithoutSigning(input, receipt.WitnessBinding)
	if err != nil {
		return err
	}
	expected.AuthoritySignature = receipt.AuthoritySignature
	expected.RecordDigest = receipt.RecordDigest
	if expected != receipt {
		return errors.New("publication commit receipt does not match exact materials")
	}
	return nil
}

func NewPublicationCommitReceiptV1WithoutSigning(input PublicationCommitReceiptInputV1, binding domainevidence.EvidenceAuthorityWitnessBindingV1) (PublicationCommitReceiptV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if len(publicKey) != ed25519.PublicKeySize || strings.TrimSpace(input.AuthorityKeyID) != domainsecurity.SHA256Hex(publicKey) {
		return PublicationCommitReceiptV1{}, errors.New("publication commit receipt authority is invalid")
	}
	candidate, index, committed := input.Candidate, input.Index, input.CommittedBundle
	receipt := PublicationCommitReceiptV1{
		SchemaVersion: PublicationCommitReceiptSchemaVersion, Purpose: PublicationCommitReceiptPurpose,
		InstallationID: strings.TrimSpace(input.InstallationID), EnrollmentID: strings.TrimSpace(input.EnrollmentID),
		ThreadID: candidate.ThreadID, TurnID: candidate.TurnID, ContextDigest: candidate.ContextDigest, CaseID: candidate.CaseID,
		CaseBindingHash: candidate.CaseBindingHash, ContextEpoch: candidate.ContextEpoch, DatasetSnapshotID: candidate.DatasetSnapshotID,
		SourceManifestHash: candidate.SourceManifestHash, CandidateReceiptID: candidate.ReceiptID, CandidateRecordDigest: candidate.RecordDigest,
		PublicationIndexDigest: index.IndexDigest, PublicationIndexGeneration: index.Generation, PublicationMutationID: index.MutationID,
		PreviousEvidenceBundleDigest: input.PreviousBundle.RecordDigest, CommittedEvidenceBundleDigest: committed.RecordDigest,
		CommittedEvidenceBundleGeneration: committed.Generation, DatasetSnapshotIndexDigest: committed.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount: committed.DatasetSnapshotCount, EvidenceRegistryIndexDigest: committed.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount: committed.EvidenceRegistryCount, CommittedPublicationIndexDigest: committed.PublicationIndexDigest,
		CommittedPublicationCount: committed.PublicationCount, WitnessBinding: binding,
		EvidenceReceiptSetDigest: candidate.EvidenceReceiptSetDigest, EvidenceReceiptCount: candidate.EvidenceReceiptCount,
		ClaimLedgerDigest: candidate.ClaimLedgerDigest, ClaimCount: candidate.ClaimCount, ReportSHA256: candidate.ReportSHA256,
		ReportByteLength: candidate.ReportByteLength, MediaType: candidate.MediaType, PIIProjectionDigest: candidate.PIIProjectionDigest,
		PIIProjectionClass: candidate.PIIProjectionClass, AuthorizationAuditDigest: candidate.AuthorizationAuditDigest,
		RenderInspectionDigest: candidate.RenderInspectionDigest, TargetIdentityDigest: candidate.TargetIdentityDigest,
		AuthorityAlgorithm: PublicationReceiptAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.CommitReceiptID = publicationCommitReceiptIDV1(receipt)
	return receipt, validatePublicationCommitReceiptUnsignedV1(receipt)
}

func validatePublicationCommitReceiptUnsignedV1(receipt PublicationCommitReceiptV1) error {
	if receipt.SchemaVersion != PublicationCommitReceiptSchemaVersion || receipt.Purpose != PublicationCommitReceiptPurpose ||
		!domainsecurity.IsSHA256Hex(receipt.CommitReceiptID) || receipt.CommitReceiptID != publicationCommitReceiptIDV1(receipt) ||
		!domainsecurity.IsSHA256Hex(receipt.InstallationID) || !domainsecurity.IsSHA256Hex(receipt.EnrollmentID) ||
		strings.TrimSpace(receipt.ThreadID) == "" || strings.TrimSpace(receipt.TurnID) == "" || !domainsecurity.IsSHA256Hex(receipt.ContextDigest) ||
		strings.TrimSpace(receipt.CaseID) == "" || receipt.CaseID == domainsecurity.UnboundCaseID || !domainsecurity.IsSHA256Hex(receipt.CaseBindingHash) ||
		receipt.ContextEpoch == 0 || !domainsecurity.IsDatasetSnapshotIDV2Syntax(receipt.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(receipt.SourceManifestHash) || !domainsecurity.IsSHA256Hex(receipt.CandidateReceiptID) ||
		!domainsecurity.IsSHA256Hex(receipt.CandidateRecordDigest) || !domainsecurity.IsSHA256Hex(receipt.PublicationIndexDigest) ||
		receipt.PublicationIndexGeneration == 0 || !domainsecurity.IsSHA256Hex(receipt.PublicationMutationID) ||
		!domainsecurity.IsSHA256Hex(receipt.PreviousEvidenceBundleDigest) || !domainsecurity.IsSHA256Hex(receipt.CommittedEvidenceBundleDigest) ||
		receipt.CommittedEvidenceBundleGeneration == 0 || !domainsecurity.IsSHA256Hex(receipt.DatasetSnapshotIndexDigest) ||
		!domainsecurity.IsSHA256Hex(receipt.EvidenceRegistryIndexDigest) || !domainsecurity.IsSHA256Hex(receipt.CommittedPublicationIndexDigest) ||
		receipt.CommittedPublicationCount == 0 || domainevidence.ValidateEvidenceAuthorityWitnessBindingV1(receipt.WitnessBinding) != nil ||
		!domainsecurity.IsSHA256Hex(receipt.EvidenceReceiptSetDigest) || receipt.EvidenceReceiptCount == 0 ||
		!domainsecurity.IsSHA256Hex(receipt.ClaimLedgerDigest) || !domainsecurity.IsSHA256Hex(receipt.ReportSHA256) || receipt.ReportByteLength == 0 ||
		strings.TrimSpace(receipt.MediaType) == "" || !domainsecurity.IsSHA256Hex(receipt.PIIProjectionDigest) ||
		(receipt.PIIProjectionClass != PIIProjectionOrdinaryMasked && receipt.PIIProjectionClass != PIIProjectionControlledFull) ||
		!domainsecurity.IsSHA256Hex(receipt.RenderInspectionDigest) || !domainsecurity.IsSHA256Hex(receipt.TargetIdentityDigest) ||
		receipt.AuthorityAlgorithm != PublicationReceiptAlgorithm || !domainsecurity.IsSHA256Hex(receipt.AuthorityKeyID) {
		return errors.New("publication commit receipt is incomplete")
	}
	if receipt.PIIProjectionClass == PIIProjectionOrdinaryMasked && receipt.AuthorizationAuditDigest != "" ||
		receipt.PIIProjectionClass == PIIProjectionControlledFull && !domainsecurity.IsSHA256Hex(receipt.AuthorizationAuditDigest) {
		return errors.New("publication commit receipt PII authority is invalid")
	}
	return nil
}

func PublicationCommitReceiptV1Bytes(receipt PublicationCommitReceiptV1) ([]byte, error) {
	if err := ValidatePublicationCommitReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func ParsePublicationCommitReceiptV1(body []byte) (PublicationCommitReceiptV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 8, MaxTokens: 512, MaxStringBytes: 64 << 10,
	}); err != nil {
		return PublicationCommitReceiptV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var receipt PublicationCommitReceiptV1
	if err := decoder.Decode(&receipt); err != nil {
		return PublicationCommitReceiptV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PublicationCommitReceiptV1{}, errors.New("publication commit receipt contains trailing JSON")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal(canonical, body) {
		return PublicationCommitReceiptV1{}, errors.New("publication commit receipt is not canonical")
	}
	return receipt, ValidatePublicationCommitReceiptV1(receipt)
}

func PublicationCommitReceiptSigningBytesV1(receipt PublicationCommitReceiptV1) []byte {
	receipt.AuthoritySignature = ""
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), publicationCommitReceiptSignatureDomainV1...)
	return append(out, digest[:]...)
}

func publicationCommitReceiptIDV1(receipt PublicationCommitReceiptV1) string {
	receipt.CommitReceiptID = ""
	receipt.AuthoritySignature = ""
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(append([]byte("analytix.publication-commit-receipt/id/v1\x00"), body...))
}

func publicationCommitReceiptRecordDigestV1(receipt PublicationCommitReceiptV1) string {
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(append([]byte("analytix.publication-commit-receipt/record/v1\x00"), body...))
}
