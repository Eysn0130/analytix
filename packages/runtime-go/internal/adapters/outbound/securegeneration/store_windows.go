//go:build windows

package securegeneration

import (
	"context"
	"os"

	domainartifact "analytix.local/runtime-go/internal/domain/artifactgeneration"
)

// Windows is intentionally unavailable until handle-relative no-replace
// directory publication, DACL/reparse validation, directory durability, and
// crash recovery are executed on an approved Windows host. Compiling this
// boundary is not a claim that those filesystem guarantees were run.
type rootAuthority struct{}

func openRootAuthority(string, bool) (rootAuthority, error) { return rootAuthority{}, ErrUnsupported }
func openRootAuthorityUnder(*os.File, string, bool) (rootAuthority, error) {
	return rootAuthority{}, ErrUnsupported
}
func closeRootAuthority(rootAuthority) error           { return nil }
func rootAuthorityCreatedByCall(rootAuthority) bool    { return false }
func rollbackCreatedRootAuthority(rootAuthority) error { return nil }
func discardExpectedUnder(*Store, context.Context, domainartifact.ReceiptV1) error {
	return ErrUnsupported
}

func discardCurrentUnder(*Store, context.Context) (DiscardCurrentResult, error) {
	return DiscardCurrentResult{}, ErrUnsupported
}

func observe(*Store, context.Context) (Observation, error) { return Observation{}, ErrUnsupported }

func publish(*Store, context.Context, domainartifact.PreparedV1, *ExpectedCurrent) (PublishResult, error) {
	return PublishResult{State: NotCommitted}, ErrUnsupported
}

func recoverStore(*Store, context.Context) (Observation, error) { return Observation{}, ErrUnsupported }
