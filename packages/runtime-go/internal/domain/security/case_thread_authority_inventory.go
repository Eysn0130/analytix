package security

import (
	"errors"
	"strings"
)

// ValidateCaseThreadAuthorityInventoryV1 validates the complete value graph.
// Installation-key trust and physical/canonical file authority remain separate
// caller obligations. The same graph rules apply to original and signed After
// observations, including records not selected by a particular turn lookup.
func ValidateCaseThreadAuthorityInventoryV1(records []CaseThreadAuthorityRecord) error {
	byDigest := make(map[string]CaseThreadAuthorityRecord, len(records))
	for _, record := range records {
		if err := ValidateCaseThreadAuthorityRecord(record); err != nil {
			return err
		}
		if _, found := byDigest[record.RecordDigest]; found {
			return errors.New("case thread authority inventory contains a duplicate digest")
		}
		byDigest[record.RecordDigest] = record
	}
	for _, record := range records {
		seen := map[string]bool{record.RecordDigest: true}
		for CaseThreadAuthorityIsLineage(record) {
			parent, found := byDigest[record.ParentRecordDigest]
			if !found || CaseThreadAuthorityThreadID(parent) != strings.TrimSpace(record.ParentThreadID) {
				return errors.New("case thread lineage lost its exact parent authority")
			}
			if seen[parent.RecordDigest] {
				return errors.New("case thread lineage inventory contains a cycle")
			}
			seen[parent.RecordDigest] = true
			record = parent
		}
	}
	return nil
}
