package toolresult

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

const (
	ProtectedSnapshotVersionV1           = 1
	ProtectedSnapshotPurposeV1           = "analytix.protected-tool-result-snapshot/v1"
	ProtectedSnapshotBindingFieldV1      = "protectedToolResultSnapshot"
	ProtectedSnapshotBodyLimitV1         = 32 << 10
	ProtectedSnapshotEnvelopeLimitV1     = 256 << 10
	ProtectedTodoSnapshotEnvelopeLimitV1 = 2 << 20
	ProtectedWebSnapshotBodyLimitV1      = 1_000_000
	ProtectedWebSnapshotEnvelopeLimitV1  = 8 << 20
	ProtectedWebSnapshotJSONLimitV1      = 64 << 10
	ProtectedWebSnapshotURLLimitV1       = 8192
	ProtectedWebSnapshotTitleLimitV1     = 4096
	ProtectedWebSnapshotErrorLimitV1     = 32 << 10
)

// CaptureV1 is produced from the actual host buffer/read, never decoded from
// a tool result map. Status and exit evidence do not come from body text.
// Read targets are private Host observations, sealed by the snapshot digest.
// The label is display text and cannot supply a navigation target.
type ProtectedReadTargetV1 struct {
	Path          string `json:"path"`
	StartLine     int    `json:"startLine"`
	ContentDigest string `json:"contentDigest"`
}

// Web captures are local observations; public projections never contain these
// fields. Targets are kept separately so display text is not effect authority.
type ProtectedWebErrorV1 struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ProtectedWebCaptureV1 struct {
	RequestURL          string               `json:"requestUrl,omitempty"`
	FinalURL            string               `json:"finalUrl,omitempty"`
	RequestURLTruncated bool                 `json:"requestUrlTruncated"`
	FinalURLTruncated   bool                 `json:"finalUrlTruncated"`
	InputJSON           string               `json:"inputJSON,omitempty"`
	SchemaJSON          string               `json:"schemaJSON,omitempty"`
	InputTruncated      bool                 `json:"inputTruncated"`
	SchemaTruncated     bool                 `json:"schemaTruncated"`
	Title               string               `json:"title,omitempty"`
	StatusCode          *int                 `json:"statusCode,omitempty"`
	FetchTruncated      *bool                `json:"fetchTruncated,omitempty"`
	ResponseComplete    bool                 `json:"responseComplete"`
	ToolIsError         bool                 `json:"toolIsError"`
	Error               *ProtectedWebErrorV1 `json:"error,omitempty"`
	StartedAt           string               `json:"startedAt"`
}
type ProtectedWebTargetsV1 struct {
	RequestURL string `json:"requestUrl,omitempty"`
	ResultURL  string `json:"resultUrl,omitempty"`
}

type ProtectedCaptureV1 struct {
	Kind           string                      `json:"kind"`
	Status         string                      `json:"status"`
	Body           string                      `json:"body"`
	Label          string                      `json:"label"`
	ExitCode       *int                        `json:"exitCode,omitempty"`
	DurationMs     int64                       `json:"durationMs"`
	Truncated      bool                        `json:"truncated"`
	StartLine      int                         `json:"startLine"`
	EndLine        int                         `json:"endLine"`
	TotalLines     int                         `json:"totalLines"`
	ReadTarget     *ProtectedReadTargetV1      `json:"readTarget,omitempty"`
	Web            *ProtectedWebCaptureV1      `json:"web,omitempty"`
	WebTargets     *ProtectedWebTargetsV1      `json:"webTargets,omitempty"`
	Search         *ProtectedSearchCaptureV1   `json:"search,omitempty"`
	Goal           *ProtectedGoalCaptureV1     `json:"goal,omitempty"`
	Todo           *ProtectedTodoCaptureV1     `json:"todo,omitempty"`
	Skill          *ProtectedSkillCaptureV1    `json:"skill,omitempty"`
	Job            *ProtectedJobCaptureV1      `json:"job,omitempty"`
	Schedule       *ProtectedScheduleReceiptV1 `json:"schedule,omitempty"`
	Present        *ProtectedPresentReceiptV1  `json:"present,omitempty"`
	PresentTargets []ProtectedPresentTargetV1  `json:"presentTargets,omitempty"`
	FileResult     *ProtectedFileResultV1      `json:"fileResult,omitempty"`
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
	Inspect          *ProtectedInspectV1        `json:"inspect,omitempty"`
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
	bodyLimit := ProtectedSnapshotBodyLimitV1
	if c.Kind == "web_fetch" {
		bodyLimit = ProtectedWebSnapshotBodyLimitV1
	}
	if !oneOf(c.Kind, "shell", "read", "web_fetch", "search", "goal", "todo", "skill", "job", "schedule", "present", "inspect", "file_result") ||
		!oneOf(c.Status, "completed", "failed", "timeout", "canceled", "unknown") ||
		!utf8.ValidString(c.Body) || len(c.Body) > bodyLimit ||
		!utf8.ValidString(c.Label) || len(c.Label) > 1024 || strings.ContainsRune(c.Label, 0) ||
		c.DurationMs < 0 || c.StartLine < 0 || c.EndLine < 0 || c.TotalLines < 0 {
		return errors.New("protected tool capture is invalid")
	}
	if (c.FileResult != nil) != (c.Kind == "file_result") {
		return errors.New("protected file result kind is invalid")
	}
	if c.Kind == "file_result" {
		return validateProtectedFileResultCaptureV1(c)
	}
	if (c.Goal != nil) != (c.Kind == "goal") || (c.Todo != nil) != (c.Kind == "todo") || (c.Skill != nil) != (c.Kind == "skill") {
		return errors.New("protected domain capture kind is invalid")
	}
	if (c.Job != nil) != (c.Kind == "job") {
		return errors.New("protected job capture kind is invalid")
	}
	if (c.Schedule != nil) != (c.Kind == "schedule") {
		return errors.New("protected schedule capture kind is invalid")
	}
	if (c.Present != nil) != (c.Kind == "present") || c.Kind != "present" && c.PresentTargets != nil {
		return errors.New("protected present capture kind is invalid")
	}
	if c.Kind == "present" {
		return validateProtectedPresentCaptureV1(c)
	}
	if c.Kind == "schedule" {
		return validateProtectedScheduleCaptureV1(c)
	}
	if c.Kind == "job" {
		return validateProtectedJobCaptureV1(c)
	}
	if c.Kind == "inspect" && (c.Body != "" || c.Label != "" || c.Truncated || c.DurationMs != 0 || c.ExitCode != nil || c.ReadTarget != nil || c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0) {
		return errors.New("protected inspect capture is invalid")
	}
	if oneOf(c.Kind, "goal", "todo", "skill") {
		return validateProtectedDomainCaptureV1(c)
	}
	if c.Kind == "read" && (c.ExitCode != nil || c.DurationMs != 0 || c.Status != "completed" ||
		c.EndLine < c.StartLine || (c.TotalLines > 0 && c.EndLine > c.TotalLines && c.Body != "")) {
		return errors.New("protected read capture is invalid")
	}
	if c.Kind == "shell" && (c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 ||
		c.ExitCode != nil && *c.ExitCode < 0) {
		return errors.New("protected shell capture is invalid")
	}
	if target := c.ReadTarget; target != nil {
		if c.Kind != "read" || !filepath.IsAbs(target.Path) || filepath.Clean(target.Path) != target.Path ||
			!utf8.ValidString(target.Path) || len(target.Path) > 4096 || strings.ContainsRune(target.Path, 0) ||
			target.StartLine != max(1, c.StartLine) || !domainsecurity.IsSHA256Hex(target.ContentDigest) {
			return errors.New("protected read target is invalid")
		}
	}
	if c.Kind == "search" {
		if err := validateProtectedSearchV1(c); err != nil {
			return err
		}
	} else if c.Search != nil {
		return errors.New("search data cannot enter another capture kind")
	}
	if c.Kind == "web_fetch" {
		if c.Web == nil || c.ReadTarget != nil || c.ExitCode != nil || c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 || validateProtectedWebCaptureV1(*c.Web) != nil || (c.Status == "completed") != c.Web.ResponseComplete {
			return errors.New("protected web capture is invalid")
		}
		if targets := c.WebTargets; targets != nil {
			if targets.RequestURL == "" && targets.ResultURL == "" ||
				targets.RequestURL != "" && (targets.RequestURL != c.Web.RequestURL || !ValidProtectedWebTargetV1(targets.RequestURL)) ||
				targets.ResultURL != "" && (targets.ResultURL != c.Web.FinalURL || !ValidProtectedWebTargetV1(targets.ResultURL)) {
				return errors.New("protected web target is invalid")
			}
		}
	} else if c.Web != nil || c.WebTargets != nil {
		return errors.New("web data cannot enter another capture kind")
	}
	return nil
}

func protectedWebTextV1(text string, limit int) bool {
	return utf8.ValidString(text) && len(text) <= limit && !strings.ContainsRune(text, 0)
}
func protectedWebJSONV1(text string) bool {
	return text == "" || domainjsonstrict.Validate([]byte(text), domainjsonstrict.Options{RequireObject: true, MaxBytes: ProtectedWebSnapshotJSONLimitV1, MaxDepth: 64, MaxTokens: 200_000, MaxStringBytes: ProtectedWebSnapshotJSONLimitV1}) == nil
}
func validateProtectedWebCaptureV1(w ProtectedWebCaptureV1) error {
	_, timeErr := time.Parse(time.RFC3339Nano, w.StartedAt)
	if !protectedWebTextV1(w.RequestURL, ProtectedWebSnapshotURLLimitV1) || !protectedWebTextV1(w.FinalURL, ProtectedWebSnapshotURLLimitV1) ||
		!protectedWebTextV1(w.Title, ProtectedWebSnapshotTitleLimitV1) || !protectedWebTextV1(w.StartedAt, 64) || timeErr != nil ||
		!protectedWebJSONV1(w.InputJSON) || !protectedWebJSONV1(w.SchemaJSON) ||
		w.InputTruncated && w.InputJSON != "" || w.SchemaTruncated && w.SchemaJSON != "" ||
		w.RequestURLTruncated && w.RequestURL != "" || w.FinalURLTruncated && w.FinalURL != "" ||
		w.StatusCode != nil && (*w.StatusCode < 100 || *w.StatusCode > 599) ||
		w.ResponseComplete && (w.StatusCode == nil || w.FetchTruncated == nil) || !w.ResponseComplete && w.FetchTruncated != nil ||
		!w.ResponseComplete && !w.ToolIsError {
		return errors.New("protected web observation is invalid")
	}
	if e := w.Error; e != nil {
		if !w.ToolIsError || e.Code == "" || !protectedWebTextV1(e.Code, 256) || !utf8.ValidString(e.Message) || len(e.Message) > ProtectedWebSnapshotErrorLimitV1 {
			return errors.New("protected web error is invalid")
		}
	}
	return nil
}
func ValidProtectedWebTargetV1(target string) bool {
	if strings.ContainsFunc(target, unicode.IsControl) {
		return false
	}
	parsed, err := url.Parse(target)
	return protectedWebTextV1(target, ProtectedWebSnapshotURLLimitV1) && err == nil && parsed != nil && parsed.IsAbs() &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil
}

func ValidateProtectedSnapshotV1(s ProtectedSnapshotV1) error {
	if s.Version != ProtectedSnapshotVersionV1 || s.Purpose != ProtectedSnapshotPurposeV1 ||
		domainidentity.ValidatePrincipalV1(s.Principal) != nil ||
		!domainthread.IsCanonicalRecordID(s.ThreadID) || !domainthread.IsCanonicalRecordID(s.TurnID) ||
		!domainmodel.IsHostToolCallIDV1(s.CallID) || s.ResultItemID != ToolResultItemIDV1(s.TurnID, s.CallID) ||
		!domainsecurity.IsSHA256Hex(s.ContextDigest) || !domainsecurity.IsSHA256Hex(s.ExecutionGrantID) || s.ContextEpoch == 0 ||
		s.Workspace == "" || len(s.Workspace) > 4096 || !utf8.ValidString(s.Workspace) || strings.ContainsRune(s.Workspace, 0) ||
		len(s.CaseID) > 256 || !utf8.ValidString(s.CaseID) || strings.ContainsRune(s.CaseID, 0) || !domainsecurity.IsSHA256Hex(s.CaseBindingHash) ||
		s.ToolName == "" || strings.TrimSpace(s.ToolName) != s.ToolName || !utf8.ValidString(s.ToolName) || len(s.ToolName) > 256 || strings.ContainsRune(s.ToolName, 0) ||
		ValidateProtectedInspectV1(s.Inspect) != nil {
		return errors.New("protected tool snapshot is invalid")
	}
	if s.Inspect != nil && s.Inspect.SchemaJSON != "" && domainmodel.ValidateToolDefinitionSnapshotV1(&domainmodel.ToolDefinitionSnapshotV1{SchemaJSON: s.Inspect.SchemaJSON}, s.ToolName) != nil {
		return errors.New("protected inspect definition is not this call's model-visible tool")
	}
	if s.Inspect != nil && s.Inspect.InputJSON == "" && !s.Inspect.InputTruncated {
		return errors.New("protected inspect original input is unavailable")
	}
	if s.Inspect != nil && s.Inspect.OutputText != nil && (s.Capture.Kind != "inspect" || s.Capture.Status != "completed" ||
		!strings.HasPrefix(s.ToolName, "mcp__") || s.Inspect.OutputError != nil) {
		return errors.New("protected success text has no admitted generic outcome")
	}
	if s.Inspect != nil && s.Inspect.OutputError != nil && !oneOf(s.Capture.Status, "failed", "timeout", "canceled") && !(s.Capture.Web != nil && s.Capture.Web.ToolIsError || s.Capture.Search != nil && s.Capture.Search.ToolIsError) {
		return errors.New("protected error has no error outcome")
	}
	// Missing legacy request facts remain invalid. New snapshots carry request
	// facts in Inspect only, so they cannot acquire a second historical value.
	if search := s.Capture.Search; search != nil {
		if s.Inspect == nil && (search.InputJSON == "") != search.InputTruncated || s.Inspect != nil && (search.InputJSON != "" || search.InputTruncated) {
			return errors.New("protected search request ownership is invalid")
		}
	}
	if web := s.Capture.Web; web != nil && s.Inspect != nil && (web.InputJSON != "" || web.InputTruncated || web.SchemaJSON != "" || web.SchemaTruncated) {
		return errors.New("protected web request ownership is invalid")
	}
	if s.Capture.Kind == "file_result" {
		if s.Capture.FileResult == nil || s.Capture.FileResult.Operation != s.ToolName || s.Inspect == nil {
			return errors.New("protected file result tool is invalid")
		}
		return ValidateProtectedCaptureV1(s.Capture)
	}
	if s.Capture.Kind == "inspect" {
		if s.Inspect == nil || s.Inspect.InputJSON == "" && !s.Inspect.InputTruncated {
			return errors.New("protected inspect request is unavailable")
		}
		return ValidateProtectedCaptureV1(s.Capture)
	}
	if s.Capture.Kind == "present" {
		if s.ToolName != "present" || s.Inspect == nil || ValidateProtectedPresentTargetsV1(s.Capture.Present, s.Capture.PresentTargets) != nil {
			return errors.New("protected present tool is invalid")
		}
		return ValidateProtectedCaptureV1(s.Capture)
	}
	if s.Capture.Kind == "schedule" {
		if s.Capture.Schedule == nil || ProtectedScheduleOperationForToolV1(s.ToolName) == "" ||
			s.Capture.Schedule.Operation != ProtectedScheduleOperationForToolV1(s.ToolName) {
			return errors.New("protected schedule tool is invalid")
		}
		return ValidateProtectedCaptureV1(s.Capture)
	}
	if s.Capture.Kind == "job" {
		operation := map[string]string{"task": "spawn", "delegate_task": "spawn", "parallel_tasks": "spawn", "list_jobs": "list", "wait": "wait", "bash_output": "output", "kill_shell": "cancel", "restart_job": "restart"}[s.ToolName]
		j := s.Capture.Job
		// Private producer bytes retain their original admitted call identity.
		// This CAS-only source is not a settled result's display variant.
		producerOutput := s.Inspect != nil && s.Capture.Status == "completed" &&
			j != nil && j.Operation == "output" && j.Receipt == nil &&
			j.Total == 1 && !j.Truncated && len(j.Jobs) == 1 && j.Jobs[0].Output.Availability == "available" &&
			(oneOf(s.ToolName, "task", "delegate_task", "parallel_tasks", "run_skill") &&
				j.Jobs[0].Kind == "subagent" && j.Jobs[0].Status == "completed" ||
				s.ToolName == "bash" && j.Jobs[0].Kind == "background-shell" &&
					oneOf(j.Jobs[0].Status, "completed", "failed", "timeout", "canceled", "killed", "interrupted", "aborted"))
		if !producerOutput && (operation == "" || j == nil || j.Operation != operation) {
			return errors.New("protected job tool is invalid")
		}
		return ValidateProtectedCaptureV1(s.Capture)
	}
	if !oneOf(s.ToolName, "bash", "read", "read_file", "web_fetch", "grep", "glob", "create_goal", "get_goal", "update_goal", "todo_write", "run_skill") ||
		(s.ToolName == "bash") != (s.Capture.Kind == "shell") ||
		(s.ToolName == "web_fetch") != (s.Capture.Kind == "web_fetch") ||
		(s.ToolName == "grep" || s.ToolName == "glob") != (s.Capture.Kind == "search") ||
		oneOf(s.ToolName, "create_goal", "get_goal", "update_goal") != (s.Capture.Kind == "goal") ||
		(s.ToolName == "todo_write") != (s.Capture.Kind == "todo") ||
		(s.ToolName == "run_skill") != (s.Capture.Kind == "skill") ||
		s.Capture.Kind == "goal" && (s.Capture.Goal == nil || s.Capture.Goal.Goal != nil && s.Capture.Goal.Goal.ThreadID != s.ThreadID) ||
		s.Capture.Kind == "todo" && (s.Capture.Todo == nil || s.Capture.Todo.Todos.ThreadID != s.ThreadID) ||
		s.Capture.Kind == "search" && (s.Capture.Search == nil || (s.ToolName == "grep") != (s.Capture.Search.Shape == "matches")) ||
		s.Capture.Kind == "web_fetch" && (s.Capture.Web == nil || s.Inspect == nil && s.Capture.Web.InputJSON == "" && !s.Capture.Web.InputTruncated) {
		return errors.New("protected tool snapshot is invalid")
	}
	return ValidateProtectedCaptureV1(s.Capture)
}

// A producer source can be stored in the existing private CAS, but only an
// actual operation result can enter settlement or protected local display.
func ValidateProtectedDisplaySnapshotV1(s ProtectedSnapshotV1) error {
	if err := ValidateProtectedSnapshotV1(s); err != nil {
		return err
	}
	if s.Capture.Kind == "job" {
		operation := map[string]string{"task": "spawn", "delegate_task": "spawn", "parallel_tasks": "spawn", "list_jobs": "list", "wait": "wait", "bash_output": "output", "kill_shell": "cancel", "restart_job": "restart"}[s.ToolName]
		if operation == "" || s.Capture.Job == nil || s.Capture.Job.Operation != operation {
			return errors.New("protected producer source is not a display result")
		}
	}
	return nil
}

func ProtectedSnapshotBytesV1(s ProtectedSnapshotV1) ([]byte, error) {
	if err := ValidateProtectedSnapshotV1(s); err != nil {
		return nil, err
	}
	body, err := json.Marshal(s)
	limit := ProtectedSnapshotEnvelopeLimitV1
	if s.Capture.Kind == "web_fetch" {
		limit = ProtectedWebSnapshotEnvelopeLimitV1
	} else if s.Capture.Kind == "todo" {
		limit = ProtectedTodoSnapshotEnvelopeLimitV1
	}
	if err != nil || len(body) > limit {
		return nil, errors.New("protected tool snapshot envelope exceeds limit")
	}
	return body, nil
}

func ParseProtectedSnapshotV1(body []byte) (ProtectedSnapshotV1, error) {
	var s ProtectedSnapshotV1
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: ProtectedWebSnapshotEnvelopeLimitV1, MaxDepth: 12, MaxTokens: 100_000, MaxStringBytes: ProtectedWebSnapshotBodyLimitV1}); err != nil {
		return s, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return s, err
	}
	if s.Inspect == nil && s.Capture.Kind != "search" && s.Capture.Kind != "todo" && s.Capture.Kind != "job" {
		if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: ProtectedWebSnapshotEnvelopeLimitV1, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: ProtectedWebSnapshotBodyLimitV1}); err != nil {
			return ProtectedSnapshotV1{}, err
		}
	}
	limit := ProtectedSnapshotEnvelopeLimitV1
	if s.Capture.Kind == "web_fetch" {
		limit = ProtectedWebSnapshotEnvelopeLimitV1
	}
	if s.Capture.Kind == "todo" {
		limit = ProtectedTodoSnapshotEnvelopeLimitV1
	}
	if len(body) > limit {
		return ProtectedSnapshotV1{}, errors.New("protected tool snapshot envelope exceeds limit")
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
	mu          sync.Mutex
	candidate   *ProtectedCaptureV1
	invalid     bool
	consumed    bool
	outputError *ProtectedOutputErrorV1
	outputText  *ProtectedOutputTextV1
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
	if collector.consumed {
		return
	}
	if collector.candidate != nil || collector.invalid || ValidateProtectedCaptureV1(candidate) != nil {
		collector.candidate = nil
		collector.invalid = true
		return
	}
	copy_ := candidate
	if candidate.ReadTarget != nil {
		target := *candidate.ReadTarget
		copy_.ReadTarget = &target
	}
	if candidate.ExitCode != nil {
		code := *candidate.ExitCode
		copy_.ExitCode = &code
	}
	if candidate.Web != nil {
		web := *candidate.Web
		if web.StatusCode != nil {
			status := *web.StatusCode
			web.StatusCode = &status
		}
		if web.FetchTruncated != nil {
			truncated := *web.FetchTruncated
			web.FetchTruncated = &truncated
		}
		if web.Error != nil {
			error_ := *web.Error
			web.Error = &error_
		}
		copy_.Web = &web
	}
	if candidate.WebTargets != nil {
		targets := *candidate.WebTargets
		copy_.WebTargets = &targets
	}
	copy_.Search = cloneProtectedSearchV1(candidate.Search)
	copy_.Job = cloneProtectedJobCaptureV1(candidate.Job)
	copy_.Schedule = cloneProtectedScheduleReceiptV1(candidate.Schedule)
	copy_.Present = cloneProtectedPresentReceiptV1(candidate.Present)
	copy_.FileResult = cloneProtectedFileResultV1(candidate.FileResult)
	copy_.PresentTargets = append([]ProtectedPresentTargetV1(nil), candidate.PresentTargets...)
	copy_.Goal, copy_.Todo, copy_.Skill = cloneProtectedDomainCaptureV1(candidate)
	collector.candidate = &copy_
}
func ConsumeProtectedToolResultV1(ctx context.Context) (ProtectedCaptureV1, bool) {
	capture, present, _ := ConsumeProtectedToolObservationV1(ctx)
	return capture, present
}

// An attached error is not a second capture candidate. It shares the existing
// call-scoped collector and is consumed/sealed once with any Todo/Job capture.
func CaptureProtectedToolErrorV1(ctx context.Context, value *ProtectedOutputErrorV1) {
	if ctx == nil {
		return
	}
	collector, _ := ctx.Value(protectedCaptureContextKeyV1{}).(*ProtectedCaptureCollectorV1)
	if collector == nil {
		return
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.consumed {
		return
	}
	if collector.outputError != nil || ValidateProtectedOutputErrorV1(value) != nil || value == nil {
		collector.outputError = &ProtectedOutputErrorV1{Availability: "unavailable", Reason: "invalid"}
		return
	}
	collector.outputError = cloneProtectedOutputErrorV1(value)
}

// Existing observers deliberately discard the new private success text. There
// is still one atomic consume owner; no second read can retrieve either body.
func ConsumeProtectedToolObservationV1(ctx context.Context) (ProtectedCaptureV1, bool, *ProtectedOutputErrorV1) {
	capture, present, error_, _ := ConsumeProtectedToolObservationWithTextV1(ctx)
	return capture, present, error_
}

func CaptureProtectedToolTextV1(ctx context.Context, value *ProtectedOutputTextV1) {
	if ctx == nil || value == nil {
		return
	}
	collector, _ := ctx.Value(protectedCaptureContextKeyV1{}).(*ProtectedCaptureCollectorV1)
	if collector == nil {
		return
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.consumed {
		return
	}
	if collector.outputText != nil || ValidateProtectedOutputTextV1(value) != nil {
		collector.outputText = &ProtectedOutputTextV1{Availability: "unavailable", Reason: "invalid"}
		return
	}
	collector.outputText = cloneProtectedOutputTextV1(value)
}

func ConsumeProtectedToolObservationWithTextV1(ctx context.Context) (ProtectedCaptureV1, bool, *ProtectedOutputErrorV1, *ProtectedOutputTextV1) {
	if ctx == nil {
		return ProtectedCaptureV1{}, false, nil, nil
	}
	collector, _ := ctx.Value(protectedCaptureContextKeyV1{}).(*ProtectedCaptureCollectorV1)
	if collector == nil {
		return ProtectedCaptureV1{}, false, nil, nil
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if collector.consumed {
		return ProtectedCaptureV1{}, false, nil, nil
	}
	collector.consumed = true
	observedText := collector.outputText
	if collector.invalid {
		observedText = nil
	}
	collector.outputText = nil
	observedError := collector.outputError
	collector.outputError = nil
	var candidate ProtectedCaptureV1
	present := !collector.invalid && collector.candidate != nil
	if present {
		candidate = *collector.candidate
	}
	collector.candidate = nil
	collector.invalid = true
	return candidate, present, observedError, observedText
}

// Domain observations select actual committed records, not tool output JSON,
// pending arguments, or a later live lookup. They use the existing snapshot CAS.
type ProtectedGoalRecordV1 struct {
	ID              string `json:"id,omitempty"`
	Revision        *int64 `json:"revision,omitempty"`
	MaxGoalRounds   *int64 `json:"maxGoalRounds,omitempty"`
	RoundsStarted   *int64 `json:"roundsStarted,omitempty"`
	ThreadID        string `json:"threadId"`
	Objective       string `json:"objective"`
	Status          string `json:"status"`
	TokenBudget     *int64 `json:"tokenBudget,omitempty"`
	TokensUsed      int64  `json:"tokensUsed"`
	TimeUsedSeconds int64  `json:"timeUsedSeconds"`
	BlockedReason   string `json:"blockedReason,omitempty"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}
type ProtectedGoalCaptureV1 struct {
	Goal            *ProtectedGoalRecordV1 `json:"goal"`
	RemainingTokens *int64                 `json:"remainingTokens"`
}
type ProtectedTodoItemV1 struct {
	ID               string `json:"id"`
	Content          string `json:"content"`
	Status           string `json:"status"`
	StatusReasonCode string `json:"statusReasonCode,omitempty"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}
type ProtectedTodoListV1 struct {
	ThreadID  string                `json:"threadId"`
	TurnID    string                `json:"turnId,omitempty"`
	Items     []ProtectedTodoItemV1 `json:"items"`
	UpdatedAt string                `json:"updatedAt"`
}
type ProtectedTodoBaselineV1 struct {
	Availability string               `json:"availability"`
	Todos        *ProtectedTodoListV1 `json:"todos"`
}
type ProtectedTodoCaptureV1 struct {
	Todos    ProtectedTodoListV1     `json:"todos"`
	Baseline ProtectedTodoBaselineV1 `json:"baseline"`
}
type ProtectedSkillIdentityV1 struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RunAs         string `json:"runAs"`
	PackageDigest string `json:"packageDigest"`
}
type ProtectedSkillChildV1 struct {
	JobID          string  `json:"jobId"`
	ChildThreadID  *string `json:"childThreadId"`
	ChildTurnID    *string `json:"childTurnId"`
	Status         string  `json:"status"`
	Background     bool    `json:"background"`
	OutputWithheld bool    `json:"outputWithheld"`
}
type ProtectedSkillCaptureV1 struct {
	RequestedName string                    `json:"requestedName"`
	Resolved      *ProtectedSkillIdentityV1 `json:"resolved"`
	Child         *ProtectedSkillChildV1    `json:"child"`
	Error         *ProtectedWebErrorV1      `json:"error"`
}

func decodeClosedRequiredV1(body []byte, target any, required, nullable []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return errors.New("closed observation must be an object")
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return errors.New("closed observation is missing a required field")
		}
	}
	for key, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !oneOf(key, nullable...) {
			return errors.New("closed observation field cannot be null")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("closed observation has trailing data")
	}
	return nil
}
func (g *ProtectedGoalRecordV1) UnmarshalJSON(body []byte) error {
	type record ProtectedGoalRecordV1
	return decodeClosedRequiredV1(body, (*record)(g), []string{"threadId", "objective", "status", "tokensUsed", "timeUsedSeconds", "createdAt", "updatedAt"}, []string{"tokenBudget"})
}
func (g *ProtectedGoalCaptureV1) UnmarshalJSON(body []byte) error {
	type record ProtectedGoalCaptureV1
	return decodeClosedRequiredV1(body, (*record)(g), []string{"goal", "remainingTokens"}, []string{"goal", "remainingTokens"})
}
func (t *ProtectedTodoItemV1) UnmarshalJSON(body []byte) error {
	type record ProtectedTodoItemV1
	return decodeClosedRequiredV1(body, (*record)(t), []string{"id", "content", "status", "createdAt", "updatedAt"}, nil)
}
func (t *ProtectedTodoListV1) UnmarshalJSON(body []byte) error {
	type record ProtectedTodoListV1
	return decodeClosedRequiredV1(body, (*record)(t), []string{"threadId", "items", "updatedAt"}, nil)
}
func (b ProtectedTodoBaselineV1) MarshalJSON() ([]byte, error) {
	if b.Availability == "unavailable" {
		return json.Marshal(struct {
			Availability string `json:"availability"`
		}{b.Availability})
	}
	type record ProtectedTodoBaselineV1
	return json.Marshal(record(b))
}
func (b *ProtectedTodoBaselineV1) UnmarshalJSON(body []byte) error {
	var header struct {
		Availability string `json:"availability"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return err
	}
	if header.Availability == "unavailable" {
		if err := decodeClosedRequiredV1(body, &header, []string{"availability"}, nil); err != nil {
			return err
		}
		*b = ProtectedTodoBaselineV1{Availability: "unavailable"}
		return nil
	}
	type record ProtectedTodoBaselineV1
	return decodeClosedRequiredV1(body, (*record)(b), []string{"availability", "todos"}, []string{"todos"})
}
func (t *ProtectedTodoCaptureV1) UnmarshalJSON(body []byte) error {
	type record ProtectedTodoCaptureV1
	return decodeClosedRequiredV1(body, (*record)(t), []string{"todos", "baseline"}, nil)
}
func (s *ProtectedSkillIdentityV1) UnmarshalJSON(body []byte) error {
	type record ProtectedSkillIdentityV1
	return decodeClosedRequiredV1(body, (*record)(s), []string{"id", "name", "runAs", "packageDigest"}, nil)
}
func (s *ProtectedSkillChildV1) UnmarshalJSON(body []byte) error {
	type record ProtectedSkillChildV1
	return decodeClosedRequiredV1(body, (*record)(s), []string{"jobId", "childThreadId", "childTurnId", "status", "background", "outputWithheld"}, []string{"childThreadId", "childTurnId"})
}
func (s *ProtectedSkillCaptureV1) UnmarshalJSON(body []byte) error {
	type record ProtectedSkillCaptureV1
	return decodeClosedRequiredV1(body, (*record)(s), []string{"requestedName", "resolved", "child", "error"}, []string{"resolved", "child", "error"})
}

func validProtectedDomainTextV1(value string, bytesLimit, unitsLimit int, nonempty bool) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && len(value) <= bytesLimit &&
		(unitsLimit == 0 || len(utf16.Encode([]rune(value))) <= unitsLimit) && (!nonempty || strings.TrimSpace(value) != "")
}
func validateProtectedDomainCaptureV1(c ProtectedCaptureV1) error {
	if c.Label != "" || c.DurationMs != 0 || c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 || c.ExitCode != nil ||
		c.ReadTarget != nil || c.Web != nil || c.WebTargets != nil || c.Search != nil {
		return errors.New("domain capture contains unrelated observations")
	}
	if c.Kind == "goal" {
		if c.Status != "completed" || c.Body != "" || c.Truncated {
			return errors.New("goal capture lifecycle is invalid")
		}
		g := c.Goal.Goal
		if g == nil {
			if c.Goal.RemainingTokens != nil {
				return errors.New("empty goal has no remaining budget")
			}
			return nil
		}
		if !domainthread.IsCanonicalRecordID(g.ThreadID) || !validProtectedDomainTextV1(g.Objective, 16000, 4000, true) ||
			!oneOf(g.Status, "active", "paused", "blocked", "usageLimited", "budgetLimited", "complete") || g.TokensUsed < 0 || g.TimeUsedSeconds < 0 ||
			!validProtectedDomainTextV1(g.BlockedReason, 4000, 1000, false) || !validProtectedDomainTextV1(g.ID, 4096, 0, false) ||
			!validProtectedDomainTextV1(g.CreatedAt, 64, 0, true) || !validProtectedDomainTextV1(g.UpdatedAt, 64, 0, true) {
			return errors.New("goal record is invalid")
		}
		if g.TokenBudget == nil {
			if c.Goal.RemainingTokens != nil {
				return errors.New("unbounded goal has no remaining token amount")
			}
		} else {
			remaining := *g.TokenBudget - g.TokensUsed
			if remaining < 0 {
				remaining = 0
			}
			if *g.TokenBudget <= 0 || c.Goal.RemainingTokens == nil || *c.Goal.RemainingTokens != remaining {
				return errors.New("goal budget observation is invalid")
			}
		}
		if (g.Revision != nil && (*g.Revision < 1 || *g.Revision > 9007199254740991)) || (g.MaxGoalRounds != nil && (*g.MaxGoalRounds < 1 || *g.MaxGoalRounds > 9007199254740991)) || (g.RoundsStarted != nil && (*g.RoundsStarted < 0 || *g.RoundsStarted > 9007199254740991)) || (g.MaxGoalRounds != nil && g.RoundsStarted != nil && *g.RoundsStarted > *g.MaxGoalRounds) {
			return errors.New("Goal recorded round facts are invalid")
		}
		return nil
	}
	if c.Kind == "todo" {
		if c.Status != "completed" || c.Body != "" || c.Truncated || validateProtectedTodoListV1(c.Todo.Todos) != nil {
			return errors.New("todo capture is invalid")
		}
		b := c.Todo.Baseline
		if b.Availability == "unavailable" {
			if b.Todos != nil {
				return errors.New("unavailable todo baseline contains data")
			}
			return nil
		}
		if b.Availability != "known" || b.Todos != nil && (validateProtectedTodoListV1(*b.Todos) != nil || b.Todos.ThreadID != c.Todo.Todos.ThreadID) {
			return errors.New("todo baseline is invalid")
		}
		return nil
	}
	s := c.Skill
	if !oneOf(c.Status, "completed", "failed") || !validProtectedDomainTextV1(s.RequestedName, 1024, 0, false) ||
		c.Status == "completed" && (s.Resolved == nil || s.Error != nil) {
		return errors.New("skill observation lifecycle is invalid")
	}
	if s.Resolved != nil && (!validProtectedDomainTextV1(s.Resolved.ID, 1024, 0, true) || !validProtectedDomainTextV1(s.Resolved.Name, 1024, 0, true) ||
		!oneOf(s.Resolved.RunAs, "inline", "subagent") || !domainsecurity.IsSHA256Hex(s.Resolved.PackageDigest)) {
		return errors.New("skill identity is invalid")
	}
	if (s.Resolved == nil || s.Resolved.RunAs != "inline") && (c.Body != "" || c.Truncated) {
		return errors.New("subagent skill body is withheld")
	}
	if s.Child != nil && (s.Resolved == nil || s.Resolved.RunAs != "subagent" || !taskJobPublicIDPatternV1.MatchString(s.Child.JobID) ||
		!s.Child.OutputWithheld || !validTaskJobWireStatusV1(s.Child.Status) ||
		s.Child.ChildThreadID != nil && !domainthread.IsCanonicalRecordID(*s.Child.ChildThreadID) ||
		s.Child.ChildTurnID != nil && !domainthread.IsCanonicalRecordID(*s.Child.ChildTurnID)) {
		return errors.New("skill child observation is invalid")
	}
	if s.Error != nil && (!validProtectedDomainTextV1(s.Error.Code, 256, 0, true) || !validProtectedDomainTextV1(s.Error.Message, ProtectedSnapshotBodyLimitV1, 0, false)) {
		return errors.New("skill error observation is invalid")
	}
	return nil
}
func validateProtectedTodoListV1(t ProtectedTodoListV1) error {
	if !domainthread.IsCanonicalRecordID(t.ThreadID) || t.TurnID != "" && !domainthread.IsCanonicalRecordID(t.TurnID) ||
		t.Items == nil || len(t.Items) > 200 || !validProtectedDomainTextV1(t.UpdatedAt, 64, 0, true) {
		return errors.New("todo list is invalid")
	}
	ids := map[string]bool{}
	for _, item := range t.Items {
		if !validProtectedDomainTextV1(item.ID, 4096, 0, true) || ids[item.ID] || !validProtectedDomainTextV1(item.Content, 4000, 1000, true) ||
			!oneOf(item.Status, "pending", "in_progress", "completed", "failed", "canceled") || !validProtectedDomainTextV1(item.CreatedAt, 64, 0, true) || !validProtectedDomainTextV1(item.UpdatedAt, 64, 0, true) {
			return errors.New("todo item is invalid")
		}
		ids[item.ID] = true
		if item.Status == "failed" {
			if !oneOf(item.StatusReasonCode, "execution_failed", "dependency_failed", "verification_failed", "timeout", "tool_failed", "subagent_failed", "runtime_failed") {
				return errors.New("failed todo reason is invalid")
			}
		} else if item.Status == "canceled" {
			if !oneOf(item.StatusReasonCode, "user_canceled", "parent_canceled", "runtime_canceled", "superseded") {
				return errors.New("canceled todo reason is invalid")
			}
		} else if item.StatusReasonCode != "" {
			return errors.New("todo reason does not match status")
		}
	}
	return nil
}

func selectedProtectedRecordV1(record map[string]any, keys []string) map[string]any {
	selected := map[string]any{}
	for _, key := range keys {
		if value, ok := record[key]; ok {
			selected[key] = value
		}
	}
	return selected
}
func NewProtectedGoalCaptureV1(goal map[string]any) (ProtectedCaptureV1, bool) {
	data := &ProtectedGoalCaptureV1{}
	if goal != nil {
		body, err := json.Marshal(selectedProtectedRecordV1(goal, []string{"id", "revision", "maxGoalRounds", "roundsStarted", "threadId", "objective", "status", "tokenBudget", "tokensUsed", "timeUsedSeconds", "blockedReason", "createdAt", "updatedAt"}))
		if err != nil {
			return ProtectedCaptureV1{}, false
		}
		var record ProtectedGoalRecordV1
		if json.Unmarshal(body, &record) != nil {
			return ProtectedCaptureV1{}, false
		}
		data.Goal = &record
		if record.TokenBudget != nil {
			remaining := *record.TokenBudget - record.TokensUsed
			if remaining < 0 {
				remaining = 0
			}
			data.RemainingTokens = &remaining
		}
	}
	c := ProtectedCaptureV1{Kind: "goal", Status: "completed", Goal: data}
	return c, ValidateProtectedCaptureV1(c) == nil
}
func protectedTodoListFromRecordV1(record map[string]any) (*ProtectedTodoListV1, bool) {
	if record == nil {
		return nil, true
	}
	selected := selectedProtectedRecordV1(record, []string{"threadId", "turnId", "updatedAt"})
	items, ok := record["items"].([]any)
	if !ok {
		return nil, false
	}
	rows := make([]any, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		rows = append(rows, selectedProtectedRecordV1(item, []string{"id", "content", "status", "statusReasonCode", "createdAt", "updatedAt"}))
	}
	selected["items"] = rows
	body, err := json.Marshal(selected)
	if err != nil {
		return nil, false
	}
	var list ProtectedTodoListV1
	if json.Unmarshal(body, &list) != nil || validateProtectedTodoListV1(list) != nil {
		return nil, false
	}
	return &list, true
}
func NewProtectedTodoCaptureV1(after, before map[string]any, baselineKnown bool) (ProtectedCaptureV1, bool) {
	next, ok := protectedTodoListFromRecordV1(after)
	if !ok || next == nil {
		return ProtectedCaptureV1{}, false
	}
	baseline := ProtectedTodoBaselineV1{Availability: "unavailable"}
	if baselineKnown {
		previous, ok := protectedTodoListFromRecordV1(before)
		if !ok {
			return ProtectedCaptureV1{}, false
		}
		baseline = ProtectedTodoBaselineV1{Availability: "known", Todos: previous}
	}
	c := ProtectedCaptureV1{Kind: "todo", Status: "completed", Todo: &ProtectedTodoCaptureV1{Todos: *next, Baseline: baseline}}
	return c, ValidateProtectedCaptureV1(c) == nil
}
func cloneProtectedTodoListV1(list *ProtectedTodoListV1) *ProtectedTodoListV1 {
	if list == nil {
		return nil
	}
	copy_ := *list
	copy_.Items = append(make([]ProtectedTodoItemV1, 0, len(list.Items)), list.Items...)
	return &copy_
}
func cloneProtectedDomainCaptureV1(c ProtectedCaptureV1) (*ProtectedGoalCaptureV1, *ProtectedTodoCaptureV1, *ProtectedSkillCaptureV1) {
	var goal *ProtectedGoalCaptureV1
	var todo *ProtectedTodoCaptureV1
	var skill *ProtectedSkillCaptureV1
	if c.Goal != nil {
		g := *c.Goal
		if g.RemainingTokens != nil {
			n := *g.RemainingTokens
			g.RemainingTokens = &n
		}
		if g.Goal != nil {
			r := *g.Goal
			for _, field := range []**int64{&r.Revision, &r.MaxGoalRounds, &r.RoundsStarted} {
				if *field != nil {
					value := **field
					*field = &value
				}
			}
			if r.TokenBudget != nil {
				n := *r.TokenBudget
				r.TokenBudget = &n
			}
			g.Goal = &r
		}
		goal = &g
	}
	if c.Todo != nil {
		t := *c.Todo
		t.Todos = *cloneProtectedTodoListV1(&t.Todos)
		t.Baseline.Todos = cloneProtectedTodoListV1(t.Baseline.Todos)
		todo = &t
	}
	if c.Skill != nil {
		s := *c.Skill
		if s.Resolved != nil {
			r := *s.Resolved
			s.Resolved = &r
		}
		if s.Error != nil {
			e := *s.Error
			s.Error = &e
		}
		if s.Child != nil {
			ch := *s.Child
			if ch.ChildThreadID != nil {
				id := *ch.ChildThreadID
				ch.ChildThreadID = &id
			}
			if ch.ChildTurnID != nil {
				id := *ch.ChildTurnID
				ch.ChildTurnID = &id
			}
			s.Child = &ch
		}
		skill = &s
	}
	return goal, todo, skill
}
