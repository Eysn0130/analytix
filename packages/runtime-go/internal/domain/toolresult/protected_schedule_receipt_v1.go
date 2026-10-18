package toolresult

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

// These are exact native Host records, not scheduler inputs or live lookups.
// Empty timestamps and absent provider identity retain their original meaning.
type ProtectedScheduleRuleV1 struct {
	Kind         string  `json:"kind"`
	EveryMinutes float64 `json:"everyMinutes"`
	TimeOfDay    string  `json:"timeOfDay"`
	AtTime       string  `json:"atTime"`
	TimeZone     *string `json:"timeZone,omitempty"`
	Weekdays     []int   `json:"weekdays,omitempty"`
	Expression   *string `json:"expression,omitempty"`
}

type ProtectedScheduleTaskV1 struct {
	ID              string                  `json:"id"`
	Title           string                  `json:"title"`
	Enabled         bool                    `json:"enabled"`
	Prompt          string                  `json:"prompt"`
	WorkspaceRoot   string                  `json:"workspaceRoot"`
	ClawChannelID   string                  `json:"clawChannelId"`
	ProviderID      *string                 `json:"providerId,omitempty"`
	Model           string                  `json:"model"`
	ReasoningEffort string                  `json:"reasoningEffort"`
	Mode            string                  `json:"mode"`
	Schedule        ProtectedScheduleRuleV1 `json:"schedule"`
	CreatedAt       string                  `json:"createdAt"`
	UpdatedAt       string                  `json:"updatedAt"`
	LastRunAt       string                  `json:"lastRunAt"`
	NextRunAt       string                  `json:"nextRunAt"`
	LastStatus      string                  `json:"lastStatus"`
	LastMessage     string                  `json:"lastMessage"`
	LastThreadID    string                  `json:"lastThreadId"`
}

type ProtectedScheduleReceiptV1 struct {
	Operation string                   `json:"operation"`
	Status    string                   `json:"status"`
	Message   string                   `json:"message"`
	Task      *ProtectedScheduleTaskV1 `json:"-"`
	TaskID    *string                  `json:"-"`
}

func ProtectedScheduleOperationForToolV1(name string) string {
	switch name {
	case "mcp__gui_schedule__gui_schedule_create", "mcp__gui_schedule__claw_schedule_create":
		return "create"
	case "mcp__gui_schedule__gui_schedule_update", "mcp__gui_schedule__claw_schedule_update":
		return "update"
	case "mcp__gui_schedule__gui_schedule_delete", "mcp__gui_schedule__claw_schedule_delete":
		return "delete"
	default:
		return ""
	}
}

func scheduleObjectV1(body []byte) (map[string]any, error) {
	return domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{
		MaxBytes: ProtectedSnapshotEnvelopeLimitV1, MaxDepth: 16, MaxTokens: 1000, MaxStringBytes: ProtectedSnapshotEnvelopeLimitV1,
	})
}

func scheduleFieldsV1(value map[string]any, required []string, optional ...string) bool {
	allowed := map[string]bool{}
	for _, key := range required {
		if _, present := value[key]; !present {
			return false
		}
		allowed[key] = true
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range value {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func scheduleStringV1(value string) bool {
	return utf8.ValidString(value) && len(value) <= ProtectedSnapshotEnvelopeLimitV1
}

func (task *ProtectedScheduleTaskV1) UnmarshalJSON(body []byte) error {
	value, err := scheduleObjectV1(body)
	strings_ := []string{"id", "title", "prompt", "workspaceRoot", "clawChannelId", "model", "reasoningEffort", "mode", "createdAt", "updatedAt", "lastRunAt", "nextRunAt", "lastStatus", "lastMessage", "lastThreadId"}
	if err != nil || !scheduleFieldsV1(value, append(append([]string{}, strings_...), "enabled", "schedule"), "providerId") {
		return errors.New("protected schedule task fields are invalid")
	}
	for _, key := range strings_ {
		if _, ok := value[key].(string); !ok {
			return errors.New("protected schedule task text is invalid")
		}
	}
	if _, ok := value["enabled"].(bool); !ok {
		return errors.New("protected schedule enabled state is invalid")
	}
	if provider, present := value["providerId"]; present {
		if _, ok := provider.(string); !ok {
			return errors.New("protected schedule provider is invalid")
		}
	}
	rule, ok := value["schedule"].(map[string]any)
	if !ok || !scheduleFieldsV1(rule, []string{"kind", "everyMinutes", "timeOfDay", "atTime"}, "timeZone", "weekdays", "expression") {
		return errors.New("protected schedule rule fields are invalid")
	}
	for _, key := range []string{"kind", "timeOfDay", "atTime"} {
		if _, ok := rule[key].(string); !ok {
			return errors.New("protected schedule rule text is invalid")
		}
	}
	if _, ok := rule["everyMinutes"].(json.Number); !ok {
		return errors.New("protected schedule frequency is invalid")
	}
	for _, key := range []string{"timeZone", "expression"} {
		if value, present := rule[key]; present {
			if _, ok := value.(string); !ok {
				return errors.New("protected schedule calendar text is invalid")
			}
		}
	}
	if value, present := rule["weekdays"]; present {
		days, ok := value.([]any)
		if !ok || len(days) == 0 || len(days) > 7 {
			return errors.New("protected schedule weekdays are invalid")
		}
		for _, day := range days {
			number, ok := day.(json.Number)
			if !ok {
				return errors.New("protected schedule weekday is invalid")
			}
			if value, err := number.Int64(); err != nil || value < 1 || value > 7 {
				return errors.New("protected schedule weekday is invalid")
			}
		}
	}
	type plainTask ProtectedScheduleTaskV1
	var decoded plainTask
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	*task = ProtectedScheduleTaskV1(decoded)
	return validateProtectedScheduleTaskV1(task)
}

func validateProtectedScheduleTaskV1(task *ProtectedScheduleTaskV1) error {
	if task == nil || task.ID == "" || trimScheduleIDV1(task.ID) != task.ID ||
		!oneOf(task.ReasoningEffort, "auto", "off", "low", "medium", "high", "max") || !oneOf(task.Mode, "agent", "plan") ||
		validateProtectedScheduleRuleV1(task.Schedule) != nil ||
		!oneOf(task.LastStatus, "idle", "running", "success", "error") {
		return errors.New("protected schedule task is invalid")
	}
	for _, value := range []string{task.ID, task.Title, task.Prompt, task.WorkspaceRoot, task.ClawChannelID, task.Model, task.ReasoningEffort, task.Mode,
		task.Schedule.Kind, task.Schedule.TimeOfDay, task.Schedule.AtTime, task.CreatedAt, task.UpdatedAt, task.LastRunAt, task.NextRunAt, task.LastStatus, task.LastMessage, task.LastThreadID} {
		if !scheduleStringV1(value) {
			return errors.New("protected schedule task text exceeds limit")
		}
	}
	if task.ProviderID != nil && !scheduleStringV1(*task.ProviderID) {
		return errors.New("protected schedule provider text exceeds limit")
	}
	return nil
}

var protectedScheduleTimeZoneShapeV1 = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+.-]*(?:/[A-Za-z0-9_+.-]+)+$`)
var protectedScheduleTimeOfDayV1 = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9](?::[0-5][0-9](?:\.[0-9]{1,3})?)?$`)
var protectedScheduleCronFieldShapeV1 = regexp.MustCompile(`^(?:\*(?:/[0-9]+)?|[0-9]+(?:-[0-9]+(?:/[0-9]+)?)?)(?:,(?:\*(?:/[0-9]+)?|[0-9]+(?:-[0-9]+(?:/[0-9]+)?)?))*$`)

// Retain a committed Host rule without normalizing or evaluating it. The Host
// shared validator owns the IANA registry and cron range/canonicalization checks;
// this boundary closes the variant, field types, presence and bounded wire shape.
func validateProtectedScheduleRuleV1(rule ProtectedScheduleRuleV1) error {
	if !oneOf(rule.Kind, "manual", "at", "daily", "interval", "weekly", "cron") || math.IsNaN(rule.EveryMinutes) || math.IsInf(rule.EveryMinutes, 0) {
		return errors.New("protected schedule rule is invalid")
	}
	if !oneOf(rule.Kind, "daily", "weekly", "cron") {
		if rule.TimeZone != nil || rule.Weekdays != nil || rule.Expression != nil {
			return errors.New("protected schedule calendar fields are invalid")
		}
		return nil
	}
	if rule.TimeZone != nil {
		zone := *rule.TimeZone
		if len(zone) > 255 || zone != "UTC" && !protectedScheduleTimeZoneShapeV1.MatchString(zone) {
			return errors.New("protected schedule time zone is invalid")
		}
	} else if rule.Kind != "daily" {
		return errors.New("protected schedule time zone is missing")
	}
	if rule.Kind != "cron" && rule.TimeZone != nil && !protectedScheduleTimeOfDayV1.MatchString(rule.TimeOfDay) {
		return errors.New("protected schedule wall clock is invalid")
	}
	if rule.Kind == "weekly" {
		if len(rule.Weekdays) == 0 || len(rule.Weekdays) > 7 {
			return errors.New("protected schedule weekdays are invalid")
		}
		seen := [8]bool{}
		for _, day := range rule.Weekdays {
			if day < 1 || day > 7 || seen[day] {
				return errors.New("protected schedule weekdays are invalid")
			}
			seen[day] = true
		}
	} else if rule.Weekdays != nil {
		return errors.New("protected schedule weekdays are misplaced")
	}
	if rule.Kind == "cron" {
		if rule.Expression == nil || len(*rule.Expression) > 512 || trimScheduleIDV1(*rule.Expression) != *rule.Expression {
			return errors.New("protected schedule expression is invalid")
		}
		fields := strings.Fields(*rule.Expression)
		if len(fields) != 5 {
			return errors.New("protected schedule expression is invalid")
		}
		for _, field := range fields {
			if !protectedScheduleCronFieldShapeV1.MatchString(field) {
				return errors.New("protected schedule expression is invalid")
			}
		}
	} else if rule.Expression != nil {
		return errors.New("protected schedule expression is misplaced")
	}
	return nil
}

// Match the native TypeScript ID trim check without normalizing retained bytes.
func trimScheduleIDV1(value string) string {
	return strings.TrimFunc(value, func(r rune) bool {
		return r >= '\u0009' && r <= '\u000d' || r == '\u0020' || r == '\u00a0' || r == '\u1680' || r >= '\u2000' && r <= '\u200a' ||
			r == '\u2028' || r == '\u2029' || r == '\u202f' || r == '\u205f' || r == '\u3000' || r == '\ufeff'
	})
}

func (receipt ProtectedScheduleReceiptV1) MarshalJSON() ([]byte, error) {
	if receipt.Operation == "delete" {
		return json.Marshal(struct {
			Operation string  `json:"operation"`
			Status    string  `json:"status"`
			Message   string  `json:"message"`
			TaskID    *string `json:"taskId"`
		}{receipt.Operation, receipt.Status, receipt.Message, receipt.TaskID})
	}
	return json.Marshal(struct {
		Operation string                   `json:"operation"`
		Status    string                   `json:"status"`
		Message   string                   `json:"message"`
		Task      *ProtectedScheduleTaskV1 `json:"task"`
	}{receipt.Operation, receipt.Status, receipt.Message, receipt.Task})
}

func (receipt *ProtectedScheduleReceiptV1) UnmarshalJSON(body []byte) error {
	value, err := scheduleObjectV1(body)
	if err != nil {
		return err
	}
	operation, ok := value["operation"].(string)
	if !ok || !oneOf(operation, "create", "update", "delete") {
		return errors.New("protected schedule operation is invalid")
	}
	field := "task"
	if operation == "delete" {
		field = "taskId"
	}
	if !scheduleFieldsV1(value, []string{"operation", "status", "message", field}) {
		return errors.New("protected schedule receipt fields are invalid")
	}
	status, statusOK := value["status"].(string)
	message, messageOK := value["message"].(string)
	if !statusOK || !messageOK {
		return errors.New("protected schedule outcome is invalid")
	}
	decoded := ProtectedScheduleReceiptV1{Operation: operation, Status: status, Message: message}
	if operation == "delete" {
		taskID, ok := value["taskId"].(string)
		if !ok {
			return errors.New("protected schedule delete identity is invalid")
		}
		decoded.TaskID = &taskID
	} else if value["task"] != nil {
		taskBytes, err := json.Marshal(value["task"])
		if err != nil {
			return err
		}
		decoded.Task = &ProtectedScheduleTaskV1{}
		if err := json.Unmarshal(taskBytes, decoded.Task); err != nil {
			return err
		}
	}
	if err := ValidateProtectedScheduleReceiptV1(&decoded); err != nil {
		return err
	}
	*receipt = decoded
	return nil
}

func ValidateProtectedScheduleReceiptV1(receipt *ProtectedScheduleReceiptV1) error {
	if receipt == nil || !oneOf(receipt.Operation, "create", "update", "delete") || !oneOf(receipt.Status, "success", "failure") || !scheduleStringV1(receipt.Message) {
		return errors.New("protected schedule receipt is invalid")
	}
	if receipt.Operation == "delete" {
		if receipt.Task != nil || receipt.TaskID == nil || !scheduleStringV1(*receipt.TaskID) {
			return errors.New("protected schedule delete receipt is invalid")
		}
	} else if receipt.TaskID != nil || (receipt.Status == "success") != (receipt.Task != nil) || receipt.Task != nil && validateProtectedScheduleTaskV1(receipt.Task) != nil {
		return errors.New("protected schedule task receipt is invalid")
	}
	body, err := json.Marshal(receipt)
	if err != nil || len(body) > ProtectedSnapshotEnvelopeLimitV1 {
		return errors.New("protected schedule receipt exceeds limit")
	}
	return nil
}

// Parsing validates a native structured result; it does not establish Host
// authority. Only the bound manager can mint the process-private carrier.
func ParseProtectedScheduleToolOutputV1(operation string, body []byte, isError bool) (*ProtectedScheduleReceiptV1, error) {
	value, err := scheduleObjectV1(body)
	field := "task"
	if operation == "delete" {
		field = "taskId"
	}
	if err != nil || !oneOf(operation, "create", "update", "delete") || !scheduleFieldsV1(value, []string{"status", "message", field}) {
		return nil, errors.New("native schedule output fields are invalid")
	}
	value["operation"] = operation
	body, err = json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var receipt ProtectedScheduleReceiptV1
	if err := json.Unmarshal(body, &receipt); err != nil {
		return nil, err
	}
	if (receipt.Status == "failure") != isError {
		return nil, errors.New("native schedule outcome is inconsistent")
	}
	return &receipt, nil
}

func validateProtectedScheduleCaptureV1(c ProtectedCaptureV1) error {
	if !oneOf(c.Status, "completed", "failed") || c.Body != "" || c.Label != "" || c.Truncated || c.DurationMs != 0 ||
		c.ExitCode != nil || c.ReadTarget != nil || c.Web != nil || c.WebTargets != nil || c.Search != nil || c.Goal != nil || c.Todo != nil || c.Skill != nil || c.Job != nil ||
		c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 || ValidateProtectedScheduleReceiptV1(c.Schedule) != nil ||
		(c.Status == "completed") != (c.Schedule.Status == "success") {
		return errors.New("protected schedule capture is invalid")
	}
	return nil
}

func cloneProtectedScheduleReceiptV1(value *ProtectedScheduleReceiptV1) *ProtectedScheduleReceiptV1 {
	if value == nil {
		return nil
	}
	copy_ := *value
	if value.TaskID != nil {
		id := *value.TaskID
		copy_.TaskID = &id
	}
	if value.Task != nil {
		task := *value.Task
		if value.Task.ProviderID != nil {
			provider := *value.Task.ProviderID
			task.ProviderID = &provider
		}
		if value.Task.Schedule.TimeZone != nil {
			zone := *value.Task.Schedule.TimeZone
			task.Schedule.TimeZone = &zone
		}
		if value.Task.Schedule.Expression != nil {
			expression := *value.Task.Schedule.Expression
			task.Schedule.Expression = &expression
		}
		if value.Task.Schedule.Weekdays != nil {
			task.Schedule.Weekdays = append([]int{}, value.Task.Schedule.Weekdays...)
		}
		copy_.Task = &task
	}
	return &copy_
}
