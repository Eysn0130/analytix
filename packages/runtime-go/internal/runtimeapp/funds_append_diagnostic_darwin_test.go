//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/server"
)

// This test-only observer measures the fixed publication/append boundary and
// samples the append goroutine at most three times. No raw stack, arguments,
// IDs, paths or Provider bodies enter diagnostics. It does not alter budgets,
// source authority, native ownership or the async completion guard.
type fundsAppendDiagnosticV1 struct {
	t        *testing.T
	mu       sync.Mutex
	fixedAt  time.Time
	appendAt time.Time
	stop     chan struct{}
	samples  sync.WaitGroup
	closed   bool
}

func (diagnostic *fundsAppendDiagnosticV1) phase(phase string) {
	diagnostic.mu.Lock()
	defer diagnostic.mu.Unlock()
	if diagnostic.closed {
		return
	}
	if phase == "case_fixed_persist" {
		diagnostic.fixedAt = time.Now()
		return
	}
	if phase == "case_candidate_persist" {
		diagnostic.fixedAt = time.Time{}
		return
	}
	if phase != "case_longitudinal_append" || diagnostic.fixedAt.IsZero() {
		return
	}
	diagnostic.stopLocked()
	diagnostic.appendAt = time.Now()
	diagnostic.stop = make(chan struct{})
	diagnostic.t.Logf("fixed_publication_to_append_ms=%d", diagnostic.appendAt.Sub(diagnostic.fixedAt).Milliseconds())
	diagnostic.samples.Add(1)
	go func(start time.Time, stop <-chan struct{}) {
		defer diagnostic.samples.Done()
		diagnostic.sample(start, stop)
	}(diagnostic.appendAt, diagnostic.stop)
}

func (diagnostic *fundsAppendDiagnosticV1) finished(observation server.AsyncTurnObservationV1) {
	if observation.Stage != "finished" {
		return
	}
	finishedAt := time.Now()
	diagnostic.mu.Lock()
	defer diagnostic.mu.Unlock()
	if diagnostic.closed {
		return
	}
	if !diagnostic.appendAt.IsZero() {
		diagnostic.t.Logf("append_to_async_finished_ms=%d completion=%s fallback=%s", finishedAt.Sub(diagnostic.appendAt).Milliseconds(), observation.CompletionErrorClass, observation.FailureRecordErrorClass)
	}
	diagnostic.stopLocked()
	diagnostic.fixedAt, diagnostic.appendAt = time.Time{}, time.Time{}
}

func (diagnostic *fundsAppendDiagnosticV1) close() {
	diagnostic.mu.Lock()
	diagnostic.closed = true
	diagnostic.stopLocked()
	diagnostic.fixedAt, diagnostic.appendAt = time.Time{}, time.Time{}
	diagnostic.mu.Unlock()
	diagnostic.samples.Wait()
}

func (diagnostic *fundsAppendDiagnosticV1) stopLocked() {
	if diagnostic.stop != nil {
		close(diagnostic.stop)
		diagnostic.stop = nil
	}
}

func (diagnostic *fundsAppendDiagnosticV1) sample(start time.Time, stop <-chan struct{}) {
	for _, elapsed := range []time.Duration{time.Second, 3 * time.Second, 8 * time.Second} {
		timer := time.NewTimer(max(time.Until(start.Add(elapsed)), 0))
		select {
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		}
		buffer := make([]byte, 1<<20)
		n := runtime.Stack(buffer, true)
		state, functions := fundsAppendStackProjectionV1(buffer[:n])
		clear(buffer)
		select {
		case <-stop:
			return
		default:
			diagnostic.t.Logf("append_sample_ms=%d sampled=%t stack_buffer_full=%t state=%s functions=%v", elapsed.Milliseconds(), len(functions) != 0, n == len(buffer), state, functions)
		}
	}
}

func fundsAppendStackProjectionV1(stack []byte) (string, []string) {
	const appendFrame = "internal/app/caseentity.AppendFinalizedCaseLongitudinalStateV1("
	const appendServiceFrame = "internal/app/caseentity.(*Service).AppendCaseLongitudinalOwnerStateV1("
	for _, block := range bytes.Split(stack, []byte("\n\n")) {
		if !bytes.Contains(block, []byte(appendFrame)) && !bytes.Contains(block, []byte(appendServiceFrame)) {
			continue
		}
		lines := bytes.Split(block, []byte("\n"))
		state := "unknown"
		if open, end := bytes.IndexByte(lines[0], '['), bytes.LastIndexByte(lines[0], ']'); open >= 0 && end > open {
			state = string(lines[0][open+1 : end])
		}
		var functions []string
		for _, line := range lines[1:] {
			if len(functions) == 40 {
				break
			}
			if len(line) == 0 || line[0] == '\t' {
				continue
			}
			arguments := bytes.LastIndexByte(line, '(')
			if arguments < 0 {
				continue
			}
			name := string(line[:arguments])
			if strings.HasPrefix(name, "analytix.local/runtime-go/internal/") || strings.HasPrefix(name, "syscall.") ||
				strings.HasPrefix(name, "golang.org/x/sys/unix.") || strings.HasPrefix(name, "runtime.") || strings.HasPrefix(name, "sync.") {
				functions = append(functions, name)
			}
		}
		return state, functions
	}
	return "not_sampled", nil
}

func TestFundsAppendStackProjectionKeepsOnlyFunctionNames(t *testing.T) {
	raw := []byte("goroutine 999 [syscall]:\n" +
		"syscall.syscalln(0xSECRET, {PRIVATE_ARGUMENT})\n\t/PRIVATE_PATH/runtime.go:10 +0xSECRET\n" +
		"analytix.local/runtime-go/internal/app/caseentity.(*Service).AppendCaseLongitudinalOwnerStateV1(0xSECRET, PRIVATE_ARGUMENT)\n\t/PRIVATE_PATH/service.go:20\n\n" +
		"goroutine 888 [running]:\nOTHER_PRIVATE_FUNCTION(OTHER_PRIVATE_ARGUMENT)\n")
	state, functions := fundsAppendStackProjectionV1(raw)
	joined := strings.Join(functions, " ")
	if state != "syscall" || len(functions) != 2 || functions[0] != "syscall.syscalln" || functions[1] != "analytix.local/runtime-go/internal/app/caseentity.(*Service).AppendCaseLongitudinalOwnerStateV1" ||
		strings.Contains(joined, "PRIVATE") || strings.Contains(joined, "SECRET") || strings.Contains(joined, "999") {
		t.Fatal("append diagnostic exposed arguments, paths, IDs or unrelated goroutines")
	}
	if state, functions := fundsAppendStackProjectionV1([]byte("goroutine 888 [running]:\nOTHER_PRIVATE_FUNCTION(OTHER_PRIVATE_ARGUMENT)\n")); state != "not_sampled" || len(functions) != 0 {
		t.Fatal("append diagnostic attributed an unrelated goroutine")
	}
	inputRead := []byte("goroutine 777 [semacquire]:\n" +
		"analytix.local/runtime-go/internal/server.(*DurableEventSessionStore).GetThread(PRIVATE_ARGUMENT)\n\t/PRIVATE_PATH/threads.go:10\n" +
		"analytix.local/runtime-go/internal/app/caseentity.AppendFinalizedCaseLongitudinalStateV1(PRIVATE_ARGUMENT)\n\t/PRIVATE_PATH/owner.go:20\n")
	if state, functions := fundsAppendStackProjectionV1(inputRead); state != "semacquire" || len(functions) != 2 || functions[0] != "analytix.local/runtime-go/internal/server.(*DurableEventSessionStore).GetThread" {
		t.Fatal("append diagnostic lost the input-reader portion of its phase")
	}
}
