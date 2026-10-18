package processdisplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	identity "analytix.local/runtime-go/internal/domain/identity"
	"analytix.local/runtime-go/internal/domain/secretprojection"
	security "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/domain/thread"
)

const SettledHistorySchemaV1 = "analytix.settled-process-history/v1"
const SettledHistoryAnchorSchemaV1 = "analytix.settled-process-history-anchor/v1"
const SettledHistoryFieldV1 = "privateProcessHistory"
const SettledHistoryEnvelopeLimitV1 = RawCaptureByteLimitV1
const SettledHistoryDisplayLimitV1 = 256 << 10
const SettledHistoryBlockLimitV1 = 128

var ErrSettledHistoryV1 = errors.New("settled process history unavailable")

// TerminalWitnessV1 describes a real prepared Primary terminal mutation. It
// grants no publication authority; the original terminal owner still commits it.
type TerminalWitnessV1 struct {
	Status          string `json:"status"`
	FinishedAt      string `json:"finishedAt"`
	AuthorityDigest string `json:"authorityDigest"`
	SourceDigest    string `json:"sourceDigest"`
}

// The private disk codec is explicit: BindingV1 and ToolBindingV1 intentionally
// marshal to empty objects. No viewer grant, handle registry or raw delta is
// stored; original producer labels remain inert source attribution.
type SettledToolBindingV1 struct {
	CallID        string `json:"callId"`
	ResultItemID  string `json:"resultItemId"`
	ToolName      string `json:"toolName"`
	GrantID       string `json:"grantId"`
	InputArgsHash string `json:"inputArgsHash,omitempty"`
}
type SettledBlockV1 struct {
	Display BlockV1               `json:"display"`
	Tool    *SettledToolBindingV1 `json:"tool,omitempty"`
}
type SettledHistoryV1 struct {
	SchemaVersion string                       `json:"schemaVersion"`
	ThreadID      string                       `json:"threadId"`
	TurnID        string                       `json:"turnId"`
	Principal     identity.PrincipalV1         `json:"principal"`
	Original      security.TurnSecurityContext `json:"original"`
	Terminal      TerminalWitnessV1            `json:"terminal"`
	Units         []UnitV2                     `json:"units"`
	Blocks        []SettledBlockV1             `json:"blocks"`
}
type SettledHistoryAnchorV1 struct {
	SchemaVersion string            `json:"schemaVersion"`
	ThreadID      string            `json:"threadId"`
	TurnID        string            `json:"turnId"`
	ContextDigest string            `json:"contextDigest"`
	Terminal      TerminalWitnessV1 `json:"terminal"`
	Availability  string            `json:"availability"`
	RecordDigest  string            `json:"recordDigest,omitempty"`
	Reason        string            `json:"reason,omitempty"`
}

func ValidTerminalWitnessV1(t TerminalWitnessV1) bool {
	if t.Status != "completed" && t.Status != "failed" && t.Status != "aborted" {
		return false
	}
	stamp, err := time.Parse(time.RFC3339Nano, t.FinishedAt)
	return err == nil && !stamp.IsZero() && security.IsSHA256Hex(t.AuthorityDigest) && security.IsSHA256Hex(t.SourceDigest)
}
func ValidateSettledHistoryAnchorV1(a SettledHistoryAnchorV1) error {
	if a.SchemaVersion != SettledHistoryAnchorSchemaV1 || !thread.IsCanonicalRecordID(a.ThreadID) || !thread.IsCanonicalRecordID(a.TurnID) || !security.IsSHA256Hex(a.ContextDigest) || !ValidTerminalWitnessV1(a.Terminal) {
		return ErrSettledHistoryV1
	}
	if a.Availability == "available" && security.IsSHA256Hex(a.RecordDigest) && a.Reason == "" {
		return nil
	}
	if a.Availability == "unavailable" && a.RecordDigest == "" && (a.Reason == "missing" || a.Reason == "limit" || a.Reason == "capacity" || a.Reason == "unavailable") {
		return nil
	}
	return ErrSettledHistoryV1
}
func canonicalHistoryValueV1(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, ErrSettledHistoryV1
	}
	decoded, err := security.DecodeCanonicalJSONValue(body)
	if err != nil {
		return nil, ErrSettledHistoryV1
	}
	return json.Marshal(decoded)
}
func ParseSettledHistoryAnchorV1(value any) (SettledHistoryAnchorV1, error) {
	body, err := canonicalHistoryValueV1(value)
	if err != nil || len(body) > 4096 {
		return SettledHistoryAnchorV1{}, ErrSettledHistoryV1
	}
	var a SettledHistoryAnchorV1
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil || ValidateSettledHistoryAnchorV1(a) != nil {
		return SettledHistoryAnchorV1{}, ErrSettledHistoryV1
	}
	again, err := canonicalHistoryValueV1(a)
	if err != nil || !bytes.Equal(body, again) {
		return SettledHistoryAnchorV1{}, ErrSettledHistoryV1
	}
	return a, nil
}
func SettledHistoryAnchorMapV1(a SettledHistoryAnchorV1) (map[string]any, error) {
	if ValidateSettledHistoryAnchorV1(a) != nil {
		return nil, ErrSettledHistoryV1
	}
	body, err := canonicalHistoryValueV1(a)
	if err != nil {
		return nil, err
	}
	return security.DecodeCanonicalJSONObject(body)
}
func canonicalOrdinalV1(s string, zero bool) bool {
	v, err := strconv.ParseUint(s, 10, 64)
	return err == nil && (zero || v > 0) && v <= 1<<53-1 && strconv.FormatUint(v, 10) == s
}
func historyLabelV1(s string) bool {
	return s != "" && s == strings.TrimSpace(s) && len(s) <= 256 && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n")
}

func ValidateSettledHistoryV1(v SettledHistoryV1) error {
	if v.SchemaVersion != SettledHistorySchemaV1 || !thread.IsCanonicalRecordID(v.ThreadID) || !thread.IsCanonicalRecordID(v.TurnID) || identity.ValidatePrincipalV1(v.Principal) != nil || security.ValidateTurnSecurityContext(v.Original) != nil || v.Original.ThreadID != v.ThreadID || v.Original.TurnID != v.TurnID || v.Original.TenantID != v.Principal.TenantID || v.Original.UserID != v.Principal.UserID || !ValidTerminalWitnessV1(v.Terminal) || v.Units == nil || v.Blocks == nil || len(v.Units) > SettledHistoryBlockLimitV1 || len(v.Blocks) > SettledHistoryBlockLimitV1 {
		return ErrSettledHistoryV1
	}
	units := map[string]UnitV2{}
	refs := map[string]CallAnchorV2{}
	callUnits := map[string]string{}
	for _, u := range v.Units {
		if !historyLabelV1(u.UnitID) || !historyLabelV1(u.ProducerID) || !canonicalOrdinalV1(u.ProviderSequence, true) || u.Calls == nil || u.PrecedingCalls == nil || len(u.Calls)+len(u.PrecedingCalls) > SettledHistoryBlockLimitV1 || (u.Placement != "pending" && u.Placement != "anchored" && u.Placement != "unit_only") {
			return ErrSettledHistoryV1
		}
		if _, duplicate := units[u.UnitID]; duplicate {
			return ErrSettledHistoryV1
		}
		units[u.UnitID] = u
		for _, c := range u.Calls {
			if !historyLabelV1(c.CallID) || !thread.IsCanonicalRecordID(c.ToolCallItemID) || c.ToolResultItemID != "" && !thread.IsCanonicalRecordID(c.ToolResultItemID) || c.StartedEventSeq != "" && !canonicalOrdinalV1(c.StartedEventSeq, false) {
				return ErrSettledHistoryV1
			}
			if _, duplicate := refs[c.CallID]; duplicate {
				return ErrSettledHistoryV1
			}
			refs[c.CallID] = c
			callUnits[c.CallID] = u.UnitID
		}
	}
	if len(refs) > SettledHistoryBlockLimitV1 {
		return ErrSettledHistoryV1
	}
	for _, u := range v.Units {
		for _, c := range u.PrecedingCalls {
			actual, found := refs[c.CallID]
			if !found || actual != c {
				return ErrSettledHistoryV1
			}
		}
	}
	ids := map[string]bool{}
	total := 0
	for _, entry := range v.Blocks {
		b := entry.Display
		u, exists := units[b.Source.UnitID]
		if !exists || len(b.BlockID) != 32 || strings.Trim(b.BlockID, "0123456789abcdef") != "" || ids[b.BlockID] || b.DurationMs < 0 || b.DurationMs > 1<<53-1 || b.Attempt > 1<<53-1 || b.Input != nil || b.DraftSegments != nil || b.DraftBytes != "" || b.RequestURL != "" || b.InputDigest != "" || b.ToolBinding != (ToolBindingV1{}) {
			return ErrSettledHistoryV1
		}
		ids[b.BlockID] = true
		if b.State != "completed" && b.State != "failed" && b.State != "timeout" && b.State != "canceled" && b.State != "unknown" && b.State != "superseded" {
			return ErrSettledHistoryV1
		}
		if b.Classification != "source_exact_local" && b.Classification != "credential_redacted" && b.Classification != "withheld" {
			return ErrSettledHistoryV1
		}
		if b.Classification == "withheld" && (b.Body != nil || b.Title != "" || b.Preview != "" || b.Path != "") {
			return ErrSettledHistoryV1
		}
		if b.CaptureDigest != "" && !security.IsSHA256Hex(b.CaptureDigest) || b.Body != nil && b.CaptureDigest != CaptureDigestV1(b) {
			return ErrSettledHistoryV1
		}
		if b.InputUtf16CodeUnits != nil && (b.Kind != "write" && b.Kind != "edit" || *b.InputUtf16CodeUnits < 0 || *b.InputUtf16CodeUnits > MaxInputUtf16CodeUnitsV1) {
			return ErrSettledHistoryV1
		}
		if b.Source.Kind == "native" {
			if b.Source.NativeIndex < 0 || uint64(b.Source.NativeIndex) > 1<<53-1 || b.NativeIndex == nil || *b.NativeIndex != b.Source.NativeIndex {
				return ErrSettledHistoryV1
			}
		} else if b.Source.NativeIndex != 0 || b.NativeIndex != nil {
			return ErrSettledHistoryV1
		}
		if b.Source.Kind == "tool" {
			c, found := refs[b.Source.CallID]
			if entry.Tool == nil || !found || callUnits[c.CallID] != b.Source.UnitID || c.ToolResultItemID == "" || b.Source.ToolCallItemID != c.ToolCallItemID || b.Source.ToolResultItemID != c.ToolResultItemID || b.Source.StartedEventSeq != c.StartedEventSeq || entry.Tool.CallID != c.CallID || entry.Tool.ResultItemID != c.ToolResultItemID || !historyLabelV1(entry.Tool.ToolName) || !historyLabelV1(entry.Tool.GrantID) || entry.Tool.InputArgsHash != "" && !security.IsSHA256Hex(entry.Tool.InputArgsHash) {
				return ErrSettledHistoryV1
			}
			if b.Kind != "write" && b.Kind != "edit" && b.Kind != "shell" {
				return ErrSettledHistoryV1
			}
		} else {
			if entry.Tool != nil || b.Attempt == 0 || b.Source.ProducerID != u.ProducerID || b.Source.ProviderSequence != u.ProviderSequence || b.Source.Attempt != b.Attempt || b.Source.ReasoningForm != "" && b.Source.ReasoningForm != "summary" {
				return ErrSettledHistoryV1
			}
			if b.Kind == "native_reasoning" {
				if b.Source.Kind != "native" && b.Source.Kind != "provider_reasoning" || !canonicalOrdinalV1(b.Source.SourceOrdinal, false) || b.State != "completed" && b.Body != nil {
					return ErrSettledHistoryV1
				}
			} else if b.Kind != "assistant_draft" || b.Source.Kind != "assistant_draft" || b.Body != nil || b.Classification != "withheld" {
				return ErrSettledHistoryV1
			}
		}
		texts := []string{b.Title, b.Preview, b.Path}
		if b.Body != nil {
			texts = append(texts, b.Body.Text, b.Body.Before, b.Body.After, b.Body.Error)
			if b.Body.AddedLines < 0 || b.Body.RemovedLines < 0 {
				return ErrSettledHistoryV1
			}
		}
		for _, text := range texts {
			if len(text) > CaptureBodyByteLimitV1 || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || secretprojection.ContainsCredentialV1(text) {
				return ErrSettledHistoryV1
			}
			total += len(text)
		}
		if total > SettledHistoryDisplayLimitV1 {
			return ErrSettledHistoryV1
		}
		if b.NativeIndex != nil && (*b.NativeIndex < 0 || uint64(*b.NativeIndex) > 1<<53-1) {
			return ErrSettledHistoryV1
		}
	}
	return nil
}

func SettledHistoryBytesV1(v SettledHistoryV1) ([]byte, error) {
	if ValidateSettledHistoryV1(v) != nil {
		return nil, ErrSettledHistoryV1
	}
	body, err := canonicalHistoryValueV1(v)
	if err != nil || len(body) > SettledHistoryEnvelopeLimitV1 {
		return nil, ErrSettledHistoryV1
	}
	return body, nil
}
func SettledHistoryAnchorForV1(v SettledHistoryV1) (SettledHistoryAnchorV1, error) {
	body, err := SettledHistoryBytesV1(v)
	if err != nil {
		return SettledHistoryAnchorV1{}, err
	}
	return SettledHistoryAnchorV1{SchemaVersion: SettledHistoryAnchorSchemaV1, ThreadID: v.ThreadID, TurnID: v.TurnID, ContextDigest: v.Original.ContextDigest, Terminal: v.Terminal, Availability: "available", RecordDigest: security.SHA256Hex(body)}, nil
}
func ParseSettledHistoryV1(body []byte) (SettledHistoryV1, error) {
	if len(body) == 0 || len(body) > SettledHistoryEnvelopeLimitV1 || !utf8.Valid(body) {
		return SettledHistoryV1{}, ErrSettledHistoryV1
	}
	decoded, err := security.DecodeCanonicalJSONValue(body)
	if err != nil {
		return SettledHistoryV1{}, ErrSettledHistoryV1
	}
	canonical, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(body, canonical) {
		return SettledHistoryV1{}, ErrSettledHistoryV1
	}
	var v SettledHistoryV1
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil {
		return SettledHistoryV1{}, ErrSettledHistoryV1
	}
	again, err := SettledHistoryBytesV1(v)
	if err != nil || !bytes.Equal(body, again) {
		return SettledHistoryV1{}, ErrSettledHistoryV1
	}
	return v, nil
}
