package fundsquerysource

// ImmutableSnapshotObjectV1 is the minimum host-private identity needed to
// install one completed analytical DuckDB before its DSV2 selection is
// published. It is inert storage metadata, not dataset authority.
type ImmutableSnapshotObjectV1 struct {
	CaseID           string
	DuckDBSHA256     string
	DuckDBByteLength uint64
}

func NewImmutableSnapshotObjectV1(
	caseID string,
	duckDBSHA256 string,
	duckDBByteLength uint64,
) (ImmutableSnapshotObjectV1, error) {
	object := ImmutableSnapshotObjectV1{
		CaseID:           caseID,
		DuckDBSHA256:     duckDBSHA256,
		DuckDBByteLength: duckDBByteLength,
	}
	if err := ValidateImmutableSnapshotObjectV1(object); err != nil {
		return ImmutableSnapshotObjectV1{}, err
	}
	return object, nil
}

func ValidateImmutableSnapshotObjectV1(object ImmutableSnapshotObjectV1) error {
	if !caseIDPatternV1.MatchString(object.CaseID) ||
		object.CaseID == "" ||
		!canonicalDigestV1(object.DuckDBSHA256) ||
		object.DuckDBByteLength == 0 ||
		object.DuckDBByteLength > MaxDuckDBByteLengthV1 {
		return ErrImmutableSnapshotObjectInvalidV1
	}
	return nil
}
