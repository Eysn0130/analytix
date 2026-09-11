package evidenceauthority

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// OriginalGraphV1 preserves every local bundle and complete stored witness
// exchange. It has no selected tip, freshness or execution capability.
type OriginalGraphV1 struct {
	Bundles      map[string]domainevidence.EvidenceAuthorityBundleV1
	Observations map[string]storeport.ObservationBundle
}

func PrepareOriginalObservationV1(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority) (*finalauthorityadapter.OriginalFixedOwnerObservationV1, error) {
	return finalauthorityadapter.PrepareOriginalFixedOwnerObservationV1(ctx, root, []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
		{Name: bundlesLeafV1, MaxBytes: maxEvidenceAuthorityBundleBytes}, {Name: observationsLeafV1, MaxBytes: maxEvidenceObservationBytes},
	}, access)
}

// ObserveOriginalGraphV1 uses only the frozen native physical owner. The
// caller supplies its independently anchored enrollment and current verifier;
// keys embedded in local history never establish trust.
func (prepared *PreparedRecoveryV1) ObserveOriginalGraphV1(ctx context.Context, installationID, enrollmentID, witnessKeyID string, witnessPublicKey []byte, verifier authorityport.Verifier) (graph OriginalGraphV1, resultErr error) {
	files, err := prepared.SnapshotOriginalFilesV1(ctx)
	if err != nil {
		return graph, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.RevalidatePhysicalV1(ctx), context.Cause(ctx))
		if resultErr != nil {
			graph = OriginalGraphV1{}
		}
	}()
	return ParseOriginalGraphV1(ctx, files, installationID, enrollmentID, witnessKeyID, witnessPublicKey, verifier)
}

func (prepared *PreparedRecoveryV1) RevalidatePhysicalV1(ctx context.Context) error {
	if ctx == nil || prepared == nil || prepared.owner == nil {
		return errors.New("original evidence authority physical observation is unavailable")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) SnapshotOriginalFilesV1(ctx context.Context) (map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1, error) {
	if prepared == nil || prepared.owner == nil {
		return nil, errors.New("original evidence authority physical observation is unavailable")
	}
	return prepared.owner.SnapshotOriginalEntriesV1(ctx)
}

// ParseOriginalGraphV1 also validates complete journal original/Final maps.
// It never observes the live filesystem or selects a current witness head.
func ParseOriginalGraphV1(ctx context.Context, files map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1, installationID, enrollmentID, witnessKeyID string, witnessPublicKey []byte, verifier authorityport.Verifier) (graph OriginalGraphV1, resultErr error) {
	if ctx == nil || verifier == nil ||
		!domainsecurity.IsSHA256Hex(installationID) || !domainsecurity.IsSHA256Hex(enrollmentID) ||
		len(witnessPublicKey) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(witnessPublicKey) != witnessKeyID ||
		len(verifier.PublicKey()) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(verifier.PublicKey()) != verifier.KeyID() {
		return graph, errors.New("original evidence authority independent trust is unavailable")
	}
	if err := context.Cause(ctx); err != nil {
		return graph, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, context.Cause(ctx))
		if resultErr != nil {
			graph = OriginalGraphV1{}
		}
	}()
	committed, err := originalCommittedFilesV1(ctx, files)
	if err != nil {
		return graph, err
	}
	graph = OriginalGraphV1{Bundles: map[string]domainevidence.EvidenceAuthorityBundleV1{}, Observations: map[string]storeport.ObservationBundle{}}
	for digest, body := range committed[bundlesLeafV1] {
		bundle, err := parseStoredBundle(digest, body)
		if err != nil {
			return graph, err
		}
		graph.Bundles[digest] = bundle
	}
	for digest, body := range committed[observationsLeafV1] {
		observation, err := parseStoredObservation(digest, body)
		if err != nil {
			return graph, err
		}
		graph.Observations[digest] = observation
	}
	for _, bundle := range graph.Bundles {
		if err := domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(bundle, installationID, enrollmentID, verifier.KeyID(), verifier.PublicKey()); err != nil {
			return graph, err
		}
		publicKey, keyErr := base64.RawURLEncoding.DecodeString(bundle.AuthorityPublicKey)
		signature, signatureErr := base64.RawURLEncoding.DecodeString(bundle.AuthoritySignature)
		if err := errors.Join(keyErr, signatureErr); err != nil {
			return graph, err
		}
		if err := verifier.VerifyTrusted(ctx, bundle.AuthorityKeyID, publicKey, domainevidence.EvidenceAuthorityBundleSigningBytesV1(bundle), signature); err != nil {
			return graph, err
		}
		if bundle.Generation > 1 {
			previous, found := graph.Bundles[bundle.PreviousBundleDigest]
			if !found || domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, bundle) != nil {
				return graph, errors.New("original evidence authority bundle lost its exact predecessor")
			}
		}
	}
	for _, stored := range graph.Observations {
		bundle, found := graph.Bundles[stored.Bundle.RecordDigest]
		if !found || !equalBundle(bundle, stored.Bundle) {
			return graph, errors.New("original witness observation lost its exact bundle")
		}
		if err := domainsecurity.ValidateMonotonicHeadObservationForRequestV1(stored.Observation, stored.Request,
			installationID, verifier.KeyID(), verifier.PublicKey(), enrollmentID, witnessKeyID, witnessPublicKey); err != nil {
			return graph, err
		}
		if err := domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(bundle, stored.Observation.Checkpoint); err != nil {
			return graph, err
		}
	}
	return graph, nil
}

// OriginalFilesHaveRecordsV1 proves absence through the complete fixed-owner
// grammar, including initialized empty leaves and retained opaque residues.
func OriginalFilesHaveRecordsV1(ctx context.Context, files map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1) (bool, error) {
	committed, err := originalCommittedFilesV1(ctx, files)
	if err != nil {
		return false, err
	}
	for _, records := range committed {
		if len(records) != 0 {
			return true, nil
		}
	}
	return false, nil
}

func originalCommittedFilesV1(ctx context.Context, files map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1) (map[string]map[string][]byte, error) {
	return finalauthorityadapter.ValidateOriginalFixedOwnerEntriesV1(ctx, files, []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
		{Name: bundlesLeafV1, MaxBytes: maxEvidenceAuthorityBundleBytes}, {Name: observationsLeafV1, MaxBytes: maxEvidenceObservationBytes},
	})
}
