package turn

import "testing"

func TestSequenceIDRecognizesCurrentAndLegacyPrefixes(t *testing.T) {
	if seq, ok := SequenceID("turn_42"); !ok || seq != 42 {
		t.Fatalf("current turn sequence mismatch: seq=%d ok=%v", seq, ok)
	}
	if seq, ok := SequenceID("turn_d0242_7"); !ok || seq != 7 {
		t.Fatalf("legacy turn sequence mismatch: seq=%d ok=%v", seq, ok)
	}
	if _, ok := SequenceID("turn_0"); ok {
		t.Fatal("zero sequence should not be accepted")
	}
}
