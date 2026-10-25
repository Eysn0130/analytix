package toolresult

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// New retained-display count limit within the existing 256 KiB ordinary
// snapshot envelope. Total and Truncated expose omitted records; this neither
// changes the Job query nor creates a second storage/export authority.
const ProtectedJobListLimitV1 = 50

type ProtectedJobOutputV1 struct {
	Availability string  `json:"availability"`
	Body         *string `json:"body,omitempty"`
	Truncated    *bool   `json:"truncated,omitempty"`
}
type ProtectedJobRecordV1 struct {
	JobID  string               `json:"jobId"`
	Kind   string               `json:"kind"`
	Status string               `json:"status"`
	Name   *string              `json:"name"`
	Label  *string              `json:"label"`
	Output ProtectedJobOutputV1 `json:"output"`
}

// Cancellation acknowledgement and a wait timeout describe this operation.
// They never supply, or overwrite, the independently observed Job status.
type ProtectedJobReceiptV1 struct {
	Operation string `json:"operation"`
	Status    string `json:"status"`
}
type ProtectedJobCaptureV1 struct {
	Operation string                 `json:"operation"`
	Jobs      []ProtectedJobRecordV1 `json:"jobs"`
	Total     int                    `json:"total"`
	Truncated bool                   `json:"truncated"`
	Receipt   *ProtectedJobReceiptV1 `json:"receipt"`
}

func ValidateProtectedJobReceiptV1(value *ProtectedJobReceiptV1) error {
	if value == nil {
		return nil
	}
	if value.Operation == "cancel" && oneOf(value.Status, "cancellation-requested", "already-finished") ||
		value.Operation == "wait" && oneOf(value.Status, "completed", "timeout") {
		return nil
	}
	return errors.New("protected job operation receipt is invalid")
}

func validateProtectedJobCaptureV1(c ProtectedCaptureV1) error {
	j := c.Job
	if j == nil || !oneOf(j.Operation, "spawn", "list", "wait", "output", "cancel", "restart") ||
		!oneOf(c.Status, "completed", "failed") ||
		c.Body != "" || c.Label != "" || c.Truncated || c.DurationMs != 0 || c.ExitCode != nil || c.ReadTarget != nil || c.Web != nil || c.WebTargets != nil || c.Search != nil ||
		c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 || j.Jobs == nil || len(j.Jobs) > ProtectedJobListLimitV1 || j.Total < len(j.Jobs) ||
		j.Truncated != (j.Total > len(j.Jobs)) || ValidateProtectedJobReceiptV1(j.Receipt) != nil ||
		j.Receipt != nil && j.Receipt.Operation != j.Operation {
		return errors.New("protected job capture is invalid")
	}
	seen := map[string]bool{}
	for _, r := range j.Jobs {
		if !taskJobPublicIDPatternV1.MatchString(r.JobID) || seen[r.JobID] ||
			!oneOf(r.Kind, "background-shell", "subagent", "child-run", "parallel-child-run", "task", "parallel_task", "bash", "unknown") ||
			!validTaskJobWireStatusV1(r.Status) {
			return errors.New("protected job identity or status is invalid")
		}
		seen[r.JobID] = true
		for _, label := range []*string{r.Name, r.Label} {
			if label != nil && (!utf8.ValidString(*label) || len(*label) > 1024 || strings.ContainsRune(*label, 0)) {
				return errors.New("protected job producer display text is invalid")
			}
		}
		o := r.Output
		if o.Availability == "available" {
			if j.Operation != "output" || o.Body == nil || o.Truncated == nil || !utf8.ValidString(*o.Body) || len(*o.Body) > ProtectedSnapshotBodyLimitV1 {
				return errors.New("protected job output is invalid")
			}
		} else if o.Availability != "unavailable" || o.Body != nil || o.Truncated != nil {
			return errors.New("protected job output availability is invalid")
		}
	}
	return nil
}

func cloneProtectedJobCaptureV1(value *ProtectedJobCaptureV1) *ProtectedJobCaptureV1 {
	if value == nil {
		return nil
	}
	copy_ := *value
	copy_.Jobs = append(make([]ProtectedJobRecordV1, 0, len(value.Jobs)), value.Jobs...)
	for i := range copy_.Jobs {
		r := &copy_.Jobs[i]
		if r.Name != nil {
			v := *r.Name
			r.Name = &v
		}
		if r.Label != nil {
			v := *r.Label
			r.Label = &v
		}
		if r.Output.Body != nil {
			v := *r.Output.Body
			r.Output.Body = &v
		}
		if r.Output.Truncated != nil {
			v := *r.Output.Truncated
			r.Output.Truncated = &v
		}
	}
	if value.Receipt != nil {
		receipt := *value.Receipt
		copy_.Receipt = &receipt
	}
	return &copy_
}
