package subagent

import (
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

const taskJobSummarySchemaVersionV1 = 1

// TaskJobMetadataProjectionV1 is the only public projection for task-job
// list, wait, get, kill, context, and diagnostic-summary surfaces. It is a
// closed metadata allowlist. A record's kind, lineage markers, missing binding,
// or historical readability can never re-enable output or diagnostics.
func TaskJobMetadataProjectionV1(record domainjob.Record, active *bool) map[string]any {
	jobID := publicTaskJobReferenceIDV1(record.ID)
	out := map[string]any{
		"schemaVersion":     taskJobSummarySchemaVersionV1,
		"id":                jobID,
		"kind":              domainjob.PublicKindV1(record.Kind),
		"status":            domainjob.PublicStatusV1(record.Status),
		"background":        record.Background,
		"terminal":          domainjob.TerminalStatusV1(record.Status),
		"outputWithheld":    true,
		"outputTrustStatus": untrustedChildOutputStatus,
		"factAnswerAllowed": false,
		"evidenceAuthority": false,
		"canReadOutput":     false,
		"canContinueParent": false,
	}
	if active != nil {
		out["active"] = *active
	}
	return out
}

// TaskJobSummaryResponseV1 keeps the historical function signature for
// callers while delegating to the single closed public projection above.
func TaskJobSummaryResponseV1(record domainjob.Record, _ time.Time, _ time.Duration, active *bool) map[string]any {
	return TaskJobMetadataProjectionV1(record, active)
}
