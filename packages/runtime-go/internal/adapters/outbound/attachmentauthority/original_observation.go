package attachmentauthority

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

// PreparedOriginalInventoryV1 observes physical membership only. A missing
// leaf is not an empty logical authority: all five present/absent plans form
// the physical denominator, and callers must validate both signed program
// endpoints before using any graph.
// Ordinary recovery and store construction still require all five leaves.
type PreparedOriginalInventoryV1 struct {
	root           string
	topology       *finalauthority.PreparedSecurePrivateCASOwnerTopologyV1
	leaves         map[string]*finalauthority.PreparedSecurePrivateCASRecoveryV1
	createResidues *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1
}

func PrepareOriginalInventoryV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*PreparedOriginalInventoryV1, error) {
	return prepareOriginalInventoryV1(ctx, root, access, nil)
}

func PrepareOriginalInventoryWithCreateResiduesV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, proof *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1) (*PreparedOriginalInventoryV1, error) {
	if proof == nil {
		return nil, errors.New("original attachment creation residue proof is unavailable")
	}
	return prepareOriginalInventoryV1(ctx, root, access, proof)
}

func prepareOriginalInventoryV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, proof *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1) (_ *PreparedOriginalInventoryV1, resultErr error) {
	if ctx == nil || root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("original attachment physical configuration is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("original attachment physical root is invalid")
	}
	allowedResidues := map[string]bool{}
	if proof != nil {
		if proof.OwnerRootV1() != absolute {
			return nil, errors.New("original attachment creation residue owner differs")
		}
		if err := proof.Revalidate(ctx); err != nil {
			return nil, err
		}
		defer func() { resultErr = errors.Join(resultErr, proof.Revalidate(ctx)) }()
		for _, relative := range proof.RelativePathsV1() {
			physical := filepath.Join(proof.DataRootV1(), filepath.FromSlash(relative))
			if filepath.Dir(physical) == absolute {
				allowedResidues[filepath.Base(physical)] = true
			}
		}
	}
	names := make([]string, 0, len(originalAttachmentLeafLimitsV1))
	for name := range originalAttachmentLeafLimitsV1 {
		names = append(names, name)
	}
	sort.Strings(names)
	topology, entries, err := finalauthority.PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2(ctx, absolute, names, len(names)+len(allowedResidues), 0, access)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedOriginalInventoryV1{root: absolute, topology: topology, leaves: make(map[string]*finalauthority.PreparedSecurePrivateCASRecoveryV1, len(entries)), createResidues: proof}
	for _, entry := range entries {
		if allowedResidues[entry.Name] {
			if !entry.Directory {
				return nil, errors.New("original attachment creation residue is not a directory")
			}
			continue
		}
		limit, known := originalAttachmentLeafLimitsV1[entry.Name]
		if !known || !entry.Directory {
			return nil, errors.New("original attachment physical owner contains an unknown leaf")
		}
		var leaf *finalauthority.PreparedSecurePrivateCASRecoveryV1
		var err error
		if originalAttachmentLeafHasCreateResiduesV1(proof, filepath.Join(absolute, entry.Name)) {
			leaf, err = finalauthority.PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx, filepath.Join(absolute, entry.Name), limit, access, proof)
		} else {
			leaf, err = finalauthority.PrepareSecurePrivateCASRecoveryIfPresent(ctx, filepath.Join(absolute, entry.Name), limit, access)
		}
		if err != nil {
			return nil, err
		}
		if !leaf.Present() {
			return nil, errors.New("original attachment physical leaf disappeared during observation")
		}
		if _, duplicate := prepared.leaves[entry.Name]; duplicate {
			return nil, errors.New("original attachment physical leaf is duplicated")
		}
		prepared.leaves[entry.Name] = leaf
	}
	for _, name := range names {
		if prepared.leaves[name] != nil {
			continue
		}
		leaf, err := finalauthority.PrepareSecurePrivateCASRecoveryIfPresent(ctx, filepath.Join(absolute, name), originalAttachmentLeafLimitsV1[name], access)
		if err != nil || leaf.Present() {
			return nil, errors.Join(errors.New("absent original attachment leaf changed during observation"), err)
		}
		prepared.leaves[name] = leaf
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *PreparedOriginalInventoryV1) Present() bool {
	return prepared != nil && prepared.topology != nil && prepared.topology.PresentV1()
}

func (prepared *PreparedOriginalInventoryV1) Revalidate(ctx context.Context) (resultErr error) {
	if ctx == nil || prepared == nil || prepared.topology == nil || len(prepared.leaves) != len(originalAttachmentLeafLimitsV1) {
		return errors.New("original attachment physical observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if prepared.createResidues != nil {
		if err := prepared.createResidues.Revalidate(ctx); err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, prepared.createResidues.Revalidate(ctx)) }()
	}
	if err := prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return err
	}
	for name := range originalAttachmentLeafLimitsV1 {
		leaf := prepared.leaves[name]
		if leaf == nil || leaf.RootPath() != filepath.Join(prepared.root, name) {
			return errors.New("original attachment physical leaf denominator is incomplete")
		}
		if err := leaf.Revalidate(ctx); err != nil {
			return err
		}
	}
	return errors.Join(prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx), ctx.Err())
}

func (prepared *PreparedOriginalInventoryV1) VisitCommittedFiles(ctx context.Context, name string, visit func(finalauthority.SecurePrivateCASFile) error) (resultErr error) {
	if _, known := originalAttachmentLeafLimitsV1[name]; !known || visit == nil {
		return errors.New("original attachment physical visitor is invalid")
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, prepared.Revalidate(ctx)) }()
	if leaf, present := prepared.leaves[name]; present {
		return leaf.VisitCommittedFiles(ctx, visit)
	}
	return nil
}

func (prepared *PreparedOriginalInventoryV1) SnapshotOriginalFilesV1(ctx context.Context) (map[string]OriginalEntryV1, error) {
	if prepared == nil {
		return nil, errors.New("original attachment physical observation is unavailable")
	}
	return snapshotOriginalAttachmentFilesV1(ctx, prepared.root, prepared)
}

// The owner proof still brackets the whole inventory. A leaf without its own
// original temporary shard uses the ordinary strict leaf scanner, which also
// rejects any later temp. It does not need a second whole-owner proof at each
// low-level read; only the leaf with a frozen shard residue needs that opening.
func originalAttachmentLeafHasCreateResiduesV1(proof *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1, leaf string) bool {
	if proof == nil {
		return false
	}
	for _, relative := range proof.RelativePathsV1() {
		if filepath.Dir(filepath.Join(proof.DataRootV1(), filepath.FromSlash(relative))) == leaf {
			return true
		}
	}
	return false
}
