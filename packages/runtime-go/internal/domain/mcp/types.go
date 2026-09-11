package mcp

import (
	"encoding/json"
	"errors"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainproviderregistry "analytix.local/runtime-go/internal/domain/providerregistry"
)

const (
	DefaultServerTimeoutMSV1 = int64(30_000)
	MaxServerTimeoutMSV1     = int64(3_600_000)
)

func ResolveServerTimeoutV1(timeoutMS int64) (time.Duration, error) {
	if timeoutMS == 0 {
		timeoutMS = DefaultServerTimeoutMSV1
	}
	if timeoutMS < 1 || timeoutMS > MaxServerTimeoutMSV1 {
		return 0, errors.New("MCP server timeout is invalid")
	}
	return time.Duration(timeoutMS) * time.Millisecond, nil
}

const HostEvidenceSettlementCarrierKey = "\x00analytixHostEvidenceSettlement"

type ToolTaskSupport string

const (
	ToolTaskSupportForbidden ToolTaskSupport = "forbidden"
	ToolTaskSupportOptional  ToolTaskSupport = "optional"
	ToolTaskSupportRequired  ToolTaskSupport = "required"
)

func NormalizeToolTaskSupport(value ToolTaskSupport) (ToolTaskSupport, bool) {
	switch value {
	case "", ToolTaskSupportForbidden:
		return ToolTaskSupportForbidden, true
	case ToolTaskSupportOptional:
		return ToolTaskSupportOptional, true
	case ToolTaskSupportRequired:
		return ToolTaskSupportRequired, true
	default:
		return "", false
	}
}

type HostEvidenceSettlementCarrier struct {
	Marker domainevidence.HostEvidenceSettlementMarker
}

func ExtractHostEvidenceSettlementCarrier(output map[string]any) (domainevidence.HostEvidenceSettlementMarker, bool) {
	if output == nil {
		return domainevidence.HostEvidenceSettlementMarker{}, false
	}
	carrier, ok := output[HostEvidenceSettlementCarrierKey].(HostEvidenceSettlementCarrier)
	if !ok || domainevidence.ValidateHostEvidenceSettlementMarker(carrier.Marker) != nil {
		return domainevidence.HostEvidenceSettlementMarker{}, false
	}
	return carrier.Marker, true
}

type ToolSpec struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
	ReadOnlyHint bool            `json:"readOnlyHint"`
	ResultText   string          `json:"resultText"`
	TaskSupport  ToolTaskSupport `json:"taskSupport"`
}

// ToolAdvertisementV1 is the immutable, host-authoritative MCP tool view for
// one provider request. Every field must be captured from the same live
// catalog observation; remote read-only hints are not authority for ReadOnly.
type ToolAdvertisementV1 struct {
	Name            string
	Description     string
	InputSchema     json.RawMessage
	OutputSchema    json.RawMessage
	TaskSupport     ToolTaskSupport
	ReadOnly        bool
	ConnectionEpoch uint64
	ServerIdentity  string
}

type PromptSpec struct {
	ServerID    string               `json:"serverId,omitempty"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Arguments   []PromptArgumentSpec `json:"arguments,omitempty"`
}

type PromptArgumentSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type ResourceSpec struct {
	ServerID    string `json:"serverId,omitempty"`
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type ServerIdentity struct {
	ProtocolVersion string `json:"protocolVersion"`
	Name            string `json:"name"`
	Version         string `json:"version"`
}

type AccountCredentialScope struct {
	Owner              string `json:"owner,omitempty"`
	Provider           string `json:"provider"`
	AccountID          string `json:"accountId"`
	Purpose            string `json:"purpose"`
	BindingFingerprint string `json:"bindingFingerprint,omitempty"`
}

type ServerSpec struct {
	ID                      string                                       `json:"id"`
	Transport               string                                       `json:"transport"`
	Command                 string                                       `json:"command,omitempty"`
	Args                    []string                                     `json:"args,omitempty"`
	Env                     map[string]string                            `json:"env,omitempty"`
	URL                     string                                       `json:"url,omitempty"`
	Headers                 map[string]string                            `json:"headers,omitempty"`
	AccountCredential       *AccountCredentialScope                      `json:"accountCredential,omitempty"`
	OAuthBinding            *domainproviderregistry.OAuthBindingMetadata `json:"oauthBinding,omitempty"`
	CWD                     string                                       `json:"cwd,omitempty"`
	ExpectedServerName      string                                       `json:"expectedServerName,omitempty"`
	ExpectedServerVersion   string                                       `json:"expectedServerVersion,omitempty"`
	IdentitySource          string                                       `json:"identitySource,omitempty"`
	ManifestSHA256          string                                       `json:"manifestSha256,omitempty"`
	EntrypointPath          string                                       `json:"entrypointPath,omitempty"`
	EntrypointSHA256        string                                       `json:"entrypointSha256,omitempty"`
	PluginRootPath          string                                       `json:"pluginRootPath,omitempty"`
	SourceTreeSHA256        string                                       `json:"sourceTreeSha256,omitempty"`
	HostInstallMarkerSHA256 string                                       `json:"-"`
	TrustScope              string                                       `json:"trustScope,omitempty"`
	TrustedWorkspaceRoots   []string                                     `json:"trustedWorkspaceRoots,omitempty"`
	LowPriority             bool                                         `json:"lowPriority"`
	BackgroundStart         bool                                         `json:"backgroundStart"`
	TimeoutMS               int64                                        `json:"timeoutMs,omitempty"`
	ReadOnlyToolNames       map[string]bool                              `json:"-"`
	Tools                   []ToolSpec                                   `json:"tools,omitempty"`
}
