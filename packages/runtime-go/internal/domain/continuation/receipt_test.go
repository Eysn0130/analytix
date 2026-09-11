package continuation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestContinuationReceiptRoundTripAndTamperRejected(t *testing.T) {
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := ReceiptBytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseReceipt(body)
	if err != nil || parsed.ReceiptID != receipt.ReceiptID || !ed25519.Verify(publicKey, ReceiptSigningBytes(parsed), mustReceiptSignature(t, parsed)) {
		t.Fatalf("receipt round trip failed parsed=%#v err=%v", parsed, err)
	}
	tampered := parsed
	tampered.Payload.ProviderRouteHash = domainsecurity.SHA256Hex([]byte("tampered-route"))
	if err := ValidateReceipt(tampered); err == nil {
		t.Fatal("tampered continuation receipt must fail closed")
	}
	tampered = parsed
	tampered.Payload.ProviderNamespace.ProviderCallSequence++
	if err := ValidateReceipt(tampered); err == nil {
		t.Fatal("tampered provider continuation namespace must fail closed")
	}
	tampered = parsed
	tampered.Payload.OrdinaryWork = !tampered.Payload.OrdinaryWork
	if err := ValidateReceipt(tampered); err == nil {
		t.Fatal("tampered provider-step effect binding must fail closed")
	}
	tampered = parsed
	tampered.Payload.ProviderStepPromptSHA256 = domainsecurity.SHA256Hex([]byte("other provider step"))
	if err := ValidateReceipt(tampered); err == nil {
		t.Fatal("tampered provider-step prompt binding must fail closed")
	}
	tampered = parsed
	tampered.Payload.PrivateProtocolObserved = !tampered.Payload.PrivateProtocolObserved
	if err := ValidateReceipt(tampered); err == nil {
		t.Fatal("tampered private-protocol observation must fail closed")
	}
	tampered.ReceiptID = payloadDigest(tampered.Payload)
	if ed25519.Verify(publicKey, ReceiptSigningBytes(tampered), mustReceiptSignature(t, parsed)) {
		t.Fatal("private-protocol observation was not bound by the V3 receipt signature")
	}
	invalidRecovery := payload
	invalidRecovery.TerminalRecoveryKind = "success"
	if err := ValidatePayload(invalidRecovery); err == nil {
		t.Fatal("unknown terminal recovery kind must fail closed")
	}
	unknown := strings.TrimSuffix(string(body), "}") + `,"unknown":true}`
	if _, err := ParseReceipt([]byte(unknown)); err == nil {
		t.Fatal("unknown continuation receipt fields must be rejected")
	}
}

func TestContinuationReceiptUsesExactClosedReasoningEffortBeforeSigning(t *testing.T) {
	for _, fixture := range []struct {
		kind          string
		toolName      string
		approvalState string
	}{
		{kind: KindApproval, toolName: "write_file", approvalState: "pending"},
		{kind: KindUserInput, toolName: "request_user_input", approvalState: "not_required"},
	} {
		payload, privateKey := continuationPayloadFixture(t, fixture.kind, fixture.toolName, fixture.approvalState, time.Now().UTC())
		payload.ReasoningEffort = "auto"
		publicKey := privateKey.Public().(ed25519.PublicKey)
		signCalls := 0
		receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
			signCalls++
			return ed25519.Sign(privateKey, message), nil
		})
		if err != nil || signCalls != 1 || receipt.Payload.ReasoningEffort != "auto" {
			t.Fatalf("%s auto effort did not round trip: receipt=%#v signCalls=%d err=%v", fixture.kind, receipt, signCalls, err)
		}
	}

	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for _, effort := range []string{" high ", "HIGH", sentinel} {
		payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
		payload.ReasoningEffort = effort
		publicKey := privateKey.Public().(ed25519.PublicKey)
		signCalls := 0
		receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
			signCalls++
			return ed25519.Sign(privateKey, message), nil
		})
		if err == nil || signCalls != 0 || receipt.ReceiptID != "" || strings.Contains(err.Error(), effort) || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("invalid effort reached receipt authority: effort=%q receipt=%#v signCalls=%d err=%v", effort, receipt, signCalls, err)
		}
	}
}

func TestContinuationDispositionRoundTripAndPurposeSeparation(t *testing.T) {
	now := time.Now().UTC()
	payload, privateKey := continuationPayloadFixture(t, KindUserInput, "request_user_input", "not_required", now)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(publicKey)
	receipt, err := NewReceipt(payload, keyID, publicKey, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := NewDisposition(receipt, StatusCancelled, "user_input_cancelled", now.Add(time.Minute), keyID, publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := DispositionBytes(disposition)
	parsed, err := ParseDisposition(body)
	if err != nil || parsed.GateID != receipt.Payload.GateID {
		t.Fatalf("disposition round trip failed parsed=%#v err=%v", parsed, err)
	}
	_, _, signature, _ := DispositionAuthorityMaterial(parsed)
	if !ed25519.Verify(publicKey, DispositionSigningBytes(parsed), signature) {
		t.Fatal("disposition signature did not verify")
	}
	if ed25519.Verify(publicKey, ReceiptSigningBytes(receipt), signature) {
		t.Fatal("receipt and disposition signatures must be domain-separated")
	}
}

func TestContinuationV3UsesV3SigningDomains(t *testing.T) {
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ReceiptSigningBytes(receipt)), "analytix/gate-continuation-receipt/v3\x00") {
		t.Fatal("V3 continuation receipt did not use the V3 signing domain")
	}
	legacySigningBytes := append([]byte("analytix/gate-continuation-receipt/v1\x00"), ReceiptSigningBytes(receipt)[len(receiptSigningDomain):]...)
	if ed25519.Verify(publicKey, legacySigningBytes, mustReceiptSignature(t, receipt)) {
		t.Fatal("V3 continuation receipt verified under the legacy V1 signing domain")
	}

	disposition, err := NewDisposition(receipt, StatusDenied, "approval_denied", time.Now().UTC(), domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(DispositionSigningBytes(disposition)), "analytix/gate-continuation-disposition/v3\x00") {
		t.Fatal("V3 continuation disposition did not use the V3 signing domain")
	}
}

func TestLegacyV2ContinuationRemainsVerifiableButCannotResume(t *testing.T) {
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	payload.Version, payload.ProviderNamespace, payload.TerminalRecoveryKind = ContractVersionV2, ProviderContinuationNamespaceV1{}, TerminalRecoveryNoneV1
	payload = withoutProviderStepBinding(payload)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt := Receipt{
		Version: ContractVersionV2, Payload: payload, ReceiptID: payloadDigest(payload), AuthorityAlgorithm: AuthorityAlgorithm,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, ReceiptSigningBytes(receipt)))
	body, err := ReceiptBytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseReceipt(body)
	if err != nil || !ed25519.Verify(publicKey, ReceiptSigningBytes(parsed), mustReceiptSignature(t, parsed)) {
		t.Fatalf("legacy V2 continuation lost audit verification: err=%v", err)
	}
	if err := ValidateReceiptForExecution(parsed); err == nil {
		t.Fatal("legacy V2 continuation authorized provider resume")
	}
	injected := parsed
	injected.Payload.PrivateProtocolObserved = true
	if err := ValidateReceipt(injected); err == nil {
		t.Fatal("legacy V2 continuation accepted an unsigned private-protocol observation")
	}
}

func TestPreBindingV3ContinuationRemainsAuditOnlyAndByteStable(t *testing.T) {
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	payload = withoutProviderStepBinding(payload)
	if err := ValidatePayload(payload); err != nil {
		t.Fatalf("pre-binding V3 payload lost audit readability: %v", err)
	}
	if HasExactProviderStepBinding(payload) || ValidatePayloadForExecution(payload) == nil {
		t.Fatal("pre-binding V3 payload regained execution authority")
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt := Receipt{
		Version: ContractVersionV3, Payload: payload, ReceiptID: payloadDigest(payload), AuthorityAlgorithm: AuthorityAlgorithm,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, ReceiptSigningBytes(receipt)))
	body, err := ReceiptBytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"logicalEffect", "ordinaryWork", "providerStepExact", "caseSourceUnavailable",
		"ordinaryResultInputIsolated", "providerStepPromptSha256",
	} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("pre-binding V3 wire unexpectedly gained %q: %s", forbidden, body)
		}
	}
	parsed, err := ParseReceipt(body)
	if err != nil || parsed.ReceiptID != receipt.ReceiptID ||
		!ed25519.Verify(publicKey, ReceiptSigningBytes(parsed), mustReceiptSignature(t, parsed)) {
		t.Fatalf("pre-binding V3 receipt lost audit verification: parsed=%#v err=%v", parsed, err)
	}
	if err := ValidateReceiptForExecution(parsed); err == nil {
		t.Fatal("pre-binding V3 receipt authorized provider resume")
	}
}

func TestProviderStepBindingRejectsPartialInvalidAndLegacyFields(t *testing.T) {
	base, _ := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	if !HasExactProviderStepBinding(base) || ValidatePayloadForExecution(base) != nil {
		t.Fatal("complete provider-step binding was not executable")
	}
	legacy := withoutProviderStepBinding(base)
	observedWithoutBinding := legacy
	observedWithoutBinding.PrivateProtocolObserved = true
	if err := ValidatePayload(observedWithoutBinding); err == nil {
		t.Fatal("private-protocol observation without provider-step authority was accepted")
	}
	partials := []Payload{
		func() Payload {
			value := legacy
			value.LogicalEffect = domainsecurity.LogicalEffectOrdinary
			return value
		}(),
		func() Payload { value := legacy; value.OrdinaryWork = true; return value }(),
		func() Payload { value := legacy; value.ProviderStepExact = true; return value }(),
		func() Payload { value := legacy; value.CaseSourceUnavailable = true; return value }(),
		func() Payload { value := legacy; value.OrdinaryResultInputIsolated = true; return value }(),
		func() Payload {
			value := legacy
			value.ProviderStepPromptSHA256 = domainsecurity.SHA256Hex([]byte("prompt"))
			return value
		}(),
	}
	for index, partial := range partials {
		if err := ValidatePayload(partial); err == nil {
			t.Fatalf("partial provider-step binding %d was accepted", index)
		}
	}

	invalid := base
	invalid.ProviderStepExact = false
	if err := ValidatePayload(invalid); err == nil {
		t.Fatal("provider step without exact provenance was accepted")
	}
	invalid = base
	invalid.LogicalEffect = "unknown"
	if err := ValidatePayload(invalid); err == nil {
		t.Fatal("provider step with unknown logical effect was accepted")
	}
	invalid = base
	invalid.ProviderStepPromptSHA256 = domainsecurity.SHA256Hex(nil)
	if err := ValidatePayload(invalid); err == nil {
		t.Fatal("provider step with an empty prompt digest was accepted")
	}
	invalid = base
	invalid.LogicalEffect = domainsecurity.LogicalEffectFundsData
	invalid.OrdinaryResultInputIsolated = true
	if err := ValidatePayload(invalid); err == nil {
		t.Fatal("protected provider input acquired ordinary-only isolation provenance")
	}
	invalid = base
	invalid.OrdinaryWork = false
	invalid.OrdinaryResultInputIsolated = false
	if err := ValidatePayload(invalid); err == nil {
		t.Fatal("ordinary provider step without ordinary work was accepted")
	}
	invalid = base
	invalid.LogicalEffect = domainsecurity.LogicalEffectFundsData
	invalid.CaseSourceUnavailable = true
	invalid.OrdinaryResultInputIsolated = false
	if err := ValidatePayload(invalid); err == nil {
		t.Fatal("protected provider step claimed unavailable-source downgrade provenance")
	}

	legacyV2 := base
	legacyV2.Version = ContractVersionV2
	legacyV2.ProviderNamespace = ProviderContinuationNamespaceV1{}
	legacyV2.TerminalRecoveryKind = TerminalRecoveryNoneV1
	if err := ValidatePayload(legacyV2); err == nil {
		t.Fatal("legacy V2 payload accepted V3 provider-step fields")
	}
}

func TestProviderStepPromptBindingStoresOnlyTrimmedDigest(t *testing.T) {
	const prompt = "  SOL_PRIVATE_PROVIDER_PROMPT_7F3C  "
	digest := CanonicalProviderStepPromptSHA256(prompt)
	if digest != domainsecurity.SHA256Hex([]byte(strings.TrimSpace(prompt))) || !domainsecurity.IsSHA256Hex(digest) {
		t.Fatalf("provider-step prompt digest mismatch: %q", digest)
	}
	if CanonicalProviderStepPromptSHA256(" \n\t ") != "" {
		t.Fatal("empty trimmed provider step acquired a digest binding")
	}
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	payload.ProviderStepPromptSHA256 = digest
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := ReceiptBytes(receipt)
	if err != nil || bytes.Contains(body, []byte(strings.TrimSpace(prompt))) || !bytes.Contains(body, []byte(digest)) {
		t.Fatalf("receipt leaked prompt text or lost its digest: body=%s err=%v", body, err)
	}
}

func TestCanonicalPrivateToolArgumentsV2PreservesValueAndGrantHash(t *testing.T) {
	raw := json.RawMessage(`{"path":"out.txt","content":"blocked"}`)
	canonical, err := CanonicalPrivateToolArgumentsV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != `{"content":"blocked","path":"out.txt"}` {
		t.Fatalf("canonical private arguments = %s", canonical)
	}
	if domainsecurity.CanonicalJSONHash(raw) != domainsecurity.CanonicalJSONHash(canonical) {
		t.Fatal("canonical private arguments changed the execution-grant value hash")
	}
}

func TestContinuationPayloadRejectsScopeAndGateKindMismatch(t *testing.T) {
	payload, _ := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	payload.ToolScope = []string{"read_file"}
	if err := ValidatePayload(payload); err == nil {
		t.Fatal("scope mismatch must be rejected")
	}
	payload, _ = continuationPayloadFixture(t, KindUserInput, "write_file", "not_required", time.Now().UTC())
	if err := ValidatePayload(payload); err == nil {
		t.Fatal("user-input receipt cannot bind an arbitrary tool")
	}
	payload, _ = continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	payload.PriorSettledToolRefs = []domainsecurity.SettledToolReference{{
		GrantID: domainsecurity.SHA256Hex([]byte("prior-grant")), ResultItemID: "item_result_prior_turn_call",
	}}
	if err := ValidatePayload(payload); err != nil {
		t.Fatalf("valid private prior settlement reference rejected: %v", err)
	}
	payload.PriorSettledToolRefs[0].ResultItemID = "synthetic_pairing_result"
	if err := ValidatePayload(payload); err == nil {
		t.Fatal("synthetic provider pairing result entered continuation authority")
	}
}

func TestV1ContinuationIsAuditOnlyAndCannotBeIssuedForResume(t *testing.T) {
	now := time.Now().UTC()
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", now)
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: payload.ThreadID, TurnID: payload.TurnID, WorkspaceRealPath: payload.SecurityContext.WorkspaceRealPath,
		ContextEpoch: payload.SecurityContext.ContextEpoch, IssuedAt: now,
	})
	payload.SecurityContext = legacy
	payload.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: legacy, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: payload.ToolName,
		ToolCallID: payload.CallID, ArgsHash: domainsecurity.CanonicalJSONHash([]byte(`{"path":"a.txt"}`)),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: payload.ExecutionGrant.ScopeHash,
		ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	payload.GateID = GateID(payload.Kind, payload.ThreadID, payload.TurnID, legacy.ContextDigest, payload.ExecutionGrant.GrantID, payload.CallID)
	payload.ItemID = "item_" + payload.GateID
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if err := ValidatePayload(payload); err != nil {
		t.Fatalf("structurally valid V1 continuation must remain audit-parseable: %v", err)
	}
	if _, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil {
		t.Fatal("audit-only V1 continuation issued a resumable receipt")
	}
}

func TestRawProviderIdentityContinuationRemainsAuditableButCannotResume(t *testing.T) {
	now := time.Now().UTC()
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", now)
	rawProviderID := "provider_call_6222020202020202020"
	payload.CallID = rawProviderID
	payload.ToolCallItemID = "item_tool_" + payload.TurnID + "_" + rawProviderID
	payload.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: payload.SecurityContext, Provider: payload.ProviderID, ServerIdentity: payload.ExecutionGrant.ServerIdentity,
		ToolName: payload.ToolName, ToolCallID: rawProviderID, ConnectionEpoch: payload.ExecutionGrant.ConnectionEpoch,
		ArgsHash: payload.ExecutionGrant.ArgsHash, SchemaHash: payload.ExecutionGrant.SchemaHash, ScopeHash: payload.ExecutionGrant.ScopeHash,
		ReadOnly: payload.ExecutionGrant.ReadOnly, ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	payload.GateID = GateID(payload.Kind, payload.ThreadID, payload.TurnID, payload.SecurityContext.ContextDigest, payload.ExecutionGrant.GrantID, rawProviderID)
	payload.ItemID = "item_" + payload.GateID
	if err := ValidatePayload(payload); err != nil {
		t.Fatalf("historical raw-ID continuation lost audit readability: %v", err)
	}
	if err := ValidatePayloadForExecution(payload); err == nil || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("historical raw-ID continuation regained execution authority: %v", err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil || receipt.ReceiptID != "" || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("new resumable receipt accepted raw provider identity: receipt=%#v err=%v", receipt, err)
	}
}

func TestContinuationRequiresExactCanonicalToolCallItemIdentity(t *testing.T) {
	payload, _ := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	otherCallID, err := domainsecurity.NewHostToolCallIDV1(bytes.Repeat([]byte{0x64}, domainsecurity.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	payload.ToolCallItemID = domaintoolcall.ToolCallItemIDV1(payload.TurnID, otherCallID)
	if err := ValidatePayload(payload); err != nil {
		t.Fatalf("mismatched current item fixture is not structurally auditable: %v", err)
	}
	if err := ValidatePayloadForExecution(payload); err == nil {
		t.Fatal("continuation accepted a canonical item identity from another tool call")
	}
}

func continuationPayloadFixture(t *testing.T, kind, toolName, approvalState string, now time.Time) (Payload, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = publicKey
	context, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")),
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := []string{toolName}
	scopeBody, _ := json.Marshal(scope)
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(bytes.Repeat([]byte{0x63}, domainsecurity.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: toolName, ToolCallID: toolCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash([]byte(`{"path":"a.txt"}`)), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scopeBody), ReadOnly: false, ApprovalState: approvalState, IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	gateID := GateID(kind, context.ThreadID, context.TurnID, context.ContextDigest, grant.GrantID, grant.ToolCallID)
	return Payload{
		Version: ContractVersion, Kind: kind, GateID: gateID, ThreadID: context.ThreadID, TurnID: context.TurnID, ItemID: "item_" + gateID,
		CallID: grant.ToolCallID, ToolName: toolName, ToolCallItemID: domaintoolcall.ToolCallItemIDV1(context.TurnID, grant.ToolCallID), Arguments: json.RawMessage(`{"path":"a.txt"}`),
		ProviderID: grant.Provider, Model: "model-a",
		ProviderRouteHash: domainsecurity.SHA256Hex([]byte("route")), ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		ToolScope: scope, ProviderNamespace: NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		OrdinaryResultInputIsolated: true, ProviderStepPromptSHA256: CanonicalProviderStepPromptSHA256("continue exact provider step"),
		SecurityContext: context, ExecutionGrant: grant, IssuedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	}, privateKey
}

func withoutProviderStepBinding(payload Payload) Payload {
	payload.LogicalEffect = ""
	payload.OrdinaryWork = false
	payload.ProviderStepExact = false
	payload.CaseSourceUnavailable = false
	payload.OrdinaryResultInputIsolated = false
	payload.ProviderStepPromptSHA256 = ""
	return payload
}

func mustReceiptSignature(t *testing.T, receipt Receipt) []byte {
	t.Helper()
	_, _, signature, err := AuthorityMaterial(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return signature
}
