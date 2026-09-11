package filestore

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ReadJSONLLines(reader io.Reader, handle func(lineNumber int, line string) error) error {
	buffer := bufio.NewReader(reader)
	lineNumber := 0
	for {
		line, err := buffer.ReadString('\n')
		if len(line) > 0 {
			lineNumber += 1
			if callbackErr := handle(lineNumber, line); callbackErr != nil {
				return callbackErr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func PreviewLine(line string) string {
	const maxPreview = 120
	if len(line) <= maxPreview {
		return line
	}
	return line[:maxPreview]
}

func ReadJSONLFileRecords[T any](path string, include func(T) bool) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := []T{}
	if err := ReadJSONLLines(file, func(_ int, rawLine string) error {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			return nil
		}
		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil
		}
		if include != nil && !include(record) {
			return nil
		}
		out = append(out, record)
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func AppendJSONLRecord(path string, record any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func WriteJSONLFileAtomic[T any](path string, tmpPattern string, records []T) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if strings.TrimSpace(tmpPattern) == "" {
		tmpPattern = ".jsonl-*.tmp"
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), tmpPattern)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	encoder := json.NewEncoder(tmp)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func RemoveAll(path string) error {
	return os.RemoveAll(path)
}
