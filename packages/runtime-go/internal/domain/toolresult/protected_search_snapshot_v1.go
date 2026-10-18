package toolresult

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type ProtectedSearchMatchV1 struct {
	LineNumber    int      `json:"lineNumber"`
	Line          string   `json:"line"`
	Column        int      `json:"column"`
	ContextBefore []string `json:"contextBefore"`
	ContextAfter  []string `json:"contextAfter"`
}
type ProtectedSearchFileV1 struct {
	Path    string                   `json:"path"`
	Matches []ProtectedSearchMatchV1 `json:"matches"`
}

// Total is absent knowledge (JSON null) when the existing bounded scan did not
// complete. It is never inferred from a configured limit or an output alias.
type ProtectedSearchCaptureV1 struct {
	Shape          string                  `json:"shape"`
	Files          []ProtectedSearchFileV1 `json:"files,omitempty"`
	Paths          []string                `json:"paths,omitempty"`
	Total          *int                    `json:"total"`
	Truncated      bool                    `json:"truncated"`
	ToolIsError    bool                    `json:"toolIsError"`
	InputJSON      string                  `json:"inputJSON,omitempty"`
	InputTruncated bool                    `json:"inputTruncated"`
	StartedAt      string                  `json:"startedAt"`
}

// The sealed copy includes every retained match and context line, independent
// of the UI's head/tail preview and per-file disclosure. No spill is implied.
func ProtectedSearchTextV1(s ProtectedSearchCaptureV1) string {
	if s.Shape == "paths" {
		return strings.Join(s.Paths, "\n")
	}
	files := make([]string, 0, len(s.Files))
	for _, file := range s.Files {
		rows := []string{file.Path}
		for _, match := range file.Matches {
			for i, line := range match.ContextBefore {
				rows = append(rows, strconv.Itoa(match.LineNumber-len(match.ContextBefore)+i)+"- "+line)
			}
			rows = append(rows, strconv.Itoa(match.LineNumber)+": "+match.Line)
			for i, line := range match.ContextAfter {
				rows = append(rows, strconv.Itoa(match.LineNumber+i+1)+"- "+line)
			}
		}
		files = append(files, strings.Join(rows, "\n"))
	}
	return strings.Join(files, "\n\n")
}

func validateProtectedSearchV1(c ProtectedCaptureV1) error {
	s := c.Search
	if s == nil || !oneOf(s.Shape, "matches", "paths") || c.ExitCode != nil || c.ReadTarget != nil ||
		c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 || c.Web != nil || c.WebTargets != nil ||
		!protectedWebJSONV1(s.InputJSON) || s.InputTruncated && s.InputJSON != "" ||
		len(s.Files) > 500 || len(s.Paths) > 1000 || s.Shape == "matches" && len(s.Paths) != 0 || s.Shape == "paths" && len(s.Files) != 0 {
		return errors.New("protected search capture is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, s.StartedAt); err != nil || len(s.StartedAt) > 64 {
		return errors.New("protected search timing is invalid")
	}
	shown := len(s.Paths)
	seen := map[string]bool{}
	validPath := func(path string) bool {
		return path != "" && len(path) <= 4096 && utf8.ValidString(path) && !strings.ContainsAny(path, "\x00\r\n")
	}
	for _, path := range s.Paths {
		if !validPath(path) || seen[path] {
			return errors.New("protected search path is invalid")
		}
		seen[path] = true
	}
	for _, file := range s.Files {
		if !validPath(file.Path) || seen[file.Path] || len(file.Matches) == 0 {
			return errors.New("protected search file is invalid")
		}
		seen[file.Path] = true
		previous := 0
		for _, match := range file.Matches {
			if match.LineNumber <= previous || match.Column < 1 || match.ContextBefore == nil || match.ContextAfter == nil ||
				len(match.ContextBefore) > 20 || len(match.ContextAfter) > 20 ||
				len(match.ContextBefore) >= match.LineNumber {
				return errors.New("protected search match is invalid")
			}
			previous = match.LineNumber
			for _, text := range append(append([]string{match.Line}, match.ContextBefore...), match.ContextAfter...) {
				if len(text) > ProtectedSnapshotBodyLimitV1 || !utf8.ValidString(text) || strings.ContainsAny(text, "\x00\r\n") {
					return errors.New("protected search line is invalid")
				}
			}
		}
		shown += len(file.Matches)
	}
	if s.Shape == "matches" && shown > 500 || shown > 1000 || s.Total != nil && (*s.Total < shown || *s.Total < 0 || s.Truncated) ||
		s.ToolIsError && c.Status == "completed" || !s.ToolIsError && (c.Body != ProtectedSearchTextV1(*s) ||
		s.Total == nil && !s.Truncated || s.Total != nil && *s.Total > shown && !c.Truncated) {
		return errors.New("protected search count or body is invalid")
	}
	return nil
}

func cloneProtectedSearchV1(s *ProtectedSearchCaptureV1) *ProtectedSearchCaptureV1 {
	if s == nil {
		return nil
	}
	copy_ := *s
	if s.Total != nil {
		total := *s.Total
		copy_.Total = &total
	}
	copy_.Paths = append([]string(nil), s.Paths...)
	copy_.Files = make([]ProtectedSearchFileV1, len(s.Files))
	for i, file := range s.Files {
		copy_.Files[i] = file
		copy_.Files[i].Matches = make([]ProtectedSearchMatchV1, len(file.Matches))
		for j, match := range file.Matches {
			copy_.Files[i].Matches[j] = match
			copy_.Files[i].Matches[j].ContextBefore = append([]string{}, match.ContextBefore...)
			copy_.Files[i].Matches[j].ContextAfter = append([]string{}, match.ContextAfter...)
		}
	}
	return &copy_
}
