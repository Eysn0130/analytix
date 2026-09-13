//go:build !windows

package secretstore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var (
	errAtomicCommitAuthorityChanged = errors.New("private commit authority changed; state preserved")
	atomicJournalPriorPresent       = []byte("analytix-secret-store-commit:v1:prior-present\n")
	atomicJournalPriorAbsent        = []byte("analytix-secret-store-commit:v1:prior-absent\n")
	atomicCommittedMarker           = []byte("analytix-secret-store-commit:v1:committed\n")
	atomicRollbackRequired          = []byte("analytix-secret-store-commit:v1:rollback-required\n")
)

func readPrivateCommittedFile(path string, maximum int64) ([]byte, error) {
	return readPrivateCommittedFileWithLinks(path, maximum, false)
}

func readPrivateCommittedFileWithLinks(path string, maximum int64, singleLink bool) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filepath.Base(path))
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("private file open failed")
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o777 != 0o600 || stat.Uid != uint32(os.Getuid()) || (singleLink && stat.Nlink != 1) {
		return nil, errors.New("private file validation failed")
	}
	content, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		clearBytes(content)
		return nil, err
	}
	if int64(len(content)) > maximum {
		clearBytes(content)
		return nil, errors.New("private file size invalid")
	}
	if singleLink {
		var current, linked unix.Stat_t
		if unix.Fstat(fd, &current) != nil || unix.Lstat(path, &linked) != nil ||
			current.Dev != stat.Dev || current.Ino != stat.Ino || current.Mode != stat.Mode ||
			current.Uid != stat.Uid || current.Gid != stat.Gid || current.Nlink != 1 || current.Size != int64(len(content)) ||
			linked.Dev != current.Dev || linked.Ino != current.Ino || linked.Mode != current.Mode ||
			linked.Uid != current.Uid || linked.Gid != current.Gid || linked.Nlink != 1 || linked.Size != current.Size {
			clearBytes(content)
			return nil, errors.New("private authority file changed during read")
		}
	}
	return content, nil
}

func writePrivateFileAtomically(path string, content []byte, hooks *atomicCommitHooks) (resultErr error) {
	directory := filepath.Dir(path)
	if err := ensurePrivateStoreDirectory(directory); err != nil {
		return err
	}
	if err := recoverPrivateFileCommit(path); err != nil {
		return err
	}
	journalPath, backupPath, committedPath := atomicRecoveryPaths(path)
	rollbackPath := atomicRollbackRequiredPath(path)
	transactionStarted := true
	commitConfirmed := false
	defer func() {
		if resultErr != nil && transactionStarted && !commitConfirmed {
			if recoverErr := recoverPrivateFileCommit(path); recoverErr != nil {
				resultErr = recoverErr
			}
		}
	}()

	prior, err := readPrivateCommittedFile(path, maxPersistentStoreBytes)
	priorExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	defer clearBytes(prior)
	journalContent := atomicJournalPriorAbsent
	if priorExists {
		journalContent = atomicJournalPriorPresent
		if err := createPrivateExclusiveFile(backupPath, prior); err != nil {
			return err
		}
	}
	if err := createPrivateExclusiveFile(journalPath, journalContent); err != nil {
		return err
	}
	if err := createPrivateExclusiveFile(rollbackPath, atomicRollbackRequired); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if info, err := temporary.Stat(); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("temporary file validation failed")
	}
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
			if errors.Is(err, errAtomicCommitAuthorityChanged) {
				// The target is unknown and was never replaced by this transaction.
				// Preserve both it and our recovery evidence; do not overwrite it.
				transactionStarted = false
			}
			return err
		}
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	if hooks != nil && hooks.afterReplaceBeforeDirectorySync != nil {
		if err := hooks.afterReplaceBeforeDirectorySync(); err != nil {
			return err
		}
	}
	if err := syncPrivateStoreDirectory(directory); err != nil {
		return err
	}
	_, err = createPrivateCommitMarker(committedPath, hooks)
	if err != nil {
		return err
	}
	if err := removeValidatedPrivateFile(rollbackPath, 64); err != nil {
		return err
	}
	confirmationErr := error(nil)
	if hooks != nil && hooks.afterRollbackRequiredRemoval != nil {
		confirmationErr = hooks.afterRollbackRequiredRemoval()
	} else {
		confirmationErr = syncPrivateStoreDirectory(directory)
	}
	if confirmationErr != nil {
		reestablishmentErr := error(nil)
		if hooks != nil && hooks.beforeRollbackRequiredReestablishment != nil {
			reestablishmentErr = hooks.beforeRollbackRequiredReestablishment()
		} else {
			reestablishmentErr = createPrivateExclusiveFile(rollbackPath, atomicRollbackRequired)
		}
		transactionStarted = false
		if rollbackErr := rollbackUnconfirmedPrivateFileCommit(path); rollbackErr != nil {
			return errors.Join(confirmationErr, reestablishmentErr, rollbackErr)
		}
		return errors.Join(confirmationErr, reestablishmentErr)
	}
	commitConfirmed = true
	transactionStarted = false
	_ = finalizeConfirmedPrivateCommit(path)
	return nil
}

func recoverPrivateFileCommit(path string) error {
	return recoverPrivateFileCommitValidated(path, nil)
}

// Authority-bearing records may restrict rollback content before recovery
// mutates any target or journal. The exact validated bytes are used below.
func recoverPrivateFileCommitValidated(path string, validate func([]byte, bool) error) error {
	journalPath, backupPath, committedPath := atomicRecoveryPaths(path)
	journal, journalExists, err := readOptionalPrivateFile(journalPath, 64)
	if err != nil {
		return err
	}
	defer clearBytes(journal)
	committed, committedExists, err := readOptionalPrivateFile(committedPath, 64)
	if err != nil {
		return err
	}
	defer clearBytes(committed)
	rollbackRequired, rollbackRequiredExists, err := readOptionalPrivateFile(atomicRollbackRequiredPath(path), 64)
	if err != nil {
		return err
	}
	defer clearBytes(rollbackRequired)
	var validatedBackup []byte
	defer func() { clearBytes(validatedBackup) }()
	if validate != nil && journalExists && (rollbackRequiredExists || !committedExists) {
		switch {
		case bytes.Equal(journal, atomicJournalPriorPresent):
			validatedBackup, err = readPrivateCommittedFileWithLinks(backupPath, maxPersistentStoreBytes, true)
			if err != nil {
				return errors.New("commit backup unavailable")
			}
			if err := validate(validatedBackup, true); err != nil {
				return err
			}
		case bytes.Equal(journal, atomicJournalPriorAbsent):
			if err := validate(nil, false); err != nil {
				return err
			}
		default:
			return errors.New("commit journal validation failed")
		}
	}
	if rollbackRequiredExists {
		if (!bytes.Equal(rollbackRequired, atomicRollbackRequired) && !bytes.Equal(rollbackRequired, atomicCommittedMarker)) || !journalExists {
			return errors.New("rollback marker validation failed")
		}
		if committedExists {
			if !bytes.Equal(committed, atomicCommittedMarker) {
				return errors.New("committed marker validation failed")
			}
			if err := removeValidatedPrivateFile(committedPath, 64); err != nil {
				return err
			}
			if err := syncPrivateStoreDirectory(filepath.Dir(path)); err != nil {
				return err
			}
			committedExists = false
		}
	}
	if committedExists {
		if !bytes.Equal(committed, atomicCommittedMarker) {
			return errors.New("committed marker validation failed")
		}
		if journalExists && !bytes.Equal(journal, atomicJournalPriorPresent) && !bytes.Equal(journal, atomicJournalPriorAbsent) {
			return errors.New("commit journal validation failed")
		}
		return finalizeConfirmedPrivateCommit(path)
	}
	if !journalExists {
		backup, backupExists, err := readOptionalPrivateFile(backupPath, maxPersistentStoreBytes)
		clearBytes(backup)
		if err != nil {
			return err
		}
		if backupExists {
			if err := removeValidatedPrivateFile(backupPath, maxPersistentStoreBytes); err != nil {
				return err
			}
			return syncPrivateStoreDirectory(filepath.Dir(path))
		}
		return nil
	}

	switch {
	case bytes.Equal(journal, atomicJournalPriorPresent):
		backup, backupExists := validatedBackup, validate != nil
		var err error
		if validate == nil {
			backup, backupExists, err = readOptionalPrivateFile(backupPath, maxPersistentStoreBytes)
		}
		if err != nil {
			return err
		}
		if !backupExists {
			return errors.New("commit backup unavailable")
		}
		defer clearBytes(backup)
		if err := replaceFromRecoveryBackup(path, backup); err != nil {
			return err
		}
	case bytes.Equal(journal, atomicJournalPriorAbsent):
		if err := removeUncommittedTargetIfPresent(path); err != nil {
			return err
		}
	default:
		return errors.New("commit journal validation failed")
	}
	if err := removeValidatedPrivateFileIfPresent(backupPath, maxPersistentStoreBytes); err != nil {
		return err
	}
	if err := removeValidatedPrivateFile(journalPath, 64); err != nil {
		return err
	}
	if err := removeValidatedPrivateFileIfPresent(atomicRollbackRequiredPath(path), 64); err != nil {
		return err
	}
	return syncPrivateStoreDirectory(filepath.Dir(path))
}

func rollbackUnconfirmedPrivateFileCommit(path string) error {
	journalPath, backupPath, committedPath := atomicRecoveryPaths(path)
	journal, journalExists, err := readOptionalPrivateFile(journalPath, 64)
	if err != nil {
		return err
	}
	defer clearBytes(journal)
	if !journalExists || (!bytes.Equal(journal, atomicJournalPriorPresent) && !bytes.Equal(journal, atomicJournalPriorAbsent)) {
		return errors.New("commit journal validation failed")
	}
	committed, committedExists, err := readOptionalPrivateFile(committedPath, 64)
	if err != nil {
		return err
	}
	defer clearBytes(committed)
	if !committedExists || !bytes.Equal(committed, atomicCommittedMarker) {
		return errors.New("committed marker validation failed")
	}
	rollbackRequired, rollbackRequiredExists, err := readOptionalPrivateFile(atomicRollbackRequiredPath(path), 64)
	if err != nil {
		return err
	}
	defer clearBytes(rollbackRequired)
	if rollbackRequiredExists && !bytes.Equal(rollbackRequired, atomicRollbackRequired) && !bytes.Equal(rollbackRequired, atomicCommittedMarker) {
		return errors.New("rollback marker validation failed")
	}
	backup, backupExists, err := readOptionalPrivateFile(backupPath, maxPersistentStoreBytes)
	if err != nil {
		return err
	}
	defer clearBytes(backup)
	if bytes.Equal(journal, atomicJournalPriorPresent) != backupExists {
		return errors.New("commit backup validation failed")
	}
	rollbackPath := atomicRollbackRequiredPath(path)
	if rollbackRequiredExists {
		if err := removeValidatedPrivateFile(committedPath, 64); err != nil {
			return err
		}
		if err := syncPrivateStoreDirectory(filepath.Dir(path)); err != nil {
			return err
		}
	} else {
		if err := os.Rename(committedPath, rollbackPath); err != nil {
			return err
		}
		if err := syncPrivateStoreDirectory(filepath.Dir(path)); err != nil {
			return err
		}
	}
	if backupExists {
		if err := replaceFromRecoveryBackup(path, backup); err != nil {
			return err
		}
	} else if err := removeUncommittedTargetIfPresent(path); err != nil {
		return err
	}
	if err := removeValidatedPrivateFileIfPresent(backupPath, maxPersistentStoreBytes); err != nil {
		return err
	}
	if err := removeValidatedPrivateFile(journalPath, 64); err != nil {
		return err
	}
	if err := removeValidatedPrivateFileIfPresent(atomicRollbackRequiredPath(path), 64); err != nil {
		return err
	}
	return syncPrivateStoreDirectory(filepath.Dir(path))
}

func finalizeConfirmedPrivateCommit(path string) error {
	journalPath, backupPath, committedPath := atomicRecoveryPaths(path)
	if err := removeValidatedPrivateFileIfPresent(backupPath, maxPersistentStoreBytes); err != nil {
		return err
	}
	if err := removeValidatedPrivateFileIfPresent(journalPath, 64); err != nil {
		return err
	}
	if err := syncPrivateStoreDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err := removeValidatedPrivateFileIfPresent(committedPath, 64); err != nil {
		return err
	}
	_ = syncPrivateStoreDirectory(filepath.Dir(path))
	return nil
}

func createPrivateCommitMarker(path string, hooks *atomicCommitHooks) (bool, error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".pending-")
	if err != nil {
		return false, err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if info, statErr := temporary.Stat(); statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return false, errors.New("commit marker temporary validation failed")
	}
	if err := writeAll(temporary, atomicCommittedMarker); err != nil {
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return false, err
	}
	if hooks != nil && hooks.afterCommitMarkerLink != nil {
		if err := hooks.afterCommitMarkerLink(); err != nil {
			return true, err
		}
	}
	if err := syncPrivateStoreDirectory(directory); err != nil {
		return true, err
	}
	return true, nil
}

func createPrivateExclusiveFile(path string, content []byte) error {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), filepath.Base(path))
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("private artifact open failed")
	}
	if err := writeAll(file, content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncPrivateStoreDirectory(filepath.Dir(path))
}

func readOptionalPrivateFile(path string, maximum int64) ([]byte, bool, error) {
	content, err := readPrivateCommittedFile(path, maximum)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return content, true, nil
}

func replaceFromRecoveryBackup(path string, content []byte) error {
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
	if info, err := temporary.Stat(); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("recovery temporary validation failed")
	}
	if err := writeAll(temporary, content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	moved = true
	return syncPrivateStoreDirectory(directory)
}

func removeUncommittedTargetIfPresent(path string) error {
	content, exists, err := readOptionalPrivateFile(path, maxPersistentStoreBytes)
	clearBytes(content)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncPrivateStoreDirectory(filepath.Dir(path))
}

func removeValidatedPrivateFileIfPresent(path string, maximum int64) error {
	content, exists, err := readOptionalPrivateFile(path, maximum)
	clearBytes(content)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return os.Remove(path)
}

func removeValidatedPrivateFile(path string, maximum int64) error {
	content, err := readPrivateCommittedFile(path, maximum)
	clearBytes(content)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func atomicRecoveryPaths(path string) (journal string, backup string, committed string) {
	directory := filepath.Dir(path)
	base := filepath.Base(path)
	return filepath.Join(directory, "."+base+".commit-journal"),
		filepath.Join(directory, "."+base+".previous"),
		filepath.Join(directory, "."+base+".committed")
}

func atomicRollbackRequiredPath(path string) string {
	directory := filepath.Dir(path)
	return filepath.Join(directory, "."+filepath.Base(path)+".rollback-required")
}

func ensurePrivateStoreDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Lstat(directory, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o777 != 0o700 || stat.Uid != uint32(os.Getuid()) {
		return errors.New("private directory validation failed")
	}
	return nil
}

func syncPrivateStoreDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
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
