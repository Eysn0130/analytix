package steering

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const ProjectionVersionV1 = 1

const maxSteeringTextBytes = 16 * 1024

type entryContentV1 struct {
	Version                 int                          `json:"version"`
	ID                      string                       `json:"id"`
	ClientUserMessageID     string                       `json:"clientUserMessageId"`
	Text                    string                       `json:"text"`
	DisplayText             string                       `json:"displayText"`
	AdmittedAt              string                       `json:"admittedAt"`
	Delivery                string                       `json:"delivery"`
	JobID                   string                       `json:"jobId"`
	ChildRunID              string                       `json:"childRunId"`
	SteerMessageID          string                       `json:"steerMessageId"`
	ParentThreadID          string                       `json:"parentThreadId"`
	ChildThreadID           string                       `json:"childThreadId"`
	SourceTurnID            string                       `json:"sourceTurnId"`
	SourceToolCallID        string                       `json:"sourceToolCallId"`
	JobProjectionVersion    int                          `json:"jobProjectionVersion"`
	JobContentDigest        string                       `json:"jobContentDigest"`
	JobContextDigest        string                       `json:"jobContextDigest"`
	JobAuthorityDigest      string                       `json:"jobAuthorityDigest"`
	JobQueueAuthorityDigest string                       `json:"jobQueueAuthorityDigest"`
	LogicalEffect           domainsecurity.LogicalEffect `json:"logicalEffect,omitempty"`
	OrdinaryWork            *bool                        `json:"ordinaryWork,omitempty"`
}

// EntryLogicalEffectBinding is host-owned steering intent metadata. It is
// signed as part of the existing V1 entry content but grants no data access.
type EntryLogicalEffectBinding struct {
	LogicalEffect domainsecurity.LogicalEffect
	OrdinaryWork  bool
}

var admissionFieldsV1 = map[string]struct{}{
	"id": {}, "clientUserMessageId": {}, "text": {}, "displayText": {}, "admittedAt": {}, "delivery": {},
	"jobId": {}, "childRunId": {}, "steerMessageId": {}, "parentThreadId": {}, "childThreadId": {},
	"sourceTurnId": {}, "sourceToolCallId": {},
	"jobProjectionVersion": {}, "jobContentDigest": {}, "jobContextDigest": {},
	"jobAuthorityDigest": {}, "jobQueueAuthorityDigest": {},
	"logicalEffect": {}, "ordinaryWork": {},
}

var pendingFieldsV1 = func() map[string]struct{} {
	out := make(map[string]struct{}, len(admissionFieldsV1)+7)
	for key := range admissionFieldsV1 {
		out[key] = struct{}{}
	}
	for _, key := range []string{"projectionVersion", "contentDigest", "contextDigest", "status"} {
		out[key] = struct{}{}
	}
	for _, key := range []string{"authorityKeyId", "authorityPublicKey", "authoritySignature"} {
		out[key] = struct{}{}
	}
	return out
}()

var promotedFieldsV1 = func() map[string]struct{} {
	out := make(map[string]struct{}, len(pendingFieldsV1)+5)
	for key := range pendingFieldsV1 {
		out[key] = struct{}{}
	}
	out["promotedAt"] = struct{}{}
	out["promotedItemId"] = struct{}{}
	out["promotionAuthorityKeyId"] = struct{}{}
	out["promotionAuthorityPublicKey"] = struct{}{}
	out["promotionAuthoritySignature"] = struct{}{}
	return out
}()

func EntryIDV1(turnID, clientUserMessageID string) string {
	turnID = strings.TrimSpace(turnID)
	clientUserMessageID = strings.TrimSpace(clientUserMessageID)
	if turnID == "" || clientUserMessageID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("analytix/steering-entry-id/v1\x00" + turnID + "\x00" + clientUserMessageID))
	return "item_steer_" + hex.EncodeToString(digest[:])
}

func BindPendingEntryV1(raw map[string]any, contextDigest string) (map[string]any, error) {
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(contextDigest)) {
		return nil, errors.New("steering context digest is invalid")
	}
	content, err := parseEntryContentV1(raw, admissionFieldsV1)
	if err != nil {
		return nil, err
	}
	if content.JobID != "" && content.JobContextDigest != strings.TrimSpace(contextDigest) {
		return nil, errors.New("task-job steering context authority is invalid")
	}
	out := entryContentMapV1(content)
	out["projectionVersion"] = ProjectionVersionV1
	out["contentDigest"] = contentDigestV1(content)
	out["contextDigest"] = strings.TrimSpace(contextDigest)
	out["status"] = "pending"
	return out, nil
}

func ValidatePendingEntryForContextV1(raw map[string]any, contextDigest string) error {
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(contextDigest)) || len(raw) == 0 {
		return errors.New("pending steering context is invalid")
	}
	content, err := parseEntryContentV1(raw, pendingFieldsV1)
	if err != nil {
		return err
	}
	if !isProjectionVersionV1(raw["projectionVersion"]) || strings.TrimSpace(stringField(raw, "status")) != "pending" ||
		strings.TrimSpace(stringField(raw, "contextDigest")) != strings.TrimSpace(contextDigest) ||
		strings.TrimSpace(stringField(raw, "contentDigest")) != contentDigestV1(content) {
		return errors.New("pending steering binding is invalid")
	}
	if content.JobID != "" && content.JobContextDigest != strings.TrimSpace(contextDigest) {
		return errors.New("pending task-job steering context is invalid")
	}
	canonical := entryContentMapV1(content)
	canonical["projectionVersion"] = ProjectionVersionV1
	canonical["contentDigest"] = contentDigestV1(content)
	canonical["contextDigest"] = strings.TrimSpace(contextDigest)
	canonical["status"] = "pending"
	if !appendAuthorityCanonicalV1(raw, canonical) {
		return errors.New("pending steering authority material is invalid")
	}
	if !sameCanonicalEntryMapV1(raw, canonical) {
		return errors.New("pending steering projection is not canonical")
	}
	return nil
}

func ValidatePromotedEntryForContextV1(raw map[string]any, contextDigest string) error {
	canonical, err := promotedEntryCanonicalBaseV1(raw, contextDigest)
	if err != nil {
		return errors.New("promoted steering authority material is invalid")
	}
	material, err := promotionAuthorityMaterialV1(raw, canonical)
	if err != nil || !appendPromotionAuthorityCanonicalV1(raw, canonical) {
		return errors.New("promoted steering authority material is invalid")
	}
	if !sameCanonicalEntryMapV1(raw, canonical) {
		return errors.New("promoted steering projection is not canonical")
	}
	if !ed25519.Verify(ed25519.PublicKey(material.PublicKey), material.SigningBytes, material.Signature) {
		return errors.New("promoted steering authority signature is invalid")
	}
	return nil
}

func promotedEntryCanonicalBaseV1(raw map[string]any, contextDigest string) (map[string]any, error) {
	contextDigest = strings.TrimSpace(contextDigest)
	if !domainsecurity.IsSHA256Hex(contextDigest) || len(raw) == 0 {
		return nil, errors.New("promoted steering context is invalid")
	}
	content, err := parseEntryContentV1(raw, promotedFieldsV1)
	if err != nil {
		return nil, err
	}
	promotedAt := stringField(raw, "promotedAt")
	promotedItemID := stringField(raw, "promotedItemId")
	if !isProjectionVersionV1(raw["projectionVersion"]) || stringField(raw, "status") != "promoted" ||
		stringField(raw, "contextDigest") != contextDigest || stringField(raw, "contentDigest") != contentDigestV1(content) ||
		promotedItemID != content.ID || promotedAt == "" || !canonicalRFC3339NanoUTC(promotedAt) ||
		(content.JobID != "" && content.JobContextDigest != contextDigest) {
		return nil, errors.New("promoted steering binding is invalid")
	}
	canonical := entryContentMapV1(content)
	canonical["projectionVersion"] = ProjectionVersionV1
	canonical["contentDigest"] = contentDigestV1(content)
	canonical["contextDigest"] = contextDigest
	canonical["status"] = "promoted"
	canonical["promotedAt"] = promotedAt
	canonical["promotedItemId"] = promotedItemID
	if !appendAuthorityCanonicalV1(raw, canonical) {
		return nil, errors.New("promoted steering admission authority is invalid")
	}
	base := cloneAuthorityMapV1(raw)
	delete(base, "promotionAuthorityKeyId")
	delete(base, "promotionAuthorityPublicKey")
	delete(base, "promotionAuthoritySignature")
	if !sameCanonicalEntryMapV1(base, canonical) {
		return nil, errors.New("promoted steering base projection is not canonical")
	}
	return canonical, nil
}

func ValidatePromotedItemForContextV1(entry, item map[string]any, threadID, turnID, contextDigest string) error {
	if ValidatePromotedEntryForContextV1(entry, contextDigest) != nil || len(item) == 0 {
		return errors.New("promoted steering item authority is invalid")
	}
	content, err := parseEntryContentV1(entry, promotedFieldsV1)
	if err != nil {
		return errors.New("promoted steering item content is invalid")
	}
	expected := promotedItemCanonicalV1(content, entry, strings.TrimSpace(threadID), strings.TrimSpace(turnID), strings.TrimSpace(contextDigest))
	if content.JobID == "" {
		expected["steeringOrigin"] = "ordinary"
	} else {
		expected["steeringOrigin"] = "task_job"
		expected["jobId"] = content.JobID
		expected["childRunId"] = content.ChildRunID
		expected["steerMessageId"] = content.SteerMessageID
	}
	if !sameCanonicalEntryMapV1(item, expected) {
		return errors.New("promoted steering item does not match its exact host projection")
	}
	return nil
}

func promotedItemCanonicalV1(content entryContentV1, entry map[string]any, threadID, turnID, contextDigest string) map[string]any {
	promotedAt := stringField(entry, "promotedAt")
	out := map[string]any{
		"id": content.ID, "turnId": turnID, "threadId": threadID, "role": "user", "status": "completed",
		"createdAt": content.AdmittedAt, "finishedAt": promotedAt, "kind": "user_message", "text": content.Text,
		"delivery": "steer", "contextDigest": contextDigest, "steeringProjectionVersion": ProjectionVersionV1,
		"steeringContentDigest": contentDigestV1(content), "clientUserMessageId": content.ClientUserMessageID,
	}
	if content.DisplayText != "" && content.DisplayText != content.Text {
		out["displayText"] = content.DisplayText
	}
	return out
}

func SamePendingEntryV1(left, right map[string]any) bool {
	leftContext := strings.TrimSpace(stringField(left, "contextDigest"))
	return leftContext != "" && leftContext == strings.TrimSpace(stringField(right, "contextDigest")) &&
		strings.TrimSpace(stringField(left, "contentDigest")) != "" &&
		strings.TrimSpace(stringField(left, "contentDigest")) == strings.TrimSpace(stringField(right, "contentDigest")) &&
		stringField(left, "authorityKeyId") == stringField(right, "authorityKeyId") &&
		stringField(left, "authorityPublicKey") == stringField(right, "authorityPublicKey") &&
		stringField(left, "authoritySignature") == stringField(right, "authoritySignature") &&
		ValidatePendingEntryForContextV1(left, leftContext) == nil && ValidatePendingEntryForContextV1(right, leftContext) == nil
}

// LogicalEffectBindingFromEntryV1 returns whether the optional host binding is
// present. Absence is valid only for legacy V1 entries; callers decide the
// conservative interpretation from the frozen turn context. A partial or
// malformed binding always fails closed. This accessor does not verify entry
// authority and must be used only after the surrounding entry/set validation.
func LogicalEffectBindingFromEntryV1(raw map[string]any) (EntryLogicalEffectBinding, bool, error) {
	effect, ordinaryWork, present, err := parseLogicalEffectBindingV1(raw)
	if err != nil || !present {
		return EntryLogicalEffectBinding{}, present, err
	}
	return EntryLogicalEffectBinding{LogicalEffect: effect, OrdinaryWork: ordinaryWork}, true, nil
}

func parseEntryContentV1(raw map[string]any, allowed map[string]struct{}) (entryContentV1, error) {
	if len(raw) == 0 {
		return entryContentV1{}, errors.New("steering entry is empty")
	}
	for key := range raw {
		if _, found := allowed[key]; !found {
			return entryContentV1{}, errors.New("steering entry contains an unknown field")
		}
	}
	content := entryContentV1{
		Version: ProjectionVersionV1, ID: strings.TrimSpace(stringField(raw, "id")),
		ClientUserMessageID: strings.TrimSpace(stringField(raw, "clientUserMessageId")),
		Text:                strings.TrimSpace(stringField(raw, "text")), DisplayText: strings.TrimSpace(stringField(raw, "displayText")),
		AdmittedAt: strings.TrimSpace(stringField(raw, "admittedAt")), Delivery: strings.TrimSpace(stringField(raw, "delivery")),
		JobID: strings.TrimSpace(stringField(raw, "jobId")), ChildRunID: strings.TrimSpace(stringField(raw, "childRunId")),
		SteerMessageID: strings.TrimSpace(stringField(raw, "steerMessageId")), ParentThreadID: strings.TrimSpace(stringField(raw, "parentThreadId")),
		ChildThreadID: strings.TrimSpace(stringField(raw, "childThreadId")), SourceTurnID: strings.TrimSpace(stringField(raw, "sourceTurnId")),
		SourceToolCallID:        strings.TrimSpace(stringField(raw, "sourceToolCallId")),
		JobProjectionVersion:    exactIntField(raw, "jobProjectionVersion"),
		JobContentDigest:        strings.TrimSpace(stringField(raw, "jobContentDigest")),
		JobContextDigest:        strings.TrimSpace(stringField(raw, "jobContextDigest")),
		JobAuthorityDigest:      strings.TrimSpace(stringField(raw, "jobAuthorityDigest")),
		JobQueueAuthorityDigest: strings.TrimSpace(stringField(raw, "jobQueueAuthorityDigest")),
	}
	logicalEffect, ordinaryWork, hasLogicalEffect, err := parseLogicalEffectBindingV1(raw)
	if err != nil {
		return entryContentV1{}, err
	}
	if hasLogicalEffect {
		content.LogicalEffect = logicalEffect
		content.OrdinaryWork = &ordinaryWork
	}
	if content.ID == "" {
		content.ID = content.ClientUserMessageID
	}
	if content.ClientUserMessageID == "" {
		content.ClientUserMessageID = content.ID
	}
	if content.ID == "" || content.ClientUserMessageID == "" || content.Text == "" || len(content.Text) > maxSteeringTextBytes ||
		content.AdmittedAt == "" || (content.Delivery != "" && content.Delivery != "steer") {
		return entryContentV1{}, errors.New("steering entry content is invalid")
	}
	if !canonicalRFC3339NanoUTC(content.AdmittedAt) {
		return entryContentV1{}, errors.New("steering admission time is invalid")
	}
	content.Delivery = "steer"
	hasTaskJobLineage := content.JobID != "" || content.ChildRunID != "" || content.SteerMessageID != "" ||
		content.ParentThreadID != "" || content.ChildThreadID != ""
	if hasTaskJobLineage && (content.JobID == "" || content.JobID != content.ChildRunID || content.SteerMessageID == "" ||
		content.ParentThreadID == "" || content.ChildThreadID == "") {
		return entryContentV1{}, errors.New("task-job steering lineage is invalid")
	}
	if !hasTaskJobLineage && (content.SourceTurnID != "" || content.SourceToolCallID != "") {
		return entryContentV1{}, errors.New("steering source provenance requires task-job lineage")
	}
	if hasTaskJobLineage && (content.JobProjectionVersion != ProjectionVersionV1 ||
		!domainsecurity.IsSHA256Hex(content.JobContentDigest) || !domainsecurity.IsSHA256Hex(content.JobContextDigest) ||
		!domainsecurity.IsSHA256Hex(content.JobAuthorityDigest) || !domainsecurity.IsSHA256Hex(content.JobQueueAuthorityDigest)) {
		return entryContentV1{}, errors.New("task-job steering authority binding is invalid")
	}
	if !hasTaskJobLineage && (content.JobProjectionVersion != 0 || content.JobContentDigest != "" || content.JobContextDigest != "" ||
		content.JobAuthorityDigest != "" || content.JobQueueAuthorityDigest != "") {
		return entryContentV1{}, errors.New("ordinary steering cannot carry task-job authority")
	}
	for key, value := range raw {
		if _, found := allowed[key]; !found || key == "projectionVersion" {
			continue
		}
		if key == "projectionVersion" || key == "jobProjectionVersion" {
			if !isExactJSONInteger(value) {
				return entryContentV1{}, errors.New("steering version field type is invalid")
			}
			continue
		}
		if key == "ordinaryWork" {
			if _, ok := value.(bool); !ok {
				return entryContentV1{}, errors.New("steering logical effect field type is invalid")
			}
			continue
		}
		if key == "status" || key == "contentDigest" || key == "contextDigest" || key == "promotedAt" || key == "promotedItemId" {
			if _, ok := value.(string); !ok {
				return entryContentV1{}, errors.New("steering authority field type is invalid")
			}
			continue
		}
		if _, ok := value.(string); !ok {
			return entryContentV1{}, errors.New("steering entry field type is invalid")
		}
	}
	return content, nil
}

func entryContentMapV1(content entryContentV1) map[string]any {
	out := map[string]any{
		"id": content.ID, "clientUserMessageId": content.ClientUserMessageID, "text": content.Text,
		"admittedAt": content.AdmittedAt, "delivery": content.Delivery,
	}
	for key, value := range map[string]string{
		"displayText": content.DisplayText, "jobId": content.JobID, "childRunId": content.ChildRunID,
		"steerMessageId": content.SteerMessageID, "parentThreadId": content.ParentThreadID,
		"childThreadId": content.ChildThreadID, "sourceTurnId": content.SourceTurnID, "sourceToolCallId": content.SourceToolCallID,
		"jobContentDigest": content.JobContentDigest, "jobContextDigest": content.JobContextDigest,
		"jobAuthorityDigest": content.JobAuthorityDigest, "jobQueueAuthorityDigest": content.JobQueueAuthorityDigest,
	} {
		if value != "" {
			out[key] = value
		}
	}
	if content.JobProjectionVersion != 0 {
		out["jobProjectionVersion"] = content.JobProjectionVersion
	}
	if content.OrdinaryWork != nil {
		out["logicalEffect"] = string(content.LogicalEffect)
		out["ordinaryWork"] = *content.OrdinaryWork
	}
	return out
}

func parseLogicalEffectBindingV1(raw map[string]any) (domainsecurity.LogicalEffect, bool, bool, error) {
	rawEffect, hasEffect := raw["logicalEffect"]
	rawOrdinaryWork, hasOrdinaryWork := raw["ordinaryWork"]
	if hasEffect != hasOrdinaryWork {
		return "", false, false, errors.New("steering logical effect binding is partial")
	}
	if !hasEffect {
		return "", false, false, nil
	}
	effectValue, effectIsString := rawEffect.(string)
	ordinaryWork, ordinaryWorkIsBool := rawOrdinaryWork.(bool)
	effect := domainsecurity.LogicalEffect(effectValue)
	if !effectIsString || !ordinaryWorkIsBool || domainsecurity.ValidateLogicalEffect(effect) != nil {
		return "", false, false, errors.New("steering logical effect binding is invalid")
	}
	return effect, ordinaryWork, true, nil
}

func contentDigestV1(content entryContentV1) string {
	payload, _ := json.Marshal(content)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func rawStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func exactIntField(record map[string]any, key string) int {
	switch value := record[key].(type) {
	case int:
		return value
	case float64:
		if value == float64(int(value)) {
			return int(value)
		}
	}
	return 0
}

func isExactJSONInteger(raw any) bool {
	switch value := raw.(type) {
	case int:
		return true
	case float64:
		return value == float64(int(value))
	default:
		return false
	}
}

func canonicalRFC3339NanoUTC(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC && parsed.UTC().Format(time.RFC3339Nano) == value
}

func sameCanonicalEntryMapV1(raw, canonical map[string]any) bool {
	if len(raw) != len(canonical) {
		return false
	}
	for key, expected := range canonical {
		actual, found := raw[key]
		if !found {
			return false
		}
		switch typed := expected.(type) {
		case int:
			if !isExactJSONInteger(actual) || exactIntField(map[string]any{"value": actual}, "value") != typed {
				return false
			}
		case string:
			value, ok := actual.(string)
			if !ok || value != typed {
				return false
			}
		case bool:
			value, ok := actual.(bool)
			if !ok || value != typed {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func isProjectionVersionV1(raw any) bool {
	switch value := raw.(type) {
	case int:
		return value == ProjectionVersionV1
	case float64:
		return value == float64(ProjectionVersionV1)
	default:
		return false
	}
}
