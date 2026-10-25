package control

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

var ErrInvalidUserInputResponse = errors.New("invalid user input answers")

// UserInputAnswer is the closed client submission carrier. Its identifiers
// become host authority only after binding to the pending request below.
type UserInputAnswer struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	OptionIDs  []string `json:"optionIds,omitempty"`
	CustomText *string  `json:"customText,omitempty"`
}

// NormalizeUserInputAnswers validates the complete batch before a gate claim.
// legacyQuestionID is a receipt-bound singleton compatibility alias, never a
// typed-answer identifier or authority to select another question.
func NormalizeUserInputAnswers(request UserInputResponse, questions []map[string]any, legacyQuestionID string) ([]map[string]string, error) {
	invalid := func() ([]map[string]string, error) { return nil, ErrInvalidUserInputResponse }
	legacyQuestionID = strings.TrimSpace(legacyQuestionID)
	if request.Cancelled {
		if request.Answers != nil || request.StructuredAnswers != nil {
			return invalid()
		}
		return nil, nil
	}
	if len(questions) == 0 || len(questions) > 3 || (request.Answers != nil && request.StructuredAnswers != nil) {
		return invalid()
	}
	byID := make(map[string]map[string]any, len(questions))
	for _, question := range questions {
		id, _ := question["id"].(string)
		if id == "" || byID[id] != nil {
			return invalid()
		}
		byID[id] = question
	}
	normalized := make(map[string]map[string]string, len(questions))
	if request.StructuredAnswers != nil {
		if len(request.StructuredAnswers) != len(questions) {
			return invalid()
		}
		for _, answer := range request.StructuredAnswers {
			question := byID[answer.ID]
			if question == nil || normalized[answer.ID] != nil {
				return invalid()
			}
			if answer.Kind == "skip" {
				if answer.OptionIDs != nil || answer.CustomText != nil {
					return invalid()
				}
				normalized[answer.ID] = map[string]string{"id": answer.ID, "kind": "skip", "label": "Skipped", "value": "[skipped]"}
				continue
			}
			if answer.Kind != "answer" || answer.OptionIDs == nil {
				return invalid()
			}
			multiSelect, _ := question["multiSelect"].(bool)
			if !multiSelect && len(answer.OptionIDs) > 1 {
				return invalid()
			}
			options, _ := question["options"].([]map[string]string)
			optionRecords := make(map[string]map[string]string, len(options))
			for _, option := range options {
				optionRecords[option["id"]] = option
			}
			selected := make(map[string]bool, len(answer.OptionIDs))
			labels := make([]string, 0, len(answer.OptionIDs))
			selectedDetails := make([]map[string]string, 0, len(answer.OptionIDs))
			for _, optionID := range answer.OptionIDs {
				option, exists := optionRecords[optionID]
				if optionID == "" || !exists || selected[optionID] {
					return invalid()
				}
				selected[optionID] = true
				labels = append(labels, option["label"])
				selectedDetails = append(selectedDetails, map[string]string{"label": option["label"], "description": option["description"]})
			}
			customText := ""
			if answer.CustomText != nil {
				customText = strings.TrimSpace(*answer.CustomText)
				if customText == "" {
					return invalid()
				}
			}
			if !multiSelect && len(labels) > 0 && customText != "" {
				return invalid()
			}
			if len(labels) == 0 && customText == "" {
				return invalid()
			}
			// Keep the selected meaning when labels repeat or contain punctuation.
			// Only the continuation/model path consumes these projected records.
			selectedJSON, _ := json.Marshal(selectedDetails)
			value := strings.Join(labels, "\n")
			if customText != "" {
				if value != "" {
					value += "\n"
				}
				value += customText
			}
			record := map[string]string{"id": answer.ID, "kind": "answer", "label": strings.Join(labels, ", "), "value": value, "selectedOptions": string(selectedJSON)}
			if customText != "" {
				record["customText"] = customText
			}
			normalized[answer.ID] = record
		}
	} else {
		if len(request.Answers) != len(questions) {
			return invalid()
		}
		for _, answer := range request.Answers {
			for key := range answer {
				if key != "id" && key != "label" && key != "value" {
					return invalid()
				}
			}
			id := answer["id"]
			if len(questions) == 1 && (id == "q1" || (legacyQuestionID != "" && id == legacyQuestionID)) {
				id, _ = questions[0]["id"].(string)
			}
			if byID[id] == nil || normalized[id] != nil || strings.TrimSpace(answer["value"]) == "" {
				return invalid()
			}
			normalized[id] = map[string]string{"id": id, "label": answer["label"], "value": answer["value"]}
		}
	}
	out := make([]map[string]string, 0, len(questions))
	for _, question := range questions {
		id, _ := question["id"].(string)
		out = append(out, normalized[id])
	}
	return out, nil
}

// NormalizeAndCaptureUserInputAnswers keeps the model's existing normalization
// while privately retaining validated source values. It performs no I/O and
// returns no capture for invalid requests or cancellation.
func NormalizeAndCaptureUserInputAnswers(request UserInputResponse, questions []map[string]any, legacyQuestionID string) ([]map[string]string, *domaincontinuation.UserInputSubmissionV1, error) {
	normalized, err := NormalizeUserInputAnswers(request, questions, legacyQuestionID)
	if err != nil {
		return nil, nil, err
	}
	if request.Cancelled {
		return normalized, nil, nil
	}
	invalid := func() ([]map[string]string, *domaincontinuation.UserInputSubmissionV1, error) {
		return nil, nil, ErrInvalidUserInputResponse
	}
	validString := func(value string) bool {
		return utf8.ValidString(value) && len(value) <= domaincontinuation.MaxUserInputSubmissionBytesV1
	}
	submission := domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1}
	if request.StructuredAnswers != nil {
		submission.Format = "structured"
		captured := make([]domaincontinuation.UserInputStructuredAnswerV1, 0, len(request.StructuredAnswers))
		for _, answer := range request.StructuredAnswers {
			if !validString(answer.ID) || !validString(answer.Kind) {
				return invalid()
			}
			record := domaincontinuation.UserInputStructuredAnswerV1{ID: answer.ID, Kind: answer.Kind}
			if answer.OptionIDs != nil {
				options := make([]string, len(answer.OptionIDs))
				for index, optionID := range answer.OptionIDs {
					if !validString(optionID) {
						return invalid()
					}
					options[index] = optionID
				}
				record.OptionIDs = &options
			}
			if answer.CustomText != nil {
				if !validString(*answer.CustomText) {
					return invalid()
				}
				text := *answer.CustomText
				record.CustomText = &text
			}
			captured = append(captured, record)
		}
		submission.Answers, err = json.Marshal(captured)
	} else {
		submission.Format = "legacy"
		captured := make([]domaincontinuation.UserInputLegacyAnswerV1, 0, len(request.Answers))
		legacyQuestionID = strings.TrimSpace(legacyQuestionID)
		for _, answer := range request.Answers {
			for _, value := range answer {
				if !validString(value) {
					return invalid()
				}
			}
			questionID := answer["id"]
			if len(questions) == 1 && (questionID == "q1" || (legacyQuestionID != "" && questionID == legacyQuestionID)) {
				questionID, _ = questions[0]["id"].(string)
			}
			record := domaincontinuation.UserInputLegacyAnswerV1{ID: answer["id"], Value: answer["value"], QuestionID: questionID}
			if label, present := answer["label"]; present {
				value := label
				record.Label = &value
			}
			captured = append(captured, record)
		}
		submission.Answers, err = json.Marshal(captured)
	}
	if err != nil || domaincontinuation.ValidateUserInputSubmissionV1(submission) != nil {
		return invalid()
	}
	return normalized, &submission, nil
}
