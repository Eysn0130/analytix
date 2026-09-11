package reportpublication

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ReportDeliveryRejectionSchemaVersion = 1
	ReportDeliveryRejectionPurpose       = "analytix.report-delivery-rejection/v1"
	ReportDeliveryRejectionStatus        = "rejected"
)

type ReportDeliveryRejectionReasonV1 string

const (
	ReportDeliveryRejectionEvidenceChangedV1 ReportDeliveryRejectionReasonV1 = "evidence_changed"
)

var (
	reportDeliveryRejectionSignatureDomainV1 = []byte("analytix.report-delivery-rejection/signature/v1\x00")
	reportDeliveryRejectionRecordDomainV1    = []byte("analytix.report-delivery-rejection/record/v1\x00")
)

// ReportDeliveryRejectionV1 is a private, signed terminal transition for a
// completed report stage that the final host gate permanently refused to
// expose. It contains only authority bindings and hashes: no report content,
// raw PII, provider output, workspace path, or free-form error text.
type ReportDeliveryRejectionV1 struct {
	SchemaVersion int                             `json:"schemaVersion"`
	Purpose       string                          `json:"purpose"`
	DeliveryID    string                          `json:"deliveryId"`
	Status        string                          `json:"status"`
	ReasonCode    ReportDeliveryRejectionReasonV1 `json:"reasonCode"`

	InstallationID string `json:"installationId"`
	EnrollmentID   string `json:"enrollmentId"`

	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	ContextDigest      string `json:"contextDigest"`
	CaseBindingHash    string `json:"caseBindingHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`

	CompletionID           string `json:"completionId"`
	CompletionRecordDigest string `json:"completionRecordDigest"`
	CompletedAt            string `json:"completedAt"`

	DecisionID           string `json:"decisionId"`
	DecisionRecordDigest string `json:"decisionRecordDigest"`
	CommitRecordDigest   string `json:"commitRecordDigest"`

	ReportStageWorkID    string `json:"reportStageWorkId"`
	ReportStageReceiptID string `json:"reportStageReceiptId"`
	GrantSettlementID    string `json:"grantSettlementId"`
	ResultItemID         string `json:"resultItemId"`
	ResultItemDigest     string `json:"resultItemDigest"`
	DispositionID        string `json:"dispositionId"`
	DispositionSHA256    string `json:"dispositionSha256"`

	WitnessObservationDigest      string `json:"witnessObservationDigest,omitempty"`
	EvidenceAuthorityBundleDigest string `json:"evidenceAuthorityBundleDigest,omitempty"`
	EvidenceRegistryIndexDigest   string `json:"evidenceRegistryIndexDigest,omitempty"`
	EvidenceRegistrySequence      uint64 `json:"evidenceRegistrySequence,omitempty"`
	EvidenceRegistryStateDigest   string `json:"evidenceRegistryStateDigest,omitempty"`

	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RecordDigest       string `json:"recordDigest"`
}

type ReportDeliveryRejectionInputV1 struct {
	Decision         ReportDeliveryDecisionV1
	GrantSettlement  ReportGrantSettlementV1
	StageReceipt     domainpendingwork.PendingWorkReceiptV1
	StageDisposition domainpendingwork.PendingWorkDispositionV1
	StageCompletion  ReportStageCompletionV1
	ReasonCode       ReportDeliveryRejectionReasonV1

	WitnessObservationDigest      string
	EvidenceAuthorityBundleDigest string
	EvidenceRegistryIndexDigest   string
	EvidenceRegistrySequence      uint64
	EvidenceRegistryStateDigest   string

	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

func NewReportDeliveryRejectionV1(
	input ReportDeliveryRejectionInputV1,
	sign PublicationReceiptSignFuncV1,
) (ReportDeliveryRejectionV1, error) {
	rejection, err := assembleReportDeliveryRejectionV1(input)
	if err != nil {
		return ReportDeliveryRejectionV1{}, err
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || rejection.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ReportDeliveryRejectionV1{}, errors.New("report delivery rejection signing authority is invalid")
	}
	signature, err := sign(ReportDeliveryRejectionSigningBytesV1(rejection))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ReportDeliveryRejectionV1{}, errors.New("report delivery rejection signing failed")
	}
	rejection.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	rejection.RecordDigest = reportDeliveryRejectionRecordDigestV1(rejection)
	if err := ValidateReportDeliveryRejectionGraphV1(rejection, input); err != nil {
		return ReportDeliveryRejectionV1{}, err
	}
	return rejection, nil
}

func ValidateReportDeliveryRejectionV1(rejection ReportDeliveryRejectionV1) error {
	if err := validateReportDeliveryRejectionUnsignedV1(rejection); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(rejection.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(rejection.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != rejection.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != rejection.AuthoritySignature ||
		rejection.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ReportDeliveryRejectionSigningBytesV1(rejection), signature) ||
		!domainsecurity.IsSHA256Hex(rejection.RecordDigest) || rejection.RecordDigest != reportDeliveryRejectionRecordDigestV1(rejection) ||
		rejection.RecordDigest == rejection.DeliveryID {
		return errors.New("report delivery rejection signature or integrity is invalid")
	}
	return nil
}

func ValidateReportDeliveryRejectionGraphV1(
	rejection ReportDeliveryRejectionV1,
	input ReportDeliveryRejectionInputV1,
) error {
	if ValidateReportDeliveryRejectionV1(rejection) != nil {
		return errors.New("report delivery rejection is invalid")
	}
	expected, err := assembleReportDeliveryRejectionV1(input)
	if err != nil {
		return err
	}
	expected.AuthoritySignature = rejection.AuthoritySignature
	expected.RecordDigest = rejection.RecordDigest
	if !reflect.DeepEqual(expected, rejection) {
		return errors.New("report delivery rejection does not match the completed report graph")
	}
	return nil
}

// ValidateReportDeliveryRejectionCompletionV1 is the outbound adapter's
// minimum authority check. Full reason proof validation remains in the app
// use case that owns the witnessed snapshot and context status observation.
func ValidateReportDeliveryRejectionCompletionV1(
	rejection ReportDeliveryRejectionV1,
	completion ReportStageCompletionV1,
) error {
	if ValidateReportDeliveryRejectionV1(rejection) != nil || ValidateReportStageCompletionV1(completion) != nil ||
		rejection.InstallationID != completion.InstallationID || rejection.EnrollmentID != completion.EnrollmentID ||
		rejection.ThreadID != completion.ThreadID || rejection.TurnID != completion.TurnID ||
		rejection.ContextDigest != completion.ContextDigest || rejection.CaseBindingHash != completion.CaseBindingHash ||
		rejection.ContextEpoch != completion.ContextEpoch || rejection.DatasetSnapshotID != completion.DatasetSnapshotID ||
		rejection.SourceManifestHash != completion.SourceManifestHash || rejection.CompletionID != completion.CompletionID ||
		rejection.CompletionRecordDigest != completion.RecordDigest || rejection.CompletedAt != completion.DisposedAt ||
		rejection.DecisionID != completion.DecisionID || rejection.DecisionRecordDigest != completion.DecisionRecordDigest ||
		rejection.CommitRecordDigest != completion.CommitRecordDigest || rejection.ReportStageWorkID != completion.ReportStageWorkID ||
		rejection.ReportStageReceiptID != completion.ReportStageReceiptID || rejection.GrantSettlementID != completion.GrantSettlementID ||
		rejection.ResultItemID != completion.ResultItemID || rejection.ResultItemDigest != completion.ResultItemDigest ||
		rejection.DispositionID != completion.DispositionID || rejection.DispositionSHA256 != completion.DispositionSHA256 ||
		rejection.AuthorityAlgorithm != completion.AuthorityAlgorithm || rejection.AuthorityKeyID != completion.AuthorityKeyID ||
		rejection.AuthorityPublicKey != completion.AuthorityPublicKey {
		return errors.New("report delivery rejection does not bind the exact stage completion")
	}
	return nil
}

func ParseReportDeliveryRejectionV1(body []byte) (ReportDeliveryRejectionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 16, MaxTokens: 1024, MaxStringBytes: 128 << 10,
	}); err != nil {
		return ReportDeliveryRejectionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var rejection ReportDeliveryRejectionV1
	if err := decoder.Decode(&rejection); err != nil {
		return ReportDeliveryRejectionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReportDeliveryRejectionV1{}, errors.New("report delivery rejection contains trailing JSON")
	}
	canonical, err := json.Marshal(rejection)
	if err != nil || !bytes.Equal(canonical, body) {
		return ReportDeliveryRejectionV1{}, errors.New("report delivery rejection is not canonically encoded")
	}
	return rejection, ValidateReportDeliveryRejectionV1(rejection)
}

func ReportDeliveryRejectionV1Bytes(rejection ReportDeliveryRejectionV1) ([]byte, error) {
	if err := ValidateReportDeliveryRejectionV1(rejection); err != nil {
		return nil, err
	}
	return json.Marshal(rejection)
}

func ReportDeliveryRejectionSigningBytesV1(rejection ReportDeliveryRejectionV1) []byte {
	rejection.AuthoritySignature = ""
	rejection.RecordDigest = ""
	body, _ := json.Marshal(rejection)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), reportDeliveryRejectionSignatureDomainV1...), digest[:]...)
}

func ReportDeliveryRejectionAuthorityMaterialV1(rejection ReportDeliveryRejectionV1) (string, []byte, []byte, error) {
	if err := ValidateReportDeliveryRejectionV1(rejection); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(rejection.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(rejection.AuthoritySignature)
	return rejection.AuthorityKeyID, publicKey, signature, nil
}

func assembleReportDeliveryRejectionV1(input ReportDeliveryRejectionInputV1) (ReportDeliveryRejectionV1, error) {
	decision := input.Decision
	completion := input.StageCompletion
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	completionInput := ReportStageCompletionInputV1{
		Decision: decision, GrantSettlement: input.GrantSettlement,
		StageReceipt: input.StageReceipt, StageDisposition: input.StageDisposition,
		AuthorityKeyID: input.AuthorityKeyID, AuthorityPublicKey: publicKey,
	}
	if ValidateReportDeliveryDecisionV1(decision) != nil ||
		ValidateReportStageCompletionGraphV1(completion, completionInput) != nil ||
		len(publicKey) != ed25519.PublicKeySize || strings.TrimSpace(input.AuthorityKeyID) != domainsecurity.SHA256Hex(publicKey) ||
		decision.AuthorityKeyID != strings.TrimSpace(input.AuthorityKeyID) ||
		decision.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(publicKey) {
		return ReportDeliveryRejectionV1{}, errors.New("report delivery rejection completed graph is invalid")
	}
	rejection := ReportDeliveryRejectionV1{
		SchemaVersion: ReportDeliveryRejectionSchemaVersion, Purpose: ReportDeliveryRejectionPurpose,
		Status: ReportDeliveryRejectionStatus, ReasonCode: input.ReasonCode,
		InstallationID: completion.InstallationID, EnrollmentID: completion.EnrollmentID,
		ThreadID: completion.ThreadID, TurnID: completion.TurnID, ContextDigest: completion.ContextDigest,
		CaseBindingHash: completion.CaseBindingHash, ContextEpoch: completion.ContextEpoch,
		DatasetSnapshotID: completion.DatasetSnapshotID, SourceManifestHash: completion.SourceManifestHash,
		CompletionID: completion.CompletionID, CompletionRecordDigest: completion.RecordDigest, CompletedAt: completion.DisposedAt,
		DecisionID: completion.DecisionID, DecisionRecordDigest: completion.DecisionRecordDigest, CommitRecordDigest: completion.CommitRecordDigest,
		ReportStageWorkID: completion.ReportStageWorkID, ReportStageReceiptID: completion.ReportStageReceiptID,
		GrantSettlementID: completion.GrantSettlementID, ResultItemID: completion.ResultItemID,
		ResultItemDigest: completion.ResultItemDigest, DispositionID: completion.DispositionID, DispositionSHA256: completion.DispositionSHA256,
		WitnessObservationDigest:      strings.TrimSpace(input.WitnessObservationDigest),
		EvidenceAuthorityBundleDigest: strings.TrimSpace(input.EvidenceAuthorityBundleDigest),
		EvidenceRegistryIndexDigest:   strings.TrimSpace(input.EvidenceRegistryIndexDigest),
		EvidenceRegistrySequence:      input.EvidenceRegistrySequence,
		EvidenceRegistryStateDigest:   strings.TrimSpace(input.EvidenceRegistryStateDigest),
		AuthorityAlgorithm:            PublicationReceiptAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	rejection.DeliveryID = ReportDeliveryOutcomeIDV1(rejection.InstallationID, rejection.EnrollmentID, rejection.CompletionID)
	if err := validateReportDeliveryRejectionUnsignedV1(rejection); err != nil {
		return ReportDeliveryRejectionV1{}, err
	}
	if rejection.ReasonCode == ReportDeliveryRejectionEvidenceChangedV1 &&
		(rejection.EvidenceRegistrySequence <= decision.EvidenceRegistrySequence ||
			rejection.EvidenceRegistryStateDigest == decision.EvidenceRegistryStateDigest) {
		return ReportDeliveryRejectionV1{}, errors.New("report delivery rejection lacks changed evidence authority")
	}
	return rejection, nil
}

func validateReportDeliveryRejectionUnsignedV1(rejection ReportDeliveryRejectionV1) error {
	completedAt, completedErr := time.Parse(time.RFC3339Nano, rejection.CompletedAt)
	if rejection.SchemaVersion != ReportDeliveryRejectionSchemaVersion || rejection.Purpose != ReportDeliveryRejectionPurpose ||
		rejection.Status != ReportDeliveryRejectionStatus || !domainsecurity.IsSHA256Hex(rejection.DeliveryID) ||
		rejection.DeliveryID != ReportDeliveryOutcomeIDV1(rejection.InstallationID, rejection.EnrollmentID, rejection.CompletionID) ||
		!domainsecurity.IsSHA256Hex(rejection.InstallationID) || !domainsecurity.IsSHA256Hex(rejection.EnrollmentID) ||
		strings.TrimSpace(rejection.ThreadID) == "" || strings.TrimSpace(rejection.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(rejection.ContextDigest) || !domainsecurity.IsSHA256Hex(rejection.CaseBindingHash) ||
		rejection.ContextEpoch == 0 || !domainsecurity.IsDatasetSnapshotIDV2Syntax(rejection.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(rejection.SourceManifestHash) || !domainsecurity.IsSHA256Hex(rejection.CompletionID) ||
		!domainsecurity.IsSHA256Hex(rejection.CompletionRecordDigest) || completedErr != nil || completedAt.IsZero() ||
		completedAt.UTC().Format(time.RFC3339Nano) != rejection.CompletedAt || !domainsecurity.IsSHA256Hex(rejection.DecisionID) ||
		!domainsecurity.IsSHA256Hex(rejection.DecisionRecordDigest) || !domainsecurity.IsSHA256Hex(rejection.CommitRecordDigest) ||
		!domainsecurity.IsSHA256Hex(rejection.ReportStageWorkID) || !domainsecurity.IsSHA256Hex(rejection.ReportStageReceiptID) ||
		!domainsecurity.IsSHA256Hex(rejection.GrantSettlementID) || strings.TrimSpace(rejection.ResultItemID) == "" ||
		!domainsecurity.IsSHA256Hex(rejection.ResultItemDigest) || !domainsecurity.IsSHA256Hex(rejection.DispositionID) ||
		!domainsecurity.IsSHA256Hex(rejection.DispositionSHA256) || rejection.AuthorityAlgorithm != PublicationReceiptAlgorithm ||
		!domainsecurity.IsSHA256Hex(rejection.AuthorityKeyID) {
		return errors.New("report delivery rejection is incomplete")
	}
	switch rejection.ReasonCode {
	case ReportDeliveryRejectionEvidenceChangedV1:
		if !domainsecurity.IsSHA256Hex(rejection.WitnessObservationDigest) ||
			!domainsecurity.IsSHA256Hex(rejection.EvidenceAuthorityBundleDigest) ||
			!domainsecurity.IsSHA256Hex(rejection.EvidenceRegistryIndexDigest) || rejection.EvidenceRegistrySequence == 0 ||
			!domainsecurity.IsSHA256Hex(rejection.EvidenceRegistryStateDigest) {
			return errors.New("report delivery rejection evidence proof is invalid")
		}
	default:
		return errors.New("report delivery rejection reason is invalid")
	}
	return nil
}

func reportDeliveryRejectionRecordDigestV1(rejection ReportDeliveryRejectionV1) string {
	rejection.RecordDigest = ""
	body, _ := json.Marshal(rejection)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), reportDeliveryRejectionRecordDomainV1...), body...))
}
