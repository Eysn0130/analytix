package reportpublication

import (
	"context"
	"errors"

	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrProjectedDeliveryUnavailable = errors.New("projected report delivery authority is unavailable")
	ErrProjectedDeliveryInvalid     = errors.New("projected report delivery selector is invalid")
	ErrProjectedDeliveryStale       = errors.New("projected report delivery context is stale")
	ErrProjectedDeliveryNotFound    = errors.New("projected report delivery is not available")
	ErrProjectedDeliveryRejected    = errors.New("report delivery was rejected")
	ErrProjectedDeliveryIntegrity   = errors.New("projected report delivery graph is invalid")
	ErrProjectedDeliveryCancelled   = errors.New("projected report delivery validation is cancelled")
)

// ProjectedDeliverySelectorV1 names one exact host-issued projected outcome.
// A publication commit alone is deliberately insufficient: callers must bind
// the stable delivery slot and the signed outcome bytes that won that slot.
type ProjectedDeliverySelectorV1 struct {
	SecurityContext         domainsecurity.TurnSecurityContext
	DeliveryID              string
	OutcomeRecordDigest     string
	PublicationCommitDigest string
}

// HistoricalProjectedDeliverySelectorV1 contains only immutable signed
// identity from a persisted audit receipt. It deliberately omits a current
// publication policy: startup verifies historical integrity without granting
// current release authority.
type HistoricalProjectedDeliverySelectorV1 struct {
	DeliveryID              string
	OutcomeRecordDigest     string
	PublicationCommitDigest string
	ThreadID                string
	TurnID                  string
	ContextDigest           string
	CaseBindingHash         string
	ContextEpoch            uint64
	DatasetSnapshotID       string
	SourceManifestHash      string
}

// ProjectedDeliveryAuthority acquires the context-effect lease, replays the
// complete durable report graph inside one current witnessed snapshot, and
// returns only a currently valid projected winner. Missing, rejected,
// incomplete, stale, mismatched, revoked, or corrupt graphs fail closed. The
// returned projection is an audit identity, not a reusable release capability;
// every later controlled side-effect boundary must resolve it again.
type ProjectedDeliveryAuthority interface {
	ResolveCurrentProjectedDelivery(
		context.Context,
		ProjectedDeliverySelectorV1,
	) (domainpublication.ReportDeliveryProjectionV1, error)
}

// LinearizedProjectedDeliveryAuthority executes use synchronously while the
// same witnessed evidence snapshot that proved the delivery remains held.
// This is the release linearization point for controlled bytes; implementations
// must re-read the stable outcome after use returns and fail closed on change.
type LinearizedProjectedDeliveryAuthority interface {
	ProjectedDeliveryAuthority
	WithCurrentProjectedDelivery(
		context.Context,
		ProjectedDeliverySelectorV1,
		func(domainpublication.ReportDeliveryProjectionV1) error,
	) error
}

// HistoricalProjectedDeliveryAuthority proves an already-recorded projected
// outcome and its immutable durable graph. It never proves current context,
// evidence membership, PII authorization, or permission to release bytes.
type HistoricalProjectedDeliveryAuthority interface {
	ResolveTrustedHistoricalProjectedDelivery(
		context.Context,
		HistoricalProjectedDeliverySelectorV1,
	) (domainpublication.ReportDeliveryProjectionV1, error)
}
