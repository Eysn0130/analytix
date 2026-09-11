package artifactdelivery

import (
	"context"
	"errors"

	domainartifactdelivery "analytix.local/runtime-go/internal/domain/artifactdelivery"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrUnavailable = errors.New("artifact delivery authority is unavailable")
	ErrInvalid     = errors.New("artifact delivery selector is invalid")
	ErrStale       = errors.New("artifact delivery context is stale")
	ErrNotFound    = errors.New("artifact delivery is not available")
	ErrRejected    = errors.New("artifact delivery was rejected")
	ErrIntegrity   = errors.New("artifact delivery graph is invalid")
	ErrCancelled   = errors.New("artifact delivery validation is cancelled")
)

type SelectorV1 struct {
	SecurityContext         domainsecurity.TurnSecurityContext
	DeliveryID              string
	OutcomeRecordDigest     string
	PublicationCommitDigest string
}

type HistoricalSelectorV1 struct {
	Context                 domainartifactdelivery.ContextBindingV1
	DeliveryID              string
	OutcomeRecordDigest     string
	PublicationCommitDigest string
}

type CurrentAuthority interface {
	ResolveCurrent(context.Context, SelectorV1) (domainartifactdelivery.VerifiedArtifactDeliveryV1, error)
}

type LinearizedCurrentAuthority interface {
	CurrentAuthority
	WithCurrent(
		context.Context,
		SelectorV1,
		func(domainartifactdelivery.VerifiedArtifactDeliveryV1) error,
	) error
}

type HistoricalAuthority interface {
	ResolveTrustedHistorical(
		context.Context,
		HistoricalSelectorV1,
	) (domainartifactdelivery.VerifiedArtifactDeliveryV1, error)
}

type ArtifactResolver interface {
	ResolveExact(context.Context, string) ([]byte, error)
}

type ControlledMetadataResolver interface {
	ResolveControlledMetadata(context.Context, string) (domainpii.ControlledPIIArtifactMetadataV1, error)
}
