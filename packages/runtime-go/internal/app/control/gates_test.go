package control

import (
	"sync"
	"testing"
)

func TestGateRegistryContinuationClaimIsExclusiveAndGenerationBound(t *testing.T) {
	registry := NewGateRegistry[string]()
	record := GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item", ToolName: "write_file"}
	if !registry.RegisterPendingApproval("approval", record, "pending") {
		t.Fatal("register pending approval")
	}
	claims := make(chan GateClaim[string], 100)
	var workers sync.WaitGroup
	for index := 0; index < 100; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if claim, ok := registry.ClaimApproval("approval"); ok {
				claims <- claim
			}
		}()
	}
	workers.Wait()
	close(claims)
	var claim GateClaim[string]
	count := 0
	for candidate := range claims {
		claim = candidate
		count++
	}
	if count != 1 || claim.ClaimToken == 0 || claim.Pending != "pending" {
		t.Fatalf("exclusive claim count/authority mismatch: count=%d claim=%#v", count, claim)
	}
	if _, _, exists, pending := registry.PeekApproval("approval"); !exists || pending {
		t.Fatalf("claimed approval remained executable: exists=%v pending=%v", exists, pending)
	}
	marked, ok := registry.MarkClaimDisposition(claim, "allowed", "approval_allowed")
	if !ok || marked.ResolutionStatus != "allowed" {
		t.Fatalf("mark signed disposition: %#v ok=%v", marked, ok)
	}
	if registry.CommitContinuationClaim(claim) {
		t.Fatal("pre-disposition claim token committed a later claim generation")
	}
	drained := registry.DrainForTurn("thread", "turn")
	if len(drained) != 1 || drained[0].ClaimToken != marked.ClaimToken ||
		drained[0].ResolutionStatus != "allowed" || drained[0].ResolutionReasonCode != "approval_allowed" {
		t.Fatalf("resolution claim did not promote exactly: %#v", drained)
	}
	if !registry.CommitDrained(drained) {
		t.Fatal("commit promoted resolution claim")
	}
}

func TestGateRequestReservationCannotBeClaimedBeforeActivation(t *testing.T) {
	registry := NewGateRegistry[string]()
	record := GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item_appr", ToolName: "write_file", ContinuationReceiptID: "receipt"}
	if !registry.ReservePendingApproval("appr", record, "pending") {
		t.Fatal("reserve approval")
	}
	if _, ok := registry.ClaimApproval("appr"); ok {
		t.Fatal("staged approval became executable")
	}
	if !registry.ReservePendingApproval("appr", record, "pending") {
		t.Fatal("exact reservation retry was not idempotent")
	}
	conflict := record
	conflict.ItemID = "item_other"
	if registry.ReservePendingApproval("appr", conflict, "pending") {
		t.Fatal("conflicting reservation reused a gate identity")
	}
	if !registry.ActivateReservedApproval("appr", "pending") {
		t.Fatal("activate approval")
	}
	if !registry.ActivateReservedApproval("appr", "pending") {
		t.Fatal("exact activation retry was not idempotent")
	}
	if _, ok := registry.ClaimApproval("appr"); !ok {
		t.Fatal("activated approval was not claimable")
	}
}

func TestGateRequestReservationIsIncludedInTerminalDrain(t *testing.T) {
	registry := NewGateRegistry[string]()
	record := GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item_input", Prompt: "Continue?", ContinuationReceiptID: "receipt"}
	if !registry.ReservePendingUserInput("input", record, "pending") {
		t.Fatal("reserve user input")
	}
	drained := registry.DrainForTurn("thread", "turn")
	if len(drained) != 1 || drained[0].Kind != "user_input" || drained[0].ID != "input" || drained[0].Pending != "pending" {
		t.Fatalf("reserved drain = %#v", drained)
	}
	if _, ok := registry.ClaimUserInput("input"); ok {
		t.Fatal("terminal reservation became executable")
	}
}

func TestGateRegistryRejectsCrossKindDispositionStatus(t *testing.T) {
	registry := NewGateRegistry[string]()
	record := GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item"}
	if !registry.RegisterPendingUserInput("input", record, "pending") {
		t.Fatal("register input")
	}
	claim, ok := registry.ClaimUserInput("input")
	if !ok {
		t.Fatal("claim input")
	}
	if _, ok := registry.MarkClaimDisposition(claim, "allowed", "approval_allowed"); ok {
		t.Fatal("approval disposition was accepted for a user-input claim")
	}
	if _, ok := registry.PromoteClaimToTerminalWithIntent(claim, "submitted", "user_input_submitted"); ok {
		t.Fatal("resolution status was accepted as terminal intent")
	}
	if _, ok := registry.PromoteClaimToTerminalWithIntent(claim, "interrupted", ""); ok {
		t.Fatal("partial terminal intent was accepted")
	}
}

func TestGateRegistryResolveDeletesPendingButKeepsRecord(t *testing.T) {
	registry := NewGateRegistry[string]()
	record := GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item", ToolName: "bash"}
	registry.RegisterApproval("appr_1", record)
	registry.SetPendingApproval("appr_1", "pending-call")

	resolvedRecord, pending, exists, hasPending := registry.ResolveApproval("appr_1")
	if !exists || !hasPending {
		t.Fatalf("expected record and pending, exists=%v hasPending=%v", exists, hasPending)
	}
	if resolvedRecord != record || pending != "pending-call" {
		t.Fatalf("resolved mismatch record=%#v pending=%q", resolvedRecord, pending)
	}

	resolvedRecord, _, exists, hasPending = registry.ResolveApproval("appr_1")
	if !exists || hasPending {
		t.Fatalf("record should remain while pending is removed, exists=%v hasPending=%v", exists, hasPending)
	}
	if resolvedRecord != record {
		t.Fatalf("record mismatch after pending removal: %#v", resolvedRecord)
	}
}

func TestGateRegistryPeekDoesNotConsumePendingContinuation(t *testing.T) {
	registry := NewGateRegistry[string]()
	if !registry.RegisterPendingApproval("appr-peek", GateRecord{ThreadID: "thread", TurnID: "turn"}, "approval-pending") ||
		!registry.RegisterPendingUserInput("input-peek", GateRecord{ThreadID: "thread", TurnID: "turn"}, "input-pending") {
		t.Fatal("register pending gates")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, pending, exists, hasPending := registry.PeekApproval("appr-peek"); !exists || !hasPending || pending != "approval-pending" {
			t.Fatalf("approval peek %d consumed or changed pending: %q exists=%v pending=%v", attempt, pending, exists, hasPending)
		}
		if _, pending, exists, hasPending := registry.PeekUserInput("input-peek"); !exists || !hasPending || pending != "input-pending" {
			t.Fatalf("input peek %d consumed or changed pending: %q exists=%v pending=%v", attempt, pending, exists, hasPending)
		}
	}
	if _, pending, _, hasPending := registry.ResolveApproval("appr-peek"); !hasPending || pending != "approval-pending" {
		t.Fatalf("approval claim lost pending after peek: %q pending=%v", pending, hasPending)
	}
	if _, pending, _, hasPending := registry.ResolveUserInput("input-peek"); !hasPending || pending != "input-pending" {
		t.Fatalf("input claim lost pending after peek: %q pending=%v", pending, hasPending)
	}
}

func TestGateRegistryRejectsDuplicateAndCrossKindOverwrite(t *testing.T) {
	registry := NewGateRegistry[string]()
	recordA := GateRecord{ThreadID: "thread-a", TurnID: "turn-a", ItemID: "item-a", ToolName: "write"}
	recordB := GateRecord{ThreadID: "thread-b", TurnID: "turn-b", ItemID: "item-b", Prompt: "secret?"}
	if !registry.RegisterPendingApproval("gate-shared", recordA, "pending-a") {
		t.Fatal("first atomic registration should succeed")
	}
	if registry.RegisterPendingApproval("gate-shared", recordB, "pending-b") || registry.RegisterPendingUserInput("gate-shared", recordB, "pending-b") {
		t.Fatal("duplicate id must not overwrite a pending gate across threads or kinds")
	}
	resolved, pending, exists, hasPending := registry.ResolveApproval("gate-shared")
	if !exists || !hasPending || resolved != recordA || pending != "pending-a" {
		t.Fatalf("original gate was overwritten: record=%#v pending=%q exists=%v has=%v", resolved, pending, exists, hasPending)
	}
}

func TestSecureGateIDBindsThreadTurnContextGrantAndCall(t *testing.T) {
	base := SecureGateID("appr", "thread-a", "turn-a", "context-a", "grant-a", "call-1")
	variants := []string{
		SecureGateID("appr", "thread-b", "turn-a", "context-a", "grant-a", "call-1"),
		SecureGateID("appr", "thread-a", "turn-b", "context-a", "grant-a", "call-1"),
		SecureGateID("appr", "thread-a", "turn-a", "context-b", "grant-a", "call-1"),
		SecureGateID("appr", "thread-a", "turn-a", "context-a", "grant-b", "call-1"),
		SecureGateID("appr", "thread-a", "turn-a", "context-a", "grant-a", "call-2"),
	}
	for _, variant := range variants {
		if variant == base {
			t.Fatalf("security-bound gate ids collided: %q", base)
		}
	}
}

func TestGateRegistryDrainForTurn(t *testing.T) {
	registry := NewGateRegistry[int]()
	registry.RegisterApproval("appr_1", GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item_a", ToolName: "edit"})
	registry.SetPendingApproval("appr_1", 1)
	registry.RegisterUserInput("input_1", GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item_i", Prompt: "answer?"})
	registry.SetPendingUserInput("input_1", 2)
	registry.RegisterApproval("appr_other", GateRecord{ThreadID: "thread", TurnID: "other", ItemID: "item_o", ToolName: "bash"})
	registry.SetPendingApproval("appr_other", 3)

	drained := registry.DrainForTurn("thread", "turn")
	if len(drained) != 2 {
		t.Fatalf("drained count = %d, want 2: %#v", len(drained), drained)
	}
	if _, _, exists, hasPending := registry.ResolveApproval("appr_1"); !exists || hasPending {
		t.Fatalf("drained approval should keep record and delete pending, exists=%v hasPending=%v", exists, hasPending)
	}
	if _, _, exists, hasPending := registry.ResolveUserInput("input_1"); !exists || hasPending {
		t.Fatalf("drained user input should keep record and delete pending, exists=%v hasPending=%v", exists, hasPending)
	}
	if _, _, exists, hasPending := registry.ResolveApproval("appr_other"); !exists || !hasPending {
		t.Fatalf("other turn pending should remain, exists=%v hasPending=%v", exists, hasPending)
	}
}

func TestGateRegistryDrainAllIsDeterministicAndOneShot(t *testing.T) {
	registry := NewGateRegistry[int]()
	registry.RegisterPendingApproval("appr_b", GateRecord{ThreadID: "thread-b", TurnID: "turn", ItemID: "item-b"}, 2)
	registry.RegisterPendingUserInput("input_a", GateRecord{ThreadID: "thread-a", TurnID: "turn", ItemID: "item-a"}, 1)
	registry.RegisterPendingApproval("appr_a", GateRecord{ThreadID: "thread-a", TurnID: "turn", ItemID: "item-aa"}, 3)
	drained := registry.DrainAll()
	if len(drained) != 3 {
		t.Fatalf("drained count = %d, want 3: %#v", len(drained), drained)
	}
	if drained[0].ID != "appr_a" || drained[1].ID != "input_a" || drained[2].ID != "appr_b" {
		t.Fatalf("drain order is not deterministic: %#v", drained)
	}
	if second := registry.DrainAll(); len(second) != 3 {
		t.Fatalf("terminal claims were not retryable: %#v", second)
	}
	if !registry.CommitDrained(drained) {
		t.Fatal("commit terminal claims")
	}
	if second := registry.DrainAll(); len(second) != 0 {
		t.Fatalf("committed terminal claims remained: %#v", second)
	}
	if _, _, exists, pending := registry.PeekApproval("appr_a"); !exists || pending {
		t.Fatalf("drain should preserve the replay record only, exists=%v pending=%v", exists, pending)
	}
}

func TestGateRegistryDrainForThreadIsDeterministicAndIsolated(t *testing.T) {
	registry := NewGateRegistry[int]()
	registry.RegisterPendingUserInput("input-b", GateRecord{ThreadID: "thread", TurnID: "turn-b", ItemID: "item-b"}, 2)
	registry.RegisterPendingApproval("approval-b", GateRecord{ThreadID: "thread", TurnID: "turn-b", ItemID: "item-ab"}, 3)
	registry.RegisterPendingApproval("approval-a", GateRecord{ThreadID: "thread", TurnID: "turn-a", ItemID: "item-aa"}, 1)
	registry.RegisterPendingApproval("approval-other", GateRecord{ThreadID: "other", TurnID: "turn-a", ItemID: "item-o"}, 4)
	drained := registry.DrainForThread(" thread ")
	if len(drained) != 3 || drained[0].ID != "approval-a" || drained[1].ID != "approval-b" || drained[2].ID != "input-b" {
		t.Fatalf("thread drain order = %#v", drained)
	}
	if _, pending, _, ok := registry.PeekApproval("approval-other"); !ok || pending != 4 {
		t.Fatalf("other thread gate changed: pending=%d ok=%v", pending, ok)
	}
	if second := registry.DrainForThread("thread"); len(second) != 3 {
		t.Fatalf("thread terminal claims were not retryable: %#v", second)
	}
	if !registry.CommitDrained(drained) {
		t.Fatal("commit thread terminal claims")
	}
	if second := registry.DrainForThread("thread"); len(second) != 0 {
		t.Fatalf("committed thread claims remained: %#v", second)
	}
}

func TestGateRegistryTerminalClaimStaysNonExecutableUntilCommit(t *testing.T) {
	registry := NewGateRegistry[string]()
	record := GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item", ToolName: "write_file"}
	if !registry.RegisterPendingApproval("approval", record, "pending") {
		t.Fatal("register pending approval")
	}
	drained := registry.DrainForTurn("thread", "turn")
	if len(drained) != 1 || !registry.RestoreDrained(drained) {
		t.Fatalf("restore failed: %#v", drained)
	}
	if _, _, exists, pending := registry.PeekApproval("approval"); !exists || pending {
		t.Fatalf("terminal claim became executable: exists=%v pending=%v", exists, pending)
	}
	second := registry.DrainForTurn("thread", "turn")
	if len(second) != 1 || second[0].ID != "approval" {
		t.Fatalf("terminal claim was not retryable: %#v", second)
	}
	if !registry.CommitDrained(second) {
		t.Fatal("terminal claim commit failed")
	}
	if retry := registry.DrainForTurn("thread", "turn"); len(retry) != 0 {
		t.Fatalf("committed terminal claim remained retryable: %#v", retry)
	}
}

func TestGateRegistryTerminalIntentIsImmutableAcrossRetry(t *testing.T) {
	registry := NewGateRegistry[string]()
	record := GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item", ToolName: "write_file"}
	if !registry.RegisterPendingApproval("approval", record, "pending") {
		t.Fatal("register pending approval")
	}
	drained := registry.DrainForTurn("thread", "turn")
	bound, ok := registry.BindTerminalIntent(drained, "interrupted", "context_epoch_changed")
	if !ok || len(bound) != 1 || bound[0].TerminalStatus != "interrupted" || bound[0].TerminalReasonCode != "context_epoch_changed" {
		t.Fatalf("first terminal intent bind = %#v ok=%v", bound, ok)
	}
	retry := registry.DrainForTurn("thread", "turn")
	rebound, ok := registry.BindTerminalIntent(retry, "interrupted", "runtime_shutdown")
	if !ok || len(rebound) != 1 || rebound[0].TerminalReasonCode != "context_epoch_changed" {
		t.Fatalf("retry overwrote the first terminal intent: %#v ok=%v", rebound, ok)
	}
	conflicting := rebound[0]
	conflicting.TerminalReasonCode = "runtime_shutdown"
	if _, ok := registry.BindTerminalIntent([]DrainedGate[string]{conflicting}, "interrupted", "runtime_shutdown"); ok {
		t.Fatal("explicit conflicting terminal intent was accepted")
	}
	if !registry.CommitDrained(drained) {
		t.Fatal("pre-bind owner token could not commit the exact frozen claim")
	}
}

func TestPendingGateCancellationsFromDrainedUsesRecordsAndFallbacks(t *testing.T) {
	type pendingTool struct {
		ToolName string
	}
	drained := []DrainedGate[pendingTool]{
		{
			Kind: "approval",
			ID:   "appr_recorded",
			Record: GateRecord{
				ThreadID: "thread-a",
				TurnID:   "turn-a",
				ItemID:   "item_recorded",
				ToolName: "write_file",
			},
		},
		{
			Kind:    "approval",
			ID:      "appr_fallback",
			Pending: pendingTool{ToolName: "bash"},
		},
		{
			Kind: "user_input",
			ID:   "input_fallback",
		},
	}
	cancellations := PendingGateCancellationsFromDrained(DrainedGateCancellationInput[pendingTool]{
		ThreadID: "thread-default",
		TurnID:   "turn-default",
		Drained:  drained,
		PendingToolName: func(pending pendingTool) string {
			return pending.ToolName
		},
	})
	if len(cancellations) != 3 {
		t.Fatalf("cancellation count mismatch: %#v", cancellations)
	}
	if cancellations[0].Record.ItemID != "item_recorded" || cancellations[0].Record.ToolName != "write_file" {
		t.Fatalf("recorded approval should be preserved: %#v", cancellations[0])
	}
	if cancellations[1].Record.ThreadID != "thread-default" || cancellations[1].Record.ItemID != "item_appr_fallback" || cancellations[1].Record.ToolName != "bash" {
		t.Fatalf("approval fallback mismatch: %#v", cancellations[1])
	}
	if cancellations[2].Record.ThreadID != "thread-default" || cancellations[2].Record.ItemID != "item_input_fallback" || cancellations[2].Record.Prompt != "User input required" {
		t.Fatalf("user input fallback mismatch: %#v", cancellations[2])
	}
}

func TestPendingGateCancellationEvents(t *testing.T) {
	cancellations := []PendingGateCancellation{
		ApprovalGateCancellation("appr_1", GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item_a", ToolName: "edit"}),
		UserInputGateCancellation("input_1", GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item_i", Prompt: "question?"}),
	}
	events := PendingGateCancellationEvents(cancellations, "turn_aborted")
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2: %#v", len(events), events)
	}
	if events[0]["kind"] != "approval_resolved" || events[0]["cancelledBy"] != "turn_aborted" {
		t.Fatalf("approval cancellation event mismatch: %#v", events[0])
	}
	if events[1]["kind"] != "user_input_resolved" || events[1]["status"] != "cancelled" {
		t.Fatalf("user input cancellation event mismatch: %#v", events[1])
	}
}

func TestDeletePendingGateStatesForTurn(t *testing.T) {
	approvals := map[string]PendingGateState[int]{
		"appr_1": {Record: GateRecord{ThreadID: "thread", TurnID: "turn"}, Pending: 1, Restored: true},
		"appr_2": {Record: GateRecord{ThreadID: "thread", TurnID: "other"}, Pending: 2, Restored: true},
	}
	inputs := map[string]PendingGateState[int]{
		"input_1": {Record: GateRecord{ThreadID: "thread", TurnID: "turn"}, Pending: 3, Restored: true},
	}
	DeletePendingGateStatesForTurn(approvals, inputs, "thread", "turn")
	if _, ok := approvals["appr_1"]; ok {
		t.Fatalf("approval for terminal turn was not removed")
	}
	if _, ok := inputs["input_1"]; ok {
		t.Fatalf("input for terminal turn was not removed")
	}
	if _, ok := approvals["appr_2"]; !ok {
		t.Fatalf("approval for other turn should remain")
	}
}

func TestApprovalUserInputManagerKeepsSubmittedAnswersOutOfReplay(t *testing.T) {
	result := RunApprovalUserInputContractExercise()
	if result["lateApprovalRejected"] != true {
		t.Fatalf("late approval should be rejected: %#v", result)
	}
	if result["submittedAnswersPrivacyBoundary"] != true {
		t.Fatalf("submitted answers must not be present in replay: %#v", result)
	}
	if result["submitStatus"] != gateStatusOK || result["cancelStatus"] != gateStatusOK {
		t.Fatalf("manager statuses mismatch: %#v", result)
	}
}
