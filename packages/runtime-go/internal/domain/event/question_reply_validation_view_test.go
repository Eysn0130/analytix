package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	domainrestrictedevidence "analytix.local/runtime-go/internal/domain/restrictedevidence"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestQuestionReplyValidationViewMatchesFrozenCB940ASTAndTypesV1(t *testing.T) {
	body := questionReplyEventFixtureV1(t, 2, "literal 你好")["text"].(string)
	carrier := questionCollectorUserCarrierV1(body)
	opaqueCalls := 0
	for _, test := range []struct {
		name  string
		value any
	}{
		{"nil", nil}, {"bool", true}, {"string", body},
		{"numbers", []any{json.Number("1e0"), float64(2.5), float32(1.25), int(-1), int8(-2), int16(-3), int32(-4), int64(-5), uint(1), uint8(2), uint16(3), uint32(4), uint64(5), uintptr(6), complex64(1 + 2i), complex128(3 + 4i)}},
		{"nil-map", map[string]any(nil)}, {"nil-slice", []any(nil)},
		{"complete-ast", map[string]any{"root": []any{carrier, map[string]any{"unrelated": []any{true, nil, json.Number("7")}}}, "empty": map[string]any{}}},
		{"typed-map-is-leaf", map[string]string{"text": body}},
		{"named-map-is-leaf", questionCollectorNamedMapV1(carrier)},
		{"named-slice-is-leaf", questionCollectorNamedSliceV1{carrier}},
		{"typed-slice-is-leaf", []map[string]any{carrier}},
		{"array-is-leaf", [1]any{carrier}}, {"pointer-is-leaf", &carrier},
		{"raw-json-is-leaf", json.RawMessage(body)},
		{"custom-marshaler-is-not-called", questionCollectorOpaqueJSONV1{calls: &opaqueCalls}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := assertQuestionValidationFrozenCB940V1(t, test.value); err != nil {
				t.Fatalf("valid view rejected: %v", err)
			}
		})
	}
	if opaqueCalls != 0 {
		t.Fatalf("opaque marshaler called %d times", opaqueCalls)
	}
}

func TestQuestionReplyValidationViewKeepsCompleteASTRawBodiesAndIsolationV1(t *testing.T) {
	body := questionReplyEventFixtureV1(t, 3, "literal 你好 <example>")["text"].(string)
	raw := " \n\t" + body + "\n "
	value := map[string]any{"metadata": []any{json.Number("1e0"), "unchanged"}, "nested": []any{questionCollectorUserCarrierV1(raw)}}
	before := questionValidationInputCopyV1(value)
	got, err := assertQuestionValidationFrozenCB940V1(t, value)
	if err != nil {
		t.Fatal(err)
	}
	out := got.(map[string]any)
	if _, ok := out["metadata"].([]any)[0].(json.Number); !ok {
		t.Fatal("ordinary numeric type changed")
	}
	decoded, err := parseQuestionReplyContentV1(raw)
	if err != nil {
		t.Fatal(err)
	}
	item := out["nested"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(item["text"], decoded) || item["kind"] != "user_message" || item["delivery"] != "steer" {
		t.Fatal("validation view lost decoded body or carrier fields")
	}
	item["text"].(map[string]any)["answers"].([]any)[0].(map[string]any)["custom"] = "changed view only"
	out["metadata"].([]any)[1] = "changed view only"
	if !reflect.DeepEqual(value, before) || value["nested"].([]any)[0].(map[string]any)["text"] != raw {
		t.Fatal("validation view changed or replaced original encoded body")
	}
	again, err := questionReplyValidationViewV1(value)
	expected, _, oldErr := questionValidationFrozenStageCB940V1(value)
	if err != nil || oldErr != nil || !reflect.DeepEqual(again, expected) {
		t.Fatal("views shared mutable state")
	}
}

func TestQuestionReplyValidationViewKeepsFrozenCodecAndSecretErrorsV1(t *testing.T) {
	body := questionReplyEventFixtureV1(t, 3, "")["text"].(string)
	secret := questionReplyEventFixtureV1(t, 3, " api_key=sk-"+strings.Repeat("x", 20)+" ")["text"].(string)
	for _, test := range []struct {
		name  string
		value any
		want  error
	}{
		{"plain-unmatched", questionCollectorUserCarrierV1("ordinary message"), nil},
		{"malformed-marker-unmatched", questionCollectorUserCarrierV1("{"), nil},
		{"trailing-json-unmatched", questionCollectorUserCarrierV1(body + " {}"), nil},
		{"unknown-carrier-is-unmatched", map[string]any{"kind": "other", "text": body}, nil},
		{"unknown-closed-key", questionCollectorUserCarrierV1(strings.Replace(body, `"custom":`, `"unexpected":true,"custom":`, 1)), ErrQuestionReplyContentV1},
		{"duplicate-closed-key", questionCollectorUserCarrierV1(strings.Replace(body, `"custom":`, `"custom":"discarded","custom":`, 1)), ErrQuestionReplyContentV1},
		{"wrong-required-type", questionCollectorUserCarrierV1(strings.Replace(body, `"header":""`, `"header":null`, 1)), ErrQuestionReplyContentV1},
		{"null-selection", questionCollectorUserCarrierV1(strings.Replace(body, `"selected":[]`, `"selected":null`, 1)), ErrQuestionReplyContentV1},
		{"whole-leaf-secret", questionCollectorUserCarrierV1(secret), ErrCredentialProjection},
		{"nested-serialized-secret", questionCollectorUserCarrierV1(questionReplyEventFixtureV1(t, 3, `{"api_key":"synthetic-value"}`)["text"]), ErrCredentialProjection},
		{"encoded-byte-limit", questionCollectorUserCarrierV1("{" + strings.Repeat(" ", MaxQuestionReplyRecordBytesV1) + "}"), domainsecret.ErrProjectionLimitV1},
		{"decoded-leaf-limit", questionReplyEventFixtureV1(t, 1<<20+1, ""), domainsecret.ErrProjectionLimitV1},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := assertQuestionValidationFrozenCB940V1(t, test.value)
			if test.want == nil && err != nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("wrong error: %v want %v", err, test.want)
			}
			if err != nil && got != nil {
				t.Fatal("error returned a partial scan AST")
			}
			if errors.Is(err, ErrCredentialProjection) && !errors.Is(err, domainsecret.ErrCredentialMaterialV1) {
				t.Fatal("secret error lost its joined cause")
			}
		})
	}
}

func TestQuestionReplyValidationViewKeepsFrozenNodeAndDepthLimitsV1(t *testing.T) {
	for _, nodes := range []int{99999, 100000, 100001} {
		t.Run(fmt.Sprintf("nodes-%d", nodes), func(t *testing.T) {
			_, err := assertQuestionValidationFrozenCB940V1(t, make([]any, nodes-1))
			if nodes <= 100000 && err != nil || nodes > 100000 && !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
				t.Fatalf("node boundary changed: %v", err)
			}
		})
	}
	for _, depth := range []int{63, 64, 65} {
		t.Run(fmt.Sprintf("scalar-depth-%d", depth), func(t *testing.T) {
			_, err := assertQuestionValidationFrozenCB940V1(t, questionCollectorNestedV1(nil, depth))
			if depth <= 64 && err != nil || depth > 64 && !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
				t.Fatalf("depth boundary changed: %v", err)
			}
		})
	}
	body := questionReplyEventFixtureV1(t, 3, "")["text"].(string)
	carrier := questionCollectorUserCarrierV1(body)
	carrier["displayText"] = body
	for _, depth := range []int{63, 64} {
		t.Run(fmt.Sprintf("carrier-depth-%d", depth), func(t *testing.T) {
			_, err := assertQuestionValidationFrozenCB940V1(t, questionCollectorNestedV1(carrier, depth))
			if depth == 63 && err != nil || depth == 64 && !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
				t.Fatalf("matched slot/metadata depth changed: %v", err)
			}
		})
	}
	// Array + 99996 scalar nodes + carrier/kind/delivery exactly use 100000.
	// Matched text and displayText still do not consume additional walk nodes.
	exact := append(make([]any, 99996), carrier)
	if _, err := assertQuestionValidationFrozenCB940V1(t, exact); err != nil {
		t.Fatalf("matched slots acquired a node charge: %v", err)
	}
	if _, err := assertQuestionValidationFrozenCB940V1(t, append(exact, nil)); !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
		t.Fatalf("node limit widened: %v", err)
	}
}

func TestQuestionReplyValidationViewPreservesFrozenDurableSanitizerV1(t *testing.T) {
	body := questionCollectorUserCarrierV1(questionReplyEventFixtureV1(t, 3, "literal <think>user example</think>")["text"])
	secret := questionReplyEventFixtureV1(t, 3, " api_key=sk-"+strings.Repeat("x", 20)+" ")
	restricted := questionReplyEventFixtureV1(t, 3, `{"purpose":"analytix.source-row-lineage/v1"}`)
	for _, test := range []struct {
		name     string
		value    any
		accepted bool
	}{
		{"ordinary-thread", map[string]any{"id": "thread", "turns": []any{map[string]any{"id": "turn", "items": []any{map[string]any{"kind": "assistant_text", "text": "public<think>private</think>tail"}, map[string]any{"kind": "user_message", "text": "literal <think>user example</think>"}}}}}, true},
		{"closed-question", body, true},
		{"question-secret", secret, false}, {"question-restricted", restricted, false},
		{"invalid-reasoning-metadata", map[string]any{"kind": "usage", "usage": map[string]any{"reasoningTokens": "invalid"}}, false},
		{"private-reasoning-kind", map[string]any{"kind": "assistant_reasoning", "text": "synthetic"}, false},
		{"legacy-tool-call", map[string]any{"kind": "tool_call", "id": "item", "threadId": "thread", "turnId": "turn", "toolName": "read_file", "callId": "call", "status": "invented", "arguments": map[string]any{"path": "synthetic.txt"}}, true},
		{"closed-tool-result", map[string]any{"kind": "tool_result", "id": "item", "threadId": "thread", "turnId": "turn", "toolName": "read_file", "callId": "call", "toolKind": "tool_call", "status": "completed", "isError": false, "contextDigest": strings.Repeat("a", 64), "contextEpoch": uint64(7), "executionGrantId": strings.Repeat("b", 64), "output": domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"))}, true},
	} {
		for _, allow := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/user-text-%t", test.name, allow), func(t *testing.T) {
				before := questionValidationInputCopyV1(test.value)
				expected, oldOK := questionValidationFrozenDurableCB940V1(test.value, allow)
				got, ok := SanitizeDurableValue(test.value, allow)
				if oldOK != test.accepted || ok != oldOK || !reflect.DeepEqual(got, expected) {
					t.Fatalf("durable pipeline changed: ok=%t old=%t expected=%t", ok, oldOK, test.accepted)
				}
				if !reflect.DeepEqual(test.value, before) {
					t.Fatal("durable sanitation changed input")
				}
				if test.name == "closed-question" && got.(map[string]any)["text"] != body["text"] {
					t.Fatal("durable sanitation changed original encoded body")
				}
				if test.name == "ordinary-thread" {
					items := got.(map[string]any)["turns"].([]any)[0].(map[string]any)["items"].([]any)
					if items[0].(map[string]any)["text"] != "publictail" || items[1].(map[string]any)["text"] != "literal <think>user example</think>" {
						t.Fatal("provider/user text semantics changed")
					}
				}
			})
		}
	}
}

func TestQuestionReplyValidationViewKeepsStageRestoreAndResourceIsolationV1(t *testing.T) {
	raw := " \n" + questionReplyEventFixtureV1(t, 3, "")["text"].(string) + "\t "
	value := questionCollectorUserCarrierV1(raw)
	staged, restore, err := StageQuestionReplyContentV1(value)
	if err != nil {
		t.Fatal(err)
	}
	scan, err := questionReplyValidationViewV1(value)
	if err != nil {
		t.Fatal(err)
	}
	scan.(map[string]any)["text"].(map[string]any)["answers"].([]any)[0].(map[string]any)["custom"] = "scan-only mutation"
	restored, ok := restore(staged)
	if !ok || !reflect.DeepEqual(restored, value) || restored.(map[string]any)["text"] != raw {
		t.Fatal("scan affected Stage restoration or raw body bytes")
	}
	staged, restore, err = StageQuestionReplyContentV1(value)
	if err != nil {
		t.Fatal(err)
	}
	staged.(map[string]any)["text"].(map[string]any)["answers"].([]any)[0].(map[string]any)["custom"] = "tampered staged AST"
	if _, ok := restore(staged); ok {
		t.Fatal("Stage placeholder snapshot aliased its mutable AST")
	}
	if value["text"] != raw {
		t.Fatal("Stage mutation changed source body")
	}

	large := questionReplyEventFixtureV1(t, 400000, "")
	budget := 100000
	expected, oldErr := questionValidationFrozenWalkCB940V1(large, nil, nil, true, &budget, 0, nil)
	resource, err := QuestionReplyResourceViewV1(large)
	if err != nil || oldErr != nil || !reflect.DeepEqual(resource, expected) {
		t.Fatalf("resource view changed: err=%v old=%v", err, oldErr)
	}
	chunks, ok := resource.(map[string]any)["text"].([]any)
	if !ok || len(chunks) < 2 {
		t.Fatal("resource accounting lost chunked original bytes")
	}
	var joined strings.Builder
	for _, chunk := range chunks {
		text, ok := chunk.(string)
		if !ok || len(text) > 1<<20 {
			t.Fatal("resource chunk changed type or byte limit")
		}
		joined.WriteString(text)
	}
	if joined.String() != large["text"] {
		t.Fatal("resource chunks lost original encoded bytes")
	}
}

func assertQuestionValidationFrozenCB940V1(t *testing.T, value any) (any, error) {
	t.Helper()
	before := questionValidationInputCopyV1(value)
	expected, _, oldErr := questionValidationFrozenStageCB940V1(value)
	got, err := questionReplyValidationViewV1(value)
	if questionCollectorErrorKindsV1(err) != questionCollectorErrorKindsV1(oldErr) || !reflect.DeepEqual(got, expected) {
		t.Fatalf("view differs from frozen cb940 full-stage walker: err=%v old=%v", err, oldErr)
	}
	if err != nil && err.Error() != oldErr.Error() {
		t.Fatalf("error text differs: err=%v old=%v", err, oldErr)
	}
	if !reflect.DeepEqual(value, before) {
		t.Fatal("validation view or frozen oracle changed input")
	}
	return got, err
}

func questionValidationInputCopyV1(value any) any {
	switch current := value.(type) {
	case map[string]any:
		if current == nil {
			return map[string]any(nil)
		}
		out := make(map[string]any, len(current))
		for key, child := range current {
			out[key] = questionValidationInputCopyV1(child)
		}
		return out
	case []any:
		if current == nil {
			return []any(nil)
		}
		out := make([]any, len(current))
		for index, child := range current {
			out[index] = questionValidationInputCopyV1(child)
		}
		return out
	default:
		return value
	}
}

// Freeze the full-stage traversal and placeholder clone from publication.go at
// cb94073e15383393b4ad6bcf53b8210cb9b341a8. This oracle never calls the current
// questionReplyViewV1/walkQuestionReplyV1/StageQuestionReplyContentV1. Only the
// unchanged slot/closed-codec, domainsecret and durable-scanner boundaries are
// shared; it is not an independent reimplementation of those validators.
func questionValidationFrozenStageCB940V1(value any) (any, []closedPlanDigestProjectionEntryV1, error) {
	entries := []closedPlanDigestProjectionEntryV1{}
	budget := 100000
	staged, err := questionValidationFrozenWalkCB940V1(value, nil, &entries, false, &budget, 0, nil)
	return staged, entries, err
}

// This is the old durable pipeline, with its discarded-restore Stage replaced
// only by the frozen full-stage oracle above. The existing restriction,
// reasoning and item projection helpers remain the actual unchanged owners.
func questionValidationFrozenDurableCB940V1(value any, allowUserText bool) (any, bool) {
	original := value
	staged, _, err := questionValidationFrozenStageCB940V1(value)
	if err != nil || domainrestrictedevidence.Validate(staged) != nil || hasInvalidPublicReasoningMetadataV1(staged) {
		return nil, false
	}
	public, ok := sanitizeValue(original, allowUserText, true)
	if !ok {
		return nil, false
	}
	bodies := func(value any) ([]string, error) {
		_, entries, err := questionValidationFrozenStageCB940V1(value)
		if err != nil {
			return nil, err
		}
		texts := make([]string, len(entries))
		for index, entry := range entries {
			texts[index] = entry.digest
		}
		sort.Strings(texts)
		return texts, nil
	}
	left, leftErr := bodies(original)
	right, rightErr := bodies(public)
	if leftErr != nil || rightErr != nil || !reflect.DeepEqual(left, right) {
		return nil, false
	}
	return public, true
}

func questionValidationFrozenWalkCB940V1(value any, path []closedPlanDigestProjectionPathV1, entries *[]closedPlanDigestProjectionEntryV1, resource bool, budget *int, depth int, bodies *[]string) (any, error) {
	if *budget <= 0 {
		return nil, domainsecret.ErrProjectionLimitV1
	}
	*budget--
	if depth > 64 {
		return nil, domainsecret.ErrProjectionLimitV1
	}
	switch current := value.(type) {
	case map[string]any:
		var out map[string]any
		if bodies == nil {
			out = make(map[string]any, len(current))
		}
		for key, child := range current {
			var childPath []closedPlanDigestProjectionPathV1
			if bodies == nil {
				childPath = append(append([]closedPlanDigestProjectionPathV1(nil), path...), closedPlanDigestProjectionPathV1{kind: "key", key: key})
			}
			parsed, matched, err := questionReplySlotContentV1(current, key)
			if err != nil {
				return nil, err
			}
			if matched {
				text := child.(string)
				if resource {
					chunks := []any{}
					for len(text) > 0 {
						size := len(text)
						if size > 1<<20 {
							size = 1 << 20
						}
						chunks = append(chunks, text[:size])
						text = text[size:]
					}
					out[key] = chunks
				} else {
					if err := domainsecret.ValidateValueV1(parsed); err != nil {
						return nil, errors.Join(ErrCredentialProjection, err)
					}
					if bodies != nil {
						*bodies = append(*bodies, text)
					} else {
						out[key] = parsed
						*entries = append(*entries, closedPlanDigestProjectionEntryV1{path: childPath, digest: text, placeholder: questionValidationFrozenCloneCB940V1(parsed).(map[string]any)})
					}
				}
				continue
			}
			projected, err := questionValidationFrozenWalkCB940V1(child, childPath, entries, resource, budget, depth+1, bodies)
			if err != nil {
				return nil, err
			}
			if out != nil {
				out[key] = projected
			}
		}
		return out, nil
	case []any:
		var out []any
		if bodies == nil {
			out = make([]any, len(current))
		}
		for index, child := range current {
			var childPath []closedPlanDigestProjectionPathV1
			if bodies == nil {
				childPath = append(append([]closedPlanDigestProjectionPathV1(nil), path...), closedPlanDigestProjectionPathV1{kind: "index", index: index})
			}
			projected, err := questionValidationFrozenWalkCB940V1(child, childPath, entries, resource, budget, depth+1, bodies)
			if err != nil {
				return nil, err
			}
			if out != nil {
				out[index] = projected
			}
		}
		return out, nil
	default:
		return value, nil
	}
}

func questionValidationFrozenCloneCB940V1(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = questionValidationFrozenCloneCB940V1(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = questionValidationFrozenCloneCB940V1(child)
		}
		return out
	default:
		return value
	}
}

func TestQuestionReplyValidationViewPreservesFrozenPublicRecordValidationV1(t *testing.T) {
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	body := " \n" + questionReplyEventFixtureV1(t, 3, "literal 你好")["text"].(string) + "\t "
	secret := questionReplyEventFixtureV1(t, 3, " api_key=sk-"+strings.Repeat("x", 20)+" ")
	for _, test := range []struct {
		name  string
		value any
		want  []error
	}{
		{"closed-question-raw-body", questionCollectorUserCarrierV1(body), nil},
		{"ordinary-types", map[string]any{"kind": "thread_updated", "details": []any{json.Number("1e0"), float64(3), true, nil}}, nil},
		{"closed-codec", questionCollectorUserCarrierV1(strings.Replace(body, `"custom":`, `"custom":"discarded","custom":`, 1)), []error{ErrQuestionReplyContentV1}},
		{"question-credential", secret, []error{ErrCredentialProjection, domainsecret.ErrCredentialMaterialV1}},
		{"question-restricted", questionReplyEventFixtureV1(t, 3, `{"purpose":"analytix.source-row-lineage/v1"}`), []error{ErrRestrictedEvidenceProjection, domainrestrictedevidence.ErrRestrictedEvidence}},
		{"restricted-before-draft", map[string]any{"kind": "assistant_text_delta", "details": map[string]any{"purpose": "analytix.source-row-lineage/v1"}}, []error{ErrRestrictedEvidenceProjection, domainrestrictedevidence.ErrRestrictedEvidence}},
		{"question-pii", questionReplyEventFixtureV1(t, 3, " account number: 6222020202020202020 "), []error{ErrPrivacyProjection, domainprivacy.ErrRestrictedPII}},
		{"question-entity", questionReplyEventFixtureV1(t, 3, " "+string(reference)+" "), []error{ErrInternalEntityReferenceProjection, domaincaseentity.ErrPublicValueInternalReferenceV1}},
		{"incomplete-reasoning", map[string]any{"kind": "provider_error", "error": "public</thi"}, []error{ErrPrivateReasoningPersistence, domainreasoningmarkup.ErrIncomplete}},
		{"assistant-draft", map[string]any{"kind": "assistant_text_delta", "threadId": "thread-1", "turnId": "turn-1", "item": map[string]any{"kind": "assistant_text", "text": "unaccepted provider draft"}}, []error{ErrAssistantDraftPersistence}},
		{"depth-limit", questionCollectorNestedV1(nil, 65), []error{domainsecret.ErrProjectionLimitV1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := questionValidationInputCopyV1(test.value)
			oldErr := questionValidationFrozenPublicRecordCB940V1(test.value)
			err := ValidatePublicRecord(test.value)
			if (err == nil) != (oldErr == nil) || (err == nil) != (len(test.want) == 0) {
				t.Fatalf("public validation outcome changed: err=%v old=%v want=%v", err, oldErr, test.want)
			}
			// Compare all relevant outer kinds and joined causes, not only the
			// narrower collector error helper. Each scanner has one defective
			// leaf; the precedence fixture also exercises a later draft gate.
			for _, cause := range []error{ErrQuestionReplyContentV1, ErrPrivateReasoningPersistence, ErrAssistantDraftPersistence, ErrRestrictedEvidenceProjection, ErrCredentialProjection, ErrPrivacyProjection, ErrInternalEntityReferenceProjection, domainsecret.ErrCredentialMaterialV1, domainsecret.ErrProjectionLimitV1, domainrestrictedevidence.ErrRestrictedEvidence, domainrestrictedevidence.ErrInspectionLimit, domainprivacy.ErrRestrictedPII, domaincaseentity.ErrPublicValueInternalReferenceV1, domaincaseentity.ErrPublicValueReferenceInspectionV1, domainreasoningmarkup.ErrIncomplete} {
				if errors.Is(err, cause) != errors.Is(oldErr, cause) {
					t.Fatalf("public validation lost an error kind/joined cause: %v err=%v old=%v", cause, err, oldErr)
				}
			}
			for _, cause := range test.want {
				if !errors.Is(err, cause) || !errors.Is(oldErr, cause) {
					t.Fatalf("expected public rejection cause missing: %v err=%v old=%v", cause, err, oldErr)
				}
			}
			if err != nil && err.Error() != oldErr.Error() {
				t.Fatalf("single-defect public error text changed: err=%v old=%v", err, oldErr)
			}
			if !reflect.DeepEqual(test.value, before) {
				t.Fatal("public validation or frozen wrapper changed input")
			}
		})
	}
}

func TestQuestionReplyValidationViewPreservesFrozenPublicSanitizerV1(t *testing.T) {
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	raw := " \n\t" + questionReplyEventFixtureV1(t, 3, "literal <think>user example</think>")["text"].(string) + "\n "
	progressText := "before<think>hidden</think>after"
	for _, test := range []struct {
		name     string
		value    any
		accepted bool
	}{
		{"closed-question-raw-body", questionCollectorUserCarrierV1(raw), true},
		{"ordinary-types", map[string]any{"kind": "thread_updated", "details": []any{json.Number("1e0"), float64(3), true, nil}}, true},
		{"reasoning-cleanup", map[string]any{"kind": "provider_progress", "text": progressText, "reasoningContent": "private"}, true},
		{"user-literal", map[string]any{"kind": "user_message", "text": progressText}, true},
		{"private-reasoning-kind", map[string]any{"kind": "assistant_reasoning", "text": "private"}, false},
		{"closed-codec", questionCollectorUserCarrierV1(strings.Replace(raw, `"selected":[]`, `"selected":null`, 1)), false},
		{"question-credential", questionReplyEventFixtureV1(t, 3, " api_key=sk-"+strings.Repeat("x", 20)+" "), false},
		{"question-restricted", questionReplyEventFixtureV1(t, 3, `{"purpose":"analytix.source-row-lineage/v1"}`), false},
		{"final-entity-scan", map[string]any{"kind": "accepted_final_label", "details": map[string]any{"text": "已核验主体 " + string(reference)}}, false},
		{"final-question-entity-scan", questionReplyEventFixtureV1(t, 3, " "+string(reference)+" "), false},
	} {
		for _, allow := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/user-text-%t", test.name, allow), func(t *testing.T) {
				before := questionValidationInputCopyV1(test.value)
				expected, oldOK := questionValidationFrozenPublicSanitizerCB940V1(test.value, allow)
				got, ok := SanitizePublicValue(test.value, allow)
				if oldOK != test.accepted || ok != oldOK || !reflect.DeepEqual(got, expected) {
					t.Fatalf("public sanitation changed: ok=%t old=%t want=%t got=%#v expected=%#v", ok, oldOK, test.accepted, got, expected)
				}
				if !reflect.DeepEqual(test.value, before) {
					t.Fatal("public sanitation or frozen wrapper changed input")
				}
				if !ok {
					if got != nil {
						t.Fatal("rejected public sanitation returned a partial value")
					}
					return
				}
				public := got.(map[string]any)
				if test.name == "closed-question-raw-body" && public["text"] != raw {
					t.Fatal("public sanitation changed original closed body bytes")
				}
				if test.name == "reasoning-cleanup" {
					want := "beforeafter"
					if allow {
						want = progressText
					}
					if public["text"] != want || public["reasoningContent"] != nil {
						t.Fatal("public reasoning cleanup or allow-user-text semantics changed")
					}
				}
				if test.name == "user-literal" && public["text"] != progressText {
					t.Fatal("public sanitation rewrote a literal user message")
				}
			})
		}
	}
}

// Freeze the cb940 public outer pipelines while replacing their discarded
// Stage calls with the independent full-stage oracle already above. Resource
// accounting, scanners, privacy helpers, item sanitation and body collection
// remain the unchanged owners; these wrappers isolate the three Stage call
// substitutions, not an independent reimplementation of every public rule.
func questionValidationFrozenPublicRecordCB940V1(value any) error {
	if containsSettledProcessHistoryV1(value) {
		return ErrPrivateReasoningPersistence
	}
	original := value
	resource, err := QuestionReplyResourceViewV1(value)
	if err != nil {
		return err
	}
	if err := domainsecret.ValidateValueLimitsV1(resource); err != nil {
		return errors.Join(ErrCredentialProjection, err)
	}
	value, _, err = questionValidationFrozenStageCB940V1(value)
	if err != nil {
		return err
	}
	if err := domainrestrictedevidence.Validate(value); err != nil {
		return errors.Join(ErrRestrictedEvidenceProjection, err)
	}
	if err := privateReasoningError(original, false); err != nil {
		return err
	}
	if err := validateClosedFailureProjectionV1(value); err != nil {
		return err
	}
	if containsAssistantDraft(value) {
		return ErrAssistantDraftPersistence
	}
	if err := domainsecret.ValidateValueV1(value); err != nil {
		return errors.Join(ErrCredentialProjection, err)
	}
	if err := domainprivacy.ValidatePublicValue(publicPrivacyValidationViewV1(value)); err != nil {
		return errors.Join(ErrPrivacyProjection, err)
	}
	if err := domaincaseentity.ValidatePublicValueWithoutReferenceV1(value); err != nil {
		return errors.Join(ErrInternalEntityReferenceProjection, err)
	}
	return nil
}

func questionValidationFrozenPublicSanitizerCB940V1(value any, allowUserText bool) (any, bool) {
	original := value
	staged, _, err := questionValidationFrozenStageCB940V1(value)
	if err != nil {
		return nil, false
	}
	value = staged
	if domainrestrictedevidence.Validate(value) != nil {
		return nil, false
	}
	if hasInvalidPublicReasoningMetadataV1(value) {
		return nil, false
	}
	public, ok := sanitizeValue(original, allowUserText, false)
	if !ok || !sameQuestionReplyBodiesV1(original, public) {
		return nil, false
	}
	scan, _, err := questionValidationFrozenStageCB940V1(public)
	if err != nil || domaincaseentity.ValidatePublicValueWithoutReferenceV1(scan) != nil {
		return nil, false
	}
	return public, true
}
