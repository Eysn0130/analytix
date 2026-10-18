package toolresult

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// WebFetchStatusV1 contains only measured response metadata. It deliberately
// excludes URLs, titles, page text, snippets, and arbitrary error details.
// Nil measurements mean the producer did not observe that value.
type WebFetchStatusV1 struct {
	Outcome    string `json:"outcome"`
	StatusCode *int64 `json:"statusCode,omitempty"`
	ByteCount  *int64 `json:"byteCount,omitempty"`
	Truncated  *bool  `json:"truncated,omitempty"`
}

func ValidateWebFetchStatusV1(web WebFetchStatusV1) error {
	if web.StatusCode != nil && (*web.StatusCode < 100 || *web.StatusCode > 599) {
		return errors.New("web fetch HTTP status is invalid")
	}
	if web.ByteCount != nil && (*web.ByteCount < 0 || *web.ByteCount > 9007199254740991 || web.StatusCode == nil) {
		return errors.New("web fetch byte count is invalid")
	}
	switch web.Outcome {
	case "success", "http_error":
		if web.StatusCode == nil || web.ByteCount == nil || web.Truncated == nil ||
			(web.Outcome == "success") != (*web.StatusCode >= 200 && *web.StatusCode < 300) {
			return errors.New("web fetch completed response metadata is invalid")
		}
	case "read_error":
		if web.StatusCode == nil || web.ByteCount == nil || web.Truncated != nil {
			return errors.New("web fetch partial read metadata is invalid")
		}
	case "timeout", "cancelled":
		if web.Truncated != nil {
			return errors.New("web fetch interrupted read cannot claim truncation")
		}
	case "transport_error", "invalid_url", "policy_blocked", "ssrf_blocked":
		if web.ByteCount != nil || web.Truncated != nil {
			return errors.New("web fetch unmeasured response cannot claim body metadata")
		}
	case "disabled":
		if web.StatusCode != nil || web.ByteCount != nil || web.Truncated != nil {
			return errors.New("disabled web fetch cannot claim response metadata")
		}
	default:
		return errors.New("web fetch outcome is invalid")
	}
	return nil
}

// UnmarshalJSON keeps the public nested object closed and distinguishes an
// absent measurement from an explicit null, which is not part of the DTO.
func (web *WebFetchStatusV1) UnmarshalJSON(data []byte) error {
	type wireWebFetchStatusV1 WebFetchStatusV1
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded wireWebFetchStatusV1
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("web fetch metadata contains trailing data")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"outcome", "statusCode", "byteCount", "truncated"} {
		if value, present := fields[key]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("web fetch metadata cannot contain null fields")
		}
	}
	value := WebFetchStatusV1(decoded)
	if err := ValidateWebFetchStatusV1(value); err != nil {
		return err
	}
	*web = value
	return nil
}

func ParseWebFetchStatusV1(value any) (WebFetchStatusV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return WebFetchStatusV1{}, err
	}
	var web WebFetchStatusV1
	if err := json.Unmarshal(body, &web); err != nil {
		return WebFetchStatusV1{}, err
	}
	return web, nil
}

func NewWebFetchStatusProjectionV1(web WebFetchStatusV1, isError bool) (PublicToolResultProjectionV1, error) {
	if err := ValidateWebFetchStatusV1(web); err != nil {
		return PublicToolResultProjectionV1{}, err
	}
	status, messageKey, code, _ := webFetchLifecycleV1(web.Outcome)
	projection := PublicToolResultProjectionV1{
		SchemaVersion: PublicProjectionSchemaVersion, ProjectionKind: ProjectionWebFetchStatus,
		Disclosure: MetadataOnlyDisclosure, MessageKey: messageKey, Status: status, Code: code,
		PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false, Web: &web,
	}
	if _, err := SettlementLifecycleStatusV1(projection, isError); err != nil {
		return PublicToolResultProjectionV1{}, err
	}
	return projection, nil
}

func webFetchLifecycleV1(outcome string) (status string, messageKey string, code string, ok bool) {
	switch outcome {
	case "success":
		return "completed", "tool_completed", "tool_completed", true
	case "http_error", "read_error", "transport_error", "invalid_url":
		return "failed", "tool_failed", "tool_failed", true
	case "policy_blocked", "ssrf_blocked", "disabled":
		return "blocked", "tool_blocked", "tool_blocked", true
	case "timeout":
		return "cancelled", "tool_cancelled", "tool_timeout", true
	case "cancelled":
		return "cancelled", "tool_cancelled", "tool_cancelled", true
	default:
		return "", "", "", false
	}
}
