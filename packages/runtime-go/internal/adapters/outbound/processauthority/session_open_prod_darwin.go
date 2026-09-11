//go:build darwin && analytix_prod

package processauthority

import (
	"bufio"
	"context"
	"os"

	"golang.org/x/sys/unix"
)

// openSession launches the admitted data-engine image directly. Production
// never turns runtime-server into a stdin-driven exec bootstrap and never
// start-suspends or SIGSTOPs the target. The trusted target establishes
// RLIMIT_NPROC, its fixed descriptor inventory, and the FD 4 owner watchdog
// before it emits readiness or reads a request.
func openSession(ctx context.Context, config SessionConfig) (sessionResult Session, returnErr error) {
	if config.Executable == nil {
		if config.ReadOnlyInput != nil && config.ReadOnlyInput.File != nil {
			_ = config.ReadOnlyInput.File.Close()
		}
		if config.ReadWriteOutput != nil && config.ReadWriteOutput.File != nil {
			_ = config.ReadWriteOutput.File.Close()
		}
		return nil, ErrRequestInvalid
	}
	executable := config.Executable
	defer executable.Close()
	ownedReadOnlyInput, err := takeDarwinReadOnlyInput(config.ReadOnlyInput)
	if err != nil {
		if config.ReadWriteOutput != nil && config.ReadWriteOutput.File != nil {
			_ = config.ReadWriteOutput.File.Close()
		}
		return nil, err
	}
	config.ReadOnlyInput = ownedReadOnlyInput
	if ownedReadOnlyInput != nil {
		defer ownedReadOnlyInput.File.Close()
	}
	ownedReadWriteOutput, err := takeDarwinReadWriteOutput(config.ReadWriteOutput)
	if err != nil {
		return nil, err
	}
	config.ReadWriteOutput = ownedReadWriteOutput
	if ownedReadWriteOutput != nil {
		defer ownedReadWriteOutput.File.Close()
	}
	if darwinAuthorityPoisoned.Load() {
		return nil, ErrUnavailable
	}
	workingAuthority, err := duplicateDarwinSessionAuthority(
		config.WorkingDirectoryAuthority,
		"process-authority-working-directory",
	)
	if err != nil {
		if config.WorkingDirectoryAuthority == nil {
			return nil, ErrRequestInvalid
		}
		return nil, ErrWorkingDirectory
	}
	stagingAuthority, err := duplicateDarwinSessionAuthority(
		config.StagingRootAuthority,
		"process-authority-staging-root",
	)
	if err != nil {
		workingCloseErr := workingAuthority.Close()
		if workingCloseErr != nil {
			darwinAuthorityPoisoned.Store(true)
			return nil, ErrTermination
		}
		if config.StagingRootAuthority == nil {
			return nil, ErrRequestInvalid
		}
		return nil, ErrExecutableIdentity
	}
	authoritiesTransferred := false
	defer func() {
		if authoritiesTransferred {
			return
		}
		workingCloseErr := workingAuthority.Close()
		stagingCloseErr := stagingAuthority.Close()
		if workingCloseErr != nil || stagingCloseErr != nil {
			darwinAuthorityPoisoned.Store(true)
			returnErr = ErrTermination
		}
	}()
	config.WorkingDirectoryAuthority = workingAuthority
	config.StagingRootAuthority = stagingAuthority
	validated, executableHash, err := validateDarwinSessionConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	workingDirectory, err := openPinnedDarwinObject(validated.WorkingDirectory, true)
	if err != nil {
		return nil, ErrWorkingDirectory
	}
	workingTransferred := false
	defer func() {
		if !workingTransferred && workingDirectory != nil && workingDirectory.file != nil {
			if err := workingDirectory.file.Close(); err != nil {
				darwinAuthorityPoisoned.Store(true)
				returnErr = ErrTermination
			}
		}
	}()
	if workingDirectory.stat.Mode&unix.S_IFMT != unix.S_IFDIR || workingDirectory.stat.Mode&0o022 != 0 ||
		!darwinDirectoryAuthorityMatches(workingDirectory, validated.WorkingDirectoryAuthority) {
		return nil, ErrWorkingDirectory
	}
	targetPath, targetFile, cleanupTarget, err := stageDarwinExecutableWithAuthority(
		ctx,
		validated.StagingRoot,
		validated.StagingRootAuthority,
		executable,
		validated.ExpectedExecutableSize,
		executableHash,
	)
	if err != nil {
		return nil, ErrExecutableIdentity
	}
	targetTransferred := false
	defer func() {
		if targetTransferred {
			return
		}
		closeErr := targetFile.Close()
		cleanupErr := cleanupTarget()
		if closeErr != nil || cleanupErr != nil {
			darwinAuthorityPoisoned.Store(true)
			returnErr = ErrTermination
		}
	}()
	targetCDHashes, err := openedFileCodeDirectoryHashes(targetFile)
	var targetStat unix.Stat_t
	if err != nil || unix.Fstat(int(targetFile.Fd()), &targetStat) != nil {
		return nil, ErrExecutableIdentity
	}
	targetObject := &pinnedDarwinObject{file: targetFile, stat: targetStat}
	currentTargetHash, hashErr := sha256OpenedFile(ctx, targetFile)
	if hashErr != nil || currentTargetHash != executableHash || !pinnedDarwinObjectUnchanged(targetObject) ||
		!darwinPathMatchesPinnedObject(targetPath, targetObject) {
		return nil, ErrExecutableIdentity
	}

	stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter, err := darwinProcessPipes()
	if err != nil {
		return nil, ErrUnavailable
	}
	ownerLivenessReader, ownerLivenessWriter, err := openDarwinOwnerLivenessPipe()
	if err != nil {
		closeFailed := false
		for _, file := range []*os.File{
			stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter,
		} {
			if file.Close() != nil {
				closeFailed = true
			}
		}
		if closeFailed {
			darwinAuthorityPoisoned.Store(true)
			return nil, ErrTermination
		}
		return nil, err
	}
	var process *os.Process
	var pid int
	var transitions *darwinProcessTransitionMonitor
	var directSession *darwinSession
	cleanupOnFailure := true
	defer func() {
		if !cleanupOnFailure {
			return
		}
		if directSession != nil {
			if directSession.cleanup(nil) != nil {
				darwinAuthorityPoisoned.Store(true)
				returnErr = ErrTermination
			}
			return
		}
		cleanupFailed := false
		if pid > 0 {
			if process == nil {
				process, _ = os.FindProcess(pid)
			}
			cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), darwinSessionCleanupDeadline)
			if terminateDirectDarwinProcess(cleanupContext, process, pid, darwinProcessIdentity{}) != nil {
				darwinAuthorityPoisoned.Store(true)
				returnErr = ErrTermination
				cleanupFailed = true
			}
			cancelCleanup()
		}
		if transitions != nil {
			if transitions.Close() != nil {
				cleanupFailed = true
			}
		}
		for _, file := range []*os.File{
			stdinReader, stdinWriter, stdoutReader, stdoutWriter, stderrReader, stderrWriter,
			ownerLivenessReader, ownerLivenessWriter,
		} {
			if file != nil {
				if file.Close() != nil {
					cleanupFailed = true
				}
			}
		}
		if cleanupFailed {
			darwinAuthorityPoisoned.Store(true)
			returnErr = ErrTermination
		}
	}()

	if darwinContextFailure(ctx) != nil || darwinAuthorityPoisoned.Load() ||
		!pinnedDarwinObjectUnchanged(targetObject) ||
		!darwinPathMatchesPinnedObject(targetPath, targetObject) ||
		!pinnedDarwinObjectUnchanged(workingDirectory) ||
		!darwinDirectoryAuthorityMatches(workingDirectory, validated.WorkingDirectoryAuthority) ||
		!darwinDirectoryPathMatchesAuthority(validated.StagingRoot, validated.StagingRootAuthority) {
		return nil, ErrExecutableIdentity
	}
	if validated.ReadOnlyInput != nil {
		// This is the final validation of the process-authority-owned duplicate
		// immediately before it is mapped to the fixed child descriptor.
		if err := validateDarwinReadOnlyInput(ctx, *validated.ReadOnlyInput); err != nil {
			return nil, err
		}
	}
	if validated.ReadWriteOutput != nil {
		// The empty unlinked inode is validated immediately before the
		// process-authority-owned duplicate is mapped to fixed child FD 5.
		if err := validateDarwinReadWriteOutput(*validated.ReadWriteOutput); err != nil {
			return nil, err
		}
	}
	extra := []darwinSpawnFile{{
		source: int(ownerLivenessReader.Fd()),
		target: darwinOwnerLivenessTargetFD,
	}}
	if validated.ReadOnlyInput != nil {
		extra = append(extra, darwinSpawnFile{
			source: int(validated.ReadOnlyInput.File.Fd()),
			target: darwinReadOnlyInputTargetFD,
		})
	}
	if validated.ReadWriteOutput != nil {
		extra = append(extra, darwinSpawnFile{
			source: int(validated.ReadWriteOutput.File.Fd()),
			target: darwinReadWriteOutputTargetFD,
		})
	}
	arguments := append([]string{targetPath}, validated.Arguments...)
	pid, err = spawnDirectWithExtraFiles(
		targetPath,
		arguments,
		validated.Environment,
		int(workingDirectory.file.Fd()),
		int(stdinReader.Fd()),
		int(stdoutWriter.Fd()),
		int(stderrWriter.Fd()),
		extra,
	)
	if err != nil {
		return nil, ErrUnavailable
	}
	process, err = os.FindProcess(pid)
	if err != nil {
		return nil, ErrTermination
	}
	transitions, err = openDarwinProcessTransitionMonitor(pid)
	if err != nil {
		return nil, err
	}
	for _, endpoint := range []**os.File{&stdinReader, &stdoutWriter, &stderrWriter, &ownerLivenessReader} {
		if *endpoint == nil || (*endpoint).Close() != nil {
			return nil, ErrTermination
		}
		*endpoint = nil
	}
	stdoutBuffered := bufio.NewReaderSize(stdoutReader, 64*1024)
	stderrBuffered := bufio.NewReaderSize(stderrReader, 64*1024)
	rootState, err := readDarwinProcessState(ctx, pid, true)
	if err != nil || rootState.Identity.PID != pid || rootState.Identity.PPID != os.Getpid() ||
		rootState.Identity.PGID != pid || rootState.Status == darwinProcStatusStopped ||
		rootState.Status == darwinProcStatusZombie {
		return nil, ErrExecutableIdentity
	}
	directSession = &darwinSession{
		gate: make(chan struct{}, 1), state: darwinSessionAwaitingReadiness, direct: true,
		pid: pid, identity: rootState.Identity, process: process,
		stdin: stdinWriter, stdout: stdoutReader, stdoutReader: stdoutBuffered,
		stderr: stderrReader, stderrReader: stderrBuffered, ownerLiveness: ownerLivenessWriter,
		targetStaged: targetObject, targetPath: targetPath, targetHash: executableHash,
		targetCDHashes: targetCDHashes, cleanupTarget: cleanupTarget,
		workingDirectory: workingDirectory, workingAuthority: validated.WorkingDirectoryAuthority,
		stagingRoot: validated.StagingRoot, stagingAuthority: validated.StagingRootAuthority,
		transitions: transitions, gateDeadline: darwinSessionCleanupDeadline,
	}
	targetTransferred = true
	workingTransferred = true
	authoritiesTransferred = true
	if !directSession.healthyDirectLocked(ctx) {
		return nil, ErrExecutableIdentity
	}
	directSession.gate <- struct{}{}
	ownerLivenessWriter = nil
	stdinWriter = nil
	stdoutReader = nil
	stderrReader = nil
	transitions = nil
	cleanupOnFailure = false
	return directSession, nil
}
