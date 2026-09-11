package mcp

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	mcpcache "analytix.local/runtime-go/internal/adapters/outbound/mcp/cache"
	httpmcp "analytix.local/runtime-go/internal/adapters/outbound/mcp/http"
	mcpidentity "analytix.local/runtime-go/internal/adapters/outbound/mcp/identity"
	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	mcpredaction "analytix.local/runtime-go/internal/adapters/outbound/mcp/redaction"
	mcpstdio "analytix.local/runtime-go/internal/adapters/outbound/mcp/stdio"
	appmcp "analytix.local/runtime-go/internal/app/mcp"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmcpprotocol "analytix.local/runtime-go/internal/domain/mcpprotocol"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	domainruntimeconfig "analytix.local/runtime-go/internal/domain/runtimeconfig"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	runtimeports "analytix.local/runtime-go/internal/ports"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

type ToolSpec = domainmcp.ToolSpec
type PromptSpec = domainmcp.PromptSpec
type PromptArgumentSpec = domainmcp.PromptArgumentSpec
type ResourceSpec = domainmcp.ResourceSpec
type ServerSpec = domainmcp.ServerSpec

const (
	caseFactHostQuarantineTransport = "host-native-quarantined"
	caseFactHostQuarantineFailure   = "case data source native authority is unavailable"
)

var (
	_ runtimeports.RuntimeMCPManager                     = (*ProductionManager)(nil)
	_ sourceprobeport.Prober                             = (*ProductionManager)(nil)
	_ sourceprobeport.LockedCurrent                      = (*ProductionManager)(nil)
	_ sourceprobeport.LockedCurrentAuthority             = (*ProductionManager)(nil)
	_ sourceprobeport.CurrentValidator                   = (*ProductionManager)(nil)
	_ sourceprobeport.LockedEvidenceReader               = (*ProductionManager)(nil)
	_ sourceprobeport.LockedEvidenceAuthorityReader      = (*ProductionManager)(nil)
	_ sourceprobeport.LockedPublicationSnapshot          = (*ProductionManager)(nil)
	_ sourceprobeport.LockedPublicationSnapshotAuthority = (*ProductionManager)(nil)
)

const (
	fundsCountEvidenceToolName = "mcp__analytix_funds__count_case_rows"
	fundsEvidenceReadMethod    = "analytix/evidenceRead"
)

type managedTool struct {
	ServerID    string
	Name        string
	RawName     string
	Tool        ToolSpec
	Placeholder bool
}

func isHostFundsCountManagedToolV1(name string, tool managedTool) bool {
	return name == fundsCountEvidenceToolName && tool.ServerID == "analytix_funds" &&
		tool.RawName == hostFundsCountToolNameV1
}

func (m *ProductionManager) managedToolCapabilityCurrentNoLock(name string, tool managedTool) bool {
	return !isHostFundsCountManagedToolV1(name, tool) || m.hostFundsSourceReadGrantedNoLock()
}

type ProductionManager struct {
	mu                           sync.Mutex
	sourceExecutionMu            sync.RWMutex
	specs                        []ServerSpec
	cacheDir                     string
	proxyURL                     string
	protectedReadDirs            []string
	clients                      map[string]mcpTransportClient
	tools                        map[string]managedTool
	prompts                      []PromptSpec
	resources                    []ResourceSpec
	failures                     map[string]string
	cachedSchemas                map[string]bool
	specFingerprints             map[string]string
	connectionEpochs             map[string]uint64
	connectionInstanceID         string
	serverIdentities             map[string]string
	sourceCatalogs               map[string]string
	sourceProbes                 map[string]domainsecurity.VerifiedSourceProbe
	sourceAdmissions             map[string]sourceEvidenceAdmissionV2
	toolContractIssues           map[string][]mcpprotocol.ToolContractIssue
	datasetAuthority             datasetsnapshotport.CurrentAuthorityV2
	allowCaseFactSource          bool
	hostFundsServer              *HostFundsServerSpecV1
	hostFundsFingerprint         string
	hostFundsSourceReadLifecycle *domainplugincapability.FundsSourceReadLifecycleV1
	hostFundsCapabilityEvents    []domainplugincapability.FundsSourceReadDecisionEventV1
	hostFundsNativeOwnerCurrent  func(context.Context) error
	accountFlowExecutor          AccountFlowExecutor
	accountCredentialResolver    mcpAccountCredentialResolver
	refreshes                    int
	restarts                     int
	lastRefreshedAt              string
	catalogHash                  string
	catalogDrift                 bool
	calls                        int
}

type mcpAccountCredentialResolver interface {
	ResolveAccountCredential(context.Context, domainregistry.PrivateAccountScope) (providerregistryapp.AccountCredentialResolution, error)
	ValidateAccountCredentialCurrent(context.Context, providerregistryapp.AccountCredentialState) error
}

type mcpAccountCredentialBoundClient struct {
	mu                sync.RWMutex
	spec              ServerSpec
	proxyURL          string
	protectedReadDirs []string
	resolver          mcpAccountCredentialResolver
	newTransport      func(ServerSpec, string, []string) (mcpTransportClient, error)
	observed          domainmcp.ServerIdentity
	closed            bool
}

func (client *mcpAccountCredentialBoundClient) accountScope() (domainregistry.PrivateAccountScope, bool) {
	if client == nil || client.resolver == nil || client.spec.AccountCredential == nil ||
		!validMCPAccountCredentialBinding(client.spec) || hasMCPAuthorizationHeader(client.spec.Headers) {
		return domainregistry.PrivateAccountScope{}, false
	}
	credential := client.spec.AccountCredential
	return domainregistry.PrivateAccountScope{
		SchemaVersion: 1,
		Owner:         normalizedMCPAccountCredentialOwner(credential.Owner),
		Provider:      credential.Provider,
		AccountID:     credential.AccountID,
		ChannelID:     client.spec.ID,
		Purpose:       credential.Purpose,
	}, true
}

func (client *mcpAccountCredentialBoundClient) withEffect(
	ctx context.Context,
	effect func(mcpTransportClient) (any, error),
) (any, error) {
	if ctx == nil {
		return nil, errors.New("MCP account effect is unavailable")
	}
	client.mu.RLock()
	closed := client.closed
	client.mu.RUnlock()
	if closed {
		return nil, errors.New("MCP account effect is unavailable")
	}
	scope, ok := client.accountScope()
	if !ok {
		return nil, errors.New("MCP account effect is unavailable")
	}
	resolution, err := client.resolver.ResolveAccountCredential(ctx, scope)
	if err != nil {
		return nil, errors.New("MCP account effect is unavailable")
	}
	defer resolution.Clear()
	if resolution.State.Status != providerregistryapp.AccountCredentialStatusReady ||
		!reflect.DeepEqual(resolution.State.Scope, scope) || len(resolution.Credential) == 0 ||
		len(resolution.Credential) > 64*1024 {
		return nil, errors.New("MCP account effect is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(resolution.Credential))
	decoder.DisallowUnknownFields()
	var draft struct {
		Kind               string          `json:"kind"`
		Token              string          `json:"token"`
		AccessToken        string          `json:"accessToken"`
		RefreshToken       string          `json:"refreshToken,omitempty"`
		TokenType          string          `json:"tokenType,omitempty"`
		ExpiresAtMS        int64           `json:"expiresAtMs,omitempty"`
		BindingFingerprint string          `json:"bindingFingerprint,omitempty"`
		OAuthBinding       json.RawMessage `json:"oauthBinding,omitempty"`
	}
	expectedKind := "mcp-oauth-token"
	if scope.Owner == "extension" {
		expectedKind = "extension-account-token"
	}
	if decoder.Decode(&draft) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return nil, errors.New("MCP account effect is unavailable")
	}
	token := draft.Token
	bundleKind := (scope.Owner == "mcp" && draft.Kind == "mcp-oauth-bundle") ||
		(scope.Owner == "extension" && draft.Kind == "extension-oauth-bundle")
	if bundleKind && draft.TokenType == "Bearer" {
		if draft.ExpiresAtMS > 0 && time.Now().UnixMilli() >= draft.ExpiresAtMS {
			return nil, errors.New("MCP account effect is unavailable")
		}
		var binding struct {
			Owner                 string   `json:"owner"`
			Issuer                string   `json:"issuer"`
			AuthorizationEndpoint string   `json:"authorizationEndpoint"`
			TokenEndpoint         string   `json:"tokenEndpoint"`
			RevocationEndpoint    string   `json:"revocationEndpoint,omitempty"`
			ClientID              string   `json:"clientId"`
			Provider              string   `json:"provider"`
			AccountID             string   `json:"accountId"`
			ChannelID             string   `json:"channelId"`
			RedirectURI           string   `json:"redirectUri"`
			Scopes                []string `json:"scopes"`
			BindingKey            string   `json:"bindingKey"`
			OwnerFingerprint      string   `json:"ownerFingerprint"`
			OwnerRevision         string   `json:"ownerRevision"`
			OwnerGeneration       string   `json:"ownerGeneration"`
			OwnerIncarnation      string   `json:"ownerIncarnation"`
			ProfileBinding        string   `json:"profileBinding"`
		}
		bindingDecoder := json.NewDecoder(bytes.NewReader(draft.OAuthBinding))
		bindingDecoder.DisallowUnknownFields()
		if bindingDecoder.Decode(&binding) != nil || !errors.Is(bindingDecoder.Decode(&struct{}{}), io.EOF) {
			return nil, errors.New("MCP account effect is unavailable")
		}
		keyFreeBinding := domainregistry.OAuthBindingMetadata{
			SchemaVersion: 1, Issuer: binding.Issuer,
			AuthorizationEndpoint: binding.AuthorizationEndpoint,
			TokenEndpoint:         binding.TokenEndpoint, RevocationEndpoint: binding.RevocationEndpoint,
			ClientID: binding.ClientID, Scopes: slices.Clone(binding.Scopes), RedirectModeVersion: 1,
		}
		if binding.Owner != scope.Owner ||
			binding.Provider != scope.Provider || binding.AccountID != scope.AccountID ||
			binding.ChannelID != scope.ChannelID || specOAuthBindingUnavailable(client.spec, keyFreeBinding) ||
			len(client.spec.AccountCredential.BindingFingerprint) != 64 ||
			binding.OwnerFingerprint != client.spec.AccountCredential.BindingFingerprint ||
			binding.OwnerIncarnation != "cfg_"+client.spec.AccountCredential.BindingFingerprint ||
			len(binding.BindingKey) != len("oauthb_")+43 ||
			binding.RedirectURI != "com.analytix.desktop:/oauth/callback/"+binding.BindingKey ||
			len(binding.ProfileBinding) != 64 || !validDecimalFence(binding.OwnerRevision) ||
			!validDecimalFence(binding.OwnerGeneration) {
			return nil, errors.New("MCP account effect is unavailable")
		}
		token = draft.AccessToken
	} else if draft.Kind != expectedKind || scope.Owner == "extension" &&
		(len(client.spec.AccountCredential.BindingFingerprint) != 64 ||
			draft.BindingFingerprint != client.spec.AccountCredential.BindingFingerprint) {
		return nil, errors.New("MCP account effect is unavailable")
	}
	if strings.TrimSpace(token) == "" || len(token) > 32*1024 {
		return nil, errors.New("MCP account effect is unavailable")
	}
	effectSpec := cloneServerSpecForAuthority(client.spec)
	effectSpec.AccountCredential = nil
	effectSpec.Headers = cloneStringMap(effectSpec.Headers)
	if effectSpec.Headers == nil {
		effectSpec.Headers = map[string]string{}
	}
	effectSpec.Headers["Authorization"] = "Bearer " + token
	draft.Token = ""
	draft.AccessToken = ""
	draft.RefreshToken = ""
	clear(draft.OAuthBinding)
	token = ""
	constructor := client.newTransport
	if constructor == nil {
		constructor = newMCPTransportClient
	}
	transport, err := constructor(effectSpec, client.proxyURL, client.protectedReadDirs)
	if err != nil {
		delete(effectSpec.Headers, "Authorization")
		return nil, errors.New("MCP account effect is unavailable")
	}
	defer func() {
		transport.Close()
		delete(effectSpec.Headers, "Authorization")
	}()
	result, effectErr := effect(transport)
	if client.resolver.ValidateAccountCredentialCurrent(ctx, resolution.State) != nil {
		return nil, errors.New("MCP account credential changed")
	}
	if effectErr != nil {
		return nil, errors.New("MCP account effect failed")
	}
	if observed, identityOK := transport.(mcpObservedIdentityClient); identityOK {
		client.mu.Lock()
		if !client.closed {
			client.observed = observed.ObservedServerIdentity()
		}
		client.mu.Unlock()
	}
	return result, nil
}

func (client *mcpAccountCredentialBoundClient) ListTools() ([]ToolSpec, error) {
	result, err := client.withEffect(context.Background(), func(inner mcpTransportClient) (any, error) {
		return inner.ListTools()
	})
	if err != nil {
		return nil, err
	}
	tools, ok := result.([]ToolSpec)
	if !ok {
		return nil, errors.New("MCP account effect failed")
	}
	return tools, nil
}

func (client *mcpAccountCredentialBoundClient) ListPrompts() ([]PromptSpec, error) {
	result, err := client.withEffect(context.Background(), func(inner mcpTransportClient) (any, error) {
		return inner.ListPrompts()
	})
	if err != nil {
		return nil, err
	}
	prompts, ok := result.([]PromptSpec)
	if !ok {
		return nil, errors.New("MCP account effect failed")
	}
	return prompts, nil
}

func (client *mcpAccountCredentialBoundClient) ListResources() ([]ResourceSpec, error) {
	result, err := client.withEffect(context.Background(), func(inner mcpTransportClient) (any, error) {
		return inner.ListResources()
	})
	if err != nil {
		return nil, err
	}
	resources, ok := result.([]ResourceSpec)
	if !ok {
		return nil, errors.New("MCP account effect failed")
	}
	return resources, nil
}

func (client *mcpAccountCredentialBoundClient) CallTool(name string, arguments map[string]any) (any, error) {
	return client.withEffect(context.Background(), func(inner mcpTransportClient) (any, error) {
		return inner.CallTool(name, arguments)
	})
}

func (client *mcpAccountCredentialBoundClient) CallToolContext(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (any, error) {
	return client.withEffect(ctx, func(inner mcpTransportClient) (any, error) {
		if contextual, ok := inner.(mcpContextTransportClient); ok {
			return contextual.CallToolContext(ctx, name, arguments)
		}
		return inner.CallTool(name, arguments)
	})
}

func (client *mcpAccountCredentialBoundClient) ListToolCatalogContext(
	ctx context.Context,
) (mcpprotocol.ToolCatalog, error) {
	result, err := client.withEffect(ctx, func(inner mcpTransportClient) (any, error) {
		return listMCPToolCatalogContext(ctx, inner)
	})
	if err != nil {
		return mcpprotocol.ToolCatalog{}, err
	}
	catalog, ok := result.(mcpprotocol.ToolCatalog)
	if !ok {
		return mcpprotocol.ToolCatalog{}, errors.New("MCP account effect failed")
	}
	return catalog, nil
}

func (client *mcpAccountCredentialBoundClient) ObservedServerIdentity() domainmcp.ServerIdentity {
	if client == nil {
		return domainmcp.ServerIdentity{}
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	return client.observed
}

func (client *mcpAccountCredentialBoundClient) Close() {
	if client == nil {
		return
	}
	client.mu.Lock()
	client.closed = true
	client.observed = domainmcp.ServerIdentity{}
	client.mu.Unlock()
}

type mcpTransportClient interface {
	ListTools() ([]ToolSpec, error)
	ListPrompts() ([]PromptSpec, error)
	ListResources() ([]ResourceSpec, error)
	CallTool(name string, arguments map[string]any) (any, error)
	Close()
}

type mcpContextTransportClient interface {
	CallToolContext(ctx context.Context, name string, arguments map[string]any) (any, error)
}

type mcpHostContextTransportClient interface {
	CallToolWithHostContext(ctx context.Context, name string, arguments map[string]any, envelope domainmcp.HostContextEnvelope) (any, error)
}

type mcpNativeProbeClient interface {
	ListToolsContext(ctx context.Context) ([]ToolSpec, error)
	CallNativeContext(ctx context.Context, method string, params map[string]any) (any, error)
}

type mcpToolCatalogClient interface {
	ListToolCatalogContext(ctx context.Context) (mcpprotocol.ToolCatalog, error)
}

type mcpNativeEvidenceClient interface {
	CallNativeLosslessContext(ctx context.Context, method string, params map[string]any) (domainmcp.LosslessToolResult, error)
}

type mcpObservedIdentityClient interface {
	ObservedServerIdentity() domainmcp.ServerIdentity
}

func listMCPToolCatalogContext(ctx context.Context, client mcpTransportClient) (mcpprotocol.ToolCatalog, error) {
	if client == nil {
		return mcpprotocol.ToolCatalog{}, errors.New("MCP transport is unavailable")
	}
	if catalogClient, ok := client.(mcpToolCatalogClient); ok {
		return catalogClient.ListToolCatalogContext(ctx)
	}
	if contextual, ok := client.(interface {
		ListToolsContext(context.Context) ([]ToolSpec, error)
	}); ok {
		tools, err := contextual.ListToolsContext(ctx)
		return mcpprotocol.ToolCatalog{Tools: tools}, err
	}
	tools, err := client.ListTools()
	return mcpprotocol.ToolCatalog{Tools: tools}, err
}

func cloneToolContractIssues(issues []mcpprotocol.ToolContractIssue) []mcpprotocol.ToolContractIssue {
	if len(issues) == 0 {
		return nil
	}
	cloned := append([]mcpprotocol.ToolContractIssue(nil), issues...)
	sort.SliceStable(cloned, func(i, j int) bool {
		if cloned[i].Name == cloned[j].Name {
			return cloned[i].Code < cloned[j].Code
		}
		return cloned[i].Name < cloned[j].Name
	})
	return cloned
}

type mcpServerCapabilities = mcpprotocol.Capabilities

type ProductionManagerOptions struct {
	CacheDir                    string
	ProxyURL                    string
	ProtectedReadDirs           []string
	DatasetAuthority            datasetsnapshotport.CurrentAuthorityV2
	HostFundsServer             *HostFundsServerSpecV1
	HostFundsNativeOwnerCurrent func(context.Context) error
	AccountFlowExecutor         AccountFlowExecutor
}

func NewProductionManager(specs []ServerSpec) *ProductionManager {
	return NewProductionManagerWithCache(specs, "")
}

func NewProductionManagerWithCache(specs []ServerSpec, cacheDir string) *ProductionManager {
	return NewProductionManagerWithOptions(specs, ProductionManagerOptions{CacheDir: cacheDir})
}

func NewProductionManagerWithOptions(specs []ServerSpec, options ProductionManagerOptions) *ProductionManager {
	normalizedSpecs := normalizeProductionSpecsForAuthority(specs, false)
	var hostFundsServer *HostFundsServerSpecV1
	hostFundsFingerprint := ""
	var hostFundsSourceReadLifecycle *domainplugincapability.FundsSourceReadLifecycleV1
	hostFundsCapabilityEvents := []domainplugincapability.FundsSourceReadDecisionEventV1{}
	var hostFundsNativeOwnerCurrent func(context.Context) error
	var accountFlowExecutor AccountFlowExecutor
	if options.DatasetAuthority != nil && options.HostFundsServer.valid() {
		// Ordinary configuration may describe the reserved namespace only as a
		// quarantined record. It must never collide with or suppress the exact
		// host-owned capability admitted below.
		normalizedSpecs = withoutReservedFundsSpecsV1(normalizedSpecs)
		hostSpec := cloneServerSpecForAuthority(options.HostFundsServer.spec)
		normalizedSpecs = append(normalizedSpecs, hostSpec)
		sort.SliceStable(normalizedSpecs, func(i, j int) bool { return normalizedSpecs[i].ID < normalizedSpecs[j].ID })
		hostFundsServer = cloneHostFundsServerSpecV1(options.HostFundsServer)
		hostFundsFingerprint = options.HostFundsServer.fingerprint
		lifecycle, event, lifecycleErr := domainplugincapability.NewFundsSourceReadLifecycleV1(
			hostFundsServer.sourceReadBinding,
		)
		if lifecycleErr == nil {
			hostFundsSourceReadLifecycle = lifecycle
			hostFundsCapabilityEvents = append(hostFundsCapabilityEvents, event)
		}
		hostFundsNativeOwnerCurrent = options.HostFundsNativeOwnerCurrent
		accountFlowExecutor = options.AccountFlowExecutor
	}
	return &ProductionManager{
		specs:                        normalizedSpecs,
		cacheDir:                     strings.TrimSpace(options.CacheDir),
		proxyURL:                     strings.TrimSpace(options.ProxyURL),
		protectedReadDirs:            append([]string(nil), options.ProtectedReadDirs...),
		clients:                      map[string]mcpTransportClient{},
		tools:                        map[string]managedTool{},
		prompts:                      []PromptSpec{},
		resources:                    []ResourceSpec{},
		failures:                     map[string]string{},
		cachedSchemas:                map[string]bool{},
		specFingerprints:             map[string]string{},
		connectionEpochs:             map[string]uint64{},
		connectionInstanceID:         newMCPConnectionInstanceID(),
		serverIdentities:             map[string]string{},
		sourceCatalogs:               map[string]string{},
		sourceProbes:                 map[string]domainsecurity.VerifiedSourceProbe{},
		sourceAdmissions:             map[string]sourceEvidenceAdmissionV2{},
		toolContractIssues:           map[string][]mcpprotocol.ToolContractIssue{},
		datasetAuthority:             options.DatasetAuthority,
		allowCaseFactSource:          options.DatasetAuthority != nil,
		hostFundsServer:              hostFundsServer,
		hostFundsFingerprint:         hostFundsFingerprint,
		hostFundsSourceReadLifecycle: hostFundsSourceReadLifecycle,
		hostFundsCapabilityEvents:    hostFundsCapabilityEvents,
		hostFundsNativeOwnerCurrent:  hostFundsNativeOwnerCurrent,
		accountFlowExecutor:          accountFlowExecutor,
	}
}

func withoutReservedFundsSpecsV1(specs []ServerSpec) []ServerSpec {
	out := make([]ServerSpec, 0, len(specs))
	for _, spec := range specs {
		if reservedCaseFactMCPServerID(spec.ID) {
			continue
		}
		out = append(out, spec)
	}
	return out
}

const maxHostFundsCapabilityEventsV1 = 64

func (m *ProductionManager) appendHostFundsCapabilityEventNoLock(
	event domainplugincapability.FundsSourceReadDecisionEventV1,
) {
	if m == nil || event.Schema == "" {
		return
	}
	if count := len(m.hostFundsCapabilityEvents); count > 0 &&
		reflect.DeepEqual(m.hostFundsCapabilityEvents[count-1], event) {
		return
	}
	m.hostFundsCapabilityEvents = append(m.hostFundsCapabilityEvents, event)
	if len(m.hostFundsCapabilityEvents) > maxHostFundsCapabilityEventsV1 {
		m.hostFundsCapabilityEvents = append(
			[]domainplugincapability.FundsSourceReadDecisionEventV1(nil),
			m.hostFundsCapabilityEvents[len(m.hostFundsCapabilityEvents)-maxHostFundsCapabilityEventsV1:]...,
		)
	}
}

func (m *ProductionManager) beginHostFundsSourceReadSetupNoLock() {
	if m == nil || m.hostFundsSourceReadLifecycle == nil {
		return
	}
	event, err := m.hostFundsSourceReadLifecycle.BeginSetup()
	if err == nil {
		m.appendHostFundsCapabilityEventNoLock(event)
	}
}

func (m *ProductionManager) revokeHostFundsSourceReadNoLock(
	state domainplugincapability.FundsSourceReadStateV1,
	reason domainplugincapability.FundsSourceReadReasonCodeV1,
) {
	if m == nil || m.hostFundsSourceReadLifecycle == nil ||
		m.hostFundsServer == nil || !m.hostFundsServer.sourceReadRequestedV1() {
		return
	}
	event, err := m.hostFundsSourceReadLifecycle.Revoke(state, reason)
	if err == nil {
		m.appendHostFundsCapabilityEventNoLock(event)
	}
}

func hostFundsSourceReadAuthorityV1(
	specFingerprint string,
	serverIdentity string,
	catalogDigest string,
	connectionEpoch uint64,
) domainplugincapability.FundsSourceReadHostAuthorityV1 {
	return domainplugincapability.FundsSourceReadHostAuthorityV1{
		SpecFingerprint:      specFingerprint,
		ServerIdentityDigest: domainsecurity.SHA256Hex([]byte(serverIdentity)),
		CatalogDigest:        catalogDigest,
		ConnectionEpoch:      connectionEpoch,
	}
}

func (m *ProductionManager) activateHostFundsSourceReadNoLock(
	specFingerprint string,
	serverIdentity string,
	catalogDigest string,
	connectionEpoch uint64,
) bool {
	if m == nil || m.hostFundsSourceReadLifecycle == nil ||
		m.hostFundsServer == nil || !m.hostFundsServer.sourceReadRequestedV1() {
		return false
	}
	_, event, err := m.hostFundsSourceReadLifecycle.Activate(hostFundsSourceReadAuthorityV1(
		specFingerprint, serverIdentity, catalogDigest, connectionEpoch,
	))
	if err != nil {
		m.revokeHostFundsSourceReadNoLock(
			domainplugincapability.FundsSourceReadStateFailedV1,
			domainplugincapability.FundsSourceReadReasonHealthFailedV1,
		)
		return false
	}
	m.appendHostFundsCapabilityEventNoLock(event)
	return true
}

func (m *ProductionManager) hostFundsSourceReadGrantedNoLock() bool {
	if m == nil || m.hostFundsSourceReadLifecycle == nil || m.hostFundsServer == nil ||
		!m.hostFundsServer.sourceReadRequestedV1() || m.hostFundsFingerprint == "" {
		return false
	}
	spec, specOK := m.specByIDNoLock("analytix_funds")
	if !specOK || !m.hostFundsServer.matches(spec, m.hostFundsFingerprint) ||
		mcpidentity.VerifyConfiguredProvenance(spec) != nil {
		m.revokeHostFundsSourceReadNoLock(
			domainplugincapability.FundsSourceReadStateFailedV1,
			domainplugincapability.FundsSourceReadReasonProvenanceFailedV1,
		)
		return false
	}
	grant, ok := m.hostFundsSourceReadLifecycle.CurrentGrant()
	if !ok {
		return false
	}
	current := m.hostFundsSourceReadLifecycle.Authorizes(
		grant,
		domainplugincapability.FundsSourceReadOperationV1,
		hostFundsSourceReadAuthorityV1(
			m.hostFundsFingerprint,
			m.serverIdentities["analytix_funds"],
			m.sourceCatalogs["analytix_funds"],
			m.connectionEpochs["analytix_funds"],
		),
	)
	if !current {
		m.revokeHostFundsSourceReadNoLock(
			domainplugincapability.FundsSourceReadStateRevokedV1,
			domainplugincapability.FundsSourceReadReasonAuthorityRevokedV1,
		)
	}
	return current
}

func (m *ProductionManager) HostFundsSourceReadCapabilityEventsV1() []domainplugincapability.FundsSourceReadDecisionEventV1 {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]domainplugincapability.FundsSourceReadDecisionEventV1(nil), m.hostFundsCapabilityEvents...)
	for index := range out {
		out[index].ScopeConstraints = append([]string(nil), out[index].ScopeConstraints...)
	}
	return out
}

// InvalidateHostFundsSourceAfterSettledNativeFailure propagates a settled
// process-local native owner failure into the existing Host-owned Funds
// lifecycle. It removes only current Funds source authority; the MCP
// connection and ordinary/non-Funds catalog remain available.
func (m *ProductionManager) InvalidateHostFundsSourceAfterSettledNativeFailure() {
	if m == nil {
		return
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidateHostFundsSourceAfterNativeFailureNoLock()
}

func (m *ProductionManager) invalidateHostFundsSourceAfterNativeFailureNoLock() {
	m.revokeHostFundsSourceReadNoLock(
		domainplugincapability.FundsSourceReadStateFailedV1,
		domainplugincapability.FundsSourceReadReasonHealthFailedV1,
	)
	m.clearSourceAuthorityForServerNoLock("analytix_funds")
}

func (m *ProductionManager) requireHostFundsNativeOwnerCurrent(
	ctx context.Context,
	serverID string,
) error {
	if m == nil {
		return errors.New("case source probe native owner is unavailable")
	}
	current := m.hostFundsNativeOwnerCurrent
	if serverID != "analytix_funds" || current == nil || current(ctx) != nil {
		m.mu.Lock()
		m.invalidateHostFundsSourceAfterNativeFailureNoLock()
		m.mu.Unlock()
		return errors.New("case source probe native owner is unavailable")
	}
	return nil
}

func (m *ProductionManager) Connect() {
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectNoLock(false)
}

func (m *ProductionManager) SetAccountCredentialResolver(resolver mcpAccountCredentialResolver) {
	if m == nil {
		return
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accountCredentialResolver = resolver
}

func (m *ProductionManager) connectNoLock(includeBackground bool) {
	m.beginHostFundsSourceReadSetupNoLock()
	for _, client := range m.clients {
		client.Close()
	}
	m.clients = map[string]mcpTransportClient{}
	m.serverIdentities = map[string]string{}
	m.sourceCatalogs = map[string]string{}
	m.tools = map[string]managedTool{}
	m.prompts = []PromptSpec{}
	m.resources = []ResourceSpec{}
	m.failures = map[string]string{}
	m.sourceProbes = map[string]domainsecurity.VerifiedSourceProbe{}
	m.sourceAdmissions = map[string]sourceEvidenceAdmissionV2{}
	m.toolContractIssues = map[string][]mcpprotocol.ToolContractIssue{}
	m.cachedSchemas = map[string]bool{}
	m.specFingerprints = map[string]string{}
	specs := m.normalizeProductionSpecsNoLock()
	namespaceFailures := productionNamespaceFailures(specs)
	for _, spec := range specs {
		if reason := namespaceFailures[spec.ID]; reason != "" {
			m.failures[spec.ID] = reason
			continue
		}
		specHash := SpecFingerprint(spec)
		m.specFingerprints[spec.ID] = specHash
		if spec.BackgroundStart && !includeBackground {
			if !m.registerCachedSchemaNoLock(spec, specHash) {
				if err := m.registerLazyConnectPlaceholderNoLock(spec); err != nil {
					m.failures[spec.ID] = "MCP server namespace is ambiguous"
				}
			}
			continue
		}
		m.connectSpecNoLock(spec, specHash)
	}
	m.refreshCatalogStateNoLock()
}

func (m *ProductionManager) refreshCatalogStateNoLock() {
	m.lastRefreshedAt = time.Now().UTC().Format(time.RFC3339Nano)
	m.catalogHash = catalogFingerprintForManagedToolsWithIssues(m.tools, m.toolContractIssues)
}

func (m *ProductionManager) connectSpecNoLock(spec ServerSpec, specHash string) bool {
	m.removeServerToolsNoLock(spec.ID)
	delete(m.serverIdentities, spec.ID)
	delete(m.toolContractIssues, spec.ID)
	if spec.Transport == caseFactHostQuarantineTransport {
		m.failures[spec.ID] = caseFactHostQuarantineFailure
		return false
	}
	if err := mcpidentity.VerifyConfiguredProvenance(spec); err != nil {
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonProvenanceFailedV1,
			)
		}
		m.failures[spec.ID] = redactMCPDiagnosticText(err.Error(), spec)
		return false
	}
	client, err := m.newMCPTransportClientNoLock(spec, specHash)
	if err != nil {
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonHealthFailedV1,
			)
		}
		m.failures[spec.ID] = redactMCPDiagnosticText(err.Error(), spec)
		if mcpCatalogFailureAllowsSchemaHint(err) {
			_ = m.registerCachedSchemaNoLock(spec, specHash)
		}
		return false
	}
	catalog, err := listMCPToolCatalogContext(context.Background(), client)
	if err != nil {
		client.Close()
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonCatalogFailedV1,
			)
		}
		m.failures[spec.ID] = redactMCPDiagnosticText(err.Error(), spec)
		if mcpCatalogFailureAllowsSchemaHint(err) {
			_ = m.registerCachedSchemaNoLock(spec, specHash)
		}
		return false
	}
	tools := catalog.Tools
	sourceReadRequested := m.hostFundsServer != nil && m.hostFundsServer.sourceReadRequestedV1()
	if m.hostFundsFingerprint != "" && spec.ID == "analytix_funds" &&
		SpecFingerprint(spec) == m.hostFundsFingerprint &&
		!exactHostFundsRuntimeToolCatalogV1(
			tools,
			len(catalog.Quarantined),
			sourceReadRequested,
			m.accountFlowExecutor != nil,
		) {
		client.Close()
		m.revokeHostFundsSourceReadNoLock(
			domainplugincapability.FundsSourceReadStateFailedV1,
			domainplugincapability.FundsSourceReadReasonCatalogFailedV1,
		)
		m.failures[spec.ID] = "host funds MCP catalog does not match the exact evidence tool contract"
		return false
	}
	identityClient, identityOK := client.(mcpObservedIdentityClient)
	if !identityOK {
		client.Close()
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonHealthFailedV1,
			)
		}
		m.failures[spec.ID] = "MCP transport did not expose verified server identity"
		return false
	}
	observed := identityClient.ObservedServerIdentity()
	if !domainmcpprotocol.SupportedVersion(observed.ProtocolVersion) || strings.TrimSpace(observed.Name) == "" || strings.TrimSpace(observed.Version) == "" ||
		mcpidentity.VerifyObserved(spec, observed) != nil {
		client.Close()
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonHealthFailedV1,
			)
		}
		m.failures[spec.ID] = "MCP transport returned invalid verified server identity"
		return false
	}
	cachedTools, err := cacheableTools(tools, spec)
	if err != nil {
		client.Close()
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonCatalogFailedV1,
			)
		}
		m.failures[spec.ID] = "MCP catalog contains an invalid strict tool contract"
		return false
	}
	nextEpoch := m.connectionEpochs[spec.ID] + 1
	verifiedIdentity := verifiedMCPServerIdentity(spec.ID, observed, m.connectionInstanceID, nextEpoch)
	if verifiedIdentity == "" {
		client.Close()
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonHealthFailedV1,
			)
		}
		m.failures[spec.ID] = "MCP verified server identity is invalid"
		return false
	}
	sourceCatalogDigest := sourceProbeCatalogFingerprintWithIssues(tools, catalog.Quarantined)
	if spec.ID == "analytix_funds" && sourceReadRequested &&
		!m.activateHostFundsSourceReadNoLock(specHash, verifiedIdentity, sourceCatalogDigest, nextEpoch) {
		tools = withoutHostFundsCountToolV1(tools)
		cachedTools = withoutHostFundsCountToolV1(cachedTools)
	}
	if err := m.registerToolsNoLock(spec, tools); err != nil {
		client.Close()
		if spec.ID == "analytix_funds" {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateFailedV1,
				domainplugincapability.FundsSourceReadReasonCatalogFailedV1,
			)
		}
		m.failures[spec.ID] = "MCP catalog contains an ambiguous tool identity"
		return false
	}
	m.toolContractIssues[spec.ID] = cloneToolContractIssues(catalog.Quarantined)
	m.sourceCatalogs[spec.ID] = sourceCatalogDigest
	m.clients[spec.ID] = client
	m.connectionEpochs[spec.ID] = nextEpoch
	m.serverIdentities[spec.ID] = verifiedIdentity
	delete(m.failures, spec.ID)
	delete(m.cachedSchemas, spec.ID)
	_ = saveCachedSchema(m.cacheDir, spec.ID, CachedSchema{
		SpecHash:      specHash,
		Tools:         cachedTools,
		LastValidated: time.Now().UTC(),
	})
	m.refreshPromptResourceCatalogAsync(spec, client)
	return true
}

func withoutHostFundsCountToolV1(tools []ToolSpec) []ToolSpec {
	out := make([]ToolSpec, 0, len(tools))
	for _, tool := range tools {
		if tool.Name != hostFundsCountToolNameV1 {
			out = append(out, tool)
		}
	}
	return out
}

func (m *ProductionManager) refreshPromptResourceCatalogAsync(spec ServerSpec, client mcpTransportClient) {
	go func() {
		prompts, promptsErr := client.ListPrompts()
		resources, resourcesErr := client.ListResources()
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.clients[spec.ID] != client {
			return
		}
		m.prompts = filterMCPPromptsForOtherServers(m.prompts, spec.ID)
		m.resources = filterMCPResourcesForOtherServers(m.resources, spec.ID)
		if promptsErr == nil {
			m.registerPromptsNoLock(spec, prompts)
		}
		if resourcesErr == nil {
			m.registerResourcesNoLock(spec, resources)
		}
		m.refreshCatalogStateNoLock()
	}()
}

func (m *ProductionManager) registerCachedSchemaNoLock(spec ServerSpec, specHash string) bool {
	if mcpidentity.IsHighRiskServerID(spec.ID) {
		if err := mcpidentity.VerifyConfiguredProvenance(spec); err != nil {
			m.failures[spec.ID] = "MCP cached catalog provenance is invalid"
			return false
		}
	}
	cached, ok := loadCachedSchema(m.cacheDir, spec.ID, specHash)
	if !ok {
		return false
	}
	if err := m.registerToolsNoLock(spec, cached.Tools); err != nil {
		m.failures[spec.ID] = "MCP cached catalog contains an ambiguous tool identity"
		return false
	}
	m.cachedSchemas[spec.ID] = true
	m.toolContractIssues[spec.ID] = cloneToolContractIssues(cached.Quarantined)
	return true
}

func (m *ProductionManager) registerLazyConnectPlaceholderNoLock(spec ServerSpec) error {
	tool := ToolSpec{
		Name:         "connect",
		Description:  fmt.Sprintf("Connect MCP server %q and refresh its real tool catalog. Use this only when the server's real tools are not listed yet.", spec.ID),
		InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"toolName":{"type":"string"},"serverId":{"type":"string"},"executed":{"type":"boolean"},"code":{"type":"string"},"result":{"type":"string"},"toolNames":{"type":"array","items":{"type":"string"}}},"required":["toolName","serverId","executed","code","result","toolNames"],"additionalProperties":false}`),
		ReadOnlyHint: true,
	}
	bindings, err := m.prepareToolBindingsNoLock(spec, []ToolSpec{tool}, true)
	if err != nil {
		return err
	}
	m.installToolBindingsNoLock(bindings)
	return nil
}

func (m *ProductionManager) registerToolsNoLock(spec ServerSpec, tools []ToolSpec) error {
	bindings, err := m.prepareToolBindingsNoLock(spec, tools, false)
	if err != nil {
		return err
	}
	m.installToolBindingsNoLock(bindings)
	return nil
}

func (m *ProductionManager) prepareToolBindingsNoLock(spec ServerSpec, tools []ToolSpec, placeholder bool) (map[string]managedTool, error) {
	if _, err := CanonicalToolPrefixChecked(spec.ID); err != nil {
		return nil, err
	}
	bindings := make(map[string]managedTool, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, errors.New("MCP tool name is empty")
		}
		taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(tool.TaskSupport)
		if !taskSupportOK {
			return nil, errors.New("MCP tool has invalid task support")
		}
		tool.TaskSupport = taskSupport
		if !placeholder && taskSupport == domainmcp.ToolTaskSupportRequired {
			continue
		}
		if spec.ReadOnlyToolNames[tool.Name] {
			tool.ReadOnlyHint = true
		}
		rawName := tool.Name
		namespaced, err := CanonicalToolNameChecked(spec.ID, rawName)
		if err != nil {
			return nil, err
		}
		binding := managedTool{ServerID: spec.ID, Name: namespaced, RawName: rawName, Tool: tool, Placeholder: placeholder}
		if prior, exists := bindings[namespaced]; exists && (prior.ServerID != binding.ServerID || prior.RawName != binding.RawName) {
			return nil, errors.New("MCP tool catalog has a normalized-name collision")
		}
		if prior, exists := m.tools[namespaced]; exists && (prior.ServerID != binding.ServerID || prior.RawName != binding.RawName) {
			return nil, errors.New("MCP tool catalog conflicts with another server binding")
		}
		bindings[namespaced] = binding
	}
	return bindings, nil
}

func (m *ProductionManager) installToolBindingsNoLock(bindings map[string]managedTool) {
	for name, binding := range bindings {
		m.tools[name] = binding
	}
}

func (m *ProductionManager) registerPromptsNoLock(spec ServerSpec, prompts []PromptSpec) {
	for _, prompt := range prompts {
		prompt.Name = strings.TrimSpace(prompt.Name)
		if prompt.Name == "" {
			continue
		}
		prompt.ServerID = spec.ID
		prompt.Description = redactMCPCatalogText(prompt.Description)
		for index := range prompt.Arguments {
			prompt.Arguments[index].Name = strings.TrimSpace(prompt.Arguments[index].Name)
			prompt.Arguments[index].Description = redactMCPCatalogText(prompt.Arguments[index].Description)
		}
		m.prompts = append(m.prompts, prompt)
	}
	sort.SliceStable(m.prompts, func(i, j int) bool {
		if m.prompts[i].ServerID != m.prompts[j].ServerID {
			return m.prompts[i].ServerID < m.prompts[j].ServerID
		}
		return m.prompts[i].Name < m.prompts[j].Name
	})
}

func (m *ProductionManager) registerResourcesNoLock(spec ServerSpec, resources []ResourceSpec) {
	for _, resource := range resources {
		resource.URI = strings.TrimSpace(resource.URI)
		if resource.URI == "" {
			continue
		}
		resource.ServerID = spec.ID
		resource.URI = redactMCPCatalogText(resource.URI)
		resource.Name = redactMCPCatalogText(resource.Name)
		resource.Description = redactMCPCatalogText(resource.Description)
		resource.MimeType = redactMCPCatalogText(resource.MimeType)
		m.resources = append(m.resources, resource)
	}
	sort.SliceStable(m.resources, func(i, j int) bool {
		if m.resources[i].ServerID != m.resources[j].ServerID {
			return m.resources[i].ServerID < m.resources[j].ServerID
		}
		return m.resources[i].URI < m.resources[j].URI
	})
}

func (m *ProductionManager) removeServerToolsNoLock(serverID string) {
	for name, tool := range m.tools {
		if tool.ServerID == serverID {
			delete(m.tools, name)
		}
	}
	m.prompts = filterMCPPromptsForOtherServers(m.prompts, serverID)
	m.resources = filterMCPResourcesForOtherServers(m.resources, serverID)
	delete(m.cachedSchemas, serverID)
	delete(m.sourceCatalogs, serverID)
	m.clearSourceAuthorityForServerNoLock(serverID)
}

func filterMCPPromptsForOtherServers(prompts []PromptSpec, serverID string) []PromptSpec {
	if len(prompts) == 0 {
		return prompts
	}
	out := prompts[:0]
	for _, prompt := range prompts {
		if prompt.ServerID != serverID {
			out = append(out, prompt)
		}
	}
	return out
}

func filterMCPResourcesForOtherServers(resources []ResourceSpec, serverID string) []ResourceSpec {
	if len(resources) == 0 {
		return resources
	}
	out := resources[:0]
	for _, resource := range resources {
		if resource.ServerID != serverID {
			out = append(out, resource)
		}
	}
	return out
}

func (m *ProductionManager) Search(query string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(query) == "" {
		return nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	matches := []string{}
	for _, name := range m.sortedToolsNoLock() {
		managed := m.tools[name]
		if !m.managedToolCapabilityCurrentNoLock(name, managed) {
			continue
		}
		tool := managed.Tool
		if strings.Contains(strings.ToLower(name+" "+tool.Description), query) {
			matches = append(matches, name)
		}
	}
	return matches
}

func (m *ProductionManager) CallTool(toolName string, approved bool, arguments ...map[string]any) map[string]any {
	return m.CallToolContext(context.Background(), toolName, approved, arguments...)
}

func (m *ProductionManager) CallToolContext(ctx context.Context, toolName string, approved bool, arguments ...map[string]any) map[string]any {
	return m.callToolContext(ctx, toolName, approved, 0, "", "", nil, false, nil, nil, arguments...)
}

func (m *ProductionManager) CallToolBoundContext(ctx context.Context, toolName string, approved bool, expectedConnectionEpoch uint64, arguments ...map[string]any) map[string]any {
	return m.callToolContext(ctx, toolName, approved, expectedConnectionEpoch, "", "", nil, true, nil, nil, arguments...)
}

func (m *ProductionManager) CallToolSecurityBoundContext(ctx context.Context, toolName string, approved bool, envelope domainmcp.HostContextEnvelope, arguments ...map[string]any) map[string]any {
	securityContext, grant, err := envelope.Authority()
	if err == nil && isHighRiskCanonicalToolName(toolName) {
		err = domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext)
	} else if err == nil {
		err = domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext)
	}
	callArguments := map[string]any{}
	if len(arguments) > 0 && arguments[0] != nil {
		callArguments = arguments[0]
	}
	argumentBytes, argumentErr := json.Marshal(callArguments)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	expectedServerIdentity, identityOK := m.ToolServerIdentity(toolName, securityContext)
	if err == nil && grant.ApprovalState == "pending" {
		return map[string]any{"toolName": toolName, "executed": false, "code": "mcp_execution_approval_pending", "error": "MCP execution grant has not been approved"}
	}
	if err != nil || argumentErr != nil || domainmcp.ProviderArgumentsContainHostAuthority(callArguments) ||
		(grant.ApprovalState != "not_required" && grant.ApprovalState != "approved") ||
		grant.ToolName != toolName || grant.ArgsHash != domainsecurity.CanonicalJSONHash(argumentBytes) ||
		expiresErr != nil || !time.Now().UTC().Before(expiresAt) || !identityOK || grant.ServerIdentity != expectedServerIdentity {
		return map[string]any{"toolName": toolName, "executed": false, "code": "mcp_host_context_invalid", "error": "MCP host context does not match the current granted tool arguments"}
	}
	var transportHostContext *domainmcp.HostContextEnvelope
	if isHighRiskCanonicalToolName(toolName) {
		transportHostContext = &envelope
	}
	return m.callToolContext(ctx, toolName, approved, grant.ConnectionEpoch, grant.SchemaHash, grant.ServerIdentity, &grant.ReadOnly, true, &securityContext, transportHostContext, callArguments)
}

// ValidateCurrentAccountFlowOuterGrant atomically rechecks the exact durable
// MCP grant against the live host-owned account-flow catalog and source probe.
// The caller already owns the high-risk source execution lease.
func (m *ProductionManager) ValidateCurrentAccountFlowOuterGrant(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
) error {
	const toolName = "mcp__analytix_funds__analyze_account_flows"
	if m == nil || ctx == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil ||
		grant.ToolName != toolName || grant.ConnectionEpoch == 0 || !grant.ReadOnly ||
		(grant.ApprovalState != "not_required" && grant.ApprovalState != "approved") {
		return errors.New("current account-flow outer grant is unavailable")
	}
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	identity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(grant.ServerIdentity)
	if expiresErr != nil || !time.Now().UTC().Before(expiresAt) || identityErr != nil ||
		!domainsecurity.VerifiedMCPServerIdentityCanAuthorizeFacts(identity) ||
		identity.ServerID != "analytix_funds" || identity.ConnectionEpoch != grant.ConnectionEpoch {
		return errors.New("current account-flow outer grant is unavailable")
	}
	m.mu.Lock()
	tool, toolOK := m.tools[toolName]
	spec, specOK := m.specByIDNoLock("analytix_funds")
	client := m.clients["analytix_funds"]
	hostClient, hostClientOK := client.(*hostFundsTransportClientV1)
	fingerprint := m.hostFundsFingerprint
	current := toolOK && !tool.Placeholder && tool.ServerID == "analytix_funds" &&
		tool.RawName == hostFundsAccountFlowToolNameV1 &&
		exactHostFundsAccountFlowToolV1(tool.Tool, hostFundsAccountFlowAnalysisOutputSchemaV1) &&
		specOK && client != nil && hostClientOK && m.accountFlowExecutor != nil &&
		m.hostFundsServer != nil && m.hostFundsServer.matches(spec, fingerprint) &&
		fingerprint != "" && SpecFingerprint(spec) == fingerprint &&
		spec.IdentitySource == mcpidentity.HostInstalledGenerationSourceV1 &&
		spec.ReadOnlyToolNames[hostFundsAccountFlowToolNameV1] &&
		m.connectionEpochs["analytix_funds"] == grant.ConnectionEpoch &&
		m.serverIdentities["analytix_funds"] == grant.ServerIdentity &&
		grant.SchemaHash == managedToolSchemaHash(toolName, tool) &&
		m.sourceProbeMatchesContextNoLock("analytix_funds", securityContext)
	m.mu.Unlock()
	if !current || !hostClient.accountFlowExecutionReadyV1() ||
		mcpidentity.VerifyConfiguredProvenance(spec) != nil {
		return errors.New("current account-flow outer grant is unavailable")
	}
	return nil
}

func (m *ProductionManager) ConsumeHostFundsAccountFlowEvidenceV1(
	result domainmcp.LosslessToolResult,
	consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
	consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
) error {
	if m == nil {
		return errHostFundsRequestInvalidV1
	}
	return ConsumeHostFundsAccountFlowEvidenceV1(result, consumeSummary, consumeRow)
}

func (m *ProductionManager) DiscardHostFundsAccountFlowEvidenceV1(result domainmcp.LosslessToolResult) {
	if m != nil {
		DiscardHostFundsAccountFlowEvidenceV1(result)
	}
}

func (m *ProductionManager) HostFundsAccountFlowProviderSemanticV1(
	result domainmcp.LosslessToolResult,
) (domainnative.AccountFlowProviderSemanticResultV1, bool) {
	if m == nil {
		return domainnative.AccountFlowProviderSemanticResultV1{}, false
	}
	return HostFundsAccountFlowProviderSemanticV1(result)
}

func (m *ProductionManager) callToolContext(ctx context.Context, toolName string, approved bool, expectedConnectionEpoch uint64, expectedSchemaHash string, expectedServerIdentity string, expectedReadOnly *bool, bound bool, securityContext *domainsecurity.TurnSecurityContext, hostContext *domainmcp.HostContextEnvelope, arguments ...map[string]any) map[string]any {
	if ctx == nil {
		ctx = context.Background()
	}
	highRiskName := isHighRiskCanonicalToolName(toolName)
	exclusiveSourceLease := highRiskName || !bound
	if exclusiveSourceLease {
		m.sourceExecutionMu.Lock()
	} else {
		m.sourceExecutionMu.RLock()
	}
	defer func() {
		if exclusiveSourceLease {
			m.sourceExecutionMu.Unlock()
			return
		}
		m.sourceExecutionMu.RUnlock()
	}()
	promoteSourceLease := func() {
		if exclusiveSourceLease {
			return
		}
		m.sourceExecutionMu.RUnlock()
		m.sourceExecutionMu.Lock()
		exclusiveSourceLease = true
	}
	m.mu.Lock()
	tool, ok := m.tools[toolName]
	if !ok {
		m.mu.Unlock()
		return map[string]any{"toolName": toolName, "executed": false, "code": "mcp_tool_not_found"}
	}
	if !m.managedToolCapabilityCurrentNoLock(toolName, tool) {
		m.mu.Unlock()
		return map[string]any{
			"toolName": toolName, "serverId": tool.ServerID, "executed": false,
			"code": "mcp_capability_grant_unavailable", "error": "MCP capability grant is not current",
		}
	}
	parsedServerID, _, parsed := domainmcpname.Parse(toolName)
	actualName, bindingErr := CanonicalToolNameChecked(tool.ServerID, tool.RawName)
	if !parsed || bindingErr != nil || actualName != toolName || !strings.HasPrefix(actualName, "mcp__"+parsedServerID+"__") {
		m.mu.Unlock()
		return map[string]any{"toolName": toolName, "executed": false, "code": "mcp_tool_binding_invalid", "error": "MCP tool binding is ambiguous"}
	}
	if expectedSchemaHash != "" && managedToolSchemaHash(toolName, tool) != expectedSchemaHash {
		m.mu.Unlock()
		return map[string]any{"toolName": toolName, "executed": false, "code": "mcp_execution_grant_schema_mismatch", "error": "MCP tool schema changed after execution authority was issued"}
	}
	taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(tool.Tool.TaskSupport)
	if !taskSupportOK || taskSupport == domainmcp.ToolTaskSupportRequired {
		m.mu.Unlock()
		return map[string]any{"toolName": toolName, "serverId": tool.ServerID, "executed": false, "code": "mcp_task_required_unsupported", "error": "MCP task-required tools cannot execute until the runtime implements the Tasks capability"}
	}
	if !approved {
		m.mu.Unlock()
		return map[string]any{"toolName": toolName, "executed": false, "code": "approval_denied"}
	}
	highRiskSource := mcpidentity.IsHighRiskServerID(tool.ServerID)
	if highRiskSource && (!highRiskName || !bound || securityContext == nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(*securityContext) != nil || !m.sourceProbeMatchesContextNoLock(tool.ServerID, *securityContext)) {
		m.mu.Unlock()
		return highRiskSourceAuthorityMismatch(toolName, tool.ServerID)
	}
	if bound && (expectedConnectionEpoch == 0 || m.connectionEpochs[tool.ServerID] != expectedConnectionEpoch) {
		actual := m.connectionEpochs[tool.ServerID]
		m.mu.Unlock()
		return mcpConnectionEpochMismatch(toolName, tool.ServerID, expectedConnectionEpoch, actual)
	}
	if bound && expectedServerIdentity != "" && m.serverIdentities[tool.ServerID] != expectedServerIdentity {
		m.mu.Unlock()
		return map[string]any{
			"toolName": toolName, "serverId": tool.ServerID, "executed": false,
			"code": "mcp_execution_grant_server_mismatch", "error": "MCP server identity changed after execution authority was issued",
		}
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return mcpContextCanceledResult(toolName, tool.ServerID, err)
	}
	spec, specOK := m.specByIDNoLock(tool.ServerID)
	if !specOK || !mcpServerSpecAuthorizedForSecurityContext(spec, securityContext) {
		m.mu.Unlock()
		return mcpWorkspaceTrustMismatch(toolName, tool.ServerID)
	}
	if tool.Placeholder {
		if bound {
			actual := m.connectionEpochs[tool.ServerID]
			m.mu.Unlock()
			return mcpConnectionEpochMismatch(toolName, tool.ServerID, expectedConnectionEpoch, actual)
		}
		serverID := tool.ServerID
		spec, specOK := m.specByIDNoLock(serverID)
		if !specOK {
			m.mu.Unlock()
			return map[string]any{"toolName": toolName, "serverId": serverID, "executed": false, "code": "mcp_not_connected", "error": "MCP server configuration is no longer available"}
		}
		specHash := m.specFingerprints[spec.ID]
		if specHash == "" {
			specHash = SpecFingerprint(spec)
		}
		if !m.connectSpecNoLock(spec, specHash) {
			failure := m.failures[serverID]
			m.refreshCatalogStateNoLock()
			m.mu.Unlock()
			return map[string]any{"toolName": toolName, "serverId": serverID, "executed": false, "code": "mcp_not_connected", "error": failure}
		}
		m.calls++
		m.refreshCatalogStateNoLock()
		toolNames := m.serverToolNamesNoLock(serverID)
		m.mu.Unlock()
		return map[string]any{
			"toolName":  toolName,
			"serverId":  serverID,
			"executed":  true,
			"code":      "mcp_lazy_connected",
			"result":    fmt.Sprintf("MCP server %q connected. Its real tools are available on the next provider turn.", serverID),
			"toolNames": toolNames,
		}
	}
	client := m.clients[tool.ServerID]
	if client == nil {
		if bound {
			actual := m.connectionEpochs[tool.ServerID]
			m.mu.Unlock()
			return mcpConnectionEpochMismatch(toolName, tool.ServerID, expectedConnectionEpoch, actual)
		}
		serverID := tool.ServerID
		if spec, specOK := m.specByIDNoLock(tool.ServerID); specOK {
			specHash := m.specFingerprints[spec.ID]
			if specHash == "" {
				specHash = SpecFingerprint(spec)
			}
			if m.connectSpecNoLock(spec, specHash) {
				m.refreshCatalogStateNoLock()
				tool, ok = m.tools[toolName]
				client = m.clients[serverID]
			}
		}
		if !ok && client != nil {
			m.mu.Unlock()
			return map[string]any{
				"toolName": toolName,
				"serverId": serverID,
				"executed": false,
				"code":     "mcp_cached_schema_stale",
				"error":    fmt.Sprintf("MCP server %q did not expose cached tool %q after reconnect", serverID, toolName),
			}
		}
		if !ok || client == nil {
			failure := m.failures[serverID]
			m.mu.Unlock()
			return map[string]any{"toolName": toolName, "serverId": serverID, "executed": false, "code": "mcp_not_connected", "error": failure}
		}
	}
	serverID := tool.ServerID
	rawName := tool.RawName
	readOnlyHint := tool.Tool.ReadOnlyHint
	outputSchema := append(json.RawMessage(nil), tool.Tool.OutputSchema...)
	spec, specOK = m.specByIDNoLock(serverID)
	hostReadOnly := specOK && spec.ReadOnlyToolNames[rawName]
	if expectedReadOnly != nil && (!specOK || hostReadOnly != *expectedReadOnly) {
		m.mu.Unlock()
		return map[string]any{
			"toolName": toolName, "serverId": serverID, "executed": false,
			"code": "mcp_execution_grant_policy_mismatch", "error": "MCP host read-only policy changed after execution authority was issued",
		}
	}
	m.mu.Unlock()
	if highRiskSource && mcpidentity.VerifyConfiguredProvenance(spec) != nil {
		return map[string]any{"toolName": toolName, "serverId": serverID, "executed": false, "code": "mcp_source_integrity_mismatch", "error": "High-risk MCP source integrity changed after connection"}
	}
	callArguments := map[string]any{}
	if len(arguments) > 0 && arguments[0] != nil {
		callArguments = arguments[0]
	}
	var accountFlowIntent domainnative.AnalyzeAccountFlowsProviderIntentV1
	if highRiskSource && rawName == hostFundsAccountFlowToolNameV1 {
		var decodeErr error
		accountFlowIntent, decodeErr = decodeHostFundsAccountFlowIntentV1(callArguments)
		if decodeErr != nil {
			return map[string]any{
				"toolName": toolName, "serverId": serverID, "executed": false,
				"code": "mcp_arguments_invalid", "error": "MCP tool arguments do not match the advertised account-flow contract",
			}
		}
	}
	var result any
	hostPrivateHandedOff := false
	defer func() {
		if !hostPrivateHandedOff {
			discardHostFundsSealedResultV1(result)
		}
	}()
	var err error
	if highRiskSource {
		binding, bindingOK := m.registeredSourceBindingForContext(serverID, *securityContext)
		if !bindingOK || serverID != "analytix_funds" {
			return highRiskSourceAuthorityMismatch(toolName, serverID)
		}
		var authorityErr error
		switch rawName {
		case hostFundsCountToolNameV1:
			authorityErr = m.withCurrentHostEvidenceUnderLease(
				ctx, serverID, *securityContext, binding,
				func(probe domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
					selection, selectionErr := capability.DatasetSelection()
					if selectionErr != nil {
						return selectionErr
					}
					projection, projectionErr := fundsCountProjectionForSelectionV2(*securityContext, selection)
					if projectionErr != nil {
						return projectionErr
					}
					return capability.UseExact(*securityContext, probe, selection, func(leaseContext context.Context) error {
						result, err = callFundsCountToolWithProjectionV2(leaseContext, client, callArguments, projection)
						return nil
					})
				},
			)
		case hostFundsAccountFlowToolNameV1:
			if hostContext == nil {
				return highRiskSourceAuthorityMismatch(toolName, serverID)
			}
			currentContext, currentGrant, hostContextErr := hostContext.Authority()
			if hostContextErr != nil || !reflect.DeepEqual(currentContext, *securityContext) ||
				currentGrant.ArgsHash != domainnative.AnalyzeAccountFlowsProviderIntentHashV1(accountFlowIntent) ||
				m.ValidateCurrentAccountFlowOuterGrant(ctx, currentContext, currentGrant) != nil {
				return highRiskSourceAuthorityMismatch(toolName, serverID)
			}
			hostClient, hostClientOK := client.(*hostFundsTransportClientV1)
			if !hostClientOK || !hostClient.accountFlowExecutionReadyV1() || m.accountFlowExecutor == nil {
				return highRiskSourceAuthorityMismatch(toolName, serverID)
			}
			sealed, executeErr := executeHostFundsAccountFlowV1(
				ctx, hostClient, m.accountFlowExecutor, currentContext, currentGrant, accountFlowIntent,
			)
			if executeErr != nil {
				err = errHostFundsExecutionFailedV1
				break
			}
			if m.ValidateCurrentAccountFlowOuterGrant(ctx, currentContext, currentGrant) != nil {
				sealed.discard()
				return highRiskSourceAuthorityMismatch(toolName, serverID)
			}
			result = sealed
		default:
			return highRiskSourceAuthorityMismatch(toolName, serverID)
		}
		if authorityErr != nil {
			return highRiskSourceAuthorityMismatch(toolName, serverID)
		}
	} else {
		result, err = callMCPTransportTool(ctx, client, rawName, callArguments, hostContext)
	}
	if err != nil {
		// Low-risk bound calls hold a shared catalog lease while transport is in
		// flight. Any fatal authority mutation is performed only after promoting
		// to the same exclusive lease used by refresh/reconnect/disconnect.
		promoteSourceLease()
		m.revokeMCPAuthorityAfterCallFailure(serverID, client, highRiskSource, err)
		if ctxErr := ctx.Err(); ctxErr != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if ctxErr != nil {
				err = ctxErr
			}
			return mcpContextCanceledResult(toolName, serverID, err)
		}
		if rpcResult, ok := mcpJSONRPCFailureResult(toolName, serverID, err); ok {
			return rpcResult
		}
		if !bound && hostReadOnly && shouldRetryMCPCall(err) {
			retryTool, retryClient, retryOK, retryCode, retryError := m.reconnectToolForRetry(toolName, serverID, rawName)
			if retryOK && retryClient != nil {
				retryResult, retryErr := callMCPTransportTool(ctx, retryClient, retryTool.RawName, callArguments, nil)
				if retryErr == nil {
					m.mu.Lock()
					m.calls++
					m.mu.Unlock()
					return mcpExecutedToolResult(toolName, serverID, retryResult, retryTool.Tool.OutputSchema, retryTool.Tool.ReadOnlyHint, true)
				}
				if ctxErr := ctx.Err(); ctxErr != nil || errors.Is(retryErr, context.Canceled) || errors.Is(retryErr, context.DeadlineExceeded) {
					m.revokeMCPAuthorityAfterCallFailure(serverID, retryClient, highRiskSource, retryErr)
					if ctxErr != nil {
						retryErr = ctxErr
					}
					return mcpContextCanceledResult(toolName, serverID, retryErr)
				}
				if rpcResult, ok := mcpJSONRPCFailureResult(toolName, serverID, retryErr); ok {
					m.revokeMCPAuthorityAfterCallFailure(serverID, retryClient, highRiskSource, retryErr)
					rpcResult["retried"] = true
					return rpcResult
				}
				m.revokeMCPAuthorityAfterCallFailure(serverID, retryClient, highRiskSource, retryErr)
				return map[string]any{"toolName": toolName, "serverId": serverID, "executed": false, "transportStatus": "failure", "semanticStatus": "failure", "isError": true, "code": "mcp_call_failed", "error": redactMCPDiagnosticText(retryErr.Error(), spec), "retried": true}
			}
			if retryCode != "" {
				return map[string]any{"toolName": toolName, "serverId": serverID, "executed": false, "code": retryCode, "error": retryError, "retried": true}
			}
		}
		return map[string]any{"toolName": toolName, "serverId": serverID, "executed": false, "transportStatus": "failure", "semanticStatus": "failure", "isError": true, "code": "mcp_call_failed", "error": redactMCPDiagnosticText(err.Error(), spec)}
	}
	m.mu.Lock()
	if bound && (m.clients[serverID] != client || m.connectionEpochs[serverID] != expectedConnectionEpoch) {
		actual := m.connectionEpochs[serverID]
		m.mu.Unlock()
		return mcpConnectionEpochMismatch(toolName, serverID, expectedConnectionEpoch, actual)
	}
	if bound && expectedServerIdentity != "" && m.serverIdentities[serverID] != expectedServerIdentity {
		m.mu.Unlock()
		return map[string]any{
			"toolName": toolName, "serverId": serverID, "executed": false,
			"code": "mcp_execution_grant_server_mismatch", "error": "MCP server identity changed during execution",
		}
	}
	currentSpec, currentSpecOK := m.specByIDNoLock(serverID)
	if !currentSpecOK || !mcpServerSpecAuthorizedForSecurityContext(currentSpec, securityContext) {
		m.mu.Unlock()
		return mcpWorkspaceTrustMismatch(toolName, serverID)
	}
	currentReadOnly := currentSpecOK && currentSpec.ReadOnlyToolNames[rawName]
	if expectedReadOnly != nil && (!currentSpecOK || currentReadOnly != *expectedReadOnly) {
		m.mu.Unlock()
		return map[string]any{
			"toolName": toolName, "serverId": serverID, "executed": false,
			"code": "mcp_execution_grant_policy_mismatch", "error": "MCP host read-only policy changed during execution",
		}
	}
	if highRiskSource && (securityContext == nil || !m.sourceProbeMatchesContextNoLock(serverID, *securityContext)) {
		m.mu.Unlock()
		return highRiskSourceAuthorityMismatch(toolName, serverID)
	}
	if rawName == hostFundsCountToolNameV1 && !m.managedToolCapabilityCurrentNoLock(toolName, tool) {
		m.mu.Unlock()
		return map[string]any{
			"toolName": toolName, "serverId": serverID, "executed": false,
			"code": "mcp_capability_grant_unavailable", "error": "MCP capability grant changed during execution",
		}
	}
	m.calls++
	m.mu.Unlock()
	output := mcpExecutedToolResult(toolName, serverID, result, outputSchema, readOnlyHint, false)
	hostPrivateHandedOff = true
	return output
}

func managedToolSchemaHash(toolName string, tool managedTool) string {
	description := strings.TrimSpace(tool.Tool.Description)
	if description == "" {
		description = "Tool from a configured MCP server."
	}
	taskSupport, ok := domainmcp.NormalizeToolTaskSupport(tool.Tool.TaskSupport)
	if !ok {
		taskSupport = domainmcp.ToolTaskSupport("invalid")
	}
	return toolcatalogapp.ToolSchemaHash([]domainmodel.ToolSchema{{
		Name: toolName, Description: description, Parameters: tool.Tool.InputSchema, OutputSchema: tool.Tool.OutputSchema,
		Source: "mcp", TaskSupport: string(taskSupport),
	}})
}

func isHighRiskCanonicalToolName(toolName string) bool {
	serverID, _, ok := domainmcpname.Parse(toolName)
	return ok && mcpidentity.IsHighRiskServerID(serverID)
}

func highRiskSourceAuthorityMismatch(toolName string, serverID string) map[string]any {
	return map[string]any{
		"toolName": toolName,
		"serverId": serverID,
		"executed": false,
		"code":     "mcp_source_probe_mismatch",
		"error":    "High-risk MCP execution requires a verified probe bound to the frozen case context",
	}
}

func mcpWorkspaceTrustMismatch(toolName string, serverID string) map[string]any {
	return map[string]any{
		"toolName": toolName,
		"serverId": serverID,
		"executed": false,
		"code":     "mcp_workspace_trust_mismatch",
		"error":    "MCP server is not authorized for the frozen turn workspace",
	}
}

func mcpConnectionEpochMismatch(toolName string, serverID string, expected uint64, actual uint64) map[string]any {
	return map[string]any{
		"toolName":                toolName,
		"serverId":                serverID,
		"executed":                false,
		"code":                    "mcp_connection_epoch_mismatch",
		"expectedConnectionEpoch": expected,
		"actualConnectionEpoch":   actual,
		"error":                   "MCP connection changed after execution authority was issued",
	}
}

func mcpExecutedToolResult(toolName string, serverID string, result any, outputSchema json.RawMessage, readOnlyHint bool, retried bool) map[string]any {
	var hostPrivate *hostFundsAccountFlowEvidenceCarrierV1
	if unsealed, ok := result.(domainmcp.LosslessToolResult); ok && unsealed.HostPrivate != nil {
		if injected, carrierOK := unsealed.HostPrivate.(*hostFundsAccountFlowEvidenceCarrierV1); carrierOK {
			injected.discard()
		}
		unsealed.HostPrivate = nil
		result = unsealed
	}
	if sealed, ok := result.(hostFundsAccountFlowSealedResultV1); ok {
		if toolName != CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1) ||
			serverID != "analytix_funds" || !sealed.valid() {
			sealed.discard()
			result = domainmcp.LosslessToolResult{}
		} else {
			result = sealed.lossless
			hostPrivate = sealed.carrier
		}
	}
	lossless, hasLossless := result.(domainmcp.LosslessToolResult)
	recomputed := mcpprotocol.ExtractLosslessToolResult(lossless.RawResult)
	projectionMatchesRaw := hasLossless && domainmcp.ValidLosslessToolResult(recomputed) && reflect.DeepEqual(lossless.Value, recomputed.Value)
	observation, validationErr := appmcp.InspectToolResultOutput(recomputed, outputSchema)
	if !projectionMatchesRaw || validationErr != nil {
		if hostPrivate != nil {
			hostPrivate.discard()
		}
		output := map[string]any{
			"toolName": toolName, "serverId": serverID, "executed": true,
			"transportStatus": "success", "semanticStatus": "failure", "isError": true,
			"code": "mcp_output_schema_invalid", "error": "MCP tool output was rejected by the host output schema",
			"readOnlyHint": readOnlyHint,
		}
		if retried {
			output["retried"] = true
		}
		if domainmcp.ValidLosslessToolResult(recomputed) {
			observation.SemanticStatus = "failure"
			observation.IsError = true
			if observation.Blocker == "" {
				observation.Blocker = "mcp_output_schema_invalid"
			}
			if observation.PartialCoverage == nil {
				observation.PartialCoverage = map[string]any{}
			}
			if observation.CandidateEvidenceReceipts == nil {
				observation.CandidateEvidenceReceipts = []map[string]any{}
			}
			if observation.UntrustedMeta == nil {
				observation.UntrustedMeta = map[string]any{}
			}
			observation.RawSHA256 = recomputed.RawSHA256
			recomputed.Observation = &observation
			output[domainmcp.HostRawToolResultKey] = recomputed
		}
		return output
	}
	lossless = recomputed
	lossless.Observation = &observation
	if hostPrivate != nil {
		if !hostPrivate.validFor(lossless.RawSHA256) {
			hostPrivate.discard()
			return map[string]any{
				"toolName": toolName, "serverId": serverID, "executed": true,
				"transportStatus": "success", "semanticStatus": "failure", "isError": true,
				"code": "mcp_output_schema_invalid", "error": "MCP tool output was rejected by the host output schema",
				"readOnlyHint": readOnlyHint,
			}
		}
		lossless.HostPrivate = hostPrivate
	}
	result = recomputed.Value
	output := map[string]any{
		"toolName": toolName, "serverId": serverID, "executed": true,
		"transportStatus": "success", "semanticStatus": observation.SemanticStatus,
		"result": result, "readOnlyHint": readOnlyHint,
	}
	if domainmcp.ValidLosslessToolResult(lossless) {
		output[domainmcp.HostRawToolResultKey] = lossless
	}
	if retried {
		output["retried"] = true
	}
	if observation.Blocker != "" {
		output["blocker"] = observation.Blocker
	}
	if len(observation.PartialCoverage) > 0 {
		output["partialCoverage"] = observation.PartialCoverage
	}
	if observation.IsError {
		output["isError"] = true
		output["code"] = "mcp_semantic_failure"
	}
	return output
}

func mcpJSONRPCFailureResult(toolName string, serverID string, err error) (map[string]any, bool) {
	var rpcError *domainmcp.JSONRPCError
	if !errors.As(err, &rpcError) || rpcError == nil {
		return nil, false
	}
	class := rpcError.Class()
	return map[string]any{
		"toolName": toolName, "serverId": serverID, "executed": false,
		"transportStatus": "success", "semanticStatus": "failure", "isError": true,
		"code":     "mcp_jsonrpc_" + class,
		"error":    "MCP JSON-RPC request was rejected (" + class + ")",
		"rpcError": domainmcp.JSONRPCDiagnosticRecord(rpcError),
	}, true
}

func callMCPTransportTool(ctx context.Context, client mcpTransportClient, name string, arguments map[string]any, hostContext *domainmcp.HostContextEnvelope) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hostContext != nil {
		hostClient, ok := client.(mcpHostContextTransportClient)
		if !ok || domainmcp.ValidateHostContextEnvelope(*hostContext) != nil {
			return nil, errors.New("MCP transport cannot carry verified host context")
		}
		return hostClient.CallToolWithHostContext(ctx, name, arguments, *hostContext)
	}
	if contextClient, ok := client.(mcpContextTransportClient); ok {
		return contextClient.CallToolContext(ctx, name, arguments)
	}
	return client.CallTool(name, arguments)
}

func mcpContextCanceledResult(toolName string, serverID string, err error) map[string]any {
	code := "mcp_call_cancelled"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "mcp_call_timeout"
	}
	message := "MCP tool call cancelled"
	if err != nil {
		message = err.Error()
	}
	return map[string]any{
		"toolName": toolName,
		"serverId": serverID,
		"executed": false,
		"code":     code,
		"error":    message,
	}
}

func (m *ProductionManager) reconnectToolForRetry(toolName string, serverID string, rawName string) (managedTool, mcpTransportClient, bool, string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	spec, ok := m.specByIDNoLock(serverID)
	if !ok {
		return managedTool{}, nil, false, "mcp_not_connected", "MCP server configuration is no longer available"
	}
	currentTool, toolOK := m.tools[toolName]
	if !toolOK || currentTool.ServerID != serverID || currentTool.RawName != rawName || !spec.ReadOnlyToolNames[rawName] {
		return managedTool{}, nil, false, "mcp_retry_authority_changed", "MCP retry authority is no longer valid"
	}
	if current := m.clients[serverID]; current != nil {
		current.Close()
		delete(m.clients, serverID)
	}
	specHash := m.specFingerprints[spec.ID]
	if specHash == "" {
		specHash = SpecFingerprint(spec)
	}
	if !m.connectSpecNoLock(spec, specHash) {
		return managedTool{}, nil, false, "mcp_not_connected", m.failures[serverID]
	}
	m.refreshCatalogStateNoLock()
	tool, ok := m.tools[toolName]
	client := m.clients[serverID]
	if !ok || client == nil || tool.ServerID != serverID || tool.RawName != rawName || !spec.ReadOnlyToolNames[rawName] {
		return managedTool{}, nil, false, "mcp_cached_schema_stale", fmt.Sprintf("MCP server %q did not expose tool %q after reconnect", serverID, toolName)
	}
	return tool, client, true, "", ""
}

func (m *ProductionManager) serverToolNamesNoLock(serverID string) []string {
	toolNames := []string{}
	for _, name := range m.sortedToolsNoLock() {
		if m.tools[name].ServerID == serverID {
			toolNames = append(toolNames, name)
		}
	}
	return toolNames
}

func (m *ProductionManager) serverCatalogFingerprintNoLock(serverID string) string {
	tools := make(map[string]managedTool)
	for name, tool := range m.tools {
		if tool.ServerID == serverID {
			tools[name] = tool
		}
	}
	issues := map[string][]mcpprotocol.ToolContractIssue{}
	if serverIssues := m.toolContractIssues[serverID]; len(serverIssues) > 0 {
		issues[serverID] = serverIssues
	}
	return catalogFingerprintForManagedToolsWithIssues(tools, issues)
}

func (m *ProductionManager) toolContractIssueDiagnosticsNoLock(serverID string) []any {
	issues := cloneToolContractIssues(m.toolContractIssues[serverID])
	out := make([]any, 0, len(issues))
	for _, issue := range issues {
		out = append(out, map[string]any{
			"toolName": redactMCPCatalogText(issue.Name),
			"code":     string(issue.Code),
		})
	}
	return out
}

func shouldRetryMCPCall(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var typed interface{ MCPTransportRetryable() bool }
	return errors.As(err, &typed) && typed.MCPTransportRetryable()
}

func mcpTransportInvalidatesIdentity(err error) bool {
	var typed interface{ MCPTransportInvalidatesIdentity() bool }
	return errors.As(err, &typed) && typed.MCPTransportInvalidatesIdentity()
}

func mcpTransportUnavailable(err error) bool {
	var typed interface{ MCPTransportUnavailable() bool }
	return errors.As(err, &typed) && typed.MCPTransportUnavailable()
}

func mcpCatalogFailureAllowsSchemaHint(err error) bool {
	if err == nil || mcpprotocol.IsToolCatalogContractError(err) {
		return false
	}
	return mcpTransportUnavailable(err)
}

func (m *ProductionManager) revokeMCPAuthorityAfterCallFailure(serverID string, client mcpTransportClient, highRisk bool, callErr error) {
	if m == nil || strings.TrimSpace(serverID) == "" || client == nil || callErr == nil {
		return
	}
	invalidateIdentity := mcpTransportInvalidatesIdentity(callErr) || mcpprotocol.IsToolCatalogContractError(callErr)
	if !highRisk && !invalidateIdentity {
		return
	}
	closeClient := false
	m.mu.Lock()
	if m.clients[serverID] == client {
		if highRisk {
			m.revokeHostFundsSourceReadNoLock(
				domainplugincapability.FundsSourceReadStateRevokedV1,
				domainplugincapability.FundsSourceReadReasonAuthorityRevokedV1,
			)
			m.clearSourceAuthorityForServerNoLock(serverID)
			delete(m.sourceCatalogs, serverID)
		}
		if invalidateIdentity {
			m.removeServerToolsNoLock(serverID)
			delete(m.toolContractIssues, serverID)
			delete(m.clients, serverID)
			delete(m.serverIdentities, serverID)
			if m.connectionEpochs[serverID] < uint64(1<<53-1) {
				m.connectionEpochs[serverID]++
			} else {
				m.connectionEpochs[serverID] = 0
			}
			if mcpprotocol.IsToolCatalogContractError(callErr) {
				m.failures[serverID] = "MCP catalog contract invalidated the verified connection authority"
			} else {
				m.failures[serverID] = "MCP transport identity was revoked after a fatal connection failure"
			}
			m.refreshCatalogStateNoLock()
			closeClient = true
		}
	}
	m.mu.Unlock()
	if closeClient {
		client.Close()
	}
}

func (m *ProductionManager) revokeMCPAuthorityAfterNativeFailure(serverID string, client mcpTransportClient, callErr error) {
	if callErr == nil {
		callErr = errors.New("MCP native authority validation failed")
	}
	m.revokeMCPAuthorityAfterCallFailure(serverID, client, true, callErr)
}

func (m *ProductionManager) specByIDNoLock(id string) (ServerSpec, bool) {
	for _, spec := range m.normalizeProductionSpecsNoLock() {
		if spec.ID == id {
			return spec, true
		}
	}
	return ServerSpec{}, false
}

func (m *ProductionManager) normalizeProductionSpecsNoLock() []ServerSpec {
	if m == nil {
		return nil
	}
	return normalizeProductionSpecsForAuthority(m.specs, m.allowCaseFactSource && m.datasetAuthority != nil)
}

func (m *ProductionManager) RefreshCatalog() map[string]any {
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	before := m.catalogHash
	m.connectNoLock(true)
	m.refreshes++
	m.catalogDrift = before != "" && before != m.catalogHash
	return m.diagnosticsNoLock()
}

func (m *ProductionManager) RestartReconnect() map[string]any {
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restarts++
	m.connectNoLock(true)
	m.refreshes++
	return m.diagnosticsNoLock()
}

func (m *ProductionManager) Disconnect() {
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokeHostFundsSourceReadNoLock(
		domainplugincapability.FundsSourceReadStateStoppedV1,
		domainplugincapability.FundsSourceReadReasonDisconnectedV1,
	)
	for _, client := range m.clients {
		client.Close()
	}
	m.clients = map[string]mcpTransportClient{}
	m.serverIdentities = map[string]string{}
	m.sourceCatalogs = map[string]string{}
	m.sourceProbes = map[string]domainsecurity.VerifiedSourceProbe{}
	m.sourceAdmissions = map[string]sourceEvidenceAdmissionV2{}
}

func (m *ProductionManager) ProbeCaseSource(ctx context.Context, input sourceprobeport.Input) (domainsecurity.VerifiedSourceProbe, error) {
	if m == nil {
		return domainsecurity.VerifiedSourceProbe{}, errors.New(domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable)
	}
	serverID := strings.TrimSpace(input.ServerID)
	if m.datasetAuthority == nil || validateSourceProbeInputV2(input) != nil {
		m.sourceExecutionMu.Lock()
		m.clearSourceAuthorityForServer(serverID)
		m.sourceExecutionMu.Unlock()
		return domainsecurity.VerifiedSourceProbe{}, errors.New(domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.VerifiedSourceProbe{}, err
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	var probe domainsecurity.VerifiedSourceProbe
	var countCanary fundsCountCanaryObservationV2
	var selection datasetsnapshotport.CurrentSelectionV2
	err := m.datasetAuthority.WithCurrentSelectionV2(
		ctx,
		datasetResolveInputForSourceProbeV2(input.Context, input.Binding),
		input.Context,
		func(current datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			if capability == nil || !datasetSelectionMatchesSourceContextV2(current, input.Context) {
				return errors.New("case source dataset selection authority is invalid")
			}
			selection = cloneCurrentDatasetSelectionV2(current)
			projection, err := fundsCountProjectionForSelectionV2(input.Context, current)
			if err != nil {
				return err
			}
			return capability.UseExact(current, input.Context, func(leaseContext context.Context) error {
				var err error
				probe, countCanary, err = m.probeCaseSourceUnderLease(leaseContext, input, sourceProbeConstraints{
					fundsCountProjection: projection,
				})
				return err
			})
		},
	)
	if err != nil || !domainsecurity.SourceProbeEligibleForHostAuthorityV2(probe) {
		m.clearSourceAuthorityForServer(serverID)
		return domainsecurity.VerifiedSourceProbe{}, errors.New(domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable)
	}
	admission := sourceEvidenceAdmissionV2{
		Context: input.Context, Binding: input.Binding, Probe: probe, Selection: selection,
		CountCanary: countCanary,
	}
	m.mu.Lock()
	if !m.sourceProbeConnectionMatchesNoLock(serverID, probe) ||
		!m.fundsCountCanaryObservationCurrentNoLock(admission) {
		m.mu.Unlock()
		m.clearSourceAuthorityForServer(serverID)
		return domainsecurity.VerifiedSourceProbe{}, errors.New("case source probe connection epoch changed")
	}
	key := sourceProbeRegistryKey(serverID, input.Context.ThreadID, input.Context.TurnID)
	m.sourceProbes[key] = probe
	m.sourceAdmissions[key] = admission
	m.mu.Unlock()
	return probe, nil
}

type sourceProbeConstraints struct {
	catalogFingerprint       string
	preserveCurrentAuthority bool
	fundsCountProjection     domainsecurity.FundsCountProjectionV2
}

type sourceEvidenceAdmissionV2 struct {
	Context     domainsecurity.TurnSecurityContext
	Binding     domainsecurity.CaseBindingObservationV1
	Probe       domainsecurity.VerifiedSourceProbe
	Selection   datasetsnapshotport.CurrentSelectionV2
	CountCanary fundsCountCanaryObservationV2
}

func (m *ProductionManager) currentHostFundsCountCanaryGrantNoLock(
	serverID string,
	expectedClient mcpTransportClient,
	expectedSpecFingerprint string,
	expectedCatalogFingerprint string,
	expectedServerIdentity string,
	expectedConnectionEpoch uint64,
	expectedGrant domainplugincapability.FundsSourceReadGrantV1,
) (domainplugincapability.FundsSourceReadGrantV1, bool) {
	if m == nil || serverID != "analytix_funds" || expectedClient == nil ||
		!domainsecurity.IsSHA256Hex(expectedSpecFingerprint) ||
		!domainsecurity.IsSHA256Hex(expectedCatalogFingerprint) ||
		strings.TrimSpace(expectedServerIdentity) == "" || expectedConnectionEpoch == 0 ||
		m.hostFundsServer == nil || m.hostFundsSourceReadLifecycle == nil {
		return domainplugincapability.FundsSourceReadGrantV1{}, false
	}
	spec, specOK := m.specByIDNoLock(serverID)
	tool, toolOK := m.tools[fundsCountEvidenceToolName]
	if !specOK || !toolOK || tool.Placeholder || !isHostFundsCountManagedToolV1(fundsCountEvidenceToolName, tool) ||
		!spec.ReadOnlyToolNames[hostFundsCountToolNameV1] ||
		m.clients[serverID] != expectedClient ||
		m.specFingerprints[serverID] != expectedSpecFingerprint ||
		SpecFingerprint(spec) != expectedSpecFingerprint ||
		m.hostFundsFingerprint != expectedSpecFingerprint ||
		!m.hostFundsServer.matches(spec, expectedSpecFingerprint) ||
		m.sourceCatalogs[serverID] != expectedCatalogFingerprint ||
		m.serverIdentities[serverID] != expectedServerIdentity ||
		m.connectionEpochs[serverID] != expectedConnectionEpoch ||
		mcpidentity.VerifyConfiguredProvenance(spec) != nil ||
		!m.hostFundsSourceReadGrantedNoLock() {
		return domainplugincapability.FundsSourceReadGrantV1{}, false
	}
	grant, ok := m.hostFundsSourceReadLifecycle.CurrentGrant()
	if !ok || !domainsecurity.IsSHA256Hex(grant.GrantDigest()) || grant.Generation() == 0 ||
		grant.ConnectionEpoch() != expectedConnectionEpoch {
		return domainplugincapability.FundsSourceReadGrantV1{}, false
	}
	if expectedGrant.GrantDigest() != "" &&
		(expectedGrant.GrantDigest() != grant.GrantDigest() ||
			expectedGrant.Generation() != grant.Generation() ||
			expectedGrant.ConnectionEpoch() != grant.ConnectionEpoch()) {
		return domainplugincapability.FundsSourceReadGrantV1{}, false
	}
	return grant, true
}

type managerHostEvidenceCapabilityV2 struct {
	mu                sync.RWMutex
	active            bool
	ctx               context.Context
	manager           *ProductionManager
	serverID          string
	admission         sourceEvidenceAdmissionV2
	datasetCapability datasetsnapshotport.CurrentSelectionCapabilityV2
}

func (*managerHostEvidenceCapabilityV2) MarshalJSON() ([]byte, error) {
	return nil, errors.New("host evidence capability is not serializable")
}

func (capability *managerHostEvidenceCapabilityV2) DatasetSelection() (datasetsnapshotport.CurrentSelectionV2, error) {
	if capability == nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("host evidence capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("host evidence capability is inactive")
	}
	return cloneCurrentDatasetSelectionV2(capability.admission.Selection), nil
}

func (capability *managerHostEvidenceCapabilityV2) UseExact(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	use func(context.Context) error,
) error {
	if capability == nil {
		return errors.New("host evidence capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || use == nil ||
		!reflect.DeepEqual(securityContext, capability.admission.Context) ||
		!reflect.DeepEqual(probe, capability.admission.Probe) ||
		!reflect.DeepEqual(selection, capability.admission.Selection) ||
		capability.datasetCapability == nil || capability.manager == nil {
		return errors.New("host evidence capability does not authorize this context, probe, and dataset selection")
	}
	return capability.datasetCapability.UseExact(
		cloneCurrentDatasetSelectionV2(capability.admission.Selection),
		capability.admission.Context,
		func(leaseContext context.Context) error {
			if leaseContext == nil || leaseContext.Err() != nil || capability.ctx.Err() != nil ||
				!capability.manager.sourceEvidenceAdmissionIsCurrent(capability.serverID, capability.admission) {
				return errors.New("host evidence admission is no longer current")
			}
			if err := use(leaseContext); err != nil {
				return err
			}
			if leaseContext.Err() != nil || capability.ctx.Err() != nil ||
				!capability.manager.sourceEvidenceAdmissionIsCurrent(capability.serverID, capability.admission) {
				return errors.New("host evidence admission changed during use")
			}
			return nil
		},
	)
}

func (capability *managerHostEvidenceCapabilityV2) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	capability.mu.Unlock()
}

func (m *ProductionManager) probeCaseSourceUnderLease(
	ctx context.Context,
	input sourceprobeport.Input,
	constraints sourceProbeConstraints,
) (domainsecurity.VerifiedSourceProbe, fundsCountCanaryObservationV2, error) {
	serverID := strings.TrimSpace(input.ServerID)
	if validateSourceProbeInputV2(input) != nil {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe authority is incomplete")
	}
	workspace := input.Context.WorkspaceRealPath
	caseID := input.Context.CaseID
	bindingHash := input.Context.CaseBindingHash
	datasetSnapshotID := input.Context.DatasetSnapshotID
	if domainsecurity.ValidateFundsCountProjectionV2(constraints.fundsCountProjection) != nil ||
		constraints.fundsCountProjection.TurnSecurityContextDigest != input.Context.ContextDigest ||
		constraints.fundsCountProjection.DatasetSnapshotID != datasetSnapshotID {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe projection authority is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	m.mu.Lock()
	spec, found := m.specByIDNoLock(serverID)
	client := m.clients[serverID]
	epoch := m.connectionEpochs[serverID]
	serverIdentity := m.serverIdentities[serverID]
	specFingerprint := m.specFingerprints[serverID]
	if !constraints.preserveCurrentAuthority {
		m.clearSourceAuthorityForServerNoLock(serverID)
	}
	m.mu.Unlock()
	if !found || !mcpServerSpecAuthorizedForWorkspace(spec, workspace) || client == nil || epoch == 0 || serverIdentity == "" || strings.TrimSpace(spec.ExpectedServerName) == "" || strings.TrimSpace(spec.ExpectedServerVersion) == "" || !domainsecurity.IsSHA256Hex(specFingerprint) {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe server authority is unavailable")
	}
	if SpecFingerprint(spec) != specFingerprint {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe configuration authority changed")
	}
	if err := mcpidentity.VerifyConfiguredProvenance(spec); err != nil {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe source integrity is invalid")
	}
	native, ok := client.(mcpNativeProbeClient)
	if !ok {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, errors.New("MCP native probe transport is unavailable"))
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe transport is unavailable")
	}
	freshCatalogResult, err := listMCPToolCatalogContext(probeContext, client)
	if err != nil {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, err)
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe catalog refresh failed")
	}
	freshTools := freshCatalogResult.Tools
	if len(freshTools) == 0 {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, errors.New("MCP native probe returned an empty catalog"))
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe catalog refresh failed")
	}
	freshCatalog := sourceProbeCatalogFingerprintWithIssues(freshTools, freshCatalogResult.Quarantined)
	if constraints.catalogFingerprint != "" && freshCatalog != constraints.catalogFingerprint {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, errors.New("MCP native probe catalog changed"))
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe catalog changed")
	}
	cachedTools, err := cacheableTools(freshTools, spec)
	if err != nil {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, err)
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe catalog has an invalid strict tool contract")
	}
	params, err := sourceProbeNativeParamsV2(constraints.fundsCountProjection)
	if err != nil {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe projection carrier is invalid")
	}
	responseValue, err := native.CallNativeContext(probeContext, "analytix/sourceProbe", params)
	if err != nil {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, err)
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe request failed")
	}
	wireResponse, err := parseFundsSourceProbeResponseV2(
		responseValue,
		constraints.fundsCountProjection,
		strings.TrimSpace(spec.ExpectedServerName),
		strings.TrimSpace(spec.ExpectedServerVersion),
	)
	if err != nil {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, errors.New("MCP native probe response did not match host authority"))
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe response is not ready or does not match host authority")
	}
	m.mu.Lock()
	grant, grantCurrent := m.currentHostFundsCountCanaryGrantNoLock(
		serverID, client, specFingerprint, freshCatalog, serverIdentity, epoch,
		domainplugincapability.FundsSourceReadGrantV1{},
	)
	m.mu.Unlock()
	if !grantCurrent {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe funds source-read grant is unavailable")
	}
	if err := m.requireHostFundsNativeOwnerCurrent(probeContext, serverID); err != nil {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, err
	}
	countResult, err := callFundsCountToolWithProjectionV2(
		probeContext,
		client,
		map[string]any{"table_name": domainsecurity.FundsCountProjectionTableV2},
		constraints.fundsCountProjection,
	)
	if err != nil {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, err)
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe count canary failed")
	}
	countCanary, err := validateFundsCountToolResultV2(countResult, constraints.fundsCountProjection)
	if err != nil {
		m.revokeMCPAuthorityAfterNativeFailure(serverID, client, err)
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe count canary result is invalid")
	}
	if err := m.requireHostFundsNativeOwnerCurrent(probeContext, serverID); err != nil {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, err
	}
	m.mu.Lock()
	postGrant, grantStillCurrent := m.currentHostFundsCountCanaryGrantNoLock(
		serverID, client, specFingerprint, freshCatalog, serverIdentity, epoch, grant,
	)
	m.mu.Unlock()
	if !grantStillCurrent || postGrant.GrantDigest() != grant.GrantDigest() {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe funds source-read grant changed during count")
	}
	countCanary.grantGeneration = grant.Generation()
	countCanary.connectionEpoch = grant.ConnectionEpoch()
	checkedAt := time.Now().UTC()
	response := domainsecurity.SourceProbeResponse{
		Version:    domainsecurity.SourceProbeVersion,
		ServerName: wireResponse.ServerName, ServerVersion: wireResponse.ServerVersion,
		CaseID: caseID, CaseBindingHash: bindingHash, DatasetSnapshotID: datasetSnapshotID,
		Ready: true, ReadOnly: true, CheckedAt: checkedAt.Format(time.RFC3339Nano),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	currentSpec, currentSpecOK := m.specByIDNoLock(serverID)
	if m.clients[serverID] != client || m.connectionEpochs[serverID] != epoch || m.serverIdentities[serverID] != serverIdentity ||
		!currentSpecOK || m.specFingerprints[serverID] != specFingerprint || SpecFingerprint(currentSpec) != specFingerprint {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe connection epoch changed")
	}
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: serverID, ServerIdentity: serverIdentity, ConnectionEpoch: epoch,
		CatalogFingerprint: freshCatalog, SpecFingerprint: specFingerprint,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, ContextEpoch: input.Context.ContextEpoch,
		ContextDigest: input.Context.ContextDigest, DatasetSnapshotID: datasetSnapshotID,
		CheckedAt: checkedAt, Response: response,
	})
	if err != nil {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, err
	}
	if !domainsecurity.SourceProbeEligibleForHostAuthorityV2(probe) {
		return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New(domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable)
	}
	if constraints.preserveCurrentAuthority {
		if m.sourceCatalogs[serverID] != freshCatalog {
			return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe catalog changed")
		}
	} else {
		bindings, err := m.prepareToolBindingsNoLock(spec, freshTools, false)
		if err != nil {
			return domainsecurity.VerifiedSourceProbe{}, fundsCountCanaryObservationV2{}, errors.New("case source probe catalog has an ambiguous tool identity")
		}
		m.removeServerToolsNoLock(serverID)
		m.installToolBindingsNoLock(bindings)
		m.toolContractIssues[serverID] = cloneToolContractIssues(freshCatalogResult.Quarantined)
		m.sourceCatalogs[serverID] = freshCatalog
		delete(m.cachedSchemas, serverID)
		m.refreshCatalogStateNoLock()
		_ = saveCachedSchema(m.cacheDir, serverID, CachedSchema{
			SpecHash: specFingerprint, Tools: cachedTools, LastValidated: time.Now().UTC(),
		})
	}
	return probe, countCanary, nil
}

func validateSourceProbeInputV2(input sourceprobeport.Input) error {
	contextValue := input.Context
	if strings.TrimSpace(input.ServerID) == "" ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(contextValue) != nil ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(contextValue.DatasetSnapshotID) ||
		domainsecurity.ValidateCaseBindingObservationV1(input.Binding) != nil ||
		input.Binding.State != domainsecurity.CaseBindingStateValid ||
		input.Binding.WorkspaceRealPath != contextValue.WorkspaceRealPath ||
		input.Binding.CaseID != contextValue.CaseID ||
		input.Binding.CaseBindingHash != contextValue.CaseBindingHash ||
		input.Binding.ObservationDigest != contextValue.PublicationPolicy.BindingObservationDigest ||
		input.WorkspaceRealPath != contextValue.WorkspaceRealPath ||
		input.ThreadID != contextValue.ThreadID ||
		input.TurnID != contextValue.TurnID ||
		input.CaseID != contextValue.CaseID ||
		input.CaseBindingHash != contextValue.CaseBindingHash ||
		input.DatasetSnapshotID != contextValue.DatasetSnapshotID ||
		input.ContextEpoch != contextValue.ContextEpoch ||
		input.ContextDigest != contextValue.ContextDigest {
		return errors.New("case source probe input does not match the frozen V2 context")
	}
	return nil
}

func validateCurrentSourceBindingV2(
	securityContext domainsecurity.TurnSecurityContext,
	binding domainsecurity.CaseBindingObservationV1,
) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(securityContext.DatasetSnapshotID) ||
		domainsecurity.ValidateCaseBindingObservationV1(binding) != nil ||
		binding.State != domainsecurity.CaseBindingStateValid ||
		binding.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		binding.CaseID != securityContext.CaseID ||
		binding.CaseBindingHash != securityContext.CaseBindingHash ||
		binding.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest {
		return errors.New("current source binding does not match the frozen V2 context")
	}
	return nil
}

func datasetResolveInputForSourceProbeV2(
	securityContext domainsecurity.TurnSecurityContext,
	binding domainsecurity.CaseBindingObservationV1,
) datasetsnapshotport.ResolveInputV2 {
	return datasetsnapshotport.ResolveInputV2{
		TenantID: securityContext.TenantID, UserID: securityContext.UserID, Observation: binding,
		ExpectedDatasetSnapshotID: securityContext.DatasetSnapshotID,
	}
}

func datasetSelectionMatchesSourceContextV2(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!selection.Head.HasBundle || !domainsecurity.IsSHA256Hex(selection.SelectionDigest) ||
		!currentDatasetSelectionDigestValidV2(selection) ||
		len(selection.DatasetIndexPath) == 0 ||
		selection.Head.Bundle.DatasetSnapshotCount != uint64(len(selection.DatasetIndexPath)) ||
		selection.Head.Bundle.DatasetSnapshotIndexDigest != selection.DatasetIndexPath[0].IndexDigest ||
		selection.Snapshot.Record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		selection.Snapshot.Record.SourceManifestHash != securityContext.SourceManifestHash ||
		selection.Snapshot.Manifest.SourceManifestHash != securityContext.SourceManifestHash ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForManifestV2(
			selection.Snapshot.Record, selection.Snapshot.Manifest,
		) != nil ||
		datasetsnapshotport.ValidateResolvedSnapshotV2(selection.Snapshot) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexRecordV2(selection.SelectedIndex, selection.Snapshot.Record) != nil {
		return false
	}
	for _, index := range selection.DatasetIndexPath {
		if reflect.DeepEqual(index, selection.SelectedIndex) {
			return true
		}
	}
	return false
}

func cloneCurrentDatasetSelectionV2(selection datasetsnapshotport.CurrentSelectionV2) datasetsnapshotport.CurrentSelectionV2 {
	selection.DatasetIndexPath = append([]domainsecurity.DatasetSnapshotIndexV1(nil), selection.DatasetIndexPath...)
	return selection
}

func (m *ProductionManager) clearSourceAuthorityForServer(serverID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.clearSourceAuthorityForServerNoLock(serverID)
	m.mu.Unlock()
}

func (m *ProductionManager) clearSourceAuthorityForServerNoLock(serverID string) {
	serverID = strings.TrimSpace(serverID)
	for key, probe := range m.sourceProbes {
		if probe.ServerID == serverID {
			delete(m.sourceProbes, key)
		}
	}
	for key, admission := range m.sourceAdmissions {
		if admission.Probe.ServerID == serverID {
			delete(m.sourceAdmissions, key)
		}
	}
}

func (m *ProductionManager) sourceProbeConnectionMatchesNoLock(
	serverID string,
	probe domainsecurity.VerifiedSourceProbe,
) bool {
	spec, found := m.specByIDNoLock(serverID)
	return found && m.clients[serverID] != nil &&
		m.connectionEpochs[serverID] == probe.ConnectionEpoch &&
		m.serverIdentities[serverID] == probe.ServerIdentity &&
		m.specFingerprints[serverID] == probe.SpecFingerprint &&
		SpecFingerprint(spec) == probe.SpecFingerprint &&
		m.sourceCatalogs[serverID] == probe.CatalogFingerprint
}

func (m *ProductionManager) sourceEvidenceAdmissionIsCurrent(
	serverID string,
	expected sourceEvidenceAdmissionV2,
) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := sourceProbeRegistryKey(serverID, expected.Context.ThreadID, expected.Context.TurnID)
	admission, found := m.sourceAdmissions[key]
	probe, probeFound := m.sourceProbes[key]
	return found && probeFound &&
		reflect.DeepEqual(admission.Context, expected.Context) &&
		reflect.DeepEqual(admission.Binding, expected.Binding) &&
		reflect.DeepEqual(admission.Probe, expected.Probe) &&
		reflect.DeepEqual(admission.CountCanary, expected.CountCanary) &&
		sameDatasetSelectionBindingV2(admission.Selection, expected.Selection) &&
		reflect.DeepEqual(probe, expected.Probe) &&
		m.sourceEvidenceAdmissionBoundToContextNoLock(serverID, admission, expected.Context)
}

func sameDatasetSelectionBindingV2(
	left datasetsnapshotport.CurrentSelectionV2,
	right datasetsnapshotport.CurrentSelectionV2,
) bool {
	return datasetSelectionChildBindingValidV2(left) &&
		datasetSelectionChildBindingValidV2(right) &&
		left.Head.Bundle.DatasetSnapshotIndexDigest == right.Head.Bundle.DatasetSnapshotIndexDigest &&
		left.Head.Bundle.DatasetSnapshotCount == right.Head.Bundle.DatasetSnapshotCount &&
		reflect.DeepEqual(left.DatasetIndexPath, right.DatasetIndexPath) &&
		reflect.DeepEqual(left.SelectedIndex, right.SelectedIndex) &&
		reflect.DeepEqual(left.Snapshot, right.Snapshot)
}

func datasetSelectionChildBindingValidV2(
	selection datasetsnapshotport.CurrentSelectionV2,
) bool {
	return selection.Head.HasBundle &&
		currentDatasetSelectionDigestValidV2(selection) &&
		len(selection.DatasetIndexPath) > 0 &&
		selection.Head.Bundle.DatasetSnapshotCount == uint64(len(selection.DatasetIndexPath)) &&
		selection.Head.Bundle.DatasetSnapshotIndexDigest == selection.DatasetIndexPath[0].IndexDigest
}

func currentDatasetSelectionDigestValidV2(selection datasetsnapshotport.CurrentSelectionV2) bool {
	return datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) == nil
}

func (m *ProductionManager) registeredSourceBindingForContext(
	serverID string,
	securityContext domainsecurity.TurnSecurityContext,
) (domainsecurity.CaseBindingObservationV1, bool) {
	if m == nil {
		return domainsecurity.CaseBindingObservationV1{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	admission, found := m.sourceAdmissions[sourceProbeRegistryKey(
		serverID, securityContext.ThreadID, securityContext.TurnID,
	)]
	if !found || !m.sourceEvidenceAdmissionBoundToContextNoLock(serverID, admission, securityContext) {
		return domainsecurity.CaseBindingObservationV1{}, false
	}
	return admission.Binding, true
}

func (m *ProductionManager) withCurrentHostEvidenceUnderLease(
	ctx context.Context,
	serverID string,
	securityContext domainsecurity.TurnSecurityContext,
	binding domainsecurity.CaseBindingObservationV1,
	callback func(domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	serverID = strings.TrimSpace(serverID)
	if m == nil || m.datasetAuthority == nil || callback == nil || serverID == "" ||
		validateCurrentSourceBindingV2(securityContext, binding) != nil {
		return errors.New("current host evidence authority is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	key := sourceProbeRegistryKey(serverID, securityContext.ThreadID, securityContext.TurnID)
	m.mu.Lock()
	registered, found := m.sourceAdmissions[key]
	probe, probeFound := m.sourceProbes[key]
	spec, specFound := m.specByIDNoLock(serverID)
	current := found && probeFound && specFound &&
		reflect.DeepEqual(registered.Context, securityContext) &&
		reflect.DeepEqual(registered.Binding, binding) &&
		reflect.DeepEqual(registered.Probe, probe) &&
		m.sourceEvidenceAdmissionBoundToContextNoLock(serverID, registered, securityContext)
	m.mu.Unlock()
	if !current || mcpidentity.VerifyConfiguredProvenance(spec) != nil {
		return errors.New("current host evidence admission is unavailable")
	}
	return m.datasetAuthority.WithCurrentSelectionV2(
		ctx,
		datasetResolveInputForSourceProbeV2(securityContext, binding),
		securityContext,
		func(selection datasetsnapshotport.CurrentSelectionV2, datasetCapability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			if datasetCapability == nil || !datasetSelectionMatchesSourceContextV2(selection, securityContext) ||
				!sameDatasetSelectionBindingV2(registered.Selection, selection) {
				m.clearSourceAuthorityForServer(serverID)
				return errors.New("current host evidence dataset selection changed")
			}
			currentAdmission := sourceEvidenceAdmissionV2{
				Context: securityContext, Binding: binding, Probe: probe,
				Selection: cloneCurrentDatasetSelectionV2(selection), CountCanary: registered.CountCanary,
			}
			capability := &managerHostEvidenceCapabilityV2{
				active: true, ctx: ctx, manager: m, serverID: serverID,
				admission: currentAdmission, datasetCapability: datasetCapability,
			}
			defer capability.close()
			if err := capability.UseExact(
				securityContext,
				probe,
				cloneCurrentDatasetSelectionV2(selection),
				func(context.Context) error { return nil },
			); err != nil {
				return err
			}
			return callback(probe, capability)
		},
	)
}

func (m *ProductionManager) WithFreshPublicationSnapshot(
	context.Context,
	sourceprobeport.PublicationInput,
	func([]domainsecurity.VerifiedSourceProbe) error,
) error {
	return errors.New("publication host evidence authority is unavailable on the compatibility surface")
}

func (m *ProductionManager) WithFreshPublicationSnapshotAuthority(
	ctx context.Context,
	input sourceprobeport.PublicationInput,
	callback func([]domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	if callback == nil || validateCurrentSourceBindingV2(input.Context, input.Binding) != nil ||
		len(input.Requirements) == 0 || m == nil || m.datasetAuthority == nil {
		return errors.New("publication snapshot authority is invalid")
	}
	expectedProbeIdentity := ""
	seenReceipts := map[string]bool{}
	for _, requirement := range input.Requirements {
		receiptID := strings.TrimSpace(requirement.ReceiptID)
		identity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(requirement.ServerIdentity)
		if receiptID == "" || seenReceipts[receiptID] || requirement.ServerID != "analytix_funds" ||
			!validHostFundsPackageVersionV1(requirement.ServerVersion) ||
			(requirement.ToolName != fundsCountEvidenceToolName &&
				requirement.ToolName != CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1)) ||
			requirement.ConnectionEpoch == 0 || requirement.DatasetSnapshotID != input.Context.DatasetSnapshotID || identityErr != nil ||
			identity.ServerID != requirement.ServerID || identity.ObservedVersion != requirement.ServerVersion || identity.ConnectionEpoch != requirement.ConnectionEpoch ||
			(expectedProbeIdentity != "" && expectedProbeIdentity != requirement.ServerIdentity) {
			return errors.New("publication snapshot requirement is invalid")
		}
		seenReceipts[receiptID] = true
		expectedProbeIdentity = requirement.ServerIdentity
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	previous, ok := m.sourceProbeForContextNoLock("analytix_funds", input.Context)
	registered, admissionOK := m.sourceAdmissions[sourceProbeRegistryKey(
		"analytix_funds", input.Context.ThreadID, input.Context.TurnID,
	)]
	current := ok && admissionOK && reflect.DeepEqual(registered.Binding, input.Binding) &&
		m.sourceProbeBoundToContextNoLock("analytix_funds", previous, input.Context) &&
		previous.ServerIdentity == expectedProbeIdentity
	m.mu.Unlock()
	if !current {
		return errors.New("publication snapshot has no current source authority")
	}
	var probe domainsecurity.VerifiedSourceProbe
	var countCanary fundsCountCanaryObservationV2
	err := m.datasetAuthority.WithCurrentSelectionV2(
		ctx,
		datasetResolveInputForSourceProbeV2(input.Context, input.Binding),
		input.Context,
		func(selection datasetsnapshotport.CurrentSelectionV2, datasetCapability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			if datasetCapability == nil || !datasetSelectionMatchesSourceContextV2(selection, input.Context) ||
				!sameDatasetSelectionBindingV2(registered.Selection, selection) {
				return errors.New("publication snapshot dataset selection changed")
			}
			projection, projectionErr := fundsCountProjectionForSelectionV2(input.Context, selection)
			if projectionErr != nil {
				return projectionErr
			}
			err := datasetCapability.UseExact(selection, input.Context, func(leaseContext context.Context) error {
				var probeErr error
				probe, countCanary, probeErr = m.probeCaseSourceUnderLease(leaseContext, sourceprobeport.Input{
					ServerID: "analytix_funds", Context: input.Context, Binding: input.Binding,
					WorkspaceRealPath: input.Context.WorkspaceRealPath, ThreadID: input.Context.ThreadID,
					TurnID: input.Context.TurnID, CaseID: input.Context.CaseID, CaseBindingHash: input.Context.CaseBindingHash,
					DatasetSnapshotID: input.Context.DatasetSnapshotID, ContextEpoch: input.Context.ContextEpoch,
					ContextDigest: input.Context.ContextDigest,
				}, sourceProbeConstraints{
					catalogFingerprint: previous.CatalogFingerprint, preserveCurrentAuthority: true,
					fundsCountProjection: projection,
				})
				return probeErr
			})
			if err != nil || probe.ServerIdentity != expectedProbeIdentity ||
				probe.SpecFingerprint != previous.SpecFingerprint || ctx.Err() != nil {
				return errors.New("publication snapshot freshness verification failed")
			}
			admission := sourceEvidenceAdmissionV2{
				Context: input.Context, Binding: input.Binding, Probe: probe,
				Selection: cloneCurrentDatasetSelectionV2(selection), CountCanary: countCanary,
			}
			m.mu.Lock()
			spec, found := m.specByIDNoLock("analytix_funds")
			stillCurrent := found && m.sourceProbeConnectionMatchesNoLock("analytix_funds", probe) &&
				m.fundsCountCanaryObservationCurrentNoLock(admission)
			if stillCurrent {
				key := sourceProbeRegistryKey("analytix_funds", input.Context.ThreadID, input.Context.TurnID)
				m.sourceProbes[key] = probe
				m.sourceAdmissions[key] = admission
			}
			m.mu.Unlock()
			if !stillCurrent || mcpidentity.VerifyConfiguredProvenance(spec) != nil {
				return errors.New("publication snapshot source integrity changed")
			}
			capability := &managerHostEvidenceCapabilityV2{
				active: true, ctx: ctx, manager: m, serverID: "analytix_funds",
				admission: admission, datasetCapability: datasetCapability,
			}
			defer capability.close()
			return callback([]domainsecurity.VerifiedSourceProbe{probe}, capability)
		},
	)
	if err != nil {
		m.clearSourceAuthorityForServer("analytix_funds")
		return err
	}
	return nil
}

func (m *ProductionManager) WithCurrentProbe(
	context.Context,
	sourceprobeport.CurrentInput,
	func(domainsecurity.VerifiedSourceProbe) error,
) error {
	return errors.New("current host evidence authority is unavailable on the compatibility surface")
}

func (m *ProductionManager) WithCurrentProbeAuthority(
	ctx context.Context,
	input sourceprobeport.CurrentInput,
	callback func(domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	if callback == nil || validateCurrentSourceBindingV2(input.Context, input.Binding) != nil ||
		strings.TrimSpace(input.ServerID) == "" || input.ConnectionEpoch == 0 {
		return errors.New("current case source probe request is invalid")
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	return m.withCurrentHostEvidenceUnderLease(
		ctx, input.ServerID, input.Context, input.Binding,
		func(probe domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
			if probe.ConnectionEpoch != input.ConnectionEpoch {
				return errors.New("current case source probe connection epoch changed")
			}
			return callback(probe, capability)
		},
	)
}

func (m *ProductionManager) ValidateCurrentProbe(ctx context.Context, serverID string, securityContext domainsecurity.TurnSecurityContext) error {
	serverID = strings.TrimSpace(serverID)
	if m == nil || ctx == nil || serverID == "" || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return errors.New("current case source probe validation is invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	m.mu.Lock()
	admission, found := m.sourceAdmissions[sourceProbeRegistryKey(serverID, securityContext.ThreadID, securityContext.TurnID)]
	m.mu.Unlock()
	if !found || !reflect.DeepEqual(admission.Context, securityContext) {
		return errors.New("current case source probe authority is unavailable")
	}
	return m.withCurrentHostEvidenceUnderLease(
		ctx, serverID, securityContext, admission.Binding,
		func(probe domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
			selection, err := capability.DatasetSelection()
			if err != nil {
				return err
			}
			return capability.UseExact(securityContext, probe, selection, func(context.Context) error { return nil })
		},
	)
}

func (m *ProductionManager) WithCurrentEvidenceRead(
	context.Context,
	sourceprobeport.EvidenceReadInput,
	func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error,
) error {
	return errors.New("current host evidence authority is unavailable on the compatibility surface")
}

func (m *ProductionManager) currentFundsEvidenceReadAuthorityNoLock(
	input sourceprobeport.EvidenceReadInput,
	probe domainsecurity.VerifiedSourceProbe,
	expectedClient mcpTransportClient,
) (ServerSpec, mcpTransportClient, bool) {
	tool, toolOK := m.tools[fundsCountEvidenceToolName]
	spec, specOK := m.specByIDNoLock("analytix_funds")
	client := m.clients["analytix_funds"]
	epoch := m.connectionEpochs["analytix_funds"]
	serverIdentity := m.serverIdentities["analytix_funds"]
	current := toolOK && !tool.Placeholder && tool.ServerID == "analytix_funds" &&
		tool.RawName == hostFundsCountToolNameV1 &&
		m.managedToolCapabilityCurrentNoLock(fundsCountEvidenceToolName, tool) &&
		specOK && client != nil && (expectedClient == nil || client == expectedClient) &&
		epoch == input.Grant.ConnectionEpoch && serverIdentity != "" &&
		input.Grant.ServerIdentity == serverIdentity && probe.ServerIdentity == serverIdentity &&
		input.Grant.SchemaHash == managedToolSchemaHash(fundsCountEvidenceToolName, tool) &&
		m.sourceProbeBoundToContextNoLock("analytix_funds", probe, input.Context) &&
		m.hostFundsFingerprint != "" && SpecFingerprint(spec) == m.hostFundsFingerprint &&
		spec.IdentitySource == mcpidentity.HostInstalledGenerationSourceV1 &&
		spec.ReadOnlyToolNames[hostFundsCountToolNameV1]
	return spec, client, current
}

func (m *ProductionManager) WithCurrentEvidenceReadAuthority(
	ctx context.Context,
	input sourceprobeport.EvidenceReadInput,
	callback func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult, sourceprobeport.HostEvidenceCapability) error,
) error {
	grantIdentity, grantIdentityErr := domainsecurity.ParseVerifiedMCPServerIdentity(input.Grant.ServerIdentity)
	if callback == nil || validateCurrentSourceBindingV2(input.Context, input.Binding) != nil ||
		domainsecurity.ValidateExecutionGrant(input.Grant) != nil || input.Grant.ContextDigest != input.Context.ContextDigest ||
		input.Grant.TurnID != input.Context.TurnID || input.Grant.ToolName != fundsCountEvidenceToolName ||
		input.Grant.ConnectionEpoch == 0 || !input.Grant.ReadOnly || input.Grant.ArgsHash != domainsecurity.CanonicalJSONHash(input.Arguments) ||
		grantIdentityErr != nil || grantIdentity.ServerID != "analytix_funds" || grantIdentity.ConnectionEpoch != input.Grant.ConnectionEpoch ||
		!exactFundsCountEvidenceArguments(input.Arguments) {
		return errors.New("current funds evidence read authority is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.sourceExecutionMu.Lock()
	defer m.sourceExecutionMu.Unlock()
	return m.withCurrentHostEvidenceUnderLease(
		ctx, "analytix_funds", input.Context, input.Binding,
		func(probe domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
			m.mu.Lock()
			spec, client, current := m.currentFundsEvidenceReadAuthorityNoLock(input, probe, nil)
			m.mu.Unlock()
			if !current || mcpidentity.VerifyConfiguredProvenance(spec) != nil {
				return errors.New("current funds evidence source authority is unavailable")
			}
			native, ok := client.(mcpNativeEvidenceClient)
			if !ok {
				return errors.New("current funds evidence native transport is unavailable")
			}
			selection, err := capability.DatasetSelection()
			if err != nil {
				return err
			}
			projection, err := fundsCountProjectionForSelectionV2(input.Context, selection)
			if err != nil {
				return errors.New("current funds evidence projection authority is invalid")
			}
			params, err := fundsEvidenceReadNativeParamsV2(projection)
			if err != nil {
				return errors.New("current funds evidence projection carrier is invalid")
			}
			var raw domainmcp.LosslessToolResult
			var nativeErr error
			err = capability.UseExact(input.Context, probe, selection, func(leaseContext context.Context) error {
				raw, nativeErr = native.CallNativeLosslessContext(leaseContext, fundsEvidenceReadMethod, params)
				if nativeErr != nil {
					return nil
				}
				if !domainmcp.ValidLosslessToolResult(raw) || domainsecurity.SHA256Hex(raw.RawResult) != raw.RawSHA256 {
					nativeErr = errors.New("MCP native evidence result integrity is invalid")
					return nil
				}
				nativeErr = domainjsonstrict.Validate(raw.RawResult, domainjsonstrict.Options{
					RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
				})
				return nil
			})
			if err != nil {
				return errors.New("current funds evidence dataset authority changed during native read")
			}
			if nativeErr != nil {
				m.revokeMCPAuthorityAfterNativeFailure("analytix_funds", client, nativeErr)
				return errors.New("current funds evidence native read failed")
			}
			m.mu.Lock()
			postSpec, _, postCurrent := m.currentFundsEvidenceReadAuthorityNoLock(input, probe, client)
			m.mu.Unlock()
			if !postCurrent || mcpidentity.VerifyConfiguredProvenance(postSpec) != nil {
				return errors.New("current funds evidence source authority changed during native read")
			}
			return callback(probe, raw, capability)
		},
	)
}

func exactFundsCountEvidenceArguments(raw json.RawMessage) bool {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	}); err != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var arguments struct {
		TableName string `json:"table_name"`
	}
	if err := decoder.Decode(&arguments); err != nil || strings.TrimSpace(arguments.TableName) != "analysis_txn_detail_idx" {
		return false
	}
	return errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func (m *ProductionManager) Diagnostics() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.diagnosticsNoLock()
}

func (m *ProductionManager) ServerDiagnostics() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.serverDiagnosticsNoLock(nil)
}

func (m *ProductionManager) ServerDiagnosticsForSecurityContext(context domainsecurity.TurnSecurityContext) []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.serverDiagnosticsNoLock(&context)
}

func (m *ProductionManager) serverDiagnosticsNoLock(context *domainsecurity.TurnSecurityContext) []any {
	specs := m.normalizeProductionSpecsNoLock()
	out := make([]any, 0, len(specs))
	for _, spec := range specs {
		toolNames := m.serverToolNamesNoLock(spec.ID)
		toolContractIssues := m.toolContractIssueDiagnosticsNoLock(spec.ID)
		promptCount := m.serverPromptCountNoLock(spec.ID)
		resourceCount := m.serverResourceCountNoLock(spec.ID)
		diagnostic, rawSecretPresent := RedactedDiagnostic(spec.Headers, spec.Env, spec.URL)
		_, connected := m.clients[spec.ID]
		cached := m.cachedSchemas[spec.ID]
		placeholder := m.serverHasLazyPlaceholderNoLock(spec.ID)
		failure := m.failures[spec.ID]
		transport := firstNonEmpty(spec.Transport, "stdio")
		authConfigured := hasAuthConfig(spec)
		authDiagnostic := diagnoseMCPAuth(transport, mcpServerConnectionStatus(spec, connected, cached, failure), failure, spec.URL, authConfigured)
		probe := domainsecurity.VerifiedSourceProbe{}
		verifiedIdentity := m.serverIdentities[spec.ID]
		sourceReady := false
		if context != nil {
			probe, sourceReady = m.sourceProbeForContextNoLock(spec.ID, *context)
			sourceReady = sourceReady && connected && m.sourceProbeBoundToContextNoLock(spec.ID, probe, *context)
			if !sourceReady {
				probe = domainsecurity.VerifiedSourceProbe{}
			}
		}
		out = append(out, map[string]any{
			"id":                          spec.ID,
			"transport":                   transport,
			"enabled":                     true,
			"available":                   connected,
			"schemaHintAvailable":         cached,
			"connectable":                 cached || placeholder,
			"connected":                   connected,
			"connectionEpoch":             float64(m.connectionEpochs[spec.ID]),
			"expectedServerName":          strings.TrimSpace(spec.ExpectedServerName),
			"expectedServerVersion":       strings.TrimSpace(spec.ExpectedServerVersion),
			"identitySource":              strings.TrimSpace(spec.IdentitySource),
			"lazyCatalogPending":          spec.BackgroundStart && !connected && failure == "",
			"lazyPlaceholder":             placeholder,
			"toolCount":                   float64(len(toolNames)),
			"toolContractQuarantineCount": float64(len(toolContractIssues)),
			"toolContractQuarantines":     toolContractIssues,
			"promptCount":                 float64(promptCount),
			"resourceCount":               float64(resourceCount),
			"toolNames":                   toolNames,
			"lastRefreshedAt":             m.lastRefreshedAt,
			"catalogFingerprint":          m.serverCatalogFingerprintNoLock(spec.ID),
			"schemaCacheHit":              cached,
			"specFingerprint":             m.specFingerprints[spec.ID],
			"diagnostic":                  diagnostic,
			"rawSecretPresent":            rawSecretPresent,
			"credentialed":                authConfigured,
			"authConfigured":              authConfigured,
			"authStatus":                  authDiagnostic.Status,
			"authUrl":                     authDiagnostic.URL,
			"failure":                     failure,
			"cwd":                         filepath.ToSlash(spec.CWD),
			"trustScope":                  spec.TrustScope,
			"trustedWorkspaceRootCount":   float64(len(spec.TrustedWorkspaceRoots)),
			"workspaceTrustConfigured":    spec.TrustScope == "workspace" && len(spec.TrustedWorkspaceRoots) > 0,
			"lowPriority":                 spec.LowPriority,
			"backgroundStart":             spec.BackgroundStart,
			"sourceProbeCount":            float64(m.sourceProbeCountNoLock(spec.ID)),
			"sourceReady":                 sourceReady,
			"sourceProbeDigest":           probe.ProbeDigest,
			"sourceCaseId":                probe.CaseID,
			"datasetSnapshotId":           probe.DatasetSnapshotID,
			"sourceCheckedAt":             probe.CheckedAt,
			"verifiedServerIdentity":      verifiedIdentity,
		})
	}
	return out
}

func (m *ProductionManager) Tools() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.tools))
	for _, name := range m.sortedToolsNoLock() {
		if m.managedToolCapabilityCurrentNoLock(name, m.tools[name]) {
			out = append(out, name)
		}
	}
	return out
}

// LiveTools returns only tools backed by a currently connected transport.
// Cached schemas and lazy placeholders are documentation hints, never source
// readiness or executable evidence for the current turn.
func (m *ProductionManager) LiveTools() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.tools))
	for name, tool := range m.tools {
		if tool.Placeholder {
			continue
		}
		if !m.managedToolCapabilityCurrentNoLock(name, tool) {
			continue
		}
		if _, connected := m.clients[tool.ServerID]; !connected {
			continue
		}
		spec, ok := m.specByIDNoLock(tool.ServerID)
		if !ok || spec.TrustScope != "user" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// LiveToolsForSecurityContext returns the executable MCP surface for one
// frozen turn. High-risk case sources additionally require a host-verified
// probe bound to the same case, binding, dataset snapshot, and connection.
// A connected transport or cached catalog alone is never sufficient.
func (m *ProductionManager) LiveToolsForSecurityContext(context domainsecurity.TurnSecurityContext) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return nil
	}
	out := make([]string, 0, len(m.tools))
	for name, tool := range m.tools {
		if tool.Placeholder || m.clients[tool.ServerID] == nil {
			continue
		}
		if !m.managedToolCapabilityCurrentNoLock(name, tool) {
			continue
		}
		spec, ok := m.specByIDNoLock(tool.ServerID)
		if !ok || !mcpServerSpecAuthorizedForSecurityContext(spec, &context) {
			continue
		}
		if mcpidentity.IsHighRiskServerID(tool.ServerID) && !m.sourceProbeMatchesContextNoLock(tool.ServerID, context) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// MCPToolAdvertisementSnapshotV1 captures every provider-visible MCP field and
// its execution authority under one manager lock. The caller may retain this
// immutable value for one provider request; execution still revalidates the
// current catalog, connection, identity, and host policy before side effects.
func (m *ProductionManager) MCPToolAdvertisementSnapshotV1(context domainsecurity.TurnSecurityContext) []toolcatalogapp.MCPToolAdvertisementV1 {
	if m == nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context) != nil {
		return nil
	}
	type capturedAdvertisement struct {
		advertisement toolcatalogapp.MCPToolAdvertisementV1
		spec          ServerSpec
		highRisk      bool
	}
	m.mu.Lock()
	captured := make([]capturedAdvertisement, 0, len(m.tools))
	for name, tool := range m.tools {
		if tool.Placeholder || m.clients[tool.ServerID] == nil {
			continue
		}
		if !m.managedToolCapabilityCurrentNoLock(name, tool) {
			continue
		}
		spec, specOK := m.specByIDNoLock(tool.ServerID)
		if !specOK || !mcpServerSpecAuthorizedForSecurityContext(spec, &context) {
			continue
		}
		highRisk := mcpidentity.IsHighRiskServerID(tool.ServerID)
		if highRisk && !m.sourceProbeMatchesContextNoLock(tool.ServerID, context) {
			continue
		}
		identity := strings.TrimSpace(m.serverIdentities[tool.ServerID])
		epoch := m.connectionEpochs[tool.ServerID]
		actualName, bindingErr := CanonicalToolNameChecked(tool.ServerID, tool.RawName)
		if !specOK || bindingErr != nil || actualName != name || identity == "" || epoch == 0 {
			continue
		}
		captured = append(captured, capturedAdvertisement{
			advertisement: toolcatalogapp.MCPToolAdvertisementV1{
				Name: name, Description: tool.Tool.Description,
				InputSchema:  append(json.RawMessage(nil), tool.Tool.InputSchema...),
				OutputSchema: append(json.RawMessage(nil), tool.Tool.OutputSchema...),
				TaskSupport:  tool.Tool.TaskSupport, ReadOnly: spec.ReadOnlyToolNames[tool.RawName],
				ConnectionEpoch: epoch, ServerIdentity: identity,
			},
			spec: spec, highRisk: highRisk,
		})
	}
	m.mu.Unlock()
	out := make([]toolcatalogapp.MCPToolAdvertisementV1, 0, len(captured))
	for _, candidate := range captured {
		if candidate.advertisement.ReadOnly && candidate.highRisk && mcpidentity.VerifyConfiguredProvenance(candidate.spec) != nil {
			candidate.advertisement.ReadOnly = false
		}
		out = append(out, candidate.advertisement)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *ProductionManager) sourceProbeMatchesContextNoLock(serverID string, context domainsecurity.TurnSecurityContext) bool {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil {
		return false
	}
	probe, ok := m.sourceProbeForContextNoLock(serverID, context)
	if !ok {
		return false
	}
	return m.sourceProbeBoundToContextNoLock(serverID, probe, context)
}

func (m *ProductionManager) sourceProbeBoundToContextNoLock(serverID string, probe domainsecurity.VerifiedSourceProbe, context domainsecurity.TurnSecurityContext) bool {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!domainsecurity.SourceProbeEligibleForHostAuthorityV2(probe) {
		return false
	}
	if probe.ServerID != serverID {
		return false
	}
	spec, specOK := m.specByIDNoLock(serverID)
	if !specOK || !mcpServerSpecAuthorizedForSecurityContext(spec, &context) {
		return false
	}
	registeredSpecFingerprint := m.specFingerprints[serverID]
	registeredCatalogFingerprint := m.sourceCatalogs[serverID]
	if !domainsecurity.IsSHA256Hex(registeredSpecFingerprint) || !domainsecurity.IsSHA256Hex(registeredCatalogFingerprint) ||
		SpecFingerprint(spec) != registeredSpecFingerprint || probe.SpecFingerprint != registeredSpecFingerprint ||
		probe.CatalogFingerprint != registeredCatalogFingerprint {
		return false
	}
	if probe.ConnectionEpoch != m.connectionEpochs[serverID] || probe.ServerIdentity != m.serverIdentities[serverID] || probe.ThreadID != context.ThreadID || probe.TurnID != context.TurnID ||
		probe.ContextEpoch != context.ContextEpoch || probe.ProbeContextDigest != context.ContextDigest ||
		probe.CaseID != context.CaseID || probe.CaseBindingHash != context.CaseBindingHash {
		return false
	}
	if probe.DatasetSnapshotID != context.DatasetSnapshotID {
		return false
	}
	admission, ok := m.sourceAdmissions[sourceProbeRegistryKey(serverID, context.ThreadID, context.TurnID)]
	return ok && reflect.DeepEqual(admission.Context, context) && reflect.DeepEqual(admission.Probe, probe) &&
		m.sourceEvidenceAdmissionBoundToContextNoLock(serverID, admission, context)
}

func (m *ProductionManager) sourceEvidenceAdmissionBoundToContextNoLock(
	serverID string,
	admission sourceEvidenceAdmissionV2,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	return admission.Probe.ServerID == serverID &&
		reflect.DeepEqual(admission.Context, securityContext) &&
		validateCurrentSourceBindingV2(securityContext, admission.Binding) == nil &&
		datasetSelectionMatchesSourceContextV2(admission.Selection, securityContext) &&
		m.sourceProbeConnectionMatchesNoLock(serverID, admission.Probe) &&
		m.fundsCountCanaryObservationCurrentNoLock(admission)
}

func (m *ProductionManager) fundsCountCanaryObservationCurrentNoLock(
	admission sourceEvidenceAdmissionV2,
) bool {
	observation := admission.CountCanary
	if m == nil || admission.Probe.ServerID != "analytix_funds" ||
		!domainsecurity.IsSHA256Hex(observation.projectionDigest) ||
		!domainsecurity.IsSHA256Hex(observation.rawResultSHA256) ||
		observation.grantGeneration == 0 || observation.connectionEpoch == 0 ||
		observation.connectionEpoch != admission.Probe.ConnectionEpoch {
		return false
	}
	projection, err := fundsCountProjectionForSelectionV2(admission.Context, admission.Selection)
	if err != nil || projection.ProjectionDigest != observation.projectionDigest ||
		m.hostFundsSourceReadLifecycle == nil || !m.hostFundsSourceReadGrantedNoLock() {
		return false
	}
	grant, ok := m.hostFundsSourceReadLifecycle.CurrentGrant()
	return ok && grant.Generation() == observation.grantGeneration &&
		grant.ConnectionEpoch() == observation.connectionEpoch
}

func (m *ProductionManager) sourceProbeForContextNoLock(serverID string, context domainsecurity.TurnSecurityContext) (domainsecurity.VerifiedSourceProbe, bool) {
	probe, ok := m.sourceProbes[sourceProbeRegistryKey(serverID, context.ThreadID, context.TurnID)]
	return probe, ok
}

func (m *ProductionManager) sourceProbeCountNoLock(serverID string) int {
	count := 0
	for _, admission := range m.sourceAdmissions {
		if admission.Probe.ServerID == strings.TrimSpace(serverID) &&
			m.sourceEvidenceAdmissionBoundToContextNoLock(serverID, admission, admission.Context) {
			count++
		}
	}
	return count
}

func sourceProbeRegistryKey(serverID string, threadID string, turnID string) string {
	return strings.TrimSpace(serverID) + "\x00" + strings.TrimSpace(threadID) + "\x00" + strings.TrimSpace(turnID)
}

func (m *ProductionManager) Prompts() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]any, 0, len(m.prompts))
	for _, prompt := range m.prompts {
		record := map[string]any{
			"serverId": prompt.ServerID,
			"name":     prompt.Name,
		}
		if prompt.Description != "" {
			record["description"] = prompt.Description
		}
		if len(prompt.Arguments) > 0 {
			args := make([]any, 0, len(prompt.Arguments))
			for _, argument := range prompt.Arguments {
				if argument.Name == "" {
					continue
				}
				arg := map[string]any{
					"name":     argument.Name,
					"required": argument.Required,
				}
				if argument.Description != "" {
					arg["description"] = argument.Description
				}
				args = append(args, arg)
			}
			record["arguments"] = args
		}
		out = append(out, record)
	}
	return out
}

func (m *ProductionManager) Resources() []any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]any, 0, len(m.resources))
	for _, resource := range m.resources {
		record := map[string]any{
			"serverId": resource.ServerID,
			"uri":      resource.URI,
		}
		if resource.Name != "" {
			record["name"] = resource.Name
		}
		if resource.Description != "" {
			record["description"] = resource.Description
		}
		if resource.MimeType != "" {
			record["mimeType"] = resource.MimeType
		}
		out = append(out, record)
	}
	return out
}

func (m *ProductionManager) ToolReadOnlyHint(toolName string) bool {
	m.mu.Lock()
	name := strings.TrimSpace(toolName)
	tool, ok := m.tools[name]
	if !ok || !m.managedToolCapabilityCurrentNoLock(name, tool) || tool.Placeholder {
		m.mu.Unlock()
		return false
	}
	spec, ok := m.specByIDNoLock(tool.ServerID)
	allowed := ok && spec.ReadOnlyToolNames[tool.RawName]
	m.mu.Unlock()
	if !allowed {
		return false
	}
	if mcpidentity.IsHighRiskServerID(spec.ID) && mcpidentity.VerifyConfiguredProvenance(spec) != nil {
		return false
	}
	return true
}

func (m *ProductionManager) ToolRemoteReadOnlyHint(toolName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(toolName)
	tool, ok := m.tools[name]
	return ok && m.managedToolCapabilityCurrentNoLock(name, tool) && tool.Tool.ReadOnlyHint
}

func (m *ProductionManager) ToolInputSchema(toolName string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(toolName)
	tool, ok := m.tools[name]
	if !ok || !m.managedToolCapabilityCurrentNoLock(name, tool) || len(tool.Tool.InputSchema) == 0 {
		return nil, false
	}
	return append(json.RawMessage(nil), tool.Tool.InputSchema...), true
}

func (m *ProductionManager) ToolOutputSchema(toolName string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(toolName)
	tool, ok := m.tools[name]
	if !ok || !m.managedToolCapabilityCurrentNoLock(name, tool) || len(tool.Tool.OutputSchema) == 0 {
		return nil, false
	}
	return append(json.RawMessage(nil), tool.Tool.OutputSchema...), true
}

// ToolTaskSupport returns the normalized, host-private MCP execution mode
// bound into advertisement and ExecutionGrant schema hashes.
func (m *ProductionManager) ToolTaskSupport(toolName string) (domainmcp.ToolTaskSupport, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tool, ok := m.tools[toolName]
	if !ok || !m.managedToolCapabilityCurrentNoLock(toolName, tool) {
		return "", false
	}
	taskSupport, valid := domainmcp.NormalizeToolTaskSupport(tool.Tool.TaskSupport)
	return taskSupport, valid
}

func (m *ProductionManager) ToolConnectionEpoch(toolName string) (uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(toolName)
	tool, ok := m.tools[name]
	if !ok || !m.managedToolCapabilityCurrentNoLock(name, tool) || tool.Placeholder || m.clients[tool.ServerID] == nil {
		return 0, false
	}
	epoch := m.connectionEpochs[tool.ServerID]
	return epoch, epoch > 0
}

func (m *ProductionManager) ToolServerIdentity(toolName string, context domainsecurity.TurnSecurityContext) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(toolName)
	tool, ok := m.tools[name]
	if !ok || !m.managedToolCapabilityCurrentNoLock(name, tool) || tool.Placeholder || m.clients[tool.ServerID] == nil || m.connectionEpochs[tool.ServerID] == 0 {
		return "", false
	}
	spec, ok := m.specByIDNoLock(tool.ServerID)
	if !ok || !mcpServerSpecAuthorizedForSecurityContext(spec, &context) {
		return "", false
	}
	identity := m.serverIdentities[tool.ServerID]
	if identity == "" {
		return "", false
	}
	if mcpidentity.IsHighRiskServerID(tool.ServerID) {
		probe, ok := m.sourceProbeForContextNoLock(tool.ServerID, context)
		if !ok || !m.sourceProbeBoundToContextNoLock(tool.ServerID, probe, context) || probe.ServerIdentity != identity {
			return "", false
		}
		identity = probe.ServerIdentity
	}
	return identity, true
}

func verifiedMCPServerIdentity(serverID string, observed domainmcp.ServerIdentity, connectionInstanceID string, epoch uint64) string {
	if _, _, ok := domainmcpname.Parse("mcp__" + serverID + "__identity"); !ok || !domainmcpprotocol.SupportedVersion(observed.ProtocolVersion) ||
		strings.TrimSpace(observed.Name) == "" || strings.TrimSpace(observed.Name) != observed.Name ||
		strings.TrimSpace(observed.Version) == "" || strings.TrimSpace(observed.Version) != observed.Version || epoch == 0 {
		return ""
	}
	identity, err := domainsecurity.NewVerifiedMCPServerIdentityForProtocol(serverID, observed.Name, observed.Version, observed.ProtocolVersion, connectionInstanceID, epoch)
	if err != nil {
		return ""
	}
	return identity
}

func newMCPConnectionInstanceID() string {
	body := make([]byte, sha256.Size)
	if _, err := cryptorand.Read(body); err != nil {
		return ""
	}
	return hex.EncodeToString(body)
}

func (m *ProductionManager) ToolDescription(toolName string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.TrimSpace(toolName)
	tool, ok := m.tools[name]
	if !ok || !m.managedToolCapabilityCurrentNoLock(name, tool) || strings.TrimSpace(tool.Tool.Description) == "" {
		return "", false
	}
	return strings.TrimSpace(tool.Tool.Description), true
}

func (m *ProductionManager) sortedToolsNoLock() []string {
	names := make([]string, 0, len(m.tools))
	for name := range m.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *ProductionManager) lazyPlaceholderCountNoLock() int {
	count := 0
	for _, tool := range m.tools {
		if tool.Placeholder {
			count++
		}
	}
	return count
}

func (m *ProductionManager) serverHasLazyPlaceholderNoLock(serverID string) bool {
	for _, tool := range m.tools {
		if tool.ServerID == serverID && tool.Placeholder {
			return true
		}
	}
	return false
}

func (m *ProductionManager) serverPromptCountNoLock(serverID string) int {
	count := 0
	for _, prompt := range m.prompts {
		if prompt.ServerID == serverID {
			count++
		}
	}
	return count
}

func (m *ProductionManager) serverResourceCountNoLock(serverID string) int {
	count := 0
	for _, resource := range m.resources {
		if resource.ServerID == serverID {
			count++
		}
	}
	return count
}

func (m *ProductionManager) diagnosticsNoLock() map[string]any {
	toolNames := m.sortedToolsNoLock()
	configured := len(m.normalizeProductionSpecsNoLock())
	connected := len(m.clients)
	cached := len(m.cachedSchemas)
	placeholders := m.lazyPlaceholderCountNoLock()
	quarantinedTools := 0
	for _, issues := range m.toolContractIssues {
		quarantinedTools += len(issues)
	}
	status := "unavailable"
	reason := "MCP servers are not configured"
	if configured > 0 {
		reason = "configured MCP servers are not connected"
	}
	if connected > 0 {
		status = "available"
		reason = "configured MCP servers are connected"
	} else if cached > 0 {
		status = "cached"
		reason = "cached MCP schemas are available; transports will reconnect on first use"
	} else if placeholders > 0 {
		status = "lazy"
		reason = "lazy MCP placeholder tools are available; transports will connect on first use"
	}
	return map[string]any{
		"enabled":                        configured > 0,
		"mode":                           "auto",
		"active":                         connected > 0,
		"available":                      connected > 0,
		"schemaHintAvailable":            cached > 0,
		"connectable":                    cached > 0 || placeholders > 0,
		"status":                         status,
		"reason":                         reason,
		"configuredServerCount":          float64(configured),
		"connectedServerCount":           float64(connected),
		"cachedSchemaHitCount":           float64(cached),
		"lazyPlaceholderCount":           float64(placeholders),
		"schemaCacheEnabled":             strings.TrimSpace(m.cacheDir) != "",
		"indexedToolCount":               float64(len(toolNames)),
		"advertisedToolCount":            float64(len(m.liveToolsNoLock())),
		"promptCount":                    float64(len(m.prompts)),
		"resourceCount":                  float64(len(m.resources)),
		"toolContractQuarantineCount":    float64(quarantinedTools),
		"topKDefault":                    float64(5),
		"topKMax":                        float64(20),
		"minScore":                       float64(0),
		"lastRefreshedAt":                m.lastRefreshedAt,
		"catalogFingerprint":             m.catalogHash,
		"catalogDrift":                   m.catalogDrift,
		"refreshCount":                   float64(m.refreshes),
		"restartReconnectCount":          float64(m.restarts),
		"failureCount":                   float64(len(m.failures)),
		"topLevelMCPIndexerRouteExposed": false,
	}
}

func (m *ProductionManager) liveToolsNoLock() []string {
	out := make([]string, 0, len(m.tools))
	for name, tool := range m.tools {
		if tool.Placeholder {
			continue
		}
		if !m.managedToolCapabilityCurrentNoLock(name, tool) {
			continue
		}
		if _, connected := m.clients[tool.ServerID]; connected {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

type rawMCPServerSpecV1 struct {
	Type                  string                               `json:"type"`
	Transport             string                               `json:"transport"`
	Command               string                               `json:"command"`
	Args                  []string                             `json:"args"`
	Env                   map[string]string                    `json:"env"`
	URL                   string                               `json:"url"`
	Headers               map[string]string                    `json:"headers"`
	AccountCredential     *domainmcp.AccountCredentialScope    `json:"accountCredential"`
	OAuthBinding          *domainregistry.OAuthBindingMetadata `json:"oauthBinding"`
	CWD                   string                               `json:"cwd"`
	ExpectedServerName    string                               `json:"expectedServerName"`
	ExpectedServerVersion string                               `json:"expectedServerVersion"`
	IdentitySource        string                               `json:"identitySource"`
	ManifestSHA256        string                               `json:"manifestSha256"`
	EntrypointPath        string                               `json:"entrypointPath"`
	EntrypointSHA256      string                               `json:"entrypointSha256"`
	PluginRootPath        string                               `json:"pluginRootPath"`
	SourceTreeSHA256      string                               `json:"sourceTreeSha256"`
	TrustScope            string                               `json:"trustScope"`
	TrustedWorkspaceRoots []string                             `json:"trustedWorkspaceRoots"`
	ExternalReadOnlyNames json.RawMessage                      `json:"readOnlyToolNames"`
	Low                   bool                                 `json:"lowPriority"`
	Background            bool                                 `json:"backgroundStart"`
	TimeoutMS             *int64                               `json:"timeoutMs"`
	Enabled               *bool                                `json:"enabled"`
	Disabled              *bool                                `json:"disabled"`
}

var allowedMCPServerConfigFieldsV1 = map[string]struct{}{
	"type": {}, "transport": {}, "command": {}, "args": {}, "env": {}, "url": {}, "headers": {}, "cwd": {},
	"expectedServerName": {}, "expectedServerVersion": {}, "identitySource": {}, "manifestSha256": {},
	"entrypointPath": {}, "entrypointSha256": {}, "pluginRootPath": {}, "sourceTreeSha256": {},
	"trustScope": {}, "trustedWorkspaceRoots": {}, "readOnlyToolNames": {}, "lowPriority": {},
	"backgroundStart": {}, "timeoutMs": {}, "enabled": {}, "disabled": {}, "accountCredential": {},
	"oauthBinding": {},
}

func LoadMCPJSON(path string, workspaceRoot string) ([]ServerSpec, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("MCP configuration path is not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(domainruntimeconfig.MaxJSONDocumentBytesV1)+1))
	if err != nil {
		return nil, err
	}
	return LoadMCPJSONDocument(data, workspaceRoot)
}

func LoadMCPJSONDocument(data []byte, workspaceRoot string) ([]ServerSpec, error) {
	normalized, err := domainruntimeconfig.NormalizeAndValidateJSONDocumentV1(data)
	if err != nil {
		return nil, err
	}
	servers, err := mcpServerObjectsV1(normalized)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(servers))
	parsed := make(map[string]rawMCPServerSpecV1, len(servers))
	for name, body := range servers {
		if _, err := domainmcpname.Namespace(name); err != nil {
			return nil, errors.New("MCP server id is not canonical")
		}
		raw, disabled, err := decodeMCPServerSpecV1(body)
		if err != nil {
			return nil, err
		}
		if disabled {
			continue
		}
		parsed[name] = raw
		names = append(names, name)
	}
	sort.Strings(names)
	specs := make([]ServerSpec, 0, len(names))
	for _, name := range names {
		raw := parsed[name]
		trustedWorkspaceRoots := normalizeMCPTrustedWorkspaceRoots(raw.TrustedWorkspaceRoots)
		trustScope := normalizeMCPTrustScope(raw.TrustScope, trustedWorkspaceRoots)
		transport, err := resolveMCPConfigTransportV1(raw)
		if err != nil {
			return nil, err
		}
		timeoutMS := domainmcp.DefaultServerTimeoutMSV1
		if raw.TimeoutMS != nil {
			timeoutMS = *raw.TimeoutMS
		}
		spec := ServerSpec{
			ID:                    name,
			Transport:             transport,
			Command:               raw.Command,
			Args:                  append([]string(nil), raw.Args...),
			Env:                   cloneStringMap(raw.Env),
			URL:                   raw.URL,
			Headers:               cloneStringMap(raw.Headers),
			AccountCredential:     cloneMCPAccountCredentialScope(raw.AccountCredential),
			OAuthBinding:          cloneMCPOAuthBinding(raw.OAuthBinding),
			CWD:                   raw.CWD,
			ExpectedServerName:    raw.ExpectedServerName,
			ExpectedServerVersion: raw.ExpectedServerVersion,
			IdentitySource:        raw.IdentitySource,
			ManifestSHA256:        raw.ManifestSHA256,
			EntrypointPath:        raw.EntrypointPath,
			EntrypointSHA256:      raw.EntrypointSHA256,
			PluginRootPath:        raw.PluginRootPath,
			SourceTreeSHA256:      raw.SourceTreeSHA256,
			TrustScope:            trustScope,
			TrustedWorkspaceRoots: trustedWorkspaceRoots,
			LowPriority:           raw.Low,
			BackgroundStart:       raw.Background,
			TimeoutMS:             timeoutMS,
		}
		if spec.AccountCredential != nil {
			if transport != "http" && transport != "streamable-http" && transport != "sse" {
				return nil, errors.New("MCP OAuth account credentials require an HTTP transport")
			}
			if !validMCPAccountCredentialBinding(spec) || hasMCPAuthorizationHeader(spec.Headers) {
				return nil, errors.New("MCP OAuth account credential binding is invalid")
			}
		}
		if spec.OAuthBinding != nil && (spec.AccountCredential == nil || spec.OAuthBinding.Validate() != nil) {
			return nil, errors.New("MCP OAuth account binding metadata is invalid")
		}
		if reservedCaseFactMCPServerID(spec.ID) {
			spec = ServerSpec{ID: strings.TrimSpace(spec.ID), Transport: caseFactHostQuarantineTransport}
		}
		specs = append(specs, ApplyKnownOverrides(spec, workspaceRoot))
	}
	return specs, nil
}

func mcpServerObjectsV1(data []byte) (map[string]json.RawMessage, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	candidates := make([]json.RawMessage, 0, 3)
	if raw, ok := top["mcpServers"]; ok {
		candidates = append(candidates, raw)
	}
	if raw, ok := top["servers"]; ok {
		candidates = append(candidates, raw)
	}
	if raw, ok := top["capabilities"]; ok {
		capabilities, err := decodeMCPObjectV1(raw, "MCP capabilities must be an object")
		if err != nil {
			return nil, err
		}
		if rawMCP, ok := capabilities["mcp"]; ok {
			mcpConfig, err := decodeMCPObjectV1(rawMCP, "MCP capability must be an object")
			if err != nil {
				return nil, err
			}
			if rawServers, ok := mcpConfig["servers"]; ok {
				candidates = append(candidates, rawServers)
			}
		}
	}
	if len(candidates) > 1 {
		return nil, errors.New("MCP server containers are ambiguous")
	}
	if len(candidates) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	return decodeMCPObjectV1(candidates[0], "MCP servers must be an object")
}

func decodeMCPObjectV1(raw json.RawMessage, message string) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New(message)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, errors.New(message)
	}
	return object, nil
}

func decodeMCPServerSpecV1(body json.RawMessage) (rawMCPServerSpecV1, bool, error) {
	fields, err := decodeMCPObjectV1(body, "MCP server configuration must be an object")
	if err != nil {
		return rawMCPServerSpecV1{}, false, err
	}
	for key, value := range fields {
		if key == "readOnlyToolNames" {
			return rawMCPServerSpecV1{}, false, errors.New("mcp readOnlyToolNames is reserved for verified host policy")
		}
		if _, allowed := allowedMCPServerConfigFieldsV1[key]; !allowed {
			return rawMCPServerSpecV1{}, false, errors.New("MCP server configuration contains an unknown field")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return rawMCPServerSpecV1{}, false, errors.New("MCP server configuration contains null")
		}
	}
	if _, enabled := fields["enabled"]; enabled {
		if _, disabled := fields["disabled"]; disabled {
			return rawMCPServerSpecV1{}, false, errors.New("MCP server enablement is ambiguous")
		}
	}
	if _, transport := fields["transport"]; transport {
		if _, legacyType := fields["type"]; legacyType {
			return rawMCPServerSpecV1{}, false, errors.New("MCP server transport is ambiguous")
		}
	}
	for _, key := range []string{"args", "trustedWorkspaceRoots"} {
		if value, ok := fields[key]; ok {
			if err := validateMCPStringArrayV1(value, key == "trustedWorkspaceRoots"); err != nil {
				return rawMCPServerSpecV1{}, false, err
			}
		}
	}
	for _, key := range []string{"env", "headers"} {
		if value, ok := fields[key]; ok {
			if err := validateMCPStringRecordV1(value); err != nil {
				return rawMCPServerSpecV1{}, false, err
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var raw rawMCPServerSpecV1
	if err := decoder.Decode(&raw); err != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return rawMCPServerSpecV1{}, false, errors.New("MCP server configuration is invalid")
	}
	for _, item := range []struct {
		key   string
		value string
	}{
		{"type", raw.Type}, {"transport", raw.Transport}, {"command", raw.Command}, {"url", raw.URL}, {"cwd", raw.CWD},
		{"expectedServerName", raw.ExpectedServerName}, {"expectedServerVersion", raw.ExpectedServerVersion},
		{"identitySource", raw.IdentitySource}, {"manifestSha256", raw.ManifestSHA256}, {"entrypointPath", raw.EntrypointPath},
		{"entrypointSha256", raw.EntrypointSHA256}, {"pluginRootPath", raw.PluginRootPath}, {"sourceTreeSha256", raw.SourceTreeSHA256},
		{"trustScope", raw.TrustScope},
	} {
		if _, present := fields[item.key]; present && (item.value == "" || item.value != strings.TrimSpace(item.value)) {
			return rawMCPServerSpecV1{}, false, errors.New("MCP server string field is invalid")
		}
	}
	if raw.IdentitySource != "" && raw.IdentitySource != "installed-plugin-manifest" {
		return rawMCPServerSpecV1{}, false, errors.New("MCP server identity source is invalid")
	}
	for _, digest := range []string{raw.ManifestSHA256, raw.EntrypointSHA256, raw.SourceTreeSHA256} {
		if digest != "" && !domainsecurity.IsSHA256Hex(digest) {
			return rawMCPServerSpecV1{}, false, errors.New("MCP server digest is invalid")
		}
	}
	if raw.TrustScope != "" && raw.TrustScope != "user" && raw.TrustScope != "workspace" {
		return rawMCPServerSpecV1{}, false, errors.New("MCP server trust scope is invalid")
	}
	if raw.TimeoutMS != nil {
		if *raw.TimeoutMS <= 0 {
			return rawMCPServerSpecV1{}, false, errors.New("MCP server timeout is invalid")
		}
		if _, err := domainmcp.ResolveServerTimeoutV1(*raw.TimeoutMS); err != nil {
			return rawMCPServerSpecV1{}, false, err
		}
	}
	if _, err := resolveMCPConfigTransportV1(raw); err != nil {
		return rawMCPServerSpecV1{}, false, err
	}
	trustedRoots := normalizeMCPTrustedWorkspaceRoots(raw.TrustedWorkspaceRoots)
	if raw.TrustScope == "" && len(trustedRoots) == 0 {
		return rawMCPServerSpecV1{}, false, errors.New("MCP server trust scope is required")
	}
	if normalizeMCPTrustScope(raw.TrustScope, trustedRoots) == "workspace" && len(trustedRoots) == 0 {
		return rawMCPServerSpecV1{}, false, errors.New("workspace MCP server requires a trusted root")
	}
	disabled := raw.Disabled != nil && *raw.Disabled || raw.Enabled != nil && !*raw.Enabled
	return raw, disabled, nil
}

func validateMCPStringArrayV1(raw json.RawMessage, requireNonEmpty bool) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return errors.New("MCP server string array is invalid")
	}
	for _, item := range items {
		trimmed := bytes.TrimSpace(item)
		if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
			return errors.New("MCP server string array is invalid")
		}
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil || requireNonEmpty && (value == "" || value != strings.TrimSpace(value)) {
			return errors.New("MCP server string array is invalid")
		}
	}
	return nil
}

func validateMCPStringRecordV1(raw json.RawMessage) error {
	record, err := decodeMCPObjectV1(raw, "MCP server string record is invalid")
	if err != nil {
		return err
	}
	for _, item := range record {
		trimmed := bytes.TrimSpace(item)
		if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
			return errors.New("MCP server string record is invalid")
		}
	}
	return nil
}

func resolveMCPConfigTransportV1(raw rawMCPServerSpecV1) (string, error) {
	transport := raw.Transport
	if transport == "" {
		transport = raw.Type
	}
	if transport == "http" {
		transport = "streamable-http"
	}
	if transport == "" {
		hasCommand := raw.Command != ""
		hasURL := raw.URL != ""
		if hasCommand == hasURL {
			return "", errors.New("MCP server transport inference is ambiguous")
		}
		if hasURL {
			transport = "streamable-http"
		} else {
			transport = "stdio"
		}
	}
	switch transport {
	case "stdio":
		if raw.Command == "" {
			return "", errors.New("stdio MCP server requires command")
		}
	case "streamable-http", "sse":
		if raw.URL == "" {
			return "", errors.New("HTTP MCP server requires URL")
		}
	default:
		return "", errors.New("MCP server transport is invalid")
	}
	return transport, nil
}

func ApplyKnownOverrides(spec ServerSpec, workspaceRoot string) ServerSpec {
	if strings.EqualFold(strings.TrimSpace(spec.ID), "codegraph") {
		if strings.TrimSpace(spec.CWD) == "" {
			spec.CWD = strings.TrimSpace(workspaceRoot)
		}
		if spec.Env == nil {
			spec.Env = map[string]string{}
		}
		if _, ok := spec.Env["CODEGRAPH_DAEMON_IDLE_TIMEOUT_MS"]; !ok {
			spec.Env["CODEGRAPH_DAEMON_IDLE_TIMEOUT_MS"] = "5000"
		}
		spec.LowPriority = true
		spec.BackgroundStart = true
	}
	return spec
}

func normalizeMCPTrustScope(value string, trustedWorkspaceRoots []string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "user":
		return "user"
	case "workspace":
		return "workspace"
	default:
		if len(trustedWorkspaceRoots) > 0 {
			return "workspace"
		}
		return "user"
	}
}

func normalizeMCPTrustedWorkspaceRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, root := range roots {
		normalized := normalizeMCPTrustPath(root)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

func mcpWorkspaceRootTrusted(workspaceRoot string, trustedWorkspaceRoots []string) bool {
	workspace := normalizeMCPTrustPath(workspaceRoot)
	if workspace == "" {
		return false
	}
	for _, root := range trustedWorkspaceRoots {
		normalizedRoot := normalizeMCPTrustPath(root)
		if normalizedRoot == "" {
			continue
		}
		if workspace == normalizedRoot || strings.HasPrefix(workspace, normalizedRoot+"/") {
			return true
		}
	}
	return false
}

func mcpServerSpecAuthorizedForSecurityContext(spec ServerSpec, securityContext *domainsecurity.TurnSecurityContext) bool {
	if securityContext != nil && domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(*securityContext) != nil {
		return false
	}
	switch spec.TrustScope {
	case "user":
		return true
	case "workspace":
		return securityContext != nil && mcpServerSpecAuthorizedForWorkspace(spec, securityContext.WorkspaceRealPath)
	default:
		return false
	}
}

func mcpServerSpecAuthorizedForWorkspace(spec ServerSpec, workspaceRealPath string) bool {
	switch spec.TrustScope {
	case "user":
		return true
	case "workspace":
		return mcpWorkspaceRootTrusted(workspaceRealPath, spec.TrustedWorkspaceRoots)
	default:
		return false
	}
}

func normalizeMCPTrustPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "/")
	normalized := filepath.ToSlash(filepath.Clean(value))
	if normalized == "." {
		return ""
	}
	for len(normalized) > 1 && strings.HasSuffix(normalized, "/") && !strings.HasSuffix(normalized, ":/") {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	return normalized
}

func (m *ProductionManager) newMCPTransportClientNoLock(spec ServerSpec, specHash string) (mcpTransportClient, error) {
	if reservedCaseFactMCPServerID(spec.ID) || mcpidentity.IsHighRiskServerID(spec.ID) {
		if m == nil || m.hostFundsServer == nil || m.hostFundsFingerprint == "" ||
			spec.ID != "analytix_funds" || specHash != m.hostFundsFingerprint ||
			SpecFingerprint(spec) != m.hostFundsFingerprint {
			return nil, errors.New("high-risk funds MCP requires an exact host-owned in-process binding")
		}
		return newHostFundsTransportClientV1(m.hostFundsServer, spec, specHash, m.accountFlowExecutor != nil)
	}
	if spec.AccountCredential == nil {
		return newMCPTransportClient(spec, m.proxyURL, m.protectedReadDirs)
	}
	if m == nil || m.accountCredentialResolver == nil || !validMCPAccountCredentialBinding(spec) ||
		hasMCPAuthorizationHeader(spec.Headers) {
		return nil, errors.New("MCP OAuth account credential is unavailable")
	}
	return &mcpAccountCredentialBoundClient{
		spec: cloneServerSpecForAuthority(spec), proxyURL: m.proxyURL,
		protectedReadDirs: append([]string(nil), m.protectedReadDirs...),
		resolver:          m.accountCredentialResolver, newTransport: newMCPTransportClient,
	}, nil
}

func newMCPTransportClient(
	spec ServerSpec,
	proxyURL string,
	protectedReadDirs []string,
) (mcpTransportClient, error) {
	if reservedCaseFactMCPServerID(spec.ID) || mcpidentity.IsHighRiskServerID(spec.ID) {
		return nil, errors.New("high-risk funds MCP cannot use a generic transport")
	}
	transport := strings.ToLower(strings.TrimSpace(spec.Transport))
	switch transport {
	case "http", "streamable-http", "sse":
		if strings.TrimSpace(spec.URL) == "" {
			return nil, errors.New("mcp http transport requires url")
		}
		return newHTTPMCPTransportClient(spec, proxyURL)
	case "", "stdio":
		if strings.TrimSpace(spec.Command) == "" {
			return nil, errors.New("mcp stdio transport requires command")
		}
		return mcpstdio.NewTransportClientWithFilesystemPolicy(
			spec,
			mcpstdio.ProcessFilesystemPolicy{
				ProtectedReadDirs:    protectedReadDirs,
				AllowLoopbackTCPPort: hostScheduleLoopbackPortV1(spec),
			},
		)
	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", spec.Transport)
	}
}

var newHTTPMCPTransportClient = httpmcp.NewTransportClient

// UseLoopbackHTTPTransportForTests installs the adapter's test-only loopback
// constructor after the adapter mints an opaque capability from the calling Go
// test frame. Production callers cannot mint that capability.
func UseLoopbackHTTPTransportForTests() error {
	authority, err := httpmcp.AuthorizeLoopbackTransportForTests()
	if err != nil {
		return err
	}
	newHTTPMCPTransportClient = func(spec domainmcp.ServerSpec, proxyURL string) (*httpmcp.TransportClient, error) {
		return httpmcp.NewLoopbackTransportClientForTests(authority, spec, proxyURL)
	}
	return nil
}

func parseJSONRPCResponse(data []byte) (json.RawMessage, error) {
	return mcpprotocol.ParseJSONRPCResponse(data)
}

func parseSSEJSONRPCResponse(data []byte, expectedID int) (json.RawMessage, error) {
	return mcpprotocol.ParseSSEJSONRPCResponse(data, expectedID)
}

func parseJSONRPCResponseWithID(data []byte) (json.RawMessage, mcpprotocol.ResponseID, error) {
	return mcpprotocol.ParseJSONRPCResponseWithID(data)
}

func parseMCPServerCapabilities(result json.RawMessage) mcpServerCapabilities {
	return mcpprotocol.ParseServerCapabilities(result)
}

func parseMCPTools(result json.RawMessage) ([]ToolSpec, error) {
	return mcpprotocol.ParseTools(result)
}

func parseMCPPrompts(result json.RawMessage) []PromptSpec {
	return mcpprotocol.ParsePrompts(result)
}

func parseMCPResources(result json.RawMessage) []ResourceSpec {
	return mcpprotocol.ParseResources(result)
}

func extractMCPToolResult(result json.RawMessage) any {
	return mcpprotocol.ExtractToolResult(result)
}

func canonicalJSONBytes(body []byte) string {
	return mcpprotocol.CanonicalJSONSchema(body)
}

func normalizeProductionSpecs(specs []ServerSpec) []ServerSpec {
	return normalizeProductionSpecsForAuthority(specs, false)
}

func normalizeProductionSpecsForAuthority(specs []ServerSpec, allowCaseFactSource bool) []ServerSpec {
	out := make([]ServerSpec, 0, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.ID) == "" {
			continue
		}
		if reservedCaseFactMCPServerID(spec.ID) && !allowCaseFactSource {
			spec = ServerSpec{ID: strings.TrimSpace(spec.ID), Transport: caseFactHostQuarantineTransport}
		}
		if strings.TrimSpace(spec.Transport) == "" {
			spec.Transport = "stdio"
		}
		spec.Env = cloneStringMap(spec.Env)
		spec.Headers = cloneStringMap(spec.Headers)
		spec.AccountCredential = cloneMCPAccountCredentialScope(spec.AccountCredential)
		spec.OAuthBinding = cloneMCPOAuthBinding(spec.OAuthBinding)
		spec.TrustedWorkspaceRoots = normalizeMCPTrustedWorkspaceRoots(spec.TrustedWorkspaceRoots)
		spec.TrustScope = normalizeMCPTrustScope(spec.TrustScope, spec.TrustedWorkspaceRoots)
		spec.ReadOnlyToolNames = cloneBoolMap(spec.ReadOnlyToolNames)
		out = append(out, spec)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func reservedCaseFactMCPServerID(value string) bool {
	normalized := strings.NewReplacer("_", "-", " ", "").Replace(strings.ToLower(strings.TrimSpace(value)))
	return normalized == "analytix-funds" || normalized == "analytix-fund-analysis"
}

func cloneServerSpecForAuthority(spec ServerSpec) ServerSpec {
	spec.Args = append([]string(nil), spec.Args...)
	spec.Env = cloneStringMap(spec.Env)
	spec.Headers = cloneStringMap(spec.Headers)
	spec.AccountCredential = cloneMCPAccountCredentialScope(spec.AccountCredential)
	spec.OAuthBinding = cloneMCPOAuthBinding(spec.OAuthBinding)
	spec.TrustedWorkspaceRoots = append([]string(nil), spec.TrustedWorkspaceRoots...)
	spec.ReadOnlyToolNames = cloneBoolMap(spec.ReadOnlyToolNames)
	spec.Tools = append([]ToolSpec(nil), spec.Tools...)
	for index := range spec.Tools {
		spec.Tools[index].InputSchema = append(json.RawMessage(nil), spec.Tools[index].InputSchema...)
		spec.Tools[index].OutputSchema = append(json.RawMessage(nil), spec.Tools[index].OutputSchema...)
	}
	return spec
}

func cloneMCPAccountCredentialScope(scope *domainmcp.AccountCredentialScope) *domainmcp.AccountCredentialScope {
	if scope == nil {
		return nil
	}
	cloned := *scope
	return &cloned
}

func cloneMCPOAuthBinding(binding *domainregistry.OAuthBindingMetadata) *domainregistry.OAuthBindingMetadata {
	if binding == nil {
		return nil
	}
	cloned := *binding
	cloned.Scopes = slices.Clone(binding.Scopes)
	return &cloned
}

func specOAuthBindingUnavailable(spec ServerSpec, binding domainregistry.OAuthBindingMetadata) bool {
	return spec.OAuthBinding == nil || binding.Validate() != nil || !reflect.DeepEqual(spec.OAuthBinding, &binding)
}

func validDecimalFence(value string) bool {
	if value == "0" {
		return true
	}
	return value != "" && value[0] >= '1' && value[0] <= '9' &&
		strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) == -1
}

func validMCPAccountCredentialScope(serverID string, scope domainmcp.AccountCredentialScope) bool {
	owner := normalizedMCPAccountCredentialOwner(scope.Owner)
	provider := strings.TrimSpace(scope.Provider)
	accountID := strings.TrimSpace(scope.AccountID)
	return strings.TrimSpace(serverID) != "" && (scope.Owner == "" || owner == scope.Owner) &&
		provider == scope.Provider && accountID == scope.AccountID &&
		provider != "" && len(provider) <= 128 && accountID != "" && len(accountID) <= 256 &&
		!strings.ContainsAny(provider, "\x00\r\n\t") && !strings.ContainsAny(accountID, "\x00\r\n\t") &&
		(scope.BindingFingerprint == "" || len(scope.BindingFingerprint) == 64 &&
			strings.IndexFunc(scope.BindingFingerprint, func(r rune) bool {
				return (r < '0' || r > '9') && (r < 'a' || r > 'f')
			}) == -1) &&
		((owner == "mcp" && scope.Purpose == "mcp-oauth-access-token") ||
			(owner == "extension" && scope.Purpose == "extension-provider-account-token"))
}

func validMCPAccountCredentialBinding(spec ServerSpec) bool {
	if spec.AccountCredential == nil || !validMCPAccountCredentialScope(spec.ID, *spec.AccountCredential) {
		return false
	}
	scope := *spec.AccountCredential
	if (spec.OAuthBinding == nil) != (scope.BindingFingerprint == "") ||
		(spec.OAuthBinding != nil && spec.OAuthBinding.Validate() != nil) {
		return false
	}
	owner := normalizedMCPAccountCredentialOwner(scope.Owner)
	switch owner {
	case "mcp":
		return scope.Owner == "mcp" && scope.Provider == spec.ID &&
			strings.TrimSpace(spec.IdentitySource) != "installed-plugin-manifest"
	case "extension":
		if scope.Owner != "extension" || spec.IdentitySource != "installed-plugin-manifest" ||
			strings.TrimSpace(spec.PluginRootPath) == "" {
			return false
		}
		pluginRoot := filepath.Clean(spec.PluginRootPath)
		pluginProvider := filepath.Base(filepath.Dir(pluginRoot))
		return pluginProvider != "." && pluginProvider != string(filepath.Separator) &&
			scope.Provider == pluginProvider
	default:
		return false
	}
}

func normalizedMCPAccountCredentialOwner(owner string) string {
	if strings.TrimSpace(owner) == "" {
		return "mcp"
	}
	return strings.TrimSpace(owner)
}

func hasMCPAuthorizationHeader(headers map[string]string) bool {
	for name := range headers {
		if strings.EqualFold(strings.TrimSpace(name), "authorization") {
			return true
		}
	}
	return false
}

func productionNamespaceFailures(specs []ServerSpec) map[string]string {
	failures := map[string]string{}
	idCount := map[string]int{}
	prefixOwners := map[string]map[string]bool{}
	for _, spec := range specs {
		idCount[spec.ID]++
		prefix, err := CanonicalToolPrefixChecked(spec.ID)
		if err != nil {
			failures[spec.ID] = "MCP server ID contains an ambiguous namespace"
			continue
		}
		if prefixOwners[prefix] == nil {
			prefixOwners[prefix] = map[string]bool{}
		}
		prefixOwners[prefix][spec.ID] = true
	}
	for id, count := range idCount {
		if count > 1 {
			failures[id] = "MCP server ID is duplicated"
		}
	}
	for _, owners := range prefixOwners {
		if len(owners) < 2 {
			continue
		}
		for id := range owners {
			failures[id] = "MCP server namespace collides after normalization"
		}
	}
	return failures
}

type CachedSchema = mcpcache.Schema

func SpecFingerprint(spec ServerSpec) string {
	spec = normalizeSpecForFingerprint(spec)
	return mcpcache.SpecFingerprint(spec)
}

func loadCachedSchema(cacheDir string, serverID string, expectedHash string) (CachedSchema, bool) {
	return mcpcache.Load(cacheDir, serverID, expectedHash)
}

func saveCachedSchema(cacheDir string, serverID string, schema CachedSchema) error {
	return mcpcache.Save(cacheDir, serverID, schema)
}

func cachedSchemaPath(cacheDir string, serverID string) string {
	return mcpcache.Path(cacheDir, serverID)
}

func cacheableTools(tools []ToolSpec, spec ServerSpec) ([]ToolSpec, error) {
	return mcpcache.CacheableTools(tools, spec)
}

func normalizeSpecForFingerprint(spec ServerSpec) ServerSpec {
	if strings.TrimSpace(spec.Transport) == "" {
		spec.Transport = "stdio"
	}
	spec.Env = cloneStringMap(spec.Env)
	spec.Headers = cloneStringMap(spec.Headers)
	spec.AccountCredential = cloneMCPAccountCredentialScope(spec.AccountCredential)
	spec.TrustedWorkspaceRoots = normalizeMCPTrustedWorkspaceRoots(spec.TrustedWorkspaceRoots)
	spec.TrustScope = normalizeMCPTrustScope(spec.TrustScope, spec.TrustedWorkspaceRoots)
	spec.ReadOnlyToolNames = cloneBoolMap(spec.ReadOnlyToolNames)
	for index := range spec.Tools {
		taskSupport, ok := domainmcp.NormalizeToolTaskSupport(spec.Tools[index].TaskSupport)
		if ok {
			spec.Tools[index].TaskSupport = taskSupport
		}
	}
	return spec
}

func catalogFingerprint(tools []string) string {
	copyTools := append([]string(nil), tools...)
	sort.Strings(copyTools)
	sum := sha256.Sum256([]byte(strings.Join(copyTools, "\n")))
	return hex.EncodeToString(sum[:])
}

func sourceProbeCatalogFingerprint(tools []ToolSpec) string {
	canonical := append([]ToolSpec(nil), tools...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Name < canonical[j].Name })
	body, _ := json.Marshal(canonical)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func sourceProbeCatalogFingerprintWithIssues(tools []ToolSpec, issues []mcpprotocol.ToolContractIssue) string {
	if len(issues) == 0 {
		return sourceProbeCatalogFingerprint(tools)
	}
	canonical := append([]ToolSpec(nil), tools...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Name < canonical[j].Name })
	canonicalIssues := cloneToolContractIssues(issues)
	type issueRecord struct {
		ToolName string `json:"toolName"`
		Code     string `json:"code"`
	}
	issueRecords := make([]issueRecord, 0, len(canonicalIssues))
	for _, issue := range canonicalIssues {
		issueRecords = append(issueRecords, issueRecord{ToolName: issue.Name, Code: string(issue.Code)})
	}
	body, _ := json.Marshal(struct {
		Version     int           `json:"version"`
		Tools       []ToolSpec    `json:"tools"`
		Quarantined []issueRecord `json:"quarantined"`
	}{
		Version: 1, Tools: canonical, Quarantined: issueRecords,
	})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func catalogFingerprintForManagedTools(tools map[string]managedTool) string {
	type record struct {
		ServerID     string `json:"serverId"`
		RawName      string `json:"rawName"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		InputSchema  string `json:"inputSchema"`
		OutputSchema string `json:"outputSchema"`
		ReadOnlyHint bool   `json:"readOnlyHint"`
		TaskSupport  string `json:"taskSupport"`
		Placeholder  bool   `json:"placeholder"`
	}
	records := make([]record, 0, len(tools))
	for name, tool := range tools {
		records = append(records, record{
			ServerID:     tool.ServerID,
			RawName:      tool.RawName,
			Name:         name,
			Description:  tool.Tool.Description,
			InputSchema:  canonicalJSONBytes(tool.Tool.InputSchema),
			OutputSchema: canonicalJSONBytes(tool.Tool.OutputSchema),
			ReadOnlyHint: tool.Tool.ReadOnlyHint,
			TaskSupport:  string(tool.Tool.TaskSupport),
			Placeholder:  tool.Placeholder,
		})
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	data, _ := json.Marshal(records)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func catalogFingerprintForManagedToolsWithIssues(tools map[string]managedTool, issuesByServer map[string][]mcpprotocol.ToolContractIssue) string {
	base := catalogFingerprintForManagedTools(tools)
	type issueRecord struct {
		ServerID string `json:"serverId"`
		ToolName string `json:"toolName"`
		Code     string `json:"code"`
	}
	issues := make([]issueRecord, 0)
	for serverID, serverIssues := range issuesByServer {
		for _, issue := range cloneToolContractIssues(serverIssues) {
			issues = append(issues, issueRecord{ServerID: serverID, ToolName: issue.Name, Code: string(issue.Code)})
		}
	}
	if len(issues) == 0 {
		return base
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].ServerID != issues[j].ServerID {
			return issues[i].ServerID < issues[j].ServerID
		}
		if issues[i].ToolName != issues[j].ToolName {
			return issues[i].ToolName < issues[j].ToolName
		}
		return issues[i].Code < issues[j].Code
	})
	body, _ := json.Marshal(struct {
		Version               int           `json:"version"`
		ExecutableFingerprint string        `json:"executableFingerprint"`
		QuarantinedToolIssues []issueRecord `json:"quarantinedToolIssues"`
	}{
		Version: 1, ExecutableFingerprint: base, QuarantinedToolIssues: issues,
	})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func hasAuthConfig(spec ServerSpec) bool {
	if spec.AccountCredential != nil {
		return true
	}
	for key, value := range spec.Headers {
		if strings.TrimSpace(value) != "" && (isAuthish(key) || containsAuthMaterial(value)) {
			return true
		}
	}
	for key, value := range spec.Env {
		if strings.TrimSpace(value) != "" && (isAuthish(key) || containsAuthMaterial(value)) {
			return true
		}
	}
	return containsAuthMaterial(spec.URL)
}

func redactMCPDiagnosticText(text string, spec ServerSpec) string {
	return mcpredaction.DiagnosticText(text, spec.Headers, spec.Env, spec.URL)
}

func redactMCPCatalogText(text string) string {
	return mcpredaction.CatalogText(text)
}

type mcpAuthDiagnosis struct {
	Status string
	URL    string
}

func diagnoseMCPAuth(transport, status, errText, rawURL string, authConfigured bool) mcpAuthDiagnosis {
	if isMCPAuthFailure(errText) {
		return mcpAuthDiagnosis{Status: "required", URL: remoteMCPAuthURL(transport, rawURL)}
	}
	if authConfigured || !isRemoteMCPTransport(transport) || !looksLikeHTTPURL(rawURL) || strings.TrimSpace(errText) != "" {
		return mcpAuthDiagnosis{Status: "none"}
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "deferred", "initializing", "disabled":
		return mcpAuthDiagnosis{Status: "possible", URL: redactAuthURL(rawURL)}
	default:
		return mcpAuthDiagnosis{Status: "none"}
	}
}

func mcpServerConnectionStatus(spec ServerSpec, connected bool, cached bool, failure string) string {
	switch {
	case connected:
		return "connected"
	case strings.TrimSpace(failure) != "":
		return "failed"
	case spec.BackgroundStart:
		return "deferred"
	case cached:
		return "deferred"
	default:
		return "initializing"
	}
}

func isMCPAuthFailure(errText string) bool {
	lower := strings.ToLower(errText)
	for _, needle := range []string{
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"invalid token",
		"login required",
		"authentication",
		"not authenticated",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func remoteMCPAuthURL(transport, rawURL string) string {
	if !isRemoteMCPTransport(transport) || !looksLikeHTTPURL(rawURL) {
		return ""
	}
	return redactAuthURL(rawURL)
}

func isRemoteMCPTransport(transport string) bool {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "http", "streamable-http", "sse":
		return true
	default:
		return false
	}
}

func looksLikeHTTPURL(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}

func containsAuthMaterial(value string) bool {
	return strings.Contains(value, "${") || containsExplicitAuthMaterial(value)
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneBoolMap(input map[string]bool) map[string]bool {
	if len(input) == 0 {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
