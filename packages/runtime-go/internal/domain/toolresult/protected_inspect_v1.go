package toolresult

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/secretprojection"
)

// Only a validated producer's literal text enters this existing private lane.
// Non-text payloads and original raw results never become a display fallback.
const ProtectedOutputTextScanLimitV1 = 1 << 20

type ProtectedOutputTextV1 struct {
	Availability   string `json:"availability"`
	Body           string `json:"body,omitempty"`
	Classification string `json:"classification,omitempty"`
	Truncated      bool   `json:"truncated"`
	Reason         string `json:"reason,omitempty"`
}

func ValidateProtectedOutputTextV1(value *ProtectedOutputTextV1) error {
	if value == nil {
		return nil
	}
	if value.Availability == "unavailable" && value.Body == "" && value.Classification == "" && !value.Truncated && oneOf(value.Reason, "limit", "invalid") {
		return nil
	}
	if value.Availability != "available" || value.Reason != "" || strings.TrimSpace(value.Body) == "" ||
		len(value.Body) > ProtectedSnapshotBodyLimitV1 || !utf8.ValidString(value.Body) || strings.ContainsRune(value.Body, 0) ||
		!oneOf(value.Classification, "source_exact_local", "credential_redacted") || secretprojection.ContainsCredentialV1(value.Body) {
		return errors.New("protected output text is invalid")
	}
	return nil
}

// Scan the complete bounded source before taking an UTF-8 prefix. A partial
// credential must never evade projection by straddling the display limit.
func ProjectProtectedOutputTextV1(text string, secrets ...string) *ProtectedOutputTextV1 {
	unavailable := func(reason string) *ProtectedOutputTextV1 {
		return &ProtectedOutputTextV1{Availability: "unavailable", Reason: reason}
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if len(text) > ProtectedOutputTextScanLimitV1 {
		return unavailable("limit")
	}
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) || len(secrets) > 64 {
		return unavailable("invalid")
	}
	for _, secret := range secrets {
		if secret != "" && (len(strings.TrimSpace(secret)) < 4 || len(secret) > ProtectedOutputTextScanLimitV1) {
			return unavailable("invalid")
		}
	}
	projected := secretprojection.ProjectTextV1(text, secrets...)
	if secretprojection.ContainsCredentialV1(projected) {
		return unavailable("invalid")
	}
	classification := "source_exact_local"
	if projected != text {
		classification = "credential_redacted"
	}
	body, truncated := BoundProtectedTextV1(projected, ProtectedSnapshotBodyLimitV1)
	value := &ProtectedOutputTextV1{Availability: "available", Body: body, Classification: classification, Truncated: truncated}
	if ValidateProtectedOutputTextV1(value) != nil {
		return unavailable("invalid")
	}
	return value
}

func cloneProtectedOutputTextV1(value *ProtectedOutputTextV1) *ProtectedOutputTextV1 {
	if value == nil {
		return nil
	}
	copy_ := *value
	return &copy_
}

type ProtectedOutputErrorEntryV1 struct {
	Message        string `json:"message"`
	Classification string `json:"classification"`
	ParallelIndex  *int   `json:"parallelIndex,omitempty"` // Actual protocol index: 1-based. Never the slice slot.
	JobID          string `json:"jobId,omitempty"`
}
type ProtectedOutputErrorV1 struct {
	Availability string                        `json:"availability"`
	Entries      []ProtectedOutputErrorEntryV1 `json:"entries,omitempty"`
	Reason       string                        `json:"reason,omitempty"`
}

func ValidateProtectedOutputErrorV1(value *ProtectedOutputErrorV1) error {
	if value == nil {
		return nil
	}
	if value.Availability == "unavailable" && len(value.Entries) == 0 && oneOf(value.Reason, "missing", "invalid", "limit", "canceled") {
		return nil
	}
	if value.Availability != "available" || value.Reason != "" || len(value.Entries) == 0 || len(value.Entries) > ProtectedJobListLimitV1 {
		return errors.New("protected error availability is invalid")
	}
	total, seen := 0, map[int]bool{}
	for _, entry := range value.Entries {
		total += len(entry.Message)
		if total > ProtectedSnapshotBodyLimitV1 || strings.TrimSpace(entry.Message) == "" || !utf8.ValidString(entry.Message) || strings.ContainsRune(entry.Message, 0) || !oneOf(entry.Classification, "source_exact_local", "credential_redacted") || entry.JobID != "" && !taskJobPublicIDPatternV1.MatchString(entry.JobID) {
			return errors.New("protected error entry is invalid")
		}
		if entry.ParallelIndex != nil {
			if *entry.ParallelIndex <= 0 || uint64(*entry.ParallelIndex) > 1<<53-1 || seen[*entry.ParallelIndex] {
				return errors.New("protected error index is invalid")
			}
			seen[*entry.ParallelIndex] = true
		} else if len(value.Entries) != 1 {
			return errors.New("protected parallel error index is missing")
		}
	}
	return nil
}

// Only credentials are projected, with the process owner's existing secret
// quota. General PII/case values never pass through public/model projection.
func ProjectProtectedOutputErrorV1(entries []ProtectedOutputErrorEntryV1, secrets ...string) *ProtectedOutputErrorV1 {
	unavailable := func(reason string) *ProtectedOutputErrorV1 {
		return &ProtectedOutputErrorV1{Availability: "unavailable", Reason: reason}
	}
	if len(entries) == 0 {
		return unavailable("missing")
	}
	if len(secrets) > 64 {
		return unavailable("invalid")
	}
	for _, secret := range secrets {
		if secret != "" && (len(strings.TrimSpace(secret)) < 4 || len(secret) > 1<<20) {
			return unavailable("invalid")
		}
	}
	total := 0
	for _, entry := range entries {
		total += len(entry.Message)
		if total > ProtectedSnapshotBodyLimitV1 {
			return unavailable("limit")
		}
		if strings.TrimSpace(entry.Message) == "" {
			return unavailable("missing")
		}
		if !utf8.ValidString(entry.Message) || strings.ContainsRune(entry.Message, 0) {
			return unavailable("invalid")
		}
	}
	value := &ProtectedOutputErrorV1{Availability: "available", Entries: append([]ProtectedOutputErrorEntryV1(nil), entries...)}
	for i := range value.Entries {
		entry := &value.Entries[i]
		original := entry.Message
		entry.Message = secretprojection.ProjectTextV1(original, secrets...)
		if secretprojection.ContainsCredentialV1(entry.Message) {
			return unavailable("invalid")
		}
		entry.Classification = "source_exact_local"
		if entry.Message != original || entries[i].Classification == "credential_redacted" {
			entry.Classification = "credential_redacted"
		}
		if entry.ParallelIndex != nil {
			index := *entry.ParallelIndex
			entry.ParallelIndex = &index
		}
	}
	total = 0
	for _, entry := range value.Entries {
		total += len(entry.Message)
	}
	if total > ProtectedSnapshotBodyLimitV1 {
		return unavailable("limit")
	}
	if ValidateProtectedOutputErrorV1(value) != nil {
		return unavailable("invalid")
	}
	return value
}

func cloneProtectedOutputErrorV1(value *ProtectedOutputErrorV1) *ProtectedOutputErrorV1 {
	if value == nil {
		return nil
	}
	copy_ := *value
	if value.Entries != nil {
		copy_.Entries = append([]ProtectedOutputErrorEntryV1(nil), value.Entries...)
		for i := range copy_.Entries {
			if copy_.Entries[i].ParallelIndex != nil {
				index := *copy_.Entries[i].ParallelIndex
				copy_.Entries[i].ParallelIndex = &index
			}
		}
	}
	return &copy_
}

// Request bytes are sealed with the original result. Event times are resolved
// only from that call's recorded events by the existing local-display reader.
// No current catalog, prompt or execution-grant definition supplies a schema.
type ProtectedInspectV1 struct {
	OutputText      *ProtectedOutputTextV1  `json:"outputText,omitempty"`
	OutputError     *ProtectedOutputErrorV1 `json:"outputError,omitempty"`
	InputJSON       string                  `json:"inputJSON,omitempty"`
	SchemaJSON      string                  `json:"schemaJSON,omitempty"`
	InputTruncated  bool                    `json:"inputTruncated"`
	SchemaTruncated bool                    `json:"schemaTruncated"`
	CallEventAt     *string                 `json:"callEventAt"`
	ResultEventAt   *string                 `json:"resultEventAt"`
	DurationMs      *int64                  `json:"durationMs"`
}

func ValidateProtectedInspectV1(value *ProtectedInspectV1) error {
	if value == nil {
		return nil
	}
	if err := ValidateProtectedOutputTextV1(value.OutputText); err != nil {
		return err
	}
	if value.OutputText != nil && value.OutputError != nil {
		return errors.New("protected success and error text cannot coexist")
	}
	if err := ValidateProtectedOutputErrorV1(value.OutputError); err != nil {
		return err
	}
	if !protectedWebJSONV1(value.InputJSON) || !protectedWebJSONV1(value.SchemaJSON) ||
		value.InputTruncated && value.InputJSON != "" || value.SchemaTruncated && value.SchemaJSON != "" {
		return errors.New("protected inspect request is invalid")
	}
	for _, at := range []*string{value.CallEventAt, value.ResultEventAt} {
		if at != nil {
			if _, err := time.Parse(time.RFC3339Nano, *at); err != nil || len(*at) > 64 {
				return errors.New("protected inspect event time is invalid")
			}
		}
	}
	if value.DurationMs != nil {
		if value.CallEventAt == nil || value.ResultEventAt == nil {
			return errors.New("inspect duration has no event pair")
		}
		call, _ := time.Parse(time.RFC3339Nano, *value.CallEventAt)
		result, _ := time.Parse(time.RFC3339Nano, *value.ResultEventAt)
		if result.Before(call) || *value.DurationMs != result.Sub(call).Milliseconds() {
			return errors.New("inspect duration differs from recorded event times")
		}
	} else if value.CallEventAt != nil && value.ResultEventAt != nil {
		call, _ := time.Parse(time.RFC3339Nano, *value.CallEventAt)
		result, _ := time.Parse(time.RFC3339Nano, *value.ResultEventAt)
		if !result.Before(call) {
			return errors.New("inspect event pair is missing its duration")
		}
	}
	return nil
}

func InspectWithEventTimesV1(value *ProtectedInspectV1, callAt, resultAt string) *ProtectedInspectV1 {
	if value == nil {
		return nil
	}
	copy_ := *value
	copy_.OutputText = cloneProtectedOutputTextV1(value.OutputText)
	copy_.OutputError = cloneProtectedOutputErrorV1(value.OutputError)
	copy_.CallEventAt, copy_.ResultEventAt, copy_.DurationMs = nil, nil, nil
	call, callErr := time.Parse(time.RFC3339Nano, callAt)
	result, resultErr := time.Parse(time.RFC3339Nano, resultAt)
	if callErr == nil {
		copy_.CallEventAt = &callAt
	}
	if resultErr == nil {
		copy_.ResultEventAt = &resultAt
	}
	if callErr == nil && resultErr == nil && !result.Before(call) {
		duration := result.Sub(call).Milliseconds()
		copy_.DurationMs = &duration
	}
	return &copy_
}
