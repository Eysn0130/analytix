package memory

import "testing"

func TestFiveMemoryOwnersKeepDistinctRetentionAndRevokePaths(t *testing.T) {
	boundaries := OwnerBoundariesV1()
	if len(boundaries) != 5 {
		t.Fatalf("owner boundary count mismatch: %d", len(boundaries))
	}
	names := map[string]struct{}{}
	retention := map[string]struct{}{}
	revoke := map[string]struct{}{}
	generalFreeform := 0
	for _, boundary := range boundaries {
		if boundary.Name == "" || boundary.PackageOwner == "" || boundary.ContentKind == "" ||
			boundary.Retention == "" || boundary.RevokeTrigger == "" {
			t.Fatalf("incomplete owner boundary: %#v", boundary)
		}
		if _, duplicate := names[boundary.Name]; duplicate {
			t.Fatalf("duplicate owner boundary: %q", boundary.Name)
		}
		names[boundary.Name] = struct{}{}
		retention[boundary.Retention] = struct{}{}
		revoke[boundary.RevokeTrigger] = struct{}{}
		if boundary.AllowsGeneralFreeform {
			generalFreeform++
			if boundary.Name != "user_approved_general_memory" || boundary.ContentKind != "manual_general_freeform" {
				t.Fatalf("general free-form authority escaped Memory: %#v", boundary)
			}
		}
	}
	if len(retention) != 5 || len(revoke) != 5 || generalFreeform != 1 {
		t.Fatalf("owners share retention/revoke or free-form authority: retention=%d revoke=%d general=%d", len(retention), len(revoke), generalFreeform)
	}
}
