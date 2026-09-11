package evidenceregistry

import (
	"context"
	"encoding/json"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type CommitPreparedInput struct {
	Context           domainsecurity.TurnSecurityContext
	Draft             domainevidence.EvidenceReceipt
	CanonicalEvidence json.RawMessage
	SettlementProof   domainevidence.EvidenceSettlementProof
	RegisteredAt      time.Time
}

type MembershipQuery struct {
	Context   domainsecurity.TurnSecurityContext
	ReceiptID string
}

type RevokeInput struct {
	Context    domainsecurity.TurnSecurityContext
	ReceiptID  string
	ReasonCode string
	RevokedAt  time.Time
}

// InventoryRecord is one complete, validated private registry. Context is
// supplied by the durable frozen turn rather than reconstructed from a
// receipt, whose public shape intentionally omits workspace and actor fields.
type InventoryRecord struct {
	Context  domainsecurity.TurnSecurityContext
	Registry domainevidence.EvidenceReceiptRegistry
}

type Registry interface {
	CommitPrepared(context.Context, CommitPreparedInput) (domainevidence.EvidenceReceipt, error)
	Resolve(context.Context, MembershipQuery) (domainevidence.RegisteredEvidence, error)
	Revoke(context.Context, RevokeInput) error
	Replay(context.Context, domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error)
}

// LockedSnapshot linearizes final-gate evaluation and publication with
// receipt issuance/revocation. The callback must use the supplied immutable
// snapshot and must not call the locking registry again.
type LockedSnapshot interface {
	WithLockedSnapshot(context.Context, domainsecurity.TurnSecurityContext, func(domainevidence.EvidenceReceiptRegistry) error) error
}

// HistoricalReplay recovers the exact prefix against which an accepted final
// was sealed. Later registry entries do not rewrite publication history.
type HistoricalReplay interface {
	ReplayAt(context.Context, domainsecurity.TurnSecurityContext, uint64) (domainevidence.EvidenceReceiptRegistry, error)
}

// Inventory performs a strict whole-root walk. Unknown files, symlinks,
// legacy schemas, path collisions, and registries detached from a supplied
// durable turn context must fail closed.
type Inventory interface {
	ListRegistries(context.Context, []domainsecurity.TurnSecurityContext) ([]InventoryRecord, error)
	HasRecords(context.Context) (bool, error)
}
