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

func TestParseGeneralTerminalPublicationArchiveOwnsNestedValuesAndPreservesNumbers(t *testing.T) {
	context, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	commit, err := NewGeneralTerminalPublicationCommitV1(context, binding, "2026-07-15T00:00:00Z", drafts)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := NewGeneralTerminalPublicationArchiveV1([]GeneralTerminalPublicationCommitV1{commit})
	if err != nil {
		t.Fatal(err)
	}
	for _, inputKind := range []string{"map", "typed"} {
		t.Run(inputKind, func(t *testing.T) {
			inputArchive := cloneGeneralTerminalPublicationArchiveV1(canonical)
			var input any = inputArchive
			if inputKind == "map" {
				input = GeneralTerminalPublicationArchiveV1Map(inputArchive)
			}
			encoded := func(value any) string {
				t.Helper()
				body, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				return string(body)
			}
			inputBefore := encoded(input)
			first, err := ParseGeneralTerminalPublicationArchiveV1(input)
			if err != nil {
				t.Fatal(err)
			}
			second, err := ParseGeneralTerminalPublicationArchiveV1(input)
			if err != nil {
				t.Fatal(err)
			}
			for _, parsed := range []GeneralTerminalPublicationArchiveV1{first, second} {
				if !reflect.DeepEqual(parsed, canonical) || parsed.ArchiveDigest != canonical.ArchiveDigest || encoded(parsed) != encoded(canonical) {
					t.Fatal("parse changed the canonical archive or deterministic digest")
				}
				usage := parsed.Commits[0].UsageEvent["usage"].(map[string]any)
				if number, ok := usage["totalTokens"].(json.Number); !ok || number.String() != "3" {
					t.Fatalf("parse changed the nested exact number representation: %T", usage["totalTokens"])
				}
			}
			first.Commits[0].UsageEvent["usage"].(map[string]any)["totalTokens"] = json.Number("9")
			first.Commits[0].TerminalEvent["status"] = "aborted"
			first.Commits[0].Events[0].Slot = "usage"
			if encoded(input) != inputBefore || !reflect.DeepEqual(second, canonical) {
				t.Fatal("parsed output shares nested maps or event slices with its input or another parse")
			}
			firstBefore := encoded(first)
			switch source := input.(type) {
			case map[string]any:
				stored := source["commits"].([]any)[0].(map[string]any)
				stored["usageEvent"].(map[string]any)["usage"].(map[string]any)["totalTokens"] = json.Number("12")
				stored["terminalEvent"].(map[string]any)["status"] = "failed"
				stored["events"].([]any)[0].(map[string]any)["slot"] = "terminal"
			case GeneralTerminalPublicationArchiveV1:
				source.Commits[0].UsageEvent["usage"].(map[string]any)["totalTokens"] = json.Number("12")
				source.Commits[0].TerminalEvent["status"] = "failed"
				source.Commits[0].Events[0].Slot = "terminal"
			}
			if encoded(first) != firstBefore || !reflect.DeepEqual(second, canonical) {
				t.Fatal("input mutation changed a previously parsed archive")
			}
			if err := ValidateGeneralTerminalPublicationArchiveV1(second); err != nil {
				t.Fatalf("isolated parse lost its canonical authority: %v", err)
			}
		})
	}
}
