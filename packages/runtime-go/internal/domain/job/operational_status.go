package job

import (
	"errors"
	"strings"
)

const PublicOperationalStatusUnknownV1 = "unknown"

// ValidateAutoContinueStatusV1 rejects open text at the durable update
// boundary. These values are host lifecycle control data, not display text.
func ValidateAutoContinueStatusV1(value string) error {
	if !ValidAutoContinueStatusV1(value) {
		return errors.New("job auto-continue status is outside the closed lifecycle allowlist")
	}
	return nil
}

func ValidAutoContinueStatusV1(value string) bool {
	switch strings.TrimSpace(value) {
	case "starting", "started", "skipped", "failed":
		return true
	default:
		return false
	}
}

func PublicAutoContinueStatusV1(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !ValidAutoContinueStatusV1(value) {
		return PublicOperationalStatusUnknownV1
	}
	return value
}

func ValidateCompletionDeliveryStatusV1(value string) error {
	if !ValidCompletionDeliveryStatusV1(value) {
		return errors.New("job completion-delivery status is outside the closed lifecycle allowlist")
	}
	return nil
}

func ValidCompletionDeliveryStatusV1(value string) bool {
	switch strings.TrimSpace(value) {
	case "pending", "retry", "delivered", "skipped", "dead_letter":
		return true
	default:
		return false
	}
}

func PublicCompletionDeliveryStatusV1(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !ValidCompletionDeliveryStatusV1(value) {
		return PublicOperationalStatusUnknownV1
	}
	return value
}

func ValidateRecoveryStatusV1(value string) error {
	if !ValidRecoveryStatusV1(value) {
		return errors.New("job recovery status is outside the closed lifecycle allowlist")
	}
	return nil
}

func ValidRecoveryStatusV1(value string) bool {
	switch strings.TrimSpace(value) {
	case "recovering", "recovered", "dead_lettered":
		return true
	default:
		return false
	}
}

func PublicRecoveryStatusV1(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !ValidRecoveryStatusV1(value) {
		return PublicOperationalStatusUnknownV1
	}
	return value
}

func ValidatePauseRequestStatusV1(value string) error {
	if !ValidPauseRequestStatusV1(value) {
		return errors.New("job pause-request status is outside the closed lifecycle allowlist")
	}
	return nil
}

func ValidPauseRequestStatusV1(value string) bool {
	switch strings.TrimSpace(value) {
	case "requested", "paused", "resumed", "rejected", "expired":
		return true
	default:
		return false
	}
}

func PublicPauseStatusV1(value string) string {
	value = strings.TrimSpace(value)
	if value == "resume_requested" || ValidPauseRequestStatusV1(value) {
		return value
	}
	return PublicOperationalStatusUnknownV1
}

func ValidatePauseRequestTransitionV1(before string, after string) error {
	before, after = strings.TrimSpace(before), strings.TrimSpace(after)
	if before == after {
		return nil
	}
	if !ValidPauseRequestStatusV1(before) || !ValidPauseRequestStatusV1(after) {
		return errors.New("job pause-request transition is invalid")
	}
	valid := false
	switch before {
	case "requested":
		valid = after == "paused" || after == "rejected" || after == "expired"
	case "paused":
		valid = after == "resumed" || after == "expired"
	}
	if !valid {
		return errors.New("job pause-request transition is invalid")
	}
	return nil
}

func PublicSteerStatusV1(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "queued", "admitted", "rejected", "expired", "blocked":
		return value
	default:
		return PublicOperationalStatusUnknownV1
	}
}

func validPersistedOperationalStatusV1(value string, valid func(string) bool) bool {
	value = strings.TrimSpace(value)
	return value == "" || valid(value)
}

func ValidatePersistedOperationalStatusesV1(record Record) error {
	if !validPersistedOperationalStatusV1(record.AutoContinueStatus, ValidAutoContinueStatusV1) {
		return errors.New("persisted job auto-continue status is invalid")
	}
	if !validPersistedOperationalStatusV1(record.CompletionDeliveryStatus, ValidCompletionDeliveryStatusV1) {
		return errors.New("persisted job completion-delivery status is invalid")
	}
	if !validPersistedOperationalStatusV1(record.RecoveryStatus, ValidRecoveryStatusV1) {
		return errors.New("persisted job recovery status is invalid")
	}
	return nil
}

// ValidateOperationalStatusTransitionV1 prevents a later update from
// reopening a settled lifecycle or bypassing the recovery sequence.
func ValidateOperationalStatusTransitionV1(before Record, after Record) error {
	if !validAutoContinueTransitionV1(before.AutoContinueStatus, after.AutoContinueStatus) {
		return errors.New("job auto-continue status transition is invalid")
	}
	if !validCompletionDeliveryTransitionV1(before.CompletionDeliveryStatus, after.CompletionDeliveryStatus) {
		return errors.New("job completion-delivery status transition is invalid")
	}
	if !validRecoveryTransitionV1(before.RecoveryStatus, after.RecoveryStatus) {
		return errors.New("job recovery status transition is invalid")
	}
	return ValidateOperationalStateV1(after)
}

func validAutoContinueTransitionV1(before string, after string) bool {
	before, after = strings.TrimSpace(before), strings.TrimSpace(after)
	if before == after {
		return true
	}
	if after == "" || !ValidAutoContinueStatusV1(after) {
		return false
	}
	switch before {
	case "":
		return true
	case "starting":
		return after == "started" || after == "skipped" || after == "failed"
	default:
		return false
	}
}

func validCompletionDeliveryTransitionV1(before string, after string) bool {
	before, after = strings.TrimSpace(before), strings.TrimSpace(after)
	if before == after {
		return true
	}
	if after == "" || !ValidCompletionDeliveryStatusV1(after) {
		return false
	}
	switch before {
	case "":
		return true
	case "pending", "retry":
		return after == "pending" || after == "retry" || after == "delivered" || after == "skipped" || after == "dead_letter"
	case "dead_letter":
		return after == "retry"
	default:
		return false
	}
}

func validRecoveryTransitionV1(before string, after string) bool {
	before, after = strings.TrimSpace(before), strings.TrimSpace(after)
	if before == after {
		return true
	}
	if after == "" || !ValidRecoveryStatusV1(after) {
		return false
	}
	switch before {
	case "":
		return true
	case "dead_lettered":
		return after == "recovering" || after == "recovered"
	case "recovering":
		return after == "recovered" || after == "dead_lettered"
	default:
		return false
	}
}

// ValidateOperationalStateV1 checks cross-field invariants after all updates
// have been applied. A valid token in the wrong lifecycle combination is not
// accepted merely because it belongs to an allowlist.
func ValidateOperationalStateV1(record Record) error {
	if err := ValidatePersistedOperationalStatusesV1(record); err != nil {
		return err
	}
	delivery := strings.TrimSpace(record.CompletionDeliveryStatus)
	recovery := strings.TrimSpace(record.RecoveryStatus)
	auto := strings.TrimSpace(record.AutoContinueStatus)
	if auto != "" && (!record.Background || !record.AutoContinueParent || !TerminalStatusV1(record.Status)) {
		return errors.New("auto-continue state requires an opted-in terminal background job")
	}
	if delivery == "dead_letter" && recovery != "dead_lettered" {
		return errors.New("dead-letter delivery requires dead-letter recovery state")
	}
	if recovery == "dead_lettered" && (delivery != "dead_letter" || strings.TrimSpace(record.DeadLetterReason) == "") {
		return errors.New("dead-letter recovery state requires dead-letter delivery and a reason code")
	}
	if recovery == "recovering" && delivery != "retry" && delivery != "pending" {
		return errors.New("recovering state requires retry or pending delivery state")
	}
	if delivery == "delivered" && (recovery == "recovering" || recovery == "dead_lettered") {
		return errors.New("delivered completion cannot retain an unresolved recovery state")
	}
	if delivery == "skipped" && recovery != "" {
		return errors.New("skipped completion cannot carry recovery state")
	}
	if (delivery == "pending" || delivery == "retry") && recovery == "recovered" {
		return errors.New("pending completion cannot be marked recovered")
	}
	if (delivery == "delivered" || delivery == "skipped" || recovery == "recovered") && strings.TrimSpace(record.DeadLetterReason) != "" {
		return errors.New("settled completion cannot retain a dead-letter reason")
	}
	return nil
}
