//go:build darwin && !analytix_prod

package processauthority

import "context"

func openSession(ctx context.Context, config SessionConfig) (Session, error) {
	return openSessionWithTargetArm(ctx, config, nil)
}
