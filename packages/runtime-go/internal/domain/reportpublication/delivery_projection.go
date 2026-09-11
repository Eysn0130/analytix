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
	ReportDeliveryProjectionSchemaVersion = 1
	ReportDeliveryProjectionPurpose       = "analytix.report-delivery-projection/v1"
	ReportDeliveryProjectionCompleted     = "projected"
)

var (
	reportDeliveryProjectionIDDomainV1        = []byte("analytix.report-delivery-projection/id/v1\x00")
	reportDeliveryProjectionSignatureDomainV1 = []byte("analytix.report-delivery-projection/signature/v1\x00")
	reportDeliveryProjectionRecordDomainV1    = []byte("analytix.report-delivery-projection/record/v1\x00")
)

// ReportDeliveryProjectionV1 is the private, signed delivery transition. It
// is created only from the complete report-stage graph. Copying a valid stage
// completion into the delivery CAS is not sufficient because that signature
// does not authorize a delivery transition.
type ReportDeliveryProjectionV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	DeliveryID    string `json:"deliveryId"`
	Status        string `json:"status"`

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

	ReportVariant            string `json:"reportVariant"`
	ClaimLedgerDigest        string `json:"claimLedgerDigest"`
	ClaimCount               uint64 `json:"claimCount"`
	PIIProjectionDigest      string `json:"piiProjectionDigest"`
	PIIProjectionClass       string `json:"piiProjectionClass"`
	AuthorizationAuditDigest string `json:"authorizationAuditDigest"`
	RenderInspectionDigest   string `json:"renderInspectionDigest"`
	ReportSHA256             string `json:"reportSha256"`
	ReportByteLength         uint64 `json:"reportByteLength"`
	MediaType                string `json:"mediaType"`
	TargetIdentityDigest     string `json:"targetIdentityDigest"`

	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RecordDigest       string `json:"recordDigest"`
}

type ReportDeliveryProjectionInputV1 struct {
	Decision           ReportDeliveryDecisionV1
	GrantSettlement    ReportGrantSettlementV1
	StageReceipt       domainpendingwork.PendingWorkReceiptV1
	StageDisposition   domainpendingwork.PendingWorkDispositionV1
	StageCompletion    ReportStageCompletionV1
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

// ReportDeliveryOutcomeIDV1 is the stable no-replace slot shared by a
// projected and rejected delivery outcome. Keep the original projection ID
// domain byte-for-byte so already persisted projections remain readable.
func ReportDeliveryOutcomeIDV1(installationID, enrollmentID, completionID string) string {
	body := append([]byte(nil), reportDeliveryProjectionIDDomainV1...)
	for _, value := range []string{installationID, enrollmentID, completionID} {
		body = append(body, strings.TrimSpace(value)...)
		body = append(body, 0)
	}
	return domainsecurity.SHA256Hex(body)
}

func ReportDeliveryProjectionIDV1(installationID, enrollmentID, completionID string) string {
	return ReportDeliveryOutcomeIDV1(installationID, enrollmentID, completionID)
}

func NewReportDeliveryProjectionV1(
	input ReportDeliveryProjectionInputV1,
	sign PublicationReceiptSignFuncV1,
) (ReportDeliveryProjectionV1, error) {
	projection, err := assembleReportDeliveryProjectionV1(input)
	if err != nil {
		return ReportDeliveryProjectionV1{}, err
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || projection.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ReportDeliveryProjectionV1{}, errors.New("report delivery projection signing authority is invalid")
	}
	signature, err := sign(ReportDeliveryProjectionSigningBytesV1(projection))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ReportDeliveryProjectionV1{}, errors.New("report delivery projection signing failed")
	}
	projection.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	projection.RecordDigest = reportDeliveryProjectionRecordDigestV1(projection)
	if err := ValidateReportDeliveryProjectionGraphV1(projection, input); err != nil {
		return ReportDeliveryProjectionV1{}, err
	}
	return projection, nil
}

func ValidateReportDeliveryProjectionV1(projection ReportDeliveryProjectionV1) error {
	if err := validateReportDeliveryProjectionUnsignedV1(projection); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(projection.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(projection.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != projection.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != projection.AuthoritySignature ||
		projection.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ReportDeliveryProjectionSigningBytesV1(projection), signature) ||
		!domainsecurity.IsSHA256Hex(projection.RecordDigest) || projection.RecordDigest != reportDeliveryProjectionRecordDigestV1(projection) ||
		projection.RecordDigest == projection.DeliveryID {
		return errors.New("report delivery projection signature or integrity is invalid")
	}
	return nil
}

func ValidateReportDeliveryProjectionGraphV1(
	projection ReportDeliveryProjectionV1,
	input ReportDeliveryProjectionInputV1,
) error {
	if ValidateReportDeliveryProjectionV1(projection) != nil {
		return errors.New("report delivery projection is invalid")
	}
	expected, err := assembleReportDeliveryProjectionV1(input)
	if err != nil {
		return err
	}
	expected.AuthoritySignature = projection.AuthoritySignature
	expected.RecordDigest = projection.RecordDigest
	if !reflect.DeepEqual(expected, projection) {
		return errors.New("report delivery projection does not match the completed report graph")
	}
	return nil
}

// ValidateReportDeliveryProjectionCompletionV1 is the adapter-side minimum:
// it proves that a projected delivery is the separately signed transition for
// the exact completion supplied by the app use case.
func ValidateReportDeliveryProjectionCompletionV1(
	projection ReportDeliveryProjectionV1,
	completion ReportStageCompletionV1,
) error {
	if ValidateReportDeliveryProjectionV1(projection) != nil || ValidateReportStageCompletionV1(completion) != nil ||
		projection.InstallationID != completion.InstallationID || projection.EnrollmentID != completion.EnrollmentID ||
		projection.ThreadID != completion.ThreadID || projection.TurnID != completion.TurnID ||
		projection.ContextDigest != completion.ContextDigest || projection.CaseBindingHash != completion.CaseBindingHash ||
		projection.ContextEpoch != completion.ContextEpoch || projection.DatasetSnapshotID != completion.DatasetSnapshotID ||
		projection.SourceManifestHash != completion.SourceManifestHash || projection.CompletionID != completion.CompletionID ||
		projection.CompletionRecordDigest != completion.RecordDigest || projection.CompletedAt != completion.DisposedAt ||
		projection.DecisionID != completion.DecisionID || projection.DecisionRecordDigest != completion.DecisionRecordDigest ||
		projection.CommitRecordDigest != completion.CommitRecordDigest || projection.ReportStageWorkID != completion.ReportStageWorkID ||
		projection.ReportStageReceiptID != completion.ReportStageReceiptID || projection.GrantSettlementID != completion.GrantSettlementID ||
		projection.ResultItemID != completion.ResultItemID || projection.ResultItemDigest != completion.ResultItemDigest ||
		projection.DispositionID != completion.DispositionID || projection.DispositionSHA256 != completion.DispositionSHA256 ||
		projection.AuthorityAlgorithm != completion.AuthorityAlgorithm || projection.AuthorityKeyID != completion.AuthorityKeyID ||
		projection.AuthorityPublicKey != completion.AuthorityPublicKey {
		return errors.New("report delivery projection does not bind the exact stage completion")
	}
	return nil
}

func ParseReportDeliveryProjectionV1(body []byte) (ReportDeliveryProjectionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 16, MaxTokens: 1024, MaxStringBytes: 128 << 10,
	}); err != nil {
		return ReportDeliveryProjectionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var projection ReportDeliveryProjectionV1
	if err := decoder.Decode(&projection); err != nil {
		return ReportDeliveryProjectionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReportDeliveryProjectionV1{}, errors.New("report delivery projection contains trailing JSON")
	}
	canonical, err := json.Marshal(projection)
	if err != nil || !bytes.Equal(canonical, body) {
		return ReportDeliveryProjectionV1{}, errors.New("report delivery projection is not canonically encoded")
	}
	return projection, ValidateReportDeliveryProjectionV1(projection)
}

func ReportDeliveryProjectionV1Bytes(projection ReportDeliveryProjectionV1) ([]byte, error) {
	if err := ValidateReportDeliveryProjectionV1(projection); err != nil {
		return nil, err
	}
	return json.Marshal(projection)
}

func ReportDeliveryProjectionSigningBytesV1(projection ReportDeliveryProjectionV1) []byte {
	projection.AuthoritySignature = ""
	projection.RecordDigest = ""
	body, _ := json.Marshal(projection)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), reportDeliveryProjectionSignatureDomainV1...), digest[:]...)
}

func ReportDeliveryProjectionAuthorityMaterialV1(projection ReportDeliveryProjectionV1) (string, []byte, []byte, error) {
	if err := ValidateReportDeliveryProjectionV1(projection); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(projection.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(projection.AuthoritySignature)
	return projection.AuthorityKeyID, publicKey, signature, nil
}

func assembleReportDeliveryProjectionV1(input ReportDeliveryProjectionInputV1) (ReportDeliveryProjectionV1, error) {
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
		return ReportDeliveryProjectionV1{}, errors.New("report delivery projection completed graph is invalid")
	}
	projection := ReportDeliveryProjectionV1{
		SchemaVersion: ReportDeliveryProjectionSchemaVersion, Purpose: ReportDeliveryProjectionPurpose,
		Status: ReportDeliveryProjectionCompleted, InstallationID: completion.InstallationID, EnrollmentID: completion.EnrollmentID,
		ThreadID: completion.ThreadID, TurnID: completion.TurnID, ContextDigest: completion.ContextDigest,
		CaseBindingHash: completion.CaseBindingHash, ContextEpoch: completion.ContextEpoch,
		DatasetSnapshotID: completion.DatasetSnapshotID, SourceManifestHash: completion.SourceManifestHash,
		CompletionID: completion.CompletionID, CompletionRecordDigest: completion.RecordDigest, CompletedAt: completion.DisposedAt,
		DecisionID: completion.DecisionID, DecisionRecordDigest: completion.DecisionRecordDigest, CommitRecordDigest: completion.CommitRecordDigest,
		ReportStageWorkID: completion.ReportStageWorkID, ReportStageReceiptID: completion.ReportStageReceiptID,
		GrantSettlementID: completion.GrantSettlementID, ResultItemID: completion.ResultItemID,
		ResultItemDigest: completion.ResultItemDigest, DispositionID: completion.DispositionID, DispositionSHA256: completion.DispositionSHA256,
		ReportVariant: decision.ReportVariant, ClaimLedgerDigest: decision.ClaimLedgerDigest, ClaimCount: decision.ClaimCount,
		PIIProjectionDigest: decision.PIIProjectionDigest, PIIProjectionClass: decision.PIIProjectionClass,
		AuthorizationAuditDigest: decision.AuthorizationAuditDigest, RenderInspectionDigest: decision.RenderInspectionDigest,
		ReportSHA256: decision.ReportSHA256, ReportByteLength: decision.ReportByteLength, MediaType: decision.MediaType,
		TargetIdentityDigest: decision.TargetIdentityDigest, AuthorityAlgorithm: PublicationReceiptAlgorithm,
		AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	projection.DeliveryID = ReportDeliveryProjectionIDV1(projection.InstallationID, projection.EnrollmentID, projection.CompletionID)
	if err := validateReportDeliveryProjectionUnsignedV1(projection); err != nil {
		return ReportDeliveryProjectionV1{}, err
	}
	return projection, nil
}

func validateReportDeliveryProjectionUnsignedV1(projection ReportDeliveryProjectionV1) error {
	completedAt, completedErr := time.Parse(time.RFC3339Nano, projection.CompletedAt)
	if projection.SchemaVersion != ReportDeliveryProjectionSchemaVersion || projection.Purpose != ReportDeliveryProjectionPurpose ||
		projection.Status != ReportDeliveryProjectionCompleted || !domainsecurity.IsSHA256Hex(projection.DeliveryID) ||
		projection.DeliveryID != ReportDeliveryProjectionIDV1(projection.InstallationID, projection.EnrollmentID, projection.CompletionID) ||
		!domainsecurity.IsSHA256Hex(projection.InstallationID) || !domainsecurity.IsSHA256Hex(projection.EnrollmentID) ||
		strings.TrimSpace(projection.ThreadID) == "" || strings.TrimSpace(projection.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(projection.ContextDigest) || !domainsecurity.IsSHA256Hex(projection.CaseBindingHash) ||
		projection.ContextEpoch == 0 || !domainsecurity.IsDatasetSnapshotIDV2Syntax(projection.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(projection.SourceManifestHash) || !domainsecurity.IsSHA256Hex(projection.CompletionID) ||
		!domainsecurity.IsSHA256Hex(projection.CompletionRecordDigest) || completedErr != nil || completedAt.IsZero() ||
		completedAt.UTC().Format(time.RFC3339Nano) != projection.CompletedAt || !domainsecurity.IsSHA256Hex(projection.DecisionID) ||
		!domainsecurity.IsSHA256Hex(projection.DecisionRecordDigest) || !domainsecurity.IsSHA256Hex(projection.CommitRecordDigest) ||
		!domainsecurity.IsSHA256Hex(projection.ReportStageWorkID) || !domainsecurity.IsSHA256Hex(projection.ReportStageReceiptID) ||
		!domainsecurity.IsSHA256Hex(projection.GrantSettlementID) || strings.TrimSpace(projection.ResultItemID) == "" ||
		!domainsecurity.IsSHA256Hex(projection.ResultItemDigest) || !domainsecurity.IsSHA256Hex(projection.DispositionID) ||
		!domainsecurity.IsSHA256Hex(projection.DispositionSHA256) || !validReportVariant(projection.ReportVariant) ||
		!domainsecurity.IsSHA256Hex(projection.ClaimLedgerDigest) ||
		(projection.ReportVariant != VerifiedNoHitReport && projection.ClaimCount == 0) ||
		!domainsecurity.IsSHA256Hex(projection.PIIProjectionDigest) || !domainsecurity.IsSHA256Hex(projection.RenderInspectionDigest) ||
		!domainsecurity.IsSHA256Hex(projection.ReportSHA256) || projection.ReportByteLength == 0 ||
		strings.TrimSpace(projection.MediaType) == "" || !domainsecurity.IsSHA256Hex(projection.TargetIdentityDigest) ||
		projection.AuthorityAlgorithm != PublicationReceiptAlgorithm || !domainsecurity.IsSHA256Hex(projection.AuthorityKeyID) {
		return errors.New("report delivery projection is incomplete")
	}
	if projection.PIIProjectionClass != PIIProjectionOrdinaryMasked && projection.PIIProjectionClass != PIIProjectionControlledFull {
		return errors.New("report delivery projection PII class is invalid")
	}
	if projection.PIIProjectionClass == PIIProjectionOrdinaryMasked && projection.AuthorizationAuditDigest != "" ||
		projection.PIIProjectionClass == PIIProjectionControlledFull && !domainsecurity.IsSHA256Hex(projection.AuthorizationAuditDigest) {
		return errors.New("report delivery projection PII authorization binding is invalid")
	}
	return nil
}

func reportDeliveryProjectionRecordDigestV1(projection ReportDeliveryProjectionV1) string {
	projection.RecordDigest = ""
	body, _ := json.Marshal(projection)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), reportDeliveryProjectionRecordDomainV1...), body...))
}
