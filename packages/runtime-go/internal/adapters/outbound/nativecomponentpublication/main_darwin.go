//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentpublication

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
	PublisherArgumentV1      = "--analytix-native-generation-publisher-v1"
	publisherRequestLimit    = 256 * 1024
	publisherResponseLimit   = 16 * 1024
	publisherCommandDeadline = 60 * time.Second
	publisherRejectDeadline  = time.Second
)

type publishRejectionV1 struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	RequestNonce  string `json:"request_nonce"`
	Blocker       string `json:"blocker"`
}

func Main() int {
	if len(os.Args) != 2 || os.Args[1] != PublisherArgumentV1 {
		return 125
	}
	output, err := openPublisherPipeV1(1, "analytix-native-generation-stdout")
	if err != nil {
		return 1
	}
	defer output.Close()
	input, err := openPublisherPipeV1(0, "analytix-native-generation-stdin")
	if err != nil {
		return rejectPublisherV1(output, "", "request_frame_invalid")
	}
	defer input.Close()
	ctx, cancel := context.WithTimeout(context.Background(), publisherCommandDeadline)
	defer cancel()
	requestBody, err := readPublisherFrameV1(ctx, input)
	if err != nil {
		return rejectPublisherV1(output, "", "request_frame_invalid")
	}
	request, err := DecodePublishRequestV1(requestBody)
	if err != nil {
		return rejectPublisherV1(output, "", "request_invalid")
	}
	// The standalone process has no in-memory Cargo execution permit. Raw JSON
	// and inherited descriptors are intentionally insufficient authority; only
	// the future same-process Go coordinator may call PublishFromDescriptorsV1.
	return rejectPublisherV1(output, request.RequestNonce, "cargo_execution_ineligible")
}

func publicationBlockerV1(err error, contextErr error) string {
	if contextErr != nil {
		return "publication_deadline_exceeded"
	}
	switch {
	case errors.Is(err, ErrCargoExecution):
		return "cargo_execution_ineligible"
	case errors.Is(err, ErrInvalidRequest):
		return "publication_request_invalid"
	case errors.Is(err, ErrProbe):
		return "publication_probe_failed"
	case errors.Is(err, ErrInvalidInput):
		return "publication_input_invalid"
	case errors.Is(err, ErrPublication):
		return "publication_commit_failed"
	default:
		return "publication_failed"
	}
}

func rejectPublisherV1(output *os.File, requestNonce string, blocker string) int {
	ctx, cancel := context.WithTimeout(context.Background(), publisherRejectDeadline)
	defer cancel()
	body, err := json.Marshal(publishRejectionV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "rejected",
		RequestNonce: requestNonce, Blocker: blocker,
	})
	if err == nil && len(body)+1 <= publisherResponseLimit {
		_ = writePublisherFrameV1(ctx, output, append(body, '\n'))
	}
	return 1
}

func openPublisherPipeV1(descriptor int, name string) (*os.File, error) {
	if descriptor < 0 || name == "" {
		return nil, ErrInvalidInput
	}
	duplicated, err := unix.FcntlInt(uintptr(descriptor), unix.F_DUPFD_CLOEXEC, 8)
	if err != nil || duplicated < 8 {
		if err == nil && duplicated >= 0 {
			_ = unix.Close(duplicated)
		}
		return nil, ErrInvalidInput
	}
	var stat unix.Stat_t
	if unix.Fstat(duplicated, &stat) != nil {
		_ = unix.Close(duplicated)
		return nil, ErrInvalidInput
	}
	kind := stat.Mode & unix.S_IFMT
	if (kind != unix.S_IFIFO && kind != unix.S_IFSOCK) || unix.SetNonblock(duplicated, true) != nil {
		_ = unix.Close(duplicated)
		return nil, ErrInvalidInput
	}
	file := os.NewFile(uintptr(duplicated), name)
	if file == nil {
		_ = unix.Close(duplicated)
		return nil, ErrInvalidInput
	}
	return file, nil
}

func inheritedInputsV1() (DescriptorInputsV1, func(), error) {
	files := make([]*os.File, 0, len(frozenComponentsV1)+1)
	for descriptor := 3; descriptor < 4+len(frozenComponentsV1); descriptor++ {
		duplicated, err := unix.FcntlInt(uintptr(descriptor), unix.F_DUPFD_CLOEXEC, 8)
		if err != nil || duplicated < 8 {
			if err == nil && duplicated >= 0 {
				_ = unix.Close(duplicated)
			}
			for _, file := range files {
				_ = file.Close()
			}
			return DescriptorInputsV1{}, func() {}, ErrInvalidInput
		}
		file := os.NewFile(uintptr(duplicated), "analytix-native-generation-input")
		if file == nil {
			_ = unix.Close(duplicated)
			for _, opened := range files {
				_ = opened.Close()
			}
			return DescriptorInputsV1{}, func() {}, ErrInvalidInput
		}
		files = append(files, file)
	}
	closeInputs := func() {
		for _, file := range files {
			_ = file.Close()
		}
	}
	return DescriptorInputsV1{Manifest: files[0], Components: files[1:]}, closeInputs, nil
}

func readPublisherFrameV1(ctx context.Context, input *os.File) ([]byte, error) {
	if ctx == nil || input == nil || ctx.Err() != nil {
		return nil, ErrInvalidRequest
	}
	if err := unix.SetNonblock(int(input.Fd()), true); err != nil {
		return nil, ErrInvalidRequest
	}
	deadline, ok := ctx.Deadline()
	if !ok || input.SetReadDeadline(deadline) != nil {
		return nil, ErrInvalidRequest
	}
	defer input.SetReadDeadline(time.Time{})
	reader := bufio.NewReaderSize(input, publisherRequestLimit+1)
	raw, err := reader.ReadSlice('\n')
	if err != nil || len(raw) <= 1 || len(raw) > publisherRequestLimit ||
		bytes.Contains(raw[:len(raw)-1], []byte{'\n'}) || bytes.ContainsAny(raw, "\r\x00") {
		return nil, ErrInvalidRequest
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidRequest
	}
	return append([]byte(nil), raw[:len(raw)-1]...), nil
}

func writePublisherFrameV1(ctx context.Context, output *os.File, body []byte) error {
	if ctx == nil || output == nil || ctx.Err() != nil || len(body) <= 1 || len(body) > publisherResponseLimit || body[len(body)-1] != '\n' {
		return ErrPublication
	}
	if err := unix.SetNonblock(int(output.Fd()), true); err != nil {
		return ErrPublication
	}
	deadline, ok := ctx.Deadline()
	if !ok || output.SetWriteDeadline(deadline) != nil {
		return ErrPublication
	}
	defer output.SetWriteDeadline(time.Time{})
	for len(body) > 0 {
		written, err := output.Write(body)
		if err != nil || written <= 0 || written > len(body) {
			return errors.Join(ErrPublication, err)
		}
		body = body[written:]
	}
	return nil
}
