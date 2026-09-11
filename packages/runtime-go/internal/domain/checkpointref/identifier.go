package checkpointref

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	checkpointPrefix = "axcp_"
	planPrefix       = "axrp_"
	applyPrefix      = "axra_"
	rescuePrefix     = "axrr_"
	capturePrefix    = "axce_"
	versionMarker    = "v2_"

	checkpointDomain = "analytix.workspace-checkpoint-id/v2"
	planDomain       = "analytix.checkpoint-rewind-plan-id/v2"
	applyDomain      = "analytix.checkpoint-rewind-apply-id/v2"
	rescueDomain     = "analytix.checkpoint-rewind-rescue-id/v2"
	captureDomain    = "analytix.checkpoint-captured-event-id/v2"
	payloadDomain    = "analytix.checkpoint-captured-payload/v2"
	publicRefDomain  = "analytix-public-checkpoint-ref.v1"
	maxLegacyIDBytes = 512
)

type LinkKind string

const (
	PlanLink   LinkKind = "plan"
	ApplyLink  LinkKind = "apply"
	RescueLink LinkKind = "rescue"
)

func RuntimeID(sourceWorkspaceCheckpointID string) string {
	sourceWorkspaceCheckpointID = strings.TrimSpace(sourceWorkspaceCheckpointID)
	if sourceWorkspaceCheckpointID == "" {
		return ""
	}
	if IsCanonicalRuntimeIDV2(sourceWorkspaceCheckpointID) {
		return sourceWorkspaceCheckpointID
	}
	return scopedID(checkpointPrefix, checkpointDomain, sourceWorkspaceCheckpointID)
}

func PlanID(checkpointID string) string {
	return scopedID(planPrefix, planDomain, strings.TrimSpace(checkpointID))
}

func ApplyID(checkpointID string) string {
	return scopedID(applyPrefix, applyDomain, strings.TrimSpace(checkpointID))
}

func RescueID(checkpointID string) string {
	return scopedID(rescuePrefix, rescueDomain, strings.TrimSpace(checkpointID))
}

func CaptureEventID(frontierDigest string) string {
	if !isSHA256Hex(frontierDigest) {
		return ""
	}
	return scopedID(capturePrefix, captureDomain, frontierDigest)
}

func IsCaptureEventIDV2(value string) bool {
	return canonicalV2ID(value, capturePrefix)
}

// CapturedPayloadDigest binds the closed checkpoint audit payload without
// exposing the private operation frontier. Case persistence recomputes this
// after metadata-only projection; public SSE/history remove both internal
// audit markers after validating them.
func CapturedPayloadDigest(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	canonical := make(map[string]any, len(payload))
	for key, value := range payload {
		if key != "capturePayloadDigest" {
			canonical[key] = value
		}
	}
	body, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(append([]byte(payloadDomain+"\x00"), body...))
	return hex.EncodeToString(digest[:])
}

func CapturedPayloadDigestMatches(payload map[string]any) bool {
	value, ok := payload["capturePayloadDigest"].(string)
	return ok && isSHA256Hex(value) && value == CapturedPayloadDigest(payload)
}

// PublicReferenceDigest returns the irreversible checkpoint correlation used
// by metadata-only UI, SSE, history, and export projections. It deliberately
// does not expose a runtime, plan, apply, or rescue identifier and conveys no
// mutation, evidence, or publication authority.
func PublicReferenceDigest(threadID, turnID, checkpointID string) string {
	digest := sha256.Sum256([]byte(publicRefDomain + "\n" + threadID + "\n" + turnID + "\n" + checkpointID))
	return hex.EncodeToString(digest[:])
}

func IsCanonicalRuntimeIDV2(value string) bool {
	return canonicalV2ID(value, checkpointPrefix)
}

// IsLegacyRuntimeIDV1 exists only so metadata-only replay can project durable
// events written before V2 domain-separated identifiers. It must never be
// used to authorize a new checkpoint mutation or private-CAS lookup.
func IsLegacyRuntimeIDV1(value string) bool {
	if IsCanonicalRuntimeIDV2(value) || strings.HasPrefix(value, checkpointPrefix+versionMarker) ||
		!strings.HasPrefix(value, checkpointPrefix) || len(value) <= len(checkpointPrefix) || len(value) > maxLegacyIDBytes {
		return false
	}
	return safeLegacySuffix(value[len(checkpointPrefix):])
}

func IsPublicReplayRuntimeID(value string) bool {
	return IsCanonicalRuntimeIDV2(value) || IsLegacyRuntimeIDV1(value)
}

// LinkedIDMatches accepts the V2 domain-separated relationship for current
// records and the exact, deterministic concatenation used by legacy public
// replay records. The legacy branch exposes metadata only and conveys no
// checkpoint execution or evidence authority.
func LinkedIDMatches(kind LinkKind, checkpointID, linkedID string) bool {
	if checkpointID == "" || linkedID == "" || checkpointID != strings.TrimSpace(checkpointID) ||
		linkedID != strings.TrimSpace(linkedID) {
		return false
	}
	prefix, current := linkDefinition(kind, checkpointID)
	if prefix == "" {
		return false
	}
	if IsCanonicalRuntimeIDV2(checkpointID) {
		return linkedID == current && canonicalV2ID(linkedID, prefix)
	}
	if IsLegacyRuntimeIDV1(checkpointID) {
		legacy := prefix + checkpointID
		return linkedID == legacy && validLegacyLinkedID(linkedID, prefix)
	}
	return false
}

func linkDefinition(kind LinkKind, checkpointID string) (string, string) {
	switch kind {
	case PlanLink:
		return planPrefix, PlanID(checkpointID)
	case ApplyLink:
		return applyPrefix, ApplyID(checkpointID)
	case RescueLink:
		return rescuePrefix, RescueID(checkpointID)
	default:
		return "", ""
	}
}

func scopedID(prefix, domain, value string) string {
	digest := sha256.Sum256([]byte(domain + "\x00" + value))
	return prefix + versionMarker + hex.EncodeToString(digest[:])
}

func canonicalV2ID(value, prefix string) bool {
	expectedPrefix := prefix + versionMarker
	if !strings.HasPrefix(value, expectedPrefix) || len(value) != len(expectedPrefix)+sha256.Size*2 {
		return false
	}
	for _, character := range value[len(expectedPrefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func safeLegacySuffix(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validLegacyLinkedID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+checkpointPrefix) && safeLegacySuffix(value[len(prefix):])
}
