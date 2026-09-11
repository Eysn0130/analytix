//go:build !analytix_prod

package conformance

import "testing"

func TestProviderCacheReleaseGuardRejectsAnyToleranceOrLowTail(t *testing.T) {
	highCurve := ProviderReleaseGuardCase{
		ID: "high", CacheHitPercentCurve: []float64{90, 91, 92, 93},
	}
	tolerance := buildG5ProviderReleaseGuard(ProviderReleaseGuard{
		FixtureOnly: true, ThresholdPercent: 90, MaxLowTailCases: 1,
		TailWindow: 4, Cases: []ProviderReleaseGuardCase{highCurve},
	})
	if tolerance["status"] != "fail" || tolerance["lowTailCases"] != 0 {
		t.Fatalf("nonzero low-tail tolerance was accepted: %#v", tolerance)
	}

	lowTail := buildG5ProviderReleaseGuard(ProviderReleaseGuard{
		FixtureOnly: true, ThresholdPercent: 90, MaxLowTailCases: 0,
		TailWindow: 4, Cases: []ProviderReleaseGuardCase{{
			ID: "low", CacheHitPercentCurve: []float64{84, 85, 86, 87},
			CompactionGuardPaused: true,
		}},
	})
	if lowTail["status"] != "fail" || lowTail["lowTailCases"] != 1 {
		t.Fatalf("low cache tail was not rejected: %#v", lowTail)
	}
	cases, ok := lowTail["cases"].([]map[string]any)
	if !ok || len(cases) != 1 || cases[0]["status"] != "fail" {
		t.Fatalf("low cache case retained a waiver: %#v", lowTail)
	}
}
