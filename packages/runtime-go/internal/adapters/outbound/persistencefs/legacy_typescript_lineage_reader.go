package persistencefs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// ReadLegacyTypeScriptLineageSourceV1 reads one snapshot-bound source during the
// pre-migration lineage freeze. An absent source must have an explicit absent
// entry; callers must first establish its thread directory in the same snapshot.
// Source selection uses present, never an operating-system not-exist fallback.
func (authority *RootAuthority) ReadLegacyTypeScriptLineageSourceV1(
	ctx context.Context,
	threadID, sourceName string,
	expected EntryRecord,
) ([]byte, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if !exactSafeRecordID(threadID) {
		return nil, false, errors.New("legacy lineage thread identity is invalid")
	}
	switch sourceName {
	case "thread.json", "metadata.jsonl", "messages.jsonl":
	default:
		return nil, false, errors.New("legacy lineage source name is invalid")
	}
	relative := filepath.Join("threads", threadID, sourceName)
	if expected.Path != "durable/"+filepath.ToSlash(relative) {
		return nil, false, errors.New("legacy lineage source snapshot path is invalid")
	}
	switch expected.Type {
	case domainstartup.ManagedEntryTypeAbsent:
		if expected.Mode != 0 || expected.Size != 0 || expected.ModTimeUnixNano != 0 || expected.SHA256 != "" || expected.RecordCount != 0 {
			return nil, false, errors.New("legacy lineage absent source snapshot is invalid")
		}
	case domainstartup.ManagedEntryTypeFile:
		if expected.Mode == 0 || expected.Size < 0 || expected.Size > domainstartup.MaxSemanticManagedFileBytesV1 || !domainsecurity.IsSHA256Hex(expected.SHA256) {
			return nil, false, errors.New("legacy lineage file source snapshot is invalid")
		}
	default:
		return nil, false, errors.New("legacy lineage source snapshot type is invalid")
	}
	roots, ok := authority.Roots()
	if !ok {
		return nil, false, errors.New("legacy lineage root authority is unavailable")
	}
	if err := authority.Validate(); err != nil {
		return nil, false, fmt.Errorf("legacy lineage root authority changed: %v", err)
	}
	body, present, err := readLegacyTypeScriptLineageManagedSourceV1(ctx, authority, roots.DurableDir, relative, domainstartup.SemanticEntryStateV1{
		Type: expected.Type, Mode: expected.Mode, Size: expected.Size, SHA256: expected.SHA256,
	})
	if err == nil {
		err = authority.Validate()
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		clear(body)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, false, contextErr
		}
		// A snapshot-present file disappearing is drift, not permission to select
		// another source. Do not expose an underlying not-exist classification.
		return nil, false, fmt.Errorf("legacy lineage source snapshot changed: %v", err)
	}
	return body, present, nil
}
