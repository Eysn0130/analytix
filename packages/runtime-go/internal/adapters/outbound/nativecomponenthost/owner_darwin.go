//go:build darwin

package nativecomponenthost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	nativecomponentrunner "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentrunner"
	packagedbuildauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	domainpackagedauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
	"golang.org/x/sys/unix"
)

const privateRuntimePrefix = ".native-component-runtime-v1-"

var privateRuntimeChildren = []string{"verification", "execution", "working"}

type privateRoots struct {
	mu          sync.Mutex
	parent      int
	parentPath  string
	runRoot     int
	runName     string
	children    []privateRootChild
	cleanupErr  error
	terminalErr error
	closed      bool
}

type privateRootChild struct {
	name     string
	file     *os.File
	unlinked bool
	closed   bool
}

// openDarwinHostCandidate assembles the release-trusted native host after the
// process authority has made the direct child fail-closed on owner loss and
// continuously checks its exact PID/PPID/PGID, image, fork, exec and exit
// identity. Trust and package layout are rejected before private roots exist.
func openDarwinHostCandidate(dataDir string) (ownerResult *Owner, returnErr error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, ErrTrust
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, ErrTrust
	}
	return openDarwinHostCandidateForExecutable(dataDir, executable)
}

func openDarwinHostCandidateForExecutable(dataDir string, executable string) (ownerResult *Owner, returnErr error) {
	canonicalExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable ||
		canonicalExecutable != executable {
		return nil, ErrTrust
	}
	trust := nativecomponentregistry.EmbeddedTrust()
	available, err := embeddedTrustState(trust)
	authorityUse := nativecomponentregistry.AuthorityUseRelease
	if err != nil || !available {
		localTrust, localErr := localBuildTrustForExecutable(context.Background(), trust, executable)
		if localErr != nil {
			if err == nil && !available && errors.Is(localErr, ErrUnavailable) {
				return nil, ErrUnavailable
			}
			return nil, localErr
		}
		trust = localTrust
		authorityUse = nativecomponentregistry.AuthorityUseLocalBuild
	}
	runtimeRoot, err := packagedDarwinRuntimeRoot(executable)
	if err != nil {
		return nil, err
	}
	roots, err := createPrivateRoots(dataDir)
	if err != nil {
		return nil, errors.Join(ErrLifecycle, err)
	}
	cleanupRoots := true
	defer func() {
		if cleanupRoots {
			if cleanupErr := roots.Close(); cleanupErr != nil {
				returnErr = errors.Join(returnErr, ErrLifecycle, cleanupErr)
			}
		}
	}()
	policy := nativecomponentregistry.PlatformPolicy{
		SHA256: trust.SigningPolicySHA256, Mode: trust.SigningMode,
		AppleTeamIdentifier: trust.AppleTeamIdentifier,
	}
	verifier, err := nativecomponentregistry.NewPlatformVerifier(nativecomponentregistry.PlatformVerifierConfig{
		WorkingDirectory: roots.childPath("working"), WorkingDirectoryAuthority: roots.authorityFile("working"),
		StagingRoot: roots.childPath("verification"), StagingRootAuthority: roots.authorityFile("verification"),
		Policy: policy,
	})
	if err != nil {
		return nil, errors.Join(ErrTrust, err)
	}
	registry, err := nativecomponentregistry.Load(nativecomponentregistry.Config{
		RuntimeRoot: runtimeRoot, Trust: trust, Verifier: verifier, AuthorityUse: authorityUse,
	})
	if err != nil {
		return nil, errors.Join(ErrTrust, err)
	}
	runner, err := nativecomponentrunner.New(nativecomponentrunner.Config{
		Registry:         registry,
		WorkingDirectory: roots.childPath("working"), WorkingDirectoryAuthority: roots.authorityFile("working"),
		StagingRoot: roots.childPath("execution"), StagingRootAuthority: roots.authorityFile("execution"),
	})
	if err != nil {
		closeErr := registry.Close()
		return nil, errors.Join(ErrLifecycle, err, closeErr)
	}
	owner, err := newOwner(runner, registry, roots)
	if err != nil {
		return nil, errors.Join(err, runner.Close(), registry.Close())
	}
	cleanupRoots = false
	return owner, nil
}

func currentLocalBuildTrust(ctx context.Context, embedded nativecomponentregistry.Trust) (nativecomponentregistry.Trust, error) {
	executable, err := os.Executable()
	if err != nil {
		return nativecomponentregistry.Trust{}, ErrTrust
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nativecomponentregistry.Trust{}, ErrTrust
	}
	return localBuildTrustForExecutable(ctx, embedded, executable)
}

func localBuildTrustForExecutable(
	ctx context.Context,
	embedded nativecomponentregistry.Trust,
	executable string,
) (nativecomponentregistry.Trust, error) {
	if embedded.ReceiptSHA256 != "" || embedded.ManifestSHA256 != "" || embedded.TargetKey != "" ||
		embedded.SigningMode != "ad-hoc" || embedded.AppleTeamIdentifier != "" ||
		embedded.SigningPolicySHA256 != "7625f1de94282690dc72ce92f7e42a293437c24fb9f4a725ff82bfec157ecaf3" {
		return nativecomponentregistry.Trust{}, ErrUnavailable
	}
	inspection, err := packagedbuildauthorityfs.InspectPackageV2(ctx, executable, runtime.GOOS, runtime.GOARCH)
	if err != nil || inspection.Authority.Development == nil || inspection.Authority.Controlled != nil ||
		inspection.Authority.DispositionKind() != domainpackagedauthority.DevelopmentDispositionKindV2 ||
		inspection.Publishable || inspection.FactToolsEnabled ||
		inspection.PackageAnchor != "macos_nonpublishable_resource_seal" {
		return nativecomponentregistry.Trust{}, ErrTrust
	}
	development := inspection.Authority.Development
	return nativecomponentregistry.Trust{
		ReceiptSHA256: development.Marker.SHA256, ManifestSHA256: inspection.AuthorityFileSHA256,
		TargetKey: development.TargetKey, SigningPolicySHA256: embedded.SigningPolicySHA256,
		SigningMode: embedded.SigningMode, AppleTeamIdentifier: embedded.AppleTeamIdentifier,
		Classification: inspection.Authority.Authority.Classification,
	}, nil
}

func LocalBuildPackageActive() bool {
	_, err := currentLocalBuildTrust(context.Background(), nativecomponentregistry.EmbeddedTrust())
	return err == nil
}

func packagedDarwinRuntimeRoot(executable string) (string, error) {
	runtimeRoot, err := PackagedRuntimeRoot(executable)
	if err != nil {
		return "", err
	}
	resourcesDirectory := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(executable))))
	contentsDirectory := filepath.Dir(resourcesDirectory)
	appDirectory := filepath.Dir(contentsDirectory)
	appName := filepath.Base(appDirectory)
	if filepath.Base(resourcesDirectory) != "Resources" || filepath.Base(contentsDirectory) != "Contents" ||
		len(appName) <= len(".app") || !strings.HasSuffix(appName, ".app") ||
		filepath.Join(resourcesDirectory, "runtime") != runtimeRoot {
		return "", ErrTrust
	}
	return runtimeRoot, nil
}

func createPrivateRoots(dataDir string) (*privateRoots, error) {
	return createPrivateRootsWithFchown(dataDir, unix.Fchown)
}

func createPrivateRootsWithFchown(dataDir string, chown func(int, int, int) error) (*privateRoots, error) {
	parent, clean, err := openPrivateDirectoryPath(dataDir)
	if err != nil {
		return nil, err
	}
	name, err := randomPrivateRuntimeName()
	if err != nil {
		_ = unix.Close(parent)
		return nil, err
	}
	if err := unix.Mkdirat(parent, name, 0o700); err != nil {
		_ = unix.Close(parent)
		return nil, err
	}
	rootCreated := true
	defer func() {
		if rootCreated {
			_ = unix.Unlinkat(parent, name, unix.AT_REMOVEDIR)
			_ = unix.Close(parent)
		}
	}()
	if err := unix.Fsync(parent); err != nil {
		return nil, err
	}
	runRoot, err := openPrivateChildDirectory(parent, name)
	if err != nil {
		return nil, err
	}
	roots := &privateRoots{parent: parent, parentPath: clean, runRoot: runRoot, runName: name}
	rootCreated = false
	if err := normalizeNewPrivateDirectoryGroup(parent, name, runRoot, chown); err != nil {
		return nil, errors.Join(err, roots.Close())
	}
	for _, child := range privateRuntimeChildren {
		childFile, childErr := createPrivateChildDirectory(runRoot, child, chown)
		if childErr != nil {
			return nil, errors.Join(childErr, roots.Close())
		}
		roots.children = append(roots.children, privateRootChild{name: child, file: childFile})
	}
	if err := unix.Fsync(runRoot); err != nil {
		return nil, errors.Join(err, roots.Close())
	}
	return roots, nil
}

func openPrivateDirectoryPath(path string) (int, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) || strings.ContainsRune(clean, 0) {
		return -1, "", ErrTrust
	}
	components := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, "", err
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = unix.Close(current)
			return -1, "", ErrTrust
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return -1, "", openErr
		}
		current = next
	}
	var stat unix.Stat_t
	if unix.Fstat(current, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o022 != 0 {
		_ = unix.Close(current)
		return -1, "", ErrTrust
	}
	return current, clean, nil
}

func createPrivateChildDirectory(parent int, name string, chown func(int, int, int) error) (_ *os.File, resultErr error) {
	if !validPrivateName(name) {
		return nil, ErrTrust
	}
	if err := unix.Mkdirat(parent, name, 0o700); err != nil {
		return nil, err
	}
	created := true
	child := -1
	defer func() {
		if created {
			if child < 0 || samePrivateDirectoryAt(parent, name, child) {
				resultErr = errors.Join(resultErr, unix.Unlinkat(parent, name, unix.AT_REMOVEDIR), unix.Fsync(parent))
			} else {
				resultErr = errors.Join(resultErr, ErrLifecycle)
			}
			if child >= 0 {
				resultErr = errors.Join(resultErr, unix.Close(child))
			}
		}
	}()
	if err := unix.Fsync(parent); err != nil {
		return nil, err
	}
	var err error
	child, err = openPrivateChildDirectory(parent, name)
	if err != nil {
		return nil, err
	}
	if err := normalizeNewPrivateDirectoryGroup(parent, name, child, chown); err != nil {
		return nil, err
	}
	if err := unix.Fsync(child); err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(child), name)
	if file == nil {
		return nil, ErrLifecycle
	}
	created = false
	return file, nil
}

// Only call for a newly created directory whose parent binding is still held.
// Darwin inherits the parent's group even when that differs from the runtime's
// effective group; snapshot consumers require the latter. Existing profiles and
// ancestors are never normalized.
func normalizeNewPrivateDirectoryGroup(parent int, name string, fd int, chown func(int, int, int) error) error {
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFDIR ||
		before.Uid != uint32(os.Geteuid()) || before.Mode&0o7777 != 0o700 || before.Nlink == 0 ||
		!samePrivateDirectoryAt(parent, name, fd) {
		return ErrTrust
	}
	group := os.Getegid()
	if before.Gid != uint32(group) {
		if err := chown(fd, -1, group); err != nil {
			return err
		}
	}
	var after unix.Stat_t
	if unix.Fstat(fd, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino ||
		before.Uid != after.Uid || before.Mode != after.Mode || before.Nlink != after.Nlink ||
		before.Flags != after.Flags || after.Gid != uint32(group) ||
		!samePrivateDirectoryAt(parent, name, fd) {
		return ErrTrust
	}
	return nil
}

func openPrivateChildDirectory(parent int, name string) (int, error) {
	if !validPrivateName(name) {
		return -1, ErrTrust
	}
	child, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if unix.Fstat(child, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		_ = unix.Close(child)
		return -1, ErrTrust
	}
	return child, nil
}

func randomPrivateRuntimeName() (string, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return privateRuntimePrefix + hex.EncodeToString(nonce[:]), nil
}

func validPrivateName(name string) bool {
	return name != "" && name == filepath.Base(name) && name != "." && name != ".." && !strings.ContainsRune(name, 0)
}

func (roots *privateRoots) childPath(name string) string {
	if roots == nil || !validPrivateName(name) || !filepath.IsAbs(roots.parentPath) {
		return ""
	}
	return filepath.Join(roots.parentPath, roots.runName, name)
}

func (roots *privateRoots) authorityFile(name string) *os.File {
	if roots == nil || !validPrivateName(name) {
		return nil
	}
	for index := range roots.children {
		child := &roots.children[index]
		if child.name == name && !child.closed && !child.unlinked {
			return child.file
		}
	}
	return nil
}

func (roots *privateRoots) Close() error {
	if roots == nil {
		return nil
	}
	roots.mu.Lock()
	defer roots.mu.Unlock()
	if roots.closed {
		return roots.terminalErr
	}
	if roots.terminalErr != nil {
		return roots.terminalErr
	}
	if !samePrivateDirectoryAt(roots.parent, roots.runName, roots.runRoot) {
		return ErrLifecycle
	}
	for index := range roots.children {
		child := &roots.children[index]
		if child.unlinked && child.closed {
			continue
		}
		if child.file == nil || child.closed ||
			(!child.unlinked && !samePrivateDirectoryAt(roots.runRoot, child.name, int(child.file.Fd()))) {
			return ErrLifecycle
		}
	}
	for index := len(roots.children) - 1; index >= 0; index-- {
		child := &roots.children[index]
		if child.unlinked && child.closed {
			continue
		}
		if child.file == nil {
			return ErrLifecycle
		}
		if !child.unlinked {
			if err := unix.Unlinkat(roots.runRoot, child.name, unix.AT_REMOVEDIR); err != nil {
				return err
			}
			child.unlinked = true
		}
		if !child.closed {
			if err := child.file.Close(); err != nil {
				roots.cleanupErr = errors.Join(roots.cleanupErr, err)
			}
			child.closed = true
			child.file = nil
		}
	}
	if err := unix.Fsync(roots.runRoot); err != nil {
		return err
	}
	if !samePrivateDirectoryAt(roots.parent, roots.runName, roots.runRoot) {
		return ErrLifecycle
	}
	if err := unix.Unlinkat(roots.parent, roots.runName, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	if err := unix.Fsync(roots.parent); err != nil {
		return err
	}
	runCloseErr := unix.Close(roots.runRoot)
	parentCloseErr := unix.Close(roots.parent)
	roots.runRoot = -1
	roots.parent = -1
	roots.closed = true
	if runCloseErr != nil || parentCloseErr != nil || roots.cleanupErr != nil {
		roots.terminalErr = errors.Join(roots.cleanupErr, runCloseErr, parentCloseErr)
		return roots.terminalErr
	}
	return nil
}

func samePrivateDirectoryAt(parent int, name string, expected int) bool {
	if parent < 0 || expected < 0 || !validPrivateName(name) {
		return false
	}
	current, err := openPrivateChildDirectory(parent, name)
	if err != nil {
		return false
	}
	defer unix.Close(current)
	var currentStat unix.Stat_t
	var expectedStat unix.Stat_t
	return unix.Fstat(current, &currentStat) == nil && unix.Fstat(expected, &expectedStat) == nil &&
		currentStat.Dev == expectedStat.Dev && currentStat.Ino == expectedStat.Ino &&
		currentStat.Mode == expectedStat.Mode && currentStat.Uid == expectedStat.Uid
}
