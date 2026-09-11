package finalauthority

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"strings"
	"sync"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const maxPrivateCASCreateRecoveryDirectoryEntries = 65_536

type privateCASCreateResidueParentSpec struct {
	relativePath        string
	fixedComponents     map[string]struct{}
	residueComponents   map[string]string
	shardParent         bool
	deferOrphanRecovery bool
}

type privateCASDirectoryRecoveryModeV1 uint8

const (
	privateCASDirectoryRecoveryCreateResiduesV1 privateCASDirectoryRecoveryModeV1 = iota + 1
	privateCASDirectoryRecoveryOrphanTopologyV1
)

var privateCASCreateResidueRecoveryTestHooks = struct {
	sync.RWMutex
	hook func(string, int) error
}{}

func privateCASCreateResidueRecoveryTestCut(phase string, index int) error {
	privateCASCreateResidueRecoveryTestHooks.RLock()
	hook := privateCASCreateResidueRecoveryTestHooks.hook
	privateCASCreateResidueRecoveryTestHooks.RUnlock()
	if hook == nil {
		return nil
	}
	return hook(phase, index)
}

func privateCASCreateResidueParentSpecsV1() []privateCASCreateResidueParentSpec {
	parentPaths := domainprivatecas.CreateRecoveryScanParentPathsV1()
	specs := make([]privateCASCreateResidueParentSpec, len(parentPaths))
	byParent := make(map[string]*privateCASCreateResidueParentSpec, len(parentPaths))
	for index, relative := range parentPaths {
		specs[index] = privateCASCreateResidueParentSpec{
			relativePath:      relative,
			fixedComponents:   make(map[string]struct{}),
			residueComponents: make(map[string]string),
		}
		byParent[relative] = &specs[index]
	}
	for _, slot := range domainprivatecas.FixedDirectorySlotsV1() {
		spec := byParent[slot.ParentRelativePath]
		if spec == nil {
			continue
		}
		spec.fixedComponents[slot.Component] = struct{}{}
		spec.residueComponents[domainprivatecas.CreateDirectoryResidueNameV1(slot.Component)] = slot.Component
	}
	for _, shardParent := range domainprivatecas.ShardParentPathsV1() {
		spec := byParent[shardParent]
		if spec == nil {
			continue
		}
		spec.shardParent = true
		for _, shard := range domainprivatecas.CanonicalShardComponentsV1() {
			spec.residueComponents[domainprivatecas.CreateDirectoryResidueNameV1(shard)] = shard
		}
	}
	for _, group := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
		if !group.DeferOrphanRecovery {
			continue
		}
		for _, leaf := range group.LeafComponents {
			if spec := byParent[path.Join(group.OwnerRelativePath, leaf)]; spec != nil {
				spec.deferOrphanRecovery = true
			}
		}
	}
	return specs
}

func privateCASCreateResiduePathDepth(relative string) int {
	if relative == "." {
		return 0
	}
	return strings.Count(path.Clean(relative), "/") + 1
}

// PreparedSecurePrivateCASDirectoryRecoveryV1 freezes one narrowly scoped
// directory-recovery projection below a runtime data root. Create residues and
// orphan final topology use separate fresh plans so pre-journal recovery never
// acquires authority to delete canonical final directories.
type PreparedSecurePrivateCASDirectoryRecoveryV1 struct {
	requestedPrivateRoot string
	access               SecurePrivateCASRecoveryAccessAuthority
	binding              privatecasport.RootBinding
	mode                 privateCASDirectoryRecoveryModeV1
	ownerScope           string
	plan                 privateCASCreateResiduePreparedPlan
	originalCreates      *PreparedSecurePrivateCASOriginalCreateResiduesV1
}

// PrepareSecurePrivateCASCreateResidueRecoveryV1 performs a complete,
// no-create observation before owner semantic recovery is allowed to inspect
// any CAS leaf. Missing parents remain missing and cannot be promoted by this
// recovery path.
func PrepareSecurePrivateCASCreateResidueRecoveryV1(
	ctx context.Context,
	dataDir string,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedSecurePrivateCASDirectoryRecoveryV1, error) {
	return prepareSecurePrivateCASDirectoryRecoveryV1(
		ctx,
		dataDir,
		access,
		privateCASDirectoryRecoveryCreateResiduesV1,
		"",
	)
}

// PrepareSecurePrivateCASOrphanTopologyRecoveryV1 is intentionally separate
// from create-residue recovery. Runtime startup calls it only after semantic
// journal recovery has settled, using a fresh observation.
func PrepareSecurePrivateCASOrphanTopologyRecoveryV1(
	ctx context.Context,
	dataDir string,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedSecurePrivateCASDirectoryRecoveryV1, error) {
	return prepareSecurePrivateCASDirectoryRecoveryV1(
		ctx,
		dataDir,
		access,
		privateCASDirectoryRecoveryOrphanTopologyV1,
		"",
	)
}

func prepareSecurePrivateCASDirectoryRecoveryV1(
	ctx context.Context,
	dataDir string,
	access SecurePrivateCASRecoveryAccessAuthority,
	mode privateCASDirectoryRecoveryModeV1,
	ownerScope string,
	originals ...*PreparedSecurePrivateCASOriginalCreateResiduesV1,
) (*PreparedSecurePrivateCASDirectoryRecoveryV1, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if access == nil {
		return nil, errors.New("private CAS create-residue recovery authority is unavailable")
	}
	if mode != privateCASDirectoryRecoveryCreateResiduesV1 &&
		mode != privateCASDirectoryRecoveryOrphanTopologyV1 {
		return nil, errors.New("private CAS directory recovery mode is invalid")
	}
	if err := domainprivatecas.ValidateRuntimeDirectorySlotsV1(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(dataDir)
	absolute, err := filepath.Abs(trimmed)
	if err != nil || trimmed == "" || trimmed != dataDir || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private CAS create-residue recovery data root is invalid")
	}
	requestedPrivateRoot := filepath.Join(absolute, "private")
	original, err := privateCASOriginalDirectoryRecoveryProofV1(mode, originals)
	if err != nil {
		return nil, err
	}
	if original != nil {
		if original.DataRootV1() != absolute {
			return nil, errors.New("original directory recovery root differs")
		}
		if err := original.Revalidate(ctx); err != nil {
			return nil, err
		}
	}
	var prepared *PreparedSecurePrivateCASDirectoryRecoveryV1
	err = withExistingPrivateCASAccess(ctx, access, requestedPrivateRoot, func(binding privatecasport.RootBinding) error {
		first, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, mode, ownerScope, original)
		if err != nil {
			return err
		}
		second, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, mode, ownerScope, original)
		if err != nil || !privateCASCreateResiduePreparedPlansEqual(first, second) {
			return errors.Join(errors.New("private CAS create-residue topology changed during preparation"), err)
		}
		prepared = &PreparedSecurePrivateCASDirectoryRecoveryV1{
			requestedPrivateRoot: requestedPrivateRoot,
			access:               access,
			binding:              binding,
			mode:                 mode,
			ownerScope:           ownerScope,
			plan:                 second,
			originalCreates:      original,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, errors.New("private CAS create-residue recovery was not bound")
	}
	return prepared, nil
}

func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) CandidateCount() int {
	if prepared == nil {
		return 0
	}
	return privateCASCreateResiduePreparedPlanCount(prepared.plan)
}

func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) ResidueCount() int {
	if prepared == nil || prepared.mode != privateCASDirectoryRecoveryCreateResiduesV1 {
		return 0
	}
	return prepared.CandidateCount()
}

func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.access == nil || prepared.requestedPrivateRoot == "" {
		return errors.New("private CAS create-residue recovery plan is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return withExistingPrivateCASAccess(ctx, prepared.access, prepared.requestedPrivateRoot, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS create-residue recovery binding changed")
		}
		first, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, prepared.mode, prepared.ownerScope, prepared.originalCreates)
		if err != nil {
			return err
		}
		second, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, prepared.mode, prepared.ownerScope, prepared.originalCreates)
		if err != nil || !privateCASCreateResiduePreparedPlansEqual(first, second) ||
			!privateCASCreateResiduePreparedPlansEqual(second, prepared.plan) {
			return errors.Join(errors.New("private CAS create-residue recovery topology changed"), err)
		}
		return nil
	})
}

// Apply deletes only the exact empty residue directories frozen by Prepare.
// The process-wide recovery barrier first drains all live CAS operations; the
// platform adapter then repeats the complete preflight before its first
// deletion and verifies the residue-free topology afterward.
func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) Apply(ctx context.Context) error {
	if prepared != nil && prepared.originalCreates != nil {
		return errors.New("original orphan recovery requires its composing preservation guard")
	}
	return prepared.applyPreservingOriginalOwnersV1(ctx, nil, nil)
}

// ApplyPreservingOriginalCreateResiduesV1 retains only the immutable physical
// residue names of the original owner proof. The complete catalog still owns
// preflight, independent cleanup and the exact final directory delta.
func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) ApplyPreservingOriginalCreateResiduesV1(ctx context.Context, proof *PreparedSecurePrivateCASOriginalCreateResiduesV1, revalidate func(context.Context) error) (resultErr error) {
	if prepared == nil || prepared.mode != privateCASDirectoryRecoveryCreateResiduesV1 || proof == nil || proof.prepared == nil || revalidate == nil || prepared.binding != proof.prepared.binding || prepared.requestedPrivateRoot != proof.prepared.requestedPrivateRoot {
		return errors.New("private CAS original creation preservation is invalid")
	}
	if err := proof.Revalidate(ctx); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, proof.Revalidate(ctx)) }()
	return prepared.applyPreservingOriginalOwnersV1(ctx, proof.RelativePathsV1(), func(ctx context.Context) error {
		return errors.Join(proof.Revalidate(ctx), revalidate(ctx))
	})
}

// ApplyPreservingOriginalDirectoriesV1 retains exact original orphan
// directories while recovering independent candidates, including later
// directories created beneath the same owner by an authenticated program.
// The complete physical plan remains the preflight and final denominator.
// Create-residue recovery has a separate lifecycle and is not authorized here.
// revalidate runs before effects within recovery exclusion using already
// prepared proofs. The composing owner checks semantic preservation after
// Apply returns, against the exact directory delta proved by the native adapter.
func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) ApplyPreservingOriginalDirectoriesV1(ctx context.Context, directories []string, revalidate func(context.Context) error) error {
	if prepared == nil || prepared.mode != privateCASDirectoryRecoveryOrphanTopologyV1 || revalidate == nil {
		return errors.New("private CAS original orphan preservation is invalid")
	}
	stableDirectories := append([]string(nil), directories...)
	seen := make(map[string]struct{}, len(stableDirectories))
	for _, directory := range stableDirectories {
		if !privateCASOriginalOwnerDirectoryV1(directory) {
			return errors.New("private CAS original orphan preservation names an unknown directory")
		}
		if _, repeated := seen[directory]; repeated {
			return errors.New("private CAS original orphan preservation repeats a directory")
		}
		seen[directory] = struct{}{}
	}
	if prepared.originalCreates != nil {
		stableDirectories = append(stableDirectories, prepared.originalCreates.RelativePathsV1()...)
	}
	return prepared.applyPreservingOriginalOwnersV1(ctx, stableDirectories, revalidate)
}

func privateCASOriginalOwnerDirectoryV1(directory string) bool {
	if path.Clean(directory) != directory || strings.Contains(directory, "\\") {
		return false
	}
	for _, group := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
		if directory == group.OwnerRelativePath {
			return true
		}
		if !strings.HasPrefix(directory, group.OwnerRelativePath+"/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(directory, group.OwnerRelativePath+"/"), "/")
		if len(parts) < 1 || len(parts) > 2 || len(parts) == 2 && !validPrivateShard(parts[1]) {
			return false
		}
		for _, leaf := range group.LeafComponents {
			if parts[0] == leaf {
				return true
			}
		}
	}
	return false
}

func privateCASDirectoryCandidatePreservedV1(parent, component string, directories []string) bool {
	target := path.Join(parent, component)
	for _, directory := range directories {
		if target == directory || strings.HasPrefix(directory, target+"/") {
			return true
		}
	}
	return false
}

func (prepared *PreparedSecurePrivateCASDirectoryRecoveryV1) applyPreservingOriginalOwnersV1(ctx context.Context, ownerRoots []string, revalidate func(context.Context) error) (resultErr error) {
	if prepared == nil || prepared.access == nil || prepared.requestedPrivateRoot == "" {
		return errors.New("private CAS create-residue recovery plan is invalid")
	}
	if prepared.ownerScope != "" {
		return errors.New("private CAS empty owner proof has no mutation authority")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := privateCASRecoveryExclusion.acquireRecovery(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseRecovery()
	if revalidate != nil {
		if err := revalidate(ctx); err != nil {
			return err
		}
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	return withExistingPrivateCASAccess(ctx, prepared.access, prepared.requestedPrivateRoot, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS create-residue recovery binding changed before mutation")
		}
		return securePrivateCASApplyCreateResidueRecovery(ctx, binding, prepared.mode, prepared.plan, ownerRoots, prepared.originalCreates)
	})
}

func PrepareSecurePrivateCASOrphanTopologyWithOriginalCreateResiduesV1(ctx context.Context, dataDir string, access SecurePrivateCASRecoveryAccessAuthority, proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) (*PreparedSecurePrivateCASDirectoryRecoveryV1, error) {
	if proof == nil {
		return nil, errors.New("original orphan directory proof is unavailable")
	}
	return prepareSecurePrivateCASDirectoryRecoveryV1(ctx, dataDir, access, privateCASDirectoryRecoveryOrphanTopologyV1, "", proof)
}

func privateCASOriginalDirectoryRecoveryProofV1(mode privateCASDirectoryRecoveryModeV1, originals []*PreparedSecurePrivateCASOriginalCreateResiduesV1) (*PreparedSecurePrivateCASOriginalCreateResiduesV1, error) {
	if len(originals) > 1 {
		return nil, errors.New("original directory recovery proof is ambiguous")
	}
	if len(originals) == 0 || originals[0] == nil {
		return nil, nil
	}
	proof := originals[0]
	if mode != privateCASDirectoryRecoveryOrphanTopologyV1 || proof.prepared == nil {
		return nil, errors.New("original directory recovery mode is invalid")
	}
	return proof, nil
}

func privateCASOriginalCreateNamesInParentV1(proof *PreparedSecurePrivateCASOriginalCreateResiduesV1, parent string) map[string]bool {
	names := map[string]bool{}
	if proof != nil {
		for _, relative := range proof.RelativePathsV1() {
			if path.Dir(relative) == parent {
				names[path.Base(relative)] = true
			}
		}
	}
	return names
}
