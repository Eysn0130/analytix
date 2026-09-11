package evidencesettlement

import (
	"context"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

// Store contains private prepared settlements only. Records in this store are
// never EvidenceReceipt registry membership and cannot support a claim.
type Store interface {
	PutPreparedIfAbsent(context.Context, domainevidence.PreparedEvidenceSettlement) error
	ResolvePrepared(context.Context, string) (domainevidence.PreparedEvidenceSettlement, error)
	ListPrepared(context.Context) ([]domainevidence.PreparedEvidenceSettlement, error)
	HasRecords(context.Context) (bool, error)
}

// HostAuthorityInput is supplied only while a callback-scoped
// HostEvidenceCapability lease is active. The capability is intentionally
// non-serializable and the existing Store methods remain the bare/V1 path.
type HostAuthorityInput struct {
	Context         domainsecurity.TurnSecurityContext
	Binding         domainsecurity.CaseBindingObservationV1
	CurrentProbe    domainsecurity.VerifiedSourceProbe
	SelectionDigest string
	Capability      sourceprobeport.HostEvidenceCapability
}

// HostAuthorityStore is an additive optional interface. Implementations must
// validate the host witness before writing; callers must invoke it from the
// active HostEvidenceCapability callback.
type HostAuthorityStore interface {
	PutPreparedIfAbsentWithHostAuthority(context.Context, domainevidence.PreparedEvidenceSettlement, HostAuthorityInput) error
}
