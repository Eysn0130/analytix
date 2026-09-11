package finalauthority

import (
	"context"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

type Verifier interface {
	KeyID() string
	PublicKey() []byte
	VerifyTrusted(context.Context, string, []byte, []byte, []byte) error
}

type Authority interface {
	Verifier
	Sign(context.Context, []byte) ([]byte, error)
}

type PrivateFinalStore interface {
	PutIfAbsent(context.Context, domainevidence.PrivateAcceptedFinalRecord) error
	Resolve(context.Context, string) (domainevidence.PrivateAcceptedFinalRecord, error)
	List(context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error)
	HasRecords(context.Context) (bool, error)
	PutDispositionIfAbsent(context.Context, domainevidence.AcceptedFinalDispositionRecord) error
	ResolveDisposition(context.Context, string) (domainevidence.AcceptedFinalDispositionRecord, error)
	ListDispositions(context.Context) ([]domainevidence.AcceptedFinalDispositionRecord, error)
}

// PrivateFinalInventoryStore exposes provisional streaming inventory for
// startup verification. Visitors must discard all accumulated state when a
// visit returns an error.
type PrivateFinalInventoryStore interface {
	PrivateFinalStore
	VisitAcceptedFinals(context.Context, func(domainevidence.PrivateAcceptedFinalRecord) error) error
	VisitDispositions(context.Context, func(domainevidence.AcceptedFinalDispositionRecord) error) error
}

type AcceptedFinalCASReader interface {
	ReadAcceptedFinalCASObservation(context.Context, string, string) (domainevidence.AcceptedFinalCASObservationV1, error)
}

// AcceptedFinalCASProjectionReader provides one primary-file observation for
// an exact turn set. Public snapshot/SSE projection uses this shape so several
// accepted finals cannot be assembled from different thread.json versions.
type AcceptedFinalCASProjectionReader interface {
	ReadAcceptedFinalCASObservations(context.Context, string, []string) (map[string]domainevidence.AcceptedFinalCASObservationV1, error)
}
