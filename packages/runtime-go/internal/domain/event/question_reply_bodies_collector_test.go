package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"

	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
)

func TestQuestionReplyBodiesCollectorMatchesFullWalkerForValueTypesV1(t *testing.T) {
	body := questionReplyEventFixtureV1(t, 2, "")["text"].(string)
	carrier := questionCollectorUserCarrierV1(body)
	marshalCalls := 0
	for _, test := range []struct {
		name  string
		value any
		want  []string
	}{
		{"nil", nil, []string{}},
		{"boolean", true, []string{}},
		{"string-is-not-a-carrier", body, []string{}},
		{"number", json.Number("3"), []string{}},
		{"integers", []any{int(-1), int8(-2), int16(-3), int32(-4), int64(-5), uint(1), uint8(2), uint16(3), uint32(4), uint64(5), uintptr(6)}, []string{}},
		{"floats-and-complex", []any{float32(1.25), float64(2.5), math.NaN(), math.Inf(1), complex64(1 + 2i), complex128(3 + 4i)}, []string{}},
		{"empty-map", map[string]any{}, []string{}},
		{"nil-map", map[string]any(nil), []string{}},
		{"empty-slice", []any{}, []string{}},
		{"nil-slice", []any(nil), []string{}},
		{"map-and-slice", map[string]any{"nested": []any{nil, carrier}}, []string{body}},
		{"typed-map-is-opaque", map[string]string{"kind": "user_message", "delivery": "steer", "text": body}, []string{}},
		{"non-string-map-is-opaque", map[any]any{"nested": carrier}, []string{}},
		{"named-map-is-opaque", questionCollectorNamedMapV1(carrier), []string{}},
		{"named-slice-is-opaque", questionCollectorNamedSliceV1{carrier}, []string{}},
		{"typed-slice-is-opaque", []map[string]any{carrier}, []string{}},
		{"array-is-opaque", [1]any{carrier}, []string{}},
		{"pointer-is-opaque", &carrier, []string{}},
		{"raw-json-is-opaque", json.RawMessage(body), []string{}},
		{"struct-is-opaque", struct{ Content any }{Content: carrier}, []string{}},
		{"function-is-opaque", func() any { return carrier }, []string{}},
		{"channel-is-opaque", make(chan any), []string{}},
		{"custom-marshaler-is-opaque", questionCollectorOpaqueJSONV1{calls: &marshalCalls}, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := assertQuestionCollectorMatchesFullWalkerV1(t, test.value)
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("unexpected bodies for %s: got=%q want=%q err=%v", test.name, got, test.want, err)
			}
			if got == nil {
				t.Fatal("successful empty collection became nil")
			}
		})
	}
	if marshalCalls != 0 {
		t.Fatalf("opaque custom JSON value was serialized %d times", marshalCalls)
	}
}

func TestQuestionReplyBodiesCollectorPreservesCarriersBytesDuplicatesAndInputV1(t *testing.T) {
	steered := questionReplyEventFixtureV1(t, 3, "literal 你好 <example>")
	original := steered["text"].(string)
	spaced := " \n\t" + original + "\n "
	escaped := strings.Replace(original, `"question_reply_content"`, `"\u0071uestion_reply_content"`, 1)
	value := map[string]any{
		"nested": []any{
			steered,
			questionCollectorUserCarrierV1(spaced),
			map[string]any{
				"delivery": "steer", "userInputReplyIdentity": false, "userInputId": "", "userInputCaptureReceiptId": 0,
				"text": escaped, "displayText": original,
			},
			questionCollectorUserCarrierV1(original),
			map[string]any{"kind": "user_message", "delivery": "normal", "text": original},
			map[string]any{"kind": "turn_steered", "threadId": "thread", "turnId": "turn", "text": original},
		},
	}
	before := cloneQuestionReplyASTV1(value)
	got, err := assertQuestionCollectorMatchesFullWalkerV1(t, value)
	want := []string{original, spaced, escaped, original, original}
	sort.Strings(want)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("original bytes, duplicate occupancy or carrier recognition changed: got=%q want=%q err=%v", got, want, err)
	}
	if !reflect.DeepEqual(value, before) {
		t.Fatal("body collection changed the input")
	}
	got[0] = "changed caller-owned result"
	again, err := QuestionReplyBodiesV1(value)
	if err != nil || !reflect.DeepEqual(again, want) || !reflect.DeepEqual(value, before) {
		t.Fatalf("returned collection aliased later results or input: bodies=%q err=%v", again, err)
	}
}

func TestQuestionReplyBodiesCollectorKeepsSlotAndClosedCodecErrorsV1(t *testing.T) {
	body := questionReplyEventFixtureV1(t, 3, "")["text"].(string)
	secretBody := questionReplyEventFixtureV1(t, 3, " api_key=sk-"+strings.Repeat("x", 20)+" ")["text"].(string)
	nestedSecretBody := questionReplyEventFixtureV1(t, 3, `{"api_key":"synthetic-value"}`)["text"].(string)
	for _, test := range []struct {
		name  string
		value any
		want  []string
		err   error
	}{
		{"valid", questionCollectorUserCarrierV1(body), []string{body}, nil},
		{"ordinary-scalar", questionCollectorUserCarrierV1("ordinary message"), []string{}, nil},
		{"malformed-marker-json-is-unmatched", questionCollectorUserCarrierV1("{"), []string{}, nil},
		{"trailing-json-is-unmatched", questionCollectorUserCarrierV1(body + " {}"), []string{}, nil},
		{"unknown-kind-is-unmatched", questionCollectorUserCarrierV1(strings.Replace(body, "question_reply_content", "other_content", 1)), []string{}, nil},
		{"wrong-marker-type-is-unmatched", questionCollectorUserCarrierV1(strings.Replace(body, `"question_reply_content"`, "3", 1)), []string{}, nil},
		{"array-text-is-unmatched", questionCollectorUserCarrierV1("[" + body + "]"), []string{}, nil},
		{"missing-text", map[string]any{"kind": "user_message", "delivery": "steer"}, []string{}, nil},
		{"nonstring-text-still-walks-its-child", questionCollectorUserCarrierV1(questionCollectorUserCarrierV1(body)), []string{body}, nil},
		{"unknown-closed-key", questionCollectorUserCarrierV1(strings.Replace(body, `"custom":`, `"unexpected":true,"custom":`, 1)), nil, ErrQuestionReplyContentV1},
		{"duplicate-closed-key", questionCollectorUserCarrierV1(strings.Replace(body, `"custom":`, `"custom":"discarded","custom":`, 1)), nil, ErrQuestionReplyContentV1},
		{"wrong-required-type", questionCollectorUserCarrierV1(strings.Replace(body, `"header":""`, `"header":null`, 1)), nil, ErrQuestionReplyContentV1},
		{"null-selection", questionCollectorUserCarrierV1(strings.Replace(body, `"selected":[]`, `"selected":null`, 1)), nil, ErrQuestionReplyContentV1},
		{"unknown-selection", questionCollectorUserCarrierV1(strings.Replace(body, `"selected":[]`, `"selected":["not-offered"]`, 1)), nil, ErrQuestionReplyContentV1},
		{"oversize-before-marker-decode", questionCollectorUserCarrierV1("{" + strings.Repeat(" ", MaxQuestionReplyRecordBytesV1) + "}"), nil, domainsecret.ErrProjectionLimitV1},
		{"credential-in-whole-string", questionCollectorUserCarrierV1(secretBody), nil, ErrCredentialProjection},
		{"credential-in-serialized-string", questionCollectorUserCarrierV1(nestedSecretBody), nil, ErrCredentialProjection},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := cloneQuestionReplyASTV1(test.value)
			got, err := assertQuestionCollectorMatchesFullWalkerV1(t, test.value)
			if !reflect.DeepEqual(got, test.want) || (test.err == nil && err != nil) || (test.err != nil && !errors.Is(err, test.err)) {
				t.Fatalf("slot/error behavior changed: bodies=%q want=%q err=%v wantErr=%v", got, test.want, err, test.err)
			}
			if errors.Is(err, ErrCredentialProjection) && !errors.Is(err, domainsecret.ErrCredentialMaterialV1) {
				t.Fatal("credential error lost the joined scanner error")
			}
			if !reflect.DeepEqual(test.value, before) {
				t.Fatal("body collection changed malformed or rejected input")
			}
		})
	}
}

func TestQuestionReplyBodiesCollectorKeepsPublicNodeAndDepthBoundariesV1(t *testing.T) {
	for _, nodes := range []int{99999, 100000, 100001} {
		t.Run(fmt.Sprintf("nodes-%d", nodes), func(t *testing.T) {
			got, err := assertQuestionCollectorMatchesFullWalkerV1(t, make([]any, nodes-1))
			if nodes <= 100000 {
				if err != nil || got == nil || len(got) != 0 {
					t.Fatalf("allowed node boundary rejected: nodes=%d bodies=%q err=%v", nodes, got, err)
				}
			} else if got != nil || !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
				t.Fatalf("node budget widened: nodes=%d bodies=%q err=%v", nodes, got, err)
			}
		})
	}
	for _, depth := range []int{63, 64, 65} {
		t.Run(fmt.Sprintf("scalar-depth-%d", depth), func(t *testing.T) {
			got, err := assertQuestionCollectorMatchesFullWalkerV1(t, questionCollectorNestedV1(nil, depth))
			if depth <= 64 {
				if err != nil || got == nil || len(got) != 0 {
					t.Fatalf("allowed depth rejected: depth=%d bodies=%q err=%v", depth, got, err)
				}
			} else if got != nil || !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
				t.Fatalf("depth limit widened: depth=%d bodies=%q err=%v", depth, got, err)
			}
		})
	}
	body := questionReplyEventFixtureV1(t, 3, "")["text"].(string)
	dual := questionCollectorUserCarrierV1(body)
	dual["displayText"] = body
	for _, depth := range []int{63, 64} {
		t.Run(fmt.Sprintf("matched-carrier-depth-%d", depth), func(t *testing.T) {
			got, err := assertQuestionCollectorMatchesFullWalkerV1(t, questionCollectorNestedV1(dual, depth))
			if depth == 63 {
				// Metadata children are at depth 64. The matched text slots are
				// validated as closed bodies and their ASTs are not walked again.
				if err != nil || !reflect.DeepEqual(got, []string{body, body}) {
					t.Fatalf("matched slot acquired extra recursive depth: bodies=%q err=%v", got, err)
				}
			} else if got != nil || !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
				t.Fatalf("carrier metadata at depth 65 bypassed the limit: bodies=%q err=%v", got, err)
			}
		})
	}
	// One slice node, 99996 scalar nodes and three carrier nodes exactly use
	// the 100000 budget. Both matched text slots consume no extra walk node.
	value := append(make([]any, 99996), dual)
	if got, err := assertQuestionCollectorMatchesFullWalkerV1(t, value); err != nil || !reflect.DeepEqual(got, []string{body, body}) {
		t.Fatalf("matched slot consumed extra node budget: bodies=%q err=%v", got, err)
	}
}

func TestQuestionReplyBodiesCollectorKeepsBudgetDepthOrderV1(t *testing.T) {
	body := questionReplyEventFixtureV1(t, 3, "")["text"].(string)
	dual := questionCollectorUserCarrierV1(body)
	dual["displayText"] = body
	for _, test := range []struct {
		name      string
		value     any
		budget    int
		depth     int
		remaining int
		want      []string
		limited   bool
	}{
		{"budget-before-depth", nil, 0, 65, 0, nil, true},
		{"negative-budget-before-depth", nil, -1, 65, -1, nil, true},
		{"depth-after-charge", nil, 2, 65, 1, nil, true},
		{"scalar-depth-64", nil, 1, 64, 0, []string{}, false},
		{"scalar-depth-65", nil, 1, 65, 0, nil, true},
		{"matched-slots-with-exact-budget", dual, 3, 63, 0, []string{body, body}, false},
		{"matched-slots-with-short-budget", dual, 2, 63, 0, nil, true},
		{"opaque-array-does-not-recurse", [1]any{dual}, 1, 64, 0, []string{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			legacyBudget, collectorBudget := test.budget, test.budget
			entries := []closedPlanDigestProjectionEntryV1{}
			_, legacyErr := questionReplyViewV1(test.value, make([]closedPlanDigestProjectionPathV1, test.depth), &entries, false, &legacyBudget)
			got := []string{}
			_, err := walkQuestionReplyV1(test.value, nil, nil, false, &collectorBudget, test.depth, &got)
			if questionCollectorErrorKindsV1(err) != questionCollectorErrorKindsV1(legacyErr) || collectorBudget != legacyBudget || collectorBudget != test.remaining {
				t.Fatalf("walk order changed: collectorErr=%v legacyErr=%v remaining=%d legacyRemaining=%d wantRemaining=%d", err, legacyErr, collectorBudget, legacyBudget, test.remaining)
			}
			if test.limited {
				if !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
					t.Fatalf("expected walk limit: %v", err)
				}
				return
			}
			sort.Strings(got)
			if err != nil || !reflect.DeepEqual(got, test.want) || !reflect.DeepEqual(got, questionCollectorEntryTextsV1(entries)) {
				t.Fatalf("small-budget result differs: bodies=%q want=%q err=%v", got, test.want, err)
			}
		})
	}
}

func TestQuestionReplyBodiesCollectorKeepsOrderedFailurePrecedenceV1(t *testing.T) {
	secretBody := questionReplyEventFixtureV1(t, 3, " sk-"+strings.Repeat("x", 20)+" ")["text"].(string)
	secret := questionCollectorUserCarrierV1(secretBody)
	tooDeep := questionCollectorNestedV1(nil, 65)
	for _, test := range []struct {
		name  string
		value any
		err   error
	}{
		{"secret-before-depth", []any{secret, tooDeep}, ErrCredentialProjection},
		{"depth-before-secret", []any{tooDeep, secret}, domainsecret.ErrProjectionLimitV1},
		{"codec-before-depth", []any{questionCollectorUserCarrierV1(strings.Replace(secretBody, `"selected":[]`, `"selected":null`, 1)), tooDeep}, ErrQuestionReplyContentV1},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := assertQuestionCollectorMatchesFullWalkerV1(t, test.value)
			if got != nil || !errors.Is(err, test.err) {
				t.Fatalf("ordered failure changed: bodies=%q err=%v wantErr=%v", got, err, test.err)
			}
		})
	}
}

func assertQuestionCollectorMatchesFullWalkerV1(t *testing.T, value any) ([]string, error) {
	t.Helper()
	entries := []closedPlanDigestProjectionEntryV1{}
	budget := 100000
	// The existing complete Stage(false) path is the oracle. Keep its AST,
	// path and placeholder construction rather than duplicating the walker.
	_, legacyErr := questionReplyViewV1(value, nil, &entries, false, &budget)
	var legacy []string
	if legacyErr == nil {
		legacy = questionCollectorEntryTextsV1(entries)
	}
	got, err := QuestionReplyBodiesV1(value)
	if questionCollectorErrorKindsV1(err) != questionCollectorErrorKindsV1(legacyErr) || !reflect.DeepEqual(got, legacy) {
		t.Fatalf("collector differs from complete walker: bodies=%q legacy=%q err=%v legacyErr=%v", got, legacy, err, legacyErr)
	}
	if err != nil && err.Error() != legacyErr.Error() {
		t.Fatalf("collector changed error text: err=%v legacyErr=%v", err, legacyErr)
	}
	return got, err
}

func questionCollectorEntryTextsV1(entries []closedPlanDigestProjectionEntryV1) []string {
	texts := make([]string, len(entries))
	for index, entry := range entries {
		texts[index] = entry.digest
	}
	sort.Strings(texts)
	return texts
}

func questionCollectorErrorKindsV1(err error) string {
	if err == nil {
		return ""
	}
	kinds := []string{}
	for _, candidate := range []error{ErrQuestionReplyContentV1, ErrCredentialProjection, domainsecret.ErrCredentialMaterialV1, domainsecret.ErrProjectionLimitV1} {
		if errors.Is(err, candidate) {
			kinds = append(kinds, candidate.Error())
		}
	}
	if len(kinds) == 0 {
		return err.Error()
	}
	return strings.Join(kinds, ";")
}

func questionCollectorUserCarrierV1(text any) map[string]any {
	return map[string]any{"kind": "user_message", "delivery": "steer", "text": text}
}

func questionCollectorNestedV1(value any, depth int) any {
	for range depth {
		value = []any{value}
	}
	return value
}

type questionCollectorNamedMapV1 map[string]any
type questionCollectorNamedSliceV1 []any

type questionCollectorOpaqueJSONV1 struct{ calls *int }

func (value questionCollectorOpaqueJSONV1) MarshalJSON() ([]byte, error) {
	*value.calls++
	return nil, errors.New("opaque value must not be serialized")
}
