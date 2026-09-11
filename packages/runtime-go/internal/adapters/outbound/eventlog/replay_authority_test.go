package eventlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStoreRejectsMismatchedThreadAtEveryReadWriteEntrypoint(t *testing.T) {
	for _, entrypoint := range []string{"single", "atomic", "raw", "replay"} {
		t.Run(entrypoint, func(t *testing.T) {
			store := NewStore(t.TempDir())
			const routeThread = "thr_route"
			const recordThread = "thr_record"
			event := map[string]any{"seq": float64(1), "kind": "usage", "threadId": recordThread}
			var err error
			switch entrypoint {
			case "single":
				err = store.AppendEvent(routeThread, event)
			case "atomic":
				err = store.AppendEventsAtomic(routeThread, []map[string]any{event})
			case "raw":
				_, _, err = store.AppendRawLine(routeThread, `{"seq":1,"kind":"usage","threadId":"thr_record"}`)
			case "replay":
				if mkdirErr := os.MkdirAll(store.ThreadDir(routeThread), 0o700); mkdirErr != nil {
					t.Fatal(mkdirErr)
				}
				if writeErr := os.WriteFile(store.EventsPath(routeThread), []byte(`{"seq":1,"kind":"usage","threadId":"thr_record"}`+"\n"), 0o600); writeErr != nil {
					t.Fatal(writeErr)
				}
				_, err = store.LoadSince(routeThread, 0)
			}
			if err == nil {
				t.Fatalf("%s accepted a mismatched thread identity", entrypoint)
			}
		})
	}
}

func TestStoreRejectsUnsafeThreadIDBeforePathResolution(t *testing.T) {
	for _, threadID := range []string{"", " thr_space", "thr_space ", "thr/path", `thr\\path`, "thr..path"} {
		t.Run(strings.ReplaceAll(threadID, "/", "_"), func(t *testing.T) {
			store := NewStore(t.TempDir())
			event := map[string]any{"seq": float64(1), "kind": "usage", "threadId": threadID}
			if err := store.AppendEvent(threadID, event); err == nil {
				t.Fatalf("unsafe threadId %q reached append", threadID)
			}
			if _, err := store.LoadSince(threadID, 0); err == nil {
				t.Fatalf("unsafe threadId %q reached replay", threadID)
			}
		})
	}
}

func TestStoreRejectsDuplicateGapAndOutOfOrderReplay(t *testing.T) {
	fixtures := map[string][]string{
		"non_positive": {`{"seq":0,"kind":"usage","threadId":"thr_non_positive"}`},
		"duplicate": {
			`{"seq":1,"kind":"usage","threadId":"thr_duplicate"}`,
			`{"seq":1,"kind":"usage","threadId":"thr_duplicate"}`,
		},
		"gap": {
			`{"seq":1,"kind":"usage","threadId":"thr_gap"}`,
			`{"seq":3,"kind":"usage","threadId":"thr_gap"}`,
		},
		"out_of_order": {
			`{"seq":2,"kind":"usage","threadId":"thr_out_of_order"}`,
			`{"seq":1,"kind":"usage","threadId":"thr_out_of_order"}`,
		},
	}
	for name, records := range fixtures {
		t.Run(name, func(t *testing.T) {
			threadID := "thr_" + name
			store := NewStore(t.TempDir())
			if err := os.MkdirAll(store.ThreadDir(threadID), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.EventsPath(threadID), []byte(strings.Join(records, "\n")+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := store.LoadSince(threadID, 0)
			if err == nil || result.Events != nil || result.Diagnostics != nil {
				t.Fatalf("invalid sequence returned a partial replay: result=%#v err=%v", result, err)
			}
		})
	}
}

func TestStoreAppendRejectsGapWithoutChangingCommittedLog(t *testing.T) {
	store := NewStore(t.TempDir())
	threadID := "thr_append_gap"
	if err := store.AppendEvent(threadID, map[string]any{"seq": float64(1), "kind": "usage", "threadId": threadID}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.EventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(threadID, map[string]any{"seq": float64(3), "kind": "usage", "threadId": threadID}); err == nil {
		t.Fatal("gap append was accepted")
	}
	after, err := os.ReadFile(store.EventsPath(threadID))
	if err != nil || string(after) != string(before) {
		t.Fatalf("rejected gap changed committed bytes: err=%v before=%q after=%q", err, before, after)
	}
}

func TestStoreReplayRejectsBlankMissingNewlineAndOversizedRecord(t *testing.T) {
	fixtures := map[string]string{
		"blank":           `{"seq":1,"kind":"usage","threadId":"thr_blank"}` + "\n\n",
		"missing_newline": `{"seq":1,"kind":"usage","threadId":"thr_missing_newline"}`,
		"oversized": `{"seq":1,"kind":"usage","threadId":"thr_oversized","value":"` +
			strings.Repeat("x", maxEventJSONLLineBytesV1) + `"}` + "\n",
	}
	for name, body := range fixtures {
		t.Run(name, func(t *testing.T) {
			threadID := "thr_" + name
			store := NewStore(t.TempDir())
			if err := os.MkdirAll(store.ThreadDir(threadID), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.EventsPath(threadID), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.LoadSince(threadID, 0); err == nil {
				t.Fatalf("%s event log was accepted", name)
			}
		})
	}
}

func TestStoreReplayRejectsSymlinkedAuthority(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink authority fixture is platform-specific")
	}
	t.Run("event_file", func(t *testing.T) {
		root := t.TempDir()
		store := NewStore(root)
		threadID := "thr_symlink_file"
		if err := os.MkdirAll(store.ThreadDir(threadID), 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "outside-events.jsonl")
		if err := os.WriteFile(target, []byte(`{"seq":1,"kind":"usage","threadId":"thr_symlink_file"}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, store.EventsPath(threadID)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LoadSince(threadID, 0); err == nil {
			t.Fatal("symlinked event file was accepted")
		}
	})
	t.Run("thread_directory", func(t *testing.T) {
		root := t.TempDir()
		store := NewStore(root)
		threadID := "thr_symlink_directory"
		outside := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "threads"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, store.ThreadDir(threadID)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LoadSince(threadID, 0); err == nil {
			t.Fatal("symlinked thread directory was accepted")
		}
	})
}

func TestStoreReplayRejectsPreCancelledContext(t *testing.T) {
	store := NewStore(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := store.LoadSinceContext(ctx, "thr_cancelled", 0)
	if !errors.Is(err, context.Canceled) || result.Events != nil || result.Diagnostics != nil {
		t.Fatalf("pre-cancelled replay did not fail before filesystem work: result=%#v err=%v", result, err)
	}
}
