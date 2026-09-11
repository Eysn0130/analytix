package gatecontinuation

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type EventOutboxDependencies struct {
	GetThread   func(string) (map[string]any, error)
	LoadEvents  func(string) ([]map[string]any, error)
	RecordEvent func(map[string]any) error
}

func RecordPendingGateRequestV1(event map[string]any, deps EventOutboxDependencies) error {
	if event == nil || deps.GetThread == nil || deps.LoadEvents == nil || deps.RecordEvent == nil {
		return errors.New("pending gate request event authority is unavailable")
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	baseKey := PendingGateRequestBaseKeyV1(event)
	if threadID == "" || baseKey == "" {
		return errors.New("pending gate request event identity is invalid")
	}
	thread, err := deps.GetThread(threadID)
	if err != nil {
		return err
	}
	expected, err := appturn.SanitizeCaseEventPublication(thread, event)
	if err != nil {
		return err
	}
	match := func() (bool, error) {
		events, loadErr := deps.LoadEvents(threadID)
		if loadErr != nil {
			return false, loadErr
		}
		matches := 0
		for _, existing := range events {
			if PendingGateRequestBaseKeyV1(existing) != baseKey {
				continue
			}
			if !ExactGateEventProjectionV1(existing, expected) {
				return false, errors.New("pending gate already has a conflicting durable request")
			}
			matches++
		}
		if matches > 1 {
			return false, errors.New("pending gate has duplicate durable requests")
		}
		return matches == 1, nil
	}
	return recordAndVerifyEventV1(event, match, deps.RecordEvent, "pending gate request event write verification failed")
}

func RecordPendingGateResolutionV1(event map[string]any, deps EventOutboxDependencies) error {
	if event == nil || deps.LoadEvents == nil || deps.RecordEvent == nil {
		return errors.New("pending gate resolution event authority is unavailable")
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	key := PendingGateResolutionKeyV1(event)
	baseKey := PendingGateResolutionBaseKeyV1(event)
	if threadID == "" || key == "" || baseKey == "" {
		return errors.New("pending gate resolution event identity is invalid")
	}
	match := func() (bool, error) {
		events, err := deps.LoadEvents(threadID)
		if err != nil {
			return false, err
		}
		matches := 0
		for _, existing := range events {
			if PendingGateResolutionBaseKeyV1(existing) != baseKey {
				continue
			}
			if PendingGateResolutionKeyV1(existing) != key {
				return false, errors.New("pending gate already has a conflicting durable resolution")
			}
			matches++
		}
		if matches > 1 {
			return false, errors.New("pending gate has duplicate durable resolutions")
		}
		return matches == 1, nil
	}
	return recordAndVerifyEventV1(event, match, deps.RecordEvent, "pending gate resolution event write verification failed")
}

func RecordApprovalGrantTransitionV1(event map[string]any, deps EventOutboxDependencies) error {
	if event == nil || deps.GetThread == nil || deps.LoadEvents == nil || deps.RecordEvent == nil {
		return errors.New("approval grant transition event authority is unavailable")
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	baseKey := ApprovalGrantTransitionBaseKeyV1(event)
	if threadID == "" || baseKey == "" {
		return errors.New("approval grant transition event identity is invalid")
	}
	thread, err := deps.GetThread(threadID)
	if err != nil {
		return err
	}
	expected, err := appturn.SanitizeCaseEventPublication(thread, event)
	if err != nil {
		return err
	}
	match := func() (bool, error) {
		events, loadErr := deps.LoadEvents(threadID)
		if loadErr != nil {
			return false, loadErr
		}
		matches := 0
		for _, existing := range events {
			if ApprovalGrantTransitionBaseKeyV1(existing) != baseKey {
				continue
			}
			if !ExactGateEventProjectionV1(existing, expected) {
				return false, errors.New("approval already has a conflicting durable grant transition")
			}
			matches++
		}
		if matches > 1 {
			return false, errors.New("approval has duplicate durable grant transitions")
		}
		return matches == 1, nil
	}
	return recordAndVerifyEventV1(event, match, deps.RecordEvent, "approval grant transition event write verification failed")
}

func recordAndVerifyEventV1(
	event map[string]any,
	match func() (bool, error),
	record func(map[string]any) error,
	verificationError string,
) error {
	if found, err := match(); err != nil || found {
		return err
	}
	if err := record(event); err != nil {
		if found, readErr := match(); readErr == nil && found {
			return nil
		}
		return err
	}
	found, err := match()
	if err != nil {
		return err
	}
	if !found {
		return errors.New(verificationError)
	}
	return nil
}

func PendingGateRequestBaseKeyV1(event map[string]any) string {
	kind := strings.TrimSpace(contracts.StringField(event, "kind"))
	id := ""
	switch kind {
	case "approval_requested":
		id = strings.TrimSpace(contracts.StringField(event, "approvalId"))
	case "user_input_requested":
		id = strings.TrimSpace(contracts.StringField(event, "inputId"))
	default:
		return ""
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	turnID := strings.TrimSpace(contracts.StringField(event, "turnId"))
	if threadID == "" || turnID == "" || id == "" {
		return ""
	}
	return strings.Join([]string{kind, threadID, turnID, id}, "\x00")
}

func PendingGateResolutionBaseKeyV1(event map[string]any) string {
	kind := strings.TrimSpace(contracts.StringField(event, "kind"))
	id := ""
	switch kind {
	case "approval_resolved":
		id = strings.TrimSpace(contracts.StringField(event, "approvalId"))
	case "user_input_resolved":
		id = strings.TrimSpace(contracts.StringField(event, "inputId"))
	default:
		return ""
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	turnID := strings.TrimSpace(contracts.StringField(event, "turnId"))
	if threadID == "" || turnID == "" || id == "" {
		return ""
	}
	return strings.Join([]string{kind, threadID, turnID, id}, "\x00")
}

func PendingGateResolutionKeyV1(event map[string]any) string {
	base := PendingGateResolutionBaseKeyV1(event)
	itemID := strings.TrimSpace(contracts.StringField(event, "itemId"))
	status := strings.TrimSpace(contracts.StringField(event, "status"))
	reason := strings.TrimSpace(contracts.StringField(event, "cancelledBy"))
	if base == "" || itemID == "" || status == "" {
		return ""
	}
	return strings.Join([]string{base, itemID, status, reason}, "\x00")
}

func ApprovalGrantTransitionBaseKeyV1(event map[string]any) string {
	if strings.TrimSpace(contracts.StringField(event, "kind")) != "execution_grant_approved" {
		return ""
	}
	threadID := strings.TrimSpace(contracts.StringField(event, "threadId"))
	turnID := strings.TrimSpace(contracts.StringField(event, "turnId"))
	approvalID := strings.TrimSpace(contracts.StringField(event, "approvalId"))
	transitionID := strings.TrimSpace(contracts.StringField(event, "approvalTransitionId"))
	if threadID == "" || turnID == "" || approvalID == "" || !domainsecurity.IsSHA256Hex(transitionID) {
		return ""
	}
	return strings.Join([]string{"execution_grant_approved", threadID, turnID, approvalID}, "\x00")
}

func ExactGateEventProjectionV1(existing, expected map[string]any) bool {
	left := contracts.CloneMap(existing)
	right := contracts.CloneMap(expected)
	delete(left, "seq")
	delete(left, "timestamp")
	delete(right, "seq")
	delete(right, "timestamp")
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
