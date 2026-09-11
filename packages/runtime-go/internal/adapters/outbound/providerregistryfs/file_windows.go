//go:build windows

package providerregistryfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

const (
	registryLockShareMode       = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
	registryFileReadAccess      = windows.GENERIC_READ
	registryDirectorySyncAccess = windows.GENERIC_WRITE
)

var (
	journalPriorPresent  = []byte("analytix-provider-registry-commit:v1:prior-present\n")
	journalPriorAbsent   = []byte("analytix-provider-registry-commit:v1:prior-absent\n")
	rollbackPriorPresent = []byte("analytix-provider-registry-commit:v1:rollback-prior-present\n")
	rollbackPriorAbsent  = []byte("analytix-provider-registry-commit:v1:rollback-prior-absent\n")
	commitMarker         = []byte("analytix-provider-registry-commit:v1:committed\n")
)

type windowsRegistryLock struct {
	handle     windows.Handle
	overlapped windows.Overlapped
}

func (lock *windowsRegistryLock) Close() error {
	if lock == nil || lock.handle == 0 || lock.handle == windows.InvalidHandle {
		return nil
	}
	unlockErr := windows.UnlockFileEx(lock.handle, 0, 1, 0, &lock.overlapped)
	closeErr := windows.CloseHandle(lock.handle)
	lock.handle = 0
	return errors.Join(unlockErr, closeErr)
}

func acquireRegistryLock(ctx context.Context, path string) (*windowsRegistryLock, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		registryLockShareMode,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	var information windows.ByHandleFileInformation
	fileType, typeErr := windows.GetFileType(handle)
	infoErr := windows.GetFileInformationByHandle(handle, &information)
	if typeErr != nil || infoErr != nil || validateWindowsRegistryFile(fileType, information, 1<<20) != nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("provider registry lock invalid")
	}
	lock := &windowsRegistryLock{handle: handle}
	for {
		err := windows.LockFileEx(
			handle,
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0, &lock.overlapped,
		)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) && !errors.Is(err, windows.ERROR_IO_PENDING) {
			_ = windows.CloseHandle(handle)
			return nil, err
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = windows.CloseHandle(handle)
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func ensureRegistryDirectory(dataDir, directory string) error {
	if err := validateWindowsDirectory(dataDir); err != nil {
		return err
	}
	privateDirectory := filepath.Join(dataDir, "private")
	if err := ensureWindowsDirectory(privateDirectory); err != nil {
		return err
	}
	if filepath.Dir(directory) != privateDirectory {
		return errors.New("provider registry directory invalid")
	}
	return ensureWindowsDirectory(directory)
}

func ensureWindowsDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return validateWindowsDirectory(path)
}

func validateWindowsDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("provider registry directory invalid")
	}
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(pathUTF16)
	if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("provider registry directory invalid")
	}
	return nil
}

func readPrivateRegistryFile(path string, maximum int) ([]byte, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathUTF16, registryFileReadAccess,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), filepath.Base(path))
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("provider registry file unavailable")
	}
	defer file.Close()
	fileType, err := windows.GetFileType(handle)
	if err != nil {
		return nil, err
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return nil, err
	}
	if validateWindowsRegistryFile(fileType, information, maximum) != nil {
		return nil, errors.New("provider registry file invalid")
	}
	content, err := io.ReadAll(io.LimitReader(file, int64(maximum)+1))
	if err != nil {
		clear(content)
		return nil, err
	}
	if len(content) > maximum {
		clear(content)
		return nil, errors.New("provider registry file oversized")
	}
	return content, nil
}

func validateWindowsRegistryFile(fileType uint32, information windows.ByHandleFileInformation, maximum int) error {
	invalidAttributes := uint32(windows.FILE_ATTRIBUTE_REPARSE_POINT | windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_DEVICE)
	size := uint64(information.FileSizeHigh)<<32 | uint64(information.FileSizeLow)
	if maximum < 0 || fileType != windows.FILE_TYPE_DISK || information.FileAttributes&invalidAttributes != 0 || size > uint64(maximum) {
		return errors.New("provider registry file invalid")
	}
	return nil
}

func writePrivateRegistryAtomically(path string, content []byte, hooks *commitHooks) (resultErr error) {
	if err := recoverAtomicCommit(path); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	journalPath, backupPath, markerPath := recoveryPaths(path)
	prior, priorExists, err := readOptionalPrivateFile(path, 32<<20)
	if err != nil {
		return err
	}
	defer clear(prior)
	if priorExists {
		if err := createExclusivePrivateFile(backupPath, prior); err != nil {
			return err
		}
	}
	journal := journalPriorAbsent
	if priorExists {
		journal = journalPriorPresent
	}
	if err := createExclusivePrivateFile(journalPath, journal); err != nil {
		_ = removePrivateFileIfPresent(backupPath, 32<<20)
		return err
	}
	if err := syncDirectory(directory); err != nil {
		return err
	}
	confirmed := false
	defer func() {
		if resultErr != nil && !confirmed {
			markerRemovalErr := removePrivateFileIfPresent(markerPath, 128)
			var markerSyncErr error
			if markerRemovalErr == nil {
				markerSyncErr = syncDirectory(directory)
			}
			backupGateErr := hooks.at(faultRollbackBackupReestablished)
			var backupErr error
			if backupGateErr == nil {
				backupErr = ensureRollbackBackup(path, prior, priorExists)
			}
			rollbackEvidence := rollbackPriorAbsent
			if priorExists {
				rollbackEvidence = rollbackPriorPresent
			}
			reestablishErr := hooks.at(faultRollbackRequiredReestablished)
			var ensureErr error
			if reestablishErr == nil {
				ensureErr = ensureRollbackRequired(path, rollbackEvidence)
			}
			if rollbackErr := rollbackAtomicCommit(path); rollbackErr != nil {
				resultErr = errors.Join(resultErr, markerRemovalErr, markerSyncErr, backupGateErr, backupErr, reestablishErr, ensureErr, rollbackErr)
			} else {
				resultErr = errors.Join(resultErr, markerRemovalErr, markerSyncErr, backupGateErr, backupErr, reestablishErr, ensureErr)
			}
		}
	}()
	rollbackEvidence := rollbackPriorAbsent
	if priorExists {
		rollbackEvidence = rollbackPriorPresent
	}
	if err := ensureRollbackRequired(path, rollbackEvidence); err != nil {
		return err
	}
	if err := hooks.at(faultRollbackRequiredEstablished); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	moved := false
	defer func() {
		_ = temporary.Close()
		if !moved {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := writeAll(temporary, content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if hooks != nil && hooks.beforeReplace != nil {
		if err := hooks.beforeReplace(); err != nil {
			return err
		}
	}
	if err := replaceWindowsFile(temporaryPath, path); err != nil {
		return err
	}
	moved = true
	if hooks != nil && hooks.afterReplaceBeforeDirectorySync != nil {
		if err := hooks.afterReplaceBeforeDirectorySync(); err != nil {
			return err
		}
	}
	if err := syncDirectory(directory); err != nil {
		return err
	}
	if hooks != nil && hooks.afterReplaceDirectorySync != nil {
		if err := hooks.afterReplaceDirectorySync(); err != nil {
			return err
		}
	}
	readback, err := readPrivateRegistryFile(path, len(content))
	if err != nil || !bytes.Equal(readback, content) {
		clear(readback)
		return errors.New("provider registry readback mismatch")
	}
	clear(readback)
	if err := createExclusivePrivateFile(markerPath, commitMarker); err != nil {
		return err
	}
	if err := hooks.at(faultCommitMarkerDirectorySync); err != nil {
		return err
	}
	if err := syncDirectory(directory); err != nil {
		return err
	}
	if err := removePrivateFileIfPresent(rollbackRequiredPath(path), 128); err != nil {
		return err
	}
	if err := hooks.at(faultCommitConfirmationDirectorySync); err != nil {
		return err
	}
	if err := syncDirectory(directory); err != nil {
		return err
	}
	confirmed = true
	if err := finalizeAtomicCommit(path, hooks); err != nil {
		if verifyCommittedCleanupState(path, content) != nil {
			return err
		}
		return nil
	}
	return nil
}

func recoverAtomicCommit(path string) error {
	journalPath, backupPath, markerPath := recoveryPaths(path)
	rollbackPath := rollbackRequiredPath(path)
	rollback, rollbackExists, err := readOptionalPrivateFile(rollbackPath, 128)
	if err != nil {
		return err
	}
	defer clear(rollback)
	if rollbackExists {
		if !bytes.Equal(rollback, rollbackPriorPresent) && !bytes.Equal(rollback, rollbackPriorAbsent) {
			return errors.New("provider registry rollback evidence invalid")
		}
		return rollbackAtomicCommit(path)
	}
	journal, journalExists, err := readOptionalPrivateFile(journalPath, 128)
	if err != nil {
		return err
	}
	defer clear(journal)
	marker, markerExists, err := readOptionalPrivateFile(markerPath, 128)
	if err != nil {
		return err
	}
	defer clear(marker)
	if markerExists {
		if !bytes.Equal(marker, commitMarker) || (journalExists &&
			!bytes.Equal(journal, journalPriorPresent) && !bytes.Equal(journal, journalPriorAbsent)) {
			return errors.New("provider registry commit evidence invalid")
		}
		content, readErr := readPrivateRegistryFile(path, 16<<20)
		clear(content)
		if readErr != nil {
			return readErr
		}
		return finalizeAtomicCommit(path, nil)
	}
	if journalExists {
		return rollbackAtomicCommit(path)
	}
	backup, backupExists, err := readOptionalPrivateFile(backupPath, 32<<20)
	if err != nil {
		clear(backup)
		return err
	}
	if backupExists {
		defer clear(backup)
		target, targetExists, readErr := readOptionalPrivateFile(path, 32<<20)
		if readErr != nil {
			clear(target)
			return readErr
		}
		if !targetExists || !bytes.Equal(target, backup) {
			clear(target)
			return errors.New("provider registry orphan backup ambiguous")
		}
		clear(target)
		if err := removePrivateFileIfPresent(backupPath, 32<<20); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(path))
	}
	return nil
}

func rollbackAtomicCommit(path string) error {
	journalPath, backupPath, markerPath := recoveryPaths(path)
	rollbackPath := rollbackRequiredPath(path)
	rollback, rollbackExists, err := readOptionalPrivateFile(rollbackPath, 128)
	if err != nil {
		return err
	}
	defer clear(rollback)
	journal, exists, err := readOptionalPrivateFile(journalPath, 128)
	if err != nil {
		return err
	}
	defer clear(journal)
	if rollbackExists {
		expectedJournal := journalPriorAbsent
		switch {
		case bytes.Equal(rollback, rollbackPriorPresent):
			expectedJournal = journalPriorPresent
		case bytes.Equal(rollback, rollbackPriorAbsent):
		default:
			return errors.New("provider registry rollback evidence invalid")
		}
		if exists && !bytes.Equal(journal, expectedJournal) {
			return errors.New("provider registry rollback evidence mismatch")
		}
		journal = expectedJournal
		exists = true
	}
	if !exists {
		return nil
	}
	switch {
	case bytes.Equal(journal, journalPriorPresent):
		backup, backupExists, err := readOptionalPrivateFile(backupPath, 32<<20)
		if err != nil || !backupExists {
			clear(backup)
			return errors.New("provider registry rollback evidence unavailable")
		}
		defer clear(backup)
		if err := replaceFromBackup(path, backup); err != nil {
			return err
		}
	case bytes.Equal(journal, journalPriorAbsent):
		backup, backupExists, backupErr := readOptionalPrivateFile(backupPath, 32<<20)
		clear(backup)
		if backupErr != nil {
			return backupErr
		}
		if backupExists {
			return errors.New("provider registry prior-absent backup ambiguous")
		}
		if err := removePrivateFileIfPresent(path, 16<<20); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
	default:
		return errors.New("provider registry journal invalid")
	}
	if err := removePrivateFileIfPresent(markerPath, 128); err != nil {
		return err
	}
	if err := removePrivateFileIfPresent(journalPath, 128); err != nil {
		return err
	}
	if err := removePrivateFileIfPresent(rollbackPath, 128); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := removePrivateFileIfPresent(backupPath, 32<<20); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func finalizeAtomicCommit(path string, hooks *commitHooks) error {
	journalPath, backupPath, markerPath := recoveryPaths(path)
	if err := removePrivateFileIfPresent(backupPath, 32<<20); err != nil {
		return err
	}
	if err := hooks.at(faultCommitBackupCleanup); err != nil {
		return err
	}
	if err := removePrivateFileIfPresent(journalPath, 128); err != nil {
		return err
	}
	if err := hooks.at(faultCommitJournalCleanup); err != nil {
		return err
	}
	if err := hooks.at(faultCommitCleanupDirectorySync); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := removePrivateFileIfPresent(markerPath, 128); err != nil {
		return err
	}
	if err := hooks.at(faultCommitMarkerRemoval); err != nil {
		return err
	}
	if err := hooks.at(faultCommitMarkerRemovalSync); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func verifyCommittedCleanupState(path string, content []byte) error {
	target, err := readPrivateRegistryFile(path, len(content))
	if err != nil {
		return err
	}
	defer clear(target)
	if !bytes.Equal(target, content) {
		return errors.New("provider registry committed readback mismatch")
	}
	journalPath, backupPath, markerPath := recoveryPaths(path)
	rollback, rollbackExists, err := readOptionalPrivateFile(rollbackRequiredPath(path), 128)
	clear(rollback)
	if err != nil || rollbackExists {
		return errors.New("provider registry committed rollback evidence invalid")
	}
	marker, markerExists, err := readOptionalPrivateFile(markerPath, 128)
	if err != nil {
		clear(marker)
		return err
	}
	defer clear(marker)
	journal, journalExists, err := readOptionalPrivateFile(journalPath, 128)
	if err != nil {
		clear(journal)
		return err
	}
	defer clear(journal)
	backup, backupExists, err := readOptionalPrivateFile(backupPath, 32<<20)
	clear(backup)
	if err != nil {
		return err
	}
	if markerExists {
		if !bytes.Equal(marker, commitMarker) || (journalExists &&
			!bytes.Equal(journal, journalPriorPresent) && !bytes.Equal(journal, journalPriorAbsent)) {
			return errors.New("provider registry committed cleanup evidence invalid")
		}
		return nil
	}
	if journalExists || backupExists {
		return errors.New("provider registry committed cleanup evidence ambiguous")
	}
	return nil
}

func replaceFromBackup(path string, content []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".rollback-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	moved := false
	defer func() {
		_ = temporary.Close()
		if !moved {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := writeAll(temporary, content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceWindowsFile(temporaryPath, path); err != nil {
		return err
	}
	moved = true
	return syncDirectory(directory)
}

func replaceWindowsFile(sourcePath, destinationPath string) error {
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func createExclusivePrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := writeAll(file, content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readOptionalPrivateFile(path string, maximum int) ([]byte, bool, error) {
	content, err := readPrivateRegistryFile(path, maximum)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return content, err == nil, err
}

func removePrivateFileIfPresent(path string, maximum int) error {
	content, exists, err := readOptionalPrivateFile(path, maximum)
	clear(content)
	if err != nil || !exists {
		return err
	}
	return os.Remove(path)
}

func ensureRollbackRequired(path string, evidence []byte) error {
	rollbackPath := rollbackRequiredPath(path)
	existing, exists, err := readOptionalPrivateFile(rollbackPath, 128)
	if err != nil {
		return err
	}
	defer clear(existing)
	if exists {
		if !bytes.Equal(existing, evidence) {
			return errors.New("provider registry rollback evidence mismatch")
		}
		return syncDirectory(filepath.Dir(path))
	}
	if !bytes.Equal(evidence, rollbackPriorPresent) && !bytes.Equal(evidence, rollbackPriorAbsent) {
		return errors.New("provider registry rollback evidence invalid")
	}
	if err := createExclusivePrivateFile(rollbackPath, evidence); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func ensureRollbackBackup(path string, prior []byte, priorExists bool) error {
	_, backupPath, _ := recoveryPaths(path)
	if !priorExists {
		return nil
	}
	existing, exists, err := readOptionalPrivateFile(backupPath, 32<<20)
	if err != nil {
		return err
	}
	defer clear(existing)
	if exists {
		if !bytes.Equal(existing, prior) {
			return errors.New("provider registry rollback backup mismatch")
		}
		return syncDirectory(filepath.Dir(path))
	}
	if err := createExclusivePrivateFile(backupPath, prior); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func rollbackRequiredPath(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".rollback-required")
}

func recoveryPaths(path string) (string, string, string) {
	directory := filepath.Dir(path)
	base := filepath.Base(path)
	return filepath.Join(directory, "."+base+".commit-journal"),
		filepath.Join(directory, "."+base+".previous"),
		filepath.Join(directory, "."+base+".committed")
}

func syncDirectory(path string) error {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathUTF16, registryDirectorySyncAccess,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.FlushFileBuffers(handle)
}

func writeAll(file *os.File, content []byte) error {
	for len(content) > 0 {
		written, err := file.Write(content)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}
