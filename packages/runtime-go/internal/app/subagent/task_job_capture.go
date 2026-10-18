package subagent

import (
	"strings"
	"unicode/utf8"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// Copies the exact records returned by the authorized operation. It never
// reads a newer Job, reconstructs labels from prompt arguments, or interprets
// child output as model/evidence authority.
func TaskJobResultCaptureV1(operation string, result TaskJobServiceResult) (domaintoolresult.ProtectedCaptureV1, bool) {
	records := result.Records
	if result.Record.ID != "" {
		records = []domainjob.Record{result.Record}
	}
	if records == nil {
		return domaintoolresult.ProtectedCaptureV1{}, false
	}
	j := &domaintoolresult.ProtectedJobCaptureV1{Operation: operation, Jobs: make([]domaintoolresult.ProtectedJobRecordV1, 0), Total: len(records), Receipt: result.Receipt}
	if result.Receipt != nil {
		receipt := *result.Receipt
		j.Receipt = &receipt
	}
	for _, record := range records {
		if len(j.Jobs) == domaintoolresult.ProtectedJobListLimitV1 {
			j.Truncated = true
			break
		}
		observed := domaintoolresult.ProtectedJobRecordV1{JobID: publicTaskJobReferenceIDV1(record.ID), Kind: domainjob.PublicKindV1(record.Kind), Status: domainjob.PublicStatusV1(record.Status), Name: taskJobProducerTextV1(record.Name), Label: taskJobProducerTextV1(record.Label), Output: domaintoolresult.ProtectedJobOutputV1{Availability: "unavailable"}}
		if operation == "output" && result.Record.ID == record.ID && result.OutputCapture != nil {
			observed.Output = *result.OutputCapture
			if observed.Output.Body != nil {
				body := *observed.Output.Body
				observed.Output.Body = &body
			}
			if observed.Output.Truncated != nil {
				truncated := *observed.Output.Truncated
				observed.Output.Truncated = &truncated
			}
		} else if operation == "output" && record.SecurityBinding == nil && record.Output != "" {
			body, truncated := domaintoolresult.BoundProtectedTextV1(record.Output, domaintoolresult.ProtectedSnapshotBodyLimitV1)
			observed.Output = domaintoolresult.ProtectedJobOutputV1{Availability: "available", Body: &body, Truncated: &truncated}
		}
		j.Jobs = append(j.Jobs, observed)
	}
	status := "completed"
	if result.IsError {
		status = "failed"
	}
	c := domaintoolresult.ProtectedCaptureV1{Kind: "job", Status: status, Job: j}
	return c, domaintoolresult.ValidateProtectedCaptureV1(c) == nil
}

func taskJobProducerTextV1(value string) *string {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return nil
	}
	return &value
}
