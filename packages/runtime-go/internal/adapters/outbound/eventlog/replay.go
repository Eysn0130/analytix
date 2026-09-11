package eventlog

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const maxEventJSONLLineBytesV1 = 4 << 20

func validateExactEventThreadID(threadID string) error {
	if threadID == "" || contracts.SafeRecordID(threadID) != threadID {
		return errors.New("event log threadId is not an exact safe record identifier")
	}
	return nil
}

func exactEventSequenceV1(value any) (int, bool) {
	const maxExactFloat64Integer = int64(1<<53 - 1)
	var parsed int64
	switch typed := value.(type) {
	case int:
		parsed = int64(typed)
	case int64:
		parsed = typed
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		parsed = int64(typed)
	case json.Number:
		value, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		parsed = value
	default:
		return 0, false
	}
	if parsed <= 0 || parsed > maxExactFloat64Integer || int64(int(parsed)) != parsed {
		return 0, false
	}
	return int(parsed), true
}

func validateEventThreadDirectory(path string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, false, errors.New("event log thread directory is unsafe")
	}
	return info, true, nil
}

func loadOpenEventLogV1(
	ctx context.Context,
	threadID string,
	path string,
	threadInfo os.FileInfo,
	file *os.File,
	openedInfo os.FileInfo,
	afterSeq int,
) (LoadResult, EventLogFrontierV1, error) {
	if ctx == nil {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log replay context is required")
	}
	if err := ctx.Err(); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}
	if threadInfo == nil || file == nil || openedInfo == nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() < 0 {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log authority is invalid")
	}
	if openedInfo.Size() == 0 {
		pathInfo, pathErr := os.Lstat(path)
		threadReadback, threadErr := os.Lstat(filepath.Dir(path))
		if pathErr != nil || threadErr != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() ||
			!os.SameFile(pathInfo, openedInfo) || threadReadback.Mode()&os.ModeSymlink != 0 || !threadReadback.IsDir() ||
			!os.SameFile(threadInfo, threadReadback) {
			return LoadResult{}, EventLogFrontierV1{}, errors.Join(
				pathErr, threadErr, errors.New("empty event log authority changed during replay"),
			)
		}
		return LoadResult{Events: []map[string]any{}, Diagnostics: []JSONLDiagnostic{}}, EventLogFrontierV1{
			Exists: true, SHA256: hex.EncodeToString(sha256.New().Sum(nil)), Size: 0,
		}, nil
	}
	last := []byte{0}
	if _, err := file.ReadAt(last, openedInfo.Size()-1); err != nil || last[0] != '\n' {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log is not newline terminated")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, err
	}

	hasher := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(file, hasher))
	scanner.Buffer(make([]byte, 64*1024), maxEventJSONLLineBytesV1+1)
	result := LoadResult{Events: []map[string]any{}, Diagnostics: []JSONLDiagnostic{}}
	lastSeq := 0
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if err := ctx.Err(); err != nil {
			return LoadResult{}, EventLogFrontierV1{}, err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			return LoadResult{}, EventLogFrontierV1{}, errors.New("event log contains a blank record")
		}
		event, err := domainjsonstrict.DecodeObject(line, domainjsonstrict.Options{
			MaxBytes: maxEventJSONLLineBytesV1,
		})
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, JSONLDiagnostic{
				Path: filepath.ToSlash(filepath.Join("threads", threadID, "events.jsonl")),
				Line: lineNumber, Error: "invalid_json", Preview: "",
			})
			continue
		}
		seq, ok := normalizeReplaySequence(event)
		if !ok || lastSeq > 0 && seq != lastSeq+1 {
			return LoadResult{}, EventLogFrontierV1{}, errors.New("event log sequence is not positive, contiguous, and physically ordered")
		}
		lastSeq = seq
		if contracts.StringField(event, "threadId") != threadID {
			return LoadResult{}, EventLogFrontierV1{}, errors.New("event log record thread identity mismatch")
		}
		if err := domainevent.ValidatePublicRecord(event); err != nil {
			projected := privateContentMigrationProjection(event, lineNumber, string(line))
			if domainevent.ValidatePublicRecord(projected) != nil || contracts.StringField(projected, "threadId") != threadID {
				return LoadResult{}, EventLogFrontierV1{}, errors.New("event log private-content projection is invalid")
			}
			result.Diagnostics = append(result.Diagnostics, JSONLDiagnostic{
				Path: filepath.ToSlash(filepath.Join("threads", threadID, "events.jsonl")),
				Line: lineNumber, Error: "invalid_public_record", Preview: "",
			})
			if seq > afterSeq {
				result.Events = append(result.Events, projected)
			}
			continue
		}
		if seq > afterSeq {
			result.Events = append(result.Events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return LoadResult{}, EventLogFrontierV1{}, errors.Join(
			err, errors.New("event log record is unreadable or exceeds the size limit"),
		)
	}
	if lineNumber == 0 {
		return LoadResult{}, EventLogFrontierV1{}, errors.New("event log contains no records")
	}
	readSize, seekErr := file.Seek(0, io.SeekCurrent)
	readbackInfo, statErr := file.Stat()
	pathInfo, pathErr := os.Lstat(path)
	threadReadback, threadErr := os.Lstat(filepath.Dir(path))
	if seekErr != nil || statErr != nil || pathErr != nil || threadErr != nil ||
		readSize != openedInfo.Size() || readbackInfo.Size() != openedInfo.Size() ||
		pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) ||
		threadReadback.Mode()&os.ModeSymlink != 0 || !threadReadback.IsDir() || !os.SameFile(threadInfo, threadReadback) {
		return LoadResult{}, EventLogFrontierV1{}, errors.Join(
			seekErr, statErr, pathErr, threadErr, errors.New("event log authority changed during replay"),
		)
	}
	return result, EventLogFrontierV1{
		Exists: true, SHA256: hex.EncodeToString(hasher.Sum(nil)), Size: openedInfo.Size(),
	}, nil
}
