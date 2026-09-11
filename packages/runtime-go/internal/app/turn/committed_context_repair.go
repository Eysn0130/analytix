package turn

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type CommittedContextRepairInput struct {
	Thread          map[string]any
	SecurityContext domainsecurity.TurnSecurityContext
	EpochState      domaincontextepoch.State
}

func RepairCommittedTurnContext(input CommittedContextRepairInput) (map[string]any, error) {
	context := input.SecurityContext
	state := input.EpochState
	if input.Thread == nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(context) != nil ||
		domaincontextepoch.ValidateState(state) != nil || state.ThreadID != context.ThreadID || state.AcceptedSnapshot.Epoch != context.ContextEpoch ||
		strings.TrimSpace(stringField(input.Thread, "id")) != context.ThreadID {
		return nil, errors.New("committed turn context repair input is invalid")
	}
	thread := cloneCommittedContextValue(input.Thread)
	turns, _ := thread["turns"].([]any)
	matched := false
	active := false
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if strings.TrimSpace(stringField(turn, "id")) != context.TurnID {
			continue
		}
		matched = true
		status := strings.TrimSpace(stringField(turn, "status"))
		active = status == "running" || status == "queued" || status == "waiting"
		if !active && status != "completed" && status != "failed" && status != "aborted" {
			return nil, errors.New("committed turn context repair status is invalid")
		}
		if err := repairSecurityContextValue(turn, "securityContext", context); err != nil {
			return nil, err
		}
		if err := repairEpochSnapshotValue(turn, "contextEpochSnapshot", state.AcceptedSnapshot); err != nil {
			return nil, err
		}
		if value := turn["acceptedFinal"]; value != nil {
			record, err := domainevidence.ParseAcceptedFinalRecord(value)
			if err != nil || record.ThreadID != context.ThreadID || record.TurnID != context.TurnID || record.ContextDigest != context.ContextDigest ||
				record.ContextEpoch != context.ContextEpoch || record.DatasetSnapshotID != context.DatasetSnapshotID {
				return nil, errors.New("committed turn context contradicts accepted-final authority")
			}
		}
		break
	}
	if !matched {
		return nil, errors.New("committed turn context repair target is missing")
	}
	if active {
		if err := repairSecurityContextValue(thread, "securityState", context); err != nil {
			return nil, err
		}
		if err := repairEpochStateValue(thread, "contextEpochState", state); err != nil {
			return nil, err
		}
	}
	return thread, nil
}

func repairSecurityContextValue(record map[string]any, key string, expected domainsecurity.TurnSecurityContext) error {
	if current, err := domainsecurity.ParseTurnSecurityContext(record[key]); err == nil {
		if !reflect.DeepEqual(current, expected) {
			return errors.New("committed turn security context conflicts with public state")
		}
		return nil
	}
	record[key] = committedContextRecord(expected)
	return nil
}

func repairEpochStateValue(record map[string]any, key string, expected domaincontextepoch.State) error {
	if current, err := domaincontextepoch.ParseState(record[key]); err == nil {
		if !reflect.DeepEqual(current, expected) {
			return errors.New("committed context epoch state conflicts with public state")
		}
		return nil
	}
	record[key] = committedContextRecord(expected)
	return nil
}

func repairEpochSnapshotValue(record map[string]any, key string, expected domaincontextepoch.Snapshot) error {
	if current, err := parseEpochSnapshot(record[key]); err == nil {
		if !reflect.DeepEqual(current, expected) {
			return errors.New("committed context epoch snapshot conflicts with public state")
		}
		return nil
	}
	record[key] = committedContextRecord(expected)
	return nil
}

func parseEpochSnapshot(value any) (domaincontextepoch.Snapshot, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return domaincontextepoch.Snapshot{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var snapshot domaincontextepoch.Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return domaincontextepoch.Snapshot{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return domaincontextepoch.Snapshot{}, errors.New("context epoch snapshot contains trailing JSON")
	}
	return snapshot, domaincontextepoch.ValidateSnapshot(snapshot)
}

func committedContextRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func cloneCommittedContextValue(value map[string]any) map[string]any {
	body, _ := json.Marshal(value)
	cloned := map[string]any{}
	_ = json.Unmarshal(body, &cloned)
	return cloned
}
