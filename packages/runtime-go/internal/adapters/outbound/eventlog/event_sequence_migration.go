package eventlog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

const (
	maxLegacyEventSequenceLineBytesV1 = 4 << 20
	maxLegacyEventSequenceRecordsV1   = 1_000_000
)

type legacyEventSequenceMigrationInput struct {
	Root                string
	RestartPreservation *SemanticRestartPreservationV1
}

// migrateLegacyEventSequenceOrderV1 canonicalizes the exact historical
// events.jsonl shape whose sequence set is complete but whose physical lines
// are out of order. It runs only in the isolated semantic-startup stage. The
// signed outer migration journal owns live-root atomicity and crash recovery.
// Primary thread.json and sidecar-only metadata layouts are both host-owned;
// event-only directories remain ineligible. Event sequence numbers and raw
// JSON record bytes are never rewritten.
func migrateLegacyEventSequenceOrderV1(input legacyEventSequenceMigrationInput) error {
	if err := input.RestartPreservation.revalidate(context.Background(), input.Root, ""); err != nil {
		return err
	}
	root := strings.TrimSpace(input.Root)
	if !filepath.IsAbs(root) {
		return errors.New("event-sequence migration root must be absolute")
	}
	threadsDir := filepath.Join(root, "threads")
	entries, err := os.ReadDir(threadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || contracts.SafeRecordID(entry.Name()) != entry.Name() {
			return errors.New("event-sequence migration found an invalid thread directory")
		}
		if input.RestartPreservation.ownsThread(entry.Name()) {
			continue
		}
		migratableOrder, err := isMigratableEventDirectoryV1(filepath.Join(threadsDir, entry.Name()))
		if err != nil {
			return err
		}
		if err := canonicalizeLegacyEventSequenceFileV1(
			filepath.Join(threadsDir, entry.Name(), "events.jsonl"),
			entry.Name(),
			migratableOrder,
		); err != nil {
			return err
		}
	}
	return nil
}

type legacyEventSequenceRecordV1 struct {
	sequence int64
	rawLine  []byte
	record   map[string]any
}

func isMigratableEventDirectoryV1(threadDir string) (bool, error) {
	residue, err := eventOrderTransactionResidueV1(threadDir)
	if err != nil {
		return false, err
	}
	if residue {
		return false, nil
	}
	primary, err := os.Lstat(filepath.Join(threadDir, "thread.json"))
	if err == nil {
		if primary.Mode()&os.ModeSymlink != 0 || !primary.Mode().IsRegular() {
			return false, errors.New("event-sequence migration primary authority is not a regular file")
		}
		return legacyPrimaryThreadAllowsEventOrderMigrationV1(
			filepath.Join(threadDir, "thread.json"),
			filepath.Base(threadDir),
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	metadata, err := os.Lstat(filepath.Join(threadDir, "metadata.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if metadata.Mode()&os.ModeSymlink != 0 || !metadata.Mode().IsRegular() {
		return false, errors.New("event-sequence migration metadata is not a regular file")
	}
	return true, nil
}

func legacyPrimaryThreadAllowsEventOrderMigrationV1(path, threadID string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Size() <= 0 || info.Size() > domainstartup.MaxSemanticManagedFileBytesV1 {
		return false, errors.New("event-sequence migration legacy primary authority is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return false, errors.New("event-sequence migration legacy primary authority is unavailable")
	}
	body, readErr := io.ReadAll(io.LimitReader(file, domainstartup.MaxSemanticManagedFileBytesV1+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || int64(len(body)) != info.Size() {
		return false, errors.New("event-sequence migration legacy primary authority could not be read exactly")
	}
	record, err := jsonstrict.DecodeObject(body, jsonstrict.Options{
		MaxBytes: int(domainstartup.MaxSemanticManagedFileBytesV1),
		MaxDepth: 256,
	})
	for index := range body {
		body[index] = 0
	}
	if err != nil || stringValue(record["id"]) != threadID {
		return false, errors.New("event-sequence migration legacy primary authority is invalid")
	}
	if domainstartup.ContainsCurrentEventOrderAuthorityV1(record) {
		return false, nil
	}
	if !domainstartup.LegacyPrimaryThreadAllowsEventOrderMigrationV1(record, threadID) {
		return false, errors.New("event-sequence migration legacy primary authority shape is invalid")
	}
	return true, nil
}

func eventOrderTransactionResidueV1(threadDir string) (bool, error) {
	directory, err := os.Open(threadDir)
	if err != nil {
		return false, err
	}
	defer directory.Close()
	const maxEntries = 64
	entries, readErr := directory.ReadDir(maxEntries + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, readErr
	}
	if len(entries) > maxEntries {
		return false, errors.New("event-sequence migration thread directory exceeds the classification limit")
	}
	for _, entry := range entries {
		if domainstartup.IsEventOrderTransactionResidueV1(entry.Name()) {
			return true, nil
		}
	}
	return false, nil
}

func canonicalizeLegacyEventSequenceFileV1(path, threadID string, allowHistoricalEventOrder bool) error {
	return processLegacyEventSequenceFileV1(path, threadID, allowHistoricalEventOrder, true)
}

// The read-only form runs the identical full sequence/current-authority
// validation, including ordering eligibility, without applying a permutation.
func processLegacyEventSequenceFileV1(path, threadID string, allowHistoricalEventOrder, apply bool) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > domainstartup.MaxSemanticManagedFileBytesV1 {
		return errors.New("event-sequence migration source is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()

	records := []legacyEventSequenceRecordV1{}
	sequences := domainevent.SequenceSetV1{}
	ordered := true
	currentAuthoritySeen := false
	reader := bufio.NewReaderSize(file, 64*1024)
	for lineNumber := 1; ; lineNumber++ {
		rawLine, readErr := reader.ReadBytes('\n')
		if len(rawLine) > maxLegacyEventSequenceLineBytesV1 {
			return fmt.Errorf("event-sequence migration line %d exceeds the size limit", lineNumber)
		}
		if len(rawLine) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if errors.Is(readErr, io.EOF) {
			return fmt.Errorf("event-sequence migration line %d is missing its final newline", lineNumber)
		}
		body := bytes.TrimSuffix(rawLine, []byte{'\n'})
		body = bytes.TrimSuffix(body, []byte{'\r'})
		if len(bytes.TrimSpace(body)) == 0 {
			return fmt.Errorf("event-sequence migration contains a blank record at line %d", lineNumber)
		}
		record, err := jsonstrict.DecodeObject(body, jsonstrict.Options{
			MaxBytes: maxLegacyEventSequenceLineBytesV1,
			MaxDepth: 256,
		})
		if err != nil {
			return fmt.Errorf("event-sequence migration record %d is invalid: %w", lineNumber, err)
		}
		if stringValue(record["threadId"]) != threadID {
			return fmt.Errorf("event-sequence migration thread identity mismatch at line %d", lineNumber)
		}
		currentAuthoritySeen = currentAuthoritySeen || domainstartup.ContainsCurrentEventOrderAuthorityV1(record)
		sequence, err := legacyEventSequenceV1(record["seq"])
		if err != nil || sequence <= 0 {
			return fmt.Errorf("event-sequence migration record %d has an invalid sequence", lineNumber)
		}
		if len(records) >= maxLegacyEventSequenceRecordsV1 {
			return errors.New("event-sequence migration record limit exceeded")
		}
		if err := sequences.AddPositiveUnique(sequence); err != nil {
			return fmt.Errorf("event-sequence migration sequence %d is invalid: %w", sequence, err)
		}
		if len(records) > 0 && records[len(records)-1].sequence >= sequence {
			ordered = false
		}
		records = append(records, legacyEventSequenceRecordV1{
			sequence: sequence,
			rawLine:  append([]byte(nil), rawLine...),
			record:   record,
		})
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	if len(records) == 0 {
		return nil
	}
	decoded := make([]map[string]any, len(records))
	for index := range records {
		decoded[index] = records[index].record
	}
	if err := validateCurrentTerminalEventGroupsV1(decoded); err != nil {
		return err
	}
	if err := sequences.ValidateContiguous(); err != nil {
		return fmt.Errorf("event-sequence migration sequence set is invalid: %w", err)
	}
	if ordered {
		return nil
	}
	if !allowHistoricalEventOrder || currentAuthoritySeen {
		return errors.New("event-sequence migration refuses to reorder an unowned or current-authority thread layout")
	}
	if !apply {
		return nil
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].sequence < records[right].sequence
	})
	var canonical bytes.Buffer
	for _, record := range records {
		if _, err := canonical.Write(record.rawLine); err != nil {
			return err
		}
	}
	return filestore.WritePrivateFileAtomic(path, canonical.Bytes())
}

// validateCurrentTerminalEventGroupsV1 is the closed durable-event parser for
// startup migration. A terminal publication marker is never a standalone hint:
// it must belong to one complete, contiguous accepted-final or general-terminal
// group. This runs before any legacy ordering decision so malformed current
// authority cannot be reclassified or repaired as historical data.
func validateCurrentTerminalEventGroupsV1(records []map[string]any) error {
	for index := 0; index < len(records); {
		record := records[index]
		switch {
		case domainevent.ContainsAcceptedFinalPublicationAuthority(record):
			if strings.TrimSpace(contracts.StringField(record, "publicationSlot")) != "assistant-final" {
				return errors.New("event-sequence migration found an incomplete accepted-final authority group")
			}
			accepted := 0
			for _, count := range []int{3, 4} {
				if index+count <= len(records) &&
					(domainevent.ValidateAcceptedFinalDeliveryEventsV2(records[index:index+count]) == nil ||
						domainevidence.ValidateHistoricalAcceptedFinalDeliveryEventsV1(records[index:index+count]) == nil) {
					accepted = count
					break
				}
			}
			if accepted == 0 {
				return errors.New("event-sequence migration found a malformed accepted-final authority group")
			}
			index += accepted
		case domainevent.ContainsGeneralTerminalPublicationAuthority(record):
			count, ok := domainevent.GeneralTerminalDeliveryExpectedEventCountV1(record)
			if !ok || index+count > len(records) || domainevent.ValidateGeneralTerminalDeliveryEventsV1(records[index:index+count]) != nil {
				return errors.New("event-sequence migration found a malformed general-terminal authority group")
			}
			index += count
		case terminalDeliveryBatchWrapperMarkerV1(record):
			return errors.New("event-sequence migration found a terminal delivery wrapper in the durable event stream")
		default:
			index++
		}
	}
	return nil
}

func terminalDeliveryBatchWrapperMarkerV1(record map[string]any) bool {
	kind := strings.TrimSpace(contracts.StringField(record, "kind"))
	purpose := strings.TrimSpace(contracts.StringField(record, "purpose"))
	return kind == "accepted_final_batch" || kind == "general_terminal_batch" ||
		purpose == "analytix.accepted-final-delivery-batch/v1" ||
		purpose == "analytix.accepted-final-delivery-batch/v2" ||
		purpose == "analytix.general-terminal-delivery-batch/v1"
}

func legacyEventSequenceV1(value any) (int64, error) {
	switch typed := value.(type) {
	case json.Number:
		return typed.Int64()
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	default:
		return 0, errors.New("event sequence is not an integer")
	}
}
