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
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PublicationAttemptSchemaVersion = 1
	PublicationAttemptPurpose       = "analytix.report-publication-attempt/v1"
	PublicationAttemptAlgorithm     = "Ed25519"
	PublicationAttemptToolName      = "stage_case_report"
)

var (
	publicationAttemptIDDomainV1                  = []byte("analytix.report-publication-attempt/id/v1\x00")
	publicationAttemptSignatureDomainV1           = []byte("analytix.report-publication-attempt/signature/v1\x00")
	publicationAttemptRecordDigestDomainV1        = []byte("analytix.report-publication-attempt/digest/v1\x00")
	publicationIndexAttemptMutationIDDomainV1     = []byte("analytix.report-publication-index/mutation/v1\x00")
	publicationAuthorityAttemptMutationIDDomainV1 = []byte("analytix.report-publication-authority/mutation/v1\x00")
)

// PublicationAttemptV1 is the immutable write-ahead plan for one report-stage
// effect. It contains only host-issued identities and hashes; report bytes,
// claim payloads, raw PII, prompts, and reasoning are deliberately excluded.
// Its presence proves only that a fixed attempt was reserved. It is never
// publication authority without the separately witnessed commit graph.
type PublicationAttemptV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	AttemptID     string `json:"attemptId"`

	InstallationID string `json:"installationId"`
	EnrollmentID   string `json:"enrollmentId"`

	ReportStageWorkID        string `json:"reportStageWorkId"`
	ReportStageReceiptID     string `json:"reportStageReceiptId"`
	ReportStageReceiptSHA256 string `json:"reportStageReceiptSha256"`

	GrantRegistrySequence    uint64 `json:"grantRegistrySequence"`
	GrantRegistryDigest      string `json:"grantRegistryDigest"`
	GrantRegistryEntryDigest string `json:"grantRegistryEntryDigest"`
	GrantID                  string `json:"grantId"`
	ToolCallID               string `json:"toolCallId"`
	ToolName                 string `json:"toolName"`

	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	ContextDigest      string `json:"contextDigest"`
	CaseBindingHash    string `json:"caseBindingHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`

	StageInputHash           string `json:"stageInputHash"`
	ReportVariant            string `json:"reportVariant"`
	ClaimLedgerDigest        string `json:"claimLedgerDigest"`
	PIIProjectionDigest      string `json:"piiProjectionDigest"`
	PIIProjectionClass       string `json:"piiProjectionClass"`
	AuthorizationAuditDigest string `json:"authorizationAuditDigest"`
	RenderInspectionDigest   string `json:"renderInspectionDigest"`
	ReportSHA256             string `json:"reportSha256"`
	ReportByteLength         uint64 `json:"reportByteLength"`
	MediaType                string `json:"mediaType"`
	TargetIdentityDigest     string `json:"targetIdentityDigest"`

	ExpectedEvidenceBundleDigest   string `json:"expectedEvidenceBundleDigest"`
	ExpectedPublicationIndexDigest string `json:"expectedPublicationIndexDigest"`
	ExpectedPublicationCount       uint64 `json:"expectedPublicationCount"`
	CandidatePublicationReceiptID  string `json:"candidatePublicationReceiptId"`
	CandidateRecordDigest          string `json:"candidateRecordDigest"`
	PublicationIndexDigest         string `json:"publicationIndexDigest"`
	NextEvidenceBundleDigest       string `json:"nextEvidenceBundleDigest"`
	AuthorityAdvanceIntentDigest   string `json:"authorityAdvanceIntentDigest"`
	PublicationIndexMutationID     string `json:"publicationIndexMutationId"`
	AuthorityAdvanceMutationID     string `json:"authorityAdvanceMutationId"`
	IssuedAt                       string `json:"issuedAt"`

	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RecordDigest       string `json:"recordDigest"`
}

type PublicationAttemptInputV1 struct {
	InstallationID string
	EnrollmentID   string

	ReportStageReceipt domainpendingwork.PendingWorkReceiptV1
	ToolCallID         string
	StageInputHash     string

	Candidate PublicationReceiptV1
	Index     PublicationIndexV1

	ExpectedEvidenceBundleDigest   string
	ExpectedPublicationIndexDigest string
	ExpectedPublicationCount       uint64
	NextEvidenceBundleDigest       string
	AuthorityAdvanceIntentDigest   string

	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

type PublicationAttemptSignFuncV1 func([]byte) ([]byte, error)

func NewPublicationAttemptV1(input PublicationAttemptInputV1, sign PublicationAttemptSignFuncV1) (PublicationAttemptV1, error) {
	receipt := input.ReportStageReceipt
	candidate := input.Candidate
	index := input.Index
	receiptBody, receiptErr := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if receiptErr != nil || !domainmodel.IsHostToolCallIDV1(input.ToolCallID) ||
		ValidatePublicationReceiptV1(candidate) != nil || ValidatePublicationIndexV1(index) != nil ||
		ValidatePublicationIndexReceiptV1(index, candidate) != nil || len(receipt.GrantMembers) != 1 {
		return PublicationAttemptV1{}, errors.New("publication attempt authority graph is invalid")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	attemptID := PublicationAttemptIDV1(input.InstallationID, input.EnrollmentID, receipt.WorkID)
	attempt := PublicationAttemptV1{
		SchemaVersion: PublicationAttemptSchemaVersion,
		Purpose:       PublicationAttemptPurpose,
		AttemptID:     attemptID,

		InstallationID: strings.TrimSpace(input.InstallationID),
		EnrollmentID:   strings.TrimSpace(input.EnrollmentID),

		ReportStageWorkID:        receipt.WorkID,
		ReportStageReceiptID:     receipt.ReceiptID,
		ReportStageReceiptSHA256: domainsecurity.SHA256Hex(receiptBody),

		GrantRegistrySequence:    receipt.GrantRegistrySequence,
		GrantRegistryDigest:      receipt.GrantRegistryDigest,
		GrantRegistryEntryDigest: receipt.GrantMembers[0].RegistryEntryDigest,
		GrantID:                  receipt.GrantMembers[0].GrantID,
		ToolCallID:               strings.TrimSpace(input.ToolCallID),
		ToolName:                 PublicationAttemptToolName,

		ThreadID: receipt.Context.ThreadID, TurnID: receipt.Context.TurnID,
		ContextDigest: receipt.Context.ContextDigest, CaseBindingHash: receipt.Context.CaseBindingHash,
		ContextEpoch: receipt.Context.ContextEpoch, DatasetSnapshotID: receipt.Context.DatasetSnapshotID,
		SourceManifestHash: receipt.Context.SourceManifestHash,

		StageInputHash: strings.TrimSpace(input.StageInputHash), ReportVariant: candidate.ReportVariant,
		ClaimLedgerDigest: candidate.ClaimLedgerDigest, PIIProjectionDigest: candidate.PIIProjectionDigest,
		PIIProjectionClass: candidate.PIIProjectionClass, AuthorizationAuditDigest: candidate.AuthorizationAuditDigest,
		RenderInspectionDigest: candidate.RenderInspectionDigest, ReportSHA256: candidate.ReportSHA256,
		ReportByteLength: candidate.ReportByteLength, MediaType: candidate.MediaType,
		TargetIdentityDigest: candidate.TargetIdentityDigest,

		ExpectedEvidenceBundleDigest:   strings.TrimSpace(input.ExpectedEvidenceBundleDigest),
		ExpectedPublicationIndexDigest: strings.TrimSpace(input.ExpectedPublicationIndexDigest),
		ExpectedPublicationCount:       input.ExpectedPublicationCount,
		CandidatePublicationReceiptID:  candidate.ReceiptID, CandidateRecordDigest: candidate.RecordDigest,
		PublicationIndexDigest:       index.IndexDigest,
		NextEvidenceBundleDigest:     strings.TrimSpace(input.NextEvidenceBundleDigest),
		AuthorityAdvanceIntentDigest: strings.TrimSpace(input.AuthorityAdvanceIntentDigest),
		PublicationIndexMutationID:   PublicationIndexMutationIDForAttemptV1(attemptID),
		AuthorityAdvanceMutationID:   PublicationAuthorityMutationIDForAttemptV1(attemptID),
		IssuedAt:                     candidate.IssuedAt,

		AuthorityAlgorithm: PublicationAttemptAlgorithm,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || attempt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		validatePublicationAttemptInputGraphV1(attempt, receipt, candidate, index) != nil {
		return PublicationAttemptV1{}, errors.New("publication attempt input is invalid")
	}
	signature, err := sign(PublicationAttemptSigningBytesV1(attempt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PublicationAttemptV1{}, errors.New("publication attempt signing failed")
	}
	attempt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	attempt.RecordDigest = publicationAttemptRecordDigestV1(attempt)
	if err := ValidatePublicationAttemptV1(attempt); err != nil {
		return PublicationAttemptV1{}, err
	}
	return attempt, nil
}

func ValidatePublicationAttemptV1(attempt PublicationAttemptV1) error {
	if err := validatePublicationAttemptUnsignedV1(attempt); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(attempt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(attempt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != attempt.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != attempt.AuthoritySignature ||
		attempt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PublicationAttemptSigningBytesV1(attempt), signature) {
		return errors.New("publication attempt signature is invalid")
	}
	if !domainsecurity.IsSHA256Hex(attempt.RecordDigest) || attempt.RecordDigest != publicationAttemptRecordDigestV1(attempt) ||
		attempt.RecordDigest == attempt.AttemptID {
		return errors.New("publication attempt record digest is invalid")
	}
	return nil
}

func ValidatePublicationAttemptForInstallationV1(attempt PublicationAttemptV1, installationID, enrollmentID, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidatePublicationAttemptV1(attempt); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if attempt.InstallationID != strings.TrimSpace(installationID) || attempt.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		len(authorityPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(authorityKeyID) != domainsecurity.SHA256Hex(authorityPublicKey) ||
		attempt.AuthorityKeyID != strings.TrimSpace(authorityKeyID) ||
		attempt.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("publication attempt installation anchor mismatch")
	}
	return nil
}

func ValidatePublicationAttemptGraphV1(attempt PublicationAttemptV1, receipt domainpendingwork.PendingWorkReceiptV1, candidate PublicationReceiptV1, index PublicationIndexV1) error {
	if err := ValidatePublicationAttemptV1(attempt); err != nil {
		return err
	}
	return validatePublicationAttemptInputGraphV1(attempt, receipt, candidate, index)
}

// ValidatePublicationAttemptStageV1 verifies the write-ahead reservation
// against its exact pending report-stage receipt even when later candidate and
// index records were not yet durable at the crash cut.
func ValidatePublicationAttemptStageV1(attempt PublicationAttemptV1, receipt domainpendingwork.PendingWorkReceiptV1) error {
	if err := ValidatePublicationAttemptV1(attempt); err != nil {
		return err
	}
	receiptBody, receiptErr := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if receiptErr != nil || len(receipt.GrantMembers) != 1 ||
		receipt.Kind != domainpendingwork.KindReportStage || receipt.GrantMembers[0].ResultItemID != "" ||
		receipt.GrantMembers[0].ResultItemDigest != "" ||
		receipt.AuthorityKeyID != attempt.AuthorityKeyID || receipt.AuthorityPublicKey != attempt.AuthorityPublicKey ||
		attempt.ReportStageWorkID != receipt.WorkID || attempt.ReportStageReceiptID != receipt.ReceiptID ||
		attempt.ReportStageReceiptSHA256 != domainsecurity.SHA256Hex(receiptBody) ||
		attempt.GrantRegistrySequence != receipt.GrantRegistrySequence || attempt.GrantRegistryDigest != receipt.GrantRegistryDigest ||
		attempt.GrantRegistryEntryDigest != receipt.GrantMembers[0].RegistryEntryDigest || attempt.GrantID != receipt.GrantMembers[0].GrantID ||
		attempt.ThreadID != receipt.Context.ThreadID || attempt.TurnID != receipt.Context.TurnID ||
		attempt.ContextDigest != receipt.Context.ContextDigest || attempt.CaseBindingHash != receipt.Context.CaseBindingHash ||
		attempt.ContextEpoch != receipt.Context.ContextEpoch || attempt.DatasetSnapshotID != receipt.Context.DatasetSnapshotID ||
		attempt.SourceManifestHash != receipt.Context.SourceManifestHash {
		return errors.New("publication attempt report-stage binding mismatch")
	}
	return nil
}

// ValidatePublicationAttemptCandidateV1 verifies the next durable suffix after
// the attempt while allowing a crash before the publication index write.
func ValidatePublicationAttemptCandidateV1(attempt PublicationAttemptV1, receipt domainpendingwork.PendingWorkReceiptV1, candidate PublicationReceiptV1) error {
	if ValidatePublicationAttemptStageV1(attempt, receipt) != nil || ValidatePublicationReceiptV1(candidate) != nil ||
		candidate.AuthorityKeyID != attempt.AuthorityKeyID || candidate.AuthorityPublicKey != attempt.AuthorityPublicKey ||
		candidate.ThreadID != attempt.ThreadID || candidate.TurnID != attempt.TurnID || candidate.ContextDigest != attempt.ContextDigest ||
		candidate.CaseBindingHash != attempt.CaseBindingHash || candidate.ContextEpoch != attempt.ContextEpoch ||
		candidate.DatasetSnapshotID != attempt.DatasetSnapshotID || candidate.SourceManifestHash != attempt.SourceManifestHash ||
		candidate.InstallationID != attempt.InstallationID || candidate.EnrollmentID != attempt.EnrollmentID ||
		attempt.ReportVariant != candidate.ReportVariant || attempt.ClaimLedgerDigest != candidate.ClaimLedgerDigest ||
		attempt.PIIProjectionDigest != candidate.PIIProjectionDigest || attempt.PIIProjectionClass != candidate.PIIProjectionClass ||
		attempt.AuthorizationAuditDigest != candidate.AuthorizationAuditDigest ||
		attempt.RenderInspectionDigest != candidate.RenderInspectionDigest || attempt.ReportSHA256 != candidate.ReportSHA256 ||
		attempt.ReportByteLength != candidate.ReportByteLength || attempt.MediaType != candidate.MediaType ||
		attempt.TargetIdentityDigest != candidate.TargetIdentityDigest ||
		attempt.ExpectedEvidenceBundleDigest != candidate.EvidenceAuthorityBundleDigest ||
		attempt.CandidatePublicationReceiptID != candidate.ReceiptID || attempt.CandidateRecordDigest != candidate.RecordDigest ||
		attempt.IssuedAt != candidate.IssuedAt {
		return errors.New("publication attempt candidate binding mismatch")
	}
	return nil
}

func ParsePublicationAttemptV1(body []byte) (PublicationAttemptV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 4, MaxTokens: 256, MaxStringBytes: 64 << 10,
	}); err != nil {
		return PublicationAttemptV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var attempt PublicationAttemptV1
	if err := decoder.Decode(&attempt); err != nil {
		return PublicationAttemptV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PublicationAttemptV1{}, errors.New("publication attempt contains trailing JSON")
	}
	canonical, err := json.Marshal(attempt)
	if err != nil || !bytes.Equal(body, canonical) {
		return PublicationAttemptV1{}, errors.New("publication attempt is not canonically encoded")
	}
	return attempt, ValidatePublicationAttemptV1(attempt)
}

func PublicationAttemptV1Bytes(attempt PublicationAttemptV1) ([]byte, error) {
	if err := ValidatePublicationAttemptV1(attempt); err != nil {
		return nil, err
	}
	return json.Marshal(attempt)
}

func PublicationAttemptSigningBytesV1(attempt PublicationAttemptV1) []byte {
	attempt.AuthoritySignature = ""
	attempt.RecordDigest = ""
	body, _ := json.Marshal(attempt)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), publicationAttemptSignatureDomainV1...)
	return append(out, digest[:]...)
}

func PublicationAttemptIDV1(installationID, enrollmentID, reportStageWorkID string) string {
	installationID = strings.TrimSpace(installationID)
	enrollmentID = strings.TrimSpace(enrollmentID)
	reportStageWorkID = strings.TrimSpace(reportStageWorkID)
	if !domainsecurity.IsSHA256Hex(installationID) || !domainsecurity.IsSHA256Hex(enrollmentID) || !domainsecurity.IsSHA256Hex(reportStageWorkID) {
		return ""
	}
	payload, _ := json.Marshal([]string{installationID, enrollmentID, reportStageWorkID})
	return domainsecurity.SHA256Hex(append(append([]byte(nil), publicationAttemptIDDomainV1...), payload...))
}

func PublicationIndexMutationIDForAttemptV1(attemptID string) string {
	return publicationAttemptDerivedDigestV1(publicationIndexAttemptMutationIDDomainV1, attemptID)
}

func PublicationAuthorityMutationIDForAttemptV1(attemptID string) string {
	return publicationAttemptDerivedDigestV1(publicationAuthorityAttemptMutationIDDomainV1, attemptID)
}

func validatePublicationAttemptUnsignedV1(attempt PublicationAttemptV1) error {
	issuedAt, timeErr := time.Parse(time.RFC3339Nano, attempt.IssuedAt)
	if attempt.SchemaVersion != PublicationAttemptSchemaVersion || attempt.Purpose != PublicationAttemptPurpose ||
		!domainsecurity.IsSHA256Hex(attempt.AttemptID) ||
		attempt.AttemptID != PublicationAttemptIDV1(attempt.InstallationID, attempt.EnrollmentID, attempt.ReportStageWorkID) ||
		!domainsecurity.IsSHA256Hex(attempt.InstallationID) || !domainsecurity.IsSHA256Hex(attempt.EnrollmentID) ||
		!domainsecurity.IsSHA256Hex(attempt.ReportStageWorkID) || !domainsecurity.IsSHA256Hex(attempt.ReportStageReceiptID) ||
		!domainsecurity.IsSHA256Hex(attempt.ReportStageReceiptSHA256) || attempt.GrantRegistrySequence == 0 ||
		!domainsecurity.IsSHA256Hex(attempt.GrantRegistryDigest) || !domainsecurity.IsSHA256Hex(attempt.GrantRegistryEntryDigest) ||
		!validPublicationAttemptOpaqueV1(attempt.GrantID) || !validPublicationAttemptOpaqueV1(attempt.ToolCallID) ||
		attempt.ToolName != PublicationAttemptToolName || !validPublicationAttemptOpaqueV1(attempt.ThreadID) ||
		!validPublicationAttemptOpaqueV1(attempt.TurnID) || !domainsecurity.IsSHA256Hex(attempt.ContextDigest) ||
		!domainsecurity.IsSHA256Hex(attempt.CaseBindingHash) || attempt.ContextEpoch == 0 ||
		!validPublicationAttemptOpaqueV1(attempt.DatasetSnapshotID) || !domainsecurity.IsSHA256Hex(attempt.SourceManifestHash) ||
		!domainsecurity.IsSHA256Hex(attempt.StageInputHash) || !validReportVariant(attempt.ReportVariant) ||
		!domainsecurity.IsSHA256Hex(attempt.ClaimLedgerDigest) || !domainsecurity.IsSHA256Hex(attempt.PIIProjectionDigest) ||
		!validPublicationAttemptPIIClassV1(attempt.PIIProjectionClass) || !domainsecurity.IsSHA256Hex(attempt.RenderInspectionDigest) ||
		!domainsecurity.IsSHA256Hex(attempt.ReportSHA256) || attempt.ReportByteLength == 0 ||
		!validPublicationAttemptOpaqueV1(attempt.MediaType) || !domainsecurity.IsSHA256Hex(attempt.TargetIdentityDigest) ||
		!domainsecurity.IsSHA256Hex(attempt.ExpectedEvidenceBundleDigest) ||
		!domainsecurity.IsSHA256Hex(attempt.ExpectedPublicationIndexDigest) ||
		!domainsecurity.IsSHA256Hex(attempt.CandidatePublicationReceiptID) || !domainsecurity.IsSHA256Hex(attempt.CandidateRecordDigest) ||
		!domainsecurity.IsSHA256Hex(attempt.PublicationIndexDigest) || !domainsecurity.IsSHA256Hex(attempt.NextEvidenceBundleDigest) ||
		!domainsecurity.IsSHA256Hex(attempt.AuthorityAdvanceIntentDigest) ||
		attempt.PublicationIndexMutationID != PublicationIndexMutationIDForAttemptV1(attempt.AttemptID) ||
		attempt.AuthorityAdvanceMutationID != PublicationAuthorityMutationIDForAttemptV1(attempt.AttemptID) ||
		timeErr != nil || issuedAt.IsZero() || attempt.AuthorityAlgorithm != PublicationAttemptAlgorithm ||
		!domainsecurity.IsSHA256Hex(attempt.AuthorityKeyID) {
		return errors.New("publication attempt identity or binding is invalid")
	}
	if attempt.PIIProjectionClass == PIIProjectionControlledFull {
		if !domainsecurity.IsSHA256Hex(attempt.AuthorizationAuditDigest) {
			return errors.New("controlled publication attempt authorization binding is invalid")
		}
	} else if attempt.AuthorizationAuditDigest != "" {
		return errors.New("ordinary publication attempt contains controlled authorization authority")
	}
	return nil
}

func validatePublicationAttemptInputGraphV1(attempt PublicationAttemptV1, receipt domainpendingwork.PendingWorkReceiptV1, candidate PublicationReceiptV1, index PublicationIndexV1) error {
	receiptBody, receiptErr := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if receiptErr != nil || ValidatePublicationReceiptV1(candidate) != nil || ValidatePublicationIndexV1(index) != nil ||
		ValidatePublicationIndexReceiptV1(index, candidate) != nil || len(receipt.GrantMembers) != 1 ||
		receipt.Kind != domainpendingwork.KindReportStage || receipt.GrantMembers[0].ResultItemID != "" ||
		receipt.GrantMembers[0].ResultItemDigest != "" ||
		receipt.AuthorityKeyID != attempt.AuthorityKeyID || receipt.AuthorityPublicKey != attempt.AuthorityPublicKey ||
		candidate.AuthorityKeyID != attempt.AuthorityKeyID || candidate.AuthorityPublicKey != attempt.AuthorityPublicKey ||
		index.AuthorityKeyID != attempt.AuthorityKeyID || index.AuthorityPublicKey != attempt.AuthorityPublicKey {
		return errors.New("publication attempt authority graph is invalid")
	}
	if attempt.ReportStageWorkID != receipt.WorkID || attempt.ReportStageReceiptID != receipt.ReceiptID ||
		attempt.ReportStageReceiptSHA256 != domainsecurity.SHA256Hex(receiptBody) ||
		attempt.GrantRegistrySequence != receipt.GrantRegistrySequence || attempt.GrantRegistryDigest != receipt.GrantRegistryDigest ||
		attempt.GrantRegistryEntryDigest != receipt.GrantMembers[0].RegistryEntryDigest || attempt.GrantID != receipt.GrantMembers[0].GrantID ||
		attempt.ThreadID != receipt.Context.ThreadID || attempt.TurnID != receipt.Context.TurnID ||
		attempt.ContextDigest != receipt.Context.ContextDigest || attempt.CaseBindingHash != receipt.Context.CaseBindingHash ||
		attempt.ContextEpoch != receipt.Context.ContextEpoch || attempt.DatasetSnapshotID != receipt.Context.DatasetSnapshotID ||
		attempt.SourceManifestHash != receipt.Context.SourceManifestHash || candidate.ThreadID != attempt.ThreadID ||
		candidate.TurnID != attempt.TurnID || candidate.ContextDigest != attempt.ContextDigest ||
		candidate.CaseBindingHash != attempt.CaseBindingHash || candidate.ContextEpoch != attempt.ContextEpoch ||
		candidate.DatasetSnapshotID != attempt.DatasetSnapshotID || candidate.SourceManifestHash != attempt.SourceManifestHash ||
		candidate.InstallationID != attempt.InstallationID || candidate.EnrollmentID != attempt.EnrollmentID ||
		attempt.ReportVariant != candidate.ReportVariant || attempt.ClaimLedgerDigest != candidate.ClaimLedgerDigest ||
		attempt.PIIProjectionDigest != candidate.PIIProjectionDigest || attempt.PIIProjectionClass != candidate.PIIProjectionClass ||
		attempt.AuthorizationAuditDigest != candidate.AuthorizationAuditDigest ||
		attempt.RenderInspectionDigest != candidate.RenderInspectionDigest || attempt.ReportSHA256 != candidate.ReportSHA256 ||
		attempt.ReportByteLength != candidate.ReportByteLength || attempt.MediaType != candidate.MediaType ||
		attempt.TargetIdentityDigest != candidate.TargetIdentityDigest ||
		attempt.ExpectedEvidenceBundleDigest != candidate.EvidenceAuthorityBundleDigest ||
		attempt.CandidatePublicationReceiptID != candidate.ReceiptID || attempt.CandidateRecordDigest != candidate.RecordDigest ||
		attempt.PublicationIndexDigest != index.IndexDigest || index.MutationID != attempt.PublicationIndexMutationID ||
		index.Generation != attempt.ExpectedPublicationCount+1 || index.PreviousIndexDigest != attempt.ExpectedPublicationIndexDigest ||
		attempt.IssuedAt != candidate.IssuedAt {
		return errors.New("publication attempt graph binding mismatch")
	}
	return nil
}

func publicationAttemptRecordDigestV1(attempt PublicationAttemptV1) string {
	attempt.RecordDigest = ""
	body, _ := json.Marshal(attempt)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), publicationAttemptRecordDigestDomainV1...), body...))
}

func publicationAttemptDerivedDigestV1(domain []byte, attemptID string) string {
	attemptID = strings.TrimSpace(attemptID)
	if !domainsecurity.IsSHA256Hex(attemptID) {
		return ""
	}
	return domainsecurity.SHA256Hex(append(append([]byte(nil), domain...), []byte(attemptID)...))
}

func validPublicationAttemptOpaqueV1(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 4096 && !strings.ContainsRune(value, '\x00')
}

func validPublicationAttemptPIIClassV1(value string) bool {
	return value == PIIProjectionOrdinaryMasked || value == PIIProjectionControlledFull
}
