package evidencesettlement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestOriginalPreparedInventoryActivatesWithoutCreatingAndGuardsWrites(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "settlements")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	original, err := ObservePreparedInventoryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	factory, ok := any(original).(interface {
		OpenStoreWithRestartPreservationV1(context.Context, func(context.Context, domainevidence.PreparedEvidenceSettlement) error) (*Store, error)
	})
	if !ok {
		t.Fatal("original settlement activation is unavailable")
	}
	denied := errors.New("original held settlement cannot be retried")
	allow := false
	calls := 0
	store, err := factory.OpenStoreWithRestartPreservationV1(ctx, func(context.Context, domainevidence.PreparedEvidenceSettlement) error {
		calls++
		if !allow {
			return denied
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if records, err := store.ListPrepared(ctx); err != nil || len(records) != 0 {
		t.Fatalf("original absent list: %v", err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("original activation/list created absent owner: %v", err)
	}
	record := preparedStoreRecord(t, 3*time.Minute)
	if err := store.putPreparedBody(ctx, record); !errors.Is(err, denied) {
		t.Fatalf("held write guard lost cause: %v", err)
	}
	if calls != 1 {
		t.Fatalf("denial effects count=%d", calls)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("held denial created absent owner: %v", err)
	}
	allow = true
	if err := store.putPreparedBody(ctx, record); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("independent write lost before/after guard: calls=%d", calls)
	}
	if records, err := store.ListPrepared(ctx); err != nil || len(records) != 1 {
		t.Fatalf("independent record not visible through original reader: %v", err)
	}
	if _, err := factory.OpenStoreWithRestartPreservationV1(ctx, nil); err == nil {
		t.Fatal("activation omitted preservation authority")
	}
}

func TestOriginalPreparedInventoryIsReadOnlyAndBoundToCompleteOwner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "settlements")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	absent, err := ObservePreparedInventoryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if records, err := absent.SnapshotInventoryV1(context.Background()); err != nil || len(records) != 0 {
		t.Fatalf("absent original inventory: count=%d err=%v", len(records), err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("read-only observation created root: %v", err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	var first string
	for i := 0; i < 70; i++ {
		record := preparedStoreRecordForSuffixV1(t, 3*time.Minute, "-"+strconv.Itoa(i))
		body, err := domainevidence.PreparedEvidenceSettlementBytes(record)
		if err != nil {
			t.Fatal(err)
		}
		path := store.recordPath(record.SettlementID)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = path
		}
	}
	if err := absent.Revalidate(context.Background()); err == nil {
		t.Fatal("absent owner observation accepted new owner")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Dir(first), 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Dir(first), 0o700) })
	}
	before, err := os.Stat(filepath.Dir(first))
	if err != nil {
		t.Fatal(err)
	}
	original, err := ObservePreparedInventoryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if records, err := original.SnapshotInventoryV1(context.Background()); err != nil || len(records) != 70 {
		t.Fatalf("complete original inventory: count=%d err=%v", len(records), err)
	}
	after, err := os.Stat(filepath.Dir(first))
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode() != after.Mode() {
		t.Fatal("original observation changed directory mode")
	}
	if err := os.WriteFile(first, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := original.SnapshotInventoryV1(context.Background()); err == nil {
		t.Fatal("original inventory admitted a changed record")
	}
}

func TestOriginalPreparedInventoryRejectsUnsafeOwnerWithoutRepair(t *testing.T) {
	for _, name := range []string{"unknown owner file", "nested shard", "hardlink", "symlink"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "settlements")
			store, err := NewStore(root)
			if err != nil {
				t.Fatal(err)
			}
			record := preparedStoreRecord(t, 3*time.Minute)
			body, err := domainevidence.PreparedEvidenceSettlementBytes(record)
			if err != nil {
				t.Fatal(err)
			}
			path := store.recordPath(record.SettlementID)
			if name == "nested shard" {
				path = filepath.Join(store.prepared, "extra", record.SettlementID[:2], record.SettlementID+".json")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "unknown owner file":
				if err := os.WriteFile(filepath.Join(root, "unknown"), []byte("retained"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(filepath.Dir(root), "original-link")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(path, filepath.Join(store.prepared, "alias")); err != nil {
					t.Fatal(err)
				}
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ObservePreparedInventoryV1(context.Background(), root, access); err == nil {
				t.Fatal("unsafe original owner accepted")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != string(body) {
				t.Fatalf("rejected observation changed original: %v", err)
			}
		})
	}
}

func TestOriginalPreparedInventoryRetainsActualEncoding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "settlements")
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	record := preparedStoreRecord(t, 3*time.Minute)
	canonical, err := domainevidence.PreparedEvidenceSettlementBytes(record)
	if err != nil {
		t.Fatal(err)
	}
	original := append(append([]byte(nil), canonical...), '\n', ' ')
	file := store.recordPath(record.SettlementID)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, original, 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := ObservePreparedInventoryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	bodies, err := observed.SnapshotPreparedFileBytesV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name := "prepared/" + record.SettlementID[:2] + "/" + record.SettlementID + ".json"
	if string(bodies[name]) != string(original) {
		t.Fatal("original prepared bytes were replaced by canonical reconstruction")
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(original) {
		t.Fatal("original prepared read changed stored encoding")
	}
}
