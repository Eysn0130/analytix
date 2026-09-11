//go:build darwin

package processauthority

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestExecuteDarwinCodeSignSystemToolUsesPinnedAuthority(t *testing.T) {
	workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
	workingAuthority, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer workingAuthority.Close()
	tool, err := openPinnedDarwinObject("/usr/bin/codesign", false)
	if err != nil {
		t.Fatal(err)
	}
	defer tool.file.Close()
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(tool.file.Fd()), &filesystem); err != nil {
		t.Fatal(err)
	}
	if !trustedDarwinSystemExecutable(tool) {
		t.Fatalf("codesign system identity rejected: stat=%#v filesystem=%#v", tool.stat, filesystem)
	}
	result, err := ExecuteSystemTool(context.Background(), SystemToolRequest{
		Tool:                      SystemToolDarwinCodeSign,
		Arguments:                 []string{"--display", "--verbose=4", "+" + strconv.Itoa(os.Getpid())},
		WorkingDirectory:          workingDirectory,
		WorkingDirectoryAuthority: workingAuthority,
		Timeout:                   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("execute codesign: result=%#v err=%v", result, err)
	}
	output := append(append([]byte(nil), result.Stdout...), result.Stderr...)
	if result.ExitCode != 0 || result.TerminationStatus != TerminationExited ||
		!bytes.Contains(output, []byte("Format=pid diskrep")) {
		t.Fatalf("codesign result=%#v output=%q", result, output)
	}
	if entries, err := os.ReadDir(workingDirectory); err != nil || len(entries) != 0 {
		t.Fatalf("system-tool working-directory residue=%#v err=%v", entries, err)
	}
}

func TestExecuteDarwinCodeSignSystemToolRejectsOpenArgumentSurface(t *testing.T) {
	workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
	workingAuthority, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer workingAuthority.Close()
	base := SystemToolRequest{
		Tool:                      SystemToolDarwinCodeSign,
		Arguments:                 []string{"--display", "--verbose=4", "+" + strconv.Itoa(os.Getpid())},
		WorkingDirectory:          workingDirectory,
		WorkingDirectoryAuthority: workingAuthority,
		Timeout:                   time.Second,
	}
	for name, mutate := range map[string]func(*SystemToolRequest){
		"unknown tool": func(request *SystemToolRequest) {
			request.Tool = "arbitrary"
		},
		"arbitrary arguments": func(request *SystemToolRequest) {
			request.Arguments = []string{"--force", "--sign", "-", "/tmp/target"}
		},
		"weakened requirement": func(request *SystemToolRequest) {
			request.Arguments = []string{"--verify", "--strict", "--verbose=2", "-R=true", strconv.Itoa(os.Getpid())}
		},
		"missing timeout": func(request *SystemToolRequest) {
			request.Timeout = 0
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := base
			request.Arguments = append([]string(nil), base.Arguments...)
			mutate(&request)
			if result, err := ExecuteSystemTool(context.Background(), request); !errors.Is(err, ErrRequestInvalid) ||
				!zeroProcessAuthorityResult(result) {
				t.Fatalf("open system-tool surface survived: result=%#v err=%v", result, err)
			}
		})
	}
}

func TestExecuteDarwinCodeSignSystemToolRejectsDirectoryReplacement(t *testing.T) {
	workingDirectory := canonicalDarwinTestPath(t, t.TempDir())
	workingAuthority, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer workingAuthority.Close()
	retained := workingDirectory + "-retained"
	if err := os.Rename(workingDirectory, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workingDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	result, runErr := ExecuteSystemTool(context.Background(), SystemToolRequest{
		Tool:                      SystemToolDarwinCodeSign,
		Arguments:                 []string{"--display", "--verbose=4", "+" + strconv.Itoa(os.Getpid())},
		WorkingDirectory:          workingDirectory,
		WorkingDirectoryAuthority: workingAuthority,
		Timeout:                   time.Second,
	})
	if !zeroProcessAuthorityResult(result) || !errors.Is(runErr, ErrWorkingDirectory) {
		t.Fatalf("replaced working directory survived: result=%#v err=%v", result, runErr)
	}
	if entries, err := os.ReadDir(workingDirectory); err != nil || len(entries) != 0 {
		t.Fatalf("replacement working directory was mutated: entries=%#v err=%v", entries, err)
	}
}

func zeroProcessAuthorityResult(result Result) bool {
	return result.ExitCode == 0 && result.TerminationStatus == "" && result.Signal == 0 &&
		len(result.Stdout) == 0 && len(result.Stderr) == 0
}

func TestSuspendedTerminationFailureClearsResultAndPoisonsAuthority(t *testing.T) {
	darwinAuthorityPoisoned.Store(false)
	t.Cleanup(func() { darwinAuthorityPoisoned.Store(false) })
	result := Result{ExitCode: 7, Stdout: []byte("must-not-survive")}
	runErr := ErrExecutableIdentity

	finalizeSuspendedDarwinTermination(false, nil, 0, &result, &runErr)
	if !errors.Is(runErr, ErrTermination) || !zeroProcessAuthorityResult(result) {
		t.Fatalf("termination failure survived as result=%#v err=%v", result, runErr)
	}
	poisonDarwinAuthorityOnTermination(runErr)
	if !darwinAuthorityPoisoned.Load() {
		t.Fatal("termination failure did not poison the process authority")
	}
}

func TestStageDarwinExecutableUsesPinnedSourceAfterPathSwap(t *testing.T) {
	sourceDirectory := canonicalDarwinTestPath(t, t.TempDir())
	stagingRoot := canonicalDarwinTestPath(t, t.TempDir())
	stagingAuthority, err := os.Open(stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer stagingAuthority.Close()
	sourcePath := filepath.Join(sourceDirectory, "helper")
	movedPath := filepath.Join(sourceDirectory, "helper-opened")
	original := []byte("opened executable bytes")
	replacement := []byte("replacement path bytes")
	if err := os.WriteFile(sourcePath, original, 0o500); err != nil {
		t.Fatalf("write source: %v", err)
	}
	pinned, err := openPinnedDarwinObject(sourcePath, false)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	defer pinned.file.Close()
	if err := os.Rename(sourcePath, movedPath); err != nil {
		t.Fatalf("rename source: %v", err)
	}
	if err := os.WriteFile(sourcePath, replacement, 0o500); err != nil {
		t.Fatalf("write replacement: %v", err)
	}
	digest := sha256.Sum256(original)
	_, staged, cleanup, err := stageDarwinExecutableWithAuthority(
		context.Background(),
		stagingRoot,
		stagingAuthority,
		pinned.file,
		int64(len(original)),
		digest,
	)
	if err != nil {
		t.Fatalf("stage pinned source: %v", err)
	}
	defer staged.Close()
	defer cleanup()
	payload, err := io.ReadAll(staged)
	if err != nil {
		t.Fatalf("read staged source: %v", err)
	}
	if !bytes.Equal(payload, original) || bytes.Equal(payload, replacement) {
		t.Fatalf("staged payload = %q, want exact opened bytes %q", payload, original)
	}
}

func TestLoadedDarwinExecutableRejectsStagePathSubstitution(t *testing.T) {
	executable := canonicalDarwinTestExecutable(t)
	source, err := os.Open(executable)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}
	digestBytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	digest := sha256.Sum256(digestBytes)
	stagingRoot := canonicalDarwinTestPath(t, t.TempDir())
	stagingAuthority, err := os.Open(stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer stagingAuthority.Close()
	stagedPath, stagedFile, cleanup, err := stageDarwinExecutableWithAuthority(
		context.Background(),
		stagingRoot,
		stagingAuthority,
		source,
		info.Size(),
		digest,
	)
	if err != nil {
		t.Fatalf("stage executable: %v", err)
	}
	defer stagedFile.Close()
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup stage: %v", err)
		}
	}()
	var stagedStat unix.Stat_t
	if unix.Fstat(int(stagedFile.Fd()), &stagedStat) != nil {
		t.Fatal("stat staged executable")
	}
	stagedObject := &pinnedDarwinObject{file: stagedFile, stat: stagedStat}
	originalPath := stagedPath + "-opened"
	if err := os.Rename(stagedPath, originalPath); err != nil {
		t.Fatalf("move staged executable: %v", err)
	}
	if err := os.WriteFile(stagedPath, digestBytes, 0o500); err != nil {
		t.Fatalf("write same-byte replacement: %v", err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		_ = os.Remove(stagedPath)
		_ = os.Rename(originalPath, stagedPath)
		restored = true
	}
	defer restore()

	workingDirectory, err := os.Open(canonicalDarwinTestPath(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open cwd: %v", err)
	}
	defer workingDirectory.Close()
	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer devNull.Close()
	pid, err := spawnSuspended(
		stagedPath,
		[]string{stagedPath, "-test.run=TestProcessAuthorityHelper", "--", "sleep"},
		[]string{"LANG=C"},
		int(workingDirectory.Fd()),
		int(devNull.Fd()),
		int(devNull.Fd()),
		int(devNull.Fd()),
	)
	if err != nil {
		t.Fatalf("spawn replacement: %v", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		killAndWaitDarwinProcess(nil, pid)
		t.Fatalf("find replacement process: %v", err)
	}
	defer killAndWaitDarwinProcess(process, pid)
	if loadedDarwinExecutableMatchesOpenedFile(context.Background(), pid, stagedObject, digest) {
		t.Fatal("same-byte replacement vnode was accepted as the opened staged executable")
	}
	restore()
}

func TestProcessAuthorityHelper(t *testing.T) {
	mode := ""
	for index, value := range os.Args {
		if value == "--" && index+1 < len(os.Args) {
			mode = os.Args[index+1]
			break
		}
	}
	if mode == "" {
		return
	}
	switch mode {
	case "sleep":
		time.Sleep(30 * time.Second)
	case "session", "session-extra-readiness", "session-hang", "session-setsid-descendant", "session-spawn-setsid-on-request", "session-exec-swap-back", "session-readonly-fd", "session-readwrite-fd", "session-owner-liveness-fd":
		swappedRequestID := os.Getenv("ANALYTIX_SESSION_EXEC_SWAP_REQUEST_ID")
		if mode == "session-readonly-fd" || mode == "session-readwrite-fd" {
			if err := validateInheritedReadOnlyFixtureFD(); err != nil {
				os.Exit(30)
			}
		}
		if mode == "session-readwrite-fd" {
			if err := validateInheritedReadWriteFixtureFD(); err != nil {
				os.Exit(32)
			}
		}
		if mode == "session-owner-liveness-fd" {
			if err := validateInheritedOwnerLivenessFD(); err != nil {
				os.Exit(31)
			}
		}
		if mode == "session-setsid-descendant" {
			if err := writeDarwinTargetContainmentProof(); err != nil {
				os.Exit(24)
			}
		}
		pid := os.Getpid()
		if swappedRequestID != "" {
			_ = os.Unsetenv("ANALYTIX_SESSION_EXEC_SWAP_REQUEST_ID")
			fmt.Printf("{\"id\":\"%s\",\"processId\":%d,\"sequence\":1}\n", swappedRequestID, pid)
		} else if mode == "session-setsid-descendant" {
			fmt.Println(`{"target_claims_containment":true}`)
		} else {
			fmt.Printf(
				"{\"protocolVersion\":\"%s\",\"launchNonce\":\"%s\",\"processId\":%d,\"ready\":true}\n",
				os.Getenv("ANALYTIX_NATIVE_PROTOCOL_VERSION"),
				os.Getenv("ANALYTIX_NATIVE_LAUNCH_NONCE"),
				pid,
			)
		}
		if mode == "session-extra-readiness" {
			fmt.Println(`{"unexpected":"second-readiness-frame"}`)
			time.Sleep(30 * time.Second)
		}
		reader := bufio.NewReader(os.Stdin)
		sequence := 0
		spawnedDetached := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				os.Exit(0)
			}
			if mode == "session-hang" {
				time.Sleep(30 * time.Second)
			}
			if mode == "session-spawn-setsid-on-request" && !spawnedDetached {
				if err := writeDarwinTargetContainmentProof(); err != nil {
					os.Exit(25)
				}
				spawnedDetached = true
			}
			sequence++
			requestID := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), `{"id":"`), `"}`)
			if mode == "session-exec-swap-back" && swappedRequestID == "" {
				environment := append(os.Environ(), "ANALYTIX_SESSION_EXEC_SWAP_REQUEST_ID="+requestID)
				arguments := append([]string{"/bin/sh", "-c", `exec "$0" "$@"`, os.Args[0]}, os.Args[1:]...)
				if err := syscall.Exec("/bin/sh", arguments, environment); err != nil {
					os.Exit(29)
				}
			}
			fmt.Printf("{\"id\":\"%s\",\"processId\":%d,\"sequence\":%d}\n", requestID, pid, sequence)
		}
	case "session-partial-readiness":
		fmt.Print(`{"protocolVersion":"1"`)
	case "detached-sleeper":
		if syscall.Getpgrp() != os.Getpid() {
			os.Exit(28)
		}
		acknowledgement := os.NewFile(3, "detached-ready")
		if acknowledgement == nil {
			os.Exit(26)
		}
		if _, err := acknowledgement.Write([]byte{1}); err != nil {
			_ = acknowledgement.Close()
			os.Exit(27)
		}
		_ = acknowledgement.Close()
		for {
			time.Sleep(30 * time.Second)
		}
	default:
		os.Exit(19)
	}
	os.Exit(0)
}

func validateInheritedReadOnlyFixtureFD() error {
	file := os.NewFile(darwinReadOnlyInputTargetFD, "inherited-private-input")
	if file == nil {
		return errors.New("inherited input is unavailable")
	}
	defer file.Close()
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return errors.New("inherited input is not read-only")
	}
	var stat unix.Stat_t
	if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Nlink != 0 || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("inherited input identity is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, 128))
	if err != nil || string(body) != "exact-private-snapshot" {
		return errors.New("inherited input bytes are invalid")
	}
	_, err = file.Seek(0, io.SeekStart)
	return err
}

func validateInheritedReadWriteFixtureFD() error {
	file := os.NewFile(darwinReadWriteOutputTargetFD, "inherited-private-output")
	if file == nil {
		return errors.New("inherited output is unavailable")
	}
	defer file.Close()
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
	var stat unix.Stat_t
	if err != nil || descriptorErr != nil || flags&unix.O_ACCMODE != unix.O_RDWR ||
		descriptorFlags&unix.FD_CLOEXEC != 0 || unix.Fstat(int(file.Fd()), &stat) != nil ||
		stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 0 ||
		stat.Mode&0o7777 != 0o600 || stat.Size != 0 || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("inherited output identity is invalid")
	}
	if _, err := file.Write([]byte("exact-private-output")); err != nil {
		return err
	}
	return file.Sync()
}

func validateInheritedOwnerLivenessFD() error {
	flags, err := unix.FcntlInt(darwinOwnerLivenessTargetFD, unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY || flags&unix.O_NONBLOCK != 0 {
		return errors.New("owner liveness is not blocking read-only")
	}
	descriptorFlags, err := unix.FcntlInt(darwinOwnerLivenessTargetFD, unix.F_GETFD, 0)
	if err != nil || descriptorFlags&unix.FD_CLOEXEC != 0 {
		return errors.New("owner liveness did not survive the controlled exec")
	}
	var stat unix.Stat_t
	if unix.Fstat(darwinOwnerLivenessTargetFD, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFIFO ||
		stat.Nlink != 0 || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("owner liveness is not an anonymous same-owner pipe")
	}
	var limit unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_NPROC, &limit) != nil || limit.Cur != 0 || limit.Max != 0 {
		return errors.New("owner liveness target lost the inherited nproc floor")
	}
	for fd := darwinOwnerLivenessTargetFD + 1; fd < 256; fd++ {
		var candidate unix.Stat_t
		if unix.Fstat(fd, &candidate) == nil && componentDarwinFileIdentity(candidate) == componentDarwinFileIdentity(stat) {
			return errors.New("owner liveness read endpoint was inherited more than once")
		}
	}
	return nil
}

func writeDarwinTargetContainmentProof() error {
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NPROC, &limit); err != nil || limit.Cur != 0 || limit.Max != 0 {
		return errors.New("target did not inherit zero process limit")
	}
	ordinary := exec.Command("/usr/bin/true")
	ordinaryErr := ordinary.Run()
	_, directSetsidErr := syscall.Setsid()
	setsid := exec.Command("/usr/bin/true")
	setsid.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	setsidErr := setsid.Run()
	doubleFork := exec.Command("/bin/sh", "-c", "(exit 0) & wait")
	doubleForkErr := doubleFork.Run()
	raiseErr := unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: 1, Max: 1})
	if !errors.Is(ordinaryErr, syscall.EAGAIN) || !errors.Is(directSetsidErr, syscall.EPERM) ||
		!errors.Is(setsidErr, syscall.EAGAIN) ||
		!errors.Is(doubleForkErr, syscall.EAGAIN) || raiseErr == nil {
		return fmt.Errorf(
			"containment failure: fork=%v setsid=%v setsid_spawn=%v double_fork=%v raise=%v",
			ordinaryErr, directSetsidErr, setsidErr, doubleForkErr, raiseErr,
		)
	}
	if err := unix.Getrlimit(unix.RLIMIT_NPROC, &limit); err != nil || limit.Cur != 0 || limit.Max != 0 {
		return errors.New("target process limit was raised")
	}
	return os.WriteFile(
		"containment-proof.txt",
		[]byte("soft=0 hard=0 fork=EAGAIN setsid=EPERM setsid_spawn=EAGAIN double_fork=EAGAIN raise=denied\n"),
		0o600,
	)
}

func spawnDetachedTestSleeper() int {
	nullFile, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return 0
	}
	defer nullFile.Close()
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		return 0
	}
	defer readyReader.Close()
	command := exec.Command(os.Args[0], "-test.run=TestProcessAuthorityHelper", "--", "detached-sleeper")
	command.Env = []string{"LANG=C", "LC_ALL=C", "TZ=UTC"}
	command.Stdin, command.Stdout, command.Stderr = nullFile, nullFile, nullFile
	command.ExtraFiles = []*os.File{readyWriter}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		_ = readyWriter.Close()
		return 0
	}
	_ = readyWriter.Close()
	_ = readyReader.SetReadDeadline(time.Now().Add(5 * time.Second))
	acknowledgement := []byte{0}
	read, readErr := readyReader.Read(acknowledgement)
	_ = readyReader.SetReadDeadline(time.Time{})
	if readErr == nil && read == 1 && acknowledgement[0] == 1 {
		return command.Process.Pid
	}
	_ = command.Process.Kill()
	_, _ = command.Process.Wait()
	return 0
}

func canonicalDarwinTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", path, err)
	}
	if err := os.Chmod(canonical, 0o700); err != nil {
		t.Fatalf("chmod %s: %v", canonical, err)
	}
	return canonical
}

func darwinTestFileSHA256(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func canonicalDarwinTestExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return canonicalDarwinTestPath(t, executable)
}
