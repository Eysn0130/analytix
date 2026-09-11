package authoritymanifest

import (
	"context"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
)

type Loader interface {
	LoadAnchoredV2(context.Context) (domainenrollment.AnchoredManifestV2, error)
}

// MigrationV1Loader exists only for an explicit V1-to-V2 migration planner.
// AnchoredManifestV1 cannot enter bootstrap or production witness APIs.
type MigrationV1Loader interface {
	LoadAnchoredV1ForMigration(context.Context) (domainenrollment.AnchoredManifestV1, error)
}
