package toolresultsnapshot

import (
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func snapshotFixtureV1(t *testing.T, body string) domaintoolresult.ProtectedSnapshotV1 {
	t.Helper()
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	return domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal,
		Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID,
		ResultItemID: domaintoolresult.ToolResultItemIDV1("turn-a", callID), ToolName: "read", ContextDigest: strings.Repeat("c", 64),
		ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64),
		Capture: domaintoolresult.ProtectedCaptureV1{Kind: "read", Status: "completed", Body: body, Label: "code.go", StartLine: 1, EndLine: 1, TotalLines: 1}}
}
func TestProtectedSnapshotStoreCountsOrphansAndKeepsExactRetryAtCapacity(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "tool-result-snapshots-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	orphan := snapshotFixtureV1(t, "orphan\r\n")
	bytes, err := domaintoolresult.ProtectedSnapshotBytesV1(orphan)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex(bytes)
	raw, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(root, EnvelopeLimitV1, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.PutIfAbsent(ctx, digest, bytes); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	store, err := NewStore(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if len(store.sizes) != 1 || store.bytes != int64(len(bytes)) {
		t.Fatal("physical unreferenced envelope was not counted")
	}
	binding, err := store.Put(ctx, orphan)
	if err != nil {
		t.Fatal(err)
	}
	store.bytes = RootByteLimitV1
	if _, err := store.Put(ctx, snapshotFixtureV1(t, "another\n")); !errors.Is(err, snapshotport.ErrCapacity) {
		t.Fatal("new capture did not refuse full root")
	}
	if again, err := store.Put(ctx, orphan); err != nil || again != binding {
		t.Fatal("exact retry was refused at capacity")
	}
	actual, err := store.Read(ctx, binding)
	if err != nil || !reflect.DeepEqual(actual, orphan) {
		t.Fatal("ordinary original body unavailable at capacity")
	}
	store.paused = true
	if _, err := store.Put(ctx, snapshotFixtureV1(t, "new\n")); !errors.Is(err, snapshotport.ErrCapacity) {
		t.Fatal("uncertain generation admitted new capture")
	}
	if _, err := store.Read(ctx, binding); err != nil {
		t.Fatal("paused generation revoked immutable reads")
	}
	binding.BodyDigest = strings.Repeat("f", 64)
	if _, err := store.Read(ctx, binding); err == nil {
		t.Fatal("wrong body digest released content")
	}
}
func TestProtectedSnapshotStoreRestartRejectsUntrustedPhysicalEnvelope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "tool-result-snapshots-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(root, EnvelopeLimitV1, access)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("{\"body\":\"not an owned envelope\"}")
	if err := raw.PutIfAbsent(ctx, domainsecurity.SHA256Hex(body), body); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if store, err := NewStore(ctx, root, access); err == nil {
		store.Close()
		t.Fatal("untrusted physical orphan was silently ignored")
	}
}
