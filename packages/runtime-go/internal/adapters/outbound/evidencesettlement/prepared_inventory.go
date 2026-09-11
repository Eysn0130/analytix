package evidencesettlement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// PreparedInventoryV1 observes the complete existing owner without creating,
// chmod, cleanup, recovery, or any current issuance capability. The topology
// retains handle-bound root, descendant, link, mode and byte identities.
type PreparedInventoryV1 struct {
	root     string
	topology *finalauthority.PreparedSecurePrivateCASOwnerTopologyV1
	access   finalauthority.SecurePrivateCASRecoveryAccessAuthority
}

// OpenStoreWithRestartPreservationV1 activates reads over the existing owner.
// Missing directories stay absent until an explicitly guarded write needs
// them. The callback must validate the record's write scope and reobserve the
// held inventory; it runs before and after each storage attempt.
func (prepared *PreparedInventoryV1) OpenStoreWithRestartPreservationV1(ctx context.Context, validateWrite func(context.Context, domainevidence.PreparedEvidenceSettlement) error) (*Store, error) {
	if ctx == nil || prepared == nil || prepared.access == nil || validateWrite == nil {
		return nil, errors.New("original settlement activation authority is unavailable")
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	store := &Store{prepared: filepath.Join(prepared.root, "prepared"), validateRestartWrite: validateWrite}
	store.originalInventory = func(ctx context.Context) ([]domainevidence.PreparedEvidenceSettlement, error) {
		current, err := ObservePreparedInventoryV1(ctx, prepared.root, prepared.access)
		if err != nil {
			return nil, err
		}
		return current.SnapshotInventoryV1(ctx)
	}
	return store, prepared.Revalidate(ctx)
}

// SnapshotPreparedFileBytesV1 retains actual original encoding, including
// whitespace accepted by the historical parser. Re-encoding a parsed record
// is not evidence of its original byte identity.
func (prepared *PreparedInventoryV1) SnapshotPreparedFileBytesV1(ctx context.Context) (files map[string][]byte, resultErr error) {
	records, err := prepared.SnapshotInventoryV1(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.Revalidate(ctx))
		if resultErr != nil {
			files = nil
		}
	}()
	files = map[string][]byte{}
	store := &Store{prepared: filepath.Join(prepared.root, "prepared")}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relative := "prepared/" + record.SettlementID[:2] + "/" + record.SettlementID + ".json"
		_, body, err := store.readPathBytes(store.recordPath(record.SettlementID))
		if err != nil {
			return nil, err
		}
		if _, err := ParsePreparedInventoryFileV1(relative, body); err != nil {
			return nil, err
		}
		files[relative] = body
	}
	return files, nil
}

// ParsePreparedInventoryFileV1 is the bounded original/candidate storage
// grammar. Installation trust and durable context association belong to the
// composing owner; this function grants no execution capability.
func ParsePreparedInventoryFileV1(relative string, body []byte) (domainevidence.PreparedEvidenceSettlement, error) {
	if len(body) == 0 || len(body) > maxPreparedEvidenceSettlementBytes {
		return domainevidence.PreparedEvidenceSettlement{}, errors.New("prepared settlement inventory file size is invalid")
	}
	record, err := domainevidence.ParsePreparedEvidenceSettlement(body)
	if err != nil {
		return domainevidence.PreparedEvidenceSettlement{}, err
	}
	if !domainsecurity.IsSHA256Hex(record.SettlementID) || relative != "prepared/"+record.SettlementID[:2]+"/"+record.SettlementID+".json" {
		return domainevidence.PreparedEvidenceSettlement{}, errors.New("prepared settlement inventory file address is invalid")
	}
	return record, nil
}

func ObservePreparedInventoryV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*PreparedInventoryV1, error) {
	if ctx == nil || root == "" || root != strings.TrimSpace(root) || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("original settlement owner path is invalid")
	}
	topology, entries, err := finalauthority.PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2(ctx, root, nil, domainstartup.MaxManagedSnapshotEntriesV1, domainstartup.MaxSemanticStagedTotalBytesV1, access)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Name != "prepared" || !entry.Directory {
			return nil, errors.New("original settlement owner contains an unknown entry")
		}
	}
	prepared := &PreparedInventoryV1{root: root, topology: topology, access: access}
	if _, err := prepared.SnapshotInventoryV1(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *PreparedInventoryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.topology == nil {
		return errors.New("original settlement owner observation is unavailable")
	}
	return prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx)
}

func (prepared *PreparedInventoryV1) SnapshotInventoryV1(ctx context.Context) (_ []domainevidence.PreparedEvidenceSettlement, resultErr error) {
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, prepared.Revalidate(ctx)) }()
	if !prepared.topology.PresentV1() || !prepared.topology.ContainsEntryV1("prepared") {
		return []domainevidence.PreparedEvidenceSettlement{}, nil
	}
	return (&Store{prepared: filepath.Join(prepared.root, "prepared")}).ListPrepared(ctx)
}

// SnapshotFileModesV1 exposes only exact managed paths and modes, alongside
// the same immutable observation used for the typed record inventory.
func (prepared *PreparedInventoryV1) SnapshotFileModesV1(ctx context.Context) (_ map[string]uint32, resultErr error) {
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, prepared.Revalidate(ctx)) }()
	modes := map[string]uint32{}
	if !prepared.topology.PresentV1() {
		return modes, nil
	}
	err := filepath.WalkDir(prepared.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(prepared.root, path)
		if err != nil {
			return err
		}
		modes[filepath.ToSlash(relative)] = uint32(info.Mode().Perm())
		return nil
	})
	return modes, err
}
