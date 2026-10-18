package filestore

import (
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

func searchObservationV1(shape string, args map[string]any, started time.Time) *domaintoolresult.ProtectedSearchCaptureV1 {
	s := &domaintoolresult.ProtectedSearchCaptureV1{Shape: shape, StartedAt: started.UTC().Format(time.RFC3339Nano)}
	raw, err := json.Marshal(args)
	encodedInput, encodeErr := json.Marshal(string(raw))
	if err != nil || encodeErr != nil || len(raw) > domaintoolresult.ProtectedWebSnapshotJSONLimitV1 || len(encodedInput) > domaintoolresult.ProtectedWebSnapshotJSONLimitV1 {
		s.InputTruncated = true
	} else {
		s.InputJSON = string(raw)
	}
	return s
}
func searchPathTextV1(path string) bool {
	return path != "" && len(path) <= 4096 && utf8.ValidString(path) && !strings.ContainsAny(path, "\x00\r\n")
}
func searchCaptureFitsV1(s *domaintoolresult.ProtectedSearchCaptureV1) bool {
	body := domaintoolresult.ProtectedSearchTextV1(*s)
	if !utf8.ValidString(body) || len(body) > domaintoolresult.ProtectedSnapshotBodyLimitV1 {
		return false
	}
	encoded, err := json.Marshal(domaintoolresult.ProtectedCaptureV1{Kind: "search", Status: "completed", Body: body, Label: "search", Search: s})
	// Reserve scalar growth from the measured duration and final tool label.
	return err == nil && len(encoded) <= domaintoolresult.ProtectedSnapshotEnvelopeLimitV1-(16<<10)-64
}
func captureSearchV1(ctx context.Context, s *domaintoolresult.ProtectedSearchCaptureV1, started time.Time, clipped bool, status string, label string) {
	label, _ = domaintoolresult.BoundProtectedTextV1(label, 1024)
	capture := domaintoolresult.ProtectedCaptureV1{Kind: "search", Status: status, Body: domaintoolresult.ProtectedSearchTextV1(*s), Label: label,
		DurationMs: max(0, time.Since(started).Milliseconds()), Truncated: clipped, Search: s}
	// Keep the original envelope budget, including worst-case JSON escaping.
	encoded, err := json.Marshal(capture)
	if err != nil || len(encoded) > domaintoolresult.ProtectedSnapshotEnvelopeLimitV1-(16<<10) {
		return
	}
	domaintoolresult.CaptureProtectedToolResultV1(ctx, capture)
}
func captureGrepResultV1(input WorkspaceSearchToolInput, result GrepScanResult, started time.Time, timedOut bool) {
	s := searchObservationV1("matches", input.Args, started)
	s.Truncated = result.LimitReached || !result.ScanComplete
	if result.ScanComplete {
		total := len(result.Matches)
		s.Total = &total
	}
	clipped := false
	for _, match := range result.Matches {
		if !searchPathTextV1(match.Path) {
			clipped = true
			break
		}
		item := domaintoolresult.ProtectedSearchMatchV1{LineNumber: match.Line, Line: match.Text, Column: match.Column,
			ContextBefore: append([]string{}, match.ContextBefore...), ContextAfter: append([]string{}, match.ContextAfter...)}
		index := len(s.Files) - 1
		newFile := index < 0 || s.Files[index].Path != match.Path
		if newFile {
			s.Files = append(s.Files, domaintoolresult.ProtectedSearchFileV1{Path: match.Path})
			index++
		}
		s.Files[index].Matches = append(s.Files[index].Matches, item)
		if !searchCaptureFitsV1(s) {
			s.Files[index].Matches = s.Files[index].Matches[:len(s.Files[index].Matches)-1]
			if newFile {
				s.Files = s.Files[:index]
			}
			clipped = true
			break
		}
	}
	status := "completed"
	if timedOut {
		status = "timeout"
	} else if input.Context != nil && input.Context.Err() != nil {
		status = "canceled"
	}
	captureSearchV1(input.Context, s, started, clipped, status, "grep")
}
func captureGlobResultV1(input WorkspaceSearchToolInput, result GlobFilesResult, started time.Time) {
	s := searchObservationV1("paths", input.Args, started)
	s.Truncated = result.LimitReached || !result.ScanComplete
	if result.ScanComplete {
		total := len(result.Matches)
		s.Total = &total
	}
	clipped := false
	for _, match := range result.Matches {
		if !searchPathTextV1(match.Path) {
			clipped = true
			break
		}
		s.Paths = append(s.Paths, match.Path)
		if !searchCaptureFitsV1(s) {
			s.Paths = s.Paths[:len(s.Paths)-1]
			clipped = true
			break
		}
	}
	captureSearchV1(input.Context, s, started, clipped, "completed", "glob")
}
func captureSearchFailureV1(input WorkspaceSearchToolInput, shape string, output any, failed bool, started time.Time) {
	if !failed {
		return
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return
	}
	body, clipped := domaintoolresult.BoundProtectedTextV1(string(raw), domaintoolresult.ProtectedSnapshotBodyLimitV1)
	s := searchObservationV1(shape, input.Args, started)
	s.ToolIsError = true
	capture := domaintoolresult.ProtectedCaptureV1{Kind: "search", Status: "failed", Body: body, Label: shape,
		DurationMs: max(0, time.Since(started).Milliseconds()), Truncated: clipped, Search: s}
	for {
		encoded, err := json.Marshal(capture)
		if err != nil {
			return
		}
		if len(encoded) <= domaintoolresult.ProtectedSnapshotEnvelopeLimitV1-(16<<10) {
			break
		}
		capture.Body, _ = domaintoolresult.BoundProtectedTextV1(capture.Body, len(capture.Body)/2)
		capture.Truncated = true
	}
	domaintoolresult.CaptureProtectedToolResultV1(input.Context, capture)
}
