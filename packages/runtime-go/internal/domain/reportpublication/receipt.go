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
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PublicationReceiptSchemaVersion = 1
	PublicationReceiptPurpose       = "analytix.publication-receipt/v1"
	PublicationReceiptAlgorithm     = "Ed25519"

	EvidenceBackedReport  = "evidence_backed_report"
	PartialEvidenceReport = "partial_evidence_report"
	VerifiedNoHitReport   = "verified_no_hit_report"
)

var publicationReceiptSignatureDomainV1 = []byte("analytix.publication-receipt/signature/v1\x00")

type PublicationReceiptV1 struct {
	SchemaVersion                 int    `json:"schemaVersion"`
	Purpose                       string `json:"purpose"`
	ReceiptID                     string `json:"receiptId"`
	InstallationID                string `json:"installationId"`
	EnrollmentID                  string `json:"enrollmentId"`
	ThreadID                      string `json:"threadId"`
	TurnID                        string `json:"turnId"`
	ContextDigest                 string `json:"contextDigest"`
	CaseID                        string `json:"caseId"`
	CaseBindingHash               string `json:"caseBindingHash"`
	ContextEpoch                  uint64 `json:"contextEpoch"`
	DatasetSnapshotID             string `json:"datasetSnapshotId"`
	SourceManifestHash            string `json:"sourceManifestHash"`
	ReportVariant                 string `json:"reportVariant"`
	EvidenceAuthorityBundleDigest string `json:"evidenceAuthorityBundleDigest"`
	EvidenceRegistryIndexDigest   string `json:"evidenceRegistryIndexDigest"`
	EvidenceRegistryCount         uint64 `json:"evidenceRegistryCount"`
	EvidenceRegistrySequence      uint64 `json:"evidenceRegistrySequence"`
	EvidenceRegistryStateDigest   string `json:"evidenceRegistryStateDigest"`
	EvidenceReceiptSetDigest      string `json:"evidenceReceiptSetDigest"`
	EvidenceReceiptCount          uint64 `json:"evidenceReceiptCount"`
	ClaimLedgerDigest             string `json:"claimLedgerDigest"`
	ClaimCount                    uint64 `json:"claimCount"`
	ReportSHA256                  string `json:"reportSha256"`
	ReportByteLength              uint64 `json:"reportByteLength"`
	MediaType                     string `json:"mediaType"`
	PIIProjectionDigest           string `json:"piiProjectionDigest"`
	PIIProjectionClass            string `json:"piiProjectionClass"`
	AuthorizationAuditDigest      string `json:"authorizationAuditDigest"`
	RenderInspectionDigest        string `json:"renderInspectionDigest"`
	Publisher                     string `json:"publisher"`
	PublisherVersion              string `json:"publisherVersion"`
	TargetIdentityDigest          string `json:"targetIdentityDigest"`
	IssuedAt                      string `json:"issuedAt"`
	AuthorityAlgorithm            string `json:"authorityAlgorithm"`
	AuthorityKeyID                string `json:"authorityKeyId"`
	AuthorityPublicKey            string `json:"authorityPublicKey"`
	AuthoritySignature            string `json:"authoritySignature"`
	RecordDigest                  string `json:"recordDigest"`
}

type PublicationReceiptInputV1 struct {
	InstallationID                string
	EnrollmentID                  string
	Context                       domainsecurity.TurnSecurityContext
	ReportVariant                 string
	EvidenceAuthorityBundleDigest string
	EvidenceRegistryIndexDigest   string
	EvidenceRegistryCount         uint64
	EvidenceRegistrySequence      uint64
	EvidenceRegistryStateDigest   string
	ClaimLedger                   ClaimLedgerV1
	ReportSHA256                  string
	ReportByteLength              uint64
	MediaType                     string
	PIIProjection                 PIIProjectionV1
	RenderInspection              RenderInspectionV1
	Publisher                     string
	PublisherVersion              string
	TargetIdentityDigest          string
	IssuedAt                      time.Time
	AuthorityKeyID                string
	AuthorityPublicKey            []byte
}

type PublicationReceiptSignFuncV1 func([]byte) ([]byte, error)

func NewPublicationReceiptV1(input PublicationReceiptInputV1, sign PublicationReceiptSignFuncV1) (PublicationReceiptV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		ValidateClaimLedgerV1(input.ClaimLedger) != nil || ValidatePIIProjectionV1(input.PIIProjection) != nil ||
		ValidateRenderInspectionV1(input.RenderInspection) != nil {
		return PublicationReceiptV1{}, errors.New("publication receipt input authority is invalid")
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	receipt := PublicationReceiptV1{
		SchemaVersion: PublicationReceiptSchemaVersion, Purpose: PublicationReceiptPurpose,
		InstallationID: strings.TrimSpace(input.InstallationID), EnrollmentID: strings.TrimSpace(input.EnrollmentID),
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, ContextDigest: input.Context.ContextDigest,
		CaseID: input.Context.CaseID, CaseBindingHash: input.Context.CaseBindingHash, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, SourceManifestHash: input.Context.SourceManifestHash,
		ReportVariant: strings.TrimSpace(input.ReportVariant), EvidenceAuthorityBundleDigest: strings.TrimSpace(input.EvidenceAuthorityBundleDigest),
		EvidenceRegistryIndexDigest: strings.TrimSpace(input.EvidenceRegistryIndexDigest), EvidenceRegistryCount: input.EvidenceRegistryCount,
		EvidenceRegistrySequence: input.EvidenceRegistrySequence, EvidenceRegistryStateDigest: strings.TrimSpace(input.EvidenceRegistryStateDigest),
		EvidenceReceiptSetDigest: EvidenceReceiptSetDigestV1(input.ClaimLedger.EvidenceReceiptIDs),
		EvidenceReceiptCount:     uint64(len(input.ClaimLedger.EvidenceReceiptIDs)), ClaimLedgerDigest: input.ClaimLedger.LedgerDigest,
		ClaimCount: uint64(len(input.ClaimLedger.Claims)), ReportSHA256: strings.TrimSpace(input.ReportSHA256),
		ReportByteLength: input.ReportByteLength, MediaType: strings.TrimSpace(input.MediaType),
		PIIProjectionDigest: input.PIIProjection.ProjectionDigest, PIIProjectionClass: input.PIIProjection.ProjectionClass,
		AuthorizationAuditDigest: input.PIIProjection.AuthorizationAuditDigest,
		RenderInspectionDigest:   input.RenderInspection.InspectionDigest,
		Publisher:                strings.TrimSpace(input.Publisher), PublisherVersion: strings.TrimSpace(input.PublisherVersion),
		TargetIdentityDigest: strings.TrimSpace(input.TargetIdentityDigest), IssuedAt: issuedAt.Format(time.RFC3339Nano),
		AuthorityAlgorithm: PublicationReceiptAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.ReceiptID = publicationReceiptIDV1(receipt)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return PublicationReceiptV1{}, errors.New("publication receipt signing authority is invalid")
	}
	if err := validatePublicationReceiptUnsignedV1(receipt); err != nil {
		return PublicationReceiptV1{}, err
	}
	if err := validatePublicationReceiptInputsV1(receipt, input.ClaimLedger, input.PIIProjection, input.RenderInspection); err != nil {
		return PublicationReceiptV1{}, err
	}
	signature, err := sign(PublicationReceiptSigningBytesV1(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PublicationReceiptV1{}, errors.New("publication receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.RecordDigest = publicationReceiptRecordDigestV1(receipt)
	if err := ValidatePublicationReceiptV1(receipt); err != nil {
		return PublicationReceiptV1{}, err
	}
	return receipt, nil
}

func ValidatePublicationReceiptV1(receipt PublicationReceiptV1) error {
	if err := validatePublicationReceiptUnsignedV1(receipt); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != receipt.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != receipt.AuthoritySignature ||
		receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PublicationReceiptSigningBytesV1(receipt), signature) {
		return errors.New("publication receipt signature is invalid")
	}
	if !domainsecurity.IsSHA256Hex(receipt.RecordDigest) || receipt.RecordDigest != publicationReceiptRecordDigestV1(receipt) ||
		receipt.RecordDigest == receipt.ReceiptID {
		return errors.New("publication receipt record digest is invalid")
	}
	return nil
}

func ValidatePublicationReceiptForInstallationV1(receipt PublicationReceiptV1, installationID, enrollmentID, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidatePublicationReceiptV1(receipt); err != nil {
		return err
	}
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if receipt.InstallationID != strings.TrimSpace(installationID) || receipt.EnrollmentID != strings.TrimSpace(enrollmentID) ||
		len(authorityPublicKey) != ed25519.PublicKeySize || strings.TrimSpace(authorityKeyID) != domainsecurity.SHA256Hex(authorityPublicKey) ||
		receipt.AuthorityKeyID != strings.TrimSpace(authorityKeyID) || receipt.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("publication receipt installation anchor mismatch")
	}
	return nil
}

func ValidatePublicationReceiptMaterialsV1(receipt PublicationReceiptV1, ledger ClaimLedgerV1, projection PIIProjectionV1, inspection RenderInspectionV1) error {
	if err := ValidatePublicationReceiptV1(receipt); err != nil {
		return err
	}
	return validatePublicationReceiptInputsV1(receipt, ledger, projection, inspection)
}

func ValidatePublicationIndexReceiptV1(index PublicationIndexV1, receipt PublicationReceiptV1) error {
	if ValidatePublicationIndexV1(index) != nil || ValidatePublicationReceiptV1(receipt) != nil ||
		index.InstallationID != receipt.InstallationID || index.EnrollmentID != receipt.EnrollmentID ||
		index.ReceiptID != receipt.ReceiptID || index.ReceiptRecordDigest != receipt.RecordDigest ||
		index.TargetIdentityDigest != receipt.TargetIdentityDigest || index.AuthorityKeyID != receipt.AuthorityKeyID ||
		index.AuthorityPublicKey != receipt.AuthorityPublicKey {
		return errors.New("publication index does not match publication receipt")
	}
	return nil
}

func ParsePublicationReceiptV1(body []byte) (PublicationReceiptV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 4, MaxTokens: 256, MaxStringBytes: 64 << 10,
	}); err != nil {
		return PublicationReceiptV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var receipt PublicationReceiptV1
	if err := decoder.Decode(&receipt); err != nil {
		return PublicationReceiptV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PublicationReceiptV1{}, errors.New("publication receipt contains trailing JSON")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal(body, canonical) {
		return PublicationReceiptV1{}, errors.New("publication receipt is not canonically encoded")
	}
	return receipt, ValidatePublicationReceiptV1(receipt)
}

func PublicationReceiptV1Bytes(receipt PublicationReceiptV1) ([]byte, error) {
	if err := ValidatePublicationReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func PublicationReceiptSigningBytesV1(receipt PublicationReceiptV1) []byte {
	receipt.AuthoritySignature = ""
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), publicationReceiptSignatureDomainV1...)
	return append(out, digest[:]...)
}

func EvidenceReceiptSetDigestV1(receiptIDs []string) string {
	canonical := canonicalEvidenceReceiptIDs(receiptIDs)
	if len(canonical) == 0 || len(canonical) != len(receiptIDs) {
		return ""
	}
	body, _ := json.Marshal(canonical)
	return domainsecurity.SHA256Hex(append([]byte("analytix.evidence-receipt-set/digest/v1\x00"), body...))
}

func validatePublicationReceiptUnsignedV1(receipt PublicationReceiptV1) error {
	issuedAt, timeErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if receipt.SchemaVersion != PublicationReceiptSchemaVersion || receipt.Purpose != PublicationReceiptPurpose ||
		!domainsecurity.IsSHA256Hex(receipt.ReceiptID) || receipt.ReceiptID != publicationReceiptIDV1(receipt) ||
		!domainsecurity.IsSHA256Hex(receipt.InstallationID) || !domainsecurity.IsSHA256Hex(receipt.EnrollmentID) ||
		strings.TrimSpace(receipt.ThreadID) == "" || strings.TrimSpace(receipt.TurnID) == "" || !domainsecurity.IsSHA256Hex(receipt.ContextDigest) ||
		strings.TrimSpace(receipt.CaseID) == "" || receipt.CaseID == domainsecurity.UnboundCaseID || !domainsecurity.IsSHA256Hex(receipt.CaseBindingHash) ||
		receipt.ContextEpoch == 0 || !domainsecurity.IsDatasetSnapshotIDV2Syntax(receipt.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(receipt.SourceManifestHash) || !validReportVariant(receipt.ReportVariant) ||
		!domainsecurity.IsSHA256Hex(receipt.EvidenceAuthorityBundleDigest) || !domainsecurity.IsSHA256Hex(receipt.EvidenceRegistryIndexDigest) ||
		receipt.EvidenceRegistryCount == 0 || receipt.EvidenceRegistrySequence == 0 || !domainsecurity.IsSHA256Hex(receipt.EvidenceRegistryStateDigest) ||
		!domainsecurity.IsSHA256Hex(receipt.EvidenceReceiptSetDigest) || receipt.EvidenceReceiptCount == 0 ||
		!domainsecurity.IsSHA256Hex(receipt.ClaimLedgerDigest) || !domainsecurity.IsSHA256Hex(receipt.ReportSHA256) || receipt.ReportByteLength == 0 ||
		strings.TrimSpace(receipt.MediaType) == "" || !domainsecurity.IsSHA256Hex(receipt.PIIProjectionDigest) ||
		(receipt.PIIProjectionClass != PIIProjectionOrdinaryMasked && receipt.PIIProjectionClass != PIIProjectionControlledFull) ||
		!domainsecurity.IsSHA256Hex(receipt.RenderInspectionDigest) || strings.TrimSpace(receipt.Publisher) == "" ||
		strings.TrimSpace(receipt.PublisherVersion) == "" || !domainsecurity.IsSHA256Hex(receipt.TargetIdentityDigest) ||
		timeErr != nil || issuedAt.IsZero() || issuedAt.UTC().Format(time.RFC3339Nano) != receipt.IssuedAt ||
		receipt.AuthorityAlgorithm != PublicationReceiptAlgorithm || !domainsecurity.IsSHA256Hex(receipt.AuthorityKeyID) {
		return errors.New("publication receipt is incomplete")
	}
	if receipt.PIIProjectionClass == PIIProjectionOrdinaryMasked && receipt.AuthorizationAuditDigest != "" ||
		receipt.PIIProjectionClass == PIIProjectionControlledFull && !domainsecurity.IsSHA256Hex(receipt.AuthorizationAuditDigest) {
		return errors.New("publication receipt PII authority is invalid")
	}
	return nil
}

func validatePublicationReceiptInputsV1(receipt PublicationReceiptV1, ledger ClaimLedgerV1, projection PIIProjectionV1, inspection RenderInspectionV1) error {
	if ValidateClaimLedgerV1(ledger) != nil || ValidatePIIProjectionV1(projection) != nil || ValidateRenderInspectionV1(inspection) != nil ||
		ledger.ThreadID != receipt.ThreadID || ledger.TurnID != receipt.TurnID || ledger.ContextDigest != receipt.ContextDigest ||
		ledger.CaseID != receipt.CaseID || ledger.CaseBindingHash != receipt.CaseBindingHash || ledger.ContextEpoch != receipt.ContextEpoch ||
		ledger.DatasetSnapshotID != receipt.DatasetSnapshotID || ledger.SourceManifestHash != receipt.SourceManifestHash ||
		receipt.EvidenceReceiptSetDigest != EvidenceReceiptSetDigestV1(ledger.EvidenceReceiptIDs) ||
		receipt.EvidenceReceiptCount != uint64(len(ledger.EvidenceReceiptIDs)) || receipt.ClaimLedgerDigest != ledger.LedgerDigest ||
		receipt.ClaimCount != uint64(len(ledger.Claims)) || receipt.PIIProjectionDigest != projection.ProjectionDigest ||
		receipt.PIIProjectionClass != projection.ProjectionClass || receipt.AuthorizationAuditDigest != projection.AuthorizationAuditDigest ||
		projection.ProjectedContentSHA256 != receipt.ReportSHA256 || receipt.RenderInspectionDigest != inspection.InspectionDigest ||
		!inspection.Passed || inspection.ReportSHA256 != receipt.ReportSHA256 || inspection.ReportByteLength != receipt.ReportByteLength ||
		inspection.MediaType != receipt.MediaType {
		return errors.New("publication receipt materials do not match")
	}
	switch receipt.ReportVariant {
	case EvidenceBackedReport:
		if ledger.VerifiedClaimCount == 0 || ledger.PartialClaimCount != 0 {
			return errors.New("evidence-backed report claim coverage is invalid")
		}
	case PartialEvidenceReport:
		if ledger.PartialClaimCount == 0 {
			return errors.New("partial report lacks partial claim authority")
		}
	case VerifiedNoHitReport:
		if ledger.VerifiedClaimCount != 0 || ledger.PartialClaimCount != 0 {
			return errors.New("verified no-hit report contains positive or partial claims")
		}
	}
	return nil
}

func validReportVariant(value string) bool {
	switch value {
	case EvidenceBackedReport, PartialEvidenceReport, VerifiedNoHitReport:
		return true
	default:
		return false
	}
}

func publicationReceiptIDV1(receipt PublicationReceiptV1) string {
	receipt.ReceiptID = ""
	receipt.AuthoritySignature = ""
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(append([]byte("analytix.publication-receipt/id/v1\x00"), body...))
}

func publicationReceiptRecordDigestV1(receipt PublicationReceiptV1) string {
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(append([]byte("analytix.publication-receipt/record/v1\x00"), body...))
}
