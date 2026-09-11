package rawartifact

import (
	"reflect"
	"testing"
)

func TestLargeOpaqueChunkStoreSurfaceCannotEnumerateOrSelectAuthority(t *testing.T) {
	typeOf := reflect.TypeOf((*LargeOpaqueChunkStore)(nil)).Elem()
	if typeOf.NumMethod() != 2 {
		t.Fatalf("large opaque chunk store method count = %d", typeOf.NumMethod())
	}
	for _, name := range []string{"PutExact", "ReadExact"} {
		if _, present := typeOf.MethodByName(name); !present {
			t.Fatalf("large opaque chunk store method %s is missing", name)
		}
	}
	for _, forbidden := range []string{"List", "Visit", "Latest", "Current", "Active", "Path", "Open"} {
		if _, present := typeOf.MethodByName(forbidden); present {
			t.Fatalf("large opaque chunk store exposes forbidden selector %s", forbidden)
		}
	}

	outcomeType := reflect.TypeOf(LargeOpaqueChunkWriteOutcome{})
	for _, forbidden := range []string{"Path", "File", "Handle", "Receipt", "Evidence"} {
		if _, present := outcomeType.FieldByName(forbidden); present {
			t.Fatalf("large opaque chunk outcome exposes forbidden field %s", forbidden)
		}
	}
}
