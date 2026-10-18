package processdisplay

import (
	"context"
	"errors"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrUnavailableV1 = errors.New("protected process display unavailable")
var ErrCapacityV1 = errors.New("settled process history capacity unavailable")

type SettledHistoryStoreV1 interface {
	Put(context.Context, display.SettledHistoryV1) (display.SettledHistoryAnchorV1, error)
	Read(context.Context, display.SettledHistoryAnchorV1) (display.SettledHistoryV1, error)
}

// History read is host-owned and still requires the ordinary fresh native
// principal/context lease. A digest or a restored record is no viewer grant.
type SettledHistoryReadAuthorityV1 interface {
	ReadSettledHistoryV1(context.Context, string, string) (display.SettledHistoryV1, display.SettledHistoryAnchorV1, error)
	ValidateSettledHistoryCurrentV1(context.Context, BindingV1, display.SettledHistoryAnchorV1) error
}

// BindingV1 is host-owned. It cannot be decoded from the viewer request.
type BindingV1 struct {
	ThreadID  string                             `json:"-"`
	TurnID    string                             `json:"-"`
	Principal domainidentity.PrincipalV1         `json:"-"`
	Original  domainsecurity.TurnSecurityContext `json:"-"`
}
type AuthorizedV1 struct{ Binding BindingV1 }
type ReadAuthorityV1 interface {
	AcquireCurrent(context.Context, BindingV1) (AuthorizedV1, func(), error)
	ValidateCurrent(context.Context, AuthorizedV1) error
	ValidateToolCurrent(context.Context, BindingV1, display.ToolBindingV1) error
}

// AssistantDraftReadAuthorityV1 adds currentness for still-running source text.
// It uses the same current native lease; it supplies no new viewer permission.
// Owners without this capability fail closed for assistant drafts.
type AssistantDraftReadAuthorityV1 interface {
	ValidateAssistantDraftCurrent(context.Context, BindingV1) error
}
