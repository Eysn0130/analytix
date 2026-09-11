package runtimego

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRuntimeServerRootTestWorkersRespectCallerLimit(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		total   int
		want    int
		invalid bool
	}{
		{name: "default maximum", total: 100, want: runtimeServerRootTestShardWorkerCount},
		{name: "available work", total: 2, want: 2},
		{name: "sequential", args: []string{"-test.parallel=1"}, total: 100, want: 1},
		{name: "bounded", args: []string{"-test.parallel", "2"}, total: 100, want: 2},
		{name: "never exceeds maximum", args: []string{"-test.parallel=64"}, total: 100, want: runtimeServerRootTestShardWorkerCount},
		{name: "never exceeds inventory", args: []string{"-test.parallel=4"}, total: 1, want: 1},
		{name: "empty inventory", total: 0, want: 0},
		{name: "zero rejected", args: []string{"-test.parallel=0"}, total: 100, invalid: true},
		{name: "negative rejected", args: []string{"-test.parallel=-1"}, total: 100, invalid: true},
		{name: "malformed rejected", args: []string{"-test.parallel=no"}, total: 100, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := runtimeServerRootTestWorkers(test.args, test.total)
			if (err != nil) != test.invalid || got != test.want {
				t.Fatalf("workers = %d, error = %v; want %d, invalid = %t", got, err, test.want, test.invalid)
			}
		})
	}
}

func TestRuntimeServerRootTestShardsAreDeterministicAndExhaustive(t *testing.T) {
	tests := []string{"TestDelta", "TestAlpha", "TestCharlie", "TestBravo", "TestEcho"}
	shards := buildRuntimeServerRootTestShards(tests)
	if len(shards) != len(tests) {
		t.Fatalf("shard count = %d, want %d", len(shards), len(tests))
	}
	want := []string{"TestAlpha", "TestBravo", "TestCharlie", "TestDelta", "TestEcho"}
	if got := sortedRuntimeServerRootTestInventory(shards); !reflect.DeepEqual(got, want) {
		t.Fatalf("sharded inventory = %v, want %v", got, want)
	}
	for index, shard := range shards {
		if shard.index != index || !reflect.DeepEqual(shard.tests, []string{tests[index]}) {
			t.Fatalf("one-test shard %d changed: %#v", index, shard)
		}
	}

	selfTests := []string{
		"TestProductAlpha",
		"TestRuntimeServerRootTestVerboseHandlesStandaloneBooleanFlag",
		"TestG1ShadowProductBoundaryMatchesContract",
		"TestProductBravo",
		"TestRuntimeServerRootTestRaceOptionsPreserveCallerPolicyAndReplaceExitSleep",
		"TestProductRegressionMatrixCoversRequiredHighRiskSurfaces",
	}
	selfTestShards := buildRuntimeServerRootTestShards(selfTests)
	if runtimeServerRootTestRaceExitSleepMillis < 0 {
		if len(selfTestShards) != len(selfTests) {
			t.Fatalf("non-race root shards = %d, want %d one-test process boundaries", len(selfTestShards), len(selfTests))
		}
		for _, shard := range selfTestShards {
			if len(shard.tests) != 1 {
				t.Fatalf("non-race test lost one-test process boundary: %#v", selfTestShards)
			}
		}
		return
	}
	if len(selfTestShards) != 3 || !reflect.DeepEqual(selfTestShards[0].tests, []string{
		"TestRuntimeServerRootTestVerboseHandlesStandaloneBooleanFlag",
		"TestG1ShadowProductBoundaryMatchesContract",
		"TestRuntimeServerRootTestRaceOptionsPreserveCallerPolicyAndReplaceExitSleep",
		"TestProductRegressionMatrixCoversRequiredHighRiskSurfaces",
	}) {
		t.Fatalf("root-sharding self-test group = %#v, want one exact harness-only group", selfTestShards)
	}
	for _, shard := range selfTestShards[1:] {
		if len(shard.tests) != 1 || !strings.HasPrefix(shard.tests[0], "TestProduct") {
			t.Fatalf("product test lost one-test process boundary: %#v", selfTestShards)
		}
	}
}

func TestRuntimeServerRootTestShardsInterleaveInventoryClusters(t *testing.T) {
	tests := make([]string, runtimeServerRootTestShardWorkerCount*2)
	for index := range tests {
		tests[index] = fmt.Sprintf("Test%02d", index)
	}
	shards := buildRuntimeServerRootTestShards(tests)
	got := make([]string, 0, len(shards))
	for _, shard := range shards {
		got = append(got, shard.tests[0])
	}
	want := make([]string, 0, len(tests))
	for offset := 0; offset < 2; offset++ {
		for lane := 0; lane < runtimeServerRootTestShardWorkerCount; lane++ {
			want = append(want, fmt.Sprintf("Test%02d", lane*2+offset))
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("shard queue = %v, want interleaved %v", got, want)
	}
}

func TestRuntimeServerRootTestTailSensitiveShardStartsBeforeFinalWave(t *testing.T) {
	tests := make([]string, runtimeServerRootTestShardWorkerCount*3)
	for index := range tests {
		tests[index] = fmt.Sprintf("TestSynthetic%02d", index)
	}
	tailSensitive := []string{
		"TestRuntimeServerCommandSeparatesGeneralOnlyFromHostPolicyCaseBoundary",
		"TestRuntimeServerDirectLightweightPromptsDoNotAdvertiseToolsToProvider",
	}
	tests[2] = tailSensitive[0]
	tests[3] = tailSensitive[1]

	shards := buildRuntimeServerRootTestShards(tests)
	for _, test := range tailSensitive {
		position := -1
		for index, shard := range shards {
			if reflect.DeepEqual(shard.tests, []string{test}) {
				position = index
				break
			}
		}
		if position < 0 || position >= runtimeServerRootTestShardWorkerCount {
			t.Fatalf("tail-sensitive shard %s position = %d, want first worker wave", test, position)
		}
	}
}

func TestRuntimeServerRootTestProductionCommandBuildRemainsShardLocal(t *testing.T) {
	environment := runtimeServerRootTestEnvironmentWithOverrides(
		[]string{
			runtimeServerRootTestProductionCommandEnvironment + "=/untrusted/caller",
			"KEEP=value",
		},
		runtimeServerRootTestProductionCommandEnvironment+"=",
	)
	want := []string{"KEEP=value", runtimeServerRootTestProductionCommandEnvironment + "="}
	if !reflect.DeepEqual(environment, want) {
		t.Fatalf("shard-local production command environment = %#v, want %#v", environment, want)
	}
}

func TestRuntimeServerRootTestShardCleanupRunsAfterChildrenAndFailsClosed(t *testing.T) {
	cleaned := make(chan int, 2)
	shards := []runtimeServerRootTestShard{
		{cleanup: func() error { cleaned <- 0; return nil }},
		{cleanup: func() error { cleaned <- 1; return fmt.Errorf("cleanup rejected") }},
	}
	cleanupRuntimeServerRootTestShardEnvironments(shards, 2)
	close(cleaned)
	seen := map[int]bool{}
	for index := range cleaned {
		seen[index] = true
	}
	if !seen[0] || !seen[1] || shards[0].cleanup != nil || shards[1].cleanup != nil {
		t.Fatalf("shard cleanup did not consume every owned environment: seen=%#v shards=%#v", seen, shards)
	}
	if shards[0].waitErr != nil || shards[1].waitErr == nil ||
		!strings.Contains(shards[1].waitErr.Error(), "cleanup isolated child config: cleanup rejected") {
		t.Fatalf("shard cleanup errors did not fail closed: %#v", shards)
	}
}

func TestRuntimeServerRootTestTimingSummaryReportsOnlyCompletedShards(t *testing.T) {
	start := time.Unix(100, 0)
	shards := []runtimeServerRootTestShard{
		{tests: []string{"TestFast"}, started: true, finished: true, startedAt: start, finishedAt: start.Add(time.Second)},
		{tests: []string{"TestSlow"}, started: true, finished: true, startedAt: start, finishedAt: start.Add(3 * time.Second)},
		{tests: []string{"TestPending"}, started: true, startedAt: start},
		{tests: []string{"TestUnstarted"}},
	}
	var output strings.Builder
	writeRuntimeServerRootTestTimingSummary(&output, shards, 1)
	body := output.String()
	for _, required := range []string{
		"total=4 started=3 completed=2 passed=2 failed=0 unstarted=1 slowest=1",
		"duration=3s test=TestSlow",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("timing summary omitted %q: %s", required, body)
		}
	}
	for _, forbidden := range []string{"test=TestFast", "test=TestPending", "test=TestUnstarted"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("timing summary included %q beyond its limit: %s", forbidden, body)
		}
	}
}

func TestRuntimeServerRootTestShardingPreservesFocusedAndInstrumentedModes(t *testing.T) {
	for _, args := range [][]string{
		{"-test.run=TestFocused"},
		{"-test.coverprofile=cover.out"},
		{"-test.cpuprofile=cpu.out"},
		{"-test.fuzz=FuzzInput"},
		{"-test.list=Test"},
		{"-test.shuffle=on"},
	} {
		if runtimeServerRootTestShardingEligible(args, "", "") {
			t.Fatalf("instrumented/focused args unexpectedly shard: %v", args)
		}
	}
	for _, args := range [][]string{
		{"-test.failfast=true"},
		{"-test.v=test2json"},
	} {
		if runtimeServerRootTestShardingEligible(args, "", "") {
			t.Fatalf("non-equivalent output/control mode unexpectedly shard: %v", args)
		}
	}
	if runtimeServerRootTestShardingEligible(nil, "child", "") {
		t.Fatal("child process recursively enabled root sharding")
	}
	if runtimeServerRootTestShardingEligible(nil, "", "/tmp/inherited-config") {
		t.Fatal("inherited user config unexpectedly enabled concurrent shards")
	}
	if !runtimeServerRootTestShardingEligible([]string{"-test.count=1", "-test.timeout=10m0s"}, "", "") {
		t.Fatal("ordinary full root test did not enable sharding")
	}
}

func TestRuntimeServerRootTestShardChildArgsReplaceAuthorityFlags(t *testing.T) {
	args := []string{
		"-test.paniconexit0", "-test.timeout=10m0s", "-test.run", "Old",
		"-test.testlogfile=/tmp/parent.log", "-test.count=1",
	}
	got := runtimeServerRootTestChildArgs(args, []string{"TestOne", "TestTwo"}, "/tmp/parent.log", 4)
	want := []string{
		"-test.paniconexit0", "-test.timeout=10m0s", "-test.count=1",
		"-test.run=^(?:TestOne|TestTwo)$", "-test.testlogfile=/tmp/parent.log.shard-004",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("child args = %#v, want %#v", got, want)
	}
}

func TestRuntimeServerRootTestCountOneOmitsOnlyUncacheableChildLogs(t *testing.T) {
	if child, uncacheable := runtimeServerRootTestChildLogParent(
		[]string{"-test.count=1"}, "/tmp/parent.log",
	); child != "" || !uncacheable {
		t.Fatalf("count-one child log = %q uncacheable=%v, want omitted true", child, uncacheable)
	}
	if child, uncacheable := runtimeServerRootTestChildLogParent(
		[]string{"-test.count=2"}, "/tmp/parent.log",
	); child != "/tmp/parent.log" || uncacheable {
		t.Fatalf("cacheable child log = %q uncacheable=%v, want parent false", child, uncacheable)
	}
}

func TestRuntimeServerRootTestRaceOptionsPreserveCallerPolicyAndReplaceExitSleep(t *testing.T) {
	got := runtimeServerRootTestRaceOptionsWithExitSleep(
		"halt_on_error=1 atexit_sleep_ms=1000 strip_path_prefix=/private atexit_sleep_ms=250",
		0,
	)
	want := "halt_on_error=1 strip_path_prefix=/private atexit_sleep_ms=0"
	if got != want {
		t.Fatalf("race options = %q, want %q", got, want)
	}
	if got := runtimeServerRootTestRaceOptionsWithExitSleep("halt_on_error=1", -1); got != "halt_on_error=1" {
		t.Fatalf("disabled race options changed caller policy: %q", got)
	}
	environment := runtimeServerRootTestEnvironmentWithOverrides(
		[]string{runtimeServerRootTestRaceOptionsEnvironment + "=exitcode=73", "KEEP=value"},
		runtimeServerRootTestRaceOptionsEnvironment+"="+want,
	)
	if value := runtimeServerRootTestEnvironmentValue(environment, runtimeServerRootTestRaceOptionsEnvironment); value != want {
		t.Fatalf("race environment = %q, want %q", value, want)
	}
}

func TestRuntimeServerRootTestChildParallelismCapsOnlyImplicitPolicy(t *testing.T) {
	tests := []struct {
		name        string
		environment []string
		current     int
		race        bool
		want        string
		configured  bool
	}{
		{name: "race bounds host parallelism", current: 12, race: true, want: "4", configured: true},
		{name: "race rounds fractional parallelism up", current: 7, race: true, want: "3", configured: true},
		{name: "race preserves two-way scheduling", current: 2, race: true, want: "2", configured: true},
		{name: "race preserves a single logical CPU", current: 1, race: true, want: "1", configured: true},
		{name: "non-race divides host across workers", current: 12, race: false, want: "2", configured: true},
		{name: "non-race preserves two-way scheduling", current: 2, race: false, want: "2", configured: true},
		{name: "non-race preserves a single logical CPU", current: 1, race: false, want: "1", configured: true},
		{
			name: "explicit caller policy remains authoritative", environment: []string{"GOMAXPROCS=4"},
			current: 12, race: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, configured := runtimeServerRootTestChildParallelism(
				test.environment,
				test.current,
				test.race,
			)
			if got != test.want || configured != test.configured {
				t.Fatalf("child parallelism = %q configured=%t, want %q configured=%t", got, configured, test.want, test.configured)
			}
		})
	}
}

func TestRuntimeServerRootTestCoordinatorPreservesZeroAndDerivesFiniteTimeout(t *testing.T) {
	zero, cancelZero, err := runtimeServerRootTestCoordinatorContext([]string{"-test.timeout=0"})
	if err != nil {
		t.Fatalf("zero timeout: %v", err)
	}
	defer cancelZero()
	if _, ok := zero.Deadline(); ok {
		t.Fatal("zero timeout unexpectedly gained a deadline")
	}

	finite, cancelFinite, err := runtimeServerRootTestCoordinatorContext([]string{"-test.timeout=2m"})
	if err != nil {
		t.Fatalf("finite timeout: %v", err)
	}
	defer cancelFinite()
	deadline, ok := finite.Deadline()
	if !ok {
		t.Fatal("finite timeout omitted coordinator deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 113*time.Second || remaining > 115*time.Second {
		t.Fatalf("finite coordinator timeout = %v, want about 114s", remaining)
	}
}

func TestRuntimeServerRootTestLogMergeAcceptsEmptyAndResetsWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	parent := filepath.Join(directory, "parent.log")
	first := filepath.Join(directory, "first.log")
	second := filepath.Join(directory, "second.log")
	if err := os.WriteFile(first, []byte(runtimeServerRootTestLogMagic), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(runtimeServerRootTestLogMagic+"open relative.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shards := []runtimeServerRootTestShard{{index: 0, testLog: first}, {index: 1, testLog: second}}
	if err := mergeRuntimeServerRootTestLogs(parent, shards); err != nil {
		t.Fatalf("merge test logs: %v", err)
	}
	body, err := os.ReadFile(parent)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wantSuffix := "chdir " + cwd + "\nopen relative.txt\n"
	if !strings.HasPrefix(string(body), runtimeServerRootTestLogMagic) || !strings.HasSuffix(string(body), wantSuffix) {
		t.Fatalf("merged test log = %q, want suffix %q", body, wantSuffix)
	}
}

func TestRuntimeServerRootTestVerboseHandlesStandaloneBooleanFlag(t *testing.T) {
	if !runtimeServerRootTestVerbose([]string{"-test.v", "-test.timeout=10m"}) {
		t.Fatal("standalone -test.v was not recognized")
	}
	if runtimeServerRootTestVerbose([]string{"-test.v=false"}) {
		t.Fatal("explicit false verbose flag was enabled")
	}
}
