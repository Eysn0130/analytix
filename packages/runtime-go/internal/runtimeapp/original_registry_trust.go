package runtimeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"reflect"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// runtimeOriginalRegistryTrustV2 binds original history to the independently
// anchored installation/enrollment. It owns no credential, witness client or
// current head, and cannot authorize a registry mutation or recovery.
type runtimeOriginalRegistryTrustV2 struct {
	projection domainenrollment.ManifestAnchorProjectionV2
	witnessKey []byte
	revalidate func(context.Context) error
}

func prepareRuntimeOriginalRegistryTrustV2(ctx context.Context, config Config, verifier finalauthorityport.Verifier) (*runtimeOriginalRegistryTrustV2, error) {
	// Keep only the immutable anchor and protected root configuration in the
	// revalidation closure; provider and witness credential values stay out.
	frozen := Config{
		AuthorityAnchorV1: config.AuthorityAnchorV1, AuthorityManifestRoot: config.AuthorityManifestRoot,
		AuthorityCredentialProfileRoot: config.AuthorityCredentialProfileRoot,
		AuthorityCredentialBundleRoot:  config.AuthorityCredentialBundleRoot,
	}
	_, projection, configured, err := loadRuntimeSharedEvidenceManifestV2(ctx, frozen)
	if err != nil || !configured {
		return nil, err
	}
	if verifier == nil || verifier.KeyID() != projection.InstallationAuthorityKeyID ||
		!bytes.Equal(verifier.PublicKey(), projection.InstallationAuthorityPublicKey) {
		return nil, errors.New("original registry installation differs from independent enrollment")
	}
	witnessKey, err := base64.RawURLEncoding.DecodeString(projection.Enrollment.WitnessPublicKey)
	if err != nil || base64.RawURLEncoding.EncodeToString(witnessKey) != projection.Enrollment.WitnessPublicKey ||
		domainsecurity.SHA256Hex(witnessKey) != projection.Enrollment.WitnessKeyID {
		return nil, errors.New("original registry enrolled witness key is invalid")
	}
	trust := &runtimeOriginalRegistryTrustV2{projection: projection, witnessKey: witnessKey}
	trust.revalidate = func(ctx context.Context) error {
		_, current, configured, err := loadRuntimeSharedEvidenceManifestV2(ctx, frozen)
		if err != nil || !configured || !reflect.DeepEqual(projection, current) ||
			verifier.KeyID() != projection.InstallationAuthorityKeyID || !bytes.Equal(verifier.PublicKey(), projection.InstallationAuthorityPublicKey) {
			return errors.Join(errors.New("original registry independent enrollment changed"), err)
		}
		return ctx.Err()
	}
	if err := trust.Revalidate(ctx); err != nil {
		return nil, err
	}
	return trust, nil
}

func (trust *runtimeOriginalRegistryTrustV2) Revalidate(ctx context.Context) error {
	if trust == nil || trust.revalidate == nil || ctx == nil {
		return errors.New("original registry independent enrollment is unavailable")
	}
	return trust.revalidate(ctx)
}
