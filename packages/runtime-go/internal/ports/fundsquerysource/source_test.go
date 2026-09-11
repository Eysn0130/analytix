package fundsquerysource

import (
	"reflect"
	"testing"
)

func TestSourceAndReadLeaseExposeNoPathInventoryOrSQLSurface(t *testing.T) {
	sourceType := reflect.TypeOf((*HostExactSource)(nil)).Elem()
	if sourceType.NumMethod() != 1 {
		t.Fatalf("funds query host source method count = %d", sourceType.NumMethod())
	}
	if _, present := sourceType.MethodByName("WithExact"); !present {
		t.Fatal("funds query host source is missing WithExact")
	}
	for _, forbidden := range []string{
		"List", "Visit", "Latest", "Current", "Path", "Open", "Handle", "Query", "SQL", "Put", "Register",
	} {
		if _, present := sourceType.MethodByName(forbidden); present {
			t.Fatalf("funds query host source exposes forbidden method %q", forbidden)
		}
	}

	leaseType := reflect.TypeOf((*ExactReadLease)(nil)).Elem()
	if leaseType.NumMethod() != 1 {
		t.Fatalf("funds query source read lease method count = %d", leaseType.NumMethod())
	}
	if _, present := leaseType.MethodByName("CopyExactTo"); !present {
		t.Fatal("funds query source read lease is missing CopyExactTo")
	}
}
