package fundscsvadmission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"time"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// This adapter creates only an empty case identity. Snapshot admission,
// Evidence Registry activation and receipts retain their existing owners.
type ImportCaseCreatorV1 interface {
	ObserveMissingImportWorkspace(string) (domainsecurity.CaseBindingObservationV1, string, error)
	CreateForMainSelectedImport(context.Context, string, domainsecurity.CaseBindingObservationV1, string, string, time.Time) (domainsecurity.CaseBindingObservationV1, error)
}

type CaseCreationRequiredV1 struct{ Intent string }

type pendingImportCaseCreationV1 struct {
	intent           string
	physicalIdentity string
	observation      domainsecurity.CaseBindingObservationV1
	principal        domainidentity.PrincipalV1
	sourcePath       string
	expiresAt        time.Time
}

func (*CaseCreationRequiredV1) Error() string {
	return "explicit case creation confirmation is required"
}

func (service *ServiceV1) resolveSelectedImportAuthorityV1(ctx context.Context, input StageInputV1) (domainidentity.PrincipalV1, domainsecurity.CaseBindingObservationV1, error) {
	principal, err := service.identity.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil || service.identity.ValidateCurrent(ctx, principal) != nil {
		return domainidentity.PrincipalV1{}, domainsecurity.CaseBindingObservationV1{}, ErrUnavailable
	}
	observation, err := service.observer.Observe(input.WorkspaceRoot)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil || observation.WorkspaceRealPath != input.WorkspaceRoot {
		return principal, observation, ErrInvalidRequest
	}
	if observation.State == domainsecurity.CaseBindingStateValid && input.CreateCaseIntent == "" {
		return principal, observation, nil
	}
	if observation.State != domainsecurity.CaseBindingStateMissing || service.caseCreator == nil {
		return principal, observation, ErrInvalidRequest
	}
	expected, physicalIdentity, err := service.caseCreator.ObserveMissingImportWorkspace(input.WorkspaceRoot)
	if err != nil || expected != observation || !domainsecurity.IsSHA256Hex(physicalIdentity) || ctx.Err() != nil || service.identity.ValidateCurrent(ctx, principal) != nil {
		return principal, observation, ErrUnavailable
	}
	if input.CreateCaseIntent == "" {
		var nonce [32]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return principal, observation, ErrUnavailable
		}
		intent := hex.EncodeToString(nonce[:])
		clear(nonce[:])
		service.stagingMu.Lock()
		if ctx.Err() != nil || service.identity.ValidateCurrent(ctx, principal) != nil {
			service.stagingMu.Unlock()
			return principal, observation, ErrUnavailable
		}
		service.pendingCaseCreation = &pendingImportCaseCreationV1{intent: intent, physicalIdentity: physicalIdentity,
			observation: observation, principal: principal, sourcePath: input.SourcePath, expiresAt: service.now().UTC().Add(ImportStagingLifetimeV1)}
		service.stagingMu.Unlock()
		return principal, observation, &CaseCreationRequiredV1{Intent: intent}
	}
	service.stagingMu.Lock()
	pending := service.pendingCaseCreation
	valid := pending != nil && pending.intent == input.CreateCaseIntent && pending.physicalIdentity == physicalIdentity &&
		pending.observation == observation && domainidentity.SamePrincipalV1(pending.principal, principal) &&
		pending.sourcePath == input.SourcePath && service.now().UTC().Before(pending.expiresAt) && ctx.Err() == nil
	if pending != nil && pending.intent == input.CreateCaseIntent {
		service.pendingCaseCreation = nil
	}
	service.stagingMu.Unlock()
	if !valid {
		return principal, observation, ErrInvalidRequest
	}
	nonce := make([]byte, 16)
	if _, err := io.ReadFull(service.random, nonce); err != nil {
		return principal, observation, ErrUnavailable
	}
	caseID := "case_" + hex.EncodeToString(nonce)
	clear(nonce)
	created, err := service.caseCreator.CreateForMainSelectedImport(ctx, input.WorkspaceRoot, expected, physicalIdentity, caseID, service.now().UTC())
	if err != nil || ctx.Err() != nil || service.identity.ValidateCurrent(ctx, principal) != nil {
		// A successfully created empty identity is preserved on uncertain response;
		// retry observes it normally and never creates a second case or receipt.
		return principal, observation, ErrUnavailable
	}
	current, err := service.observer.Observe(input.WorkspaceRoot)
	if err != nil || current != created || created.State != domainsecurity.CaseBindingStateValid || created.CaseID != caseID || domainsecurity.ValidateCaseBindingObservationV1(created) != nil {
		return principal, observation, ErrUnavailable
	}
	return principal, created, nil
}
