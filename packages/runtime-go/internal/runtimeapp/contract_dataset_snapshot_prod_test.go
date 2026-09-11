//go:build analytix_prod

package runtimeapp

import "testing"

func TestContractDatasetSnapshotAuthorityIsAlwaysDisabledInProduction(t *testing.T) {
	authority, enabled, err := newContractDatasetSnapshotAuthority(Config{DurableTempDir: t.TempDir()}, nil)
	if err != nil || enabled || authority != nil {
		t.Fatalf("production contract dataset authority must remain disabled: authority=%T enabled=%t err=%v", authority, enabled, err)
	}
}
