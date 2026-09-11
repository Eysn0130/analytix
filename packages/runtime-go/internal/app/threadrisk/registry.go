package threadrisk

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	storeport "analytix.local/runtime-go/internal/ports/threadriskpolicy"
)

type ResolveOrRaiseInput struct {
	ThreadID          string
	WorkspaceRealPath string
	RequestedRisk     string
	Origin            string
	SignalsDigest     string
	IssuedAt          time.Time
}

// Registry owns the in-memory projection of the signed, immutable policy
// chains. It is intentionally not connected to runtime composition yet.
type Registry struct {
	authority authorityport.Authority
	store     storeport.Store

	mu       sync.RWMutex
	byDigest map[string]domainsecurity.ThreadRiskPolicyV1
	current  map[string]domainsecurity.ThreadRiskPolicyV1
}

type inventory struct {
	byDigest map[string]domainsecurity.ThreadRiskPolicyV1
	current  map[string]domainsecurity.ThreadRiskPolicyV1
}

func NewRegistry(ctx context.Context, authority authorityport.Authority, store storeport.Store) (*Registry, error) {
	if authority == nil || store == nil || !validInstallationAuthority(authority) {
		return nil, errors.New("thread risk policy registry dependencies are invalid")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	registry := &Registry{
		authority: authority,
		store:     store,
		byDigest:  map[string]domainsecurity.ThreadRiskPolicyV1{},
		current:   map[string]domainsecurity.ThreadRiskPolicyV1{},
	}
	registry.mu.Lock()
	err := registry.refreshLocked(ctx)
	registry.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return registry, nil
}

// ResolveOrRaise returns the current policy for an equal or lower request and
// appends exactly one signed child for a general-to-case transition. A general
// thread may move to another host-resolved workspace through a signed
// successor; a case thread still requires a separate signed rebind contract.
func (registry *Registry) ResolveOrRaise(ctx context.Context, input ResolveOrRaiseInput) (domainsecurity.ThreadRiskPolicyV1, error) {
	if registry == nil || registry.authority == nil || registry.store == nil {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy registry is unavailable")
	}
	if err := validateResolveInput(input); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	if err := contextError(ctx); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	// Refresh before every mutation so a cancellation or another process cannot
	// leave this projection silently behind durable CAS state.
	if err := registry.refreshLocked(ctx); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}

	current, exists := registry.current[input.ThreadID]
	if exists {
		if current.WorkspaceRealPath != input.WorkspaceRealPath {
			if current.RiskClass == domainsecurity.RiskClassCase {
				return domainsecurity.ThreadRiskPolicyV1{}, errors.New("case thread risk policy workspace change requires an explicit signed rebind contract")
			}
		} else if current.RiskClass == input.RequestedRisk || current.RiskClass == domainsecurity.RiskClassCase {
			return current, nil
		}
		if current.RiskClass != domainsecurity.RiskClassGeneral || input.RequestedRisk != domainsecurity.RiskClassCase {
			if current.WorkspaceRealPath == input.WorkspaceRealPath || input.RequestedRisk != domainsecurity.RiskClassGeneral {
				return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy transition is invalid")
			}
		}
	}

	predecessor := ""
	if exists {
		predecessor = current.PolicyDigest
	}
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath,
		RiskClass: input.RequestedRisk, Origin: input.Origin, SignalsDigest: input.SignalsDigest,
		PredecessorPolicyDigest: predecessor, IssuedAt: input.IssuedAt,
		AuthorityKeyID: registry.authority.KeyID(), AuthorityPublicKey: registry.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) {
		return registry.authority.Sign(ctx, message)
	})
	if err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, contextErr
		}
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	if err := registry.verifyTrusted(ctx, policy); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	if exists {
		if err := domainsecurity.ValidateThreadRiskPolicyTransitionV1(current, policy); err != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, err
		}
	}
	if err := registry.store.PutIfAbsent(ctx, policy); err != nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, contextErr
		}
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	readback, err := registry.store.Resolve(ctx, policy.PolicyDigest)
	if err != nil || readback != policy {
		if contextErr := contextError(ctx); contextErr != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, contextErr
		}
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy persistence readback failed")
	}
	if err := registry.verifyTrusted(ctx, readback); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy persistence authority verification failed")
	}
	if err := registry.refreshLocked(ctx); err != nil {
		return domainsecurity.ThreadRiskPolicyV1{}, err
	}
	stored, found := registry.byDigest[policy.PolicyDigest]
	if !found || stored != policy {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy persistence inventory verification failed")
	}
	resolved, found := registry.current[input.ThreadID]
	if !found {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("thread risk policy current head is missing after persistence")
	}
	return resolved, nil
}

func (registry *Registry) Current(threadID string) (domainsecurity.ThreadRiskPolicyV1, bool) {
	if registry == nil || threadID != strings.TrimSpace(threadID) || threadID == "" {
		return domainsecurity.ThreadRiskPolicyV1{}, false
	}
	registry.mu.RLock()
	policy, found := registry.current[threadID]
	registry.mu.RUnlock()
	return policy, found
}

// Resolve returns an exact policy only from the installation-verified startup
// inventory. A syntactically valid or merely non-empty digest is not authority.
func (registry *Registry) Resolve(digest string) (domainsecurity.ThreadRiskPolicyV1, bool) {
	if registry == nil || digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return domainsecurity.ThreadRiskPolicyV1{}, false
	}
	registry.mu.RLock()
	policy, found := registry.byDigest[digest]
	registry.mu.RUnlock()
	return policy, found
}

// ValidateContext binds a V2 turn context to an exact installation-signed
// thread risk policy. Historical policies remain resolvable for audit/recovery;
// callers that require the live head must additionally compare Current.
func (registry *Registry) ValidateContext(securityContext domainsecurity.TurnSecurityContext) error {
	if registry == nil || securityContext.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		domainsecurity.ValidateTurnSecurityContext(securityContext) != nil {
		return errors.New("thread risk policy context must be a valid V2 security context")
	}
	policy, found := registry.Resolve(securityContext.PublicationPolicy.ThreadRiskPolicyDigest)
	if !found {
		return errors.New("turn context thread risk policy is absent from the installation-verified inventory")
	}
	if err := registry.verifyTrusted(nil, policy); err != nil {
		return errors.New("turn context thread risk policy lost installation authority")
	}
	if policy.ThreadID != securityContext.ThreadID || policy.WorkspaceRealPath != securityContext.WorkspaceRealPath {
		return errors.New("turn context thread risk policy identity is invalid")
	}
	if err := domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, policy); err != nil {
		return errors.New("turn context publication policy is not bound to its signed thread risk policy")
	}
	return nil
}

func (registry *Registry) refreshLocked(ctx context.Context) error {
	policies, err := registry.store.List(ctx)
	if err != nil {
		return err
	}
	loaded, err := registry.validateInventory(ctx, policies)
	if err != nil {
		return err
	}
	registry.byDigest = loaded.byDigest
	registry.current = loaded.current
	return nil
}

func (registry *Registry) validateInventory(ctx context.Context, policies []domainsecurity.ThreadRiskPolicyV1) (inventory, error) {
	loaded := inventory{
		byDigest: make(map[string]domainsecurity.ThreadRiskPolicyV1, len(policies)),
		current:  map[string]domainsecurity.ThreadRiskPolicyV1{},
	}
	byThread := map[string][]domainsecurity.ThreadRiskPolicyV1{}
	for _, policy := range policies {
		if err := contextError(ctx); err != nil {
			return inventory{}, err
		}
		if err := registry.verifyTrusted(ctx, policy); err != nil {
			return inventory{}, err
		}
		if _, duplicate := loaded.byDigest[policy.PolicyDigest]; duplicate {
			return inventory{}, errors.New("thread risk policy inventory contains a duplicate digest")
		}
		loaded.byDigest[policy.PolicyDigest] = policy
		byThread[policy.ThreadID] = append(byThread[policy.ThreadID], policy)
	}
	threadIDs := make([]string, 0, len(byThread))
	for threadID := range byThread {
		threadIDs = append(threadIDs, threadID)
	}
	sort.Strings(threadIDs)
	for _, threadID := range threadIDs {
		current, err := validateThreadChain(byThread[threadID])
		if err != nil {
			return inventory{}, fmt.Errorf("thread risk policy chain %q: %w", threadID, err)
		}
		loaded.current[threadID] = current
	}
	return loaded, nil
}

func (registry *Registry) verifyTrusted(ctx context.Context, policy domainsecurity.ThreadRiskPolicyV1) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	publicKey := registry.authority.PublicKey()
	if err := domainsecurity.ValidateThreadRiskPolicyV1ForInstallation(policy, registry.authority.KeyID(), publicKey); err != nil {
		return errors.New("thread risk policy is not signed by the installation authority")
	}
	recordPublicKey, publicErr := base64.RawURLEncoding.DecodeString(policy.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(policy.AuthoritySignature)
	verifyErr := registry.authority.VerifyTrusted(ctx, policy.AuthorityKeyID, recordPublicKey,
		domainsecurity.ThreadRiskPolicySigningBytesV1(policy), signature)
	if contextErr := contextError(ctx); contextErr != nil {
		return contextErr
	}
	if publicErr != nil || signatureErr != nil || verifyErr != nil {
		return errors.New("thread risk policy installation signature is invalid")
	}
	return nil
}

func validateThreadChain(policies []domainsecurity.ThreadRiskPolicyV1) (domainsecurity.ThreadRiskPolicyV1, error) {
	byDigest := make(map[string]domainsecurity.ThreadRiskPolicyV1, len(policies))
	children := map[string][]string{}
	roots := make([]string, 0, 1)
	for _, policy := range policies {
		byDigest[policy.PolicyDigest] = policy
		if policy.PredecessorPolicyDigest == "" {
			roots = append(roots, policy.PolicyDigest)
		}
	}
	if len(roots) != 1 {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("inventory must contain exactly one root")
	}
	for _, policy := range policies {
		if policy.PredecessorPolicyDigest == "" {
			continue
		}
		parent, found := byDigest[policy.PredecessorPolicyDigest]
		if !found {
			return domainsecurity.ThreadRiskPolicyV1{}, errors.New("inventory contains a detached predecessor")
		}
		if err := domainsecurity.ValidateThreadRiskPolicyTransitionV1(parent, policy); err != nil {
			return domainsecurity.ThreadRiskPolicyV1{}, err
		}
		children[parent.PolicyDigest] = append(children[parent.PolicyDigest], policy.PolicyDigest)
		if len(children[parent.PolicyDigest]) > 1 {
			return domainsecurity.ThreadRiskPolicyV1{}, errors.New("inventory contains a policy fork")
		}
	}
	visited := map[string]bool{}
	head := roots[0]
	for {
		if visited[head] {
			return domainsecurity.ThreadRiskPolicyV1{}, errors.New("inventory contains a policy cycle")
		}
		visited[head] = true
		next := children[head]
		if len(next) == 0 {
			break
		}
		head = next[0]
	}
	if len(visited) != len(policies) {
		return domainsecurity.ThreadRiskPolicyV1{}, errors.New("inventory contains a disconnected chain or cycle")
	}
	return byDigest[head], nil
}

func validateResolveInput(input ResolveOrRaiseInput) error {
	if input.ThreadID == "" || input.ThreadID != strings.TrimSpace(input.ThreadID) ||
		input.WorkspaceRealPath == "" || input.WorkspaceRealPath != strings.TrimSpace(input.WorkspaceRealPath) ||
		input.SignalsDigest != strings.TrimSpace(input.SignalsDigest) || !domainsecurity.IsSHA256Hex(input.SignalsDigest) ||
		input.IssuedAt.IsZero() {
		return errors.New("thread risk policy resolution input is incomplete")
	}
	switch input.RequestedRisk {
	case domainsecurity.RiskClassGeneral:
		if input.Origin != domainsecurity.RiskPolicyOriginGeneralWorkspace && input.Origin != domainsecurity.RiskPolicyOriginLegacyMigration {
			return errors.New("general thread risk policy origin is invalid")
		}
	case domainsecurity.RiskClassCase:
		if !validCaseOrigin(input.Origin) {
			return errors.New("case thread risk policy origin is invalid")
		}
	default:
		return errors.New("thread risk policy requested class is invalid")
	}
	return nil
}

func validCaseOrigin(origin string) bool {
	switch origin {
	case domainsecurity.RiskPolicyOriginDesktopCaseEntry,
		domainsecurity.RiskPolicyOriginBindingMarkerPresent,
		domainsecurity.RiskPolicyOriginValidCaseBinding,
		domainsecurity.RiskPolicyOriginSignedCaseLineage,
		domainsecurity.RiskPolicyOriginTrustedCaseFinal,
		domainsecurity.RiskPolicyOriginLegacyMigration,
		domainsecurity.RiskPolicyOriginLexicalGuard:
		return true
	default:
		return false
	}
}

func validInstallationAuthority(authority authorityport.Authority) bool {
	publicKey := authority.PublicKey()
	return len(publicKey) == ed25519.PublicKeySize && authority.KeyID() == domainsecurity.SHA256Hex(publicKey)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
