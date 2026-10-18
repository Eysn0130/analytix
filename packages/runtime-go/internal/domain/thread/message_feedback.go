// Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
// Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md.
package thread

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const MessageFeedbackNoteMaxBytesV1 = 8192
const MessageFeedbackPutKindV1 = "message_feedback_put"
const MessageFeedbackDeleteKindV1 = "message_feedback_delete"

var ErrMessageFeedbackInvalidV1 = errors.New("message feedback is invalid")
var ErrMessageFeedbackTargetNotFoundV1 = errors.New("message feedback target not found")
var ErrMessageFeedbackNoteBlankV1 = errors.New("message feedback note is blank")
var ErrMessageFeedbackNoteTooLargeV1 = errors.New("message feedback note exceeds byte limit")
var messageFeedbackVersionV1 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type MessageFeedbackItemV1 struct {
	MessageID string  `json:"messageId"`
	Rating    string  `json:"rating"`
	Note      *string `json:"note,omitempty"`
	Category  *string `json:"category,omitempty"`
	Version   string  `json:"version"`
	CreatedAt int64   `json:"createdAt"`
	UpdatedAt int64   `json:"updatedAt"`
}

type MessageFeedbackPutRequestV1 struct {
	Rating    string  `json:"rating"`
	Note      *string `json:"note,omitempty"`
	Category  *string `json:"category,omitempty"`
	IfVersion *string `json:"ifVersion"`
}

type MessageFeedbackVersionConflictV1 struct{ Current *MessageFeedbackItemV1 }

func (e *MessageFeedbackVersionConflictV1) Error() string { return "message feedback version conflict" }

func IsMessageFeedbackEventV1(event map[string]any) bool {
	kind, _ := event["kind"].(string)
	return kind == MessageFeedbackPutKindV1 || kind == MessageFeedbackDeleteKindV1
}

func ValidateMessageFeedbackPutV1(request MessageFeedbackPutRequestV1) error {
	if request.Rating != "positive" && request.Rating != "negative" {
		return ErrMessageFeedbackInvalidV1
	}
	if request.IfVersion != nil && !messageFeedbackVersionV1.MatchString(*request.IfVersion) {
		return ErrMessageFeedbackInvalidV1
	}
	if request.Note != nil {
		if !utf8.ValidString(*request.Note) {
			return ErrMessageFeedbackInvalidV1
		}
		if strings.TrimSpace(*request.Note) == "" {
			return ErrMessageFeedbackNoteBlankV1
		}
		if len(*request.Note) > MessageFeedbackNoteMaxBytesV1 {
			return ErrMessageFeedbackNoteTooLargeV1
		}
	}
	if request.Category != nil {
		switch *request.Category {
		case "task-result", "instruction-following", "product-interaction", "service-stability", "resource-cost", "security-privacy-permission", "other":
		default:
			return ErrMessageFeedbackInvalidV1
		}
	}
	return nil
}

func ValidateMessageFeedbackVersionV1(version string) error {
	if !messageFeedbackVersionV1.MatchString(version) {
		return ErrMessageFeedbackInvalidV1
	}
	return nil
}

func ValidateMessageFeedbackItemV1(item MessageFeedbackItemV1) error {
	if !IsCanonicalRecordID(item.MessageID) || len(item.MessageID) > 256 || item.CreatedAt < 0 || item.UpdatedAt < item.CreatedAt || item.UpdatedAt > 9007199254740991 {
		return ErrMessageFeedbackInvalidV1
	}
	if err := ValidateMessageFeedbackVersionV1(item.Version); err != nil {
		return err
	}
	return ValidateMessageFeedbackPutV1(MessageFeedbackPutRequestV1{Rating: item.Rating, Note: item.Note, Category: item.Category})
}

func DecodeMessageFeedbackV1(body []byte, target any) error {
	if domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: 65536, MaxDepth: 4, MaxTokens: 64, MaxStringBytes: MessageFeedbackNoteMaxBytesV1}) != nil {
		return ErrMessageFeedbackInvalidV1
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrMessageFeedbackInvalidV1
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrMessageFeedbackInvalidV1
	}
	return nil
}

func ParseMessageFeedbackItemV1(value any) (MessageFeedbackItemV1, error) {
	var item MessageFeedbackItemV1
	body, err := json.Marshal(value)
	if err != nil {
		return item, ErrMessageFeedbackInvalidV1
	}
	fields, err := domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{MaxBytes: 65536, MaxDepth: 4, MaxTokens: 64, MaxStringBytes: MessageFeedbackNoteMaxBytesV1})
	if err != nil {
		return item, ErrMessageFeedbackInvalidV1
	}
	for _, key := range []string{"messageId", "rating", "version", "createdAt", "updatedAt"} {
		if value, exists := fields[key]; !exists || value == nil {
			return item, ErrMessageFeedbackInvalidV1
		}
	}
	for _, key := range []string{"note", "category"} {
		if value, exists := fields[key]; exists && value == nil {
			return item, ErrMessageFeedbackInvalidV1
		}
	}
	if err := DecodeMessageFeedbackV1(body, &item); err != nil {
		return item, err
	}
	return item, ValidateMessageFeedbackItemV1(item)
}

// These records belong only to the canonical log. They never become turn items.
// The lifecycle validator checks the complete payload, not merely the new kind.
func ValidateMessageFeedbackEventV1(event map[string]any) error {
	if !IsMessageFeedbackEventV1(event) {
		return ErrMessageFeedbackInvalidV1
	}
	allowed := map[string]bool{"kind": true, "threadId": true, "threadCreatedAt": true, "seq": true, "timestamp": true}
	if event["kind"] == MessageFeedbackPutKindV1 {
		allowed["feedback"] = true
	} else {
		allowed["messageId"] = true
	}
	for key := range event {
		if !allowed[key] {
			return ErrMessageFeedbackInvalidV1
		}
	}
	threadID, _ := event["threadId"].(string)
	createdAt, _ := event["threadCreatedAt"].(string)
	if !IsCanonicalRecordID(threadID) || createdAt == "" {
		return ErrMessageFeedbackInvalidV1
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return ErrMessageFeedbackInvalidV1
	}
	timestamp, _ := event["timestamp"].(string)
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		return ErrMessageFeedbackInvalidV1
	}
	seq := int64(0)
	switch value := event["seq"].(type) {
	case int:
		seq = int64(value)
	case int64:
		seq = value
	case float64:
		seq = int64(value)
		if value != float64(seq) {
			return ErrMessageFeedbackInvalidV1
		}
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return ErrMessageFeedbackInvalidV1
		}
		seq = parsed
	default:
		return ErrMessageFeedbackInvalidV1
	}
	if seq < 1 || seq > 9007199254740991 {
		return ErrMessageFeedbackInvalidV1
	}
	if event["kind"] == MessageFeedbackPutKindV1 {
		_, err := ParseMessageFeedbackItemV1(event["feedback"])
		return err
	}
	messageID, _ := event["messageId"].(string)
	if !IsCanonicalRecordID(messageID) || len(messageID) > 256 {
		return ErrMessageFeedbackInvalidV1
	}
	return nil
}

func MessageFeedbackCurrentV1(events []map[string]any, threadID, createdAt string) ([]MessageFeedbackItemV1, error) {
	items := map[string]MessageFeedbackItemV1{}
	order := []string{}
	seen := map[string]bool{}
	for _, event := range events {
		if !IsMessageFeedbackEventV1(event) {
			continue
		}
		if err := ValidateMessageFeedbackEventV1(event); err != nil {
			return nil, err
		}
		if event["threadId"] != threadID || event["threadCreatedAt"] != createdAt {
			return nil, ErrMessageFeedbackInvalidV1
		}
		if event["kind"] == MessageFeedbackDeleteKindV1 {
			delete(items, event["messageId"].(string))
			continue
		}
		item, err := ParseMessageFeedbackItemV1(event["feedback"])
		if err != nil {
			return nil, err
		}
		if !seen[item.MessageID] {
			seen[item.MessageID] = true
			order = append(order, item.MessageID)
		}
		items[item.MessageID] = item
	}
	out := make([]MessageFeedbackItemV1, 0, len(items))
	for _, id := range order {
		if item, ok := items[id]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}
