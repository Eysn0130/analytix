//go:build !darwin && !windows

package packagedbuildauthorityfs

import (
	"context"
	"errors"

	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

func verifyPackageAnchorV2(_ context.Context, _ string, _ domainauthority.ParsedAuthorityV2) (string, error) {
	return "", errors.New("packaged authority v2 has no independently verified platform resource anchor")
}
