package startup

import (
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestManagedStartupSnapshotAndBaselineRejectTampering(t *testing.T) {
	entries := []ManagedEntryStateV1{
		{Path: "data/attachments", Type: ManagedEntryTypeAbsent},
		{Path: "data/private/record.json", Type: ManagedEntryTypeFile, Mode: 0o600, Size: 2, ModTimeUnixNano: 1, SHA256: domainsecurity.SHA256Hex([]byte("{}")), RecordCount: 1},
	}
	snapshot, err := NewManagedSnapshotV1([]string{"/durable", "/data"}, domainsecurity.SHA256Hex([]byte("raw")), entries)
	if err != nil || ValidateManagedSnapshotV1(snapshot) != nil {
		t.Fatalf("seal managed snapshot: snapshot=%#v err=%v", snapshot, err)
	}
	baseline, err := NewReadOnlyStartupBaselineV1(snapshot, domainsecurity.SHA256Hex([]byte("config")), time.Date(2026, 7, 11, 2, 3, 4, 0, time.UTC))
	if err != nil || ValidateReadOnlyStartupBaselineV1(baseline) != nil {
		t.Fatalf("seal startup baseline: baseline=%#v err=%v", baseline, err)
	}
	tampered := snapshot
	tampered.Entries = append([]ManagedEntryStateV1(nil), snapshot.Entries...)
	tampered.Entries[1].SHA256 = domainsecurity.SHA256Hex([]byte("forged"))
	if ValidateManagedSnapshotV1(tampered) == nil {
		t.Fatal("tampered managed snapshot retained authority")
	}
	badBaseline := baseline
	badBaseline.ConfigurationDigest = domainsecurity.SHA256Hex([]byte("other-config"))
	if ValidateReadOnlyStartupBaselineV1(badBaseline) == nil {
		t.Fatal("tampered configuration binding retained startup authority")
	}
}

func TestManagedStartupSnapshotDistinguishesAbsentAndEmptyDirectory(t *testing.T) {
	rawDigest := domainsecurity.SHA256Hex([]byte("raw"))
	absent, err := NewManagedSnapshotV1([]string{"/data"}, rawDigest, []ManagedEntryStateV1{{Path: "data/memory", Type: ManagedEntryTypeAbsent}})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := NewManagedSnapshotV1([]string{"/data"}, rawDigest, []ManagedEntryStateV1{{Path: "data/memory", Type: ManagedEntryTypeDirectory, Mode: 0o700, ModTimeUnixNano: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if absent.SnapshotDigest == empty.SnapshotDigest {
		t.Fatal("absent and empty managed directories shared one digest")
	}
	if _, err := NewManagedSnapshotV1([]string{"/data", "/data"}, rawDigest, []ManagedEntryStateV1{{Path: "data/memory", Type: ManagedEntryTypeAbsent}}); err == nil {
		t.Fatal("duplicate startup roots were accepted")
	}
}

func TestManagedSnapshotHasSemanticInputV1RequiresAHostCapturedFile(t *testing.T) {
	rawDigest := domainsecurity.SHA256Hex([]byte("raw"))
	for _, fixture := range []struct {
		name    string
		entries []ManagedEntryStateV1
		want    bool
	}{
		{name: "absent", entries: []ManagedEntryStateV1{{Path: "durable/threads", Type: ManagedEntryTypeAbsent}}},
		{name: "empty directory", entries: []ManagedEntryStateV1{{Path: "durable/threads", Type: ManagedEntryTypeDirectory, Mode: 0o700, ModTimeUnixNano: 1}}},
		{name: "managed file", entries: []ManagedEntryStateV1{
			{Path: "durable/threads", Type: ManagedEntryTypeDirectory, Mode: 0o700, ModTimeUnixNano: 1},
			{Path: "durable/threads/thread-a/messages.jsonl", Type: ManagedEntryTypeFile, Mode: 0o600, Size: 3, ModTimeUnixNano: 2, SHA256: domainsecurity.SHA256Hex([]byte("{}\n")), RecordCount: 1},
		}, want: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			snapshot, err := NewManagedSnapshotV1([]string{"/data", "/durable"}, rawDigest, fixture.entries)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ManagedSnapshotHasSemanticInputV1(snapshot)
			if err != nil || got != fixture.want {
				t.Fatalf("semantic input = %v, %v; want %v", got, err, fixture.want)
			}
		})
	}

	snapshot, err := NewManagedSnapshotV1([]string{"/data"}, rawDigest, []ManagedEntryStateV1{{Path: "data/memory", Type: ManagedEntryTypeAbsent}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SnapshotDigest = domainsecurity.SHA256Hex([]byte("forged"))
	if _, err := ManagedSnapshotHasSemanticInputV1(snapshot); err == nil {
		t.Fatal("tampered managed snapshot supplied semantic-input authority")
	}
}
