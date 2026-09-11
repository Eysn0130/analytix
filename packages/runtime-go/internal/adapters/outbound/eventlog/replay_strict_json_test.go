package eventlog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStoreReplayReportsMalformedStrictJSONWithFixedMetadataAndContinues(t *testing.T) {
	for _, test := range []struct {
		name    string
		invalid string
	}{
		{
			name:    "malformed",
			invalid: `{"private":"PRIVATE_MALFORMED_JSON_SENTINEL"`,
		},
		{
			name:    "duplicate-key",
			invalid: `{"seq":2,"seq":999,"kind":"usage","threadId":"thr_strict_json"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			const threadID = "thr_strict_json"
			writeStrictJSONReplayFixture(t, store, threadID, []string{
				`{"seq":1,"kind":"usage","threadId":"thr_strict_json"}`,
				test.invalid,
				`{"seq":2,"kind":"usage","threadId":"thr_strict_json"}`,
			})

			result, err := store.LoadSince(threadID, 0)
			if err != nil {
				t.Fatalf("load replay around malformed strict JSON: %v", err)
			}
			if len(result.Events) != 2 {
				t.Fatalf("valid replay events = %d, want 2: %#v", len(result.Events), result.Events)
			}
			for index, expectedSeq := range []int{1, 2} {
				seq, ok := exactEventSequenceV1(result.Events[index]["seq"])
				if !ok || seq != expectedSeq {
					t.Fatalf("event %d sequence = %#v, want %d", index, result.Events[index]["seq"], expectedSeq)
				}
			}
			expectedDiagnostic := JSONLDiagnostic{
				Path: "threads/thr_strict_json/events.jsonl", Line: 2, Error: "invalid_json", Preview: "",
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0] != expectedDiagnostic {
				t.Fatalf("strict JSON diagnostic = %#v, want %#v", result.Diagnostics, expectedDiagnostic)
			}
			serialized, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if strings.Contains(string(serialized), "PRIVATE_MALFORMED_JSON_SENTINEL") ||
				strings.Contains(string(serialized), test.invalid) {
				t.Fatalf("strict JSON diagnostic exposed rejected record bytes: %s", serialized)
			}
		})
	}
}

func TestStoreReplayStillRejectsSequenceGapAfterMalformedStrictJSON(t *testing.T) {
	store := NewStore(t.TempDir())
	const threadID = "thr_strict_json_gap"
	writeStrictJSONReplayFixture(t, store, threadID, []string{
		`{"seq":1,"kind":"usage","threadId":"thr_strict_json_gap"}`,
		`{"private":"PRIVATE_MALFORMED_JSON_SENTINEL"`,
		`{"seq":3,"kind":"usage","threadId":"thr_strict_json_gap"}`,
	})

	result, err := store.LoadSince(threadID, 0)
	if err == nil || !strings.Contains(err.Error(), "sequence is not positive, contiguous, and physically ordered") {
		t.Fatalf("sequence gap after malformed JSON was not rejected: result=%#v err=%v", result, err)
	}
	if result.Events != nil || result.Diagnostics != nil {
		t.Fatalf("sequence failure returned partial replay authority: %#v", result)
	}
}

func TestStoreAppendFailsClosedOnMalformedStrictJSONDiagnostic(t *testing.T) {
	store := NewStore(t.TempDir())
	const threadID = "thr_strict_json_append"
	writeStrictJSONReplayFixture(t, store, threadID, []string{
		`{"seq":1,"kind":"usage","threadId":"thr_strict_json_append"}`,
		`{"private":"PRIVATE_MALFORMED_JSON_SENTINEL"`,
	})
	before, err := os.ReadFile(store.EventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}

	err = store.AppendEvent(threadID, map[string]any{
		"seq": float64(2), "kind": "usage", "threadId": threadID,
	})
	if err == nil || !strings.Contains(err.Error(), "semantic content migration") {
		t.Fatalf("append did not fail closed on replay diagnostic: %v", err)
	}
	if strings.Contains(err.Error(), "PRIVATE_MALFORMED_JSON_SENTINEL") {
		t.Fatalf("append error exposed rejected record bytes: %v", err)
	}
	after, readErr := os.ReadFile(store.EventsPath(threadID))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Fatalf("rejected append changed committed bytes:\nbefore=%q\nafter=%q", before, after)
	}
}

func writeStrictJSONReplayFixture(t *testing.T, store *Store, threadID string, records []string) {
	t.Helper()
	if err := os.MkdirAll(store.ThreadDir(threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.EventsPath(threadID), []byte(strings.Join(records, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
