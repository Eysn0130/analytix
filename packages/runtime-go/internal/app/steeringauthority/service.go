package steeringauthority

import (
	"context"
	"errors"

	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type Service struct {
	authority finalauthorityport.Authority
}

func NewService(authority finalauthorityport.Authority) *Service {
	if authority == nil {
		return nil
	}
	return &Service{authority: authority}
}

func (service *Service) KeyID() string {
	if service == nil || service.authority == nil {
		return ""
	}
	return service.authority.KeyID()
}

func (service *Service) PublicKey() []byte {
	if service == nil || service.authority == nil {
		return nil
	}
	return append([]byte(nil), service.authority.PublicKey()...)
}

func (service *Service) Seal(
	ctx context.Context,
	entry map[string]any,
	contextDigest string,
) (map[string]any, error) {
	if service == nil || service.authority == nil {
		return nil, errors.New("steering admission authority is unavailable")
	}
	signingBytes, err := domainsteering.PendingEntrySigningBytesV1(entry, contextDigest)
	if err != nil {
		return nil, err
	}
	signature, err := service.authority.Sign(ctx, signingBytes)
	if err != nil {
		return nil, err
	}
	sealed, err := domainsteering.SealPendingEntryAuthorityV1(
		entry, contextDigest, service.authority.KeyID(), service.authority.PublicKey(), signature,
	)
	if err != nil || service.Verify(ctx, sealed, contextDigest) != nil {
		return nil, errors.New("steering admission authority cannot be sealed")
	}
	return sealed, nil
}

func (service *Service) Promote(
	ctx context.Context,
	entry map[string]any,
	contextDigest string,
) (map[string]any, error) {
	if service == nil || service.authority == nil {
		return nil, errors.New("steering promotion authority is unavailable")
	}
	admission, err := domainsteering.AdmissionAuthorityMaterialV1(entry, contextDigest)
	if err != nil || service.authority.VerifyTrusted(
		ctx, admission.KeyID, admission.PublicKey, admission.SigningBytes, admission.Signature,
	) != nil {
		return nil, errors.New("steering admission authority is not trusted for promotion")
	}
	signingBytes, err := domainsteering.PromotedEntrySigningBytesV1(entry, contextDigest)
	if err != nil {
		return nil, err
	}
	signature, err := service.authority.Sign(ctx, signingBytes)
	if err != nil {
		return nil, err
	}
	sealed, err := domainsteering.SealPromotedEntryAuthorityV1(
		entry, contextDigest, service.authority.KeyID(), service.authority.PublicKey(), signature,
	)
	if err != nil || service.Verify(ctx, sealed, contextDigest) != nil {
		return nil, errors.New("steering promotion authority cannot be sealed")
	}
	return sealed, nil
}

func (service *Service) Verify(ctx context.Context, entry map[string]any, contextDigest string) error {
	if service == nil || service.authority == nil {
		return errors.New("steering admission authority is unavailable")
	}
	material, err := domainsteering.EntryAuthorityMaterialV1(entry, contextDigest)
	if err != nil {
		return err
	}
	return service.authority.VerifyTrusted(ctx, material.KeyID, material.PublicKey, material.SigningBytes, material.Signature)
}

// VerifyTrusted exposes only the verification side of the installation
// authority for provider-history projection. Signing remains owned by this
// service and is never passed into the transitional server facade.
func (service *Service) VerifyTrusted(
	ctx context.Context,
	keyID string,
	publicKey []byte,
	message []byte,
	signature []byte,
) error {
	if service == nil || service.authority == nil {
		return errors.New("steering admission authority is unavailable")
	}
	return service.authority.VerifyTrusted(ctx, keyID, publicKey, message, signature)
}
