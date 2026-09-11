//go:build !windows

package proc

import (
	"bufio"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSetProcessGroupKillStartsIndependentSession(t *testing.T) {
	cmd := exec.Command("true")
	SetProcessGroupKill(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid || cmd.SysProcAttr.Setpgid {
		t.Fatalf("tracked process does not use an independent session: %#v", cmd.SysProcAttr)
	}
}

func TestKillTrackedReapsProcessGroupGrandchild(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 60 & echo $!; wait")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	job, err := StartTracked(cmd)
	if err != nil {
		t.Fatalf("StartTracked: %v", err)
	}
	if job != 0 {
		t.Fatalf("non-Windows job handle = %d, want 0", job)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read grandchild pid: %v", err)
	}
	grandchildPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || grandchildPID <= 0 {
		t.Fatalf("invalid grandchild pid %q: %v", line, err)
	}
	if err := syscall.Kill(grandchildPID, 0); err != nil {
		t.Fatalf("grandchild %d not alive before kill: %v", grandchildPID, err)
	}

	KillTracked(cmd, job)
	_ = cmd.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for !errors.Is(syscall.Kill(grandchildPID, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d survived tracked process-group kill", grandchildPID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
