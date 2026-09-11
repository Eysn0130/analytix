package datasetsnapshot

import (
	"context"
	"crypto/ed25519"
	"errors"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

// OriginalInventoryV1 contains every original node, without selecting a head
// or establishing admission, freshness or current execution authority.
type OriginalInventoryV1 struct {
	Records map[string]domainsecurity.VersionedDatasetSnapshotAuthorityRecord
	Bundles map[string]datasetport.AuthorityBundleV2
	Indexes map[string]domainsecurity.DatasetSnapshotIndexV1
}

func PrepareOriginalObservationV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*finalauthority.OriginalFixedOwnerObservationV1, error) {
	return finalauthority.PrepareOriginalFixedOwnerObservationV1(ctx, root, []finalauthority.SecurePrivateCASOwnerLeafV1{
		{Name: legacyRecordsLeafV2, MaxBytes: maxDatasetSnapshotRecordBytes}, {Name: authorityBundlesLeafV2, MaxBytes: maxAuthorityBundleBytesV2},
		{Name: indexesLeafV2, MaxBytes: maxDatasetSnapshotIndexBytes}, {Name: materialsLeafV2, MaxBytes: maxAdmissionMaterialBytesV2},
	}, access)
}

// ParseOriginalInventoryV1 validates a native original or signed journal
// endpoint using an independently supplied installation key and enrollment.
// The caller separately checks the complete nested immutable material graph.
func ParseOriginalInventoryV1(ctx context.Context, files map[string]finalauthority.SecurePrivateCASOriginalEntryV1, installationID, enrollmentID, keyID string, publicKey []byte) (inventory OriginalInventoryV1, resultErr error) {
	if ctx == nil || !domainsecurity.IsSHA256Hex(installationID) || !domainsecurity.IsSHA256Hex(enrollmentID) ||
		len(publicKey) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(publicKey) != keyID {
		return inventory, errors.New("original dataset independent trust is unavailable")
	}
	defer func() {
		resultErr = errors.Join(resultErr, context.Cause(ctx))
		if resultErr != nil {
			inventory = OriginalInventoryV1{}
		}
	}()
	committed, err := originalCommittedFilesV1(ctx, files)
	if err != nil {
		return inventory, err
	}
	// Reuse the existing fixed-owner semantic checks, with an in-memory visitor.
	visit := func(leaf string, read func(finalauthority.SecurePrivateCASFile) error) error {
		for digest, body := range committed[leaf] {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := read(finalauthority.SecurePrivateCASFile{Digest: digest, Body: body}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := (&PreparedRecoveryV2{}).validateDomainSemantics(ctx, visit, nil); err != nil {
		return inventory, err
	}
	inventory = OriginalInventoryV1{
		Records: map[string]domainsecurity.VersionedDatasetSnapshotAuthorityRecord{},
		Bundles: map[string]datasetport.AuthorityBundleV2{},
		Indexes: map[string]domainsecurity.DatasetSnapshotIndexV1{},
	}
	for digest, body := range committed[legacyRecordsLeafV2] {
		record, err := parseRecord(digest, body)
		if err != nil {
			return inventory, err
		}
		if err := domainsecurity.ValidateDatasetSnapshotAuthorityRecordForInstallationV1(record, installationID, keyID, publicKey); err != nil {
			return inventory, err
		}
		inventory.Records[digest] = domainsecurity.VersionedDatasetSnapshotAuthorityRecord{SchemaVersion: 1, V1: &record}
	}
	for digest, body := range committed[authorityBundlesLeafV2] {
		bundle, err := parseAuthorityBundleV2(digest, body)
		if err != nil {
			return inventory, err
		}
		if err := domainsecurity.ValidateDatasetSnapshotAuthorityRecordForInstallationV2(bundle.Record, installationID, keyID, publicKey); err != nil {
			return inventory, err
		}
		inventory.Bundles[digest] = bundle
		record := bundle.Record
		inventory.Records[digest] = domainsecurity.VersionedDatasetSnapshotAuthorityRecord{SchemaVersion: 2, V2: &record}
	}
	for digest, body := range committed[indexesLeafV2] {
		index, err := parseIndex(digest, body)
		if err != nil {
			return inventory, err
		}
		record := inventory.Records[index.SnapshotRecordDigest]
		switch {
		case record.V1 != nil:
			err = domainsecurity.ValidateDatasetSnapshotIndexRecordForInstallationV1(index, *record.V1, installationID, enrollmentID, keyID, publicKey)
		case record.V2 != nil:
			err = domainsecurity.ValidateDatasetSnapshotIndexRecordForInstallationV2(index, *record.V2, installationID, enrollmentID, keyID, publicKey)
		default:
			err = errors.New("original dataset index lost its exact record")
		}
		if err != nil {
			return inventory, err
		}
		inventory.Indexes[digest] = index
	}
	for _, record := range inventory.Records {
		var predecessor string
		if record.V1 != nil {
			predecessor = record.V1.PredecessorRecordDigest
		} else {
			predecessor = record.V2.PredecessorRecordDigest
		}
		if predecessor != "" {
			previous, found := inventory.Records[predecessor]
			if !found || domainsecurity.ValidateVersionedDatasetSnapshotAuthorityTransition(previous, record) != nil {
				return inventory, errors.New("original dataset record lost its exact predecessor")
			}
		}
	}
	for _, index := range inventory.Indexes {
		if index.Generation > 1 {
			previous, found := inventory.Indexes[index.PreviousIndexDigest]
			if !found || domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(previous, index) != nil {
				return inventory, errors.New("original dataset index lost its exact predecessor")
			}
		}
	}
	return inventory, validateOriginalDatasetIndexBranchesV1(ctx, inventory)
}

// Walk each actual branch once with branch-local state. Sibling candidates
// remain independent; an A -> B -> A replay cannot hide behind valid edges.
func validateOriginalDatasetIndexBranchesV1(ctx context.Context, inventory OriginalInventoryV1) error {
	children := map[string][]domainsecurity.DatasetSnapshotIndexV1{}
	for _, index := range inventory.Indexes {
		children[index.PreviousIndexDigest] = append(children[index.PreviousIndexDigest], index)
	}
	records, snapshots, producers, mutations := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	latest := map[string]domainsecurity.VersionedDatasetSnapshotAuthorityRecord{}
	visited := 0
	var walk func(domainsecurity.DatasetSnapshotIndexV1) error
	walk = func(index domainsecurity.DatasetSnapshotIndexV1) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		record := inventory.Records[index.SnapshotRecordDigest]
		var snapshot, predecessor, producer string
		if record.V1 != nil {
			snapshot, predecessor = record.V1.DatasetSnapshotID, record.V1.PredecessorRecordDigest
		} else {
			snapshot, predecessor = record.V2.DatasetSnapshotID, record.V2.PredecessorRecordDigest
			producer = inventory.Bundles[index.SnapshotRecordDigest].Manifest.ProducerContentID
		}
		if records[index.SnapshotRecordDigest] || snapshots[snapshot] || mutations[index.MutationID] || producer != "" && producers[producer] {
			return errors.New("original dataset index branch replays immutable authority")
		}
		binding := index.Binding.BindingKeyDigest
		previous, found := latest[binding]
		if found {
			if err := domainsecurity.ValidateVersionedDatasetSnapshotAuthorityTransition(previous, record); err != nil {
				return err
			}
		} else if predecessor != "" {
			return errors.New("original dataset index branch lost its binding genesis")
		}
		records[index.SnapshotRecordDigest], snapshots[snapshot], mutations[index.MutationID] = true, true, true
		if producer != "" {
			producers[producer] = true
		}
		latest[binding] = record
		visited++
		for _, next := range children[index.IndexDigest] {
			if err := walk(next); err != nil {
				return err
			}
		}
		delete(records, index.SnapshotRecordDigest)
		delete(snapshots, snapshot)
		delete(mutations, index.MutationID)
		delete(producers, producer)
		if found {
			latest[binding] = previous
		} else {
			delete(latest, binding)
		}
		return nil
	}
	for _, root := range children[domainsecurity.DatasetSnapshotIndexGenesisDigestV1()] {
		if err := walk(root); err != nil {
			return err
		}
	}
	if visited != len(inventory.Indexes) {
		return errors.New("original dataset index graph does not reach genesis")
	}
	return nil
}

// OriginalFilesHaveRecordsV1 proves an empty committed denominator using the
// complete fixed-owner grammar. Raw directories and residues remain preserved.
func OriginalFilesHaveRecordsV1(ctx context.Context, files map[string]finalauthority.SecurePrivateCASOriginalEntryV1) (bool, error) {
	committed, err := originalCommittedFilesV1(ctx, files)
	if err != nil {
		return false, err
	}
	for _, records := range committed {
		if len(records) != 0 {
			return true, nil
		}
	}
	return false, nil
}

func originalCommittedFilesV1(ctx context.Context, files map[string]finalauthority.SecurePrivateCASOriginalEntryV1) (map[string]map[string][]byte, error) {
	return finalauthority.ValidateOriginalFixedOwnerEntriesV1(ctx, files, []finalauthority.SecurePrivateCASOwnerLeafV1{
		{Name: legacyRecordsLeafV2, MaxBytes: maxDatasetSnapshotRecordBytes},
		{Name: authorityBundlesLeafV2, MaxBytes: maxAuthorityBundleBytesV2},
		{Name: indexesLeafV2, MaxBytes: maxDatasetSnapshotIndexBytes},
		{Name: materialsLeafV2, MaxBytes: maxAdmissionMaterialBytesV2},
	})
}
