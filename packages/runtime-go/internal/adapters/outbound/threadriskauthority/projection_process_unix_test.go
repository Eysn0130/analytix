//go:build darwin || linux

package threadriskauthority

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

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestMain(m *testing.M) {
	if os.Getenv("ANALYTIX_THREAD_RISK_PROJECTION_HELPER") == "1" {
		if err := runThreadRiskProjectionCrossProcessHelper(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProjectionCrossProcessCASHasOneExactWinner(t *testing.T) {
	fixture := newAuthorityStoreFixture(97)
	first := fixture.firstIndex(t, "process-projection")
	left := fixture.nextIndex(t, first, "process-left")
	right := fixture.nextIndex(t, first, "process-right")
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
	for position, index := range []domainsecurity.ThreadRiskAuthorityIndexV1{left, right} {
		body := canonicalIndexBody(t, index)
		child := &child{
			ready:  filepath.Join(parent, "ready-"+string(rune('0'+position))),
			result: filepath.Join(parent, "result-"+string(rune('0'+position))),
		}
		child.command = exec.Command(os.Args[0], "-test.run=^$", "-test.count=1")
		child.command.Env = append(os.Environ(),
			"ANALYTIX_THREAD_RISK_PROJECTION_HELPER=1",
			"ANALYTIX_THREAD_RISK_PROJECTION_ROOT="+root,
			"ANALYTIX_THREAD_RISK_PROJECTION_INDEX="+base64.RawURLEncoding.EncodeToString(body),
			"ANALYTIX_THREAD_RISK_PROJECTION_START="+start,
			"ANALYTIX_THREAD_RISK_PROJECTION_READY="+child.ready,
			"ANALYTIX_THREAD_RISK_PROJECTION_RESULT="+child.result,
		)
		child.command.Stdout = &child.output
		child.command.Stderr = &child.output
		if err := child.command.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
	}
	for _, child := range children {
		waitForProjectionPath(t, child.ready)
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
	if len(results) != 2 || results[0] != "conflict" || results[1] != "success" {
		t.Fatalf("cross-process projection outcomes=%v, want [conflict success]", results)
	}
}

func runThreadRiskProjectionCrossProcessHelper() error {
	body, err := base64.RawURLEncoding.DecodeString(os.Getenv("ANALYTIX_THREAD_RISK_PROJECTION_INDEX"))
	if err != nil {
		return err
	}
	index, err := domainsecurity.ParseThreadRiskAuthorityIndexV1(body)
	if err != nil {
		return err
	}
	projection, err := NewProjection(os.Getenv("ANALYTIX_THREAD_RISK_PROJECTION_ROOT"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(os.Getenv("ANALYTIX_THREAD_RISK_PROJECTION_READY"), []byte("ready"), 0o600); err != nil {
		return err
	}
	if err := waitForProjectionPathE(os.Getenv("ANALYTIX_THREAD_RISK_PROJECTION_START")); err != nil {
		return err
	}
	err = projection.ProjectWitnessSelected(context.Background(), index)
	result := "success"
	if errors.Is(err, ErrProjectionConflict) {
		result = "conflict"
	} else if err != nil {
		return err
	}
	return os.WriteFile(os.Getenv("ANALYTIX_THREAD_RISK_PROJECTION_RESULT"), []byte(result), 0o600)
}

func waitForProjectionPath(t *testing.T, path string) {
	t.Helper()
	if err := waitForProjectionPathE(path); err != nil {
		t.Fatal(err)
	}
}

func waitForProjectionPathE(path string) error {
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
