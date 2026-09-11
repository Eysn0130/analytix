package attachment

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	AttachmentUseReceiptSchemaVersionV1 = 1
	AttachmentUseReceiptPurposeV1       = "analytix.attachment-use-receipt/v1"
	AttachmentUseDispositionPurposeV1   = "analytix.attachment-use-disposition/v1"
	AttachmentUseAuthorityAlgorithmV1   = "Ed25519"

	AttachmentUseConsumerPrimaryProviderV1 = "primary_provider"
	AttachmentUseConsumerVisionBridgeV1    = "vision_bridge"
	// AttachmentUseConsumerProviderPipelineV1 binds one ordered attachment set
	// to a single effect lease whose images may pass through the configured
	// vision bridge before the primary provider consumes the derived text.
	AttachmentUseConsumerProviderPipelineV1 = "provider_attempt_pipeline"

	AttachmentUseDispositionConsumedV1       = "consumed"
	AttachmentUseDispositionCancelledV1      = "cancelled"
	AttachmentUseDispositionFailedV1         = "failed"
	AttachmentUseDispositionRejectedV1       = "rejected"
	AttachmentUseDispositionRestartInvalidV1 = "restart_invalid"
	AttachmentUseDispositionStaleContextV1   = "stale_context"

	// MaxAttachmentUseRecordBytesV1 is shared by canonical parsing and the
	// private CAS adapter so storage and domain acceptance cannot drift.
	MaxAttachmentUseRecordBytesV1 = 256 * 1024
	// MaxAttachmentUseSetMembersV1 bounds one ordered owner/projection set.
	MaxAttachmentUseSetMembersV1 = 512
	maxAttachmentUseTextBytes    = 4096
)

var (
	attachmentUseReceiptSigningDomainV1     = []byte("analytix.attachment-use-receipt/signature/v1\x00")
	attachmentUseReceiptDigestDomainV1      = []byte("analytix.attachment-use-receipt/record/v1\x00")
	attachmentUseIDDomainV1                 = []byte("analytix.attachment-use/id/v1\x00")
	attachmentUseSetDigestDomainV1          = []byte("analytix.attachment-use-set/digest/v1\x00")
	attachmentUseDispositionIDDomainV1      = []byte("analytix.attachment-use-disposition/id/v1\x00")
	attachmentUseDispositionSigningDomainV1 = []byte("analytix.attachment-use-disposition/signature/v1\x00")
	attachmentUseDispositionDigestDomainV1  = []byte("analytix.attachment-use-disposition/record/v1\x00")
	attachmentUseReasonCodePattern          = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,95}$`)
)

// AttachmentUseContextBindingV1 is a private, exact reference to the frozen
// turn context. ContextDigest also commits to the V2 publication and risk
// authority fields, which are deliberately not copied into this receipt.
type AttachmentUseContextBindingV1 struct {
	ContextVersion     int    `json:"contextVersion"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	WorkspaceRealPath  string `json:"workspaceRealPath"`
	TenantID           string `json:"tenantId"`
	UserID             string `json:"userId"`
	CaseID             string `json:"caseId"`
	CaseBindingHash    string `json:"caseBindingHash"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	ContextIssuedAt    string `json:"contextIssuedAt"`
	ContextDigest      string `json:"contextDigest"`
}

// AttachmentUseSetMemberV1 preserves caller order with a zero-based index.
// Reordering an otherwise identical attachment set changes its set digest and
// every member UseID.
type AttachmentUseSetMemberV1 struct {
	BatchIndex         uint32 `json:"batchIndex"`
	AttachmentID       string `json:"attachmentId"`
	OwnerDigest        string `json:"ownerDigest"`
	BlobSHA256         string `json:"blobSHA256"`
	BlobByteSize       int64  `json:"blobByteSize"`
	MIMEType           string `json:"mimeType"`
	ProjectionSHA256   string `json:"projectionSHA256"`
	ProjectionByteSize int64  `json:"projectionByteSize"`
}

// AttachmentUseProjectionV1 is the actual route-specific provider projection
// for one owner. It must be supplied explicitly and in the same order as the
// owner set; an upload metadata projection is never used as a fallback.
type AttachmentUseProjectionV1 struct {
	AttachmentID       string `json:"attachmentId"`
	ProjectionSHA256   string `json:"projectionSHA256"`
	ProjectionByteSize int64  `json:"projectionByteSize"`
}

// AttachmentUseReceiptV1 is private host authority to read exactly one owner
// as one member of an ordered attachment set for one already-authorized
// effect and consumer. It is not evidence, citation, PII export, or fact
// publication authority. Production verification must additionally anchor
// the self-contained signature to the installation's trusted authority.
type AttachmentUseReceiptV1 struct {
	SchemaVersion         int                           `json:"schemaVersion"`
	Purpose               string                        `json:"purpose"`
	UseID                 string                        `json:"useId"`
	Context               AttachmentUseContextBindingV1 `json:"context"`
	AttachmentID          string                        `json:"attachmentId"`
	OwnerDigest           string                        `json:"ownerDigest"`
	BlobSHA256            string                        `json:"blobSHA256"`
	BlobByteSize          int64                         `json:"blobByteSize"`
	MIMEType              string                        `json:"mimeType"`
	ProjectionSHA256      string                        `json:"projectionSHA256"`
	ProjectionByteSize    int64                         `json:"projectionByteSize"`
	AttachmentSetDigest   string                        `json:"attachmentSetDigest"`
	BatchIndex            uint32                        `json:"batchIndex"`
	BatchCount            uint32                        `json:"batchCount"`
	EffectBindingDigest   string                        `json:"effectBindingDigest"`
	ConsumerKind          string                        `json:"consumerKind"`
	ConsumerBindingDigest string                        `json:"consumerBindingDigest"`
	IssuedAt              string                        `json:"issuedAt"`
	AuthorityAlgorithm    string                        `json:"authorityAlgorithm"`
	AuthorityKeyID        string                        `json:"authorityKeyId"`
	AuthorityPublicKey    string                        `json:"authorityPublicKey"`
	AuthoritySignature    string                        `json:"authoritySignature"`
	ReceiptDigest         string                        `json:"receiptDigest"`
}

type AttachmentUseReceiptInputV1 struct {
	SecurityContext       domainsecurity.TurnSecurityContext
	Owner                 OwnerRecordV1
	AttachmentSet         []OwnerRecordV1
	ProjectionSet         []AttachmentUseProjectionV1
	BatchIndex            uint32
	EffectBindingDigest   string
	ConsumerKind          string
	ConsumerBindingDigest string
	IssuedAt              time.Time
	AuthorityKeyID        string
	AuthorityPublicKey    []byte
}

// AttachmentUseDispositionV1 closes the private use intent. Stores must key
// dispositions by UseID, not DispositionID, so mutually exclusive outcomes
// cannot be persisted for one logical effect.
type AttachmentUseDispositionV1 struct {
	SchemaVersion         int    `json:"schemaVersion"`
	Purpose               string `json:"purpose"`
	DispositionID         string `json:"dispositionId"`
	UseID                 string `json:"useId"`
	ReceiptDigest         string `json:"receiptDigest"`
	ContextDigest         string `json:"contextDigest"`
	ContextEpoch          uint64 `json:"contextEpoch"`
	DatasetSnapshotID     string `json:"datasetSnapshotId"`
	AttachmentID          string `json:"attachmentId"`
	OwnerDigest           string `json:"ownerDigest"`
	BlobSHA256            string `json:"blobSHA256"`
	BlobByteSize          int64  `json:"blobByteSize"`
	MIMEType              string `json:"mimeType"`
	ProjectionSHA256      string `json:"projectionSHA256"`
	ProjectionByteSize    int64  `json:"projectionByteSize"`
	AttachmentSetDigest   string `json:"attachmentSetDigest"`
	BatchIndex            uint32 `json:"batchIndex"`
	BatchCount            uint32 `json:"batchCount"`
	EffectBindingDigest   string `json:"effectBindingDigest"`
	ConsumerKind          string `json:"consumerKind"`
	ConsumerBindingDigest string `json:"consumerBindingDigest"`
	Status                string `json:"status"`
	ReasonCode            string `json:"reasonCode"`
	DisposedAt            string `json:"disposedAt"`
	AuthorityAlgorithm    string `json:"authorityAlgorithm"`
	AuthorityKeyID        string `json:"authorityKeyId"`
	AuthorityPublicKey    string `json:"authorityPublicKey"`
	AuthoritySignature    string `json:"authoritySignature"`
	RecordDigest          string `json:"recordDigest"`
}

type AttachmentUseSignFuncV1 func([]byte) ([]byte, error)

func NewAttachmentUseReceiptV1(input AttachmentUseReceiptInputV1, sign AttachmentUseSignFuncV1) (AttachmentUseReceiptV1, error) {
	contextBinding, err := attachmentUseContextBindingFromExecutionContextV1(input.SecurityContext)
	if err != nil {
		return AttachmentUseReceiptV1{}, err
	}
	members, err := attachmentUseSetMembersV1(input.AttachmentSet, input.ProjectionSet)
	if err != nil || input.BatchIndex >= uint32(len(members)) {
		return AttachmentUseReceiptV1{}, errors.New("attachment use ordered set is invalid")
	}
	if err := validateAttachmentUseOwnersForContextV1(input.AttachmentSet, input.SecurityContext); err != nil {
		return AttachmentUseReceiptV1{}, err
	}
	selected := members[input.BatchIndex]
	if ValidateOwnerRecordV1(input.Owner) != nil || selected.AttachmentID != input.Owner.AttachmentID || selected.OwnerDigest != input.Owner.OwnerDigest {
		return AttachmentUseReceiptV1{}, errors.New("attachment use selected owner does not match its ordered set member")
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	contextIssuedAt, _ := time.Parse(time.RFC3339Nano, input.SecurityContext.IssuedAt)
	if issuedAt.Before(contextIssuedAt) || attachmentUseReceiptPredatesOwnerSetV1(issuedAt, input.AttachmentSet) {
		return AttachmentUseReceiptV1{}, errors.New("attachment use receipt predates its context or owner")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	receipt := AttachmentUseReceiptV1{
		SchemaVersion: AttachmentUseReceiptSchemaVersionV1, Purpose: AttachmentUseReceiptPurposeV1,
		Context: contextBinding, AttachmentID: selected.AttachmentID, OwnerDigest: selected.OwnerDigest,
		BlobSHA256: selected.BlobSHA256, BlobByteSize: selected.BlobByteSize, MIMEType: selected.MIMEType,
		ProjectionSHA256: selected.ProjectionSHA256, ProjectionByteSize: selected.ProjectionByteSize,
		AttachmentSetDigest: attachmentUseSetDigestFromMembersV1(members), BatchIndex: input.BatchIndex,
		BatchCount: uint32(len(members)), EffectBindingDigest: strings.TrimSpace(input.EffectBindingDigest),
		ConsumerKind: strings.TrimSpace(input.ConsumerKind), ConsumerBindingDigest: strings.TrimSpace(input.ConsumerBindingDigest),
		IssuedAt: issuedAt.Format(time.RFC3339Nano), AuthorityAlgorithm: AttachmentUseAuthorityAlgorithmV1,
		AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.UseID = attachmentUseIDV1(receipt)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return AttachmentUseReceiptV1{}, errors.New("attachment use receipt signing authority is invalid")
	}
	if err := validateAttachmentUseReceiptUnsignedV1(receipt); err != nil {
		return AttachmentUseReceiptV1{}, err
	}
	signature, err := sign(AttachmentUseReceiptV1SigningBytes(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return AttachmentUseReceiptV1{}, errors.New("attachment use receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.ReceiptDigest = attachmentUseReceiptDigestV1(receipt)
	if err := ValidateAttachmentUseReceiptForBindingsV1(receipt, input.SecurityContext, input.Owner, input.AttachmentSet, input.ProjectionSet,
		input.EffectBindingDigest, input.ConsumerKind, input.ConsumerBindingDigest); err != nil {
		return AttachmentUseReceiptV1{}, err
	}
	return receipt, nil
}

func NewAttachmentUseDispositionV1(
	receipt AttachmentUseReceiptV1,
	status string,
	reasonCode string,
	disposedAt time.Time,
	keyID string,
	publicKey []byte,
	sign AttachmentUseSignFuncV1,
) (AttachmentUseDispositionV1, error) {
	if err := ValidateAttachmentUseReceiptV1(receipt); err != nil {
		return AttachmentUseDispositionV1{}, err
	}
	disposedAt = disposedAt.UTC()
	if disposedAt.IsZero() {
		disposedAt = time.Now().UTC()
	}
	issuedAt, _ := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if disposedAt.Before(issuedAt) {
		return AttachmentUseDispositionV1{}, errors.New("attachment use disposition predates its receipt")
	}
	publicKey = append([]byte(nil), publicKey...)
	disposition := AttachmentUseDispositionV1{
		SchemaVersion: AttachmentUseReceiptSchemaVersionV1, Purpose: AttachmentUseDispositionPurposeV1,
		UseID: receipt.UseID, ReceiptDigest: receipt.ReceiptDigest,
		ContextDigest: receipt.Context.ContextDigest, ContextEpoch: receipt.Context.ContextEpoch,
		DatasetSnapshotID: receipt.Context.DatasetSnapshotID, AttachmentID: receipt.AttachmentID,
		OwnerDigest: receipt.OwnerDigest, BlobSHA256: receipt.BlobSHA256, BlobByteSize: receipt.BlobByteSize,
		MIMEType: receipt.MIMEType, ProjectionSHA256: receipt.ProjectionSHA256, ProjectionByteSize: receipt.ProjectionByteSize,
		AttachmentSetDigest: receipt.AttachmentSetDigest, BatchIndex: receipt.BatchIndex, BatchCount: receipt.BatchCount,
		EffectBindingDigest: receipt.EffectBindingDigest, ConsumerKind: receipt.ConsumerKind,
		ConsumerBindingDigest: receipt.ConsumerBindingDigest, Status: strings.TrimSpace(status), ReasonCode: strings.TrimSpace(reasonCode),
		DisposedAt: disposedAt.Format(time.RFC3339Nano), AuthorityAlgorithm: AttachmentUseAuthorityAlgorithmV1,
		AuthorityKeyID: strings.TrimSpace(keyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	disposition.DispositionID = attachmentUseDispositionIDV1(disposition)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		disposition.AuthorityKeyID != receipt.AuthorityKeyID || disposition.AuthorityPublicKey != receipt.AuthorityPublicKey {
		return AttachmentUseDispositionV1{}, errors.New("attachment use disposition signing authority is invalid")
	}
	if err := validateAttachmentUseDispositionUnsignedV1(disposition); err != nil {
		return AttachmentUseDispositionV1{}, err
	}
	signature, err := sign(AttachmentUseDispositionV1SigningBytes(disposition))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return AttachmentUseDispositionV1{}, errors.New("attachment use disposition signing failed")
	}
	disposition.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	disposition.RecordDigest = attachmentUseDispositionRecordDigestV1(disposition)
	if err := ValidateAttachmentUseDispositionForReceiptV1(disposition, receipt); err != nil {
		return AttachmentUseDispositionV1{}, err
	}
	return disposition, nil
}

func ValidateAttachmentUseReceiptV1(receipt AttachmentUseReceiptV1) error {
	if err := validateAttachmentUseReceiptUnsignedV1(receipt); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != receipt.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != receipt.AuthoritySignature ||
		receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), AttachmentUseReceiptV1SigningBytes(receipt), signature) {
		return errors.New("attachment use receipt signature is invalid")
	}
	if !attachmentUseCanonicalSHA256(receipt.ReceiptDigest) || receipt.ReceiptDigest != attachmentUseReceiptDigestV1(receipt) ||
		receipt.ReceiptDigest == receipt.UseID {
		return errors.New("attachment use receipt digest is invalid")
	}
	return nil
}

func ValidateAttachmentUseDispositionV1(disposition AttachmentUseDispositionV1) error {
	if err := validateAttachmentUseDispositionUnsignedV1(disposition); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != disposition.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != disposition.AuthoritySignature ||
		disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), AttachmentUseDispositionV1SigningBytes(disposition), signature) {
		return errors.New("attachment use disposition signature is invalid")
	}
	if !attachmentUseCanonicalSHA256(disposition.RecordDigest) || disposition.RecordDigest != attachmentUseDispositionRecordDigestV1(disposition) ||
		disposition.RecordDigest == disposition.DispositionID || disposition.RecordDigest == disposition.UseID {
		return errors.New("attachment use disposition record digest is invalid")
	}
	return nil
}

func ValidateAttachmentUseReceiptForBindingsV1(
	receipt AttachmentUseReceiptV1,
	securityContext domainsecurity.TurnSecurityContext,
	owner OwnerRecordV1,
	attachmentSet []OwnerRecordV1,
	projectionSet []AttachmentUseProjectionV1,
	effectBindingDigest string,
	consumerKind string,
	consumerBindingDigest string,
) error {
	if err := ValidateAttachmentUseReceiptV1(receipt); err != nil {
		return err
	}
	expectedContext, err := attachmentUseContextBindingFromExecutionContextV1(securityContext)
	if err != nil || receipt.Context != expectedContext || ValidateOwnerRecordV1(owner) != nil ||
		!attachmentUseOwnerMatchesContextV1(owner, securityContext) {
		return errors.New("attachment use receipt context or owner binding is invalid")
	}
	members, err := attachmentUseSetMembersV1(attachmentSet, projectionSet)
	if err != nil || receipt.BatchIndex >= uint32(len(members)) || receipt.BatchCount != uint32(len(members)) ||
		receipt.AttachmentSetDigest != attachmentUseSetDigestFromMembersV1(members) {
		return errors.New("attachment use receipt ordered set binding is invalid")
	}
	for _, setOwner := range attachmentSet {
		if !attachmentUseOwnerMatchesContextV1(setOwner, securityContext) {
			return errors.New("attachment use receipt set crosses its frozen turn context")
		}
	}
	receiptIssuedAt, _ := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if attachmentUseReceiptPredatesOwnerSetV1(receiptIssuedAt, attachmentSet) {
		return errors.New("attachment use receipt predates an owner in its ordered set")
	}
	selected := members[receipt.BatchIndex]
	if selected.AttachmentID != owner.AttachmentID || selected.OwnerDigest != owner.OwnerDigest ||
		receipt.AttachmentID != selected.AttachmentID || receipt.OwnerDigest != selected.OwnerDigest ||
		receipt.BlobSHA256 != selected.BlobSHA256 || receipt.BlobByteSize != selected.BlobByteSize || receipt.MIMEType != selected.MIMEType ||
		receipt.ProjectionSHA256 != selected.ProjectionSHA256 || receipt.ProjectionByteSize != selected.ProjectionByteSize ||
		receipt.EffectBindingDigest != strings.TrimSpace(effectBindingDigest) || receipt.ConsumerKind != strings.TrimSpace(consumerKind) ||
		receipt.ConsumerBindingDigest != strings.TrimSpace(consumerBindingDigest) {
		return errors.New("attachment use receipt effect or consumer binding is invalid")
	}
	return nil
}

func ValidateAttachmentUseDispositionForReceiptV1(disposition AttachmentUseDispositionV1, receipt AttachmentUseReceiptV1) error {
	if err := ValidateAttachmentUseReceiptV1(receipt); err != nil {
		return err
	}
	if err := ValidateAttachmentUseDispositionV1(disposition); err != nil {
		return err
	}
	if disposition.UseID != receipt.UseID || disposition.ReceiptDigest != receipt.ReceiptDigest ||
		disposition.ContextDigest != receipt.Context.ContextDigest || disposition.ContextEpoch != receipt.Context.ContextEpoch ||
		disposition.DatasetSnapshotID != receipt.Context.DatasetSnapshotID || disposition.AttachmentID != receipt.AttachmentID ||
		disposition.OwnerDigest != receipt.OwnerDigest || disposition.BlobSHA256 != receipt.BlobSHA256 ||
		disposition.BlobByteSize != receipt.BlobByteSize || disposition.MIMEType != receipt.MIMEType ||
		disposition.ProjectionSHA256 != receipt.ProjectionSHA256 || disposition.ProjectionByteSize != receipt.ProjectionByteSize ||
		disposition.AttachmentSetDigest != receipt.AttachmentSetDigest ||
		disposition.BatchIndex != receipt.BatchIndex || disposition.BatchCount != receipt.BatchCount ||
		disposition.EffectBindingDigest != receipt.EffectBindingDigest || disposition.ConsumerKind != receipt.ConsumerKind ||
		disposition.ConsumerBindingDigest != receipt.ConsumerBindingDigest || disposition.AuthorityKeyID != receipt.AuthorityKeyID ||
		disposition.AuthorityPublicKey != receipt.AuthorityPublicKey {
		return errors.New("attachment use disposition does not bind the exact receipt")
	}
	issuedAt, _ := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	disposedAt, _ := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if disposedAt.Before(issuedAt) {
		return errors.New("attachment use disposition predates its receipt")
	}
	return nil
}

func AttachmentUseSetDigestV1(owners []OwnerRecordV1, projections []AttachmentUseProjectionV1) string {
	members, err := attachmentUseSetMembersV1(owners, projections)
	if err != nil {
		return ""
	}
	return attachmentUseSetDigestFromMembersV1(members)
}

func ComputeAttachmentUseIDV1(input AttachmentUseReceiptInputV1) (string, error) {
	contextBinding, err := attachmentUseContextBindingFromExecutionContextV1(input.SecurityContext)
	if err != nil {
		return "", err
	}
	members, err := attachmentUseSetMembersV1(input.AttachmentSet, input.ProjectionSet)
	if err != nil || input.BatchIndex >= uint32(len(members)) || ValidateOwnerRecordV1(input.Owner) != nil ||
		members[input.BatchIndex].AttachmentID != input.Owner.AttachmentID || members[input.BatchIndex].OwnerDigest != input.Owner.OwnerDigest ||
		validateAttachmentUseOwnersForContextV1(input.AttachmentSet, input.SecurityContext) != nil ||
		!attachmentUseValidConsumer(input.ConsumerKind) || !attachmentUseCanonicalSHA256(strings.TrimSpace(input.EffectBindingDigest)) ||
		!attachmentUseCanonicalSHA256(strings.TrimSpace(input.ConsumerBindingDigest)) {
		return "", errors.New("attachment use identity input is invalid")
	}
	selected := members[input.BatchIndex]
	receipt := AttachmentUseReceiptV1{
		SchemaVersion: AttachmentUseReceiptSchemaVersionV1, Purpose: AttachmentUseReceiptPurposeV1,
		Context: contextBinding, AttachmentID: selected.AttachmentID, OwnerDigest: selected.OwnerDigest,
		BlobSHA256: selected.BlobSHA256, BlobByteSize: selected.BlobByteSize, MIMEType: selected.MIMEType,
		ProjectionSHA256: selected.ProjectionSHA256, ProjectionByteSize: selected.ProjectionByteSize,
		AttachmentSetDigest: attachmentUseSetDigestFromMembersV1(members), BatchIndex: input.BatchIndex,
		BatchCount: uint32(len(members)), EffectBindingDigest: strings.TrimSpace(input.EffectBindingDigest),
		ConsumerKind: strings.TrimSpace(input.ConsumerKind), ConsumerBindingDigest: strings.TrimSpace(input.ConsumerBindingDigest),
	}
	return attachmentUseIDV1(receipt), nil
}

func ParseAttachmentUseReceiptV1(body []byte) (AttachmentUseReceiptV1, error) {
	var receipt AttachmentUseReceiptV1
	if err := decodeAttachmentUseRecordV1(body, &receipt); err != nil {
		return AttachmentUseReceiptV1{}, err
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal(body, canonical) {
		return AttachmentUseReceiptV1{}, errors.New("attachment use receipt is not canonically encoded")
	}
	return receipt, ValidateAttachmentUseReceiptV1(receipt)
}

func ParseAttachmentUseDispositionV1(body []byte) (AttachmentUseDispositionV1, error) {
	var disposition AttachmentUseDispositionV1
	if err := decodeAttachmentUseRecordV1(body, &disposition); err != nil {
		return AttachmentUseDispositionV1{}, err
	}
	canonical, err := json.Marshal(disposition)
	if err != nil || !bytes.Equal(body, canonical) {
		return AttachmentUseDispositionV1{}, errors.New("attachment use disposition is not canonically encoded")
	}
	return disposition, ValidateAttachmentUseDispositionV1(disposition)
}

func AttachmentUseReceiptV1Bytes(receipt AttachmentUseReceiptV1) ([]byte, error) {
	if err := ValidateAttachmentUseReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func AttachmentUseDispositionV1Bytes(disposition AttachmentUseDispositionV1) ([]byte, error) {
	if err := ValidateAttachmentUseDispositionV1(disposition); err != nil {
		return nil, err
	}
	return json.Marshal(disposition)
}

func AttachmentUseReceiptV1SigningBytes(receipt AttachmentUseReceiptV1) []byte {
	receipt.AuthoritySignature = ""
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), attachmentUseReceiptSigningDomainV1...)
	return append(out, digest[:]...)
}

func AttachmentUseDispositionV1SigningBytes(disposition AttachmentUseDispositionV1) []byte {
	disposition.AuthoritySignature = ""
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), attachmentUseDispositionSigningDomainV1...)
	return append(out, digest[:]...)
}

func AttachmentUseReceiptV1AuthorityMaterial(receipt AttachmentUseReceiptV1) (string, []byte, []byte, error) {
	if err := ValidateAttachmentUseReceiptV1(receipt); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	return receipt.AuthorityKeyID, publicKey, signature, nil
}

func AttachmentUseDispositionV1AuthorityMaterial(disposition AttachmentUseDispositionV1) (string, []byte, []byte, error) {
	if err := ValidateAttachmentUseDispositionV1(disposition); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	return disposition.AuthorityKeyID, publicKey, signature, nil
}

func validateAttachmentUseReceiptUnsignedV1(receipt AttachmentUseReceiptV1) error {
	issuedAt, timeErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	contextIssuedAt, contextTimeErr := time.Parse(time.RFC3339Nano, receipt.Context.ContextIssuedAt)
	if receipt.SchemaVersion != AttachmentUseReceiptSchemaVersionV1 || receipt.Purpose != AttachmentUseReceiptPurposeV1 ||
		!attachmentUseCanonicalSHA256(receipt.UseID) || receipt.UseID != attachmentUseIDV1(receipt) ||
		validateAttachmentUseContextBindingV1(receipt.Context) != nil || !attachmentUseValidAttachmentID(receipt.AttachmentID) ||
		!attachmentUseCanonicalSHA256(receipt.OwnerDigest) || !attachmentUseCanonicalSHA256(receipt.BlobSHA256) ||
		receipt.BlobByteSize <= 0 || !attachmentUseCanonicalText(receipt.MIMEType) ||
		!attachmentUseCanonicalSHA256(receipt.ProjectionSHA256) || receipt.ProjectionByteSize <= 0 ||
		!attachmentUseCanonicalSHA256(receipt.AttachmentSetDigest) ||
		receipt.BatchCount == 0 || receipt.BatchCount > MaxAttachmentUseSetMembersV1 || receipt.BatchIndex >= receipt.BatchCount ||
		!attachmentUseCanonicalSHA256(receipt.EffectBindingDigest) || !attachmentUseValidConsumer(receipt.ConsumerKind) ||
		!attachmentUseCanonicalSHA256(receipt.ConsumerBindingDigest) || timeErr != nil || contextTimeErr != nil || issuedAt.IsZero() ||
		issuedAt.Before(contextIssuedAt) ||
		issuedAt.UTC().Format(time.RFC3339Nano) != receipt.IssuedAt || receipt.AuthorityAlgorithm != AttachmentUseAuthorityAlgorithmV1 ||
		!attachmentUseCanonicalSHA256(receipt.AuthorityKeyID) {
		return errors.New("attachment use receipt is incomplete")
	}
	body, err := json.Marshal(receipt)
	if err != nil || len(body) > MaxAttachmentUseRecordBytesV1 {
		return errors.New("attachment use receipt exceeds its canonical bound")
	}
	return nil
}

func validateAttachmentUseDispositionUnsignedV1(disposition AttachmentUseDispositionV1) error {
	disposedAt, timeErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if disposition.SchemaVersion != AttachmentUseReceiptSchemaVersionV1 || disposition.Purpose != AttachmentUseDispositionPurposeV1 ||
		!attachmentUseCanonicalSHA256(disposition.DispositionID) || disposition.DispositionID != attachmentUseDispositionIDV1(disposition) ||
		!attachmentUseCanonicalSHA256(disposition.UseID) || !attachmentUseCanonicalSHA256(disposition.ReceiptDigest) ||
		!attachmentUseCanonicalSHA256(disposition.ContextDigest) || disposition.ContextEpoch == 0 ||
		!attachmentUseValidDatasetSnapshotID(disposition.DatasetSnapshotID) || !attachmentUseValidAttachmentID(disposition.AttachmentID) ||
		!attachmentUseCanonicalSHA256(disposition.OwnerDigest) || !attachmentUseCanonicalSHA256(disposition.BlobSHA256) ||
		disposition.BlobByteSize <= 0 || !attachmentUseCanonicalText(disposition.MIMEType) ||
		!attachmentUseCanonicalSHA256(disposition.ProjectionSHA256) || disposition.ProjectionByteSize <= 0 ||
		!attachmentUseCanonicalSHA256(disposition.AttachmentSetDigest) ||
		disposition.BatchCount == 0 || disposition.BatchCount > MaxAttachmentUseSetMembersV1 || disposition.BatchIndex >= disposition.BatchCount ||
		!attachmentUseCanonicalSHA256(disposition.EffectBindingDigest) || !attachmentUseValidConsumer(disposition.ConsumerKind) ||
		!attachmentUseCanonicalSHA256(disposition.ConsumerBindingDigest) || !attachmentUseValidDispositionStatus(disposition.Status) ||
		!attachmentUseReasonCodePattern.MatchString(disposition.ReasonCode) || timeErr != nil || disposedAt.IsZero() ||
		disposedAt.UTC().Format(time.RFC3339Nano) != disposition.DisposedAt || disposition.AuthorityAlgorithm != AttachmentUseAuthorityAlgorithmV1 ||
		!attachmentUseCanonicalSHA256(disposition.AuthorityKeyID) {
		return errors.New("attachment use disposition is incomplete")
	}
	body, err := json.Marshal(disposition)
	if err != nil || len(body) > MaxAttachmentUseRecordBytesV1 {
		return errors.New("attachment use disposition exceeds its canonical bound")
	}
	return nil
}

func attachmentUseContextBindingFromExecutionContextV1(context domainsecurity.TurnSecurityContext) (AttachmentUseContextBindingV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return AttachmentUseContextBindingV1{}, errors.New("attachment use requires current turn effect authority")
	}
	binding := AttachmentUseContextBindingV1{
		ContextVersion: context.Version, ThreadID: context.ThreadID, TurnID: context.TurnID,
		WorkspaceRealPath: context.WorkspaceRealPath, TenantID: context.TenantID, UserID: context.UserID,
		CaseID: context.CaseID, CaseBindingHash: context.CaseBindingHash, DatasetSnapshotID: context.DatasetSnapshotID,
		SourceManifestHash: context.SourceManifestHash, ContextEpoch: context.ContextEpoch,
		ContextIssuedAt: context.IssuedAt, ContextDigest: context.ContextDigest,
	}
	return binding, validateAttachmentUseContextBindingV1(binding)
}

func validateAttachmentUseContextBindingV1(binding AttachmentUseContextBindingV1) error {
	issuedAt, timeErr := time.Parse(time.RFC3339Nano, binding.ContextIssuedAt)
	if binding.ContextVersion != domainsecurity.TurnSecurityContextVersionV2 || !attachmentUseCanonicalText(binding.ThreadID) ||
		!attachmentUseCanonicalText(binding.TurnID) || !attachmentUseCanonicalText(binding.WorkspaceRealPath) ||
		!attachmentUseCanonicalText(binding.TenantID) || !attachmentUseCanonicalText(binding.UserID) ||
		!attachmentUseCanonicalText(binding.CaseID) || !attachmentUseCanonicalSHA256(binding.CaseBindingHash) ||
		!attachmentUseValidDatasetSnapshotID(binding.DatasetSnapshotID) ||
		!attachmentUseCanonicalSHA256(binding.SourceManifestHash) || binding.ContextEpoch == 0 ||
		timeErr != nil || issuedAt.IsZero() || issuedAt.UTC().Format(time.RFC3339Nano) != binding.ContextIssuedAt ||
		!attachmentUseCanonicalSHA256(binding.ContextDigest) {
		return errors.New("attachment use context binding is invalid")
	}
	unboundHash := domainsecurity.UnboundCaseBindingHash(binding.WorkspaceRealPath)
	if binding.CaseID == domainsecurity.UnboundCaseID {
		if binding.CaseBindingHash != unboundHash ||
			binding.DatasetSnapshotID != domainsecurity.NoDatasetSnapshotID ||
			binding.SourceManifestHash != domainsecurity.EmptySourceManifestHash {
			return errors.New("attachment use ordinary context binding is invalid")
		}
	} else if binding.CaseBindingHash == unboundHash ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(binding.DatasetSnapshotID) {
		return errors.New("attachment use case context binding is invalid")
	}
	return nil
}

func validateAttachmentUseOwnersForContextV1(owners []OwnerRecordV1, context domainsecurity.TurnSecurityContext) error {
	if len(owners) == 0 || len(owners) > MaxAttachmentUseSetMembersV1 {
		return errors.New("attachment use owner set count is invalid")
	}
	for _, owner := range owners {
		if !attachmentUseOwnerMatchesContextV1(owner, context) {
			return errors.New("attachment use owner set crosses its frozen turn context")
		}
	}
	return nil
}

// AttachmentUseOwnerMatchesFrozenContextV1 checks immutable owner/context
// relations. It reads no current binding and grants no execution authority.
func AttachmentUseOwnerMatchesFrozenContextV1(owner OwnerRecordV1, context domainsecurity.TurnSecurityContext) bool {
	if ValidateOwnerRecordV1(owner) != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil ||
		owner.ThreadID != context.ThreadID || owner.WorkspaceRealPath != context.WorkspaceRealPath {
		return false
	}
	if owner.CaseBindingObservation == nil {
		return context.PublicationPolicy.CaseBindingState == domainsecurity.CaseBindingStateMissing &&
			context.CaseID == domainsecurity.UnboundCaseID &&
			context.CaseBindingHash == domainsecurity.UnboundCaseBindingHash(context.WorkspaceRealPath) &&
			context.DatasetSnapshotID == domainsecurity.NoDatasetSnapshotID &&
			context.SourceManifestHash == domainsecurity.EmptySourceManifestHash
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil {
		return false
	}
	observation := *owner.CaseBindingObservation
	return domainsecurity.ValidateCaseBindingObservationV1(observation) == nil &&
		observation.State == domainsecurity.CaseBindingStateValid && observation.WorkspaceRealPath == context.WorkspaceRealPath &&
		observation.CaseID == context.CaseID && observation.CaseBindingHash == context.CaseBindingHash
}

func attachmentUseOwnerMatchesContextV1(owner OwnerRecordV1, context domainsecurity.TurnSecurityContext) bool {
	return AttachmentUseOwnerMatchesFrozenContextV1(owner, context)
}

func attachmentUseReceiptPredatesOwnerSetV1(issuedAt time.Time, owners []OwnerRecordV1) bool {
	if issuedAt.IsZero() {
		return true
	}
	for _, owner := range owners {
		createdAt, err := time.Parse(time.RFC3339Nano, owner.CreatedAt)
		if err != nil || issuedAt.Before(createdAt) {
			return true
		}
	}
	return false
}

func attachmentUseSetMembersV1(owners []OwnerRecordV1, projections []AttachmentUseProjectionV1) ([]AttachmentUseSetMemberV1, error) {
	if len(owners) == 0 || len(owners) > MaxAttachmentUseSetMembersV1 || len(projections) != len(owners) {
		return nil, errors.New("attachment use set count is invalid")
	}
	members := make([]AttachmentUseSetMemberV1, 0, len(owners))
	seenAttachments := map[string]bool{}
	seenOwners := map[string]bool{}
	for index, owner := range owners {
		projection := projections[index]
		if ValidateOwnerRecordV1(owner) != nil || owner.ByteSize <= 0 || !attachmentUseCanonicalText(owner.MIMEType) ||
			seenAttachments[owner.AttachmentID] || seenOwners[owner.OwnerDigest] ||
			projection.AttachmentID != owner.AttachmentID || !attachmentUseCanonicalSHA256(projection.ProjectionSHA256) ||
			projection.ProjectionByteSize <= 0 {
			return nil, errors.New("attachment use set member is invalid")
		}
		seenAttachments[owner.AttachmentID] = true
		seenOwners[owner.OwnerDigest] = true
		members = append(members, AttachmentUseSetMemberV1{
			BatchIndex: uint32(index), AttachmentID: owner.AttachmentID, OwnerDigest: owner.OwnerDigest,
			BlobSHA256: owner.BlobSHA256, BlobByteSize: owner.ByteSize, MIMEType: owner.MIMEType,
			ProjectionSHA256: projection.ProjectionSHA256, ProjectionByteSize: projection.ProjectionByteSize,
		})
	}
	return members, nil
}

func attachmentUseSetDigestFromMembersV1(members []AttachmentUseSetMemberV1) string {
	body, _ := json.Marshal(members)
	payload := append([]byte(nil), attachmentUseSetDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func attachmentUseIDV1(receipt AttachmentUseReceiptV1) string {
	identity := struct {
		SchemaVersion         int                           `json:"schemaVersion"`
		Purpose               string                        `json:"purpose"`
		Context               AttachmentUseContextBindingV1 `json:"context"`
		AttachmentID          string                        `json:"attachmentId"`
		OwnerDigest           string                        `json:"ownerDigest"`
		BlobSHA256            string                        `json:"blobSHA256"`
		BlobByteSize          int64                         `json:"blobByteSize"`
		MIMEType              string                        `json:"mimeType"`
		ProjectionSHA256      string                        `json:"projectionSHA256"`
		ProjectionByteSize    int64                         `json:"projectionByteSize"`
		AttachmentSetDigest   string                        `json:"attachmentSetDigest"`
		BatchIndex            uint32                        `json:"batchIndex"`
		BatchCount            uint32                        `json:"batchCount"`
		EffectBindingDigest   string                        `json:"effectBindingDigest"`
		ConsumerKind          string                        `json:"consumerKind"`
		ConsumerBindingDigest string                        `json:"consumerBindingDigest"`
	}{
		receipt.SchemaVersion, receipt.Purpose, receipt.Context, receipt.AttachmentID, receipt.OwnerDigest,
		receipt.BlobSHA256, receipt.BlobByteSize, receipt.MIMEType, receipt.ProjectionSHA256, receipt.ProjectionByteSize,
		receipt.AttachmentSetDigest, receipt.BatchIndex,
		receipt.BatchCount, receipt.EffectBindingDigest, receipt.ConsumerKind, receipt.ConsumerBindingDigest,
	}
	body, _ := json.Marshal(identity)
	payload := append([]byte(nil), attachmentUseIDDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func attachmentUseReceiptDigestV1(receipt AttachmentUseReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	payload := append([]byte(nil), attachmentUseReceiptDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func attachmentUseDispositionIDV1(disposition AttachmentUseDispositionV1) string {
	disposition.DispositionID = ""
	disposition.AuthoritySignature = ""
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	payload := append([]byte(nil), attachmentUseDispositionIDDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func attachmentUseDispositionRecordDigestV1(disposition AttachmentUseDispositionV1) string {
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	payload := append([]byte(nil), attachmentUseDispositionDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func decodeAttachmentUseRecordV1(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxAttachmentUseRecordBytesV1, MaxDepth: 8, MaxTokens: 1024,
		MaxStringBytes: maxAttachmentUseTextBytes,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("attachment use record contains trailing JSON")
	}
	return nil
}

func attachmentUseValidConsumer(value string) bool {
	switch value {
	case AttachmentUseConsumerPrimaryProviderV1, AttachmentUseConsumerVisionBridgeV1,
		AttachmentUseConsumerProviderPipelineV1:
		return true
	default:
		return false
	}
}

func attachmentUseValidDispositionStatus(value string) bool {
	switch value {
	case AttachmentUseDispositionConsumedV1, AttachmentUseDispositionCancelledV1, AttachmentUseDispositionFailedV1,
		AttachmentUseDispositionRejectedV1, AttachmentUseDispositionRestartInvalidV1, AttachmentUseDispositionStaleContextV1:
		return true
	default:
		return false
	}
}

func attachmentUseValidAttachmentID(value string) bool {
	return strings.HasPrefix(value, "att_") && len(value) == len("att_")+24 &&
		attachmentUseCanonicalHex(strings.TrimPrefix(value, "att_"), 24)
}

func attachmentUseValidDatasetSnapshotID(value string) bool {
	return value == domainsecurity.NoDatasetSnapshotID ||
		domainsecurity.IsDatasetSnapshotIDV2Syntax(value)
}

func attachmentUseCanonicalSHA256(value string) bool {
	return value == strings.TrimSpace(value) && value == strings.ToLower(value) && domainsecurity.IsSHA256Hex(value)
}

func attachmentUseCanonicalHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func attachmentUseCanonicalText(value string) bool {
	if value == "" || len(value) > maxAttachmentUseTextBytes || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
