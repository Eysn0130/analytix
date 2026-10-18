package toolresult

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const ProtectedPresentMaxFilesV1 = 8

// A receipt retains declaration order and text. Its index is the zero-based
// ordinal in this call, never a model-supplied event or session sequence.
// The enclosing snapshot supplies the original turn/call/result identity.
type ProtectedPresentedFileV1 struct {
	Index       int     `json:"index"`
	Path        string  `json:"path"`
	Description *string `json:"description,omitempty"`
}

type ProtectedPresentReceiptV1 struct {
	Files []ProtectedPresentedFileV1 `json:"files"`
}

// Current-file navigation is independent of Read's frozen content digest.
// Targets are actual Host observations, separated from declaration text.
type ProtectedPresentTargetV1 struct {
	Index int    `json:"index"`
	Path  string `json:"path"`
}

func presentTextV1(value string) bool {
	return utf8.ValidString(value) && len(value) <= 4096 && !strings.ContainsRune(value, 0)
}

func ValidateProtectedPresentReceiptV1(receipt *ProtectedPresentReceiptV1) error {
	if receipt == nil || len(receipt.Files) < 1 || len(receipt.Files) > ProtectedPresentMaxFilesV1 {
		return errors.New("protected present receipt is invalid")
	}
	for index, file := range receipt.Files {
		if file.Index != index || !presentTextV1(file.Path) || strings.TrimSpace(file.Path) == "" ||
			file.Description != nil && !presentTextV1(*file.Description) {
			return errors.New("protected present declaration is invalid")
		}
	}
	return nil
}

func ValidateProtectedPresentTargetsV1(receipt *ProtectedPresentReceiptV1, targets []ProtectedPresentTargetV1) error {
	if ValidateProtectedPresentReceiptV1(receipt) != nil || len(targets) != len(receipt.Files) {
		return errors.New("protected present targets are invalid")
	}
	for index, target := range targets {
		if target.Index != index || !presentTextV1(target.Path) || !filepath.IsAbs(target.Path) || filepath.Clean(target.Path) != target.Path {
			return errors.New("protected present target is invalid")
		}
	}
	return nil
}

func validateProtectedPresentCaptureV1(c ProtectedCaptureV1) error {
	if c.Status != "completed" || c.Body != "" || c.Label != "" || c.Truncated || c.DurationMs != 0 ||
		c.ExitCode != nil || c.ReadTarget != nil || c.Web != nil || c.WebTargets != nil || c.Search != nil ||
		c.Goal != nil || c.Todo != nil || c.Skill != nil || c.Job != nil || c.Schedule != nil ||
		c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 || ValidateProtectedPresentReceiptV1(c.Present) != nil ||
		c.PresentTargets != nil && ValidateProtectedPresentTargetsV1(c.Present, c.PresentTargets) != nil {
		return errors.New("protected present capture is invalid")
	}
	return nil
}

func cloneProtectedPresentReceiptV1(value *ProtectedPresentReceiptV1) *ProtectedPresentReceiptV1 {
	if value == nil {
		return nil
	}
	copy_ := &ProtectedPresentReceiptV1{Files: append([]ProtectedPresentedFileV1(nil), value.Files...)}
	for index, file := range copy_.Files {
		if file.Description != nil {
			description := *file.Description
			copy_.Files[index].Description = &description
		}
	}
	return copy_
}
