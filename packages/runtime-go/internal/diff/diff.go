// Package diff computes bounded, dependency-free unified diffs for runtime file tools.
// Portions adapted from DeepSeek-Reasonix and modified by Analytix
// contributors. See THIRD_PARTY_NOTICES.md and the upstream provenance ledger.
package diff

import "strings"

// Kind describes the file-level operation represented by a change.
type Kind string

const (
	Create Kind = "create"
	Modify Kind = "modify"
	Delete Kind = "delete"
)

// Change is the provider-visible summary of a file mutation.
type Change struct {
	Path      string `json:"path"`
	Kind      Kind   `json:"kind"`
	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Diff      string `json:"diff,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

const (
	defaultContext = 3
	maxDiffEdits   = 2000
)

// Build computes a bounded line-level diff from oldText to newText. Large
// rewrites keep line tallies but skip the noisy hunk body.
func Build(path, oldText, newText string, kind Kind) Change {
	change := Change{Path: path, Kind: kind}
	if isBinary(oldText) || isBinary(newText) {
		change.Binary = true
		return change
	}
	if oldText == newText {
		return change
	}

	oldLines, oldEOL := splitLines(oldText)
	newLines, newEOL := splitLines(newText)
	ops, ok := myers(oldLines, newLines)
	if !ok {
		change.Added, change.Removed = approxTally(oldLines, newLines)
		change.Truncated = true
		change.Diff = "(diff omitted: change too large to render - +" + itoa(change.Added) + " / -" + itoa(change.Removed) + " lines)"
		return change
	}

	for _, op := range ops {
		switch op.typ {
		case opInsert:
			change.Added++
		case opDelete:
			change.Removed++
		}
	}
	change.Diff = unified(path, ops, oldEOL, newEOL, defaultContext)
	return change
}

func approxTally(oldLines, newLines []string) (added, removed int) {
	counts := make(map[string]int, len(oldLines))
	for _, line := range oldLines {
		counts[line]++
	}
	for _, line := range newLines {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		added++
	}
	for _, count := range counts {
		removed += count
	}
	return added, removed
}

func isBinary(s string) bool {
	return strings.IndexByte(s, 0) >= 0
}

func splitLines(s string) (lines []string, endsWithNewline bool) {
	if s == "" {
		return nil, true
	}
	endsWithNewline = strings.HasSuffix(s, "\n")
	if endsWithNewline {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n"), endsWithNewline
}

type opType int

const (
	opEqual opType = iota
	opDelete
	opInsert
)

type op struct {
	typ  opType
	line string
}

func myersMaxD(n, m int) int {
	if n >= maxDiffEdits {
		return maxDiffEdits
	}
	maxD := n
	for i := 0; i < m && maxD < maxDiffEdits; i++ {
		maxD++
	}
	return maxD
}

func myersVectorLen(maxD int) int {
	width := 1
	for i := 0; i < maxD; i++ {
		width += 2
	}
	return width
}

func myers(a, b []string) ([]op, bool) {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil, true
	}
	maxD := myersMaxD(n, m)
	offset := maxD
	v := make([]int, myersVectorLen(maxD))
	var trace [][]int

	for d := 0; d <= maxD; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, offset), true
			}
		}
	}
	return nil, false
}

func backtrack(trace [][]int, a, b []string, offset int) []op {
	x, y := len(a), len(b)
	var ops []op
	for d := len(trace) - 1; d > 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			ops = append(ops, op{typ: opEqual, line: a[x-1]})
			x--
			y--
		}
		if x == prevX {
			ops = append(ops, op{typ: opInsert, line: b[y-1]})
		} else {
			ops = append(ops, op{typ: opDelete, line: a[x-1]})
		}
		x, y = prevX, prevY
	}
	for x > 0 && y > 0 {
		ops = append(ops, op{typ: opEqual, line: a[x-1]})
		x--
		y--
	}
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

type lineRef struct {
	op           op
	oldNo, newNo int
}

func unified(path string, ops []op, oldEOL, newEOL bool, context int) string {
	refs := number(ops)
	hunks := group(refs, context)
	if len(hunks) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("--- a/" + path + "\n")
	builder.WriteString("+++ b/" + path + "\n")

	lastOldNo, lastNewNo := lastLineNumbers(refs)
	for _, hunk := range hunks {
		writeHunkHeader(&builder, refs, hunk)
		for i := hunk.start; i < hunk.end; i++ {
			ref := refs[i]
			switch ref.op.typ {
			case opEqual:
				builder.WriteString(" " + ref.op.line + "\n")
			case opDelete:
				builder.WriteString("-" + ref.op.line + "\n")
				if !oldEOL && ref.oldNo == lastOldNo {
					builder.WriteString("\\ No newline at end of file\n")
				}
			case opInsert:
				builder.WriteString("+" + ref.op.line + "\n")
				if !newEOL && ref.newNo == lastNewNo {
					builder.WriteString("\\ No newline at end of file\n")
				}
			}
		}
	}
	return builder.String()
}

func number(ops []op) []lineRef {
	refs := make([]lineRef, len(ops))
	oldNo, newNo := 0, 0
	for i, item := range ops {
		ref := lineRef{op: item}
		switch item.typ {
		case opEqual:
			oldNo++
			newNo++
			ref.oldNo, ref.newNo = oldNo, newNo
		case opDelete:
			oldNo++
			ref.oldNo = oldNo
		case opInsert:
			newNo++
			ref.newNo = newNo
		}
		refs[i] = ref
	}
	return refs
}

func lastLineNumbers(refs []lineRef) (lastOld, lastNew int) {
	for _, ref := range refs {
		if ref.oldNo > lastOld {
			lastOld = ref.oldNo
		}
		if ref.newNo > lastNew {
			lastNew = ref.newNo
		}
	}
	return lastOld, lastNew
}

type hunk struct {
	start int
	end   int
}

func group(refs []lineRef, context int) []hunk {
	var changes []int
	for i, ref := range refs {
		if ref.op.typ != opEqual {
			changes = append(changes, i)
		}
	}
	if len(changes) == 0 {
		return nil
	}

	start := intMax(0, changes[0]-context)
	end := intMin(len(refs), changes[0]+context+1)
	hunks := make([]hunk, 0, len(changes))
	for _, changedIndex := range changes[1:] {
		if changedIndex-context <= end {
			end = intMin(len(refs), changedIndex+context+1)
			continue
		}
		hunks = append(hunks, hunk{start: start, end: end})
		start = intMax(0, changedIndex-context)
		end = intMin(len(refs), changedIndex+context+1)
	}
	hunks = append(hunks, hunk{start: start, end: end})
	return hunks
}

func writeHunkHeader(builder *strings.Builder, refs []lineRef, hunk hunk) {
	oldStart, oldCount, newStart, newCount := 0, 0, 0, 0
	for i := hunk.start; i < hunk.end; i++ {
		ref := refs[i]
		if ref.oldNo != 0 {
			if oldStart == 0 {
				oldStart = ref.oldNo
			}
			oldCount++
		}
		if ref.newNo != 0 {
			if newStart == 0 {
				newStart = ref.newNo
			}
			newCount++
		}
	}
	if oldCount == 0 {
		oldStart = 0
	}
	if newCount == 0 {
		newStart = 0
	}
	builder.WriteString("@@ -")
	builder.WriteString(rangeSpec(oldStart, oldCount))
	builder.WriteString(" +")
	builder.WriteString(rangeSpec(newStart, newCount))
	builder.WriteString(" @@\n")
}

func rangeSpec(start, count int) string {
	if count == 1 {
		return itoa(start)
	}
	return itoa(start) + "," + itoa(count)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
