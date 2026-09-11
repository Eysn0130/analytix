//go:build analytix_prod

package runtimeapp

import "testing"

func TestContractThreadRiskAuthorityIsAlwaysDisabledInProduction(t *testing.T) {
	for _, config := range []Config{
		{DurableTempDir: t.TempDir()},
		{DurableTempDir: t.TempDir(), ProductionDurableRoot: t.TempDir()},
	} {
		authority, enabled, err := newContractThreadRiskAuthority(config, nil)
		if err != nil || enabled || authority != nil {
			t.Fatalf("production local witness fallback was enabled: authority=%T enabled=%v err=%v", authority, enabled, err)
		}
	}
}
