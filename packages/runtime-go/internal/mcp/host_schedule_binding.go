package mcp

import (
	"errors"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

const (
	hostScheduleMCPServerIDV1           = "gui_schedule"
	hostScheduleMCPServerNameV1         = "analytix-schedule"
	hostScheduleMCPServerVersionV1      = "0.1.0"
	hostScheduleMCPIdentitySourceV1     = "host-schedule-binding-v1"
	hostScheduleMCPListToolNameV1       = "gui_schedule_list"
	hostScheduleMCPLegacyListToolNameV1 = "claw_schedule_list"
	hostScheduleMCPTimeoutMSV1          = 5000
)

// ValidateHostScheduleMCPBindingV1 validates the exact stdio projection that
// Electron may pass through the private startup frame. It deliberately does
// not grant anything; BindHostScheduleMCPServerV1 owns that one-way step after
// the projection matches the immutable runtime configuration snapshot.
func ValidateHostScheduleMCPBindingV1(spec ServerSpec) (uint16, error) {
	invalid := func() (uint16, error) {
		return 0, errors.New("host schedule MCP binding is invalid")
	}
	if spec.ID != hostScheduleMCPServerIDV1 || spec.Transport != "stdio" ||
		spec.Command == "" || spec.Command != trimECMAScriptWhitespaceV1(spec.Command) ||
		!filepath.IsAbs(spec.Command) || filepath.Clean(spec.Command) != spec.Command ||
		strings.HasSuffix(spec.Command, string(filepath.Separator)) ||
		(len(spec.Args) != 4 && len(spec.Args) != 6) ||
		len(spec.Args) < 4 || spec.Args[0] == "" ||
		spec.Args[0] != trimECMAScriptWhitespaceV1(spec.Args[0]) ||
		!filepath.IsAbs(spec.Args[0]) || filepath.Clean(spec.Args[0]) != spec.Args[0] ||
		strings.HasSuffix(spec.Args[0], string(filepath.Separator)) ||
		spec.Args[1] != "--gui-schedule-mcp-server" ||
		spec.Args[2] != "--base-url" ||
		len(spec.Env) != 1 || spec.Env["ELECTRON_RUN_AS_NODE"] != "1" ||
		spec.TrustScope != "user" || spec.TimeoutMS != hostScheduleMCPTimeoutMSV1 ||
		spec.URL != "" || len(spec.Headers) != 0 || spec.CWD != "" ||
		spec.ExpectedServerName != "" || spec.ExpectedServerVersion != "" ||
		spec.IdentitySource != "" || spec.ManifestSHA256 != "" ||
		spec.EntrypointPath != "" || spec.EntrypointSHA256 != "" ||
		spec.PluginRootPath != "" || spec.SourceTreeSHA256 != "" ||
		spec.HostInstallMarkerSHA256 != "" || len(spec.TrustedWorkspaceRoots) != 0 ||
		spec.LowPriority || spec.BackgroundStart || len(spec.ReadOnlyToolNames) != 0 ||
		len(spec.Tools) != 0 {
		return invalid()
	}
	if len(spec.Args) == 6 && (spec.Args[4] != "--secret" ||
		spec.Args[5] == "" || spec.Args[5] != trimECMAScriptWhitespaceV1(spec.Args[5])) {
		return invalid()
	}
	if strings.ContainsRune(spec.Command, '\x00') {
		return invalid()
	}
	for _, argument := range spec.Args {
		if strings.ContainsRune(argument, '\x00') {
			return invalid()
		}
	}
	parsed, err := url.Parse(spec.Args[3])
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return invalid()
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 || port > 65535 ||
		spec.Args[3] != "http://127.0.0.1:"+strconv.Itoa(port) {
		return invalid()
	}
	return uint16(port), nil
}

func trimECMAScriptWhitespaceV1(value string) string {
	return strings.TrimFunc(value, func(r rune) bool {
		switch {
		case r >= '\u0009' && r <= '\u000d':
			return true
		case r == '\u0020' || r == '\u00a0' || r == '\u1680' ||
			(r >= '\u2000' && r <= '\u200a') || r == '\u2028' ||
			r == '\u2029' || r == '\u202f' || r == '\u205f' ||
			r == '\u3000' || r == '\ufeff':
			return true
		default:
			return false
		}
	})
}

// BindHostScheduleMCPServerV1 adds host-only identity, effect and read-only
// policy only when one private startup projection is structurally identical
// to one server from the immutable runtime configuration snapshot.
func BindHostScheduleMCPServerV1(specs []ServerSpec, binding *ServerSpec) []ServerSpec {
	out := make([]ServerSpec, len(specs))
	configured := make([]ServerSpec, len(specs))
	for index := range specs {
		configured[index] = cloneServerSpecForAuthority(specs[index])
		out[index] = stripHostScheduleMCPPolicyV1(configured[index])
	}
	if binding == nil {
		return out
	}
	bound := cloneServerSpecForAuthority(*binding)
	if _, err := ValidateHostScheduleMCPBindingV1(bound); err != nil {
		return out
	}
	match := -1
	for index := range configured {
		if reflect.DeepEqual(
			canonicalHostScheduleMCPBindingV1(configured[index]),
			canonicalHostScheduleMCPBindingV1(bound),
		) {
			if match >= 0 {
				return out
			}
			match = index
		}
	}
	if match < 0 {
		return out
	}
	effective := out[match]
	effective.ExpectedServerName = hostScheduleMCPServerNameV1
	effective.ExpectedServerVersion = hostScheduleMCPServerVersionV1
	effective.IdentitySource = hostScheduleMCPIdentitySourceV1
	effective.ReadOnlyToolNames = map[string]bool{
		hostScheduleMCPListToolNameV1:       true,
		hostScheduleMCPLegacyListToolNameV1: true,
	}
	out[match] = effective
	return out
}

func stripHostScheduleMCPPolicyV1(spec ServerSpec) ServerSpec {
	if spec.ID != hostScheduleMCPServerIDV1 {
		return spec
	}
	if spec.IdentitySource == hostScheduleMCPIdentitySourceV1 {
		spec.IdentitySource = ""
	}
	spec.ReadOnlyToolNames = nil
	return spec
}

func canonicalHostScheduleMCPBindingV1(spec ServerSpec) ServerSpec {
	if len(spec.Headers) == 0 {
		spec.Headers = nil
	}
	if len(spec.TrustedWorkspaceRoots) == 0 {
		spec.TrustedWorkspaceRoots = nil
	}
	if len(spec.ReadOnlyToolNames) == 0 {
		spec.ReadOnlyToolNames = nil
	}
	if len(spec.Tools) == 0 {
		spec.Tools = nil
	}
	return spec
}

func hostScheduleLoopbackPortV1(spec ServerSpec) uint16 {
	if spec.ExpectedServerName != hostScheduleMCPServerNameV1 ||
		spec.ExpectedServerVersion != hostScheduleMCPServerVersionV1 ||
		spec.IdentitySource != hostScheduleMCPIdentitySourceV1 ||
		len(spec.ReadOnlyToolNames) != 2 ||
		!spec.ReadOnlyToolNames[hostScheduleMCPListToolNameV1] ||
		!spec.ReadOnlyToolNames[hostScheduleMCPLegacyListToolNameV1] {
		return 0
	}
	bound := cloneServerSpecForAuthority(spec)
	bound.ExpectedServerName = ""
	bound.ExpectedServerVersion = ""
	bound.IdentitySource = ""
	bound.ReadOnlyToolNames = nil
	port, err := ValidateHostScheduleMCPBindingV1(bound)
	if err != nil {
		return 0
	}
	return port
}
