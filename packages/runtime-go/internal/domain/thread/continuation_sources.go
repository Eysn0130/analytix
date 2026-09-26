package thread

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const ContinuationSourceBudgetBytes = 1 << 20
const ContinuationInlineBudgetBytes = 16 << 10

// These are chronological original user messages, not inferred permanent
// constraints, system instructions, or evidence. Newer corrections remain
// after their sources. Tool/model output cannot create an entry in this list.
type ContinuationUserSourceV1 struct {
	Reference string `json:"reference"`
	Text      string `json:"text"`
	Digest    string `json:"digest"`
}

type ContinuationUserHistoryV1 struct {
	Version     string                     `json:"version"`
	ScopeDigest string                     `json:"scopeDigest"`
	Sources     []ContinuationUserSourceV1 `json:"sources"`
}

func ContinuationSourceDigestV1(scope, reference, text string) string {
	body, _ := json.Marshal([]string{"continuation-user-source.v1", scope, reference, text})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (history *ContinuationUserHistoryV1) Validate() error {
	if history == nil {
		return nil
	}
	if history.Version != "continuation-user-history.v1" || !isContinuationSHA256V1(history.ScopeDigest) || len(history.Sources) > 256 {
		return errors.New("continuation user history is invalid")
	}
	bytes := 0
	seen := map[string]bool{}
	for _, source := range history.Sources {
		bytes += len(source.Text)
		if bytes > ContinuationSourceBudgetBytes || !utf8.ValidString(source.Text) || strings.TrimSpace(source.Text) == "" ||
			!isContinuationSHA256V1(source.Reference) || seen[source.Reference] || source.Digest != ContinuationSourceDigestV1(history.ScopeDigest, source.Reference, source.Text) {
			return errors.New("continuation user source is invalid or exceeds its budget")
		}
		seen[source.Reference] = true
	}
	return nil
}

// ContinuationProviderReferenceV1 is a stable, domain-separated selector for
// an existing local digest. Its letter-only body cannot collide with decimal
// PII detection. It grants no authority; reads still resolve against the current
// thread/principal/workspace. Persisted sealed references stay unchanged.
func ContinuationProviderReferenceV1(digest string) string {
	if !isContinuationSHA256V1(digest) {
		return ""
	}
	sum := sha256.Sum256([]byte("analytix.continuation-provider-reference/v1\x00" + digest))
	body := make([]byte, 64)
	for i, value := range sum {
		body[2*i] = 'a' + value>>4
		body[2*i+1] = 'a' + value&15
	}
	return "hist1_" + string(body)
}

func ValidContinuationProviderReferenceV1(value string) bool {
	if len(value) != 70 || !strings.HasPrefix(value, "hist1_") {
		return false
	}
	for _, ch := range value[6:] {
		if ch < 'a' || ch > 'p' {
			return false
		}
	}
	return true
}

// ProviderContinuationMapV1 leaves persisted sealed snapshots unchanged. Large
// sources become bounded references for the scope-checked read_task_history
// tool; they are never silently truncated to a supposed complete constraint.
func ProviderContinuationMapV1(snapshot TaskContinuationSnapshotV1) map[string]any {
	out := TaskContinuationSnapshotMapV1(snapshot)
	delete(out, "stateDigest")
	out["sourceSnapshotReference"] = ContinuationProviderReferenceV1(snapshot.StateDigest)
	out["projectionVersion"] = "provider-continuation-view.v2"
	if snapshot.UserHistory == nil {
		return out
	}
	bytes := 0
	for _, source := range snapshot.UserHistory.Sources {
		bytes += len(source.Text)
	}
	refs := make([]map[string]any, 0, len(snapshot.UserHistory.Sources))
	for _, source := range snapshot.UserHistory.Sources {
		ref := map[string]any{"reference": ContinuationProviderReferenceV1(source.Reference), "sourceCommitment": ContinuationProviderReferenceV1(source.Digest), "runes": utf8.RuneCountInString(source.Text)}
		if bytes <= ContinuationInlineBudgetBytes {
			ref["text"] = source.Text
		}
		refs = append(refs, ref)
	}
	out["userHistory"] = map[string]any{"version": snapshot.UserHistory.Version, "scopeCommitment": ContinuationProviderReferenceV1(snapshot.UserHistory.ScopeDigest), "sources": refs, "readTool": "read_task_history", "chronological": true}
	return out
}
