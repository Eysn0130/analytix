package cachetelemetry

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type providerLedgerFixtureV1 struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
	keyID      string
}

func TestProviderLedgerV1StrictSignedRoundTrip(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 1, "2026-07-14T00:00:00Z", validShape("call-1", 1, "2026-07-14T00:00:00Z"))
	settlement := fixture.settlement(t, intent, ProviderDispatchStateSent, succeededObservationForShapeV1(intent.Shape, "2026-07-14T00:00:01Z"), "provider_succeeded")
	closure := fixture.closure(t, intent.TurnBindingHMAC, []ProviderAttemptIntentV1{intent}, []ProviderAttemptSettlementV1{settlement}, ProviderTurnTerminalSuccessV1, "2026-07-14T00:00:02Z")

	intentBody, err := ProviderAttemptIntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	settlementBody, err := ProviderAttemptSettlementV1Bytes(settlement)
	if err != nil {
		t.Fatal(err)
	}
	closureBody, err := ProviderTurnClosureV1Bytes(closure)
	if err != nil {
		t.Fatal(err)
	}
	parsedIntent, err := ParseProviderAttemptIntentV1(intentBody)
	if err != nil || parsedIntent != intent {
		t.Fatalf("intent round trip failed: parsed=%#v err=%v", parsedIntent, err)
	}
	parsedSettlement, err := ParseProviderAttemptSettlementV1(settlementBody)
	if err != nil || parsedSettlement != settlement {
		t.Fatalf("settlement round trip failed: parsed=%#v err=%v", parsedSettlement, err)
	}
	parsedClosure, err := ParseProviderTurnClosureV1(closureBody)
	if err != nil || parsedClosure != closure {
		t.Fatalf("closure round trip failed: parsed=%#v err=%v", parsedClosure, err)
	}
	if err := ValidateProviderAttemptSettlementForIntentV1(parsedSettlement, parsedIntent); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProviderTurnClosureForInventoryV1(parsedClosure, []ProviderAttemptIntentV1{parsedIntent}, []ProviderAttemptSettlementV1{parsedSettlement}); err != nil {
		t.Fatal(err)
	}
	for name, material := range map[string]func() (string, []byte, []byte, error){
		"intent": func() (string, []byte, []byte, error) { return ProviderAttemptIntentV1AuthorityMaterial(intent) },
		"settlement": func() (string, []byte, []byte, error) {
			return ProviderAttemptSettlementV1AuthorityMaterial(settlement)
		},
		"closure": func() (string, []byte, []byte, error) { return ProviderTurnClosureV1AuthorityMaterial(closure) },
	} {
		t.Run(name+" authority material", func(t *testing.T) {
			keyID, publicKey, signature, err := material()
			if err != nil || keyID != fixture.keyID || !bytes.Equal(publicKey, fixture.publicKey) || len(signature) != ed25519.SignatureSize {
				t.Fatalf("unexpected authority material: key=%q public=%x signature=%x err=%v", keyID, publicKey, signature, err)
			}
		})
	}
}

func TestProviderLedgerV1RejectsUnknownMissingNullDuplicateAndTrailingJSON(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 1, "2026-07-14T00:00:00Z", validShape("call-1", 1, "2026-07-14T00:00:00Z"))
	settlement := fixture.settlement(t, intent, ProviderDispatchStateSent, succeededObservationForShapeV1(intent.Shape, "2026-07-14T00:00:01Z"), "provider_succeeded")
	closure := fixture.closure(t, intent.TurnBindingHMAC, []ProviderAttemptIntentV1{intent}, []ProviderAttemptSettlementV1{settlement}, ProviderTurnTerminalSuccessV1, "2026-07-14T00:00:02Z")
	tests := []struct {
		name     string
		body     any
		required string
		parse    func([]byte) error
	}{
		{name: "intent", body: intent, required: "purpose", parse: func(body []byte) error { _, err := ParseProviderAttemptIntentV1(body); return err }},
		{name: "settlement", body: settlement, required: "safeReasonCode", parse: func(body []byte) error { _, err := ParseProviderAttemptSettlementV1(body); return err }},
		{name: "closure", body: closure, required: "terminalReasonCode", parse: func(body []byte) error { _, err := ParseProviderTurnClosureV1(body); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.body)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]any
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatal(err)
			}
			unknown := cloneProviderLedgerObjectV1(t, object)
			unknown["rawPrompt"] = "untrusted"
			assertProviderLedgerParseFailsV1(t, test.parse, mustJSONV1(t, unknown))
			missing := cloneProviderLedgerObjectV1(t, object)
			delete(missing, test.required)
			assertProviderLedgerParseFailsV1(t, test.parse, mustJSONV1(t, missing))
			nullValue := cloneProviderLedgerObjectV1(t, object)
			nullValue[test.required] = nil
			assertProviderLedgerParseFailsV1(t, test.parse, mustJSONV1(t, nullValue))
			duplicated := strings.Replace(string(raw), `"`+test.required+`":`, `"`+test.required+`":null,"`+test.required+`":`, 1)
			assertProviderLedgerParseFailsV1(t, test.parse, []byte(duplicated))
			assertProviderLedgerParseFailsV1(t, test.parse, append(raw, []byte(` {}`)...))
		})
	}
}

func TestProviderLedgerV1RejectsSignatureAndPayloadMutation(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 1, "2026-07-14T00:00:00Z", validShape("call-1", 1, "2026-07-14T00:00:00Z"))

	mutatedPayload := intent
	mutatedPayload.ChannelOrdinal = 2
	if err := ValidateProviderAttemptIntentV1(mutatedPayload); err == nil {
		t.Fatal("expected signed payload mutation rejection")
	}
	mutatedSignature := intent
	mutatedSignature.AuthoritySignature = strings.Repeat("A", len(intent.AuthoritySignature))
	if err := ValidateProviderAttemptIntentV1(mutatedSignature); err == nil {
		t.Fatal("expected signature mutation rejection")
	}
}

func TestProviderTurnTerminalReasonsV1AreClosedAndUnique(t *testing.T) {
	reasons := AllProviderTurnTerminalReasonsV1()
	if len(reasons) != 18 {
		t.Fatalf("unexpected provider terminal reason count: %d", len(reasons))
	}
	seen := make(map[ProviderTurnTerminalReasonV1]struct{}, len(reasons))
	for _, reason := range reasons {
		if err := ValidateProviderTurnTerminalReasonV1(reason); err != nil {
			t.Fatalf("declared provider terminal reason %q is invalid: %v", reason, err)
		}
		if _, exists := seen[reason]; exists {
			t.Fatalf("duplicate provider terminal reason %q", reason)
		}
		seen[reason] = struct{}{}
	}
	if err := ValidateProviderTurnTerminalReasonV1("completed"); err == nil {
		t.Fatal("free-form provider terminal reason was accepted")
	}

	fixture := newProviderLedgerFixtureV1(t)
	if _, err := NewProviderTurnClosureV1(ProviderTurnClosureInputV1{
		TurnBindingHMAC: testHMAC("turn-unknown-reason"), TerminalReasonCode: "completed",
		ClosedAt:       mustProviderLedgerTimeV1(t, "2026-07-14T00:00:00Z"),
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign); err == nil {
		t.Fatal("signed closure accepted an unknown terminal reason")
	}
}

func TestProviderTurnBenchmarkEligibilityRequiresEvidenceBearingTerminal(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-benchmark", 1, 1, ProviderChannelPrimary, 1, "2026-07-14T00:00:00Z", validShape("call-benchmark", 1, "2026-07-14T00:00:00Z"))
	settlement := fixture.settlement(t, intent, ProviderDispatchStateSent, succeededObservationForShapeV1(intent.Shape, "2026-07-14T00:00:01Z"), "provider_succeeded")
	allowed := map[ProviderTurnTerminalReasonV1]bool{
		ProviderTurnTerminalSuccessV1: true, ProviderTurnTerminalApprovalV1: true,
		ProviderTurnTerminalUserInputV1: true, ProviderTurnTerminalResumeV1: true,
		ProviderTurnTerminalBackgroundCompletionV1: true,
	}
	for _, reason := range AllProviderTurnTerminalReasonsV1() {
		closure := fixture.closure(t, intent.TurnBindingHMAC, []ProviderAttemptIntentV1{intent}, []ProviderAttemptSettlementV1{settlement}, reason, "2026-07-14T00:00:02Z")
		if closure.BenchmarkEligible != allowed[reason] {
			t.Fatalf("terminal reason %q benchmark eligibility=%v, want %v", reason, closure.BenchmarkEligible, allowed[reason])
		}
	}
}

func TestProviderLedgerV1RestartInterruptedIsIndeterminateAndUsageUnknown(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 1, "2026-07-14T00:00:00Z", validShape("call-1", 1, "2026-07-14T00:00:00Z"))
	observation := ProviderCallObservationV1{
		SchemaVersion: ProviderCallObservationV1SchemaVersion, Shape: intent.Shape,
		Status: ProviderCallStatusRestartInterrupted, Usage: ProviderUsageV1{}, SettledAt: "2026-07-14T00:00:01Z",
	}
	settlement := fixture.settlement(t, intent, ProviderDispatchStateIndeterminate, observation, "restart_interrupted")
	closure := fixture.closure(t, intent.TurnBindingHMAC, []ProviderAttemptIntentV1{intent}, []ProviderAttemptSettlementV1{settlement}, ProviderTurnTerminalRestartV1, "2026-07-14T00:00:02Z")
	if closure.BenchmarkEligible {
		t.Fatal("restart-interrupted provider call became benchmark eligible")
	}

	if _, err := NewProviderAttemptSettlementV1(ProviderAttemptSettlementInputV1{
		Intent: intent, DispatchState: ProviderDispatchStateSent, Observation: observation,
		SafeReasonCode: "restart_interrupted", AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign); err == nil {
		t.Fatal("expected sent restart settlement rejection")
	}
	knownUsage := observation
	knownUsage.Usage.InputTokens = TokenCountV1{Known: true, Value: 1}
	if _, err := NewProviderAttemptSettlementV1(ProviderAttemptSettlementInputV1{
		Intent: intent, DispatchState: ProviderDispatchStateIndeterminate, Observation: knownUsage,
		SafeReasonCode: "restart_interrupted", AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign); err == nil {
		t.Fatal("expected known restart usage rejection")
	}
}

func TestProviderTurnClosureV1RequiresExactOrderedGraph(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	shape1 := validShape("call-1", 1, "2026-07-14T00:00:00Z")
	intent1 := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 1, shape1.StartedAt, shape1)
	settlement1 := fixture.settlement(t, intent1, ProviderDispatchStateSent, succeededObservationForShapeV1(shape1, "2026-07-14T00:00:01Z"), "provider_succeeded")
	shape2 := validShape("call-1", 2, "2026-07-14T00:00:02Z")
	intent2 := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 2, shape2.StartedAt, shape2)
	settlement2 := fixture.settlement(t, intent2, ProviderDispatchStateSent, succeededObservationForShapeV1(shape2, "2026-07-14T00:00:03Z"), "provider_succeeded")
	intents := []ProviderAttemptIntentV1{intent2, intent1}
	settlements := []ProviderAttemptSettlementV1{settlement2, settlement1}
	closure := fixture.closure(t, intent1.TurnBindingHMAC, intents, settlements, ProviderTurnTerminalSuccessV1, "2026-07-14T00:00:04Z")
	if err := ValidateProviderTurnClosureForInventoryV1(closure, []ProviderAttemptIntentV1{intent1, intent2}, []ProviderAttemptSettlementV1{settlement1, settlement2}); err != nil {
		t.Fatalf("inventory order changed closure meaning: %v", err)
	}
	if err := ValidateProviderTurnClosureForInventoryV1(closure, []ProviderAttemptIntentV1{intent1}, []ProviderAttemptSettlementV1{settlement1}); err == nil {
		t.Fatal("expected incomplete exact-set inventory rejection")
	}

	overlappingShape := validShape("call-1", 2, "2026-07-14T00:00:00.5Z")
	overlappingIntent := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 2, overlappingShape.StartedAt, overlappingShape)
	overlappingSettlement := fixture.settlement(t, overlappingIntent, ProviderDispatchStateSent, succeededObservationForShapeV1(overlappingShape, "2026-07-14T00:00:03Z"), "provider_succeeded")
	if _, err := NewProviderTurnClosureV1(ProviderTurnClosureInputV1{
		TurnBindingHMAC: intent1.TurnBindingHMAC,
		Intents:         []ProviderAttemptIntentV1{intent1, overlappingIntent}, Settlements: []ProviderAttemptSettlementV1{settlement1, overlappingSettlement},
		TerminalReasonCode: ProviderTurnTerminalSuccessV1, ClosedAt: mustProviderLedgerTimeV1(t, "2026-07-14T00:00:04Z"),
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign); err == nil {
		t.Fatal("expected overlapping physical-attempt rejection")
	}
}

func TestProviderLedgerV1SeparatesCacheNamespaces(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	shape1 := validShape("call-1", 1, "2026-07-14T00:00:00Z")
	intent1 := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 1, shape1.StartedAt, shape1)
	settlement1 := fixture.settlement(t, intent1, ProviderDispatchStateSent, succeededObservationForShapeV1(shape1, "2026-07-14T00:00:01Z"), "provider_succeeded")
	shape2 := validShape("call-2", 1, "2026-07-14T00:00:02Z")
	shape2.ModelHMAC = testHMAC("another-model")
	intent2 := fixture.intent(t, "call-2", 2, 1, ProviderChannelAttachmentVision, 1, shape2.StartedAt, shape2)
	observation2 := succeededObservationForShapeV1(shape2, "2026-07-14T00:00:03Z")
	observation2.Usage.InputTokens.Value = 40
	observation2.Usage.CacheHitTokens.Value = 10
	observation2.Usage.CacheMissTokens.Value = 30
	settlement2 := fixture.settlement(t, intent2, ProviderDispatchStateSent, observation2, "provider_succeeded")
	aggregates, err := BuildProviderLedgerNamespaceAggregatesV1(
		[]ProviderAttemptIntentV1{intent2, intent1}, []ProviderAttemptSettlementV1{settlement1, settlement2},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 2 {
		t.Fatalf("incompatible provider calls were combined into %d namespace(s)", len(aggregates))
	}
	for _, aggregate := range aggregates {
		if aggregate.AttemptCount != 1 || aggregate.DispatchCounts.Sent != 1 || aggregate.StatusCounts.Succeeded != 1 {
			t.Fatalf("unexpected namespace aggregate: %#v", aggregate)
		}
	}
	closure := fixture.closure(t, intent1.TurnBindingHMAC,
		[]ProviderAttemptIntentV1{intent2, intent1}, []ProviderAttemptSettlementV1{settlement2, settlement1},
		ProviderTurnTerminalSuccessV1, "2026-07-14T00:00:04Z")
	if !closure.BenchmarkEligible {
		t.Fatal("complete successful namespace-local usage was not benchmark eligible")
	}
}

func TestProviderLedgerV1PersistsNoRawPromptPIIEndpointOrReasoning(t *testing.T) {
	fixture := newProviderLedgerFixtureV1(t)
	intent := fixture.intent(t, "call-1", 1, 1, ProviderChannelPrimary, 1, "2026-07-14T00:00:00Z", validShape("call-1", 1, "2026-07-14T00:00:00Z"))
	settlement := fixture.settlement(t, intent, ProviderDispatchStateSent, succeededObservationForShapeV1(intent.Shape, "2026-07-14T00:00:01Z"), "provider_succeeded")
	closure := fixture.closure(t, intent.TurnBindingHMAC, []ProviderAttemptIntentV1{intent}, []ProviderAttemptSettlementV1{settlement}, ProviderTurnTerminalSuccessV1, "2026-07-14T00:00:02Z")
	body, err := json.Marshal([]any{intent, settlement, closure})
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(body))
	for _, forbidden := range []string{
		"合成样例事件甲", "6222021234567890123", "320101199001011234", "sk-secret-provider-key",
		"https://api.example.test/v1/chat?token=secret", "/users/sun/projects/case-a", "case-a",
		`"prompt"`, `"reasoningcontent"`, `"reasoning_content"`, `"requestbody"`, `"authorization"`,
		`"endpointurl"`, `"caseid"`, `"workspacepath"`,
	} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("provider ledger contains forbidden raw value or field %q: %s", forbidden, body)
		}
	}
}

func newProviderLedgerFixtureV1(t *testing.T) providerLedgerFixtureV1 {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x5a}, ed25519.SeedSize))
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	return providerLedgerFixtureV1{publicKey: publicKey, privateKey: privateKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}

func (fixture providerLedgerFixtureV1) sign(body []byte) ([]byte, error) {
	return ed25519.Sign(fixture.privateKey, body), nil
}

func (fixture providerLedgerFixtureV1) intent(t *testing.T, logicalCall string, logicalOrdinal, channelOrdinal uint32, channel ProviderChannelV1, attempt uint32, startedAt string, shape CacheVisibleShapeV1) ProviderAttemptIntentV1 {
	t.Helper()
	shape.LogicalCallHMAC = testHMAC(logicalCall)
	shape.Attempt = attempt
	shape.StartedAt = startedAt
	intent, err := NewProviderAttemptIntentV1(ProviderAttemptIntentInputV1{
		TurnBindingHMAC: testHMAC("turn-1"), UsageSource: ProviderUsageSourceTurn,
		ChildRunHMAC: testHMAC("root-run"), Channel: channel, ChannelOrdinal: channelOrdinal,
		LogicalCallOrdinal: logicalOrdinal, Shape: shape, WireHeadersHMAC: testHMAC("headers-" + logicalCall),
		IssuedAt: mustProviderLedgerTimeV1(t, startedAt), AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func (fixture providerLedgerFixtureV1) settlement(t *testing.T, intent ProviderAttemptIntentV1, dispatch ProviderDispatchStateV1, observation ProviderCallObservationV1, reason string) ProviderAttemptSettlementV1 {
	t.Helper()
	settlement, err := NewProviderAttemptSettlementV1(ProviderAttemptSettlementInputV1{
		Intent: intent, DispatchState: dispatch, Observation: observation, SafeReasonCode: reason,
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return settlement
}

func (fixture providerLedgerFixtureV1) closure(t *testing.T, turnBinding string, intents []ProviderAttemptIntentV1, settlements []ProviderAttemptSettlementV1, reason ProviderTurnTerminalReasonV1, closedAt string) ProviderTurnClosureV1 {
	t.Helper()
	closure, err := NewProviderTurnClosureV1(ProviderTurnClosureInputV1{
		TurnBindingHMAC: turnBinding, Intents: intents, Settlements: settlements,
		TerminalReasonCode: reason, ClosedAt: mustProviderLedgerTimeV1(t, closedAt),
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return closure
}

func succeededObservationForShapeV1(shape CacheVisibleShapeV1, settledAt string) ProviderCallObservationV1 {
	return ProviderCallObservationV1{
		SchemaVersion: ProviderCallObservationV1SchemaVersion, Shape: shape, Status: ProviderCallStatusSucceeded,
		Usage: ProviderUsageV1{
			InputTokens: TokenCountV1{Known: true, Value: 100}, OutputTokens: TokenCountV1{Known: true, Value: 20},
			CacheHitTokens: TokenCountV1{Known: true, Value: 90}, CacheMissTokens: TokenCountV1{Known: true, Value: 10},
			ReasoningTokens: TokenCountV1{Known: false},
		},
		SettledAt: settledAt,
	}
}

func mustProviderLedgerTimeV1(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func cloneProviderLedgerObjectV1(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	body := mustJSONV1(t, value)
	var clone map[string]any
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func mustJSONV1(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func assertProviderLedgerParseFailsV1(t *testing.T, parse func([]byte) error, body []byte) {
	t.Helper()
	if err := parse(body); err == nil {
		t.Fatalf("expected provider ledger parse failure for %s", body)
	}
}
