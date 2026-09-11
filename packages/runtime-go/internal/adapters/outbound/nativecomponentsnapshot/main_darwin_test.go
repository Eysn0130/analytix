//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentsnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

const snapshotCommandHelperEnvironmentV1 = "ANALYTIX_TEST_NATIVE_SOURCE_SNAPSHOT_HELPER"

func TestSnapshotCommandHelperV1(t *testing.T) {
	mode := os.Getenv(snapshotCommandHelperEnvironmentV1)
	if mode == "" {
		return
	}
	if mode == "discard" {
		os.Args = []string{os.Args[0], ArgumentDiscardV1}
		os.Exit(MainDiscard())
	}
	os.Args = []string{os.Args[0], ArgumentV1}
	os.Exit(Main())
}

func TestSnapshotCommandCreateVerifyAndRejectFrames(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	containerPath := filepath.Join(t.TempDir(), "container")
	mustMkdirV1(t, containerPath, 0o700)
	request := createRequestV1()
	parentPath := filepath.Join(containerPath, request.SessionName)
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()
	container := mustOpenV1(t, containerPath)
	defer container.Close()
	if err := validateSessionParentBindingV1(container, parent, request.SessionName); err != nil {
		parentInfo, _ := parent.Stat()
		containerInfo, _ := container.Stat()
		t.Fatalf("session binding parent=%v container=%v err=%v", parentInfo, containerInfo, err)
	}
	descriptors := []*os.File{manifest, repositoryFile, parent, container}

	created := runSnapshotCommandV1(t, request, descriptors)
	if created.exitCode != 0 || len(created.stderr) != 0 {
		t.Fatalf("create exit=%d stdout=%s stderr=%s", created.exitCode, created.stdout, created.stderr)
	}
	var createResponse ResponseV1
	decodeSnapshotCommandFrameV1(t, created.stdout, &createResponse)
	if createResponse.Status != "committed" || createResponse.Operation != OperationCreateV1 ||
		createResponse.RequestNonce != request.RequestNonce || createResponse.SessionName != request.SessionName {
		t.Fatalf("create response=%#v", createResponse)
	}

	verified := runSnapshotCommandV1(t, verifyRequestForResponseV1(createResponse), descriptors)
	if verified.exitCode != 0 || len(verified.stderr) != 0 {
		t.Fatalf("verify exit=%d stdout=%s stderr=%s", verified.exitCode, verified.stdout, verified.stderr)
	}
	var verifyResponse ResponseV1
	decodeSnapshotCommandFrameV1(t, verified.stdout, &verifyResponse)
	if verifyResponse.Status != "verified" || verifyResponse.GenerationID != createResponse.GenerationID ||
		verifyResponse.InventorySHA256 != createResponse.InventorySHA256 ||
		verifyResponse.GenerationReceiptSHA256 != createResponse.GenerationReceiptSHA256 {
		t.Fatalf("verify response=%#v", verifyResponse)
	}
	discarded := runSnapshotDiscardCommandV1(
		t, discardRequestV1(createResponse), []*os.File{parent, container},
	)
	if discarded.exitCode != 0 || len(discarded.stderr) != 0 {
		t.Fatalf("discard exit=%d stdout=%s stderr=%s", discarded.exitCode, discarded.stdout, discarded.stderr)
	}
	var discardResponse DiscardResponseV1
	decodeSnapshotCommandFrameV1(t, discarded.stdout, &discardResponse)
	if discardResponse.Status != "discarded" || discardResponse.GenerationID != createResponse.GenerationID ||
		discardResponse.InventorySHA256 != createResponse.InventorySHA256 ||
		discardResponse.GenerationReceiptSHA256 != createResponse.GenerationReceiptSHA256 ||
		discardResponse.SessionName != request.SessionName ||
		discardResponse.Disposition != DispositionGenerationDiscardedV1 || !discardResponse.SessionRemoved {
		t.Fatalf("discard response=%#v", discardResponse)
	}
	if _, err := os.Lstat(parentPath); !os.IsNotExist(err) {
		t.Fatalf("discarded session parent survived: %v", err)
	}
	reconciled := runSnapshotDiscardCommandV1(
		t, reconcileDiscardRequestV1(), []*os.File{parent, container},
	)
	if reconciled.exitCode != 0 || len(reconciled.stderr) != 0 {
		parentInfo, parentErr := parent.Stat()
		containerInfo, containerErr := container.Stat()
		t.Fatalf(
			"reconcile exit=%d stdout=%s stderr=%s parent=%#v parentErr=%v container=%#v containerErr=%v",
			reconciled.exitCode, reconciled.stdout, reconciled.stderr,
			parentInfo, parentErr, containerInfo, containerErr,
		)
	}
	var reconcileResponse DiscardResponseV1
	decodeSnapshotCommandFrameV1(t, reconciled.stdout, &reconcileResponse)
	if reconcileResponse.Status != "discarded" ||
		reconcileResponse.Disposition != DispositionAlreadyDiscardedV1 ||
		!reconcileResponse.SessionRemoved || reconcileResponse.GenerationID != "" ||
		reconcileResponse.InventorySHA256 != "" || reconcileResponse.GenerationReceiptSHA256 != "" {
		t.Fatalf("reconcile response=%#v", reconcileResponse)
	}
	mustMkdirV1(t, parentPath, 0o700)
	replaced := runSnapshotDiscardCommandV1(
		t, reconcileDiscardRequestV1(), []*os.File{parent, container},
	)
	if replaced.exitCode == 0 || len(replaced.stderr) != 0 {
		t.Fatalf("replacement accepted exit=%d stdout=%s stderr=%s", replaced.exitCode, replaced.stdout, replaced.stderr)
	}
	var replacementRejection map[string]any
	decodeSnapshotCommandFrameV1(t, replaced.stdout, &replacementRejection)
	if replacementRejection["blocker"] != "snapshot_destroy_failed" {
		t.Fatalf("replacement rejection=%#v", replacementRejection)
	}
	assertDirectoryNamesV1(t, parentPath)

	invalidContainerPath := filepath.Join(t.TempDir(), "container")
	mustMkdirV1(t, invalidContainerPath, 0o700)
	invalidParentPath := filepath.Join(invalidContainerPath, request.SessionName)
	mustMkdirV1(t, invalidParentPath, 0o700)
	invalidParent := mustOpenV1(t, invalidParentPath)
	defer invalidParent.Close()
	invalidContainer := mustOpenV1(t, invalidContainerPath)
	defer invalidContainer.Close()
	invalid := runSnapshotCommandRawV1(
		t, []byte("{}\n{}\n"), []*os.File{manifest, repositoryFile, invalidParent, invalidContainer},
	)
	if invalid.exitCode == 0 || len(invalid.stderr) != 0 {
		t.Fatalf("invalid exit=%d stdout=%s stderr=%s", invalid.exitCode, invalid.stdout, invalid.stderr)
	}
	var rejection map[string]any
	decodeSnapshotCommandFrameV1(t, invalid.stdout, &rejection)
	keys := make([]string, 0, len(rejection))
	for key := range rejection {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	wantKeys := []string{"blocker", "kind", "operation", "request_nonce", "schema_version", "status"}
	if !reflect.DeepEqual(keys, wantKeys) || rejection["blocker"] != "request_frame_invalid" ||
		rejection["operation"] != "" || rejection["request_nonce"] != "" {
		t.Fatalf("rejection=%#v", rejection)
	}
	assertDirectoryNamesV1(t, invalidParentPath)

	missingDescriptors := runSnapshotCommandV1(t, createRequestV1(), nil)
	if missingDescriptors.exitCode == 0 {
		t.Fatalf("missing descriptors accepted: %s", missingDescriptors.stdout)
	}
	decodeSnapshotCommandFrameV1(t, missingDescriptors.stdout, &rejection)
	if rejection["blocker"] != "descriptor_input_invalid" ||
		rejection["operation"] != OperationCreateV1 || rejection["request_nonce"] != createRequestV1().RequestNonce {
		t.Fatalf("descriptor rejection=%#v", rejection)
	}
}

type snapshotCommandResultV1 struct {
	exitCode int
	stdout   []byte
	stderr   []byte
}

func runSnapshotCommandV1(t *testing.T, request RequestV1, descriptors []*os.File) snapshotCommandResultV1 {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return runSnapshotCommandRawModeV1(t, append(body, '\n'), descriptors, "snapshot")
}

func runSnapshotDiscardCommandV1(t *testing.T, request RequestV1, descriptors []*os.File) snapshotCommandResultV1 {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return runSnapshotCommandRawModeV1(t, append(body, '\n'), descriptors, "discard")
}

func runSnapshotCommandRawV1(t *testing.T, input []byte, descriptors []*os.File) snapshotCommandResultV1 {
	return runSnapshotCommandRawModeV1(t, input, descriptors, "snapshot")
}

func runSnapshotCommandRawModeV1(
	t *testing.T,
	input []byte,
	descriptors []*os.File,
	mode string,
) snapshotCommandResultV1 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestSnapshotCommandHelperV1")
	command.Env = append(os.Environ(), snapshotCommandHelperEnvironmentV1+"="+mode)
	command.Stdin = bytes.NewReader(input)
	command.ExtraFiles = descriptors
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	return snapshotCommandResultV1{
		exitCode: exitCode,
		stdout:   append([]byte(nil), stdout.Bytes()...),
		stderr:   append([]byte(nil), stderr.Bytes()...),
	}
}

func decodeSnapshotCommandFrameV1(t *testing.T, frame []byte, destination any) {
	t.Helper()
	if len(frame) <= 1 || frame[len(frame)-1] != '\n' || bytes.Contains(frame[:len(frame)-1], []byte{'\n'}) {
		t.Fatalf("invalid response frame: %q", frame)
	}
	if err := json.Unmarshal(frame[:len(frame)-1], destination); err != nil {
		t.Fatal(err)
	}
}
