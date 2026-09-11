package pendingwork

import (
	"context"
	"errors"
	"reflect"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ReportRestartInheritedHistoryV1 observes signed Original derivation and the
// complete execution inventory. It only qualifies inert history inside an
// exact whole-primary hold. It cannot issue a context, grant, or retry.
type ReportRestartInheritedHistoryV1 interface {
	ValidateInheritedTurnV1(context.Context, map[string]any, map[string]any) error
	Revalidate(context.Context) error
}

// ValidateInheritedTurnV1 carries the already qualified, denial-only Original
// history proof into startup readers. It neither reclassifies a new record nor
// grants execution. The private scope was constructed from complete signed
// inventories; its immutable whole-primary digest must still match.
func (scope ReportRestartScopeV1) ValidateInheritedTurnV1(ctx context.Context, thread, turn map[string]any) error {
	threadID, _ := thread["id"].(string)
	turnID, _ := turn["id"].(string)
	if scope.inheritedHistory == nil || threadID == "" || turnID == "" {
		return errors.New("sealed inherited history is unavailable")
	}
	if _, present := turn["securityContext"]; present {
		return errors.New("executed history cannot use an inherited observation")
	}
	snapshot, err := scope.ReadPrimaryThreadSnapshotV1(ctx, threadID)
	if err != nil {
		return err
	}
	for _, raw := range snapshot.Thread["turns"].([]any) {
		original := raw.(map[string]any)
		if original["id"] != turnID {
			continue
		}
		if _, present := original["securityContext"]; present || !reflect.DeepEqual(original, turn) {
			return errors.New("inherited observation differs from the sealed original turn")
		}
		return ctx.Err()
	}
	return errors.New("inherited observation is outside the sealed original inventory")
}

func (scope *ReportRestartScopeV1) observeOriginalTurnsV1(ctx context.Context, threadID string, thread map[string]any, inventory TrustedInventoryV1) error {
	executionSeen, inheritedSeen := false, false
	for _, raw := range thread["turns"].([]any) {
		turn := raw.(map[string]any) // The caller proved primary identity and shape.
		turnID := turn["id"].(string)
		value, present := turn["securityContext"]
		if !present {
			if executionSeen || scope.inheritedHistory == nil {
				return errors.New("report restart inherited history observation is unavailable")
			}
			// Every kind and disposition counts. A pending execution identity
			// cannot be reinterpreted as historical text by erasing its context.
			for _, receipt := range inventory.Receipts {
				if receipt.Context.ThreadID == threadID && receipt.Context.TurnID == turnID {
					return errors.New("report restart executed turn lost its context")
				}
			}
			if err := scope.inheritedHistory.ValidateInheritedTurnV1(ctx, thread, turn); err != nil {
				return err
			}
			inheritedSeen = true
		} else {
			frozen, err := domainsecurity.ParseTurnSecurityContext(value)
			if err != nil || domainsecurity.ValidateTurnSecurityContext(frozen) != nil || frozen.Version != domainsecurity.TurnSecurityContextVersionV2 || frozen.ThreadID != threadID || frozen.TurnID != turnID {
				return errors.New("report restart thread context inventory is incomplete or invalid")
			}
			executionSeen = true
			scope.contexts[frozen.ContextDigest] = frozen
		}
		// Historical IDs remain occupied even though they confer no context.
		scope.turnIDs = append(scope.turnIDs, turnID)
	}
	if inheritedSeen {
		return scope.inheritedHistory.Revalidate(ctx)
	}
	return ctx.Err()
}
