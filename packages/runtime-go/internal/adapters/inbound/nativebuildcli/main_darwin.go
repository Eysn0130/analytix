//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativebuildcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	appnativebuild "analytix.local/runtime-go/internal/app/nativebuild"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	portnativebuild "analytix.local/runtime-go/internal/ports/nativebuild"

	"golang.org/x/sys/unix"
)

const (
	ArgumentV1        = "--analytix-native-build-coordinator-v1"
	RequestKindV1     = "analytix_native_build_coordinator_request"
	ResponseKindV1    = "analytix_native_build_coordinator_response"
	SchemaVersionV1   = 1
	requestLimitV1    = 8 * 1024
	responseLimitV1   = 16 * 1024
	commandDeadlineV1 = 30 * time.Minute
)

type RequestV1 struct {
	Kind            string `json:"kind"`
	SchemaVersion   int    `json:"schema_version"`
	RequestNonce    string `json:"request_nonce"`
	RepositoryRoot  string `json:"repository_root"`
	PublicationRoot string `json:"publication_root"`
	TargetKey       string `json:"target_key"`
}

type ResponseV1 struct {
	Kind                     string `json:"kind"`
	SchemaVersion            int    `json:"schema_version"`
	Status                   string `json:"status"`
	RequestNonce             string `json:"request_nonce"`
	Blocker                  string `json:"blocker"`
	GenerationID             string `json:"generation_id"`
	InventorySHA256          string `json:"inventory_sha256"`
	GenerationReceiptSHA256  string `json:"generation_receipt_sha256"`
	ComponentReceiptSHA256   string `json:"component_receipt_sha256"`
	PublicationBindingSHA256 string `json:"publication_binding_sha256"`
}

type HostOpenerV1 func(RequestV1) (portnativebuild.Host, error)

func Main(openHost HostOpenerV1) int {
	if len(os.Args) != 2 || os.Args[1] != ArgumentV1 {
		return 125
	}
	output, err := openPipeV1(1, "analytix-native-build-coordinator-stdout")
	if err != nil {
		return 1
	}
	defer output.Close()
	input, err := openPipeV1(0, "analytix-native-build-coordinator-stdin")
	if err != nil {
		return writeRejectionV1(output, "", "request_frame_invalid")
	}
	defer input.Close()

	ctx, cancel := context.WithTimeout(context.Background(), commandDeadlineV1)
	defer cancel()
	frame, err := readFrameV1(ctx, input)
	if err != nil {
		return writeRejectionV1(output, "", "request_frame_invalid")
	}
	request, err := DecodeRequestV1(frame)
	if err != nil {
		return writeRejectionV1(output, "", "request_invalid")
	}
	response, err := RunV1(ctx, request, openHost)
	if err != nil {
		return writeRejectionV1(output, request.RequestNonce, "coordinator_failed")
	}
	if err := writeResponseV1(ctx, output, response); err != nil {
		return 1
	}
	if response.Status != string(appnativebuild.StatusPublished) {
		return 1
	}
	return 0
}

func DecodeRequestV1(body []byte) (RequestV1, error) {
	var request RequestV1
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: requestLimitV1, MaxDepth: 2,
		MaxTokens: 32, MaxStringBytes: 4096, MaxNumberBytes: 8, MaxAbsExponent: 1,
	}); err != nil {
		return RequestV1{}, errors.New("native_build_coordinator_request_invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return RequestV1{}, errors.New("native_build_coordinator_request_invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return RequestV1{}, errors.New("native_build_coordinator_request_invalid")
	}
	canonical, err := json.Marshal(request)
	if err != nil || !bytes.Equal(body, canonical) || request.Kind != RequestKindV1 ||
		request.SchemaVersion != SchemaVersionV1 || !validDigestV1(request.RequestNonce) {
		return RequestV1{}, errors.New("native_build_coordinator_request_invalid")
	}
	return request, nil
}

func RunV1(ctx context.Context, request RequestV1, openHost HostOpenerV1) (ResponseV1, error) {
	if openHost == nil {
		return ResponseV1{}, errors.New("native_build_coordinator_host_invalid")
	}
	host, err := openHost(request)
	if err != nil {
		return ResponseV1{}, err
	}
	if host == nil {
		return ResponseV1{}, errors.New("native_build_coordinator_host_invalid")
	}
	coordinator, err := appnativebuild.NewCoordinator(host)
	if err != nil {
		return ResponseV1{}, err
	}
	result, err := coordinator.RunV1(ctx, appnativebuild.RequestV1{RequestNonce: request.RequestNonce})
	if err != nil {
		return ResponseV1{}, err
	}
	return ResponseV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1,
		Status: string(result.Status), RequestNonce: request.RequestNonce, Blocker: result.Blocker,
		GenerationID: result.Publication.GenerationID, InventorySHA256: result.Publication.InventorySHA256,
		GenerationReceiptSHA256:  result.Publication.GenerationReceiptSHA256,
		ComponentReceiptSHA256:   result.Publication.ComponentReceiptSHA256,
		PublicationBindingSHA256: result.Publication.PublicationBindingSHA256,
	}, nil
}

func writeRejectionV1(output *os.File, nonce, blocker string) int {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response := ResponseV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "failed",
		RequestNonce: nonce, Blocker: blocker,
	}
	_ = writeResponseV1(ctx, output, response)
	return 1
}

func writeResponseV1(ctx context.Context, output *os.File, response ResponseV1) error {
	body, err := json.Marshal(response)
	if err != nil || len(body)+1 > responseLimitV1 {
		return errors.New("native_build_coordinator_response_invalid")
	}
	return writeFrameV1(ctx, output, append(body, '\n'))
}

func openPipeV1(descriptor int, name string) (*os.File, error) {
	duplicated, err := unix.FcntlInt(uintptr(descriptor), unix.F_DUPFD_CLOEXEC, 8)
	if err != nil || duplicated < 8 {
		if err == nil && duplicated >= 0 {
			_ = unix.Close(duplicated)
		}
		return nil, errors.New("native_build_coordinator_pipe_invalid")
	}
	var stat unix.Stat_t
	if unix.Fstat(duplicated, &stat) != nil {
		_ = unix.Close(duplicated)
		return nil, errors.New("native_build_coordinator_pipe_invalid")
	}
	kind := stat.Mode & unix.S_IFMT
	if (kind != unix.S_IFIFO && kind != unix.S_IFSOCK) || unix.SetNonblock(duplicated, true) != nil {
		_ = unix.Close(duplicated)
		return nil, errors.New("native_build_coordinator_pipe_invalid")
	}
	file := os.NewFile(uintptr(duplicated), name)
	if file == nil {
		_ = unix.Close(duplicated)
		return nil, errors.New("native_build_coordinator_pipe_invalid")
	}
	return file, nil
}

func readFrameV1(ctx context.Context, input *os.File) ([]byte, error) {
	if ctx == nil || input == nil || ctx.Err() != nil {
		return nil, errors.New("native_build_coordinator_request_invalid")
	}
	deadline, ok := ctx.Deadline()
	if !ok || input.SetReadDeadline(deadline) != nil {
		return nil, errors.New("native_build_coordinator_request_invalid")
	}
	defer input.SetReadDeadline(time.Time{})
	reader := bufio.NewReaderSize(input, requestLimitV1+1)
	raw, err := reader.ReadSlice('\n')
	if err != nil || len(raw) <= 1 || len(raw) > requestLimitV1 || bytes.ContainsAny(raw, "\r\x00") {
		return nil, errors.New("native_build_coordinator_request_invalid")
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		return nil, errors.New("native_build_coordinator_request_invalid")
	}
	return append([]byte(nil), raw[:len(raw)-1]...), nil
}

func writeFrameV1(ctx context.Context, output *os.File, body []byte) error {
	if ctx == nil || output == nil || ctx.Err() != nil || len(body) <= 1 || len(body) > responseLimitV1 || body[len(body)-1] != '\n' {
		return errors.New("native_build_coordinator_response_invalid")
	}
	deadline, ok := ctx.Deadline()
	if !ok || output.SetWriteDeadline(deadline) != nil {
		return errors.New("native_build_coordinator_response_invalid")
	}
	defer output.SetWriteDeadline(time.Time{})
	for len(body) > 0 {
		written, err := output.Write(body)
		if err != nil || written <= 0 || written > len(body) {
			return errors.New("native_build_coordinator_response_invalid")
		}
		body = body[written:]
	}
	return nil
}

func validDigestV1(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}
