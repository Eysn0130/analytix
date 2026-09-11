package piiauthorization

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ControlledArtifactAccessReceiptSchemaVersionV2 = 2
	ControlledArtifactAccessReceiptPurposeV2       = "analytix.controlled-artifact-access-receipt/v2"
	ControlledArtifactAccessDispositionPurposeV2   = "analytix.controlled-artifact-access-disposition/v2"
	MaxControlledArtifactAccessRecordBytesV2       = MaxControlledArtifactAccessRecordBytesV1

	ControlledArtifactAccessReasonAuthorityUnavailableV2 = "authority_unavailable"
	ControlledArtifactAccessReasonAuthorityIntegrityV2   = "authority_integrity_failure"
)

var (
	controlledArtifactAccessReceiptSigningDomainV2     = []byte("analytix.controlled-artifact-access-receipt/signature/v2\x00")
	controlledArtifactAccessReceiptDigestDomainV2      = []byte("analytix.controlled-artifact-access-receipt/record/v2\x00")
	controlledArtifactAccessIDDomainV2                 = []byte("analytix.controlled-artifact-access/id/v2\x00")
	controlledArtifactAccessDispositionIDDomainV2      = []byte("analytix.controlled-artifact-access-disposition/id/v2\x00")
	controlledArtifactAccessDispositionSigningDomainV2 = []byte("analytix.controlled-artifact-access-disposition/signature/v2\x00")
	controlledArtifactAccessDispositionDigestDomainV2  = []byte("analytix.controlled-artifact-access-disposition/record/v2\x00")
)

// ControlledArtifactAccessReceiptV2 binds one use slot to the exact stable
// projected delivery winner. A publication commit alone is never sufficient
// controlled-release authority. The record contains only opaque identities,
// hashes, counts, and fixed enums; complete PII remains in protected bytes.
type ControlledArtifactAccessReceiptV2 struct {
	SchemaVersion               int              `json:"schemaVersion"`
	Purpose                     string           `json:"purpose"`
	AccessID                    string           `json:"accessId"`
	Context                     ContextBindingV1 `json:"context"`
	RequesterUserID             string           `json:"requesterUserId"`
	AccessAction                string           `json:"accessAction"`
	ControlledHandleDigest      string           `json:"controlledHandleDigest"`
	UseSlotDigest               string           `json:"useSlotDigest"`
	RendererPrincipalDigest     string           `json:"rendererPrincipalDigest"`
	RendererGeneration          uint64           `json:"rendererGeneration"`
	BackendGeneration           uint64           `json:"backendGeneration"`
	AccessPolicyDigest          string           `json:"accessPolicyDigest"`
	RetentionPolicyDigest       string           `json:"retentionPolicyDigest"`
	DeliveryID                  string           `json:"deliveryId"`
	DeliveryOutcomeRecordDigest string           `json:"deliveryOutcomeRecordDigest"`
	PublicationCommitDigest     string           `json:"publicationCommitDigest"`
	PublicationReceiptDigest    string           `json:"publicationReceiptDigest"`
	PIIProjectionDigest         string           `json:"piiProjectionDigest"`
	PIIAuthorizationDigest      string           `json:"piiAuthorizationDigest"`
	ClaimLedgerDigest           string           `json:"claimLedgerDigest"`
	TargetIdentityDigest        string           `json:"targetIdentityDigest"`
	ReleaseTargetIdentityDigest string           `json:"releaseTargetIdentityDigest"`
	ArtifactSHA256              string           `json:"artifactSha256"`
	ArtifactByteLength          uint64           `json:"artifactByteLength"`
	MediaType                   string           `json:"mediaType"`
	RequestedAt                 string           `json:"requestedAt"`
	AuthorizedUntil             string           `json:"authorizedUntil"`
	AuthorityAlgorithm          string           `json:"authorityAlgorithm"`
	AuthorityKeyID              string           `json:"authorityKeyId"`
	AuthorityPublicKey          string           `json:"authorityPublicKey"`
	AuthoritySignature          string           `json:"authoritySignature"`
	RecordDigest                string           `json:"recordDigest"`
}

type ControlledArtifactAccessReceiptInputV2 struct {
	SecurityContext             domainsecurity.TurnSecurityContext
	AccessAction                string
	ControlledHandleDigest      string
	UseSlotDigest               string
	RendererPrincipalDigest     string
	RendererGeneration          uint64
	BackendGeneration           uint64
	AccessPolicyDigest          string
	RetentionPolicyDigest       string
	DeliveryID                  string
	DeliveryOutcomeRecordDigest string
	PublicationCommitDigest     string
	PublicationReceiptDigest    string
	PIIProjectionDigest         string
	PIIAuthorizationDigest      string
	ClaimLedgerDigest           string
	TargetIdentityDigest        string
	ReleaseTargetIdentityDigest string
	ArtifactSHA256              string
	ArtifactByteLength          uint64
	MediaType                   string
	RequestedAt                 time.Time
	AuthorizedUntil             time.Time
	AuthorityKeyID              string
	AuthorityPublicKey          []byte
}

type ControlledArtifactAccessDispositionV2 struct {
	SchemaVersion               int    `json:"schemaVersion"`
	Purpose                     string `json:"purpose"`
	DispositionID               string `json:"dispositionId"`
	AccessID                    string `json:"accessId"`
	AccessReceiptDigest         string `json:"accessReceiptDigest"`
	ContextDigest               string `json:"contextDigest"`
	ContextEpoch                uint64 `json:"contextEpoch"`
	DatasetSnapshotID           string `json:"datasetSnapshotId"`
	AccessAction                string `json:"accessAction"`
	ControlledHandleDigest      string `json:"controlledHandleDigest"`
	UseSlotDigest               string `json:"useSlotDigest"`
	RendererPrincipalDigest     string `json:"rendererPrincipalDigest"`
	RendererGeneration          uint64 `json:"rendererGeneration"`
	BackendGeneration           uint64 `json:"backendGeneration"`
	AccessPolicyDigest          string `json:"accessPolicyDigest"`
	RetentionPolicyDigest       string `json:"retentionPolicyDigest"`
	DeliveryID                  string `json:"deliveryId"`
	DeliveryOutcomeRecordDigest string `json:"deliveryOutcomeRecordDigest"`
	PublicationCommitDigest     string `json:"publicationCommitDigest"`
	PublicationReceiptDigest    string `json:"publicationReceiptDigest"`
	PIIProjectionDigest         string `json:"piiProjectionDigest"`
	PIIAuthorizationDigest      string `json:"piiAuthorizationDigest"`
	ClaimLedgerDigest           string `json:"claimLedgerDigest"`
	TargetIdentityDigest        string `json:"targetIdentityDigest"`
	ReleaseTargetIdentityDigest string `json:"releaseTargetIdentityDigest"`
	ArtifactSHA256              string `json:"artifactSha256"`
	ArtifactByteLength          uint64 `json:"artifactByteLength"`
	MediaType                   string `json:"mediaType"`
	ReleasedByteLength          uint64 `json:"releasedByteLength"`
	Status                      string `json:"status"`
	ReasonCode                  string `json:"reasonCode"`
	DisposedAt                  string `json:"disposedAt"`
	AuthorityAlgorithm          string `json:"authorityAlgorithm"`
	AuthorityKeyID              string `json:"authorityKeyId"`
	AuthorityPublicKey          string `json:"authorityPublicKey"`
	AuthoritySignature          string `json:"authoritySignature"`
	RecordDigest                string `json:"recordDigest"`
}

func NewControlledArtifactAccessReceiptV2(
	input ControlledArtifactAccessReceiptInputV2,
	sign ControlledArtifactAccessSignFuncV1,
) (ControlledArtifactAccessReceiptV2, error) {
	contextBinding, err := contextBindingFromSecurityContextV1(input.SecurityContext)
	if err != nil {
		return ControlledArtifactAccessReceiptV2{}, err
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	receipt := ControlledArtifactAccessReceiptV2{
		SchemaVersion: ControlledArtifactAccessReceiptSchemaVersionV2,
		Purpose:       ControlledArtifactAccessReceiptPurposeV2,
		Context:       contextBinding, RequesterUserID: input.SecurityContext.UserID,
		AccessAction: strings.TrimSpace(input.AccessAction), ControlledHandleDigest: strings.TrimSpace(input.ControlledHandleDigest),
		UseSlotDigest: strings.TrimSpace(input.UseSlotDigest), RendererPrincipalDigest: strings.TrimSpace(input.RendererPrincipalDigest),
		RendererGeneration: input.RendererGeneration, BackendGeneration: input.BackendGeneration,
		AccessPolicyDigest: strings.TrimSpace(input.AccessPolicyDigest), RetentionPolicyDigest: strings.TrimSpace(input.RetentionPolicyDigest),
		DeliveryID: strings.TrimSpace(input.DeliveryID), DeliveryOutcomeRecordDigest: strings.TrimSpace(input.DeliveryOutcomeRecordDigest),
		PublicationCommitDigest: strings.TrimSpace(input.PublicationCommitDigest), PublicationReceiptDigest: strings.TrimSpace(input.PublicationReceiptDigest),
		PIIProjectionDigest: strings.TrimSpace(input.PIIProjectionDigest), PIIAuthorizationDigest: strings.TrimSpace(input.PIIAuthorizationDigest),
		ClaimLedgerDigest: strings.TrimSpace(input.ClaimLedgerDigest), TargetIdentityDigest: strings.TrimSpace(input.TargetIdentityDigest),
		ReleaseTargetIdentityDigest: strings.TrimSpace(input.ReleaseTargetIdentityDigest),
		ArtifactSHA256:              strings.TrimSpace(input.ArtifactSHA256), ArtifactByteLength: input.ArtifactByteLength,
		MediaType: strings.TrimSpace(input.MediaType), RequestedAt: input.RequestedAt.UTC().Format(time.RFC3339Nano),
		AuthorizedUntil: input.AuthorizedUntil.UTC().Format(time.RFC3339Nano), AuthorityAlgorithm: ControlledArtifactAccessAuthorityAlgorithmV1,
		AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.AccessID = controlledArtifactAccessIDV2(receipt)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ControlledArtifactAccessReceiptV2{}, errors.New("controlled artifact access V2 signing authority is invalid")
	}
	if err := validateControlledArtifactAccessReceiptUnsignedV2(receipt); err != nil {
		return ControlledArtifactAccessReceiptV2{}, err
	}
	signature, err := sign(ControlledArtifactAccessReceiptSigningBytesV2(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ControlledArtifactAccessReceiptV2{}, errors.New("controlled artifact access V2 receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.RecordDigest = controlledArtifactAccessReceiptRecordDigestV2(receipt)
	return receipt, ValidateControlledArtifactAccessReceiptV2(receipt)
}

func NewControlledArtifactAccessDispositionV2(
	receipt ControlledArtifactAccessReceiptV2,
	status string,
	reasonCode string,
	releasedByteLength uint64,
	disposedAt time.Time,
	keyID string,
	publicKey []byte,
	sign ControlledArtifactAccessSignFuncV1,
) (ControlledArtifactAccessDispositionV2, error) {
	if err := ValidateControlledArtifactAccessReceiptV2(receipt); err != nil {
		return ControlledArtifactAccessDispositionV2{}, err
	}
	publicKey = append([]byte(nil), publicKey...)
	disposition := ControlledArtifactAccessDispositionV2{
		SchemaVersion: ControlledArtifactAccessReceiptSchemaVersionV2, Purpose: ControlledArtifactAccessDispositionPurposeV2,
		AccessID: receipt.AccessID, AccessReceiptDigest: receipt.RecordDigest,
		ContextDigest: receipt.Context.ContextDigest, ContextEpoch: receipt.Context.ContextEpoch, DatasetSnapshotID: receipt.Context.DatasetSnapshotID,
		AccessAction: receipt.AccessAction, ControlledHandleDigest: receipt.ControlledHandleDigest, UseSlotDigest: receipt.UseSlotDigest,
		RendererPrincipalDigest: receipt.RendererPrincipalDigest, RendererGeneration: receipt.RendererGeneration, BackendGeneration: receipt.BackendGeneration,
		AccessPolicyDigest: receipt.AccessPolicyDigest, RetentionPolicyDigest: receipt.RetentionPolicyDigest,
		DeliveryID: receipt.DeliveryID, DeliveryOutcomeRecordDigest: receipt.DeliveryOutcomeRecordDigest,
		PublicationCommitDigest: receipt.PublicationCommitDigest, PublicationReceiptDigest: receipt.PublicationReceiptDigest,
		PIIProjectionDigest: receipt.PIIProjectionDigest, PIIAuthorizationDigest: receipt.PIIAuthorizationDigest,
		ClaimLedgerDigest: receipt.ClaimLedgerDigest, TargetIdentityDigest: receipt.TargetIdentityDigest,
		ReleaseTargetIdentityDigest: receipt.ReleaseTargetIdentityDigest,
		ArtifactSHA256:              receipt.ArtifactSHA256, ArtifactByteLength: receipt.ArtifactByteLength, MediaType: receipt.MediaType,
		ReleasedByteLength: releasedByteLength, Status: strings.TrimSpace(status), ReasonCode: strings.TrimSpace(reasonCode),
		DisposedAt: disposedAt.UTC().Format(time.RFC3339Nano), AuthorityAlgorithm: ControlledArtifactAccessAuthorityAlgorithmV1,
		AuthorityKeyID: strings.TrimSpace(keyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	disposition.DispositionID = controlledArtifactAccessDispositionIDV2(disposition)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || disposition.AuthorityKeyID != receipt.AuthorityKeyID ||
		disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) || disposition.AuthorityPublicKey != receipt.AuthorityPublicKey {
		return ControlledArtifactAccessDispositionV2{}, errors.New("controlled artifact access V2 disposition authority is invalid")
	}
	if err := validateControlledArtifactAccessDispositionUnsignedV2(disposition); err != nil {
		return ControlledArtifactAccessDispositionV2{}, err
	}
	signature, err := sign(ControlledArtifactAccessDispositionSigningBytesV2(disposition))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ControlledArtifactAccessDispositionV2{}, errors.New("controlled artifact access V2 disposition signing failed")
	}
	disposition.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	disposition.RecordDigest = controlledArtifactAccessDispositionRecordDigestV2(disposition)
	return disposition, ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt)
}

func ValidateControlledArtifactAccessReceiptV2(receipt ControlledArtifactAccessReceiptV2) error {
	if err := validateControlledArtifactAccessReceiptUnsignedV2(receipt); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != receipt.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != receipt.AuthoritySignature ||
		receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ControlledArtifactAccessReceiptSigningBytesV2(receipt), signature) ||
		!canonicalControlledAccessSHA256V1(receipt.RecordDigest) || receipt.RecordDigest != controlledArtifactAccessReceiptRecordDigestV2(receipt) {
		return errors.New("controlled artifact access V2 receipt signature is invalid")
	}
	return nil
}

func ValidateControlledArtifactAccessReceiptForGrantV2(
	receipt ControlledArtifactAccessReceiptV2,
	grant PIIProjectionGrantV1,
) error {
	requestedAt, requestErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, authorizedErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	grantIssuedAt, grantIssueErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	grantExpiresAt, grantExpiryErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if ValidateControlledArtifactAccessReceiptV2(receipt) != nil || ValidatePIIProjectionGrantV1(grant) != nil ||
		receipt.Context != grant.Context || receipt.RequesterUserID != grant.RequesterUserID ||
		receipt.PIIAuthorizationDigest != grant.RecordDigest || receipt.ClaimLedgerDigest != grant.ClaimLedgerDigest ||
		receipt.TargetIdentityDigest != grant.TargetIdentityDigest || receipt.ArtifactSHA256 != grant.ProjectedContentSHA256 ||
		receipt.AccessPolicyDigest != grant.AccessPolicyDigest || receipt.RetentionPolicyDigest != grant.RetentionPolicyDigest ||
		!controlledArtifactAccessActionAllowedV1(receipt.AccessAction, grant.AllowedAccessActions) ||
		requestErr != nil || authorizedErr != nil || grantIssueErr != nil || grantExpiryErr != nil ||
		requestedAt.Before(grantIssuedAt) || !requestedAt.Before(grantExpiresAt) || authorizedUntil.After(grantExpiresAt) {
		return errors.New("controlled artifact access V2 receipt exceeds its PII grant")
	}
	return nil
}

func ValidateControlledArtifactAccessDispositionV2(disposition ControlledArtifactAccessDispositionV2) error {
	if err := validateControlledArtifactAccessDispositionUnsignedV2(disposition); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != disposition.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != disposition.AuthoritySignature ||
		disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ControlledArtifactAccessDispositionSigningBytesV2(disposition), signature) ||
		!canonicalControlledAccessSHA256V1(disposition.RecordDigest) || disposition.RecordDigest != controlledArtifactAccessDispositionRecordDigestV2(disposition) {
		return errors.New("controlled artifact access V2 disposition signature is invalid")
	}
	return nil
}

func ValidateControlledArtifactAccessDispositionForReceiptV2(
	disposition ControlledArtifactAccessDispositionV2,
	receipt ControlledArtifactAccessReceiptV2,
) error {
	requestedAt, requestedErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, authorizedErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if ValidateControlledArtifactAccessReceiptV2(receipt) != nil || ValidateControlledArtifactAccessDispositionV2(disposition) != nil ||
		disposition.AccessID != receipt.AccessID || disposition.AccessReceiptDigest != receipt.RecordDigest ||
		disposition.ContextDigest != receipt.Context.ContextDigest || disposition.ContextEpoch != receipt.Context.ContextEpoch ||
		disposition.DatasetSnapshotID != receipt.Context.DatasetSnapshotID || disposition.AccessAction != receipt.AccessAction ||
		disposition.ControlledHandleDigest != receipt.ControlledHandleDigest || disposition.UseSlotDigest != receipt.UseSlotDigest ||
		disposition.RendererPrincipalDigest != receipt.RendererPrincipalDigest || disposition.RendererGeneration != receipt.RendererGeneration ||
		disposition.BackendGeneration != receipt.BackendGeneration || disposition.AccessPolicyDigest != receipt.AccessPolicyDigest ||
		disposition.RetentionPolicyDigest != receipt.RetentionPolicyDigest || disposition.DeliveryID != receipt.DeliveryID ||
		disposition.DeliveryOutcomeRecordDigest != receipt.DeliveryOutcomeRecordDigest || disposition.PublicationCommitDigest != receipt.PublicationCommitDigest ||
		disposition.PublicationReceiptDigest != receipt.PublicationReceiptDigest || disposition.PIIProjectionDigest != receipt.PIIProjectionDigest ||
		disposition.PIIAuthorizationDigest != receipt.PIIAuthorizationDigest || disposition.ClaimLedgerDigest != receipt.ClaimLedgerDigest ||
		disposition.TargetIdentityDigest != receipt.TargetIdentityDigest ||
		disposition.ReleaseTargetIdentityDigest != receipt.ReleaseTargetIdentityDigest ||
		disposition.ArtifactSHA256 != receipt.ArtifactSHA256 ||
		disposition.ArtifactByteLength != receipt.ArtifactByteLength || disposition.MediaType != receipt.MediaType ||
		disposition.AuthorityKeyID != receipt.AuthorityKeyID || disposition.AuthorityPublicKey != receipt.AuthorityPublicKey ||
		requestedErr != nil || authorizedErr != nil || disposedErr != nil || disposedAt.Before(requestedAt) ||
		disposition.Status == ControlledArtifactAccessDispositionHostReleaseCommittedV1 && !disposedAt.Before(authorizedUntil) {
		return errors.New("controlled artifact access V2 disposition does not close its exact receipt")
	}
	return nil
}

func ControlledArtifactAccessReceiptV2Bytes(receipt ControlledArtifactAccessReceiptV2) ([]byte, error) {
	if err := ValidateControlledArtifactAccessReceiptV2(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func ControlledArtifactAccessDispositionV2Bytes(disposition ControlledArtifactAccessDispositionV2) ([]byte, error) {
	if err := ValidateControlledArtifactAccessDispositionV2(disposition); err != nil {
		return nil, err
	}
	return json.Marshal(disposition)
}

func ParseControlledArtifactAccessReceiptV2(body []byte) (ControlledArtifactAccessReceiptV2, error) {
	var receipt ControlledArtifactAccessReceiptV2
	if err := parseControlledArtifactAccessRecordV1(body, &receipt); err != nil {
		return ControlledArtifactAccessReceiptV2{}, err
	}
	return receipt, ValidateControlledArtifactAccessReceiptV2(receipt)
}

func ParseControlledArtifactAccessDispositionV2(body []byte) (ControlledArtifactAccessDispositionV2, error) {
	var disposition ControlledArtifactAccessDispositionV2
	if err := parseControlledArtifactAccessRecordV1(body, &disposition); err != nil {
		return ControlledArtifactAccessDispositionV2{}, err
	}
	return disposition, ValidateControlledArtifactAccessDispositionV2(disposition)
}

func ControlledArtifactAccessReceiptSigningBytesV2(receipt ControlledArtifactAccessReceiptV2) []byte {
	receipt.AuthoritySignature = ""
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), controlledArtifactAccessReceiptSigningDomainV2...), digest[:]...)
}

func ControlledArtifactAccessDispositionSigningBytesV2(disposition ControlledArtifactAccessDispositionV2) []byte {
	disposition.AuthoritySignature = ""
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), controlledArtifactAccessDispositionSigningDomainV2...), digest[:]...)
}

func ControlledArtifactAccessReceiptAuthorityMaterialV2(receipt ControlledArtifactAccessReceiptV2) (string, []byte, []byte, error) {
	if err := ValidateControlledArtifactAccessReceiptV2(receipt); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	return receipt.AuthorityKeyID, publicKey, signature, nil
}

func ControlledArtifactAccessDispositionAuthorityMaterialV2(disposition ControlledArtifactAccessDispositionV2) (string, []byte, []byte, error) {
	if err := ValidateControlledArtifactAccessDispositionV2(disposition); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	return disposition.AuthorityKeyID, publicKey, signature, nil
}

func validateControlledArtifactAccessReceiptUnsignedV2(receipt ControlledArtifactAccessReceiptV2) error {
	requestedAt, requestErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, expiryErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	contextIssuedAt, contextErr := time.Parse(time.RFC3339Nano, receipt.Context.ContextIssuedAt)
	if receipt.SchemaVersion != ControlledArtifactAccessReceiptSchemaVersionV2 || receipt.Purpose != ControlledArtifactAccessReceiptPurposeV2 ||
		!canonicalControlledAccessSHA256V1(receipt.AccessID) || receipt.AccessID != controlledArtifactAccessIDV2(receipt) ||
		!validContextBindingV1(receipt.Context) || receipt.RequesterUserID != receipt.Context.UserID || !validControlledArtifactAccessActionV1(receipt.AccessAction) ||
		!canonicalControlledAccessSHA256V1(receipt.ControlledHandleDigest) || !canonicalControlledAccessSHA256V1(receipt.UseSlotDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.RendererPrincipalDigest) || receipt.RendererGeneration == 0 || receipt.BackendGeneration == 0 ||
		!canonicalControlledAccessSHA256V1(receipt.AccessPolicyDigest) || !canonicalControlledAccessSHA256V1(receipt.RetentionPolicyDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.DeliveryID) || !canonicalControlledAccessSHA256V1(receipt.DeliveryOutcomeRecordDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.PublicationCommitDigest) || !canonicalControlledAccessSHA256V1(receipt.PublicationReceiptDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.PIIProjectionDigest) || !canonicalControlledAccessSHA256V1(receipt.PIIAuthorizationDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.ClaimLedgerDigest) || !canonicalControlledAccessSHA256V1(receipt.TargetIdentityDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.ReleaseTargetIdentityDigest) ||
		receipt.ReleaseTargetIdentityDigest == receipt.TargetIdentityDigest ||
		!canonicalControlledAccessSHA256V1(receipt.ArtifactSHA256) || receipt.ArtifactByteLength == 0 ||
		receipt.ArtifactByteLength > MaxControlledPIIArtifactBytesV1 || receipt.MediaType != ControlledPIIArtifactMediaTypeV1 ||
		requestErr != nil || expiryErr != nil || contextErr != nil || requestedAt.IsZero() || authorizedUntil.IsZero() ||
		requestedAt.Before(contextIssuedAt) || !requestedAt.Before(authorizedUntil) || authorizedUntil.Sub(requestedAt) > PIIProjectionGrantMaxTTL ||
		requestedAt.UTC().Format(time.RFC3339Nano) != receipt.RequestedAt || authorizedUntil.UTC().Format(time.RFC3339Nano) != receipt.AuthorizedUntil ||
		receipt.AuthorityAlgorithm != ControlledArtifactAccessAuthorityAlgorithmV1 || !canonicalControlledAccessSHA256V1(receipt.AuthorityKeyID) ||
		strings.TrimSpace(receipt.AuthorityPublicKey) == "" {
		return errors.New("controlled artifact access V2 receipt is incomplete")
	}
	return nil
}

func validateControlledArtifactAccessDispositionUnsignedV2(disposition ControlledArtifactAccessDispositionV2) error {
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if disposition.SchemaVersion != ControlledArtifactAccessReceiptSchemaVersionV2 || disposition.Purpose != ControlledArtifactAccessDispositionPurposeV2 ||
		!canonicalControlledAccessSHA256V1(disposition.DispositionID) || disposition.DispositionID != controlledArtifactAccessDispositionIDV2(disposition) ||
		!canonicalControlledAccessSHA256V1(disposition.AccessID) || !canonicalControlledAccessSHA256V1(disposition.AccessReceiptDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.ContextDigest) || disposition.ContextEpoch == 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(disposition.DatasetSnapshotID) ||
		!validControlledArtifactAccessActionV1(disposition.AccessAction) ||
		!canonicalControlledAccessSHA256V1(disposition.ControlledHandleDigest) || !canonicalControlledAccessSHA256V1(disposition.UseSlotDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.RendererPrincipalDigest) || disposition.RendererGeneration == 0 || disposition.BackendGeneration == 0 ||
		!canonicalControlledAccessSHA256V1(disposition.AccessPolicyDigest) || !canonicalControlledAccessSHA256V1(disposition.RetentionPolicyDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.DeliveryID) || !canonicalControlledAccessSHA256V1(disposition.DeliveryOutcomeRecordDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.PublicationCommitDigest) || !canonicalControlledAccessSHA256V1(disposition.PublicationReceiptDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.PIIProjectionDigest) || !canonicalControlledAccessSHA256V1(disposition.PIIAuthorizationDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.ClaimLedgerDigest) || !canonicalControlledAccessSHA256V1(disposition.TargetIdentityDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.ReleaseTargetIdentityDigest) ||
		disposition.ReleaseTargetIdentityDigest == disposition.TargetIdentityDigest ||
		!canonicalControlledAccessSHA256V1(disposition.ArtifactSHA256) || disposition.ArtifactByteLength == 0 ||
		disposition.ArtifactByteLength > MaxControlledPIIArtifactBytesV1 || disposition.MediaType != ControlledPIIArtifactMediaTypeV1 ||
		disposition.ReleasedByteLength > disposition.ArtifactByteLength || !validControlledArtifactAccessDispositionStatusV1(disposition.Status) ||
		!validControlledArtifactAccessReasonV2(disposition.Status, disposition.ReasonCode) || disposedErr != nil || disposedAt.IsZero() ||
		disposedAt.UTC().Format(time.RFC3339Nano) != disposition.DisposedAt ||
		!validControlledArtifactAccessByteStateV2(disposition.Status, disposition.ReleasedByteLength, disposition.ArtifactByteLength) ||
		disposition.AuthorityAlgorithm != ControlledArtifactAccessAuthorityAlgorithmV1 || !canonicalControlledAccessSHA256V1(disposition.AuthorityKeyID) ||
		strings.TrimSpace(disposition.AuthorityPublicKey) == "" {
		return errors.New("controlled artifact access V2 disposition is incomplete")
	}
	return nil
}

func validControlledArtifactAccessReasonV2(status string, reason string) bool {
	if status == ControlledArtifactAccessDispositionFailedV1 {
		switch reason {
		case ControlledArtifactAccessReasonHostReleaseFailedV1,
			ControlledArtifactAccessReasonAuthorityUnavailableV2,
			ControlledArtifactAccessReasonAuthorityIntegrityV2:
			return true
		default:
			return false
		}
	}
	return validControlledArtifactAccessReasonV1(status, reason)
}

func validControlledArtifactAccessByteStateV2(status string, released uint64, artifact uint64) bool {
	switch status {
	case ControlledArtifactAccessDispositionHostReleaseCommittedV1,
		ControlledArtifactAccessDispositionReleaseIndeterminateV1:
		return released == artifact
	case ControlledArtifactAccessDispositionFailedV1,
		ControlledArtifactAccessDispositionRejectedV1,
		ControlledArtifactAccessDispositionCancelledV1,
		ControlledArtifactAccessDispositionRestartInvalidV1,
		ControlledArtifactAccessDispositionStaleContextV1:
		return released == 0
	default:
		return false
	}
}

func controlledArtifactAccessIDV2(receipt ControlledArtifactAccessReceiptV2) string {
	return domainsecurity.SHA256Hex(append(append([]byte(nil), controlledArtifactAccessIDDomainV2...), receipt.UseSlotDigest...))
}

func controlledArtifactAccessReceiptRecordDigestV2(receipt ControlledArtifactAccessReceiptV2) string {
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), controlledArtifactAccessReceiptDigestDomainV2...), body...))
}

func controlledArtifactAccessDispositionIDV2(disposition ControlledArtifactAccessDispositionV2) string {
	disposition.DispositionID = ""
	disposition.AuthorityAlgorithm = ""
	disposition.AuthorityKeyID = ""
	disposition.AuthorityPublicKey = ""
	disposition.AuthoritySignature = ""
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), controlledArtifactAccessDispositionIDDomainV2...), body...))
}

func controlledArtifactAccessDispositionRecordDigestV2(disposition ControlledArtifactAccessDispositionV2) string {
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), controlledArtifactAccessDispositionDigestDomainV2...), body...))
}
