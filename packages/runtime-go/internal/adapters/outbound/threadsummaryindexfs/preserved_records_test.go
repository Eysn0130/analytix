package threadsummaryindexfs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func preservedSummaryFixtureV1(t *testing.T, body string) (string, *PreservedRecordsV1) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "thread_summaries.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	preserved, err := PreparePreservedRecordsV1(context.Background(), path, []string{"held"})
	if err != nil {
		t.Fatal(err)
	}
	return path, preserved
}

func TestPreservedSummaryRecordsKeepEveryHeldRowAndOriginalFraming(t *testing.T) {
	heldFirst := " { \"summary\": {\"id\":\"held\", \"title\":\"FIRST\"}, \"schemaVersion\":1 } \r\n"
	active := "{\"schemaVersion\":1,\"threadId\":\"active\",\"summary\":{\"id\":\"active\",\"legacy\":\"REMOVE\"}}\n"
	heldLast := "\t{\"deleted\":true, \"threadId\":\"held\",\"schemaVersion\":1,\"summary\":{\"id\":\"held\",\"title\":\"LAST\"}}"
	path, preserved := preservedSummaryFixtureV1(t, "\n"+heldFirst+active+heldLast)
	var first []byte
	for iteration := 0; iteration < 2; iteration++ {
		calls := 0
		if err := preserved.Transform(context.Background(), func(record map[string]any, line int, _ string) (map[string]any, bool, error) {
			calls++
			if line != 3 {
				t.Errorf("transform reached held or reordered row: %d", line)
			}
			summary := record["summary"].(map[string]any)
			_, changed := summary["legacy"]
			delete(summary, "legacy")
			return record, changed, nil
		}); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 || !bytes.HasPrefix(body, []byte("\n"+heldFirst)) || !bytes.HasSuffix(body, []byte(heldLast)) || bytes.Contains(body, []byte("REMOVE")) {
			t.Fatal("shared transformation changed held rows or failed independent cleanup")
		}
		if iteration == 0 {
			first = body
		} else if !bytes.Equal(first, body) {
			t.Fatal("repeat migration changed already stable shared index")
		}
		if err := preserved.Revalidate(context.Background()); err != nil {
			t.Fatal(err)
		}
		parent, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if parent.Mode().Perm() != 0o750 {
			t.Fatal("summary writer changed parent permissions")
		}
	}
}

func TestPreservedSummaryRecordsRejectAmbiguousCompleteInventoryBeforeCallbacks(t *testing.T) {
	valid := "{\"threadId\":\"active\",\"summary\":{\"id\":\"active\"}}\n"
	for _, bad := range []string{
		`{"threadId":"held","threadId":"active","summary":{"id":"active"}}`,
		`{"threadId":"active","summary":{"id":"held"}}`,
		`{"summary":{"title":"no owner"}}`,
		`{"threadId":null,"summary":{"id":"held"}}`,
		`{"threadId":" held","summary":{"id":"held"}}`,
		`{"threadId":"held","summary":null}`,
	} {
		path := filepath.Join(t.TempDir(), "index.jsonl")
		body := []byte(valid + bad)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if scope, err := PreparePreservedRecordsV1(context.Background(), path, []string{"held"}); err == nil || scope != nil {
			t.Fatal("ambiguous summary acquired preservation identity")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(body, after) {
			t.Fatal("invalid summary preparation wrote original state")
		}
	}
	path, preserved := preservedSummaryFixtureV1(t, valid)
	changed := valid + `{"threadId":"held","threadId":"active","summary":{"id":"active"}}`
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := preserved.Transform(context.Background(), func(row map[string]any, _ int, _ string) (map[string]any, bool, error) {
		calls++
		return row, true, nil
	}); err == nil || calls != 0 {
		t.Fatal("early independent callback ran before late malformed row refusal")
	}
}

func TestPreservedSummaryRecordsRejectDriftReclassificationAndCancellation(t *testing.T) {
	original := "{\"threadId\":\"active\",\"summary\":{\"id\":\"active\"}}\n {\"summary\":{\"id\":\"held\"}}"
	for _, mode := range []string{"held_whitespace", "held_removed", "held_added", "source_drift", "relabel", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			path, preserved := preservedSummaryFixtureV1(t, original)
			want := []byte(original)
			if mode == "held_whitespace" {
				want = []byte(strings.Replace(original, " {\"summary", "  {\"summary", 1))
			}
			if mode == "held_removed" {
				want = []byte(strings.Split(original, "\n")[0] + "\n")
			}
			if mode == "held_added" {
				want = []byte(original + "\n{\"threadId\":\"held\",\"summary\":{\"id\":\"held\"}}")
			}
			if err := os.WriteFile(path, want, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			calls := 0
			err := preserved.Transform(ctx, func(row map[string]any, _ int, _ string) (map[string]any, bool, error) {
				calls++
				if mode == "source_drift" {
					want = []byte("\n" + original)
					if err := os.WriteFile(path, want, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "relabel" {
					row["threadId"] = "held"
					row["summary"].(map[string]any)["id"] = "held"
				}
				return row, true, nil
			})
			if err == nil {
				t.Fatal("unsafe shared index transformation succeeded")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancelled transform did not preserve cause")
			}
			if mode != "source_drift" && mode != "relabel" && calls != 0 {
				t.Fatal("held inventory fault reached independent callback")
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(after, want) {
				t.Fatal("failed transform overwrote original or concurrently changed source")
			}
		})
	}
}

func TestPreservedSummaryRecordsNoopAndAbsentIndexDoNotWrite(t *testing.T) {
	path, preserved := preservedSummaryFixtureV1(t, "{\"threadId\":\"held\",\"summary\":{\"id\":\"held\"}}")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := preserved.Transform(context.Background(), func(map[string]any, int, string) (map[string]any, bool, error) {
		t.Error("held row callback invoked")
		return nil, false, nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("no-op transform replaced original file")
	}
	missing := filepath.Join(t.TempDir(), "absent.jsonl")
	empty, err := PreparePreservedRecordsV1(context.Background(), missing, []string{"held"})
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Transform(context.Background(), func(map[string]any, int, string) (map[string]any, bool, error) {
		t.Error("absent index callback invoked")
		return nil, false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("absent index was materialized")
	}
}

func TestPreservedSummaryRecordsSerializeIndependentTransforms(t *testing.T) {
	path, preserved := preservedSummaryFixtureV1(t, "{\"threadId\":\"active\",\"summary\":{\"id\":\"active\",\"count\":0}}\n{\"threadId\":\"held\",\"summary\":{\"id\":\"held\"}}")
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := preserved.Transform(context.Background(), func(row map[string]any, _ int, _ string) (map[string]any, bool, error) {
				summary := row["summary"].(map[string]any)
				count, err := summary["count"].(json.Number).Int64()
				if err != nil {
					return nil, false, err
				}
				summary["count"] = count + 1
				return row, true, nil
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"count":4`)) {
		t.Fatal("concurrent independent update was lost")
	}
	if err := preserved.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
}
