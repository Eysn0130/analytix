package client

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"time"
)

const providerBodyAuditTimeoutV1 = 15 * time.Second
const providerBodyAuditMaximumAckBytesV1 = 4 * 1024

type ProviderRequestBodyAuditor interface {
	AuditProviderRequestBody(context.Context, string, int, []byte) error
}

type unixProviderRequestBodyAuditorV1 struct {
	socketPath string
	timeout    time.Duration
}

type providerBodyAuditHeaderV1 struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Purpose        string `json:"purpose"`
	ProviderFamily string `json:"providerFamily"`
	Attempt        int    `json:"attempt"`
	BodyByteLength int    `json:"bodyByteLength"`
	BodySHA256     string `json:"bodySha256"`
}

type providerBodyAuditAckV1 struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Purpose        string `json:"purpose"`
	Accepted       bool   `json:"accepted"`
	ProviderFamily string `json:"providerFamily"`
	Attempt        int    `json:"attempt"`
	BodySHA256     string `json:"bodySha256"`
}

func NewUnixProviderRequestBodyAuditorV1(socketPath string) (ProviderRequestBodyAuditor, error) {
	trimmed := strings.TrimSpace(socketPath)
	if trimmed == "" || strings.ContainsRune(trimmed, '\x00') || !filepath.IsAbs(trimmed) ||
		filepath.Clean(trimmed) != trimmed {
		return nil, errors.New("provider request audit socket is invalid")
	}
	return &unixProviderRequestBodyAuditorV1{
		socketPath: trimmed,
		timeout:    providerBodyAuditTimeoutV1,
	}, nil
}

func (auditor *unixProviderRequestBodyAuditorV1) AuditProviderRequestBody(
	ctx context.Context,
	providerFamily string,
	attempt int,
	body []byte,
) error {
	if auditor == nil || strings.TrimSpace(auditor.socketPath) == "" ||
		strings.TrimSpace(providerFamily) == "" || attempt <= 0 || len(body) == 0 {
		return errors.New("provider request audit input is invalid")
	}
	digest := sha256.Sum256(body)
	bodySHA256 := hex.EncodeToString(digest[:])
	header, err := json.Marshal(providerBodyAuditHeaderV1{
		SchemaVersion:  1,
		Purpose:        "analytix.provider-request-body-audit/v1",
		ProviderFamily: strings.TrimSpace(providerFamily),
		Attempt:        attempt,
		BodyByteLength: len(body),
		BodySHA256:     bodySHA256,
	})
	if err != nil {
		return errors.New("provider request audit header is unavailable")
	}
	header = append(header, '\n')

	dialer := net.Dialer{Timeout: auditor.timeout}
	connection, err := dialer.DialContext(ctx, "unix", auditor.socketPath)
	if err != nil {
		return errors.New("provider request audit scanner is unavailable")
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(auditor.timeout)); err != nil {
		return errors.New("provider request audit scanner deadline is unavailable")
	}
	if err := writeProviderBodyAuditBytesV1(connection, header); err != nil {
		return err
	}
	if err := writeProviderBodyAuditBytesV1(connection, body); err != nil {
		return err
	}

	ackLine, err := bufio.NewReader(io.LimitReader(
		connection, providerBodyAuditMaximumAckBytesV1+1,
	)).ReadBytes('\n')
	if err != nil || len(ackLine) == 0 || len(ackLine) > providerBodyAuditMaximumAckBytesV1 {
		return errors.New("provider request audit scanner acknowledgement is invalid")
	}
	var ack providerBodyAuditAckV1
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(string(ackLine))))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ack); err != nil {
		return errors.New("provider request audit scanner acknowledgement is invalid")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || ack.SchemaVersion != 1 ||
		ack.Purpose != "analytix.provider-request-body-audit-ack/v1" ||
		ack.ProviderFamily != strings.TrimSpace(providerFamily) || ack.Attempt != attempt ||
		ack.BodySHA256 != bodySHA256 {
		return errors.New("provider request audit scanner acknowledgement is invalid")
	}
	if !ack.Accepted {
		return errors.New("provider request audit rejected outbound body")
	}
	return nil
}

func writeProviderBodyAuditBytesV1(writer io.Writer, value []byte) error {
	for len(value) > 0 {
		count, err := writer.Write(value)
		if err != nil || count <= 0 {
			return errors.New("provider request audit scanner write failed")
		}
		value = value[count:]
	}
	return nil
}
