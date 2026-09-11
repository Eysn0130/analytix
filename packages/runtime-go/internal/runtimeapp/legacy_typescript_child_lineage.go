package runtimeapp

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"analytix.local/runtime-go/internal/contracts"
	jsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	"analytix.local/runtime-go/internal/jobs"
)

type legacyTypeScriptThreadReaderV1 interface {
	GetThread(string) (map[string]any, error)
}

// legacyTypeScriptRawThreadReaderV1 exists only for the pre-migration lineage
// freeze. Normal thread recovery uses the closed public sidecar allowlist and
// must never rehydrate retired provider tool-call identities. The lineage
// freeze instead needs the exact raw parent tuple long enough to issue a
// source-hash-bound witness before semantic migration removes it.
type legacyTypeScriptRawThreadReaderV1 struct {
	root string
}

func newLegacyTypeScriptRawThreadReaderV1(root string) legacyTypeScriptRawThreadReaderV1 {
	return legacyTypeScriptRawThreadReaderV1{root: filepath.Clean(strings.TrimSpace(root))}
}

func (reader legacyTypeScriptRawThreadReaderV1) GetThread(threadID string) (map[string]any, error) {
	threadID = strings.TrimSpace(threadID)
	if reader.root == "" || contracts.SafeRecordID(threadID) != threadID {
		return nil, errors.New("legacy lineage thread identity is invalid")
	}
	threadDir := filepath.Join(reader.root, "threads", threadID)
	if err := rejectLegacyLineageSymlinkV1(threadDir, true); err != nil {
		return nil, err
	}
	threadPath := filepath.Join(threadDir, "thread.json")
	thread, err := readLegacyLineageObjectV1(threadPath, 64*1024*1024)
	if errors.Is(err, os.ErrNotExist) {
		thread, err = readLegacyLineageMetadataV1(filepath.Join(threadDir, "metadata.jsonl"), threadID)
	}
	if err != nil {
		return nil, err
	}
	if thread == nil || runtimeappStringFieldV1(thread, "id") != threadID {
		return nil, errors.New("legacy lineage thread snapshot is invalid")
	}
	return hydrateLegacyLineageToolCallsV1(
		threadID, thread, filepath.Join(threadDir, "messages.jsonl"),
	)
}

func readLegacyLineageObjectV1(path string, maxBytes int) (map[string]any, error) {
	if err := rejectLegacyLineageSymlinkV1(path, false); err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return jsonstrict.DecodeObject(body, jsonstrict.Options{
		MaxBytes: maxBytes, MaxDepth: 256, MaxTokens: 2_000_000, MaxStringBytes: 16 * 1024 * 1024,
	})
}

func readLegacyLineageMetadataV1(path, threadID string) (map[string]any, error) {
	records, err := readLegacyLineageJSONLV1(path)
	if err != nil {
		return nil, err
	}
	var latest map[string]any
	for _, record := range records {
		if runtimeappStringFieldV1(record, "kind") != "thread_metadata" {
			continue
		}
		thread, _ := record["thread"].(map[string]any)
		if thread == nil || runtimeappStringFieldV1(thread, "id") != threadID {
			return nil, errors.New("legacy lineage metadata identity is invalid")
		}
		latest = thread
	}
	if latest == nil {
		return nil, errors.New("legacy lineage metadata snapshot is unavailable")
	}
	return latest, nil
}

func hydrateLegacyLineageToolCallsV1(threadID string, thread map[string]any, path string) (map[string]any, error) {
	turns := runtimeappListAnyV1(thread["turns"])
	emptyTurns := map[string]bool{}
	knownTurns := map[string]bool{}
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		turnID := runtimeappStringFieldV1(turn, "id")
		if turnID == "" || knownTurns[turnID] {
			return nil, errors.New("legacy lineage turn inventory is invalid")
		}
		knownTurns[turnID] = true
		emptyTurns[turnID] = len(runtimeappListAnyV1(turn["items"])) == 0
	}
	records, err := readLegacyLineageJSONLV1(path)
	if errors.Is(err, os.ErrNotExist) {
		return thread, nil
	}
	if err != nil {
		return nil, err
	}
	latestByID := map[string]map[string]any{}
	order := []string{}
	for _, record := range records {
		if runtimeappStringFieldV1(record, "kind") != "tool_call" {
			continue
		}
		itemID := runtimeappStringFieldV1(record, "id")
		turnID := runtimeappStringFieldV1(record, "turnId")
		itemThreadID := runtimeappStringFieldV1(record, "threadId")
		if itemID == "" || contracts.SafeRecordID(itemID) != itemID || !knownTurns[turnID] ||
			itemThreadID != "" && itemThreadID != threadID {
			return nil, errors.New("legacy lineage messages inventory is invalid")
		}
		if _, seen := latestByID[itemID]; !seen {
			order = append(order, itemID)
		}
		latestByID[itemID] = record
	}
	itemsByTurn := map[string][]any{}
	for _, itemID := range order {
		item := latestByID[itemID]
		turnID := runtimeappStringFieldV1(item, "turnId")
		if emptyTurns[turnID] {
			itemsByTurn[turnID] = append(itemsByTurn[turnID], item)
		}
	}
	hydrated := contracts.CloneMap(thread)
	hydratedTurns := runtimeappListAnyV1(hydrated["turns"])
	for index, rawTurn := range hydratedTurns {
		turn, _ := rawTurn.(map[string]any)
		turnID := runtimeappStringFieldV1(turn, "id")
		if emptyTurns[turnID] && len(itemsByTurn[turnID]) > 0 {
			turn["items"] = itemsByTurn[turnID]
			hydratedTurns[index] = turn
		}
	}
	hydrated["turns"] = hydratedTurns
	return hydrated, nil
}

func readLegacyLineageJSONLV1(path string) ([]map[string]any, error) {
	if err := rejectLegacyLineageSymlinkV1(path, false); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	records := []map[string]any{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			return nil, fmt.Errorf("legacy lineage JSONL record %d is empty", line)
		}
		record, err := jsonstrict.DecodeObject([]byte(raw), jsonstrict.Options{
			MaxBytes: 16 * 1024 * 1024, MaxDepth: 256, MaxTokens: 1_000_000, MaxStringBytes: 8 * 1024 * 1024,
		})
		if err != nil {
			return nil, fmt.Errorf("legacy lineage JSONL record %d is invalid: %w", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func rejectLegacyLineageSymlinkV1(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || directory && !info.IsDir() || !directory && !info.Mode().IsRegular() {
		return errors.New("legacy lineage source type is invalid")
	}
	return nil
}

func newFrozenLegacyTypeScriptChildLineageWitnessV1(
	childRunRoot string,
	threads legacyTypeScriptThreadReaderV1,
) (*jobs.FrozenLegacyTypeScriptLineageWitnessV1, error) {
	return jobs.FreezeLegacyTypeScriptLineageWitnessV1(
		childRunRoot,
		newLegacyTypeScriptChildLineageVerifierV1(threads),
	)
}

func newLegacyTypeScriptChildLineageVerifierV1(
	threads legacyTypeScriptThreadReaderV1,
) jobs.LegacyTypeScriptLineageVerifierV1 {
	return jobs.LegacyTypeScriptLineageVerifyFuncV1(func(lineage jobs.LegacyTypeScriptLineageV1) error {
		if threads == nil {
			return errors.New("staged durable thread reader is unavailable")
		}
		thread, err := threads.GetThread(lineage.ParentThreadID)
		if err != nil {
			return errors.New("staged durable thread cannot be read")
		}
		if thread == nil {
			return errors.New("legacy parent thread is unavailable")
		}
		if runtimeappStringFieldV1(thread, "id") != lineage.ParentThreadID {
			return errors.New("legacy parent thread is unavailable")
		}
		matchingTurns := 0
		matchingCalls := 0
		for _, rawTurn := range runtimeappListAnyV1(thread["turns"]) {
			turn, ok := rawTurn.(map[string]any)
			if !ok {
				continue
			}
			turnID := runtimeappStringFieldV1(turn, "id")
			if turnID == lineage.ParentTurnID {
				matchingTurns++
			}
			for _, rawItem := range runtimeappListAnyV1(turn["items"]) {
				item, ok := rawItem.(map[string]any)
				if !ok || runtimeappStringFieldV1(item, "kind") != "tool_call" ||
					runtimeappStringFieldV1(item, "callId") != lineage.ParentToolCallID {
					continue
				}
				matchingCalls++
				if turnID != lineage.ParentTurnID ||
					!legacyTypeScriptDelegationToolNameV1(runtimeappStringFieldV1(item, "toolName")) {
					return errors.New("legacy parent tool call identity or tool binding is invalid")
				}
				if itemTurnID := runtimeappStringFieldV1(item, "turnId"); itemTurnID != "" && itemTurnID != lineage.ParentTurnID {
					return errors.New("legacy parent tool call turn binding is invalid")
				}
			}
		}
		if matchingTurns != 1 || matchingCalls != 1 {
			return errors.New("legacy parent turn/tool lineage is ambiguous or missing")
		}
		return nil
	})
}

func legacyTypeScriptDelegationToolNameV1(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "delegate_task", "task", "parallel_tasks":
		return true
	default:
		return false
	}
}

func runtimeappStringFieldV1(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func runtimeappListAnyV1(value any) []any {
	items, _ := value.([]any)
	return items
}
