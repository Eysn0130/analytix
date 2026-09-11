package executionpolicy

import "strings"

const (
	CurrentVersion        = 2
	DefaultApprovalPolicy = "on-request"
	DefaultSandboxMode    = "workspace-write"
	LegacyApprovalPolicy  = "auto"
	LegacySandboxMode     = "danger-full-access"
)

func Normalize(approvalPolicy string, sandboxMode string) (string, string) {
	approvalPolicy = normalizeApproval(approvalPolicy)
	if approvalPolicy == "" {
		approvalPolicy = DefaultApprovalPolicy
	}
	sandboxMode = normalizeSandbox(sandboxMode)
	if sandboxMode == "" {
		sandboxMode = DefaultSandboxMode
	}
	return approvalPolicy, sandboxMode
}

func normalizeApproval(value string) string {
	switch strings.TrimSpace(value) {
	case "always", "auto", "on-request", "untrusted", "suggest", "never":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeSandbox(value string) string {
	switch strings.TrimSpace(value) {
	case "read-only", "workspace-write", "danger-full-access", "external-sandbox":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
