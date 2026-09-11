//go:build darwin

package finalauthority

import "testing"

func TestLargeOpaqueDarwinProvenanceFormatIsClosed(t *testing.T) {
	valid := []byte{0x01, 0x02, 0x00, 1, 0, 0, 0, 0, 0, 0, 0}
	if !validLargeOpaqueDarwinProvenance(valid) {
		t.Fatal("valid Darwin provenance record was rejected")
	}
	for _, value := range [][]byte{
		nil,
		{0x01, 0x02, 0x00},
		{0x01, 0x02, 0x00, 0, 0, 0, 0, 0, 0, 0, 0},
		{0x00, 0x02, 0x00, 1, 0, 0, 0, 0, 0, 0, 0},
		{0x01, 0x03, 0x00, 1, 0, 0, 0, 0, 0, 0, 0},
		{0x01, 0x02, 0x01, 1, 0, 0, 0, 0, 0, 0, 0},
		{0x01, 0x02, 0x00, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	} {
		if validLargeOpaqueDarwinProvenance(value) {
			t.Fatalf("invalid Darwin provenance record was accepted: %x", value)
		}
	}
}
