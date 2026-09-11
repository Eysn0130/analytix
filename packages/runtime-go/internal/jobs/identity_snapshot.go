package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

// ChildRunIdentitySnapshotV1 retains raw committed records and the complete
// physical denominator, including occupied artifact/residue names. No legacy
// projection, normalization, completion recovery, or allocation runs here.
type ChildRunIdentitySnapshotV1 struct {
	Inventory           ChildRunInventoryV1
	Records             []Record
	HasLegacyTypeScript bool
}

func ReadChildRunIdentitySnapshotV1(ctx context.Context, root string) (ChildRunIdentitySnapshotV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return ChildRunIdentitySnapshotV1{}, errors.New("child-run identity snapshot context is unavailable")
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		return ChildRunIdentitySnapshotV1{}, err
	}
	result := ChildRunIdentitySnapshotV1{Inventory: inventory}
	for _, entry := range inventory.Entries {
		if err := ctx.Err(); err != nil {
			return ChildRunIdentitySnapshotV1{}, err
		}
		if entry.Kind == ChildRunInventoryLegacyTypeScriptRecordV1 {
			// The retired producer cannot carry a current job-N childProducer
			// witness. Preserve this fact for the Core startup refusal; never
			// project a replacement identity while collecting the denominator.
			result.HasLegacyTypeScript = true
			continue
		}
		if entry.Kind != ChildRunInventoryRecordV1 {
			continue
		}
		data, err := readChildRunInventoryEntryV1(root, entry)
		if err != nil {
			return ChildRunIdentitySnapshotV1{}, err
		}
		record, err := ParseChildRunIdentityRecordV1(data, entry.JobID)
		if err != nil {
			return ChildRunIdentitySnapshotV1{}, err
		}
		result.Records = append(result.Records, record)
	}
	if err := ValidateChildRunInventoryV1(root, inventory); err != nil {
		return ChildRunIdentitySnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return ChildRunIdentitySnapshotV1{}, err
	}
	return result, nil
}

// ParseChildRunIdentityRecordV1 is the same strict, read-only identity parser
// for original committed bytes and a signed in-memory candidate. It performs
// no normalization or operational/completion-authority admission.
func ParseChildRunIdentityRecordV1(data []byte, jobID string) (Record, error) {
	if err := domainjsonstrict.Validate(data, domainjsonstrict.Options{RequireObject: true, MaxBytes: childRunInventoryRecordMaxBytesV1, MaxTokens: 200_000, MaxStringBytes: 8 * 1024 * 1024}); err != nil {
		return Record{}, errors.New("child-run identity JSON is invalid")
	}
	// Compatibility only unwraps a historical handoff field, without changing
	// record identity or the source bytes.
	data, err := compactEmbeddedForegroundHandoffReceiptV1(data)
	if err != nil {
		return Record{}, err
	}
	var record Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, errors.New("child-run identity record shape is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) || !validPersistedJobID(record.ID) || record.ID != jobID {
		return Record{}, errors.New("child-run identity record binding is invalid")
	}
	return record, nil
}
