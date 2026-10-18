package userinputdisplay

import (
	"context"
	"errors"
	"strings"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

var ErrUnavailableV1 = errors.New("protected user input display unavailable")

const MaxResponseBytesV1 = (8 << 20) + (64 << 10)

type SelectorV1 struct {
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId"`
	ItemID    string `json:"itemId"`
	RequestID string `json:"requestId"`
}

func ValidateSelectorV1(s SelectorV1) bool {
	return domainthread.IsCanonicalRecordID(s.ThreadID) && domainthread.IsCanonicalRecordID(s.TurnID) &&
		s.ItemID == "item_"+s.RequestID && strings.HasPrefix(s.RequestID, "input_") &&
		domainsecurity.IsSHA256Hex(strings.TrimPrefix(s.RequestID, "input_"))
}

// AuthorizedV1 is an internal Host witness. It is never decoded from the wire
// and is never included in a display response.
type AuthorizedV1 struct {
	ReadOnly      bool
	Origin        SelectorV1
	HistoryDigest string
	Selector      SelectorV1
	Principal     domainidentity.PrincipalV1
	Workspace     string
	Original      domainsecurity.TurnSecurityContext
	ReceiptID     string
	Status        string
}

type AccessAuthority interface {
	AcquireCurrent(context.Context, SelectorV1) (AuthorizedV1, func(), error)
	ValidateCurrent(context.Context, AuthorizedV1) error
}
