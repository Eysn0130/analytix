//go:build darwin

package processauthority

import (
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

func TestLoadedDarwinProcessPathSelf(t *testing.T) {
	path, err := loadedDarwinProcessPath(os.Getpid())
	if err != nil || path == "" {
		t.Fatalf("self process path = %q, err=%v", path, err)
	}
}

func TestSpawnSuspendedUsesOpenedWorkingDirectoryAndDoesNotRunBeforeResume(t *testing.T) {
	workingDirectory := t.TempDir()
	cwd, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatalf("open cwd: %v", err)
	}
	defer cwd.Close()
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	defer stdinReader.Close()
	defer stdinWriter.Close()
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	defer stderrReader.Close()
	defer stderrWriter.Close()

	pid, err := spawnSuspended(
		"/bin/pwd",
		[]string{"pwd"},
		[]string{"LANG=C", "PATH=/usr/bin:/bin"},
		int(cwd.Fd()),
		int(stdinReader.Fd()),
		int(stdoutWriter.Fd()),
		int(stderrWriter.Fd()),
	)
	if err != nil {
		t.Fatalf("spawn suspended: %v", err)
	}
	executable, err := os.Open("/bin/pwd")
	if err != nil {
		t.Fatalf("open executable: %v", err)
	}
	defer executable.Close()
	openedCDHashes, err := openedFileCodeDirectoryHashes(executable)
	if err != nil {
		t.Fatalf("parse opened executable code directory: %v", err)
	}
	loadedCDHash, err := loadedProcessCDHash(pid)
	if err != nil {
		t.Fatalf("read loaded executable CDHash: %v", err)
	}
	if !codeDirectoryHashMatches(loadedCDHash, openedCDHashes) {
		t.Fatal("loaded executable CDHash does not match the opened executable")
	}
	loadedPath, err := loadedDarwinProcessPath(pid)
	if err != nil {
		t.Fatalf("read loaded executable path: %v", err)
	}
	if loadedPath != "/bin/pwd" && loadedPath != "/usr/bin/pwd" {
		t.Fatalf("loaded executable path = %q, want /bin/pwd identity", loadedPath)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	defer func() {
		_ = process.Kill()
		_, _ = process.Wait()
	}()

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	readDone := make(chan []byte, 1)
	go func() {
		payload, _ := io.ReadAll(stdoutReader)
		readDone <- payload
	}()
	select {
	case payload := <-readDone:
		t.Fatalf("suspended child produced output before resume: %q", payload)
	case <-time.After(100 * time.Millisecond):
	}

	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		t.Fatalf("resume child: %v", err)
	}
	status, err := process.Wait()
	if err != nil {
		t.Fatalf("wait child: %v", err)
	}
	if !status.Success() {
		t.Fatalf("child status: %v", status)
	}
	select {
	case payload := <-readDone:
		canonicalWorkingDirectory, err := filepath.EvalSymlinks(workingDirectory)
		if err != nil {
			t.Fatalf("canonicalize cwd: %v", err)
		}
		if string(payload) != canonicalWorkingDirectory+"\n" {
			t.Fatalf("pwd output = %q, want %q", payload, canonicalWorkingDirectory+"\n")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading resumed child output")
	}
}

func TestSpawnSuspendedSurvivesDeepGrowingStacks(t *testing.T) {
	workingDirectory := t.TempDir()
	cwd, err := os.Open(workingDirectory)
	if err != nil {
		t.Fatalf("open cwd: %v", err)
	}
	defer cwd.Close()
	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	defer devNull.Close()
	for iteration := 0; iteration < 32; iteration++ {
		spawnAndWaitDarwinAtDepth(t, 48, cwd, devNull)
	}
}

func TestSpawnSuspendedPinsDescriptorsAboveClosedStandardFDs(t *testing.T) {
	workingDirectory := t.TempDir()
	resultPath := filepath.Join(t.TempDir(), "result")
	outputPath := filepath.Join(t.TempDir(), "output")
	command := exec.Command(os.Args[0], "-test.run=^TestSpawnSuspendedClosedStandardFDHelper$")
	command.Env = append(os.Environ(),
		"ANALYTIX_CLOSED_FD_HELPER=1",
		"ANALYTIX_CLOSED_FD_WORKDIR="+workingDirectory,
		"ANALYTIX_CLOSED_FD_RESULT="+resultPath,
		"ANALYTIX_CLOSED_FD_OUTPUT="+outputPath,
	)
	if output, err := command.CombinedOutput(); err != nil {
		result, _ := os.ReadFile(resultPath)
		t.Fatalf("closed-fd helper: %v output=%q result=%q", err, output, result)
	}
	result, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read helper result: %v", err)
	}
	if string(result) != "ok" {
		t.Fatalf("helper result = %q, want ok", result)
	}
}

func TestSpawnSuspendedClosedStandardFDHelper(t *testing.T) {
	if os.Getenv("ANALYTIX_CLOSED_FD_HELPER") != "1" {
		return
	}
	resultPath := os.Getenv("ANALYTIX_CLOSED_FD_RESULT")
	fail := func(code string) {
		_ = os.WriteFile(resultPath, []byte(code), 0o600)
		os.Exit(23)
	}
	cwdSource, err := unix.Open(os.Getenv("ANALYTIX_CLOSED_FD_WORKDIR"), unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		fail("cwd")
	}
	devNullSource, err := unix.Open("/dev/null", unix.O_RDWR, 0)
	if err != nil {
		fail("stdin")
	}
	outputSource, err := unix.Open(os.Getenv("ANALYTIX_CLOSED_FD_OUTPUT"), unix.O_RDWR|unix.O_CREAT|unix.O_EXCL, 0o600)
	if err != nil {
		fail("stdout")
	}
	if unix.Dup2(cwdSource, 0) != nil || unix.Dup2(devNullSource, 1) != nil || unix.Dup2(outputSource, 2) != nil {
		fail("dup2")
	}
	for _, descriptor := range []int{cwdSource, devNullSource, outputSource} {
		if descriptor > 2 {
			_ = unix.Close(descriptor)
		}
	}
	pid, err := spawnSuspended(
		"/bin/pwd",
		[]string{"pwd"},
		[]string{"LANG=C"},
		0,
		1,
		2,
		2,
	)
	if err != nil {
		fail("spawn")
	}
	process, err := os.FindProcess(pid)
	if err != nil || syscall.Kill(pid, syscall.SIGCONT) != nil {
		killAndWaitDarwinProcess(process, pid)
		fail("resume")
	}
	if err := observeDarwinProcessExit(pid); err != nil {
		killAndWaitDarwinProcess(process, pid)
		fail("observe")
	}
	killDarwinProcessGroup(pid)
	status, err := process.Wait()
	if err != nil || !status.Success() {
		fail("wait")
	}
	if err := unix.Close(2); err != nil {
		fail("close")
	}
	payload, err := os.ReadFile(os.Getenv("ANALYTIX_CLOSED_FD_OUTPUT"))
	if err != nil {
		fail("read")
	}
	canonicalWorkingDirectory, err := filepath.EvalSymlinks(os.Getenv("ANALYTIX_CLOSED_FD_WORKDIR"))
	if err != nil || string(payload) != canonicalWorkingDirectory+"\n" {
		fail("cwd-mismatch")
	}
	if err := os.WriteFile(resultPath, []byte("ok"), 0o600); err != nil {
		os.Exit(24)
	}
}

func spawnAndWaitDarwinAtDepth(t *testing.T, depth int, cwd, devNull *os.File) {
	t.Helper()
	var stack [4096]byte
	stack[depth%len(stack)] = byte(depth)
	if depth > 0 {
		spawnAndWaitDarwinAtDepth(t, depth-1, cwd, devNull)
		runtime.KeepAlive(&stack)
		return
	}
	pid, err := spawnSuspended(
		"/usr/bin/true",
		[]string{"true"},
		[]string{"LANG=C"},
		int(cwd.Fd()),
		int(devNull.Fd()),
		int(devNull.Fd()),
		int(devNull.Fd()),
	)
	if err != nil {
		t.Fatalf("spawn suspended: %v", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		killAndWaitDarwinProcess(nil, pid)
		t.Fatalf("find process: %v", err)
	}
	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		killAndWaitDarwinProcess(process, pid)
		t.Fatalf("resume child: %v", err)
	}
	if err := observeDarwinProcessExit(pid); err != nil {
		killAndWaitDarwinProcess(process, pid)
		t.Fatalf("observe child: %v", err)
	}
	killDarwinProcessGroup(pid)
	status, err := process.Wait()
	if err != nil || !status.Success() {
		t.Fatalf("wait child: status=%v err=%v", status, err)
	}
	runtime.KeepAlive(&stack)
}
