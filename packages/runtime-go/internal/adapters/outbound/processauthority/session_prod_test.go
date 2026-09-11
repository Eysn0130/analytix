//go:build darwin && analytix_prod

package processauthority

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const productionDirectTestArgument = "--analytix-production-direct-test="

func init() {
	mode := ""
	for _, argument := range os.Args[1:] {
		if strings.HasPrefix(argument, productionDirectTestArgument) {
			mode = strings.TrimPrefix(argument, productionDirectTestArgument)
			break
		}
	}
	if mode == "" {
		return
	}
	runProductionDirectTestTarget(mode)
}

func runProductionDirectTestTarget(mode string) {
	if mode == "crash-before-arm" {
		time.Sleep(300 * time.Millisecond)
		os.Exit(71)
	}
	if unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: 0, Max: 0}) != nil {
		os.Exit(72)
	}
	var limit unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_NPROC, &limit) != nil || limit.Cur != 0 || limit.Max != 0 {
		os.Exit(72)
	}
	armed := make(chan struct{})
	go func() {
		close(armed)
		var payload [1]byte
		read, _ := unix.Read(darwinOwnerLivenessTargetFD, payload[:])
		if read == 0 {
			os.Exit(190)
		}
		os.Exit(191)
	}()
	<-armed
	if mode == "crash-after-arm" {
		time.Sleep(300 * time.Millisecond)
		os.Exit(73)
	}
	ready := fmt.Sprintf(
		`{"kind":"analytix_native_ready","schema_version":3,"component_id":"data-engine","launch_nonce":"%s","protocol_version":"analytix-native-v1","process_id":%d}`+"\n",
		strings.Repeat("a", 64),
		os.Getpid(),
	)
	if _, err := io.WriteString(os.Stdout, ready); err != nil {
		os.Exit(74)
	}
	if mode == "crash-after-readiness" {
		time.Sleep(300 * time.Millisecond)
		os.Exit(75)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		frame, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) && frame == "" {
			os.Exit(0)
		}
		if err != nil || frame == "" {
			os.Exit(76)
		}
		if mode == "cwd-churn" {
			workingDirectory, err := os.Getwd()
			if err != nil {
				os.Exit(78)
			}
			ownedEntry := filepath.Join(workingDirectory, "production-direct-owned-entry")
			if err := os.WriteFile(ownedEntry, []byte("owned"), 0o600); err != nil {
				os.Exit(78)
			}
			if err := os.Remove(ownedEntry); err != nil {
				os.Exit(78)
			}
		}
		response := fmt.Sprintf(`{"ok":true,"process_id":%d}`+"\n", os.Getpid())
		if _, err := io.WriteString(os.Stdout, response); err != nil {
			os.Exit(77)
		}
	}
}

func TestProductionBootstrapDispatchIsDisabled(t *testing.T) {
	if currentBootstrapMainAllowed() {
		t.Fatal("analytix_prod retained a runtime-server bootstrap identity")
	}
}

func TestProductionOpenSessionDirectlyRunsPinnedTargetWithoutStoppedState(t *testing.T) {
	darwinAuthorityPoisoned.Store(false)
	config, stagingRoot := productionDirectSessionConfig(t, "ready")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opened, err := OpenSession(ctx, config)
	if err != nil {
		t.Fatalf("open direct production session: %v", err)
	}
	session, ok := opened.(*darwinSession)
	if !ok || !session.direct || session.tree != nil || session.bootstrapStage != nil {
		t.Fatalf("production session used a bootstrap path: %#v", opened)
	}
	if err := config.WorkingDirectoryAuthority.Close(); err != nil {
		t.Fatalf("close caller working authority: %v", err)
	}
	if err := config.StagingRootAuthority.Close(); err != nil {
		t.Fatalf("close caller staging authority: %v", err)
	}
	assertProductionDirectProcessRunning(t, session)
	if err := session.AuthenticateReadiness(ctx, 4096, func(frame []byte, processID int) bool {
		var ready struct {
			Kind      string `json:"kind"`
			ProcessID int    `json:"process_id"`
		}
		return json.Unmarshal(frame, &ready) == nil && ready.Kind == "analytix_native_ready" &&
			ready.ProcessID == processID
	}); err != nil {
		t.Fatalf("authenticate direct readiness: %v", err)
	}
	assertProductionDirectProcessRunning(t, session)
	response, err := session.RoundTrip(ctx, []byte("{\"request_id\":\"direct\"}\n"), 4096)
	if err != nil || !strings.Contains(string(response), `"ok":true`) {
		t.Fatalf("direct round trip response=%q err=%v", response, err)
	}
	pid := session.pid
	identity := session.identity
	if err := session.Close(); err != nil {
		t.Fatalf("close direct production session: %v", err)
	}
	assertProductionDirectProcessGone(t, pid, identity)
	assertProductionStagingEmpty(t, stagingRoot)
}

func TestProductionDirectSessionAllowsPinnedWorkingDirectoryEntryChurn(t *testing.T) {
	session, config, stagingRoot, ctx := openReadyProductionDirectSession(t, "cwd-churn")
	response, err := session.RoundTrip(ctx, []byte("{\"request_id\":\"cwd-churn\"}\n"), 4096)
	if err != nil || !strings.Contains(string(response), `"ok":true`) {
		t.Fatalf("cwd churn round trip response=%q err=%v", response, err)
	}
	entries, err := os.ReadDir(config.WorkingDirectory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("direct target retained cwd entries: entries=%#v err=%v", entries, err)
	}
	pid := session.pid
	identity := session.identity
	if err := session.Close(); err != nil {
		t.Fatalf("close cwd-churn production session: %v", err)
	}
	assertProductionDirectProcessGone(t, pid, identity)
	assertProductionStagingEmpty(t, stagingRoot)
}

func TestProductionDirectSessionRejectsWorkingDirectoryAuthorityDriftAfterLaunch(t *testing.T) {
	t.Run("path replacement", func(t *testing.T) {
		session, config, stagingRoot, ctx := openReadyProductionDirectSession(t, "ready")
		retainedPath := config.WorkingDirectory + "-retained"
		if err := os.Rename(config.WorkingDirectory, retainedPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(config.WorkingDirectory, 0o700); err != nil {
			_ = os.Rename(retainedPath, config.WorkingDirectory)
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Remove(config.WorkingDirectory)
			_ = os.Rename(retainedPath, config.WorkingDirectory)
		})
		pid := session.pid
		identity := session.identity
		if _, err := session.RoundTrip(ctx, []byte("{\"request_id\":\"path-replaced\"}\n"), 4096); err == nil {
			t.Fatal("replaced working-directory path remained authorized")
		}
		assertProductionDirectProcessGone(t, pid, identity)
		assertProductionStagingEmpty(t, stagingRoot)
	})

	t.Run("mode drift", func(t *testing.T) {
		session, config, stagingRoot, ctx := openReadyProductionDirectSession(t, "ready")
		if err := os.Chmod(config.WorkingDirectory, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(config.WorkingDirectory, 0o700) })
		pid := session.pid
		identity := session.identity
		if _, err := session.RoundTrip(ctx, []byte("{\"request_id\":\"mode-drift\"}\n"), 4096); err == nil {
			t.Fatal("working-directory mode drift remained authorized")
		}
		assertProductionDirectProcessGone(t, pid, identity)
		assertProductionStagingEmpty(t, stagingRoot)
	})
}

func TestProductionDirectStartupCrashesNeverLeaveStoppedOrphans(t *testing.T) {
	for _, mode := range []string{"crash-before-arm", "crash-after-arm", "crash-after-readiness"} {
		t.Run(mode, func(t *testing.T) {
			darwinAuthorityPoisoned.Store(false)
			config, stagingRoot := productionDirectSessionConfig(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			opened, err := OpenSession(ctx, config)
			if err != nil {
				t.Fatalf("open crash-injection session: %v", err)
			}
			session := opened.(*darwinSession)
			pid := session.pid
			identity := session.identity
			assertProductionDirectProcessRunning(t, session)
			readinessErr := session.AuthenticateReadiness(ctx, 4096, func([]byte, int) bool { return true })
			if mode == "crash-after-readiness" && readinessErr == nil {
				_, readinessErr = session.RoundTrip(ctx, []byte("{}\n"), 4096)
			}
			if readinessErr == nil {
				t.Fatal("injected target crash was accepted")
			}
			assertProductionDirectProcessGone(t, pid, identity)
			assertProductionStagingEmpty(t, stagingRoot)
		})
	}
}

func productionDirectSessionConfig(t *testing.T, mode string) (SessionConfig, string) {
	t.Helper()
	executablePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executablePath, err = filepath.EvalSymlinks(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Open(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := executable.Stat()
	if err != nil {
		_ = executable.Close()
		t.Fatal(err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, executable); err != nil {
		_ = executable.Close()
		t.Fatal(err)
	}
	if _, err := executable.Seek(0, io.SeekStart); err != nil {
		_ = executable.Close()
		t.Fatal(err)
	}
	workingDirectory := productionDirectTestDirectory(t)
	stagingRoot := productionDirectTestDirectory(t)
	workingAuthority, err := os.Open(workingDirectory)
	if err != nil {
		_ = executable.Close()
		t.Fatal(err)
	}
	stagingAuthority, err := os.Open(stagingRoot)
	if err != nil {
		_ = executable.Close()
		_ = workingAuthority.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = workingAuthority.Close()
		_ = stagingAuthority.Close()
	})
	return SessionConfig{
		Executable: executable, ExpectedExecutableSHA256: fmt.Sprintf("%x", hasher.Sum(nil)),
		ExpectedExecutableSize: stat.Size(), Arguments: []string{productionDirectTestArgument + mode},
		WorkingDirectory: workingDirectory, WorkingDirectoryAuthority: workingAuthority,
		StagingRoot: stagingRoot, StagingRootAuthority: stagingAuthority,
		Environment: []string{
			"ANALYTIX_NATIVE_LAUNCH_NONCE=" + strings.Repeat("a", 64),
			"ANALYTIX_NATIVE_PROTOCOL_VERSION=analytix-native-v1",
			"LANG=C", "LC_ALL=C", "TZ=UTC",
		},
	}, stagingRoot
}

func openReadyProductionDirectSession(
	t *testing.T,
	mode string,
) (*darwinSession, SessionConfig, string, context.Context) {
	t.Helper()
	darwinAuthorityPoisoned.Store(false)
	config, stagingRoot := productionDirectSessionConfig(t, mode)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	opened, err := OpenSession(ctx, config)
	if err != nil {
		t.Fatalf("open direct production session: %v", err)
	}
	session, ok := opened.(*darwinSession)
	if !ok || !session.direct || session.tree != nil || session.bootstrapStage != nil {
		t.Fatalf("production session used a bootstrap path: %#v", opened)
	}
	if err := config.WorkingDirectoryAuthority.Close(); err != nil {
		t.Fatalf("close caller working authority: %v", err)
	}
	if err := config.StagingRootAuthority.Close(); err != nil {
		t.Fatalf("close caller staging authority: %v", err)
	}
	assertProductionDirectProcessRunning(t, session)
	if err := session.AuthenticateReadiness(ctx, 4096, func(frame []byte, processID int) bool {
		var ready struct {
			Kind      string `json:"kind"`
			ProcessID int    `json:"process_id"`
		}
		return json.Unmarshal(frame, &ready) == nil && ready.Kind == "analytix_native_ready" &&
			ready.ProcessID == processID
	}); err != nil {
		t.Fatalf("authenticate direct readiness: %v", err)
	}
	assertProductionDirectProcessRunning(t, session)
	return session, config, stagingRoot, ctx
}

func productionDirectTestDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertProductionDirectProcessRunning(t *testing.T, session *darwinSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state, err := readDarwinProcessState(ctx, session.pid, true)
	if err != nil || !state.sameIdentity(session.identity) || state.Identity.PPID != os.Getpid() ||
		state.Identity.PGID != session.pid || state.Status == darwinProcStatusStopped ||
		state.Status == darwinProcStatusZombie {
		t.Fatalf("direct target was not an exact running child: state=%#v err=%v", state, err)
	}
}

func assertProductionDirectProcessGone(t *testing.T, pid int, identity darwinProcessIdentity) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := waitDarwinIdentityESRCHWith(ctx, identity, realDarwinProcInfoCaller); err != nil {
		t.Fatalf("direct pid survived cleanup: pid=%d err=%v", pid, err)
	}
	if err := waitDarwinProcessGroupESRCH(ctx, pid); err != nil {
		t.Fatalf("direct process group survived cleanup: pgid=%d err=%v", pid, err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("direct process remained observable: pid=%d err=%v", pid, err)
	}
}

func assertProductionStagingEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("direct session retained staging: %#v", entries)
	}
}
