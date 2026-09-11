//go:build darwin || linux

package finalauthority

import (
	"fmt"
	"reflect"
	"testing"
)

func TestLargeOpaqueRecoveryAccumulatorIsOrderAndPartitionInvariant(t *testing.T) {
	type record struct {
		kind     largeOpaqueRecoveryEntryKind
		address  string
		name     string
		identity largeOpaqueUnixFileIdentity
	}
	records := make([]record, 257)
	for index := range records {
		records[index] = record{
			kind:    largeOpaqueRecoveryEntryCommitted,
			address: fmt.Sprintf("%064x", index+1),
			name:    fmt.Sprintf("%064x.blob", index+1),
			identity: largeOpaqueUnixFileIdentity{
				device: 1, inode: uint64(index + 10), mode: 0o100400, links: 1,
				uid: 501, gid: 20, size: int64(index + 1),
			},
		}
	}
	var direct largeOpaqueRecoveryAccumulator
	for _, record := range records {
		direct.add(record.kind, record.address, record.name, record.identity)
	}
	var reversed largeOpaqueRecoveryAccumulator
	for index := len(records) - 1; index >= 0; index-- {
		record := records[index]
		reversed.add(record.kind, record.address, record.name, record.identity)
	}
	if reversed != direct {
		t.Fatal("large opaque recovery accumulator depends on enumeration order")
	}
	var merged largeOpaqueRecoveryAccumulator
	for offset := 0; offset < len(records); offset += 17 {
		end := offset + 17
		if end > len(records) {
			end = len(records)
		}
		var partition largeOpaqueRecoveryAccumulator
		for _, record := range records[offset:end] {
			partition.add(record.kind, record.address, record.name, record.identity)
		}
		merged.merge(partition)
	}
	if merged != direct {
		t.Fatal("large opaque recovery accumulator merge changed its commitment")
	}
}

func TestLargeOpaqueRecoveryAccumulatorBindsFileTimes(t *testing.T) {
	identity := largeOpaqueUnixFileIdentity{
		device: 1, inode: 2, mode: 0o100400, links: 1,
		uid: 501, gid: 20, size: 3,
	}
	var before largeOpaqueRecoveryAccumulator
	before.add(largeOpaqueRecoveryEntryCommitted, "address", "name", identity)
	identity.times[31] = 1
	var after largeOpaqueRecoveryAccumulator
	after.add(largeOpaqueRecoveryEntryCommitted, "address", "name", identity)
	if before == after {
		t.Fatal("large opaque recovery accumulator ignored file time identity")
	}
}

func TestLargeOpaqueRecoveryObservationCannotHoldMaterializedInventory(t *testing.T) {
	assertLargeOpaqueRecoveryConstantMemoryType(t, reflect.TypeOf(largeOpaqueRecoveryObservation{}), map[reflect.Type]bool{})
}

func assertLargeOpaqueRecoveryConstantMemoryType(
	t *testing.T,
	typeOf reflect.Type,
	visiting map[reflect.Type]bool,
) {
	t.Helper()
	if visiting[typeOf] {
		return
	}
	visiting[typeOf] = true
	defer delete(visiting, typeOf)
	switch typeOf.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.String:
		t.Fatalf("large opaque recovery observation contains dynamic inventory-capable type %s", typeOf)
	case reflect.Array:
		assertLargeOpaqueRecoveryConstantMemoryType(t, typeOf.Elem(), visiting)
	case reflect.Struct:
		for index := 0; index < typeOf.NumField(); index++ {
			assertLargeOpaqueRecoveryConstantMemoryType(t, typeOf.Field(index).Type, visiting)
		}
	}
}
