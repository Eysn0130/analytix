//go:build darwin || linux

package securegeneration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	domainartifact "analytix.local/runtime-go/internal/domain/artifactgeneration"

	"golang.org/x/sys/unix"
)

var errSimulatedCrash = errors.New("secure_generation_simulated_crash")

type rootAuthority struct {
	path          string
	parentFD      int
	rootName      string
	dev           uint64
	ino           uint64
	createdByCall bool
}

// PrivateDirectoryLease is a descriptor-pinned, process-local capability for
// one private directory created beneath an already-open private parent. It is
// used by build coordinators that must never reopen a caller path between
// mkdir and use. The directory name is bookkeeping only; every operation also
// verifies the exact device/inode held by the lease.
type PrivateDirectoryLease struct {
	mu            sync.Mutex
	authority     rootAuthority
	directory     *os.File
	removed       bool
	indeterminate bool
	closed        bool
}

// CreatePrivateDirectoryLeaseUnder creates exactly one new private directory.
// Existing names are never adopted. Validation and rollback reuse the same
// extended-security, durability, and identity rules as immutable generations.
func CreatePrivateDirectoryLeaseUnder(
	ctx context.Context,
	parent *os.File,
	name string,
) (*PrivateDirectoryLease, error) {
	if ctx == nil || ctx.Err() != nil || parent == nil || !validPublicName(name) {
		return nil, ErrInvalidInput
	}
	authority, err := openRootAuthorityUnder(parent, name, true)
	if err != nil {
		return nil, err
	}
	if !authority.createdByCall {
		return nil, errors.Join(ErrUnsafeRoot, closeRootAuthority(authority))
	}
	directoryFD, err := openDirectoryAt(authority.parentFD, authority.rootName)
	if err != nil {
		return nil, errors.Join(ErrCleanupIndeterminate, err, rollbackCreatedRootAuthority(authority), closeRootAuthority(authority))
	}
	stat, validateErr := validatePrivateDirectory(directoryFD)
	if validateErr != nil || uint64(stat.Dev) != authority.dev || stat.Ino != authority.ino {
		closeErr := unix.Close(directoryFD)
		return nil, errors.Join(ErrCleanupIndeterminate, validateErr, closeErr, rollbackCreatedRootAuthority(authority), closeRootAuthority(authority))
	}
	directory := os.NewFile(uintptr(directoryFD), "secure-private-directory")
	if directory == nil {
		closeErr := unix.Close(directoryFD)
		return nil, errors.Join(ErrCleanupIndeterminate, closeErr, rollbackCreatedRootAuthority(authority), closeRootAuthority(authority))
	}
	return &PrivateDirectoryLease{authority: authority, directory: directory}, nil
}

// Duplicate returns a CLOEXEC descriptor for the exact held directory. A
// caller owns the duplicate but receives no path authority.
func (lease *PrivateDirectoryLease) Duplicate() (*os.File, error) {
	if lease == nil {
		return nil, ErrInvalidInput
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.closed || lease.removed || lease.indeterminate || lease.directory == nil {
		return nil, ErrUnsafeRoot
	}
	stat, err := validatePrivateDirectory(int(lease.directory.Fd()))
	if err != nil || uint64(stat.Dev) != lease.authority.dev || stat.Ino != lease.authority.ino {
		lease.indeterminate = true
		return nil, errors.Join(ErrUnsafeRoot, err)
	}
	duplicate, err := duplicateFileDescriptor(lease.directory)
	if err != nil {
		return nil, errors.Join(ErrUnsafeRoot, err)
	}
	file := os.NewFile(uintptr(duplicate), "secure-private-directory-duplicate")
	if file == nil {
		return nil, errors.Join(ErrUnsafeRoot, unix.Close(duplicate))
	}
	return file, nil
}

// RemoveEmpty removes only the exact empty directory represented by the
// lease. If the name was replaced, the replacement is never removed. The bool
// is true only when the exact directory had already been detached.
func (lease *PrivateDirectoryLease) RemoveEmpty(ctx context.Context) (alreadyRemoved bool, returnErr error) {
	if lease == nil || ctx == nil || ctx.Err() != nil {
		return false, ErrInvalidInput
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.closed || lease.directory == nil {
		return false, ErrUnsafeRoot
	}
	if lease.indeterminate {
		return false, ErrCleanupIndeterminate
	}
	if lease.removed {
		return true, nil
	}
	rootFD := int(lease.directory.Fd())
	stat, err := validatePrivateDirectory(rootFD)
	if err != nil || uint64(stat.Dev) != lease.authority.dev || stat.Ino != lease.authority.ino {
		lease.indeterminate = true
		return false, errors.Join(ErrCleanupIndeterminate, ErrUnsafeRoot, err)
	}
	if _, err := validatePrivateDirectory(lease.authority.parentFD); err != nil {
		lease.indeterminate = true
		return false, errors.Join(ErrCleanupIndeterminate, err)
	}
	if err := unix.Flock(rootFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lease.indeterminate = true
		return false, errors.Join(ErrCleanupIndeterminate, err)
	}
	defer func() {
		if unlockErr := unix.Flock(rootFD, unix.LOCK_UN); unlockErr != nil {
			lease.indeterminate = true
			alreadyRemoved = false
			returnErr = errors.Join(returnErr, ErrCleanupIndeterminate, unlockErr)
		}
	}()
	for range 2 {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		entries, readErr := readDirectoryEntries(rootFD)
		if errors.Is(readErr, unix.ENOENT) {
			// Linux getdents may reject an already-unlinked directory rather
			// than return an empty inventory. This is not evidence of emptiness:
			// require the exact held inode to have no links and the platform to
			// confirm detachment. The final name check below still owns cleanup.
			var held unix.Stat_t
			heldErr := unix.Fstat(rootFD, &held)
			detached, detachedErr := privateDirectoryDetached(rootFD)
			if heldErr == nil && held.Nlink == 0 && uint64(held.Dev) == lease.authority.dev &&
				held.Ino == lease.authority.ino && detachedErr == nil && detached {
				break
			}
		}
		if readErr != nil || len(entries) != 0 {
			lease.indeterminate = true
			return false, errors.Join(ErrCleanupIndeterminate, ErrResidue, readErr)
		}
	}
	if err := unix.Fsync(rootFD); err != nil {
		lease.indeterminate = true
		return false, errors.Join(ErrCleanupIndeterminate, err)
	}
	current, statErr := statAtNoFollow(lease.authority.parentFD, lease.authority.rootName)
	if statErr == nil && uint64(current.Dev) == lease.authority.dev && current.Ino == lease.authority.ino &&
		current.Mode&unix.S_IFMT == unix.S_IFDIR {
		if err := unix.Unlinkat(lease.authority.parentFD, lease.authority.rootName, unix.AT_REMOVEDIR); err != nil {
			lease.indeterminate = true
			return false, errors.Join(ErrCleanupIndeterminate, err)
		}
		lease.removed = true
		if _, err := statAtNoFollow(lease.authority.parentFD, lease.authority.rootName); !errors.Is(err, unix.ENOENT) {
			lease.indeterminate = true
			return false, errors.Join(ErrCleanupIndeterminate, ErrResidue, err)
		}
		if err := unix.Fsync(lease.authority.parentFD); err != nil {
			lease.indeterminate = true
			return false, errors.Join(ErrCleanupIndeterminate, err)
		}
		return false, nil
	}
	detached, detachedErr := privateDirectoryDetached(rootFD)
	if detachedErr == nil && detached && (errors.Is(statErr, unix.ENOENT) || statErr == nil) {
		// The exact held directory was already detached. A same-name replacement
		// is deliberately left untouched.
		lease.removed = true
		if err := unix.Fsync(lease.authority.parentFD); err != nil {
			lease.indeterminate = true
			return false, errors.Join(ErrCleanupIndeterminate, err)
		}
		return true, nil
	}
	lease.indeterminate = true
	return false, errors.Join(ErrCleanupIndeterminate, ErrUnsafeRoot, statErr, detachedErr)
}

// Close releases descriptors on every path. A caller that did not first prove
// exact removal receives an indeterminate cleanup error rather than silently
// turning a leaked directory into success.
func (lease *PrivateDirectoryLease) Close() error {
	if lease == nil {
		return ErrInvalidInput
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.closed {
		return nil
	}
	lease.closed = true
	var stateErr error
	if !lease.removed || lease.indeterminate {
		stateErr = ErrCleanupIndeterminate
	}
	var directoryErr error
	if lease.directory != nil {
		directoryErr = lease.directory.Close()
		lease.directory = nil
	}
	parentErr := closeRootAuthority(lease.authority)
	lease.authority.parentFD = -1
	return errors.Join(stateErr, directoryErr, parentErr)
}

type objectIdentity struct {
	dev uint64
	ino uint64
}

type unixGenerationRecord struct {
	generationRecord
	identity objectIdentity
}

type topInventory struct {
	publicPresent   bool
	previousPresent bool
	journalPresent  bool
	stageName       string
	retainedName    string
	commitName      string
}

type transactionState struct {
	journalBytes  []byte
	journalExists bool
	commitBytes   []byte
	commitExists  bool
	commitName    string
	stageName     string
	stageExists   bool
	retainedName  string
	retainedMoved bool
	oldMoved      bool
	newVisible    bool
	prior         *unixGenerationRecord
	retained      *unixGenerationRecord
	next          *unixGenerationRecord
}

func openRootAuthority(root string, create bool) (rootAuthority, error) {
	canonical, err := canonicalRootPath(root)
	if err != nil {
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
	}
	fd, created, err := openAbsoluteDirectory(canonical, create)
	if err != nil {
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
	}
	defer unix.Close(fd)
	if created {
		if err := unix.Fchmod(fd, domainartifact.DirectoryModeV1); err != nil {
			return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
		}
		if err := clearCreatedExtendedSecurity(fd); err != nil {
			return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
		}
	}
	stat, err := validatePrivateDirectory(fd)
	if err != nil {
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
	}
	if err := unix.Fsync(fd); err != nil {
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
	}
	return rootAuthority{path: canonical, parentFD: -1, dev: uint64(stat.Dev), ino: stat.Ino}, nil
}

func openRootAuthorityUnder(parentFile *os.File, rootName string, create bool) (rootAuthority, error) {
	if parentFile == nil || !validPublicName(rootName) {
		return rootAuthority{}, ErrInvalidInput
	}
	parent, err := duplicateFileDescriptor(parentFile)
	if err != nil {
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
	}
	closeParent := true
	defer func() {
		if closeParent {
			_ = unix.Close(parent)
		}
	}()
	if _, err := validatePrivateDirectory(parent); err != nil {
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
	}
	root, err := openDirectoryAt(parent, rootName)
	createdByCall := false
	createdAuthority := rootAuthority{}
	if errors.Is(err, unix.ENOENT) && create {
		if err = unix.Mkdirat(parent, rootName, domainartifact.DirectoryModeV1); err != nil {
			return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
		}
		createdByCall = true
		root, err = openDirectoryAt(parent, rootName)
		if err != nil {
			return rootAuthority{}, errors.Join(ErrUnsafeRoot, ErrCleanupIndeterminate, err)
		}
		var createdStat unix.Stat_t
		if err = unix.Fstat(root, &createdStat); err != nil || createdStat.Mode&unix.S_IFMT != unix.S_IFDIR {
			_ = unix.Close(root)
			return rootAuthority{}, errors.Join(ErrUnsafeRoot, ErrCleanupIndeterminate, err)
		}
		createdAuthority = rootAuthority{
			parentFD: parent, rootName: rootName,
			dev: uint64(createdStat.Dev), ino: createdStat.Ino, createdByCall: true,
		}
		if err == nil {
			err = unix.Fchmod(root, domainartifact.DirectoryModeV1)
		}
		if err == nil {
			err = clearCreatedExtendedSecurity(root)
		}
		if err == nil {
			err = unix.Fsync(root)
		}
		if err == nil {
			err = unix.Fsync(parent)
		}
		if err != nil {
			cleanupErr := rollbackCreatedRootAuthority(createdAuthority)
			closeErr := unix.Close(root)
			return rootAuthority{}, errors.Join(ErrUnsafeRoot, err, cleanupErr, closeErr)
		}
	}
	if err != nil {
		if root >= 0 {
			_ = unix.Close(root)
		}
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, err)
	}
	stat, validateErr := validatePrivateDirectory(root)
	closeErr := unix.Close(root)
	if validateErr != nil || closeErr != nil {
		var cleanupErr error
		if createdByCall {
			cleanupErr = rollbackCreatedRootAuthority(createdAuthority)
		}
		return rootAuthority{}, errors.Join(ErrUnsafeRoot, validateErr, closeErr, cleanupErr)
	}
	closeParent = false
	return rootAuthority{
		parentFD: parent, rootName: rootName,
		dev: uint64(stat.Dev), ino: stat.Ino, createdByCall: createdByCall,
	}, nil
}

func closeRootAuthority(authority rootAuthority) error {
	if authority.parentFD >= 0 {
		return unix.Close(authority.parentFD)
	}
	return nil
}

func rootAuthorityCreatedByCall(authority rootAuthority) bool {
	return authority.createdByCall
}

// rollbackCreatedRootAuthority removes only an empty root created by this
// call. The parent descriptor and the root's exact device/inode remain the
// authority; an existing root, a populated root, or an identity ambiguity is
// never deleted.
func rollbackCreatedRootAuthority(authority rootAuthority) error {
	if !authority.createdByCall || authority.parentFD < 0 || !validPublicName(authority.rootName) {
		return errors.Join(ErrCleanupIndeterminate, ErrUnsafeRoot)
	}
	if _, err := validatePrivateDirectory(authority.parentFD); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	root, err := openDirectoryAt(authority.parentFD, authority.rootName)
	if err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	defer unix.Close(root)
	stat, err := validatePrivateDirectory(root)
	if err != nil || uint64(stat.Dev) != authority.dev || stat.Ino != authority.ino {
		return errors.Join(ErrCleanupIndeterminate, ErrUnsafeRoot, err)
	}
	if err := unix.Flock(root, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	defer unix.Flock(root, unix.LOCK_UN)
	for range 2 {
		entries, readErr := readDirectoryEntries(root)
		if readErr != nil || len(entries) != 0 {
			return errors.Join(ErrCleanupIndeterminate, ErrResidue, readErr)
		}
		current, statErr := statAtNoFollow(authority.parentFD, authority.rootName)
		if statErr != nil || uint64(current.Dev) != authority.dev || current.Ino != authority.ino ||
			current.Mode&unix.S_IFMT != unix.S_IFDIR {
			return errors.Join(ErrCleanupIndeterminate, ErrUnsafeRoot, statErr)
		}
	}
	if err := unix.Fsync(root); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	if err := unix.Unlinkat(authority.parentFD, authority.rootName, unix.AT_REMOVEDIR); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	if _, err := statAtNoFollow(authority.parentFD, authority.rootName); !errors.Is(err, unix.ENOENT) {
		return errors.Join(ErrCleanupIndeterminate, ErrResidue, err)
	}
	if err := unix.Fsync(authority.parentFD); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	return nil
}

func discardExpectedUnder(
	store *Store,
	ctx context.Context,
	expected domainartifact.ReceiptV1,
) (returnErr error) {
	if store == nil || store.root.parentFD < 0 || !validExpectedReceipt(expected, store.limits) {
		return ErrInvalidInput
	}
	root, release, err := lockRoot(ctx, store.root)
	if err != nil {
		return err
	}
	released := false
	defer func() {
		if !released {
			returnErr = errors.Join(returnErr, release())
		}
	}()
	top, err := inspectTop(root, store)
	if err != nil {
		return err
	}
	if !top.publicPresent || top.previousPresent || top.journalPresent ||
		top.stageName != "" || top.retainedName != "" || top.commitName != "" {
		return ErrResidue
	}
	current, err := readGenerationAt(root, store.publicName, store.limits)
	if err != nil {
		return err
	}
	if current.receipt != expected {
		return ErrCurrentGeneration
	}
	if err := removeGenerationAt(root, store.publicName, current, store.limits); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	if err := removeEmptyPinnedRootLocked(store, root); err != nil {
		return err
	}
	releaseErr := release()
	released = true
	if releaseErr != nil {
		return errors.Join(ErrCleanupIndeterminate, releaseErr)
	}
	return nil
}

func discardCurrentUnder(
	store *Store,
	ctx context.Context,
) (result DiscardCurrentResult, returnErr error) {
	if store == nil || store.root.parentFD < 0 {
		return DiscardCurrentResult{}, ErrInvalidInput
	}
	root, release, err := lockRoot(ctx, store.root)
	if err != nil {
		return DiscardCurrentResult{}, err
	}
	released := false
	defer func() {
		if !released {
			returnErr = errors.Join(returnErr, release())
		}
	}()
	top, err := inspectTop(root, store)
	if err != nil {
		return DiscardCurrentResult{}, err
	}
	if top.previousPresent || top.journalPresent || top.stageName != "" ||
		top.retainedName != "" || top.commitName != "" {
		return DiscardCurrentResult{}, ErrResidue
	}
	if top.publicPresent {
		current, readErr := readGenerationAt(root, store.publicName, store.limits)
		if readErr != nil {
			return DiscardCurrentResult{}, readErr
		}
		result = DiscardCurrentResult{
			Installed:     true,
			Current:       current.receipt,
			ReceiptDigest: domainartifact.DigestBytesV1(current.receiptBytes),
		}
		if err := removeGenerationAt(root, store.publicName, current, store.limits); err != nil {
			return DiscardCurrentResult{}, errors.Join(ErrCleanupIndeterminate, err)
		}
	}
	if err := removeEmptyPinnedRootLocked(store, root); err != nil {
		return DiscardCurrentResult{}, err
	}
	releaseErr := release()
	released = true
	if releaseErr != nil {
		return DiscardCurrentResult{}, errors.Join(ErrCleanupIndeterminate, releaseErr)
	}
	return result, nil
}

func removeEmptyPinnedRootLocked(store *Store, root int) error {
	if store == nil || store.root.parentFD < 0 || root < 0 {
		return ErrInvalidInput
	}
	for range 2 {
		entries, readErr := readDirectoryEntries(root)
		if readErr != nil || len(entries) != 0 {
			return errors.Join(ErrCleanupIndeterminate, ErrResidue, readErr)
		}
		currentRoot, statErr := statAtNoFollow(store.root.parentFD, store.root.rootName)
		if statErr != nil || uint64(currentRoot.Dev) != store.root.dev || currentRoot.Ino != store.root.ino ||
			currentRoot.Mode&unix.S_IFMT != unix.S_IFDIR {
			return errors.Join(ErrCleanupIndeterminate, ErrUnsafeRoot, statErr)
		}
	}
	if err := unix.Fsync(root); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	if err := unix.Unlinkat(store.root.parentFD, store.root.rootName, unix.AT_REMOVEDIR); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	if _, err := statAtNoFollow(store.root.parentFD, store.root.rootName); !errors.Is(err, unix.ENOENT) {
		return errors.Join(ErrCleanupIndeterminate, ErrResidue, err)
	}
	if err := unix.Fsync(store.root.parentFD); err != nil {
		return errors.Join(ErrCleanupIndeterminate, err)
	}
	return nil
}

func observe(store *Store, ctx context.Context) (Observation, error) {
	root, release, err := lockRoot(ctx, store.root)
	if err != nil {
		return Observation{}, err
	}
	defer release()
	top, err := inspectTop(root, store)
	if err != nil {
		return Observation{}, err
	}
	if top.journalPresent || top.stageName != "" || top.retainedName != "" || top.commitName != "" {
		return Observation{}, ErrRecoveryRequired
	}
	if !top.publicPresent {
		if top.previousPresent {
			return Observation{}, ErrResidue
		}
		if err := requireRootAuthorityPath(store.root); err != nil {
			return Observation{}, err
		}
		return Observation{}, nil
	}
	current, err := readGenerationAt(root, store.publicName, store.limits)
	if err != nil {
		return Observation{}, err
	}
	observation := Observation{Installed: true, Current: current.receipt}
	if top.previousPresent {
		previous, err := readGenerationAt(root, store.previousName, store.limits)
		if err != nil || previous.receipt.GenerationID == current.receipt.GenerationID {
			return Observation{}, errors.Join(ErrResidue, err)
		}
		copy := previous.receipt
		observation.Previous = &copy
	}
	if err := requireRootAuthorityPath(store.root); err != nil {
		return Observation{}, err
	}
	return observation, nil
}

func publish(
	store *Store,
	ctx context.Context,
	prepared domainartifact.PreparedV1,
	expected *ExpectedCurrent,
) (PublishResult, error) {
	nextReceipt := prepared.Receipt()
	notCommitted := PublishResult{State: NotCommitted}
	indeterminate := PublishResult{State: Indeterminate, Receipt: nextReceipt}
	committed := PublishResult{State: Committed, Receipt: nextReceipt}
	if err := contextError(ctx); err != nil {
		return notCommitted, err
	}
	root, release, err := lockRoot(ctx, store.root)
	if err != nil {
		return notCommitted, err
	}
	released := false
	releaseOnce := func() error {
		if released {
			return nil
		}
		released = true
		return release()
	}
	defer releaseOnce()

	top, err := inspectTop(root, store)
	if err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	if top.journalPresent || top.stageName != "" || top.retainedName != "" || top.commitName != "" {
		return notCommitted, errors.Join(ErrRecoveryRequired, releaseOnce())
	}
	var prior *unixGenerationRecord
	var retained *unixGenerationRecord
	if top.publicPresent {
		current, err := readGenerationAt(root, store.publicName, store.limits)
		if err != nil {
			return notCommitted, errors.Join(err, releaseOnce())
		}
		prior = &current
	}
	if top.previousPresent {
		currentRetained, err := readGenerationAt(root, store.previousName, store.limits)
		if err != nil {
			return notCommitted, errors.Join(err, releaseOnce())
		}
		retained = &currentRetained
	}
	if prior == nil && retained != nil || prior != nil && retained != nil && prior.receipt.GenerationID == retained.receipt.GenerationID {
		return notCommitted, errors.Join(ErrResidue, releaseOnce())
	}
	if prior != nil && prior.receipt.GenerationID == nextReceipt.GenerationID {
		stable := transactionState{next: prior, prior: retained}
		if err := errors.Join(
			contextError(ctx),
			requireStableVisibleState(root, store, &stable, "after_last_readback_before_return"),
			requireRootAuthorityPath(store.root),
		); err != nil {
			return indeterminate, errors.Join(ErrCommitIndeterminate, err, releaseOnce())
		}
		return committed, releaseOnce()
	}
	if expected != nil && !currentMatchesExpectation(prior, *expected) {
		return notCommitted, errors.Join(ErrCurrentGeneration, releaseOnce())
	}
	operation, err := operationID()
	if err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	stageName := "." + store.publicName + ".stage-" + operation
	next, err := stageGeneration(root, stageName, prepared, store.limits)
	if err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	state := transactionState{stageName: stageName, stageExists: true, prior: prior, next: &next}
	verifiedCommittedError := func(cause error, cleanupPending bool) (PublishResult, error) {
		if err := errors.Join(
			requireStableVisibleState(root, store, &state, "after_last_readback_before_return"),
			requireRootAuthorityPath(store.root),
		); err != nil {
			return indeterminate, errors.Join(ErrCommitIndeterminate, cause, err, releaseOnce())
		}
		if cleanupPending {
			cause = errors.Join(ErrCommittedCleanupPending, cause)
		}
		return committed, errors.Join(cause, releaseOnce())
	}
	previsibilityCleanup := func(cause error) (PublishResult, error) {
		cleanupErr := cleanupPrevisibility(root, store, &state)
		if cleanupErr != nil {
			return indeterminate, errors.Join(ErrCommitIndeterminate, cause, cleanupErr, releaseOnce())
		}
		return notCommitted, errors.Join(cause, releaseOnce())
	}
	if faultErr, crash := injectFault(store, "after_stage_sync"); faultErr != nil {
		if crash {
			// No journal exists, so startup has no authority to select or
			// delete this otherwise valid stage. It remains quarantined.
			return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
		}
		return previsibilityCleanup(faultErr)
	}
	if err := requireRootAuthorityPath(store.root); err != nil {
		return previsibilityCleanup(err)
	}

	journal, err := newJournalV1(store.publicName, operation, recordPointer(prior), recordPointer(retained), next.generationRecord)
	if err != nil {
		return previsibilityCleanup(err)
	}
	journalBytes, err := canonicalJournalBytesV1(journal, store.publicName)
	if err != nil {
		return previsibilityCleanup(err)
	}
	if err := writeExclusiveFileAt(root, store.journalName, journalBytes, maxJournalBytes, domainartifact.RegularFileModeV1, false); err != nil {
		return previsibilityCleanup(err)
	}
	state.journalBytes = journalBytes
	state.journalExists = true
	state.retainedName = journal.RetainedName
	state.commitName = journal.CommitName
	state.retained = retained
	if faultErr, crash := injectFault(store, "after_journal_sync"); faultErr != nil {
		if crash {
			return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
		}
		return previsibilityCleanup(faultErr)
	}
	if err := errors.Join(contextError(ctx), requireRootAuthorityPath(store.root)); err != nil {
		return previsibilityCleanup(err)
	}
	rollbackFailure := func(cause error) (PublishResult, error) {
		if rollbackErr := rollbackTransaction(root, store, &state); rollbackErr != nil {
			return indeterminate, errors.Join(ErrCommitIndeterminate, cause, rollbackErr, releaseOnce())
		}
		return notCommitted, errors.Join(cause, releaseOnce())
	}
	postVisibilityCut := func(phase string) (PublishResult, error, bool) {
		faultErr, crash := injectFault(store, phase)
		if faultErr == nil {
			return PublishResult{}, nil, false
		}
		if crash {
			return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce()), true
		}
		result, resultErr := rollbackFailure(faultErr)
		return result, resultErr, true
	}
	if retained != nil {
		if faultErr, crash := injectFault(store, "before_retained_previous_prune"); faultErr != nil {
			if crash {
				return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
			}
			return previsibilityCleanup(faultErr)
		}
		if err := errors.Join(requireRootAuthorityPath(store.root), requireGenerationAt(root, store.previousName, *retained, store.limits)); err != nil {
			return previsibilityCleanup(err)
		}
		if err := renameBoundNoReplace(root, store.previousName, journal.RetainedName, retained.identity); err != nil {
			return previsibilityCleanup(err)
		}
		state.retainedMoved = true
		if result, cutErr, stop := postVisibilityCut("after_retained_to_third"); stop {
			return result, cutErr
		}
		if err := publicationFsync(store, root, "retained_parent"); err != nil {
			return rollbackFailure(err)
		}
		if result, cutErr, stop := postVisibilityCut("after_retained_previous_prune"); stop {
			return result, cutErr
		}
		if err := requireRootAuthorityPath(store.root); err != nil {
			return rollbackFailure(err)
		}
	}
	if prior != nil {
		if faultErr, crash := injectFault(store, "before_old_to_previous"); faultErr != nil {
			if crash {
				return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
			}
			return rollbackFailure(faultErr)
		}
		if err := requireRootAuthorityPath(store.root); err != nil {
			return rollbackFailure(err)
		}
		if err := requireGenerationAt(root, store.publicName, *prior, store.limits); err != nil {
			return rollbackFailure(err)
		}
		if err := renameBoundNoReplace(root, store.publicName, store.previousName, prior.identity); err != nil {
			return rollbackFailure(err)
		}
		state.oldMoved = true
		if result, cutErr, stop := postVisibilityCut("after_old_to_previous"); stop {
			return result, cutErr
		}
		if err := publicationFsync(store, root, "old_parent"); err != nil {
			return rollbackFailure(err)
		}
		if result, cutErr, stop := postVisibilityCut("after_old_parent_sync"); stop {
			return result, cutErr
		}
		if err := requireRootAuthorityPath(store.root); err != nil {
			return rollbackFailure(err)
		}
	}
	if err := errors.Join(contextError(ctx), requireRootAuthorityPath(store.root)); err != nil {
		return rollbackFailure(err)
	}
	if result, cutErr, stop := postVisibilityCut("before_stage_to_public"); stop {
		return result, cutErr
	}
	if err := requireRootAuthorityPath(store.root); err != nil {
		return rollbackFailure(err)
	}
	if err := requireGenerationAt(root, stageName, next, store.limits); err != nil {
		return rollbackFailure(err)
	}
	if err := renameBoundNoReplace(root, stageName, store.publicName, next.identity); err != nil {
		return rollbackFailure(err)
	}
	state.stageExists = false
	state.newVisible = true
	if result, cutErr, stop := postVisibilityCut("after_stage_to_public"); stop {
		return result, cutErr
	}
	if err := publicationFsync(store, root, "public_parent"); err != nil {
		return rollbackFailure(err)
	}
	if result, cutErr, stop := postVisibilityCut("after_public_parent_sync"); stop {
		return result, cutErr
	}
	if err := requireRootAuthorityPath(store.root); err != nil {
		return rollbackFailure(err)
	}
	if err := requireGenerationAt(root, store.publicName, next, store.limits); err != nil {
		return rollbackFailure(err)
	}
	if prior != nil {
		if err := requireGenerationAt(root, store.previousName, *prior, store.limits); err != nil {
			return rollbackFailure(err)
		}
	}
	if result, cutErr, stop := postVisibilityCut("after_visibility_readback"); stop {
		return result, cutErr
	}
	if err := publicationFsync(store, root, "final_parent"); err != nil {
		return rollbackFailure(err)
	}
	if result, cutErr, stop := postVisibilityCut("after_final_parent_sync"); stop {
		return result, cutErr
	}
	if faultErr, crash := injectFault(store, "before_commit_stability_readback"); faultErr != nil {
		if crash {
			return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
		}
		return rollbackFailure(faultErr)
	}
	if err := requireStableVisibleState(root, store, &state, "between_precommit_stability_readbacks"); err != nil {
		return rollbackFailure(err)
	}
	commitBytes, err := canonicalCommitMarkerBytesV1(journal, store.publicName)
	if err != nil {
		return rollbackFailure(err)
	}
	if err := errors.Join(contextError(ctx), requireRootAuthorityPath(store.root)); err != nil {
		return rollbackFailure(err)
	}
	if err := writeExclusiveFileAt(root, journal.CommitName, commitBytes, maxJournalBytes, domainartifact.RegularFileModeV1, false); err != nil {
		return rollbackFailure(err)
	}
	state.commitBytes = commitBytes
	state.commitExists = true
	if faultErr, crash := injectFault(store, "after_commit_marker_sync"); faultErr != nil {
		if crash {
			return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
		}
		return verifiedCommittedError(faultErr, true)
	}

	if state.retainedMoved {
		if err := removeGenerationAt(root, state.retainedName, *state.retained, store.limits); err != nil {
			return verifiedCommittedError(err, true)
		}
		state.retainedMoved = false
	}
	if err := unlinkExactFileAt(root, store.journalName, journalBytes, maxJournalBytes, true); err != nil {
		return verifiedCommittedError(err, true)
	}
	state.journalExists = false
	if faultErr, crash := injectFault(store, "after_journal_unlink"); faultErr != nil {
		if crash {
			return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
		}
		return verifiedCommittedError(faultErr, true)
	}
	if err := unlinkExactFileAt(root, journal.CommitName, commitBytes, maxJournalBytes, true); err != nil {
		return verifiedCommittedError(err, true)
	}
	state.commitExists = false
	if faultErr, crash := injectFault(store, "after_commit_marker_unlink"); faultErr != nil {
		if crash {
			return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
		}
		return verifiedCommittedError(faultErr, false)
	}
	if faultErr, _ := injectFault(store, "before_return_stability_readback"); faultErr != nil {
		return indeterminate, errors.Join(ErrCommitIndeterminate, faultErr, releaseOnce())
	}
	if err := errors.Join(requireStableVisibleState(root, store, &state, "after_last_readback_before_return"), requireRootAuthorityPath(store.root)); err != nil {
		return indeterminate, errors.Join(ErrCommitIndeterminate, err, releaseOnce())
	}
	return committed, releaseOnce()
}

func currentMatchesExpectation(current *unixGenerationRecord, expected ExpectedCurrent) bool {
	switch expected.Kind {
	case ExpectedCurrentAbsent:
		return current == nil
	case ExpectedCurrentGeneration:
		return current != nil && current.receipt.GenerationID == expected.GenerationID
	default:
		return false
	}
}

func requireStableVisibleState(root int, store *Store, state *transactionState, betweenPhase string) error {
	if state == nil || state.next == nil {
		return ErrCommitIndeterminate
	}
	for pass := 0; pass < 2; pass++ {
		if err := requireRootAuthorityPath(store.root); err != nil {
			return err
		}
		if err := requireGenerationAt(root, store.publicName, *state.next, store.limits); err != nil {
			return err
		}
		if state.prior != nil {
			if err := requireGenerationAt(root, store.previousName, *state.prior, store.limits); err != nil {
				return err
			}
		}
		if state.retainedMoved {
			if state.retained == nil || state.retainedName == "" {
				return ErrCommitIndeterminate
			}
			if err := requireGenerationAt(root, state.retainedName, *state.retained, store.limits); err != nil {
				return err
			}
		}
		top, err := inspectTop(root, store)
		if err != nil || !top.publicPresent || top.previousPresent != (state.prior != nil) || top.journalPresent != state.journalExists ||
			top.stageName != "" || top.retainedName != expectedOptionalName(state.retainedMoved, state.retainedName) ||
			top.commitName != expectedOptionalName(state.commitExists, state.commitName) {
			return errors.Join(ErrCommitIndeterminate, err)
		}
		if pass == 0 && betweenPhase != "" {
			if faultErr, _ := injectFault(store, betweenPhase); faultErr != nil {
				return faultErr
			}
		}
	}
	return nil
}

func expectedOptionalName(present bool, name string) string {
	if present {
		return name
	}
	return ""
}

func recoverStore(store *Store, ctx context.Context) (Observation, error) {
	if err := contextError(ctx); err != nil {
		return Observation{}, err
	}
	root, release, err := lockRoot(ctx, store.root)
	if err != nil {
		return Observation{}, err
	}
	defer release()
	top, err := inspectTop(root, store)
	if err != nil {
		return Observation{}, err
	}
	if !top.journalPresent && top.commitName == "" {
		if top.stageName != "" || top.retainedName != "" || !top.publicPresent && top.previousPresent {
			return Observation{}, ErrResidue
		}
		return observeLocked(root, store, top)
	}
	if top.commitName != "" {
		return recoverCommittedStore(root, store, top)
	}
	journalFile, err := readFileAt(root, store.journalName, maxJournalBytes, domainartifact.RegularFileModeV1, false)
	if err != nil {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	journal, err := parseJournalV1(journalFile.body, store.publicName)
	if err != nil || top.stageName != "" && top.stageName != journal.StageName ||
		top.retainedName != "" && top.retainedName != journal.RetainedName {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	if err := requireRootAuthorityPath(store.root); err != nil {
		return Observation{}, err
	}

	publicRecord, err := readGenerationIfPresent(root, store.publicName, top.publicPresent, store.limits)
	if err != nil {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	previousRecord, err := readGenerationIfPresent(root, store.previousName, top.previousPresent, store.limits)
	if err != nil {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	stageRecord, err := readGenerationIfPresent(root, journal.StageName, top.stageName != "", store.limits)
	if err != nil {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	retainedRecord, err := readGenerationIfPresent(root, journal.RetainedName, top.retainedName != "", store.limits)
	if err != nil {
		return Observation{}, errors.Join(ErrResidue, err)
	}

	state := transactionState{
		journalBytes: journalFile.body, journalExists: true,
		stageName: journal.StageName, stageExists: stageRecord != nil,
		retainedName: journal.RetainedName, retainedMoved: retainedRecord != nil,
		commitName: journal.CommitName,
	}
	if !journal.HadPrior {
		switch {
		case publicRecord == nil && previousRecord == nil && retainedRecord == nil && matchesNext(stageRecord, journal):
			state.next = stageRecord
		case matchesNext(publicRecord, journal) && previousRecord == nil && retainedRecord == nil && stageRecord == nil:
			state.next, state.newVisible = publicRecord, true
		case publicRecord == nil && previousRecord == nil && retainedRecord == nil && stageRecord == nil:
		default:
			return Observation{}, ErrResidue
		}
	} else if !journal.HadRetainedPrevious {
		switch {
		case matchesPrior(publicRecord, journal) && previousRecord == nil && retainedRecord == nil && matchesNext(stageRecord, journal):
			state.prior, state.next = publicRecord, stageRecord
		case publicRecord == nil && matchesPrior(previousRecord, journal) && retainedRecord == nil && matchesNext(stageRecord, journal):
			state.prior, state.next, state.oldMoved = previousRecord, stageRecord, true
		case matchesNext(publicRecord, journal) && matchesPrior(previousRecord, journal) && retainedRecord == nil && stageRecord == nil:
			state.prior, state.next, state.oldMoved, state.newVisible = previousRecord, publicRecord, true, true
		case matchesPrior(publicRecord, journal) && previousRecord == nil && retainedRecord == nil && stageRecord == nil:
			state.prior = publicRecord
		default:
			return Observation{}, ErrResidue
		}
	} else {
		switch {
		case matchesPrior(publicRecord, journal) && matchesRetained(previousRecord, journal) && retainedRecord == nil && matchesNext(stageRecord, journal):
			state.prior, state.retained, state.next = publicRecord, previousRecord, stageRecord
			state.retainedMoved = false
		case matchesPrior(publicRecord, journal) && previousRecord == nil && matchesRetained(retainedRecord, journal) && matchesNext(stageRecord, journal):
			state.prior, state.retained, state.next = publicRecord, retainedRecord, stageRecord
		case publicRecord == nil && matchesPrior(previousRecord, journal) && matchesRetained(retainedRecord, journal) && matchesNext(stageRecord, journal):
			state.prior, state.retained, state.next, state.oldMoved = previousRecord, retainedRecord, stageRecord, true
		case matchesNext(publicRecord, journal) && matchesPrior(previousRecord, journal) && matchesRetained(retainedRecord, journal) && stageRecord == nil:
			state.prior, state.retained, state.next, state.oldMoved, state.newVisible = previousRecord, retainedRecord, publicRecord, true, true
		case matchesPrior(publicRecord, journal) && matchesRetained(previousRecord, journal) && retainedRecord == nil && stageRecord == nil:
			state.prior, state.retained = publicRecord, previousRecord
			state.retainedMoved = false
		default:
			return Observation{}, ErrResidue
		}
	}
	if err := rollbackTransaction(root, store, &state); err != nil {
		return Observation{}, errors.Join(ErrCommitIndeterminate, err)
	}
	if err := requireRootAuthorityPath(store.root); err != nil {
		return Observation{}, err
	}
	top, err = inspectTop(root, store)
	if err != nil {
		return Observation{}, err
	}
	return observeLocked(root, store, top)
}

func recoverCommittedStore(root int, store *Store, top topInventory) (Observation, error) {
	markerFile, err := readFileAt(root, top.commitName, maxJournalBytes, domainartifact.RegularFileModeV1, false)
	if err != nil {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	marker, err := parseCommitMarkerV1(markerFile.body, store.publicName)
	if err != nil || marker.Journal.CommitName != top.commitName || top.stageName != "" ||
		top.retainedName != "" && top.retainedName != marker.Journal.RetainedName {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	journal := marker.Journal
	if top.journalPresent {
		journalFile, err := readFileAt(root, store.journalName, maxJournalBytes, domainartifact.RegularFileModeV1, false)
		canonical, canonicalErr := canonicalJournalBytesV1(journal, store.publicName)
		if err != nil || canonicalErr != nil || !bytes.Equal(journalFile.body, canonical) {
			return Observation{}, errors.Join(ErrResidue, err, canonicalErr)
		}
	}
	publicRecord, err := readGenerationIfPresent(root, store.publicName, top.publicPresent, store.limits)
	if err != nil || !matchesNext(publicRecord, journal) {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	previousRecord, err := readGenerationIfPresent(root, store.previousName, top.previousPresent, store.limits)
	if err != nil || journal.HadPrior != (previousRecord != nil) || journal.HadPrior && !matchesPrior(previousRecord, journal) {
		return Observation{}, errors.Join(ErrResidue, err)
	}
	if top.retainedName != "" {
		retainedRecord, readErr := readGenerationAt(root, journal.RetainedName, store.limits)
		if readErr == nil {
			if !journal.HadRetainedPrevious || !matchesRetained(&retainedRecord, journal) {
				return Observation{}, ErrResidue
			}
			if err := removeGenerationAt(root, journal.RetainedName, retainedRecord, store.limits); err != nil {
				return Observation{}, errors.Join(ErrCommittedCleanupPending, err)
			}
		} else {
			if !journal.HadRetainedPrevious {
				return Observation{}, errors.Join(ErrResidue, readErr)
			}
			if err := resumeRetainedGenerationPrune(root, store, journal.RetainedName, journal); err != nil {
				return Observation{}, errors.Join(ErrResidue, readErr, err)
			}
		}
	}
	if top.journalPresent {
		canonical, err := canonicalJournalBytesV1(journal, store.publicName)
		unlinkErr := error(nil)
		if err == nil {
			unlinkErr = unlinkExactFileAt(root, store.journalName, canonical, maxJournalBytes, true)
		}
		if err != nil || unlinkErr != nil {
			return Observation{}, errors.Join(ErrCommittedCleanupPending, err, unlinkErr)
		}
	}
	if err := unlinkExactFileAt(root, journal.CommitName, markerFile.body, maxJournalBytes, true); err != nil {
		return Observation{}, errors.Join(ErrCommittedCleanupPending, err)
	}
	top, err = inspectTop(root, store)
	if err != nil {
		return Observation{}, err
	}
	return observeLocked(root, store, top)
}

func readGenerationIfPresent(root int, name string, present bool, limits Limits) (*unixGenerationRecord, error) {
	if !present {
		return nil, nil
	}
	record, err := readGenerationAt(root, name, limits)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func observeLocked(root int, store *Store, top topInventory) (Observation, error) {
	if top.journalPresent || top.stageName != "" || top.retainedName != "" || top.commitName != "" || !top.publicPresent && top.previousPresent {
		return Observation{}, ErrResidue
	}
	if !top.publicPresent {
		if err := requireRootAuthorityPath(store.root); err != nil {
			return Observation{}, err
		}
		return Observation{}, nil
	}
	current, err := readGenerationAt(root, store.publicName, store.limits)
	if err != nil {
		return Observation{}, err
	}
	result := Observation{Installed: true, Current: current.receipt}
	if top.previousPresent {
		previous, err := readGenerationAt(root, store.previousName, store.limits)
		if err != nil || previous.receipt.GenerationID == current.receipt.GenerationID {
			return Observation{}, errors.Join(ErrResidue, err)
		}
		copy := previous.receipt
		result.Previous = &copy
	}
	if err := requireRootAuthorityPath(store.root); err != nil {
		return Observation{}, err
	}
	return result, nil
}

func cleanupPrevisibility(root int, store *Store, state *transactionState) error {
	var joined error
	if state.stageExists && state.next != nil {
		if err := removeGenerationAt(root, state.stageName, *state.next, store.limits); err != nil {
			joined = errors.Join(joined, err)
		} else {
			state.stageExists = false
		}
	}
	if state.journalExists && joined == nil {
		if err := unlinkExactFileAt(root, store.journalName, state.journalBytes, maxJournalBytes, true); err != nil {
			joined = errors.Join(joined, err)
		} else {
			state.journalExists = false
		}
	}
	return joined
}

func rollbackTransaction(root int, store *Store, state *transactionState) error {
	if state == nil || state.commitExists {
		return errors.New("committed generation cannot use pre-commit rollback")
	}
	if state.newVisible {
		if state.next == nil {
			return errors.New("visible generation has no bound identity")
		}
		if err := renameBoundNoReplace(root, store.publicName, state.stageName, state.next.identity); err != nil {
			return err
		}
		state.newVisible = false
		state.stageExists = true
		if err := unix.Fsync(root); err != nil {
			return err
		}
	}
	if state.oldMoved {
		if state.prior == nil {
			return errors.New("prior generation has no bound identity")
		}
		if err := renameBoundNoReplace(root, store.previousName, store.publicName, state.prior.identity); err != nil {
			return err
		}
		state.oldMoved = false
		if err := unix.Fsync(root); err != nil {
			return err
		}
	}
	if state.retainedMoved {
		if state.retained == nil || state.retainedName == "" {
			return errors.New("retained generation has no bound identity")
		}
		if err := renameBoundNoReplace(root, state.retainedName, store.previousName, state.retained.identity); err != nil {
			return err
		}
		state.retainedMoved = false
		if err := unix.Fsync(root); err != nil {
			return err
		}
	}
	if state.prior != nil {
		if err := requireGenerationAt(root, store.publicName, *state.prior, store.limits); err != nil {
			return err
		}
	}
	if state.retained != nil {
		if err := requireGenerationAt(root, store.previousName, *state.retained, store.limits); err != nil {
			return err
		}
	}
	if state.stageExists {
		if state.next == nil {
			return errors.New("rollback stage has no exact generation record")
		}
		if err := removeGenerationAt(root, state.stageName, *state.next, store.limits); err != nil {
			return err
		}
		state.stageExists = false
		if top, err := inspectTop(root, store); err != nil || state.prior != nil && !top.publicPresent {
			return errors.Join(fmt.Errorf("stage cleanup lost prior public name: %s", debugTopology(top)), err)
		}
	}
	if state.journalExists {
		if err := unlinkExactFileAt(root, store.journalName, state.journalBytes, maxJournalBytes, true); err != nil {
			return err
		}
		state.journalExists = false
	}
	top, err := inspectTop(root, store)
	if err != nil || top.journalPresent || top.stageName != "" || top.retainedName != "" || top.commitName != "" ||
		top.previousPresent != (state.retained != nil) ||
		state.prior != nil && !top.publicPresent || state.prior == nil && top.publicPresent {
		return errors.Join(fmt.Errorf("rollback did not restore the prior topology: %s", debugTopology(top)), err)
	}
	if state.prior != nil {
		if err := requireGenerationAt(root, store.publicName, *state.prior, store.limits); err != nil {
			return err
		}
	}
	if state.retained != nil {
		return requireGenerationAt(root, store.previousName, *state.retained, store.limits)
	}
	return nil
}

func stageGeneration(root int, stageName string, prepared domainartifact.PreparedV1, limits Limits) (result unixGenerationRecord, resultErr error) {
	if err := unix.Mkdirat(root, stageName, domainartifact.DirectoryModeV1); err != nil {
		return unixGenerationRecord{}, err
	}
	if err := unix.Fchmodat(root, stageName, domainartifact.DirectoryModeV1, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		_ = unix.Unlinkat(root, stageName, unix.AT_REMOVEDIR)
		return unixGenerationRecord{}, fmt.Errorf("secure generation stage chmod: %w", err)
	}
	stage, err := openDirectoryAt(root, stageName)
	if err != nil {
		_ = unix.Unlinkat(root, stageName, unix.AT_REMOVEDIR)
		return unixGenerationRecord{}, err
	}
	if err := unix.Fchmod(stage, domainartifact.DirectoryModeV1); err != nil {
		_ = unix.Close(stage)
		return unixGenerationRecord{}, fmt.Errorf("secure generation opened stage chmod: %w", err)
	}
	if err := clearCreatedExtendedSecurity(stage); err != nil {
		_ = unix.Close(stage)
		return unixGenerationRecord{}, fmt.Errorf("secure generation stage extended security: %w", err)
	}
	stat, err := validatePrivateDirectory(stage)
	if err != nil {
		_ = unix.Close(stage)
		return unixGenerationRecord{}, fmt.Errorf("secure generation stage validation: %w", err)
	}
	identity := objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, removePartialGenerationAt(root, stageName, identity))
		}
	}()
	files := prepared.Files()
	directories := generationDirectories(files)
	for _, directory := range directories {
		if err := createDirectoryPath(stage, directory); err != nil {
			_ = unix.Close(stage)
			return unixGenerationRecord{}, fmt.Errorf("secure generation payload directory %q: %w", directory, err)
		}
	}
	for _, file := range files {
		parent, name, err := openPayloadParent(stage, file.Path)
		if err != nil {
			_ = unix.Close(stage)
			return unixGenerationRecord{}, err
		}
		writeErr := writeExclusiveFileAt(parent, name, file.Body, int(limits.MaxFileBytes), file.Mode, true)
		closeErr := unix.Close(parent)
		if writeErr != nil || closeErr != nil {
			_ = unix.Close(stage)
			return unixGenerationRecord{}, fmt.Errorf("secure generation payload %q: %w", file.Path, errors.Join(writeErr, closeErr))
		}
	}
	for _, metadata := range []struct {
		name string
		body []byte
	}{
		{name: domainartifact.InventoryFileNameV1, body: prepared.InventoryBytes()},
		{name: domainartifact.ReceiptFileNameV1, body: prepared.ReceiptBytes()},
	} {
		if err := writeExclusiveFileAt(stage, metadata.name, metadata.body, domainartifact.MaxContractBytesV1, domainartifact.RegularFileModeV1, false); err != nil {
			_ = unix.Close(stage)
			return unixGenerationRecord{}, fmt.Errorf("secure generation metadata %q: %w", metadata.name, err)
		}
	}
	for index := len(directories) - 1; index >= 0; index-- {
		directory, err := openDirectoryPath(stage, directories[index])
		if err != nil {
			_ = unix.Close(stage)
			return unixGenerationRecord{}, err
		}
		syncErr := unix.Fsync(directory)
		closeErr := unix.Close(directory)
		if syncErr != nil || closeErr != nil {
			_ = unix.Close(stage)
			return unixGenerationRecord{}, errors.Join(syncErr, closeErr)
		}
	}
	if err := unix.Fsync(stage); err != nil {
		_ = unix.Close(stage)
		return unixGenerationRecord{}, err
	}
	if err := unix.Close(stage); err != nil {
		return unixGenerationRecord{}, err
	}
	if err := unix.Fsync(root); err != nil {
		return unixGenerationRecord{}, err
	}
	record, err := readGenerationAt(root, stageName, limits)
	if err != nil || record.identity != identity || record.receipt != prepared.Receipt() {
		return unixGenerationRecord{}, errors.Join(errors.New("staged generation readback failed"), err)
	}
	return record, nil
}

func removePartialGenerationAt(root int, name string, expected objectIdentity) error {
	directory, err := openDirectoryAt(root, name)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	stat, err := validatePrivateDirectory(directory)
	if err != nil || (objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}) != expected {
		_ = unix.Close(directory)
		return errors.Join(errors.New("partial generation identity changed before cleanup"), err)
	}
	if err := removeDirectoryContents(directory); err != nil {
		_ = unix.Close(directory)
		return err
	}
	if err := unix.Fsync(directory); err != nil {
		_ = unix.Close(directory)
		return err
	}
	if err := unix.Close(directory); err != nil {
		return err
	}
	current, err := statAtNoFollow(root, name)
	if err != nil || (objectIdentity{dev: uint64(current.Dev), ino: current.Ino}) != expected {
		return errors.Join(errors.New("partial generation path changed before cleanup"), err)
	}
	if err := unix.Unlinkat(root, name, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(root)
}

func readGenerationAt(root int, name string, limits Limits) (unixGenerationRecord, error) {
	directory, err := openDirectoryAt(root, name)
	if err != nil {
		return unixGenerationRecord{}, err
	}
	defer unix.Close(directory)
	stat, err := validatePrivateDirectory(directory)
	if err != nil {
		return unixGenerationRecord{}, err
	}
	inventoryFile, err := readFileAt(directory, domainartifact.InventoryFileNameV1, domainartifact.MaxContractBytesV1, domainartifact.RegularFileModeV1, false)
	if err != nil {
		return unixGenerationRecord{}, err
	}
	receiptFile, err := readFileAt(directory, domainartifact.ReceiptFileNameV1, maxJournalBytes, domainartifact.RegularFileModeV1, false)
	if err != nil {
		return unixGenerationRecord{}, err
	}
	inventory, err := domainartifact.ParseInventoryV1(inventoryFile.body)
	if err != nil || int(inventory.FileCount) > limits.MaxFiles || inventory.TotalBytes > uint64(limits.MaxTotalBytes) {
		return unixGenerationRecord{}, errors.Join(ErrResidue, err)
	}
	receipt, err := domainartifact.ParseReceiptV1(receiptFile.body, inventoryFile.body)
	if err != nil {
		return unixGenerationRecord{}, errors.Join(ErrResidue, err)
	}
	actual := make([]domainartifact.FileInventoryEntryV1, 0, len(inventory.Files))
	actualDirectories := make([]string, 0)
	expectedFiles := make(map[string]domainartifact.FileInventoryEntryV1, len(inventory.Files))
	for _, entry := range inventory.Files {
		expectedFiles[entry.Path] = entry
	}
	seenMetadata := map[string]bool{}
	entryCount := 0
	if err := inspectGenerationDirectory(directory, "", 1, limits, &entryCount, seenMetadata, expectedFiles, &actualDirectories, &actual); err != nil {
		return unixGenerationRecord{}, err
	}
	if !seenMetadata[domainartifact.InventoryFileNameV1] || !seenMetadata[domainartifact.ReceiptFileNameV1] || len(actual) != len(inventory.Files) {
		return unixGenerationRecord{}, ErrResidue
	}
	sort.Slice(actual, func(left, right int) bool { return actual[left].Path < actual[right].Path })
	sort.Strings(actualDirectories)
	expectedDirectories := inventoryDirectorySet(inventory.Files)
	if len(actualDirectories) != len(expectedDirectories) {
		return unixGenerationRecord{}, ErrResidue
	}
	for _, directory := range actualDirectories {
		if _, ok := expectedDirectories[directory]; !ok {
			return unixGenerationRecord{}, ErrResidue
		}
	}
	for index := range actual {
		if actual[index] != inventory.Files[index] || actual[index].Size > uint64(limits.MaxFileBytes) {
			return unixGenerationRecord{}, ErrResidue
		}
	}
	check, err := openDirectoryAt(root, name)
	if err != nil {
		return unixGenerationRecord{}, err
	}
	defer unix.Close(check)
	checkStat, err := validatePrivateDirectory(check)
	if err != nil || uint64(checkStat.Dev) != uint64(stat.Dev) || checkStat.Ino != stat.Ino {
		return unixGenerationRecord{}, errors.Join(errors.New("generation path identity changed during inspection"), err)
	}
	return unixGenerationRecord{
		generationRecord: generationRecord{
			inventory:      inventory,
			receipt:        receipt,
			inventoryBytes: append([]byte(nil), inventoryFile.body...), receiptBytes: append([]byte(nil), receiptFile.body...),
		},
		identity: objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino},
	}, nil
}

func inspectGenerationDirectory(directory int, prefix string, depth int, limits Limits, entryCount *int, metadata map[string]bool, expectedFiles map[string]domainartifact.FileInventoryEntryV1, actualDirectories *[]string, actualFiles *[]domainartifact.FileInventoryEntryV1) error {
	if depth > limits.MaxDepth {
		return ErrResidue
	}
	entries, err := readDirectoryEntries(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		*entryCount++
		if *entryCount > limits.MaxFiles*limits.MaxDepth+2 {
			return ErrResidue
		}
		name := entry.Name()
		relative := name
		if prefix != "" {
			relative = prefix + "/" + name
		}
		if prefix == "" && (name == domainartifact.InventoryFileNameV1 || name == domainartifact.ReceiptFileNameV1) {
			if entry.IsDir() || metadata[name] {
				return ErrResidue
			}
			metadata[name] = true
			continue
		}
		if err := domainartifact.ValidatePayloadPathV1(relative); err != nil {
			return ErrResidue
		}
		stat, err := statAtNoFollow(directory, name)
		if err != nil {
			return err
		}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			*actualDirectories = append(*actualDirectories, relative)
			child, err := openDirectoryAt(directory, name)
			if err != nil {
				return err
			}
			_, validateErr := validatePrivateDirectory(child)
			if validateErr == nil {
				validateErr = inspectGenerationDirectory(child, relative, depth+1, limits, entryCount, metadata, expectedFiles, actualDirectories, actualFiles)
			}
			closeErr := unix.Close(child)
			if validateErr != nil || closeErr != nil {
				return errors.Join(validateErr, closeErr)
			}
		case unix.S_IFREG:
			expected, ok := expectedFiles[relative]
			if !ok || expected.Size > uint64(limits.MaxFileBytes) {
				return ErrResidue
			}
			file, err := readFileAt(directory, name, int(limits.MaxFileBytes), expected.Mode, true)
			if err != nil {
				return err
			}
			*actualFiles = append(*actualFiles, domainartifact.FileInventoryEntryV1{
				Path: relative, Type: expected.Type, Mode: expected.Mode,
				SHA256: domainartifact.DigestBytesV1(file.body), Size: uint64(len(file.body)),
			})
		default:
			return ErrResidue
		}
	}
	return nil
}

func requireGenerationAt(root int, name string, expected unixGenerationRecord, limits Limits) error {
	current, err := readGenerationAt(root, name, limits)
	if err != nil || current.identity != expected.identity || current.receipt != expected.receipt ||
		!bytes.Equal(current.inventoryBytes, expected.inventoryBytes) || !bytes.Equal(current.receiptBytes, expected.receiptBytes) {
		return errors.Join(errors.New("generation identity or contracts changed"), err)
	}
	return nil
}

func removeGenerationAt(root int, name string, expected unixGenerationRecord, limits Limits) error {
	if err := requireGenerationAt(root, name, expected, limits); err != nil {
		return err
	}
	directory, err := openDirectoryAt(root, name)
	if err != nil {
		return err
	}
	stat, err := validatePrivateDirectory(directory)
	if err != nil || (objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}) != expected.identity {
		_ = unix.Close(directory)
		return errors.Join(errors.New("generation changed before removal"), err)
	}
	if err := removeGenerationContentsOrdered(directory, expected); err != nil {
		_ = unix.Close(directory)
		return err
	}
	if err := unix.Fsync(directory); err != nil {
		_ = unix.Close(directory)
		return err
	}
	if err := unix.Close(directory); err != nil {
		return err
	}
	current, err := statAtNoFollow(root, name)
	if err != nil || (objectIdentity{dev: uint64(current.Dev), ino: current.Ino}) != expected.identity {
		return errors.Join(errors.New("generation path changed before removal"), err)
	}
	if err := unix.Unlinkat(root, name, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(root)
}

func resumeRetainedGenerationPrune(root int, store *Store, name string, journal journalV1) error {
	if name == "" || name != journal.RetainedName {
		return ErrResidue
	}
	directory, err := openDirectoryAt(root, name)
	if err != nil {
		return err
	}
	stat, err := validatePrivateDirectory(directory)
	if err != nil {
		_ = unix.Close(directory)
		return err
	}
	identity := objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}

	inventoryFile, inventoryErr := readOptionalFileAt(directory, domainartifact.InventoryFileNameV1, domainartifact.MaxContractBytesV1, domainartifact.RegularFileModeV1)
	receiptFile, receiptErr := readOptionalFileAt(directory, domainartifact.ReceiptFileNameV1, maxJournalBytes, domainartifact.RegularFileModeV1)
	if inventoryErr != nil || receiptErr != nil {
		_ = unix.Close(directory)
		return errors.Join(inventoryErr, receiptErr)
	}
	var inventory *domainartifact.InventoryV1
	if inventoryFile != nil {
		parsed, err := domainartifact.ParseInventoryV1(inventoryFile.body)
		if err != nil || parsed.GenerationID != journal.RetainedGenerationID ||
			domainartifact.DigestBytesV1(inventoryFile.body) != journal.RetainedInventoryDigest {
			_ = unix.Close(directory)
			return errors.Join(ErrResidue, err)
		}
		inventory = &parsed
	}
	if receiptFile != nil {
		if domainartifact.DigestBytesV1(receiptFile.body) != journal.RetainedReceiptDigest {
			_ = unix.Close(directory)
			return ErrResidue
		}
		if inventory != nil {
			parsed, err := domainartifact.ParseReceiptV1(receiptFile.body, inventoryFile.body)
			if err != nil || parsed.GenerationID != journal.RetainedGenerationID {
				_ = unix.Close(directory)
				return errors.Join(ErrResidue, err)
			}
		}
	}

	actual := make([]domainartifact.FileInventoryEntryV1, 0)
	actualDirectories := make([]string, 0)
	expectedFiles := make(map[string]domainartifact.FileInventoryEntryV1)
	if inventory != nil {
		for _, entry := range inventory.Files {
			expectedFiles[entry.Path] = entry
		}
	}
	metadata := map[string]bool{}
	entryCount := 0
	if err := inspectGenerationDirectory(directory, "", 1, store.limits, &entryCount, metadata, expectedFiles, &actualDirectories, &actual); err != nil {
		_ = unix.Close(directory)
		return err
	}
	if inventory == nil && (len(actual) != 0 || len(actualDirectories) != 0) {
		_ = unix.Close(directory)
		return ErrResidue
	}
	if receiptFile == nil && (len(actual) != 0 || len(actualDirectories) != 0) || inventoryFile == nil && receiptFile != nil {
		_ = unix.Close(directory)
		return ErrResidue
	}
	if inventory != nil {
		expected := make(map[string]domainartifact.FileInventoryEntryV1, len(inventory.Files))
		for _, entry := range inventory.Files {
			expected[entry.Path] = entry
		}
		for _, entry := range actual {
			if expected[entry.Path] != entry {
				_ = unix.Close(directory)
				return ErrResidue
			}
		}
		expectedDirectories := inventoryDirectorySet(inventory.Files)
		for _, actualDirectory := range actualDirectories {
			if _, ok := expectedDirectories[actualDirectory]; !ok {
				_ = unix.Close(directory)
				return ErrResidue
			}
		}
	}
	if inventoryFile != nil && !metadata[domainartifact.InventoryFileNameV1] || inventoryFile == nil && metadata[domainartifact.InventoryFileNameV1] ||
		receiptFile != nil && !metadata[domainartifact.ReceiptFileNameV1] || receiptFile == nil && metadata[domainartifact.ReceiptFileNameV1] {
		_ = unix.Close(directory)
		return ErrResidue
	}

	if inventory != nil {
		if err := removeInventoryPayload(directory, *inventory, false); err != nil {
			_ = unix.Close(directory)
			return err
		}
	}
	if err := requireOnlyMetadata(directory, inventoryFile != nil, receiptFile != nil); err != nil {
		_ = unix.Close(directory)
		return err
	}
	if receiptFile != nil {
		if err := unlinkExactFileAt(directory, domainartifact.ReceiptFileNameV1, receiptFile.body, maxJournalBytes, true); err != nil {
			_ = unix.Close(directory)
			return err
		}
	}
	if inventoryFile != nil {
		if err := unlinkExactFileAt(directory, domainartifact.InventoryFileNameV1, inventoryFile.body, domainartifact.MaxContractBytesV1, true); err != nil {
			_ = unix.Close(directory)
			return err
		}
	}
	if err := unix.Fsync(directory); err != nil {
		_ = unix.Close(directory)
		return err
	}
	if err := unix.Close(directory); err != nil {
		return err
	}
	current, err := statAtNoFollow(root, name)
	if err != nil || (objectIdentity{dev: uint64(current.Dev), ino: current.Ino}) != identity {
		return errors.Join(errors.New("partial retained predecessor changed before cleanup"), err)
	}
	if err := unix.Unlinkat(root, name, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return unix.Fsync(root)
}

func removeGenerationContentsOrdered(directory int, expected unixGenerationRecord) error {
	if err := removeInventoryPayload(directory, expected.inventory, true); err != nil {
		return err
	}
	if err := requireOnlyMetadata(directory, true, true); err != nil {
		return err
	}
	if err := unlinkExactFileAt(directory, domainartifact.ReceiptFileNameV1, expected.receiptBytes, maxJournalBytes, true); err != nil {
		return err
	}
	return unlinkExactFileAt(directory, domainartifact.InventoryFileNameV1, expected.inventoryBytes, domainartifact.MaxContractBytesV1, true)
}

func removeInventoryPayload(directory int, inventory domainartifact.InventoryV1, requireAll bool) error {
	for _, entry := range inventory.Files {
		parent, name, err := openPayloadParent(directory, entry.Path)
		if err != nil {
			if !requireAll && errors.Is(err, unix.ENOENT) {
				continue
			}
			return err
		}
		file, readErr := readFileAt(parent, name, int(entry.Size), entry.Mode, true)
		if readErr != nil {
			closeErr := unix.Close(parent)
			if !requireAll && errors.Is(readErr, unix.ENOENT) {
				if closeErr != nil {
					return closeErr
				}
				continue
			}
			return errors.Join(readErr, closeErr)
		}
		if uint64(len(file.body)) != entry.Size || domainartifact.DigestBytesV1(file.body) != entry.SHA256 {
			_ = unix.Close(parent)
			return ErrResidue
		}
		stat, statErr := statAtNoFollow(parent, name)
		if statErr != nil || (objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}) != file.identity {
			_ = unix.Close(parent)
			return errors.Join(errors.New("inventory payload identity changed before unlink"), statErr)
		}
		unlinkErr := unix.Unlinkat(parent, name, 0)
		syncErr := error(nil)
		if unlinkErr == nil {
			syncErr = unix.Fsync(parent)
		}
		closeErr := unix.Close(parent)
		if unlinkErr != nil || syncErr != nil || closeErr != nil {
			return errors.Join(unlinkErr, syncErr, closeErr)
		}
	}

	directories := make([]string, 0)
	for relative := range inventoryDirectorySet(inventory.Files) {
		directories = append(directories, relative)
	}
	sort.Slice(directories, func(left, right int) bool {
		leftDepth := strings.Count(directories[left], "/")
		rightDepth := strings.Count(directories[right], "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return directories[left] > directories[right]
	})
	for _, relative := range directories {
		parent, name, err := openPayloadParent(directory, relative)
		if err != nil {
			if !requireAll && errors.Is(err, unix.ENOENT) {
				continue
			}
			return err
		}
		child, openErr := openDirectoryAt(parent, name)
		if openErr != nil {
			closeErr := unix.Close(parent)
			if !requireAll && errors.Is(openErr, unix.ENOENT) {
				if closeErr != nil {
					return closeErr
				}
				continue
			}
			return errors.Join(openErr, closeErr)
		}
		childStat, validateErr := validatePrivateDirectory(child)
		entries, readErr := readDirectoryEntries(child)
		if validateErr == nil && (readErr != nil || len(entries) != 0) {
			validateErr = errors.Join(ErrResidue, readErr)
		}
		closeChildErr := unix.Close(child)
		if validateErr != nil || closeChildErr != nil {
			_ = unix.Close(parent)
			return errors.Join(validateErr, closeChildErr)
		}
		identity := objectIdentity{dev: uint64(childStat.Dev), ino: childStat.Ino}
		current, statErr := statAtNoFollow(parent, name)
		if statErr != nil || (objectIdentity{dev: uint64(current.Dev), ino: current.Ino}) != identity {
			_ = unix.Close(parent)
			return errors.Join(errors.New("inventory directory identity changed before unlink"), statErr)
		}
		unlinkErr := unix.Unlinkat(parent, name, unix.AT_REMOVEDIR)
		syncErr := error(nil)
		if unlinkErr == nil {
			syncErr = unix.Fsync(parent)
		}
		closeParentErr := unix.Close(parent)
		if unlinkErr != nil || syncErr != nil || closeParentErr != nil {
			return errors.Join(unlinkErr, syncErr, closeParentErr)
		}
	}
	return nil
}

func requireOnlyMetadata(directory int, inventoryPresent, receiptPresent bool) error {
	entries, err := readDirectoryEntries(directory)
	if err != nil {
		return err
	}
	expectedCount := 0
	if inventoryPresent {
		expectedCount++
	}
	if receiptPresent {
		expectedCount++
	}
	if len(entries) != expectedCount {
		return ErrResidue
	}
	for _, entry := range entries {
		switch entry.Name() {
		case domainartifact.InventoryFileNameV1:
			if !inventoryPresent {
				return ErrResidue
			}
		case domainartifact.ReceiptFileNameV1:
			if !receiptPresent {
				return ErrResidue
			}
		default:
			return ErrResidue
		}
	}
	return nil
}

func readOptionalFileAt(parent int, name string, maxBytes int, expectedMode uint32) (*readFile, error) {
	file, err := readFileAt(parent, name, maxBytes, expectedMode, false)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func removeDirectoryContents(directory int) error {
	if _, err := validatePrivateDirectory(directory); err != nil {
		return err
	}
	entries, err := readDirectoryEntries(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		stat, err := statAtNoFollow(directory, name)
		if err != nil {
			return err
		}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFREG:
			mode := uint32(stat.Mode & 0o7777)
			if mode != domainartifact.RegularFileModeV1 && mode != domainartifact.NativeExecutableModeV1 {
				return ErrResidue
			}
			fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			validated, validateErr := validatePrivateFile(fd, stat.Size, mode, true)
			closeErr := unix.Close(fd)
			if validateErr != nil || closeErr != nil || uint64(validated.Dev) != uint64(stat.Dev) || validated.Ino != stat.Ino {
				return errors.Join(ErrResidue, validateErr, closeErr)
			}
			if current, statErr := statAtNoFollow(directory, name); statErr != nil || current.Ino != stat.Ino || uint64(current.Dev) != uint64(stat.Dev) {
				return errors.Join(ErrResidue, statErr)
			}
			if stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
				return ErrResidue
			}
			if err := unix.Unlinkat(directory, name, 0); err != nil {
				return err
			}
		case unix.S_IFDIR:
			child, err := openDirectoryAt(directory, name)
			if err != nil {
				return err
			}
			if _, err := validatePrivateDirectory(child); err != nil {
				_ = unix.Close(child)
				return err
			}
			if err := removeDirectoryContents(child); err != nil {
				_ = unix.Close(child)
				return err
			}
			if err := unix.Fsync(child); err != nil {
				_ = unix.Close(child)
				return err
			}
			if err := unix.Close(child); err != nil {
				return err
			}
			if err := unix.Unlinkat(directory, name, unix.AT_REMOVEDIR); err != nil {
				return err
			}
		default:
			return ErrResidue
		}
		if err := unix.Fsync(directory); err != nil {
			return err
		}
	}
	return nil
}

func inspectTop(root int, store *Store) (topInventory, error) {
	entries, err := readDirectoryEntries(root)
	if err != nil {
		return topInventory{}, err
	}
	if len(entries) > 6 {
		return topInventory{}, ErrResidue
	}
	var top topInventory
	for _, entry := range entries {
		name := entry.Name()
		stat, err := statAtNoFollow(root, name)
		if err != nil {
			return topInventory{}, err
		}
		switch name {
		case store.publicName:
			if stat.Mode&unix.S_IFMT != unix.S_IFDIR || top.publicPresent {
				return topInventory{}, ErrResidue
			}
			top.publicPresent = true
		case store.previousName:
			if stat.Mode&unix.S_IFMT != unix.S_IFDIR || top.previousPresent {
				return topInventory{}, ErrResidue
			}
			top.previousPresent = true
		case store.journalName:
			if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || top.journalPresent {
				return topInventory{}, ErrResidue
			}
			top.journalPresent = true
		default:
			switch {
			case validStageName(store.publicName, name):
				if stat.Mode&unix.S_IFMT != unix.S_IFDIR || top.stageName != "" {
					return topInventory{}, ErrResidue
				}
				top.stageName = name
			case validRetainedName(store.publicName, name):
				if stat.Mode&unix.S_IFMT != unix.S_IFDIR || top.retainedName != "" {
					return topInventory{}, ErrResidue
				}
				top.retainedName = name
			case validCommitName(store.publicName, name):
				if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || top.commitName != "" {
					return topInventory{}, ErrResidue
				}
				top.commitName = name
			default:
				return topInventory{}, ErrResidue
			}
		}
	}
	return top, nil
}

func matchesPrior(record *unixGenerationRecord, journal journalV1) bool {
	return record != nil && record.receipt.GenerationID == journal.PriorGenerationID &&
		domainartifact.DigestBytesV1(record.inventoryBytes) == journal.PriorInventoryDigest &&
		domainartifact.DigestBytesV1(record.receiptBytes) == journal.PriorReceiptDigest
}

func matchesNext(record *unixGenerationRecord, journal journalV1) bool {
	return record != nil && record.receipt.GenerationID == journal.NextGenerationID &&
		domainartifact.DigestBytesV1(record.inventoryBytes) == journal.NextInventoryDigest &&
		domainartifact.DigestBytesV1(record.receiptBytes) == journal.NextReceiptDigest
}

func matchesRetained(record *unixGenerationRecord, journal journalV1) bool {
	return record != nil && record.receipt.GenerationID == journal.RetainedGenerationID &&
		domainartifact.DigestBytesV1(record.inventoryBytes) == journal.RetainedInventoryDigest &&
		domainartifact.DigestBytesV1(record.receiptBytes) == journal.RetainedReceiptDigest
}

func recordPointer(record *unixGenerationRecord) *generationRecord {
	if record == nil {
		return nil
	}
	copy := record.generationRecord
	return &copy
}

func validStageName(publicName, name string) bool {
	prefix := "." + publicName + ".stage-"
	return strings.HasPrefix(name, prefix) && validOperationID(strings.TrimPrefix(name, prefix))
}

func validRetainedName(publicName, name string) bool {
	prefix := "." + publicName + ".third-"
	return strings.HasPrefix(name, prefix) && validOperationID(strings.TrimPrefix(name, prefix))
}

func validCommitName(publicName, name string) bool {
	prefix := "." + publicName + ".commit-"
	suffix := ".v1.json"
	return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) &&
		validOperationID(strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix))
}

func injectFault(store *Store, phase string) (error, bool) {
	if store == nil || store.faults == nil || store.faults.cut == nil {
		return nil, false
	}
	err := store.faults.cut(phase)
	return err, errors.Is(err, errSimulatedCrash)
}

func publicationFsync(store *Store, fd int, phase string) error {
	if store != nil && store.faults != nil && store.faults.fsync != nil {
		return store.faults.fsync(phase, fd)
	}
	return unix.Fsync(fd)
}

func writeExclusiveFileAt(parent int, name string, body []byte, maxBytes int, expectedMode uint32, allowEmpty bool) error {
	if len(body) > maxBytes || !allowEmpty && len(body) == 0 {
		return ErrInvalidInput
	}
	fd, err := unix.Openat(parent, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, expectedMode)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return ErrInvalidInput
	}
	succeeded := false
	defer func() {
		_ = file.Close()
		if !succeeded {
			_ = unix.Unlinkat(parent, name, 0)
		}
	}()
	if err := unix.Fchmod(fd, expectedMode); err != nil {
		return err
	}
	if err := clearCreatedExtendedSecurity(fd); err != nil {
		return err
	}
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil || count <= 0 {
			return errors.Join(errors.New("exclusive generation write was incomplete"), writeErr)
		}
		written += count
	}
	if err := unix.Fchmod(fd, expectedMode); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	readback, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || !bytes.Equal(readback, body) {
		return errors.Join(errors.New("exclusive generation write readback failed"), err)
	}
	stat, err := validatePrivateFile(fd, int64(maxBytes), expectedMode, allowEmpty)
	if err != nil {
		return err
	}
	identity := objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}
	if err := file.Close(); err != nil {
		return err
	}
	check, err := readFileAt(parent, name, maxBytes, expectedMode, allowEmpty)
	if err != nil || check.identity != identity || !bytes.Equal(check.body, body) {
		return errors.Join(errors.New("exclusive generation path readback failed"), err)
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	succeeded = true
	return nil
}

type readFile struct {
	body     []byte
	identity objectIdentity
}

func readFileAt(parent int, name string, maxBytes int, expectedMode uint32, allowEmpty bool) (readFile, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return readFile{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return readFile{}, ErrResidue
	}
	defer file.Close()
	stat, err := validatePrivateFile(fd, int64(maxBytes), expectedMode, allowEmpty)
	if err != nil {
		return readFile{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || int64(len(body)) != stat.Size {
		return readFile{}, errors.Join(ErrResidue, err)
	}
	identity := objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}
	check, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return readFile{}, err
	}
	defer unix.Close(check)
	checkStat, err := validatePrivateFile(check, int64(maxBytes), expectedMode, allowEmpty)
	if err != nil || uint64(checkStat.Dev) != identity.dev || checkStat.Ino != identity.ino || checkStat.Size != stat.Size {
		return readFile{}, errors.Join(errors.New("generation file path changed during read"), err)
	}
	return readFile{body: body, identity: identity}, nil
}

func unlinkExactFileAt(parent int, name string, expected []byte, maxBytes int, syncParent bool) error {
	file, err := readFileAt(parent, name, maxBytes, domainartifact.RegularFileModeV1, false)
	if err != nil || !bytes.Equal(file.body, expected) {
		return errors.Join(errors.New("journal changed before unlink"), err)
	}
	stat, err := statAtNoFollow(parent, name)
	if err != nil || (objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}) != file.identity {
		return errors.Join(errors.New("journal identity changed before unlink"), err)
	}
	if err := unix.Unlinkat(parent, name, 0); err != nil {
		return err
	}
	if syncParent {
		return unix.Fsync(parent)
	}
	return nil
}

func renameBoundNoReplace(parent int, source, target string, expected objectIdentity) error {
	stat, err := statAtNoFollow(parent, source)
	if err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || (objectIdentity{dev: uint64(stat.Dev), ino: stat.Ino}) != expected {
		return errors.Join(errors.New("rename source identity changed"), err)
	}
	if _, err := statAtNoFollow(parent, target); err == nil || !errors.Is(err, unix.ENOENT) {
		return errors.Join(errors.New("rename target is not absent"), err)
	}
	if err := renameNoReplace(parent, source, target); err != nil {
		return err
	}
	destination, err := statAtNoFollow(parent, target)
	if err != nil || (objectIdentity{dev: uint64(destination.Dev), ino: destination.Ino}) != expected {
		return errors.Join(errors.New("rename destination identity changed"), err)
	}
	if _, err := statAtNoFollow(parent, source); !errors.Is(err, unix.ENOENT) {
		return errors.New("rename source name survived")
	}
	return nil
}

func generationDirectories(files []domainartifact.SourceFileV1) []string {
	set := map[string]struct{}{}
	for _, file := range files {
		parts := strings.Split(file.Path, "/")
		for index := 1; index < len(parts); index++ {
			set[strings.Join(parts[:index], "/")] = struct{}{}
		}
	}
	directories := make([]string, 0, len(set))
	for directory := range set {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(left, right int) bool {
		leftDepth := strings.Count(directories[left], "/")
		rightDepth := strings.Count(directories[right], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return directories[left] < directories[right]
	})
	return directories
}

func inventoryDirectorySet(files []domainartifact.FileInventoryEntryV1) map[string]struct{} {
	directories := make(map[string]struct{})
	for _, file := range files {
		parts := strings.Split(file.Path, "/")
		for index := 1; index < len(parts); index++ {
			directories[strings.Join(parts[:index], "/")] = struct{}{}
		}
	}
	return directories
}

func createDirectoryPath(root int, relative string) error {
	parts := strings.Split(relative, "/")
	parent := root
	ownedParent := false
	for index, part := range parts {
		if index == len(parts)-1 {
			if err := unix.Mkdirat(parent, part, domainartifact.DirectoryModeV1); err != nil {
				if ownedParent {
					_ = unix.Close(parent)
				}
				return err
			}
			if err := unix.Fchmodat(parent, part, domainartifact.DirectoryModeV1, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				if ownedParent {
					_ = unix.Close(parent)
				}
				return err
			}
			child, err := openDirectoryAt(parent, part)
			if err == nil {
				err = unix.Fchmod(child, domainartifact.DirectoryModeV1)
			}
			if err == nil {
				err = clearCreatedExtendedSecurity(child)
			}
			if err == nil {
				_, err = validatePrivateDirectory(child)
			}
			if err == nil {
				err = unix.Fsync(child)
			}
			if child >= 0 {
				err = errors.Join(err, unix.Close(child))
			}
			err = errors.Join(err, unix.Fsync(parent))
			if ownedParent {
				err = errors.Join(err, unix.Close(parent))
			}
			return err
		}
		child, err := openDirectoryAt(parent, part)
		if ownedParent {
			err = errors.Join(err, unix.Close(parent))
		}
		if err != nil {
			return err
		}
		parent = child
		ownedParent = true
	}
	return nil
}

func openPayloadParent(root int, relative string) (int, string, error) {
	parts := strings.Split(relative, "/")
	name := parts[len(parts)-1]
	if len(parts) == 1 {
		duplicate, err := duplicateCloseOnExec(root)
		return duplicate, name, err
	}
	parent, err := openDirectoryPath(root, strings.Join(parts[:len(parts)-1], "/"))
	return parent, name, err
}

func openDirectoryPath(root int, relative string) (int, error) {
	current, err := duplicateCloseOnExec(root)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(relative, "/") {
		next, openErr := openDirectoryAt(current, part)
		closeErr := unix.Close(current)
		if openErr != nil || closeErr != nil {
			if next >= 0 {
				_ = unix.Close(next)
			}
			return -1, errors.Join(openErr, closeErr)
		}
		current = next
	}
	return current, nil
}

func duplicateCloseOnExec(fd int) (int, error) {
	return unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
}

func duplicateFileDescriptor(file *os.File) (int, error) {
	if file == nil {
		return -1, ErrInvalidInput
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return -1, err
	}
	duplicate := -1
	var duplicateErr error
	controlErr := raw.Control(func(fd uintptr) {
		duplicate, duplicateErr = duplicateCloseOnExec(int(fd))
	})
	if controlErr != nil || duplicateErr != nil || duplicate < 0 {
		return -1, errors.Join(controlErr, duplicateErr, ErrUnsafeRoot)
	}
	return duplicate, nil
}

func openDirectoryAt(parent int, name string) (int, error) {
	return unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}

func readDirectoryEntries(directory int) ([]os.DirEntry, error) {
	// dup(2) shares the directory stream offset with the authority descriptor.
	// Open an independent file description so repeated inventories never turn
	// an already-read root into a false empty observation.
	duplicate, err := unix.Openat(directory, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "generation-directory")
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, ErrResidue
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	return entries, nil
}

func statAtNoFollow(parent int, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}

func validatePrivateDirectory(fd int) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) ||
		uint32(stat.Mode&0o7777) != domainartifact.DirectoryModeV1 || !privateExtendedSecuritySafe(fd) || !privateFileFlagsSafe(fd) {
		return unix.Stat_t{}, errors.Join(ErrUnsafeRoot, err)
	}
	return stat, nil
}

func validatePrivateFile(fd int, maxBytes int64, expectedMode uint32, allowEmpty bool) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) ||
		uint32(stat.Mode&0o7777) != expectedMode || stat.Size < 0 || !allowEmpty && stat.Size == 0 || stat.Size > maxBytes ||
		!privateExtendedSecuritySafe(fd) || !privateFileFlagsSafe(fd) {
		return unix.Stat_t{}, errors.Join(ErrResidue, err)
	}
	return stat, nil
}

func lockRoot(ctx context.Context, authority rootAuthority) (int, func() error, error) {
	root, err := openAuthorityRoot(authority)
	if err != nil {
		return -1, nil, err
	}
	stat, err := validatePrivateDirectory(root)
	if err != nil || uint64(stat.Dev) != authority.dev || stat.Ino != authority.ino {
		_ = unix.Close(root)
		return -1, nil, errors.Join(ErrUnsafeRoot, err)
	}
	for {
		err = unix.Flock(root, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return root, func() error { return errors.Join(unix.Flock(root, unix.LOCK_UN), unix.Close(root)) }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = unix.Close(root)
			return -1, nil, err
		}
		select {
		case <-contextDone(ctx):
			_ = unix.Close(root)
			return -1, nil, contextError(ctx)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func requireRootAuthorityPath(authority rootAuthority) error {
	root, err := openAuthorityRoot(authority)
	if err != nil {
		return errors.Join(ErrUnsafeRoot, err)
	}
	stat, validateErr := validatePrivateDirectory(root)
	closeErr := unix.Close(root)
	if validateErr != nil || uint64(stat.Dev) != authority.dev || stat.Ino != authority.ino || closeErr != nil {
		return errors.Join(ErrUnsafeRoot, validateErr, closeErr)
	}
	return nil
}

func openAuthorityRoot(authority rootAuthority) (int, error) {
	if authority.parentFD >= 0 {
		return openDirectoryAt(authority.parentFD, authority.rootName)
	}
	root, _, err := openAbsoluteDirectory(authority.path, false)
	return root, err
}

func canonicalRootPath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || value != trimmed {
		return "", ErrInvalidInput
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if info, err := os.Lstat(absolute); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", ErrUnsafeRoot
	}
	current := absolute
	missing := make([]string, 0)
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", ErrUnsafeRoot
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	// A publication request names one lexical root. Resolving a caller-owned
	// ancestor symlink would silently retarget that request between the caller's
	// CAS/lock check and this authority. The only exception is an immutable
	// root-owned system alias under a root-owned, non-writable parent (for
	// example macOS /var -> /private/var); it is resolved before the O_NOFOLLOW
	// authority walk below.
	for ancestor := current; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if err != nil || info.Mode()&os.ModeSymlink != 0 && !trustedSystemAncestorSymlink(ancestor) {
			return "", errors.Join(ErrUnsafeRoot, err)
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for ancestor := resolved; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.Join(ErrUnsafeRoot, err)
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	for index := len(missing) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, missing[index])
	}
	return filepath.Clean(resolved), nil
}

func trustedSystemAncestorSymlink(path string) bool {
	var link unix.Stat_t
	if unix.Lstat(path, &link) != nil || link.Mode&unix.S_IFMT != unix.S_IFLNK || link.Uid != 0 {
		return false
	}
	var parent unix.Stat_t
	if unix.Stat(filepath.Dir(path), &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR ||
		parent.Uid != 0 || parent.Mode&0o022 != 0 {
		return false
	}
	return true
}

func openAbsoluteDirectory(path string, create bool) (int, bool, error) {
	if !filepath.IsAbs(path) {
		return -1, false, ErrUnsafeRoot
	}
	current, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, false, err
	}
	components := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	if len(components) == 1 && components[0] == "" {
		return current, false, nil
	}
	createdFinal := false
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = unix.Close(current)
			return -1, false, ErrUnsafeRoot
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) && create {
			mkdirErr := unix.Mkdirat(current, component, domainartifact.DirectoryModeV1)
			if mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(current)
				return -1, false, mkdirErr
			}
			if mkdirErr == nil {
				if chmodErr := unix.Fchmodat(current, component, domainartifact.DirectoryModeV1, unix.AT_SYMLINK_NOFOLLOW); chmodErr != nil {
					_ = unix.Close(current)
					return -1, false, chmodErr
				}
			}
			createdFinal = mkdirErr == nil && index == len(components)-1
			if syncErr := unix.Fsync(current); syncErr != nil {
				_ = unix.Close(current)
				return -1, false, syncErr
			}
			next, openErr = unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		closeErr := unix.Close(current)
		if openErr != nil || closeErr != nil {
			if next >= 0 {
				_ = unix.Close(next)
			}
			return -1, false, errors.Join(openErr, closeErr)
		}
		current = next
	}
	return current, createdFinal, nil
}

func debugTopology(top topInventory) string {
	return fmt.Sprintf("public=%t previous=%t journal=%t stage=%q retained=%q commit=%q", top.publicPresent, top.previousPresent, top.journalPresent, top.stageName, top.retainedName, top.commitName)
}
