//go:build windows

package persistencefs

import "testing"

func TestCanonicalPathsEqualForDeviceUsesWindowsCaseSemantics(t *testing.T) {
	if !canonicalPathsEqualForDevice(`C:\Analytix\Data`, `c:\analytix\data`, 0) {
		t.Fatal("Windows device-aware path equality rejected a case alias")
	}
	if canonicalPathsEqualForDevice(`C:\Analytix\Data`, `C:\Analytix\Other`, 0) {
		t.Fatal("Windows device-aware path equality accepted a different path")
	}
}
