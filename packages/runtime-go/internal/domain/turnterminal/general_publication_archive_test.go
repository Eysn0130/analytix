package turnterminal

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func TestGeneralTerminalPublicationArchiveIsDeterministicAndContainsNoAssistantText(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := NewGeneralTerminalPublicationArchiveV1(nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := AppendGeneralTerminalPublicationArchiveV1(empty, commit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AppendGeneralTerminalPublicationArchiveV1(first, commit)
	if err != nil || !reflect.DeepEqual(first, second) || len(second.Commits) != 1 {
		t.Fatalf("archive append was not idempotent: first=%#v second=%#v err=%v", first, second, err)
	}
	body, _ := json.Marshal(second)
	if strings.Contains(string(body), domainevent.GeneralTerminalCompletedBoundaryTextV1) {
		t.Fatalf("archive retained assistant prose: %s", body)
	}
	parsed, err := ParseGeneralTerminalPublicationArchiveV1(GeneralTerminalPublicationArchiveV1Map(second))
	if err != nil || !reflect.DeepEqual(parsed, second) {
		t.Fatalf("strict archive round trip diverged: parsed=%#v err=%v", parsed, err)
	}
}

func TestGeneralTerminalPublicationArchiveRejectsConflictUnknownAndTampering(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	firstCommit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := NewGeneralTerminalPublicationArchiveV1([]GeneralTerminalPublicationCommitV1{firstCommit})
	if err != nil {
		t.Fatal(err)
	}
	otherContext, otherBinding, otherDrafts := generalTerminalPublicationFixture(t, "")
	conflict, err := NewGeneralTerminalPublicationCommitV1(otherContext, otherBinding, "2026-07-15T00:00:00Z", otherDrafts[1:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AppendGeneralTerminalPublicationArchiveV1(archive, conflict); err == nil {
		t.Fatal("same-turn conflicting commit was accepted")
	}

	unknown := GeneralTerminalPublicationArchiveV1Map(archive)
	unknown["unexpected"] = true
	if _, err := ParseGeneralTerminalPublicationArchiveV1(unknown); err == nil {
		t.Fatal("archive accepted an unknown top-level field")
	}
	unknownCommit := GeneralTerminalPublicationArchiveV1Map(archive)
	unknownCommit["commits"].([]any)[0].(map[string]any)["unexpected"] = true
	if _, err := ParseGeneralTerminalPublicationArchiveV1(unknownCommit); err == nil {
		t.Fatal("archive accepted an unknown nested commit field")
	}

	tampered := cloneGeneralTerminalPublicationArchiveV1(archive)
	tampered.Commits[0].TurnID = "turn-other"
	tampered.ArchiveDigest = generalTerminalPublicationArchiveDigestV1(tampered)
	if ValidateGeneralTerminalPublicationArchiveV1(tampered) == nil {
		t.Fatal("archive accepted a recomputed outer digest over a tampered commit")
	}
	duplicate := cloneGeneralTerminalPublicationArchiveV1(archive)
	duplicate.Commits = append(duplicate.Commits, cloneGeneralTerminalPublicationCommitV1(firstCommit))
	duplicate.ArchiveDigest = generalTerminalPublicationArchiveDigestV1(duplicate)
	if ValidateGeneralTerminalPublicationArchiveV1(duplicate) == nil {
		t.Fatal("archive accepted a duplicate commit")
	}
}
