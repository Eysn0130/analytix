//go:build darwin

package processauthority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// SuspendedInspectionConfig identifies an already-open executable and the
// private directory authority used to stage it. The executable is never
// resumed: inspection observes the kernel-loaded image while it is suspended.
type SuspendedInspectionConfig struct {
	Executable               *os.File
	ExpectedExecutableSHA256 string
	ExpectedExecutableSize   int64
	StagingRoot              string
	StagingRootAuthority     *os.File
}

// SuspendedInspector inspects a live, start-suspended process. The process ID
// remains owned by this package and is guaranteed to be killed and waited.
type SuspendedInspector func(context.Context, SuspendedProcessIdentity) error

// SuspendedProcessIdentity is read from the kernel-loaded process image, not
// from an executable path supplied by the caller or verifier.
type SuspendedProcessIdentity struct {
	PID    int
	CDHash string
}

// InspectSuspendedDarwinExecutable binds an exact opened file to the image
// loaded by the kernel, invokes inspector against that live process, validates
// the binding again, and terminates the process without executing its first
// instruction.
func InspectSuspendedDarwinExecutable(
	ctx context.Context,
	config SuspendedInspectionConfig,
	inspector SuspendedInspector,
) (returnErr error) {
	if inspector == nil {
		return ErrRequestInvalid
	}
	validated, expectedHash, executableObject, err := validateSuspendedInspection(ctx, config)
	if err != nil {
		return err
	}
	stagedPath, stagedFile, cleanupStage, err := stageDarwinExecutableWithAuthority(
		ctx,
		validated.StagingRoot,
		validated.StagingRootAuthority,
		validated.Executable,
		validated.ExpectedExecutableSize,
		expectedHash,
	)
	if err != nil {
		return ErrExecutableIdentity
	}
	defer func() {
		stageCloseErr := stagedFile.Close()
		cleanupErr := cleanupStage()
		if stageCloseErr != nil || cleanupErr != nil {
			darwinAuthorityPoisoned.Store(true)
			returnErr = ErrTermination
		}
	}()
	openedCDHashes, err := openedFileCodeDirectoryHashes(stagedFile)
	if err != nil {
		return ErrExecutableIdentity
	}
	var stagedStat unix.Stat_t
	if unix.Fstat(int(stagedFile.Fd()), &stagedStat) != nil {
		return ErrExecutableIdentity
	}
	stagedObject := &pinnedDarwinObject{file: stagedFile, stat: stagedStat}

	stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter, err := darwinProcessPipes()
	if err != nil {
		return ErrUnavailable
	}
	defer stdinReader.Close()
	defer stdinWriter.Close()
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	defer stderrReader.Close()
	defer stderrWriter.Close()

	if darwinContextFailure(ctx) != nil || darwinAuthorityPoisoned.Load() ||
		!pinnedDarwinObjectUnchanged(executableObject) ||
		!darwinDirectoryPathMatchesAuthority(validated.StagingRoot, validated.StagingRootAuthority) {
		return ErrExecutableIdentity
	}
	pid, err := spawnSuspended(
		stagedPath,
		[]string{stagedPath},
		[]string{"LANG=C", "LC_ALL=C", "TZ=UTC"},
		int(validated.StagingRootAuthority.Fd()),
		int(stdinReader.Fd()),
		int(stdoutWriter.Fd()),
		int(stderrWriter.Fd()),
	)
	if err != nil {
		return ErrUnavailable
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		if !killAndWaitDarwinProcess(nil, pid) {
			darwinAuthorityPoisoned.Store(true)
			return ErrTermination
		}
		return ErrUnavailable
	}
	defer func() {
		if !killAndWaitDarwinProcess(process, pid) {
			darwinAuthorityPoisoned.Store(true)
			returnErr = ErrTermination
		}
	}()

	loadedIdentity, ok := suspendedDarwinImageIdentity(
		ctx,
		pid,
		executableObject,
		stagedObject,
		expectedHash,
		openedCDHashes,
		validated.StagingRoot,
		validated.StagingRootAuthority,
	)
	if !ok {
		return ErrExecutableIdentity
	}
	if err := inspector(ctx, loadedIdentity); err != nil {
		return err
	}
	postInspectionIdentity, ok := suspendedDarwinImageIdentity(
		ctx,
		pid,
		executableObject,
		stagedObject,
		expectedHash,
		openedCDHashes,
		validated.StagingRoot,
		validated.StagingRootAuthority,
	)
	if !ok || postInspectionIdentity != loadedIdentity {
		return ErrExecutableIdentity
	}
	return nil
}

func validateSuspendedInspection(
	ctx context.Context,
	config SuspendedInspectionConfig,
) (SuspendedInspectionConfig, [sha256.Size]byte, *pinnedDarwinObject, error) {
	var expected [sha256.Size]byte
	deadline, hasDeadline := time.Time{}, false
	if ctx != nil {
		deadline, hasDeadline = ctx.Deadline()
	}
	if darwinAuthorityPoisoned.Load() {
		return SuspendedInspectionConfig{}, expected, nil, ErrUnavailable
	}
	if ctx == nil || darwinContextFailure(ctx) != nil || !hasDeadline || !deadline.After(time.Now()) ||
		config.Executable == nil || config.StagingRootAuthority == nil ||
		config.ExpectedExecutableSize <= 0 || config.ExpectedExecutableSize > maxExecutableBytes ||
		!filepath.IsAbs(config.StagingRoot) ||
		len(config.ExpectedExecutableSHA256) != sha256.Size*2 ||
		config.ExpectedExecutableSHA256 != strings.ToLower(config.ExpectedExecutableSHA256) {
		return SuspendedInspectionConfig{}, expected, nil, ErrRequestInvalid
	}
	decoded, err := hex.DecodeString(config.ExpectedExecutableSHA256)
	if err != nil {
		return SuspendedInspectionConfig{}, expected, nil, ErrRequestInvalid
	}
	copy(expected[:], decoded)
	var before unix.Stat_t
	if unix.Fstat(int(config.Executable.Fd()), &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG ||
		before.Nlink != 1 || before.Size != config.ExpectedExecutableSize || before.Mode&0o022 != 0 {
		return SuspendedInspectionConfig{}, expected, nil, ErrExecutableIdentity
	}
	currentHash, err := sha256OpenedFile(ctx, config.Executable)
	var after unix.Stat_t
	if err != nil || currentHash != expected || unix.Fstat(int(config.Executable.Fd()), &after) != nil ||
		componentDarwinFileIdentity(before) != componentDarwinFileIdentity(after) {
		return SuspendedInspectionConfig{}, expected, nil, ErrExecutableIdentity
	}
	root, err := openPinnedDarwinObject(config.StagingRoot, true)
	if err != nil {
		return SuspendedInspectionConfig{}, expected, nil, ErrExecutableIdentity
	}
	rootValid := root.stat.Mode&unix.S_IFMT == unix.S_IFDIR && root.stat.Mode&0o077 == 0 &&
		darwinDirectoryAuthorityMatches(root, config.StagingRootAuthority)
	rootCloseErr := root.file.Close()
	if !rootValid || rootCloseErr != nil {
		if rootCloseErr != nil {
			darwinAuthorityPoisoned.Store(true)
			return SuspendedInspectionConfig{}, expected, nil, ErrTermination
		}
		return SuspendedInspectionConfig{}, expected, nil, ErrExecutableIdentity
	}
	config.StagingRoot = filepath.Clean(config.StagingRoot)
	return config, expected, &pinnedDarwinObject{file: config.Executable, stat: after}, nil
}

func suspendedDarwinImageIdentity(
	ctx context.Context,
	pid int,
	executable *pinnedDarwinObject,
	staged *pinnedDarwinObject,
	expectedHash [sha256.Size]byte,
	openedCDHashes [][cdHashBytes]byte,
	stagingRoot string,
	stagingRootAuthority *os.File,
) (SuspendedProcessIdentity, bool) {
	if darwinContextFailure(ctx) != nil || darwinAuthorityPoisoned.Load() ||
		!pinnedDarwinObjectUnchanged(executable) || !pinnedDarwinObjectUnchanged(staged) ||
		!darwinDirectoryPathMatchesAuthority(stagingRoot, stagingRootAuthority) {
		return SuspendedProcessIdentity{}, false
	}
	loadedCDHash, err := loadedProcessCDHash(pid)
	if err != nil || !codeDirectoryHashMatches(loadedCDHash, openedCDHashes) ||
		!loadedDarwinExecutableMatchesOpenedFile(ctx, pid, staged, expectedHash) {
		return SuspendedProcessIdentity{}, false
	}
	return SuspendedProcessIdentity{PID: pid, CDHash: hex.EncodeToString(loadedCDHash[:])}, true
}
