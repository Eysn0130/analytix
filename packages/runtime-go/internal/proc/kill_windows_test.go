//go:build windows

package proc

import (
	"errors"
	"io"
	"os/exec"
	"testing"
	"time"
)

func TestStartTrackedRequiresJobAndResumesChild(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit", "7")
	job, err := StartTracked(cmd)
	if err != nil {
		t.Fatalf("StartTracked: %v", err)
	}
	if job == 0 {
		t.Fatal("StartTracked succeeded without a Job Object")
	}
	defer ReapTracked(cmd, job)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
			t.Fatalf("exit = %v, want exit status 7", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("suspended child was not resumed")
	}
}

func TestKillTrackedIsRepeatableAndReapsDescendantPipe(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "ping", "-n", "30", "127.0.0.1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	job, err := StartTracked(cmd)
	if err != nil {
		t.Fatalf("StartTracked: %v", err)
	}
	if job == 0 {
		t.Fatal("StartTracked succeeded without a Job Object")
	}
	go func() { _, _ = io.Copy(io.Discard, stdout) }()
	time.Sleep(500 * time.Millisecond)

	KillTracked(cmd, job)
	KillTracked(cmd, job)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("tracked descendant retained the inherited pipe")
	}
	ReapTracked(cmd, job)
}
