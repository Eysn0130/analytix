package fundsquerysource

import (
	"errors"
	"testing"
)

func TestImmutableSnapshotObjectV1Validation(t *testing.T) {
	valid, err := NewImmutableSnapshotObjectV1(
		"case-install-001",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		4096,
	)
	if err != nil || ValidateImmutableSnapshotObjectV1(valid) != nil {
		t.Fatalf("valid immutable snapshot object rejected: object=%#v err=%v", valid, err)
	}
	for name, candidate := range map[string]ImmutableSnapshotObjectV1{
		"path case":  {CaseID: "../case", DuckDBSHA256: valid.DuckDBSHA256, DuckDBByteLength: valid.DuckDBByteLength},
		"uppercase":  {CaseID: valid.CaseID, DuckDBSHA256: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", DuckDBByteLength: valid.DuckDBByteLength},
		"empty":      {CaseID: valid.CaseID, DuckDBSHA256: valid.DuckDBSHA256},
		"over limit": {CaseID: valid.CaseID, DuckDBSHA256: valid.DuckDBSHA256, DuckDBByteLength: MaxDuckDBByteLengthV1 + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateImmutableSnapshotObjectV1(candidate); !errors.Is(err, ErrImmutableSnapshotObjectInvalidV1) {
				t.Fatalf("invalid immutable snapshot object error=%v", err)
			}
		})
	}
}
