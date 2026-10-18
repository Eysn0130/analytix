package job

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const HostChildInputMarkerFieldV1 = "hostChildInputsV1"

// HostChildInputIdentityV1 reserves an idempotency identity in the existing job.
// It never grants execution. ClaimedAt is written only by the existing durable
// run claim, and lets receipt readback distinguish commit from real ownership.
type HostChildInputIdentityV1 struct {
	SchemaVersion   int    `json:"schemaVersion"`
	ClientMessageID string `json:"clientMessageId"`
	InputDigest     string `json:"inputDigest"`
	ClaimedAt       string `json:"claimedAt,omitempty"`
}

// HostChildInputBindingV1 exists only on the actual child primary admission.
// Its IDs and context describe the committed turn/item, not a reusable grant.
type HostChildInputBindingV1 struct {
	SchemaVersion   int    `json:"schemaVersion"`
	ClientMessageID string `json:"clientMessageId"`
	InputDigest     string `json:"inputDigest"`
	ParentThreadID  string `json:"parentThreadId"`
	SourceJobID     string `json:"sourceJobId"`
	JobID           string `json:"jobId"`
	ChildThreadID   string `json:"childThreadId"`
	ChildTurnID     string `json:"childTurnId"`
	MessageID       string `json:"messageId"`
	Action          string `json:"action"`
	ContextDigest   string `json:"contextDigest"`
	ContentDigest   string `json:"contentDigest"`
	AdmittedAt      string `json:"admittedAt"`
}

func ValidateHostChildInputIdentityV1(identity *HostChildInputIdentityV1) error {
	if identity == nil || identity.SchemaVersion != 1 || !contracts.IsCanonicalOpaqueUUIDV4(identity.ClientMessageID) ||
		!domainsecurity.IsSHA256Hex(identity.InputDigest) {
		return errors.New("Host child input identity is invalid")
	}
	if identity.ClaimedAt != "" {
		if at, err := time.Parse(time.RFC3339Nano, identity.ClaimedAt); err != nil || at.IsZero() {
			return errors.New("Host child input claim is invalid")
		}
	}
	return nil
}

func CloneHostChildInputIdentityV1(identity *HostChildInputIdentityV1) *HostChildInputIdentityV1 {
	if identity == nil {
		return nil
	}
	copy := *identity
	return &copy
}

func ValidateHostChildInputRecordV1(record Record) error {
	if record.HostChildInput == nil {
		return nil
	}
	if ValidateHostChildInputIdentityV1(record.HostChildInput) != nil || record.Kind != "subagent" || !record.Background ||
		record.SecurityBinding != nil || record.CaseDelegation != nil || record.ChildCompletionReceipt != nil ||
		record.ForegroundChildHandoffReceipt != nil || record.ParentToolItemID != "" || record.ParentToolCallID != "" || record.Output != "" || record.Usage != nil ||
		!hostChildInputIDV1(record.ID) || !hostChildInputIDV1(record.ParentThreadID) ||
		!hostChildInputIDV1(record.ChildThreadID) || !hostChildInputIDV1(record.ChildTurnID) ||
		!hostChildInputIDV1(record.ContinueFrom) || record.SourceRef != record.ContinueFrom || record.ForkFrom != "" || record.AutoContinueParent {
		return errors.New("Host child input record is outside its closed run scope")
	}
	if ValidateDelegatedToolManifestV1(record.DelegatedToolManifest, record.ToolScope, record.ToolSchemaHash) != nil {
		return errors.New("Host child input delegated tools are invalid")
	}
	return nil
}

func ValidateHostChildInputBindingV1(binding HostChildInputBindingV1) error {
	if binding.SchemaVersion != 1 || !contracts.IsCanonicalOpaqueUUIDV4(binding.ClientMessageID) ||
		!domainsecurity.IsSHA256Hex(binding.InputDigest) || !domainsecurity.IsSHA256Hex(binding.ContextDigest) ||
		!domainsecurity.IsSHA256Hex(binding.ContentDigest) || (binding.Action != "start" && binding.Action != "steer") {
		return errors.New("Host child input binding is invalid")
	}
	for _, id := range []string{binding.ParentThreadID, binding.SourceJobID, binding.JobID, binding.ChildThreadID, binding.ChildTurnID, binding.MessageID} {
		if !hostChildInputIDV1(id) {
			return errors.New("Host child input binding identity is invalid")
		}
	}
	if at, err := time.Parse(time.RFC3339Nano, binding.AdmittedAt); err != nil || at.IsZero() {
		return errors.New("Host child input binding time is invalid")
	}
	return nil
}

func HostChildInputTextDigestV1(text string) string { return domainsecurity.SHA256Hex([]byte(text)) }

func (binding HostChildInputBindingV1) RecordV1() map[string]any {
	body, _ := json.Marshal(binding)
	var record map[string]any
	_ = json.Unmarshal(body, &record)
	return record
}

func DecodeHostChildInputBindingV1(value any) (HostChildInputBindingV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return HostChildInputBindingV1{}, err
	}
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{MaxBytes: 8192, MaxDepth: 2, MaxTokens: 64, MaxStringBytes: 256})
	if err != nil {
		return HostChildInputBindingV1{}, err
	}
	allowed := map[string]bool{
		"schemaVersion": true, "clientMessageId": true, "inputDigest": true, "parentThreadId": true,
		"sourceJobId": true, "jobId": true, "childThreadId": true, "childTurnId": true, "messageId": true,
		"action": true, "contextDigest": true, "contentDigest": true, "admittedAt": true,
	}
	for key := range fields {
		if !allowed[key] {
			return HostChildInputBindingV1{}, errors.New("Host child input binding has an unsupported field")
		}
	}
	var binding HostChildInputBindingV1
	if err := json.Unmarshal(body, &binding); err != nil || ValidateHostChildInputBindingV1(binding) != nil {
		return HostChildInputBindingV1{}, errors.New("Host child input binding codec is invalid")
	}
	return binding, nil
}

func hostChildInputIDV1(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return true
}
