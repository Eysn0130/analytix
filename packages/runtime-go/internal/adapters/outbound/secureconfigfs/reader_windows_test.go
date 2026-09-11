//go:build windows

package secureconfigfs

import "testing"

func TestWindowsSafeComponentRejectsAliasesADSAndDeviceNames(t *testing.T) {
	for _, valid := range []string{"Users", "Analytix Config", "manifest.json", "资料"} {
		if !windowsSafeComponent(valid) {
			t.Fatalf("safe Windows component %q was rejected", valid)
		}
	}
	for _, invalid := range []string{
		"", ".", "..", "name ", "name.", "name:stream", `a\b`, "NUL", "nul.tar.gz", "COM1.txt", "LPT³.log", "bad\x00name",
	} {
		if windowsSafeComponent(invalid) {
			t.Fatalf("unsafe Windows component %q was accepted", invalid)
		}
	}
	for _, invalid := range []string{"Manifest.json", "资料", "name space", "CON.txt"} {
		if windowsSafeInventoryName(invalid) {
			t.Fatalf("unsafe Windows inventory name %q was accepted", invalid)
		}
	}
}

func TestWindowsBundleNamesRejectCaseFoldAliases(t *testing.T) {
	input := normalizedBundle{
		expectedNames: []string{"profile.json", "PROFILE.JSON"},
		files:         []BundleFile{{Name: "profile.json", MaxBytes: 1}, {Name: "PROFILE.JSON", MaxBytes: 1}},
	}
	if windowsBundleNamesSafe(input) {
		t.Fatal("case-fold aliasing Windows inventory was accepted")
	}
}
