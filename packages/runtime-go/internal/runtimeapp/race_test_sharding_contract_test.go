//go:build race

package runtimeapp

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestRuntimeAppRaceTestShardsAreExactSingletons(t *testing.T) {
	tests := []string{"TestDelta", "TestAlpha", "TestCharlie", "TestBravo"}
	shards := buildRuntimeAppRaceTestShards(tests)
	if runtimeAppRaceTestShardWorkerCount != 3 {
		t.Fatalf("race shard workers = %d, want 3", runtimeAppRaceTestShardWorkerCount)
	}
	if len(shards) != len(tests) {
		t.Fatalf("shard count = %d, want %d", len(shards), len(tests))
	}
	for index, shard := range shards {
		if shard.index != index || shard.test != tests[index] {
			t.Fatalf("singleton shard %d changed: %#v", index, shard)
		}
	}
	want := append([]string(nil), tests...)
	sort.Strings(want)
	if got := sortedRuntimeAppRaceTestInventory(shards); !reflect.DeepEqual(got, want) {
		t.Fatalf("sharded inventory = %v, want %v", got, want)
	}
}

func TestRuntimeAppRaceTestShardingPreservesFocusedAndInstrumentedModes(t *testing.T) {
	for _, args := range [][]string{
		{"-test.run=TestFocused"},
		{"-test.skip=TestSkipped"},
		{"-test.coverprofile=cover.out"},
		{"-test.cpuprofile=cpu.out"},
		{"-test.fuzz=FuzzInput"},
		{"-test.list=Test"},
		{"-test.shuffle=on"},
		{"-test.cpu=1"},
		{"-test.cpu", "1,2"},
		{"-test.parallel=2"},
		{"-test.parallel", "2"},
		{"-test.failfast=true"},
		{"-test.v=test2json"},
	} {
		if runtimeAppRaceTestShardingEligible(args, "", "") {
			t.Fatalf("focused/instrumented args unexpectedly shard: %v", args)
		}
	}
	if runtimeAppRaceTestShardingEligible(nil, "child", "") {
		t.Fatal("child process recursively enabled runtimeapp race sharding")
	}
	if runtimeAppRaceTestShardingEligible(nil, "", "/tmp/inherited-config") {
		t.Fatal("inherited user config unexpectedly enabled runtimeapp race sharding")
	}
	if !runtimeAppRaceTestShardingEligible([]string{"-test.count=1", "-test.timeout=10m0s"}, "", "") {
		t.Fatal("ordinary full runtimeapp race test did not enable sharding")
	}
}

func TestRuntimeAppRaceTestShardScheduleInterleavesInventoryDeterministically(t *testing.T) {
	for _, test := range []struct {
		name        string
		shardCount  int
		workerCount int
		want        []int
	}{
		{
			name:        "uneven segments",
			shardCount:  10,
			workerCount: 3,
			want:        []int{0, 4, 8, 1, 5, 9, 2, 6, 3, 7},
		},
		{
			name:        "more workers than shards",
			shardCount:  3,
			workerCount: 8,
			want:        []int{0, 1, 2},
		},
		{
			name:        "single worker",
			shardCount:  4,
			workerCount: 1,
			want:        []int{0, 1, 2, 3},
		},
		{
			name:        "invalid worker count",
			shardCount:  3,
			workerCount: 0,
			want:        []int{0, 1, 2},
		},
		{
			name:        "empty inventory",
			shardCount:  0,
			workerCount: 3,
			want:        nil,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			first := runtimeAppRaceTestShardSchedule(test.shardCount, test.workerCount)
			second := runtimeAppRaceTestShardSchedule(test.shardCount, test.workerCount)
			if !reflect.DeepEqual(first, test.want) {
				t.Fatalf("schedule = %v, want %v", first, test.want)
			}
			if !reflect.DeepEqual(second, first) {
				t.Fatalf("schedule is not deterministic: first=%v second=%v", first, second)
			}
			seen := make(map[int]struct{}, len(first))
			for _, index := range first {
				if index < 0 || index >= test.shardCount {
					t.Fatalf("schedule index %d is outside [0,%d)", index, test.shardCount)
				}
				if _, duplicate := seen[index]; duplicate {
					t.Fatalf("schedule repeats index %d: %v", index, first)
				}
				seen[index] = struct{}{}
			}
			if len(seen) != test.shardCount {
				t.Fatalf("schedule covers %d shards, want %d: %v", len(seen), test.shardCount, first)
			}
		})
	}
}

func TestRuntimeAppRaceTestChildArgsPreserveAuthorityFlags(t *testing.T) {
	args := []string{
		"-test.paniconexit0",
		"-test.timeout=10m0s",
		"-test.run",
		"Old",
		"-test.skip=Skipped",
		"-test.list=Listed",
		"-test.testlogfile=/tmp/parent.log",
		"-test.count=1",
	}
	got := runtimeAppRaceTestChildArgs(args, "TestOne.WithRegex", "/tmp/parent.log", 4)
	want := []string{
		"-test.paniconexit0",
		"-test.timeout=10m0s",
		"-test.count=1",
		"-test.run=^TestOne\\.WithRegex$",
		"-test.testlogfile=/tmp/parent.log.shard-004",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("child args = %#v, want %#v", got, want)
	}
	for _, arg := range got {
		if strings.HasPrefix(arg, "-test.parallel") || strings.HasPrefix(arg, "-test.failfast") {
			t.Fatalf("singleton child changed test scheduler semantics: %#v", got)
		}
	}
}

func TestRuntimeAppRaceTestCoordinatorPreservesZeroAndDerivesFiniteTimeout(t *testing.T) {
	zero, cancelZero, err := runtimeAppRaceTestCoordinatorContext([]string{"-test.timeout=0"})
	if err != nil {
		t.Fatalf("zero timeout: %v", err)
	}
	defer cancelZero()
	if _, ok := zero.Deadline(); ok {
		t.Fatal("zero timeout unexpectedly gained a deadline")
	}

	finite, cancelFinite, err := runtimeAppRaceTestCoordinatorContext([]string{"-test.timeout=2m"})
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

func TestRuntimeAppRaceTestChildEnvironmentIsUniqueAndOwned(t *testing.T) {
	first, cleanupFirst, err := runtimeAppRaceTestChildEnvironment(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupFirst(); err != nil {
			t.Error(err)
		}
	}()
	second, cleanupSecond, err := runtimeAppRaceTestChildEnvironment(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupSecond(); err != nil {
			t.Error(err)
		}
	}()
	firstRoot := runtimeAppRaceTestEnvironmentValue(first, runtimeAppRaceTestUserConfigEnvironment)
	secondRoot := runtimeAppRaceTestEnvironmentValue(second, runtimeAppRaceTestUserConfigEnvironment)
	if firstRoot == "" || secondRoot == "" || firstRoot == secondRoot {
		t.Fatalf("child config roots are not unique: first=%q second=%q", firstRoot, secondRoot)
	}
	for _, environment := range [][]string{first, second} {
		root := runtimeAppRaceTestEnvironmentValue(environment, runtimeAppRaceTestUserConfigEnvironment)
		home := runtimeAppRaceTestEnvironmentValue(environment, "HOME")
		config := runtimeAppRaceTestEnvironmentValue(environment, "XDG_CONFIG_HOME")
		if home != filepath.Join(root, "home") || config != filepath.Join(root, "config") {
			t.Fatalf("child environment is not bound to root %q: HOME=%q XDG_CONFIG_HOME=%q", root, home, config)
		}
	}
}

func TestRuntimeAppRaceTestLogMergeAcceptsEmptyAndResetsWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	parent := filepath.Join(directory, "parent.log")
	first := filepath.Join(directory, "first.log")
	second := filepath.Join(directory, "second.log")
	if err := os.WriteFile(first, []byte(runtimeAppRaceTestLogMagic), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(runtimeAppRaceTestLogMagic+"open relative.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shards := []runtimeAppRaceTestShard{{index: 0, testLog: first}, {index: 1, testLog: second}}
	if err := mergeRuntimeAppRaceTestLogs(parent, shards); err != nil {
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
	if !strings.HasPrefix(string(body), runtimeAppRaceTestLogMagic) || !strings.HasSuffix(string(body), wantSuffix) {
		t.Fatalf("merged test log = %q, want suffix %q", body, wantSuffix)
	}
}

func TestRuntimeAppRaceTestVerboseHandlesStandaloneBooleanFlag(t *testing.T) {
	if !runtimeAppRaceTestVerbose([]string{"-test.v", "-test.timeout=10m"}) {
		t.Fatal("standalone -test.v was not recognized")
	}
	if runtimeAppRaceTestVerbose([]string{"-test.v=false"}) {
		t.Fatal("explicit false verbose flag was enabled")
	}
}

func runtimeAppRaceTestEnvironmentValue(environment []string, name string) string {
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && key == name {
			return value
		}
	}
	return ""
}
