package persistencefs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var ErrPersistenceInUse = errors.New("persistence roots are already in use")
var ErrPersistenceActivationInUse = errors.New("persistence lease already owns a runtime activation")

type CompositeLease struct {
	mu                   sync.Mutex
	journalPublicationMu sync.Mutex
	accessCond           *sync.Cond
	activeAccesses       int
	closing              bool
	roots                RootSet
	authority            *RootAuthority
	journalAuthority     *JournalNamespaceAuthority
	journalLocator       *RootAuthority
	startupUserDataRoot  string
	separateOwners       map[string]*SeparateOwnerRootAuthority
	separateDigest       string
	kernel               *kernelScopeLease
	scope                platformScopeLease
	files                []*os.File
	paths                []string
	closed               bool
	activationOpen       bool
	activated            bool
}

// ActivationClaim makes one CompositeLease a one-shot runtime owner. The
// filesystem lease already excludes other processes; this claim also prevents
// two handlers in one process from sharing that same authority concurrently.
// Failed construction releases the provisional claim, while a successful
// activation remains consumed until the lease is closed.
type ActivationClaim struct {
	lease *CompositeLease
	once  sync.Once
}

func (lease *CompositeLease) BeginActivation() (*ActivationClaim, error) {
	if lease == nil {
		return nil, errors.New("persistence activation lease is unavailable")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() {
		return nil, errors.New("persistence activation lease is not live")
	}
	if lease.activationOpen || lease.activated {
		return nil, ErrPersistenceActivationInUse
	}
	lease.activationOpen = true
	return &ActivationClaim{lease: lease}, nil
}

func (claim *ActivationClaim) Complete(success bool) {
	if claim == nil || claim.lease == nil {
		return
	}
	claim.once.Do(func() {
		claim.lease.mu.Lock()
		defer claim.lease.mu.Unlock()
		claim.lease.activationOpen = false
		if success && !claim.lease.closed && !claim.lease.closing {
			claim.lease.activated = true
		}
	})
}

func AcquireCompositeLease(roots RootSet) (*CompositeLease, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	directory, err := defaultLeaseDirectory()
	if err != nil {
		return nil, err
	}
	return acquireCompositeLeaseAtPaths(resolved, CanonicalRoots(resolved), directory)
}

// AcquireCompositeLeaseWithStartupUserData freezes the desktop startup
// authority beneath the exact Electron userData root. Ordinary desktop
// runtime activation uses this constructor so it cannot silently drift from
// the pre-activation migration namespace through ambient host configuration.
func AcquireCompositeLeaseWithStartupUserData(roots RootSet, userDataDir string) (*CompositeLease, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	journalPath, canonicalUserData, err := persistentStartupNamespacePathForUserData(resolved, userDataDir, false)
	if err != nil {
		return nil, err
	}
	directory, err := defaultLeaseDirectory()
	if err != nil {
		return nil, err
	}
	return acquireCompositeLeaseAtPathsAndSeparateOwners(
		resolved, CanonicalRoots(resolved), nil, directory, true, journalPath, canonicalUserData,
	)
}

// AcquireCompositeLeaseWithSeparateOwnerRoots coordinates the ordinary Go
// roots and separately owned security roots under one OS lease. Separate roots
// never enter RootSet, RootAuthority, or the Go semantic journal.
func AcquireCompositeLeaseWithSeparateOwnerRoots(roots RootSet, ownerRoots ...string) (*CompositeLease, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	owners, err := ResolveSeparateOwnerRoots(resolved, ownerRoots...)
	if err != nil {
		return nil, err
	}
	paths := append(CanonicalRoots(resolved), owners...)
	directory, err := defaultLeaseDirectory()
	if err != nil {
		return nil, err
	}
	return acquireCompositeLeaseAtPathsAndSeparateOwners(resolved, paths, owners, directory, false, "", "")
}

// AcquireCompositeLeaseWithStartupUserDataAndSeparateOwnerRoots gives the
// desktop migration one shared coordination lease while keeping the Go
// semantic roots, Electron-owned roots, and startup journal responsibilities
// distinct. The startup journal still resolves only from userDataDir.
func AcquireCompositeLeaseWithStartupUserDataAndSeparateOwnerRoots(
	roots RootSet,
	userDataDir string,
	ownerRoots ...string,
) (*CompositeLease, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	journalPath, canonicalUserData, err := persistentStartupNamespacePathForUserData(resolved, userDataDir, false)
	if err != nil {
		return nil, err
	}
	owners, err := ResolveSeparateOwnerRoots(resolved, ownerRoots...)
	if err != nil {
		return nil, err
	}
	paths := append(CanonicalRoots(resolved), owners...)
	directory, err := defaultLeaseDirectory()
	if err != nil {
		return nil, err
	}
	return acquireCompositeLeaseAtPathsAndSeparateOwners(
		resolved, paths, owners, directory, false, journalPath, canonicalUserData,
	)
}

func acquireCompositeLeaseAt(roots RootSet, directory string) (*CompositeLease, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	return acquireCompositeLeaseAtPaths(resolved, CanonicalRoots(resolved), directory)
}

func acquireCompositeLeaseAtPaths(roots RootSet, coordinationRoots []string, directory string) (*CompositeLease, error) {
	return acquireCompositeLeaseAtPathsAndSeparateOwners(roots, coordinationRoots, nil, directory, true, "", "")
}

func acquireCompositeLeaseAtPathsAndSeparateOwners(
	roots RootSet,
	coordinationRoots []string,
	separateOwnerRoots []string,
	directory string,
	bootstrapSingleOwnerJournal bool,
	journalPath string,
	startupUserDataRoot string,
) (*CompositeLease, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	roots = resolved
	if journalPath == "" {
		journalPath, err = persistentStartupNamespacePath(roots, false)
		if err != nil {
			return nil, err
		}
	}
	journalKey := canonicalPathKey(journalPath)
	for _, managedRoot := range CanonicalRoots(roots) {
		if journalKey == managedRoot || pathContains(journalKey, managedRoot) || pathContains(managedRoot, journalKey) {
			return nil, errors.New("startup journal namespace must remain outside managed persistence roots")
		}
	}
	specs := compositeLeaseSpecsForPaths(coordinationRoots)
	if len(specs) == 0 {
		return nil, errors.New("persistence coordination roots are unavailable")
	}
	lease := &CompositeLease{roots: roots, startupUserDataRoot: startupUserDataRoot}
	lease.accessCond = sync.NewCond(&lease.mu)
	kernel, err := acquireKernelScopeLeaseForSpecs(specs)
	if err != nil {
		return nil, err
	}
	lease.kernel = kernel
	scope, err := acquirePlatformScopeLeaseForSpecs(specs)
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	lease.scope = scope
	if err := prepareLeaseDirectory(directory); err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	for _, spec := range specs {
		digest := sha256.Sum256([]byte(spec.path))
		path := filepath.Join(directory, hex.EncodeToString(digest[:])+".lock")
		file, err := openLeaseFile(path)
		if err != nil {
			return nil, errors.Join(err, lease.Close())
		}
		locked, err := tryPlatformFileLock(file, spec.exclusive)
		if err != nil {
			_ = file.Close()
			return nil, errors.Join(err, lease.Close())
		}
		if !locked {
			_ = file.Close()
			return nil, errors.Join(fmt.Errorf("%w: %s", ErrPersistenceInUse, hex.EncodeToString(digest[:8])), lease.Close())
		}
		lease.files = append(lease.files, file)
		lease.paths = append(lease.paths, path)
	}
	authority, err := FreezeRootAuthority(roots)
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	lease.authority = authority
	lease.separateOwners = make(map[string]*SeparateOwnerRootAuthority, len(separateOwnerRoots))
	for _, root := range separateOwnerRoots {
		ownerAuthority, err := freezeSeparateOwnerRootAuthority(root)
		if err != nil {
			return nil, errors.Join(err, lease.Close())
		}
		key := canonicalPathKey(root)
		if _, duplicate := lease.separateOwners[key]; duplicate {
			return nil, errors.Join(errors.New("duplicate separate-owner root authority"), lease.Close())
		}
		if err := validateSeparateOwnerDisjointFromManaged(authority, ownerAuthority, lease.separateOwners); err != nil {
			return nil, errors.Join(err, lease.Close())
		}
		lease.separateOwners[key] = ownerAuthority
	}
	lease.separateDigest = separateOwnerAuthoritiesDigest(lease.separateOwners)
	journalLocator, err := FreezeRootAuthority(RootSet{DataDir: journalPath, DurableDir: journalPath})
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	lease.journalLocator = journalLocator
	if bootstrapSingleOwnerJournal {
		journalAuthority, err := lease.createJournalAuthorityCandidateAfterPreflightV1()
		if err != nil {
			return nil, errors.Join(err, lease.Close())
		}
		if _, err := lease.commitJournalAuthorityCandidateV1(journalAuthority, nil); err != nil {
			return nil, errors.Join(err, lease.Close())
		}
	}
	return lease, nil
}

type compositeLeaseSpec struct {
	path      string
	exclusive bool
}

func compositeLeaseSpecs(roots RootSet) []compositeLeaseSpec {
	return compositeLeaseSpecsForPaths(CanonicalRoots(roots))
}

func compositeLeaseSpecsForPaths(roots []string) []compositeLeaseSpec {
	modes := map[string]bool{}
	for _, root := range roots {
		chain := []string{}
		current := canonicalPathKey(root)
		for {
			chain = append(chain, current)
			parent := canonicalPathKey(filepath.Dir(current))
			if parent == current {
				break
			}
			current = parent
		}
		for index := len(chain) - 1; index >= 0; index-- {
			path := chain[index]
			exclusive := index == 0
			modes[path] = modes[path] || exclusive
		}
	}
	paths := make([]string, 0, len(modes))
	for path := range modes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	specs := make([]compositeLeaseSpec, 0, len(paths))
	for _, path := range paths {
		specs = append(specs, compositeLeaseSpec{path: path, exclusive: modes[path]})
	}
	return specs
}

func (lease *CompositeLease) Close() error {
	if lease == nil {
		return nil
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	lease.ensureAccessCondLocked()
	if lease.closed {
		return nil
	}
	lease.closing = true
	for lease.activeAccesses > 0 {
		lease.accessCond.Wait()
	}
	var closeErr error
	if lease.kernel != nil {
		if err := lease.kernel.Close(); err != nil {
			return err
		}
		lease.kernel = nil
	}
	for index := len(lease.files) - 1; index >= 0; index-- {
		file := lease.files[index]
		closeErr = errors.Join(closeErr, unlockPlatformFile(file), file.Close())
	}
	if lease.scope != nil {
		closeErr = errors.Join(closeErr, lease.scope.Close())
	}
	for _, authority := range lease.separateOwners {
		closeErr = errors.Join(closeErr, authority.close())
	}
	lease.closed = true
	lease.activationOpen = false
	lease.activated = false
	lease.files = nil
	lease.paths = nil
	lease.scope = nil
	lease.authority = nil
	lease.journalAuthority = nil
	lease.journalLocator = nil
	lease.startupUserDataRoot = ""
	lease.separateOwners = nil
	lease.separateDigest = ""
	lease.accessCond.Broadcast()
	return closeErr
}

// ValidateStartupUserData proves that the caller is still using the same
// explicit desktop authority mode and userData root that were frozen when the
// lease was acquired. Blank means the legacy ambient constructor; callers may
// not switch between explicit and ambient authority after acquisition.
func (lease *CompositeLease) ValidateStartupUserData(userDataDir string) error {
	if lease == nil {
		return errors.New("startup authority lease is unavailable")
	}
	lease.mu.Lock()
	if !lease.liveLocked() {
		lease.mu.Unlock()
		return errors.New("startup authority lease is not live")
	}
	roots := lease.roots
	frozenUserData := lease.startupUserDataRoot
	locatorRoots, held := lease.journalLocator.Roots()
	lease.mu.Unlock()
	if !held {
		return errors.New("startup authority locator is unavailable")
	}
	if frozenUserData == "" {
		if strings.TrimSpace(userDataDir) != "" {
			return errors.New("startup authority mode changed after lease acquisition")
		}
		return nil
	}
	journalPath, canonicalUserData, err := persistentStartupNamespacePathForUserData(roots, userDataDir, false)
	if err != nil || canonicalPathKey(canonicalUserData) != canonicalPathKey(frozenUserData) ||
		canonicalPathKey(journalPath) != canonicalPathKey(locatorRoots.DataDir) {
		return errors.New("startup authority user data root changed after lease acquisition")
	}
	return nil
}

func (lease *CompositeLease) ensureAccessCondLocked() {
	if lease.accessCond == nil {
		lease.accessCond = sync.NewCond(&lease.mu)
	}
}

func (lease *CompositeLease) FrozenRoots() (RootSet, bool) {
	if lease == nil {
		return RootSet{}, false
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() {
		return RootSet{}, false
	}
	return lease.roots, true
}

func (lease *CompositeLease) liveLocked() bool {
	return lease.liveLockedWithRootValidation(true)
}

func (lease *CompositeLease) liveLockedWithRootValidation(validateRoot bool) bool {
	if lease.closed || lease.closing || len(lease.files) == 0 || lease.kernel == nil || lease.kernel.Validate() != nil ||
		lease.scope == nil || lease.scope.Validate() != nil ||
		lease.authority == nil || validateRoot && lease.authority.Validate() != nil || lease.validateSeparateOwnersLocked() != nil ||
		lease.journalLocator == nil || lease.journalLocator.Validate() != nil ||
		(lease.journalAuthority != nil && !lease.journalAuthorityMatchesLocatorLocked(lease.journalAuthority)) {
		return false
	}
	for index, file := range lease.files {
		openedInfo, openedErr := file.Stat()
		pathInfo, pathErr := os.Lstat(lease.paths[index])
		if openedErr != nil || pathErr != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, pathInfo) {
			return false
		}
	}
	return true
}

func (lease *CompositeLease) FrozenAuthority() (*RootAuthority, bool) {
	if lease == nil {
		return nil, false
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() {
		return nil, false
	}
	return lease.authority, true
}

func (lease *CompositeLease) FrozenJournalAuthority() (*JournalNamespaceAuthority, bool) {
	if lease == nil {
		return nil, false
	}
	lease.journalPublicationMu.Lock()
	defer lease.journalPublicationMu.Unlock()
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() || lease.journalAuthority == nil || lease.journalAuthority.Validate() != nil ||
		!lease.journalAuthorityMatchesLocatorLocked(lease.journalAuthority) {
		return nil, false
	}
	return lease.journalAuthority, true
}

// OpenExistingJournalAuthorityV1 performs no-create phase-zero discovery. A
// missing namespace is reported as present=false; an unsafe or changed
// namespace fails closed. The authority is retained only by this live lease.
func (lease *CompositeLease) OpenExistingJournalAuthorityV1() (*JournalNamespaceAuthority, bool, error) {
	authority, present, err := lease.openExistingJournalAuthorityCandidateV1()
	if err != nil || !present {
		return nil, present, err
	}
	committed, err := lease.commitJournalAuthorityCandidateV1(authority, nil)
	return committed, err == nil, err
}

// openExistingJournalAuthorityCandidateV1 captures an existing namespace as a
// provisional capability. It never publishes the capability through the
// lease; a prepared owner proof must revalidate and commit it separately.
func (lease *CompositeLease) openExistingJournalAuthorityCandidateV1() (*JournalNamespaceAuthority, bool, error) {
	if lease == nil {
		return nil, false, errors.New("persistence lease is unavailable")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() {
		return nil, false, errors.New("persistence lease is not live")
	}
	if lease.journalAuthority != nil {
		if lease.journalAuthority.Validate() != nil || !lease.journalAuthority.matchesRoots(lease.roots) {
			return nil, false, errors.New("startup journal namespace authority changed")
		}
		return lease.journalAuthority, true, nil
	}
	locatorRoots, locatorHeld := lease.journalLocator.Roots()
	if !locatorHeld {
		return nil, false, errors.New("startup journal namespace locator is unavailable")
	}
	locatorBindings := lease.journalLocator.bindings()
	if len(locatorBindings) != 1 || locatorBindings[0].RootIdentity == "" {
		return nil, false, nil
	}
	directory := locatorRoots.DataDir
	authority, err := freezeJournalNamespaceAuthorityForResolvedPath(lease.roots, directory, false)
	if err != nil {
		return nil, false, err
	}
	if lease.journalLocator.Validate() != nil || !authority.matchesRoots(lease.roots) || authority.Validate() != nil ||
		!lease.journalAuthorityMatchesLocatorLocked(authority) {
		return nil, false, errors.New("existing startup journal namespace authority is invalid")
	}
	return authority, true, nil
}

// createJournalAuthorityAfterPreflightV1 is the namespace creation primitive.
// Composite-owner production paths reach it only through a prepared complete-
// owner proof; FrozenJournalAuthority itself is always a pure accessor.
func (lease *CompositeLease) createJournalAuthorityAfterPreflightV1() (*JournalNamespaceAuthority, error) {
	authority, err := lease.createJournalAuthorityCandidateAfterPreflightV1()
	if err != nil {
		return nil, err
	}
	return lease.commitJournalAuthorityCandidateV1(authority, nil)
}

// createJournalAuthorityCandidateAfterPreflightV1 creates and captures a
// provisional namespace without publishing it through FrozenJournalAuthority.
// The caller must commit it only after its complete owner proof revalidates.
func (lease *CompositeLease) createJournalAuthorityCandidateAfterPreflightV1() (*JournalNamespaceAuthority, error) {
	if lease == nil {
		return nil, errors.New("persistence lease is unavailable")
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() {
		return nil, errors.New("persistence lease is not live")
	}
	if lease.journalAuthority != nil {
		if lease.journalAuthority.Validate() != nil || !lease.journalAuthority.matchesRoots(lease.roots) {
			return nil, errors.New("startup journal namespace authority changed")
		}
		return lease.journalAuthority, nil
	}
	if lease.journalLocator.Validate() != nil {
		return nil, errors.New("startup journal namespace locator changed")
	}
	locatorRoots, locatorHeld := lease.journalLocator.Roots()
	if !locatorHeld {
		return nil, errors.New("startup journal namespace locator is unavailable")
	}
	directory := locatorRoots.DataDir
	if err := secureEnsureManagedRoot(lease.journalLocator, directory); err != nil {
		return nil, err
	}
	authority, err := freezeJournalNamespaceAuthorityForResolvedPath(lease.roots, directory, false)
	if err != nil {
		return nil, err
	}
	locatorBindings := lease.journalLocator.bindings()
	if len(locatorBindings) != 1 || locatorBindings[0].RootIdentity == "" ||
		!authority.matchesRoots(lease.roots) || authority.Validate() != nil || lease.journalLocator.Validate() != nil ||
		!lease.journalAuthorityMatchesLocatorLocked(authority) {
		return nil, errors.New("startup journal namespace authority is invalid")
	}
	return authority, nil
}

// commitJournalAuthorityCandidateV1 is the only point that makes a candidate
// visible through FrozenJournalAuthority. The optional owner guard runs before
// the publication critical section; candidates are never stored on the lease,
// so concurrent readers cannot observe a provisional capability. Owner guards
// may use ordinary lease-scoped read access and must not request journal
// authority themselves.
func (lease *CompositeLease) commitJournalAuthorityCandidateV1(
	authority *JournalNamespaceAuthority,
	ownerGuard func() error,
) (*JournalNamespaceAuthority, error) {
	if lease == nil || authority == nil {
		return nil, errors.New("startup journal namespace candidate is unavailable")
	}
	if ownerGuard != nil {
		if err := ownerGuard(); err != nil {
			return nil, err
		}
	}
	lease.journalPublicationMu.Lock()
	defer lease.journalPublicationMu.Unlock()
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() || authority.Validate() != nil || !authority.matchesRoots(lease.roots) ||
		lease.journalLocator.Validate() != nil || !lease.journalAuthorityMatchesLocatorLocked(authority) {
		return nil, errors.New("startup journal namespace candidate changed before publication")
	}
	if lease.journalAuthority != nil {
		if lease.journalAuthority.Validate() != nil || !lease.journalAuthority.matchesRoots(lease.roots) ||
			lease.journalAuthority.path() != authority.path() {
			return nil, errors.New("startup journal namespace authority changed")
		}
		return lease.journalAuthority, nil
	}
	lease.journalAuthority = authority
	return authority, nil
}

func (lease *CompositeLease) journalAuthorityMatchesLocatorLocked(authority *JournalNamespaceAuthority) bool {
	if lease == nil || authority == nil || lease.journalLocator == nil {
		return false
	}
	locatorRoots, held := lease.journalLocator.Roots()
	return held && canonicalPathKey(authority.path()) == canonicalPathKey(locatorRoots.DataDir)
}

func defaultLeaseDirectory() (string, error) {
	path, err := platformLeaseDirectory()
	if err != nil {
		return "", err
	}
	return canonicalPathWithoutCreate(path)
}

func prepareLeaseDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("persistence lease directory is not a private directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("persistence lease directory integrity check failed")
	}
	return os.Chmod(path, 0o700)
}

func openLeaseFile(path string) (*os.File, error) {
	var initialInfo os.FileInfo
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("persistence lease path is not a regular file")
		}
		initialInfo = info
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || (initialInfo != nil && !os.SameFile(initialInfo, openedInfo)) {
		_ = file.Close()
		return nil, errors.New("persistence lease file integrity check failed")
	}
	currentInfo, err := os.Lstat(path)
	if err != nil || currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, currentInfo) {
		_ = file.Close()
		return nil, errors.New("persistence lease file path changed during open")
	}
	_, links, identityErr := regularFileIdentity(file, openedInfo)
	if identityErr != nil || links > 1 {
		_ = file.Close()
		return nil, errors.New("persistence lease file identity check failed")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
