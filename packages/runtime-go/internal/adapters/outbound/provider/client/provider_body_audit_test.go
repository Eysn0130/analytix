package client

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixProviderRequestBodyAuditorBindsExactBodyAndAcknowledgement(t *testing.T) {
	socketRoot, err := os.MkdirTemp(os.Getenv("TMPDIR"), "pa-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketRoot) })
	socketPath := filepath.Join(socketRoot, "audit.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"safe"}]}`)
	serverDone := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		headerLine, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			serverDone <- readErr
			return
		}
		var header providerBodyAuditHeaderV1
		if decodeErr := json.Unmarshal(headerLine, &header); decodeErr != nil {
			serverDone <- decodeErr
			return
		}
		observed := make([]byte, header.BodyByteLength)
		if _, readErr := io.ReadFull(reader, observed); readErr != nil {
			serverDone <- readErr
			return
		}
		digest := sha256.Sum256(observed)
		ack, marshalErr := json.Marshal(providerBodyAuditAckV1{
			SchemaVersion: 1, Purpose: "analytix.provider-request-body-audit-ack/v1",
			Accepted: true, ProviderFamily: header.ProviderFamily, Attempt: header.Attempt,
			BodySHA256: hex.EncodeToString(digest[:]),
		})
		if marshalErr == nil {
			ack = append(ack, '\n')
			_, marshalErr = connection.Write(ack)
		}
		serverDone <- marshalErr
	}()
	auditor, err := NewUnixProviderRequestBodyAuditorV1(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := auditor.AuditProviderRequestBody(context.Background(), "analytix-hub", 2, body); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}
