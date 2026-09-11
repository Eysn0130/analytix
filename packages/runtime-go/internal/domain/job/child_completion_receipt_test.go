package job

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type childCompletionReceiptFixture struct {
	input         ChildCompletionReceiptInputV1
	parentContext domainsecurity.TurnSecurityContext
	childContext  domainsecurity.TurnSecurityContext
	binding       *SecurityBinding
	acceptedFinal domainevidence.AcceptedFinalRecord
	privateKey    ed25519.PrivateKey
}

func TestChildCompletionReceiptV1CanonicalRoundTripAndExactBindings(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	receipt := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)

	if !receipt.OutputWithheld || receipt.CanReadOutput || receipt.FactAnswerAllowed || receipt.EvidenceAuthority ||
		receipt.ParentGoalCompletionAllowed || !receipt.CanContinueParent {
		t.Fatalf("child completion safety disposition is invalid: %#v", receipt)
	}
	if receipt.ParentContext.ContextEpoch == receipt.ChildContext.ContextEpoch {
		t.Fatal("test fixture did not exercise independent thread-local epochs")
	}
	if !ChildCompletionContextRefsShareCaseScopeV1(receipt.ParentContext, receipt.ChildContext) ||
		!ChildCompletionContextRefV1MatchesContext(receipt.ParentContext, fixture.parentContext) ||
		!ChildCompletionContextRefV1MatchesContext(receipt.ChildContext, fixture.childContext) {
		t.Fatalf("receipt did not bind the exact parent/child contexts: %#v", receipt)
	}
	if receipt.SecurityBindingDigest != fixture.binding.BindingDigest || receipt.AcceptedFinalDigest != fixture.acceptedFinal.RecordDigest ||
		receipt.PrivateRecordDigest != fixture.acceptedFinal.PrivateRecordDigest || receipt.EnvelopeDigest != fixture.acceptedFinal.EnvelopeDigest {
		t.Fatalf("receipt did not bind exact job/final records: %#v", receipt)
	}
	if receipt.ParentExecutionGrantID != fixture.binding.ParentExecutionGrantID ||
		receipt.ParentToolCallID != fixture.binding.ParentToolCallID ||
		receipt.FinalVariant != fixture.acceptedFinal.Variant || receipt.TerminalReason != fixture.acceptedFinal.TerminalReason {
		t.Fatalf("receipt did not bind exact parent execution and final classification: %#v", receipt)
	}
	if err := ValidateChildCompletionReceiptForBindingsV1(
		receipt, fixture.parentContext, fixture.childContext, fixture.binding, fixture.acceptedFinal,
	); err != nil {
		t.Fatal(err)
	}
	body, err := ChildCompletionReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseChildCompletionReceiptV1(body)
	if err != nil || parsed != receipt {
		t.Fatalf("canonical receipt round trip failed: parsed=%#v err=%v", parsed, err)
	}
	keyID, publicKey, signature, err := ChildCompletionReceiptV1AuthorityMaterial(parsed)
	if err != nil || keyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(publicKey, ChildCompletionReceiptV1SigningBytes(parsed), signature) {
		t.Fatalf("receipt signature material failed: keyID=%s err=%v", keyID, err)
	}
	duplicate := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)
	if duplicate != receipt {
		t.Fatalf("identical canonical input produced a different receipt:\nfirst=%#v\nsecond=%#v", receipt, duplicate)
	}
	tamperedDigest := receipt
	tamperedDigest.ReceiptDigest = domainsecurity.SHA256Hex([]byte("other receipt"))
	if err := ValidateChildCompletionReceiptV1(tamperedDigest); err == nil {
		t.Fatal("tampered receipt digest was accepted")
	}
	tamperedFinal := receipt
	tamperedFinal.AcceptedFinalDigest = domainsecurity.SHA256Hex([]byte("other final"))
	if err := ValidateChildCompletionReceiptV1(tamperedFinal); err == nil {
		t.Fatal("tampered accepted-final binding was accepted")
	}
}

func TestChildCompletionReceiptV1RejectsUnknownDuplicateAndNonCanonicalJSON(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	receipt := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)
	body, err := ChildCompletionReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}

	unknownTop := strings.TrimSuffix(string(body), "}") + `,"rawOutput":"6222020000000000000"}`
	if _, err := ParseChildCompletionReceiptV1([]byte(unknownTop)); err == nil {
		t.Fatal("unknown top-level receipt property was accepted")
	}
	unknownNested := bytes.Replace(body, []byte(`"parentContext":{`), []byte(`"parentContext":{"safeToAnswer":true,`), 1)
	if _, err := ParseChildCompletionReceiptV1(unknownNested); err == nil {
		t.Fatal("unknown nested context property was accepted")
	}
	duplicateNested := bytes.Replace(body, []byte(`"threadId":"thread-parent"`), []byte(`"threadId":"thread-parent","threadId":"thread-attacker"`), 1)
	if _, err := ParseChildCompletionReceiptV1(duplicateNested); err == nil {
		t.Fatal("duplicate nested context property was accepted")
	}
	if _, err := ParseChildCompletionReceiptV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("leading non-canonical whitespace was accepted")
	}
	if _, err := ParseChildCompletionReceiptV1(append(body, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	var reordered map[string]any
	if err := json.Unmarshal(body, &reordered); err != nil {
		t.Fatal(err)
	}
	reorderedBody, _ := json.Marshal(reordered)
	if bytes.Equal(reorderedBody, body) {
		t.Fatal("test fixture did not reorder canonical struct properties")
	}
	if _, err := ParseChildCompletionReceiptV1(reorderedBody); err == nil {
		t.Fatal("non-canonical property order was accepted")
	}
}

func TestChildCompletionReceiptV1FixedSafetyDispositionCannotBeResignedAway(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	base := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)
	largeCanonicalText := strings.Repeat("a", maxChildCompletionTextBytes-1)
	tests := []struct {
		name   string
		mutate func(*ChildCompletionReceiptV1)
	}{
		{name: "output exposed", mutate: func(receipt *ChildCompletionReceiptV1) { receipt.OutputWithheld = false }},
		{name: "output readable", mutate: func(receipt *ChildCompletionReceiptV1) { receipt.CanReadOutput = true }},
		{name: "fact authority", mutate: func(receipt *ChildCompletionReceiptV1) { receipt.FactAnswerAllowed = true }},
		{name: "evidence authority", mutate: func(receipt *ChildCompletionReceiptV1) { receipt.EvidenceAuthority = true }},
		{name: "parent goal completion", mutate: func(receipt *ChildCompletionReceiptV1) { receipt.ParentGoalCompletionAllowed = true }},
		{name: "evidence-backed final continuation", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.FinalVariant = domainevidence.EvidenceBackedAnswer
			receipt.CanContinueParent = true
		}},
		{name: "partial final continuation", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.FinalVariant = domainevidence.PartialEvidenceAnswer
			receipt.CanContinueParent = true
		}},
		{name: "verified no-hit final continuation", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.FinalVariant = domainevidence.VerifiedNoHitAnswer
			receipt.CanContinueParent = true
		}},
		{name: "shared case mismatch", mutate: func(receipt *ChildCompletionReceiptV1) { receipt.ChildContext.CaseID = "case-other" }},
		{name: "shared snapshot mismatch", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.ChildContext.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID("snapshot-other")
		}},
		{name: "same parent and child thread", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.ChildContext.ThreadID = receipt.ParentContext.ThreadID
		}},
		{name: "uppercase digest", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.EnvelopeDigest = strings.ToUpper(receipt.EnvelopeDigest)
		}},
		{name: "canonical size bound", mutate: func(receipt *ChildCompletionReceiptV1) {
			for _, context := range []*ChildCompletionContextRefV1{&receipt.ParentContext, &receipt.ChildContext} {
				context.WorkspaceRealPath = largeCanonicalText
				context.TenantID = largeCanonicalText
				context.UserID = largeCanonicalText
				context.CaseID = largeCanonicalText
				context.DatasetSnapshotID = largeCanonicalText
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := base
			test.mutate(&receipt)
			resignChildCompletionReceiptV1(&receipt, fixture.privateKey)
			if err := ValidateChildCompletionReceiptV1(receipt); err == nil {
				t.Fatalf("authentically resigned unsafe receipt was accepted: %#v", receipt)
			}
		})
	}

	nonContinuable := base
	nonContinuable.CanContinueParent = false
	resignChildCompletionReceiptV1(&nonContinuable, fixture.privateKey)
	if err := ValidateChildCompletionReceiptV1(nonContinuable); err != nil {
		t.Fatalf("auditable non-continuable receipt was rejected: %v", err)
	}
}

func TestNewChildCompletionReceiptV1RequiresExactFinalBindingAndCaseScope(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	for _, invalidID := range []string{"job-0", "job-01", "job-../17", "job-18446744073709551615", "17"} {
		invalidRun := fixture.input
		invalidRun.ChildRunID = invalidID
		if _, err := NewChildCompletionReceiptV1(invalidRun, childCompletionSigner(fixture.privateKey)); err == nil {
			t.Fatalf("invalid durable child run id %q was accepted", invalidID)
		}
	}

	otherChild := childCompletionCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child-other", TurnID: "turn-child-other", WorkspaceRealPath: fixture.childContext.WorkspaceRealPath,
		TenantID: fixture.childContext.TenantID, UserID: fixture.childContext.UserID, CaseID: fixture.childContext.CaseID,
		CaseBindingHash: fixture.childContext.CaseBindingHash, DatasetSnapshotID: fixture.childContext.DatasetSnapshotID,
		SourceManifestHash: fixture.childContext.SourceManifestHash, ContextEpoch: 19, IssuedAt: fixture.input.IssuedAt.Add(-time.Minute),
	})
	otherFinal := childCompletionAcceptedFinal(t, otherChild, fixture.input.IssuedAt.Add(-30*time.Second), fixture.privateKey)
	mismatchedFinal := fixture.input
	mismatchedFinal.AcceptedFinal = otherFinal
	if _, err := NewChildCompletionReceiptV1(mismatchedFinal, childCompletionSigner(fixture.privateKey)); err == nil {
		t.Fatal("accepted final from another child context was accepted")
	}

	otherParent := childCompletionCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-parent-other", TurnID: "turn-parent-other", WorkspaceRealPath: fixture.parentContext.WorkspaceRealPath,
		TenantID: fixture.parentContext.TenantID, UserID: fixture.parentContext.UserID, CaseID: fixture.parentContext.CaseID,
		CaseBindingHash: fixture.parentContext.CaseBindingHash, DatasetSnapshotID: fixture.parentContext.DatasetSnapshotID,
		SourceManifestHash: fixture.parentContext.SourceManifestHash, ContextEpoch: 8, IssuedAt: fixture.input.IssuedAt.Add(-2 * time.Minute),
	})
	otherBinding := childCompletionSecurityBinding(t, otherParent, "call-other")
	mismatchedBinding := fixture.input
	mismatchedBinding.SecurityBinding = otherBinding
	if _, err := NewChildCompletionReceiptV1(mismatchedBinding, childCompletionSigner(fixture.privateKey)); err == nil {
		t.Fatal("security binding from another parent context was accepted")
	}

	crossCaseChild := childCompletionCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child-case-b", TurnID: "turn-child-case-b", WorkspaceRealPath: fixture.childContext.WorkspaceRealPath,
		TenantID: fixture.childContext.TenantID, UserID: fixture.childContext.UserID, CaseID: "case-b",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("case-binding-b")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-b"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest-b")), ContextEpoch: 1,
		IssuedAt: fixture.input.IssuedAt.Add(-time.Minute),
	})
	crossCase := fixture.input
	crossCase.ChildContext = crossCaseChild
	crossCase.AcceptedFinal = childCompletionAcceptedFinal(t, crossCaseChild, fixture.input.IssuedAt.Add(-30*time.Second), fixture.privateKey)
	if _, err := NewChildCompletionReceiptV1(crossCase, childCompletionSigner(fixture.privateKey)); err == nil {
		t.Fatal("cross-case child completion was accepted")
	}

	sameThreadChild := childCompletionCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.parentContext.ThreadID, TurnID: "turn-child-same-thread", WorkspaceRealPath: fixture.childContext.WorkspaceRealPath,
		TenantID: fixture.childContext.TenantID, UserID: fixture.childContext.UserID, CaseID: fixture.childContext.CaseID,
		CaseBindingHash: fixture.childContext.CaseBindingHash, DatasetSnapshotID: fixture.childContext.DatasetSnapshotID,
		SourceManifestHash: fixture.childContext.SourceManifestHash, ContextEpoch: 20, IssuedAt: fixture.input.IssuedAt.Add(-time.Minute),
	})
	sameThread := fixture.input
	sameThread.ChildContext = sameThreadChild
	sameThread.AcceptedFinal = childCompletionAcceptedFinal(t, sameThreadChild, fixture.input.IssuedAt.Add(-30*time.Second), fixture.privateKey)
	if _, err := NewChildCompletionReceiptV1(sameThread, childCompletionSigner(fixture.privateKey)); err == nil {
		t.Fatal("same-thread parent and child completion was accepted")
	}

	predatesFinal := fixture.input
	acceptedAt, _ := time.Parse(time.RFC3339Nano, fixture.acceptedFinal.AcceptedAt)
	predatesFinal.IssuedAt = acceptedAt.Add(-time.Nanosecond)
	if _, err := NewChildCompletionReceiptV1(predatesFinal, childCompletionSigner(fixture.privateKey)); err == nil {
		t.Fatal("receipt predating the accepted final was accepted")
	}

	receipt := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)
	if err := ValidateChildCompletionReceiptForBindingsV1(
		receipt, fixture.parentContext, fixture.childContext, fixture.binding, otherFinal,
	); err == nil {
		t.Fatal("valid receipt was rebound to another accepted final")
	}
}

func TestValidateChildCompletionReceiptForBindingsV1RejectsResignedAuthorityMetadataTamper(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	base := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)
	tests := []struct {
		name   string
		mutate func(*ChildCompletionReceiptV1)
	}{
		{name: "parent execution grant", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.ParentExecutionGrantID = domainsecurity.SHA256Hex([]byte("other-parent-grant"))
		}},
		{name: "parent tool call", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.ParentToolCallID = "call-other"
		}},
		{name: "final variant", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.FinalVariant = domainevidence.NeedsEvidenceAnswer
		}},
		{name: "terminal reason", mutate: func(receipt *ChildCompletionReceiptV1) {
			receipt.TerminalReason = "recovery"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := base
			test.mutate(&receipt)
			resignChildCompletionReceiptV1(&receipt, fixture.privateKey)
			if err := ValidateChildCompletionReceiptV1(receipt); err != nil {
				t.Fatalf("test metadata mutation was not structurally authentic: %v", err)
			}
			if err := ValidateChildCompletionReceiptForBindingsV1(
				receipt, fixture.parentContext, fixture.childContext, fixture.binding, fixture.acceptedFinal,
			); err == nil {
				t.Fatalf("authentically resigned %s mutation matched original authority", test.name)
			}
		})
	}
}

func TestChildCompletionReceiptV1ContainsNoRawOutputReasoningOrPIIPayload(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	receipt := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)
	body, err := ChildCompletionReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbiddenField := range []string{
		`"output"`, `"rawOutput"`, `"outputText"`, `"reasoning"`, `"thinking"`, `"claims"`,
		`"evidence"`, `"renderedText"`, `"pii"`, `"bankAccount"`, `"cardNumber"`, `"parentGoalId"`,
	} {
		if bytes.Contains(body, []byte(forbiddenField)) {
			t.Fatalf("child completion receipt exposed forbidden field %s: %s", forbiddenField, body)
		}
	}
	for _, forbiddenValue := range []string{
		"6222020000000000000", "private child reasoning", "raw provider output", "model-authored case fact",
	} {
		if bytes.Contains(body, []byte(forbiddenValue)) {
			t.Fatalf("child completion receipt exposed forbidden value %q: %s", forbiddenValue, body)
		}
	}
}

func TestChildCompletionReceiptV1SignatureIsPurposeSeparatedFromAcceptedFinal(t *testing.T) {
	fixture := newChildCompletionReceiptFixture(t)
	receipt := mustChildCompletionReceiptV1(t, fixture.input, fixture.privateKey)
	_, publicKey, receiptSignature, err := ChildCompletionReceiptV1AuthorityMaterial(receipt)
	if err != nil {
		t.Fatal(err)
	}
	_, acceptedFinalPublicKey, acceptedFinalSignature, err := domainevidence.AcceptedFinalAuthorityMaterial(fixture.acceptedFinal)
	if err != nil || !bytes.Equal(publicKey, acceptedFinalPublicKey) {
		t.Fatalf("test fixture did not use one key for both domains: %v", err)
	}
	if ed25519.Verify(publicKey, domainevidence.AcceptedFinalSigningBytes(fixture.acceptedFinal), receiptSignature) {
		t.Fatal("child completion signature validated as an accepted-final signature")
	}
	if ed25519.Verify(publicKey, ChildCompletionReceiptV1SigningBytes(receipt), acceptedFinalSignature) {
		t.Fatal("accepted-final signature validated as a child-completion signature")
	}
}

func newChildCompletionReceiptFixture(t *testing.T) childCompletionReceiptFixture {
	t.Helper()
	now := time.Date(2026, 7, 13, 14, 0, 0, 123, time.UTC)
	shared := domainsecurity.TurnSecurityContextInput{
		WorkspaceRealPath: "/workspace/case-a", TenantID: "tenant-a", UserID: "user-a", CaseID: "case-a",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("case-binding-a")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest-a")),
	}
	parentInput := shared
	parentInput.ThreadID = "thread-parent"
	parentInput.TurnID = "turn-parent"
	parentInput.ContextEpoch = 7
	parentInput.IssuedAt = now.Add(-2 * time.Minute)
	parentContext := childCompletionCaseContextV2(t, parentInput)
	childInput := shared
	childInput.ThreadID = "thread-child"
	childInput.TurnID = "turn-child"
	childInput.ContextEpoch = 3
	childInput.IssuedAt = now.Add(-time.Minute)
	childContext := childCompletionCaseContextV2(t, childInput)
	binding := childCompletionSecurityBinding(t, parentContext, "call-child")
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x43}, ed25519.SeedSize))
	acceptedFinal := childCompletionAcceptedFinal(t, childContext, now.Add(-time.Second), privateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	input := ChildCompletionReceiptInputV1{
		ChildRunID: "job-17", SecurityBinding: binding, ParentContext: parentContext, ChildContext: childContext,
		AcceptedFinal: acceptedFinal, CanContinueParent: true, IssuedAt: now,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}
	return childCompletionReceiptFixture{
		input: input, parentContext: parentContext, childContext: childContext,
		binding: binding, acceptedFinal: acceptedFinal, privateKey: privateKey,
	}
}

func childCompletionCaseContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	context, err := securitycontexttest.CaseExecutionContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func childCompletionSecurityBinding(t *testing.T, context domainsecurity.TurnSecurityContext, callID string) *SecurityBinding {
	t.Helper()
	callID = jobTestHostToolCallID(callID)
	contextIssuedAt, err := time.Parse(time.RFC3339Nano, context.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args:" + callID)), SchemaHash: domainsecurity.SHA256Hex([]byte("schema:task")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope:" + callID)), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: contextIssuedAt.Add(time.Second),
	})
	binding, err := NewSecurityBinding(context, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func childCompletionAcceptedFinal(
	t *testing.T,
	context domainsecurity.TurnSecurityContext,
	acceptedAt time.Time,
	privateKey ed25519.PrivateKey,
) domainevidence.AcceptedFinalRecord {
	t.Helper()
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: context, TerminalReason: "source_unavailable",
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: acceptedAt.Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(context)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: acceptedAt.Add(-time.Second).Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(context, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	record, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: context, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: acceptedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func childCompletionSigner(privateKey ed25519.PrivateKey) ChildCompletionReceiptSignFuncV1 {
	return func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}
}

func mustChildCompletionReceiptV1(t *testing.T, input ChildCompletionReceiptInputV1, privateKey ed25519.PrivateKey) ChildCompletionReceiptV1 {
	t.Helper()
	receipt, err := NewChildCompletionReceiptV1(input, childCompletionSigner(privateKey))
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func resignChildCompletionReceiptV1(receipt *ChildCompletionReceiptV1, privateKey ed25519.PrivateKey) {
	receipt.AuthoritySignature = ""
	receipt.ReceiptDigest = ""
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, ChildCompletionReceiptV1SigningBytes(*receipt)))
	receipt.ReceiptDigest = childCompletionReceiptDigestV1(*receipt)
}
