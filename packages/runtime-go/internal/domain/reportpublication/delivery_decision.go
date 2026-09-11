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

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ReportDeliveryDecisionSchemaVersion = 1
	ReportDeliveryDecisionPurpose       = "analytix.report-delivery-decision/v1"
	ReportDeliveryDecisionAdmitted      = "admitted"
)

var (
	reportDeliveryDecisionIDDomainV1        = []byte("analytix.report-delivery-decision/id/v1\x00")
	reportDeliveryDecisionSignatureDomainV1 = []byte("analytix.report-delivery-decision/signature/v1\x00")
	reportDeliveryDecisionRecordDomainV1    = []byte("analytix.report-delivery-decision/record/v1\x00")
)

// ReportDeliveryDecisionV1 is the only durable authority that may expose a
// formal report. A publication commit proves witness selection, but cannot by
// itself authorize delivery. The decision additionally binds the exact frozen
// context, report-stage grant, witnessed evidence state, claim/PII projection,
// renderer inspection, and immutable artifact. Raw report bytes and raw PII
// are deliberately excluded.
type ReportDeliveryDecisionV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	DecisionID    string `json:"decisionId"`
	Status        string `json:"status"`

	InstallationID string                             `json:"installationId"`
	EnrollmentID   string                             `json:"enrollmentId"`
	Context        domainsecurity.TurnSecurityContext `json:"context"`

	AttemptID           string `json:"attemptId"`
	AttemptRecordDigest string `json:"attemptRecordDigest"`
	StageInputHash      string `json:"stageInputHash"`

	ReportStageWorkID        string `json:"reportStageWorkId"`
	ReportStageReceiptID     string `json:"reportStageReceiptId"`
	ReportStageReceiptSHA256 string `json:"reportStageReceiptSha256"`

	GrantRegistrySequence    uint64 `json:"grantRegistrySequence"`
	GrantRegistryDigest      string `json:"grantRegistryDigest"`
	GrantRegistryEntryDigest string `json:"grantRegistryEntryDigest"`
	GrantID                  string `json:"grantId"`
	ToolCallID               string `json:"toolCallId"`
	ToolName                 string `json:"toolName"`

	CandidateReceiptID     string `json:"candidateReceiptId"`
	CandidateRecordDigest  string `json:"candidateRecordDigest"`
	PublicationIndexDigest string `json:"publicationIndexDigest"`

	CommitSelectionID                string `json:"commitSelectionId"`
	CommitSelectionRecordDigest      string `json:"commitSelectionRecordDigest"`
	AuthorityAdvanceSettlementDigest string `json:"authorityAdvanceSettlementDigest"`
	CommitReceiptID                  string `json:"commitReceiptId"`
	CommitRecordDigest               string `json:"commitRecordDigest"`

	PreviousEvidenceAuthorityBundleDigest  string `json:"previousEvidenceAuthorityBundleDigest"`
	CommittedEvidenceAuthorityBundleDigest string `json:"committedEvidenceAuthorityBundleDigest"`
	EvidenceRegistryIndexDigest            string `json:"evidenceRegistryIndexDigest"`
	EvidenceRegistryCount                  uint64 `json:"evidenceRegistryCount"`
	EvidenceRegistrySequence               uint64 `json:"evidenceRegistrySequence"`
	EvidenceRegistryStateDigest            string `json:"evidenceRegistryStateDigest"`
	EvidenceReceiptSetDigest               string `json:"evidenceReceiptSetDigest"`
	EvidenceReceiptCount                   uint64 `json:"evidenceReceiptCount"`

	ReportVariant            string `json:"reportVariant"`
	ClaimLedgerDigest        string `json:"claimLedgerDigest"`
	ClaimCount               uint64 `json:"claimCount"`
	PIIProjectionDigest      string `json:"piiProjectionDigest"`
	PIIProjectionClass       string `json:"piiProjectionClass"`
	AuthorizationAuditDigest string `json:"authorizationAuditDigest"`
	RenderInspectionDigest   string `json:"renderInspectionDigest"`

	ReportSHA256         string `json:"reportSha256"`
	ReportByteLength     uint64 `json:"reportByteLength"`
	MediaType            string `json:"mediaType"`
	TargetIdentityDigest string `json:"targetIdentityDigest"`

	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
	RecordDigest       string `json:"recordDigest"`
}

type ReportDeliveryDecisionInputV1 struct {
	Context      domainsecurity.TurnSecurityContext
	StageReceipt domainpendingwork.PendingWorkReceiptV1
	Attempt      PublicationAttemptV1
	Candidate    PublicationReceiptV1
	Index        PublicationIndexV1
	Selection    PublicationCommitSelectionV1
	Commit       PublicationCommitReceiptV1
	Ledger       ClaimLedgerV1
	Projection   PIIProjectionV1
	Inspection   RenderInspectionV1

	CommitInput                      PublicationCommitReceiptInputV1
	AuthorityAdvanceSettlementDigest string

	WitnessedEvidenceAuthorityBundleDigest string
	WitnessedEvidenceRegistryIndexDigest   string
	EvidenceRegistryCount                  uint64
	EvidenceRegistrySequence               uint64
	EvidenceRegistryStateDigest            string
}

func ReportDeliveryDecisionIDV1(installationID, enrollmentID, attemptID string) string {
	body := append([]byte(nil), reportDeliveryDecisionIDDomainV1...)
	body = append(body, strings.TrimSpace(installationID)...)
	body = append(body, 0)
	body = append(body, strings.TrimSpace(enrollmentID)...)
	body = append(body, 0)
	body = append(body, strings.TrimSpace(attemptID)...)
	return domainsecurity.SHA256Hex(body)
}

func NewReportDeliveryDecisionV1(
	input ReportDeliveryDecisionInputV1,
	sign PublicationReceiptSignFuncV1,
) (ReportDeliveryDecisionV1, error) {
	decision, err := newReportDeliveryDecisionV1WithoutSigning(input)
	if err != nil {
		return ReportDeliveryDecisionV1{}, err
	}
	publicKey := append([]byte(nil), input.CommitInput.AuthorityPublicKey...)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize ||
		decision.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ReportDeliveryDecisionV1{}, errors.New("report delivery decision signing authority is invalid")
	}
	signature, err := sign(ReportDeliveryDecisionSigningBytesV1(decision))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ReportDeliveryDecisionV1{}, errors.New("report delivery decision signing failed")
	}
	decision.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	decision.RecordDigest = reportDeliveryDecisionRecordDigestV1(decision)
	if err := ValidateReportDeliveryDecisionGraphV1(decision, input); err != nil {
		return ReportDeliveryDecisionV1{}, err
	}
	return decision, nil
}

func ValidateReportDeliveryDecisionV1(decision ReportDeliveryDecisionV1) error {
	if err := validateReportDeliveryDecisionUnsignedV1(decision); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(decision.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(decision.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != decision.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != decision.AuthoritySignature ||
		decision.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ReportDeliveryDecisionSigningBytesV1(decision), signature) ||
		!domainsecurity.IsSHA256Hex(decision.RecordDigest) ||
		decision.RecordDigest != reportDeliveryDecisionRecordDigestV1(decision) || decision.RecordDigest == decision.DecisionID {
		return errors.New("report delivery decision signature or integrity is invalid")
	}
	return nil
}

func ValidateReportDeliveryDecisionGraphV1(
	decision ReportDeliveryDecisionV1,
	input ReportDeliveryDecisionInputV1,
) error {
	if err := ValidateReportDeliveryDecisionV1(decision); err != nil {
		return err
	}
	if err := validateReportDeliveryDecisionInputGraphV1(input); err != nil {
		return err
	}
	expected := assembleReportDeliveryDecisionV1(input)
	expected.AuthoritySignature = decision.AuthoritySignature
	expected.RecordDigest = decision.RecordDigest
	if !reflect.DeepEqual(expected, decision) {
		return errors.New("report delivery decision does not match exact publication materials")
	}
	return nil
}

// ValidateReportDeliveryDecisionDurableGraphV1 rebinds a decision to every
// locally durable report-publication member. Shared-witness freshness, the
// exact stage receipt, and current PII authorization remain app-layer checks;
// a self-contained decision signature alone is never registry membership.
func ValidateReportDeliveryDecisionDurableGraphV1(
	decision ReportDeliveryDecisionV1,
	attempt PublicationAttemptV1,
	candidate PublicationReceiptV1,
	index PublicationIndexV1,
	selection PublicationCommitSelectionV1,
	commit PublicationCommitReceiptV1,
	ledger ClaimLedgerV1,
	projection PIIProjectionV1,
	inspection RenderInspectionV1,
) error {
	if ValidateReportDeliveryDecisionV1(decision) != nil ||
		ValidatePublicationAttemptV1(attempt) != nil ||
		ValidatePublicationReceiptMaterialsV1(candidate, ledger, projection, inspection) != nil ||
		ValidatePublicationIndexReceiptV1(index, candidate) != nil ||
		ValidatePublicationCommitSelectionMaterialsV1(
			selection, attempt, candidate, index, decision.AuthorityAdvanceSettlementDigest,
		) != nil || ValidatePublicationCommitReceiptMaterialsV1(commit, candidate, index) != nil ||
		ValidatePublicationCommitSelectionCommitV1(selection, commit) != nil {
		return errors.New("report delivery decision durable graph is invalid")
	}
	context := decision.Context
	if !reportDeliveryCandidateMatchesContextV1(candidate, context) ||
		decision.InstallationID != attempt.InstallationID || decision.EnrollmentID != attempt.EnrollmentID ||
		decision.AttemptID != attempt.AttemptID || decision.AttemptRecordDigest != attempt.RecordDigest ||
		decision.StageInputHash != attempt.StageInputHash ||
		decision.ReportStageWorkID != attempt.ReportStageWorkID ||
		decision.ReportStageReceiptID != attempt.ReportStageReceiptID ||
		decision.ReportStageReceiptSHA256 != attempt.ReportStageReceiptSHA256 ||
		decision.GrantRegistrySequence != attempt.GrantRegistrySequence ||
		decision.GrantRegistryDigest != attempt.GrantRegistryDigest ||
		decision.GrantRegistryEntryDigest != attempt.GrantRegistryEntryDigest ||
		decision.GrantID != attempt.GrantID || decision.ToolCallID != attempt.ToolCallID || decision.ToolName != attempt.ToolName ||
		decision.CandidateReceiptID != candidate.ReceiptID || decision.CandidateRecordDigest != candidate.RecordDigest ||
		decision.PublicationIndexDigest != index.IndexDigest ||
		decision.CommitSelectionID != selection.SelectionID ||
		decision.CommitSelectionRecordDigest != selection.RecordDigest ||
		decision.AuthorityAdvanceSettlementDigest != selection.AuthorityAdvanceSettlementDigest ||
		decision.CommitReceiptID != commit.CommitReceiptID || decision.CommitRecordDigest != commit.RecordDigest ||
		decision.PreviousEvidenceAuthorityBundleDigest != commit.PreviousEvidenceBundleDigest ||
		decision.CommittedEvidenceAuthorityBundleDigest != commit.CommittedEvidenceBundleDigest ||
		decision.EvidenceRegistryIndexDigest != commit.EvidenceRegistryIndexDigest ||
		decision.EvidenceRegistryCount != commit.EvidenceRegistryCount ||
		decision.EvidenceRegistrySequence != candidate.EvidenceRegistrySequence ||
		decision.EvidenceRegistryStateDigest != candidate.EvidenceRegistryStateDigest ||
		decision.EvidenceReceiptSetDigest != commit.EvidenceReceiptSetDigest ||
		decision.EvidenceReceiptCount != commit.EvidenceReceiptCount ||
		decision.ReportVariant != candidate.ReportVariant || decision.ClaimLedgerDigest != ledger.LedgerDigest ||
		decision.ClaimCount != uint64(len(ledger.Claims)) ||
		decision.PIIProjectionDigest != projection.ProjectionDigest ||
		decision.PIIProjectionClass != projection.ProjectionClass ||
		decision.AuthorizationAuditDigest != projection.AuthorizationAuditDigest ||
		decision.RenderInspectionDigest != inspection.InspectionDigest ||
		decision.ReportSHA256 != commit.ReportSHA256 || decision.ReportByteLength != commit.ReportByteLength ||
		decision.MediaType != commit.MediaType || decision.TargetIdentityDigest != commit.TargetIdentityDigest ||
		decision.AuthorityAlgorithm != commit.AuthorityAlgorithm || decision.AuthorityKeyID != commit.AuthorityKeyID ||
		decision.AuthorityPublicKey != commit.AuthorityPublicKey {
		return errors.New("report delivery decision does not match exact durable report materials")
	}
	return nil
}

func ParseReportDeliveryDecisionV1(body []byte) (ReportDeliveryDecisionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 512 << 10, MaxDepth: 32, MaxTokens: 4096, MaxStringBytes: 128 << 10,
	}); err != nil {
		return ReportDeliveryDecisionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var decision ReportDeliveryDecisionV1
	if err := decoder.Decode(&decision); err != nil {
		return ReportDeliveryDecisionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReportDeliveryDecisionV1{}, errors.New("report delivery decision contains trailing JSON")
	}
	canonical, err := json.Marshal(decision)
	if err != nil || !bytes.Equal(canonical, body) {
		return ReportDeliveryDecisionV1{}, errors.New("report delivery decision is not canonically encoded")
	}
	return decision, ValidateReportDeliveryDecisionV1(decision)
}

func ReportDeliveryDecisionV1Bytes(decision ReportDeliveryDecisionV1) ([]byte, error) {
	if err := ValidateReportDeliveryDecisionV1(decision); err != nil {
		return nil, err
	}
	return json.Marshal(decision)
}

func ReportDeliveryDecisionSigningBytesV1(decision ReportDeliveryDecisionV1) []byte {
	decision.AuthoritySignature = ""
	decision.RecordDigest = ""
	body, _ := json.Marshal(decision)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), reportDeliveryDecisionSignatureDomainV1...), digest[:]...)
}

func newReportDeliveryDecisionV1WithoutSigning(input ReportDeliveryDecisionInputV1) (ReportDeliveryDecisionV1, error) {
	if err := validateReportDeliveryDecisionInputGraphV1(input); err != nil {
		return ReportDeliveryDecisionV1{}, err
	}
	decision := assembleReportDeliveryDecisionV1(input)
	if err := validateReportDeliveryDecisionUnsignedV1(decision); err != nil {
		return ReportDeliveryDecisionV1{}, err
	}
	return decision, nil
}

func assembleReportDeliveryDecisionV1(input ReportDeliveryDecisionInputV1) ReportDeliveryDecisionV1 {
	attempt, candidate, selection, commit := input.Attempt, input.Candidate, input.Selection, input.Commit
	return ReportDeliveryDecisionV1{
		SchemaVersion: ReportDeliveryDecisionSchemaVersion, Purpose: ReportDeliveryDecisionPurpose,
		DecisionID: ReportDeliveryDecisionIDV1(attempt.InstallationID, attempt.EnrollmentID, attempt.AttemptID),
		Status:     ReportDeliveryDecisionAdmitted, InstallationID: attempt.InstallationID, EnrollmentID: attempt.EnrollmentID,
		Context:   input.Context,
		AttemptID: attempt.AttemptID, AttemptRecordDigest: attempt.RecordDigest, StageInputHash: attempt.StageInputHash,
		ReportStageWorkID: attempt.ReportStageWorkID, ReportStageReceiptID: attempt.ReportStageReceiptID,
		ReportStageReceiptSHA256: attempt.ReportStageReceiptSHA256,
		GrantRegistrySequence:    attempt.GrantRegistrySequence, GrantRegistryDigest: attempt.GrantRegistryDigest,
		GrantRegistryEntryDigest: attempt.GrantRegistryEntryDigest, GrantID: attempt.GrantID,
		ToolCallID: attempt.ToolCallID, ToolName: attempt.ToolName,
		CandidateReceiptID: candidate.ReceiptID, CandidateRecordDigest: candidate.RecordDigest,
		PublicationIndexDigest: input.Index.IndexDigest,
		CommitSelectionID:      selection.SelectionID, CommitSelectionRecordDigest: selection.RecordDigest,
		AuthorityAdvanceSettlementDigest: strings.TrimSpace(input.AuthorityAdvanceSettlementDigest),
		CommitReceiptID:                  commit.CommitReceiptID, CommitRecordDigest: commit.RecordDigest,
		PreviousEvidenceAuthorityBundleDigest:  commit.PreviousEvidenceBundleDigest,
		CommittedEvidenceAuthorityBundleDigest: strings.TrimSpace(input.WitnessedEvidenceAuthorityBundleDigest),
		EvidenceRegistryIndexDigest:            strings.TrimSpace(input.WitnessedEvidenceRegistryIndexDigest),
		EvidenceRegistryCount:                  input.EvidenceRegistryCount,
		EvidenceRegistrySequence:               input.EvidenceRegistrySequence, EvidenceRegistryStateDigest: strings.TrimSpace(input.EvidenceRegistryStateDigest),
		EvidenceReceiptSetDigest: commit.EvidenceReceiptSetDigest, EvidenceReceiptCount: commit.EvidenceReceiptCount,
		ReportVariant: candidate.ReportVariant, ClaimLedgerDigest: input.Ledger.LedgerDigest, ClaimCount: uint64(len(input.Ledger.Claims)),
		PIIProjectionDigest: input.Projection.ProjectionDigest, PIIProjectionClass: input.Projection.ProjectionClass,
		AuthorizationAuditDigest: input.Projection.AuthorizationAuditDigest, RenderInspectionDigest: input.Inspection.InspectionDigest,
		ReportSHA256: commit.ReportSHA256, ReportByteLength: commit.ReportByteLength, MediaType: commit.MediaType,
		TargetIdentityDigest: commit.TargetIdentityDigest,
		AuthorityAlgorithm:   PublicationReceiptAlgorithm,
		AuthorityKeyID:       strings.TrimSpace(input.CommitInput.AuthorityKeyID),
		AuthorityPublicKey:   base64.RawURLEncoding.EncodeToString(input.CommitInput.AuthorityPublicKey),
	}
}

func validateReportDeliveryDecisionInputGraphV1(input ReportDeliveryDecisionInputV1) error {
	contextBinding, bindingErr := domainpendingwork.ContextBindingFromSecurityContextV1(input.Context)
	selectionInput := PublicationCommitSelectionInputV1{
		Attempt: input.Attempt, SettlementDigest: input.AuthorityAdvanceSettlementDigest, CommitInput: input.CommitInput,
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainpendingwork.ValidatePendingWorkReceiptV1(input.StageReceipt) != nil ||
		bindingErr != nil || input.StageReceipt.Context != contextBinding ||
		!reportDeliveryMaterialsMatchContextV1(input) ||
		input.CommitInput.Candidate != input.Candidate || input.CommitInput.Index != input.Index ||
		input.CommitInput.InstallationID != input.Attempt.InstallationID ||
		input.CommitInput.EnrollmentID != input.Attempt.EnrollmentID ||
		ValidatePublicationAttemptGraphV1(input.Attempt, input.StageReceipt, input.Candidate, input.Index) != nil ||
		ValidatePublicationReceiptMaterialsV1(input.Candidate, input.Ledger, input.Projection, input.Inspection) != nil ||
		ValidatePublicationCommitReceiptMaterialsV1(input.Commit, input.Candidate, input.Index) != nil ||
		ValidatePublicationCommitReceiptExactV1(input.Commit, input.CommitInput) != nil ||
		ValidatePublicationCommitSelectionExactV1(input.Selection, selectionInput) != nil ||
		ValidatePublicationCommitSelectionMaterialsV1(
			input.Selection, input.Attempt, input.Candidate, input.Index, input.AuthorityAdvanceSettlementDigest,
		) != nil || ValidatePublicationCommitSelectionCommitV1(input.Selection, input.Commit) != nil {
		return errors.New("report delivery decision publication graph is invalid")
	}
	publicKey := append([]byte(nil), input.CommitInput.AuthorityPublicKey...)
	if len(publicKey) != ed25519.PublicKeySize || strings.TrimSpace(input.CommitInput.AuthorityKeyID) != domainsecurity.SHA256Hex(publicKey) ||
		input.Attempt.AuthorityKeyID != strings.TrimSpace(input.CommitInput.AuthorityKeyID) ||
		input.Attempt.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(publicKey) ||
		input.Commit.CommittedEvidenceBundleDigest != strings.TrimSpace(input.WitnessedEvidenceAuthorityBundleDigest) ||
		input.Commit.EvidenceRegistryIndexDigest != strings.TrimSpace(input.WitnessedEvidenceRegistryIndexDigest) ||
		input.Commit.EvidenceRegistryCount != input.EvidenceRegistryCount ||
		input.Candidate.EvidenceRegistryIndexDigest != strings.TrimSpace(input.WitnessedEvidenceRegistryIndexDigest) ||
		input.Candidate.EvidenceRegistryCount != input.EvidenceRegistryCount ||
		input.Candidate.EvidenceRegistrySequence != input.EvidenceRegistrySequence ||
		input.Candidate.EvidenceRegistryStateDigest != strings.TrimSpace(input.EvidenceRegistryStateDigest) {
		return errors.New("report delivery decision witnessed evidence binding is invalid")
	}
	return nil
}

func reportDeliveryMaterialsMatchContextV1(input ReportDeliveryDecisionInputV1) bool {
	context := input.Context
	candidate := input.Candidate
	commit := input.Commit
	ledger := input.Ledger
	attempt := input.Attempt
	return reportDeliveryCandidateMatchesContextV1(candidate, context) &&
		commit.ThreadID == context.ThreadID && commit.TurnID == context.TurnID &&
		commit.ContextDigest == context.ContextDigest && commit.CaseID == context.CaseID &&
		commit.CaseBindingHash == context.CaseBindingHash && commit.ContextEpoch == context.ContextEpoch &&
		commit.DatasetSnapshotID == context.DatasetSnapshotID && commit.SourceManifestHash == context.SourceManifestHash &&
		ledger.ThreadID == context.ThreadID && ledger.TurnID == context.TurnID &&
		ledger.ContextDigest == context.ContextDigest && ledger.CaseID == context.CaseID &&
		ledger.CaseBindingHash == context.CaseBindingHash && ledger.ContextEpoch == context.ContextEpoch &&
		ledger.DatasetSnapshotID == context.DatasetSnapshotID && ledger.SourceManifestHash == context.SourceManifestHash &&
		attempt.ThreadID == context.ThreadID && attempt.TurnID == context.TurnID &&
		attempt.ContextDigest == context.ContextDigest && attempt.CaseBindingHash == context.CaseBindingHash &&
		attempt.ContextEpoch == context.ContextEpoch && attempt.DatasetSnapshotID == context.DatasetSnapshotID &&
		attempt.SourceManifestHash == context.SourceManifestHash
}

func reportDeliveryCandidateMatchesContextV1(candidate PublicationReceiptV1, context domainsecurity.TurnSecurityContext) bool {
	return candidate.ThreadID == context.ThreadID && candidate.TurnID == context.TurnID &&
		candidate.ContextDigest == context.ContextDigest && candidate.CaseID == context.CaseID &&
		candidate.CaseBindingHash == context.CaseBindingHash && candidate.ContextEpoch == context.ContextEpoch &&
		candidate.DatasetSnapshotID == context.DatasetSnapshotID && candidate.SourceManifestHash == context.SourceManifestHash
}

func validateReportDeliveryDecisionUnsignedV1(decision ReportDeliveryDecisionV1) error {
	if decision.SchemaVersion != ReportDeliveryDecisionSchemaVersion || decision.Purpose != ReportDeliveryDecisionPurpose ||
		decision.Status != ReportDeliveryDecisionAdmitted || !domainsecurity.IsSHA256Hex(decision.DecisionID) ||
		decision.DecisionID != ReportDeliveryDecisionIDV1(decision.InstallationID, decision.EnrollmentID, decision.AttemptID) ||
		!domainsecurity.IsSHA256Hex(decision.InstallationID) || !domainsecurity.IsSHA256Hex(decision.EnrollmentID) ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(decision.Context) != nil ||
		!domainsecurity.IsSHA256Hex(decision.AttemptID) || !domainsecurity.IsSHA256Hex(decision.AttemptRecordDigest) ||
		!domainsecurity.IsSHA256Hex(decision.StageInputHash) || !domainsecurity.IsSHA256Hex(decision.ReportStageWorkID) ||
		!domainsecurity.IsSHA256Hex(decision.ReportStageReceiptID) || !domainsecurity.IsSHA256Hex(decision.ReportStageReceiptSHA256) ||
		decision.GrantRegistrySequence == 0 || !domainsecurity.IsSHA256Hex(decision.GrantRegistryDigest) ||
		!domainsecurity.IsSHA256Hex(decision.GrantRegistryEntryDigest) || !domainsecurity.IsSHA256Hex(decision.GrantID) ||
		strings.TrimSpace(decision.ToolCallID) == "" || decision.ToolName != PublicationAttemptToolName ||
		!domainsecurity.IsSHA256Hex(decision.CandidateReceiptID) || !domainsecurity.IsSHA256Hex(decision.CandidateRecordDigest) ||
		!domainsecurity.IsSHA256Hex(decision.PublicationIndexDigest) || !domainsecurity.IsSHA256Hex(decision.CommitSelectionID) ||
		!domainsecurity.IsSHA256Hex(decision.CommitSelectionRecordDigest) ||
		!domainsecurity.IsSHA256Hex(decision.AuthorityAdvanceSettlementDigest) ||
		!domainsecurity.IsSHA256Hex(decision.CommitReceiptID) || !domainsecurity.IsSHA256Hex(decision.CommitRecordDigest) ||
		!domainsecurity.IsSHA256Hex(decision.PreviousEvidenceAuthorityBundleDigest) ||
		!domainsecurity.IsSHA256Hex(decision.CommittedEvidenceAuthorityBundleDigest) ||
		!domainsecurity.IsSHA256Hex(decision.EvidenceRegistryIndexDigest) ||
		decision.EvidenceRegistryCount == 0 || decision.EvidenceRegistrySequence == 0 ||
		!domainsecurity.IsSHA256Hex(decision.EvidenceRegistryStateDigest) ||
		!domainsecurity.IsSHA256Hex(decision.EvidenceReceiptSetDigest) || decision.EvidenceReceiptCount == 0 ||
		!validReportVariant(decision.ReportVariant) || !domainsecurity.IsSHA256Hex(decision.ClaimLedgerDigest) ||
		(decision.ReportVariant != VerifiedNoHitReport && decision.ClaimCount == 0) ||
		!domainsecurity.IsSHA256Hex(decision.PIIProjectionDigest) ||
		(decision.PIIProjectionClass != PIIProjectionOrdinaryMasked && decision.PIIProjectionClass != PIIProjectionControlledFull) ||
		!domainsecurity.IsSHA256Hex(decision.RenderInspectionDigest) || !domainsecurity.IsSHA256Hex(decision.ReportSHA256) ||
		decision.ReportByteLength == 0 || strings.TrimSpace(decision.MediaType) == "" ||
		!domainsecurity.IsSHA256Hex(decision.TargetIdentityDigest) || decision.AuthorityAlgorithm != PublicationReceiptAlgorithm ||
		!domainsecurity.IsSHA256Hex(decision.AuthorityKeyID) {
		return errors.New("report delivery decision is incomplete")
	}
	if decision.PIIProjectionClass == PIIProjectionOrdinaryMasked && decision.AuthorizationAuditDigest != "" ||
		decision.PIIProjectionClass == PIIProjectionControlledFull && !domainsecurity.IsSHA256Hex(decision.AuthorizationAuditDigest) {
		return errors.New("report delivery decision PII authority is invalid")
	}
	return nil
}

func reportDeliveryDecisionRecordDigestV1(decision ReportDeliveryDecisionV1) string {
	decision.RecordDigest = ""
	body, _ := json.Marshal(decision)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), reportDeliveryDecisionRecordDomainV1...), body...))
}
