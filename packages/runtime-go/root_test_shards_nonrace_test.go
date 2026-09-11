//go:build !race

package runtimego

import "testing"

const (
	// One-test child processes retain state and process-tree isolation. The
	// interleaved queue prevents one resource-heavy inventory cluster from
	// occupying every worker at once.
	runtimeServerRootTestShardWorkerCount    = 9
	runtimeServerRootTestRaceExitSleepMillis = -1
)

var runtimeServerRootTestProductionCommandBuildFlags = []string{}

func TestRuntimeServerRootTestShardChildSchedulerContract(t *testing.T) {
	if runtimeServerRootTestShardWorkerCount != 9 {
		t.Fatalf("non-race shard workers = %d, want 9", runtimeServerRootTestShardWorkerCount)
	}
	if runtimeServerRootTestRaceExitSleepMillis != -1 {
		t.Fatalf("non-race child race exit sleep = %d, want disabled", runtimeServerRootTestRaceExitSleepMillis)
	}
	if len(runtimeServerRootTestProductionCommandBuildFlags) != 0 {
		t.Fatalf("non-race production command build flags = %v, want none", runtimeServerRootTestProductionCommandBuildFlags)
	}
}
