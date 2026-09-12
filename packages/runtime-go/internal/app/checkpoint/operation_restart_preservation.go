package checkpoint

import (
	"context"
	"errors"
	"sort"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
)

var ErrRestartPreserved = errors.New("checkpoint operation is preserved after restart")

type operationRestartPreservationV1 struct {
	threads  map[string]bool
	contexts map[string]domainsecurity.TurnSecurityContext
}

// NewOperationServiceWithRestartPreservationV1 binds denial-only scope before
// exposing this service to any consumer. The startup owner must prove these
// complete original Core contexts; this constructor cannot authorize recovery
// or replace original private case-registry validation.
func NewOperationServiceWithRestartPreservationV1(ctx context.Context, service OperationService, contexts []domainsecurity.TurnSecurityContext) (OperationService, error) {
	if ctx == nil || service.Observer == nil || !service.Authority.Available() || service.restartPreserved != nil {
		return OperationService{}, errors.New("checkpoint restart preservation constructor is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return OperationService{}, err
	}
	scope := &operationRestartPreservationV1{threads: map[string]bool{}, contexts: map[string]domainsecurity.TurnSecurityContext{}}
	for _, frozen := range contexts {
		if domainsecurity.ValidateTurnSecurityContext(frozen) != nil || frozen.Version != domainsecurity.TurnSecurityContextVersionV2 {
			return OperationService{}, errors.New("checkpoint restart preservation context is invalid")
		}
		if _, duplicate := scope.contexts[frozen.ContextDigest]; duplicate {
			return OperationService{}, errors.New("checkpoint restart preservation context is duplicated")
		}
		scope.contexts[frozen.ContextDigest] = frozen
		scope.threads[frozen.ThreadID] = true
	}
	service.restartPreserved = scope
	if err := service.validateRestartPreservedInventoryV1(ctx); err != nil {
		return OperationService{}, err
	}
	return service, nil
}

func (scope *operationRestartPreservationV1) ownsThread(id string) bool {
	return scope != nil && scope.threads[id]
}

func (scope *operationRestartPreservationV1) validateContext(frozen domainsecurity.TurnSecurityContext) error {
	if !scope.ownsThread(frozen.ThreadID) {
		return nil
	}
	if expected, found := scope.contexts[frozen.ContextDigest]; !found || expected != frozen {
		return errors.New("checkpoint preserved context is outside the proved original inventory")
	}
	return nil
}

func (service OperationService) validateRestartPreservedInventoryV1(ctx context.Context) error {
	if service.restartPreserved == nil {
		return nil
	}
	// Read every authoritative group, including already settled groups. A hold cannot
	// hide a corrupt record or a lost original context behind an open-only view.
	states, err := service.Authority.OperationGroups(ctx)
	if err != nil {
		return err
	}
	sequences := map[string][]domaincheckpoint.OperationGroupStateV2{}
	for _, state := range states {
		if err := service.restartPreserved.validateContext(state.Intent.SecurityContext); err != nil {
			return err
		}
		key := state.Intent.SecurityContext.ThreadID + "\x00" + state.Intent.CheckpointID
		sequences[key] = append(sequences[key], state)
	}
	for _, sequence := range sequences {
		sort.Slice(sequence, func(i, j int) bool { return sequence[i].Intent.OperationOrdinal < sequence[j].Intent.OperationOrdinal })
		if err := validateOperationGroupSequenceV1(sequence); err != nil {
			return err
		}
	}
	if err := service.restartPreserved.validateRecoveryResources(states, service.Observer); err != nil {
		return err
	}
	return ctx.Err()
}

func operationResourceV1(intent domaincheckpoint.OperationGroupIntentV2, operationPath domaincheckpoint.OperationPathV2) checkpointfileport.PathAuthority {
	root := operationPath.AuthorityRoot
	if operationPath.PathAuthoritySchemaVersion == 0 {
		root = intent.SecurityContext.WorkspaceRealPath
	}
	return checkpointfileport.PathAuthority{Root: root, RootIdentity: operationPath.AuthorityRootIdentity, RelativePath: operationPath.RelativePath}
}

func (scope *operationRestartPreservationV1) validateRecoveryResources(states []domaincheckpoint.OperationGroupStateV2, observer checkpointfileport.Observer) error {
	var held []checkpointfileport.PathAuthority
	for _, state := range states {
		if state.Terminal != nil || !scope.ownsThread(state.Intent.SecurityContext.ThreadID) {
			continue
		}
		for _, path := range state.Intent.Paths {
			held = append(held, operationResourceV1(state.Intent, path))
		}
	}
	for _, state := range states {
		if state.Terminal != nil || scope.ownsThread(state.Intent.SecurityContext.ThreadID) || state.Intent.ToolName != "move_file" {
			continue
		}
		for _, path := range state.Intent.Paths {
			// Prepared move recovery can restore its source. Its destination is
			// checked for absence but is never installed by a recovery action.
			if path.Role != "source" {
				continue
			}
			candidate := operationResourceV1(state.Intent, path)
			for _, protected := range held {
				if observer.ResourcesOverlap(candidate, protected) {
					return errors.Join(ErrRestartPreserved, errors.New("checkpoint recovery source overlaps a preserved operation resource"))
				}
			}
		}
	}
	return nil
}
