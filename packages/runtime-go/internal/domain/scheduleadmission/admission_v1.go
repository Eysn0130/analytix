package scheduleadmission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

const MaximumRequestBytesV1 = 32 * 1024
const MaximumPromptBytesV1 = 16 * 1024
const MarkerFieldV1 = "scheduleAdmissionsV1"

var ErrInvalidV1 = errors.New("schedule admission is invalid")
var ErrConflictV1 = errors.New("schedule admission identity conflicts")
var ErrExistingV1 = errors.New("schedule admission already exists")
var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// OriginV1 records historical creation provenance. It grants no future rights.
type OriginV1 struct {
	ThreadID   string `json:"threadId"`
	TurnID     string `json:"turnId"`
	ToolCallID string `json:"toolCallId"`
	GrantID    string `json:"grantId"`
}
type ExecutionV1 struct {
	ProviderID      *string `json:"providerId,omitempty"`
	Model           *string `json:"model,omitempty"`
	ReasoningEffort *string `json:"reasoningEffort,omitempty"`
	Mode            *string `json:"mode,omitempty"`
}
type RequestV1 struct {
	SchemaVersion int         `json:"schemaVersion"`
	Kind          string      `json:"kind"`
	TaskID        string      `json:"taskId"`
	Origin        OriginV1    `json:"origin"`
	AttemptID     string      `json:"attemptId"`
	ScheduledAt   string      `json:"scheduledAt"`
	Prompt        string      `json:"prompt"`
	Execution     ExecutionV1 `json:"execution"`
}
type ReceiptV1 struct {
	TaskID      string `json:"taskId"`
	AttemptID   string `json:"attemptId"`
	MessageID   string `json:"messageId"`
	ThreadID    string `json:"threadId"`
	TurnID      string `json:"turnId"`
	Delivery    string `json:"delivery"`
	ScheduledAt string `json:"scheduledAt"`
	DeliveredAt string `json:"deliveredAt"`
}
type ResponseV1 struct {
	SchemaVersion int        `json:"schemaVersion"`
	Status        string     `json:"status"`
	Code          string     `json:"code,omitempty"`
	Receipt       *ReceiptV1 `json:"receipt,omitempty"`
}

// MarkerV1 is a private primary admission fact, never an execution grant.
// ContentDigest binds the exact privacy-projected native message;
// InputDigest binds the complete frozen Host request without storing its prompt.
type MarkerV1 struct {
	SchemaVersion int       `json:"schemaVersion"`
	Origin        OriginV1  `json:"origin"`
	InputDigest   string    `json:"inputDigest"`
	ContextDigest string    `json:"contextDigest"`
	ContentDigest string    `json:"contentDigest"`
	Receipt       ReceiptV1 `json:"receipt"`
}

func ValidOriginV1(o OriginV1) bool {
	return domainthread.IsCanonicalRecordID(o.ThreadID) && domainthread.IsCanonicalRecordID(o.TurnID) && domainmodel.IsHostToolCallIDV1(o.ToolCallID) && domainthread.IsCanonicalRecordID(o.GrantID)
}
func canonicalTime(s string) bool {
	t, e := time.Parse(time.RFC3339Nano, s)
	return e == nil && t.UTC().Format(time.RFC3339Nano) == s
}
func selection(s *string) bool {
	return s == nil || (*s != "" && len(*s) <= 255 && strings.TrimSpace(*s) == *s && !strings.ContainsAny(*s, "\x00\r\n"))
}
func ValidateRequestV1(r RequestV1) error {
	if r.SchemaVersion != 1 || (r.Kind != "submit" && r.Kind != "query") || !domainthread.IsCanonicalRecordID(r.TaskID) || !ValidOriginV1(r.Origin) || !uuidV4.MatchString(r.AttemptID) || !canonicalTime(r.ScheduledAt) || !utf8.ValidString(r.Prompt) || len(r.Prompt) > MaximumPromptBytesV1 || strings.TrimSpace(r.Prompt) == "" || strings.ContainsRune(r.Prompt, '\x00') {
		return ErrInvalidV1
	}
	e := r.Execution
	if !selection(e.ProviderID) || !selection(e.Model) || !selection(e.ReasoningEffort) || !selection(e.Mode) {
		return ErrInvalidV1
	}
	if e.ReasoningEffort != nil && domainmodel.ValidateReasoningEffortV1(*e.ReasoningEffort) != nil {
		return ErrInvalidV1
	}
	if e.Mode != nil && *e.Mode != "agent" && *e.Mode != "plan" {
		return ErrInvalidV1
	}
	return nil
}
func exactObject(raw json.RawMessage, required, optional []string) (map[string]json.RawMessage, error) {
	if jsonstrict.Validate(raw, jsonstrict.Options{RequireObject: true, MaxBytes: MaximumRequestBytesV1, MaxDepth: 8}) != nil {
		return nil, ErrInvalidV1
	}
	var o map[string]json.RawMessage
	if json.Unmarshal(raw, &o) != nil {
		return nil, ErrInvalidV1
	}
	allowed := map[string]bool{}
	for _, k := range required {
		allowed[k] = true
		if len(o[k]) == 0 {
			return nil, ErrInvalidV1
		}
	}
	for _, k := range optional {
		allowed[k] = true
	}
	for k, v := range o {
		if !allowed[k] || strings.TrimSpace(string(v)) == "null" {
			return nil, ErrInvalidV1
		}
	}
	return o, nil
}
func DecodeRequestV1(raw []byte) (RequestV1, error) {
	o, e := exactObject(raw, []string{"schemaVersion", "kind", "taskId", "origin", "attemptId", "scheduledAt", "prompt", "execution"}, nil)
	if e != nil {
		return RequestV1{}, e
	}
	if _, e = exactObject(o["origin"], []string{"threadId", "turnId", "toolCallId", "grantId"}, nil); e != nil {
		return RequestV1{}, e
	}
	if _, e = exactObject(o["execution"], nil, []string{"providerId", "model", "reasoningEffort", "mode"}); e != nil {
		return RequestV1{}, e
	}
	var r RequestV1
	if json.Unmarshal(raw, &r) != nil {
		return r, ErrInvalidV1
	}
	return r, ValidateRequestV1(r)
}
func TextDigestV1(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
func InputDigestV1(r RequestV1) string {
	r.Kind = ""
	r.SchemaVersion = 0
	b, _ := json.Marshal(r)
	return TextDigestV1(string(b))
}
func NewMarkerV1(r RequestV1, turnID, messageID, delivery, at, contextDigest, content string) MarkerV1 {
	return MarkerV1{1, r.Origin, InputDigestV1(r), contextDigest, TextDigestV1(content), ReceiptV1{r.TaskID, r.AttemptID, messageID, r.Origin.ThreadID, turnID, delivery, r.ScheduledAt, at}}
}
func (m MarkerV1) RecordV1() map[string]any {
	b, _ := json.Marshal(m)
	var r map[string]any
	_ = json.Unmarshal(b, &r)
	return r
}
func DecodeMarkerV1(value any) (MarkerV1, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return MarkerV1{}, ErrInvalidV1
	}
	o, e := exactObject(b, []string{"schemaVersion", "origin", "inputDigest", "contextDigest", "contentDigest", "receipt"}, nil)
	if e != nil {
		return MarkerV1{}, e
	}
	if _, e = exactObject(o["origin"], []string{"threadId", "turnId", "toolCallId", "grantId"}, nil); e != nil {
		return MarkerV1{}, e
	}
	if _, e = exactObject(o["receipt"], []string{"taskId", "attemptId", "messageId", "threadId", "turnId", "delivery", "scheduledAt", "deliveredAt"}, nil); e != nil {
		return MarkerV1{}, e
	}
	var m MarkerV1
	if json.Unmarshal(b, &m) != nil {
		return m, ErrInvalidV1
	}
	r := m.Receipt
	if m.SchemaVersion != 1 || !ValidOriginV1(m.Origin) || !domainsecurity.IsSHA256Hex(m.InputDigest) || !domainsecurity.IsSHA256Hex(m.ContextDigest) || !domainsecurity.IsSHA256Hex(m.ContentDigest) || !domainthread.IsCanonicalRecordID(r.TaskID) || !uuidV4.MatchString(r.AttemptID) || !domainthread.IsCanonicalRecordID(r.MessageID) || !domainthread.IsCanonicalRecordID(r.TurnID) || r.ThreadID != m.Origin.ThreadID || (r.Delivery != "start" && r.Delivery != "steer") || !canonicalTime(r.ScheduledAt) || !canonicalTime(r.DeliveredAt) {
		return m, ErrInvalidV1
	}
	return m, nil
}
func ResultV1(status string, receipt *ReceiptV1) ResponseV1 {
	r := ResponseV1{SchemaVersion: 1, Status: status, Receipt: receipt}
	if status == "conflict" {
		r.Code = "schedule_admission_conflict"
	} else if status == "rejected" || status == "unknown" {
		r.Code = "schedule_admission_unavailable"
	}
	return r
}
func (r ResponseV1) HTTPStatusV1() int {
	switch r.Status {
	case "accepted", "not_found":
		return 200
	case "conflict", "rejected":
		return 409
	case "invalid":
		return 400
	default:
		return 503
	}
}
