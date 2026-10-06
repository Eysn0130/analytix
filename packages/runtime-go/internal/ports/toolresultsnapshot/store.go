package toolresultsnapshot

import (
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
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
}

type AccessAuthority interface {
	AcquireCurrent(context.Context, SelectorV1) (AuthorizedV1, func(), error)
	ValidateCurrent(context.Context, AuthorizedV1) error
}
