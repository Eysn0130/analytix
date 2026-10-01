package checkpoint

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type OperationPathRequest struct {
	ResolvedPath         string
	ArgumentKey          string
	RequestedPath        string
	Role                 string
	ExpectedAfterExisted bool
	ExpectedAfterHash    string
}

type OperationDraft struct {
	Enabled         bool
	Workspace       string
	AuthorityIntent domaincheckpoint.OperationGroupIntentV2
	Paths           []OperationPathRequest
}

type OperationService struct {
	Authority        SnapshotAuthority
	Observer         checkpointfileport.Observer
	Recovery         checkpointfileport.OperationRecoveryPlanner
	restartPreserved *operationRestartPreservationV1
}

type BeginOperationInput struct {
	GenerationPrincipalDigest   string
	SecurityContext             domainsecurity.TurnSecurityContext
	ExecutionGrant              domainsecurity.ExecutionGrant
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	Workspace                   string
	ToolName                    string
	ArgumentsJSON               []byte
	Paths                       []OperationPathRequest
	CreatedAt                   time.Time
}

type OperationRecoveryResult struct {
	Intent   domaincheckpoint.OperationGroupIntentV2
	Terminal domaincheckpoint.OperationGroupTerminalV2
	Receipt  privatecasport.AdditionReceiptV2
	Created  bool
}

type preparedOperationRecoveryItem struct {
	intent domaincheckpoint.OperationGroupIntentV2
	action checkpointfileport.PreparedOperationRecovery
}

// PreparedOpenOperationRecovery is the read-only semantic decision for the
// exact durable open-intent inventory. Apply revalidates that inventory,
// repairs any authorized private move pending, and only then writes terminal
// authority records.
type PreparedOpenOperationRecovery struct {
	service OperationService
	items   []preparedOperationRecoveryItem
}

var ErrOperationRecoveryQuarantined = errors.New("checkpoint operation recovery quarantined a divergent workspace state")

func (service OperationService) Begin(ctx context.Context, input BeginOperationInput) (OperationDraft, error) {
	if service.restartPreserved.ownsThread(input.SecurityContext.ThreadID) {
		return OperationDraft{}, ErrRestartPreserved
	}
	if service.Observer == nil || !service.Authority.Available() || len(input.Paths) == 0 {
		return OperationDraft{}, errors.New("checkpoint operation service is unavailable")
	}
	domainPaths := make([]domaincheckpoint.OperationPathInputV2, 0, len(input.Paths))
	for _, request := range input.Paths {
		before, err := service.Observer.CaptureBefore(ctx, input.Workspace, request.ResolvedPath)
		if err != nil {
			return OperationDraft{}, err
		}
		domainPaths = append(domainPaths, domaincheckpoint.OperationPathInputV2{
			ArgumentKey: request.ArgumentKey, RequestedPath: request.RequestedPath,
			PathAuthoritySchemaVersion: before.PathAuthority.SchemaVersion,
			AuthorityKind:              before.PathAuthority.Kind, AuthorityRoot: before.PathAuthority.Root,
			AuthorityRootIdentity: before.PathAuthority.RootIdentity,
			AuthorityRootHash:     before.PathAuthority.RootHash,
			RelativePath:          before.PathAuthority.RelativePath, Role: request.Role,
			BeforeExisted: before.Existed, BeforeAvailable: before.ContentAvailable,
			BeforeHash: before.Hash, BeforeSizeBytes: int64(len(before.RawBytes)),
			BeforeSnapshotSchemaVersion: operationBeforeSnapshotVersion(before),
			BeforeEncoding:              before.Encoding, BeforeBytesBase64: operationBeforeBytes(before),
			ExpectedAfterExisted: request.ExpectedAfterExisted, ExpectedAfterHash: strings.TrimSpace(request.ExpectedAfterHash),
		})
	}
	if input.ToolName == "move_file" {
		sourceHash := ""
		for _, path := range domainPaths {
			if path.Role == "source" {
				sourceHash = path.BeforeHash
			}
		}
		for index := range domainPaths {
			if domainPaths[index].Role == "destination" && domainPaths[index].ExpectedAfterHash == "" {
				domainPaths[index].ExpectedAfterHash = sourceHash
			}
		}
	}
	intent, terminal, existing, err := service.Authority.BeginOperationGroup(ctx, BeginOperationGroupAuthorityInput{
		GenerationPrincipalDigest: input.GenerationPrincipalDigest,
		SecurityContext:           input.SecurityContext, ExecutionGrant: input.ExecutionGrant,
		CheckpointID: input.CheckpointID, SourceWorkspaceCheckpointID: input.SourceWorkspaceCheckpointID,
		ToolName: input.ToolName, ArgumentsJSON: input.ArgumentsJSON, Paths: domainPaths, CreatedAt: input.CreatedAt,
	})
	if err != nil {
		return OperationDraft{}, err
	}
	if existing {
		if terminal != nil {
			return OperationDraft{}, errors.New("checkpoint operation group is already settled and cannot be re-executed")
		}
		return OperationDraft{}, errors.New("checkpoint operation group requires startup reconciliation before execution")
	}
	return OperationDraft{
		Enabled: true, Workspace: strings.TrimSpace(input.Workspace), AuthorityIntent: intent,
		Paths: append([]OperationPathRequest(nil), input.Paths...),
	}, nil
}

func (service OperationService) Settle(ctx context.Context, draft OperationDraft, mutationSucceeded bool, settledAt time.Time) (domaincheckpoint.OperationGroupTerminalV2, error) {
	if service.restartPreserved.ownsThread(draft.AuthorityIntent.SecurityContext.ThreadID) {
		return domaincheckpoint.OperationGroupTerminalV2{}, ErrRestartPreserved
	}
	if !draft.Enabled || service.Observer == nil || !service.Authority.Available() || len(draft.Paths) != len(draft.AuthorityIntent.Paths) {
		return domaincheckpoint.OperationGroupTerminalV2{}, errors.New("checkpoint operation draft is invalid")
	}
	observed := make([]domaincheckpoint.ObservedOperationPathV2, 0, len(draft.AuthorityIntent.Paths))
	for index, expected := range draft.AuthorityIntent.Paths {
		resolvedPath := ""
		if index < len(draft.Paths) && strings.TrimSpace(draft.Paths[index].Role) == expected.Role {
			resolvedPath = draft.Paths[index].ResolvedPath
		}
		observed = append(observed, service.observeOperationPath(draft.AuthorityIntent, false,
			ctx, draft.Workspace, operationPathAuthority(expected, draft.Workspace), resolvedPath,
		))
	}
	status, ok := domaincheckpoint.ClassifyObservedOperationGroup(draft.AuthorityIntent, observed)
	if !ok {
		return domaincheckpoint.OperationGroupTerminalV2{}, errors.New("checkpoint operation filesystem observation is invalid")
	}
	reason := operationSettlementReason(status, observed, mutationSucceeded)
	return service.Authority.SettleOperationGroup(ctx, draft.AuthorityIntent.OperationGroupID, status, reason, observed, settledAt)
}

func (service OperationService) PrepareRecoverOpen(ctx context.Context) (PreparedOpenOperationRecovery, error) {
	if service.Observer == nil || !service.Authority.Available() {
		return PreparedOpenOperationRecovery{}, errors.New("checkpoint operation recovery is unavailable")
	}
	if err := service.validateRestartPreservedInventoryV1(ctx); err != nil {
		return PreparedOpenOperationRecovery{}, err
	}
	open, err := service.Authority.OpenOperationGroups(ctx)
	if err != nil {
		return PreparedOpenOperationRecovery{}, err
	}
	prepared := PreparedOpenOperationRecovery{service: service, items: make([]preparedOperationRecoveryItem, 0, len(open))}
	for _, intent := range open {
		if err := service.restartPreserved.validateContext(intent.SecurityContext); err != nil {
			return PreparedOpenOperationRecovery{}, err
		}
	}
	for _, intent := range open {
		var action checkpointfileport.PreparedOperationRecovery
		if intent.ToolName == "move_file" {
			if service.Recovery == nil {
				return PreparedOpenOperationRecovery{}, errors.New("conditional move operation recovery is unavailable")
			}
			action, err = service.Recovery.PrepareOperationRecovery(ctx, intent)
			if err != nil {
				return PreparedOpenOperationRecovery{}, err
			}
		}
		prepared.items = append(prepared.items, preparedOperationRecoveryItem{intent: intent, action: action})
	}
	return prepared, nil
}

func (prepared PreparedOpenOperationRecovery) Apply(ctx context.Context, observedAt time.Time) ([]OperationRecoveryResult, error) {
	service := prepared.service
	if service.Observer == nil || !service.Authority.Available() || observedAt.IsZero() {
		return nil, errors.New("prepared checkpoint operation recovery is unavailable")
	}
	current, err := service.Authority.OpenOperationGroups(ctx)
	if err != nil {
		return nil, err
	}
	if !samePreparedOperationInventory(prepared.items, current) {
		return nil, errors.New("checkpoint open operation inventory changed after semantic planning")
	}
	if err := service.validateRestartPreservedInventoryV1(ctx); err != nil {
		return nil, err
	}
	for _, item := range prepared.items {
		if service.restartPreserved.ownsThread(item.intent.SecurityContext.ThreadID) {
			continue
		}
		if item.action != nil {
			if err := item.action.Apply(ctx); err != nil {
				return nil, err
			}
		}
	}
	results := make([]OperationRecoveryResult, 0, len(prepared.items))
	var quarantineErr error
	for _, item := range prepared.items {
		intent := item.intent
		if service.restartPreserved.ownsThread(intent.SecurityContext.ThreadID) {
			continue
		}
		observed := make([]domaincheckpoint.ObservedOperationPathV2, 0, len(intent.Paths))
		for _, expected := range intent.Paths {
			observed = append(observed, service.observeOperationPath(intent, true,
				ctx, intent.SecurityContext.WorkspaceRealPath,
				operationPathAuthority(expected, intent.SecurityContext.WorkspaceRealPath), "",
			))
		}
		status, ok := domaincheckpoint.ClassifyObservedOperationGroup(intent, observed)
		if !ok {
			return results, errors.New("checkpoint recovery observation is invalid")
		}
		reason := operationRecoveryReason(status, observed)
		terminal, receipt, created, settleErr := service.Authority.SettleOperationGroupForRecovery(ctx, intent.OperationGroupID, status, reason, observed, observedAt)
		if settleErr != nil {
			return results, settleErr
		}
		results = append(results, OperationRecoveryResult{Intent: intent, Terminal: terminal, Receipt: receipt, Created: created})
		if status == "quarantined" {
			quarantineErr = ErrOperationRecoveryQuarantined
		}
	}
	return results, quarantineErr
}

func (service OperationService) observeOperationPath(intent domaincheckpoint.OperationGroupIntentV2, relative bool,
	ctx context.Context, workspace string, authority checkpointfileport.PathAuthority, resolvedPath string,
) domaincheckpoint.ObservedOperationPathV2 {
	if intent.ToolName == "generate_office_document" {
		observer, ok := service.Observer.(checkpointfileport.GeneratedFileObserver)
		if !ok || len(intent.Paths) != 1 || intent.Paths[0].BeforeExisted || !intent.Paths[0].ExpectedAfterExisted {
			return domaincheckpoint.ObservedOperationPathV2{
				PathAuthoritySchemaVersion: authority.SchemaVersion, AuthorityKind: authority.Kind,
				AuthorityRootHash: authority.RootHash, RelativePath: authority.RelativePath,
				ObservationStatus: "unavailable", BlockerCode: "path_unsafe",
			}
		}
		if relative {
			return observer.ObserveGeneratedRelative(ctx, workspace, authority)
		}
		return observer.ObserveGenerated(ctx, workspace, authority, resolvedPath)
	}
	if relative {
		return service.Observer.ObserveRelative(ctx, workspace, authority)
	}
	return service.Observer.Observe(ctx, workspace, authority, resolvedPath)
}

func (service OperationService) RecoverOpen(ctx context.Context, observedAt time.Time) ([]OperationRecoveryResult, error) {
	prepared, err := service.PrepareRecoverOpen(ctx)
	if err != nil {
		return nil, err
	}
	return prepared.Apply(ctx, observedAt)
}

func samePreparedOperationInventory(
	prepared []preparedOperationRecoveryItem,
	current []domaincheckpoint.OperationGroupIntentV2,
) bool {
	if len(prepared) != len(current) {
		return false
	}
	byID := make(map[string]string, len(current))
	for _, intent := range current {
		if _, duplicate := byID[intent.OperationGroupID]; duplicate {
			return false
		}
		byID[intent.OperationGroupID] = intent.IntentDigest
	}
	for _, item := range prepared {
		if byID[item.intent.OperationGroupID] != item.intent.IntentDigest {
			return false
		}
	}
	return true
}

func operationBeforeSnapshotVersion(before checkpointfileport.BeforeState) int {
	if before.Existed && before.ContentAvailable {
		return 1
	}
	return 0
}

func operationBeforeBytes(before checkpointfileport.BeforeState) string {
	if !before.Existed || !before.ContentAvailable {
		return ""
	}
	return base64.StdEncoding.EncodeToString(before.RawBytes)
}

func operationPathAuthority(path domaincheckpoint.OperationPathV2, workspace string) checkpointfileport.PathAuthority {
	if path.PathAuthoritySchemaVersion == 0 {
		return checkpointfileport.PathAuthority{Root: strings.TrimSpace(workspace), RelativePath: path.RelativePath}
	}
	return checkpointfileport.PathAuthority{
		SchemaVersion: path.PathAuthoritySchemaVersion, Kind: path.AuthorityKind,
		Root: path.AuthorityRoot, RootIdentity: path.AuthorityRootIdentity,
		RootHash: path.AuthorityRootHash, RelativePath: path.RelativePath,
	}
}

func operationSettlementReason(status string, observed []domaincheckpoint.ObservedOperationPathV2, mutationSucceeded bool) string {
	switch status {
	case "completed":
		if mutationSucceeded {
			return "mutation_completed"
		}
		return "recovery_observed_after"
	case "no_effect":
		if mutationSucceeded {
			return "recovery_observed_before"
		}
		return "mutation_failed_before_effect"
	case "quarantined":
		for _, path := range observed {
			if path.ObservationStatus == "unavailable" {
				return "filesystem_observation_failed"
			}
		}
		return "mixed_filesystem_state"
	default:
		return ""
	}
}

func operationRecoveryReason(status string, observed []domaincheckpoint.ObservedOperationPathV2) string {
	switch status {
	case "completed":
		return "recovery_observed_after"
	case "no_effect":
		return "recovery_observed_before"
	case "quarantined":
		for _, path := range observed {
			if path.ObservationStatus == "unavailable" {
				return "filesystem_observation_failed"
			}
		}
		return "mixed_filesystem_state"
	default:
		return ""
	}
}
