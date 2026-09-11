//go:build race

package runtimego

import (
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const (
	// The race detector increases process startup, execution, and memory cost.
	// Eleven isolated workers preserve one-test process boundaries without
	// changing the runtime scheduler semantics exercised by each child. Full
	// module gates serialize outer packages with -p=1. The race runtime's
	// default one-second exit sleep is redundant because each child's output is
	// captured synchronously and Wait observes its completed exit.
	runtimeServerRootTestShardWorkerCount    = 11
	runtimeServerRootTestRaceExitSleepMillis = 0
)

var runtimeServerRootTestProductionCommandBuildFlags = []string{"-race"}

func TestRuntimeServerRootTestShardChildSchedulerContract(t *testing.T) {
	if runtimeServerRootTestShardWorkerCount != 11 {
		t.Fatalf("race shard workers = %d, want 11", runtimeServerRootTestShardWorkerCount)
	}
	if runtimeServerRootTestRaceExitSleepMillis != 0 {
		t.Fatalf("race child exit sleep = %d, want 0", runtimeServerRootTestRaceExitSleepMillis)
	}
	if !reflect.DeepEqual(runtimeServerRootTestProductionCommandBuildFlags, []string{"-race"}) {
		t.Fatalf(
			"race production command build flags = %v, want [-race]",
			runtimeServerRootTestProductionCommandBuildFlags,
		)
	}
}

func TestRuntimeServerRootTestRaceDetectorReportsWithZeroExitSleep(t *testing.T) {
	const probeEnvironment = "ANALYTIX_GO_ROOT_TEST_RACE_REPORT_PROBE"
	if os.Getenv(probeEnvironment) == "child" {
		var value int
		started := make(chan struct{})
		done := make(chan struct{})
		go func() {
			close(started)
			for index := 0; index < 100_000; index++ {
				value++
			}
			close(done)
		}()
		<-started
		for index := 0; index < 100_000; index++ {
			value++
		}
		<-done
		runtime.KeepAlive(value)
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestRuntimeServerRootTestRaceDetectorReportsWithZeroExitSleep$")
	command.Env = runtimeServerRootTestEnvironmentWithOverrides(
		os.Environ(),
		runtimeServerRootTestShardChildEnvironment+"=1",
		probeEnvironment+"=child",
		runtimeServerRootTestRaceOptionsEnvironment+"=halt_on_error=1 exitcode=73 atexit_sleep_ms=0",
	)
	output, err := command.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 73 {
		t.Fatalf("race probe exit = %v, want 73: %s", err, output)
	}
	if body := string(output); !strings.Contains(body, "WARNING: DATA RACE") {
		t.Fatalf("race probe omitted detector report: %s", body)
	}
}
