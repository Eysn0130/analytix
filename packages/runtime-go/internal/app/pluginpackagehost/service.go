// Package pluginpackagehost serializes contribution calls with persistent
// activation transitions. Runtime composition owns one Host for these packages;
// the Host creates neither installations nor credentials and grants no release
// admission. Its development-source registrations are never caller-writable.
package pluginpackagehost

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	materializationport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

const (
	MaxInputBytes                 = 24 << 20
	MaxOutputBytes                = 24 << 20
	MaxOperations                 = 18
	WorkspaceEditorContributionID = "workspace-editor"
)

// These closed sentinels are the complete public error vocabulary. Never expose
// an underlying store, authority, adapter or filesystem error to the caller.
var (
	ErrUnavailable        = errors.New("plugin_host_unavailable")
	ErrInvalid            = errors.New("plugin_host_invalid_request")
	ErrIdentity           = errors.New("plugin_host_identity_invalid")
	ErrNotFound           = errors.New("plugin_host_package_not_found")
	ErrConflict           = errors.New("plugin_host_conflict")
	ErrDisabled           = errors.New("plugin_host_disabled")
	ErrAdapterUnavailable = errors.New("plugin_host_adapter_unavailable")
	ErrPersistence        = errors.New("plugin_host_persistence_failure")
)

type ActiveResolver interface {
	ResolveActive(context.Context) (materializationport.ResultV1, error)
}

type Registration struct {
	Identity                 domainpackage.PackageIdentityV1
	SourceRegistrationSHA256 string
	Materialization          ActiveResolver
	State                    materializationport.PackageStateStore
	Adapter                  adapterport.Adapter
	SkillReader              adapterport.SkillReader
}

type PackageView struct {
	PackageID      string `json:"packageId"`
	PackageVersion string `json:"packageVersion"`
	DisplayName    string `json:"displayName"`
	Origin         string `json:"origin"`
	Publishable    bool   `json:"publishable"`
	Materialized   bool   `json:"materialized"`
	GenerationID   string `json:"generationId"`
	// unset means no signed activation exists; unavailable means it cannot be verified.
	ActivationState    string                      `json:"activationState"`
	DesiredState       domainplugin.DesiredStateV1 `json:"desiredState,omitempty"`
	ActivationRevision uint64                      `json:"activationRevision"`
	ActivationID       string                      `json:"activationId,omitempty"`
	Available          bool                        `json:"available"`
	UnavailableReason  string                      `json:"unavailableReason,omitempty"`
	Operations         []string                    `json:"operations"`
}

type SetDesiredStateRequest struct {
	PackageID        string
	GenerationID     string
	ExpectedRevision uint64
	DesiredState     domainplugin.DesiredStateV1
}

type InvokeRequest struct {
	PackageID        string
	GenerationID     string
	ExpectedRevision uint64
	ContributionID   string
	Operation        string
	Input            json.RawMessage
}

type InvokeResult struct {
	Output json.RawMessage `json:"output"`
}

type Service struct {
	identity      identityport.Authority
	authority     materializationport.InstallationAuthority
	registrations map[string]Registration
	packageIDs    []string
	now           func() time.Time
	// Hold across readiness, verification and the complete adapter invocation.
	// Disable cannot commit between authorization and adapter start/completion.
	mu sync.Mutex
}

func New(identity identityport.Authority, authority materializationport.InstallationAuthority, registrations []Registration, now func() time.Time) (*Service, error) {
	if identity == nil || authority == nil || now == nil || len(registrations) > 3 {
		return nil, ErrUnavailable
	}
	s := &Service{identity: identity, authority: authority, registrations: make(map[string]Registration, len(registrations)), now: now}
	for _, registration := range registrations {
		if !domainpackage.ValidPackageIdentityV1(registration.Identity) || !domainpackage.ValidDevelopmentSourcePackageIDV1(registration.Identity.PackageID) ||
			!domainplugin.IsCanonicalSHA256V1(registration.SourceRegistrationSHA256) || registration.Materialization == nil || registration.State == nil {
			return nil, ErrInvalid
		}
		id := registration.Identity.PackageID
		if _, exists := s.registrations[id]; exists {
			return nil, ErrInvalid
		}
		s.registrations[id] = registration
		s.packageIDs = append(s.packageIDs, id)
	}
	sort.Strings(s.packageIDs)
	return s, nil
}

func (s *Service) principal(ctx context.Context) (domainidentity.PrincipalV1, error) {
	if s == nil || s.identity == nil || s.authority == nil || s.now == nil || ctx == nil || ctx.Err() != nil {
		return domainidentity.PrincipalV1{}, ErrUnavailable
	}
	principal, err := s.identity.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil || s.identity.ValidateCurrent(ctx, principal) != nil {
		return domainidentity.PrincipalV1{}, ErrIdentity
	}
	return principal, nil
}

func (s *Service) validatePrincipal(ctx context.Context, principal domainidentity.PrincipalV1) error {
	if ctx.Err() != nil || s.identity.ValidateCurrent(ctx, principal) != nil {
		return ErrIdentity
	}
	return nil
}

func (s *Service) resolve(ctx context.Context, registration Registration) (materializationport.ResultV1, error) {
	result, err := registration.Materialization.ResolveActive(ctx)
	receipt := result.Receipt
	if err != nil || receipt.Origin != domainplugin.DevelopmentSourceOriginV1 || receipt.PackageAuthoritySHA256 != "" ||
		receipt.SourceRegistrationSHA256 != registration.SourceRegistrationSHA256 || receipt.PluginName != registration.Identity.PackageID || receipt.PluginVersion != registration.Identity.PackageVersion ||
		domainplugin.ValidateTrustedReceiptV1(receipt, s.authority.KeyID(), s.authority.PublicKey()) != nil || domainplugin.ValidateIndexForReceiptV1(result.Index, receipt) != nil {
		return materializationport.ResultV1{}, ErrUnavailable
	}
	return result, nil
}

func (s *Service) activation(ctx context.Context, registration Registration, current materializationport.ResultV1) (domainplugin.ActivationV1, bool, error) {
	activation, err := registration.State.ReadActivation(ctx, s.authority)
	if errors.Is(err, materializationport.ErrCorrupt) {
		return domainplugin.ActivationV1{}, false, ErrUnavailable
	}
	if errors.Is(err, materializationport.ErrNotFound) {
		return domainplugin.ActivationV1{}, false, nil
	}
	if err != nil || domainplugin.ValidateTrustedActivationForReceiptV1(activation, current.Receipt, s.authority.KeyID(), s.authority.PublicKey()) != nil {
		return domainplugin.ActivationV1{}, false, ErrUnavailable
	}
	return activation, true, nil
}

func bindingFor(registration Registration, current materializationport.ResultV1, revision uint64) adapterport.Binding {
	return adapterport.Binding{PackageID: registration.Identity.PackageID, PackageVersion: registration.Identity.PackageVersion, GenerationID: current.Receipt.GenerationID, ActivationRevision: revision, SourceRegistrationSHA256: registration.SourceRegistrationSHA256}
}

func readiness(ctx context.Context, registration Registration, binding adapterport.Binding) (adapterport.Readiness, error) {
	if registration.Adapter == nil {
		return adapterport.Readiness{}, ErrAdapterUnavailable
	}
	ready, err := registration.Adapter.Readiness(ctx, binding)
	if err != nil || !ready.Available || len(ready.Operations) == 0 || len(ready.Operations) > MaxOperations {
		return adapterport.Readiness{}, ErrAdapterUnavailable
	}
	seen := make(map[string]bool, len(ready.Operations))
	for _, operation := range ready.Operations {
		if !validOperation(operation) || seen[operation] {
			return adapterport.Readiness{}, ErrAdapterUnavailable
		}
		seen[operation] = true
	}
	ready.Operations = append([]string(nil), ready.Operations...)
	return ready, nil
}

func baseView(registration Registration) PackageView {
	names := map[string]string{"analytix-documents": "Documents", "analytix-spreadsheets": "Spreadsheets", "analytix-presentations": "Presentations"}
	return PackageView{PackageID: registration.Identity.PackageID, PackageVersion: registration.Identity.PackageVersion, DisplayName: names[registration.Identity.PackageID], Origin: domainplugin.DevelopmentSourceOriginV1, ActivationState: "unavailable", UnavailableReason: "materialization_unavailable", Operations: []string{}}
}

func (s *Service) view(ctx context.Context, registration Registration) PackageView {
	view := baseView(registration)
	current, err := s.resolve(ctx, registration)
	if err != nil {
		return view
	}
	view.Materialized = true
	view.GenerationID = current.Receipt.GenerationID
	activation, exists, err := s.activation(ctx, registration, current)
	if err != nil {
		view.UnavailableReason = "activation_unavailable"
		return view
	}
	if !exists {
		view.ActivationState = "unset"
		view.UnavailableReason = "activation_unset"
		return view
	}
	view.ActivationState = "recorded"
	view.DesiredState = activation.DesiredState
	view.ActivationRevision = activation.Revision
	view.ActivationID = activation.ActivationID
	if activation.DesiredState != domainplugin.DesiredEnabledV1 {
		view.UnavailableReason = "disabled"
		return view
	}
	ready, err := readiness(ctx, registration, bindingFor(registration, current, activation.Revision))
	if err != nil {
		view.UnavailableReason = "adapter_unavailable"
		return view
	}
	view.Available = true
	view.UnavailableReason = ""
	view.Operations = ready.Operations
	return view
}

func (s *Service) List(ctx context.Context) ([]PackageView, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	principal, err := s.principal(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]PackageView, 0, len(s.packageIDs))
	for _, id := range s.packageIDs {
		views = append(views, s.view(ctx, s.registrations[id]))
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return nil, err
	}
	return views, nil
}

func (s *Service) SetDesiredState(ctx context.Context, request SetDesiredStateRequest) (PackageView, error) {
	if s == nil {
		return PackageView{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	principal, err := s.principal(ctx)
	if err != nil {
		return PackageView{}, err
	}
	if !domainplugin.IsCanonicalSHA256V1(request.GenerationID) || !domainplugin.ValidDesiredStateV1(request.DesiredState) || request.ExpectedRevision == math.MaxUint64 {
		return PackageView{}, ErrInvalid
	}
	registration, exists := s.registrations[request.PackageID]
	if !exists {
		return PackageView{}, ErrNotFound
	}
	current, err := s.resolve(ctx, registration)
	if err != nil {
		return PackageView{}, err
	}
	if current.Receipt.GenerationID != request.GenerationID {
		return PackageView{}, ErrConflict
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return PackageView{}, err
	}
	committed, err := registration.State.SetDesiredState(ctx, materializationport.SetDesiredStateRequestV1{GenerationID: request.GenerationID, ExpectedRevision: request.ExpectedRevision, DesiredState: request.DesiredState}, s.authority, s.now().UTC())
	if err != nil {
		if errors.Is(err, materializationport.ErrConflict) {
			return PackageView{}, ErrConflict
		}
		return PackageView{}, ErrPersistence
	}
	if domainplugin.ValidateTrustedActivationForReceiptV1(committed, current.Receipt, s.authority.KeyID(), s.authority.PublicKey()) != nil || committed.Revision != request.ExpectedRevision+1 || committed.DesiredState != request.DesiredState {
		return PackageView{}, ErrPersistence
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return PackageView{}, err
	}
	view := s.view(ctx, registration)
	if !view.Materialized || view.GenerationID != request.GenerationID || view.ActivationState != "recorded" || view.ActivationID != committed.ActivationID || view.ActivationRevision != committed.Revision {
		return PackageView{}, ErrPersistence
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return PackageView{}, err
	}
	return view, nil
}

func (s *Service) Invoke(ctx context.Context, request InvokeRequest) (InvokeResult, error) {
	if s == nil {
		return InvokeResult{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	principal, err := s.principal(ctx)
	if err != nil {
		return InvokeResult{}, err
	}
	if !domainplugin.IsCanonicalSHA256V1(request.GenerationID) || request.ExpectedRevision == 0 || request.ContributionID != WorkspaceEditorContributionID || !validOperation(request.Operation) || !validInput(request.Input, request.Operation) {
		return InvokeResult{}, ErrInvalid
	}
	registration, exists := s.registrations[request.PackageID]
	if !exists {
		return InvokeResult{}, ErrNotFound
	}
	current, err := s.resolve(ctx, registration)
	if err != nil {
		return InvokeResult{}, err
	}
	if current.Receipt.GenerationID != request.GenerationID {
		return InvokeResult{}, ErrConflict
	}
	activation, exists, err := s.activation(ctx, registration, current)
	if err != nil {
		return InvokeResult{}, err
	}
	if !exists || activation.DesiredState != domainplugin.DesiredEnabledV1 {
		return InvokeResult{}, ErrDisabled
	}
	if activation.Revision != request.ExpectedRevision {
		return InvokeResult{}, ErrConflict
	}
	binding := bindingFor(registration, current, activation.Revision)
	ready, err := readiness(ctx, registration, binding)
	if err != nil {
		return InvokeResult{}, err
	}
	supported := false
	for _, operation := range ready.Operations {
		if operation == request.Operation {
			supported = true
			break
		}
	}
	if !supported {
		return InvokeResult{}, ErrInvalid
	}
	// Readiness is not authority: revalidate the signed generation/source after it.
	latest, err := s.resolve(ctx, registration)
	if err != nil {
		return InvokeResult{}, err
	}
	if latest != current {
		return InvokeResult{}, ErrConflict
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return InvokeResult{}, err
	}
	result, err := registration.Adapter.Invoke(ctx, adapterport.Call{Binding: binding, Principal: principal, ContributionID: request.ContributionID, Operation: request.Operation, Input: append(json.RawMessage(nil), request.Input...)})
	if err != nil {
		return InvokeResult{}, ErrAdapterUnavailable
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return InvokeResult{}, err
	}
	if jsonstrict.Validate(result.Output, jsonstrict.Options{MaxBytes: MaxOutputBytes, MaxDepth: 32, MaxTokens: 131072, MaxStringBytes: MaxOutputBytes}) != nil {
		return InvokeResult{}, ErrAdapterUnavailable
	}
	return InvokeResult{Output: append(json.RawMessage(nil), result.Output...)}, nil
}

func validOperation(operation string) bool {
	if len(operation) == 0 || len(operation) > 64 || operation[0] < 'a' || operation[0] > 'z' {
		return false
	}
	for _, character := range operation {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func validInput(body json.RawMessage, operation string) bool {
	value, err := jsonstrict.DecodeObject(body, jsonstrict.Options{MaxBytes: MaxInputBytes, MaxDepth: 16, MaxTokens: 32768, MaxStringBytes: MaxInputBytes})
	if err != nil {
		return false
	}
	// Only this named adapter operation accepts a document selector. It cannot
	// select plugin roots or an executable, and the Office adapter strictly
	// validates this envelope before the existing Core file authority resolves it.
	if operation == "open-object" {
		selected, ok := value["object"].(map[string]any)
		if !ok || len(selected) != 2 {
			return false
		}
		workspace, workspaceOK := selected["workspace"].(string)
		path, pathOK := selected["path"].(string)
		if !workspaceOK || !pathOK || workspace == "" || path == "" {
			return false
		}
		delete(value, "object")
	}
	return !hasExecutionSelector(value)
}

// Paths and executable selectors belong to trusted composition or separate Core
// object selection, never to the generic contribution invocation envelope.
func hasExecutionSelector(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			switch strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", "")) {
			case "path", "filepath", "root", "sourceroot", "workspaceroot", "pluginroot", "activeroot", "sourcepath", "runtimehome", "datadir", "script", "scriptpath", "command", "commandline", "executable", "cwd", "args":
				return true
			}
			if hasExecutionSelector(item) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if hasExecutionSelector(item) {
				return true
			}
		}
	}
	return false
}
