package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	"analytix.local/runtime-go/internal/app/officeediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func TestDocumentsStorageDoesNotRequireLibreOfficeAssets(t *testing.T) {
	ctx := context.Background()
	principal, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{DataDir: t.TempDir(), DevelopmentPluginSourceRoot: root}
	adapters := newOfficeEditingAdapters(ctx, config, editingTestIdentity{principal}, nil)
	documents := adapters["analytix-documents"]
	if documents == nil {
		t.Fatal("DOCX storage was gated on missing LibreOffice assets")
	}
	if adapters["analytix-spreadsheets"] != nil || adapters["analytix-presentations"] != nil {
		t.Fatal("native Office became available without its assets")
	}
	ready, err := documents.Readiness(ctx, adapterport.Binding{PackageID: "analytix-documents"})
	if err != nil || !ready.Available {
		t.Fatalf("storage not ready: %v", err)
	}
	for _, op := range ready.Operations {
		if op == "create-object" {
			t.Fatal("create exposed without codec/projector")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	ready, _ = documents.Readiness(cancelled, adapterport.Binding{PackageID: "analytix-documents"})
	if ready.Available {
		t.Fatal("cancelled authority accepted")
	}
}

// Error text must never be evaluated by the classifier or fixed encoder.
type manualPhasePrivateErrorV1 struct{}

func (manualPhasePrivateErrorV1) Error() string { panic("private_error_text_evaluated") }

type manualPhaseWriterV1 struct {
	bytes.Buffer
	writes int
	err    error
}

func (w *manualPhaseWriterV1) Write(frame []byte) (int, error) {
	w.writes++
	if w.err != nil {
		return 0, w.err
	}
	return w.Buffer.Write(frame)
}

func TestNativeManualBeginErrorKindsKeepPrivateErrorsOpaque(t *testing.T) {
	for _, test := range []struct {
		cause error
		want  string
	}{
		{fmt.Errorf("private: %w", context.Canceled), "context_canceled"},
		{fmt.Errorf("SYNTH_PRIVATE_MANUAL_TRACE /private/example token=hidden: %w", os.ErrPermission), "permission"},
		{managededitingapp.ErrCaptureFailed, "baseline_failed"},
		{errors.New("SYNTH_PRIVATE_MANUAL_TRACE /private/example token=hidden"), "other"},
		{manualPhasePrivateErrorV1{}, "other"},
	} {
		if got := nativeManualBeginErrorKindV1(test.cause); got != test.want {
			t.Fatalf("diagnostic_error_classification_changed: got %s want %s", got, test.want)
		}
	}
}

func TestNativeManualBeginPhaseEncoderIsClosedBoundedAndSingleWrite(t *testing.T) {
	valid := nativeManualBeginPhaseV1{
		NativeManualBeginPhaseV1: officeediting.NativeManualBeginPhaseV1{
			Version: 1, Phase: "prepare", ManualServicePresent: true, CaptureBound: true,
			CaptureEntered: true, BaselineEntered: true, BaselineOpenOK: true,
			SessionMatch: true, RevisionMatch: true, CaptureAcquired: true, PrepareEntered: true,
		},
		ErrorKind: nativeManualBeginErrorKindV1(manualPhasePrivateErrorV1{}),
	}
	writer := &manualPhaseWriterV1{}
	emitNativeManualBeginPhaseV1(writer, valid)
	frame := writer.Bytes()
	if writer.writes != 1 || len(frame) > nativeManualBeginPhaseMaxBytesV1 || bytes.Count(frame, []byte("\n")) != 1 || !bytes.HasPrefix(frame, []byte(nativeManualBeginPhasePrefixV1)) {
		t.Fatal("diagnostic_not_one_bounded_prefixed_frame")
	}
	for _, private := range []string{"SYNTH_PRIVATE", "/workspace", "/private", "token=hidden"} {
		if bytes.Contains(frame, []byte(private)) {
			t.Fatal("diagnostic_leaked_private_value")
		}
	}
	body := bytes.TrimSuffix(bytes.TrimPrefix(frame, []byte(nativeManualBeginPhasePrefixV1)), []byte("\n"))
	var got nativeManualBeginPhaseV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var fields map[string]any
	if decoder.Decode(&got) != nil || json.Unmarshal(body, &fields) != nil || got != valid || len(fields) != 14 {
		t.Fatal("diagnostic_closed_shape_changed")
	}
	for _, key := range []string{"version", "phase", "errorKind", "manualServicePresent", "captureBound", "captureEntered", "captureReused", "baselineEntered", "baselineOpenOK", "sessionMatch", "revisionMatch", "captureAcquired", "prepareEntered", "prepareOK"} {
		if _, present := fields[key]; !present {
			t.Fatal("diagnostic_required_field_missing")
		}
	}
	for _, field := range []string{"phase", "errorKind", "version"} {
		invalid := valid
		switch field {
		case "phase":
			invalid.Phase = strings.Repeat("PRIVATE", 1024)
		case "errorKind":
			invalid.ErrorKind = "PRIVATE"
		case "version":
			invalid.Version = 2
		}
		output := &manualPhaseWriterV1{}
		emitNativeManualBeginPhaseV1(output, invalid)
		if output.writes != 0 || output.Len() != 0 {
			t.Fatal("unqualified_diagnostic_value_emitted")
		}
	}
	emitNativeManualBeginPhaseV1(nil, valid)
	failed := &manualPhaseWriterV1{err: io.ErrClosedPipe}
	emitNativeManualBeginPhaseV1(failed, valid)
	if failed.writes != 1 || failed.Len() != 0 {
		t.Fatal("diagnostic_retried_failed_write")
	}
}

func TestNativeManualBeginCompositionUsesStderrAndPreservesFailure(t *testing.T) {
	ctx := context.Background()
	principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	adapters := composeOfficeEditingAdapters(Config{DataDir: t.TempDir()}, editingTestIdentity{principal}, nil, func(context.Context) bool { return true }, "docx")
	documents := adapters["analytix-documents"]
	if documents == nil {
		t.Fatal("manual_begin_composition_missing")
	}
	call := adapterport.Call{
		Binding: adapterport.Binding{PackageID: "analytix-documents"}, Principal: principal,
		ContributionID: "workspace-editor", Operation: "begin-manual-edit", Admit: func() error { return nil },
		Input: json.RawMessage(`{"threadId":"thread_main","operationId":"SYNTH_PRIVATE_MANUAL_OP_001","sessionId":"` + strings.Repeat("b", 48) + `"}`),
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal("diagnostic_pipe_unavailable")
	}
	defer reader.Close()
	defer writer.Close()
	result, invokeErr := func() (adapterport.Result, error) {
		original := os.Stderr
		os.Stderr = writer
		defer func() { os.Stderr = original }()
		return documents.Invoke(ctx, call)
	}()
	if writer.Close() != nil {
		t.Fatal("diagnostic_writer_close_failed")
	}
	frame, err := io.ReadAll(io.LimitReader(reader, nativeManualBeginPhaseMaxBytesV1+1))
	if err != nil {
		t.Fatal("diagnostic_frame_read_failed")
	}
	var out map[string]any
	if invokeErr != nil || json.Unmarshal(result.Output, &out) != nil || len(out) != 3 || out["ok"] != false || out["code"] != "invalid_request" || out["message"] != "The object operation could not be completed." {
		t.Fatal("composed_diagnostic_changed_operation_result")
	}
	if len(frame) > nativeManualBeginPhaseMaxBytesV1 || bytes.Count(frame, []byte("\n")) != 1 || !bytes.HasPrefix(frame, []byte(nativeManualBeginPhasePrefixV1)) {
		t.Fatal("composed_diagnostic_missing_or_unbounded")
	}
	for _, private := range []string{"SYNTH_PRIVATE", "thread_main", strings.Repeat("b", 48)} {
		if bytes.Contains(frame, []byte(private)) {
			t.Fatal("composed_diagnostic_leaked_request_value")
		}
	}
	body := bytes.TrimSuffix(bytes.TrimPrefix(frame, []byte(nativeManualBeginPhasePrefixV1)), []byte("\n"))
	var phase nativeManualBeginPhaseV1
	if json.Unmarshal(body, &phase) != nil || phase.Version != 1 || phase.Phase != "input" || phase.ErrorKind != "invalid_request" || !phase.ManualServicePresent || phase.CaptureBound {
		t.Fatal("composed_diagnostic_state_changed")
	}
	closedResult, closedErr := func() (adapterport.Result, error) {
		original := os.Stderr
		os.Stderr = writer
		defer func() { os.Stderr = original }()
		return documents.Invoke(ctx, call)
	}()
	if closedErr != nil || !bytes.Equal(closedResult.Output, result.Output) {
		t.Fatal("failed_diagnostic_write_changed_operation_result")
	}
}
