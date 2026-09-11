package usageindexfs

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
	"sort"
	"strings"
	"sync"

	usageapp "analytix.local/runtime-go/internal/app/usage"
	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

var ErrRestartPreserved = errors.New("usage authority is preserved after restart")

// RestartPreservationV1 retains original derived rows without interpreting
// them as completed usage. The root prepares it before semantic recovery;
// callers retain their exclusive persistence owner around mutations.
type RestartPreservationV1 struct {
	mu         sync.Mutex
	root       string
	parents    map[string]os.FileInfo
	threads    map[string]bool
	heldLines  []string
	files      map[string]preservedUsageFileV1
	globalMode os.FileMode
}

type preservedUsageFileV1 struct {
	info os.FileInfo
	body []byte
}

type preservedUsageLineV1 struct {
	raw []byte
	id  string
}

func PrepareRestartPreservationV1(ctx context.Context, root string, threadIDs []string) (*RestartPreservationV1, error) {
	if ctx == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("usage preservation root is invalid")
	}
	preserved := &RestartPreservationV1{root: root, parents: map[string]os.FileInfo{}, threads: map[string]bool{}, files: map[string]preservedUsageFileV1{}}
	for _, id := range threadIDs {
		if !domainthread.IsCanonicalRecordID(id) || preserved.threads[id] {
			return nil, errors.New("usage preservation thread inventory is invalid")
		}
		preserved.threads[id] = true
	}
	for _, path := range []string{root, filepath.Join(root, "usage_events"), filepath.Join(root, "usage_events", "threads")} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && path != root {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.Join(errors.New("usage preservation parent is invalid"), err)
		}
		preserved.parents[path] = info
	}
	global, err := preserved.readFile(ctx, "index.jsonl")
	if err != nil {
		return nil, err
	}
	lines, err := parsePreservedUsageLinesV1(global.body, "")
	if err != nil {
		return nil, err
	}
	preserved.heldLines = preserved.held(lines)
	if global.info != nil {
		preserved.globalMode = global.info.Mode()
	}
	for id := range preserved.threads {
		file, err := preserved.readFile(ctx, "threads/"+id+".jsonl")
		if err != nil {
			return nil, err
		}
		if _, err := parsePreservedUsageLinesV1(file.body, id); err != nil {
			return nil, err
		}
		preserved.files[id] = file
	}
	if err := preserved.Revalidate(ctx, root); err != nil {
		return nil, err
	}
	return preserved, nil
}

func (preserved *RestartPreservationV1) OwnsThread(id string) bool {
	return preserved != nil && preserved.threads[id]
}

func (preserved *RestartPreservationV1) ThreadIDsV1() []string {
	if preserved == nil {
		return nil
	}
	ids := make([]string, 0, len(preserved.threads))
	for id := range preserved.threads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (preserved *RestartPreservationV1) Revalidate(ctx context.Context, root string) error {
	if preserved == nil {
		return nil
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	if root != preserved.root {
		return errors.New("usage preservation belongs to another root")
	}
	return preserved.revalidate(ctx)
}

func (preserved *RestartPreservationV1) revalidate(ctx context.Context) error {
	global, err := preserved.readFile(ctx, "index.jsonl")
	if err != nil {
		return err
	}
	lines, err := parsePreservedUsageLinesV1(global.body, "")
	if err != nil || !reflect.DeepEqual(preserved.heldLines, preserved.held(lines)) {
		return errors.Join(errors.New("original held usage rows changed"), err)
	}
	if len(preserved.heldLines) != 0 && (global.info == nil || global.info.Mode() != preserved.globalMode) {
		return errors.New("original held usage global mode changed")
	}
	for id, original := range preserved.files {
		current, err := preserved.readFile(ctx, "threads/"+id+".jsonl")
		if err != nil || !samePreservedUsageFileV1(original, current) {
			return errors.Join(errors.New("original held usage file changed"), err)
		}
	}
	_, err = preserved.threadFiles(ctx)
	return err
}

func (preserved *RestartPreservationV1) threadFiles(ctx context.Context) (map[string]preservedUsageFileV1, error) {
	entries, err := os.ReadDir(filepath.Join(preserved.root, "usage_events", "threads"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]preservedUsageFileV1{}, nil
	}
	if err != nil {
		return nil, err
	}
	files := make(map[string]preservedUsageFileV1, len(entries))
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") || !domainthread.IsCanonicalRecordID(id) {
			return nil, errors.New("usage file inventory contains an invalid target")
		}
		file, err := preserved.readFile(ctx, "threads/"+entry.Name())
		if err != nil || file.info == nil {
			return nil, errors.Join(errors.New("usage file disappeared during inventory"), err)
		}
		if _, err := parsePreservedUsageLinesV1(file.body, id); err != nil {
			return nil, err
		}
		files[id] = file
	}
	return files, nil
}

func samePreservedUsageFileV1(before, after preservedUsageFileV1) bool {
	return bytes.Equal(before.body, after.body) && (before.info == nil) == (after.info == nil) &&
		(before.info == nil || os.SameFile(before.info, after.info) && before.info.Mode() == after.info.Mode() && before.info.Size() == after.info.Size())
}

func (preserved *RestartPreservationV1) readFile(ctx context.Context, relative string) (preservedUsageFileV1, error) {
	empty := preservedUsageFileV1{}
	if ctx == nil {
		return empty, errors.New("usage preservation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	for path, before := range preserved.parents {
		after, err := os.Lstat(path)
		if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
			return empty, errors.Join(errors.New("usage preservation parent changed"), err)
		}
	}
	path := filepath.Join(preserved.root, "usage_events", relative)
	for parent := filepath.Dir(path); parent != preserved.root; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return empty, errors.Join(errors.New("usage file parent is invalid"), err)
		}
	}
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil || !before.Mode().IsRegular() {
		return empty, errors.Join(errors.New("usage preservation requires a regular file"), err)
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, opened) {
		return empty, errors.Join(errors.New("usage file changed while opening"), statErr, file.Close())
	}
	body, readErr := io.ReadAll(file)
	if err := errors.Join(readErr, file.Close()); err != nil {
		return empty, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return empty, errors.Join(errors.New("usage file changed during observation"), err)
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return preservedUsageFileV1{info: after, body: body}, nil
}

func parsePreservedUsageLinesV1(body []byte, threadID string) ([]preservedUsageLineV1, error) {
	reader := bufio.NewReader(bytes.NewReader(body))
	var lines []preservedUsageLineV1
	for {
		raw, err := reader.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if len(raw) == 0 {
			break
		}
		line := preservedUsageLineV1{raw: raw}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) != 0 {
			if err := jsonstrict.Validate(trimmed, jsonstrict.Options{RequireObject: true, MaxBytes: 16 * 1024 * 1024}); err != nil {
				return nil, err
			}
			var record usageapp.IndexRecord
			if err := json.Unmarshal(trimmed, &record); err != nil || !domainthread.IsCanonicalRecordID(record.ThreadID) || threadID != "" && record.ThreadID != threadID {
				return nil, errors.Join(errors.New("usage row identity is invalid"), err)
			}
			line.id = record.ThreadID
		}
		lines = append(lines, line)
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return lines, nil
}

func (preserved *RestartPreservationV1) held(lines []preservedUsageLineV1) []string {
	var held []string
	for _, line := range lines {
		if preserved.threads[line.id] {
			held = append(held, string(line.raw))
		}
	}
	return held
}

// ValidateOriginalSemanticOperationV1 requires the authenticated operation's
// original Before. Applied After bytes never reconstruct a lost original row.
func (preserved *RestartPreservationV1) ValidateOriginalSemanticOperationV1(ctx context.Context, operation domainstartup.SemanticStartupOperationV1, after []byte) error {
	if preserved == nil {
		return errors.New("usage original preservation is unavailable")
	}
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	if err := preserved.revalidate(ctx); err != nil {
		return err
	}
	const root = "durable/usage_events"
	if operation.Path == root || operation.Path == root+"/threads" || strings.HasPrefix(root, operation.Path+"/") {
		if operation.Kind == domainstartup.SemanticOperationCreateDirectory && operation.Before.Type == domainstartup.ManagedEntryTypeAbsent {
			return nil
		}
		return ErrRestartPreserved
	}
	if !strings.HasPrefix(operation.Path, root+"/") {
		return nil
	}
	relative := strings.TrimPrefix(operation.Path, root+"/")
	threadID := ""
	if relative != "index.jsonl" {
		if !strings.HasPrefix(relative, "threads/") || !strings.HasSuffix(relative, ".jsonl") {
			return errors.New("usage semantic target grammar is invalid")
		}
		threadID = strings.TrimSuffix(strings.TrimPrefix(relative, "threads/"), ".jsonl")
		if !domainthread.IsCanonicalRecordID(threadID) {
			return errors.New("usage semantic thread identity is invalid")
		}
		if preserved.threads[threadID] {
			return ErrRestartPreserved
		}
	}
	current, err := preserved.readFile(ctx, relative)
	if err != nil {
		return err
	}
	if relative == "index.jsonl" {
		if len(preserved.heldLines) != 0 && (operation.Before.Mode != uint32(preserved.globalMode) || operation.After.Mode != uint32(preserved.globalMode)) {
			return ErrRestartPreserved
		}
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeFile:
			if current.info == nil || int64(len(current.body)) != operation.Before.Size || domainsecurity.SHA256Hex(current.body) != operation.Before.SHA256 {
				return errors.New("usage semantic original Before bytes are unavailable")
			}
		case domainstartup.ManagedEntryTypeAbsent:
			if len(preserved.heldLines) != 0 {
				return errors.New("usage absent Before cannot contain held After rows")
			}
		default:
			return errors.New("usage semantic original state is invalid")
		}
	}
	switch operation.Kind {
	case domainstartup.SemanticOperationSetMode:
		return nil
	case domainstartup.SemanticOperationInstallFile:
		if int64(len(after)) != operation.After.Size || domainsecurity.SHA256Hex(after) != operation.After.SHA256 {
			return errors.New("usage semantic After bytes lost integrity")
		}
	case domainstartup.SemanticOperationRemoveFile:
		after = nil
	default:
		return errors.New("usage semantic operation is invalid")
	}
	lines, err := parsePreservedUsageLinesV1(after, threadID)
	if err != nil {
		return err
	}
	if relative == "index.jsonl" && !reflect.DeepEqual(preserved.heldLines, preserved.held(lines)) {
		return ErrRestartPreserved
	}
	return preserved.revalidate(ctx)
}

// writeIndependent retains raw held rows, their relative order and any final
// unterminated row. It never creates, removes or rewrites a held thread file.
func (preserved *RestartPreservationV1) writeIndependent(ctx context.Context, records []usageapp.IndexRecord, appendOnly bool) error {
	preserved.mu.Lock()
	defer preserved.mu.Unlock()
	if err := preserved.revalidate(ctx); err != nil {
		return err
	}
	global, err := preserved.readFile(ctx, "index.jsonl")
	if err != nil {
		return err
	}
	files, err := preserved.threadFiles(ctx)
	if err != nil {
		return err
	}
	var encoded bytes.Buffer
	byThread := map[string][]byte{}
	for _, record := range records {
		if !domainthread.IsCanonicalRecordID(record.ThreadID) || preserved.threads[record.ThreadID] {
			return ErrRestartPreserved
		}
		body, err := json.Marshal(record)
		if err != nil {
			return err
		}
		body = append(body, '\n')
		encoded.Write(body)
		byThread[record.ThreadID] = append(byThread[record.ThreadID], body...)
	}
	lines, err := parsePreservedUsageLinesV1(global.body, "")
	if err != nil {
		return err
	}
	var output bytes.Buffer
	if appendOnly {
		position := len(global.body)
		if len(lines) != 0 {
			last := lines[len(lines)-1]
			if preserved.threads[last.id] && !bytes.HasSuffix(last.raw, []byte("\n")) {
				position -= len(last.raw)
			}
		}
		output.Write(global.body[:position])
		if position != 0 && global.body[position-1] != '\n' {
			output.WriteByte('\n')
		}
		output.Write(encoded.Bytes())
		output.Write(global.body[position:])
	} else {
		output.Write(encoded.Bytes())
		for _, line := range lines {
			if line.id == "" || preserved.threads[line.id] {
				output.Write(line.raw)
			}
		}
	}
	current, err := preserved.readFile(ctx, "index.jsonl")
	if err != nil || !samePreservedUsageFileV1(global, current) {
		return errors.Join(errors.New("usage global source changed before write"), err)
	}
	if err := preserved.revalidate(ctx); err != nil {
		return err
	}
	if !bytes.Equal(global.body, output.Bytes()) {
		if err := writePreservedUsageAtomicV1(ctx, filepath.Join(preserved.root, "usage_events", "index.jsonl"), output.Bytes(), global.info); err != nil {
			return err
		}
	}
	if !appendOnly {
		for id, before := range files {
			if preserved.threads[id] || byThread[id] != nil {
				continue
			}
			if err := preserved.revalidate(ctx); err != nil {
				return err
			}
			current, err := preserved.readFile(ctx, "threads/"+id+".jsonl")
			if err != nil || !samePreservedUsageFileV1(before, current) {
				return errors.Join(errors.New("independent usage removal source changed"), err)
			}
			if err := os.Remove(filepath.Join(preserved.root, "usage_events", "threads", id+".jsonl")); err != nil {
				return err
			}
		}
	}
	for id, body := range byThread {
		if err := preserved.revalidate(ctx); err != nil {
			return err
		}
		before := files[id]
		current, err := preserved.readFile(ctx, "threads/"+id+".jsonl")
		if err != nil || !samePreservedUsageFileV1(before, current) {
			return errors.Join(errors.New("independent usage source changed before write"), err)
		}
		if appendOnly {
			prefix := append([]byte(nil), before.body...)
			if len(prefix) != 0 && prefix[len(prefix)-1] != '\n' {
				prefix = append(prefix, '\n')
			}
			body = append(prefix, body...)
		}
		if !bytes.Equal(before.body, body) {
			if err := writePreservedUsageAtomicV1(ctx, filepath.Join(preserved.root, "usage_events", "threads", id+".jsonl"), body, before.info); err != nil {
				return err
			}
		}
	}
	return preserved.revalidate(ctx)
}

func writePreservedUsageAtomicV1(ctx context.Context, path string, body []byte, original os.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".usage-preservation-*.tmp")
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
