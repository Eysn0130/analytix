package threadriskpolicy

import (
	"context"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// Store is the immutable persistence boundary for installation-signed thread
// risk policies. PolicyDigest is the sole content address.
type Store interface {
	PutIfAbsent(context.Context, domainsecurity.ThreadRiskPolicyV1) error
	Resolve(context.Context, string) (domainsecurity.ThreadRiskPolicyV1, error)
	List(context.Context) ([]domainsecurity.ThreadRiskPolicyV1, error)
	HasRecords(context.Context) (bool, error)
}
