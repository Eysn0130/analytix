//go:build darwin || linux

package casethreadauthority

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRejectsAuthorityRecordWithASecondHardLink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "case-thread-authority")
	store, err := newTestCaseThreadStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	record := testCaseThreadAuthorityRecord(t)
	if err := store.PutIfAbsent(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, record.RecordDigest[:2], record.RecordDigest+".json")
	if err := os.Link(path, filepath.Join(t.TempDir(), "authority-hardlink.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("case thread authority accepted a multiply-linked private record")
	}
}
