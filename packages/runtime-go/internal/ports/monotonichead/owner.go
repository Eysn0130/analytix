package monotonichead

import (
	"context"
	"crypto/ed25519"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ObservePathV1         = "/v1/monotonic-head/observe"
	AdvancePathV1         = "/v1/monotonic-head/advance"
	MutationResolvePathV1 = "/v1/monotonic-head/mutation/resolve"
)

// OwnerAnchorV1 comes from accepted installation enrollment, never the owner
// root being opened. Transport composition must supply an existing-only opener.
type OwnerAnchorV1 struct {
	InstallationID     string
	AuthorityKeyID     string
	AuthorityPublicKey ed25519.PublicKey
	EnrollmentID       string
	GenesisCheckpoint  domainsecurity.MonotonicHeadCheckpointV1
}

type ExistingOwnerV1 interface {
	Witness
	MutationRecoveryWitness
	Close() error
}

type ExistingOwnerOpenerV1 func(context.Context, string, OwnerAnchorV1) (ExistingOwnerV1, error)
