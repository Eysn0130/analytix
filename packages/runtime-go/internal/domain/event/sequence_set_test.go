package event

import "testing"

func TestSequenceSetV1RequiresPositiveUniqueContiguousValues(t *testing.T) {
	var set SequenceSetV1
	for _, sequence := range []int64{7, 9, 8, 10} {
		if err := set.AddPositiveUnique(sequence); err != nil {
			t.Fatal(err)
		}
	}
	if err := set.ValidateContiguous(); err != nil {
		t.Fatal(err)
	}
	if err := set.AddPositiveUnique(8); err == nil {
		t.Fatal("duplicate event sequence was accepted")
	}

	var gap SequenceSetV1
	for _, sequence := range []int64{7, 9} {
		if err := gap.AddPositiveUnique(sequence); err != nil {
			t.Fatal(err)
		}
	}
	if err := gap.ValidateContiguous(); err == nil {
		t.Fatal("event sequence gap was accepted")
	}
	if err := gap.AddPositiveUnique(0); err == nil {
		t.Fatal("zero event sequence was accepted")
	}
}
