//go:build darwin && !analytix_prod

package processauthority

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	darwinBootstrapCommandMagic = "ANLXBOOT"
	darwinBootstrapCommandLimit = 2 * 1024 * 1024
	darwinBootstrapStringLimit  = 1024 * 1024
)

type darwinBootstrapCommand struct {
	target      string
	arguments   []string
	environment []string
}

func init() {
	// Only the exact build-tagged bootstrap identity may enter before ordinary
	// argument parsing. File names are not identities because an executable can
	// be staged under an arbitrary name.
	if !currentBootstrapMainAllowed() || os.Getenv(darwinBootstrapEnvironment) != "1" || bootstrapMarkerIndex(os.Args) <= 1 {
		return
	}
	handled, err := dispatchBootstrapInvocation(os.Args)
	if !handled || err != nil {
		os.Exit(125)
	}
	// A successful dispatch replaces the process image and cannot return.
	os.Exit(125)
}

func dispatchBootstrap(_ []string, _ int) error {
	nonce := os.Getenv(darwinBootstrapNonceEnv)
	decodedNonce, err := hex.DecodeString(nonce)
	if err != nil || len(decodedNonce) != 32 || nonce != hex.EncodeToString(decodedNonce) {
		return errBootstrapInvalid
	}
	if err := prepareDarwinBuildProbeBootstrap(); err != nil {
		return err
	}
	if err := unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return errBootstrapInvalid
	}
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NPROC, &limit); err != nil || limit.Cur != 0 || limit.Max != 0 {
		return errBootstrapInvalid
	}
	if err := proveDarwinForkDenied(); err != nil {
		return err
	}
	proof := []byte(
		`{"kind":"analytix_process_authority_bootstrap","schema_version":1,"nonce":"` + nonce +
			`","process_id":` + strconv.Itoa(os.Getpid()) +
			`,"nproc_soft":0,"nproc_hard":0,"fork_probe_errno":"EAGAIN"}` + "\n",
	)
	if err := writeDarwinBootstrapAll(os.Stdout, proof); err != nil {
		return errBootstrapInvalid
	}
	command, err := readDarwinBootstrapCommand(os.Stdin)
	if err != nil {
		return err
	}
	if err := syscall.Exec(
		command.target,
		append([]string{command.target}, command.arguments...),
		command.environment,
	); err != nil {
		return errBootstrapInvalid
	}
	return errBootstrapInvalid
}

func proveDarwinForkDenied() error {
	pid, err := syscall.ForkExec(
		"/usr/bin/true",
		[]string{"/usr/bin/true"},
		&syscall.ProcAttr{
			Env:   []string{"LANG=C", "LC_ALL=C", "TZ=UTC"},
			Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		},
	)
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = syscall.Wait4(pid, nil, 0, nil)
		return errBootstrapInvalid
	}
	if !errors.Is(err, syscall.EAGAIN) {
		return errBootstrapInvalid
	}
	return nil
}

func encodeDarwinBootstrapCommand(command darwinBootstrapCommand) ([]byte, error) {
	if command.target == "" || len(command.target) > darwinBootstrapStringLimit ||
		len(command.arguments) > MaxArguments-1 || len(command.environment) == 0 ||
		len(command.environment) > MaxEnvironment {
		return nil, errBootstrapInvalid
	}
	payload := bytes.NewBuffer(make([]byte, 0, 4096))
	payload.WriteString(darwinBootstrapCommandMagic)
	payload.Write(make([]byte, 4))
	if err := binary.Write(payload, binary.BigEndian, uint32(len(command.arguments))); err != nil {
		return nil, errBootstrapInvalid
	}
	if err := binary.Write(payload, binary.BigEndian, uint32(len(command.environment))); err != nil {
		return nil, errBootstrapInvalid
	}
	if err := appendDarwinBootstrapString(payload, command.target); err != nil {
		return nil, err
	}
	for _, value := range command.arguments {
		if err := appendDarwinBootstrapString(payload, value); err != nil {
			return nil, err
		}
	}
	for _, value := range command.environment {
		if err := appendDarwinBootstrapString(payload, value); err != nil {
			return nil, err
		}
	}
	if payload.Len() > darwinBootstrapCommandLimit {
		return nil, errBootstrapInvalid
	}
	binary.BigEndian.PutUint32(payload.Bytes()[8:12], uint32(payload.Len()-12))
	return append([]byte(nil), payload.Bytes()...), nil
}

func appendDarwinBootstrapString(payload *bytes.Buffer, value string) error {
	if payload == nil || value == "" || len(value) > darwinBootstrapStringLimit || bytes.IndexByte([]byte(value), 0) >= 0 {
		return errBootstrapInvalid
	}
	if err := binary.Write(payload, binary.BigEndian, uint32(len(value))); err != nil {
		return errBootstrapInvalid
	}
	_, _ = payload.WriteString(value)
	return nil
}

func readDarwinBootstrapCommand(reader io.Reader) (darwinBootstrapCommand, error) {
	var result darwinBootstrapCommand
	header := make([]byte, 20)
	if _, err := io.ReadFull(reader, header); err != nil || string(header[:8]) != darwinBootstrapCommandMagic {
		return result, errBootstrapInvalid
	}
	payloadLength := binary.BigEndian.Uint32(header[8:12])
	argumentCount := binary.BigEndian.Uint32(header[12:16])
	environmentCount := binary.BigEndian.Uint32(header[16:20])
	if payloadLength < 8 || payloadLength > darwinBootstrapCommandLimit-12 ||
		argumentCount > MaxArguments-1 || environmentCount == 0 || environmentCount > MaxEnvironment {
		return result, errBootstrapInvalid
	}
	payload := make([]byte, payloadLength-8)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return result, errBootstrapInvalid
	}
	cursor := 0
	readString := func() (string, bool) {
		if cursor > len(payload)-4 {
			return "", false
		}
		length := int(binary.BigEndian.Uint32(payload[cursor : cursor+4]))
		cursor += 4
		if length <= 0 || length > darwinBootstrapStringLimit || length > len(payload)-cursor {
			return "", false
		}
		value := payload[cursor : cursor+length]
		cursor += length
		if bytes.IndexByte(value, 0) >= 0 {
			return "", false
		}
		return string(value), true
	}
	var ok bool
	if result.target, ok = readString(); !ok {
		return darwinBootstrapCommand{}, errBootstrapInvalid
	}
	result.arguments = make([]string, argumentCount)
	for index := range result.arguments {
		if result.arguments[index], ok = readString(); !ok {
			return darwinBootstrapCommand{}, errBootstrapInvalid
		}
	}
	result.environment = make([]string, environmentCount)
	for index := range result.environment {
		if result.environment[index], ok = readString(); !ok {
			return darwinBootstrapCommand{}, errBootstrapInvalid
		}
	}
	if cursor != len(payload) {
		return darwinBootstrapCommand{}, errBootstrapInvalid
	}
	return result, nil
}

func writeDarwinBootstrapAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil || written <= 0 {
			return errBootstrapInvalid
		}
		payload = payload[written:]
	}
	return nil
}
