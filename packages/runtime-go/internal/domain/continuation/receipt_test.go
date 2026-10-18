package continuation

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
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

func TestContinuationReceiptSignsExactOptionalModelVisibleDefinitionV1(t *testing.T) {
	base, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	for _, definition := range []*domainmodel.ToolDefinitionSnapshotV1{
		nil,
		{SchemaJSON: `{"name":"write_file","description":"ACTUAL_REQUEST","parameters":{"type":"object"}}`},
		{Truncated: true},
	} {
		// Old nil and both new observations use the same authority key and
		// original payload; adding display facts does not rotate authority.
		payload := base
		payload.ModelVisibleToolDefinition = definition
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
		if err != nil || !ed25519.Verify(publicKey, ReceiptSigningBytes(parsed), mustReceiptSignature(t, parsed)) || parsed.Payload.ExecutionGrant.SchemaHash != payload.ExecutionGrant.SchemaHash {
			t.Fatal("optional display definition changed grant authority or lost its signature", err)
		}
		roundTrip, err := ReceiptBytes(parsed)
		if err != nil || !bytes.Equal(roundTrip, body) {
			t.Fatal("signed receipt bytes changed after parsing")
		}
		if definition == nil {
			if parsed.Payload.ModelVisibleToolDefinition != nil || bytes.Contains(body, []byte("modelVisibleToolDefinition")) {
				t.Fatal("old nil definition changed the wire")
			}
			continue
		}
		if parsed.Payload.ModelVisibleToolDefinition == nil || *parsed.Payload.ModelVisibleToolDefinition != *definition {
			t.Fatal("original definition was not retained")
		}
		definition.SchemaJSON, definition.Truncated = `{"name":"write_file","description":"LATER_CATALOG","parameters":{}}`, false
		if *receipt.Payload.ModelVisibleToolDefinition == *definition {
			t.Fatal("receipt kept the producer's mutable pointer")
		}
		tampered := parsed
		tampered.Payload.ModelVisibleToolDefinition = domainmodel.CloneToolDefinitionSnapshotV1(definition)
		if ValidateReceipt(tampered) == nil {
			t.Fatal("changed display definition reused the receipt digest")
		}
		tampered.ReceiptID = payloadDigest(tampered.Payload)
		if ed25519.Verify(publicKey, ReceiptSigningBytes(tampered), mustReceiptSignature(t, parsed)) {
			t.Fatal("model-visible definition was not signed")
		}
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
	currentPayload := payload
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
	if bytes.Contains(body, []byte("modelVisibleToolDefinition")) {
		t.Fatal("legacy nil definition changed signed wire bytes")
	}
	parsed, err := ParseReceipt(body)
	if err != nil || !ed25519.Verify(publicKey, ReceiptSigningBytes(parsed), mustReceiptSignature(t, parsed)) {
		t.Fatalf("legacy V2 continuation lost audit verification: err=%v", err)
	}
	currentPayload.ModelVisibleToolDefinition = &domainmodel.ToolDefinitionSnapshotV1{SchemaJSON: `{"name":"write_file","description":"ACTUAL_REQUEST","parameters":{}}`}
	current, err := NewReceipt(currentPayload, parsed.AuthorityKeyID, publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil || current.AuthorityKeyID != parsed.AuthorityKeyID || !ed25519.Verify(publicKey, ReceiptSigningBytes(current), mustReceiptSignature(t, current)) {
		t.Fatal("old V2 and new V3 definition did not verify under the same existing authority key", err)
	}
	if err := ValidateReceiptForExecution(parsed); err == nil {
		t.Fatal("legacy V2 continuation authorized provider resume")
	}
	if parsed.Payload.ModelVisibleToolDefinition != nil {
		t.Fatal("legacy receipt acquired a current definition")
	}
	withDefinition := parsed
	withDefinition.Payload.ModelVisibleToolDefinition = &domainmodel.ToolDefinitionSnapshotV1{SchemaJSON: `{"name":"write_file","description":"new","parameters":{}}`}
	if ValidateReceipt(withDefinition) == nil {
		t.Fatal("legacy V2 accepted an unsigned new definition")
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

func TestPrivateQuestionDraftReceiptIsPurposeSeparatedAndNeverAGate(t *testing.T) {
	now := time.Now().UTC()
	payload, private := continuationPayloadFixture(t, KindUserInput, "request_user_input", "not_required", now)
	public := private.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(public)
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil }
	source, err := NewReceipt(payload, keyID, public, sign)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []UserInputDraftAnswerV1{{QuestionID: source.Payload.GateID + "_1", OptionIDs: []string{}, CustomText: "  unfinished\n  "}}}
	revision := "00000000-0000-4000-8000-000000000001"
	draft, err := NewUserInputDraftReceiptV1(source, revision, nil, snapshot, domainsecurity.SHA256Hex([]byte("synthetic-empty-capture-set")), now.Add(2*time.Second), keyID, public, sign)
	if err != nil || draft.Payload.Kind != KindUserInputDraft || ValidateUserInputDraftSourceV1(draft, source) != nil {
		t.Fatal("draft source authority failed", err)
	}
	body, err := ReceiptBytes(draft)
	parsed, parseErr := ParseReceipt(body)
	if err != nil || parseErr != nil || !ed25519.Verify(public, ReceiptSigningBytes(parsed), mustReceiptSignature(t, parsed)) || !bytes.HasPrefix(ReceiptSigningBytes(parsed), []byte(userInputDraftSigningDomainV1)) {
		t.Fatal("signed draft roundtrip failed")
	}
	// The new private purpose must not serialize zero security or provider
	// authority, and must reject both omitted linkage and extra authority.
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		t.Fatal("draft envelope is invalid")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(envelope["payload"], &fields) != nil || len(fields) != 9 {
		t.Fatal("draft payload did not use its minimal wire")
	}
	for _, bad := range []string{
		strings.Replace(string(body), `"parentRevision":null,`, ``, 1),
		strings.Replace(string(body), `"kind":"user_input_draft"`, `"kind":"user_input_draft","arguments":null`, 1),
		strings.Replace(string(body), `"kind":"user_input_draft"`, `"kind":"user_input_draft","kind":"user_input_draft"`, 1),
		strings.Replace(string(body), `"snapshot":`, `"snapshot":null,"snapshot":`, 1),
	} {
		if _, err := ParseReceipt([]byte(bad)); err == nil {
			t.Fatal("nonclosed draft wire accepted")
		}
	}
	type ordinaryWire Payload
	ordinaryBytes, ordinaryErr := json.Marshal(ordinaryWire(source.Payload))
	actualBytes, actualErr := json.Marshal(source.Payload)
	if ordinaryErr != nil || actualErr != nil || !bytes.Equal(ordinaryBytes, actualBytes) {
		t.Fatal("ordinary payload signing wire changed")
	}
	if ValidateReceiptForExecution(draft) == nil {
		t.Fatal("draft authorized execution")
	}
	if _, err := NewDisposition(draft, StatusSubmitted, "user_input_submitted", now.Add(3*time.Second), keyID, public, sign); err == nil {
		t.Fatal("draft acquired a gate disposition")
	}
	other, err := NewUserInputDraftReceiptV1(source, "00000000-0000-4000-8000-000000000002", nil, snapshot, draft.Payload.UserInputDraft.CaptureSetSHA256, now.Add(2*time.Second), keyID, public, sign)
	if err != nil || other.Payload.GateID != draft.Payload.GateID || other.ReceiptID == draft.ReceiptID {
		t.Fatal("different children escaped the same parent slot")
	}
	copied := CloneUserInputDraftSnapshotV1(snapshot)
	copied.Answers[0].CustomText = "changed"
	tampered := draft
	link := *draft.Payload.UserInputDraft
	link.Snapshot = copied
	tampered.Payload.UserInputDraft = &link
	tampered.ReceiptID = payloadDigest(tampered.Payload)
	if ed25519.Verify(public, ReceiptSigningBytes(tampered), mustReceiptSignature(t, draft)) {
		t.Fatal("draft signature did not cover unfinished content")
	}
	tampered = draft
	tampered.Payload.Arguments = json.RawMessage(`{"raw":"not-draft"}`)
	if _, err := json.Marshal(tampered.Payload); err == nil {
		t.Fatal("private wire silently dropped extra authority")
	}
	if ValidatePayload(tampered.Payload) == nil {
		t.Fatal("draft carried copied model/tool authority")
	}
}

func TestPrivateQuestionDraftSnapshotRequiresClosedPartialState(t *testing.T) {
	inputID := "input_" + strings.Repeat("a", 64)
	valid := `{"currentPage":0,"answers":[{"questionId":"` + inputID + `_1","optionIds":[],"customText":" \n ","skipped":false}]}`
	var snapshot UserInputDraftSnapshotV1
	if json.Unmarshal([]byte(valid), &snapshot) != nil || ValidateUserInputDraftSnapshotV1(snapshot, inputID) != nil {
		t.Fatal("blank unfinished text rejected")
	}
	for _, raw := range []string{
		strings.Replace(valid, `"skipped":false`, `"skipped":false,"skipped":true`, 1),
		strings.Replace(valid, `,"skipped":false`, ``, 1),
		strings.Replace(valid, `"optionIds":[]`, `"optionIds":null`, 1),
		strings.Replace(valid, `"currentPage":0`, `"currentPage":0,"extra":true`, 1),
		strings.Replace(valid, `"customText":" \n "`, `"customText":"\ud800"`, 1),
	} {
		if json.Unmarshal([]byte(raw), &snapshot) == nil {
			t.Fatal("open/ambiguous partial draft accepted")
		}
	}
}

func TestQuestionUnknownContinuedIsReasonBoundAndNeverLiveUnlimitedTimeout(t *testing.T) {
	now := time.Now().UTC()
	payload, privateKey := continuationPayloadFixture(t, KindUserInput, "request_user_input", "not_required", now)
	payload.UserInputTimedWait = &UserInputTimedWaitV1{TimeoutSeconds: -1}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(publicKey)
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(privateKey, body), nil }
	receipt, err := NewReceipt(payload, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewDisposition(receipt, StatusContinued, ReasonUserInputContinuedV1, now.Add(2*time.Second), keyID, publicKey, sign); err == nil {
		t.Fatal("live unlimited call became a successful foreground timeout")
	}
	disposition, err := NewDisposition(receipt, StatusContinued, ReasonUserInputOutcomeUnknownAfterRestartV1, now.Add(2*time.Second), keyID, publicKey, sign)
	if err != nil || ValidateDisposition(disposition) != nil || !ValidUserInputContinuedReasonV1(payload.UserInputTimedWait, disposition.ReasonCode) {
		t.Fatal("unknown reason did not preserve interrupted unlimited association", err)
	}
	if projection, ok := ResolveProjection(KindUserInput, disposition.Status, disposition.ReasonCode); !ok || projection.PublicStatus != "continued" {
		t.Fatal("unknown Question public reopen relation lost")
	}
	if ValidUserInputContinuedReasonV1(nil, ReasonUserInputOutcomeUnknownAfterRestartV1) {
		t.Fatal("ordinary legacy Question was converted to timed continued")
	}
}

func TestPrivateDraftWaitChoicePreservesOmissionAndClosedClone(t *testing.T) {
	inputID := "input_" + strings.Repeat("a", 64)
	for _, wait := range []string{"waiting", "editing"} {
		raw := `{"currentPage":0,"answers":[],"wait":"` + wait + `"}`
		var snapshot UserInputDraftSnapshotV1
		if json.Unmarshal([]byte(raw), &snapshot) != nil || ValidateUserInputDraftSnapshotV1(snapshot, inputID) != nil || snapshot.Wait == nil || *snapshot.Wait != wait {
			t.Fatal("private wait choice did not round trip")
		}
		copy := CloneUserInputDraftSnapshotV1(snapshot)
		*copy.Wait = "waiting"
		if wait == "editing" && *snapshot.Wait != "editing" {
			t.Fatal("draft wait clone retained mutable pointer")
		}
	}
	for _, raw := range []string{`{"currentPage":0,"answers":[],"wait":null}`, `{"currentPage":0,"answers":[],"wait": null }`, `{"currentPage":0,"answers":[],"wait":"approved"}`} {
		var snapshot UserInputDraftSnapshotV1
		if err := json.Unmarshal([]byte(raw), &snapshot); err == nil && ValidateUserInputDraftSnapshotV1(snapshot, inputID) == nil {
			t.Fatal("invalid wait choice became authority")
		}
	}
	var old UserInputDraftSnapshotV1
	if json.Unmarshal([]byte(`{"currentPage":0,"answers":[]}`), &old) != nil || old.Wait != nil {
		t.Fatal("old signed snapshot omission changed")
	}
	body, _ := json.Marshal(old)
	if bytes.Contains(body, []byte(`"wait"`)) {
		t.Fatal("old canonical signing bytes gained a wait field")
	}
}

func TestPlanReviewReceiptRemainsLegacyBlocking(t *testing.T) {
	payload, _ := continuationPayloadFixture(t, KindUserInput, "exit_plan_mode", "not_required", time.Now().UTC())
	if err := ValidatePayload(payload); err != nil {
		t.Fatal(err)
	}
	for _, seconds := range []int{-1, 1, 120} {
		payload.UserInputTimedWait = &UserInputTimedWaitV1{TimeoutSeconds: seconds}
		if ValidatePayload(payload) == nil {
			t.Fatal("plan review acquired timed policy")
		}
	}
}

func TestOutputCapContinuationIsSignedScopedAndLegacyOmitted(t *testing.T) {
	payload, privateKey := continuationPayloadFixture(t, KindApproval, "write_file", "pending", time.Now().UTC())
	legacy, _ := json.Marshal(payload)
	if bytes.Contains(legacy, []byte(`"outputCapLogicalSequence"`)) {
		t.Fatal("legacy signing bytes changed")
	}
	payload.OutputCapLogicalSequence = 1
	publicKey := privateKey.Public().(ed25519.PublicKey)
	receipt, err := NewReceipt(payload, domainsecurity.SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ReceiptBytes(receipt)
	parsed, err := ParseReceipt(body)
	if err != nil || parsed.Payload.OutputCapLogicalSequence != 1 || parsed.Payload.ThreadID != payload.ThreadID || parsed.Payload.TurnID != payload.TurnID {
		t.Fatal("output cap did not survive its exact signed turn continuation")
	}
	tampered := parsed
	tampered.Payload.OutputCapLogicalSequence = 0
	if ValidateReceipt(tampered) == nil {
		t.Fatal("unsigned cap removal accepted")
	}
	payload.OutputCapLogicalSequence = 2
	if ValidatePayload(payload) == nil {
		t.Fatal("future output cap sequence accepted")
	}
}
