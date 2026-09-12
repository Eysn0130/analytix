package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// Entries are bound to the exact original family and primary digest captured
// before recovery. Normalized views and sidecars cannot replace that source.
type runtimeOriginalPrimaryEntryV1 struct {
	reader          *finalauthority.AcceptedFinalCASReader
	digest          string
	primary, events persistencefs.EntryRecord
	eventRoot       string
}

type runtimeOriginalPrimaryInventoryV1 struct {
	entries map[string]runtimeOriginalPrimaryEntryV1
	roots   persistencefs.RootSet
}

func (inventory *runtimeOriginalPrimaryInventoryV1) ObserveOriginalPrimaryPresenceV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, bool, error) {
	if inventory == nil || ctx == nil || !domainthread.IsCanonicalRecordID(id) {
		return recoveryport.PrimaryThreadSnapshotV1{}, false, errors.New("original child primary presence authority is unavailable")
	}
	if _, exists := inventory.entries[id]; exists {
		snapshot, err := inventory.ReadPrimaryThreadSnapshotV1(ctx, id)
		return snapshot, err == nil, err
	}
	// The complete original two-family inventory proved this identity absent.
	// Reobserve the managed denominator without creating or normalizing files;
	// neither a read error nor a newly appearing tree can stand in for absence.
	current, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, inventory.roots)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, false, err
	}
	for _, entry := range current.Entries {
		for _, path := range []string{"durable/threads/" + id, "durable/runtime-go/threads/" + id} {
			if entry.Path == path || strings.HasPrefix(entry.Path, path+"/") {
				return recoveryport.PrimaryThreadSnapshotV1{}, false, errors.New("reserved absent child primary appeared")
			}
		}
	}
	return recoveryport.PrimaryThreadSnapshotV1{}, false, ctx.Err()
}

func (inventory *runtimeOriginalPrimaryInventoryV1) ReadPrimaryThreadSnapshotV1(ctx context.Context, id string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if inventory == nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("original primary inventory is unavailable")
	}
	entry, found := inventory.entries[id]
	if !found {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("thread is outside original primary inventory")
	}
	snapshot, err := entry.reader.ReadPrimaryThreadSnapshotV1(ctx, id)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	if snapshot.ThreadID != id || snapshot.ThreadFileSHA256 != entry.digest {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("original primary changed after inventory")
	}
	return snapshot, nil
}

func (inventory *runtimeOriginalPrimaryInventoryV1) ReadCommittedEventLogSHA256V1(ctx context.Context, id string) (string, error) {
	if inventory == nil {
		return "", errors.New("original primary inventory is unavailable")
	}
	entry, found := inventory.entries[id]
	if !found {
		return "", errors.New("thread is outside original primary inventory")
	}
	return entry.reader.ReadCommittedEventLogSHA256V1(ctx, id)
}

func readRuntimeOriginalPrimaryInventoryV1(ctx context.Context, roots persistencefs.RootSet, snapshot persistencefs.RawSnapshot) (*runtimeOriginalPrimaryInventoryV1, []map[string]any, error) {
	return readRuntimeOriginalPrimaryInventoryWithRetiredAuditV1(ctx, roots, snapshot, nil)
}

// readRuntimeOriginalPrimaryInventoryForRetiredAuditV1 keeps the strict
// original-primary denominator intact while allowing the desktop migration to
// carry a witnessed metadata-only parent through its retired audit phase. A
// parent is eligible only when its ID came from the exact, source-hash-bound
// legacy lineage witness; the stage callback subsequently materializes its
// canonical primary before the ordinary startup path is run.
func readRuntimeOriginalPrimaryInventoryForRetiredAuditV1(
	ctx context.Context,
	roots persistencefs.RootSet,
	snapshot persistencefs.RawSnapshot,
	retiredParentIDs map[string]struct{},
) (*runtimeOriginalPrimaryInventoryV1, []map[string]any, error) {
	for id := range retiredParentIDs {
		if !domainthread.IsCanonicalRecordID(id) {
			return nil, nil, pendingworkapp.ErrChildProducerInventoryIncomplete
		}
	}
	return readRuntimeOriginalPrimaryInventoryWithRetiredAuditV1(ctx, roots, snapshot, retiredParentIDs)
}

func readRuntimeOriginalPrimaryInventoryWithRetiredAuditV1(
	ctx context.Context,
	roots persistencefs.RootSet,
	snapshot persistencefs.RawSnapshot,
	retiredParentIDs map[string]struct{},
) (*runtimeOriginalPrimaryInventoryV1, []map[string]any, error) {
	inventory := &runtimeOriginalPrimaryInventoryV1{entries: map[string]runtimeOriginalPrimaryEntryV1{}, roots: roots}
	files := make(map[string]persistencefs.EntryRecord, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		files[entry.Path] = entry
	}
	var threads []map[string]any
	for _, family := range []struct{ label, root string }{
		{"durable/threads/", roots.DurableDir},
		{"durable/runtime-go/threads/", filepath.Join(roots.DurableDir, "runtime-go")},
	} {
		root, exists := files[strings.TrimSuffix(family.label, "/")]
		if !exists || (root.Type != "absent" && root.Type != "directory") {
			return nil, nil, pendingworkapp.ErrChildProducerInventoryIncomplete
		}
		var reader *finalauthority.AcceptedFinalCASReader
		for _, entry := range snapshot.Entries {
			if !strings.HasPrefix(entry.Path, family.label) {
				continue
			}
			id := strings.TrimPrefix(entry.Path, family.label)
			if strings.Contains(id, "/") {
				continue
			}
			if entry.Type != "directory" {
				return nil, nil, pendingworkapp.ErrChildProducerInventoryIncomplete
			}
			primary, exists := files[entry.Path+"/thread.json"]
			if !exists {
				if family.label == "durable/threads/" {
					if _, retired := retiredParentIDs[id]; retired {
						continue
					}
				}
				return nil, nil, pendingworkapp.ErrChildProducerInventoryIncomplete
			}
			if primary.Type != "file" {
				return nil, nil, pendingworkapp.ErrChildProducerInventoryIncomplete
			}
			if reader == nil {
				var err error
				reader, err = finalauthority.NewAcceptedFinalCASReader(family.root)
				if err != nil {
					return nil, nil, err
				}
			}
			observed, err := reader.ReadPrimaryThreadSnapshotV1(ctx, id)
			if err != nil {
				return nil, nil, err
			}
			if observed.ThreadFileSHA256 != primary.SHA256 {
				return nil, nil, errors.New("Core child primary changed during identity inventory")
			}
			if _, exists := inventory.entries[id]; exists {
				return nil, nil, pendingworkapp.ErrChildProducerInventoryIncomplete
			}
			inventory.entries[id] = runtimeOriginalPrimaryEntryV1{reader: reader, digest: primary.SHA256, primary: primary, events: files[entry.Path+"/events.jsonl"], eventRoot: family.root}
			threads = append(threads, observed.Thread)
		}
	}
	return inventory, threads, nil
}
