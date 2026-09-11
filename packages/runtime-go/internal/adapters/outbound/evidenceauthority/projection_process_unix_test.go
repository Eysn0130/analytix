//go:build darwin || linux

package evidenceauthority

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

func TestMain(m *testing.M) {
	if os.Getenv("ANALYTIX_EVIDENCE_PROJECTION_HELPER") == "1" {
		if err := runEvidenceProjectionCrossProcessHelper(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProjectionCrossProcessRejectsUnprovenGenerationSkip(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(111)
	first := fixture.firstBundle(t, "process-projection")
	leftSecond := fixture.nextBundle(t, first, "dataset", "process-left-2")
	leftThird := fixture.nextBundle(t, leftSecond, "registry", "process-left-3")
	rightSecond := fixture.nextBundle(t, first, "publication", "process-right-2")
	rightThird := fixture.nextBundle(t, rightSecond, "dataset", "process-right-3")
	parent := t.TempDir()
	root := filepath.Join(parent, "projection")
	projection, err := NewProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ProjectWitnessSelected(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	start := filepath.Join(parent, "start")
	type child struct {
		command *exec.Cmd
		ready   string
		result  string
		output  bytes.Buffer
	}
	children := make([]*child, 0, 2)
	for position, bundle := range []domainevidence.EvidenceAuthorityBundleV1{leftThird, rightThird} {
		body := canonicalEvidenceBundleBody(t, bundle)
		child := &child{
			ready:  filepath.Join(parent, "ready-"+string(rune('0'+position))),
			result: filepath.Join(parent, "result-"+string(rune('0'+position))),
		}
		child.command = exec.Command(os.Args[0], "-test.run=^$", "-test.count=1")
		child.command.Env = append(os.Environ(),
			"ANALYTIX_EVIDENCE_PROJECTION_HELPER=1",
			"ANALYTIX_EVIDENCE_PROJECTION_ROOT="+root,
			"ANALYTIX_EVIDENCE_PROJECTION_BUNDLE="+base64.RawURLEncoding.EncodeToString(body),
			"ANALYTIX_EVIDENCE_PROJECTION_START="+start,
			"ANALYTIX_EVIDENCE_PROJECTION_READY="+child.ready,
			"ANALYTIX_EVIDENCE_PROJECTION_RESULT="+child.result,
		)
		child.command.Stdout = &child.output
		child.command.Stderr = &child.output
		if err := child.command.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
	}
	for _, child := range children {
		waitForEvidenceProjectionPath(t, child.ready)
	}
	if err := os.WriteFile(start, []byte("start"), 0o600); err != nil {
		t.Fatal(err)
	}
	results := make([]string, 0, 2)
	for _, child := range children {
		if err := child.command.Wait(); err != nil {
			t.Fatalf("projection child failed: %v\n%s", err, child.output.String())
		}
		body, err := os.ReadFile(child.result)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, string(body))
	}
	sort.Strings(results)
	if len(results) != 2 || results[0] != "conflict" || results[1] != "conflict" {
		t.Fatalf("cross-process projection outcomes=%v, want [conflict conflict]", results)
	}
	selected, err := os.ReadFile(filepath.Join(root, evidenceWitnessProjectionName))
	if err != nil {
		t.Fatal(err)
	}
	firstBody := canonicalEvidenceBundleBody(t, first)
	if !bytes.Equal(selected, firstBody) {
		t.Fatal("unproven cross-process generation skip mutated the projected authority")
	}
}

func runEvidenceProjectionCrossProcessHelper() error {
	body, err := base64.RawURLEncoding.DecodeString(os.Getenv("ANALYTIX_EVIDENCE_PROJECTION_BUNDLE"))
	if err != nil {
		return err
	}
	bundle, err := domainevidence.ParseEvidenceAuthorityBundleV1(body)
	if err != nil {
		return err
	}
	projection, err := NewProjection(os.Getenv("ANALYTIX_EVIDENCE_PROJECTION_ROOT"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(os.Getenv("ANALYTIX_EVIDENCE_PROJECTION_READY"), []byte("ready"), 0o600); err != nil {
		return err
	}
	if err := waitForEvidenceProjectionPathE(os.Getenv("ANALYTIX_EVIDENCE_PROJECTION_START")); err != nil {
		return err
	}
	err = projection.ProjectWitnessSelected(context.Background(), bundle)
	result := "success"
	if errors.Is(err, ErrProjectionConflict) {
		result = "conflict"
	} else if err != nil {
		return err
	}
	return os.WriteFile(os.Getenv("ANALYTIX_EVIDENCE_PROJECTION_RESULT"), []byte(result), 0o600)
}

func waitForEvidenceProjectionPath(t *testing.T, path string) {
	t.Helper()
	if err := waitForEvidenceProjectionPathE(path); err != nil {
		t.Fatal(err)
	}
}

func waitForEvidenceProjectionPathE(path string) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
