package toolresultsnapshot

import (
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	"context"
	"reflect"
)

type Service struct {
	Store     snapshotport.Store
	Authority snapshotport.AccessAuthority
}

type ReadTargetV1 struct {
	domaintoolresult.ProtectedReadTargetV1
	WorkspaceRoot string `json:"workspaceRoot"`
}

type PresentTargetsV1 struct {
	WorkspaceRoot string                                      `json:"workspaceRoot"`
	Files         []domaintoolresult.ProtectedPresentTargetV1 `json:"files"`
}

type DisplayV1 struct {
	PresentTargets *PresentTargetsV1                       `json:"presentTargets,omitempty"`
	SchemaVersion  int                                     `json:"schemaVersion"`
	SnapshotDigest string                                  `json:"snapshotDigest"`
	ResultItemID   string                                  `json:"resultItemId"`
	ToolName       string                                  `json:"toolName"`
	Capture        domaintoolresult.ProtectedCaptureV1     `json:"capture"`
	Inspect        *domaintoolresult.ProtectedInspectV1    `json:"inspect,omitempty"`
	ReadTarget     *ReadTargetV1                           `json:"readTarget,omitempty"`
	WebTargets     *domaintoolresult.ProtectedWebTargetsV1 `json:"webTargets,omitempty"`
}

func ValidateSelectorV1(s snapshotport.SelectorV1) bool {
	return domainthread.IsCanonicalRecordID(s.ThreadID) && domainthread.IsCanonicalRecordID(s.TurnID) &&
		domainmodel.IsHostToolCallIDV1(s.CallID) && s.ResultItemID == domaintoolresult.ToolResultItemIDV1(s.TurnID, s.CallID)
}

type PresentHistoryDisplayV1 struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	Kind          string                              `json:"kind"`
	ThreadID      string                              `json:"threadId"`
	TurnID        string                              `json:"turnId"`
	OrderedItems  []snapshotport.PresentHistoryItemV1 `json:"orderedItems"`
}

func ValidatePresentHistorySelectorV1(s snapshotport.PresentHistorySelectorV1) bool {
	return s.Kind == snapshotport.PresentHistoryKindV1 && domainthread.IsCanonicalRecordID(s.ThreadID) && domainthread.IsCanonicalRecordID(s.TurnID)
}

func validPresentHistoryAuthorizedV1(a snapshotport.PresentHistoryAuthorizedV1, selector snapshotport.PresentHistorySelectorV1) bool {
	if a.Selector != selector || domainidentity.ValidatePrincipalV1(a.Principal) != nil ||
		domainsecurity.ValidateTurnSecurityContext(a.Original) != nil || !domainsecurity.IsSHA256Hex(a.ThreadFileSHA256) ||
		a.Original.ThreadID != selector.ThreadID || a.Original.TurnID != selector.TurnID || a.Workspace != a.Original.WorkspaceRealPath ||
		a.Principal.TenantID != a.Original.TenantID || a.Principal.UserID != a.Original.UserID {
		return false
	}
	seen := map[string]bool{}
	for _, item := range a.OrderedItems {
		if !domainthread.IsCanonicalRecordID(item.ItemID) || seen[item.ItemID] {
			return false
		}
		seen[item.ItemID] = true
		switch item.Kind {
		case "assistant_text":
			if item.CallID != "" || item.ToolName != "" {
				return false
			}
		case "tool_result":
			if item.ToolName != "present" || !ValidateSelectorV1(snapshotport.SelectorV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, CallID: item.CallID, ResultItemID: item.ItemID}) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (s *Service) ReadPresentHistoryV1(ctx context.Context, selector snapshotport.PresentHistorySelectorV1) (PresentHistoryDisplayV1, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || !ValidatePresentHistorySelectorV1(selector) {
		return PresentHistoryDisplayV1{}, snapshotport.ErrUnavailable
	}
	owner, ok := s.Authority.(snapshotport.PresentHistoryAccessAuthorityV1)
	if !ok {
		return PresentHistoryDisplayV1{}, snapshotport.ErrUnavailable
	}
	authorized, release, err := owner.AcquirePresentHistoryCurrentV1(ctx, selector)
	if err != nil || release == nil {
		if release != nil {
			release()
		}
		return PresentHistoryDisplayV1{}, snapshotport.ErrUnavailable
	}
	defer release()
	if !validPresentHistoryAuthorizedV1(authorized, selector) || owner.ValidatePresentHistoryCurrentV1(ctx, authorized) != nil {
		return PresentHistoryDisplayV1{}, snapshotport.ErrUnavailable
	}
	items := append([]snapshotport.PresentHistoryItemV1{}, authorized.OrderedItems...)
	// The complete primary observation and admitted closing owner must still be
	// current. A concurrent append/change denies this response, never mixes reads.
	if owner.ValidatePresentHistoryCurrentV1(ctx, authorized) != nil || ctx.Err() != nil {
		return PresentHistoryDisplayV1{}, snapshotport.ErrUnavailable
	}
	return PresentHistoryDisplayV1{SchemaVersion: 1, Kind: snapshotport.PresentHistoryKindV1, ThreadID: selector.ThreadID, TurnID: selector.TurnID, OrderedItems: items}, nil
}

func (s *Service) Read(ctx context.Context, selector snapshotport.SelectorV1) (DisplayV1, error) {
	if s == nil || s.Store == nil || s.Authority == nil || ctx == nil || ctx.Err() != nil || !ValidateSelectorV1(selector) {
		return DisplayV1{}, snapshotport.ErrUnavailable
	}
	authority, release, err := s.Authority.AcquireCurrent(ctx, selector)
	if err != nil || release == nil {
		if release != nil {
			release()
		}
		return DisplayV1{}, snapshotport.ErrUnavailable
	}
	defer release()
	if authority.Selector != selector || s.Authority.ValidateCurrent(ctx, authority) != nil {
		return DisplayV1{}, snapshotport.ErrUnavailable
	}
	snapshot, err := s.Store.Read(ctx, authority.Binding)
	if err != nil || !matchesAuthority(snapshot, authority) {
		return DisplayV1{}, snapshotport.ErrUnavailable
	}
	// Final fresh check observes the same primary binding, Host principal and
	// current workspace/case/risk authority under the existing scope read gate.
	if s.Authority.ValidateCurrent(ctx, authority) != nil || ctx.Err() != nil {
		return DisplayV1{}, snapshotport.ErrUnavailable
	}
	capture := snapshot.Capture
	var target *ReadTargetV1
	if capture.ReadTarget != nil {
		target = &ReadTargetV1{ProtectedReadTargetV1: *capture.ReadTarget, WorkspaceRoot: snapshot.Workspace}
		capture.ReadTarget = nil
	}
	webTargets := capture.WebTargets
	if webTargets != nil {
		targets := *webTargets
		webTargets = &targets
		capture.WebTargets = nil
	}
	var presentTargets *PresentTargetsV1
	if capture.PresentTargets != nil {
		presentTargets = &PresentTargetsV1{WorkspaceRoot: snapshot.Workspace, Files: append([]domaintoolresult.ProtectedPresentTargetV1(nil), capture.PresentTargets...)}
		capture.PresentTargets = nil
	}
	inspect := domaintoolresult.InspectWithEventTimesV1(snapshot.Inspect, authority.CallEventAt, authority.ResultEventAt)
	return DisplayV1{PresentTargets: presentTargets, WebTargets: webTargets, ReadTarget: target, SchemaVersion: 1, SnapshotDigest: authority.Binding.SnapshotDigest, ResultItemID: selector.ResultItemID, ToolName: snapshot.ToolName, Capture: capture, Inspect: inspect}, nil
}

func matchesAuthority(s domaintoolresult.ProtectedSnapshotV1, a snapshotport.AuthorizedV1) bool {
	return domaintoolresult.ValidateProtectedDisplaySnapshotV1(s) == nil && domainidentity.SamePrincipalV1(s.Principal, a.Principal) &&
		s.Workspace == a.Workspace && s.ThreadID == a.Selector.ThreadID && s.TurnID == a.Selector.TurnID && s.CallID == a.Selector.CallID &&
		s.ResultItemID == a.Selector.ResultItemID && s.ToolName == a.ToolName && s.ContextDigest == a.ContextDigest && s.ContextEpoch == a.ContextEpoch &&
		s.ExecutionGrantID == a.ExecutionGrantID && s.CaseID == a.CaseID && s.CaseBindingHash == a.CaseBindingHash
}

// Existing signed owners provide read-only observations; this use case neither
// issues grants nor writes/reconstructs primary, job or context state.
type HistoricalScopeDependenciesV1 struct {
	Primary             recoveryport.PrimaryThreadReaderV1
	ObserveCaseContexts func(context.Context) (*casethreadapp.VerifiedCommittedContextInventoryV1, error)
	ObserveChildren     func(context.Context) (pendingworkapp.TrustedInventoryV1, []domainjob.Record, bool, error)
	// Denial-only composition of existing current authorities; never issues a grant.
	ValidateCurrentOrigin func(context.Context, map[string]any, domainsecurity.TurnSecurityContext) (domainsecurity.TurnSecurityContext, error)
}

// The current pointer must name an actual own frozen turn. Fork-projected
// history and a copied top-level securityState cannot supply this witness.
func CurrentOwnContextV1(thread map[string]any, original domainsecurity.TurnSecurityContext) (domainsecurity.TurnSecurityContext, error) {
	unavailable := domainsecurity.TurnSecurityContext{}
	id, _ := thread["id"].(string)
	historical, err := appturn.FrozenSecurityContextForTurn(thread, original.TurnID)
	if err != nil || id != original.ThreadID || historical != original {
		return unavailable, snapshotport.ErrUnavailable
	}
	current, found, err := turnsecurityapp.LatestContext(thread)
	if err != nil || !found || current.ThreadID != id || current.TenantID != original.TenantID || current.UserID != original.UserID ||
		current.WorkspaceRealPath != original.WorkspaceRealPath || current.CaseID != original.CaseID || current.CaseBindingHash != original.CaseBindingHash {
		return unavailable, snapshotport.ErrUnavailable
	}
	own, err := appturn.FrozenSecurityContextForTurn(thread, current.TurnID)
	if err != nil || own.ThreadID != id || own.TurnID != current.TurnID || own != current {
		return unavailable, snapshotport.ErrUnavailable
	}
	return current, nil
}

func ValidateHistoricalScopeV1(readCtx context.Context, originalThread map[string]any, original domainsecurity.TurnSecurityContext, dependencies HistoricalScopeDependenciesV1) error {
	if readCtx == nil || readCtx.Err() != nil || dependencies.Primary == nil || dependencies.ObserveCaseContexts == nil || dependencies.ObserveChildren == nil || dependencies.ValidateCurrentOrigin == nil {
		return snapshotport.ErrUnavailable
	}

	unavailable := snapshotport.ErrUnavailable
	var committed *casethreadapp.VerifiedCommittedContextInventoryV1
	requireCommitted := func(frozen domainsecurity.TurnSecurityContext) error {
		if !domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(frozen) {
			return nil
		}
		if committed == nil {
			verified, err := dependencies.ObserveCaseContexts(readCtx)
			if err != nil || verified == nil {
				return unavailable
			}
			committed = verified
		}
		exact, found := committed.CommittedContextV1(frozen.ThreadID, frozen.TurnID)
		if !found || exact != frozen {
			return unavailable
		}
		return nil
	}
	current, currentErr := CurrentOwnContextV1(originalThread, original)
	if currentErr != nil || requireCommitted(original) != nil || requireCommitted(current) != nil {
		return unavailable
	}
	inventory, records, legacy, err := dependencies.ObserveChildren(readCtx)
	if err != nil || legacy {
		return unavailable
	}
	if _, err := pendingworkapp.DeriveChildIdentityFloorsV1(inventory, records, nil); err != nil {
		return unavailable
	}
	signedChildren := map[string]bool{}
	for _, receipt := range inventory.Receipts {
		if receipt.ChildProducer != nil {
			for _, target := range receipt.ChildProducer.Children {
				signedChildren[target.ChildThreadID] = true
			}
		}
	}
	hasTurn := func(thread map[string]any, turnID string) (bool, error) {
		turns, _ := thread["turns"].([]any)
		seen := false
		for _, raw := range turns {
			turn, ok := raw.(map[string]any)
			if !ok {
				return false, unavailable
			}
			if turn["id"] == turnID {
				if seen {
					return false, unavailable
				}
				seen = true
			}
		}
		return seen, nil
	}
	thread := originalThread
	origin := original
	visited := map[string]bool{}
	for {
		threadID, ok := thread["id"].(string)
		if !ok || threadID == "" || visited[threadID] {
			return unavailable
		}
		visited[threadID] = true
		parentID, _ := thread["parentThreadId"].(string)
		relation, _ := thread["relation"].(string)
		if !signedChildren[threadID] {
			primary := relation == "primary" && parentID == ""
			sourceID, _ := thread["forkedFromThreadId"].(string)
			fork := (relation == "side" || relation == "fork") && domainthread.IsCanonicalRecordID(parentID) && parentID != threadID && sourceID == parentID
			if !primary && !fork {
				return unavailable
			}
			own, err := CurrentOwnContextV1(thread, origin)
			if err != nil {
				return unavailable
			}
			validated, err := dependencies.ValidateCurrentOrigin(readCtx, thread, origin)
			if err != nil || validated != own || requireCommitted(own) != nil {
				return unavailable
			}
			return readCtx.Err()
		}
		if relation != "side" || parentID == "" {
			return unavailable
		}
		var parentThread map[string]any
		witness := false
		child, err := dependencies.Primary.ReadPrimaryThreadSnapshotV1(readCtx, threadID)
		if err != nil {
			return unavailable
		}
		for _, record := range records {
			if record.Kind != "subagent" || record.ChildThreadID != threadID {
				continue
			}
			if record.ParentThreadID != parentID || record.SecurityBinding == nil {
				return unavailable
			}
			present, err := hasTurn(child.Thread, record.ChildTurnID)
			if err != nil {
				return unavailable
			}
			// An absent later reservation neither supplies nor revokes an older exact
			// origin witness. A present malformed first turn is not absence.
			if !present {
				continue
			}
			first, err := appturn.FrozenSecurityContextForTurn(child.Thread, record.ChildTurnID)
			if err != nil || first.ThreadID != threadID || first.TurnID != record.ChildTurnID || first.TenantID != original.TenantID || first.UserID != original.UserID || !domainjob.SecurityBindingMatchesWorkspaceScope(record.SecurityBinding, first) || requireCommitted(first) != nil {
				return unavailable
			}
			parent, err := dependencies.Primary.ReadPrimaryThreadSnapshotV1(readCtx, parentID)
			if err != nil {
				return unavailable
			}
			parentFrozen, err := appturn.FrozenSecurityContextForTurn(parent.Thread, record.ParentTurnID)
			if err != nil || parentFrozen.TenantID != original.TenantID || parentFrozen.UserID != original.UserID || requireCommitted(parentFrozen) != nil || !subagentapp.RecordMatchesSecurityContext(record, parentFrozen) {
				return unavailable
			}
			registry, err := executiongrantapp.RegistryFromThread(parentID, parent.Thread, record.ParentTurnID)
			if err != nil {
				return unavailable
			}
			entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, record.SecurityBinding.ParentExecutionGrantID)
			if !found || (entry.Status != domainsecurity.GrantRegistryActive && entry.Status != domainsecurity.GrantRegistrySettled) || executiongrantapp.VerifyThreadGrantMembership(parentID, parent.Thread, record.ParentTurnID, entry.Grant, domainsecurity.GrantRegistryActive, domainsecurity.GrantRegistrySettled) != nil {
				return unavailable
			}
			expected, err := domainjob.NewSecurityBinding(parentFrozen, entry.Grant, record.ParentToolCallID)
			if err != nil || !reflect.DeepEqual(expected, record.SecurityBinding) {
				return unavailable
			}
			witness = true
			parentThread = parent.Thread
			origin = parentFrozen
		}
		if !witness || parentThread == nil {
			return unavailable
		}
		thread = parentThread
	}

}
