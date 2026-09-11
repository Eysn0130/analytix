package plugincapability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"

	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

const (
	FundsSourceReadCapabilityIDV1      = "funds.source.read"
	FundsSourceReadProtocolVersionV1   = 1
	FundsSourceReadOperationV1         = "count_case_rows"
	FundsSourceReadPackageIDV1         = domainpluginpackage.FirstPartyFundsPackageIDV1
	FundsSourceReadDecisionSchemaV1    = "analytix.plugin-capability-decision/v1"
	FundsSourceReadScopeCaseBoundV1    = "case:bound"
	FundsSourceReadScopeSourceVerifyV1 = "source:verified"
)

var (
	ErrFundsSourceReadBindingInvalidV1   = errors.New("funds source-read admission binding is invalid")
	ErrFundsSourceReadLifecycleInvalidV1 = errors.New("funds source-read lifecycle transition is invalid")
)

var fundsSourceReadScopesV1 = [...]string{
	FundsSourceReadScopeCaseBoundV1,
	FundsSourceReadScopeSourceVerifyV1,
}

type FundsSourceReadStateV1 string

const (
	FundsSourceReadStateSetupV1    FundsSourceReadStateV1 = "setup"
	FundsSourceReadStateReadyV1    FundsSourceReadStateV1 = "ready"
	FundsSourceReadStateFailedV1   FundsSourceReadStateV1 = "failed"
	FundsSourceReadStateStoppedV1  FundsSourceReadStateV1 = "stopped"
	FundsSourceReadStateDisabledV1 FundsSourceReadStateV1 = "disabled"
	FundsSourceReadStateRevokedV1  FundsSourceReadStateV1 = "revoked"
)

type FundsSourceReadHealthV1 string

const (
	FundsSourceReadHealthUnknownV1   FundsSourceReadHealthV1 = "unknown"
	FundsSourceReadHealthHealthyV1   FundsSourceReadHealthV1 = "healthy"
	FundsSourceReadHealthUnhealthyV1 FundsSourceReadHealthV1 = "unhealthy"
	FundsSourceReadHealthStoppedV1   FundsSourceReadHealthV1 = "stopped"
	FundsSourceReadHealthDisabledV1  FundsSourceReadHealthV1 = "disabled"
)

type FundsSourceReadDecisionV1 string

const (
	FundsSourceReadDecisionPendingV1 FundsSourceReadDecisionV1 = "pending"
	FundsSourceReadDecisionGrantedV1 FundsSourceReadDecisionV1 = "granted"
	FundsSourceReadDecisionDeniedV1  FundsSourceReadDecisionV1 = "denied"
	FundsSourceReadDecisionRevokedV1 FundsSourceReadDecisionV1 = "revoked"
)

type FundsSourceReadReasonCodeV1 string

const (
	FundsSourceReadReasonSetupV1            FundsSourceReadReasonCodeV1 = "host_setup"
	FundsSourceReadReasonRequestMissingV1   FundsSourceReadReasonCodeV1 = "capability_request_missing"
	FundsSourceReadReasonRequestMismatchV1  FundsSourceReadReasonCodeV1 = "capability_request_mismatch"
	FundsSourceReadReasonAuthorityCurrentV1 FundsSourceReadReasonCodeV1 = "host_authority_current"
	FundsSourceReadReasonRevalidatingV1     FundsSourceReadReasonCodeV1 = "host_revalidating"
	FundsSourceReadReasonDisconnectedV1     FundsSourceReadReasonCodeV1 = "host_disconnected"
	FundsSourceReadReasonCatalogFailedV1    FundsSourceReadReasonCodeV1 = "host_catalog_failed"
	FundsSourceReadReasonProvenanceFailedV1 FundsSourceReadReasonCodeV1 = "host_provenance_failed"
	FundsSourceReadReasonHealthFailedV1     FundsSourceReadReasonCodeV1 = "host_health_failed"
	FundsSourceReadReasonAuthorityRevokedV1 FundsSourceReadReasonCodeV1 = "host_authority_revoked"
)

// FundsSourceReadBindingV1 is minted only by re-running static first-party
// admission over the canonical declaration bytes. Its unexported fields keep a
// package-authored decision or lifecycle value from being substituted by a
// caller.
type FundsSourceReadBindingV1 struct {
	packageID         string
	packageVersion    string
	declarationDigest string
	specFingerprint   string
	admissionDigest   string
	bindingDigest     string
	exactRequest      bool
	requestReason     FundsSourceReadReasonCodeV1
}

type fundsSourceReadAdmissionDigestV1 struct {
	SchemaVersion              int                                       `json:"schemaVersion"`
	Identity                   domainpluginpackage.PackageIdentityV1     `json:"identity"`
	DeclarationRawSHA256       string                                    `json:"declarationRawSha256"`
	DeclarationCanonicalSHA256 string                                    `json:"declarationCanonicalSha256"`
	RequestedCapabilities      []domainpluginpackage.CapabilityRequestV1 `json:"requestedCapabilities"`
	Lifecycle                  domainpluginpackage.LifecycleV1           `json:"lifecycle"`
}

type fundsSourceReadBindingDigestV1 struct {
	SchemaVersion     int      `json:"schemaVersion"`
	PackageID         string   `json:"packageId"`
	PackageVersion    string   `json:"packageVersion"`
	DeclarationSHA256 string   `json:"declarationSha256"`
	SpecFingerprint   string   `json:"specFingerprint"`
	CapabilityID      string   `json:"capabilityId"`
	ProtocolVersion   int      `json:"protocolVersion"`
	ScopeConstraints  []string `json:"scopeConstraints"`
	Operation         string   `json:"operation"`
}

// BindFundsSourceReadV1 binds the exact admitted package request to one opaque
// Host Funds specification. A missing or narrowed request produces a valid but
// ineligible binding so independent Funds lanes can remain additive.
func BindFundsSourceReadV1(
	input domainpluginpackage.StaticAdmissionInputV1,
	specFingerprint string,
) (FundsSourceReadBindingV1, error) {
	decision, err := domainpluginpackage.AdmitStaticFirstPartyV1(input)
	if err != nil || !canonicalSHA256V1(specFingerprint) ||
		decision.Identity.PackageID != FundsSourceReadPackageIDV1 {
		return FundsSourceReadBindingV1{}, ErrFundsSourceReadBindingInvalidV1
	}
	admissionDigest, err := digestJSONV1(fundsSourceReadAdmissionDigestV1{
		SchemaVersion: 1, Identity: decision.Identity,
		DeclarationRawSHA256:       decision.DeclarationRawSHA256,
		DeclarationCanonicalSHA256: decision.DeclarationCanonicalSHA256,
		RequestedCapabilities:      decision.RequestedCapabilities,
		Lifecycle:                  decision.Lifecycle,
	})
	if err != nil {
		return FundsSourceReadBindingV1{}, ErrFundsSourceReadBindingInvalidV1
	}
	exactRequest := false
	reason := FundsSourceReadReasonRequestMissingV1
	for _, request := range decision.RequestedCapabilities {
		if request.ID != FundsSourceReadCapabilityIDV1 {
			continue
		}
		exactRequest = request.ProtocolVersion == FundsSourceReadProtocolVersionV1 &&
			reflect.DeepEqual(request.ScopeConstraints, fundsSourceReadScopesV1[:])
		if !exactRequest {
			reason = FundsSourceReadReasonRequestMismatchV1
		}
		break
	}
	bindingDigest, err := digestJSONV1(fundsSourceReadBindingDigestV1{
		SchemaVersion: 1,
		PackageID:     decision.Identity.PackageID, PackageVersion: decision.Identity.PackageVersion,
		DeclarationSHA256: decision.DeclarationCanonicalSHA256,
		SpecFingerprint:   specFingerprint,
		CapabilityID:      FundsSourceReadCapabilityIDV1, ProtocolVersion: FundsSourceReadProtocolVersionV1,
		ScopeConstraints: append([]string(nil), fundsSourceReadScopesV1[:]...),
		Operation:        FundsSourceReadOperationV1,
	})
	if err != nil {
		return FundsSourceReadBindingV1{}, ErrFundsSourceReadBindingInvalidV1
	}
	return FundsSourceReadBindingV1{
		packageID: decision.Identity.PackageID, packageVersion: decision.Identity.PackageVersion,
		declarationDigest: decision.DeclarationCanonicalSHA256, specFingerprint: specFingerprint,
		admissionDigest: admissionDigest, bindingDigest: bindingDigest,
		exactRequest: exactRequest, requestReason: reason,
	}, nil
}

func (binding FundsSourceReadBindingV1) Valid() bool {
	return binding.packageID == FundsSourceReadPackageIDV1 && binding.packageVersion != "" &&
		canonicalSHA256V1(binding.declarationDigest) && canonicalSHA256V1(binding.specFingerprint) &&
		canonicalSHA256V1(binding.admissionDigest) && canonicalSHA256V1(binding.bindingDigest) &&
		((binding.exactRequest && binding.requestReason == FundsSourceReadReasonRequestMissingV1) ||
			(!binding.exactRequest && (binding.requestReason == FundsSourceReadReasonRequestMissingV1 ||
				binding.requestReason == FundsSourceReadReasonRequestMismatchV1)))
}

func (binding FundsSourceReadBindingV1) ExactRequest() bool {
	return binding.Valid() && binding.exactRequest
}
func (binding FundsSourceReadBindingV1) PackageID() string {
	if !binding.Valid() {
		return ""
	}
	return binding.packageID
}
func (binding FundsSourceReadBindingV1) PackageVersion() string {
	if !binding.Valid() {
		return ""
	}
	return binding.packageVersion
}
func (binding FundsSourceReadBindingV1) BindingDigest() string {
	if !binding.Valid() {
		return ""
	}
	return binding.bindingDigest
}
func (binding FundsSourceReadBindingV1) AdmissionDigest() string {
	if !binding.Valid() {
		return ""
	}
	return binding.admissionDigest
}

type FundsSourceReadHostAuthorityV1 struct {
	SpecFingerprint      string
	ServerIdentityDigest string
	CatalogDigest        string
	ConnectionEpoch      uint64
}

func (authority FundsSourceReadHostAuthorityV1) Digest() (string, error) {
	if !canonicalSHA256V1(authority.SpecFingerprint) ||
		!canonicalSHA256V1(authority.ServerIdentityDigest) ||
		!canonicalSHA256V1(authority.CatalogDigest) || authority.ConnectionEpoch == 0 {
		return "", ErrFundsSourceReadLifecycleInvalidV1
	}
	return digestJSONV1(struct {
		SchemaVersion        int    `json:"schemaVersion"`
		SpecFingerprint      string `json:"specFingerprint"`
		ServerIdentityDigest string `json:"serverIdentityDigest"`
		CatalogDigest        string `json:"catalogDigest"`
		ConnectionEpoch      uint64 `json:"connectionEpoch"`
	}{1, authority.SpecFingerprint, authority.ServerIdentityDigest, authority.CatalogDigest, authority.ConnectionEpoch})
}

// FundsSourceReadGrantV1 is non-serializable and has no wall-clock expiry.
// Revocation is owned by lifecycle generation plus the current connection
// epoch and Host authority digest.
type FundsSourceReadGrantV1 struct {
	bindingDigest   string
	authorityDigest string
	grantDigest     string
	generation      uint64
	connectionEpoch uint64
}

func (FundsSourceReadGrantV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("funds source-read grant is host-private")
}

func (grant FundsSourceReadGrantV1) BindingDigest() string   { return grant.bindingDigest }
func (grant FundsSourceReadGrantV1) GrantDigest() string     { return grant.grantDigest }
func (grant FundsSourceReadGrantV1) Generation() uint64      { return grant.generation }
func (grant FundsSourceReadGrantV1) ConnectionEpoch() uint64 { return grant.connectionEpoch }

type FundsSourceReadDecisionEventV1 struct {
	Schema           string                      `json:"schema"`
	PackageID        string                      `json:"packageId"`
	PackageVersion   string                      `json:"packageVersion"`
	CapabilityID     string                      `json:"capabilityId"`
	ProtocolVersion  int                         `json:"protocolVersion"`
	ScopeConstraints []string                    `json:"scopeConstraints"`
	Operation        string                      `json:"operation"`
	State            FundsSourceReadStateV1      `json:"state"`
	Health           FundsSourceReadHealthV1     `json:"health"`
	Decision         FundsSourceReadDecisionV1   `json:"decision"`
	ReasonCode       FundsSourceReadReasonCodeV1 `json:"reasonCode"`
	Generation       uint64                      `json:"generation"`
	BindingDigest    string                      `json:"bindingDigest"`
	AuthorityDigest  string                      `json:"authorityDigest,omitempty"`
	GrantDigest      string                      `json:"grantDigest,omitempty"`
}

type FundsSourceReadLifecycleV1 struct {
	mu              sync.RWMutex
	binding         FundsSourceReadBindingV1
	state           FundsSourceReadStateV1
	health          FundsSourceReadHealthV1
	decision        FundsSourceReadDecisionV1
	reason          FundsSourceReadReasonCodeV1
	generation      uint64
	authorityDigest string
	grant           FundsSourceReadGrantV1
}

func NewFundsSourceReadLifecycleV1(
	binding FundsSourceReadBindingV1,
) (*FundsSourceReadLifecycleV1, FundsSourceReadDecisionEventV1, error) {
	if !binding.Valid() {
		return nil, FundsSourceReadDecisionEventV1{}, ErrFundsSourceReadBindingInvalidV1
	}
	lifecycle := &FundsSourceReadLifecycleV1{binding: binding, state: FundsSourceReadStateSetupV1,
		health: FundsSourceReadHealthUnknownV1, decision: FundsSourceReadDecisionPendingV1,
		reason: FundsSourceReadReasonSetupV1}
	if !binding.ExactRequest() {
		lifecycle.state = FundsSourceReadStateDisabledV1
		lifecycle.health = FundsSourceReadHealthDisabledV1
		lifecycle.decision = FundsSourceReadDecisionDeniedV1
		lifecycle.reason = binding.requestReason
	}
	return lifecycle, lifecycle.eventNoLock(), nil
}

func (lifecycle *FundsSourceReadLifecycleV1) BeginSetup() (FundsSourceReadDecisionEventV1, error) {
	if lifecycle == nil {
		return FundsSourceReadDecisionEventV1{}, ErrFundsSourceReadLifecycleInvalidV1
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.binding.ExactRequest() {
		return lifecycle.eventNoLock(), nil
	}
	if lifecycle.state == FundsSourceReadStateSetupV1 && lifecycle.grant.grantDigest == "" {
		return lifecycle.eventNoLock(), nil
	}
	if lifecycle.state != FundsSourceReadStateSetupV1 || lifecycle.grant.grantDigest != "" {
		lifecycle.generation++
	}
	lifecycle.state = FundsSourceReadStateSetupV1
	lifecycle.health = FundsSourceReadHealthUnknownV1
	lifecycle.decision = FundsSourceReadDecisionPendingV1
	lifecycle.reason = FundsSourceReadReasonRevalidatingV1
	lifecycle.authorityDigest = ""
	lifecycle.grant = FundsSourceReadGrantV1{}
	return lifecycle.eventNoLock(), nil
}

func (lifecycle *FundsSourceReadLifecycleV1) Activate(
	authority FundsSourceReadHostAuthorityV1,
) (FundsSourceReadGrantV1, FundsSourceReadDecisionEventV1, error) {
	if lifecycle == nil {
		return FundsSourceReadGrantV1{}, FundsSourceReadDecisionEventV1{}, ErrFundsSourceReadLifecycleInvalidV1
	}
	authorityDigest, err := authority.Digest()
	if err != nil {
		return FundsSourceReadGrantV1{}, FundsSourceReadDecisionEventV1{}, err
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.binding.ExactRequest() || lifecycle.state != FundsSourceReadStateSetupV1 {
		return FundsSourceReadGrantV1{}, lifecycle.eventNoLock(), ErrFundsSourceReadLifecycleInvalidV1
	}
	lifecycle.generation++
	grantDigest, err := digestJSONV1(struct {
		SchemaVersion   int    `json:"schemaVersion"`
		BindingDigest   string `json:"bindingDigest"`
		AuthorityDigest string `json:"authorityDigest"`
		Generation      uint64 `json:"generation"`
	}{1, lifecycle.binding.BindingDigest(), authorityDigest, lifecycle.generation})
	if err != nil {
		return FundsSourceReadGrantV1{}, lifecycle.eventNoLock(), ErrFundsSourceReadLifecycleInvalidV1
	}
	lifecycle.state = FundsSourceReadStateReadyV1
	lifecycle.health = FundsSourceReadHealthHealthyV1
	lifecycle.decision = FundsSourceReadDecisionGrantedV1
	lifecycle.reason = FundsSourceReadReasonAuthorityCurrentV1
	lifecycle.authorityDigest = authorityDigest
	lifecycle.grant = FundsSourceReadGrantV1{
		bindingDigest: lifecycle.binding.BindingDigest(), authorityDigest: authorityDigest,
		grantDigest: grantDigest, generation: lifecycle.generation,
		connectionEpoch: authority.ConnectionEpoch,
	}
	return lifecycle.grant, lifecycle.eventNoLock(), nil
}

func (lifecycle *FundsSourceReadLifecycleV1) Revoke(
	state FundsSourceReadStateV1,
	reason FundsSourceReadReasonCodeV1,
) (FundsSourceReadDecisionEventV1, error) {
	if lifecycle == nil || !validRevocationV1(state, reason) {
		return FundsSourceReadDecisionEventV1{}, ErrFundsSourceReadLifecycleInvalidV1
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.state == state && lifecycle.reason == reason && lifecycle.grant.grantDigest == "" {
		return lifecycle.eventNoLock(), nil
	}
	if lifecycle.binding.ExactRequest() {
		lifecycle.generation++
	}
	lifecycle.state = state
	lifecycle.health = FundsSourceReadHealthUnhealthyV1
	lifecycle.decision = FundsSourceReadDecisionRevokedV1
	if state == FundsSourceReadStateStoppedV1 {
		lifecycle.health = FundsSourceReadHealthStoppedV1
	}
	if state == FundsSourceReadStateDisabledV1 {
		lifecycle.health = FundsSourceReadHealthDisabledV1
		lifecycle.decision = FundsSourceReadDecisionDeniedV1
	}
	lifecycle.reason = reason
	lifecycle.authorityDigest = ""
	lifecycle.grant = FundsSourceReadGrantV1{}
	return lifecycle.eventNoLock(), nil
}

func (lifecycle *FundsSourceReadLifecycleV1) Authorizes(
	grant FundsSourceReadGrantV1,
	operation string,
	authority FundsSourceReadHostAuthorityV1,
) bool {
	if lifecycle == nil || operation != FundsSourceReadOperationV1 {
		return false
	}
	authorityDigest, err := authority.Digest()
	if err != nil {
		return false
	}
	lifecycle.mu.RLock()
	defer lifecycle.mu.RUnlock()
	return lifecycle.state == FundsSourceReadStateReadyV1 &&
		lifecycle.health == FundsSourceReadHealthHealthyV1 &&
		lifecycle.decision == FundsSourceReadDecisionGrantedV1 &&
		grant.bindingDigest == lifecycle.binding.BindingDigest() &&
		grant.authorityDigest == authorityDigest && grant.authorityDigest == lifecycle.authorityDigest &&
		grant.grantDigest != "" && grant.grantDigest == lifecycle.grant.grantDigest &&
		grant.generation == lifecycle.generation && grant.connectionEpoch == authority.ConnectionEpoch
}

func (lifecycle *FundsSourceReadLifecycleV1) CurrentGrant() (FundsSourceReadGrantV1, bool) {
	if lifecycle == nil {
		return FundsSourceReadGrantV1{}, false
	}
	lifecycle.mu.RLock()
	defer lifecycle.mu.RUnlock()
	if lifecycle.state != FundsSourceReadStateReadyV1 || lifecycle.decision != FundsSourceReadDecisionGrantedV1 ||
		lifecycle.grant.grantDigest == "" {
		return FundsSourceReadGrantV1{}, false
	}
	return lifecycle.grant, true
}

func (lifecycle *FundsSourceReadLifecycleV1) Snapshot() FundsSourceReadDecisionEventV1 {
	if lifecycle == nil {
		return FundsSourceReadDecisionEventV1{}
	}
	lifecycle.mu.RLock()
	defer lifecycle.mu.RUnlock()
	return lifecycle.eventNoLock()
}

func (lifecycle *FundsSourceReadLifecycleV1) eventNoLock() FundsSourceReadDecisionEventV1 {
	return FundsSourceReadDecisionEventV1{
		Schema:    FundsSourceReadDecisionSchemaV1,
		PackageID: lifecycle.binding.PackageID(), PackageVersion: lifecycle.binding.PackageVersion(),
		CapabilityID: FundsSourceReadCapabilityIDV1, ProtocolVersion: FundsSourceReadProtocolVersionV1,
		ScopeConstraints: append([]string(nil), fundsSourceReadScopesV1[:]...),
		Operation:        FundsSourceReadOperationV1,
		State:            lifecycle.state, Health: lifecycle.health, Decision: lifecycle.decision,
		ReasonCode: lifecycle.reason, Generation: lifecycle.generation,
		BindingDigest: lifecycle.binding.BindingDigest(), AuthorityDigest: lifecycle.authorityDigest,
		GrantDigest: lifecycle.grant.grantDigest,
	}
}

func validRevocationV1(state FundsSourceReadStateV1, reason FundsSourceReadReasonCodeV1) bool {
	switch state {
	case FundsSourceReadStateFailedV1:
		return reason == FundsSourceReadReasonCatalogFailedV1 ||
			reason == FundsSourceReadReasonProvenanceFailedV1 ||
			reason == FundsSourceReadReasonHealthFailedV1
	case FundsSourceReadStateStoppedV1:
		return reason == FundsSourceReadReasonDisconnectedV1
	case FundsSourceReadStateDisabledV1:
		return reason == FundsSourceReadReasonRequestMissingV1 || reason == FundsSourceReadReasonRequestMismatchV1
	case FundsSourceReadStateRevokedV1:
		return reason == FundsSourceReadReasonAuthorityRevokedV1
	default:
		return false
	}
}

func digestJSONV1(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func canonicalSHA256V1(value string) bool {
	if value != strings.TrimSpace(value) || len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}
