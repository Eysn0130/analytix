package subagent

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	modelapp "analytix.local/runtime-go/internal/app/model"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestCaseForegroundCarrierIsHostCreatedCurrentOneUseAndSerializedZeroByte(t *testing.T) {
	fixture, binding, selection := newCaseForegroundHandoffFixtureV1(t)
	if _, err := fixture.authority.SubmitCase(
		context.Background(), fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, selection,
	); err != nil {
		t.Fatal(err)
	}
	verifiedCompletion, err := fixture.completions.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	completionReceipt, err := verifiedCompletion.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	fixture.record.ChildCompletionReceipt = completionReceipt
	prepared, err := fixture.authority.Prepare(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	handoffReceipt, err := prepared.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	completed := fixture.record
	completed.Status = string(domainjob.StatusCompleted)
	completed.ChildTurnID = fixture.child.TurnID
	completed.ForegroundChildHandoffReceipt = handoffReceipt
	verified, err := fixture.authority.Consume(context.Background(), completed, prepared)
	if err != nil {
		t.Fatal(err)
	}
	output := verified.OutputProjection(completed)
	if output["caseResult"] != nil || output["typedResultAccepted"] != true || output["factAnswerAllowed"] != false {
		t.Fatalf("case carrier escaped or gained authority: %#v", output)
	}
	capability, ok := output[ForegroundParentCapabilityFieldV1].(*ForegroundParentResultCapabilityV1)
	if !ok {
		t.Fatalf("case output lacks process-local capability: %#v", output)
	}
	if digest, allowed := capability.VerifyParentContinuation(fixture.parent, fixture.parentGrant, fixture.parentGrant.ToolCallID); allowed || digest != "" {
		t.Fatalf("unopened case carrier authorized parent continuation: digest=%q allowed=%v", digest, allowed)
	}
	pending := modelapp.PendingToolCall{
		SecurityContext: fixture.parent, ExecutionGrant: fixture.parentGrant,
		Call: domainmodel.ToolCall{ID: fixture.parentGrant.ToolCallID, Name: fixture.parentGrant.ToolName, Arguments: fixture.parentCallArguments},
	}
	for index := 0; index < 2; index++ {
		public, applicable, valid := ProjectForegroundParentPublicToolOutputV1(pending, output)
		if !applicable || !valid || public["answerSlotCount"] != 1 || public["gapCount"] != len(binding.AnswerSlot.Gaps) {
			t.Fatalf("side-effect-free public inspection %d failed: %#v applicable=%v valid=%v", index, public, applicable, valid)
		}
	}
	private, public, applicable, valid := ProjectForegroundParentToolOutputV1(pending, output)
	privateResult, privateResultOK := private["caseResult"].(domainjob.CaseForegroundChildResultV1)
	if !applicable || !valid || !privateResultOK || public["caseResult"] != nil ||
		!reflect.DeepEqual(privateResult.AnswerSlots[0], binding.AnswerSlot) {
		t.Fatalf("one-use private projection lost its typed current-attempt carrier: private=%#v public=%#v", private, public)
	}
	wrongGrant := fixture.childGrant
	if digest, allowed := capability.VerifyParentContinuation(fixture.parent, wrongGrant, fixture.parentGrant.ToolCallID); allowed || digest != "" {
		t.Fatalf("cross-grant case carrier authorized parent continuation: digest=%q allowed=%v", digest, allowed)
	}
	if digest, allowed := capability.VerifyParentContinuation(fixture.parent, fixture.parentGrant, "other-parent-call"); allowed || digest != "" {
		t.Fatalf("cross-call case carrier authorized parent continuation: digest=%q allowed=%v", digest, allowed)
	}
	if digest, allowed := capability.VerifyParentContinuation(fixture.parent, fixture.parentGrant, fixture.parentGrant.ToolCallID); !allowed || digest != capability.receipt.ReceiptDigest {
		t.Fatalf("opened case carrier did not authorize the exact parent continuation: digest=%q allowed=%v", digest, allowed)
	}
	if digest, allowed := capability.VerifyParentContinuation(fixture.parent, fixture.parentGrant, fixture.parentGrant.ToolCallID); allowed || digest != "" {
		t.Fatalf("case carrier authorized a second parent continuation: digest=%q allowed=%v", digest, allowed)
	}
	serialized, err := json.Marshal(map[string]any{"output": outputWithoutForegroundCapabilityV1(output), "public": public})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"caseResult", "answerSlots", `"claims":`, `"evidence":`, `"inflowMinor":"` + binding.AnswerSlot.InflowMinor + `"`,
		binding.Claims[0].Digest, binding.Evidence[0].Digest, binding.AnswerSlot.QueryScopeRef,
	} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("serialized case projection contains private carrier canary %q: %s", forbidden, serialized)
		}
	}
	var restored struct {
		Output map[string]any `json:"output"`
	}
	if err := json.Unmarshal(serialized, &restored); err != nil {
		t.Fatal(err)
	}
	if restoredPrivate, restoredPublic, applicable, valid := ProjectForegroundParentToolOutputV1(pending, restored.Output); !applicable || valid || restoredPrivate != nil || restoredPublic["code"] != "foreground_handoff_projection_invalid" {
		t.Fatalf("serialized/restarted case metadata recreated private authority: private=%#v public=%#v applicable=%v valid=%v", restoredPrivate, restoredPublic, applicable, valid)
	}
	restoredContinuation := ResolveParentContinuationAuthorityV1(
		context.Background(), pending, restored.Output, nil, nil, nil,
	)
	if decision := apploop.EvaluateProviderContinuation(
		fixture.parent, fixture.parentGrant, pending.Call,
		apploop.SettledToolExecution{ParentContinuation: restoredContinuation},
	); !decision.Blocked {
		t.Fatal("serialized/restarted case metadata recreated parent continuation authority")
	}
	if invalidPrivate, _, applicable, valid := ProjectForegroundParentToolOutputV1(pending, output); !applicable || valid || invalidPrivate != nil {
		t.Fatalf("case carrier opened twice: private=%#v applicable=%v valid=%v", invalidPrivate, applicable, valid)
	}
}

func TestCaseForegroundSubmissionRejectsCrossScopeTerminalDuplicateAndRestart(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*domainjob.Record, *domainsecurity.TurnSecurityContext, *domainsecurity.ExecutionGrant, *string, *domainjob.CaseForegroundChildSelectionV1)
		restart bool
	}{
		{name: "cross parent thread", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.ParentThreadID = "thread-other"
		}},
		{name: "cross parent turn run", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.ParentTurnID = "turn-other"
		}},
		{name: "cross parent tool call", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.ParentToolCallID = "call-other"
		}},
		{name: "cross child thread", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.ThreadID = "thread-other"
		}},
		{name: "cross child turn run", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.TurnID = "turn-other"
		}},
		{name: "cross workspace", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.WorkspaceRealPath += "-other"
		}},
		{name: "cross tenant", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.TenantID = "tenant-other"
		}},
		{name: "cross user", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.UserID = "user-other"
		}},
		{name: "cross case", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.CaseID = "case-other"
		}},
		{name: "cross binding", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.CaseBindingHash = domainsecurity.SHA256Hex([]byte("binding-other"))
		}},
		{name: "cross snapshot", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.DatasetSnapshotID = "snapshot-other"
		}},
		{name: "cross source", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.SourceManifestHash = domainsecurity.SHA256Hex([]byte("source-other"))
		}},
		{name: "cross epoch compaction", mutate: func(_ *domainjob.Record, child *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			child.ContextEpoch++
		}},
		{name: "background", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.Background = true
		}},
		{name: "failed", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.Status = string(domainjob.StatusFailed)
		}},
		{name: "canceled", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.Status = string(domainjob.StatusCanceled)
		}},
		{name: "timeout", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.Status = string(domainjob.StatusTimeout)
		}},
		{name: "aborted", mutate: func(record *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			record.Status = string(domainjob.StatusAborted)
		}},
		{name: "cross child grant", mutate: func(_ *domainjob.Record, _ *domainsecurity.TurnSecurityContext, grant *domainsecurity.ExecutionGrant, _ *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			grant.ToolCallID = "call-other"
		}},
		{name: "cross child tool call", mutate: func(_ *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, callID *string, _ *domainjob.CaseForegroundChildSelectionV1) {
			*callID = "call-other"
		}},
		{name: "cross delegation", mutate: func(_ *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, selection *domainjob.CaseForegroundChildSelectionV1) {
			selection.DelegationDigest = domainjob.CaseDelegationProviderCommitmentTokenV1(domainsecurity.SHA256Hex([]byte("delegation-other")))
		}},
		{name: "cross answer slot", mutate: func(_ *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, selection *domainjob.CaseForegroundChildSelectionV1) {
			selection.AnswerSlotDigest = domainjob.CaseDelegationProviderCommitmentTokenV1(domainsecurity.SHA256Hex([]byte("slot-other")))
		}},
		{name: "raw selector ordinary hex", mutate: func(_ *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, selection *domainjob.CaseForegroundChildSelectionV1) {
			selection.DelegationDigest = strings.Repeat("a", 64)
			selection.AnswerSlotDigest = strings.Repeat("b", 64)
		}},
		{name: "raw selector account shaped", mutate: func(_ *domainjob.Record, _ *domainsecurity.TurnSecurityContext, _ *domainsecurity.ExecutionGrant, _ *string, selection *domainjob.CaseForegroundChildSelectionV1) {
			selection.DelegationDigest = strings.Repeat("9", 64)
			selection.AnswerSlotDigest = strings.Repeat("9", 64)
		}},
		{name: "restart resume fork", restart: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, selection := newCaseForegroundHandoffFixtureV1(t)
			record, child, grant, callID := fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID
			if test.mutate != nil {
				test.mutate(&record, &child, &grant, &callID, &selection)
			}
			authority := fixture.authority
			if test.restart {
				authority = NewForegroundHandoffAuthorityWithCaseTyped(
					NewForegroundSubmissionRegistry(), fixture.authority.threads, fixture.authority.contexts,
					fixture.authority.validateCurrent, fixture.authority.terminals, fixture.authority.caseCompletions,
					fixture.authority.resolveCase, fixture.authority.validateCaseDelegation,
				)
			}
			if output, err := authority.SubmitCase(context.Background(), record, child, grant, callID, selection); err == nil || output != nil {
				t.Fatalf("hostile case submission was accepted: output=%#v err=%v", output, err)
			}
			fixture.authority.registry.mu.Lock()
			entry := fixture.authority.registry.entries[fixture.record.ID]
			fixture.authority.registry.mu.Unlock()
			if entry.CaseAllowedBinding == nil || entry.CaseResult != nil || entry.ConsumptionNonce != "" || entry.Consumed {
				t.Fatalf("rejected case submission mutated parent-visible state: %#v", entry)
			}
		})
	}

	fixture, _, selection := newCaseForegroundHandoffFixtureV1(t)
	first, err := fixture.authority.SubmitCase(
		context.Background(), fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, selection,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.authority.registry.mu.Lock()
	before := fixture.authority.registry.entries[fixture.record.ID]
	fixture.authority.registry.mu.Unlock()
	if duplicate, err := fixture.authority.SubmitCase(
		context.Background(), fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, selection,
	); err == nil || duplicate != nil {
		t.Fatalf("exact duplicate case submission was accepted twice: %#v err=%v", duplicate, err)
	}
	fixture.authority.registry.mu.Lock()
	after := fixture.authority.registry.entries[fixture.record.ID]
	fixture.authority.registry.mu.Unlock()
	if first["submissionDigest"] != before.SubmissionDigest || before.SubmissionDigest != after.SubmissionDigest ||
		before.ConsumptionNonce == "" || before.ConsumptionNonce != after.ConsumptionNonce || !reflect.DeepEqual(before.CaseResult, after.CaseResult) {
		t.Fatalf("duplicate case submission changed the live result: before=%#v after=%#v", before, after)
	}
}

func TestCaseForegroundConsumeRevalidatesLiveGrantResultAndTerminalState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*caseForegroundHandoffFixtureV1, *domainjob.Record)
	}{
		{name: "child grant changed", mutate: func(fixture *caseForegroundHandoffFixtureV1, _ *domainjob.Record) {
			fixture.authority.registry.mu.Lock()
			entry := fixture.authority.registry.entries[fixture.record.ID]
			entry.ChildExecutionGrantID = domainsecurity.SHA256Hex([]byte("other child grant"))
			fixture.authority.registry.entries[fixture.record.ID] = entry
			fixture.authority.registry.mu.Unlock()
		}},
		{name: "child tool call changed", mutate: func(fixture *caseForegroundHandoffFixtureV1, _ *domainjob.Record) {
			fixture.authority.registry.mu.Lock()
			entry := fixture.authority.registry.entries[fixture.record.ID]
			entry.ChildToolCallID = "other-child-call"
			fixture.authority.registry.entries[fixture.record.ID] = entry
			fixture.authority.registry.mu.Unlock()
		}},
		{name: "typed result changed", mutate: func(fixture *caseForegroundHandoffFixtureV1, _ *domainjob.Record) {
			fixture.authority.registry.mu.Lock()
			entry := fixture.authority.registry.entries[fixture.record.ID]
			entry.CaseResult.Claims[0].Digest = domainsecurity.SHA256Hex([]byte("other claim"))
			fixture.authority.registry.entries[fixture.record.ID] = entry
			fixture.authority.registry.mu.Unlock()
		}},
		{name: "current registry revoked", mutate: func(fixture *caseForegroundHandoffFixtureV1, _ *domainjob.Record) {
			fixture.authority.resolveCase = func(context.Context, domainsecurity.TurnSecurityContext, domainjob.CaseForegroundChildResultV1) error {
				return context.Canceled
			}
		}},
		{name: "failed terminal", mutate: func(_ *caseForegroundHandoffFixtureV1, record *domainjob.Record) {
			record.Status = string(domainjob.StatusFailed)
		}},
		{name: "canceled terminal", mutate: func(_ *caseForegroundHandoffFixtureV1, record *domainjob.Record) {
			record.Status = string(domainjob.StatusCanceled)
		}},
		{name: "timed out terminal", mutate: func(_ *caseForegroundHandoffFixtureV1, record *domainjob.Record) {
			record.Status = string(domainjob.StatusTimeout)
		}},
		{name: "background", mutate: func(_ *caseForegroundHandoffFixtureV1, record *domainjob.Record) {
			record.Background = true
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, selection := newCaseForegroundHandoffFixtureV1(t)
			completed, prepared := prepareCaseForegroundConsumptionV1(t, &fixture, selection)
			test.mutate(&fixture, &completed)
			if verified, err := fixture.authority.Consume(context.Background(), completed, prepared); err == nil || verified.valid {
				t.Fatalf("hostile case consumption was accepted: verified=%#v err=%v", verified, err)
			}
			fixture.authority.registry.mu.Lock()
			entry := fixture.authority.registry.entries[fixture.record.ID]
			fixture.authority.registry.mu.Unlock()
			if entry.Consumed || entry.CaseResult == nil {
				t.Fatalf("rejected case consumption changed parent-visible state: %#v", entry)
			}
		})
	}

	fixture, _, selection := newCaseForegroundHandoffFixtureV1(t)
	completed, prepared := prepareCaseForegroundConsumptionV1(t, &fixture, selection)
	if _, err := fixture.authority.Consume(context.Background(), completed, prepared); err != nil {
		t.Fatal(err)
	}
	if duplicate, err := fixture.authority.Consume(context.Background(), completed, prepared); err == nil || duplicate.valid {
		t.Fatalf("case handoff was consumed twice: verified=%#v err=%v", duplicate, err)
	}
	fixture.authority.registry.mu.Lock()
	consumed := fixture.authority.registry.entries[fixture.record.ID]
	fixture.authority.registry.mu.Unlock()
	if !consumed.Consumed || consumed.CaseResult != nil {
		t.Fatalf("duplicate consumption changed the settled one-use state: %#v", consumed)
	}
}

func TestCaseForegroundCarrierDoubleOpenRaceHasOneWinner(t *testing.T) {
	fixture, _, selection := newCaseForegroundHandoffFixtureV1(t)
	capability, output := caseForegroundProjectionCapabilityV1(t, fixture, selection)
	var wait sync.WaitGroup
	winners := make(chan bool, 2)
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, opened := capability.OpenCaseForParent(
				fixture.parent, fixture.parentGrant.GrantID, fixture.parentGrant.ToolCallID, output,
			)
			winners <- opened
		}()
	}
	wait.Wait()
	close(winners)
	count := 0
	for won := range winners {
		if won {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("double-open race had %d winners", count)
	}
}

func TestCaseForegroundCarrierContinuationRaceHasOneWinner(t *testing.T) {
	fixture, _, selection := newCaseForegroundHandoffFixtureV1(t)
	capability, output := caseForegroundProjectionCapabilityV1(t, fixture, selection)
	if _, opened := capability.OpenCaseForParent(
		fixture.parent, fixture.parentGrant.GrantID, fixture.parentGrant.ToolCallID, output,
	); !opened {
		t.Fatal("case carrier did not open before continuation race")
	}
	var wait sync.WaitGroup
	winners := make(chan bool, 2)
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, allowed := capability.VerifyParentContinuation(
				fixture.parent, fixture.parentGrant, fixture.parentGrant.ToolCallID,
			)
			winners <- allowed
		}()
	}
	wait.Wait()
	close(winners)
	count := 0
	for won := range winners {
		if won {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("parent continuation race had %d winners", count)
	}
}

type caseForegroundHandoffFixtureV1 struct {
	parent, child       domainsecurity.TurnSecurityContext
	parentGrant         domainsecurity.ExecutionGrant
	childGrant          domainsecurity.ExecutionGrant
	record              domainjob.Record
	authority           *ForegroundHandoffAuthority
	completions         *ChildCompletionAuthority
	parentCallArguments json.RawMessage
}

func newCaseForegroundHandoffFixtureV1(t *testing.T) (caseForegroundHandoffFixtureV1, domainjob.CaseDelegatedAnswerSlotBindingV1, domainjob.CaseForegroundChildSelectionV1) {
	t.Helper()
	base := newChildCompletionFixture(t)
	parentArguments := map[string]any{"prompt": "bounded case", "max_steps": 2, "token_budget": 512, "time_budget_ms": 30000}
	parentArgumentsBody, _ := json.Marshal(parentArguments)
	parentGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: base.parent, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "task",
		ToolCallID: base.grant.ToolCallID, ArgsHash: domainsecurity.CanonicalJSONHash(parentArgumentsBody),
		SchemaHash: domainsecurity.SHA256Hex([]byte("case-parent-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("case-parent-scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: base.issuedAt.Add(-2 * time.Minute), ExpiresAt: base.issuedAt.Add(5 * time.Minute),
	})
	securityBinding, err := domainjob.NewSecurityBinding(base.parent, parentGrant, parentGrant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	grantBody, _ := json.Marshal(parentGrant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	thread := map[string]any{
		"id": base.parent.ThreadID, "securityState": jobSecurityContractRecord(base.parent),
		"turns": []any{map[string]any{
			"id": base.parent.TurnID, "threadId": base.parent.ThreadID, "securityContext": jobSecurityContractRecord(base.parent),
			"items": []any{map[string]any{
				"id": domaintoolcall.ToolCallItemIDV1(base.parent.TurnID, parentGrant.ToolCallID), "threadId": base.parent.ThreadID,
				"turnId": base.parent.TurnID, "kind": "tool_call", "toolName": "task", "callId": parentGrant.ToolCallID,
				"arguments": parentArguments, "createdAt": base.issuedAt.Add(-2 * time.Minute).Format(time.RFC3339Nano),
				"contextDigest": base.parent.ContextDigest, "contextEpoch": float64(base.parent.ContextEpoch),
				"executionGrantId": parentGrant.GrantID, "executionGrant": grantRecord,
			}},
		}},
	}
	queryHash := domainsecurity.SHA256Hex([]byte("case-foreground-unit-query"))
	resultHash := domainsecurity.SHA256Hex([]byte("case-foreground-unit-result"))
	queryScope, err := domainevidence.NewAccountFlowQueryScopeRefV1(base.parent.ContextDigest, queryHash)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := domainnative.NewAccountFlowProviderOutcomeV1("acct:1", true, true, queryHash, resultHash)
	if err != nil {
		t.Fatal(err)
	}
	slot := domainnative.AccountFlowDelegatedAnswerSlotV1{
		SubjectAlias: "acct:1", StartInclusive: "2026-01-01T00:00:00.000000Z", EndInclusive: "2026-01-31T23:59:59.000000Z",
		Timezone: "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		InflowMinor: "731", OutflowMinor: "19", NetMinor: "712", TransactionCount: 3,
		AggregateComplete: true, EvidenceRowsComplete: true, CounterpartySemanticsComplete: false,
		Gaps: []string{domainnative.AccountFlowGapCounterpartyResolutionV1}, Currentness: domaincaseentity.SnapshotCurrentV1,
		QueryScopeRef: queryScope, QueryHash: queryHash, ResultHash: resultHash, Outcome: outcome,
	}
	if err := domainnative.ValidateAccountFlowDelegatedAnswerSlotV1(slot); err != nil {
		t.Fatal(err)
	}
	binding := domainjob.CaseDelegatedAnswerSlotBindingV1{
		AnswerSlot: slot,
		Claims: []domainjob.CaseDelegatedClaimReferenceV1{
			{Digest: domainsecurity.SHA256Hex([]byte("case-claim-1")), InvestigationState: domaincaseentity.InvestigationConfirmedV1},
			{Digest: domainsecurity.SHA256Hex([]byte("case-claim-2")), InvestigationState: domaincaseentity.InvestigationConfirmedV1},
			{Digest: domainsecurity.SHA256Hex([]byte("case-claim-3")), InvestigationState: domaincaseentity.InvestigationConfirmedV1},
		},
		Evidence: []domainjob.CaseDelegatedEvidenceReferenceV1{{
			Digest: domainsecurity.SHA256Hex([]byte("case-evidence-1")), Currentness: domaincaseentity.SnapshotCurrentV1,
		}},
	}
	sort.Slice(binding.Claims, func(left, right int) bool { return binding.Claims[left].Digest < binding.Claims[right].Digest })
	commitment, err := domainjob.NewCaseDelegatedAnswerSlotCommitmentV1(binding)
	if err != nil {
		t.Fatal(err)
	}
	delegation, err := domainjob.NewCaseDelegationContextV1(securityBinding, domainjob.CaseDelegationSemanticContextV1{
		TaskKind: domainjob.CaseDelegationTaskKindV1,
		Entities: []domainjob.CaseDelegatedEntitySemanticV1{{
			Alias: "acct:1", EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			ResolutionDigest:     domainsecurity.SHA256Hex([]byte("case-resolution")),
		}},
		Currentness: domaincaseentity.SnapshotCurrentV1,
		Claims:      []domainjob.CaseDelegatedClaimReferenceV1{}, Evidence: []domainjob.CaseDelegatedEvidenceReferenceV1{},
		Continuations: []domainjob.CaseDelegatedContinuationReferenceV1{}, AnswerSlots: []domainjob.CaseDelegatedAnswerSlotCommitmentV1{commitment},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := domainjob.CaseDelegationProviderPromptV1(delegation, securityBinding)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"inflowMinor", "outflowMinor", "netMinor", "transactionCount", "queryHash", "resultHash", "queryScopeRef",
		binding.Claims[0].Digest, binding.Evidence[0].Digest, `"claims":`, `"evidence":`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("durable case prompt contains private carrier canary %q: %s", forbidden, prompt)
		}
	}
	record := base.record
	record.SecurityBinding = securityBinding
	record.ParentToolCallID = parentGrant.ToolCallID
	record.Status = string(domainjob.StatusQueued)
	record.Workspace = base.child.WorkspaceRealPath
	record.ToolScope = []string{toolcatalogForegroundSubmitToolName}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	record.CaseDelegation = delegation
	record.Name = domainjob.CaseDelegationNameV1
	record.Label = domainjob.CaseDelegationLabelV1
	record.Prompt = prompt
	registry := NewForegroundSubmissionRegistry()
	registry.now = func() time.Time { return base.issuedAt }
	contexts := map[string]domainsecurity.TurnSecurityContext{
		base.parent.ThreadID + "\x00" + base.parent.TurnID: base.parent,
		base.child.ThreadID + "\x00" + base.child.TurnID:   base.child,
	}
	finalStub, ok := base.authority.finals.(childCompletionFinalStub)
	if !ok {
		t.Fatal("child completion final fixture is unavailable")
	}
	completions := NewChildCompletionAuthority(
		func(threadID, turnID string) (domainsecurity.TurnSecurityContext, bool) {
			value, ok := contexts[threadID+"\x00"+turnID]
			return value, ok
		}, finalStub, base.host, jobSecurityThreadStub{thread: thread},
		func(_ context.Context, current domainsecurity.TurnSecurityContext) error {
			if value, ok := contexts[current.ThreadID+"\x00"+current.TurnID]; !ok || value != current {
				return context.Canceled
			}
			return nil
		},
	)
	completions.now = func() time.Time { return base.issuedAt }
	authority := NewForegroundHandoffAuthorityWithCaseTyped(
		registry, jobSecurityThreadStub{thread: thread},
		func(threadID, turnID string) (domainsecurity.TurnSecurityContext, bool) {
			value, ok := contexts[threadID+"\x00"+turnID]
			return value, ok
		},
		func(_ context.Context, current domainsecurity.TurnSecurityContext) error {
			if value, ok := contexts[current.ThreadID+"\x00"+current.TurnID]; !ok || value != current {
				return context.Canceled
			}
			return nil
		}, nil, completions,
		func(_ context.Context, _ domainsecurity.TurnSecurityContext, result domainjob.CaseForegroundChildResultV1) error {
			if !reflect.DeepEqual(result.AnswerSlots[0], binding.AnswerSlot) ||
				!reflect.DeepEqual(result.Claims, binding.Claims) || !reflect.DeepEqual(result.Evidence, binding.Evidence) {
				return context.Canceled
			}
			return nil
		},
		func(_ context.Context, current domainjob.Record, parent domainsecurity.TurnSecurityContext) error {
			if parent != base.parent || !domainjob.CaseDelegationContextsEqualV1(current.CaseDelegation, delegation) {
				return context.Canceled
			}
			return nil
		},
	)
	if err := authority.StageCase(record, base.parent, binding); err != nil {
		t.Fatal(err)
	}
	record.Status = string(domainjob.StatusRunning)
	childGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: base.child, Provider: "provider", ServerIdentity: "host:builtin", ToolName: toolcatalogForegroundSubmitToolName,
		ToolCallID: subagentTestHostToolCallID("case-child-submit"), ArgsHash: domainsecurity.SHA256Hex([]byte("case-child-args")),
		SchemaHash: domainsecurity.SHA256Hex([]byte("case-child-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("case-child-scope")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: base.issuedAt.Add(-time.Second), ExpiresAt: base.issuedAt.Add(5 * time.Minute),
	})
	selection := domainjob.CaseForegroundChildSelectionV1{
		SchemaVersion:    domainjob.CaseForegroundChildResultSchemaVersionV1,
		Purpose:          domainjob.CaseForegroundChildSelectionPurposeV1,
		DelegationDigest: domainjob.CaseDelegationProviderCommitmentTokenV1(delegation.DelegationDigest),
		AnswerSlotDigest: domainjob.CaseDelegationProviderCommitmentTokenV1(commitment.Digest),
	}
	return caseForegroundHandoffFixtureV1{
		parent: base.parent, child: base.child, parentGrant: parentGrant, childGrant: childGrant,
		record: record, authority: authority, completions: completions,
		parentCallArguments: append(json.RawMessage(nil), parentArgumentsBody...),
	}, binding, selection
}

func caseForegroundProjectionCapabilityV1(
	t *testing.T,
	fixture caseForegroundHandoffFixtureV1,
	selection domainjob.CaseForegroundChildSelectionV1,
) (*ForegroundParentResultCapabilityV1, map[string]any) {
	t.Helper()
	if _, err := fixture.authority.SubmitCase(context.Background(), fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, selection); err != nil {
		t.Fatal(err)
	}
	verifiedCompletion, err := fixture.completions.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := verifiedCompletion.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	fixture.record.ChildCompletionReceipt = receipt
	prepared, err := fixture.authority.Prepare(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := prepared.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	completed := fixture.record
	completed.Status = string(domainjob.StatusCompleted)
	completed.ChildTurnID = fixture.child.TurnID
	completed.ForegroundChildHandoffReceipt = handoff
	verified, err := fixture.authority.Consume(context.Background(), completed, prepared)
	if err != nil {
		t.Fatal(err)
	}
	output := verified.OutputProjection(completed)
	capability, ok := output[ForegroundParentCapabilityFieldV1].(*ForegroundParentResultCapabilityV1)
	if !ok {
		t.Fatalf("case output lacks private capability: %#v", output)
	}
	return capability, output
}

func prepareCaseForegroundConsumptionV1(
	t *testing.T,
	fixture *caseForegroundHandoffFixtureV1,
	selection domainjob.CaseForegroundChildSelectionV1,
) (domainjob.Record, PreparedForegroundHandoff) {
	t.Helper()
	if _, err := fixture.authority.SubmitCase(
		context.Background(), fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, selection,
	); err != nil {
		t.Fatal(err)
	}
	verifiedCompletion, err := fixture.completions.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	completionReceipt, err := verifiedCompletion.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	fixture.record.ChildCompletionReceipt = completionReceipt
	prepared, err := fixture.authority.Prepare(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	handoffReceipt, err := prepared.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	completed := fixture.record
	completed.Status = string(domainjob.StatusCompleted)
	completed.ChildTurnID = fixture.child.TurnID
	completed.ForegroundChildHandoffReceipt = handoffReceipt
	return completed, prepared
}

func outputWithoutForegroundCapabilityV1(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, entry := range value {
		if key != ForegroundParentCapabilityFieldV1 {
			out[key] = entry
		}
	}
	return out
}

type foregroundChildRunReaderStub struct {
	record            domainjob.Record
	ignoreRequestedID bool
}

func (reader foregroundChildRunReaderStub) LoadChildRun(id string) (domainjob.Record, error) {
	if !reader.ignoreRequestedID && id != reader.record.ID {
		return domainjob.Record{}, context.Canceled
	}
	return reader.record, nil
}

type typedNilForegroundChildRunReader struct{}

func (*typedNilForegroundChildRunReader) LoadChildRun(string) (domainjob.Record, error) {
	panic("typed-nil foreground child reader was called")
}

func TestSubmitForegroundChildResultOwnsPendingIdentityAndScope(t *testing.T) {
	fixture := newForegroundHandoffFixture(t)
	pending := modelapp.PendingToolCall{
		ProviderNamespace: modelapp.ProviderNamespace("subagent", fixture.record.ID, 1, nil),
		SubagentDepth:     1,
		ToolScope:         []string{toolcatalogForegroundSubmitToolName},
		SecurityContext:   fixture.child,
		ExecutionGrant:    fixture.childGrant,
		Call: domainmodel.ToolCall{
			ID: fixture.childGrant.ToolCallID, Name: toolcatalogForegroundSubmitToolName,
		},
	}
	reader := foregroundChildRunReaderStub{record: fixture.record}
	var typedNilReader *typedNilForegroundChildRunReader
	if output, isError := SubmitForegroundChildResult(context.Background(), fixture.authority, typedNilReader, pending, map[string]any{"result": "ignored"}); !isError ||
		output.(map[string]any)["code"] != "foreground_handoff_unavailable" {
		t.Fatalf("typed-nil child reader was accepted: %#v", output)
	}
	invalid := pending
	invalid.SubagentDepth = 0
	if output, isError := SubmitForegroundChildResult(context.Background(), fixture.authority, reader, invalid, map[string]any{"result": "ignored"}); !isError ||
		output.(map[string]any)["code"] != "foreground_handoff_unavailable" {
		t.Fatalf("invalid child scope was accepted: %#v", output)
	}
	mismatched := reader
	mismatched.record.ID = "different-child-run"
	mismatched.ignoreRequestedID = true
	if output, isError := SubmitForegroundChildResult(context.Background(), fixture.authority, mismatched, pending, map[string]any{"result": "ignored"}); !isError ||
		output.(map[string]any)["code"] != "foreground_handoff_invalid" {
		t.Fatalf("mismatched child record was accepted: %#v", output)
	}
	if output, isError := SubmitForegroundChildResult(context.Background(), fixture.authority, reader, pending, map[string]any{"result": 42}); !isError ||
		output.(map[string]any)["code"] != "validation_error" {
		t.Fatalf("non-string child result was accepted: %#v", output)
	}
	output, isError := SubmitForegroundChildResult(
		context.Background(), fixture.authority, reader, pending, map[string]any{"result": "bounded public result"},
	)
	response, ok := output.(map[string]any)
	if isError || !ok || response["status"] != "accepted" || response["factAnswerAllowed"] != false ||
		response["evidenceAuthority"] != false {
		t.Fatalf("valid child result did not settle through the foreground authority: %#v", output)
	}
	if duplicate, isError := SubmitForegroundChildResult(
		context.Background(), fixture.authority, reader, pending, map[string]any{"result": "second"},
	); !isError || duplicate.(map[string]any)["code"] != "foreground_handoff_rejected" {
		t.Fatalf("duplicate child result was accepted: %#v", duplicate)
	}
}

func TestForegroundSubmissionRegistryRejectsDuplicateMismatchAndReasoning(t *testing.T) {
	fixture := newForegroundHandoffFixture(t)
	registry := fixture.registry
	otherChild := fixture.child
	otherChild.ThreadID = "forged-thread"
	if _, err := registry.Submit(fixture.record, otherChild, fixture.childGrant, fixture.childGrant.ToolCallID, "cross thread"); err == nil {
		t.Fatal("cross-thread foreground submission was accepted")
	}
	otherChild = fixture.child
	otherChild.WorkspaceRealPath += "/other"
	if _, err := registry.Submit(fixture.record, otherChild, fixture.childGrant, fixture.childGrant.ToolCallID, "cross workspace"); err == nil {
		t.Fatal("cross-workspace foreground submission was accepted")
	}
	otherChild = fixture.child
	otherChild.ContextEpoch++
	if _, err := registry.Submit(fixture.record, otherChild, fixture.childGrant, fixture.childGrant.ToolCallID, "cross epoch"); err == nil {
		t.Fatal("cross-epoch foreground submission was accepted")
	}
	if _, err := registry.Submit(fixture.record, fixture.child, fixture.childGrant, subagentTestHostToolCallID("wrong-call"), "wrong call"); err == nil {
		t.Fatal("wrong foreground tool call was accepted")
	}
	wrongGrant := fixture.childGrant
	wrongGrant.GrantID = domainsecurity.SHA256Hex([]byte("wrong-grant"))
	if _, err := registry.Submit(fixture.record, fixture.child, wrongGrant, wrongGrant.ToolCallID, "wrong grant"); err == nil {
		t.Fatal("tampered foreground grant was accepted")
	}
	failed := fixture.record
	failed.Status = string(domainjob.StatusFailed)
	if _, err := registry.Submit(failed, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, "failed child"); err == nil {
		t.Fatal("failed foreground child submitted a result")
	}
	accepted, err := registry.Submit(fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, "bounded public result")
	if err != nil {
		t.Fatal(err)
	}
	if accepted["status"] != "accepted" || accepted["factAnswerAllowed"] != false || accepted["evidenceAuthority"] != false {
		t.Fatalf("submission response gained authority: %#v", accepted)
	}
	if _, err := registry.Submit(fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, "duplicate"); err == nil {
		t.Fatal("duplicate foreground submission was accepted")
	}

	second := newForegroundHandoffFixture(t)
	if _, err := second.registry.Submit(second.record, second.child, second.childGrant, second.childGrant.ToolCallID, "<think>private chain</think>"); err == nil {
		t.Fatal("reasoning-only foreground submission was accepted")
	}
	caseFact := newForegroundHandoffFixture(t)
	if _, err := caseFact.registry.Submit(caseFact.record, caseFact.child, caseFact.childGrant, caseFact.childGrant.ToolCallID,
		"张某实际控制甲公司，涉案金额为￥2,645,472.00。"); err == nil {
		t.Fatal("protected case fact candidate entered the parent handoff")
	}
}

func TestForegroundHandoffConsumesLiveProjectedBodyExactlyOnceAndNeverPersistsIt(t *testing.T) {
	fixture := newForegroundHandoffFixture(t)
	registry := fixture.registry
	privateResult := "bounded parent-only result"
	if _, err := registry.Submit(fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, privateResult); err != nil {
		t.Fatal(err)
	}
	prepared, err := fixture.authority.Prepare(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := prepared.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	completed := fixture.record
	completed.Status = string(domainjob.StatusCompleted)
	completed.ChildTurnID = fixture.child.TurnID
	completed.ForegroundChildHandoffReceipt = receipt
	verified, err := fixture.authority.Consume(context.Background(), completed, prepared)
	if err != nil {
		t.Fatal(err)
	}
	projection := verified.OutputProjection(completed)
	if projection["result"] != privateResult || projection["factAnswerAllowed"] != false ||
		projection["parentGoalCompletionAllowed"] != false || projection["parentTodoCompletionAllowed"] != false {
		t.Fatalf("foreground output projection drifted: %#v", projection)
	}
	capability, ok := projection[ForegroundParentCapabilityFieldV1].(*ForegroundParentResultCapabilityV1)
	if !ok {
		t.Fatalf("foreground output lacks its process-local capability: %#v", projection)
	}
	if result, opened := capability.OpenForParent(fixture.parent, fixture.parentGrant.GrantID, fixture.parentGrant.ToolCallID, projection); !opened || result != privateResult {
		t.Fatalf("exact parent could not open foreground result: result=%q opened=%v", result, opened)
	}
	if _, opened := capability.OpenForParent(fixture.parent, domainsecurity.SHA256Hex([]byte("other-parent-grant")), fixture.parentGrant.ToolCallID, projection); opened {
		t.Fatal("cross-grant foreground capability opened")
	}
	if _, opened := capability.OpenForParent(fixture.parent, fixture.parentGrant.GrantID, subagentTestHostToolCallID("other-parent-call"), projection); opened {
		t.Fatal("cross-call foreground capability opened")
	}
	if _, err := fixture.authority.Consume(context.Background(), completed, prepared); err == nil {
		t.Fatal("foreground handoff receipt was consumed twice")
	}
	persisted := domainjob.NormalizePersistedRecordV1(completed)
	body, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Output != "" || strings.Contains(string(body), privateResult) {
		t.Fatalf("foreground body entered durable job state: %s", body)
	}
	if live := registry.entries[fixture.record.ID]; live.ProjectedResult != "" || !live.Consumed {
		t.Fatalf("consumed foreground body remained live: %#v", live)
	}
}

func TestForegroundHandoffRejectsStaleContextAtFinalConsumption(t *testing.T) {
	fixture := newForegroundHandoffFixture(t)
	if _, err := fixture.registry.Submit(fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, "bounded result"); err != nil {
		t.Fatal(err)
	}
	prepared, err := fixture.authority.Prepare(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := prepared.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	completed := fixture.record
	completed.Status = string(domainjob.StatusCompleted)
	completed.ChildTurnID = fixture.child.TurnID
	completed.ForegroundChildHandoffReceipt = receipt
	*fixture.rejectCurrent = true
	if _, err := fixture.authority.Consume(context.Background(), completed, prepared); err == nil {
		t.Fatal("stale context consumed a foreground handoff")
	}
	if live := fixture.registry.entries[fixture.record.ID]; live.Consumed || live.ProjectedResult == "" {
		t.Fatalf("failed stale consume changed the live capability: %#v", live)
	}
}

func TestForegroundHandoffRejectsNonSuccessfulAndChangedChildTerminal(t *testing.T) {
	for _, terminal := range []struct {
		name   string
		status string
		reason string
	}{
		{name: "failed", status: "failed", reason: "failure"},
		{name: "canceled", status: "aborted", reason: "cancel"},
		{name: "timed_out", status: "failed", reason: "timeout"},
	} {
		t.Run(terminal.name, func(t *testing.T) {
			fixture := newForegroundHandoffFixture(t)
			fixture.terminal.TerminalStatus = terminal.status
			fixture.terminal.TerminalReason = terminal.reason
			if _, err := fixture.registry.Submit(fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, "bounded result"); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.authority.Prepare(context.Background(), fixture.record, fixture.child.TurnID); err == nil {
				t.Fatal("non-successful foreground child terminal was accepted")
			}
		})
	}

	fixture := newForegroundHandoffFixture(t)
	if _, err := fixture.registry.Submit(fixture.record, fixture.child, fixture.childGrant, fixture.childGrant.ToolCallID, "bounded result"); err != nil {
		t.Fatal(err)
	}
	prepared, err := fixture.authority.Prepare(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := prepared.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	completed := fixture.record
	completed.Status = string(domainjob.StatusCompleted)
	completed.ChildTurnID = fixture.child.TurnID
	completed.ForegroundChildHandoffReceipt = receipt
	fixture.terminal.TerminalDigest = domainsecurity.SHA256Hex([]byte("replaced-terminal"))
	if _, err := fixture.authority.Consume(context.Background(), completed, prepared); err == nil {
		t.Fatal("changed foreground child terminal was consumed")
	}
}

type foregroundHandoffFixture struct {
	now           time.Time
	parent        domainsecurity.TurnSecurityContext
	child         domainsecurity.TurnSecurityContext
	parentGrant   domainsecurity.ExecutionGrant
	childGrant    domainsecurity.ExecutionGrant
	record        domainjob.Record
	registry      *ForegroundSubmissionRegistry
	authority     *ForegroundHandoffAuthority
	rejectCurrent *bool
	terminal      *ForegroundChildTerminalV1
	terminalOK    *bool
}

func newForegroundHandoffFixture(t *testing.T) foregroundHandoffFixture {
	t.Helper()
	now := time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC)
	workspace := t.TempDir()
	parent, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "foreground-parent", TurnID: "foreground-parent-turn", WorkspaceRealPath: workspace,
		ContextEpoch: 2, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "foreground-child", TurnID: "foreground-child-turn", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"prompt": "bounded", "max_steps": 2, "token_budget": 512, "time_budget_ms": 30000}
	argumentsBody, _ := json.Marshal(arguments)
	parentCallID := subagentTestHostToolCallID("foreground-parent")
	parentGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: parent, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: parentCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentsBody), SchemaHash: domainsecurity.SHA256Hex([]byte("parent-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("parent-scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(5 * time.Minute),
	})
	binding, err := domainjob.NewSecurityBinding(parent, parentGrant, parentCallID)
	if err != nil {
		t.Fatal(err)
	}
	childCallID := subagentTestHostToolCallID("foreground-child")
	childGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: child, Provider: "provider", ServerIdentity: "host:builtin", ToolName: toolcatalogForegroundSubmitToolName, ToolCallID: childCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("child-args")), SchemaHash: domainsecurity.SHA256Hex([]byte("child-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("child-scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
	})
	record := domainjob.Record{
		ID: "foreground-run", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID,
		ParentToolCallID: parentCallID, SecurityBinding: binding, ChildThreadID: child.ThreadID,
		Kind: "subagent", Status: string(domainjob.StatusRunning), Workspace: workspace,
		ToolScope: []string{toolcatalogForegroundSubmitToolName},
	}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	registry := NewForegroundSubmissionRegistry()
	registry.now = func() time.Time { return now }
	contexts := map[string]domainsecurity.TurnSecurityContext{
		parent.ThreadID + "\x00" + parent.TurnID: parent,
		child.ThreadID + "\x00" + child.TurnID:   child,
	}
	grantBody, _ := json.Marshal(parentGrant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	thread := map[string]any{
		"id": parent.ThreadID, "securityState": jobSecurityContractRecord(parent),
		"turns": []any{map[string]any{
			"id": parent.TurnID, "threadId": parent.ThreadID, "securityContext": jobSecurityContractRecord(parent),
			"items": []any{map[string]any{
				"id": domaintoolcall.ToolCallItemIDV1(parent.TurnID, parentCallID), "threadId": parent.ThreadID,
				"turnId": parent.TurnID, "kind": "tool_call", "toolName": "task", "callId": parentCallID,
				"arguments": arguments, "createdAt": now.Add(-time.Minute).Format(time.RFC3339Nano),
				"contextDigest": parent.ContextDigest, "contextEpoch": float64(parent.ContextEpoch),
				"executionGrantId": parentGrant.GrantID, "executionGrant": grantRecord,
			}},
		}},
	}
	rejectCurrent := false
	terminal := ForegroundChildTerminalV1{
		SecurityContext: child, TerminalDigest: domainsecurity.SHA256Hex([]byte("foreground-child-general-terminal")),
		TerminalStatus: "completed", TerminalReason: "success",
	}
	terminalOK := true
	authority := NewForegroundHandoffAuthority(
		registry, jobSecurityThreadStub{thread: thread},
		func(threadID, turnID string) (domainsecurity.TurnSecurityContext, bool) {
			value, ok := contexts[threadID+"\x00"+turnID]
			return value, ok
		},
		func(_ context.Context, current domainsecurity.TurnSecurityContext) error {
			if rejectCurrent {
				return context.Canceled
			}
			value, ok := contexts[current.ThreadID+"\x00"+current.TurnID]
			if !ok || value != current {
				return context.Canceled
			}
			return nil
		},
		func(threadID, turnID string) (ForegroundChildTerminalV1, bool) {
			if !terminalOK || threadID != child.ThreadID || turnID != child.TurnID {
				return ForegroundChildTerminalV1{}, false
			}
			return terminal, true
		},
	)
	return foregroundHandoffFixture{
		now: now, parent: parent, child: child, parentGrant: parentGrant, childGrant: childGrant,
		record: record, registry: registry, authority: authority, rejectCurrent: &rejectCurrent,
		terminal: &terminal, terminalOK: &terminalOK,
	}
}
