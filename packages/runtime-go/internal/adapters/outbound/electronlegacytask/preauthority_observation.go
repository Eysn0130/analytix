package electronlegacytask

import (
	"context"
	"encoding/json"
	"errors"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

// PreAuthorityObservationV2 is a content-complete, read-only observation of
// the separately owned Electron target and retirement journal. It deliberately
// does not require or create the installation signing namespace.
type PreAuthorityObservationV2 struct {
	digest         string
	journalPresent bool
	targetPresent  bool
}

func (observation PreAuthorityObservationV2) Digest() string       { return observation.digest }
func (observation PreAuthorityObservationV2) JournalPresent() bool { return observation.journalPresent }
func (observation PreAuthorityObservationV2) TargetPresent() bool  { return observation.targetPresent }

// ObserveBeforeJournalAuthorityV2 captures the exact target plus every
// bounded journal object through the separate-owner lease. Existing journal
// signatures are authenticated only after no-create authority discovery; this
// phase proves that discovery/bootstrap itself cannot precede owner preflight.
func ObserveBeforeJournalAuthorityV2(
	ctx context.Context,
	lease *persistencefs.CompositeLease,
	root string,
) (PreAuthorityObservationV2, error) {
	if lease == nil {
		return PreAuthorityObservationV2{}, errors.New("legacy Electron pre-authority lease is unavailable")
	}
	authority, ok := lease.FrozenSeparateOwnerAuthority(root)
	if !ok || authority == nil || authority.Root() == "" {
		return PreAuthorityObservationV2{}, errors.New("legacy Electron pre-authority owner is unavailable")
	}
	var inventory InventoryV1
	view := missingJournalView()
	err := lease.WithSeparateOwnerRootAccess(ctx, authority, func(access *persistencefs.SeparateOwnerRootAccess) error {
		if access == nil {
			return errors.New("legacy Electron pre-authority access is unavailable")
		}
		if !access.Present() {
			var err error
			inventory, err = newInventory(InventoryV1{AuthorityDigest: authority.Digest()})
			return err
		}
		opened, err := access.Directory()
		if err != nil {
			return err
		}
		inventory, err = capturePreAuthorityInventoryV2(ctx, authority, opened)
		if err != nil {
			return err
		}
		journal, present, err := opened.OpenDirectory(ctx, journalDirectoryV1, true)
		if err != nil {
			return err
		}
		if present {
			view, err = captureJournalView(ctx, journal, nil)
		}
		if journal != nil {
			err = errors.Join(err, journal.Close())
		}
		if err != nil {
			return err
		}
		after, err := capturePreAuthorityInventoryV2(ctx, authority, opened)
		if err != nil || after != inventory {
			return errors.New("legacy Electron pre-authority inventory changed")
		}
		finalJournal, finalPresent, err := opened.OpenDirectory(ctx, journalDirectoryV1, true)
		if err != nil || finalPresent != present {
			if finalJournal != nil {
				_ = finalJournal.Close()
			}
			return errors.New("legacy Electron pre-authority journal presence changed")
		}
		if finalPresent {
			finalState := finalJournal.State()
			closeErr := finalJournal.Close()
			if closeErr != nil || finalState != view.directory {
				return errors.New("legacy Electron pre-authority journal identity changed")
			}
		}
		return nil
	})
	if err != nil || !validInventory(inventory) || !isSHA256(view.digest) {
		return PreAuthorityObservationV2{}, errors.Join(
			errors.New("legacy Electron pre-authority observation failed"), err,
		)
	}
	body, err := json.Marshal(struct {
		SchemaVersion   int    `json:"schemaVersion"`
		InventoryDigest string `json:"inventoryDigest"`
		JournalDigest   string `json:"journalDigest"`
	}{2, inventory.Digest, view.digest})
	if err != nil {
		return PreAuthorityObservationV2{}, err
	}
	return PreAuthorityObservationV2{
		digest: sha256Hex(body), journalPresent: view.present, targetPresent: inventory.TargetPresent,
	}, nil
}

func ValidateBeforeJournalAuthorityV2(
	ctx context.Context,
	lease *persistencefs.CompositeLease,
	root string,
	expected PreAuthorityObservationV2,
) error {
	if !isSHA256(expected.digest) {
		return errors.New("legacy Electron pre-authority observation is invalid")
	}
	current, err := ObserveBeforeJournalAuthorityV2(ctx, lease, root)
	if err != nil || current != expected {
		return errors.New("legacy Electron pre-authority observation changed")
	}
	return nil
}

func capturePreAuthorityInventoryV2(
	ctx context.Context,
	authority *persistencefs.SeparateOwnerRootAuthority,
	root *persistencefs.SeparateOwnerDirectory,
) (InventoryV1, error) {
	if authority == nil || root == nil {
		return InventoryV1{}, errors.New("legacy Electron pre-authority inventory source is unavailable")
	}
	target, _, targetPresent, err := root.CaptureFile(
		ctx, BackgroundTaskFileV1, MaxBackgroundTaskBytesV1, true, false,
	)
	if err != nil {
		return InventoryV1{}, err
	}
	return newInventory(InventoryV1{
		AuthorityDigest: authority.Digest(), RootPresent: true, Root: root.State(),
		TargetPresent: targetPresent, Target: target,
	})
}
