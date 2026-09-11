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
	ReportStageCompletionSchemaVersion = 1
	ReportStageCompletionPurpose       = "analytix.report-stage-completion/v1"
	ReportStageCompletionCompleted     = "completed"
)

var (
	reportStageCompletionIDDomainV1        = []byte("analytix.report-stage-completion/id/v1\x00")
	reportStageCompletionSignatureDomainV1 = []byte("analytix.report-stage-completion/signature/v1\x00")
	reportStageCompletionRecordDomainV1    = []byte("analytix.report-stage-completion/record/v1\x00")
)

// ReportStageCompletionV1 is the private bridge from an admitted report's
// exact durable grant settlement to the exact signed completed report-stage
// disposition. Neither a generic disposition nor a grant settlement alone
// authorizes user-visible delivery.
type ReportStageCompletionV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	CompletionID  string `json:"completionId"`
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

	DecisionID           string `json:"decisionId"`
	DecisionRecordDigest string `json:"decisionRecordDigest"`
	CommitRecordDigest   string `json:"commitRecordDigest"`

	ReportStageWorkID        string `json:"reportStageWorkId"`
	ReportStageReceiptID     string `json:"reportStageReceiptId"`
	ReportStageReceiptSHA256 string `json:"reportStageReceiptSha256"`

	GrantSettlementID           string `json:"grantSettlementId"`
	GrantSettlementRecordDigest string `json:"grantSettlementRecordDigest"`
	GrantID                     string `json:"grantId"`
	ToolCallID                  string `json:"toolCallId"`
	ToolName                    string `json:"toolName"`
	ResultItemID                string `json:"resultItemId"`
	ResultItemDigest            string `json:"resultItemDigest"`

	DispositionID     string `json:"dispositionId"`
	DispositionSHA256 string `json:"dispositionSha256"`
	DispositionStatus string `json:"dispositionStatus"`
	DispositionReason string `json:"dispositionReason"`
	DisposedAt        string `json:"disposedAt"`

	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RecordDigest       string `json:"recordDigest"`
}

type ReportStageCompletionInputV1 struct {
	Decision           ReportDeliveryDecisionV1
	GrantSettlement    ReportGrantSettlementV1
	StageReceipt       domainpendingwork.PendingWorkReceiptV1
	StageDisposition   domainpendingwork.PendingWorkDispositionV1
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

func ReportStageCompletionIDV1(installationID, enrollmentID, decisionID string) string {
	body := append([]byte(nil), reportStageCompletionIDDomainV1...)
	for _, value := range []string{installationID, enrollmentID, decisionID} {
		body = append(body, strings.TrimSpace(value)...)
		body = append(body, 0)
	}
	return domainsecurity.SHA256Hex(body)
}

func NewReportStageCompletionV1(
	input ReportStageCompletionInputV1,
	sign PublicationReceiptSignFuncV1,
) (ReportStageCompletionV1, error) {
	completion, err := assembleReportStageCompletionV1(input)
	if err != nil {
		return ReportStageCompletionV1{}, err
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize ||
		completion.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ReportStageCompletionV1{}, errors.New("report stage completion signing authority is invalid")
	}
	signature, err := sign(ReportStageCompletionSigningBytesV1(completion))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ReportStageCompletionV1{}, errors.New("report stage completion signing failed")
	}
	completion.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	completion.RecordDigest = reportStageCompletionRecordDigestV1(completion)
	if err := ValidateReportStageCompletionGraphV1(completion, input); err != nil {
		return ReportStageCompletionV1{}, err
	}
	return completion, nil
}

func ValidateReportStageCompletionV1(completion ReportStageCompletionV1) error {
	if err := validateReportStageCompletionUnsignedV1(completion); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(completion.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(completion.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize || base64.RawURLEncoding.EncodeToString(publicKey) != completion.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != completion.AuthoritySignature ||
		completion.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ReportStageCompletionSigningBytesV1(completion), signature) ||
		!domainsecurity.IsSHA256Hex(completion.RecordDigest) ||
		completion.RecordDigest != reportStageCompletionRecordDigestV1(completion) || completion.RecordDigest == completion.CompletionID {
		return errors.New("report stage completion signature or integrity is invalid")
	}
	return nil
}

func ValidateReportStageCompletionGraphV1(
	completion ReportStageCompletionV1,
	input ReportStageCompletionInputV1,
) error {
	if ValidateReportStageCompletionV1(completion) != nil {
		return errors.New("report stage completion is invalid")
	}
	expected, err := assembleReportStageCompletionV1(input)
	if err != nil {
		return err
	}
	expected.AuthoritySignature = completion.AuthoritySignature
	expected.RecordDigest = completion.RecordDigest
	if !reflect.DeepEqual(expected, completion) {
		return errors.New("report stage completion does not match the exact decision, settlement, receipt, and disposition")
	}
	return nil
}

// ValidateReportStageCompletionDecisionSettlementV1 proves the report-owner
// portion of the graph. Restart must still resolve and validate the exact
// pending-work receipt and disposition before projection.
func ValidateReportStageCompletionDecisionSettlementV1(
	completion ReportStageCompletionV1,
	decision ReportDeliveryDecisionV1,
	settlement ReportGrantSettlementV1,
) error {
	if ValidateReportStageCompletionV1(completion) != nil ||
		ValidateReportGrantSettlementDecisionV1(settlement, decision) != nil ||
		completion.InstallationID != decision.InstallationID || completion.EnrollmentID != decision.EnrollmentID ||
		completion.ThreadID != decision.Context.ThreadID || completion.TurnID != decision.Context.TurnID ||
		completion.ContextDigest != decision.Context.ContextDigest || completion.CaseBindingHash != decision.Context.CaseBindingHash ||
		completion.ContextEpoch != decision.Context.ContextEpoch || completion.DatasetSnapshotID != decision.Context.DatasetSnapshotID ||
		completion.SourceManifestHash != decision.Context.SourceManifestHash ||
		completion.DecisionID != decision.DecisionID || completion.DecisionRecordDigest != decision.RecordDigest ||
		completion.CommitRecordDigest != decision.CommitRecordDigest || completion.ReportStageWorkID != decision.ReportStageWorkID ||
		completion.ReportStageReceiptID != decision.ReportStageReceiptID ||
		completion.ReportStageReceiptSHA256 != decision.ReportStageReceiptSHA256 ||
		completion.GrantSettlementID != settlement.SettlementID ||
		completion.GrantSettlementRecordDigest != settlement.RecordDigest || completion.GrantID != settlement.GrantID ||
		completion.ToolCallID != settlement.ToolCallID || completion.ToolName != settlement.ToolName ||
		completion.ResultItemID != settlement.ResultItemID || completion.ResultItemDigest != settlement.ResultItemDigest ||
		completion.DisposedAt != settlement.SettledAt || completion.AuthorityAlgorithm != decision.AuthorityAlgorithm ||
		completion.AuthorityKeyID != decision.AuthorityKeyID || completion.AuthorityPublicKey != decision.AuthorityPublicKey {
		return errors.New("report stage completion does not bind the admitted decision and grant settlement")
	}
	return nil
}

func ParseReportStageCompletionV1(body []byte) (ReportStageCompletionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 16, MaxTokens: 512, MaxStringBytes: 128 << 10,
	}); err != nil {
		return ReportStageCompletionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var completion ReportStageCompletionV1
	if err := decoder.Decode(&completion); err != nil {
		return ReportStageCompletionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReportStageCompletionV1{}, errors.New("report stage completion contains trailing JSON")
	}
	canonical, err := json.Marshal(completion)
	if err != nil || !bytes.Equal(canonical, body) {
		return ReportStageCompletionV1{}, errors.New("report stage completion is not canonically encoded")
	}
	return completion, ValidateReportStageCompletionV1(completion)
}

func ReportStageCompletionV1Bytes(completion ReportStageCompletionV1) ([]byte, error) {
	if err := ValidateReportStageCompletionV1(completion); err != nil {
		return nil, err
	}
	return json.Marshal(completion)
}

func ReportStageCompletionSigningBytesV1(completion ReportStageCompletionV1) []byte {
	completion.AuthoritySignature = ""
	completion.RecordDigest = ""
	body, _ := json.Marshal(completion)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), reportStageCompletionSignatureDomainV1...), digest[:]...)
}

func ReportStageCompletionAuthorityMaterialV1(
	completion ReportStageCompletionV1,
) (string, []byte, []byte, error) {
	if err := ValidateReportStageCompletionV1(completion); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(completion.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(completion.AuthoritySignature)
	return completion.AuthorityKeyID, publicKey, signature, nil
}

func assembleReportStageCompletionV1(input ReportStageCompletionInputV1) (ReportStageCompletionV1, error) {
	decision := input.Decision
	settlement := input.GrantSettlement
	receipt := input.StageReceipt
	disposition := input.StageDisposition
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if ValidateReportGrantSettlementDecisionV1(settlement, decision) != nil ||
		domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, receipt) != nil {
		return ReportStageCompletionV1{}, errors.New("report stage completion input authority is invalid")
	}
	receiptBody, receiptErr := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	dispositionBody, dispositionErr := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	context := decision.Context
	if receiptErr != nil || dispositionErr != nil || receipt.Kind != domainpendingwork.KindReportStage ||
		len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].ResultItemID != "" || receipt.GrantMembers[0].ResultItemDigest != "" ||
		receipt.Context.ThreadID != context.ThreadID || receipt.Context.TurnID != context.TurnID ||
		receipt.Context.ContextDigest != context.ContextDigest || receipt.Context.CaseBindingHash != context.CaseBindingHash ||
		receipt.Context.ContextEpoch != context.ContextEpoch || receipt.Context.DatasetSnapshotID != context.DatasetSnapshotID ||
		receipt.Context.SourceManifestHash != context.SourceManifestHash ||
		receipt.WorkID != decision.ReportStageWorkID || receipt.ReceiptID != decision.ReportStageReceiptID ||
		domainsecurity.SHA256Hex(receiptBody) != decision.ReportStageReceiptSHA256 ||
		receipt.GrantRegistrySequence != decision.GrantRegistrySequence || receipt.GrantRegistryDigest != decision.GrantRegistryDigest ||
		receipt.GrantMembers[0].RegistryEntryDigest != decision.GrantRegistryEntryDigest ||
		receipt.GrantMembers[0].GrantID != decision.GrantID ||
		disposition.Status != domainpendingwork.StatusCompleted || disposition.ReasonCode != "report_stage_completed" ||
		disposition.DisposedAt != settlement.SettledAt {
		return ReportStageCompletionV1{}, errors.New("report stage completion does not bind the exact completed report stage")
	}
	if receipt.AuthorityAlgorithm != decision.AuthorityAlgorithm || receipt.AuthorityKeyID != decision.AuthorityKeyID ||
		receipt.AuthorityPublicKey != decision.AuthorityPublicKey || disposition.AuthorityAlgorithm != decision.AuthorityAlgorithm ||
		disposition.AuthorityKeyID != decision.AuthorityKeyID || disposition.AuthorityPublicKey != decision.AuthorityPublicKey ||
		len(publicKey) != ed25519.PublicKeySize || strings.TrimSpace(input.AuthorityKeyID) != domainsecurity.SHA256Hex(publicKey) ||
		decision.AuthorityKeyID != strings.TrimSpace(input.AuthorityKeyID) ||
		decision.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(publicKey) {
		return ReportStageCompletionV1{}, errors.New("report stage completion installation authority is invalid")
	}
	settledAt, settledErr := time.Parse(time.RFC3339Nano, settlement.SettledAt)
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	receiptIssuedAt, issuedErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	receiptExpiresAt, expiresErr := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
	if settledErr != nil || disposedErr != nil || issuedErr != nil || expiresErr != nil ||
		!settledAt.Equal(disposedAt) || settledAt.Before(receiptIssuedAt) || !settledAt.Before(receiptExpiresAt) {
		return ReportStageCompletionV1{}, errors.New("report stage completion time is invalid")
	}
	completion := ReportStageCompletionV1{
		SchemaVersion: ReportStageCompletionSchemaVersion, Purpose: ReportStageCompletionPurpose,
		Status: ReportStageCompletionCompleted, InstallationID: decision.InstallationID, EnrollmentID: decision.EnrollmentID,
		ThreadID: context.ThreadID, TurnID: context.TurnID, ContextDigest: context.ContextDigest,
		CaseBindingHash: context.CaseBindingHash, ContextEpoch: context.ContextEpoch,
		DatasetSnapshotID: context.DatasetSnapshotID, SourceManifestHash: context.SourceManifestHash,
		DecisionID: decision.DecisionID, DecisionRecordDigest: decision.RecordDigest, CommitRecordDigest: decision.CommitRecordDigest,
		ReportStageWorkID: decision.ReportStageWorkID, ReportStageReceiptID: decision.ReportStageReceiptID,
		ReportStageReceiptSHA256: decision.ReportStageReceiptSHA256,
		GrantSettlementID:        settlement.SettlementID, GrantSettlementRecordDigest: settlement.RecordDigest,
		GrantID: settlement.GrantID, ToolCallID: settlement.ToolCallID, ToolName: settlement.ToolName,
		ResultItemID: settlement.ResultItemID, ResultItemDigest: settlement.ResultItemDigest,
		DispositionID: disposition.DispositionID, DispositionSHA256: domainsecurity.SHA256Hex(dispositionBody),
		DispositionStatus: disposition.Status, DispositionReason: disposition.ReasonCode, DisposedAt: disposition.DisposedAt,
		AuthorityAlgorithm: PublicationReceiptAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	completion.CompletionID = ReportStageCompletionIDV1(completion.InstallationID, completion.EnrollmentID, completion.DecisionID)
	if err := validateReportStageCompletionUnsignedV1(completion); err != nil {
		return ReportStageCompletionV1{}, err
	}
	return completion, nil
}

func validateReportStageCompletionUnsignedV1(completion ReportStageCompletionV1) error {
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, completion.DisposedAt)
	if completion.SchemaVersion != ReportStageCompletionSchemaVersion || completion.Purpose != ReportStageCompletionPurpose ||
		completion.Status != ReportStageCompletionCompleted || !domainsecurity.IsSHA256Hex(completion.CompletionID) ||
		completion.CompletionID != ReportStageCompletionIDV1(completion.InstallationID, completion.EnrollmentID, completion.DecisionID) ||
		!domainsecurity.IsSHA256Hex(completion.InstallationID) || !domainsecurity.IsSHA256Hex(completion.EnrollmentID) ||
		strings.TrimSpace(completion.ThreadID) == "" || strings.TrimSpace(completion.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(completion.ContextDigest) || !domainsecurity.IsSHA256Hex(completion.CaseBindingHash) ||
		completion.ContextEpoch == 0 || !domainsecurity.IsDatasetSnapshotIDV2Syntax(completion.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(completion.SourceManifestHash) || !domainsecurity.IsSHA256Hex(completion.DecisionID) ||
		!domainsecurity.IsSHA256Hex(completion.DecisionRecordDigest) || !domainsecurity.IsSHA256Hex(completion.CommitRecordDigest) ||
		!domainsecurity.IsSHA256Hex(completion.ReportStageWorkID) || !domainsecurity.IsSHA256Hex(completion.ReportStageReceiptID) ||
		!domainsecurity.IsSHA256Hex(completion.ReportStageReceiptSHA256) || !domainsecurity.IsSHA256Hex(completion.GrantSettlementID) ||
		!domainsecurity.IsSHA256Hex(completion.GrantSettlementRecordDigest) || !domainsecurity.IsSHA256Hex(completion.GrantID) ||
		strings.TrimSpace(completion.ToolCallID) == "" || completion.ToolName != "stage_case_report" ||
		strings.TrimSpace(completion.ResultItemID) == "" || !domainsecurity.IsSHA256Hex(completion.ResultItemDigest) ||
		!domainsecurity.IsSHA256Hex(completion.DispositionID) || !domainsecurity.IsSHA256Hex(completion.DispositionSHA256) ||
		completion.DispositionStatus != domainpendingwork.StatusCompleted || completion.DispositionReason != "report_stage_completed" ||
		disposedErr != nil || disposedAt.IsZero() || disposedAt.UTC().Format(time.RFC3339Nano) != completion.DisposedAt ||
		completion.AuthorityAlgorithm != PublicationReceiptAlgorithm || !domainsecurity.IsSHA256Hex(completion.AuthorityKeyID) {
		return errors.New("report stage completion is incomplete")
	}
	return nil
}

func reportStageCompletionRecordDigestV1(completion ReportStageCompletionV1) string {
	completion.RecordDigest = ""
	body, _ := json.Marshal(completion)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), reportStageCompletionRecordDomainV1...), body...))
}
