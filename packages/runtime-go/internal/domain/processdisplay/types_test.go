package processdisplay

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/secretprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCompleteToolInputRejectsDecodedCredentialMaterialV2(t *testing.T) {
	const secret = "opaque-current-attempt-value-9251"
	for _, test := range []struct {
		name    string
		raw     string
		secrets []string
		fact    func(map[string]any) bool
	}{
		{"unicode command", `{"command":"echo \u006fpaque-current-attempt-value-9251"}`, []string{secret}, func(value map[string]any) bool {
			return value["command"] == "echo "+secret
		}},
		{"nested array value", `{"nested":[{"value":"opaque-\u0063urrent-attempt-value-9251"}]}`, []string{secret}, func(value map[string]any) bool {
			return value["nested"].([]any)[0].(map[string]any)["value"] == secret
		}},
		{"nested escaped key", `{"nested":[{"\u006fpaque-current-attempt-value-9251":"Alice"}]}`, []string{secret}, func(value map[string]any) bool {
			return value["nested"].([]any)[0].(map[string]any)[secret] == "Alice"
		}},
		{"mixed literal and escaped", `{"literal":"opaque-current-attempt-value-9251","nested":{"value":"\u006fpaque-current-attempt-value-9251"}}`, []string{secret}, func(value map[string]any) bool {
			return value["literal"] == secret && value["nested"].(map[string]any)["value"] == secret
		}},
		{"escaped credential field", `{"nested":[{"api_\u006bey":"opaque-current-attempt-value-9251"}]}`, nil, func(value map[string]any) bool {
			return value["nested"].([]any)[0].(map[string]any)["api_key"] == secret
		}},
		{"escaped metadata field with material", `{"nested":[{"hasApi\u004bey":"opaque-current-attempt-value-9251"}]}`, nil, func(value map[string]any) bool {
			return value["nested"].([]any)[0].(map[string]any)["hasApiKey"] == secret
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := json.RawMessage(test.raw)
			decoded, err := domainsecurity.DecodeCanonicalJSONObject(raw)
			if err != nil || !test.fact(decoded) {
				t.Fatal("fixture did not decode to its actual credential; literal backslash-u is not this regression")
			}
			input, class := ProjectToolInputV2(raw, test.secrets...)
			if input.Availability != "unavailable" || input.InputJSON != "" || class != "withheld" {
				t.Fatal("semantic JSON credential survived the complete-input boundary")
			}
		})
	}
}

func TestCompleteToolInputSemanticGuardPreservesOriginalSafeSourceAndBudgetsV2(t *testing.T) {
	const secret = "opaque-current-attempt-value-9251"
	for name, raw := range map[string]json.RawMessage{
		"exact escaped source":       json.RawMessage(" {\n  \"number\":9007199254740993, \"exponent\":1e+03, \"nested\":[{\"A\\u006cice\":\"A\\u006cice alice@example.invalid\"}]\n} "),
		"existing depth":             json.RawMessage(`{"nested":` + strings.Repeat("[", 70) + `"Alice"` + strings.Repeat("]", 70) + `}`),
		"existing node budget":       json.RawMessage(`{"nested":[` + strings.Repeat("0,", 10000) + `0]}`),
		"empty and boolean metadata": json.RawMessage(` {"hasApi\u004bey":false,"password":"  "," \t": ["  \t",null]} `),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domainsecurity.DecodeCanonicalJSONObject(raw); err != nil || len(raw) > ToolInputJSONByteLimitV2 {
				t.Fatal("safe fixture exceeded the existing complete-input decoder or byte budget")
			}
			input, class := ProjectToolInputV2(raw, secret)
			if input.Availability != "available" || input.InputJSON != string(raw) || class != "source_exact_local" {
				t.Fatal("semantic credential guard rebuilt safe original JSON or reduced its accepted budget")
			}
		})
	}
}

func TestCompleteToolInputKeepsOriginalNumbersWhitespaceAndCaseV2(t *testing.T) {
	raw := json.RawMessage(" {\n  \"number\":9007199254740993, \"exponent\":1e+03, \"case\":\"Alice alice@example.invalid\"\n} ")
	input, class := ProjectToolInputV2(raw)
	if input.Availability != "available" || input.InputJSON != string(raw) || class != "source_exact_local" {
		t.Fatal("complete input was rebuilt or public-projected")
	}
	redacted, class := ProjectToolInputV2(json.RawMessage(`{"case":"Alice","api_key":"synthetic-private-key"}`))
	if redacted.Availability != "available" || class != "credential_redacted" || strings.Contains(redacted.InputJSON, "synthetic-private-key") || !strings.Contains(redacted.InputJSON, "Alice") {
		t.Fatal("credential-only projection lost its exact-source classification")
	}
}

func TestCompleteToolInputWithholdsWholeUnavailableSourcesV2(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`[]`), json.RawMessage(`{"a":1,"a":2}`), json.RawMessage("{\"a\":\"\xff\"}")} {
		input, class := ProjectToolInputV2(raw)
		if input.Availability != "unavailable" || input.InputJSON != "" || class != "withheld" {
			t.Fatal("missing or malformed input became a truth source")
		}
	}
	raw := json.RawMessage(`{"text":"` + strings.Repeat("x", ToolInputJSONByteLimitV2) + `"}`)
	input, _ := ProjectToolInputV2(raw)
	if input.Reason != "limit" || input.InputJSON != "" {
		t.Fatal("oversize input was prefix-truncated")
	}
	input, _ = ProjectToolInputV2(json.RawMessage(`{"actual":true}`), make([]string, 65)...)
	if input.Availability != "unavailable" {
		t.Fatal("closed secret scope became available")
	}
}

// These are producer-side tests: a display copy is made only from a complete
// host capture, before any response limit. No provider continuation is changed.
func TestCompleteCaptureProjectsCredentialsBeforeTruncationV1(t *testing.T) {
	secret := "process-display-test-credential-7839"
	raw := RawCaptureV1{
		Kind: "native_reasoning", Complete: true,
		Body:   strings.Repeat("x", CaptureBodyByteLimitV1-16) + secret + "\napi_key=" + secret,
		Title:  "api_\x1b[31mkey\x1b[0m=" + secret,
		Path:   "/private/" + secret + "/report.txt",
		Before: "customer Alice: " + secret,
		After:  "customer Alice: corrected " + secret,
		Error:  "failure echoed " + secret,
	}
	original := raw
	display := ProjectCaptureV1(raw, secret)
	if !reflect.DeepEqual(raw, original) {
		t.Fatal("projection changed the original producer capture")
	}
	if display.Classification != "credential_redacted" || !display.Truncated {
		t.Fatalf("classification/truncation = %q/%v", display.Classification, display.Truncated)
	}
	for _, value := range []string{display.Body, display.Preview, display.Title, display.Path, display.Before, display.After, display.Error} {
		if strings.Contains(value, secret) || strings.ContainsRune(value, '\x1b') {
			t.Fatal("display field retained credential or terminal escape")
		}
	}
	if !strings.Contains(display.Before, "customer Alice") || !strings.Contains(display.Body, secretprojection.RedactedV1) {
		t.Fatal("authorized local PII or redaction marker was lost")
	}
}

func TestCompleteCapturePreviewUsesWireByteBudgetV1(t *testing.T) {
	for _, test := range []struct {
		name, line, want string
	}{
		{"above old limit", strings.Repeat("中🙂", 100), strings.Repeat("中🙂", 100)},
		{"exact wire limit", strings.Repeat("中🙂", 146) + "ab", strings.Repeat("中🙂", 146) + "ab"},
		{"split Chinese", strings.Repeat("中", 342), strings.Repeat("中", 341)},
		{"split emoji", strings.Repeat("中", 341) + "🙂", strings.Repeat("中", 341)},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := test.line + "\n**Later paragraph**\nremaining body"
			got := ProjectCaptureV1(RawCaptureV1{Kind: "native_reasoning", Complete: true, Body: body})
			if got.Preview != test.want || !utf8.ValidString(got.Preview) || len(got.Preview) > 1024 {
				t.Fatalf("first-line preview: bytes=%d, want=%d", len(got.Preview), len(test.want))
			}
			if got.Body != body || got.Truncated || got.Classification != "source_exact_local" {
				t.Fatal("preview limit changed body, truncation or classification")
			}
		})
	}
}

func TestCompleteNonReasoningPreviewKeepsExistingByteBudgetV1(t *testing.T) {
	body := strings.Repeat("x", 700) + "\n**Later paragraph**\nremaining body"
	for _, kind := range []string{"shell", "write", "edit", "web_fetch", ""} {
		t.Run("kind="+kind, func(t *testing.T) {
			got := ProjectCaptureV1(RawCaptureV1{Kind: kind, Complete: true, Body: body, Diff: body})
			if got.Preview != strings.Repeat("x", 256) || got.Body != body || got.Truncated || got.Classification != "source_exact_local" {
				t.Fatal("non-reasoning preview or body budget changed")
			}
		})
	}
}

func TestCompleteCaptureKeepsExistingFieldBudgetsV1(t *testing.T) {
	got := ProjectCaptureV1(RawCaptureV1{
		Kind: "shell", Complete: true,
		Body: strings.Repeat("b", (32<<10)+1), Title: strings.Repeat("t", 1025),
		Path: strings.Repeat("p", 4097), Before: strings.Repeat("a", (32<<10)+1),
		After: strings.Repeat("c", (32<<10)+1), Error: strings.Repeat("e", (32<<10)+1),
	})
	if got.Body != strings.Repeat("b", 32<<10) || got.Before != strings.Repeat("a", 32<<10) || got.After != strings.Repeat("c", 32<<10) || got.Error != strings.Repeat("e", 32<<10) || got.Title != strings.Repeat("t", 1024) || got.Path != strings.Repeat("p", 4096) || !got.Truncated {
		t.Fatal("existing body or metadata field budgets changed")
	}
	if got.Preview != strings.Repeat("b", 256) {
		t.Fatal("non-reasoning preview did not retain its existing budget")
	}
}

func TestCompleteCaptureFindsSecretsAcrossFieldsV1(t *testing.T) {
	secret := "test-value-without-token-shaped-prefix-5467"
	display := ProjectCaptureV1(RawCaptureV1{
		Kind: "shell", Complete: true,
		Title: "curl --api-key " + secret,
		Body:  "echoed " + secret,
		Path:  "/tmp/" + secret,
		Error: "host error " + secret,
	})
	for _, value := range []string{display.Body, display.Preview, display.Title, display.Path, display.Error} {
		if strings.Contains(value, secret) {
			t.Fatal("cross-field credential remained in display")
		}
	}
}

func TestIncompleteOrOverflowCaptureWithholdsAllSensitiveFieldsV1(t *testing.T) {
	for _, raw := range []RawCaptureV1{
		{Kind: "native_reasoning", Body: "unfinished api_key=test-sensitive-prefix", Title: "sensitive preview"},
		{Kind: "shell", Complete: true, Overflow: true, Body: "prefix before discarded credential", Path: "/private/source"},
		{Kind: "write", Complete: true, Body: strings.Repeat("x", RawCaptureByteLimitV1+1), Before: "sensitive original", After: "sensitive replacement"},
		{Kind: "shell", Complete: true, Body: "unfinished \x1b]8;;https://private.example/"},
	} {
		display := ProjectCaptureV1(raw)
		if display.Classification != "withheld" {
			t.Fatalf("classification = %q", display.Classification)
		}
		if display.Body != "" || display.Preview != "" || display.Title != "" || display.Path != "" || display.Before != "" || display.After != "" || display.Error != "" {
			t.Fatal("withheld capture retained sensitive fields")
		}
	}
}

func TestCaptureShortKnownSecretsAndAggregateQuotaFailClosedV1(t *testing.T) {
	for _, raw := range []RawCaptureV1{
		{Kind: "shell", Complete: true, Body: "echo ab", Title: "command"},
		{Kind: "edit", Complete: true, Before: strings.Repeat("a", RawCaptureByteLimitV1/2+1), After: strings.Repeat("b", RawCaptureByteLimitV1/2+1)},
	} {
		display := ProjectCaptureV1(raw, "ab")
		if display.Classification != "withheld" || display.Body != "" || display.Preview != "" {
			t.Fatal("short known secret or aggregate over-budget capture was exposed")
		}
	}
}

func TestCapturePreservesAuthorizedLocalTextAndNeverSerializesRawV1(t *testing.T) {
	raw := RawCaptureV1{Kind: "native_reasoning", Complete: true, Body: "Alice, account 123456: compare the two rows."}
	display := ProjectCaptureV1(raw)
	if display.Classification != "source_exact_local" || display.Body != raw.Body || display.Preview != raw.Body {
		t.Fatal("protected-local copy did not preserve noncredential text")
	}
	encoded, err := json.Marshal(raw)
	if err != nil || strings.Contains(string(encoded), "Alice") || strings.Contains(string(encoded), "123456") {
		t.Fatal("raw producer capture entered generic JSON")
	}
}

func TestFullQuotaCrossFieldCredentialIsStillClassifiedV1(t *testing.T) {
	secret := "opaque-process-boundary-test-value-9481"
	title := "api_key=" + secret
	body := secret + strings.Repeat("x", RawCaptureByteLimitV1-len(title)-len(secret))
	display := ProjectCaptureV1(RawCaptureV1{Kind: "shell", Complete: true, Title: title, Body: body})
	if strings.Contains(display.Body, secret) || strings.Contains(display.Preview, secret) {
		t.Fatal("full-budget cross-field credential escaped projection")
	}
}

func TestPrivateDisplayCopyHasZeroGenericJSONV1(t *testing.T) {
	copy := ProjectCaptureV1(RawCaptureV1{Kind: "native_reasoning", Complete: true, Body: "private local text"})
	encoded, err := json.Marshal(copy)
	if err != nil || string(encoded) != "{}" {
		t.Fatal("private helper copy entered generic JSON")
	}
}

// These tests exercise the existing helper API before the mutable registry is
// added, so the first regression run reaches behavioral assertions.
func TestKnownSecretsDeduplicatesHostValuesV1(t *testing.T) {
	secret := "opaque-prepared-request-key-7364"
	ctx := WithKnownSecretsV1(context.Background(), "", secret, secret, "")
	if got := KnownSecretsV1(ctx); !reflect.DeepEqual(got, []string{secret}) {
		t.Fatalf("known-secret registry did not deduplicate bounded host values: count=%d", len(got))
	}
}

func TestKnownSecretsCountAndAggregateOverflowFailClosedV1(t *testing.T) {
	for name, secrets := range map[string][]string{
		"count": func() []string {
			values := make([]string, 65)
			for i := range values {
				values[i] = strings.Repeat("x", i+4)
			}
			return values
		}(),
		"aggregate": {strings.Repeat("a", RawCaptureByteLimitV1/2+1), strings.Repeat("b", RawCaptureByteLimitV1/2+1)},
	} {
		t.Run(name, func(t *testing.T) {
			known := KnownSecretsV1(WithKnownSecretsV1(context.Background(), secrets...))
			if len(known) <= 64 {
				t.Fatal("registry overflow did not invalidate display capture")
			}
			for _, value := range known {
				if value != "" {
					t.Fatal("overflow sentinel retained a host credential")
				}
			}
			display := ProjectCaptureV1(RawCaptureV1{Kind: "shell", Complete: true, Body: "bounded tool output"}, known...)
			if display.Classification != "withheld" || display.Body != "" {
				t.Fatal("overflow registry admitted a display body")
			}
		})
	}
}

func TestPreparedRequestSecretsReachNativeAndLaterToolCaptureV1(t *testing.T) {
	initial := "initial-request-key-5763"
	prepared := "opaque-current-attempt-value-9251"
	prior := "opaque-prior-attempt-value-8620"
	ctx := WithKnownSecretsV1(context.Background(), initial)
	attemptCtx := context.WithValue(ctx, struct{ name string }{"attempt"}, true)
	RecordKnownSecretsV1(attemptCtx, prior)
	RecordKnownSecretsV1(attemptCtx, prepared, prepared)
	// Both captures use the current turn registry, even if no reasoning frame
	// was emitted by either actual prepared request.
	for _, kind := range []string{"native_reasoning", "shell"} {
		display := ProjectCaptureV1(RawCaptureV1{Kind: kind, Complete: true, Body: prepared + " echoed " + prior}, KnownSecretsV1(ctx)...)
		if display.Classification != "credential_redacted" || strings.Contains(display.Body, prepared) || strings.Contains(display.Body, prior) {
			t.Fatal("prepared request credential escaped a subsequent display capture")
		}
	}
	known := KnownSecretsV1(ctx)
	if len(known) != 3 {
		t.Fatalf("deduplicated prepared-secret count=%d", len(known))
	}
	known[0] = "caller-controlled"
	if KnownSecretsV1(ctx)[0] != initial {
		t.Fatal("caller changed the private registry through a returned slice")
	}
}

func TestKnownSecretRegistryClearCannotBeRevivedV1(t *testing.T) {
	ctx := WithKnownSecretsV1(context.Background(), "initial-host-value-4278")
	staleCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ClearKnownSecretsV1(ctx)
	RecordKnownSecretsV1(staleCtx, "late-prepared-value-8624")
	known := KnownSecretsV1(ctx)
	if len(known) <= 64 {
		t.Fatal("closed turn registry permitted a late capture")
	}
	for _, value := range known {
		if value != "" {
			t.Fatal("closed registry retained or re-admitted a credential")
		}
	}
	if got := ProjectCaptureV1(RawCaptureV1{Kind: "shell", Complete: true, Body: "late tool output"}, known...); got.Classification != "withheld" {
		t.Fatal("closed registry did not fail closed for stale tool settlement")
	}
	fresh := WithKnownSecretsV1(context.Background(), "fresh-turn-key-5137")
	if got := KnownSecretsV1(fresh); !reflect.DeepEqual(got, []string{"fresh-turn-key-5137"}) {
		t.Fatal("closed registry contaminated a fresh turn")
	}
}

func TestRecordKnownSecretsShortOrOverflowValueInvalidatesRegistryV1(t *testing.T) {
	for name, value := range map[string]string{
		"short": "ab",
		"large": strings.Repeat("a", RawCaptureByteLimitV1+1),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := WithKnownSecretsV1(context.Background(), "initial-host-value-6314")
			RecordKnownSecretsV1(ctx, value)
			if len(KnownSecretsV1(ctx)) <= 64 {
				t.Fatal("unrepresentable actual prepared key permitted a display copy")
			}
		})
	}
	RecordKnownSecretsV1(nil, "ignored-without-registry-5823")
	ClearKnownSecretsV1(nil)
	if KnownSecretsV1(nil) != nil || KnownSecretsV1(context.Background()) != nil {
		t.Fatal("helper manufactured a registry without turn composition")
	}
}

func TestCompletedNativeWhitespacePreservesNormalizedSourceV1(t *testing.T) {
	for name, body := range map[string]string{
		"whitespace":      " \t\n\r\n ",
		"ansi-whitespace": "\x1b[31m \t\x1b[0m\n\r\n ",
	} {
		t.Run(name, func(t *testing.T) {
			normalized, ok := plainTerminalV1(body)
			if !ok || normalized == "" {
				t.Fatal("whitespace fixture was not a complete nonempty source")
			}
			copy := ProjectCaptureV1(RawCaptureV1{Kind: "native_reasoning", Complete: true, Body: body}, "host-credential-5297")
			if copy.Body != normalized || copy.Classification != "source_exact_local" || copy.Truncated {
				t.Fatalf("safe complete whitespace lost source exactness: bytes=%d classification=%q truncated=%v", len(copy.Body), copy.Classification, copy.Truncated)
			}
		})
	}
}

func TestMutationCaptureReadinessIsNonConsumingAndMatchesOnlyActualAliasesV1(t *testing.T) {
	for kind, aliases := range map[string][]string{"write": {"write", "write_file"}, "edit": {"edit", "edit_file", "multi_edit"}} {
		t.Run(kind, func(t *testing.T) {
			ctx := WithCollectorV1(context.Background(), &CollectorV1{})
			effect := true
			raw := RawCaptureV1{Kind: kind, Complete: true, Status: "completed", Before: "source before", After: "source after", EffectApplied: &effect}
			CaptureV1(ctx, raw)
			effect = false // The producer's pointer is copied at collection.
			for repeat := 0; repeat < 2; repeat++ {
				for _, alias := range aliases {
					if !MutationCaptureReadyV1(ctx, alias) {
						t.Fatal("real matching mutation capture lost non-consuming readiness")
					}
				}
			}
			wrong := "write"
			if kind == "write" {
				wrong = "edit"
			}
			for _, name := range []string{wrong, "bash", "read", "Write", "write_file_extra", ""} {
				if MutationCaptureReadyV1(ctx, name) {
					t.Fatal("mismatched producer or unowned alias selected ordinary settlement")
				}
			}
			got, present := ConsumeV1(ctx)
			if !present || got.Kind != kind || got.Before != raw.Before || got.After != raw.After || got.EffectApplied == nil || !*got.EffectApplied {
				t.Fatal("readiness consumed or changed the actual producer capture")
			}
			for _, alias := range aliases {
				if MutationCaptureReadyV1(ctx, alias) {
					t.Fatal("consumed capture retained readiness")
				}
			}
			if _, present := ConsumeV1(ctx); present {
				t.Fatal("capture consumed twice")
			}
			CaptureV1(ctx, raw)
			if MutationCaptureReadyV1(ctx, aliases[0]) {
				t.Fatal("late producer revived consumed capture")
			}
		})
	}
}

func TestMutationCaptureReadinessFailsClosedWithoutProducerAfterDuplicateAndCancellationV1(t *testing.T) {
	if MutationCaptureReadyV1(nil, "write") || MutationCaptureReadyV1(context.Background(), "write") {
		t.Fatal("readiness manufactured a collector")
	}
	empty := WithCollectorV1(context.Background(), &CollectorV1{})
	if MutationCaptureReadyV1(empty, "write") {
		t.Fatal("empty collector supplied readiness")
	}
	duplicate := WithCollectorV1(context.Background(), &CollectorV1{})
	CaptureV1(duplicate, RawCaptureV1{Kind: "write", Complete: true})
	CaptureV1(duplicate, RawCaptureV1{Kind: "write", Complete: true})
	if MutationCaptureReadyV1(duplicate, "write") {
		t.Fatal("duplicate producer retained readiness")
	}
	if _, present := ConsumeV1(duplicate); present {
		t.Fatal("duplicate producer released a capture")
	}
	base := WithCollectorV1(context.Background(), &CollectorV1{})
	CaptureV1(base, RawCaptureV1{Kind: "edit", Complete: true})
	canceled, cancel := context.WithCancel(base)
	cancel()
	if MutationCaptureReadyV1(canceled, "edit") {
		t.Fatal("canceled context selected exact ordinary settlement")
	}
	if !MutationCaptureReadyV1(base, "edit") {
		t.Fatal("readiness consumed a shared collector while checking cancellation")
	}
	for name, raw := range map[string]RawCaptureV1{
		"incomplete": {Kind: "write"}, "overflow": {Kind: "write", Complete: true, Overflow: true},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := WithCollectorV1(context.Background(), &CollectorV1{})
			CaptureV1(ctx, raw)
			if !MutationCaptureReadyV1(ctx, "write") {
				t.Fatal("actual bounded producer lost its lifecycle owner because display bytes are withheld")
			}
			capture, present := ConsumeV1(ctx)
			if !present || ProjectCaptureV1(capture).Classification != "withheld" {
				t.Fatal("readiness granted display disclosure")
			}
		})
	}
}

func TestV2SourceSerializationKeepsCoordinateKindsClosed(t *testing.T) {
	native := SourceV2{Kind: "native", UnitID: "unit-a", ProducerID: "producer-a", ProviderSequence: "7", Attempt: 2, NativeIndex: 0, SourceOrdinal: "3", CallID: "must-not-leak", ToolCallItemID: "must-not-leak"}
	tool := SourceV2{Kind: "tool", UnitID: "unit-a", CallID: "host-call", ToolCallItemID: "actual-call-item", ToolResultItemID: "actual-result-item", StartedEventSeq: "11", ProducerID: "must-not-leak", SourceOrdinal: "must-not-leak", Attempt: 9}
	for _, source := range []SourceV2{native, tool} {
		raw, err := json.Marshal(source)
		if err != nil || strings.Contains(string(raw), "must-not-leak") {
			t.Fatal("source coordinate union opened")
		}
	}
}

func TestReasoningSummaryFormIsOptionalAndConfinedToReasoningSourcesV2(t *testing.T) {
	for _, kind := range []string{"native", "provider_reasoning", "assistant_draft", "tool_preparation", "tool"} {
		for _, form := range []string{"", "summary"} {
			source := SourceV2{Kind: kind, UnitID: "unit", ProducerID: "producer", ProviderSequence: "7", Attempt: 1, SourceOrdinal: "1", ReasoningForm: form}
			raw, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			got, present := fields["reasoningForm"]
			want := form == "summary" && (kind == "native" || kind == "provider_reasoning")
			if present != want || present && string(got) != `"summary"` {
				t.Fatalf("source=%s form=%q escaped its source union", kind, form)
			}
		}
	}
}

func TestCollectorOwnsInputSizeWithoutGenericJSONV1(t *testing.T) {
	units := int64(1025)
	ctx := WithCollectorV1(context.Background(), &CollectorV1{})
	CaptureV1(ctx, RawCaptureV1{Kind: "write", Complete: true, InputUtf16CodeUnits: &units})
	units = 1
	capture, ok := ConsumeV1(ctx)
	if !ok || capture.InputUtf16CodeUnits == nil || *capture.InputUtf16CodeUnits != 1025 {
		t.Fatal("collector retained producer-owned size pointer")
	}
	encoded, err := json.Marshal(capture)
	if err != nil || string(encoded) != "{}" {
		t.Fatal("input size escaped raw protected capture")
	}
}

func TestAssistantDraftProjectionWithholdsChunkCandidatesAndPreservesSegmentsV1(t *testing.T) {
	known := "synth-known-request-secret-with spaces"
	cases := []struct {
		raw       string
		forbidden []string
	}{
		{"ready text " + known[:12], []string{known[:12]}},
		{"ready text " + known[:32], []string{known[:12], known[:32]}},
		{"ready text " + known + " ", []string{known}},
		{"ready text " + known, []string{known, known[:12]}},
		{"ready text password=\"multi word secret\"", []string{"multi", "word secret"}},
		{"ready text api_key=\"unclosed value with spaces ", []string{"unclosed", "value with"}},
		{"ready text sk-fixtureCred", []string{"sk-fixtureCred"}},
		{"ready text api_\x1b[31mkey\x1b[0m=opaqueSyntheticKey ", []string{"opaqueSyntheticKey"}},
		{"ready text -----BEGIN PRIVATE KEY-----\nprivate material ", []string{"private material"}},
		{"ready text https://example.invalid/?api_key=opaqueSyntheticURL ", []string{"opaqueSyntheticURL"}},
	}
	for i, c := range cases {
		capture := ProjectAssistantDraftV1(c.raw, known)
		for _, forbidden := range c.forbidden {
			if strings.Contains(capture.Body, forbidden) {
				t.Fatalf("candidate %d escaped projection", i)
			}
		}
		if capture.Classification != "withheld" && !strings.HasPrefix(capture.Body, "ready text ") {
			t.Fatalf("candidate %d lost safe source", i)
		}
	}
	// Multiple windows are one complete projected revision, not prefixes
	// claimed to be complete captures. Redaction offsets count projected bytes.
	body := strings.Repeat("字 ", CaptureBodyByteLimitV1/2) + known + " "
	capture := ProjectAssistantDraftV1(body, known)
	segments := SegmentAssistantDraftV1(capture.Body)
	joined := ""
	for _, segment := range segments {
		if segment.OffsetBytes != fmt.Sprint(len(joined)) || len(segment.Text) > CaptureBodyByteLimitV1 || !utf8.ValidString(segment.Text) {
			t.Fatal("invalid projected draft window")
		}
		joined += segment.Text
	}
	if len(segments) < 2 || joined != capture.Body || strings.Contains(joined, known) {
		t.Fatal("draft segmentation lost bytes or leaked key")
	}
	if got := ProjectCaptureV1(RawCaptureV1{Kind: "native_reasoning", Body: "still incomplete "}, known); got.Classification != "withheld" {
		t.Fatal("draft support changed native completion authority")
	}
}

func TestAssistantDraftSourceDoesNotForgeNativeOrToolIdentityV1(t *testing.T) {
	encoded, err := json.Marshal(SourceV2{Kind: "assistant_draft", UnitID: "unit", ProducerID: "producer", ProviderSequence: "7", Attempt: 2, NativeIndex: 99, CallID: "forged"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"nativeIndex", "sourceOrdinal", "callId", "toolCallItemId", "forged"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("draft source serialized fabricated identity")
		}
	}
	if !strings.Contains(string(encoded), `"attempt":2`) || !strings.Contains(string(encoded), `"providerSequence":"7"`) {
		t.Fatal("draft source lost actual coordinates")
	}
}

func TestAssistantDraftCandidateGuardClosesOverlappingAndTrimmedKeysV1(t *testing.T) {
	for _, c := range []struct {
		raw       string
		keys      []string
		forbidden string
	}{
		{"safe abcd efgh zzzTAIL", []string{"abcd efgh", "efgh zzzz"}, "abcd"},
		{"safe opaque long secret", []string{"  opaque long secret  "}, "opaque"},
		{"safe api_key=\"opaque long value\"", []string{"long value zzzz"}, "opaque"},
	} {
		got := ProjectAssistantDraftV1(c.raw, c.keys...)
		if strings.Contains(got.Body, c.forbidden) {
			t.Fatal("candidate cut exposed an earlier secret prefix")
		}
	}
}

func TestAssistantDraftInvalidProjectionCannotStallSegmentationV1(t *testing.T) {
	capture := ProjectAssistantDraftV1("safe Ѐabc ", "\x80abc")
	if capture.Classification != "withheld" || capture.Body != "" {
		t.Fatal("malformed known credential produced invalid display UTF8")
	}
	if segments := SegmentAssistantDraftV1("\xd0invalid"); segments != nil {
		t.Fatal("invalid UTF8 segmentation did not fail closed")
	}
}

func TestWebInputURLRetainsWholeBoundedHTTPSourceV1(t *testing.T) {
	const prefix = "https://example.invalid/"
	exact := prefix + strings.Repeat("x", WebRequestURLByteLimitV1-len(prefix))
	for _, value := range []string{"https://example.invalid/full/path?case=alice", "https://example.invalid/a b", "http://example.invalid/", exact} {
		if !ValidWebRequestURLV1(value) {
			t.Fatal("complete HTTP input rejected")
		}
	}
	for _, value := range []string{"", exact + "x", "javascript:alert(1)", "file:///tmp/test", "https://user:password@example.invalid/", "https://example.invalid/\n", "https://example.invalid/\x00", " https://example.invalid/", "https://"} {
		if ValidWebRequestURLV1(value) {
			t.Fatal("invalid or over-budget target admitted")
		}
	}
	tool := ToolBindingV1{CallID: "call", ToolName: "web_fetch", GrantID: strings.Repeat("1", 64), InputArgsHash: strings.Repeat("2", 64)}
	digest := WebInputDigestV1(tool, "https://example.invalid/request")
	other := tool
	other.InputArgsHash = strings.Repeat("3", 64)
	if digest == WebInputDigestV1(other, "https://example.invalid/request") || digest == WebInputDigestV1(tool, "https://example.invalid/other") {
		t.Fatal("input digest failed to bind actual source")
	}
	encoded, _ := json.Marshal(tool)
	if string(encoded) != "{}" {
		t.Fatal("private active grant/hash became JSON")
	}
}
