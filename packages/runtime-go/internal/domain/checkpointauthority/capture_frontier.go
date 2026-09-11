package checkpointauthority

import (
	"encoding/json"
	"errors"
	"sort"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const OperationGroupCaptureFrontierPurpose = "analytix.checkpoint-capture-frontier/v2"

type OperationGroupCaptureFrontierV2 struct {
	CaptureEventID               string
	FrontierDigest               string
	SecurityContext              domainsecurity.TurnSecurityContext
	CheckpointID                 string
	SourceWorkspaceCheckpointID  string
	CreatedAt                    string
	CompletedOperationGroupCount int
}

type operationGroupCaptureFrontierEntryV2 struct {
	OperationOrdinal uint64 `json:"operationOrdinal"`
	OperationGroupID string `json:"operationGroupId"`
	IntentDigest     string `json:"intentDigest"`
	TerminalDigest   string `json:"terminalDigest"`
	Status           string `json:"status"`
}

type operationGroupCaptureFrontierIdentityV2 struct {
	Purpose                     string                                 `json:"purpose"`
	ContextDigest               string                                 `json:"contextDigest"`
	ThreadID                    string                                 `json:"threadId"`
	TurnID                      string                                 `json:"turnId"`
	CheckpointID                string                                 `json:"checkpointId"`
	SourceWorkspaceCheckpointID string                                 `json:"sourceWorkspaceCheckpointId"`
	Entries                     []operationGroupCaptureFrontierEntryV2 `json:"entries"`
}

func NewOperationGroupCaptureFrontierV2(states []OperationGroupStateV2) (OperationGroupCaptureFrontierV2, bool, error) {
	if len(states) == 0 {
		return OperationGroupCaptureFrontierV2{}, false, errors.New("checkpoint capture frontier is empty")
	}
	ordered := append([]OperationGroupStateV2(nil), states...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Intent.OperationOrdinal < ordered[j].Intent.OperationOrdinal
	})
	first := ordered[0].Intent
	identity := operationGroupCaptureFrontierIdentityV2{
		Purpose:       OperationGroupCaptureFrontierPurpose,
		ContextDigest: first.SecurityContext.ContextDigest,
		ThreadID:      first.SecurityContext.ThreadID, TurnID: first.SecurityContext.TurnID,
		CheckpointID: first.CheckpointID, SourceWorkspaceCheckpointID: first.SourceWorkspaceCheckpointID,
		Entries: make([]operationGroupCaptureFrontierEntryV2, 0, len(ordered)),
	}
	completed := 0
	for index, state := range ordered {
		intent := state.Intent
		if state.Terminal == nil || ValidateOperationGroupIntentV2(intent) != nil ||
			ValidateOperationGroupTerminalForIntentV2(*state.Terminal, intent) != nil ||
			intent.OperationOrdinal != uint64(index+1) || intent.SecurityContext != first.SecurityContext ||
			intent.CheckpointID != first.CheckpointID || intent.SourceWorkspaceCheckpointID != first.SourceWorkspaceCheckpointID {
			return OperationGroupCaptureFrontierV2{}, false, errors.New("checkpoint capture frontier authority is invalid")
		}
		switch state.Terminal.Status {
		case "completed":
			completed++
		case "no_effect":
		default:
			return OperationGroupCaptureFrontierV2{}, false, errors.New("checkpoint capture frontier contains a blocked terminal")
		}
		identity.Entries = append(identity.Entries, operationGroupCaptureFrontierEntryV2{
			OperationOrdinal: intent.OperationOrdinal, OperationGroupID: intent.OperationGroupID,
			IntentDigest: intent.IntentDigest, TerminalDigest: state.Terminal.TerminalDigest, Status: state.Terminal.Status,
		})
	}
	body, err := json.Marshal(identity)
	if err != nil {
		return OperationGroupCaptureFrontierV2{}, false, err
	}
	digest := domainsecurity.SHA256Hex(body)
	eventID := domaincheckpointref.CaptureEventID(digest)
	if eventID == "" {
		return OperationGroupCaptureFrontierV2{}, false, errors.New("checkpoint capture frontier id is invalid")
	}
	return OperationGroupCaptureFrontierV2{
		CaptureEventID: eventID, FrontierDigest: digest, SecurityContext: first.SecurityContext,
		CheckpointID: first.CheckpointID, SourceWorkspaceCheckpointID: first.SourceWorkspaceCheckpointID,
		CreatedAt: first.CreatedAt, CompletedOperationGroupCount: completed,
	}, completed != 0, nil
}
