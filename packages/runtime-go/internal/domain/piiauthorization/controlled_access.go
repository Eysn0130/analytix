package piiauthorization

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
	ControlledArtifactAccessReceiptSchemaVersionV1 = 1
	ControlledArtifactAccessReceiptPurposeV1       = "analytix.controlled-artifact-access-receipt/v1"
	ControlledArtifactAccessDispositionPurposeV1   = "analytix.controlled-artifact-access-disposition/v1"
	ControlledArtifactAccessAuthorityAlgorithmV1   = "Ed25519"

	ControlledArtifactAccessActionDisplayV1 = "display"
	ControlledArtifactAccessActionExportV1  = "export"

	ControlledArtifactAccessDispositionHostReleaseCommittedV1 = "host_release_committed"
	ControlledArtifactAccessDispositionReleaseIndeterminateV1 = "release_indeterminate"
	ControlledArtifactAccessDispositionFailedV1               = "failed"
	ControlledArtifactAccessDispositionRejectedV1             = "rejected"
	ControlledArtifactAccessDispositionCancelledV1            = "cancelled"
	ControlledArtifactAccessDispositionRestartInvalidV1       = "restart_invalid"
	ControlledArtifactAccessDispositionStaleContextV1         = "stale_context"

	ControlledArtifactAccessReasonHostReleaseCommittedV1 = "host_release_committed"
	ControlledArtifactAccessReasonReleaseIndeterminateV1 = "release_indeterminate"
	ControlledArtifactAccessReasonHostReleaseFailedV1    = "host_release_failed"
	ControlledArtifactAccessReasonAccessRejectedV1       = "access_rejected"
	ControlledArtifactAccessReasonAccessCancelledV1      = "access_cancelled"
	ControlledArtifactAccessReasonRestartInvalidV1       = "restart_invalid"
	ControlledArtifactAccessReasonStaleContextV1         = "stale_context"

	MaxControlledArtifactAccessRecordBytesV1 = 256 * 1024
)

var (
	controlledArtifactAccessReceiptSigningDomainV1     = []byte("analytix.controlled-artifact-access-receipt/signature/v1\x00")
	controlledArtifactAccessReceiptDigestDomainV1      = []byte("analytix.controlled-artifact-access-receipt/record/v1\x00")
	controlledArtifactAccessIDDomainV1                 = []byte("analytix.controlled-artifact-access/id/v1\x00")
	controlledArtifactAccessDispositionIDDomainV1      = []byte("analytix.controlled-artifact-access-disposition/id/v1\x00")
	controlledArtifactAccessDispositionSigningDomainV1 = []byte("analytix.controlled-artifact-access-disposition/signature/v1\x00")
	controlledArtifactAccessDispositionDigestDomainV1  = []byte("analytix.controlled-artifact-access-disposition/record/v1\x00")
)

// ControlledArtifactAccessReceiptV1 is private host authority for one exact
// access to already-published controlled bytes. It contains only hashes and
// bounded metadata; complete account/card values remain solely in the
// protected artifact store and the in-memory response body.
type ControlledArtifactAccessReceiptV1 struct {
	SchemaVersion            int              `json:"schemaVersion"`
	Purpose                  string           `json:"purpose"`
	AccessID                 string           `json:"accessId"`
	Context                  ContextBindingV1 `json:"context"`
	RequesterUserID          string           `json:"requesterUserId"`
	AccessAction             string           `json:"accessAction"`
	ControlledHandleDigest   string           `json:"controlledHandleDigest"`
	UseSlotDigest            string           `json:"useSlotDigest"`
	RendererPrincipalDigest  string           `json:"rendererPrincipalDigest"`
	RendererGeneration       uint64           `json:"rendererGeneration"`
	BackendGeneration        uint64           `json:"backendGeneration"`
	AccessPolicyDigest       string           `json:"accessPolicyDigest"`
	RetentionPolicyDigest    string           `json:"retentionPolicyDigest"`
	PublicationCommitDigest  string           `json:"publicationCommitDigest"`
	PublicationReceiptDigest string           `json:"publicationReceiptDigest"`
	PIIProjectionDigest      string           `json:"piiProjectionDigest"`
	PIIAuthorizationDigest   string           `json:"piiAuthorizationDigest"`
	ClaimLedgerDigest        string           `json:"claimLedgerDigest"`
	TargetIdentityDigest     string           `json:"targetIdentityDigest"`
	ArtifactSHA256           string           `json:"artifactSha256"`
	ArtifactByteLength       uint64           `json:"artifactByteLength"`
	MediaType                string           `json:"mediaType"`
	RequestedAt              string           `json:"requestedAt"`
	AuthorizedUntil          string           `json:"authorizedUntil"`
	AuthorityAlgorithm       string           `json:"authorityAlgorithm"`
	AuthorityKeyID           string           `json:"authorityKeyId"`
	AuthorityPublicKey       string           `json:"authorityPublicKey"`
	AuthoritySignature       string           `json:"authoritySignature"`
	RecordDigest             string           `json:"recordDigest"`
}

type ControlledArtifactAccessReceiptInputV1 struct {
	SecurityContext          domainsecurity.TurnSecurityContext
	AccessAction             string
	ControlledHandleDigest   string
	UseSlotDigest            string
	RendererPrincipalDigest  string
	RendererGeneration       uint64
	BackendGeneration        uint64
	AccessPolicyDigest       string
	RetentionPolicyDigest    string
	PublicationCommitDigest  string
	PublicationReceiptDigest string
	PIIProjectionDigest      string
	PIIAuthorizationDigest   string
	ClaimLedgerDigest        string
	TargetIdentityDigest     string
	ArtifactSHA256           string
	ArtifactByteLength       uint64
	MediaType                string
	RequestedAt              time.Time
	AuthorizedUntil          time.Time
	AuthorityKeyID           string
	AuthorityPublicKey       []byte
}

// ControlledArtifactAccessDispositionV1 closes an access receipt. Stores key
// dispositions by AccessID so a logical access can have only one outcome.
type ControlledArtifactAccessDispositionV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	DispositionID            string `json:"dispositionId"`
	AccessID                 string `json:"accessId"`
	AccessReceiptDigest      string `json:"accessReceiptDigest"`
	ContextDigest            string `json:"contextDigest"`
	ContextEpoch             uint64 `json:"contextEpoch"`
	DatasetSnapshotID        string `json:"datasetSnapshotId"`
	AccessAction             string `json:"accessAction"`
	ControlledHandleDigest   string `json:"controlledHandleDigest"`
	UseSlotDigest            string `json:"useSlotDigest"`
	RendererPrincipalDigest  string `json:"rendererPrincipalDigest"`
	RendererGeneration       uint64 `json:"rendererGeneration"`
	BackendGeneration        uint64 `json:"backendGeneration"`
	AccessPolicyDigest       string `json:"accessPolicyDigest"`
	RetentionPolicyDigest    string `json:"retentionPolicyDigest"`
	PublicationCommitDigest  string `json:"publicationCommitDigest"`
	PublicationReceiptDigest string `json:"publicationReceiptDigest"`
	PIIProjectionDigest      string `json:"piiProjectionDigest"`
	PIIAuthorizationDigest   string `json:"piiAuthorizationDigest"`
	ClaimLedgerDigest        string `json:"claimLedgerDigest"`
	TargetIdentityDigest     string `json:"targetIdentityDigest"`
	ArtifactSHA256           string `json:"artifactSha256"`
	ArtifactByteLength       uint64 `json:"artifactByteLength"`
	MediaType                string `json:"mediaType"`
	ReleasedByteLength       uint64 `json:"releasedByteLength"`
	Status                   string `json:"status"`
	ReasonCode               string `json:"reasonCode"`
	DisposedAt               string `json:"disposedAt"`
	AuthorityAlgorithm       string `json:"authorityAlgorithm"`
	AuthorityKeyID           string `json:"authorityKeyId"`
	AuthorityPublicKey       string `json:"authorityPublicKey"`
	AuthoritySignature       string `json:"authoritySignature"`
	RecordDigest             string `json:"recordDigest"`
}

type ControlledArtifactAccessSignFuncV1 func([]byte) ([]byte, error)

func NewControlledArtifactAccessReceiptV1(
	input ControlledArtifactAccessReceiptInputV1,
	sign ControlledArtifactAccessSignFuncV1,
) (ControlledArtifactAccessReceiptV1, error) {
	contextBinding, err := contextBindingFromSecurityContextV1(input.SecurityContext)
	if err != nil {
		return ControlledArtifactAccessReceiptV1{}, err
	}
	requestedAt := input.RequestedAt.UTC()
	authorizedUntil := input.AuthorizedUntil.UTC()
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	receipt := ControlledArtifactAccessReceiptV1{
		SchemaVersion: ControlledArtifactAccessReceiptSchemaVersionV1,
		Purpose:       ControlledArtifactAccessReceiptPurposeV1,
		Context:       contextBinding, RequesterUserID: input.SecurityContext.UserID,
		AccessAction:           strings.TrimSpace(input.AccessAction),
		ControlledHandleDigest: strings.TrimSpace(input.ControlledHandleDigest), UseSlotDigest: strings.TrimSpace(input.UseSlotDigest),
		RendererPrincipalDigest: strings.TrimSpace(input.RendererPrincipalDigest), RendererGeneration: input.RendererGeneration,
		BackendGeneration: input.BackendGeneration, AccessPolicyDigest: strings.TrimSpace(input.AccessPolicyDigest),
		RetentionPolicyDigest:    strings.TrimSpace(input.RetentionPolicyDigest),
		PublicationCommitDigest:  strings.TrimSpace(input.PublicationCommitDigest),
		PublicationReceiptDigest: strings.TrimSpace(input.PublicationReceiptDigest),
		PIIProjectionDigest:      strings.TrimSpace(input.PIIProjectionDigest),
		PIIAuthorizationDigest:   strings.TrimSpace(input.PIIAuthorizationDigest),
		ClaimLedgerDigest:        strings.TrimSpace(input.ClaimLedgerDigest),
		TargetIdentityDigest:     strings.TrimSpace(input.TargetIdentityDigest),
		ArtifactSHA256:           strings.TrimSpace(input.ArtifactSHA256), ArtifactByteLength: input.ArtifactByteLength,
		MediaType: strings.TrimSpace(input.MediaType), RequestedAt: requestedAt.Format(time.RFC3339Nano),
		AuthorizedUntil: authorizedUntil.Format(time.RFC3339Nano), AuthorityAlgorithm: ControlledArtifactAccessAuthorityAlgorithmV1,
		AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.AccessID = controlledArtifactAccessIDV1(receipt)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ControlledArtifactAccessReceiptV1{}, errors.New("controlled artifact access signing authority is invalid")
	}
	if err := validateControlledArtifactAccessReceiptUnsignedV1(receipt); err != nil {
		return ControlledArtifactAccessReceiptV1{}, err
	}
	signature, err := sign(ControlledArtifactAccessReceiptSigningBytesV1(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ControlledArtifactAccessReceiptV1{}, errors.New("controlled artifact access receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.RecordDigest = controlledArtifactAccessReceiptRecordDigestV1(receipt)
	if err := ValidateControlledArtifactAccessReceiptV1(receipt); err != nil {
		return ControlledArtifactAccessReceiptV1{}, err
	}
	return receipt, nil
}

func NewControlledArtifactAccessDispositionV1(
	receipt ControlledArtifactAccessReceiptV1,
	status string,
	reasonCode string,
	releasedByteLength uint64,
	disposedAt time.Time,
	keyID string,
	publicKey []byte,
	sign ControlledArtifactAccessSignFuncV1,
) (ControlledArtifactAccessDispositionV1, error) {
	if err := ValidateControlledArtifactAccessReceiptV1(receipt); err != nil {
		return ControlledArtifactAccessDispositionV1{}, err
	}
	disposedAt = disposedAt.UTC()
	publicKey = append([]byte(nil), publicKey...)
	disposition := ControlledArtifactAccessDispositionV1{
		SchemaVersion: ControlledArtifactAccessReceiptSchemaVersionV1,
		Purpose:       ControlledArtifactAccessDispositionPurposeV1,
		AccessID:      receipt.AccessID, AccessReceiptDigest: receipt.RecordDigest,
		ContextDigest: receipt.Context.ContextDigest, ContextEpoch: receipt.Context.ContextEpoch,
		DatasetSnapshotID: receipt.Context.DatasetSnapshotID, AccessAction: receipt.AccessAction,
		ControlledHandleDigest: receipt.ControlledHandleDigest, UseSlotDigest: receipt.UseSlotDigest,
		RendererPrincipalDigest: receipt.RendererPrincipalDigest, RendererGeneration: receipt.RendererGeneration,
		BackendGeneration: receipt.BackendGeneration, AccessPolicyDigest: receipt.AccessPolicyDigest,
		RetentionPolicyDigest:   receipt.RetentionPolicyDigest,
		PublicationCommitDigest: receipt.PublicationCommitDigest, PublicationReceiptDigest: receipt.PublicationReceiptDigest,
		PIIProjectionDigest: receipt.PIIProjectionDigest, PIIAuthorizationDigest: receipt.PIIAuthorizationDigest,
		ClaimLedgerDigest: receipt.ClaimLedgerDigest, TargetIdentityDigest: receipt.TargetIdentityDigest,
		ArtifactSHA256: receipt.ArtifactSHA256, ArtifactByteLength: receipt.ArtifactByteLength,
		MediaType: receipt.MediaType, ReleasedByteLength: releasedByteLength,
		Status: strings.TrimSpace(status), ReasonCode: strings.TrimSpace(reasonCode),
		DisposedAt: disposedAt.Format(time.RFC3339Nano), AuthorityAlgorithm: ControlledArtifactAccessAuthorityAlgorithmV1,
		AuthorityKeyID: strings.TrimSpace(keyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	disposition.DispositionID = controlledArtifactAccessDispositionIDV1(disposition)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || disposition.AuthorityKeyID != receipt.AuthorityKeyID ||
		disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) || disposition.AuthorityPublicKey != receipt.AuthorityPublicKey {
		return ControlledArtifactAccessDispositionV1{}, errors.New("controlled artifact access disposition signing authority is invalid")
	}
	if err := validateControlledArtifactAccessDispositionUnsignedV1(disposition); err != nil {
		return ControlledArtifactAccessDispositionV1{}, err
	}
	signature, err := sign(ControlledArtifactAccessDispositionSigningBytesV1(disposition))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ControlledArtifactAccessDispositionV1{}, errors.New("controlled artifact access disposition signing failed")
	}
	disposition.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	disposition.RecordDigest = controlledArtifactAccessDispositionRecordDigestV1(disposition)
	if err := ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt); err != nil {
		return ControlledArtifactAccessDispositionV1{}, err
	}
	return disposition, nil
}

func ValidateControlledArtifactAccessReceiptV1(receipt ControlledArtifactAccessReceiptV1) error {
	if err := validateControlledArtifactAccessReceiptUnsignedV1(receipt); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != receipt.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != receipt.AuthoritySignature ||
		receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ControlledArtifactAccessReceiptSigningBytesV1(receipt), signature) ||
		!canonicalControlledAccessSHA256V1(receipt.RecordDigest) || receipt.RecordDigest != controlledArtifactAccessReceiptRecordDigestV1(receipt) {
		return errors.New("controlled artifact access receipt signature is invalid")
	}
	return nil
}

// ValidateControlledArtifactAccessReceiptForGrantV1 proves that one private
// access reservation is a strict subset of the exact signed PII grant. It does
// not prove installation trust or publication graph membership; the app layer
// must verify those authorities before reserving or releasing any bytes.
func ValidateControlledArtifactAccessReceiptForGrantV1(
	receipt ControlledArtifactAccessReceiptV1,
	grant PIIProjectionGrantV1,
) error {
	requestedAt, requestErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, authorizedErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	grantIssuedAt, grantIssueErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	grantExpiresAt, grantExpiryErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if ValidateControlledArtifactAccessReceiptV1(receipt) != nil || ValidatePIIProjectionGrantV1(grant) != nil ||
		receipt.Context != grant.Context || receipt.RequesterUserID != grant.RequesterUserID ||
		receipt.PIIAuthorizationDigest != grant.RecordDigest || receipt.ClaimLedgerDigest != grant.ClaimLedgerDigest ||
		receipt.TargetIdentityDigest != grant.TargetIdentityDigest || receipt.ArtifactSHA256 != grant.ProjectedContentSHA256 ||
		receipt.AccessPolicyDigest != grant.AccessPolicyDigest || receipt.RetentionPolicyDigest != grant.RetentionPolicyDigest ||
		!controlledArtifactAccessActionAllowedV1(receipt.AccessAction, grant.AllowedAccessActions) ||
		requestErr != nil || authorizedErr != nil || grantIssueErr != nil || grantExpiryErr != nil ||
		requestedAt.Before(grantIssuedAt) || !requestedAt.Before(grantExpiresAt) ||
		authorizedUntil.After(grantExpiresAt) {
		return errors.New("controlled artifact access receipt exceeds its PII grant")
	}
	return nil
}

func ValidateControlledArtifactAccessDispositionV1(disposition ControlledArtifactAccessDispositionV1) error {
	if err := validateControlledArtifactAccessDispositionUnsignedV1(disposition); err != nil {
		return err
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != disposition.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != disposition.AuthoritySignature ||
		disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ControlledArtifactAccessDispositionSigningBytesV1(disposition), signature) ||
		!canonicalControlledAccessSHA256V1(disposition.RecordDigest) || disposition.RecordDigest != controlledArtifactAccessDispositionRecordDigestV1(disposition) {
		return errors.New("controlled artifact access disposition signature is invalid")
	}
	return nil
}

func ValidateControlledArtifactAccessDispositionForReceiptV1(
	disposition ControlledArtifactAccessDispositionV1,
	receipt ControlledArtifactAccessReceiptV1,
) error {
	requestedAt, requestedErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, authorizedErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if ValidateControlledArtifactAccessReceiptV1(receipt) != nil || ValidateControlledArtifactAccessDispositionV1(disposition) != nil ||
		disposition.AccessID != receipt.AccessID || disposition.AccessReceiptDigest != receipt.RecordDigest ||
		disposition.ContextDigest != receipt.Context.ContextDigest || disposition.ContextEpoch != receipt.Context.ContextEpoch ||
		disposition.DatasetSnapshotID != receipt.Context.DatasetSnapshotID || disposition.AccessAction != receipt.AccessAction ||
		disposition.ControlledHandleDigest != receipt.ControlledHandleDigest || disposition.UseSlotDigest != receipt.UseSlotDigest ||
		disposition.RendererPrincipalDigest != receipt.RendererPrincipalDigest || disposition.RendererGeneration != receipt.RendererGeneration ||
		disposition.BackendGeneration != receipt.BackendGeneration || disposition.AccessPolicyDigest != receipt.AccessPolicyDigest ||
		disposition.RetentionPolicyDigest != receipt.RetentionPolicyDigest ||
		disposition.PublicationCommitDigest != receipt.PublicationCommitDigest ||
		disposition.PublicationReceiptDigest != receipt.PublicationReceiptDigest ||
		disposition.PIIProjectionDigest != receipt.PIIProjectionDigest ||
		disposition.PIIAuthorizationDigest != receipt.PIIAuthorizationDigest ||
		disposition.ClaimLedgerDigest != receipt.ClaimLedgerDigest ||
		disposition.TargetIdentityDigest != receipt.TargetIdentityDigest || disposition.ArtifactSHA256 != receipt.ArtifactSHA256 ||
		disposition.ArtifactByteLength != receipt.ArtifactByteLength || disposition.MediaType != receipt.MediaType ||
		disposition.AuthorityKeyID != receipt.AuthorityKeyID ||
		disposition.AuthorityPublicKey != receipt.AuthorityPublicKey || requestedErr != nil || authorizedErr != nil || disposedErr != nil ||
		disposedAt.Before(requestedAt) ||
		disposition.Status == ControlledArtifactAccessDispositionHostReleaseCommittedV1 && !disposedAt.Before(authorizedUntil) {
		return errors.New("controlled artifact access disposition does not close its exact receipt")
	}
	return nil
}

func ControlledArtifactAccessReceiptV1Bytes(receipt ControlledArtifactAccessReceiptV1) ([]byte, error) {
	if err := ValidateControlledArtifactAccessReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func ControlledArtifactAccessDispositionV1Bytes(disposition ControlledArtifactAccessDispositionV1) ([]byte, error) {
	if err := ValidateControlledArtifactAccessDispositionV1(disposition); err != nil {
		return nil, err
	}
	return json.Marshal(disposition)
}

func ParseControlledArtifactAccessReceiptV1(body []byte) (ControlledArtifactAccessReceiptV1, error) {
	var receipt ControlledArtifactAccessReceiptV1
	if err := parseControlledArtifactAccessRecordV1(body, &receipt); err != nil {
		return ControlledArtifactAccessReceiptV1{}, err
	}
	return receipt, ValidateControlledArtifactAccessReceiptV1(receipt)
}

func ParseControlledArtifactAccessDispositionV1(body []byte) (ControlledArtifactAccessDispositionV1, error) {
	var disposition ControlledArtifactAccessDispositionV1
	if err := parseControlledArtifactAccessRecordV1(body, &disposition); err != nil {
		return ControlledArtifactAccessDispositionV1{}, err
	}
	return disposition, ValidateControlledArtifactAccessDispositionV1(disposition)
}

func ControlledArtifactAccessReceiptSigningBytesV1(receipt ControlledArtifactAccessReceiptV1) []byte {
	receipt.AuthoritySignature = ""
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), controlledArtifactAccessReceiptSigningDomainV1...), digest[:]...)
}

func ControlledArtifactAccessDispositionSigningBytesV1(disposition ControlledArtifactAccessDispositionV1) []byte {
	disposition.AuthoritySignature = ""
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	digest := sha256.Sum256(body)
	return append(append([]byte(nil), controlledArtifactAccessDispositionSigningDomainV1...), digest[:]...)
}

func ControlledArtifactAccessReceiptAuthorityMaterialV1(receipt ControlledArtifactAccessReceiptV1) (string, []byte, []byte, error) {
	if err := ValidateControlledArtifactAccessReceiptV1(receipt); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	return receipt.AuthorityKeyID, publicKey, signature, nil
}

func ControlledArtifactAccessDispositionAuthorityMaterialV1(disposition ControlledArtifactAccessDispositionV1) (string, []byte, []byte, error) {
	if err := ValidateControlledArtifactAccessDispositionV1(disposition); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	return disposition.AuthorityKeyID, publicKey, signature, nil
}

func validateControlledArtifactAccessReceiptUnsignedV1(receipt ControlledArtifactAccessReceiptV1) error {
	requestedAt, requestErr := time.Parse(time.RFC3339Nano, receipt.RequestedAt)
	authorizedUntil, expiryErr := time.Parse(time.RFC3339Nano, receipt.AuthorizedUntil)
	contextIssuedAt, contextErr := time.Parse(time.RFC3339Nano, receipt.Context.ContextIssuedAt)
	if receipt.SchemaVersion != ControlledArtifactAccessReceiptSchemaVersionV1 || receipt.Purpose != ControlledArtifactAccessReceiptPurposeV1 ||
		!canonicalControlledAccessSHA256V1(receipt.AccessID) || receipt.AccessID != controlledArtifactAccessIDV1(receipt) ||
		!validContextBindingV1(receipt.Context) || receipt.RequesterUserID != receipt.Context.UserID || !validControlledArtifactAccessActionV1(receipt.AccessAction) ||
		!canonicalControlledAccessSHA256V1(receipt.ControlledHandleDigest) || !canonicalControlledAccessSHA256V1(receipt.UseSlotDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.RendererPrincipalDigest) || receipt.RendererGeneration == 0 || receipt.BackendGeneration == 0 ||
		!canonicalControlledAccessSHA256V1(receipt.AccessPolicyDigest) || !canonicalControlledAccessSHA256V1(receipt.RetentionPolicyDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.PublicationCommitDigest) || !canonicalControlledAccessSHA256V1(receipt.PublicationReceiptDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.PIIProjectionDigest) || !canonicalControlledAccessSHA256V1(receipt.PIIAuthorizationDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.ClaimLedgerDigest) || !canonicalControlledAccessSHA256V1(receipt.TargetIdentityDigest) ||
		!canonicalControlledAccessSHA256V1(receipt.ArtifactSHA256) || receipt.ArtifactByteLength == 0 ||
		receipt.ArtifactByteLength > MaxControlledPIIArtifactBytesV1 || receipt.MediaType != ControlledPIIArtifactMediaTypeV1 ||
		requestErr != nil || expiryErr != nil || contextErr != nil || requestedAt.IsZero() || authorizedUntil.IsZero() ||
		requestedAt.Before(contextIssuedAt) || !requestedAt.Before(authorizedUntil) || authorizedUntil.Sub(requestedAt) > PIIProjectionGrantMaxTTL ||
		requestedAt.UTC().Format(time.RFC3339Nano) != receipt.RequestedAt || authorizedUntil.UTC().Format(time.RFC3339Nano) != receipt.AuthorizedUntil ||
		receipt.AuthorityAlgorithm != ControlledArtifactAccessAuthorityAlgorithmV1 || !canonicalControlledAccessSHA256V1(receipt.AuthorityKeyID) ||
		strings.TrimSpace(receipt.AuthorityPublicKey) == "" {
		return errors.New("controlled artifact access receipt is incomplete")
	}
	return nil
}

func validateControlledArtifactAccessDispositionUnsignedV1(disposition ControlledArtifactAccessDispositionV1) error {
	disposedAt, disposedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if disposition.SchemaVersion != ControlledArtifactAccessReceiptSchemaVersionV1 || disposition.Purpose != ControlledArtifactAccessDispositionPurposeV1 ||
		!canonicalControlledAccessSHA256V1(disposition.DispositionID) || disposition.DispositionID != controlledArtifactAccessDispositionIDV1(disposition) ||
		!canonicalControlledAccessSHA256V1(disposition.AccessID) || !canonicalControlledAccessSHA256V1(disposition.AccessReceiptDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.ContextDigest) || disposition.ContextEpoch == 0 ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(disposition.DatasetSnapshotID) ||
		!validControlledArtifactAccessActionV1(disposition.AccessAction) ||
		!canonicalControlledAccessSHA256V1(disposition.ControlledHandleDigest) || !canonicalControlledAccessSHA256V1(disposition.UseSlotDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.RendererPrincipalDigest) || disposition.RendererGeneration == 0 || disposition.BackendGeneration == 0 ||
		!canonicalControlledAccessSHA256V1(disposition.AccessPolicyDigest) || !canonicalControlledAccessSHA256V1(disposition.RetentionPolicyDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.PublicationCommitDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.PublicationReceiptDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.PIIProjectionDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.PIIAuthorizationDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.ClaimLedgerDigest) ||
		!canonicalControlledAccessSHA256V1(disposition.TargetIdentityDigest) || !canonicalControlledAccessSHA256V1(disposition.ArtifactSHA256) ||
		disposition.ArtifactByteLength == 0 || disposition.ArtifactByteLength > MaxControlledPIIArtifactBytesV1 ||
		disposition.MediaType != ControlledPIIArtifactMediaTypeV1 ||
		disposition.ReleasedByteLength > disposition.ArtifactByteLength ||
		!validControlledArtifactAccessDispositionStatusV1(disposition.Status) ||
		!validControlledArtifactAccessReasonV1(disposition.Status, disposition.ReasonCode) || disposedErr != nil || disposedAt.IsZero() ||
		disposedAt.UTC().Format(time.RFC3339Nano) != disposition.DisposedAt ||
		!validControlledArtifactAccessByteStateV1(disposition.Status, disposition.ReleasedByteLength, disposition.ArtifactByteLength) ||
		disposition.AuthorityAlgorithm != ControlledArtifactAccessAuthorityAlgorithmV1 || !canonicalControlledAccessSHA256V1(disposition.AuthorityKeyID) ||
		strings.TrimSpace(disposition.AuthorityPublicKey) == "" {
		return errors.New("controlled artifact access disposition is incomplete")
	}
	return nil
}

func parseControlledArtifactAccessRecordV1(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxControlledArtifactAccessRecordBytesV1, MaxDepth: 6, MaxTokens: 256, MaxStringBytes: 32 << 10,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("controlled artifact access record contains trailing JSON")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(body, canonical) {
		return errors.New("controlled artifact access record is not canonical JSON")
	}
	return nil
}

func controlledArtifactAccessIDV1(receipt ControlledArtifactAccessReceiptV1) string {
	return domainsecurity.SHA256Hex(
		append(append([]byte(nil), controlledArtifactAccessIDDomainV1...), []byte(receipt.UseSlotDigest)...),
	)
}

func controlledArtifactAccessReceiptRecordDigestV1(receipt ControlledArtifactAccessReceiptV1) string {
	receipt.RecordDigest = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), controlledArtifactAccessReceiptDigestDomainV1...), body...))
}

func controlledArtifactAccessDispositionIDV1(disposition ControlledArtifactAccessDispositionV1) string {
	disposition.DispositionID = ""
	disposition.AuthorityAlgorithm = ""
	disposition.AuthorityKeyID = ""
	disposition.AuthorityPublicKey = ""
	disposition.AuthoritySignature = ""
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), controlledArtifactAccessDispositionIDDomainV1...), body...))
}

func controlledArtifactAccessDispositionRecordDigestV1(disposition ControlledArtifactAccessDispositionV1) string {
	disposition.RecordDigest = ""
	body, _ := json.Marshal(disposition)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), controlledArtifactAccessDispositionDigestDomainV1...), body...))
}

func validControlledArtifactAccessActionV1(action string) bool {
	return action == ControlledArtifactAccessActionDisplayV1 || action == ControlledArtifactAccessActionExportV1
}

func controlledArtifactAccessActionAllowedV1(action string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == action {
			return true
		}
	}
	return false
}

func validControlledArtifactAccessDispositionStatusV1(status string) bool {
	switch status {
	case ControlledArtifactAccessDispositionHostReleaseCommittedV1, ControlledArtifactAccessDispositionReleaseIndeterminateV1,
		ControlledArtifactAccessDispositionFailedV1,
		ControlledArtifactAccessDispositionRejectedV1, ControlledArtifactAccessDispositionCancelledV1,
		ControlledArtifactAccessDispositionRestartInvalidV1, ControlledArtifactAccessDispositionStaleContextV1:
		return true
	default:
		return false
	}
}

func validControlledArtifactAccessReasonV1(status string, reason string) bool {
	switch status {
	case ControlledArtifactAccessDispositionHostReleaseCommittedV1:
		return reason == ControlledArtifactAccessReasonHostReleaseCommittedV1
	case ControlledArtifactAccessDispositionReleaseIndeterminateV1:
		return reason == ControlledArtifactAccessReasonReleaseIndeterminateV1
	case ControlledArtifactAccessDispositionFailedV1:
		return reason == ControlledArtifactAccessReasonHostReleaseFailedV1
	case ControlledArtifactAccessDispositionRejectedV1:
		return reason == ControlledArtifactAccessReasonAccessRejectedV1
	case ControlledArtifactAccessDispositionCancelledV1:
		return reason == ControlledArtifactAccessReasonAccessCancelledV1
	case ControlledArtifactAccessDispositionRestartInvalidV1:
		return reason == ControlledArtifactAccessReasonRestartInvalidV1
	case ControlledArtifactAccessDispositionStaleContextV1:
		return reason == ControlledArtifactAccessReasonStaleContextV1
	default:
		return false
	}
}

func validControlledArtifactAccessByteStateV1(status string, delivered uint64, artifact uint64) bool {
	switch status {
	case ControlledArtifactAccessDispositionHostReleaseCommittedV1:
		return delivered == artifact
	case ControlledArtifactAccessDispositionReleaseIndeterminateV1:
		return delivered <= artifact
	case ControlledArtifactAccessDispositionFailedV1, ControlledArtifactAccessDispositionCancelledV1:
		return delivered < artifact
	case ControlledArtifactAccessDispositionRejectedV1, ControlledArtifactAccessDispositionRestartInvalidV1,
		ControlledArtifactAccessDispositionStaleContextV1:
		return delivered == 0
	default:
		return false
	}
}

func canonicalControlledAccessSHA256V1(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}
