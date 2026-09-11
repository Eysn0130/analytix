package startup

import (
	"context"
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

type startupSnapshotReaderStub struct {
	snapshots []domainstartup.ManagedSnapshotV1
	calls     int
}

func (stub *startupSnapshotReaderStub) CaptureManagedSnapshotV1(context.Context) (domainstartup.ManagedSnapshotV1, error) {
	if stub.calls >= len(stub.snapshots) {
		return domainstartup.ManagedSnapshotV1{}, errors.New("unexpected capture")
	}
	snapshot := stub.snapshots[stub.calls]
	stub.calls++
	return snapshot, nil
}

func TestStartupPlanSnapshotDriftRejectsBeforeJournal(t *testing.T) {
	first := startupSnapshotForTest(t, "first")
	drifted := startupSnapshotForTest(t, "drifted")
	reader := &startupSnapshotReaderStub{snapshots: []domainstartup.ManagedSnapshotV1{first, drifted}}
	session, err := BeginReadOnlyStartupPlanV1(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.BindConfiguration(context.Background(), domainsecurity.SHA256Hex([]byte("config")), time.Now().UTC())
	if !errors.Is(err, ErrStartupSnapshotDrift) || reader.calls != 2 {
		t.Fatalf("startup snapshot drift was not rejected before apply: err=%v calls=%d", err, reader.calls)
	}
}

func TestReadOnlyStartupBaselineBindsStableSnapshotAndConfiguration(t *testing.T) {
	snapshot := startupSnapshotForTest(t, "stable")
	reader := &startupSnapshotReaderStub{snapshots: []domainstartup.ManagedSnapshotV1{snapshot, snapshot}}
	session, err := BeginReadOnlyStartupPlanV1(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	configurationDigest := domainsecurity.SHA256Hex([]byte("config"))
	baseline, err := session.BindConfiguration(context.Background(), configurationDigest, time.Date(2026, 7, 11, 3, 4, 5, 0, time.UTC))
	if err != nil || domainstartup.ValidateReadOnlyStartupBaselineV1(baseline) != nil || baseline.ManagedSnapshotDigest != snapshot.SnapshotDigest ||
		baseline.ConfigurationDigest != configurationDigest || reader.calls != 2 {
		t.Fatalf("stable startup baseline mismatch: baseline=%#v err=%v calls=%d", baseline, err, reader.calls)
	}
}

func startupSnapshotForTest(t *testing.T, content string) domainstartup.ManagedSnapshotV1 {
	t.Helper()
	snapshot, err := domainstartup.NewManagedSnapshotV1([]string{"/data", "/durable"}, domainsecurity.SHA256Hex([]byte("raw-"+content)), []domainstartup.ManagedEntryStateV1{{
		Path: "data/private/record.json", Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(content)),
		ModTimeUnixNano: 1, SHA256: domainsecurity.SHA256Hex([]byte(content)), RecordCount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
