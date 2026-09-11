//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentsnapshot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const (
	commandDeadlineV1 = 60 * time.Second
	rejectDeadlineV1  = time.Second
	responseLimitV1   = 64 * 1024
)

type rejectionV1 struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	Operation     string `json:"operation"`
	RequestNonce  string `json:"request_nonce"`
	Blocker       string `json:"blocker"`
}

func Main() int {
	return mainV1(false)
}

func MainDiscard() int {
	return mainV1(true)
}

func mainV1(discardOnly bool) int {
	expectedArgument := ArgumentV1
	if discardOnly {
		expectedArgument = ArgumentDiscardV1
	}
	if len(os.Args) != 2 || os.Args[1] != expectedArgument {
		return 125
	}
	output, err := openCommandPipeV1(1, "analytix-native-source-snapshot-stdout")
	if err != nil {
		return 1
	}
	defer output.Close()
	input, err := openCommandPipeV1(0, "analytix-native-source-snapshot-stdin")
	if err != nil {
		return rejectV1(output, RequestV1{}, "request_frame_invalid")
	}
	defer input.Close()

	ctx, cancel := context.WithTimeout(context.Background(), commandDeadlineV1)
	defer cancel()
	frame, err := readRequestFrameV1(ctx, input)
	if err != nil {
		return rejectV1(output, RequestV1{}, "request_frame_invalid")
	}
	request, err := DecodeRequestV1(frame)
	if err != nil {
		return rejectV1(output, RequestV1{}, "request_invalid")
	}
	if discardOnly {
		return runDiscardMainV1(ctx, output, request)
	}
	if request.Operation != OperationCreateV1 && request.Operation != OperationVerifyV1 {
		return rejectV1(output, request, "request_invalid")
	}
	manifest, err := duplicateCommandFileV1(3, "analytix-native-source-snapshot-manifest")
	if err != nil {
		return rejectV1(output, request, "descriptor_input_invalid")
	}
	defer manifest.Close()
	repository, err := duplicateCommandFileV1(4, "analytix-native-source-snapshot-repository")
	if err != nil {
		return rejectV1(output, request, "descriptor_input_invalid")
	}
	defer repository.Close()
	parent, err := duplicateCommandFileV1(5, "analytix-native-source-snapshot-parent")
	if err != nil {
		return rejectV1(output, request, "descriptor_input_invalid")
	}
	defer parent.Close()
	container, err := duplicateCommandFileV1(6, "analytix-native-source-snapshot-container")
	if err != nil {
		return rejectV1(output, request, "descriptor_input_invalid")
	}
	defer container.Close()
	if err := validateSessionParentBindingV1(container, parent, request.SessionName); err != nil {
		return rejectV1(output, request, "descriptor_input_invalid")
	}

	var response any
	switch request.Operation {
	case OperationCreateV1:
		response, err = CreateFromDescriptorsV1(ctx, request, manifest, repository, parent)
	case OperationVerifyV1:
		response, err = VerifyFromDescriptorsV1(ctx, request, manifest, repository, parent)
	default:
		err = ErrInvalidInput
	}
	if err == nil {
		err = validateSessionParentBindingV1(container, parent, request.SessionName)
	}
	if err != nil || ctx.Err() != nil {
		return rejectV1(output, request, blockerV1(err, ctx.Err()))
	}
	body, err := json.Marshal(response)
	if err != nil || len(body)+1 > responseLimitV1 || writeResponseV1(ctx, output, append(body, '\n')) != nil {
		return 1
	}
	return 0
}

func runDiscardMainV1(ctx context.Context, output *os.File, request RequestV1) int {
	if request.Operation != OperationDiscardV1 && request.Operation != OperationReconcileDiscardV1 {
		return rejectV1(output, request, "request_invalid")
	}
	parent, err := duplicateCommandFileV1(3, "analytix-native-source-snapshot-parent")
	if err != nil {
		return rejectV1(output, request, "descriptor_input_invalid")
	}
	defer parent.Close()
	container, err := duplicateCommandFileV1(4, "analytix-native-source-snapshot-container")
	if err != nil {
		return rejectV1(output, request, "descriptor_input_invalid")
	}
	defer container.Close()

	detached := false
	if request.Operation == OperationReconcileDiscardV1 {
		detached, err = validateReconcileSessionParentV1(container, parent, request.SessionName)
	} else {
		err = validateSessionParentBindingV1(container, parent, request.SessionName)
	}
	var response DiscardResponseV1
	if err == nil && detached {
		response = DiscardResponseV1{
			Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "discarded",
			Operation: request.Operation, RequestNonce: request.RequestNonce, SessionName: request.SessionName,
			Disposition: DispositionAlreadyDiscardedV1, SessionRemoved: true,
		}
	} else if err == nil {
		if request.Operation == OperationDiscardV1 {
			response, err = DiscardFromDescriptorsV1(ctx, request, parent)
		} else {
			response, err = ReconcileDiscardFromDescriptorsV1(ctx, request, parent)
		}
		if err == nil {
			err = destroySessionParentV1(container, parent, request.SessionName)
			if err == nil {
				response.SessionRemoved = true
			}
		}
	}
	if err != nil || ctx.Err() != nil {
		return rejectV1(output, request, blockerV1(errors.Join(ErrDestruction, err), ctx.Err()))
	}
	body, err := json.Marshal(response)
	if err != nil || len(body)+1 > responseLimitV1 || writeResponseV1(ctx, output, append(body, '\n')) != nil {
		return 1
	}
	return 0
}

func blockerV1(err, contextErr error) string {
	if contextErr != nil {
		return "snapshot_deadline_exceeded"
	}
	switch {
	case errors.Is(err, ErrGeneration):
		return "snapshot_generation_mismatch"
	case errors.Is(err, ErrDestruction):
		return "snapshot_destroy_failed"
	case errors.Is(err, ErrSourceChanged):
		return "snapshot_source_changed"
	case errors.Is(err, ErrPublication):
		return "snapshot_publication_failed"
	case errors.Is(err, ErrInvalidInput):
		return "snapshot_input_invalid"
	default:
		return "snapshot_failed"
	}
}

func rejectV1(output *os.File, request RequestV1, blocker string) int {
	ctx, cancel := context.WithTimeout(context.Background(), rejectDeadlineV1)
	defer cancel()
	body, err := json.Marshal(rejectionV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "rejected",
		Operation: request.Operation, RequestNonce: request.RequestNonce, Blocker: blocker,
	})
	if err == nil && len(body)+1 <= responseLimitV1 {
		_ = writeResponseV1(ctx, output, append(body, '\n'))
	}
	return 1
}

func openCommandPipeV1(descriptor int, name string) (*os.File, error) {
	if descriptor < 0 || name == "" {
		return nil, ErrInvalidInput
	}
	duplicate, err := unix.FcntlInt(uintptr(descriptor), unix.F_DUPFD_CLOEXEC, 8)
	if err != nil || duplicate < 8 {
		if duplicate >= 0 {
			_ = unix.Close(duplicate)
		}
		return nil, ErrInvalidInput
	}
	var stat unix.Stat_t
	if unix.Fstat(duplicate, &stat) != nil {
		_ = unix.Close(duplicate)
		return nil, ErrInvalidInput
	}
	kind := stat.Mode & unix.S_IFMT
	if (kind != unix.S_IFIFO && kind != unix.S_IFSOCK) || unix.SetNonblock(duplicate, true) != nil {
		_ = unix.Close(duplicate)
		return nil, ErrInvalidInput
	}
	file := os.NewFile(uintptr(duplicate), name)
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, ErrInvalidInput
	}
	return file, nil
}

func duplicateCommandFileV1(fd int, name string) (*os.File, error) {
	duplicate, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 8)
	if err != nil || duplicate < 8 {
		if duplicate >= 0 {
			_ = unix.Close(duplicate)
		}
		return nil, ErrInvalidInput
	}
	file := os.NewFile(uintptr(duplicate), name)
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, ErrInvalidInput
	}
	return file, nil
}

func readRequestFrameV1(ctx context.Context, input *os.File) ([]byte, error) {
	if ctx == nil || input == nil || ctx.Err() != nil {
		return nil, ErrInvalidInput
	}
	deadline, ok := ctx.Deadline()
	if !ok || input.SetReadDeadline(deadline) != nil {
		return nil, ErrInvalidInput
	}
	stop := context.AfterFunc(ctx, func() { _ = input.SetReadDeadline(time.Now()) })
	defer func() {
		stop()
		_ = input.SetReadDeadline(time.Time{})
	}()
	buffered := bufio.NewReaderSize(input, requestLimitV1+2)
	raw, err := buffered.ReadSlice('\n')
	if err != nil || len(raw) <= 1 || len(raw) > requestLimitV1+1 ||
		bytes.IndexByte(raw[:len(raw)-1], '\n') >= 0 || bytes.IndexByte(raw, '\r') >= 0 || bytes.IndexByte(raw, 0) >= 0 {
		return nil, ErrInvalidInput
	}
	if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidInput
	}
	return append([]byte(nil), raw[:len(raw)-1]...), nil
}

func writeResponseV1(ctx context.Context, output *os.File, body []byte) error {
	if ctx == nil || output == nil || ctx.Err() != nil || len(body) <= 1 || len(body) > responseLimitV1 || body[len(body)-1] != '\n' {
		return ErrInvalidInput
	}
	deadline, ok := ctx.Deadline()
	if !ok || output.SetWriteDeadline(deadline) != nil {
		return ErrInvalidInput
	}
	stop := context.AfterFunc(ctx, func() { _ = output.SetWriteDeadline(time.Now()) })
	defer func() {
		stop()
		_ = output.SetWriteDeadline(time.Time{})
	}()
	for len(body) > 0 {
		written, err := output.Write(body)
		if err != nil || written <= 0 || written > len(body) {
			return errors.Join(ErrInvalidInput, err)
		}
		body = body[written:]
	}
	return nil
}
