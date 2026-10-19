package event

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
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
	domainprocess.SettledHistorySchemaV1:               true,
	domainprocess.SettledHistoryAnchorSchemaV1:         true,
	"general-terminal-publication.v1":                  true,
	"general-terminal-publication-archive.v1":          true,
	"general-terminal-cas-binding.v1":                  true,
	"analytix.general-terminal-publication/v1":         true,
	"analytix.general-terminal-publication-archive/v1": true,
	"analytix.general-terminal-cas-binding/v1":         true,
}

var privateKeyNormalizer = strings.NewReplacer("_", "", "-", "", " ", "")

func ValidatePublicRecord(value any) error {
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
	value, err = questionReplyValidationViewV1(value)
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

// publicPrivacyValidationViewV1 keeps host-bound, closed plan/artifact digests
// and timestamps, or closed tool-failure diagnostic hashes, out of prose PII
// classification without weakening display fields. Closed root logical-step
// events likewise keep only their validated correlation ID out of prose scanning.
// These are structural metadata: the canonical tool-result projection has
// already validated their syntax, lifecycle, and host tool-call identity.
// Lookalike maps and non-canonical tool results remain on the ordinary scanner.
func publicPrivacyValidationViewV1(value any) any {
	if record, ok := value.(map[string]any); ok {
		if _, valid := ProjectClosedLogicalStepEventDetailsV1(record); valid {
			projected := clonePublicRecordMapV1(record)
			details := clonePublicRecordMapV1(record["details"].(map[string]any))
			details["logicalStepId"] = "logical-step-id"
			projected["details"] = details
			value = projected
		}
	}
	projected, changed := projectClosedPlanDigestForPrivacyValidationV1(value)
	if !changed {
		return value
	}
	return projected
}

// ProjectPublicValuePreservingClosedPlanDigestV1 applies the ordinary PII
// projector while keeping SHA-256 and timestamp metadata of an exact host tool
// result byte-stable. Each staged placeholder is path-bound and restored only if the
// generic projector returns it unchanged; all plan display fields still pass
// through the ordinary projector. The same bounded mechanism preserves the five
// SHA-256 fields of exact tool_not_advertised failure records and the validated
// logicalStepId of a closed root logical-step event. Nested lookalikes are prose.
func ProjectPublicValuePreservingClosedPlanDigestV1(value any) (any, bool) {
	if ValidateQuestionReplyResourceLimitsV1(value) != nil {
		return nil, true
	}
	value, restoreQuestion, err := StageQuestionReplyContentV1(value)
	if err != nil {
		return nil, true
	}
	entries := []closedPlanDigestProjectionEntryV1{}
	staged := stageClosedPlanDigestsV1(value, nil, &entries)
	staged = stageRootLogicalStepIDV1(staged, &entries)
	for index := range entries {
		entries[index].placeholder["total"] = float64(len(entries))
		entries[index].stagedPlaceholder["total"] = float64(len(entries))
	}
	projected, changed := domainprivacy.ProjectPublicValue(staged)
	restored, ok := restoreClosedPlanDigestsV1(projected, entries)
	if !ok {
		return nil, true
	}
	restored, ok = restoreQuestion(restored)
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

// StageClosedLogicalStepIDV1 protects only the validated correlation ID of a
// closed root logical-step event during ordinary projection. It does not walk
// nested values or stage tool-result metadata. Restoration requires the exact
// placeholder to survive at details.logicalStepId; this grants no publication
// authority and leaves malformed or embedded lookalikes on the prose path.
func StageClosedLogicalStepIDV1(value any) (any, func(any) (any, bool)) {
	entries := []closedPlanDigestProjectionEntryV1{}
	staged := stageRootLogicalStepIDV1(value, &entries)
	for index := range entries {
		entries[index].placeholder["total"] = float64(len(entries))
		entries[index].stagedPlaceholder["total"] = float64(len(entries))
	}
	return staged, func(projected any) (any, bool) { return restoreClosedPlanDigestsV1(projected, entries) }
}

// This must run once at the publication root, never inside the recursive tool
// metadata walker: a complete pipeline lookalike embedded in user content is
// still ordinary prose. Only the exact details.logicalStepId path is staged.
func stageRootLogicalStepIDV1(value any, entries *[]closedPlanDigestProjectionEntryV1) any {
	record, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if _, valid := ProjectClosedLogicalStepEventDetailsV1(record); !valid {
		return value
	}
	staged := clonePublicRecordMapV1(record)
	details := clonePublicRecordMapV1(record["details"].(map[string]any))
	placeholder := map[string]any{
		"kind": "closed_logical_step_id_placeholder_v1", "ordinal": float64(len(*entries)),
	}
	stagedPlaceholder := clonePublicRecordMapV1(placeholder)
	*entries = append(*entries, closedPlanDigestProjectionEntryV1{
		path:   []closedPlanDigestProjectionPathV1{{kind: "key", key: "details"}, {kind: "key", key: "logicalStepId"}},
		digest: details["logicalStepId"].(string), placeholder: placeholder, stagedPlaceholder: stagedPlaceholder,
	})
	details["logicalStepId"] = stagedPlaceholder
	staged["details"] = details
	return staged
}

func stageClosedPlanDigestsV1(
	value any,
	path []closedPlanDigestProjectionPathV1,
	entries *[]closedPlanDigestProjectionEntryV1,
) any {
	switch typed := value.(type) {
	case map[string]any:
		if details, ok := closedToolFailureDetailsForPrivacyV1(typed); ok {
			staged := clonePublicRecordMapV1(typed)
			metadata := clonePublicRecordMapV1(details)
			for _, field := range closedToolFailureDigestFieldsV1 {
				placeholder := map[string]any{
					"kind": "closed_tool_failure_digest_placeholder_v1", "ordinal": float64(len(*entries)),
				}
				stagedPlaceholder := clonePublicRecordMapV1(placeholder)
				metadataPath := append(append([]closedPlanDigestProjectionPathV1(nil), path...),
					closedPlanDigestProjectionPathV1{kind: "key", key: "details"},
					closedPlanDigestProjectionPathV1{kind: "key", key: field},
				)
				*entries = append(*entries, closedPlanDigestProjectionEntryV1{
					path: metadataPath, digest: details[field].(string), placeholder: placeholder, stagedPlaceholder: stagedPlaceholder,
				})
				metadata[field] = stagedPlaceholder
			}
			staged["details"] = metadata
			return staged
		}
		if container, fields, ok := ClosedToolResultPrivacyMetadataV1(typed); ok {
			staged := clonePublicRecordMapV1(typed)
			output := clonePublicRecordMapV1(staged["output"].(map[string]any))
			metadata := clonePublicRecordMapV1(output[container].(map[string]any))
			for _, field := range fields {
				placeholder := map[string]any{
					"kind": "closed_plan_digest_placeholder_v1", "ordinal": float64(len(*entries)),
				}
				stagedPlaceholder := clonePublicRecordMapV1(placeholder)
				metadataPath := append(append([]closedPlanDigestProjectionPathV1(nil), path...),
					closedPlanDigestProjectionPathV1{kind: "key", key: "output"},
					closedPlanDigestProjectionPathV1{kind: "key", key: container},
					closedPlanDigestProjectionPathV1{kind: "key", key: field},
				)
				*entries = append(*entries, closedPlanDigestProjectionEntryV1{
					path: metadataPath, digest: metadata[field].(string), placeholder: placeholder, stagedPlaceholder: stagedPlaceholder,
				})
				metadata[field] = stagedPlaceholder
			}
			output[container] = metadata
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
		if details, ok := closedToolFailureDetailsForPrivacyV1(typed); ok {
			projected := clonePublicRecordMapV1(typed)
			metadata := clonePublicRecordMapV1(details)
			for _, field := range closedToolFailureDigestFieldsV1 {
				metadata[field] = "sha256-digest"
			}
			projected["details"] = metadata
			return projected, true
		}
		if projected, ok := closedToolResultPrivacyValidationViewV1(typed); ok {
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

var closedToolFailureDigestFieldsV1 = []string{
	"rejectedToolNormalizedNameSha256", "advertisedToolManifestHash", "advertisedNameSetSortedHash",
	"providerRequestToolManifestHash", "runToolStepManifestHash",
}

// This is a privacy classification, not publication authority. The existing
// terminal CAS/outbox validation still authenticates the complete record.
// Never exempt a bare details map, an arbitrary *Hash key, or a failure with
// unknown fields: only five validated digest values leave the prose scanner.
func closedToolFailureDetailsForPrivacyV1(record map[string]any) (map[string]any, bool) {
	if record["code"] != "tool_not_advertised" || record["status"] != "failed" || !isClosedFailureRecordV1(record) {
		return nil, false
	}
	details, ok := record["details"].(map[string]any)
	if !ok || !domainfailure.ValidateToolNotAdvertisedDetails(details) {
		return nil, false
	}
	required := []string{"kind", "threadId", "turnId", "status", "code", "message", "severity", "details"}
	var optional []string
	switch record["kind"] {
	case "error":
		if record["role"] != "system" {
			return nil, false
		}
		required = append(required, "id", "role", "createdAt", "finishedAt")
		optional = []string{"generalTerminalCASBinding"}
	case "turn_failed":
		if record["terminalReason"] != "tool_failure" {
			return nil, false
		}
		required = append(required, "itemId", "timestamp", "terminalReason")
		optional = append([]string{"error", "seq", "generalTerminalCASBindingDigest"}, generalTerminalDeliveryPublicationMarkerNames...)
	default:
		return nil, false
	}
	if !generalTerminalDeliveryAllowedKeysV1(record, required, optional) {
		return nil, false
	}
	return details, true
}

func closedToolResultPrivacyValidationViewV1(item map[string]any) (map[string]any, bool) {
	container, fields, ok := ClosedToolResultPrivacyMetadataV1(item)
	if !ok {
		return nil, false
	}
	canonical, _ := domaintoolresult.PrivateDurableToolResultItemRecordV1(item)
	out := clonePublicRecordMapV1(canonical)
	output, _ := out["output"].(map[string]any)
	if output == nil {
		return nil, false
	}
	output = clonePublicRecordMapV1(output)
	metadata, _ := output[container].(map[string]any)
	if metadata == nil {
		return nil, false
	}
	metadata = clonePublicRecordMapV1(metadata)
	for _, field := range fields {
		metadata[field] = "closed-tool-metadata"
	}
	output[container] = metadata
	out["output"] = output
	return out, true
}

func canonicalClosedPlanToolResultDigestV1(item map[string]any) (string, bool) {
	return domaintoolresult.ClosedPlanToolResultDigestV1(item)
}

// ClosedToolResultPrivacyMetadataV1 identifies validated metadata fields under
// output that public projectors must preserve byte-for-byte. It grants no
// execution or artifact-open authority and accepts only canonical host items.
func ClosedToolResultPrivacyMetadataV1(item map[string]any) (string, []string, bool) {
	if _, ok := canonicalClosedPlanToolResultDigestV1(item); ok {
		return "plan", []string{"contentHash", "savedAt"}, true
	}
	if canonicalClosedGeneratedArtifactToolResultV1(item) {
		return "artifact", []string{"artifactId", "contentHash", "savedAt"}, true
	}
	return "", nil, false
}

// This is structural privacy classification, not artifact-open authority. Only
// the exact generation result may protect its validated metadata; bare output
// maps, other tools, unknown fields and inconsistent lifecycle stay on the
// ordinary scanner. Artifact opening still resolves current host authority.
func canonicalClosedGeneratedArtifactToolResultV1(item map[string]any) bool {
	threadID, _ := item["threadId"].(string)
	turnID, _ := item["turnId"].(string)
	callID, _ := item["callId"].(string)
	output, outputOK := item["output"].(map[string]any)
	artifact, artifactOK := output["artifact"].(map[string]any)
	if threadID == "" || strings.TrimSpace(threadID) != threadID || turnID == "" || strings.TrimSpace(turnID) != turnID ||
		!domainmodel.IsHostToolCallIDV1(callID) || item["id"] != domaintoolresult.ToolResultItemIDV1(turnID, callID) ||
		item["toolName"] != "generate_office_document" || item["toolKind"] != "file_change" ||
		item["kind"] != "tool_result" || item["role"] != "tool" || item["status"] != "completed" || item["isError"] != false ||
		!outputOK || output == nil || !artifactOK || artifact == nil {
		return false
	}
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(item["output"])
	if err != nil || projection.ProjectionKind != domaintoolresult.ProjectionArtifactStatus || projection.Artifact == nil {
		return false
	}
	canonical, ok := domaintoolresult.PrivateDurableToolResultItemRecordV1(item)
	canonicalBody, canonicalErr := json.Marshal(canonical)
	itemBody, itemErr := json.Marshal(item)
	return ok && canonicalErr == nil && itemErr == nil && bytes.Equal(canonicalBody, itemBody)
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
			if normalizePrivateKey(key) == "privateprocesshistory" || strings.HasPrefix(normalizePrivateKey(key), "generalterminal") || ContainsPrivateTerminalAuthority(child) {
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
	original := value
	staged, err := questionReplyValidationViewV1(value)
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
	scan, err := questionReplyValidationViewV1(public)
	if err != nil || domaincaseentity.ValidatePublicValueWithoutReferenceV1(scan) != nil {
		return nil, false
	}
	return public, true
}

// SanitizeDurableValue removes private reasoning and exact tool arguments but
// retains a validated host-issued execution grant for private registry replay.
// It must never be used for HTTP, SSE, renderer, export, or report projection.
func SanitizeDurableValue(value any, allowUserText bool) (any, bool) {
	original := value
	staged, err := questionReplyValidationViewV1(value)
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
	public, ok := sanitizeValue(original, allowUserText, true)
	if !ok || !sameQuestionReplyBodiesV1(original, public) {
		return nil, false
	}
	return public, true
}

func sanitizeValue(value any, allowUserText bool, privateDurable bool) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		// The exact Primary anchor key below has a separate strict metadata
		// admission. Neither a full record nor an anchor under a neutral key is
		// ordinary durable history, even when this is the private sanitizer.
		if schema, _ := typed["schemaVersion"].(string); schema == domainprocess.SettledHistorySchemaV1 || schema == domainprocess.SettledHistoryAnchorSchemaV1 {
			return nil, false
		}
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
			if normalizedKey == "privateprocesshistory" {
				if !privateDurable {
					continue
				}
				anchor, err := domainprocess.ParseSettledHistoryAnchorV1(child)
				if err != nil || key != domainprocess.SettledHistoryFieldV1 {
					return nil, false
				}
				value, err := domainprocess.SettledHistoryAnchorMapV1(anchor)
				if err != nil {
					return nil, false
				}
				out[key] = value
				continue
			}
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
			parsed, matched, err := questionReplySlotContentV1(typed, key)
			if err != nil {
				return nil, false
			}
			if matched && !childAllowsUserText {
				public, ok := sanitizeValue(parsed, false, privateDurable)
				if !ok || !reflect.DeepEqual(public, parsed) {
					return nil, false
				}
				out[key] = child
				continue
			}
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

func containsSettledProcessHistoryV1(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if schema, _ := v["schemaVersion"].(string); schema == domainprocess.SettledHistorySchemaV1 || schema == domainprocess.SettledHistoryAnchorSchemaV1 {
			return true
		}
		for k, child := range v {
			if normalizePrivateKey(k) == "privateprocesshistory" || containsSettledProcessHistoryV1(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsSettledProcessHistoryV1(child) {
				return true
			}
		}
	}
	return false
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
			schemaVersion := compactionSchemaVersionV1(typed["schemaVersion"])
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
			parsed, matched, err := questionReplySlotContentV1(typed, key)
			if err != nil {
				return err
			}
			if matched && !childAllowsUserText {
				if err := privateReasoningError(parsed, false); err != nil {
					return err
				}
				continue
			}
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
	case "reasoningtokens", "firstreasoninglatencyms":
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

// Preserve the existing typed schema representations while validating strict
// integers. Other public numeric fields retain their own validators.
func compactionSchemaVersionV1(value any) int {
	switch typed := value.(type) {
	case int32:
		value = int(typed)
	case float32:
		value = float64(typed)
	}
	return generalCompactionNumeric(value)
}

func normalizePrivateKey(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = privateKeyNormalizer.Replace(normalized)
	return normalized
}

// This is the existing per-record resource budget, not a new per-answer limit.
// A closed Question wrapper is decoded once; every real string remains subject
// to the ordinary 1MiB scanner. Original encoded bytes count toward the 8MiB
// record budget, including every durable/promoted copy.
const MaxQuestionReplyRecordBytesV1 = 8 << 20

var ErrQuestionReplyContentV1 = errors.New("closed Question content is invalid")

func ValidateQuestionReplyContentV1(text string) error {
	_, err := parseQuestionReplyContentV1(text)
	return err
}

func parseQuestionReplyContentV1(text string) (map[string]any, error) {
	if len(text) > MaxQuestionReplyRecordBytesV1 {
		return nil, domainsecret.ErrProjectionLimitV1
	}
	value, err := domainjsonstrict.DecodeObject([]byte(text), domainjsonstrict.Options{RequireObject: true, MaxBytes: MaxQuestionReplyRecordBytesV1, MaxDepth: 6, MaxTokens: 100000, MaxStringBytes: MaxQuestionReplyRecordBytesV1})
	if err != nil {
		return nil, ErrQuestionReplyContentV1
	}
	if !questionReplyKeysV1(value, []string{"kind", "questions", "answers"}, nil) || value["kind"] != "question_reply_content" {
		return nil, ErrQuestionReplyContentV1
	}
	questions, qok := value["questions"].([]any)
	answers, aok := value["answers"].([]any)
	if !qok || !aok || len(questions) < 1 || len(questions) > 3 || len(answers) != len(questions) {
		return nil, ErrQuestionReplyContentV1
	}
	for index, raw := range questions {
		question, ok := raw.(map[string]any)
		if !ok || !questionReplyKeysV1(question, []string{"question", "header", "options", "multiSelect"}, []string{"detail"}) {
			return nil, ErrQuestionReplyContentV1
		}
		if _, ok := question["question"].(string); !ok || question["question"] == "" {
			return nil, ErrQuestionReplyContentV1
		}
		if _, ok := question["header"].(string); !ok {
			return nil, ErrQuestionReplyContentV1
		}
		if detail, present := question["detail"]; present {
			if _, ok := detail.(string); !ok {
				return nil, ErrQuestionReplyContentV1
			}
		}
		multi, mok := question["multiSelect"].(bool)
		options, ook := question["options"].([]any)
		if !mok || !ook || len(options) > 3 {
			return nil, ErrQuestionReplyContentV1
		}
		labels := map[string]int{}
		for _, rawOption := range options {
			option, ok := rawOption.(map[string]any)
			if !ok || !questionReplyKeysV1(option, []string{"label", "description"}, nil) {
				return nil, ErrQuestionReplyContentV1
			}
			label, lok := option["label"].(string)
			if _, dok := option["description"].(string); !lok || !dok {
				return nil, ErrQuestionReplyContentV1
			}
			labels[label]++
		}
		answer, ok := answers[index].(map[string]any)
		if !ok || !questionReplyKeysV1(answer, []string{"selected"}, []string{"custom"}) {
			return nil, ErrQuestionReplyContentV1
		}
		selected, sok := answer["selected"].([]any)
		custom := ""
		if rawCustom, present := answer["custom"]; present {
			var cok bool
			custom, cok = rawCustom.(string)
			if !cok {
				return nil, ErrQuestionReplyContentV1
			}
		}
		if !sok || len(selected) > len(options) || (!multi && (len(selected) > 1 || (len(selected) > 0 && custom != ""))) {
			return nil, ErrQuestionReplyContentV1
		}
		for _, rawSelected := range selected {
			label, ok := rawSelected.(string)
			if !ok || labels[label] == 0 {
				return nil, ErrQuestionReplyContentV1
			}
			labels[label]--
		}
	}
	if err := domainsecret.ValidateValueLimitsV1(value); err != nil {
		return nil, err
	}
	return value, nil
}

func questionReplyKeysV1(value map[string]any, required, optional []string) bool {
	for _, key := range required {
		if _, ok := value[key]; !ok {
			return false
		}
	}
	for key := range value {
		found := false
		for _, allowed := range required {
			found = found || key == allowed
		}
		for _, allowed := range optional {
			found = found || key == allowed
		}
		if !found {
			return false
		}
	}
	return true
}

func questionReplyTextSlotV1(record map[string]any) bool {
	if record["kind"] == "turn_steered" {
		return record["clientUserMessageId"] != nil && record["threadId"] != nil && record["turnId"] != nil
	}
	if record["kind"] == "user_message" {
		// Public message projection removes internal steering metadata. This is
		// only a closed content/resource view; signature and source authority
		// remain with the existing private steering entry.
		return record["delivery"] == "steer"
	}
	return record["delivery"] == "steer" && record["userInputReplyIdentity"] != nil && record["userInputId"] != nil && record["userInputCaptureReceiptId"] != nil
}

// Only known Question carrier slots use the closed wire codec. Receipt and
// signature authority is still verified at the steering owner; a content view
// grants no admission, model authority or access to private data.
func questionReplySlotContentV1(record map[string]any, key string) (map[string]any, bool, error) {
	if (key != "text" && key != "displayText") || !questionReplyTextSlotV1(record) {
		return nil, false, nil
	}
	text, ok := record[key].(string)
	if !ok || !strings.HasPrefix(strings.TrimSpace(text), "{") {
		return nil, false, nil
	}
	if len(text) > MaxQuestionReplyRecordBytesV1 {
		return nil, false, domainsecret.ErrProjectionLimitV1
	}
	var marker struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal([]byte(text), &marker) != nil || marker.Kind != "question_reply_content" {
		return nil, false, nil
	}
	parsed, err := parseQuestionReplyContentV1(text)
	return parsed, true, err
}

func ValidateQuestionReplyResourceLimitsV1(value any) error {
	view, err := QuestionReplyResourceViewV1(value)
	if err != nil {
		return err
	}
	return domainsecret.ValidateValueLimitsV1(view)
}

// Chunks are exclusively a byte-accounting view. They must never enter any
// credential, PII, restricted-evidence or case-reference scanner.
func QuestionReplyResourceViewV1(value any) (any, error) {
	budget := 100000
	return questionReplyViewV1(value, nil, nil, true, &budget)
}

// A complete decoded AST preserves all original content, including serialized
// JSON nested in a real string. The original scanners inspect that AST. Exact
// path restoration succeeds only if the complete content remained unchanged.
func StageQuestionReplyContentV1(value any) (any, func(any) (any, bool), error) {
	entries := []closedPlanDigestProjectionEntryV1{}
	budget := 100000
	staged, err := questionReplyViewV1(value, nil, &entries, false, &budget)
	return staged, func(projected any) (any, bool) { return restoreClosedPlanDigestsV1(projected, entries) }, err
}

// Read-only scanners need the complete decoded AST, without restoration data.
func questionReplyValidationViewV1(value any) (any, error) {
	budget := 100000
	return questionReplyViewV1(value, nil, nil, false, &budget)
}

func questionReplyViewV1(value any, path []closedPlanDigestProjectionPathV1, entries *[]closedPlanDigestProjectionEntryV1, resource bool, budget *int) (any, error) {
	return walkQuestionReplyV1(value, path, entries, resource, budget, len(path), nil)
}

// A non-nil bodies collector shares the complete validation walk without
// constructing a staged AST, restoration paths or placeholders.
func walkQuestionReplyV1(value any, path []closedPlanDigestProjectionPathV1, entries *[]closedPlanDigestProjectionEntryV1, resource bool, budget *int, depth int, bodies *[]string) (any, error) {
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
			if entries != nil && bodies == nil {
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
						if entries != nil {
							*entries = append(*entries, closedPlanDigestProjectionEntryV1{path: childPath, digest: text, placeholder: cloneQuestionReplyASTV1(parsed).(map[string]any)})
						}
					}
				}
				continue
			}
			projected, err := walkQuestionReplyV1(child, childPath, entries, resource, budget, depth+1, bodies)
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
			if entries != nil && bodies == nil {
				childPath = append(append([]closedPlanDigestProjectionPathV1(nil), path...), closedPlanDigestProjectionPathV1{kind: "index", index: index})
			}
			projected, err := walkQuestionReplyV1(child, childPath, entries, resource, budget, depth+1, bodies)
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

// Bodies are returned unchanged for recovery completeness checks, not display.
func QuestionReplyBodiesV1(value any) ([]string, error) {
	texts := []string{}
	budget := 100000
	_, err := walkQuestionReplyV1(value, nil, nil, false, &budget, 0, &texts)
	if err != nil {
		return nil, err
	}
	sort.Strings(texts)
	return texts, nil
}

func sameQuestionReplyBodiesV1(left, right any) bool {
	a, aerr := QuestionReplyBodiesV1(left)
	b, berr := QuestionReplyBodiesV1(right)
	return aerr == nil && berr == nil && reflect.DeepEqual(a, b)
}

// The expected AST never aliases the staged value given to a projector.
func cloneQuestionReplyASTV1(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = cloneQuestionReplyASTV1(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = cloneQuestionReplyASTV1(child)
		}
		return out
	default:
		return value
	}
}
