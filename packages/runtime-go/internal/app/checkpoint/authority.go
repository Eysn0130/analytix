package checkpoint

import (
	"context"
	"errors"
	"sort"
	"time"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
)

type SnapshotAuthority struct {
	Store checkpointport.Store
}

type BeginSnapshotAuthorityInput struct {
	SecurityContext             domainsecurity.TurnSecurityContext
	ExecutionGrant              domainsecurity.ExecutionGrant
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	RelativePath                string
	BeforeExisted               bool
	BeforeAvailable             bool
	BeforeHash                  string
	BeforeContent               string
	CreatedAt                   time.Time
}

type CapturedAuthorityInput struct {
	ThreadID                    string
	TurnID                      string
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	AuthorityWorkspace          string
	WorkspaceFallback           string
	CreatedAtFallback           string
}

func (authority SnapshotAuthority) Available() bool {
	return authority.Store != nil
}

func (authority SnapshotAuthority) Begin(
	ctx context.Context,
	input BeginSnapshotAuthorityInput,
) (domaincheckpoint.SnapshotIntentV1, error) {
	if authority.Store == nil {
		return domaincheckpoint.SnapshotIntentV1{}, errors.New("checkpoint snapshot authority is unavailable")
	}
	return authority.Store.BeginSnapshot(ctx, domaincheckpoint.SnapshotIntentInputV1{
		SecurityContext: input.SecurityContext, ExecutionGrant: input.ExecutionGrant,
		CheckpointID: input.CheckpointID, SourceWorkspaceCheckpointID: input.SourceWorkspaceCheckpointID,
		RelativePath: input.RelativePath, BeforeExisted: input.BeforeExisted, BeforeAvailable: input.BeforeAvailable,
		BeforeHash: input.BeforeHash, BeforeContent: input.BeforeContent, CreatedAt: input.CreatedAt,
	})
}

func (authority SnapshotAuthority) Complete(
	ctx context.Context,
	intentID string,
	afterExisted bool,
	afterHash string,
	completedAt time.Time,
) error {
	if authority.Store == nil {
		return errors.New("checkpoint snapshot authority is unavailable")
	}
	_, err := authority.Store.CompleteSnapshot(ctx, intentID, afterExisted, afterHash, completedAt)
	return err
}

func (authority SnapshotAuthority) Abort(ctx context.Context, intentID string, closedAt time.Time) error {
	if authority.Store == nil {
		return errors.New("checkpoint snapshot authority is unavailable")
	}
	_, err := authority.Store.AbortSnapshot(ctx, intentID, "mutation_failed", closedAt)
	return err
}

func (authority SnapshotAuthority) ResolveRecords(
	ctx context.Context,
	threadID string,
	checkpointID string,
) ([]map[string]any, error) {
	if authority.Store == nil {
		return nil, errors.New("checkpoint snapshot authority is unavailable")
	}
	groups, groupErr := authority.Store.ResolveOperationGroups(ctx, threadID, checkpointID)
	if groupErr == nil {
		return operationGroupsToRecords(groups)
	}
	if !errors.Is(groupErr, checkpointport.ErrNotFound) {
		return nil, groupErr
	}
	resolved, err := authority.Store.ResolveCheckpoint(ctx, threadID, checkpointID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(resolved, func(i, j int) bool {
		return resolved[i].Intent.MutationOrdinal < resolved[j].Intent.MutationOrdinal
	})
	byPath := map[string]map[string]any{}
	type pathState struct {
		initialExisted bool
		initialHash    string
		finalExisted   bool
		finalHash      string
	}
	states := map[string]pathState{}
	pathOrder := make([]string, 0, len(resolved))
	contextDigest := ""
	sourceCheckpointID := ""
	for _, snapshot := range resolved {
		intent := snapshot.Intent
		completion := snapshot.Completion
		if contextDigest == "" {
			contextDigest = intent.SecurityContext.ContextDigest
			sourceCheckpointID = intent.SourceWorkspaceCheckpointID
		} else if contextDigest != intent.SecurityContext.ContextDigest {
			return nil, errors.New("checkpoint snapshot materialization crosses security contexts")
		} else if sourceCheckpointID != intent.SourceWorkspaceCheckpointID {
			return nil, errors.New("checkpoint snapshot materialization crosses source checkpoint ids")
		}
		record := byPath[intent.RelativePath]
		if record == nil {
			record = map[string]any{
				"schemaVersion":               float64(2),
				"checkpointId":                intent.CheckpointID,
				"sourceWorkspaceCheckpointId": intent.SourceWorkspaceCheckpointID,
				"threadId":                    intent.SecurityContext.ThreadID,
				"turnId":                      intent.SecurityContext.TurnID,
				"workspace":                   intent.SecurityContext.WorkspaceRealPath,
				"relativePath":                intent.RelativePath,
				"createdAt":                   intent.CreatedAt,
				"contextDigest":               intent.SecurityContext.ContextDigest,
				"contextEpoch":                float64(intent.SecurityContext.ContextEpoch),
				"caseId":                      intent.SecurityContext.CaseID,
				"caseBindingHash":             intent.SecurityContext.CaseBindingHash,
				"datasetSnapshotId":           intent.SecurityContext.DatasetSnapshotID,
				"snapshotStorage":             "runtime_private_cas",
			}
			if intent.BeforeAvailable {
				record["beforeHash"] = intent.BeforeHash
				record["before"] = map[string]any{
					"hash": intent.BeforeHash, "content": intent.BeforeContent, "encoding": "utf8",
				}
			}
			byPath[intent.RelativePath] = record
			pathOrder = append(pathOrder, intent.RelativePath)
			states[intent.RelativePath] = pathState{
				initialExisted: intent.BeforeExisted, initialHash: intent.BeforeHash,
				finalExisted: completion.AfterExisted, finalHash: completion.AfterHash,
			}
		} else {
			state := states[intent.RelativePath]
			if state.finalExisted != intent.BeforeExisted ||
				state.finalExisted && (!intent.BeforeAvailable || state.finalHash != intent.BeforeHash) ||
				!state.finalExisted && (intent.BeforeAvailable || intent.BeforeHash != "" || intent.BeforeContent != "") {
				return nil, errors.New("checkpoint snapshot path history is disconnected")
			}
			state.finalExisted = completion.AfterExisted
			state.finalHash = completion.AfterHash
			states[intent.RelativePath] = state
		}
		record["updatedAt"] = completion.CompletedAt
		if completion.AfterHash != "" {
			record["afterHash"] = completion.AfterHash
		} else {
			delete(record, "afterHash")
		}
	}
	sort.Strings(pathOrder)
	records := make([]map[string]any, 0, len(pathOrder))
	for _, relativePath := range pathOrder {
		record := byPath[relativePath]
		state := states[relativePath]
		switch {
		case !state.initialExisted && !state.finalExisted:
			continue
		case state.initialExisted && state.finalExisted && state.initialHash != "" && state.initialHash == state.finalHash:
			continue
		case !state.initialExisted && state.finalExisted:
			record["changeKind"] = "created"
		case state.initialExisted && !state.finalExisted:
			record["changeKind"] = "deleted"
		case state.initialExisted && state.finalExisted:
			record["changeKind"] = "modified"
		default:
			return nil, errors.New("checkpoint snapshot path net state is invalid")
		}
		if state.finalExisted {
			record["afterHash"] = state.finalHash
		} else {
			delete(record, "afterHash")
		}
		records = append(records, record)
	}
	return records, nil
}

func operationGroupsToRecords(groups []domaincheckpoint.MaterializedOperationGroupV2) ([]map[string]any, error) {
	type pathState struct {
		initialExisted bool
		initialHash    string
		finalExisted   bool
		finalHash      string
	}
	byPath := map[string]map[string]any{}
	states := map[string]pathState{}
	pathOrder := make([]string, 0)
	contextDigest := ""
	sourceCheckpointID := ""
	for _, group := range groups {
		intent := group.Intent
		terminal := group.Terminal
		generatedCreation := intent.ToolName == "generate_office_document"
		if generatedCreation && (domaincheckpoint.ValidateOperationGroupIntentV2(intent) != nil ||
			domaincheckpoint.ValidateOperationGroupTerminalForIntentV2(terminal, intent) != nil ||
			terminal.Status != "completed" || len(intent.Paths) != 1 || intent.Paths[0].BeforeExisted) {
			return nil, errors.New("generated checkpoint creation authority is invalid")
		}
		if contextDigest == "" {
			contextDigest = intent.SecurityContext.ContextDigest
			sourceCheckpointID = intent.SourceWorkspaceCheckpointID
		} else if contextDigest != intent.SecurityContext.ContextDigest || sourceCheckpointID != intent.SourceWorkspaceCheckpointID {
			return nil, errors.New("checkpoint operation materialization crosses frozen authority")
		}
		observed := map[string]domaincheckpoint.ObservedOperationPathV2{}
		for _, item := range terminal.ObservedPaths {
			observed[materializedObservedOperationPathKey(item)] = item
		}
		for _, path := range intent.Paths {
			final, found := observed[materializedOperationPathKey(path)]
			if !found || final.ObservationStatus != "exact" || final.Existed != path.ExpectedAfterExisted || final.Hash != path.ExpectedAfterHash {
				return nil, errors.New("completed checkpoint operation lacks exact after authority")
			}
			pathKey := materializedOperationPathKey(path)
			record := byPath[pathKey]
			if record == nil {
				record = map[string]any{
					"schemaVersion":               float64(2),
					"checkpointId":                intent.CheckpointID,
					"sourceWorkspaceCheckpointId": intent.SourceWorkspaceCheckpointID,
					"threadId":                    intent.SecurityContext.ThreadID,
					"turnId":                      intent.SecurityContext.TurnID,
					"workspace":                   intent.SecurityContext.WorkspaceRealPath,
					"relativePath":                path.RelativePath,
					"createdAt":                   intent.CreatedAt,
					"contextDigest":               intent.SecurityContext.ContextDigest,
					"contextEpoch":                float64(intent.SecurityContext.ContextEpoch),
					"caseId":                      intent.SecurityContext.CaseID,
					"caseBindingHash":             intent.SecurityContext.CaseBindingHash,
					"datasetSnapshotId":           intent.SecurityContext.DatasetSnapshotID,
					"snapshotStorage":             "runtime_private_cas",
				}
				if path.PathAuthoritySchemaVersion != 0 {
					record["pathAuthoritySchemaVersion"] = float64(path.PathAuthoritySchemaVersion)
					record["authorityKind"] = path.AuthorityKind
					record["authorityRoot"] = path.AuthorityRoot
					record["authorityRootIdentity"] = path.AuthorityRootIdentity
					record["authorityRootHash"] = path.AuthorityRootHash
				}
				if path.BeforeAvailable {
					record["beforeHash"] = path.BeforeHash
					record["before"] = operationBeforeSnapshotRecord(path)
				}
				byPath[pathKey] = record
				pathOrder = append(pathOrder, pathKey)
				states[pathKey] = pathState{
					initialExisted: path.BeforeExisted, initialHash: path.BeforeHash,
					finalExisted: final.Existed, finalHash: final.Hash,
				}
			} else {
				state := states[pathKey]
				if state.finalExisted != path.BeforeExisted ||
					state.finalExisted && (!path.BeforeAvailable || state.finalHash != path.BeforeHash) ||
					!state.finalExisted && (path.BeforeAvailable || path.BeforeHash != "" || path.BeforeContent != "") {
					return nil, errors.New("checkpoint operation path history is disconnected")
				}
				state.finalExisted = final.Existed
				state.finalHash = final.Hash
				states[pathKey] = state
			}
			if generatedCreation {
				// Retain the operation's provenance, not a filename inference. The
				// native editor undo lane is independent of checkpoint file deletion.
				record["generatedOfficeCreation"] = true
			}
			record["updatedAt"] = terminal.SettledAt
		}
	}
	sort.Strings(pathOrder)
	records := make([]map[string]any, 0, len(pathOrder))
	for _, pathKey := range pathOrder {
		record := byPath[pathKey]
		state := states[pathKey]
		switch {
		case !state.initialExisted && !state.finalExisted:
			continue
		case state.initialExisted && state.finalExisted && state.initialHash == state.finalHash:
			continue
		case !state.initialExisted && state.finalExisted:
			record["changeKind"] = "created"
		case state.initialExisted && !state.finalExisted:
			record["changeKind"] = "deleted"
		case state.initialExisted && state.finalExisted:
			record["changeKind"] = "modified"
		default:
			return nil, errors.New("checkpoint operation net state is invalid")
		}
		if state.finalExisted {
			record["afterHash"] = state.finalHash
		}
		records = append(records, record)
	}
	return records, nil
}

func materializedOperationPathKey(path domaincheckpoint.OperationPathV2) string {
	if path.PathAuthoritySchemaVersion == 0 {
		return "legacy-workspace\x00" + path.RelativePath
	}
	return path.AuthorityKind + "\x00" + path.AuthorityRootHash + "\x00" + path.RelativePath
}

func materializedObservedOperationPathKey(path domaincheckpoint.ObservedOperationPathV2) string {
	if path.PathAuthoritySchemaVersion == 0 {
		return "legacy-workspace\x00" + path.RelativePath
	}
	return path.AuthorityKind + "\x00" + path.AuthorityRootHash + "\x00" + path.RelativePath
}

func operationBeforeSnapshotRecord(path domaincheckpoint.OperationPathV2) map[string]any {
	if path.BeforeSnapshotSchemaVersion == 1 {
		return map[string]any{
			"schemaVersion": float64(1), "hash": path.BeforeHash,
			"encoding": path.BeforeEncoding, "bytesBase64": path.BeforeBytesBase64,
		}
	}
	return map[string]any{"hash": path.BeforeHash, "content": path.BeforeContent, "encoding": "utf8"}
}

// FrozenContext returns the exact host-owned turn authority that captured a
// checkpoint. A later rewind may use a newer turn only after separately
// proving that the current context still has the same case, epoch, dataset,
// workspace, tenant, user, and publication policy binding.
func (authority SnapshotAuthority) FrozenContext(
	ctx context.Context,
	threadID string,
	checkpointID string,
) (domainsecurity.TurnSecurityContext, error) {
	if authority.Store == nil {
		return domainsecurity.TurnSecurityContext{}, errors.New("checkpoint snapshot authority is unavailable")
	}
	groups, groupErr := authority.Store.ResolveOperationGroups(ctx, threadID, checkpointID)
	if groupErr == nil {
		if len(groups) == 0 {
			return domainsecurity.TurnSecurityContext{}, checkpointport.ErrNotFound
		}
		frozen := groups[0].Intent.SecurityContext
		if domainsecurity.ValidateTurnSecurityContextForExecution(frozen) != nil {
			return domainsecurity.TurnSecurityContext{}, errors.New("checkpoint security context is not executable")
		}
		for _, group := range groups[1:] {
			if group.Intent.SecurityContext != frozen {
				return domainsecurity.TurnSecurityContext{}, errors.New("checkpoint operation authority crosses frozen turn contexts")
			}
		}
		return frozen, nil
	}
	if !errors.Is(groupErr, checkpointport.ErrNotFound) {
		return domainsecurity.TurnSecurityContext{}, groupErr
	}
	resolved, err := authority.Store.ResolveCheckpoint(ctx, threadID, checkpointID)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	if len(resolved) == 0 {
		return domainsecurity.TurnSecurityContext{}, checkpointport.ErrNotFound
	}
	frozen := resolved[0].Intent.SecurityContext
	if domainsecurity.ValidateTurnSecurityContextForExecution(frozen) != nil {
		return domainsecurity.TurnSecurityContext{}, errors.New("checkpoint security context is not executable")
	}
	for _, snapshot := range resolved[1:] {
		if snapshot.Intent.SecurityContext != frozen {
			return domainsecurity.TurnSecurityContext{}, errors.New("checkpoint snapshot authority crosses frozen turn contexts")
		}
	}
	return frozen, nil
}

func (authority SnapshotAuthority) CapturedMetadata(
	ctx context.Context,
	input CapturedAuthorityInput,
) (map[string]any, bool) {
	records, err := authority.ResolveRecords(ctx, input.ThreadID, input.CheckpointID)
	if err != nil {
		return nil, false
	}
	return CapturedMetadataFromAuthorityRecords(AuthorityMetadataInput{
		ThreadID: input.ThreadID, CheckpointID: input.CheckpointID, AuthorityWorkspace: input.AuthorityWorkspace,
		WorkspaceFallback: input.WorkspaceFallback, CreatedAtFallback: input.CreatedAtFallback, Records: records,
	})
}

func (authority SnapshotAuthority) CapturedEvent(
	ctx context.Context,
	input CapturedAuthorityInput,
) (map[string]any, error) {
	operationInventory, operationErr := authority.CapturedOperationInventory(ctx, input.ThreadID, input.CheckpointID)
	if operationErr == nil {
		if input.TurnID != "" && input.TurnID != operationInventory.SecurityContext.TurnID ||
			input.SourceWorkspaceCheckpointID != "" && input.SourceWorkspaceCheckpointID != operationInventory.SourceWorkspaceCheckpointID {
			return nil, errors.New("checkpoint captured audit caller does not match private authority")
		}
		if len(operationInventory.Audits) == 0 {
			return nil, checkpointport.ErrNotFound
		}
		return operationInventory.Audits[len(operationInventory.Audits)-1].Event, nil
	}
	if !errors.Is(operationErr, checkpointport.ErrNotFound) {
		return nil, operationErr
	}
	records, err := authority.ResolveRecords(ctx, input.ThreadID, input.CheckpointID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, checkpointport.ErrNotFound
	}
	event, ok := BuildCapturedCheckpointEvent(CapturedCheckpointInput{
		ThreadID: input.ThreadID, TurnID: input.TurnID, CheckpointID: input.CheckpointID,
		SourceWorkspaceCheckpointID: input.SourceWorkspaceCheckpointID,
		WorkspaceFallback:           input.WorkspaceFallback, CreatedAtFallback: input.CreatedAtFallback, Records: records,
	})
	if !ok {
		return nil, errors.New("checkpoint captured audit event is invalid")
	}
	return event, nil
}

func (authority SnapshotAuthority) Evidence(ctx context.Context, threadID, checkpointID string) map[string]map[string]any {
	records, err := authority.ResolveRecords(ctx, threadID, checkpointID)
	if err != nil {
		return nil
	}
	return SnapshotEvidenceFromRecords(records)
}
