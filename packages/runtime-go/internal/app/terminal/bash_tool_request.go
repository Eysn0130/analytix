package terminal

import (
	"strings"
)

const (
	DefaultBashTimeoutSeconds = 120
	MaxBashTimeoutSeconds     = 120
)

type BashToolRequest struct {
	Command        string
	Workspace      string
	TimeoutSeconds int
}

func BashToolRequestFromArgs(args map[string]any, workspace string, workspaceAbsolute bool, sandboxMode string, defaultTimeout int, maxTimeout int) (BashToolRequest, map[string]any, bool) {
	if strings.TrimSpace(sandboxMode) != "danger-full-access" {
		return BashToolRequest{}, map[string]any{"code": "sandbox_blocked", "error": "bash requires danger-full-access"}, true
	}
	command := strings.TrimSpace(firstNonEmptyToolArg(args["command"]))
	if command == "" {
		return BashToolRequest{}, map[string]any{"code": "validation_error", "error": "command is required"}, true
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || !workspaceAbsolute {
		return BashToolRequest{}, map[string]any{"code": "invalid_workspace", "error": "workspace must be an absolute path"}, true
	}
	return BashToolRequest{
		Command:        command,
		Workspace:      workspace,
		TimeoutSeconds: positiveToolLimit(args["timeout"], defaultTimeout, maxTimeout),
	}, nil, false
}

func firstNonEmptyToolArg(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case []byte:
			if strings.TrimSpace(string(typed)) != "" {
				return string(typed)
			}
		}
	}
	return ""
}

func positiveToolLimit(value any, defaultValue int, maxValue int) int {
	out := defaultValue
	switch typed := value.(type) {
	case int:
		out = typed
	case int64:
		out = int(typed)
	case float64:
		out = int(typed)
	case jsonNumberLike:
		if parsed, err := typed.Float64(); err == nil {
			out = int(parsed)
		}
	}
	if out <= 0 {
		out = defaultValue
	}
	if maxValue > 0 && out > maxValue {
		out = maxValue
	}
	return out
}

type jsonNumberLike interface {
	Float64() (float64, error)
}
