package persistencefs

import (
	"context"
	"errors"
	"strings"
)

// ValidateNoSemanticStartupStateForPrivateCASRecoveryV1 is a read-only
// coexistence guard. A signed private-CAS deletion session is created only
// after semantic journal/planning cleanup, so observing either state on resume
// is an impossible overlap and must preserve both authorities fail-closed.
func ValidateNoSemanticStartupStateForPrivateCASRecoveryV1(
	ctx context.Context,
	roots RootSet,
	authority *JournalNamespaceAuthority,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if authority == nil || !authority.matchesRoots(roots) || authority.Validate() != nil {
		return errors.New("private CAS recovery semantic coexistence authority is invalid")
	}
	root, err := secureStartupOpenRootDirectory(authority.root)
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := root.ReadEntriesBoundedContext(ctx, maxSemanticNamespaceEntries)
	if err != nil {
		return err
	}
	planningName := "planning-" + rootBindingDigest(roots)
	for _, entry := range entries {
		name := entry.Name()
		if name == "journal" || name == planningName || strings.HasPrefix(name, ".retired-journal-") {
			return errors.New("signed private CAS recovery cannot coexist with semantic startup state")
		}
	}
	return authority.Validate()
}
