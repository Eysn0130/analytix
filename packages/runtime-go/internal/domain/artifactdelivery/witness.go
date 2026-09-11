package artifactdelivery

import (
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const ExposureClassRestrictedExactV1 = "restricted_exact"

type ContextBindingV1 struct {
	Version            int
	ThreadID           string
	TurnID             string
	WorkspaceRealPath  string
	TenantID           string
	UserID             string
	CaseID             string
	CaseBindingHash    string
	DatasetSnapshotID  string
	SourceManifestHash string
	ContextEpoch       uint64
	IssuedAt           string
	ContextDigest      string
}

// VerifiedArtifactDeliveryV1 is an in-process anti-corruption view of one
// fully verified report delivery graph. It contains no artifact bytes, path,
// bearer token, controlled handle, or raw PII and must never be persisted as a
// second publication authority. Trust comes from resolving it through the
// current or historical artifact-delivery authority on every use.
type VerifiedArtifactDeliveryV1 struct {
	Context                  ContextBindingV1
	DeliveryID               string
	OutcomeRecordDigest      string
	PublicationCommitDigest  string
	PublicationReceiptDigest string
	ClaimLedgerDigest        string
	ContentProjectionDigest  string
	AuthorizationAuditDigest string
	TargetIdentityDigest     string
	ArtifactSHA256           string
	ArtifactByteLength       uint64
	MediaType                string
	ExposureClass            string
	PublicationIssuedAt      string
}

func ContextBindingFromTurnSecurityContextV1(
	securityContext domainsecurity.TurnSecurityContext,
) (ContextBindingV1, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return ContextBindingV1{}, errors.New("artifact delivery current context is invalid")
	}
	return ContextBindingV1{
		Version: securityContext.Version, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		WorkspaceRealPath: securityContext.WorkspaceRealPath, TenantID: securityContext.TenantID,
		UserID: securityContext.UserID, CaseID: securityContext.CaseID,
		CaseBindingHash: securityContext.CaseBindingHash, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		SourceManifestHash: securityContext.SourceManifestHash, ContextEpoch: securityContext.ContextEpoch,
		IssuedAt: securityContext.IssuedAt, ContextDigest: securityContext.ContextDigest,
	}, nil
}

func ValidateContextBindingV1(binding ContextBindingV1) error {
	for _, value := range []string{
		binding.ThreadID, binding.TurnID, binding.WorkspaceRealPath, binding.TenantID, binding.UserID,
		binding.CaseID, binding.CaseBindingHash, binding.DatasetSnapshotID, binding.SourceManifestHash,
		binding.IssuedAt, binding.ContextDigest,
	} {
		if value == "" || value != strings.TrimSpace(value) {
			return errors.New("artifact delivery context binding is incomplete")
		}
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, binding.IssuedAt)
	if binding.Version != domainsecurity.TurnSecurityContextVersionV2 || binding.ContextEpoch == 0 ||
		binding.CaseID == domainsecurity.UnboundCaseID || !domainsecurity.IsSHA256Hex(binding.CaseBindingHash) ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(binding.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(binding.SourceManifestHash) || !domainsecurity.IsSHA256Hex(binding.ContextDigest) ||
		err != nil || issuedAt.UTC().Format(time.RFC3339Nano) != binding.IssuedAt {
		return errors.New("artifact delivery context binding is invalid")
	}
	return nil
}

func ValidateVerifiedArtifactDeliveryV1(delivery VerifiedArtifactDeliveryV1) error {
	if err := ValidateContextBindingV1(delivery.Context); err != nil {
		return errors.Join(errors.New("artifact delivery security context is invalid"), err)
	}
	for _, digest := range []string{
		delivery.DeliveryID,
		delivery.OutcomeRecordDigest,
		delivery.PublicationCommitDigest,
		delivery.PublicationReceiptDigest,
		delivery.ClaimLedgerDigest,
		delivery.ContentProjectionDigest,
		delivery.AuthorizationAuditDigest,
		delivery.TargetIdentityDigest,
		delivery.ArtifactSHA256,
	} {
		if !domainsecurity.IsSHA256Hex(strings.TrimSpace(digest)) {
			return errors.New("artifact delivery digest is invalid")
		}
	}
	if delivery.ArtifactByteLength == 0 || strings.TrimSpace(delivery.MediaType) == "" ||
		delivery.MediaType != strings.TrimSpace(delivery.MediaType) ||
		delivery.ExposureClass != ExposureClassRestrictedExactV1 {
		return errors.New("artifact delivery projection is invalid")
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, delivery.PublicationIssuedAt)
	if err != nil || issuedAt.IsZero() || delivery.PublicationIssuedAt != issuedAt.UTC().Format(time.RFC3339Nano) {
		return errors.New("artifact delivery publication time is invalid")
	}
	return nil
}
