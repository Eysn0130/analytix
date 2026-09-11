package control

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubDriver struct {
	sendRequests      []StartTurnRequest
	steerRequests     []SteerTurnRequest
	interruptRequests []InterruptTurnRequest
	approvalRequests  []ApprovalDecision
	inputRequests     []UserInputResponse
}

type blockingSteerDriver struct {
	stubDriver
	entered chan struct{}
	release chan struct{}
}

type controllerWaitIntentContext struct {
	context.Context
	entered chan chan struct{}
}

func newControllerWaitIntentContext() *controllerWaitIntentContext {
	return &controllerWaitIntentContext{Context: context.Background(), entered: make(chan chan struct{})}
}

func (ctx *controllerWaitIntentContext) Done() <-chan struct{} {
	resume := make(chan struct{})
	ctx.entered <- resume
	<-resume
	return ctx.Context.Done()
}

func waitForControllerWaitIntent(t *testing.T, ctx *controllerWaitIntentContext) func() {
	t.Helper()
	select {
	case resume := <-ctx.entered:
		var once sync.Once
		return func() { once.Do(func() { close(resume) }) }
	case <-time.After(time.Second):
		t.Fatal("controller wait intent was not entered")
		return func() {}
	}
}

func (driver *blockingSteerDriver) SteerTurn(_ context.Context, request SteerTurnRequest) (ActionResult, error) {
	driver.steerRequests = append(driver.steerRequests, request)
	close(driver.entered)
	<-driver.release
	return ActionResult{StatusCode: 200, Body: map[string]any{"ok": true}}, nil
}

func (d *stubDriver) SendTurn(_ context.Context, request StartTurnRequest) (map[string]any, error) {
	d.sendRequests = append(d.sendRequests, request)
	return map[string]any{"threadId": request.ThreadID, "accepted": true}, nil
}

func (d *stubDriver) SteerTurn(_ context.Context, request SteerTurnRequest) (ActionResult, error) {
	d.steerRequests = append(d.steerRequests, request)
	return ActionResult{StatusCode: 200, Body: map[string]any{"ok": true}}, nil
}

func (d *stubDriver) InterruptTurn(_ context.Context, request InterruptTurnRequest) (ActionResult, error) {
	d.interruptRequests = append(d.interruptRequests, request)
	return ActionResult{StatusCode: 200, Body: map[string]any{"cancelled": true}}, nil
}

func (d *stubDriver) ApproveTool(_ context.Context, request ApprovalDecision) (ActionResult, error) {
	d.approvalRequests = append(d.approvalRequests, request)
	return ActionResult{StatusCode: 200, Body: map[string]any{"decision": request.Decision}}, nil
}

func (d *stubDriver) RespondUserInput(_ context.Context, request UserInputResponse) (ActionResult, error) {
	d.inputRequests = append(d.inputRequests, request)
	return ActionResult{StatusCode: 200, Body: map[string]any{"inputId": request.InputID}}, nil
}

func TestControllerSendTurnValidatesThreadAndPrompt(t *testing.T) {
	driver := &stubDriver{}
	controller := NewController(driver)
	if _, err := controller.SendTurn(context.Background(), StartTurnRequest{ThreadID: "thr"}); !errors.Is(err, ErrMissingPrompt) {
		t.Fatalf("expected missing prompt, got %v", err)
	}
	if _, err := controller.SendTurn(context.Background(), StartTurnRequest{Prompt: "hello"}); !errors.Is(err, ErrMissingThreadID) {
		t.Fatalf("expected missing thread id, got %v", err)
	}
	response, err := controller.SendTurn(context.Background(), StartTurnRequest{ThreadID: " thr_1 ", Prompt: "hello"})
	if err != nil {
		t.Fatalf("send turn: %v", err)
	}
	if response["threadId"] != "thr_1" {
		t.Fatalf("expected normalized thread id, got %#v", response)
	}
	if len(driver.sendRequests) != 1 || driver.sendRequests[0].Prompt != "hello" {
		t.Fatalf("driver did not receive normalized request: %#v", driver.sendRequests)
	}
}

func TestControllerSendTurnNormalizesStartTurnRequest(t *testing.T) {
	driver := &stubDriver{}
	controller := NewController(driver)
	steps := 3
	if _, err := controller.SendTurn(context.Background(), StartTurnRequest{
		ThreadID:              " thr_1 ",
		Prompt:                "  hello  ",
		RiskIntent:            " case ",
		Model:                 " gpt-4.1 ",
		ProviderID:            " openai ",
		EndpointFormat:        " responses ",
		ReasoningEffort:       "high",
		Mode:                  " plan ",
		ApprovalPolicy:        " on-request ",
		SandboxMode:           " workspace-write ",
		AttachmentIDs:         []string{" att_1 ", ""},
		FileReferences:        []any{map[string]any{"path": " /tmp/a.go ", "relativePath": " a.go ", "name": " a.go ", "kind": " file "}},
		WorkspaceCheckpointID: " chk_1 ",
		MaxModelSteps:         &steps,
	}); err != nil {
		t.Fatalf("send turn: %v", err)
	}
	request := driver.sendRequests[0]
	if request.ThreadID != "thr_1" ||
		request.Prompt != "  hello  " ||
		request.RiskIntent != "case" ||
		request.Model != "gpt-4.1" ||
		request.ProviderID != "openai" ||
		request.EndpointFormat != "responses" ||
		request.ReasoningEffort != "high" ||
		request.Mode != "plan" ||
		request.ApprovalPolicy != "on-request" ||
		request.SandboxMode != "workspace-write" ||
		request.WorkspaceCheckpointID != "chk_1" {
		t.Fatalf("request was not normalized: %+v", request)
	}
	if len(request.AttachmentIDs) != 1 || request.AttachmentIDs[0] != "att_1" {
		t.Fatalf("unexpected attachment ids: %#v", request.AttachmentIDs)
	}
	if len(request.FileReferences) != 1 {
		t.Fatalf("unexpected file references: %#v", request.FileReferences)
	}
	if request.MaxModelSteps == nil || *request.MaxModelSteps != 3 {
		t.Fatalf("unexpected max model steps: %#v", request.MaxModelSteps)
	}
}

func TestControllerSendTurnUsesExactClosedReasoningEffort(t *testing.T) {
	driver := &stubDriver{}
	controller := NewController(driver)
	if _, err := controller.SendTurn(context.Background(), StartTurnRequest{
		ThreadID: "thread-1", Prompt: "hello", ReasoningEffort: "auto",
	}); err != nil || len(driver.sendRequests) != 1 || driver.sendRequests[0].ReasoningEffort != "auto" {
		t.Fatalf("auto effort did not reach the driver exactly: requests=%#v err=%v", driver.sendRequests, err)
	}
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for _, effort := range []string{" high ", "HIGH", sentinel} {
		before := len(driver.sendRequests)
		if _, err := controller.SendTurn(context.Background(), StartTurnRequest{
			ThreadID: "thread-1", Prompt: "hello", ReasoningEffort: effort,
		}); err == nil || strings.Contains(err.Error(), sentinel) || len(driver.sendRequests) != before {
			t.Fatalf("invalid effort reached or was reflected by driver: effort=%q requests=%#v err=%v", effort, driver.sendRequests, err)
		}
	}
}

func TestControllerReservedAutoContinueCarrierRejectsNormalizationErasedExtraField(t *testing.T) {
	driver := &stubDriver{}
	controller := NewController(driver)
	request := StartTurnRequest{
		ThreadID: "thread-1", Prompt: InternalAutoContinuePromptV1, Async: true, InternalUsageSource: "turn",
		InternalTurnID: "turn_2", InternalAutoContinueJobID: "job-1", InternalAutoContinueParentTurnID: "turn_1",
	}
	if _, err := controller.SendTurn(context.Background(), request); err != nil || len(driver.sendRequests) != 1 {
		t.Fatalf("exact reserved carrier was rejected: requests=%d err=%v", len(driver.sendRequests), err)
	}
	request.RiskIntent = "normalization-would-erase-this"
	if _, err := controller.SendTurn(context.Background(), request); err == nil || len(driver.sendRequests) != 1 {
		t.Fatalf("normalization-erased extra field reached driver: requests=%d err=%v", len(driver.sendRequests), err)
	}
}

func TestNormalizeStartTurnRiskIntentCannotDowngradePolicy(t *testing.T) {
	if got := NormalizeStartTurnRequest(StartTurnRequest{RiskIntent: " case "}).RiskIntent; got != "case" {
		t.Fatalf("expected case risk raise to survive normalization, got %q", got)
	}
	for _, value := range []string{"", "general", "unknown", " case-policy "} {
		if got := NormalizeStartTurnRequest(StartTurnRequest{RiskIntent: value}).RiskIntent; got != "" {
			t.Fatalf("expected non-case risk intent %q to be removed, got %q", value, got)
		}
	}
}

func TestControllerRoutesTypedActions(t *testing.T) {
	driver := &stubDriver{}
	controller := NewController(driver)
	if _, err := controller.SteerTurn(context.Background(), SteerTurnRequest{
		ThreadID: " thr ", TurnID: " turn ", Text: " more ", RiskIntent: " case ",
		AttachmentIDs:  []string{" att "},
		FileReferences: []any{map[string]any{"path": " /workspace/a ", "relativePath": " a ", "name": " a "}},
	}); err != nil {
		t.Fatalf("steer: %v", err)
	}
	if _, err := controller.InterruptTurn(context.Background(), InterruptTurnRequest{ThreadID: "thr", TurnID: "turn", Discard: true}); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	if _, err := controller.ApproveTool(context.Background(), ApprovalDecision{ApprovalID: "appr", Decision: "allow"}); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if _, err := controller.RespondUserInput(context.Background(), UserInputResponse{InputID: "input", Answers: []map[string]string{{"value": "yes"}}}); err != nil {
		t.Fatalf("user input: %v", err)
	}
	if len(driver.steerRequests) != 1 || len(driver.interruptRequests) != 1 || len(driver.approvalRequests) != 1 || len(driver.inputRequests) != 1 {
		t.Fatalf("driver request counts mismatch: %#v", driver)
	}
	steer := driver.steerRequests[0]
	if steer.ThreadID != "thr" || steer.TurnID != "turn" || steer.Text != "more" || steer.RiskIntent != "case" ||
		len(steer.AttachmentIDs) != 1 || steer.AttachmentIDs[0] != "att" || len(steer.FileReferences) != 1 {
		t.Fatalf("steer request was not normalized and propagated: %#v", steer)
	}
}

func TestNormalizeSteerTurnRiskIntentCannotDowngradePolicy(t *testing.T) {
	if got := NormalizeSteerTurnRequest(SteerTurnRequest{RiskIntent: " case "}).RiskIntent; got != "case" {
		t.Fatalf("expected case risk raise to survive normalization, got %q", got)
	}
	for _, value := range []string{"", "general", "unknown", " case-policy "} {
		if got := NormalizeSteerTurnRequest(SteerTurnRequest{RiskIntent: value}).RiskIntent; got != "" {
			t.Fatalf("expected non-case risk intent %q to be removed, got %q", value, got)
		}
	}
}

func TestControllerRejectsInvalidApprovalDecision(t *testing.T) {
	controller := NewController(&stubDriver{})
	if _, err := controller.ApproveTool(context.Background(), ApprovalDecision{ApprovalID: "appr", Decision: "maybe"}); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("expected invalid decision, got %v", err)
	}
}

func TestControllerTracksTurnCancels(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := 0
	if !controller.RegisterTurnCancel(" thr ", " turn ", func() { cancelled++ }) {
		t.Fatal("expected cancel registration")
	}
	if controller.ActiveTurnCount() != 1 {
		t.Fatalf("expected one active turn, got %d", controller.ActiveTurnCount())
	}
	if !controller.CancelRegisteredTurn("thr", "turn") {
		t.Fatal("expected cancel to run")
	}
	if cancelled != 1 {
		t.Fatalf("expected one cancel call, got %d", cancelled)
	}
	if !controller.UnregisterTurnCancel("thr", "turn") {
		t.Fatal("expected unregister to report existing turn")
	}
	if controller.ActiveTurnCount() != 0 {
		t.Fatalf("expected no active turns, got %d", controller.ActiveTurnCount())
	}
}

func TestControllerCancelsAllRegisteredTurns(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := 0
	controller.RegisterTurnCancel("thr", "turn_1", func() { cancelled++ })
	controller.RegisterTurnCancel("thr", "turn_2", func() { cancelled++ })
	if count := controller.CancelAllRegisteredTurns(); count != 2 {
		t.Fatalf("expected 2 cancels, got %d", count)
	}
	if cancelled != 2 {
		t.Fatalf("expected 2 cancel calls, got %d", cancelled)
	}
}

func TestControllerCancelsOneThreadAndWaitsForExecutionOwnershipRelease(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := make(chan struct{})
	if !controller.RegisterTurnCancel("thread-a", "turn-a", func() { close(cancelled) }) {
		t.Fatal("expected thread-a registration")
	}
	if !controller.RegisterTurnCancel("thread-b", "turn-b", func() {}) {
		t.Fatal("expected thread-b registration")
	}
	if controller.RegisterTurnCancel("thread-a", "turn-a", func() {}) {
		t.Fatal("duplicate execution registration replaced its cancellation owner")
	}

	waited := make(chan error, 1)
	go func() {
		waited <- controller.CancelThreadTurnsAndWait(context.Background(), "thread-a")
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("thread cancellation was not delivered")
	}
	select {
	case err := <-waited:
		t.Fatalf("wait returned before terminal persistence ownership was released: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if !controller.UnregisterTurnCancel("thread-a", "turn-a") {
		t.Fatal("expected thread-a ownership release")
	}
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("thread wait returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("thread wait did not release")
	}
	if controller.ActiveTurnCount() != 1 || !controller.UnregisterTurnCancel("thread-b", "turn-b") {
		t.Fatal("thread-a barrier changed another thread's execution ownership")
	}
}

func TestControllerInterruptWaitsForExactExecutionOwnershipRelease(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := make(chan struct{})
	if !controller.RegisterTurnCancel("thread", "turn-active", func() { close(cancelled) }) ||
		!controller.RegisterTurnCancel("thread", "turn-other", func() {}) {
		t.Fatal("expected exact-turn registrations")
	}
	waited := make(chan error, 1)
	go func() {
		active, err := controller.CancelRegisteredTurnAndWait(context.Background(), "thread", "turn-active")
		if !active && err == nil {
			err = errors.New("active execution was not observed")
		}
		waited <- err
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("exact-turn cancellation was not delivered")
	}
	if !controller.TurnInterruptReserved("thread", "turn-active") {
		t.Fatal("interrupt terminal ownership was not reserved while execution settled")
	}
	select {
	case err := <-waited:
		t.Fatalf("interrupt wait returned before execution ownership release: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if !controller.UnregisterTurnCancel("thread", "turn-active") {
		t.Fatal("expected exact execution ownership release")
	}
	select {
	case err := <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("interrupt wait did not release")
	}
	if controller.TurnInterruptReserved("thread", "turn-active") {
		t.Fatal("interrupt terminal ownership remained reserved after quiescence")
	}
	if controller.ActiveTurnCount() != 1 || !controller.UnregisterTurnCancel("thread", "turn-other") {
		t.Fatal("exact-turn interrupt waited for or changed another execution")
	}
}

func TestControllerInterruptFirstRejectsCandidateUntilDurableOwnerReleases(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := make(chan struct{})
	if !controller.RegisterTurnCancel("thread", "turn", func() { close(cancelled) }) {
		t.Fatal("expected execution registration")
	}
	type reservationResult struct {
		cancelled bool
		release   func()
		err       error
	}
	reserved := make(chan reservationResult, 1)
	go func() {
		cancelIssued, release, err := controller.ReserveInterruptTerminalAndWait(context.Background(), "thread", "turn")
		reserved <- reservationResult{cancelled: cancelIssued, release: release, err: err}
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("interrupt did not cancel the execution owner")
	}
	if release, err := controller.AcquireCandidateTerminal(context.Background(), "thread", "turn", "digest"); !errors.Is(err, context.Canceled) || release != nil {
		t.Fatalf("interrupt-first arbitration admitted candidate: release=%v err=%v", release != nil, err)
	}
	if !controller.UnregisterTurnCancel("thread", "turn") {
		t.Fatal("expected execution ownership release")
	}
	result := <-reserved
	if result.err != nil || !result.cancelled || result.release == nil {
		t.Fatalf("interrupt reservation result = %#v", result)
	}
	if !controller.TurnInterruptReserved("thread", "turn") {
		t.Fatal("reservation was released before the interrupt durable CAS")
	}
	result.release()
	if controller.TurnInterruptReserved("thread", "turn") {
		t.Fatal("reservation remained after interrupt durable CAS release")
	}
}

func TestControllerCandidateFirstHoldsInterruptUntilCandidateAndOwnerRelease(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := make(chan struct{}, 1)
	if !controller.RegisterTurnCancel("thread", "turn", func() { cancelled <- struct{}{} }) {
		t.Fatal("expected execution registration")
	}
	releaseCandidate, err := controller.AcquireCandidateTerminal(context.Background(), "thread", "turn", "digest")
	if err != nil || releaseCandidate == nil {
		t.Fatalf("candidate permit was not acquired: %v", err)
	}
	type reservationResult struct {
		cancelled bool
		release   func()
		err       error
	}
	reserved := make(chan reservationResult, 1)
	go func() {
		cancelIssued, release, reserveErr := controller.ReserveInterruptTerminalAndWait(context.Background(), "thread", "turn")
		reserved <- reservationResult{cancelled: cancelIssued, release: release, err: reserveErr}
	}()
	deadline := time.Now().Add(time.Second)
	for !controller.TurnInterruptReserved("thread", "turn") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !controller.TurnInterruptReserved("thread", "turn") {
		t.Fatal("interrupt did not establish its reservation")
	}
	select {
	case <-cancelled:
		t.Fatal("late interrupt cancelled the linearized candidate")
	case <-time.After(25 * time.Millisecond):
	}
	select {
	case result := <-reserved:
		t.Fatalf("interrupt returned before candidate CAS: %#v", result)
	default:
	}
	releaseCandidate()
	select {
	case result := <-reserved:
		t.Fatalf("interrupt returned before execution owner release: %#v", result)
	case <-time.After(25 * time.Millisecond):
	}
	controller.UnregisterTurnCancel("thread", "turn")
	result := <-reserved
	if result.err != nil || result.cancelled || result.release == nil {
		t.Fatalf("candidate-first interrupt result = %#v", result)
	}
	result.release()
}

func TestControllerThreadBarrierWaitsForTerminalTailAfterOwnerRelease(t *testing.T) {
	controller := NewController(&stubDriver{})
	if !controller.RegisterTurnCancel("thread", "turn-old", func() {}) {
		t.Fatal("expected execution registration")
	}
	releaseTerminal, err := controller.AcquireHostTerminal(context.Background(), "thread", "turn-old", "digest-old")
	if err != nil || releaseTerminal == nil {
		t.Fatalf("host terminal permit was not acquired: %v", err)
	}
	if !controller.UnregisterTurnCancel("thread", "turn-old") {
		t.Fatal("expected execution owner release")
	}

	waited := make(chan error, 1)
	go func() { waited <- controller.WaitForThreadTurns(context.Background(), "thread") }()
	select {
	case err := <-waited:
		t.Fatalf("thread barrier crossed an active terminal tail: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if controller.RegisterTurnCancel("thread", "turn-new", func() {}) {
		t.Fatal("new turn crossed the previous turn terminal tail")
	}
	if !controller.RegisterTurnCancel("other-thread", "turn-new", func() {}) {
		t.Fatal("terminal tail blocked an unrelated thread")
	}

	releaseTerminal()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("thread barrier did not release after terminal tail")
	}
	if !controller.RegisterTurnCancel("thread", "turn-new", func() {}) {
		t.Fatal("new turn was not admitted after terminal tail release")
	}
	controller.UnregisterTurnCancel("thread", "turn-new")
	controller.UnregisterTurnCancel("other-thread", "turn-new")
}

func TestControllerThreadBarrierWaitsForInterruptTerminalReservation(t *testing.T) {
	controller := NewController(&stubDriver{})
	_, release, err := controller.ReserveInterruptTerminalAndWait(context.Background(), "thread", "turn-old")
	if err != nil || release == nil {
		t.Fatalf("interrupt terminal reservation was not acquired: %v", err)
	}
	waitContext, cancelWait := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancelWait()
	if err := controller.WaitForThreadTurns(waitContext, "thread"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("thread barrier crossed interrupt terminal reservation: %v", err)
	}
	if controller.RegisterTurnCancel("thread", "turn-new", func() {}) {
		t.Fatal("new turn crossed interrupt terminal reservation")
	}
	release()
	if err := controller.WaitForThreadTurns(context.Background(), "thread"); err != nil {
		t.Fatal(err)
	}
}

func TestControllerThreadTransitionReservationBlocksLateTurnUntilRelease(t *testing.T) {
	controller := NewController(&stubDriver{})
	release, err := controller.ReserveThreadTransition(" thread ")
	if err != nil || release == nil {
		t.Fatalf("thread transition reservation failed: %v", err)
	}
	if controller.RegisterTurnCancel("thread", "turn-late", func() {}) {
		t.Fatal("thread transition admitted a late foreground owner")
	}
	if !controller.RegisterTurnCancel("other", "turn-other", func() {}) {
		t.Fatal("thread transition blocked an unrelated thread")
	}
	if duplicate, duplicateErr := controller.ReserveThreadTransition("thread"); !errors.Is(duplicateErr, ErrThreadTransition) || duplicate != nil {
		t.Fatalf("duplicate transition reservation = release:%v err:%v", duplicate != nil, duplicateErr)
	}
	release()
	release()
	if !controller.RegisterTurnCancel("thread", "turn-late", func() {}) {
		t.Fatal("thread transition reservation was not released")
	}
	controller.UnregisterTurnCancel("thread", "turn-late")
	controller.UnregisterTurnCancel("other", "turn-other")
}

func TestControllerRejectsConcurrentTerminalReservationsAcrossTurnsInSameThread(t *testing.T) {
	controller := NewController(&stubDriver{})
	if !controller.RegisterTurnCancel("thread", "turn-a", func() {}) ||
		!controller.RegisterTurnCancel("thread", "turn-b", func() {}) ||
		!controller.RegisterTurnCancel("other", "turn-c", func() {}) {
		t.Fatal("expected execution registrations")
	}
	releaseA, err := controller.AcquireCandidateTerminal(context.Background(), "thread", "turn-a", "digest-a")
	if err != nil || releaseA == nil {
		t.Fatalf("first terminal reservation failed: %v", err)
	}
	if releaseB, err := controller.AcquireHostTerminal(context.Background(), "thread", "turn-b", "digest-b"); !errors.Is(err, ErrTerminalArbitration) || releaseB != nil {
		t.Fatalf("same-thread terminal overlap = release:%v err:%v", releaseB != nil, err)
	}
	if _, releaseInterrupt, err := controller.ReserveInterruptTerminalAndWait(context.Background(), "thread", "turn-b"); !errors.Is(err, ErrTerminalArbitration) || releaseInterrupt != nil {
		t.Fatalf("same-thread interrupt overlap = release:%v err:%v", releaseInterrupt != nil, err)
	}
	releaseOther, err := controller.AcquireCandidateTerminal(context.Background(), "other", "turn-c", "digest-c")
	if err != nil || releaseOther == nil {
		t.Fatalf("unrelated-thread terminal was blocked: %v", err)
	}
	releaseOther()
	releaseA()
	controller.UnregisterTurnCancel("thread", "turn-a")
	controller.UnregisterTurnCancel("thread", "turn-b")
	controller.UnregisterTurnCancel("other", "turn-c")
}

func TestControllerConcurrentThreadBarriersCancelOwnerOnlyOnce(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := make(chan struct{}, 4)
	if !controller.RegisterTurnCancel("thread", "turn", func() { cancelled <- struct{}{} }) {
		t.Fatal("expected execution registration")
	}
	waits := make(chan error, 2)
	go func() { waits <- controller.CancelThreadTurnsAndWait(context.Background(), "thread") }()
	go func() { waits <- controller.CancelThreadTurnsAndWait(context.Background(), "thread") }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("owner was not cancelled")
	}
	select {
	case <-cancelled:
		t.Fatal("concurrent thread barriers delivered duplicate cancellation")
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case err := <-waits:
		t.Fatalf("barrier returned before owner release: %v", err)
	default:
	}
	controller.UnregisterTurnCancel("thread", "turn")
	for range 2 {
		select {
		case err := <-waits:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent barrier did not release")
		}
	}
}

func TestControllerThreadBarrierCanExcludeAdmittedStartOwner(t *testing.T) {
	controller := NewController(&stubDriver{})
	currentCancelled := 0
	oldCancelled := make(chan struct{}, 1)
	if !controller.RegisterTurnCancel("thread", "turn-current", func() { currentCancelled++ }) ||
		!controller.RegisterTurnCancel("thread", "turn-old", func() { oldCancelled <- struct{}{} }) {
		t.Fatal("register turn owners")
	}
	waited := make(chan error, 1)
	go func() {
		waited <- controller.CancelThreadTurnsAndWaitExcept(context.Background(), "thread", "turn-current")
	}()
	select {
	case <-oldCancelled:
	case <-time.After(time.Second):
		t.Fatal("old owner was not cancelled")
	}
	if currentCancelled != 0 {
		t.Fatal("current start owner was cancelled by its own transition barrier")
	}
	controller.UnregisterTurnCancel("thread", "turn-old")
	select {
	case err := <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("excluded barrier did not release after old owner")
	}
	if controller.ActiveTurnCount() != 1 {
		t.Fatalf("excluded current owner was lost: active=%d", controller.ActiveTurnCount())
	}
	controller.UnregisterTurnCancel("thread", "turn-current")
}

func TestControllerNonInterruptCancelCannotCutThroughTerminalPermit(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := 0
	if !controller.RegisterTurnCancel("thread", "turn", func() { cancelled++ }) {
		t.Fatal("expected execution registration")
	}
	releaseCandidate, err := controller.AcquireCandidateTerminal(context.Background(), "thread", "turn", "digest")
	if err != nil {
		t.Fatal(err)
	}
	if controller.CancelRegisteredTurn("thread", "turn") || cancelled != 0 {
		t.Fatal("non-interrupt cancellation cut through a terminal permit")
	}
	releaseCandidate()
	if !controller.CancelRegisteredTurn("thread", "turn") || cancelled != 1 {
		t.Fatal("pre-terminal cancellation was not delivered")
	}
	if release, acquireErr := controller.AcquireCandidateTerminal(context.Background(), "thread", "turn", "digest"); !errors.Is(acquireErr, context.Canceled) || release != nil {
		t.Fatalf("cancelled execution acquired provider candidate permit: release=%v err=%v", release != nil, acquireErr)
	}
	releaseHost, hostErr := controller.AcquireHostTerminal(context.Background(), "thread", "turn", "digest")
	if hostErr != nil || releaseHost == nil {
		t.Fatalf("host fixed terminal could not settle cancellation: %v", hostErr)
	}
	releaseHost()
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerShutdownFirstRejectsCandidateAndAllowsFixedSettlement(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := 0
	if !controller.RegisterTurnCancel("thread", "turn", func() { cancelled++ }) {
		t.Fatal("expected execution registration")
	}
	if count := controller.BeginShutdown(); count != 1 {
		t.Fatalf("shutdown owner count = %d", count)
	}
	if release, err := controller.AcquireCandidateTerminal(context.Background(), "thread", "turn", "digest"); !errors.Is(err, context.Canceled) || release != nil {
		t.Fatalf("shutdown-first arbitration admitted candidate: release=%v err=%v", release != nil, err)
	}
	if count := controller.CancelAllRegisteredTurns(); count != 1 || cancelled != 1 {
		t.Fatalf("shutdown cancellation = %d calls=%d", count, cancelled)
	}
	release, err := controller.AcquireHostTerminal(context.Background(), "thread", "turn", "digest")
	if err != nil || release == nil {
		t.Fatalf("shutdown fixed terminal was rejected: %v", err)
	}
	release()
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerSteerFirstCompletesBeforeLoopTerminalClaim(t *testing.T) {
	driver := &blockingSteerDriver{entered: make(chan struct{}), release: make(chan struct{})}
	controller := NewController(driver)
	if !controller.RegisterTurnCancel("thread", "turn", func() {}) {
		t.Fatal("expected execution registration")
	}
	steered := make(chan ActionResult, 1)
	go func() {
		releaseAdmission, err := controller.ReserveSteerAdmission("thread", "turn")
		if err != nil {
			steered <- ActionResult{StatusCode: 500, Body: map[string]any{"error": err.Error()}}
			return
		}
		defer releaseAdmission()
		result, _ := driver.SteerTurn(context.Background(), SteerTurnRequest{
			ThreadID: "thread", TurnID: "turn", Text: "continue",
		})
		steered <- result
	}()
	select {
	case <-driver.entered:
	case <-time.After(time.Second):
		t.Fatal("steer did not enter the durable driver")
	}
	type terminalClaim struct {
		ctx     context.Context
		release func()
		err     error
	}
	claimed := make(chan terminalClaim, 1)
	go func() {
		claimCtx, release, err := controller.AcquireCandidateTerminalForLoop(
			context.Background(), "thread", "turn", "digest",
		)
		claimed <- terminalClaim{ctx: claimCtx, release: release, err: err}
	}()
	waitForControllerCandidateIntent(t, controller, "thread", "turn")
	if lateRelease, err := controller.ReserveSteerAdmission("thread", "turn"); !errors.Is(err, ErrTurnTerminalizing) || lateRelease != nil {
		t.Fatalf("late steer crossed the waiting terminal intent: release=%v err=%v", lateRelease != nil, err)
	}
	select {
	case claim := <-claimed:
		t.Fatalf("terminal crossed an active steer admission: %#v", claim)
	case <-time.After(25 * time.Millisecond):
	}
	close(driver.release)
	if result := <-steered; result.StatusCode != 200 {
		t.Fatalf("steer result = %#v", result)
	}
	claim := <-claimed
	if claim.err != nil || claim.ctx == nil || claim.release == nil {
		t.Fatalf("terminal claim after steer = %#v", claim)
	}
	if nested, err := controller.AcquireCandidateTerminal(context.WithoutCancel(claim.ctx), "thread", "turn", "digest"); err != nil || nested == nil {
		t.Fatalf("publication did not consume the exact loop claim: release=%v err=%v", nested != nil, err)
	} else {
		nested()
	}
	if lateRelease, err := controller.ReserveSteerAdmission("thread", "turn"); !errors.Is(err, ErrTurnTerminalizing) || lateRelease != nil || len(driver.steerRequests) != 1 {
		t.Fatalf("terminal-first late steer reservation = release:%v err=%v requests=%d", lateRelease != nil, err, len(driver.steerRequests))
	}
	claim.release()
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerLoopTerminalFirstRejectsSteerBeforeDurableDriver(t *testing.T) {
	driver := &stubDriver{}
	controller := NewController(driver)
	if !controller.RegisterTurnCancel("thread", "turn", func() {}) {
		t.Fatal("expected execution registration")
	}
	claimCtx, release, err := controller.AcquireCandidateTerminalForLoop(
		context.Background(), "thread", "turn", "digest",
	)
	if err != nil || claimCtx == nil || release == nil {
		t.Fatalf("candidate terminal claim failed: context=%v release=%v err=%v", claimCtx != nil, release != nil, err)
	}
	if forged, err := controller.AcquireCandidateTerminal(context.Background(), "thread", "turn", "digest"); !errors.Is(err, ErrTerminalArbitration) || forged != nil {
		t.Fatalf("terminal claim was re-entered without its host token: release=%v err=%v", forged != nil, err)
	}
	if lateRelease, err := controller.ReserveSteerAdmission("thread", "turn"); !errors.Is(err, ErrTurnTerminalizing) || lateRelease != nil || len(driver.steerRequests) != 0 {
		t.Fatalf("late steer reached durable reservation: release=%v err=%v requests=%d", lateRelease != nil, err, len(driver.steerRequests))
	}
	release()
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerLoopTerminalWaitsForOtherTurnSteerTail(t *testing.T) {
	controller := NewController(&stubDriver{})
	if !controller.RegisterTurnCancel("thread", "turn-terminal", func() {}) {
		t.Fatal("expected terminal execution registration")
	}
	releaseAdmission, err := controller.ReserveSteerAdmission("thread", "turn-other")
	if err != nil {
		t.Fatal(err)
	}
	type claimResult struct {
		ctx     context.Context
		release func()
		err     error
	}
	claimed := make(chan claimResult, 1)
	go func() {
		claimContext, release, claimErr := controller.AcquireCandidateTerminalForLoop(
			context.Background(), "thread", "turn-terminal", "digest",
		)
		claimed <- claimResult{ctx: claimContext, release: release, err: claimErr}
	}()
	waitForControllerCandidateIntent(t, controller, "thread", "turn-terminal")
	select {
	case got := <-claimed:
		t.Fatalf("thread terminal crossed another turn's steer tail: %#v", got)
	case <-time.After(25 * time.Millisecond):
	}
	releaseAdmission()
	select {
	case got := <-claimed:
		if got.err != nil || got.ctx == nil || got.release == nil {
			t.Fatalf("thread terminal claim after steer tail = %#v", got)
		}
		got.release()
	case <-time.After(time.Second):
		t.Fatal("thread terminal remained blocked after other steer tail")
	}
	controller.UnregisterTurnCancel("thread", "turn-terminal")
}

func TestControllerNewTurnWaitsForPriorThreadTail(t *testing.T) {
	controller := NewController(&stubDriver{})
	releaseTerminal, err := controller.AcquireHostTerminal(
		context.Background(), "thread", "turn-old", "digest-old",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseTerminal()
	releaseTransition, err := controller.ReserveThreadTransition("thread")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseTransition()
	waitContext := newControllerWaitIntentContext()
	registered := make(chan error, 1)
	go func() {
		registered <- controller.RegisterTurnCancelAfterThreadTail(
			waitContext, "thread", "turn-new", func() {},
		)
	}()
	resumeTerminalWait := waitForControllerWaitIntent(t, waitContext)
	select {
	case err := <-registered:
		resumeTerminalWait()
		t.Fatalf("new turn crossed prior terminal and transition tail: %v", err)
	default:
	}
	resumeTerminalWait()
	releaseTerminal()
	resumeTransitionWait := waitForControllerWaitIntent(t, waitContext)
	select {
	case err := <-registered:
		resumeTransitionWait()
		t.Fatalf("new turn crossed prior transition tail: %v", err)
	default:
	}
	resumeTransitionWait()
	releaseTransition()
	select {
	case err := <-registered:
		if err != nil {
			t.Fatalf("new turn was not admitted after prior tail: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("new turn remained blocked after prior tail release")
	}
	controller.UnregisterTurnCancel("thread", "turn-new")
}

func TestControllerLoopTerminalTokenFailsClosedAfterReleaseAndAcrossABA(t *testing.T) {
	controller := NewController(&stubDriver{})
	if !controller.RegisterTurnCancel("thread", "turn", func() {}) {
		t.Fatal("expected execution registration")
	}
	staleContext, releaseStale, err := controller.AcquireCandidateTerminalForLoop(
		context.Background(), "thread", "turn", "digest",
	)
	if err != nil {
		t.Fatal(err)
	}
	releaseStale()
	if release, err := controller.AcquireCandidateTerminal(staleContext, "thread", "turn", "digest"); !errors.Is(err, ErrTerminalArbitration) || release != nil {
		t.Fatalf("released terminal token reacquired its turn: release=%v err=%v", release != nil, err)
	}
	currentContext, releaseCurrent, err := controller.AcquireCandidateTerminalForLoop(
		context.Background(), "thread", "turn", "digest",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCurrent()
	if release, err := controller.AcquireCandidateTerminal(staleContext, "thread", "turn", "digest"); !errors.Is(err, ErrTerminalArbitration) || release != nil {
		t.Fatalf("stale token crossed a new claim ABA boundary: release=%v err=%v", release != nil, err)
	}
	if release, err := controller.AcquireCandidateTerminal(currentContext, "thread", "turn", "wrong-digest"); !errors.Is(err, ErrTerminalArbitration) || release != nil {
		t.Fatalf("terminal token crossed a digest boundary: release=%v err=%v", release != nil, err)
	}
	if release, err := controller.AcquireCandidateTerminal(context.WithoutCancel(currentContext), "thread", "turn", "digest"); err != nil || release == nil {
		t.Fatalf("current terminal token did not survive cancellation shielding: release=%v err=%v", release != nil, err)
	} else {
		release()
	}
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerLoopTerminalTokenHasOnePublicationConsumer(t *testing.T) {
	controller := NewController(&stubDriver{})
	if !controller.RegisterTurnCancel("thread", "turn", func() {}) {
		t.Fatal("expected execution registration")
	}
	claimContext, releaseClaim, err := controller.AcquireCandidateTerminalForLoop(
		context.Background(), "thread", "turn", "digest",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseClaim()

	type acquisition struct {
		release func()
		err     error
	}
	start := make(chan struct{})
	results := make(chan acquisition, 2)
	for _, hostFixed := range []bool{false, true} {
		hostFixed := hostFixed
		go func() {
			<-start
			if hostFixed {
				release, err := controller.AcquireHostTerminal(
					context.WithoutCancel(claimContext), "thread", "turn", "digest",
				)
				results <- acquisition{release: release, err: err}
				return
			}
			release, err := controller.AcquireCandidateTerminal(
				context.WithoutCancel(claimContext), "thread", "turn", "digest",
			)
			results <- acquisition{release: release, err: err}
		}()
	}
	close(start)
	succeeded := 0
	rejected := 0
	for range 2 {
		result := <-results
		switch {
		case result.err == nil && result.release != nil:
			succeeded++
			result.release()
		case errors.Is(result.err, ErrTerminalArbitration) && result.release == nil:
			rejected++
		default:
			t.Fatalf("unexpected copied-token acquisition: release=%v err=%v", result.release != nil, result.err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("copied terminal token consumers succeeded=%d rejected=%d", succeeded, rejected)
	}
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerTransitionFirstRejectsLoopCandidate(t *testing.T) {
	controller := NewController(&stubDriver{})
	if !controller.RegisterTurnCancel("thread", "turn", func() {}) {
		t.Fatal("expected execution registration")
	}
	releaseTransition, err := controller.ReserveThreadTransition("thread")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseTransition()
	if claimContext, release, err := controller.AcquireCandidateTerminalForLoop(
		context.Background(), "thread", "turn", "digest",
	); !errors.Is(err, ErrThreadTransition) || claimContext != nil || release != nil {
		t.Fatalf("transition-first candidate = context:%v release:%v err:%v", claimContext != nil, release != nil, err)
	}
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerInterruptWaitsForAdmittedSteerTailBeforeCancellation(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := make(chan struct{}, 1)
	if !controller.RegisterTurnCancel("thread", "turn", func() { cancelled <- struct{}{} }) {
		t.Fatal("expected execution registration")
	}
	releaseAdmission, err := controller.ReserveSteerAdmission("thread", "turn-other")
	if err != nil {
		t.Fatal(err)
	}
	type interruptResult struct {
		cancelled bool
		release   func()
		err       error
	}
	result := make(chan interruptResult, 1)
	go func() {
		cancelIssued, release, err := controller.ReserveInterruptTerminalAndWait(context.Background(), "thread", "turn")
		result <- interruptResult{cancelled: cancelIssued, release: release, err: err}
	}()
	waitForControllerInterruptIntent(t, controller, "thread", "turn")
	if lateRelease, err := controller.ReserveSteerAdmission("thread", "turn"); !errors.Is(err, ErrTurnTerminalizing) || lateRelease != nil {
		t.Fatalf("late steer crossed interrupt intent: release=%v err=%v", lateRelease != nil, err)
	}
	select {
	case <-cancelled:
		t.Fatal("interrupt cancelled the owner before the admitted steer tail finished")
	default:
	}
	releaseAdmission()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("interrupt did not cancel after the steer tail finished")
	}
	controller.UnregisterTurnCancel("thread", "turn")
	got := <-result
	if got.err != nil || !got.cancelled || got.release == nil {
		t.Fatalf("interrupt result = %#v", got)
	}
	got.release()
}

func TestControllerHostFixedTerminalWaitsForAdmittedSteerTail(t *testing.T) {
	controller := NewController(&stubDriver{})
	releaseAdmission, err := controller.ReserveSteerAdmission("thread", "turn")
	if err != nil {
		t.Fatal(err)
	}
	type claimResult struct {
		release func()
		err     error
	}
	claimed := make(chan claimResult, 1)
	go func() {
		release, err := controller.AcquireHostTerminal(context.Background(), "thread", "turn", "digest")
		claimed <- claimResult{release: release, err: err}
	}()
	waitForControllerCandidateIntent(t, controller, "thread", "turn")
	select {
	case got := <-claimed:
		t.Fatalf("host fixed terminal crossed admitted steer tail: %#v", got)
	default:
	}
	releaseAdmission()
	got := <-claimed
	if got.err != nil || got.release == nil {
		t.Fatalf("host fixed terminal claim = %#v", got)
	}
	got.release()
}

func TestControllerThreadBarrierWaitsForAdmittedSteerTail(t *testing.T) {
	controller := NewController(&stubDriver{})
	releaseAdmission, err := controller.ReserveSteerAdmission("thread", "turn")
	if err != nil {
		t.Fatal(err)
	}
	releaseTransition, err := controller.ReserveThreadTransition("thread")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseTransition()
	waitContext := newControllerWaitIntentContext()
	waited := make(chan error, 1)
	go func() { waited <- controller.WaitForThreadTurns(waitContext, "thread") }()
	resumeWait := waitForControllerWaitIntent(t, waitContext)
	select {
	case err := <-waited:
		resumeWait()
		t.Fatalf("thread barrier crossed admitted steer tail: %v", err)
	default:
	}
	resumeWait()
	releaseAdmission()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("thread barrier did not release after the steer tail")
	}
}

func TestControllerShutdownWaitsForAdmittedSteerAndRejectsLateSteer(t *testing.T) {
	driver := &blockingSteerDriver{entered: make(chan struct{}), release: make(chan struct{})}
	controller := NewController(driver)
	done := make(chan ActionResult, 1)
	go func() {
		result, _ := controller.SteerTurn(context.Background(), SteerTurnRequest{ThreadID: "thread", TurnID: "turn", Text: "continue"})
		done <- result
	}()
	select {
	case <-driver.entered:
	case <-time.After(time.Second):
		t.Fatal("steer did not enter the driver")
	}
	controller.BeginShutdown()
	waitContext, cancelWait := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancelWait()
	if err := controller.WaitForTurnOperations(waitContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown did not wait for admitted steer: %v", err)
	}
	late, err := controller.SteerTurn(context.Background(), SteerTurnRequest{ThreadID: "thread", TurnID: "turn", Text: "late"})
	if err != nil || late.StatusCode != 503 || len(driver.steerRequests) != 1 {
		t.Fatalf("late steer admission = %#v err=%v requests=%d", late, err, len(driver.steerRequests))
	}
	close(driver.release)
	if result := <-done; result.StatusCode != 200 {
		t.Fatalf("admitted steer result = %#v", result)
	}
	if err := controller.WaitForTurnOperations(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func waitForControllerCandidateIntent(t *testing.T, controller *Controller, threadID, turnID string) {
	t.Helper()
	key := turnKey{threadID: threadID, turnID: turnID}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		controller.mu.Lock()
		_, active := controller.candidateTerminals[key]
		controller.mu.Unlock()
		if active {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("candidate terminal intent was not installed")
}

func waitForControllerInterruptIntent(t *testing.T, controller *Controller, threadID, turnID string) {
	t.Helper()
	key := turnKey{threadID: threadID, turnID: turnID}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		controller.mu.Lock()
		_, active := controller.interruptReservations[key]
		controller.mu.Unlock()
		if active {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("interrupt terminal intent was not installed")
}

func TestControllerInterruptWaitIsCancellableAndNoOwnerIsImmediate(t *testing.T) {
	controller := NewController(&stubDriver{})
	if active, err := controller.CancelRegisteredTurnAndWait(context.Background(), "thread", "missing"); active || err != nil {
		t.Fatalf("missing owner was not an immediate no-op: active=%v err=%v", active, err)
	}
	if !controller.RegisterTurnCancel("thread", "turn", func() {}) {
		t.Fatal("expected active execution registration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if active, err := controller.CancelRegisteredTurnAndWait(ctx, "thread", "turn"); !active || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait did not fail closed: active=%v err=%v", active, err)
	}
	if controller.TurnInterruptReserved("thread", "turn") {
		t.Fatal("cancelled interrupt wait leaked terminal ownership")
	}
	controller.UnregisterTurnCancel("thread", "turn")
}

func TestControllerShutdownWaitsForOwnedTurnAndRejectsLateAdmission(t *testing.T) {
	controller := NewController(&stubDriver{})
	cancelled := make(chan struct{})
	if !controller.RegisterTurnCancel("thread", "turn", func() { close(cancelled) }) {
		t.Fatal("expected active cancel registration")
	}
	finish, admitted := controller.BeginTurnOperation()
	if !admitted {
		t.Fatal("expected turn operation admission before shutdown")
	}
	if count := controller.BeginShutdown(); count != 1 {
		t.Fatalf("shutdown active count = %d, want 1", count)
	}
	if count := controller.CancelAllRegisteredTurns(); count != 1 {
		t.Fatalf("shutdown cancel count = %d, want 1", count)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the admitted turn")
	}
	if controller.RegisterTurnCancel("thread", "late", func() {}) {
		t.Fatal("shutdown admitted a late turn cancel registration")
	}
	if lateDone, ok := controller.BeginTurnOperation(); ok {
		lateDone()
		t.Fatal("shutdown admitted a late turn operation")
	}
	waitContext := newControllerWaitIntentContext()
	waited := make(chan error, 1)
	go func() { waited <- controller.WaitForTurnOperations(waitContext) }()
	resumeWait := waitForControllerWaitIntent(t, waitContext)
	select {
	case err := <-waited:
		resumeWait()
		t.Fatalf("shutdown barrier returned before terminal ownership completed: %v", err)
	default:
	}
	resumeWait()
	finish()
	finish()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("shutdown barrier returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown barrier did not release after terminal ownership completed")
	}
	if !controller.ShuttingDown() {
		t.Fatal("controller lost its shutdown admission state")
	}
}
