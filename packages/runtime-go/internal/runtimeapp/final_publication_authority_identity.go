package runtimeapp

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// FinalPublicationAuthorityIdentityV1 is the public installation trust anchor
// handed to the exact process launcher. It never contains signing material.
type FinalPublicationAuthorityIdentityV1 struct {
	KeyID     string
	PublicKey []byte
}

type FinalPublicationAuthorityIdentitySourceV1 interface {
	FinalPublicationAuthorityIdentityV1() (FinalPublicationAuthorityIdentityV1, error)
}

type finalPublicationAuthorityBoundHandlerV1 struct {
	http.Handler
	identity FinalPublicationAuthorityIdentityV1
}

func ValidateFinalPublicationAuthorityIdentityV1(identity FinalPublicationAuthorityIdentityV1) error {
	if len(identity.PublicKey) != ed25519.PublicKeySize ||
		!domainsecurity.IsSHA256Hex(identity.KeyID) ||
		domainsecurity.SHA256Hex(identity.PublicKey) != identity.KeyID {
		return errors.New("final publication authority identity is invalid")
	}
	return nil
}

func bindFinalPublicationAuthorityIdentityV1(
	handler http.Handler,
	authority finalauthorityport.Verifier,
) (http.Handler, error) {
	if handler == nil || authority == nil {
		return nil, errors.New("final publication authority identity is unavailable")
	}
	publicKey := authority.PublicKey()
	identity := FinalPublicationAuthorityIdentityV1{
		KeyID: authority.KeyID(), PublicKey: append([]byte(nil), publicKey...),
	}
	if err := ValidateFinalPublicationAuthorityIdentityV1(identity); err != nil {
		return nil, err
	}
	return &finalPublicationAuthorityBoundHandlerV1{Handler: handler, identity: identity}, nil
}

func (handler *finalPublicationAuthorityBoundHandlerV1) FinalPublicationAuthorityIdentityV1() (FinalPublicationAuthorityIdentityV1, error) {
	if handler == nil || len(handler.identity.PublicKey) != ed25519.PublicKeySize {
		return FinalPublicationAuthorityIdentityV1{}, errors.New("final publication authority identity is unavailable")
	}
	return FinalPublicationAuthorityIdentityV1{
		KeyID: handler.identity.KeyID, PublicKey: append([]byte(nil), handler.identity.PublicKey...),
	}, nil
}

func (handler *finalPublicationAuthorityBoundHandlerV1) Shutdown(ctx context.Context) error {
	if lifecycle, ok := handler.Handler.(interface{ Shutdown(context.Context) error }); ok {
		return lifecycle.Shutdown(ctx)
	}
	return nil
}
