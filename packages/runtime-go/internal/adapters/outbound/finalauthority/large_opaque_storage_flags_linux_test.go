//go:build linux

package finalauthority

import "testing"

func TestLargeOpaqueStorageFlagsOnlyAllowExtentFormat(t *testing.T) {
	const extent = 0x00080000
	for _, format := range []int{0, extent} {
		if !largeOpaqueStorageFlagsSafe(format) {
			t.Fatalf("ordinary storage format %#x rejected", format)
		}
		for bit := uint(0); bit < 32; bit++ {
			flag := int(uint32(1) << bit)
			if flag != extent && largeOpaqueStorageFlagsSafe(format|flag) {
				t.Errorf("non-format inode flag %#x accepted with format %#x", flag, format)
			}
		}
	}
	if largeOpaqueStorageFlagsSafe(-1) {
		t.Fatal("invalid flags accepted")
	}
}
