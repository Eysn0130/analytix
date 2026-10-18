package processdisplay

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	identity "analytix.local/runtime-go/internal/domain/identity"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	security "analytix.local/runtime-go/internal/domain/security"
	port "analytix.local/runtime-go/internal/ports/processdisplay"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func storeHistoryFixtureV1(t *testing.T) display.SettledHistoryV1 {
	t.Helper()
	principal, err := identity.NewPrincipalV1(strings.Repeat("1", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	original := security.NewTurnSecurityContext(security.TurnSecurityContextInput{ThreadID: "thread-history", TurnID: "turn-history", WorkspaceRealPath: "/synthetic/workspace", TenantID: "local", UserID: "local"})
	return display.SettledHistoryV1{SchemaVersion: display.SettledHistorySchemaV1, ThreadID: original.ThreadID, TurnID: original.TurnID, Principal: principal, Original: original, Terminal: display.TerminalWitnessV1{Status: "completed", FinishedAt: "2026-10-14T01:00:00Z", AuthorityDigest: strings.Repeat("a", 64), SourceDigest: security.SHA256Hex([]byte("[]"))}, Units: []display.UnitV2{}, Blocks: []display.SettledBlockV1{}}
}
func TestSettledHistoryStoreCapacityRetryRestartAndWrongAnchorV1(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "process-history-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(ctx, root, access, LimitsV1{}); err == nil {
		t.Fatal("unadmitted zero purpose budget opened store")
	}
	for _, limits := range []LimitsV1{{Bytes: RootByteLimitV1 + 1, Records: 1}, {Bytes: 1, Records: RootRecordLimitV1 + 1}} {
		if _, err := NewStore(ctx, root, access, limits); err == nil {
			t.Fatal("purpose hard budget widened")
		}
	}
	if got := DefaultLimitsV1(); got.Bytes != 128<<20 || got.Records != 4096 {
		t.Fatal("approved purpose default budget drifted")
	}
	s, err := NewStore(ctx, root, access, LimitsV1{Bytes: 1 << 20, Records: 1})
	if err != nil {
		t.Fatal(err)
	}
	v := storeHistoryFixtureV1(t)
	a, err := s.Put(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	v2 := v
	v2.Terminal.FinishedAt = "2026-10-14T01:00:01Z"
	if _, err := s.Put(ctx, v2); !errors.Is(err, port.ErrCapacityV1) {
		t.Fatal("capacity widened")
	}
	if again, err := s.Put(ctx, v); err != nil || again != a {
		t.Fatal("exact retry rejected at capacity")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewStore(ctx, root, access, LimitsV1{Bytes: 1, Records: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Read(ctx, a)
	if err != nil || !reflect.DeepEqual(got, v) {
		t.Fatal("restart or capacity lost immutable record", err)
	}
	if len(s.sizes) != 1 || s.bytes <= 1 {
		t.Fatal("full committed/orphan inventory not counted")
	}
	wrong := a
	wrong.Terminal.AuthorityDigest = strings.Repeat("f", 64)
	if _, err := s.Read(ctx, wrong); err == nil {
		t.Fatal("wrong terminal anchor released body")
	}
	s.paused = true
	if _, err := s.Put(ctx, v2); !errors.Is(err, port.ErrCapacityV1) {
		t.Fatal("uncertain write generation admitted new record")
	}
	if _, err := s.Read(ctx, a); err != nil {
		t.Fatal("paused writes revoked old immutable reads")
	}
}
