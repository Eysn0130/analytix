//go:build darwin && analytix_native_build_probe_fixture && !analytix_prod

package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var (
	component string
	engine    string
	mode      string
)

type pingRequest struct {
	RequestID string `json:"request_id"`
	Command   string `json:"command"`
}

func main() {
	nonce := os.Getenv("ANALYTIX_NATIVE_LAUNCH_NONCE")
	if !validSHA256(nonce) || os.Getenv("ANALYTIX_NATIVE_PROTOCOL_VERSION") != "analytix-native-v1" ||
		!validIdentity() || !validArguments() || !validEnvironment(nonce) || !validProcessBoundary() {
		os.Exit(2)
	}
	if mode == "hang-ready" {
		park()
	}
	readyComponent := component
	if mode == "bad-ready" {
		readyComponent = "forged-component"
	}
	writeFrame(struct {
		Kind            string `json:"kind"`
		SchemaVersion   int    `json:"schema_version"`
		ComponentID     string `json:"component_id"`
		LaunchNonce     string `json:"launch_nonce"`
		ProtocolVersion string `json:"protocol_version"`
		ProcessID       int    `json:"process_id"`
	}{"analytix_native_ready", 3, readyComponent, nonce, "analytix-native-v1", os.Getpid()})
	if mode == "crash-after-ready" {
		os.Exit(42)
	}
	request := readPing()
	if mode == "wrong-cwd" {
		if err := os.Chdir(string(filepath.Separator)); err != nil {
			os.Exit(5)
		}
	}
	if mode == "bad-response" {
		request.RequestID = "0000000000000000000000000000000000000000000000000000000000000000"
	}
	response := struct {
		RequestID   string          `json:"request_id"`
		OK          bool            `json:"ok"`
		Data        pingData        `json:"data"`
		Diagnostics pingDiagnostics `json:"diagnostics"`
	}{
		RequestID: request.RequestID,
		OK:        true,
		Data:      pingData{Pong: true, PID: os.Getpid()},
		Diagnostics: pingDiagnostics{
			Engine: engine, Command: "ping", CaseBound: false, DBBound: false,
			PID: os.Getpid(), QueueWaitMS: 0, RunMS: 0, OwnerEpoch: "",
		},
	}
	if maliciousBuildProbeMode() {
		time.Sleep(750 * time.Millisecond)
	}
	switch mode {
	case "extra-stdout":
		writeFrameWithSuffix(response, []byte("{}\n"))
	case "extra-stderr":
		writeRaw(os.Stderr, []byte("unexpected stderr\n"))
		writeFrame(response)
	case "flood-after-response":
		writeFrameWithSuffix(response, bytes.Repeat([]byte{'x'}, 3072))
		for {
			writeRaw(os.Stdout, bytes.Repeat([]byte{'x'}, 4096))
		}
	case "sigcont-handler":
		continued := make(chan os.Signal, 1)
		signal.Notify(continued, syscall.SIGCONT)
		go func() {
			<-continued
			writeRaw(os.Stdout, []byte("resumed-after-terminal-response\n"))
		}()
		writeRaw(os.Stderr, []byte("sigcont-handler-armed\n"))
		writeFrame(response)
	default:
		writeFrame(response)
	}
	park()
}

func maliciousBuildProbeMode() bool {
	switch mode {
	case "extra-stdout", "extra-stderr", "flood-after-response", "sigcont-handler", "wrong-cwd":
		return true
	default:
		return false
	}
}

type pingData struct {
	Pong bool `json:"pong"`
	PID  int  `json:"pid"`
}

type pingDiagnostics struct {
	Engine      string `json:"engine"`
	Command     string `json:"command"`
	CaseBound   bool   `json:"case_bound"`
	DBBound     bool   `json:"db_bound"`
	PID         int    `json:"pid"`
	QueueWaitMS uint64 `json:"queue_wait_ms"`
	RunMS       uint64 `json:"run_ms"`
	OwnerEpoch  string `json:"owner_epoch"`
}

func validIdentity() bool {
	switch component {
	case "import-accelerator":
		return engine == "analytix-import-accelerator"
	case "cleaning-ops":
		return engine == "analytix-cleaning-ops"
	case "analysis-compute":
		return engine == "analytix-analysis-compute"
	case "data-engine":
		return engine == "analytix-data-engine"
	default:
		return false
	}
}

func validArguments() bool {
	return len(os.Args) == 2 && os.Args[1] == "--analytix-native-probe"
}

func validEnvironment(nonce string) bool {
	want := []string{
		"ANALYTIX_NATIVE_LAUNCH_NONCE=" + nonce,
		"ANALYTIX_NATIVE_PROTOCOL_VERSION=analytix-native-v1",
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
	}
	got := append([]string(nil), os.Environ()...)
	sort.Strings(got)
	sort.Strings(want)
	return len(got) == len(want) && fmt.Sprint(got) == fmt.Sprint(want)
}

func validProcessBoundary() bool {
	var limit unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_NPROC, &limit) != nil || limit.Cur != 0 || limit.Max != 0 {
		return false
	}
	workingDirectory, err := os.Getwd()
	if err != nil || !filepath.IsAbs(workingDirectory) || filepath.Base(workingDirectory) != "work" {
		return false
	}
	var stat unix.Stat_t
	return unix.Stat(workingDirectory, &stat) == nil && stat.Mode&unix.S_IFMT == unix.S_IFDIR &&
		stat.Mode&0o777 == 0o700 && stat.Uid == uint32(os.Geteuid())
}

func readPing() pingRequest {
	reader := bufio.NewReaderSize(os.Stdin, 4096)
	frame, err := reader.ReadSlice('\n')
	if err != nil || len(frame) <= 1 || len(frame) > 4096 || reader.Buffered() != 0 ||
		bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		os.Exit(3)
	}
	frame = frame[:len(frame)-1]
	var shape map[string]json.RawMessage
	if json.Unmarshal(frame, &shape) != nil || len(shape) != 2 {
		os.Exit(3)
	}
	var request pingRequest
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.Command != "ping" || !validSHA256(request.RequestID) {
		os.Exit(3)
	}
	return request
}

func writeFrame(value any) {
	body, err := json.Marshal(value)
	if err != nil || len(body)+1 > 4096 {
		os.Exit(4)
	}
	writeRaw(os.Stdout, append(body, '\n'))
}

func writeFrameWithSuffix(value any, suffix []byte) {
	body, err := json.Marshal(value)
	if err != nil || len(body)+1+len(suffix) > 4096 {
		os.Exit(4)
	}
	payload := append(append(body, '\n'), suffix...)
	writeRaw(os.Stdout, payload)
}

func writeRaw(file *os.File, body []byte) {
	for len(body) > 0 {
		written, writeErr := file.Write(body)
		if writeErr != nil || written <= 0 {
			os.Exit(4)
		}
		body = body[written:]
	}
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == fmt.Sprintf("%x", decoded)
}

func park() {
	for {
		_, _ = io.CopyN(io.Discard, os.Stdin, 1)
		time.Sleep(time.Hour)
	}
}
