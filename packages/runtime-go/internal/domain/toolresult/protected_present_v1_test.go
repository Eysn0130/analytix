package toolresult

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
)

func protectedPresentFixtureV1() ProtectedCaptureV1 {
	description := "  exact private description  "
	empty := ""
	return ProtectedCaptureV1{Kind: "present", Status: "completed", Present: &ProtectedPresentReceiptV1{Files: []ProtectedPresentedFileV1{
		{Index: 0, Path: "a.docx", Description: &description}, {Index: 1, Path: "b.pdf", Description: &empty}, {Index: 2, Path: "a.docx"},
	}}, PresentTargets: []ProtectedPresentTargetV1{{Index: 0, Path: "/synthetic/a.docx"}, {Index: 1, Path: "/synthetic/b.pdf"}, {Index: 2, Path: "/synthetic/a.docx"}}}
}

func TestProtectedPresentReceiptOrderOptionalDescriptionAndSingleCollectorV1(t *testing.T) {
	original := protectedPresentFixtureV1()
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, original)
	CaptureProtectedToolErrorV1(ctx, ProjectProtectedOutputErrorV1([]ProtectedOutputErrorEntryV1{{Message: "separate actual failure"}}))
	*original.Present.Files[0].Description = "replacement"
	original.Present.Files[1].Path = "replacement"
	original.PresentTargets[0].Path = "/replacement"
	actual, present, outputError := ConsumeProtectedToolObservationV1(ctx)
	if !present || !reflect.DeepEqual(actual, protectedPresentFixtureV1()) || outputError == nil || outputError.Entries[0].Message != "separate actual failure" {
		t.Fatal("capture borrowed declaration/target buffers or overwrote error observation")
	}
	if _, present, outputError := ConsumeProtectedToolObservationV1(ctx); present || outputError != nil {
		t.Fatal("present observation consumed twice")
	}
	duplicate := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(duplicate, actual)
	CaptureProtectedToolResultV1(duplicate, actual)
	if _, present := ConsumeProtectedToolResultV1(duplicate); present {
		t.Fatal("duplicate producer chose a receipt")
	}
}

func TestProtectedPresentRejectsMixedKindsFailedStatusAndInvalidTargetsV1(t *testing.T) {
	for name, mutate := range map[string]func(*ProtectedCaptureV1){
		"failed":           func(c *ProtectedCaptureV1) { c.Status = "failed" },
		"mixed":            func(c *ProtectedCaptureV1) { c.Kind = "read" },
		"body":             func(c *ProtectedCaptureV1) { c.Body = "copied file contents" },
		"index":            func(c *ProtectedCaptureV1) { c.Present.Files[1].Index = 0 },
		"target_index":     func(c *ProtectedCaptureV1) { c.PresentTargets[1].Index = 0 },
		"relative_target":  func(c *ProtectedCaptureV1) { c.PresentTargets[0].Path = "relative.docx" },
		"target_count":     func(c *ProtectedCaptureV1) { c.PresentTargets = c.PresentTargets[:1] },
		"empty":            func(c *ProtectedCaptureV1) { c.Present.Files = nil },
		"empty_path":       func(c *ProtectedCaptureV1) { c.Present.Files[0].Path = "  " },
		"nul":              func(c *ProtectedCaptureV1) { c.Present.Files[0].Path = "a\x00b" },
		"long_description": func(c *ProtectedCaptureV1) { text := strings.Repeat("a", 4097); c.Present.Files[0].Description = &text },
	} {
		t.Run(name, func(t *testing.T) {
			capture := protectedPresentFixtureV1()
			mutate(&capture)
			if ValidateProtectedCaptureV1(capture) == nil {
				t.Fatal("invalid present observation admitted")
			}
		})
	}
}

func TestProtectedPresentStrictSnapshotIdentityAndNoContentDigestV1(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	snapshot := ProtectedSnapshotV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, Principal: principal,
		Workspace: "/synthetic", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: ToolResultItemIDV1("turn-a", callID), ToolName: "present",
		ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64),
		Capture: protectedPresentFixtureV1(), Inspect: &ProtectedInspectV1{InputJSON: `{"files":[{"path":"a.docx"}]}`}}
	body, err := ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProtectedSnapshotV1(body)
	if err != nil || !reflect.DeepEqual(parsed, snapshot) || bytes.Contains(body, []byte("contentDigest")) {
		t.Fatal("receipt changed identity or acquired frozen preview semantics")
	}
	for _, invalid := range [][]byte{
		bytes.Replace(body, []byte(`"index":0`), []byte(`"index":0,"seq":10`), 1),
		bytes.Replace(body, []byte(`"index":0`), []byte(`"index":0,"index":0`), 1),
		bytes.Replace(body, []byte(`"index":0,`), nil, 1),
	} {
		if _, err := ParseProtectedSnapshotV1(invalid); err == nil {
			t.Fatal("open or noncanonical present bytes admitted")
		}
	}
	snapshot.Capture.PresentTargets = nil
	if ValidateProtectedSnapshotV1(snapshot) == nil {
		t.Fatal("unobserved targets entered private snapshot")
	}
	snapshot.Capture = protectedPresentFixtureV1()
	snapshot.ToolName = "read"
	if ValidateProtectedSnapshotV1(snapshot) == nil {
		t.Fatal("receipt assigned to another tool")
	}
}

func TestProtectedPresentUTF8ByteBoundsV1(t *testing.T) {
	for _, field := range []string{"path", "description"} {
		for _, size := range []int{4096, 4097} {
			capture := protectedPresentFixtureV1()
			value := strings.Repeat("中", 1365) + strings.Repeat("a", size-4095)
			if field == "path" {
				capture.Present.Files[0].Path = value
			} else {
				capture.Present.Files[0].Description = &value
			}
			if valid := ValidateProtectedCaptureV1(capture) == nil; valid != (size == 4096) {
				t.Fatal("UTF8 bound used characters rather than bytes", field, size)
			}
		}
	}
}
