package event

import (
	"errors"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

var ErrRawFailurePersistence = errors.New("raw failure content is not persistable")

// validateClosedFailureProjectionV1 permits only host-authored failure
// records. Provider, tool, process, and job errors may remain on ephemeral
// protocol paths, but their raw strings have no durable event representation.
func validateClosedFailureProjectionV1(value any) error {
	return validateClosedFailureProjectionTreeV1(closedUsageFailureValidationViewV1(value))
}

// Within generic public-record validation, only a root usage event may classify
// this closed telemetry enum as metadata. The original record still passes
// every other public-record validator, and the terminal CAS/outbox remains
// responsible for publication authority. Nested lookalikes never acquire this
// exception through recursive validation.
func closedUsageFailureValidationViewV1(value any) any {
	record, ok := value.(map[string]any)
	if !ok || record["kind"] != "usage" || !generalTerminalDeliveryAllowedKeysV1(record,
		[]string{"kind", "threadId", "turnId", "model", "usage", "cacheDiagnostics", "timestamp"},
		append([]string{"seq", "usageSource", "childRunId", "usageFinalStatus"}, generalTerminalDeliveryPublicationMarkerNames...),
	) {
		return value
	}
	_, modelOK := record["model"].(string)
	usage, usageOK := record["usage"].(map[string]any)
	diagnostics, diagnosticsOK := record["cacheDiagnostics"].(map[string]any)
	if !modelOK || !generalTerminalDeliveryPublicIDV1(contracts.StringField(record, "threadId")) ||
		!generalTerminalDeliveryPublicIDV1(contracts.StringField(record, "turnId")) ||
		!generalTerminalDeliveryTimestampV1(contracts.StringField(record, "timestamp")) ||
		!usageOK || !diagnosticsOK || diagnostics["providerAttemptTelemetryError"] != "invalid_or_incomplete" {
		return value
	}
	diagnosticView := ClosedTerminalTelemetryFailureValidationViewV1(usage, diagnostics)
	if _, present := diagnosticView["providerAttemptTelemetryError"]; present {
		return value
	}
	view := make(map[string]any, len(record))
	for key, child := range record {
		view[key] = child
	}
	view["cacheDiagnostics"] = diagnosticView
	return view
}

// ClosedTerminalTelemetryFailureValidationViewV1 classifies only the fixed
// host telemetry error enum after validating the complete real usage/diagnostic
// pair. Callers must check the original inputs' resource limits first, then run
// every public-record scanner on usage and the returned diagnostic view. This
// view grants no event identity or publication authority and never changes the
// original payload. Unrecognized pairs retain the ordinary strict scan.
func ClosedTerminalTelemetryFailureValidationViewV1(usage, diagnostics map[string]any) map[string]any {
	if diagnostics["providerAttemptTelemetryError"] != "invalid_or_incomplete" ||
		!domainterminaltelemetry.ValidateTerminalTelemetryPublicMapsV1(usage, diagnostics) {
		return diagnostics
	}
	view := make(map[string]any, len(diagnostics)-1)
	for key, child := range diagnostics {
		if key != "providerAttemptTelemetryError" {
			view[key] = child
		}
	}
	return view
}

func validateClosedFailureProjectionTreeV1(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		acceptedFinalFailure := isAcceptedFinalFailureProjectionV1(typed)
		stagedTerminalFailure := isStagedTerminalFailureItemV1(typed)
		closedFailure := isClosedFailureRecordV1(typed)
		kind, _ := typed["kind"].(string)
		kind = strings.ToLower(strings.TrimSpace(kind))
		if (kind == "error" || strings.HasSuffix(kind, "_error")) &&
			!closedFailure && !acceptedFinalFailure && !stagedTerminalFailure {
			return ErrRawFailurePersistence
		}
		for key, child := range typed {
			normalized := normalizePrivateKey(key)
			if normalized == "providererror" {
				if !isClosedProviderDiagnosticV1(child) {
					return ErrRawFailurePersistence
				}
				continue
			}
			if isRawFailureContentKeyV1(normalized) {
				if normalized == "error" && (closedFailure || acceptedFinalFailure) {
					continue
				}
				if child == nil {
					continue
				}
				if text, ok := child.(string); ok && strings.TrimSpace(text) == "" {
					continue
				}
				return ErrRawFailurePersistence
			}
			if err := validateClosedFailureProjectionTreeV1(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateClosedFailureProjectionTreeV1(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func isClosedProviderDiagnosticV1(value any) bool {
	record, ok := value.(map[string]any)
	if !ok || len(record) == 0 {
		return false
	}
	return domainfailure.ValidatePublicDetails(record)
}

// Accepted-final failures use a separate closed, host-issued terminal
// vocabulary. Their event ID and payload digest bind the exact public bytes;
// the batch/registry layer still owns publication authority.
func isAcceptedFinalFailureProjectionV1(record map[string]any) bool {
	commitID := strings.TrimSpace(contracts.StringField(record, "publicationCommitId"))
	slot := strings.TrimSpace(contracts.StringField(record, "publicationSlot"))
	if !domainsecurity.IsSHA256Hex(commitID) || contracts.StringField(record, "acceptedFinalDigest") != commitID ||
		contracts.StringField(record, "publicationEventId") != acceptedFinalDeliveryEventID(commitID, slot) ||
		contracts.StringField(record, "publicationPayloadDigest") != acceptedFinalDeliveryPayloadDigest(record) {
		return false
	}
	switch slot {
	case "terminal-error-item":
		item, ok := record["item"].(map[string]any)
		if !ok || contracts.StringField(record, "kind") != "item_completed" || contracts.StringField(item, "kind") != "error" ||
			contracts.StringField(item, "acceptedFinalDigest") != commitID || contracts.StringField(item, "id") == "" ||
			contracts.StringField(record, "itemId") != contracts.StringField(item, "id") {
			return false
		}
		return isStagedTerminalFailureItemV1(item)
	case "terminal":
		reason := strings.TrimSpace(contracts.StringField(record, "terminalReason"))
		projection, ok := domainterminal.FailureProjectionV1(reason)
		if !ok || contracts.StringField(record, "kind") != "turn_"+projection.Status ||
			contracts.StringField(record, "status") != projection.Status ||
			contracts.StringField(record, "code") != projection.Code {
			return false
		}
		if projection.Status != "failed" {
			_, hasError := record["error"]
			return !hasError
		}
		return contracts.StringField(record, "message") == projection.Message &&
			contracts.StringField(record, "error") == projection.Message
	default:
		return false
	}
}

func isStagedTerminalFailureItemV1(record map[string]any) bool {
	if !exactFailureProjectionKeysV1(record, []string{
		"id", "turnId", "threadId", "role", "status", "kind", "createdAt", "finishedAt",
		"code", "message", "severity", "acceptedFinalDigest",
	}) {
		return false
	}
	turnID := strings.TrimSpace(contracts.StringField(record, "turnId"))
	threadID := strings.TrimSpace(contracts.StringField(record, "threadId"))
	code := strings.TrimSpace(contracts.StringField(record, "code"))
	reason := strings.TrimPrefix(code, "case_terminal_")
	projection, ok := domainterminal.FailureProjectionV1(reason)
	createdAt := strings.TrimSpace(contracts.StringField(record, "createdAt"))
	parsedAt, timeErr := time.Parse(time.RFC3339Nano, createdAt)
	return ok && code == projection.Code && turnID != "" && threadID != "" &&
		contracts.StringField(record, "id") == "item_"+turnID+"_case_terminal" &&
		contracts.StringField(record, "role") == "system" && contracts.StringField(record, "kind") == "error" &&
		contracts.StringField(record, "status") == projection.Status && contracts.StringField(record, "message") == projection.Message &&
		contracts.StringField(record, "severity") == projection.Severity && domainsecurity.IsSHA256Hex(contracts.StringField(record, "acceptedFinalDigest")) &&
		timeErr == nil && parsedAt.Location() == time.UTC && parsedAt.Format(time.RFC3339Nano) == createdAt &&
		contracts.StringField(record, "finishedAt") == createdAt
}

func exactFailureProjectionKeysV1(record map[string]any, expected []string) bool {
	if len(record) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, ok := record[key]; !ok {
			return false
		}
	}
	return true
}

func isRawFailureContentKeyV1(normalized string) bool {
	switch normalized {
	case "iserror", "errorcode":
		return false
	case "error", "stderr", "stack", "stacktrace", "traceback", "exception", "exceptionmessage":
		return true
	}
	return strings.HasSuffix(normalized, "error") || strings.HasSuffix(normalized, "errormessage")
}

func isClosedFailureRecordV1(record map[string]any) bool {
	code, codeOK := record["code"].(string)
	message, messageOK := record["message"].(string)
	if !codeOK || !messageOK || !failureProjectionExactTextV1(code) ||
		!failureProjectionExactTextV1(message) {
		return false
	}
	expected := domainfailure.New(code, nil)
	if expected.Code() != code || expected.Message() != message {
		return false
	}
	if severity, present := record["severity"]; present {
		text, ok := severity.(string)
		if !ok || !failureProjectionExactTextV1(text) || text != expected.Severity() {
			return false
		}
	}
	if raw, present := record["error"]; present {
		text, ok := raw.(string)
		if !ok || !failureProjectionExactTextV1(text) || text != message {
			return false
		}
	}
	if raw, present := record["details"]; present {
		details, ok := raw.(map[string]any)
		if !ok || !domainfailure.ValidatePublicDetails(details) {
			return false
		}
	}
	return true
}

func failureProjectionExactTextV1(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		!strings.HasPrefix(value, "\ufeff") && !strings.HasSuffix(value, "\ufeff")
}
