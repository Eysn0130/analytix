package filetools

import runtimediff "analytix.local/runtime-go/internal/diff"

type DiffKind = runtimediff.Kind

const (
	DiffCreate = runtimediff.Create
	DiffModify = runtimediff.Modify
)

func MergeDiffOutput(output map[string]any, relativePath, before, after string, kind DiffKind) {
	change := runtimediff.Build(relativePath, before, after, kind)
	output["diff_kind"] = string(change.Kind)
	output["added"] = float64(change.Added)
	output["removed"] = float64(change.Removed)
	if change.Diff != "" {
		output["diff"] = change.Diff
	}
	if change.Binary {
		output["diff_binary"] = true
		output["diff_omitted_reason"] = "binary"
	}
	if change.Truncated {
		output["diff_truncated"] = true
		output["diff_omitted_reason"] = "large_change"
	}
}
