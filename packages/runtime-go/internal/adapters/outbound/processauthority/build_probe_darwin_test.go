//go:build darwin && analytix_native_build_probe && !analytix_prod

package processauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type buildProbeCommandResult struct {
	stdout  []byte
	stderr  []byte
	err     error
	elapsed time.Duration
}

type buildProbeProcessTree struct {
	coordinator darwinProcessIdentity
	guardian    darwinProcessIdentity
	target      darwinProcessIdentity
}

func TestBuildProbeSystemTempParentMeetsStrictMetadataBoundary(t *testing.T) {
	parent, err := openPinnedDarwinObject(buildProbeTempParent, true)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.file.Close()
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(parent.file.Fd()), &stat); err != nil {
		t.Fatal(err)
	}
	xattrs, xattrErr := unix.Flistxattr(int(parent.file.Fd()), nil)
	if !darwinBuildProbeObjectSafe(parent.file, unix.S_IFDIR, 0o1777, 0) {
		t.Fatalf("unsafe fixed temp parent: mode=%#o uid=%d flags=%#x xattrs=%d xattr_err=%v acl_safe=%v",
			stat.Mode, stat.Uid, stat.Flags, xattrs, xattrErr, darwinBuildProbeHasNoExtendedACL(int(parent.file.Fd())))
	}
}

func TestBuildProbeRequestReadsOneClosedPipeFrame(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	payload := encodeBuildProbeTestRequest(t, validBuildProbeTestRequest(BuildProbeDataEngine))
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := readBuildProbeRequest(ctx, reader); err != nil {
		t.Fatalf("closed pipe frame rejected: %v", err)
	}
}

func TestBuildProbeAuthorityIdentityMatchesExactOpenedCurrentImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	identity, err := currentBuildProbeAuthorityIdentity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wantSHA256, wantSize := buildProbeHashAndSize(t, executable)
	if identity.SHA256 != wantSHA256 || identity.Size != wantSize || identity.HostTarget != buildProbeHostTarget() ||
		!validBuildProbeAuthorityIdentity(identity) {
		t.Fatalf("current identity = %+v, want sha=%s size=%d target=%s", identity, wantSHA256, wantSize, buildProbeHostTarget())
	}
}

func TestBuildProbeTargetDescriptorValidatesCallerDigestAndSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact")
	payload := []byte("descriptor-bound-build-artifact")
	if err := os.WriteFile(path, payload, 0o500); err != nil {
		t.Fatal(err)
	}
	target, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	digest := sha256.Sum256(payload)
	request := validBuildProbeTestRequest(BuildProbeDataEngine)
	request.ExpectedExecutableSHA256 = hex.EncodeToString(digest[:])
	request.ExpectedExecutableSize = int64(len(payload))
	if err := validateBuildProbeTargetDescriptor(context.Background(), target, request); err != nil {
		t.Fatalf("valid pinned descriptor rejected: %v", err)
	}
	wrongDigest := request
	wrongDigest.ExpectedExecutableSHA256 = buildProbeTestTarget
	if err := validateBuildProbeTargetDescriptor(context.Background(), target, wrongDigest); !errors.Is(err, ErrExecutableIdentity) {
		t.Fatalf("digest mismatch error = %v", err)
	}
	wrongSize := request
	wrongSize.ExpectedExecutableSize++
	if err := validateBuildProbeTargetDescriptor(context.Background(), target, wrongSize); !errors.Is(err, ErrExecutableIdentity) {
		t.Fatalf("size mismatch error = %v", err)
	}
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := validateBuildProbeTargetDescriptor(context.Background(), directory, request); !errors.Is(err, ErrExecutableIdentity) {
		t.Fatalf("directory descriptor error = %v", err)
	}
}

func TestBuildProbePrivateDirectoriesAreStrictAndRemovable(t *testing.T) {
	directories, err := openBuildProbePrivateDirectories()
	if err != nil {
		t.Fatal(err)
	}
	if !directories.valid() || !buildProbeDirectoryEmpty(directories.workAuthority) ||
		!buildProbeDirectoryEmpty(directories.stageAuthority) {
		t.Fatal("private directories did not retain exact authority")
	}
	root := directories.root
	if err := directories.close(); err != nil {
		t.Fatalf("close private directories: %v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private root still exists: %v", err)
	}
}

func TestBuildProbePrivateDirectoriesRejectACLXattrFlagsAndModeDrift(t *testing.T) {
	for _, mutation := range []string{"inherited-acl", "xattr", "flags", "mode"} {
		t.Run(mutation, func(t *testing.T) {
			directories, err := openBuildProbePrivateDirectories()
			if err != nil {
				t.Fatal(err)
			}
			restore := func() {}
			switch mutation {
			case "inherited-acl":
				permission := "everyone allow list,search,add_file,delete_child,file_inherit,directory_inherit"
				if output, err := exec.Command("chmod", "+a", permission, directories.stage).CombinedOutput(); err != nil {
					t.Fatalf("install inherited ACL fixture: %v: %s", err, output)
				}
				restore = func() { _, _ = exec.Command("chmod", "-N", directories.stage).CombinedOutput() }
			case "xattr":
				if err := unix.Fsetxattr(int(directories.workAuthority.Fd()), "com.analytix.injected", []byte("unsafe"), 0); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fremovexattr(int(directories.workAuthority.Fd()), "com.analytix.injected") }
			case "flags":
				if err := unix.Fchflags(int(directories.workAuthority.Fd()), unix.UF_HIDDEN); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fchflags(int(directories.workAuthority.Fd()), 0) }
			case "mode":
				if err := unix.Fchmod(int(directories.workAuthority.Fd()), 0o710); err != nil {
					t.Fatal(err)
				}
				restore = func() { _ = unix.Fchmod(int(directories.workAuthority.Fd()), 0o700) }
			}
			if directories.valid() {
				t.Fatalf("%s metadata drift was accepted", mutation)
			}
			restore()
			if err := directories.close(); err != nil {
				t.Fatalf("close restored directories: %v", err)
			}
		})
	}
}

func TestBuildProbeCreatedDirectoryProvenanceIsStable(t *testing.T) {
	identifier, err := randomBuildProbeID()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(buildProbeTempParent, "analytix-provenance-tree-"+identifier)
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	rootFile, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer rootFile.Close()
	rootProvenance, rootOK := darwinBuildProbeProvenance(int(rootFile.Fd()))
	if !rootOK {
		t.Fatalf("root provenance invalid: %x", rootProvenance)
	}
	workFile, err := createBuildProbePrivateDirectory(int(rootFile.Fd()), "work", rootProvenance)
	if err != nil {
		t.Fatal(err)
	}
	defer workFile.Close()
	workProvenance, workOK := darwinBuildProbeProvenance(int(workFile.Fd()))
	if !rootOK || !workOK || !bytes.Equal(rootProvenance, workProvenance) {
		t.Fatalf("created provenance drifted: root=%x/%v work=%x/%v", rootProvenance, rootOK, workProvenance, workOK)
	}
}

func TestBuildProbeAllowsOnlyBoundKernelManagedProvenance(t *testing.T) {
	identifier, err := randomBuildProbeID()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(buildProbeTempParent, "analytix-build-probe-metadata-"+identifier)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	provenance, ok := darwinBuildProbeProvenance(int(file.Fd()))
	if !ok || !darwinBuildProbeCreatedObjectSafe(file, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), provenance) {
		t.Fatalf("kernel-managed provenance was not accepted: %x", provenance)
	}
	if len(provenance) == 0 {
		t.Skip("this Darwin version did not inject com.apple.provenance")
	}
	hostile := append([]byte(nil), provenance...)
	hostile[len(hostile)-1] ^= 0xff
	setErr := unix.Fsetxattr(int(file.Fd()), darwinBuildProbeProvenanceName, hostile, 0)
	after, afterOK := darwinBuildProbeProvenance(int(file.Fd()))
	if !afterOK || !bytes.Equal(after, provenance) || bytes.Equal(after, hostile) {
		t.Fatalf("caller controlled kernel provenance: set_err=%v before=%x hostile=%x after=%x", setErr, provenance, hostile, after)
	}
	if err := unix.Fsetxattr(int(file.Fd()), "com.analytix.injected", []byte("attacker-controlled"), 0); err != nil {
		t.Fatalf("install extra xattr fixture: %v", err)
	}
	if darwinBuildProbeCreatedObjectSafe(file, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), provenance) {
		t.Fatal("created authority accepted an additional xattr")
	}
}

func TestBuildProbeRejectsProvenanceFormatVariants(t *testing.T) {
	valid := []byte{0x01, 0x02, 0x00, 1, 2, 3, 4, 5, 6, 7, 8}
	if !validDarwinBuildProbeProvenance(valid) {
		t.Fatal("documented Darwin provenance shape was rejected")
	}
	for name, value := range map[string][]byte{
		"short":       valid[:len(valid)-1],
		"version":     append([]byte{0x02}, valid[1:]...),
		"record type": append([]byte{0x01, 0x03}, valid[2:]...),
		"reserved":    append([]byte{0x01, 0x02, 0x01}, valid[3:]...),
		"zero token":  {0x01, 0x02, 0x00, 0, 0, 0, 0, 0, 0, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if validDarwinBuildProbeProvenance(value) {
				t.Fatalf("accepted provenance variant: %x", value)
			}
		})
	}
}

func TestBuildProbeStrictExecutableStagingLifecycle(t *testing.T) {
	directories, err := openBuildProbePrivateDirectories()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := directories.close(); err != nil {
			t.Errorf("close directories: %v", err)
		}
	}()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	hash, size := buildProbeHashAndSize(t, executable)
	digest, err := hex.DecodeString(hash)
	if err != nil {
		t.Fatal(err)
	}
	var expected [sha256.Size]byte
	copy(expected[:], digest)
	_, staged, cleanup, err := stageDarwinBuildProbeExecutableWithAuthority(
		context.Background(), directories.stage, directories.stageAuthority, source, size, expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !darwinBuildProbeCreatedObjectSafe(staged, unix.S_IFREG, 0o500, uint32(os.Geteuid()), directories.provenance) {
		t.Fatal("staged payload lost exact 0500/provenance authority")
	}
	if err := staged.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildProbeFrozenCWDIsReadFromKernelVnodeAuthority(t *testing.T) {
	resetDarwinSessionAuthority(t)
	opened, _, workingDirectory := openDarwinTestSessionWithPaths(t, "session")
	authenticateDarwinTestSession(t, opened)
	session, ok := opened.(*darwinSession)
	if !ok {
		t.Fatalf("session type = %T", opened)
	}
	authority, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.acquire(ctx); err != nil {
		t.Fatal(err)
	}
	defer session.release()
	if err := session.tree.Freeze(ctx); err != nil {
		t.Fatal(err)
	}
	if !darwinBuildProbeFrozenCWDMatches(ctx, session.pid, workingDirectory, authority) {
		t.Fatal("kernel cwd vnode did not bind to retained working-directory authority")
	}
	if err := session.tree.ResumeRoot(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestNativeBuildProbeAuthenticatesAllFourHiddenProtocols(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	probeHash, _ := buildProbeHashAndSize(t, probe)
	cases := []struct {
		component BuildProbeComponent
		engine    string
	}{
		{BuildProbeImportAccelerator, "analytix-import-accelerator"},
		{BuildProbeCleaningOps, "analytix-cleaning-ops"},
		{BuildProbeAnalysisCompute, "analytix-analysis-compute"},
		{BuildProbeDataEngine, "analytix-data-engine"},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.component), func(t *testing.T) {
			target := buildProbeBuildFixture(t, moduleRoot, testCase.component, testCase.engine, "success")
			request := buildProbeRequestForTarget(t, testCase.component, target, "success")
			result := runBuildProbeTestCommand(t, probe, target, request, 15*time.Second)
			if result.err != nil {
				t.Fatalf("probe failed: %v stderr=%q", result.err, result.stderr)
			}
			if len(result.stderr) != 0 {
				t.Fatalf("successful probe wrote stderr: %q", result.stderr)
			}
			var receipt BuildProbeReceipt
			decoder := json.NewDecoder(bytes.NewReader(result.stdout))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&receipt) != nil || !validBuildProbeReceipt(receipt, request) {
				t.Fatalf("invalid receipt: %q", result.stdout)
			}
			if receipt.AuthoritySHA256 != probeHash {
				t.Fatalf("authority hash = %q, want loaded coordinator hash %q", receipt.AuthoritySHA256, probeHash)
			}
			if result.elapsed >= buildProbeDeadline {
				t.Fatalf("successful probe took %s", result.elapsed)
			}
		})
	}
}

func TestProbePinnedBuildArtifactRejectsCallerDigestAndSizeMismatch(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	target := buildProbeBuildFixture(
		t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", "success",
	)
	valid := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, "descriptor-mismatch")
	for name, request := range map[string]BuildProbeRequestV1{
		"digest": func() BuildProbeRequestV1 {
			request := valid
			request.ExpectedExecutableSHA256 = buildProbeTestTarget
			return request
		}(),
		"size": func() BuildProbeRequestV1 {
			request := valid
			request.ExpectedExecutableSize++
			return request
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			result := runBuildProbeTestCommand(t, probe, target, request, 5*time.Second)
			if result.err == nil || len(result.stdout) != 0 || len(result.stderr) != 0 {
				t.Fatalf("descriptor mismatch failed open: err=%v stdout=%q stderr=%q", result.err, result.stdout, result.stderr)
			}
			if result.elapsed >= buildProbeDeadline {
				t.Fatalf("descriptor mismatch was not rejected before execution: %s", result.elapsed)
			}
		})
	}
}

func TestNativeBuildProbeRejectsTargetCrashAndForgedProtocol(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	for _, mode := range []string{"crash-after-ready", "bad-ready", "bad-response"} {
		t.Run(mode, func(t *testing.T) {
			target := buildProbeBuildFixture(
				t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", mode,
			)
			request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, mode)
			result := runBuildProbeTestCommand(t, probe, target, request, 15*time.Second)
			if result.err == nil {
				t.Fatalf("%s target produced an accepted receipt: %q", mode, result.stdout)
			}
			if len(result.stdout) != 0 || len(result.stderr) != 0 {
				t.Fatalf("failed probe exposed output: stdout=%q stderr=%q", result.stdout, result.stderr)
			}
			if result.elapsed >= buildProbeDeadline {
				t.Fatalf("%s was not rejected promptly: %s", mode, result.elapsed)
			}
		})
	}
}

func TestNativeBuildProbeTerminalFreezeRejectsMaliciousResponseChannelsWithoutOrphan(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	for _, mode := range []string{
		"sigcont-handler", "extra-stdout", "extra-stderr", "flood-after-response", "wrong-cwd",
	} {
		t.Run(mode, func(t *testing.T) {
			target := buildProbeBuildFixture(
				t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", mode,
			)
			request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, mode)
			targetHash, _ := buildProbeHashAndSize(t, target)
			command, stdout, stderr := startBuildProbeTestCommand(t, probe, target, request)
			tree := waitBuildProbeProcessTree(t, command.Process.Pid, targetHash)
			if err := waitBuildProbeCommand(command, 5*time.Second); err == nil {
				t.Fatalf("%s produced an accepted receipt: %q", mode, stdout.Bytes())
			}
			assertBuildProbeIdentityGone(t, tree.guardian)
			assertBuildProbeIdentityGone(t, tree.target)
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("%s exposed output: stdout=%q stderr=%q", mode, stdout.Bytes(), stderr.Bytes())
			}
		})
	}
}

func TestNativeBuildProbeCommandAcceptsNoTargetPathOrExtraArgument(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	target := buildProbeBuildFixture(
		t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", "success",
	)
	request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, "closed-command")

	for _, testCase := range []struct {
		name       string
		arguments  []string
		withTarget bool
	}{
		{name: "missing inherited fd3"},
		{name: "target path argument", arguments: []string{target}, withTarget: true},
		{name: "help argument", arguments: []string{"--help"}, withTarget: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.Command(probe, testCase.arguments...)
			command.Env = []string{"LANG=C", "LC_ALL=C", "TZ=UTC"}
			command.Stdin = bytes.NewReader(encodeBuildProbeTestRequest(t, request))
			var targetFile *os.File
			if testCase.withTarget {
				var err error
				targetFile, err = os.Open(target)
				if err != nil {
					t.Fatalf("open target: %v", err)
				}
				command.ExtraFiles = []*os.File{targetFile}
			}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if targetFile != nil {
				_ = targetFile.Close()
			}
			if err == nil || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("command surface failed open: err=%v stdout=%q stderr=%q", err, stdout.Bytes(), stderr.Bytes())
			}
		})
	}
}

func TestNativeBuildProbeIgnoresUntrustedTMPDIR(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	target := buildProbeBuildFixture(
		t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", "success",
	)
	request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, "untrusted-tmpdir")
	trap := t.TempDir()
	if err := os.Chmod(trap, 0o777); err != nil {
		t.Fatal(err)
	}
	result := runBuildProbeTestCommandWithEnvironment(
		t, probe, target, request, 15*time.Second,
		[]string{"LANG=C", "LC_ALL=C", "TZ=UTC", "TMPDIR=" + trap},
	)
	if result.err != nil || len(result.stderr) != 0 {
		t.Fatalf("fixed temp authority failed: err=%v stdout=%q stderr=%q", result.err, result.stdout, result.stderr)
	}
	entries, err := os.ReadDir(trap)
	if err != nil || len(entries) != 0 {
		t.Fatalf("untrusted TMPDIR was used: entries=%+v err=%v", entries, err)
	}
}

func TestNativeBuildProbeTimeoutLeavesNoReceipt(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	target := buildProbeBuildFixture(
		t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", "hang-ready",
	)
	request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, "timeout")
	result := runBuildProbeTestCommand(t, probe, target, request, 15*time.Second)
	if result.err == nil || len(result.stdout) != 0 || len(result.stderr) != 0 {
		t.Fatalf("timeout did not fail closed: err=%v stdout=%q stderr=%q", result.err, result.stdout, result.stderr)
	}
	if result.elapsed < 9*time.Second || result.elapsed > 13*time.Second {
		t.Fatalf("timeout duration = %s, want the fixed 10 second authority window", result.elapsed)
	}
}

func TestNativeBuildProbePartialStdinKeptOpenUsesFixedDeadlineWithoutTree(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	target := buildProbeBuildFixture(
		t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", "success",
	)
	request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, "partial-open-stdin")
	targetFile, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(probe)
	command.Env = []string{"LANG=C", "LC_ALL=C", "TZ=UTC"}
	command.ExtraFiles = []*os.File{targetFile}
	stdin, err := command.StdinPipe()
	if err != nil {
		_ = targetFile.Close()
		t.Fatal(err)
	}
	defer stdin.Close()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	if err := command.Start(); err != nil {
		_ = targetFile.Close()
		t.Fatal(err)
	}
	if err := targetFile.Close(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	payload := encodeBuildProbeTestRequest(t, request)
	partial := payload[:len(payload)/2]
	if bytes.IndexByte(partial, '\n') >= 0 {
		t.Fatal("partial fixture unexpectedly contains a terminator")
	}
	if _, err := stdin.Write(partial); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	observationDeadline := started.Add(9 * time.Second)
	for time.Now().Before(observationDeadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		coordinator, stateErr := readDarwinProcessState(ctx, command.Process.Pid, true)
		if stateErr != nil {
			cancel()
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("coordinator exited before fixed deadline: %v", stateErr)
		}
		children, childErr := listDarwinDirectChildren(ctx, coordinator.Identity)
		cancel()
		if childErr != nil || len(children) != 0 {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("unterminated stdin spawned a process tree: children=%+v err=%v", children, childErr)
		}
		time.Sleep(25 * time.Millisecond)
	}
	commandErr := waitBuildProbeCommand(command, 4*time.Second)
	elapsed := time.Since(started)
	if commandErr == nil || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unterminated stdin failed open: err=%v stdout=%q stderr=%q", commandErr, stdout.Bytes(), stderr.Bytes())
	}
	if elapsed < 9*time.Second || elapsed > 13*time.Second {
		t.Fatalf("unterminated stdin duration = %s, want fixed 10 second deadline", elapsed)
	}
}

func TestNativeBuildProbeCoordinatorCrashGuardianReapsTarget(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	target := buildProbeBuildFixture(
		t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", "hang-ready",
	)
	request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, "coordinator-crash")
	targetHash, _ := buildProbeHashAndSize(t, target)
	command, stdout, stderr := startBuildProbeTestCommand(t, probe, target, request)
	tree := waitBuildProbeProcessTree(t, command.Process.Pid, targetHash)
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill coordinator: %v", err)
	}
	if err := waitBuildProbeCommand(command, 5*time.Second); err == nil {
		t.Fatal("killed coordinator reported success")
	}
	assertBuildProbeIdentityGone(t, tree.guardian)
	assertBuildProbeIdentityGone(t, tree.target)
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("coordinator crash exposed output: stdout=%q stderr=%q", stdout.Bytes(), stderr.Bytes())
	}
}

func TestNativeBuildProbeGuardianCrashCoordinatorReapsTarget(t *testing.T) {
	moduleRoot := buildProbeModuleRoot(t)
	probe := buildProbeBuildCommand(t, moduleRoot)
	target := buildProbeBuildFixture(
		t, moduleRoot, BuildProbeImportAccelerator, "analytix-import-accelerator", "hang-ready",
	)
	request := buildProbeRequestForTarget(t, BuildProbeImportAccelerator, target, "guardian-crash")
	targetHash, _ := buildProbeHashAndSize(t, target)
	command, stdout, stderr := startBuildProbeTestCommand(t, probe, target, request)
	tree := waitBuildProbeProcessTree(t, command.Process.Pid, targetHash)
	if err := syscall.Kill(tree.guardian.PID, syscall.SIGKILL); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("kill guardian: %v", err)
	}
	if err := waitBuildProbeCommand(command, 5*time.Second); err == nil {
		t.Fatal("guardian crash produced an accepted receipt")
	}
	assertBuildProbeIdentityGone(t, tree.guardian)
	assertBuildProbeIdentityGone(t, tree.target)
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("guardian crash exposed output: stdout=%q stderr=%q", stdout.Bytes(), stderr.Bytes())
	}
}

func buildProbeModuleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../.."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve module root %q: %v", root, err)
	}
	return root
}

func buildProbeBuildCommand(t *testing.T, moduleRoot string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "native-component-build-probe")
	command := exec.Command(
		"go", "build", "-trimpath", "-tags", "analytix_native_build_probe",
		"-o", output, "./cmd/native-component-build-probe",
	)
	command.Dir = moduleRoot
	if body, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build probe command: %v\n%s", err, body)
	}
	return output
}

func buildProbeBuildFixture(
	t *testing.T,
	moduleRoot string,
	component BuildProbeComponent,
	engine string,
	mode string,
) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "native-component-fixture")
	linker := fmt.Sprintf("-X=main.component=%s -X=main.engine=%s -X=main.mode=%s", component, engine, mode)
	command := exec.Command(
		"go", "build", "-trimpath", "-tags", "analytix_native_build_probe_fixture",
		"-ldflags", linker, "-o", output,
		"./internal/adapters/outbound/processauthority/testdata/buildprobefixture",
	)
	command.Dir = moduleRoot
	if body, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, body)
	}
	return output
}

func buildProbeRequestForTarget(
	t *testing.T,
	component BuildProbeComponent,
	target string,
	discriminator string,
) buildProbeRequest {
	t.Helper()
	targetHash, targetSize := buildProbeHashAndSize(t, target)
	manifest := sha256.Sum256([]byte("manifest:" + string(component)))
	nonce := sha256.Sum256([]byte("nonce:" + string(component) + ":" + discriminator))
	request := validBuildProbeTestRequest(component)
	request.ManifestSHA256 = hex.EncodeToString(manifest[:])
	request.ExpectedExecutableSHA256 = targetHash
	request.ExpectedExecutableSize = targetSize
	request.RequestNonce = hex.EncodeToString(nonce[:])
	return request
}

func buildProbeHashAndSize(t *testing.T, path string) (string, int64) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatalf("hash %q: %v", path, err)
	}
	return hex.EncodeToString(hash.Sum(nil)), stat.Size()
}

func runBuildProbeTestCommand(
	t *testing.T,
	probe string,
	target string,
	request buildProbeRequest,
	limit time.Duration,
) buildProbeCommandResult {
	return runBuildProbeTestCommandWithEnvironment(
		t, probe, target, request, limit, []string{"LANG=C", "LC_ALL=C", "TZ=UTC"},
	)
}

func runBuildProbeTestCommandWithEnvironment(
	t *testing.T,
	probe string,
	target string,
	request buildProbeRequest,
	limit time.Duration,
	environment []string,
) buildProbeCommandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	targetFile, err := os.Open(target)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer targetFile.Close()
	command := exec.CommandContext(ctx, probe)
	command.Env = append([]string(nil), environment...)
	command.ExtraFiles = []*os.File{targetFile}
	command.Stdin = bytes.NewReader(encodeBuildProbeTestRequest(t, request))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	started := time.Now()
	err = command.Run()
	return buildProbeCommandResult{
		stdout: append([]byte(nil), stdout.Bytes()...),
		stderr: append([]byte(nil), stderr.Bytes()...),
		err:    err, elapsed: time.Since(started),
	}
}

func startBuildProbeTestCommand(
	t *testing.T,
	probe string,
	target string,
	request buildProbeRequest,
) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	targetFile, err := os.Open(target)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	command := exec.Command(probe)
	command.Env = []string{"LANG=C", "LC_ALL=C", "TZ=UTC"}
	command.ExtraFiles = []*os.File{targetFile}
	command.Stdin = bytes.NewReader(encodeBuildProbeTestRequest(t, request))
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		_ = targetFile.Close()
		t.Fatalf("start probe: %v", err)
	}
	if err := targetFile.Close(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("close parent target descriptor: %v", err)
	}
	return command, stdout, stderr
}

func waitBuildProbeProcessTree(t *testing.T, coordinatorPID int, targetHash string) buildProbeProcessTree {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		coordinator, coordinatorErr := readDarwinProcessState(ctx, coordinatorPID, true)
		if coordinatorErr == nil {
			guardians, guardianErr := listDarwinDirectChildren(ctx, coordinator.Identity)
			if guardianErr == nil && len(guardians) == 1 {
				targets, targetErr := listDarwinDirectChildren(ctx, guardians[0].Identity)
				if targetErr == nil && len(targets) == 1 && buildProbeLoadedHash(ctx, targets[0].Identity.PID) == targetHash {
					cancel()
					return buildProbeProcessTree{
						coordinator: coordinator.Identity,
						guardian:    guardians[0].Identity,
						target:      targets[0].Identity,
					}
				}
			}
		}
		cancel()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for authenticated coordinator/guardian/target tree")
	return buildProbeProcessTree{}
}

func buildProbeLoadedHash(ctx context.Context, pid int) string {
	path, err := loadedDarwinProcessPath(pid)
	if err != nil {
		return ""
	}
	opened, err := openPinnedDarwinObject(path, false)
	if err != nil {
		return ""
	}
	defer opened.file.Close()
	digest, err := sha256OpenedFile(ctx, opened.file)
	if err != nil || !pinnedDarwinObjectUnchanged(opened) {
		return ""
	}
	return hex.EncodeToString(digest[:])
}

func waitBuildProbeCommand(command *exec.Cmd, limit time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		_ = command.Process.Kill()
		<-done
		return errors.New("build probe command did not exit")
	}
}

func assertBuildProbeIdentityGone(t *testing.T, identity darwinProcessIdentity) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := waitDarwinUnreusableIdentityESRCH(ctx, darwinUnreusableIdentityFrom(identity)); err != nil {
		t.Fatalf("pid %d identity remains reusable/alive: %v", identity.PID, err)
	}
}
