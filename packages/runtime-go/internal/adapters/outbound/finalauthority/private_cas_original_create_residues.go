package finalauthority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// PreparedSecurePrivateCASOriginalCreateResiduesV1 is physical preservation
// evidence, not logical owner authority or permission to recover a directory.
// It binds the exact original empty creation residues, including an owner-root
// residue beside the owner. Its unexported projection cannot adopt new residue.
type PreparedSecurePrivateCASOriginalCreateResiduesV1 struct {
	prepared *PreparedSecurePrivateCASDirectoryRecoveryV1
	owner    string
	owners   []string
	plan     privateCASCreateResiduePreparedPlan
}

func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) OriginalCreateResiduesV1(ctx context.Context, owner string) (*PreparedSecurePrivateCASOriginalCreateResiduesV1, error) {
	if prepared == nil || prepared.mode != privateCASDirectoryRecoveryCreateResiduesV1 || prepared.ownerScope != "" {
		return nil, errors.New("original creation residues require complete pre-recovery observation")
	}
	return prepared.OriginalCreateResiduesForOwnersV1(ctx, []string{owner})
}

// OriginalCreateResiduesForOwnersV1 freezes a finite set of catalog owners
// from the same complete native observation. Each owner keeps its own root;
// the private parent never becomes a synthetic owner or write capability.
func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) OriginalCreateResiduesForOwnersV1(ctx context.Context, owners []string) (*PreparedSecurePrivateCASOriginalCreateResiduesV1, error) {
	if prepared == nil || prepared.mode != privateCASDirectoryRecoveryCreateResiduesV1 || prepared.ownerScope != "" || len(owners) == 0 || len(owners) > len(domainprivatecas.RecoverableOwnerDirectoryGroupsV1()) {
		return nil, errors.New("original creation owner set is invalid")
	}
	owners = append([]string(nil), owners...)
	sort.Strings(owners)
	for i, owner := range owners {
		known := false
		for _, group := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
			known = known || group.OwnerRelativePath == owner
		}
		if !known || (i > 0 && owners[i-1] == owner) {
			return nil, errors.New("original creation owner set is unknown or duplicated")
		}
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	proof := &PreparedSecurePrivateCASOriginalCreateResiduesV1{prepared: prepared, owners: owners}
	if len(owners) == 1 {
		proof.owner = owners[0]
	}
	proof.plan = proof.projectV1(prepared.plan)
	if err := proof.Revalidate(ctx); err != nil {
		return nil, err
	}
	return proof, nil
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) ownerNamesV1() []string {
	if proof == nil {
		return nil
	}
	if len(proof.owners) != 0 {
		return append([]string(nil), proof.owners...)
	}
	if proof.owner != "" {
		return []string{proof.owner}
	}
	return nil
}

// OwnerRootsV1 enumerates exact catalog roots, including a multi-owner proof.
func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) OwnerRootsV1() []string {
	var roots []string
	for _, owner := range proof.ownerNamesV1() {
		roots = append(roots, filepath.Join(proof.DataRootV1(), filepath.FromSlash(owner)))
	}
	return roots
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) projectV1(plan privateCASCreateResiduePreparedPlan) privateCASCreateResiduePreparedPlan {
	return privateCASOriginalCreateResidueProjectionForOwnersV1(plan, proof.ownerNamesV1())
}

// ProjectOwnerV1 retains one already selected owner's original proof.
func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) ProjectOwnerV1(ctx context.Context, owner string) (*PreparedSecurePrivateCASOriginalCreateResiduesV1, error) {
	if !slices.Contains(proof.ownerNamesV1(), owner) {
		return nil, errors.New("original creation projection selects an unobserved owner")
	}
	if err := proof.Revalidate(ctx); err != nil {
		return nil, err
	}
	projected := &PreparedSecurePrivateCASOriginalCreateResiduesV1{prepared: proof.prepared, owner: owner, owners: []string{owner}, plan: privateCASOriginalCreateResidueProjectionV1(proof.plan, owner)}
	if err := projected.Revalidate(ctx); err != nil {
		return nil, err
	}
	return projected, nil
}

// SelectOwnersV1 can only narrow this already observed finite owner set.
func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) SelectOwnersV1(ctx context.Context, owners []string) (*PreparedSecurePrivateCASOriginalCreateResiduesV1, error) {
	if err := proof.Revalidate(ctx); err != nil {
		return nil, err
	}
	owners = append([]string(nil), owners...)
	sort.Strings(owners)
	if len(owners) == 0 {
		return nil, errors.New("original creation selection is empty")
	}
	for index, owner := range owners {
		if !slices.Contains(proof.ownerNamesV1(), owner) || index > 0 && owners[index-1] == owner {
			return nil, errors.New("original creation set selects an unobserved owner")
		}
	}
	selected := &PreparedSecurePrivateCASOriginalCreateResiduesV1{prepared: proof.prepared, owners: owners}
	if len(owners) == 1 {
		selected.owner = owners[0]
	}
	selected.plan = selected.projectV1(proof.plan)
	if err := selected.Revalidate(ctx); err != nil {
		return nil, err
	}
	return selected, nil
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) DataRootV1() string {
	if proof == nil || proof.prepared == nil {
		return ""
	}
	return filepath.Dir(proof.prepared.requestedPrivateRoot)
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) OwnerRootV1() string {
	if proof == nil || proof.prepared == nil || proof.owner == "" {
		return ""
	}
	return filepath.Join(proof.DataRootV1(), filepath.FromSlash(proof.owner))
}

// RelativePathsV1 returns a copy of data-root-relative physical names. A caller
// must also revalidate this typed proof; the names alone confer no authority.
func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) RelativePathsV1() []string {
	if proof == nil || proof.prepared == nil {
		return nil
	}
	return privateCASOriginalCreateResiduePathsV1(proof.plan)
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) Revalidate(ctx context.Context) error {
	if ctx == nil || proof == nil || proof.prepared == nil || proof.prepared.access == nil || len(proof.ownerNamesV1()) == 0 {
		return errors.New("original creation residue proof is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	prepared := proof.prepared
	return withExistingPrivateCASAccess(ctx, prepared.access, prepared.requestedPrivateRoot, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("original creation residue binding changed")
		}
		return proof.revalidatePhysicalV1(ctx)
	})
}

func privateCASCreationTargetInOwnerV1(parent, component, owner string) bool {
	target := path.Join(parent, component)
	return target == owner || strings.HasPrefix(target, owner+"/")
}

func privateCASCreationParentInOwnerV1(parent, owner string) bool {
	return parent == "." || parent == owner || strings.HasPrefix(parent, owner+"/") || strings.HasPrefix(owner, parent+"/")
}

// The composing operation must retain a matching private-root access callback.
// This helper acquires neither an access lease nor a live/recovery barrier.
func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) revalidatePhysicalV1(ctx context.Context) error {
	if proof == nil || proof.prepared == nil {
		return errors.New("original creation residue proof is unavailable")
	}
	binding := proof.prepared.binding
	first, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, privateCASDirectoryRecoveryCreateResiduesV1, "")
	if err != nil {
		return err
	}
	second, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, privateCASDirectoryRecoveryCreateResiduesV1, "")
	if err != nil || !privateCASCreateResiduePreparedPlansEqual(first, second) ||
		!privateCASOriginalCreateResiduePlansEqualV1(proof.plan, proof.projectV1(second)) {
		return errors.Join(errors.New("original creation residue inventory changed"), err)
	}
	return ctx.Err()
}

func privateCASOriginalCreateOpeningCompatibleV1(original, current *PreparedSecurePrivateCASOriginalCreateResiduesV1) bool {
	if original == nil || current == nil {
		return original == current
	}
	return original.prepared != nil && current.prepared != nil && slices.Equal(original.ownerNamesV1(), current.ownerNamesV1()) && original.prepared.binding == current.prepared.binding && privateCASOriginalCreateResiduePlansEqualV1(original.plan, current.plan)
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) leafRelativeV1(root string) (string, error) {
	if proof == nil || proof.prepared == nil || len(proof.ownerNamesV1()) == 0 {
		return "", errors.New("original creation residue proof is unavailable")
	}
	relative, err := filepath.Rel(proof.DataRootV1(), root)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(relative)
	for _, group := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
		if !slices.Contains(proof.ownerNamesV1(), group.OwnerRelativePath) {
			continue
		}
		if len(group.LeafComponents) == 0 && relative == group.OwnerRelativePath {
			return relative, nil
		}
		for _, leaf := range group.LeafComponents {
			if relative == path.Join(group.OwnerRelativePath, leaf) {
				return relative, nil
			}
		}
	}
	return "", errors.New("original creation residue proof does not bind this CAS leaf")
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) validateLeafBindingV1(root string, binding privatecasport.RootBinding) error {
	if _, err := proof.leafRelativeV1(root); err != nil {
		return err
	}
	parent := proof.prepared.binding
	if binding.RootPath != parent.RootPath || binding.RootIdentity != parent.RootIdentity || filepath.Join(binding.RootPath, binding.RelativePath) != root {
		return errors.New("original creation residue leaf binding differs from its frozen root")
	}
	return nil
}

// Called within the unchanged exact-leaf access callback. Only this leaf's
// original directory residue subset is checked here; the composing owner
// separately revalidates its complete proof outside the leaf callback.
func observePrivateCASWithOriginalCreatesV1(ctx context.Context, authority privateCASRootAuthority, root string, maxBytes int, proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) (privateCASRecoveryObservation, error) {
	if proof == nil {
		return securePrivateCASObserveRecovery(ctx, authority, maxBytes)
	}
	relative, err := proof.leafRelativeV1(root)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	observed, err := securePrivateCASObserveRecoveryIncludingOriginalCreatesV1(ctx, authority, maxBytes)
	if err != nil {
		return privateCASRecoveryObservation{}, err
	}
	if !privateCASOriginalCreateLeafMatchesV1(proof.plan, relative, observed.plan) {
		return privateCASRecoveryObservation{}, errors.New("original CAS leaf creation residue inventory changed")
	}
	return observed, nil
}

// PrepareSecurePrivateCASOriginalRecoveryIfPresentV1 explicitly combines an
// exact leaf observation with an immutable original directory residue proof.
// Access still goes through the existing exact-leaf producer; no parent read
// binding is promoted into leaf write authority. The composing owner remains
// responsible for its complete semantic graph and held scopes.
func PrepareSecurePrivateCASOriginalRecoveryIfPresentV1(ctx context.Context, root string, maxBytes int, access SecurePrivateCASRecoveryAccessAuthority, proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) (prepared *PreparedSecurePrivateCASRecoveryV1, resultErr error) {
	if proof == nil {
		return nil, errors.New("original CAS preparation requires its creation residue proof")
	}
	if err := proof.Revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, proof.Revalidate(ctx))
		if resultErr != nil {
			prepared = nil
		}
	}()
	return prepareSecurePrivateCASRecoveryIfPresentV1(ctx, root, maxBytes, access, proof)
}

func privateCASOriginalRecoveryRootNamesEqualV1(expected []string, entries []os.DirEntry) bool {
	if len(expected) != len(entries) {
		return false
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		if !entry.IsDir() || !validPrivateShard(entry.Name()) && !domainprivatecas.CreateDirectoryResidueMatchesShardV1(entry.Name()) {
			return false
		}
		names[index] = entry.Name()
	}
	sort.Strings(names)
	for index, name := range names {
		if name != expected[index] {
			return false
		}
	}
	return true
}

type privateCASOriginalWriteValidationV1 struct {
	ctx        context.Context
	generation *privateCASRootGeneration
}

// Native writes and receipt observations retain their ordinary root check
// unless the live generation carries an explicit original creation proof.
// This is called under the same exact-leaf access and generation lock.
func validateOriginalPrivateCASWriteRootV1(authority privateCASRootAuthority, pins map[string]privateCASShardIdentity, maxBytes int, validations []privateCASOriginalWriteValidationV1) (bool, error) {
	if len(validations) == 0 {
		return false, nil
	}
	if len(validations) != 1 || validations[0].generation == nil {
		return true, errors.New("original CAS native write validation is invalid")
	}
	validation := validations[0]
	generation := validation.generation
	if generation.originalCreates == nil {
		return false, nil
	}
	if validation.ctx == nil {
		validation.ctx = context.Background()
	}
	if generation.revoked || generation.root != authority || generation.maxBytes != maxBytes {
		return true, errors.New("original CAS native write authority changed")
	}
	_, err := observePrivateCASOriginalResiduesV1(validation.ctx, authority, generation.key.rootPath, pins, maxBytes, generation.originalResidues, generation.originalCreates)
	return true, err
}

func privateCASOriginalWriteHasTargetShardResidueV1(shard string, validations []privateCASOriginalWriteValidationV1) bool {
	if len(validations) != 1 || validations[0].generation == nil {
		return false
	}
	generation := validations[0].generation
	proof := generation.originalCreates
	if proof == nil {
		return false
	}
	leaf, err := proof.leafRelativeV1(generation.key.rootPath)
	return err == nil && privateCASOriginalCreateTargetShardV1(proof.plan, leaf, shard)
}

func privateCASOriginalBeforeRecordStageV1(validations []privateCASOriginalWriteValidationV1) error {
	if len(validations) == 1 && validations[0].generation != nil {
		generation := validations[0].generation
		if generation.originalCreates != nil && generation.beforeOriginalRecordStage != nil {
			return generation.beforeOriginalRecordStage()
		}
	}
	return nil
}

// FingerprintV1 binds the immutable original objects and their stable parent
// identities. Parent child-count and timestamps may change during authorized
// independent cleanup; original empty residue identity remains exact.
func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) FingerprintV1() string {
	if proof == nil || proof.prepared == nil {
		return ""
	}
	ownerIdentity := proof.owner
	if len(proof.ownerNamesV1()) > 1 {
		ownerIdentity = fmt.Sprintf("%q", proof.ownerNamesV1())
	}
	body := fmt.Sprintf("analytix.original-create-residues/v1\n%q\n%#v\n%s", ownerIdentity, proof.prepared.binding, privateCASOriginalCreateFingerprintMaterialV1(proof.plan))
	digest := sha256.Sum256([]byte(body))
	return hex.EncodeToString(digest[:])
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) DirectoryStatesV1() []privatecasport.OriginalCreateDirectoryV1 {
	if proof == nil || proof.prepared == nil {
		return nil
	}
	return privateCASOriginalCreateDirectoryStatesV1(proof.plan)
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) ObserveCopiedOriginalCreateResiduesV1(ctx context.Context, dataDir string, access privatecasport.RecoveryAccessAuthority) (_ privatecasport.OriginalCreateResiduesV1, resultErr error) {
	if err := proof.Revalidate(ctx); err != nil {
		return nil, err
	}
	if filepath.Clean(dataDir) == proof.DataRootV1() {
		return nil, errors.New("original creation copy requires an independent root")
	}
	defer func() { resultErr = errors.Join(resultErr, proof.Revalidate(ctx)) }()
	prepared, err := PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, dataDir, access)
	if err != nil {
		return nil, err
	}
	copied, err := prepared.OriginalCreateResiduesForOwnersV1(ctx, proof.ownerNamesV1())
	if err != nil {
		return nil, err
	}
	if !slices.Equal(proof.DirectoryStatesV1(), copied.DirectoryStatesV1()) {
		return nil, errors.New("original creation copy changed the exact directory set or modes")
	}
	return copied, nil
}
