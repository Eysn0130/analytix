package terminal

import "testing"

func TestBashToolRequestFromArgsValidatesSandboxWorkspaceAndTimeout(t *testing.T) {
	workspace := t.TempDir()
	request, failure, failed := BashToolRequestFromArgs(
		map[string]any{"command": "printf ok", "timeout": float64(500)},
		workspace,
		true,
		"danger-full-access",
		DefaultBashTimeoutSeconds,
		MaxBashTimeoutSeconds,
	)
	if failed || failure != nil {
		t.Fatalf("valid request should parse: request=%#v failure=%#v failed=%v", request, failure, failed)
	}
	if request.Command != "printf ok" || request.Workspace != workspace || request.TimeoutSeconds != MaxBashTimeoutSeconds {
		t.Fatalf("request should trim command and clamp timeout: %#v", request)
	}
	for name, testCase := range map[string]struct {
		value any
		want  int
	}{
		"omitted":  {want: DefaultBashTimeoutSeconds},
		"explicit": {value: float64(45), want: 45},
		"zero":     {value: float64(0), want: DefaultBashTimeoutSeconds},
		"over max": {value: float64(MaxBashTimeoutSeconds + 1), want: MaxBashTimeoutSeconds},
	} {
		t.Run(name, func(t *testing.T) {
			args := map[string]any{"command": "printf ok"}
			if testCase.value != nil {
				args["timeout"] = testCase.value
			}
			parsed, parseFailure, parseFailed := BashToolRequestFromArgs(
				args, workspace, true, "danger-full-access",
				DefaultBashTimeoutSeconds, MaxBashTimeoutSeconds,
			)
			if parseFailed || parseFailure != nil || parsed.TimeoutSeconds != testCase.want {
				t.Fatalf("timeout parse mismatch: request=%#v failure=%#v failed=%v", parsed, parseFailure, parseFailed)
			}
		})
	}

	for _, sandboxMode := range []string{"", "read-only", "workspace-write", "external-sandbox"} {
		if _, failure, failed := BashToolRequestFromArgs(map[string]any{"command": "printf ok"}, "/tmp/work", true, sandboxMode, DefaultBashTimeoutSeconds, MaxBashTimeoutSeconds); !failed || stringValue(failure["code"]) != "sandbox_blocked" {
			t.Fatalf("sandbox %q should block bash: %#v failed=%v", sandboxMode, failure, failed)
		}
	}
	if _, failure, failed := BashToolRequestFromArgs(map[string]any{}, "/tmp/work", true, "danger-full-access", DefaultBashTimeoutSeconds, MaxBashTimeoutSeconds); !failed || stringValue(failure["code"]) != "validation_error" {
		t.Fatalf("missing command should fail validation: %#v failed=%v", failure, failed)
	}
	if _, failure, failed := BashToolRequestFromArgs(map[string]any{"command": "printf ok"}, "relative", false, "danger-full-access", DefaultBashTimeoutSeconds, MaxBashTimeoutSeconds); !failed || stringValue(failure["code"]) != "invalid_workspace" {
		t.Fatalf("relative workspace should fail validation: %#v failed=%v", failure, failed)
	}
}
