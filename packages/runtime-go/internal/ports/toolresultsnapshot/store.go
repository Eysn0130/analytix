package toolresultsnapshot

import (
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"context"
	"errors"
)

var ErrUnavailable = errors.New("protected tool result unavailable")
var ErrCapacity = errors.New("protected tool result snapshot capacity unavailable")

type Store interface {
	Put(context.Context, domaintoolresult.ProtectedSnapshotV1) (domaintoolresult.ProtectedSnapshotBindingV1, error)
	Read(context.Context, domaintoolresult.ProtectedSnapshotBindingV1) (domaintoolresult.ProtectedSnapshotV1, error)
}

type SelectorV1 struct {
	ThreadID     string `json:"threadId"`
	TurnID       string `json:"turnId"`
	CallID       string `json:"callId"`
	ResultItemID string `json:"resultItemId"`
}

// AuthorizedV1 is created by Host current authority, never by a wire decoder.
// Its tuple and immutable binding come only from original strict primary data.
type AuthorizedV1 struct {
	Selector         SelectorV1
	Principal        domainidentity.PrincipalV1
	Workspace        string
	ToolName         string
	ContextDigest    string
	ContextEpoch     uint64
	ExecutionGrantID string
	CaseID           string
	CaseBindingHash  string
	Binding          domaintoolresult.ProtectedSnapshotBindingV1
	CallEventAt      string
	ResultEventAt    string
}

type AccessAuthority interface {
	AcquireCurrent(context.Context, SelectorV1) (AuthorizedV1, func(), error)
	ValidateCurrent(context.Context, AuthorizedV1) error
}

// Present history carries only real primary identities. A result anchor is a
// receipt candidate, not a successful declaration or a filesystem capability.
const PresentHistoryKindV1 = "present_history"

type PresentHistorySelectorV1 struct {
	Kind     string `json:"kind"`
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type PresentHistoryItemV1 struct {
	Kind     string `json:"kind"`
	ItemID   string `json:"itemId"`
	CallID   string `json:"callId,omitempty"`
	ToolName string `json:"toolName,omitempty"`
}

// The source hash and authority are Host-internal fresh observations. Neither
// becomes a wire revision, public coordinate, or new historical grant.
type PresentHistoryAuthorizedV1 struct {
	Selector         PresentHistorySelectorV1
	Principal        domainidentity.PrincipalV1
	Workspace        string
	Original         domainsecurity.TurnSecurityContext
	ThreadFileSHA256 string
	OrderedItems     []PresentHistoryItemV1
}

type PresentHistoryAccessAuthorityV1 interface {
	AcquirePresentHistoryCurrentV1(context.Context, PresentHistorySelectorV1) (PresentHistoryAuthorizedV1, func(), error)
	ValidatePresentHistoryCurrentV1(context.Context, PresentHistoryAuthorizedV1) error
}
