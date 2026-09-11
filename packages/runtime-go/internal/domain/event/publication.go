package event

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	domainrestrictedevidence "analytix.local/runtime-go/internal/domain/restrictedevidence"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

var (
	ErrPrivateReasoningPersistence       = errors.New("private reasoning is not persistable")
	ErrAssistantDraftPersistence         = errors.New("provider assistant drafts are not persistable")
	ErrRestrictedEvidenceProjection      = errors.New("restricted evidence is not projectable through ordinary records")
	ErrCredentialProjection              = errors.New("credential material is not projectable through ordinary records")
	ErrPrivacyProjection                 = errors.New("restricted PII is not projectable through ordinary records")
	ErrInternalEntityReferenceProjection = errors.New(
		"internal case entity reference is not projectable through public records",
	)
)

var privateGeneralTerminalSchemaValues = map[string]bool{
	"general-terminal-publication.v1":                  true,
	"general-terminal-publication-archive.v1":          true,
	"general-terminal-cas-binding.v1":                  true,
	"analytix.general-terminal-publication/v1":         true,
	"analytix.general-terminal-publication-archive/v1": true,
	"analytix.general-terminal-cas-binding/v1":         true,
}

var privateKeyNormalizer = strings.NewReplacer("_", "", "-", "", " ", "")

func ValidatePublicRecord(value any) error {
	if err := domainrestrictedevidence.Validate(value); err != nil {
		return errors.Join(ErrRestrictedEvidenceProjection, err)
	}
	if err := privateReasoningError(value, false); err != nil {
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

// publicPrivacyValidationViewV1 keeps a host-bound, closed plan digest out of
// prose PII classification without weakening the display-bearing plan fields.
// SHA-256 is structural metadata here: the canonical tool-result projection
// has already validated its syntax, lifecycle, and host tool-call identity.
// Lookalike maps and non-canonical tool results remain on the ordinary scanner.
func publicPrivacyValidationViewV1(value any) any {
	projected, changed := projectClosedPlanDigestForPrivacyValidationV1(value)
	if !changed {
		return value
	}
	return projected
}

// ProjectPublicValuePreservingClosedPlanDigestV1 applies the ordinary PII
// projector while keeping the SHA-256 metadata of an exact host tool result
// byte-stable. The staged placeholder is path-bound and restored only if the
// generic projector returns it unchanged; all plan display fields still pass
// through the ordinary projector.
func ProjectPublicValuePreservingClosedPlanDigestV1(value any) (any, bool) {
	entries := []closedPlanDigestProjectionEntryV1{}
	staged := stageClosedPlanDigestsV1(value, nil, &entries)
	for index := range entries {
		entries[index].placeholder["total"] = float64(len(entries))
		entries[index].stagedPlaceholder["total"] = float64(len(entries))
	}
	projected, changed := domainprivacy.ProjectPublicValue(staged)
	restored, ok := restoreClosedPlanDigestsV1(projected, entries)
	if !ok {
		return nil, true
	}
	return restored, changed
}

type closedPlanDigestProjectionPathV1 struct {
	kind  string
	key   string
	index int
}

type closedPlanDigestProjectionEntryV1 struct {
	path              []closedPlanDigestProjectionPathV1
	digest            string
	placeholder       map[string]any
	stagedPlaceholder map[string]any
}

func stageClosedPlanDigestsV1(
	value any,
	path []closedPlanDigestProjectionPathV1,
	entries *[]closedPlanDigestProjectionEntryV1,
) any {
	switch typed := value.(type) {
	case map[string]any:
		if digest, ok := canonicalClosedPlanToolResultDigestV1(typed); ok {
			staged := clonePublicRecordMapV1(typed)
			output := clonePublicRecordMapV1(staged["output"].(map[string]any))
			plan := clonePublicRecordMapV1(output["plan"].(map[string]any))
			placeholder := map[string]any{
				"kind": "closed_plan_digest_placeholder_v1", "ordinal": float64(len(*entries)),
			}
			stagedPlaceholder := clonePublicRecordMapV1(placeholder)
			digestPath := append(append([]closedPlanDigestProjectionPathV1(nil), path...),
				closedPlanDigestProjectionPathV1{kind: "key", key: "output"},
				closedPlanDigestProjectionPathV1{kind: "key", key: "plan"},
				closedPlanDigestProjectionPathV1{kind: "key", key: "contentHash"},
			)
			*entries = append(*entries, closedPlanDigestProjectionEntryV1{
				path: digestPath, digest: digest, placeholder: placeholder, stagedPlaceholder: stagedPlaceholder,
			})
			plan["contentHash"] = stagedPlaceholder
			output["plan"] = plan
			staged["output"] = output
			return staged
		}
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			childPath := append(append([]closedPlanDigestProjectionPathV1(nil), path...),
				closedPlanDigestProjectionPathV1{kind: "key", key: key},
			)
			out[key] = stageClosedPlanDigestsV1(child, childPath, entries)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			childPath := append(append([]closedPlanDigestProjectionPathV1(nil), path...),
				closedPlanDigestProjectionPathV1{kind: "index", index: index},
			)
			out[index] = stageClosedPlanDigestsV1(child, childPath, entries)
		}
		return out
	default:
		return value
	}
}

func restoreClosedPlanDigestsV1(value any, entries []closedPlanDigestProjectionEntryV1) (any, bool) {
	current := value
	for _, entry := range entries {
		var ok bool
		current, ok = restoreClosedPlanDigestAtPathV1(current, entry.path, entry)
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func restoreClosedPlanDigestAtPathV1(
	value any,
	path []closedPlanDigestProjectionPathV1,
	entry closedPlanDigestProjectionEntryV1,
) (any, bool) {
	if len(path) == 0 {
		if !reflect.DeepEqual(value, entry.placeholder) {
			return nil, false
		}
		return entry.digest, true
	}
	segment := path[0]
	switch segment.kind {
	case "key":
		record, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		child, present := record[segment.key]
		if !present {
			return nil, false
		}
		restored, ok := restoreClosedPlanDigestAtPathV1(child, path[1:], entry)
		if !ok {
			return nil, false
		}
		record[segment.key] = restored
		return record, true
	case "index":
		values, ok := value.([]any)
		if !ok || segment.index < 0 || segment.index >= len(values) {
			return nil, false
		}
		restored, ok := restoreClosedPlanDigestAtPathV1(values[segment.index], path[1:], entry)
		if !ok {
			return nil, false
		}
		values[segment.index] = restored
		return values, true
	default:
		return nil, false
	}
}

func projectClosedPlanDigestForPrivacyValidationV1(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if projected, ok := closedPlanToolResultPrivacyValidationViewV1(typed); ok {
			return projected, true
		}
		var out map[string]any
		for key, child := range typed {
			projected, changed := projectClosedPlanDigestForPrivacyValidationV1(child)
			if !changed {
				continue
			}
			if out == nil {
				out = make(map[string]any, len(typed))
				for originalKey, originalValue := range typed {
					out[originalKey] = originalValue
				}
			}
			out[key] = projected
		}
		if out != nil {
			return out, true
		}
		return value, false
	case []any:
		var out []any
		for index, child := range typed {
			projected, changed := projectClosedPlanDigestForPrivacyValidationV1(child)
			if !changed {
				continue
			}
			if out == nil {
				out = append([]any(nil), typed...)
			}
			out[index] = projected
		}
		if out != nil {
			return out, true
		}
		return value, false
	default:
		return value, false
	}
}

func closedPlanToolResultPrivacyValidationViewV1(item map[string]any) (map[string]any, bool) {
	if _, ok := canonicalClosedPlanToolResultDigestV1(item); !ok {
		return nil, false
	}
	canonical, _ := domaintoolresult.PrivateDurableToolResultItemRecordV1(item)
	out := clonePublicRecordMapV1(canonical)
	output, _ := out["output"].(map[string]any)
	if output == nil {
		return nil, false
	}
	output = clonePublicRecordMapV1(output)
	plan, _ := output["plan"].(map[string]any)
	if plan == nil {
		return nil, false
	}
	plan = clonePublicRecordMapV1(plan)
	plan["contentHash"] = "sha256-digest"
	output["plan"] = plan
	out["output"] = output
	return out, true
}

func canonicalClosedPlanToolResultDigestV1(item map[string]any) (string, bool) {
	return domaintoolresult.ClosedPlanToolResultDigestV1(item)
}

func clonePublicRecordMapV1(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, child := range value {
		out[key] = child
	}
	return out
}

func containsAssistantDraft(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if IsLegacyAssistantDraftItem(typed) {
			return true
		}
		kind, _ := typed["kind"].(string)
		if strings.EqualFold(strings.TrimSpace(kind), "assistant_text_delta") {
			return true
		}
		for _, child := range typed {
			if containsAssistantDraft(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsAssistantDraft(child) {
				return true
			}
		}
	}
	return false
}

// IsPrivateTerminalAuthorityObject detects a private terminal contract by
// semantic schema/purpose identity even when an attacker hides it below a
// neutral wrapper key.
func IsPrivateTerminalAuthorityObject(value map[string]any) bool {
	if value == nil {
		return false
	}
	for _, key := range []string{"schemaVersion", "purpose"} {
		text, _ := value[key].(string)
		if privateGeneralTerminalSchemaValues[strings.TrimSpace(text)] {
			return true
		}
	}
	return false
}

// ContainsPrivateTerminalAuthority is the fail-closed sidecar/import check.
// Canonical keys and semantically identified objects are both forbidden.
func ContainsPrivateTerminalAuthority(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if IsPrivateTerminalAuthorityObject(typed) {
			return true
		}
		for key, child := range typed {
			if strings.HasPrefix(normalizePrivateKey(key), "generalterminal") || ContainsPrivateTerminalAuthority(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if ContainsPrivateTerminalAuthority(child) {
				return true
			}
		}
	}
	return false
}

func SanitizePublicValue(value any, allowUserText bool) (any, bool) {
	if domainrestrictedevidence.Validate(value) != nil {
		return nil, false
	}
	if hasInvalidPublicReasoningMetadataV1(value) {
		return nil, false
	}
	public, ok := sanitizeValue(value, allowUserText, false)
	if !ok || domaincaseentity.ValidatePublicValueWithoutReferenceV1(public) != nil {
		return nil, false
	}
	return public, true
}

// SanitizeDurableValue removes private reasoning and exact tool arguments but
// retains a validated host-issued execution grant for private registry replay.
// It must never be used for HTTP, SSE, renderer, export, or report projection.
func SanitizeDurableValue(value any, allowUserText bool) (any, bool) {
	if domainrestrictedevidence.Validate(value) != nil {
		return nil, false
	}
	if hasInvalidPublicReasoningMetadataV1(value) {
		return nil, false
	}
	return sanitizeValue(value, allowUserText, true)
}

func sanitizeValue(value any, allowUserText bool, privateDurable bool) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if !privateDurable && IsPrivateTerminalAuthorityObject(typed) {
			return nil, false
		}
		for key, child := range typed {
			normalizedKey := normalizePrivateKey(key)
			if (normalizedKey == "type" || normalizedKey == "kind" || normalizedKey == "channel") &&
				domainreasoningmarkup.IsPrivateReasoningDiscriminatorV1(key, child) {
				return nil, false
			}
		}
		kind, _ := typed["kind"].(string)
		kind = strings.ToLower(strings.TrimSpace(kind))
		switch kind {
		case "assistant_reasoning", "assistant_reasoning_delta", "agent_reasoning":
			return nil, false
		case "tool_call":
			if privateDurable {
				return domaintoolcall.PrivateDurableToolCallItemRecordV1(typed)
			}
			return domaintoolcall.PublicToolCallItemRecordV1(typed), true
		case "tool_result":
			if privateDurable {
				return domaintoolresult.PrivateDurableToolResultItemRecordV1(typed)
			}
			return domaintoolresult.PublicToolResultItemRecordV1(typed), true
		}
		userMessage := kind == "user_message"
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			normalizedKey := normalizePrivateKey(key)
			recognizedReasoningMetadata, validReasoningMetadata := validPublicReasoningMetadataV1(normalizedKey, child)
			if recognizedReasoningMetadata {
				if !validReasoningMetadata {
					return nil, false
				}
				if legacyEmptyReasoningMetadataV1(normalizedKey, child) {
					continue
				}
			} else if isPrivateReasoningKey(normalizedKey) {
				continue
			}
			if !privateDurable && strings.HasPrefix(normalizedKey, "generalterminal") {
				continue
			}
			if !privateDurable && normalizedKey == "executiongrant" {
				continue
			}
			childAllowsUserText := (allowUserText || userMessage) && normalizedKey == "text"
			public, ok := sanitizeValue(child, childAllowsUserText, privateDurable)
			if ok {
				out[key] = public
			}
		}
		return out, true
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			public, ok := sanitizeValue(child, allowUserText, privateDurable)
			if ok {
				out = append(out, public)
			}
		}
		return out, true
	case string:
		if allowUserText {
			return typed, true
		}
		public, err := FilterPublicText(typed)
		if err != nil {
			return nil, false
		}
		return public, true
	default:
		return value, true
	}
}

func privateReasoningError(value any, allowUserText bool) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if domainreasoningmarkup.IsPrivateReasoningDiscriminatorV1(key, child) {
				return ErrPrivateReasoningPersistence
			}
		}
		kind, _ := typed["kind"].(string)
		kind = strings.ToLower(strings.TrimSpace(kind))
		switch kind {
		case "assistant_reasoning", "assistant_reasoning_delta", "agent_reasoning":
			return ErrPrivateReasoningPersistence
		}
		if kind == "compaction" {
			reasoningExcluded, _ := typed["reasoningExcluded"].(bool)
			schemaVersion := numericValue(typed["schemaVersion"])
			proof, _ := typed["reasoningExclusionProof"].(string)
			if !reasoningExcluded || schemaVersion < 2 || proof == "" || proof != ReasoningExclusionProof(typed) {
				return ErrPrivateReasoningPersistence
			}
		}
		if kind == "compaction_completed" {
			proof, _ := typed["reasoningExclusionProof"].(string)
			reasoningExcluded, _ := typed["reasoningExcluded"].(bool)
			if !reasoningExcluded || !strings.HasPrefix(proof, "sha256:") || len(proof) != len("sha256:")+64 {
				return ErrPrivateReasoningPersistence
			}
		}
		userMessage := kind == "user_message"
		for key, child := range typed {
			normalizedKey := normalizePrivateKey(key)
			if recognized, valid := validPublicReasoningMetadataV1(normalizedKey, child); recognized {
				if !valid {
					return ErrPrivateReasoningPersistence
				}
			} else if isPrivateReasoningKey(normalizedKey) {
				return ErrPrivateReasoningPersistence
			}
			childAllowsUserText := (allowUserText || userMessage) && normalizedKey == "text"
			if err := privateReasoningError(child, childAllowsUserText); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for _, child := range typed {
			if err := privateReasoningError(child, allowUserText); err != nil {
				return err
			}
		}
	case string:
		if allowUserText {
			return nil
		}
		public, err := FilterPublicText(typed)
		if err != nil {
			return err
		}
		if public != typed {
			return ErrPrivateReasoningPersistence
		}
	}
	return nil
}

func ReasoningExclusionProof(record map[string]any) string {
	canonical := make(map[string]any, len(record))
	for key, value := range record {
		if key == "reasoningExclusionProof" {
			continue
		}
		canonical[key] = value
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}

func isPrivateReasoningKey(normalizedKey string) bool {
	return strings.Contains(normalizedKey, "reasoning") || strings.Contains(normalizedKey, "thinking")
}

// validPublicReasoningMetadataV1 admits only typed, bounded metadata. A key
// name alone is never enough to classify provider-originated bytes as public.
func validPublicReasoningMetadataV1(normalizedKey string, value any) (recognized bool, valid bool) {
	switch normalizedKey {
	case "reasoningtokens":
		return true, nonNegativeIntegerMetadataV1(value)
	case "reasoningtoken", "reasoningdurationms", "reasoningstartedat", "reasoningfinishedat":
		return true, false
	case "reasoningeffort":
		effort, ok := value.(string)
		if !ok {
			return true, false
		}
		canonical, valid := domainmodel.ProjectReasoningEffortV1(effort)
		return true, valid && canonical == effort
	case "reasoningexcluded":
		excluded, ok := value.(bool)
		return true, ok && excluded
	case "reasoningexclusionproof":
		proof, ok := value.(string)
		return true, ok && isSHA256ProofV1(proof)
	default:
		return false, false
	}
}

func legacyEmptyReasoningMetadataV1(normalizedKey string, value any) bool {
	text, ok := value.(string)
	return normalizedKey == "reasoningeffort" && ok && text == ""
}

func hasInvalidPublicReasoningMetadataV1(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if recognized, valid := validPublicReasoningMetadataV1(normalizePrivateKey(key), child); recognized && !valid {
				return true
			}
			if hasInvalidPublicReasoningMetadataV1(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasInvalidPublicReasoningMetadataV1(child) {
				return true
			}
		}
	}
	return false
}

func nonNegativeIntegerMetadataV1(value any) bool {
	const maxSafeIntegerV1 = int64(1<<53 - 1)
	switch typed := value.(type) {
	case int:
		return typed >= 0 && int64(typed) <= maxSafeIntegerV1
	case int8:
		return typed >= 0
	case int16:
		return typed >= 0
	case int32:
		return typed >= 0
	case int64:
		return typed >= 0 && typed <= maxSafeIntegerV1
	case uint, uint8, uint16, uint32, uint64:
		return safeUnsignedIntegerMetadataV1(typed, uint64(maxSafeIntegerV1))
	case float32:
		value := float64(typed)
		return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 &&
			value <= float64(maxSafeIntegerV1) && math.Trunc(value) == value
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0) && typed >= 0 &&
			typed <= float64(maxSafeIntegerV1) && math.Trunc(typed) == typed
	case json.Number:
		parsed, err := typed.Int64()
		return err == nil && parsed >= 0 && parsed <= maxSafeIntegerV1
	default:
		return false
	}
}

func safeUnsignedIntegerMetadataV1(value any, max uint64) bool {
	switch typed := value.(type) {
	case uint:
		return uint64(typed) <= max
	case uint8:
		return true
	case uint16:
		return true
	case uint32:
		return true
	case uint64:
		return typed <= max
	default:
		return false
	}
}

func isSHA256ProofV1(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func numericValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	default:
		return 0
	}
}

func normalizePrivateKey(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = privateKeyNormalizer.Replace(normalized)
	return normalized
}
