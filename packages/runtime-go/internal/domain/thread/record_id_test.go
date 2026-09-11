package thread

import "testing"

func TestCanonicalRecordIDRejectsStorageAliases(t *testing.T) {
	for _, value := range []string{"thr_durable_1", "thread-a", "session.1", "A-0_1"} {
		if !IsCanonicalRecordID(value) {
			t.Fatalf("canonical record ID rejected: %q", value)
		}
	}
	for _, value := range []string{
		"", " thr_durable_1", "thr_durable_1 ", "thr:durable_1", "thr/durable/1", `thr\durable\1`,
		".", "..", "thr..durable", "_leading", "含案件事实",
	} {
		if IsCanonicalRecordID(value) {
			t.Fatalf("storage alias accepted: %q", value)
		}
	}
}
