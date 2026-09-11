package job

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestForegroundChildHandoffReceiptV1CanonicalRoundTripAndSafetyCeiling(t *testing.T) {
	input := foregroundChildHandoffFixtureV1()
	receipt, err := NewForegroundChildHandoffReceiptV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != ForegroundChildHandoffReceiptAcceptedV1 || receipt.FactAnswerAllowed || receipt.EvidenceAuthority ||
		receipt.ParentGoalCompletionAllowed || receipt.ParentTodoCompletionAllowed {
		t.Fatalf("foreground child handoff safety ceiling is invalid: %#v", receipt)
	}
	if receipt.ParentThreadID != input.ParentThreadID || receipt.ParentTurnID != input.ParentTurnID ||
		receipt.ParentRunID != input.ParentRunID || receipt.ChildThreadID != input.ChildThreadID ||
		receipt.ChildTurnID != input.ChildTurnID || receipt.ChildRunID != input.ChildRunID ||
		receipt.WorkspaceRealPath != input.WorkspaceRealPath || receipt.ParentContextEpoch != input.ParentContextEpoch ||
		receipt.ChildContextEpoch != input.ChildContextEpoch || receipt.ParentExecutionGrantID != input.ParentExecutionGrantID ||
		receipt.ParentToolCallID != input.ParentToolCallID || receipt.ChildAcceptedTerminalDigest != input.ChildAcceptedTerminalDigest ||
		receipt.SubmissionDigest != input.SubmissionDigest || receipt.PrivacyProjectionDigest != input.PrivacyProjectionDigest ||
		receipt.ConsumptionNonce != input.ConsumptionNonce {
		t.Fatalf("foreground child handoff did not preserve exact bindings: %#v", receipt)
	}
	body, err := ForegroundChildHandoffReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CaseBinding != nil || bytes.Contains(body, []byte(`"caseBinding"`)) {
		t.Fatalf("ordinary foreground receipt changed its canonical wire shape: %s", body)
	}
	parsed, err := ParseForegroundChildHandoffReceiptV1(body)
	if err != nil || parsed != receipt {
		t.Fatalf("canonical handoff round trip failed: parsed=%#v err=%v", parsed, err)
	}
	duplicate, err := NewForegroundChildHandoffReceiptV1(input)
	if err != nil || duplicate != receipt {
		t.Fatalf("same input did not produce a deterministic receipt: duplicate=%#v err=%v", duplicate, err)
	}
	clone := CloneForegroundChildHandoffReceiptV1(&receipt)
	if clone == nil || *clone != receipt || clone == &receipt || CloneForegroundChildHandoffReceiptV1(nil) != nil {
		t.Fatalf("handoff receipt clone is invalid: %#v", clone)
	}
	if err := ValidateForegroundChildHandoffReceiptForConsumptionV1(receipt, input.ConsumptionNonce, input.IssuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("valid consume-once capability was rejected: %v", err)
	}
}

func TestForegroundChildHandoffReceiptV1CaseBindingIsDigestOnlyAndCanonical(t *testing.T) {
	input := foregroundChildHandoffFixtureV1()
	binding := &ForegroundCaseHandoffBindingV1{
		Purpose:                      ForegroundCaseHandoffBindingPurposeV1,
		ParentContextDigest:          domainsecurity.SHA256Hex([]byte("parent context")),
		ChildContextDigest:           domainsecurity.SHA256Hex([]byte("child context")),
		WorkspaceScopeDigest:         domainsecurity.SHA256Hex([]byte("workspace scope")),
		CaseScopeDigest:              domainsecurity.SHA256Hex([]byte("case scope")),
		SecurityBindingDigest:        domainsecurity.SHA256Hex([]byte("security binding")),
		DelegationDigest:             domainsecurity.SHA256Hex([]byte("delegation")),
		ToolManifestDigest:           domainsecurity.SHA256Hex([]byte("tool manifest")),
		ChildExecutionGrantID:        domainsecurity.SHA256Hex([]byte("child execution grant")),
		ChildToolCallDigest:          domainsecurity.SHA256Hex([]byte("child tool call")),
		ChildCompletionReceiptDigest: domainsecurity.SHA256Hex([]byte("child completion receipt")),
		ChildAcceptedFinalDigest:     input.ChildAcceptedTerminalDigest,
		TypedResultDigest:            input.SubmissionDigest,
	}
	binding.BindingDigest = foregroundCaseHandoffBindingDigestV1(*binding)
	input.CaseBinding = binding
	input.PrivacyProjectionDigest = input.SubmissionDigest
	receipt, err := NewForegroundChildHandoffReceiptV1(input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ForegroundChildHandoffReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseForegroundChildHandoffReceiptV1(body)
	if err != nil || parsed.CaseBinding == nil || *parsed.CaseBinding != *binding {
		t.Fatalf("case binding canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	for _, forbidden := range []string{
		`"caseResult"`, `"answerSlots"`, `"claims"`, `"evidence"`, `"gaps"`,
		`"inflowMinor"`, `"outflowMinor"`, `"netMinor"`, `"reasoning"`, `"rawRow"`,
	} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("case receipt exposed private typed material %q: %s", forbidden, body)
		}
	}
	tampered := receipt
	tampered.CaseBinding = cloneForegroundCaseHandoffBindingV1(receipt.CaseBinding)
	tampered.CaseBinding.TypedResultDigest = domainsecurity.SHA256Hex([]byte("other typed result"))
	tampered.CaseBinding.BindingDigest = foregroundCaseHandoffBindingDigestV1(*tampered.CaseBinding)
	tampered.ReceiptDigest = foregroundChildHandoffReceiptDigestV1(tampered)
	if err := ValidateForegroundChildHandoffReceiptV1(tampered); err == nil {
		t.Fatal("case binding was detached from the receipt submission digest")
	}
}

func TestForegroundChildHandoffReceiptV1RejectsAmbiguousAndNonCanonicalJSON(t *testing.T) {
	receipt, err := NewForegroundChildHandoffReceiptV1(foregroundChildHandoffFixtureV1())
	if err != nil {
		t.Fatal(err)
	}
	body, err := ForegroundChildHandoffReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	unknown := strings.TrimSuffix(string(body), "}") + `,"childOutput":"private"}`
	if _, err := ParseForegroundChildHandoffReceiptV1([]byte(unknown)); err == nil {
		t.Fatal("unknown field was accepted")
	}
	duplicate := bytes.Replace(body, []byte(`"status":"accepted"`), []byte(`"status":"accepted","status":"accepted"`), 1)
	if _, err := ParseForegroundChildHandoffReceiptV1(duplicate); err == nil {
		t.Fatal("duplicate field was accepted")
	}
	if _, err := ParseForegroundChildHandoffReceiptV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("non-canonical leading whitespace was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	reordered, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(reordered, body) {
		t.Fatal("map encoding unexpectedly retained struct field order")
	}
	if _, err := ParseForegroundChildHandoffReceiptV1(reordered); err == nil {
		t.Fatal("non-canonical field order was accepted")
	}
}

func TestForegroundChildHandoffReceiptV1RejectsTamperEvenWithRecomputedDigest(t *testing.T) {
	base, err := NewForegroundChildHandoffReceiptV1(foregroundChildHandoffFixtureV1())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*ForegroundChildHandoffReceiptV1)
	}{
		{name: "non-accepted status", mutate: func(value *ForegroundChildHandoffReceiptV1) { value.Status = "completed" }},
		{name: "fact authority", mutate: func(value *ForegroundChildHandoffReceiptV1) { value.FactAnswerAllowed = true }},
		{name: "evidence authority", mutate: func(value *ForegroundChildHandoffReceiptV1) { value.EvidenceAuthority = true }},
		{name: "parent goal authority", mutate: func(value *ForegroundChildHandoffReceiptV1) { value.ParentGoalCompletionAllowed = true }},
		{name: "parent todo authority", mutate: func(value *ForegroundChildHandoffReceiptV1) { value.ParentTodoCompletionAllowed = true }},
		{name: "same thread", mutate: func(value *ForegroundChildHandoffReceiptV1) { value.ChildThreadID = value.ParentThreadID }},
		{name: "same run", mutate: func(value *ForegroundChildHandoffReceiptV1) { value.ChildRunID = value.ParentRunID }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			candidate.ReceiptDigest = foregroundChildHandoffReceiptDigestV1(candidate)
			if err := ValidateForegroundChildHandoffReceiptV1(candidate); err == nil {
				t.Fatalf("%s mutation was accepted after digest recomputation", test.name)
			}
		})
	}

	tampered := base
	tampered.SubmissionDigest = domainsecurity.SHA256Hex([]byte("other submission"))
	if err := ValidateForegroundChildHandoffReceiptV1(tampered); err == nil {
		t.Fatal("binding tamper with stale receipt digest was accepted")
	}
}

func TestForegroundChildHandoffReceiptV1ConsumptionNonceAndExpiryFailClosed(t *testing.T) {
	input := foregroundChildHandoffFixtureV1()
	receipt, err := NewForegroundChildHandoffReceiptV1(input)
	if err != nil {
		t.Fatal(err)
	}
	wrongNonce := domainsecurity.SHA256Hex([]byte("wrong nonce"))
	if err := ValidateForegroundChildHandoffReceiptForConsumptionV1(receipt, wrongNonce, input.IssuedAt.Add(time.Minute)); err == nil {
		t.Fatal("mismatched consumption nonce was accepted")
	}
	if err := ValidateForegroundChildHandoffReceiptForConsumptionV1(receipt, input.ConsumptionNonce, time.Time{}); err == nil {
		t.Fatal("missing consumption time was accepted")
	}
	if err := ValidateForegroundChildHandoffReceiptForConsumptionV1(receipt, input.ConsumptionNonce, input.IssuedAt.Add(-time.Nanosecond)); err == nil {
		t.Fatal("consumption before issuance was accepted")
	}
	if err := ValidateForegroundChildHandoffReceiptForConsumptionV1(receipt, input.ConsumptionNonce, input.ExpiresAt); err == nil {
		t.Fatal("expired handoff receipt was accepted")
	}

	overlong := input
	overlong.ExpiresAt = overlong.IssuedAt.Add(maxForegroundChildHandoffTTL + time.Nanosecond)
	if _, err := NewForegroundChildHandoffReceiptV1(overlong); err == nil {
		t.Fatal("overlong handoff capability was accepted")
	}
	zeroWindow := input
	zeroWindow.ExpiresAt = zeroWindow.IssuedAt
	if _, err := NewForegroundChildHandoffReceiptV1(zeroWindow); err == nil {
		t.Fatal("empty handoff validity window was accepted")
	}
}

func TestForegroundChildHandoffReceiptV1ContainsOnlyDigestsForPrivateMaterial(t *testing.T) {
	receipt, err := NewForegroundChildHandoffReceiptV1(foregroundChildHandoffFixtureV1())
	if err != nil {
		t.Fatal(err)
	}
	body, err := ForegroundChildHandoffReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"output"`, `"reasoning"`, `"thinking"`, `"evidence"`, `"pii"`, `"bankAccount"`,
		`"parentGoalId"`, `"parentTodoId"`, "6222020000000000000", "private child reasoning",
	} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("handoff receipt exposed forbidden material %q: %s", forbidden, body)
		}
	}
}

func foregroundChildHandoffFixtureV1() ForegroundChildHandoffReceiptInputV1 {
	issuedAt := time.Date(2026, 7, 23, 10, 0, 0, 123456789, time.UTC)
	return ForegroundChildHandoffReceiptInputV1{
		ParentThreadID: "thread-parent", ParentTurnID: "turn-parent", ParentRunID: "run-parent-7",
		ChildThreadID: "thread-child", ChildTurnID: "turn-child", ChildRunID: "job-17",
		WorkspaceRealPath: "/workspace/case-a", ParentContextEpoch: 7, ChildContextEpoch: 2,
		ParentExecutionGrantID:      domainsecurity.SHA256Hex([]byte("parent grant")),
		ParentToolCallID:            "call-delegate-17",
		ChildAcceptedTerminalDigest: domainsecurity.SHA256Hex([]byte("accepted child terminal")),
		SubmissionDigest:            domainsecurity.SHA256Hex([]byte("child submission")),
		PrivacyProjectionDigest:     domainsecurity.SHA256Hex([]byte("privacy-safe projection")),
		ConsumptionNonce:            domainsecurity.SHA256Hex([]byte("single-use nonce")),
		IssuedAt:                    issuedAt,
		ExpiresAt:                   issuedAt.Add(5 * time.Minute),
	}
}
