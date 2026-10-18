package control

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

func userInputAnswerText(value string) *string { return &value }

func userInputQuestionFixture() []map[string]any {
	return []map[string]any{
		{"id": "input_host_1", "multiSelect": true, "options": []map[string]string{{"id": "input_host_1_option_1", "label": "Same", "description": "First meaning"}, {"id": "input_host_1_option_2", "label": "Same", "description": "Second meaning"}}},
		{"id": "input_host_2", "multiSelect": false, "options": []map[string]string{{"id": "input_host_2_option_1", "label": "Ship"}, {"id": "input_host_2_option_2", "label": "Wait"}}},
		{"id": "input_host_3", "multiSelect": false, "options": []map[string]string{}},
	}
}

func TestNormalizeUserInputAnswersKeepsBatchSemanticsAndCoreOrder(t *testing.T) {
	request := UserInputResponse{StructuredAnswers: []UserInputAnswer{
		{ID: "input_host_3", Kind: "skip"},
		{ID: "input_host_2", Kind: "answer", OptionIDs: []string{"input_host_2_option_1"}},
		{ID: "input_host_1", Kind: "answer", OptionIDs: []string{"input_host_1_option_1", "input_host_1_option_2"}, CustomText: userInputAnswerText("  add context  ")},
	}}
	answers, err := NormalizeUserInputAnswers(request, userInputQuestionFixture(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != 3 || answers[0]["id"] != "input_host_1" || answers[1]["value"] != "Ship" || answers[2]["kind"] != "skip" || answers[2]["value"] != "[skipped]" {
		t.Fatalf("complete batch semantics lost: %#v", answers)
	}
	if answers[0]["selectedOptions"] != `[{"description":"First meaning","label":"Same"},{"description":"Second meaning","label":"Same"}]` || answers[0]["customText"] != "add context" {
		t.Fatalf("multi-select/custom semantics lost: %#v", answers[0])
	}
	if *request.StructuredAnswers[2].CustomText != "  add context  " {
		t.Fatal("submission mutated")
	}
}

func TestNormalizeUserInputAnswersRejectsInvalidBatch(t *testing.T) {
	valid := []UserInputAnswer{
		{ID: "input_host_1", Kind: "answer", OptionIDs: []string{"input_host_1_option_1"}},
		{ID: "input_host_2", Kind: "answer", OptionIDs: []string{"input_host_2_option_1"}},
		{ID: "input_host_3", Kind: "skip"},
	}
	tests := []struct {
		name   string
		change func(*UserInputResponse)
	}{
		{"missing question", func(r *UserInputResponse) { r.StructuredAnswers = r.StructuredAnswers[:2] }},
		{"duplicate question", func(r *UserInputResponse) { r.StructuredAnswers[2] = r.StructuredAnswers[0] }},
		{"foreign question", func(r *UserInputResponse) { r.StructuredAnswers[0].ID = "input_other_1" }},
		{"typed legacy alias", func(r *UserInputResponse) { r.StructuredAnswers[0].ID = "q1" }},
		{"foreign option", func(r *UserInputResponse) { r.StructuredAnswers[0].OptionIDs = []string{"input_other_1_option_1"} }},
		{"cross question option", func(r *UserInputResponse) { r.StructuredAnswers[0].OptionIDs = []string{"input_host_2_option_1"} }},
		{"duplicate option", func(r *UserInputResponse) {
			r.StructuredAnswers[0].OptionIDs = []string{"input_host_1_option_1", "input_host_1_option_1"}
		}},
		{"single select with custom", func(r *UserInputResponse) { r.StructuredAnswers[1].CustomText = userInputAnswerText("custom") }},
		{"single select cardinality", func(r *UserInputResponse) {
			r.StructuredAnswers[1].OptionIDs = []string{"input_host_2_option_1", "input_host_2_option_2"}
		}},
		{"implicit blank skip", func(r *UserInputResponse) {
			r.StructuredAnswers[2] = UserInputAnswer{ID: "input_host_3", Kind: "answer", OptionIDs: []string{}, CustomText: userInputAnswerText(" \n ")}
		}},
		{"empty answer", func(r *UserInputResponse) {
			r.StructuredAnswers[2] = UserInputAnswer{ID: "input_host_3", Kind: "answer", OptionIDs: []string{}}
		}},
		{"missing option list", func(r *UserInputResponse) { r.StructuredAnswers[0].OptionIDs = nil }},
		{"skip with option list", func(r *UserInputResponse) { r.StructuredAnswers[2].OptionIDs = []string{} }},
		{"skip with custom text", func(r *UserInputResponse) { r.StructuredAnswers[2].CustomText = userInputAnswerText("ignored") }},
		{"unknown answer kind", func(r *UserInputResponse) { r.StructuredAnswers[2].Kind = "cancel" }},
		{"cancel and answers", func(r *UserInputResponse) { r.Cancelled = true }},
		{"mixed carriers", func(r *UserInputResponse) { r.Answers = []map[string]string{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := UserInputResponse{StructuredAnswers: append([]UserInputAnswer(nil), valid...)}
			test.change(&request)
			if _, err := NormalizeUserInputAnswers(request, userInputQuestionFixture(), "q1"); !errors.Is(err, ErrInvalidUserInputResponse) {
				t.Fatalf("invalid batch admitted: %v", err)
			}
		})
	}
}

func TestNormalizeUserInputAnswersLegacySingletonAliasesAreBounded(t *testing.T) {
	questions := userInputQuestionFixture()[2:]
	for _, id := range []string{"input_host_3", "q1", "old-question"} {
		request := UserInputResponse{Answers: []map[string]string{{"id": id, "label": "Custom", "value": "answer"}}}
		answers, err := NormalizeUserInputAnswers(request, questions, "old-question")
		if err != nil || !reflect.DeepEqual(answers, []map[string]string{{"id": "input_host_3", "label": "Custom", "value": "answer"}}) {
			t.Fatalf("legacy singleton %q: %#v %v", id, answers, err)
		}
	}
	for _, request := range []UserInputResponse{
		{Answers: []map[string]string{{"id": "foreign", "value": "answer"}}},
		{Answers: []map[string]string{{"id": "q1", "value": " "}}},
		{Answers: []map[string]string{{"id": "q1", "value": "answer", "authority": "client"}}},
		{StructuredAnswers: []UserInputAnswer{{ID: "q1", Kind: "skip"}}},
	} {
		if _, err := NormalizeUserInputAnswers(request, questions, "old-question"); !errors.Is(err, ErrInvalidUserInputResponse) {
			t.Fatalf("unbound singleton admitted: %#v %v", request, err)
		}
	}
	request := UserInputResponse{Answers: []map[string]string{{"id": "q1", "value": "answer"}, {"id": "input_host_2", "value": "answer"}, {"id": "input_host_3", "value": "answer"}}}
	if _, err := NormalizeUserInputAnswers(request, userInputQuestionFixture(), "q1"); !errors.Is(err, ErrInvalidUserInputResponse) {
		t.Fatal("legacy alias selected one of several questions")
	}
	if answers, err := NormalizeUserInputAnswers(UserInputResponse{Cancelled: true}, questions, ""); err != nil || answers != nil {
		t.Fatalf("cancel: %#v %v", answers, err)
	}
}

func TestNormalizeUserInputAnswersAcceptsCustomOnlyFreeform(t *testing.T) {
	questions := userInputQuestionFixture()[2:]
	answers, err := NormalizeUserInputAnswers(UserInputResponse{StructuredAnswers: []UserInputAnswer{{ID: "input_host_3", Kind: "answer", OptionIDs: []string{}, CustomText: userInputAnswerText("  context  ")}}}, questions, "")
	if err != nil || len(answers) != 1 || answers[0]["customText"] != "context" || answers[0]["selectedOptions"] != "[]" || answers[0]["value"] != "context" {
		t.Fatalf("freeform custom answer failed: %#v %v", answers, err)
	}
}

func TestNormalizeUserInputAnswersRetainsChosenDescriptionForDuplicateLabels(t *testing.T) {
	questions := userInputQuestionFixture()[:1]
	answers, err := NormalizeUserInputAnswers(UserInputResponse{StructuredAnswers: []UserInputAnswer{{ID: "input_host_1", Kind: "answer", OptionIDs: []string{"input_host_1_option_2"}}}}, questions, "")
	if err != nil || answers[0]["selectedOptions"] != `[{"description":"Second meaning","label":"Same"}]` {
		t.Fatalf("chosen meaning was lost behind duplicate labels: %#v %v", answers, err)
	}
}

func TestCaptureUserInputAnswersPreservesSourceValuesAndEmptyOptionPresence(t *testing.T) {
	rawText := "  original@example.invalid\n  "
	request := UserInputResponse{StructuredAnswers: []UserInputAnswer{
		{ID: "input_host_1", Kind: "answer", OptionIDs: []string{"input_host_1_option_2", "input_host_1_option_1"}, CustomText: &rawText},
		{ID: "input_host_2", Kind: "skip"},
		{ID: "input_host_3", Kind: "answer", OptionIDs: []string{}, CustomText: userInputAnswerText("  自定义  ")},
	}}
	normalized, submission, err := NormalizeAndCaptureUserInputAnswers(request, userInputQuestionFixture(), "")
	if err != nil || submission == nil {
		t.Fatal(err)
	}
	if normalized[0]["customText"] != "original@example.invalid" {
		t.Fatal("existing model normalization changed")
	}
	request.StructuredAnswers[0].OptionIDs[0] = "mutated"
	rawText = "mutated"
	var records []domaincontinuation.UserInputStructuredAnswerV1
	if json.Unmarshal(submission.Answers, &records) != nil || *records[0].CustomText != "  original@example.invalid\n  " || (*records[0].OptionIDs)[0] != "input_host_1_option_2" || records[1].OptionIDs != nil || records[2].OptionIDs == nil || len(*records[2].OptionIDs) != 0 {
		t.Fatal("capture aliased input or lost original values")
	}
}

func TestCaptureLegacyAnswersBindsCoreQuestionWithoutChangingOriginalID(t *testing.T) {
	questions := userInputQuestionFixture()[2:]
	for _, id := range []string{"q1", "old-question", "input_host_3"} {
		t.Run(id, func(t *testing.T) {
			request := UserInputResponse{Answers: []map[string]string{{"id": id, "value": "  original  "}}}
			_, submission, err := NormalizeAndCaptureUserInputAnswers(request, questions, "old-question")
			var records []domaincontinuation.UserInputLegacyAnswerV1
			if err != nil || submission == nil || submission.Format != "legacy" || json.Unmarshal(submission.Answers, &records) != nil || len(records) != 1 || records[0].ID != id || records[0].QuestionID != "input_host_3" || records[0].Value != "  original  " || records[0].Label != nil {
				t.Fatal("legacy source or Core binding changed", err)
			}
		})
	}
	request := UserInputResponse{Answers: []map[string]string{{"id": "input_host_2", "label": " Same ", "value": " two "}, {"id": "input_host_1", "value": " one "}, {"id": "input_host_3", "value": " three "}}}
	_, submission, err := NormalizeAndCaptureUserInputAnswers(request, userInputQuestionFixture(), "")
	var records []domaincontinuation.UserInputLegacyAnswerV1
	if err != nil || json.Unmarshal(submission.Answers, &records) != nil || records[0].QuestionID != "input_host_2" || records[1].QuestionID != "input_host_1" || *records[0].Label != " Same " {
		t.Fatal("legacy batch binding guessed order or label", err)
	}
}

func TestCaptureAnswersRejectsNonUTF8AndByteLimitBeforePersistence(t *testing.T) {
	questions := userInputQuestionFixture()[2:]
	for name, text := range map[string]string{"invalid UTF8": string([]byte{0xff}), "UTF8 byte limit": strings.Repeat("界", domaincontinuation.MaxUserInputSubmissionBytesV1/3+1), "JSON total limit": strings.Repeat("x", domaincontinuation.MaxUserInputSubmissionBytesV1)} {
		t.Run(name, func(t *testing.T) {
			request := UserInputResponse{StructuredAnswers: []UserInputAnswer{{ID: "input_host_3", Kind: "answer", OptionIDs: []string{}, CustomText: &text}}}
			if _, submission, err := NormalizeAndCaptureUserInputAnswers(request, questions, ""); !errors.Is(err, ErrInvalidUserInputResponse) || submission != nil {
				t.Fatal("invalid private capture admitted")
			}
		})
	}
	if _, submission, err := NormalizeAndCaptureUserInputAnswers(UserInputResponse{Cancelled: true}, questions, ""); err != nil || submission != nil {
		t.Fatal("cancellation acquired an answer record")
	}
}
