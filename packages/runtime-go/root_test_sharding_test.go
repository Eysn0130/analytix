package runtimego

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"analytix.local/runtime-go/internal/proc"
	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

const (
	runtimeServerRootTestShardChildEnvironment        = "ANALYTIX_GO_ROOT_TEST_SHARD_CHILD"
	runtimeServerRootTestShardIndexEnvironment        = "ANALYTIX_GO_ROOT_TEST_SHARD_INDEX"
	runtimeServerRootTestUserConfigEnvironment        = "ANALYTIX_GO_TEST_USER_CONFIG_ROOT"
	runtimeServerRootTestProductionCommandEnvironment = "ANALYTIX_GO_ROOT_TEST_PRODUCTION_COMMAND"
	runtimeServerRootTestRaceOptionsEnvironment       = "GORACE"
	runtimeServerRootTestParallelismEnvironment       = "GOMAXPROCS"
	runtimeServerRootTestLogMagic                     = "# test log\n"
)

type runtimeServerRootTestShard struct {
	index      int
	tests      []string
	command    *exec.Cmd
	stdout     bytes.Buffer
	stderr     bytes.Buffer
	testLog    string
	cleanup    func() error
	waitErr    error
	started    bool
	finished   bool
	startedAt  time.Time
	finishedAt time.Time
}

func runRuntimeServerRootTestShards() (int, bool) {
	args := os.Args[1:]
	if !runtimeServerRootTestShardingEligible(
		args,
		os.Getenv(runtimeServerRootTestShardChildEnvironment),
		os.Getenv(runtimeServerRootTestUserConfigEnvironment),
	) {
		return 0, false
	}

	ctx, cancel, err := runtimeServerRootTestCoordinatorContext(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "analytix root test shard timeout is invalid:", err)
		return 2, true
	}
	defer cancel()

	tests, err := runtimeServerRootTestNames(ctx, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "analytix root test shard discovery failed:", err)
		return 2, true
	}
	if len(tests) < 2 {
		return 0, false
	}
	parentTestLog := runtimeServerRootTestFlagValue(args, "-test.testlogfile")
	childTestLogParent, uncacheableRun := runtimeServerRootTestChildLogParent(args, parentTestLog)
	shards := buildRuntimeServerRootTestShards(tests)
	defer cleanupRuntimeServerRootTestLogs(shards)

	jobs := make(chan int)
	var workers sync.WaitGroup
	workerCount := runtimeServerRootTestShardWorkerCount
	if workerCount > len(shards) {
		workerCount = len(shards)
	}
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				runRuntimeServerRootTestShard(ctx, args, childTestLogParent, &shards[index])
			}
		}()
	}
	for index := range shards {
		select {
		case jobs <- index:
		case <-ctx.Done():
			shards[index].waitErr = ctx.Err()
		}
	}
	close(jobs)
	workers.Wait()
	cleanupRuntimeServerRootTestShardEnvironments(shards, workerCount)

	failed := false
	for index := range shards {
		if shards[index].waitErr != nil {
			failed = true
		}
	}
	if uncacheableRun && parentTestLog != "" {
		if err := os.WriteFile(parentTestLog, []byte(runtimeServerRootTestLogMagic), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "analytix root test shard uncacheable log failed:", err)
			failed = true
		}
	} else if err := mergeRuntimeServerRootTestLogs(parentTestLog, shards); err != nil {
		fmt.Fprintln(os.Stderr, "analytix root test shard log merge failed:", err)
		failed = true
	}
	verbose := runtimeServerRootTestVerbose(args)
	for index := range shards {
		shard := &shards[index]
		if verbose || shard.waitErr != nil {
			writeRuntimeServerRootTestShardOutput(os.Stdout, shard.stdout.Bytes())
			_, _ = os.Stderr.Write(shard.stderr.Bytes())
		}
		if shard.waitErr != nil {
			fmt.Fprintf(
				os.Stderr,
				"analytix root test shard %d (%s) failed: %v\n",
				shard.index,
				strings.Join(shard.tests, ","),
				shard.waitErr,
			)
		}
	}
	if failed {
		writeRuntimeServerRootTestTimingSummary(os.Stderr, shards, 25)
		return 1, true
	}
	if verbose {
		fmt.Fprintln(os.Stdout, "PASS")
	}
	return 0, true
}

func runRuntimeServerRootTestShard(
	ctx context.Context,
	args []string,
	parentTestLog string,
	shard *runtimeServerRootTestShard,
) {
	if err := ctx.Err(); err != nil {
		shard.waitErr = err
		return
	}
	childArgs := runtimeServerRootTestChildArgs(args, shard.tests, parentTestLog, shard.index)
	shard.testLog = runtimeServerRootTestFlagValue(childArgs, "-test.testlogfile")
	shard.command = exec.Command(os.Args[0], childArgs...)
	childEnvironment, cleanup, err := userconfigtest.CreateChildEnvironment(os.Environ())
	if err != nil {
		shard.waitErr = fmt.Errorf("create isolated child config: %w", err)
		return
	}
	shard.cleanup = cleanup
	environmentOverrides := []string{
		// The production-command contract builds inside its own isolated
		// shard so compilation overlaps independent tests. Clear any caller
		// value rather than sharing a coordinator-owned binary whose serialized
		// build would consume the root package's fixed deadline.
		runtimeServerRootTestProductionCommandEnvironment + "=",
		runtimeServerRootTestShardChildEnvironment + "=1",
		fmt.Sprintf("%s=%d", runtimeServerRootTestShardIndexEnvironment, shard.index),
	}
	if runtimeServerRootTestRaceExitSleepMillis >= 0 {
		environmentOverrides = append(
			environmentOverrides,
			runtimeServerRootTestRaceOptionsEnvironment+"="+runtimeServerRootTestRaceOptionsWithExitSleep(
				runtimeServerRootTestEnvironmentValue(childEnvironment, runtimeServerRootTestRaceOptionsEnvironment),
				runtimeServerRootTestRaceExitSleepMillis,
			),
		)
	}
	if parallelism, configured := runtimeServerRootTestChildParallelism(
		childEnvironment,
		runtime.GOMAXPROCS(0),
		runtimeServerRootTestRaceExitSleepMillis >= 0,
	); configured {
		environmentOverrides = append(
			environmentOverrides,
			runtimeServerRootTestParallelismEnvironment+"="+parallelism,
		)
	}
	shard.command.Env = runtimeServerRootTestEnvironmentWithOverrides(childEnvironment, environmentOverrides...)
	shard.command.Stdout = &shard.stdout
	shard.command.Stderr = &shard.stderr

	job, err := proc.StartTracked(shard.command)
	if err != nil {
		shard.waitErr = err
		return
	}
	shard.started = true
	shard.startedAt = time.Now()
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
	shard.finishedAt = time.Now()
}

func writeRuntimeServerRootTestTimingSummary(
	w io.Writer,
	shards []runtimeServerRootTestShard,
	limit int,
) {
	type completedShard struct {
		name     string
		duration time.Duration
	}
	completed := make([]completedShard, 0, len(shards))
	started := 0
	passed := 0
	failed := 0
	for _, shard := range shards {
		if shard.started {
			started++
		}
		if !shard.finished || shard.startedAt.IsZero() || shard.finishedAt.Before(shard.startedAt) {
			continue
		}
		if shard.waitErr == nil {
			passed++
		} else {
			failed++
		}
		completed = append(completed, completedShard{
			name: strings.Join(shard.tests, ","), duration: shard.finishedAt.Sub(shard.startedAt),
		})
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].duration > completed[j].duration })
	if limit < 0 {
		limit = 0
	}
	if limit > len(completed) {
		limit = len(completed)
	}
	fmt.Fprintf(
		w,
		"analytix root test timing summary: total=%d started=%d completed=%d passed=%d failed=%d unstarted=%d slowest=%d\n",
		len(shards), started, len(completed), passed, failed, len(shards)-started, limit,
	)
	for index := 0; index < limit; index++ {
		fmt.Fprintf(w, "analytix root test slow shard: duration=%s test=%s\n", completed[index].duration.Round(time.Millisecond), completed[index].name)
	}
}

func cleanupRuntimeServerRootTestShardEnvironments(shards []runtimeServerRootTestShard, workerCount int) {
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(shards) {
		workerCount = len(shards)
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				shard := &shards[index]
				if shard.cleanup == nil {
					continue
				}
				if err := shard.cleanup(); err != nil {
					shard.waitErr = errors.Join(shard.waitErr, fmt.Errorf("cleanup isolated child config: %w", err))
				}
				shard.cleanup = nil
			}
		}()
	}
	for index := range shards {
		if shards[index].cleanup != nil {
			jobs <- index
		}
	}
	close(jobs)
	workers.Wait()
}

func buildRuntimeServerRootTestProductionCommand(ctx context.Context, destination string) ([]byte, error) {
	goBin := strings.TrimSpace(os.Getenv("ANALYTIX_GO_BIN"))
	if goBin == "" {
		goBin = "go"
	}
	arguments := append([]string{"build"}, runtimeServerRootTestProductionCommandBuildFlags...)
	arguments = append(arguments, "-tags", "analytix_prod", "-o", destination, "./cmd/runtime-server")
	command := exec.CommandContext(ctx, goBin, arguments...)
	command.Env = os.Environ()
	return command.CombinedOutput()
}

func runtimeServerRootTestEnvironmentWithOverrides(base []string, overrides ...string) []string {
	names := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		name, _, found := strings.Cut(entry, "=")
		if found {
			names[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := names[name]; replaced {
				continue
			}
		}
		result = append(result, entry)
	}
	return append(result, overrides...)
}

func runtimeServerRootTestEnvironmentValue(environment []string, name string) string {
	for _, entry := range environment {
		entryName, value, found := strings.Cut(entry, "=")
		if found && entryName == name {
			return value
		}
	}
	return ""
}

func runtimeServerRootTestChildParallelism(
	environment []string,
	current int,
	race bool,
) (string, bool) {
	if current < 1 || strings.TrimSpace(
		runtimeServerRootTestEnvironmentValue(environment, runtimeServerRootTestParallelismEnvironment),
	) != "" {
		return "", false
	}
	// Every isolated child inherits the host-wide default. With nine ordinary
	// workers on a 12-CPU host that created 108 runnable Ps, starving nested
	// production-command startup even though the outer package is serialized.
	// Keep real process parallelism but divide ordinary CPU ownership across the
	// active workers. Race children retain the proven one-third cap because
	// instrumentation spends substantial time outside ordinary Go scheduling.
	target := (current + runtimeServerRootTestShardWorkerCount - 1) / runtimeServerRootTestShardWorkerCount
	if race {
		target = (current + 2) / 3
	}
	if current > 1 && target < 2 {
		target = 2
	}
	return strconv.Itoa(target), true
}

func runtimeServerRootTestRaceOptionsWithExitSleep(options string, milliseconds int) string {
	if milliseconds < 0 {
		return options
	}
	fields := strings.Fields(options)
	result := make([]string, 0, len(fields)+1)
	for _, field := range fields {
		name, _, found := strings.Cut(field, "=")
		if found && name == "atexit_sleep_ms" {
			continue
		}
		result = append(result, field)
	}
	return strings.Join(append(result, fmt.Sprintf("atexit_sleep_ms=%d", milliseconds)), " ")
}

func runtimeServerRootTestShardingEligible(args []string, childMarker, inheritedUserConfig string) bool {
	if childMarker != "" || inheritedUserConfig != "" || runtimeServerRootTestShardWorkerCount < 2 {
		return false
	}
	for _, arg := range args {
		for _, prefix := range []string{
			"-test.run", "-test.list", "-test.bench", "-test.fuzz", "-test.failfast",
			"-test.coverprofile", "-test.gocoverdir", "-test.cpuprofile",
			"-test.memprofile", "-test.blockprofile", "-test.mutexprofile",
			"-test.trace", "-test.outputdir", "-test.artifacts", "-test.shuffle",
		} {
			if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
				return false
			}
		}
	}
	return runtimeServerRootTestFlagValue(args, "-test.v") != "test2json"
}

func runtimeServerRootTestCoordinatorContext(args []string) (context.Context, context.CancelFunc, error) {
	base, stopSignals := signal.NotifyContext(context.Background(), runtimeServerRootTestTerminationSignals()...)
	timeoutValue := runtimeServerRootTestFlagValue(args, "-test.timeout")
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

func runtimeServerRootTestNames(ctx context.Context, args []string) ([]string, error) {
	listArgs := runtimeServerRootTestArgsWithoutFlags(args, "-test.run", "-test.list", "-test.testlogfile")
	listArgs = append(listArgs, "-test.list=.")
	command := exec.Command(os.Args[0], listArgs...)
	childEnvironment, cleanup, err := userconfigtest.CreateChildEnvironment(os.Environ())
	if err != nil {
		return nil, fmt.Errorf("create discovery config isolation: %w", err)
	}
	environmentOverrides := []string{runtimeServerRootTestShardChildEnvironment + "=1"}
	if runtimeServerRootTestRaceExitSleepMillis >= 0 {
		environmentOverrides = append(
			environmentOverrides,
			runtimeServerRootTestRaceOptionsEnvironment+"="+runtimeServerRootTestRaceOptionsWithExitSleep(
				runtimeServerRootTestEnvironmentValue(childEnvironment, runtimeServerRootTestRaceOptionsEnvironment),
				runtimeServerRootTestRaceExitSleepMillis,
			),
		)
	}
	command.Env = runtimeServerRootTestEnvironmentWithOverrides(childEnvironment, environmentOverrides...)
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
		return nil, fmt.Errorf("list root tests: %w: %s", err, strings.TrimSpace(output.String()))
	}

	testName := regexp.MustCompile(`^(?:Test|Example|Fuzz)\S*$`)
	seen := make(map[string]struct{})
	tests := make([]string, 0, 320)
	for _, line := range strings.Split(output.String(), "\n") {
		name := strings.TrimSpace(line)
		if !testName.MatchString(name) {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate root test %q", name)
		}
		seen[name] = struct{}{}
		tests = append(tests, name)
	}
	if len(tests) == 0 {
		return nil, errors.New("root test inventory is empty")
	}
	return tests, nil
}

func buildRuntimeServerRootTestShards(tests []string) []runtimeServerRootTestShard {
	if len(tests) == 0 {
		return nil
	}
	sharedProcessTests := make([]string, 0, 32)
	productTests := make([]string, 0, len(tests))
	for _, test := range tests {
		if runtimeServerRootTestSharedProcessSafe(test) {
			sharedProcessTests = append(sharedProcessTests, test)
		} else {
			productTests = append(productTests, test)
		}
	}
	earlyTests := make([]string, 0, len(productTests))
	ordinaryTests := make([]string, 0, len(productTests))
	for _, test := range productTests {
		if runtimeServerRootTestShardRunsEarly(test) {
			earlyTests = append(earlyTests, test)
		} else {
			ordinaryTests = append(ordinaryTests, test)
		}
	}
	neutralOrder := append(earlyTests, interleaveRuntimeServerRootTests(ordinaryTests)...)
	shards := make([]runtimeServerRootTestShard, 0, len(productTests)+1)
	if len(sharedProcessTests) > 0 {
		// These tests exercise only pure coordinator or stateless fixture
		// contracts. Keeping them in one child prevents validation-only checks
		// from multiplying race-runtime startup cost while every stateful
		// product and lifecycle test retains a one-test process boundary.
		shards = append(shards, runtimeServerRootTestShard{
			index: len(shards), tests: sharedProcessTests,
		})
	}
	for _, test := range neutralOrder {
		shards = append(shards, runtimeServerRootTestShard{
			index: len(shards), tests: []string{test},
		})
	}
	return shards
}

func interleaveRuntimeServerRootTests(tests []string) []string {
	if len(tests) == 0 {
		return nil
	}
	workerCount := runtimeServerRootTestShardWorkerCount
	if workerCount > len(tests) {
		workerCount = len(tests)
	}
	laneSize := (len(tests) + workerCount - 1) / workerCount
	ordered := make([]string, 0, len(tests))
	for offset := 0; offset < laneSize; offset++ {
		for lane := 0; lane < workerCount; lane++ {
			testIndex := lane*laneSize + offset
			if testIndex < len(tests) {
				ordered = append(ordered, tests[testIndex])
			}
		}
	}
	return ordered
}

func runtimeServerRootTestSharedProcessSafe(test string) bool {
	if runtimeServerRootTestRaceExitSleepMillis < 0 {
		return false
	}
	if strings.HasPrefix(test, "TestRuntimeServerRootTest") &&
		test != "TestRuntimeServerRootTestRaceDetectorReportsWithZeroExitSleep" {
		return true
	}
	switch test {
	case "TestG1ShadowRoutesMatchTypeScriptContract",
		"TestG1ShadowProductBoundaryMatchesContract",
		"TestG1ShadowDoesNotExposeRendererVisibleGoRoute",
		"TestG2ShadowRoutesReplayTypeScriptContract",
		"TestG2ShadowProductBoundaryMatchesContract",
		"TestG2ShadowDoesNotExposeRendererVisibleGoRoute",
		"TestG4ToolsConformanceOutputMatchesTypeScriptContract",
		"TestMCPProtocolVersionSingleSourceOfTruth",
		"TestMCPNormalizationMatchesReasonixNamespacing",
		"TestMCPRedactionRemovesAuthMaterial",
		"TestMCPManagerRedactsCredentialedFixtureDiagnostics",
		"TestRuntimeMCPMatrixScaffoldRunsFixtureAndSkipsCredentialedWithoutEnv",
		"TestReasonixSuperiorityMatrixClosesCodeStageOnly",
		"TestReasonixAbsorptionContractSeparatesCodeStageFromLiveCutover",
		"TestKunAnalytixBaselineGuardIsFullFunctionAndImmutable",
		"TestReasonixAbsorptionMatrixProtectsKunAnalytixBaseline",
		"TestGoalEvidenceCommandMatchesReasonixCompatibility",
		"TestGoalEvidenceRepeatingSameBlockBecomesErrored",
		"TestReasonixIntegrationTopologyBindsEngineToAnalytixContracts",
		"TestReasonixCapabilityAuditMatrixIsMachineReadable",
		"TestProductRegressionMatrixCoversRequiredHighRiskSurfaces",
		"TestRootG5UsageHelpersDelegateToInternalServer":
		return true
	default:
		return false
	}
}

func runtimeServerRootTestShardRunsEarly(test string) bool {
	// These full-stack shards have fixed product deadlines inside their test
	// process. Neutral cluster interleaving can otherwise start them only after
	// many resource-heavy shards, where host contention can starve a nested
	// production command without exercising the intended product boundary.
	// Keep the one-test process and worker contracts intact while admitting them
	// in wave one.
	switch test {
	case "TestRuntimeServerCommandSeparatesGeneralOnlyFromHostPolicyCaseBoundary",
		"TestRuntimeServerDirectLightweightPromptsDoNotAdvertiseToolsToProvider":
		return true
	default:
		return false
	}
}

func runtimeServerRootTestChildArgs(
	args []string,
	tests []string,
	parentTestLog string,
	shardIndex int,
) []string {
	child := runtimeServerRootTestArgsWithoutFlags(args, "-test.run", "-test.list", "-test.testlogfile")
	quoted := make([]string, 0, len(tests))
	for _, test := range tests {
		quoted = append(quoted, regexp.QuoteMeta(test))
	}
	child = append(child, "-test.run=^(?:"+strings.Join(quoted, "|")+")$")
	if parentTestLog != "" {
		child = append(child, fmt.Sprintf("-test.testlogfile=%s.shard-%03d", parentTestLog, shardIndex))
	}
	return child
}

func runtimeServerRootTestChildLogParent(args []string, parent string) (string, bool) {
	if runtimeServerRootTestFlagValue(args, "-test.count") != "1" {
		return parent, false
	}
	// Every A0 matrix deliberately disables result-cache reuse. Child
	// dependency logs can reach many megabytes per filesystem-heavy shard and
	// cannot make a -count=1 result cacheable, so keep only the parent protocol
	// header in that exact mode. Default/cacheable invocations still collect and
	// merge the complete dependency log.
	return "", true
}

func runtimeServerRootTestArgsWithoutFlags(args []string, names ...string) []string {
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

func runtimeServerRootTestFlagValue(args []string, name string) string {
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

func runtimeServerRootTestVerbose(args []string) bool {
	for _, arg := range args {
		if arg == "-test.v" {
			return true
		}
	}
	value := runtimeServerRootTestFlagValue(args, "-test.v")
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err != nil || parsed
}

func mergeRuntimeServerRootTestLogs(parent string, shards []runtimeServerRootTestShard) error {
	if parent == "" {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil || !strings.HasPrefix(cwd, string(os.PathSeparator)) {
		return errors.New("resolve absolute root test working directory")
	}
	var merged bytes.Buffer
	merged.WriteString(runtimeServerRootTestLogMagic)
	for _, shard := range shards {
		if shard.testLog == "" {
			return errors.New("root test shard omitted its test log path")
		}
		body, err := os.ReadFile(shard.testLog)
		if err != nil {
			return fmt.Errorf("read shard %d test log: %w", shard.index, err)
		}
		if !bytes.HasPrefix(body, []byte(runtimeServerRootTestLogMagic)) {
			return fmt.Errorf("shard %d test log is malformed", shard.index)
		}
		body = body[len(runtimeServerRootTestLogMagic):]
		if len(body) == 0 {
			continue
		}
		if body[len(body)-1] != '\n' {
			return fmt.Errorf("shard %d test log is truncated", shard.index)
		}
		fmt.Fprintf(&merged, "chdir %s\n", cwd)
		merged.Write(body)
	}
	return os.WriteFile(parent, merged.Bytes(), 0o600)
}

func cleanupRuntimeServerRootTestLogs(shards []runtimeServerRootTestShard) {
	for _, shard := range shards {
		if shard.testLog == "" {
			continue
		}
		_ = os.Remove(shard.testLog)
	}
}

func writeRuntimeServerRootTestShardOutput(destination *os.File, output []byte) {
	lines := bytes.Split(output, []byte{'\n'})
	for _, line := range lines {
		if bytes.Equal(bytes.TrimSpace(line), []byte("PASS")) || len(line) == 0 {
			continue
		}
		_, _ = destination.Write(append(append([]byte(nil), line...), '\n'))
	}
}

func sortedRuntimeServerRootTestInventory(shards []runtimeServerRootTestShard) []string {
	var tests []string
	for _, shard := range shards {
		tests = append(tests, shard.tests...)
	}
	sort.Strings(tests)
	return tests
}
