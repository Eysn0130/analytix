package threadsummaryindexfs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// PreservedRecordsV1 is a denial-only guard for a shared summary index. It
// preserves every original row for held IDs, including historical/deleted
// rows and their exact framing. Callers own the exclusive semantic stage or
// live store lock; this does not replace the root physical/Core preflight.
type PreservedRecordsV1 struct {
	mu        sync.Mutex
	path      string
	parent    os.FileInfo
	threads   map[string]bool
	heldLines []string
}

var ErrRestartPreserved = errors.New("thread summary is preserved after restart")

// AppendIndependent keeps physical last-writer ordering for independent IDs.
// If the final held row has no newline, insertion occurs immediately before
// that row, after all earlier independent records, leaving its framing intact.
func (preserved *PreservedRecordsV1) AppendIndependent(ctx context.Context, record map[string]any) error {
	if preserved == nil {
		return errors.New("summary preservation is unavailable")
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	id, err := preservedSummaryThreadIDV1(record)
	if err != nil {
		return err
	}
	if preserved.threads[id] {
		return ErrRestartPreserved
	}
	snapshot, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	if err := preserved.validateHeld(snapshot.lines); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	position := len(snapshot.body)
	if len(snapshot.lines) > 0 {
		last := snapshot.lines[len(snapshot.lines)-1]
		if preserved.threads[last.threadID] && !bytes.HasSuffix(last.raw, []byte("\n")) {
			position -= len(last.raw)
		}
	}
	var output bytes.Buffer
	output.Write(snapshot.body[:position])
	if position > 0 && snapshot.body[position-1] != '\n' {
		output.WriteByte('\n')
	}
	output.Write(encoded)
	output.Write(snapshot.body[position:])
	return preserved.writeIfCurrent(ctx, snapshot, output.Bytes())
}

// ReplaceIndependent rebuilds independent records while retaining every held
// historical row in its original relative order. New records precede retained
// rows so a held final line never needs a new terminator.
func (preserved *PreservedRecordsV1) ReplaceIndependent(ctx context.Context, records []map[string]any) error {
	if preserved == nil {
		return errors.New("summary preservation is unavailable")
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	var output bytes.Buffer
	for _, record := range records {
		id, err := preservedSummaryThreadIDV1(record)
		if err != nil {
			return err
		}
		if preserved.threads[id] {
			return ErrRestartPreserved
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		output.Write(encoded)
		output.WriteByte('\n')
	}
	snapshot, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	if err := preserved.validateHeld(snapshot.lines); err != nil {
		return err
	}
	for _, line := range snapshot.lines {
		if line.record == nil || preserved.threads[line.threadID] {
			output.Write(line.raw)
		}
	}
	return preserved.writeIfCurrent(ctx, snapshot, output.Bytes())
}

type preservedSummaryLineV1 struct {
	raw      []byte
	record   map[string]any
	threadID string
	line     int
}

type preservedSummarySnapshotV1 struct {
	body  []byte
	info  os.FileInfo
	lines []preservedSummaryLineV1
}

// PreparePreservedRecordsV1 freezes only the supplied thread identities. It
// does not classify any thread or establish authority from index contents.
func PreparePreservedRecordsV1(ctx context.Context, path string, threadIDs []string) (*PreservedRecordsV1, error) {
	if ctx == nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("summary preservation input is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	threads := map[string]bool{}
	for _, id := range threadIDs {
		if !domainthread.IsCanonicalRecordID(id) || threads[id] {
			return nil, errors.New("summary preservation scope is invalid")
		}
		threads[id] = true
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("summary preservation parent is invalid")
	}
	preserved := &PreservedRecordsV1{path: path, parent: parent, threads: threads}
	snapshot, err := preserved.read(ctx)
	if err != nil {
		return nil, err
	}
	preserved.heldLines = preserved.held(snapshot.lines)
	return preserved, nil
}

func (preserved *PreservedRecordsV1) OwnsThread(id string) bool {
	return preserved != nil && preserved.threads[id]
}

func (preserved *PreservedRecordsV1) Revalidate(ctx context.Context) error {
	if preserved == nil {
		return errors.New("summary preservation is unavailable")
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	snapshot, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	return preserved.validateHeld(snapshot.lines)
}

// ValidateReplacementV1 checks a complete proposed index without writing it.
// Independent rows may change; held rows retain their exact original framing
// and relative order. The caller still owns file identity and transaction proof.
func (preserved *PreservedRecordsV1) ValidateReplacementV1(ctx context.Context, body []byte) error {
	if preserved == nil {
		return errors.New("summary preservation is unavailable")
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	current, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	if err := preserved.validateHeld(current.lines); err != nil {
		return err
	}
	lines, err := parsePreservedSummaryLinesV1(body)
	if err != nil {
		return err
	}
	return preserved.validateHeld(lines)
}

// ValidateOriginalSemanticOperationV1 checks the complete authenticated index
// operation against a provable Before. The root owns journal authentication,
// staged After integrity and the actual cursor. A changed file cannot supply
// lost Before rows; an absent Before proves zero held rows independently of
// whatever the current physical After contains.
func (preserved *PreservedRecordsV1) ValidateOriginalSemanticOperationV1(ctx context.Context, operation domainstartup.SemanticStartupOperationV1, after []byte) (resultErr error) {
	if preserved == nil {
		return errors.New("summary preservation is unavailable")
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	current, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	defer func() {
		next, err := preserved.read(ctx)
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			return
		}
		if !bytes.Equal(current.body, next.body) || (current.info == nil) != (next.info == nil) || current.info != nil && (!os.SameFile(current.info, next.info) || current.info.Mode() != next.info.Mode()) {
			resultErr = errors.Join(resultErr, errors.New("summary original observation changed"))
		}
	}()
	var originalHeld []string
	switch operation.Before.Type {
	case domainstartup.ManagedEntryTypeFile:
		if current.info == nil || int64(len(current.body)) != operation.Before.Size || domainsecurity.SHA256Hex(current.body) != operation.Before.SHA256 {
			return errors.New("summary semantic original Before bytes are unavailable")
		}
		originalHeld = preserved.held(current.lines)
	case domainstartup.ManagedEntryTypeAbsent:
		if len(preserved.held(current.lines)) != 0 {
			return errors.New("summary absent Before cannot contain held After rows")
		}
	default:
		return errors.New("summary semantic original state is invalid")
	}
	if !reflect.DeepEqual(originalHeld, preserved.heldLines) {
		return errors.New("summary original held rows changed")
	}
	switch operation.Kind {
	case domainstartup.SemanticOperationSetMode:
		return nil
	case domainstartup.SemanticOperationInstallFile:
		if int64(len(after)) != operation.After.Size || domainsecurity.SHA256Hex(after) != operation.After.SHA256 {
			return errors.New("summary semantic After bytes lost integrity")
		}
	case domainstartup.SemanticOperationRemoveFile:
		after = nil
	default:
		return errors.New("summary semantic operation is invalid")
	}
	lines, err := parsePreservedSummaryLinesV1(after)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(originalHeld, preserved.held(lines)) {
		return errors.New("summary semantic After changed original held rows")
	}
	return nil
}

// Transform changes independent rows only. Every row is parsed and assigned
// before the first callback. Unchanged rows retain their bytes; changed rows
// retain their original newline convention and location. A callback cannot
// insert, erase, or relabel a held row, or overwrite a changed source file.
func (preserved *PreservedRecordsV1) Transform(ctx context.Context, transform func(map[string]any, int, string) (map[string]any, bool, error)) error {
	if preserved == nil || transform == nil {
		return errors.New("summary preservation transform is unavailable")
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	snapshot, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	if err := preserved.validateHeld(snapshot.lines); err != nil {
		return err
	}
	var output bytes.Buffer
	for _, line := range snapshot.lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		if line.record == nil || preserved.threads[line.threadID] {
			output.Write(line.raw)
			continue
		}
		projected, changed, err := transform(line.record, line.line, strings.TrimSpace(string(line.raw)))
		if err != nil {
			return err
		}
		if !changed {
			output.Write(line.raw)
			continue
		}
		id, err := preservedSummaryThreadIDV1(projected)
		if err != nil || id != line.threadID {
			return errors.New("summary transform changed row identity")
		}
		body, err := json.Marshal(projected)
		if err != nil {
			return err
		}
		output.Write(body)
		if bytes.HasSuffix(line.raw, []byte("\r\n")) {
			output.WriteString("\r\n")
		} else if bytes.HasSuffix(line.raw, []byte("\n")) {
			output.WriteByte('\n')
		}
	}
	return preserved.writeIfCurrent(ctx, snapshot, output.Bytes())
}

func (preserved *PreservedRecordsV1) held(lines []preservedSummaryLineV1) []string {
	var held []string
	for _, line := range lines {
		if preserved.threads[line.threadID] {
			held = append(held, string(line.raw))
		}
	}
	return held
}

func (preserved *PreservedRecordsV1) validateHeld(lines []preservedSummaryLineV1) error {
	if !reflect.DeepEqual(preserved.heldLines, preserved.held(lines)) {
		return errors.New("preserved summary rows changed")
	}
	return nil
}

func (preserved *PreservedRecordsV1) read(ctx context.Context) (preservedSummarySnapshotV1, error) {
	if ctx == nil {
		return preservedSummarySnapshotV1{}, errors.New("summary preservation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	parent, err := os.Lstat(filepath.Dir(preserved.path))
	if err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	if !os.SameFile(parent, preserved.parent) || parent.Mode() != preserved.parent.Mode() {
		return preservedSummarySnapshotV1{}, errors.New("summary preservation parent changed")
	}
	before, err := os.Lstat(preserved.path)
	if errors.Is(err, os.ErrNotExist) {
		return preservedSummarySnapshotV1{}, nil
	}
	if err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	if !before.Mode().IsRegular() {
		return preservedSummarySnapshotV1{}, errors.New("summary preservation requires an original regular file")
	}
	file, err := os.Open(preserved.path)
	if err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, opened) {
		return preservedSummarySnapshotV1{}, errors.Join(errors.New("summary file changed while opening"), statErr, file.Close())
	}
	body, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	after, err := os.Lstat(preserved.path)
	if err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	if !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return preservedSummarySnapshotV1{}, errors.New("summary file changed during observation")
	}
	lines, err := parsePreservedSummaryLinesV1(body)
	if err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return preservedSummarySnapshotV1{}, err
	}
	return preservedSummarySnapshotV1{body: body, info: after, lines: lines}, nil
}

func parsePreservedSummaryLinesV1(body []byte) ([]preservedSummaryLineV1, error) {
	reader := bufio.NewReader(bytes.NewReader(body))
	var lines []preservedSummaryLineV1
	for number := 1; ; number++ {
		raw, err := reader.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if len(raw) == 0 {
			break
		}
		line := preservedSummaryLineV1{raw: raw, line: number}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) > 0 {
			record, decodeErr := jsonstrict.DecodeObject(trimmed, jsonstrict.Options{RequireObject: true, MaxBytes: 16 * 1024 * 1024})
			if decodeErr != nil {
				return nil, errors.New("summary row is not a strict object")
			}
			id, identityErr := preservedSummaryThreadIDV1(record)
			if identityErr != nil {
				return nil, identityErr
			}
			line.record, line.threadID = record, id
		}
		lines = append(lines, line)
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return lines, nil
}

func preservedSummaryThreadIDV1(record map[string]any) (string, error) {
	if record == nil {
		return "", errors.New("summary row identity is unavailable")
	}
	outer, outerPresent := record["threadId"]
	summary, summaryPresent := record["summary"].(map[string]any)
	if !summaryPresent {
		return "", errors.New("summary row body is unavailable")
	}
	inner, innerPresent := summary["id"]
	outerID, outerString := outer.(string)
	innerID, innerString := inner.(string)
	if (outerPresent && (!outerString || !domainthread.IsCanonicalRecordID(outerID))) || (innerPresent && (!innerString || !domainthread.IsCanonicalRecordID(innerID))) {
		return "", errors.New("summary row identity is invalid")
	}
	if outerPresent && innerPresent && outerID != innerID {
		return "", errors.New("summary row identities conflict")
	}
	if outerPresent {
		return outerID, nil
	}
	if innerPresent {
		return innerID, nil
	}
	return "", errors.New("summary row identity is missing")
}

func (preserved *PreservedRecordsV1) writeIfCurrent(ctx context.Context, before preservedSummarySnapshotV1, body []byte) error {
	lines, err := parsePreservedSummaryLinesV1(body)
	if err != nil {
		return err
	}
	if err := preserved.validateHeld(lines); err != nil {
		return err
	}
	current, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	if !bytes.Equal(before.body, current.body) || (before.info == nil) != (current.info == nil) || (before.info != nil && !os.SameFile(before.info, current.info)) {
		return errors.New("summary transform source changed before write")
	}
	if bytes.Equal(body, current.body) {
		return nil
	}
	if err := writePreservedSummaryAtomicV1(ctx, preserved.path, body, current.info); err != nil {
		return err
	}
	after, err := preserved.read(ctx)
	if err != nil {
		return err
	}
	if !bytes.Equal(body, after.body) {
		return errors.New("summary transform readback changed")
	}
	return preserved.validateHeld(after.lines)
}

// The parent already exists under the caller's exclusive persistence owner.
// Unlike the generic private writer, this never chmods or creates the parent.
func writePreservedSummaryAtomicV1(ctx context.Context, path string, body []byte, original os.FileInfo) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".summary-preservation-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	mode := os.FileMode(0o600)
	if original != nil {
		mode = original.Mode().Perm()
	}
	if err := file.Chmod(mode); err != nil {
		return errors.Join(err, file.Close())
	}
	if _, err := file.Write(body); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
