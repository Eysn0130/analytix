//go:build race

package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"analytix.local/runtime-go/internal/proc"
	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

const (
	runtimeAppRaceTestShardChildEnvironment = "ANALYTIX_GO_RUNTIMEAPP_RACE_TEST_SHARD_CHILD"
	runtimeAppRaceTestShardIndexEnvironment = "ANALYTIX_GO_RUNTIMEAPP_RACE_TEST_SHARD_INDEX"
	runtimeAppRaceTestUserConfigEnvironment = "ANALYTIX_GO_TEST_USER_CONFIG_ROOT"
	runtimeAppRaceTestLogMagic              = "# test log\n"
	runtimeAppRaceTestShardWorkerCount      = 3
)

type runtimeAppRaceTestShard struct {
	index    int
	test     string
	command  *exec.Cmd
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	testLog  string
	waitErr  error
	started  bool
	finished bool
}

func runRuntimeAppRaceTestShards() (int, bool) {
	args := os.Args[1:]
	if !runtimeAppRaceTestShardingEligible(
		args,
		os.Getenv(runtimeAppRaceTestShardChildEnvironment),
		os.Getenv(runtimeAppRaceTestUserConfigEnvironment),
	) {
		return 0, false
	}

	ctx, cancel, err := runtimeAppRaceTestCoordinatorContext(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "analytix runtimeapp race test shard timeout is invalid:", err)
		return 2, true
	}
	defer cancel()

	tests, err := runtimeAppRaceTestNames(ctx, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "analytix runtimeapp race test shard discovery failed:", err)
		return 2, true
	}
	if len(tests) < 2 {
		return 0, false
	}

	parentTestLog := runtimeAppRaceTestFlagValue(args, "-test.testlogfile")
	shards := buildRuntimeAppRaceTestShards(tests)
	defer cleanupRuntimeAppRaceTestLogs(shards)

	jobs := make(chan int)
	var workers sync.WaitGroup
	workerCount := runtimeAppRaceTestShardWorkerCount
	if workerCount > len(shards) {
		workerCount = len(shards)
	}
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				runRuntimeAppRaceTestShard(ctx, args, parentTestLog, &shards[index])
			}
		}()
	}
	for _, index := range runtimeAppRaceTestShardSchedule(len(shards), workerCount) {
		select {
		case jobs <- index:
		case <-ctx.Done():
			shards[index].waitErr = ctx.Err()
		}
	}
	close(jobs)
	workers.Wait()

	failed := false
	for index := range shards {
		if shards[index].waitErr != nil {
			failed = true
		}
	}
	if err := mergeRuntimeAppRaceTestLogs(parentTestLog, shards); err != nil {
		fmt.Fprintln(os.Stderr, "analytix runtimeapp race test shard log merge failed:", err)
		failed = true
	}
	verbose := runtimeAppRaceTestVerbose(args)
	for index := range shards {
		shard := &shards[index]
		if verbose || shard.waitErr != nil {
			writeRuntimeAppRaceTestShardOutput(os.Stdout, shard.stdout.Bytes())
			_, _ = os.Stderr.Write(shard.stderr.Bytes())
		}
		if shard.waitErr != nil {
			fmt.Fprintf(
				os.Stderr,
				"analytix runtimeapp race test shard %d (%s) failed: %v\n",
				shard.index,
				shard.test,
				shard.waitErr,
			)
		}
	}
	if failed {
		return 1, true
	}
	if verbose {
		fmt.Fprintln(os.Stdout, "PASS")
	}
	return 0, true
}

func runRuntimeAppRaceTestShard(
	ctx context.Context,
	args []string,
	parentTestLog string,
	shard *runtimeAppRaceTestShard,
) {
	if err := ctx.Err(); err != nil {
		shard.waitErr = err
		return
	}
	childArgs := runtimeAppRaceTestChildArgs(args, shard.test, parentTestLog, shard.index)
	shard.testLog = runtimeAppRaceTestFlagValue(childArgs, "-test.testlogfile")
	shard.command = exec.Command(os.Args[0], childArgs...)
	childEnvironment, cleanup, err := runtimeAppRaceTestChildEnvironment(os.Environ())
	if err != nil {
		shard.waitErr = fmt.Errorf("create isolated child config: %w", err)
		return
	}
	defer func() {
		if err := cleanup(); err != nil {
			shard.waitErr = errors.Join(shard.waitErr, fmt.Errorf("cleanup isolated child config: %w", err))
		}
	}()
	shard.command.Env = append(
		childEnvironment,
		runtimeAppRaceTestShardChildEnvironment+"=1",
		fmt.Sprintf("%s=%d", runtimeAppRaceTestShardIndexEnvironment, shard.index),
	)
	shard.command.Stdout = &shard.stdout
	shard.command.Stderr = &shard.stderr

	job, err := proc.StartTracked(shard.command)
	if err != nil {
		shard.waitErr = err
		return
	}
	shard.started = true
	done := make(chan error, 1)
	go func() { done <- shard.command.Wait() }()
	select {
	case shard.waitErr = <-done:
	case <-ctx.Done():
		proc.KillTracked(shard.command, job)
		waitErr := <-done
		shard.waitErr = errors.Join(ctx.Err(), waitErr)
	}
	proc.ReapTracked(shard.command, job)
	shard.finished = true
}

func runtimeAppRaceTestShardingEligible(args []string, childMarker, inheritedUserConfig string) bool {
	if childMarker != "" || inheritedUserConfig != "" || runtimeAppRaceTestShardWorkerCount < 2 {
		return false
	}
	for _, arg := range args {
		for _, prefix := range []string{
			"-test.run", "-test.skip", "-test.list", "-test.bench", "-test.fuzz", "-test.failfast",
			"-test.coverprofile", "-test.gocoverdir", "-test.cpuprofile",
			"-test.memprofile", "-test.blockprofile", "-test.mutexprofile",
			"-test.trace", "-test.outputdir", "-test.artifacts", "-test.shuffle",
			"-test.cpu", "-test.parallel",
		} {
			if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
				return false
			}
		}
	}
	return runtimeAppRaceTestFlagValue(args, "-test.v") != "test2json"
}

func runtimeAppRaceTestCoordinatorContext(args []string) (context.Context, context.CancelFunc, error) {
	base, stopSignals := signal.NotifyContext(context.Background(), runtimeAppRaceTestTerminationSignals()...)
	timeoutValue := runtimeAppRaceTestFlagValue(args, "-test.timeout")
	if timeoutValue == "" {
		return base, stopSignals, nil
	}
	timeout, err := time.ParseDuration(timeoutValue)
	if err != nil || timeout < 0 {
		stopSignals()
		if err == nil {
			err = fmt.Errorf("negative duration %q", timeoutValue)
		}
		return nil, nil, err
	}
	if timeout == 0 {
		return base, stopSignals, nil
	}
	grace := timeout / 20
	if grace < 250*time.Millisecond {
		grace = 250 * time.Millisecond
	}
	if grace > 10*time.Second {
		grace = 10 * time.Second
	}
	if grace >= timeout {
		stopSignals()
		return nil, nil, fmt.Errorf("duration %q is too short for contained cleanup", timeoutValue)
	}
	ctx, cancelTimeout := context.WithTimeout(base, timeout-grace)
	return ctx, func() {
		cancelTimeout()
		stopSignals()
	}, nil
}

func runtimeAppRaceTestNames(ctx context.Context, args []string) ([]string, error) {
	listArgs := runtimeAppRaceTestArgsWithoutFlags(args, "-test.run", "-test.skip", "-test.list", "-test.testlogfile")
	listArgs = append(listArgs, "-test.list=.")
	command := exec.Command(os.Args[0], listArgs...)
	childEnvironment, cleanup, err := runtimeAppRaceTestChildEnvironment(os.Environ())
	if err != nil {
		return nil, fmt.Errorf("create discovery config isolation: %w", err)
	}
	command.Env = append(childEnvironment, runtimeAppRaceTestShardChildEnvironment+"=1")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	job, err := proc.StartTracked(command)
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		proc.KillTracked(command, job)
		err = errors.Join(ctx.Err(), <-done)
	}
	proc.ReapTracked(command, job)
	err = errors.Join(err, cleanup())
	if err != nil {
		return nil, fmt.Errorf("list runtimeapp race tests: %w: %s", err, strings.TrimSpace(output.String()))
	}

	testName := regexp.MustCompile(`^(?:Test|Example|Fuzz)\S*$`)
	seen := make(map[string]struct{})
	tests := make([]string, 0, 128)
	for _, line := range strings.Split(output.String(), "\n") {
		name := strings.TrimSpace(line)
		if !testName.MatchString(name) {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate runtimeapp race test %q", name)
		}
		seen[name] = struct{}{}
		tests = append(tests, name)
	}
	if len(tests) == 0 {
		return nil, errors.New("runtimeapp race test inventory is empty")
	}
	return tests, nil
}

func runtimeAppRaceTestChildEnvironment(base []string) ([]string, func() error, error) {
	return userconfigtest.CreateChildEnvironment(base)
}

func buildRuntimeAppRaceTestShards(tests []string) []runtimeAppRaceTestShard {
	shards := make([]runtimeAppRaceTestShard, 0, len(tests))
	for index, test := range tests {
		shards = append(shards, runtimeAppRaceTestShard{index: index, test: test})
	}
	return shards
}

func runtimeAppRaceTestShardSchedule(shardCount, workerCount int) []int {
	if shardCount <= 0 {
		return nil
	}
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > shardCount {
		workerCount = shardCount
	}
	stride := (shardCount + workerCount - 1) / workerCount
	order := make([]int, 0, shardCount)
	for offset := 0; offset < stride; offset++ {
		for lane := 0; lane < workerCount; lane++ {
			index := lane*stride + offset
			if index < shardCount {
				order = append(order, index)
			}
		}
	}
	return order
}

func runtimeAppRaceTestChildArgs(
	args []string,
	test string,
	parentTestLog string,
	shardIndex int,
) []string {
	child := runtimeAppRaceTestArgsWithoutFlags(
		args,
		"-test.run",
		"-test.skip",
		"-test.list",
		"-test.testlogfile",
	)
	child = append(child, "-test.run=^"+regexp.QuoteMeta(test)+"$")
	if parentTestLog != "" {
		child = append(child, fmt.Sprintf("-test.testlogfile=%s.shard-%03d", parentTestLog, shardIndex))
	}
	return child
}

func runtimeAppRaceTestArgsWithoutFlags(args []string, names ...string) []string {
	blocked := make(map[string]struct{}, len(names))
	for _, name := range names {
		blocked[name] = struct{}{}
	}
	result := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		name := arg
		if equals := strings.IndexByte(arg, '='); equals >= 0 {
			name = arg[:equals]
		}
		if _, remove := blocked[name]; !remove {
			result = append(result, arg)
			continue
		}
		if arg == name && index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
			index++
		}
	}
	return result
}

func runtimeAppRaceTestFlagValue(args []string, name string) string {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimPrefix(arg, name+"=")
		}
		if arg == name && index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
			return args[index+1]
		}
	}
	return ""
}

func runtimeAppRaceTestVerbose(args []string) bool {
	for _, arg := range args {
		if arg == "-test.v" {
			return true
		}
	}
	value := runtimeAppRaceTestFlagValue(args, "-test.v")
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err != nil || parsed
}

func mergeRuntimeAppRaceTestLogs(parent string, shards []runtimeAppRaceTestShard) error {
	if parent == "" {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil || !filepath.IsAbs(cwd) {
		return errors.New("resolve absolute runtimeapp race test working directory")
	}
	var merged bytes.Buffer
	merged.WriteString(runtimeAppRaceTestLogMagic)
	for _, shard := range shards {
		if shard.testLog == "" {
			return errors.New("runtimeapp race test shard omitted its test log path")
		}
		body, err := os.ReadFile(shard.testLog)
		if err != nil {
			return fmt.Errorf("read runtimeapp race shard %d test log: %w", shard.index, err)
		}
		if !bytes.HasPrefix(body, []byte(runtimeAppRaceTestLogMagic)) {
			return fmt.Errorf("runtimeapp race shard %d test log is malformed", shard.index)
		}
		body = body[len(runtimeAppRaceTestLogMagic):]
		if len(body) == 0 {
			continue
		}
		if body[len(body)-1] != '\n' {
			return fmt.Errorf("runtimeapp race shard %d test log is truncated", shard.index)
		}
		fmt.Fprintf(&merged, "chdir %s\n", cwd)
		merged.Write(body)
	}
	return os.WriteFile(parent, merged.Bytes(), 0o600)
}

func cleanupRuntimeAppRaceTestLogs(shards []runtimeAppRaceTestShard) {
	for _, shard := range shards {
		if shard.testLog == "" {
			continue
		}
		_ = os.Remove(shard.testLog)
	}
}

func writeRuntimeAppRaceTestShardOutput(destination *os.File, output []byte) {
	lines := bytes.Split(output, []byte{'\n'})
	for _, line := range lines {
		if bytes.Equal(bytes.TrimSpace(line), []byte("PASS")) || len(line) == 0 {
			continue
		}
		_, _ = destination.Write(append(append([]byte(nil), line...), '\n'))
	}
}

func sortedRuntimeAppRaceTestInventory(shards []runtimeAppRaceTestShard) []string {
	tests := make([]string, 0, len(shards))
	for _, shard := range shards {
		tests = append(tests, shard.test)
	}
	sort.Strings(tests)
	return tests
}
