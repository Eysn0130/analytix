package authoritycredentials

import (
	"context"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

type EnrolledWitnessesV1 struct {
	ManifestDigest    string
	ProfileDigest     string
	ProfileGeneration uint64
	ThreadRisk        monotonicheadport.MutationRecoveryWitness
	SharedEvidence    monotonicheadport.MutationRecoveryWitness
}

// Loader consumes the opaque manifest authority and never exposes raw
// certificate or private-key bytes to app/runtime composition.
type Loader interface {
	LoadCurrent(context.Context, domainenrollment.AnchoredManifestV2) (EnrolledWitnessesV1, error)
}
