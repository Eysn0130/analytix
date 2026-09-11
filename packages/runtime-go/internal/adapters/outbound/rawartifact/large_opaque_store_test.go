//go:build linux

package rawartifact_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	rawartifactadapter "analytix.local/runtime-go/internal/adapters/outbound/rawartifact"
	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
	"golang.org/x/sys/unix"
)

func TestLargeOpaqueChunkStoreRoundTripRestartAndIdempotence(t *testing.T) {
	store, root, lease := openProvisionedLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("evidence-chunk-"), 32_768)
	descriptor := buildLargeOpaqueDescriptor(t, "round-trip", body)

	outcome, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Disposition != rawartifactport.LargeOpaqueChunkCreated {
		t.Fatalf("first put disposition = %q", outcome.Disposition)
	}
	assertLargeOpaqueRead(t, store, descriptor, body)

	// A new adapter over the same live frozen lease models process-local reopen;
	// the durable restart path is tested separately by closing and reacquiring.
	reopened := newLargeOpaqueChunkStoreOnLease(t, lease)
	outcome, err = reopened.PutExact(context.Background(), descriptor, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Disposition != rawartifactport.LargeOpaqueChunkExistingEqual {
		t.Fatalf("idempotent put disposition = %q", outcome.Disposition)
	}
	assertLargeOpaqueRead(t, reopened, descriptor, body)

	info, err := os.Stat(largeOpaqueChunkPath(root, descriptor))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o400 || !info.Mode().IsRegular() {
		t.Fatalf("committed chunk mode = %v", info.Mode())
	}
}

func TestLargeOpaqueChunkStoreSurvivesPersistenceLeaseRestart(t *testing.T) {
	base := t.TempDir()
	roots := persistencefs.RootSet{
		DataDir: filepath.Join(base, "data"), DurableDir: filepath.Join(base, "durable"),
	}
	for _, root := range []string{roots.DataDir, roots.DurableDir} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	body := bytes.Repeat([]byte("restart-evidence"), 8192)
	descriptor := buildLargeOpaqueDescriptor(t, "lease-restart", body)

	firstLease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	first := newLargeOpaqueChunkStoreOnLease(t, firstLease)
	provisionLargeOpaqueStoreForTest(t, filepath.Join(roots.DataDir, rawartifactadapter.LargeOpaqueChunkStoreRootNameV1))
	if _, err := first.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
		_ = firstLease.Close()
		t.Fatal(err)
	}
	if err := firstLease.Close(); err != nil {
		t.Fatal(err)
	}

	secondLease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := secondLease.Close(); err != nil {
			t.Error(err)
		}
	})
	second := newLargeOpaqueChunkStoreOnLease(t, secondLease)
	assertLargeOpaqueRead(t, second, descriptor, body)
	outcome, err := second.PutExact(context.Background(), descriptor, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Disposition != rawartifactport.LargeOpaqueChunkExistingEqual {
		t.Fatalf("restart put disposition = %q", outcome.Disposition)
	}
}

func TestLargeOpaqueChunkStoreNeverCreatesMissingOwnerTopology(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		store, root, _ := openLargeOpaqueChunkStore(t)
		body := []byte("missing-root-reader-must-not-run")
		descriptor := buildLargeOpaqueDescriptor(t, "missing-root", body)
		reader := &readCallWitness{body: body}
		if _, err := store.PutExact(context.Background(), descriptor, reader); err == nil ||
			!errors.Is(err, largeopaqueport.ErrIndeterminate) || !errors.Is(err, rawartifactport.ErrUnavailable) {
			t.Fatalf("missing root error = %v", err)
		}
		if reader.calls != 0 {
			t.Fatalf("missing root consumed the source reader %d times", reader.calls)
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing root write mutated owner topology: %v", err)
		}
	})

	t.Run("shard", func(t *testing.T) {
		store, root, _ := openLargeOpaqueChunkStore(t)
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		body := []byte("missing-shard-reader-must-not-run")
		descriptor := buildLargeOpaqueDescriptor(t, "missing-shard", body)
		reader := &readCallWitness{body: body}
		if _, err := store.PutExact(context.Background(), descriptor, reader); err == nil ||
			!errors.Is(err, largeopaqueport.ErrIndeterminate) || !errors.Is(err, rawartifactport.ErrUnavailable) {
			t.Fatalf("missing shard error = %v", err)
		}
		if reader.calls != 0 {
			t.Fatalf("missing shard consumed the source reader %d times", reader.calls)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("missing shard write mutated owner topology: %v", entries)
		}
	})
}

func TestLargeOpaqueChunkStoreConcurrentEqualPutHasSingleCreator(t *testing.T) {
	store, _, _ := openProvisionedLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("concurrent-evidence"), 16_384)
	descriptor := buildLargeOpaqueDescriptor(t, "concurrent", body)

	const workers = 16
	var wait sync.WaitGroup
	wait.Add(workers)
	results := make(chan rawartifactport.LargeOpaqueChunkWriteDisposition, workers)
	errorsFound := make(chan error, workers)
	for range workers {
		go func() {
			defer wait.Done()
			outcome, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body))
			if err != nil {
				errorsFound <- err
				return
			}
			results <- outcome.Disposition
		}()
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent put failed: %v", err)
	}
	created := 0
	existing := 0
	for disposition := range results {
		switch disposition {
		case rawartifactport.LargeOpaqueChunkCreated:
			created++
		case rawartifactport.LargeOpaqueChunkExistingEqual:
			existing++
		default:
			t.Errorf("unexpected disposition %q", disposition)
		}
	}
	if created != 1 || existing != workers-1 {
		t.Fatalf("created=%d existing=%d", created, existing)
	}
}

func TestLargeOpaqueChunkStoreRejectsInexactSourcesAndCleansTemps(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader func([]byte) io.Reader
		ctx    func() context.Context
	}{
		{name: "short", reader: func(body []byte) io.Reader { return bytes.NewReader(body[:len(body)-1]) }},
		{name: "extra", reader: func(body []byte) io.Reader { return bytes.NewReader(append(append([]byte(nil), body...), 1)) }},
		{name: "wrong hash", reader: func(body []byte) io.Reader {
			wrong := append([]byte(nil), body...)
			wrong[len(wrong)-1] ^= 0xff
			return bytes.NewReader(wrong)
		}},
		{name: "no progress", reader: func([]byte) io.Reader { return noProgressReader{} }},
		{name: "cancelled", reader: func(body []byte) io.Reader { return bytes.NewReader(body) }, ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
			body := bytes.Repeat([]byte("exact-source"), 4096)
			descriptor := buildLargeOpaqueDescriptor(t, test.name, body)
			ctx := context.Background()
			if test.ctx != nil {
				ctx = test.ctx()
			}
			_, putErr := store.PutExact(ctx, descriptor, test.reader(body))
			if putErr == nil {
				t.Fatal("inexact source was accepted")
			}
			if test.name == "cancelled" {
				if !errors.Is(putErr, context.Canceled) || errors.Is(putErr, rawartifactport.ErrCorrupt) ||
					errors.Is(putErr, largeopaqueport.ErrIntegrity) {
					t.Fatalf("cancelled source classification = %v", putErr)
				}
			} else if !errors.Is(putErr, rawartifactport.ErrCorrupt) ||
				!errors.Is(putErr, largeopaqueport.ErrSourceMismatch) ||
				errors.Is(putErr, rawartifactport.ErrMismatch) {
				t.Fatalf("inexact source classification = %v", putErr)
			}
			if _, err := os.Lstat(largeOpaqueChunkPath(root, descriptor)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("inexact source left a committed object: %v", err)
			}
			shard := filepath.Join(root, descriptor.DescriptorDigest[:2])
			entries, err := os.ReadDir(shard)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("inexact source left residue: %v", entries)
			}
		})
	}
}

func TestLargeOpaqueChunkStoreRejectsUnsafeNamedObjects(t *testing.T) {
	for _, test := range []struct {
		name   string
		tamper func(string) error
	}{
		{name: "writable mode", tamper: func(path string) error { return os.Chmod(path, 0o600) }},
		{name: "hard link", tamper: func(path string) error { return os.Link(path, path+".alias") }},
		{name: "symlink", tamper: func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Symlink(filepath.Base(path)+".target", path)
		}},
		{name: "fifo", tamper: func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return unix.Mkfifo(path, 0o400)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
			body := []byte("unsafe-named-object")
			descriptor := buildLargeOpaqueDescriptor(t, test.name, body)
			if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
				t.Fatal(err)
			}
			if err := test.tamper(largeOpaqueChunkPath(root, descriptor)); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := store.ReadExact(context.Background(), descriptor, &output); err == nil ||
				!errors.Is(err, largeopaqueport.ErrIntegrity) || !errors.Is(err, rawartifactport.ErrCorrupt) {
				t.Fatalf("unsafe named object classification = %v", err)
			}
		})
	}
}

func TestLargeOpaqueChunkStoreRejectsShortWriter(t *testing.T) {
	store, _, _ := openProvisionedLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("writer-evidence"), 4096)
	descriptor := buildLargeOpaqueDescriptor(t, "short-writer", body)
	if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if err := store.ReadExact(context.Background(), descriptor, shortWriter{}); !errors.Is(err, io.ErrShortWrite) ||
		!errors.Is(err, largeopaqueport.ErrDelivery) || errors.Is(err, largeopaqueport.ErrIntegrity) ||
		errors.Is(err, rawartifactport.ErrCorrupt) || errors.Is(err, rawartifactport.ErrUnavailable) {
		t.Fatalf("short writer error = %v", err)
	}
}

func TestLargeOpaqueChunkStorePreservesExtraProbeReaderError(t *testing.T) {
	store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("extra-probe-error"), 4096)
	descriptor := buildLargeOpaqueDescriptor(t, "extra-probe-error", body)
	reader := &exactThenErrorReader{body: body, err: context.Canceled}
	_, err := store.PutExact(context.Background(), descriptor, reader)
	if !errors.Is(err, largeopaqueport.ErrSourceRead) || !errors.Is(err, rawartifactport.ErrUnavailable) ||
		errors.Is(err, context.Canceled) || errors.Is(err, largeopaqueport.ErrSourceMismatch) ||
		errors.Is(err, rawartifactport.ErrCorrupt) {
		t.Fatalf("extra-probe reader error classification = %v", err)
	}
	if !reader.extraProbe {
		t.Fatal("reader error was not reached by the exact-length extra-byte probe")
	}
	if _, statErr := os.Lstat(largeOpaqueChunkPath(root, descriptor)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("reader error committed a chunk: %v", statErr)
	}
}

func TestLargeOpaqueChunkStoreRejectsJoinedEOFSentinels(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader func([]byte, error) io.Reader
	}{
		{name: "joined EOF on final body read", reader: func(body []byte, sentinel error) io.Reader {
			return &joinedEOFFinalReader{body: body, err: errors.Join(io.EOF, sentinel)}
		}},
		{name: "joined EOF on extra probe", reader: func(body []byte, sentinel error) io.Reader {
			return &exactThenErrorReader{body: body, err: errors.Join(io.EOF, sentinel)}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
			body := bytes.Repeat([]byte("joined-eof"), 2048)
			descriptor := buildLargeOpaqueDescriptor(t, test.name, body)
			sentinel := errors.New("adversarial EOF companion error")
			_, err := store.PutExact(context.Background(), descriptor, test.reader(body, sentinel))
			if errors.Is(err, sentinel) || !errors.Is(err, largeopaqueport.ErrSourceRead) ||
				errors.Is(err, largeopaqueport.ErrSourceMismatch) || !errors.Is(err, rawartifactport.ErrUnavailable) {
				t.Fatalf("joined EOF classification = %v", err)
			}
			if _, statErr := os.Lstat(largeOpaqueChunkPath(root, descriptor)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("joined EOF committed a chunk: %v", statErr)
			}
		})
	}
}

func TestLargeOpaqueChunkStoreRejectsInjectedReaderAndWriterSentinels(t *testing.T) {
	store, _, _ := openProvisionedLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("injected-sentinel"), 2048)
	descriptor := buildLargeOpaqueDescriptor(t, "injected-sentinel", body)
	for _, injected := range []error{
		context.Canceled,
		largeopaqueport.ErrIntegrity,
		largeopaqueport.ErrIndeterminate,
		largeopaqueport.ErrDelivery,
	} {
		reader := &exactThenErrorReader{body: body, err: injected}
		_, err := store.PutExact(context.Background(), descriptor, reader)
		if !errors.Is(err, largeopaqueport.ErrSourceRead) || !errors.Is(err, rawartifactport.ErrUnavailable) ||
			errors.Is(err, injected) {
			t.Fatalf("reader-injected %v classification = %v", injected, err)
		}
	}
	if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	for _, injected := range []error{largeopaqueport.ErrIntegrity, largeopaqueport.ErrIndeterminate} {
		err := store.ReadExact(context.Background(), descriptor, injectedErrorWriter{err: injected})
		if !errors.Is(err, largeopaqueport.ErrDelivery) || errors.Is(err, injected) ||
			errors.Is(err, rawartifactport.ErrCorrupt) || errors.Is(err, rawartifactport.ErrUnavailable) {
			t.Fatalf("writer-injected %v classification = %v", injected, err)
		}
	}
}

func TestLargeOpaqueChunkStoreRejectsTypedNilStreamsAsInputMismatch(t *testing.T) {
	store, _, _ := openProvisionedLargeOpaqueChunkStore(t)
	body := []byte("typed-nil-evidence")
	descriptor := buildLargeOpaqueDescriptor(t, "typed-nil", body)
	var reader *bytes.Reader
	if _, err := store.PutExact(context.Background(), descriptor, reader); !errors.Is(err, rawartifactport.ErrMismatch) {
		t.Fatalf("typed-nil reader error = %v", err)
	}
	if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	var writer *bytes.Buffer
	if err := store.ReadExact(context.Background(), descriptor, writer); !errors.Is(err, rawartifactport.ErrMismatch) {
		t.Fatalf("typed-nil writer error = %v", err)
	}
}

func TestLargeOpaqueChunkStoreRejectsCaseOnlyFinalAlias(t *testing.T) {
	store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("case-exact-evidence"), 4096)
	descriptor := buildLargeOpaqueDescriptor(t, "case-only-final", body)
	if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	finalPath := largeOpaqueChunkPath(root, descriptor)
	aliasPath := filepath.Join(filepath.Dir(finalPath), strings.ToUpper(filepath.Base(finalPath)))
	if err := os.Rename(finalPath, aliasPath); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := store.ReadExact(context.Background(), descriptor, &output); !errors.Is(err, largeopaqueport.ErrIntegrity) ||
		!errors.Is(err, rawartifactport.ErrCorrupt) {
		t.Fatalf("case-only read error = %v", err)
	}
	if outcome, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err == nil ||
		outcome.Disposition == rawartifactport.LargeOpaqueChunkExistingEqual ||
		!errors.Is(err, largeopaqueport.ErrIntegrity) {
		t.Fatalf("case-only idempotent put outcome=%+v err=%v", outcome, err)
	}
}

func TestLargeOpaqueChunkStoreRejectsCaseOnlyUnsafeAliases(t *testing.T) {
	for _, test := range []struct {
		name   string
		create func(string, []byte) error
	}{
		{name: "zero-mode file", create: func(path string, body []byte) error {
			if err := os.WriteFile(path, body, 0o600); err != nil {
				return err
			}
			return os.Chmod(path, 0)
		}},
		{name: "symlink", create: func(path string, _ []byte) error {
			return os.Symlink(filepath.Base(path)+".target", path)
		}},
		{name: "fifo", create: func(path string, _ []byte) error {
			return unix.Mkfifo(path, 0o400)
		}},
		{name: "socket", create: func(path string, _ []byte) error {
			shortRoot, err := os.MkdirTemp("/tmp", "axlo-")
			if err != nil {
				return err
			}
			if err := os.Remove(shortRoot); err != nil {
				return err
			}
			if err := os.Symlink(filepath.Dir(path), shortRoot); err != nil {
				return err
			}
			defer os.Remove(shortRoot)
			fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
			if err != nil {
				return err
			}
			unix.CloseOnExec(fd)
			shortPath := filepath.Join(shortRoot, "s")
			if err := unix.Bind(fd, &unix.SockaddrUnix{Name: shortPath}); err != nil {
				return errors.Join(err, unix.Close(fd))
			}
			if err := unix.Close(fd); err != nil {
				return err
			}
			return os.Rename(filepath.Join(filepath.Dir(path), "s"), path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
			body := bytes.Repeat([]byte("case-only-unsafe-alias"), 128)
			descriptor := buildLargeOpaqueDescriptor(t, test.name, body)
			if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
				t.Fatal(err)
			}
			finalPath := largeOpaqueChunkPath(root, descriptor)
			if err := os.Remove(finalPath); err != nil {
				t.Fatal(err)
			}
			aliasPath := filepath.Join(filepath.Dir(finalPath), strings.ToUpper(filepath.Base(finalPath)))
			if err := test.create(aliasPath, body); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			err := store.ReadExact(context.Background(), descriptor, &output)
			if !errors.Is(err, largeopaqueport.ErrIntegrity) || !errors.Is(err, rawartifactport.ErrCorrupt) {
				t.Fatalf("case-only unsafe alias classification = %v", err)
			}
		})
	}
}

func TestLargeOpaqueChunkStoreDeliversOnlyItsPreviouslyVerifiedBuffer(t *testing.T) {
	for _, test := range []struct {
		name   string
		tamper func(string, []byte) error
	}{
		{name: "replace final inode", tamper: replaceLargeOpaqueFinal},
		{name: "replace shard", tamper: replaceLargeOpaqueShard},
		{name: "replace root", tamper: replaceLargeOpaqueRoot},
		{name: "add hardlink", tamper: func(path string, _ []byte) error { return os.Link(path, path+".hardlink") }},
		{name: "make writable", tamper: func(path string, _ []byte) error { return os.Chmod(path, 0o600) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
			body := bytes.Repeat([]byte("delivery-cut-point"), 16_384)
			descriptor := buildLargeOpaqueDescriptor(t, test.name, body)
			if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(body)); err != nil {
				t.Fatal(err)
			}
			writer := &cutPointWriter{onFirst: func() error {
				return test.tamper(largeOpaqueChunkPath(root, descriptor), body)
			}}
			err := store.ReadExact(context.Background(), descriptor, writer)
			if err != nil {
				t.Fatalf("verified-buffer delivery failed after source validation: %v", err)
			}
			if !bytes.Equal(writer.buffer.Bytes(), body) {
				t.Fatal("post-validation source mutation changed the already verified delivery buffer")
			}
		})
	}
}

func TestLargeOpaqueChunkStoreUmaskCannotLeaveFailedTemp(t *testing.T) {
	store, root, _ := openProvisionedLargeOpaqueChunkStore(t)
	body := bytes.Repeat([]byte("umask-evidence"), 4096)
	descriptor := buildLargeOpaqueDescriptor(t, "umask-cleanup", body)
	shard := filepath.Join(root, descriptor.DescriptorDigest[:2])
	if err := os.MkdirAll(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	wrong := append([]byte(nil), body...)
	wrong[len(wrong)-1] ^= 0xff
	previous := unix.Umask(0o777)
	defer unix.Umask(previous)
	if _, err := store.PutExact(context.Background(), descriptor, bytes.NewReader(wrong)); err == nil {
		t.Fatal("wrong-hash source passed under restrictive umask")
	}
	entries, err := os.ReadDir(shard)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("restrictive umask left temp residue: %v", entries)
	}
}

type noProgressReader struct{}

func (noProgressReader) Read([]byte) (int, error) { return 0, nil }

type readCallWitness struct {
	body  []byte
	calls int
}

func (reader *readCallWitness) Read(target []byte) (int, error) {
	reader.calls++
	if len(reader.body) == 0 {
		return 0, io.EOF
	}
	read := copy(target, reader.body)
	reader.body = reader.body[read:]
	return read, nil
}

type shortWriter struct{}

func (shortWriter) Write(body []byte) (int, error) {
	if len(body) == 0 {
		return 0, nil
	}
	return len(body) - 1, nil
}

type injectedErrorWriter struct{ err error }

func (writer injectedErrorWriter) Write([]byte) (int, error) { return 0, writer.err }

type exactThenErrorReader struct {
	body       []byte
	err        error
	offset     int
	extraProbe bool
}

func (reader *exactThenErrorReader) Read(target []byte) (int, error) {
	if reader.offset < len(reader.body) {
		read := copy(target, reader.body[reader.offset:])
		reader.offset += read
		return read, nil
	}
	reader.extraProbe = true
	return 0, reader.err
}

type joinedEOFFinalReader struct {
	body []byte
	err  error
	done bool
}

func (reader *joinedEOFFinalReader) Read(target []byte) (int, error) {
	if reader.done {
		return 0, io.EOF
	}
	reader.done = true
	return copy(target, reader.body), reader.err
}

type cutPointWriter struct {
	once    sync.Once
	onFirst func() error
	buffer  bytes.Buffer
}

func (writer *cutPointWriter) Write(body []byte) (int, error) {
	var cutErr error
	writer.once.Do(func() {
		if writer.onFirst != nil {
			cutErr = writer.onFirst()
		}
	})
	if cutErr != nil {
		return 0, cutErr
	}
	return writer.buffer.Write(body)
}

func replaceLargeOpaqueFinal(path string, body []byte) error {
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, body, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(replacement, 0o400); err != nil {
		return err
	}
	return os.Rename(replacement, path)
}

func replaceLargeOpaqueShard(path string, body []byte) error {
	shard := filepath.Dir(path)
	if err := os.Rename(shard, shard+".old"); err != nil {
		return err
	}
	if err := os.Mkdir(shard, 0o700); err != nil {
		return err
	}
	return writeLargeOpaqueReplacement(path, body)
}

func replaceLargeOpaqueRoot(path string, body []byte) error {
	root := filepath.Dir(filepath.Dir(path))
	if err := os.Rename(root, root+".old"); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeLargeOpaqueReplacement(path, body)
}

func writeLargeOpaqueReplacement(path string, body []byte) error {
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o400)
}
