//go:build darwin || linux

package finalauthority

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"
	"time"

	"golang.org/x/sys/unix"
)

type secureMutableProjectionUnixIdentity struct {
	dev uint64
	ino uint64
}

type secureMutableProjectionUnixResidue struct {
	SecureMutableProjectionResidue
	identity secureMutableProjectionUnixIdentity
}

func openSecureMutableProjection(root, name string, maxBytes int) (privateRootAuthority, error) {
	if !securePrivateNamedComponent(name) {
		return privateRootAuthority{}, errors.New("secure mutable projection file name is invalid")
	}
	authority, err := capturePrivateRootAuthority(root)
	if err != nil {
		return privateRootAuthority{}, err
	}
	fd, release, err := secureMutableProjectionUnixLock(context.Background(), authority)
	if err != nil {
		return privateRootAuthority{}, err
	}
	if err := secureMutableProjectionUnixValidateRoot(fd); err != nil {
		_ = release()
		return privateRootAuthority{}, err
	}
	_, _, inspectErr := secureMutableProjectionUnixInspectAt(fd, name, maxBytes)
	releaseErr := release()
	if inspectErr != nil || releaseErr != nil {
		return privateRootAuthority{}, errors.Join(inspectErr, releaseErr)
	}
	return authority, nil
}

func observeSecureMutableProjection(ctx context.Context, authority privateRootAuthority, name string, maxBytes int) (SecureMutableProjectionObservation, error) {
	if err := mutableProjectionContextError(ctx); err != nil {
		return SecureMutableProjectionObservation{}, err
	}
	root, release, err := secureMutableProjectionUnixLock(ctx, authority)
	if err != nil {
		return SecureMutableProjectionObservation{}, err
	}
	observation, residues, observeErr := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
	releaseErr := release()
	if observeErr != nil || releaseErr != nil {
		return SecureMutableProjectionObservation{}, errors.Join(observeErr, releaseErr)
	}
	if len(residues) > 0 {
		return observation, secureMutableProjectionUnixResidueError(observation, residues)
	}
	return observation, nil
}

func replaceSecureMutableProjection(
	ctx context.Context,
	authority privateRootAuthority,
	name string,
	maxBytes int,
	expectedDigest string,
	next []byte,
	faults *secureMutableProjectionFaults,
) (SecureMutableProjectionReplaceResult, error) {
	nextDigest := secureMutableProjectionDigest(next)
	notCommitted := SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}
	indeterminate := SecureMutableProjectionReplaceResult{State: SecureMutableProjectionIndeterminate, Digest: nextDigest}
	committed := SecureMutableProjectionReplaceResult{State: SecureMutableProjectionCommitted, Digest: nextDigest}
	if err := mutableProjectionContextError(ctx); err != nil {
		return notCommitted, err
	}
	root, release, err := secureMutableProjectionUnixLock(ctx, authority)
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
	if err := secureMutableProjectionUnixValidateRoot(root); err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	current, residues, err := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
	if err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	if len(residues) > 0 {
		return notCommitted, errors.Join(secureMutableProjectionUnixResidueError(current, residues), releaseOnce())
	}
	if !secureMutableProjectionExpected(current, expectedDigest) {
		return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, releaseOnce())
	}
	temporary, stagedIdentity, err := secureMutableProjectionUnixStage(root, name, next, maxBytes)
	if err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	tempExists := true
	cleanupTemp := func() error {
		if !tempExists {
			return nil
		}
		tempExists = false
		if err := unix.Unlinkat(root, temporary, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
		return unix.Fsync(root)
	}
	defer cleanupTemp()
	if faults != nil && faults.BeforeExpectedRecheck != nil {
		faults.BeforeExpectedRecheck()
	}
	if err := mutableProjectionContextError(ctx); err != nil {
		return notCommitted, errors.Join(err, cleanupTemp(), releaseOnce())
	}
	current, residues, err = secureMutableProjectionUnixInspectAt(root, name, maxBytes)
	if err != nil {
		return notCommitted, errors.Join(err, cleanupTemp(), releaseOnce())
	}
	if len(residues) != 1 || residues[0].Name != temporary || residues[0].Digest != nextDigest || residues[0].identity != stagedIdentity {
		return notCommitted, errors.Join(errors.New("secure mutable projection staged inventory changed"), cleanupTemp(), releaseOnce())
	}
	if !secureMutableProjectionExpected(current, expectedDigest) {
		return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, cleanupTemp(), releaseOnce())
	}
	if faults != nil && faults.BeforeAtomicReplace != nil {
		faults.BeforeAtomicReplace()
	}
	if err := secureMutableProjectionUnixValidateRoot(root); err != nil {
		return notCommitted, errors.Join(err, cleanupTemp(), releaseOnce())
	}
	if current.Present {
		err = secureMutableProjectionExchange(root, temporary, name)
	} else {
		err = securePrivateCommitNoReplace(root, temporary, name)
	}
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			err = ErrSecureMutableProjectionCompareFailed
		}
		return notCommitted, errors.Join(err, cleanupTemp(), releaseOnce())
	}
	// From here onward the target name has referred to nextBytes. Any failure
	// must report committed or indeterminate, never not_committed.
	if current.Present {
		old, oldIdentity, oldErr := secureMutableProjectionUnixReadAt(root, temporary, maxBytes)
		if oldErr != nil || secureMutableProjectionDigest(old) != expectedDigest {
			rollbackErr := secureMutableProjectionExchange(root, temporary, name)
			if rollbackErr == nil {
				rollbackErr = unix.Fsync(root)
			}
			if rollbackErr == nil {
				rolledBack, observeErr := secureMutableProjectionUnixObserveTargetAt(root, name, maxBytes)
				if observeErr != nil || rolledBack.Digest != secureMutableProjectionDigest(old) || oldIdentity == stagedIdentity {
					rollbackErr = errors.Join(observeErr, errors.New("secure mutable projection rollback verification failed"))
				}
			}
			if rollbackErr == nil {
				return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, oldErr, cleanupTemp(), releaseOnce())
			}
			return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, oldErr, rollbackErr, releaseOnce())
		}
	}
	tempExists = current.Present // after exchange, the old projection remains at the temp name
	if err := secureMutableProjectionUnixValidateRoot(root); err != nil {
		tempExists = false
		return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, err, releaseOnce())
	}
	if faults != nil && faults.AfterAtomicReplace != nil {
		if faultErr := faults.AfterAtomicReplace(); faultErr != nil {
			// Preserve any exchanged-out predecessor as crash residue. A later
			// observation exposes it for external-witness reconciliation;
			// cleaning it here would silently choose a local winner.
			tempExists = false
			return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, faultErr, releaseOnce())
		}
	}
	if err := unix.Fsync(root); err != nil {
		tempExists = false
		return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, err, releaseOnce())
	}
	if faults != nil && faults.AfterDirectorySync != nil {
		if faultErr := faults.AfterDirectorySync(); faultErr != nil {
			// The replacement is already durable. Preserve committed in the
			// result while still surfacing the injected post-commit failure.
			return committed, errors.Join(faultErr, cleanupTemp(), releaseOnce())
		}
	}
	if faults != nil && faults.BeforeReadback != nil {
		faults.BeforeReadback()
	}
	written, identity, readErr := secureMutableProjectionUnixReadAt(root, name, maxBytes)
	if readErr != nil || identity != stagedIdentity || !bytes.Equal(written, next) {
		tempExists = false
		return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, readErr, errors.New("secure mutable projection readback verification failed"), releaseOnce())
	}
	if faults != nil && faults.BeforeCleanup != nil {
		if faultErr := faults.BeforeCleanup(); faultErr != nil {
			// The target is durable and read back exactly. Preserve the valid
			// predecessor residue so Observe can quarantine it and an external
			// verifier can reconcile explicitly.
			tempExists = false
			return committed, errors.Join(faultErr, releaseOnce())
		}
	}
	cleanupErr := cleanupTemp()
	rootErr := secureMutableProjectionUnixValidateRoot(root)
	releaseErr := releaseOnce()
	if contextErr := mutableProjectionContextError(ctx); contextErr != nil {
		return committed, errors.Join(contextErr, cleanupErr, rootErr, releaseErr)
	}
	return committed, errors.Join(cleanupErr, rootErr, releaseErr)
}

func reconcileSecureMutableProjection(
	ctx context.Context,
	authority privateRootAuthority,
	name string,
	maxBytes int,
	expectedTargetDigest string,
	verifiedProjectionDigest string,
	faults *secureMutableProjectionFaults,
) (SecureMutableProjectionReplaceResult, error) {
	notCommitted := SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}
	committed := SecureMutableProjectionReplaceResult{State: SecureMutableProjectionCommitted, Digest: verifiedProjectionDigest}
	indeterminate := SecureMutableProjectionReplaceResult{State: SecureMutableProjectionIndeterminate, Digest: verifiedProjectionDigest}
	if err := mutableProjectionContextError(ctx); err != nil {
		return notCommitted, err
	}
	root, release, err := secureMutableProjectionUnixLock(ctx, authority)
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
	target, residues, err := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
	if err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	if !secureMutableProjectionExpected(target, expectedTargetDigest) {
		return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, releaseOnce())
	}
	if len(residues) == 0 {
		if secureMutableProjectionExpected(target, verifiedProjectionDigest) {
			return committed, releaseOnce()
		}
		return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, releaseOnce())
	}
	if verifiedProjectionDigest == "" {
		if target.Present {
			return notCommitted, errors.Join(secureMutableProjectionUnixResidueError(target, residues), ErrSecureMutableProjectionCompareFailed, releaseOnce())
		}
		if faults != nil && faults.BeforeReconcileCleanup != nil {
			faults.BeforeReconcileCleanup()
		}
		current, _, currentErr := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
		if currentErr != nil || current.Present || secureMutableProjectionUnixValidateRoot(root) != nil {
			return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, currentErr, releaseOnce())
		}
		if err := secureMutableProjectionUnixRemoveResidues(root, residues); err != nil {
			return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, err, releaseOnce())
		}
		verifiedTarget, verifiedResidues, verifyErr := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
		if verifyErr != nil || verifiedTarget.Present || len(verifiedResidues) != 0 {
			return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, verifyErr, errors.New("secure mutable projection absent reconciliation verification failed"), releaseOnce())
		}
		return committed, errors.Join(secureMutableProjectionUnixValidateRoot(root), releaseOnce())
	}
	if target.Present && target.Digest == verifiedProjectionDigest {
		if faults != nil && faults.BeforeReconcileCleanup != nil {
			faults.BeforeReconcileCleanup()
		}
		currentBody, _, currentErr := secureMutableProjectionUnixReadAt(root, name, maxBytes)
		if currentErr != nil || !bytes.Equal(currentBody, target.Body) || secureMutableProjectionUnixValidateRoot(root) != nil {
			return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, currentErr, releaseOnce())
		}
		if err := secureMutableProjectionUnixRemoveResidues(root, residues); err != nil {
			return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, err, releaseOnce())
		}
		verifiedTarget, verifiedResidues, verifyErr := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
		if verifyErr != nil || verifiedTarget.Digest != verifiedProjectionDigest || len(verifiedResidues) != 0 {
			return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, verifyErr, errors.New("secure mutable projection witness reconciliation verification failed"), releaseOnce())
		}
		return committed, errors.Join(secureMutableProjectionUnixValidateRoot(root), releaseOnce())
	}
	candidateIndex := -1
	for index := range residues {
		// ReplaceExact never stages an empty projection. A zero-length crash
		// fragment may be discarded when the external witness selects target
		// or absence, but it can never become a valid target itself.
		if len(residues[index].Body) == 0 || residues[index].Digest != verifiedProjectionDigest {
			continue
		}
		if candidateIndex >= 0 {
			return notCommitted, errors.Join(secureMutableProjectionUnixResidueError(target, residues), ErrSecureMutableProjectionCompareFailed, errors.New("secure mutable projection witness matches multiple residue files"), releaseOnce())
		}
		candidateIndex = index
	}
	if candidateIndex < 0 {
		return notCommitted, errors.Join(secureMutableProjectionUnixResidueError(target, residues), ErrSecureMutableProjectionCompareFailed, releaseOnce())
	}
	candidate := residues[candidateIndex]
	var expectedTargetBody []byte
	var expectedTargetIdentity secureMutableProjectionUnixIdentity
	if target.Present {
		expectedTargetBody, expectedTargetIdentity, err = secureMutableProjectionUnixReadAt(root, name, maxBytes)
		if err != nil || secureMutableProjectionDigest(expectedTargetBody) != expectedTargetDigest || !bytes.Equal(expectedTargetBody, target.Body) {
			return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, err, releaseOnce())
		}
	}
	if faults != nil && faults.BeforeReconcileExchange != nil {
		faults.BeforeReconcileExchange()
	}
	if err := secureMutableProjectionUnixValidateRoot(root); err != nil {
		return notCommitted, errors.Join(err, releaseOnce())
	}
	if target.Present {
		err = secureMutableProjectionExchange(root, candidate.Name, name)
	} else {
		err = securePrivateCommitNoReplace(root, candidate.Name, name)
	}
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			err = ErrSecureMutableProjectionCompareFailed
		}
		return notCommitted, errors.Join(err, releaseOnce())
	}
	if target.Present {
		displaced, displacedIdentity, displacedErr := secureMutableProjectionUnixReadAt(root, candidate.Name, maxBytes)
		if displacedErr != nil || displacedIdentity != expectedTargetIdentity || !bytes.Equal(displaced, expectedTargetBody) {
			rollbackErr := secureMutableProjectionExchange(root, candidate.Name, name)
			if rollbackErr == nil {
				rollbackErr = unix.Fsync(root)
			}
			if rollbackErr == nil {
				restored, restoreErr := secureMutableProjectionUnixObserveTargetAt(root, name, maxBytes)
				if restoreErr != nil || !restored.Present || restored.Digest != secureMutableProjectionDigest(displaced) {
					rollbackErr = errors.Join(restoreErr, errors.New("secure mutable projection reconciliation rollback verification failed"))
				}
			}
			if rollbackErr == nil {
				return notCommitted, errors.Join(ErrSecureMutableProjectionCompareFailed, displacedErr, releaseOnce())
			}
			return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, displacedErr, rollbackErr, releaseOnce())
		}
	}
	if err := secureMutableProjectionUnixValidateRoot(root); err != nil {
		return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, err, releaseOnce())
	}
	if err := unix.Fsync(root); err != nil {
		return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, err, releaseOnce())
	}
	written, identity, readErr := secureMutableProjectionUnixReadAt(root, name, maxBytes)
	if readErr != nil || identity != candidate.identity || secureMutableProjectionDigest(written) != verifiedProjectionDigest {
		return indeterminate, errors.Join(ErrSecureMutableProjectionCommitIndeterminate, readErr, errors.New("secure mutable projection reconciled target readback failed"), releaseOnce())
	}
	_, remaining, inspectErr := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
	if inspectErr != nil {
		return committed, errors.Join(inspectErr, releaseOnce())
	}
	if err := secureMutableProjectionUnixRemoveResidues(root, remaining); err != nil {
		return committed, errors.Join(err, secureMutableProjectionUnixResidueError(SecureMutableProjectionObservation{Present: true, Digest: verifiedProjectionDigest, Body: written}, remaining), releaseOnce())
	}
	verifiedTarget, verifiedResidues, verifyErr := secureMutableProjectionUnixInspectAt(root, name, maxBytes)
	if verifyErr != nil || verifiedTarget.Digest != verifiedProjectionDigest || len(verifiedResidues) != 0 {
		return committed, errors.Join(verifyErr, errors.New("secure mutable projection reconciliation cleanup verification failed"), releaseOnce())
	}
	return committed, errors.Join(secureMutableProjectionUnixValidateRoot(root), releaseOnce())
}

func secureMutableProjectionUnixLock(ctx context.Context, authority privateRootAuthority) (int, func() error, error) {
	root, err := authority.open()
	if err != nil {
		return -1, nil, err
	}
	if err := secureMutableProjectionUnixValidateRoot(root); err != nil {
		_ = unix.Close(root)
		return -1, nil, err
	}
	// The lock is attached to the already identity-pinned directory object,
	// not to a replaceable lock-file path. Darwin and Linux local filesystems
	// implement flock across processes for such descriptors; a filesystem that
	// rejects the operation fails closed here. Subprocess tests exercise the
	// one-winner property on the current release host.
	for {
		err = unix.Flock(root, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return root, func() error {
				return errors.Join(unix.Flock(root, unix.LOCK_UN), unix.Close(root))
			}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = unix.Close(root)
			return -1, nil, err
		}
		select {
		case <-mutableProjectionContextDone(ctx):
			_ = unix.Close(root)
			return -1, nil, mutableProjectionContextError(ctx)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func secureMutableProjectionUnixValidateRoot(root int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 || stat.Uid != uint32(os.Geteuid()) ||
		!existingPrivateAuthorityExtendedSecuritySafe(root) {
		return errors.New("secure mutable projection root ownership or permissions are unsafe")
	}
	return nil
}

func secureMutableProjectionUnixInspectAt(root int, name string, maxBytes int) (SecureMutableProjectionObservation, []secureMutableProjectionUnixResidue, error) {
	entries, err := securePrivateReadDir(root)
	if err != nil {
		return SecureMutableProjectionObservation{}, nil, err
	}
	var target SecureMutableProjectionObservation
	residues := make([]secureMutableProjectionUnixResidue, 0, 1)
	for _, entry := range entries {
		if entry.Name() == name {
			if entry.IsDir() {
				return SecureMutableProjectionObservation{}, nil, errors.New("secure mutable projection target is not a file")
			}
			body, _, err := secureMutableProjectionUnixReadAt(root, name, maxBytes)
			if err != nil {
				return SecureMutableProjectionObservation{}, nil, err
			}
			target = SecureMutableProjectionObservation{Present: true, Digest: secureMutableProjectionDigest(body), Body: body}
			continue
		}
		if entry.IsDir() || !secureMutableProjectionUnixTempName(name, entry.Name()) {
			return SecureMutableProjectionObservation{}, nil, errors.New("secure mutable projection root contains unknown residue")
		}
		body, identity, err := secureMutableProjectionUnixReadBoundedAt(root, entry.Name(), maxBytes, true)
		if err != nil {
			return SecureMutableProjectionObservation{}, nil, err
		}
		residues = append(residues, secureMutableProjectionUnixResidue{
			SecureMutableProjectionResidue: SecureMutableProjectionResidue{Name: entry.Name(), Digest: secureMutableProjectionDigest(body), Body: body},
			identity:                       identity,
		})
	}
	sort.Slice(residues, func(left, right int) bool { return residues[left].Name < residues[right].Name })
	return target, residues, nil
}

func secureMutableProjectionUnixResidueError(target SecureMutableProjectionObservation, residues []secureMutableProjectionUnixResidue) error {
	public := make([]SecureMutableProjectionResidue, len(residues))
	for index := range residues {
		public[index] = SecureMutableProjectionResidue{
			Name:   residues[index].Name,
			Digest: residues[index].Digest,
			Body:   append([]byte(nil), residues[index].Body...),
		}
	}
	target.Body = append([]byte(nil), target.Body...)
	return &SecureMutableProjectionResidueError{Target: target, Residues: public}
}

func secureMutableProjectionUnixRemoveResidues(root int, residues []secureMutableProjectionUnixResidue) error {
	for index := range residues {
		body, identity, err := secureMutableProjectionUnixReadBoundedAt(root, residues[index].Name, len(residues[index].Body), true)
		if err != nil || identity != residues[index].identity || !bytes.Equal(body, residues[index].Body) {
			return errors.Join(err, errors.New("secure mutable projection residue identity changed before reconciliation"))
		}
	}
	for index := range residues {
		if err := unix.Unlinkat(root, residues[index].Name, 0); err != nil {
			return err
		}
	}
	return unix.Fsync(root)
}

func secureMutableProjectionUnixTempName(name, candidate string) bool {
	prefix := "." + name + "-"
	const suffix = ".tmp"
	if len(candidate) != len(prefix)+24+len(suffix) || candidate[:len(prefix)] != prefix || candidate[len(candidate)-len(suffix):] != suffix {
		return false
	}
	random := candidate[len(prefix) : len(candidate)-len(suffix)]
	for _, char := range random {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func secureMutableProjectionUnixObserveTargetAt(root int, name string, maxBytes int) (SecureMutableProjectionObservation, error) {
	body, _, err := secureMutableProjectionUnixReadAt(root, name, maxBytes)
	if errors.Is(err, os.ErrNotExist) {
		return SecureMutableProjectionObservation{}, nil
	}
	if err != nil {
		return SecureMutableProjectionObservation{}, err
	}
	return SecureMutableProjectionObservation{Present: true, Digest: secureMutableProjectionDigest(body), Body: body}, nil
}

func secureMutableProjectionUnixReadAt(root int, name string, maxBytes int) ([]byte, secureMutableProjectionUnixIdentity, error) {
	return secureMutableProjectionUnixReadBoundedAt(root, name, maxBytes, false)
}

func secureMutableProjectionUnixReadBoundedAt(root int, name string, maxBytes int, allowEmpty bool) ([]byte, secureMutableProjectionUnixIdentity, error) {
	fd, err := unix.Openat(root, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, secureMutableProjectionUnixIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return nil, secureMutableProjectionUnixIdentity{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, secureMutableProjectionUnixIdentity{}, errors.New("secure mutable projection file handle is invalid")
	}
	defer file.Close()
	stat, err := secureMutableProjectionUnixValidateFile(fd, int64(maxBytes), allowEmpty)
	if err != nil {
		return nil, secureMutableProjectionUnixIdentity{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || int64(len(body)) != stat.Size {
		return nil, secureMutableProjectionUnixIdentity{}, errors.New("secure mutable projection exact read failed")
	}
	identity := secureMutableProjectionUnixIdentity{dev: uint64(stat.Dev), ino: stat.Ino}
	check, err := unix.Openat(root, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, secureMutableProjectionUnixIdentity{}, errors.New("secure mutable projection path changed during read")
	}
	defer unix.Close(check)
	checkStat, err := secureMutableProjectionUnixValidateFile(check, int64(maxBytes), allowEmpty)
	if err != nil || uint64(checkStat.Dev) != identity.dev || checkStat.Ino != identity.ino || checkStat.Size != stat.Size {
		return nil, secureMutableProjectionUnixIdentity{}, errors.New("secure mutable projection path identity changed during read")
	}
	return body, identity, nil
}

func secureMutableProjectionUnixValidateFile(fd int, maxBytes int64, allowEmpty bool) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || !privateStatRegular(stat) || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) || stat.Size < 0 ||
		!allowEmpty && stat.Size == 0 || stat.Size > maxBytes || !existingPrivateAuthorityExtendedSecuritySafe(fd) {
		return unix.Stat_t{}, errors.New("secure mutable projection file ownership, mode, link count, or size is unsafe")
	}
	return stat, nil
}

func secureMutableProjectionUnixStage(root int, name string, body []byte, maxBytes int) (string, secureMutableProjectionUnixIdentity, error) {
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return "", secureMutableProjectionUnixIdentity{}, err
	}
	temporary := "." + name + "-" + hex.EncodeToString(suffix) + ".tmp"
	fd, err := unix.Openat(root, temporary, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return "", secureMutableProjectionUnixIdentity{}, err
	}
	file := os.NewFile(uintptr(fd), temporary)
	if file == nil {
		_ = unix.Close(fd)
		return "", secureMutableProjectionUnixIdentity{}, errors.New("secure mutable projection temp handle is invalid")
	}
	succeeded := false
	defer func() {
		_ = file.Close()
		if !succeeded {
			_ = unix.Unlinkat(root, temporary, 0)
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return "", secureMutableProjectionUnixIdentity{}, writeErr
		}
		if count <= 0 {
			return "", secureMutableProjectionUnixIdentity{}, errors.New("secure mutable projection temp write was incomplete")
		}
		written += count
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return "", secureMutableProjectionUnixIdentity{}, err
	}
	if err := file.Sync(); err != nil {
		return "", secureMutableProjectionUnixIdentity{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", secureMutableProjectionUnixIdentity{}, err
	}
	staged, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || !bytes.Equal(staged, body) {
		return "", secureMutableProjectionUnixIdentity{}, errors.New("secure mutable projection staged write verification failed")
	}
	stat, err := secureMutableProjectionUnixValidateFile(fd, int64(maxBytes), false)
	if err != nil {
		return "", secureMutableProjectionUnixIdentity{}, err
	}
	succeeded = true
	return temporary, secureMutableProjectionUnixIdentity{dev: uint64(stat.Dev), ino: stat.Ino}, nil
}

func secureMutableProjectionExpected(current SecureMutableProjectionObservation, expectedDigest string) bool {
	if expectedDigest == "" {
		return !current.Present
	}
	return current.Present && current.Digest == expectedDigest
}

func secureMutableProjectionDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func mutableProjectionContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func mutableProjectionContextDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return make(chan struct{})
	}
	return ctx.Done()
}
