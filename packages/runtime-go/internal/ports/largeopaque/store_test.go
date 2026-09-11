package largeopaque

import (
	"reflect"
	"testing"
)

func TestStoreSurfaceIsExactAndNonEnumerable(t *testing.T) {
	typeOf := reflect.TypeOf((*Store)(nil)).Elem()
	if typeOf.NumMethod() != 2 {
		t.Fatalf("large opaque store method count = %d", typeOf.NumMethod())
	}
	for _, name := range []string{"PutExact", "ReadExact"} {
		if _, present := typeOf.MethodByName(name); !present {
			t.Fatalf("large opaque store method %s is missing", name)
		}
	}
	for _, forbidden := range []string{"List", "Visit", "Latest", "Current", "Active", "Path", "Open"} {
		if _, present := typeOf.MethodByName(forbidden); present {
			t.Fatalf("large opaque store exposes forbidden selector %s", forbidden)
		}
	}
}
