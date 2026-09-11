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
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const (
	ReportGrantSettlementSchemaVersion = 1
	ReportGrantSettlementPurpose       = "analytix.report-grant-settlement/v1"
)

var (
	reportGrantSettlementIDDomainV1        = []byte("analytix.report-grant-settlement/id/v1\x00")
	reportGrantSettlementSignatureDomainV1 = []byte("analytix.report-grant-settlement/signature/v1\x00")
	reportGrantSettlementRecordDomainV1    = []byte("analytix.report-grant-settlement/record/v1\x00")
)

// ReportGrantSettlementV1 proves that the exact report-stage execution grant
// bound by a delivery decision has one durable tool_result and an exact
// active-to-settled registry transition. It contains no report bytes, claim
// payloads, raw PII, provider output, or reasoning.
type ReportGrantSettlementV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	SettlementID  string `json:"settlementId"`

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
	ReportStageWorkID    string `json:"reportStageWorkId"`
	ReportStageReceiptID string `json:"reportStageReceiptId"`

	GrantID    string `json:"grantId"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`

	ResultItemID     string `json:"resultItemId"`
	ResultItemDigest string `json:"resultItemDigest"`

	ActiveRegistrySequence     uint64 `json:"activeRegistrySequence"`
	ActiveRegistryStateDigest  string `json:"activeRegistryStateDigest"`
	SettledRegistrySequence    uint64 `json:"settledRegistrySequence"`
	SettledRegistryStateDigest string `json:"settledRegistryStateDigest"`
	SettledAt                  string `json:"settledAt"`

	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RecordDigest       string `json:"recordDigest"`
}

type ReportGrantSettlementInputV1 struct {
	Decision           ReportDeliveryDecisionV1
	Grant              domainsecurity.ExecutionGrant
	ActiveRegistry     domainsecurity.ExecutionGrantRegistry
	SettledRegistry    domainsecurity.ExecutionGrantRegistry
	ResultItemID       string
	ResultItemDigest   string
	SettledAt          time.Time
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

func ReportGrantSettlementIDV1(installationID, enrollmentID, decisionID, grantID, resultItemID string) string {
	body := append([]byte(nil), reportGrantSettlementIDDomainV1...)
	for _, value := range []string{installationID, enrollmentID, decisionID, grantID, resultItemID} {
		body = append(body, strings.TrimSpace(value)...)
		body = append(body, 0)
	}
	return domainsecurity.SHA256Hex(body)
}

func NewReportGrantSettlementV1(
	input ReportGrantSettlementInputV1,
	sign PublicationReceiptSignFuncV1,
) (ReportGrantSettlementV1, error) {
	settlement, err := assembleReportGrantSettlementV1(input)
	if err != nil {
		return ReportGrantSettlementV1{}, err
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize ||
		settlement.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement signing authority is invalid")
	}
	signature, err := sign(ReportGrantSettlementSigningBytesV1(settlement))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement signing failed")
	}
	settlement.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	settlement.RecordDigest = reportGrantSettlementRecordDigestV1(settlement)
	if err := ValidateReportGrantSettlementGraphV1(settlement, input); err != nil {
		return ReportGrantSettlementV1{}, err
	}
	return settlement, nil
}

func ValidateReportGrantSettlementV1(settlement ReportGrantSettlementV1) error {
	if err := validateReportGrantSettlementUnsignedV1(settlement); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(settlement.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(settlement.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize || base64.RawURLEncoding.EncodeToString(publicKey) != settlement.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != settlement.AuthoritySignature ||
		settlement.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ReportGrantSettlementSigningBytesV1(settlement), signature) ||
		!domainsecurity.IsSHA256Hex(settlement.RecordDigest) ||
		settlement.RecordDigest != reportGrantSettlementRecordDigestV1(settlement) ||
		settlement.RecordDigest == settlement.SettlementID {
		return errors.New("report grant settlement signature or integrity is invalid")
	}
	return nil
}

func ValidateReportGrantSettlementGraphV1(
	settlement ReportGrantSettlementV1,
	input ReportGrantSettlementInputV1,
) error {
	if ValidateReportGrantSettlementV1(settlement) != nil {
		return errors.New("report grant settlement is invalid")
	}
	expected, err := assembleReportGrantSettlementV1(input)
	if err != nil {
		return err
	}
	expected.AuthoritySignature = settlement.AuthoritySignature
	expected.RecordDigest = settlement.RecordDigest
	if !reflect.DeepEqual(expected, settlement) {
		return errors.New("report grant settlement does not match exact decision and registry transition")
	}
	return nil
}

// ValidateReportGrantSettlementDecisionV1 rebinds a durable settlement to its
// admitted decision without claiming that the thread registry transition has
// been replayed. Restart must additionally reconstruct and validate that
// transition before stage completion or delivery.
func ValidateReportGrantSettlementDecisionV1(
	settlement ReportGrantSettlementV1,
	decision ReportDeliveryDecisionV1,
) error {
	if ValidateReportGrantSettlementV1(settlement) != nil || ValidateReportDeliveryDecisionV1(decision) != nil ||
		settlement.InstallationID != decision.InstallationID || settlement.EnrollmentID != decision.EnrollmentID ||
		settlement.ThreadID != decision.Context.ThreadID || settlement.TurnID != decision.Context.TurnID ||
		settlement.ContextDigest != decision.Context.ContextDigest || settlement.CaseBindingHash != decision.Context.CaseBindingHash ||
		settlement.ContextEpoch != decision.Context.ContextEpoch || settlement.DatasetSnapshotID != decision.Context.DatasetSnapshotID ||
		settlement.SourceManifestHash != decision.Context.SourceManifestHash || settlement.DecisionID != decision.DecisionID ||
		settlement.DecisionRecordDigest != decision.RecordDigest || settlement.CommitRecordDigest != decision.CommitRecordDigest ||
		settlement.ReportStageWorkID != decision.ReportStageWorkID || settlement.ReportStageReceiptID != decision.ReportStageReceiptID ||
		settlement.GrantID != decision.GrantID || settlement.ToolCallID != decision.ToolCallID || settlement.ToolName != decision.ToolName ||
		settlement.ActiveRegistrySequence != decision.GrantRegistrySequence ||
		settlement.ActiveRegistryStateDigest != decision.GrantRegistryDigest ||
		settlement.AuthorityAlgorithm != decision.AuthorityAlgorithm || settlement.AuthorityKeyID != decision.AuthorityKeyID ||
		settlement.AuthorityPublicKey != decision.AuthorityPublicKey {
		return errors.New("report grant settlement does not bind the admitted decision")
	}
	return nil
}

func ParseReportGrantSettlementV1(body []byte) (ReportGrantSettlementV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 16, MaxTokens: 512, MaxStringBytes: 128 << 10,
	}); err != nil {
		return ReportGrantSettlementV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var settlement ReportGrantSettlementV1
	if err := decoder.Decode(&settlement); err != nil {
		return ReportGrantSettlementV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement contains trailing JSON")
	}
	canonical, err := json.Marshal(settlement)
	if err != nil || !bytes.Equal(canonical, body) {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement is not canonically encoded")
	}
	return settlement, ValidateReportGrantSettlementV1(settlement)
}

func ReportGrantSettlementV1Bytes(settlement ReportGrantSettlementV1) ([]byte, error) {
	if err := ValidateReportGrantSettlementV1(settlement); err != nil {
		return nil, err
	}
	return json.Marshal(settlement)
}

func ReportGrantSettlementSigningBytesV1(settlement ReportGrantSettlementV1) []byte {
	settlement.AuthoritySignature = ""
	settlement.RecordDigest = ""
	body, _ := json.Marshal(settlement)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), reportGrantSettlementSignatureDomainV1...), digest[:]...)
}

func ReportGrantSettlementAuthorityMaterialV1(
	settlement ReportGrantSettlementV1,
) (string, []byte, []byte, error) {
	if err := ValidateReportGrantSettlementV1(settlement); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(settlement.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(settlement.AuthoritySignature)
	return settlement.AuthorityKeyID, publicKey, signature, nil
}

func assembleReportGrantSettlementV1(input ReportGrantSettlementInputV1) (ReportGrantSettlementV1, error) {
	decision := input.Decision
	grant := input.Grant
	active := input.ActiveRegistry
	settled := input.SettledRegistry
	settledAt := input.SettledAt.UTC()
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if ValidateReportDeliveryDecisionV1(decision) != nil {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement decision is invalid")
	}
	if domainsecurity.ValidateExecutionGrantForContext(grant, decision.Context) != nil ||
		grant.GrantID != decision.GrantID || grant.ToolCallID != decision.ToolCallID ||
		grant.ToolName != decision.ToolName || grant.ToolName != "stage_case_report" || grant.ReadOnly || grant.ApprovalState != "approved" {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement execution grant is invalid")
	}
	if err := domainsecurity.ValidateExecutionGrantRegistry(active); err != nil {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement active registry is invalid")
	}
	if err := domainsecurity.ValidateExecutionGrantRegistry(settled); err != nil {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement settled registry is invalid")
	}
	if settledAt.IsZero() || active.ThreadID != decision.Context.ThreadID || settled.ThreadID != decision.Context.ThreadID {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement registry context is invalid")
	}
	if active.Sequence != settled.Sequence || active.StateDigest == settled.StateDigest {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement registry state transition is invalid")
	}
	activeEntry, found := domainsecurity.ExecutionGrantRegistryEntryByID(active, grant.GrantID)
	if !found || active.Sequence != decision.GrantRegistrySequence ||
		active.StateDigest != decision.GrantRegistryDigest ||
		activeEntry.EntryDigest != decision.GrantRegistryEntryDigest {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement registry does not match the signed report stage")
	}
	if domainsecurity.VerifyExecutionGrantMembership(
		active, decision.Context.ThreadID, decision.Context.TurnID, grant, domainsecurity.GrantRegistryActive,
	) != nil || domainsecurity.VerifyExecutionGrantMembership(
		settled, decision.Context.ThreadID, decision.Context.TurnID, grant, domainsecurity.GrantRegistrySettled,
	) != nil {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement registry membership is invalid")
	}
	if !domainmodel.IsHostToolCallIDV1(grant.ToolCallID) ||
		input.ResultItemID != domaintoolresult.ToolResultItemIDV1(decision.Context.TurnID, grant.ToolCallID) ||
		!domainsecurity.IsSHA256Hex(input.ResultItemDigest) {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement result authority is invalid")
	}
	if len(publicKey) != ed25519.PublicKeySize || strings.TrimSpace(input.AuthorityKeyID) != domainsecurity.SHA256Hex(publicKey) ||
		decision.AuthorityKeyID != strings.TrimSpace(input.AuthorityKeyID) ||
		decision.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(publicKey) {
		return ReportGrantSettlementV1{}, errors.New("report grant settlement authority is invalid")
	}
	settlement := ReportGrantSettlementV1{
		SchemaVersion: ReportGrantSettlementSchemaVersion, Purpose: ReportGrantSettlementPurpose,
		InstallationID: decision.InstallationID, EnrollmentID: decision.EnrollmentID,
		ThreadID: decision.Context.ThreadID, TurnID: decision.Context.TurnID,
		ContextDigest: decision.Context.ContextDigest, CaseBindingHash: decision.Context.CaseBindingHash,
		ContextEpoch: decision.Context.ContextEpoch, DatasetSnapshotID: decision.Context.DatasetSnapshotID,
		SourceManifestHash: decision.Context.SourceManifestHash,
		DecisionID:         decision.DecisionID, DecisionRecordDigest: decision.RecordDigest,
		CommitRecordDigest: decision.CommitRecordDigest, ReportStageWorkID: decision.ReportStageWorkID,
		ReportStageReceiptID: decision.ReportStageReceiptID,
		GrantID:              grant.GrantID, ToolCallID: grant.ToolCallID, ToolName: grant.ToolName,
		ResultItemID: input.ResultItemID, ResultItemDigest: input.ResultItemDigest,
		ActiveRegistrySequence: active.Sequence, ActiveRegistryStateDigest: active.StateDigest,
		SettledRegistrySequence: settled.Sequence, SettledRegistryStateDigest: settled.StateDigest,
		SettledAt: settledAt.Format(time.RFC3339Nano), AuthorityAlgorithm: PublicationReceiptAlgorithm,
		AuthorityKeyID:     strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	settlement.SettlementID = ReportGrantSettlementIDV1(
		settlement.InstallationID, settlement.EnrollmentID, settlement.DecisionID, settlement.GrantID, settlement.ResultItemID,
	)
	if err := validateReportGrantSettlementUnsignedV1(settlement); err != nil {
		return ReportGrantSettlementV1{}, err
	}
	return settlement, nil
}

func validateReportGrantSettlementUnsignedV1(settlement ReportGrantSettlementV1) error {
	settledAt, settledAtErr := time.Parse(time.RFC3339Nano, settlement.SettledAt)
	if settlement.SchemaVersion != ReportGrantSettlementSchemaVersion || settlement.Purpose != ReportGrantSettlementPurpose ||
		!domainsecurity.IsSHA256Hex(settlement.SettlementID) ||
		settlement.SettlementID != ReportGrantSettlementIDV1(
			settlement.InstallationID, settlement.EnrollmentID, settlement.DecisionID, settlement.GrantID, settlement.ResultItemID,
		) || !domainsecurity.IsSHA256Hex(settlement.InstallationID) || !domainsecurity.IsSHA256Hex(settlement.EnrollmentID) ||
		strings.TrimSpace(settlement.ThreadID) == "" || strings.TrimSpace(settlement.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(settlement.ContextDigest) || !domainsecurity.IsSHA256Hex(settlement.CaseBindingHash) ||
		settlement.ContextEpoch == 0 || !domainsecurity.IsDatasetSnapshotIDV2Syntax(settlement.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(settlement.SourceManifestHash) || !domainsecurity.IsSHA256Hex(settlement.DecisionID) ||
		!domainsecurity.IsSHA256Hex(settlement.DecisionRecordDigest) || !domainsecurity.IsSHA256Hex(settlement.CommitRecordDigest) ||
		strings.TrimSpace(settlement.ReportStageWorkID) == "" || !domainsecurity.IsSHA256Hex(settlement.ReportStageReceiptID) ||
		!domainsecurity.IsSHA256Hex(settlement.GrantID) || !domainmodel.IsHostToolCallIDV1(settlement.ToolCallID) ||
		settlement.ToolName != "stage_case_report" ||
		settlement.ResultItemID != domaintoolresult.ToolResultItemIDV1(settlement.TurnID, settlement.ToolCallID) ||
		!domainsecurity.IsSHA256Hex(settlement.ResultItemDigest) || settlement.ActiveRegistrySequence == 0 ||
		settlement.ActiveRegistrySequence != settlement.SettledRegistrySequence ||
		!domainsecurity.IsSHA256Hex(settlement.ActiveRegistryStateDigest) ||
		!domainsecurity.IsSHA256Hex(settlement.SettledRegistryStateDigest) ||
		settlement.ActiveRegistryStateDigest == settlement.SettledRegistryStateDigest || settledAtErr != nil || settledAt.IsZero() ||
		settledAt.UTC().Format(time.RFC3339Nano) != settlement.SettledAt ||
		settlement.AuthorityAlgorithm != PublicationReceiptAlgorithm || !domainsecurity.IsSHA256Hex(settlement.AuthorityKeyID) {
		return errors.New("report grant settlement is incomplete")
	}
	return nil
}

func reportGrantSettlementRecordDigestV1(settlement ReportGrantSettlementV1) string {
	settlement.RecordDigest = ""
	body, _ := json.Marshal(settlement)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), reportGrantSettlementRecordDomainV1...), body...))
}
