package loop

import (
	"context"
	"errors"
	"testing"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type toolAdmissionLeaseDriver struct {
	*toolStepDriverStub
	gate               *effectgateapp.Gate
	readyPersisted     chan struct{}
	allowPersistReturn chan struct{}
	gateIssued         chan struct{}
	allowGateReturn    chan struct{}
}

func (driver *toolAdmissionLeaseDriver) AcquireToolCallAdmission(ctx context.Context, pending appmodel.PendingToolCall) (context.Context, func(), error) {
	return driver.gate.AcquireEffect(ctx, pending.SecurityContext)
}

func (driver *toolAdmissionLeaseDriver) PersistToolCallReady(ctx context.Context, threadID, turnID string, call domainmodel.ToolCall, count int, securityContext domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant) (string, error) {
	if ctx == nil || ctx.Err() != nil {
		return "", errors.New("ready persistence lost its admission context")
	}
	driver.readyPersisted <- struct{}{}
	<-driver.allowPersistReturn
	return driver.toolStepDriverStub.PersistToolCallReady(ctx, threadID, turnID, call, count, securityContext, grant)
}

func (driver *toolAdmissionLeaseDriver) RequestApproval(ctx context.Context, pending appmodel.PendingToolCall) (string, error) {
	nestedCtx, release, err := driver.gate.AcquireEffect(ctx, pending.SecurityContext)
	if err != nil {
		return "", err
	}
	defer release()
	if nestedCtx == nil || nestedCtx.Err() != nil {
		return "", errors.New("approval gate lost its nested admission lease")
	}
	driver.gateIssued <- struct{}{}
	<-driver.allowGateReturn
	return driver.toolStepDriverStub.RequestApproval(nestedCtx, pending)
}

func (driver *toolAdmissionLeaseDriver) RequestUserInput(ctx context.Context, pending appmodel.PendingToolCall) (string, error) {
	nestedCtx, release, err := driver.gate.AcquireEffect(ctx, pending.SecurityContext)
	if err != nil {
		return "", err
	}
	defer release()
	if nestedCtx == nil || nestedCtx.Err() != nil {
		return "", errors.New("user-input gate lost its nested admission lease")
	}
	driver.gateIssued <- struct{}{}
	<-driver.allowGateReturn
	return driver.toolStepDriverStub.RequestUserInput(nestedCtx, pending)
}

func TestGrantReadyLeaseBlocksContextTransitionUntilDurableAdmission(t *testing.T) {
	gate := effectgateapp.New()
	driver := &toolAdmissionLeaseDriver{
		toolStepDriverStub: &toolStepDriverStub{},
		gate:               gate,
		readyPersisted:     make(chan struct{}, 1),
		allowPersistReturn: make(chan struct{}),
		gateIssued:         make(chan struct{}, 1),
		allowGateReturn:    make(chan struct{}),
	}
	securityContext := newLoopGeneralContextV2(t, "thread-ready-lease", "turn-ready-lease", t.TempDir())
	resultCh := make(chan error, 1)
	go func() {
		_, err := RunToolStep(context.Background(), hostToolStepInputForTest(ToolStepInput{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: "provider-ready-lease",
			Workspace: securityContext.WorkspaceRealPath, EffectiveMaxModelSteps: 2,
			ToolCalls:   []domainmodel.ToolCall{{ID: "call-ready-lease", Name: "read", Arguments: []byte(`{}`)}},
			ToolSchemas: zeroArgumentToolSchemas("read"), AdvertisedTools: advertisedToolNames("read"),
			SecurityContext: securityContext, Driver: driver,
		}))
		resultCh <- err
	}()
	waitToolAdmissionSignal(t, driver.readyPersisted, "ready persistence did not start")
	transition := beginToolAdmissionTransition(gate, securityContext)
	assertToolAdmissionTransitionBlocked(t, transition)
	close(driver.allowPersistReturn)
	if err := waitToolAdmissionResult(t, resultCh); err != nil {
		t.Fatalf("tool step failed after durable ready admission: %v", err)
	}
	release := waitToolAdmissionTransition(t, transition)
	release()
	if len(driver.ready) != 1 || len(driver.executed) != 1 {
		t.Fatalf("ready admission or execution count mismatch: %#v", driver.toolStepDriverStub)
	}
}

func TestApprovalIssueIsAtomicWithGrantReadyAuthority(t *testing.T) {
	testGateIssueIsAtomicWithGrantReadyAuthority(t, "approval")
}

func TestUserInputIssueIsAtomicWithGrantReadyAuthority(t *testing.T) {
	testGateIssueIsAtomicWithGrantReadyAuthority(t, "user_input")
}

func testGateIssueIsAtomicWithGrantReadyAuthority(t *testing.T, kind string) {
	t.Helper()
	gate := effectgateapp.New()
	driver := &toolAdmissionLeaseDriver{
		toolStepDriverStub: &toolStepDriverStub{},
		gate:               gate,
		readyPersisted:     make(chan struct{}, 1),
		allowPersistReturn: make(chan struct{}),
		gateIssued:         make(chan struct{}, 1),
		allowGateReturn:    make(chan struct{}),
	}
	securityContext := newLoopGeneralContextV2(t, "thread-gate-"+kind, "turn-gate-"+kind, t.TempDir())
	toolName := "write"
	approvalPolicy := "on-request"
	if kind == "user_input" {
		toolName = "request_user_input"
		approvalPolicy = "auto"
	}
	resultCh := make(chan struct {
		result ToolStepResult
		err    error
	}, 1)
	go func() {
		result, err := RunToolStep(context.Background(), hostToolStepInputForTest(ToolStepInput{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: "provider-gate-" + kind,
			Workspace: securityContext.WorkspaceRealPath, ApprovalPolicy: approvalPolicy, SandboxMode: "workspace-write",
			EffectiveMaxModelSteps: 2,
			ToolCalls:              []domainmodel.ToolCall{{ID: "call-gate-" + kind, Name: toolName, Arguments: []byte(`{}`)}},
			ToolSchemas:            zeroArgumentToolSchemas(toolName), AdvertisedTools: advertisedToolNames(toolName),
			SecurityContext: securityContext, Driver: driver,
		}))
		resultCh <- struct {
			result ToolStepResult
			err    error
		}{result: result, err: err}
	}()
	waitToolAdmissionSignal(t, driver.readyPersisted, "ready persistence did not reach the gate boundary")
	transition := beginToolAdmissionTransition(gate, securityContext)
	assertToolAdmissionTransitionBlocked(t, transition)
	close(driver.allowPersistReturn)
	waitToolAdmissionSignal(t, driver.gateIssued, "nested gate issue deadlocked behind a queued transition")
	assertToolAdmissionTransitionBlocked(t, transition)
	close(driver.allowGateReturn)
	select {
	case outcome := <-resultCh:
		if outcome.err != nil || !outcome.result.Paused || outcome.result.PendingKind != kind {
			t.Fatalf("gate issue did not pause atomically: result=%#v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(time.Second):
		t.Fatal("gate issue did not complete after its durable records were released")
	}
	release := waitToolAdmissionTransition(t, transition)
	release()
	if len(driver.ready) != 1 {
		t.Fatalf("gate issue lost or duplicated its ready grant: %#v", driver.toolStepDriverStub)
	}
	if kind == "approval" && len(driver.approvals) != 1 || kind == "user_input" && len(driver.userInputs) != 1 {
		t.Fatalf("gate issue count mismatch: %#v", driver.toolStepDriverStub)
	}
}

func beginToolAdmissionTransition(gate *effectgateapp.Gate, securityContext domainsecurity.TurnSecurityContext) <-chan func() {
	result := make(chan func(), 1)
	go func() {
		release, err := gate.AcquireTransition(context.Background(), securityContext)
		if err != nil {
			result <- nil
			return
		}
		result <- release
	}()
	return result
}

func assertToolAdmissionTransitionBlocked(t *testing.T, transition <-chan func()) {
	t.Helper()
	select {
	case release := <-transition:
		if release != nil {
			release()
		}
		t.Fatal("context transition crossed an incomplete tool admission")
	case <-time.After(25 * time.Millisecond):
	}
}

func waitToolAdmissionTransition(t *testing.T, transition <-chan func()) func() {
	t.Helper()
	select {
	case release := <-transition:
		if release == nil {
			t.Fatal("context transition failed after tool admission released")
		}
		return release
	case <-time.After(time.Second):
		t.Fatal("context transition remained blocked after tool admission released")
		return func() {}
	}
}

func waitToolAdmissionSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func waitToolAdmissionResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("tool admission did not complete")
		return nil
	}
}
