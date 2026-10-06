package toolresult

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

const (
	ProtectedSnapshotVersionV1       = 1
	ProtectedSnapshotPurposeV1       = "analytix.protected-tool-result-snapshot/v1"
	ProtectedSnapshotBindingFieldV1  = "protectedToolResultSnapshot"
	ProtectedSnapshotBodyLimitV1     = 32 << 10
	ProtectedSnapshotEnvelopeLimitV1 = 256 << 10
)

// CaptureV1 is produced from the actual host buffer/read, never decoded from
// a tool result map. Status and exit evidence do not come from body text.
type ProtectedCaptureV1 struct {
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Body       string `json:"body"`
	Label      string `json:"label"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Truncated  bool   `json:"truncated"`
	StartLine  int    `json:"startLine"`
	EndLine    int    `json:"endLine"`
	TotalLines int    `json:"totalLines"`
}

type ProtectedSnapshotV1 struct {
	Version          int                        `json:"version"`
	Purpose          string                     `json:"purpose"`
	Principal        domainidentity.PrincipalV1 `json:"principal"`
	Workspace        string                     `json:"workspace"`
	ThreadID         string                     `json:"threadId"`
	TurnID           string                     `json:"turnId"`
	CallID           string                     `json:"callId"`
	ResultItemID     string                     `json:"resultItemId"`
	ToolName         string                     `json:"toolName"`
	ContextDigest    string                     `json:"contextDigest"`
	ContextEpoch     uint64                     `json:"contextEpoch"`
	ExecutionGrantID string                     `json:"executionGrantId"`
	CaseID           string                     `json:"caseId"`
	CaseBindingHash  string                     `json:"caseBindingHash"`
	Capture          ProtectedCaptureV1         `json:"capture"`
}

type ProtectedSnapshotBindingV1 struct {
	Version        int    `json:"version"`
	Purpose        string `json:"purpose"`
	SnapshotDigest string `json:"snapshotDigest"`
	BodyDigest     string `json:"bodyDigest"`
}

// BoundProtectedTextV1 retains a UTF-8-safe prefix. An invalid byte or a
// partial trailing rune ends the display capture; no synthetic marker is body.
func BoundProtectedTextV1(body string, limit int) (string, bool) {
	if limit < 0 {
		limit = 0
	}
	end := len(body)
	if end > limit {
		end = limit
	}
	offset := 0
	for offset < end {
		rune_, size := utf8.DecodeRuneInString(body[offset:end])
		if rune_ == utf8.RuneError && size == 1 {
			break
		}
		offset += size
	}
	return body[:offset], offset < len(body)
}

func ValidateProtectedCaptureV1(c ProtectedCaptureV1) error {
	if (c.Kind != "shell" && c.Kind != "read") ||
		!oneOf(c.Status, "completed", "failed", "timeout", "canceled", "unknown") ||
		!utf8.ValidString(c.Body) || len(c.Body) > ProtectedSnapshotBodyLimitV1 ||
		!utf8.ValidString(c.Label) || len(c.Label) > 1024 || strings.ContainsRune(c.Label, 0) ||
		c.DurationMs < 0 || c.StartLine < 0 || c.EndLine < 0 || c.TotalLines < 0 {
		return errors.New("protected tool capture is invalid")
	}
	if c.Kind == "read" && (c.ExitCode != nil || c.DurationMs != 0 || c.Status != "completed" ||
		c.EndLine < c.StartLine || (c.TotalLines > 0 && c.EndLine > c.TotalLines && c.Body != "")) {
		return errors.New("protected read capture is invalid")
	}
	if c.Kind == "shell" && (c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 ||
		c.ExitCode != nil && *c.ExitCode < 0) {
		return errors.New("protected shell capture is invalid")
	}
	return nil
}

func ValidateProtectedSnapshotV1(s ProtectedSnapshotV1) error {
	if s.Version != ProtectedSnapshotVersionV1 || s.Purpose != ProtectedSnapshotPurposeV1 ||
		domainidentity.ValidatePrincipalV1(s.Principal) != nil ||
		!domainthread.IsCanonicalRecordID(s.ThreadID) || !domainthread.IsCanonicalRecordID(s.TurnID) ||
		!domainmodel.IsHostToolCallIDV1(s.CallID) || s.ResultItemID != ToolResultItemIDV1(s.TurnID, s.CallID) ||
		!domainsecurity.IsSHA256Hex(s.ContextDigest) || !domainsecurity.IsSHA256Hex(s.ExecutionGrantID) || s.ContextEpoch == 0 ||
		s.Workspace == "" || len(s.Workspace) > 4096 || !utf8.ValidString(s.Workspace) || strings.ContainsRune(s.Workspace, 0) ||
		len(s.CaseID) > 256 || !utf8.ValidString(s.CaseID) || strings.ContainsRune(s.CaseID, 0) || !domainsecurity.IsSHA256Hex(s.CaseBindingHash) ||
		!oneOf(s.ToolName, "bash", "read", "read_file") ||
		(s.ToolName == "bash") != (s.Capture.Kind == "shell") {
		return errors.New("protected tool snapshot is invalid")
	}
	return ValidateProtectedCaptureV1(s.Capture)
}

func ProtectedSnapshotBytesV1(s ProtectedSnapshotV1) ([]byte, error) {
	if err := ValidateProtectedSnapshotV1(s); err != nil {
		return nil, err
	}
	body, err := json.Marshal(s)
	if err != nil || len(body) > ProtectedSnapshotEnvelopeLimitV1 {
		return nil, errors.New("protected tool snapshot envelope exceeds limit")
	}
	return body, nil
}

func ParseProtectedSnapshotV1(body []byte) (ProtectedSnapshotV1, error) {
	var s ProtectedSnapshotV1
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: ProtectedSnapshotEnvelopeLimitV1, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: ProtectedSnapshotBodyLimitV1}); err != nil {
		return s, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return s, err
	}
	canonical, err := ProtectedSnapshotBytesV1(s)
	if err != nil || !bytes.Equal(body, canonical) {
		return ProtectedSnapshotV1{}, errors.New("protected tool snapshot is not exact canonical bytes")
	}
	return s, nil
}

func ProtectedSnapshotBindingForV1(s ProtectedSnapshotV1) (ProtectedSnapshotBindingV1, error) {
	body, err := ProtectedSnapshotBytesV1(s)
	if err != nil {
		return ProtectedSnapshotBindingV1{}, err
	}
	return ProtectedSnapshotBindingV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, SnapshotDigest: domainsecurity.SHA256Hex(body), BodyDigest: domainsecurity.SHA256Hex([]byte(s.Capture.Body))}, nil
}

func ParseProtectedSnapshotBindingV1(value any) (ProtectedSnapshotBindingV1, error) {
	var b ProtectedSnapshotBindingV1
	body, err := json.Marshal(value)
	if err != nil {
		return b, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&b); err != nil {
		return b, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF || b.Version != 1 || b.Purpose != ProtectedSnapshotPurposeV1 ||
		!domainsecurity.IsSHA256Hex(b.SnapshotDigest) || !domainsecurity.IsSHA256Hex(b.BodyDigest) {
		return ProtectedSnapshotBindingV1{}, errors.New("protected tool snapshot binding is invalid")
	}
	return b, nil
}

func ProtectedSnapshotBindingRecordV1(b ProtectedSnapshotBindingV1) map[string]any {
	if _, err := ParseProtectedSnapshotBindingV1(b); err != nil {
		return nil
	}
	body, _ := json.Marshal(b)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	return out
}

// The collector is per host execution, process-local and deliberately absent
// from maps, events and provider messages. Duplicate producer calls invalidate
// the capture instead of selecting a potentially different body.
type ProtectedCaptureCollectorV1 struct {
	mu        sync.Mutex
	candidate *ProtectedCaptureV1
	invalid   bool
}
type protectedCaptureContextKeyV1 struct{}

func WithProtectedCaptureV1(ctx context.Context, collector *ProtectedCaptureCollectorV1) context.Context {
	return context.WithValue(ctx, protectedCaptureContextKeyV1{}, collector)
}
func CaptureProtectedToolResultV1(ctx context.Context, candidate ProtectedCaptureV1) {
	if ctx == nil {
		return
	}
	collector, _ := ctx.Value(protectedCaptureContextKeyV1{}).(*ProtectedCaptureCollectorV1)
	if collector == nil {
		return
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.candidate != nil || collector.invalid || ValidateProtectedCaptureV1(candidate) != nil {
		collector.candidate = nil
		collector.invalid = true
		return
	}
	copy_ := candidate
	if candidate.ExitCode != nil {
		code := *candidate.ExitCode
		copy_.ExitCode = &code
	}
	collector.candidate = &copy_
}
func ConsumeProtectedToolResultV1(ctx context.Context) (ProtectedCaptureV1, bool) {
	if ctx == nil {
		return ProtectedCaptureV1{}, false
	}
	collector, _ := ctx.Value(protectedCaptureContextKeyV1{}).(*ProtectedCaptureCollectorV1)
	if collector == nil {
		return ProtectedCaptureV1{}, false
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.invalid || collector.candidate == nil {
		return ProtectedCaptureV1{}, false
	}
	candidate := *collector.candidate
	collector.candidate = nil
	collector.invalid = true
	return candidate, true
}
