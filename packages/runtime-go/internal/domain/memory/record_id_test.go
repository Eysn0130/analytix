package memory

import "testing"

func TestCanonicalRecordIDRejectsFilesystemAliases(t *testing.T) {
	for _, value := range []string{"mem_go_1", "mem_go_999"} {
		if !IsCanonicalRecordID(value) {
			t.Fatalf("canonical memory id rejected: %q", value)
		}
	}
	for _, value := range []string{
		"", "mem_go_0", "mem_go_01", "mem_go_-1", "mem:go_1", "mem/go_1",
		" mem_go_1", "mem_go_1 ", "../mem_go_1", "mem_go_9223372036854775808",
	} {
		if IsCanonicalRecordID(value) {
			t.Fatalf("memory alias accepted: %q", value)
		}
	}
}
