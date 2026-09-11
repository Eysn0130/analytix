package persistencefs

import (
	"errors"
	"testing"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

func TestPresentRootCapabilityValidationUsesOneStrongIdentitySnapshot(t *testing.T) {
	identity := privatecasport.DirectoryIdentity{
		Kind:   privatecasport.DirectoryIdentityUnix,
		Device: 7,
		Inode:  11,
	}
	legacy, err := formatStrongDirectoryIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	capability := frozenRootCapability{
		Root:           "/frozen-root",
		Anchor:         "/frozen-root",
		AnchorIdentity: legacy,
		RootIdentity:   legacy,
	}
	calls := 0
	probe := func(path string) (privatecasport.DirectoryIdentity, string, error) {
		calls++
		if path != capability.Root {
			return privatecasport.DirectoryIdentity{}, "", errors.New("unexpected path")
		}
		return identity, legacy, nil
	}
	if err := validateFrozenRootCapabilityWithStrongIdentity(capability, identity, true, probe); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("strong identity snapshots = %d, want 1", calls)
	}
}
