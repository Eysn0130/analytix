package toolcall

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PublicProjectionSchemaVersion = 1
	MetadataOnlyDisclosure        = "metadata_only"
	ProjectionWithheld            = "withheld"
	ArgumentsWithheldMessageKey   = "tool_arguments_withheld"
)

// PublicToolCallArgumentsProjectionV1 is the only arguments value allowed in
// ordinary durable history and across the public HTTP/SSE/desktop boundary.
// Exact provider arguments remain attempt-private and, only when a host gate
// must survive restart, inside the signed private continuation receipt.
type PublicToolCallArgumentsProjectionV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	ProjectionKind         string `json:"projectionKind"`
	Disclosure             string `json:"disclosure"`
	MessageKey             string `json:"messageKey"`
	PrivatePayloadWithheld bool   `json:"privatePayloadWithheld"`
	FactAnswerAllowed      bool   `json:"factAnswerAllowed"`
	EvidenceAuthority      bool   `json:"evidenceAuthority"`
}

func WithheldArgumentsProjectionV1() PublicToolCallArgumentsProjectionV1 {
	return PublicToolCallArgumentsProjectionV1{
		SchemaVersion:          PublicProjectionSchemaVersion,
		ProjectionKind:         ProjectionWithheld,
		Disclosure:             MetadataOnlyDisclosure,
		MessageKey:             ArgumentsWithheldMessageKey,
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
	}
}

func ValidatePublicToolCallArgumentsProjectionV1(projection PublicToolCallArgumentsProjectionV1) error {
	if projection.SchemaVersion != PublicProjectionSchemaVersion || projection.ProjectionKind != ProjectionWithheld ||
		projection.Disclosure != MetadataOnlyDisclosure || projection.MessageKey != ArgumentsWithheldMessageKey ||
		!projection.PrivatePayloadWithheld || projection.FactAnswerAllowed || projection.EvidenceAuthority {
		return errors.New("public tool call arguments projection is invalid")
	}
	return nil
}

func ParsePublicToolCallArgumentsProjectionV1(value any) (PublicToolCallArgumentsProjectionV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return PublicToolCallArgumentsProjectionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var projection PublicToolCallArgumentsProjectionV1
	if err := decoder.Decode(&projection); err != nil {
		return PublicToolCallArgumentsProjectionV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return PublicToolCallArgumentsProjectionV1{}, errors.New("public tool call arguments projection contains trailing data")
	}
	if err := ValidatePublicToolCallArgumentsProjectionV1(projection); err != nil {
		return PublicToolCallArgumentsProjectionV1{}, err
	}
	return projection, nil
}

func PublicToolCallArgumentsProjectionRecordV1() map[string]any {
	body, _ := json.Marshal(WithheldArgumentsProjectionV1())
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

// PublicToolCallItemRecordV1 drops execution authority, raw arguments, paths,
// summaries, media, and arbitrary extension fields. Pairing and lifecycle
// identity are the only tool-call metadata exposed publicly.
func PublicToolCallItemRecordV1(item map[string]any) map[string]any {
	projected := toolCallLifecycleRecordV1(item)
	turnID, callID := stringValue(item, "turnId"), stringValue(item, "callId")
	itemID := ToolCallItemIDV1(turnID, callID)
	if !domainmodel.IsHostToolCallIDV1(callID) || itemID == "" {
		return map[string]any{
			"kind": "tool_call", "status": "failed", "legacyIdentityWithheld": true,
			"arguments": PublicToolCallArgumentsProjectionRecordV1(),
		}
	}
	projected["id"] = itemID
	projected["callId"] = callID
	if projected["toolKind"] == "skill" {
		projected["toolKind"] = "tool_call"
	}
	projected["arguments"] = PublicToolCallArgumentsProjectionRecordV1()
	return projected
}

func toolCallLifecycleRecordV1(item map[string]any) map[string]any {
	projected := map[string]any{}
	for _, key := range []string{
		"id", "turnId", "threadId", "createdAt", "finishedAt",
		"toolName", "callId", "toolKind",
	} {
		if value, ok := item[key]; ok {
			projected[key] = cloneJSONValue(value)
		}
	}
	projected["kind"] = "tool_call"
	projected["role"] = "tool"
	status, valid := publicToolCallStatusV1(stringValue(item, "status"))
	projected["status"] = status
	if !valid {
		projected["lifecycleStatusWithheld"] = true
	}
	if toolKind := publicToolKindV1(stringValue(item, "toolKind")); toolKind != "" {
		projected["toolKind"] = toolKind
	} else {
		delete(projected, "toolKind")
	}
	return projected
}

func publicToolCallStatusV1(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "pending", "running", "completed", "failed", "aborted", "cancelled":
		return strings.TrimSpace(value), true
	default:
		return "failed", false
	}
}

func publicToolKindV1(value string) string {
	switch strings.TrimSpace(value) {
	case "tool_call", "file_change", "command_execution", "skill", "subagent":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

// PrivateDurableToolCallItemRecordV1 is the host-only persistence shape. It
// withholds exact provider arguments while retaining the host-issued grant
// needed to rebuild registry membership. Public projection must always use
// PublicToolCallItemRecordV1, which removes the grant as well.
func PrivateDurableToolCallItemRecordV1(item map[string]any) (map[string]any, bool) {
	projected := toolCallLifecycleRecordV1(item)
	projected["arguments"] = PublicToolCallArgumentsProjectionRecordV1()
	contextDigest := stringValue(item, "contextDigest")
	grantID := stringValue(item, "executionGrantId")
	if contextDigest == "" && grantID == "" && item["executionGrant"] == nil {
		return projected, true
	}
	if contextDigest == "" || grantID == "" || item["executionGrant"] == nil {
		return projected, false
	}
	grant, err := domainsecurity.ParseExecutionGrant(item["executionGrant"])
	if err != nil || grant.GrantID != grantID || grant.ContextDigest != contextDigest ||
		grant.TurnID != stringValue(item, "turnId") || grant.ToolName != stringValue(item, "toolName") ||
		grant.ToolCallID != stringValue(item, "callId") {
		return projected, false
	}
	arguments, found := item["arguments"]
	if !found {
		return projected, false
	}
	if _, err := ParsePublicToolCallArgumentsProjectionV1(arguments); err != nil {
		body, marshalErr := json.Marshal(arguments)
		if marshalErr != nil || domainsecurity.CanonicalJSONHash(body) != grant.ArgsHash {
			return projected, false
		}
	}
	projected["contextDigest"] = contextDigest
	projected["executionGrantId"] = grantID
	if contextEpoch, ok := item["contextEpoch"]; ok {
		projected["contextEpoch"] = cloneJSONValue(contextEpoch)
	}
	projected["executionGrant"] = cloneJSONValue(item["executionGrant"])
	return projected, true
}

func stringValue(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func cloneJSONValue(value any) any {
	body, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var cloned any
	if json.Unmarshal(body, &cloned) != nil {
		return nil
	}
	return cloned
}
