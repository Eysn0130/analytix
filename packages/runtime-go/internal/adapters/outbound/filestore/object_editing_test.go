//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
)

func objectEditingFixture(t *testing.T, raw []byte) (*ObjectEditingFiles, string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspace")
	receipts := filepath.Join(root, "receipts")
	for _, dir := range []string{workspace, receipts} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(workspace, "report.txt")
	if err := os.WriteFile(path, raw, 0o640); err != nil {
		t.Fatal(err)
	}
	store, err := NewObjectEditingFiles(receipts, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store, workspace, path
}

func objectEditingInput(t *testing.T, s *ObjectEditingFiles, workspace, path, content string) objectediting.CommitInput {
	t.Helper()
	doc, err := s.Read(context.Background(), workspace, path)
	if err != nil {
		t.Fatal(err)
	}
	return objectediting.CommitInput{Workspace: workspace, Path: path, BaseRevision: doc.Revision, Content: content, ObjectIdentity: digestAtomicText([]byte("synthetic principal\x00" + doc.Workspace + "\x00" + doc.IdentityPath)), OperationID: "operation-0001"}
}

type objectEditingReadFunc func([]byte) (int, error)

func (f objectEditingReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestObjectEditingBoundedReadStopsFileGrowthAfterStat(t *testing.T) {
	_, _, path := objectEditingFixture(t, []byte("base"))
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	const limit = int64(64)
	before, err := file.Stat()
	if err != nil || before.Size() > limit {
		t.Fatalf("initial stat: %v %v", before, err)
	}
	readBytes, grew := 0, false
	reader := objectEditingReadFunc(func(p []byte) (int, error) {
		if !grew {
			grew = true
			writer, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				return 0, err
			}
			_, writeErr := writer.Write(bytes.Repeat([]byte("x"), 2048))
			closeErr := writer.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				return 0, err
			}
		}
		n, err := file.Read(p)
		readBytes += n
		return n, err
	})
	content, err := readAtomicTextContent(reader, limit)
	if !errors.Is(err, ErrAtomicTextTooLarge) || content != nil || int64(readBytes) != limit+1 {
		t.Fatalf("growth read: bytes=%d content=%d err=%v", readBytes, len(content), err)
	}
}

func TestObjectEditingAtomicByteLimitChecksEveryReplacementPhase(t *testing.T) {
	const limit = int64(64)
	oversized := bytes.Repeat([]byte("x"), int(limit+1))
	for _, phase := range []string{"initial", "current", "displaced"} {
		t.Run(phase, func(t *testing.T) {
			_, _, path := objectEditingFixture(t, []byte("base"))
			grow := func() {
				if err := os.WriteFile(path, oversized, 0o640); err != nil {
					t.Fatal(err)
				}
			}
			hooks := &atomicTextTestHooks{}
			switch phase {
			case "initial":
				grow()
			case "current":
				hooks.AfterInitialValidation = grow
			case "displaced":
				hooks.BeforeReplace = func() error { grow(); return nil }
			}
			err := atomicReplaceTextWithHooks(atomicTextReplaceRequest{Path: path, Content: []byte("edit"), MaxBytes: limit, ExpectedExists: true, ExpectedHash: digestAtomicText([]byte("base"))}, hooks)
			wantErr := ErrAtomicTextTooLarge
			if phase == "displaced" {
				wantErr = ErrAtomicTextBeforeDrift // The exchange is rolled back.
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("phase %s: %v", phase, err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, oversized) {
				t.Fatalf("external growth was overwritten: %v", err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary file left behind: %v %v", entries, err)
			}
		})
	}
	_, _, path := objectEditingFixture(t, oversized)
	if _, err := inspectAtomicTextTargetBounded(path, false, limit); !errors.Is(err, ErrAtomicTextTooLarge) {
		t.Fatalf("bounded inspection: %v", err)
	}
	if state, err := inspectAtomicTextTarget(path, false); err != nil || !bytes.Equal(state.Content, oversized) {
		t.Fatalf("legacy unbounded inspection changed: %v", err)
	}
	request := atomicTextReplaceRequest{Path: path, Content: oversized, MaxBytes: limit, ExpectedExists: true, ExpectedHash: digestAtomicText(oversized)}
	if err := atomicReplaceText(request); !errors.Is(err, ErrAtomicTextTooLarge) {
		t.Fatalf("oversized replacement: %v", err)
	}
	request.Content = bytes.Repeat([]byte("a"), int(limit))
	request.MaxBytes = 0
	if err := atomicReplaceText(request); err != nil {
		t.Fatalf("legacy replacement: %v", err)
	}
	if state, err := inspectAtomicTextTargetBounded(path, false, limit); err != nil || !bytes.Equal(state.Content, request.Content) {
		t.Fatalf("exact limit inspection: %v", err)
	}
	request.ExpectedHash = digestAtomicText(request.Content)
	request.MaxBytes = limit
	if err := atomicReplaceText(request); err != nil {
		t.Fatalf("exact limit replacement: %v", err)
	}
	for _, invalid := range []int64{-1, 1<<63 - 1} {
		if _, err := inspectAtomicTextTargetBounded(path, false, invalid); err == nil {
			t.Fatal("invalid read limit accepted")
		}
		request.MaxBytes = invalid
		if err := atomicReplaceText(request); err == nil {
			t.Fatal("invalid replacement limit accepted")
		}
	}
}

func TestObjectEditingPropagatesHardLimitsToDocumentAndJournal(t *testing.T) {
	s, workspace, path := objectEditingFixture(t, []byte("base"))
	input := objectEditingInput(t, s, workspace, path, "edit")
	journalWrites := 0
	s.replaceJournal = func(request atomicTextReplaceRequest) error {
		journalWrites++
		if request.MaxBytes != 16384 {
			t.Fatal("journal read limit missing")
		}
		return atomicReplaceText(request)
	}
	s.replaceDocument = func(request atomicTextReplaceRequest) error {
		if request.MaxBytes != objectediting.MaxTextBytes {
			t.Fatal("document read limit missing")
		}
		return atomicReplaceText(request)
	}
	if receipt, err := s.Commit(context.Background(), input); err != nil || receipt.Status != objectediting.StatusCommitted || journalWrites != 2 {
		t.Fatalf("bounded commit: %#v %v writes=%d", receipt, err, journalWrites)
	}
	if err := os.WriteFile(s.recordPath(input.ObjectIdentity, input.OperationID), bytes.Repeat([]byte(" "), 16385), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Status(context.Background(), input.ObjectIdentity, input.OperationID, workspace, path); !errors.Is(err, objectediting.ErrPersistence) {
		t.Fatalf("oversized receipt: %v", err)
	}
}

func TestObjectEditingNativeEncodingsAndDurableReplay(t *testing.T) {
	for _, encoding := range []string{filetoolsapp.TextEncodingUTF8, filetoolsapp.TextEncodingUTF8BOM, filetoolsapp.TextEncodingUTF16LE, filetoolsapp.TextEncodingUTF16BE, filetoolsapp.TextEncodingUTF16LENoBOM, filetoolsapp.TextEncodingUTF16BENoBOM} {
		t.Run(encoding, func(t *testing.T) {
			raw := filetoolsapp.EncodeTextBytes("Original UTF text 中文\r\n", encoding)
			s, w, p := objectEditingFixture(t, raw)
			in := objectEditingInput(t, s, w, p, "Edited UTF text 中文 😀\r\n")
			beforeInfo, _ := os.Stat(p)
			r, err := s.Commit(context.Background(), in)
			if err != nil || r.Status != objectediting.StatusCommitted || r.SavedAt == "" {
				t.Fatalf("commit: %#v %v", r, err)
			}
			want := filetoolsapp.EncodeTextBytes(in.Content, encoding)
			got, _ := os.ReadFile(p)
			if !bytes.Equal(got, want) || r.Revision != digestAtomicText(want) {
				t.Fatal("encoded output/revision mismatch")
			}
			afterInfo, _ := os.Stat(p)
			if beforeInfo.Mode().Perm() != afterInfo.Mode().Perm() {
				t.Fatal("mode changed")
			}
			restarted, err := NewObjectEditingFiles(s.receiptRoot, nil)
			if err != nil {
				t.Fatal(err)
			}
			status, err := restarted.Status(context.Background(), in.ObjectIdentity, in.OperationID, w, p)
			if err != nil || status != r {
				t.Fatalf("recovery: %#v %v", status, err)
			}
			if err := os.WriteFile(p, []byte("Later external edit"), 0o640); err != nil {
				t.Fatal(err)
			}
			replayed, err := restarted.Commit(context.Background(), in)
			if err != nil || replayed != r {
				t.Fatalf("replay: %#v %v", replayed, err)
			}
			got, _ = os.ReadFile(p)
			if string(got) != "Later external edit" {
				t.Fatal("replay overwrote external edit")
			}
			in.Content = "Different payload"
			if _, err := restarted.Commit(context.Background(), in); !errors.Is(err, objectediting.ErrOperationMismatch) {
				t.Fatalf("mismatch: %v", err)
			}
		})
	}
}

func TestObjectEditingConflictAndPathBinding(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("original"))
	in := objectEditingInput(t, s, w, p, "requested")
	if err := os.WriteFile(p, []byte("external"), 0o640); err != nil {
		t.Fatal(err)
	}
	r, err := s.Commit(context.Background(), in)
	if !errors.Is(err, objectediting.ErrConflict) || r.Status != objectediting.StatusConflict {
		t.Fatalf("conflict: %#v %v", r, err)
	}
	other := filepath.Join(w, "other.txt")
	if err := os.WriteFile(other, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Status(context.Background(), in.ObjectIdentity, in.OperationID, w, other); !errors.Is(err, objectediting.ErrOperationMismatch) {
		t.Fatalf("cross path: %v", err)
	}
	if _, err := s.Status(context.Background(), in.ObjectIdentity, "operation-absent", w, p); !errors.Is(err, objectediting.ErrOperationNotFound) {
		t.Fatalf("absent: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "external" {
		t.Fatal("conflict overwrote file")
	}
}

func TestObjectEditingRecoveryAfterWriteAndReceiptFailure(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	in := objectEditingInput(t, s, w, p, "after")
	s.replaceJournal = func(req atomicTextReplaceRequest) error {
		var r objectEditingRecord
		if err := json.Unmarshal(req.Content, &r); err != nil {
			return err
		}
		if r.Status == objectediting.StatusCommitted {
			return errors.New("injected receipt failure")
		}
		return atomicReplaceText(req)
	}
	r, err := s.Commit(context.Background(), in)
	if !errors.Is(err, objectediting.ErrPersistence) || r.Status != objectediting.StatusUnknown {
		t.Fatalf("unknown: %#v %v", r, err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "after" {
		t.Fatal("expected real document side effect")
	}
	restarted, err := NewObjectEditingFiles(s.receiptRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err = restarted.Status(context.Background(), in.ObjectIdentity, in.OperationID, w, p)
	if err != nil || r.Status != objectediting.StatusCommitted || r.Revision != digestAtomicText(got) {
		t.Fatalf("recovered: %#v %v", r, err)
	}
	// Receipt is now durable and does not require the file to remain unchanged.
	if err := os.WriteFile(p, []byte("later"), 0o640); err != nil {
		t.Fatal(err)
	}
	again, err := restarted.Status(context.Background(), in.ObjectIdentity, in.OperationID, w, p)
	if err != nil || again != r {
		t.Fatalf("historical receipt: %#v %v", again, err)
	}
}

func TestObjectEditingReplacementErrorDoesNotInventNoSideEffect(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	in := objectEditingInput(t, s, w, p, "after")
	s.replaceDocument = func(req atomicTextReplaceRequest) error {
		if err := atomicReplaceText(req); err != nil {
			return err
		}
		return errors.New("failure after replacement")
	}
	r, err := s.Commit(context.Background(), in)
	if err != nil || r.Status != objectediting.StatusCommitted {
		t.Fatalf("committed after error: %#v %v", r, err)
	}
}

func TestObjectEditingPendingDoesNotBlindlyRetry(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-remains", true: "external-drift"}[external], func(t *testing.T) {
			s, w, p := objectEditingFixture(t, []byte("before"))
			in := objectEditingInput(t, s, w, p, "after")
			s.replaceDocument = func(atomicTextReplaceRequest) error { return errors.New("before write failure") }
			r, err := s.Commit(context.Background(), in)
			if err == nil || r.Status != objectediting.StatusUnknown {
				t.Fatalf("pending: %#v %v", r, err)
			}
			if external {
				if err := os.WriteFile(p, []byte("external"), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			restarted, err := NewObjectEditingFiles(s.receiptRoot, nil)
			if err != nil {
				t.Fatal(err)
			}
			r, err = restarted.Commit(context.Background(), in)
			want := objectediting.StatusUnknown
			if external {
				want = objectediting.StatusConflict
			}
			if err != nil || r.Status != want {
				t.Fatalf("restart: %#v %v", r, err)
			}
			got, _ := os.ReadFile(p)
			if string(got) == "after" {
				t.Fatal("pending operation blindly rewrote document")
			}
		})
	}
}

func TestObjectEditingCancelledAfterIntentIsRecoverableUnknown(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	in := objectEditingInput(t, s, w, p, "after")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.replaceJournal = func(req atomicTextReplaceRequest) error { err := atomicReplaceText(req); cancel(); return err }
	r, err := s.Commit(ctx, in)
	if !errors.Is(err, context.Canceled) || r.Status != objectediting.StatusUnknown {
		t.Fatalf("cancel: %#v %v", r, err)
	}
	s.replaceJournal = nil
	r, err = s.Status(context.Background(), in.ObjectIdentity, in.OperationID, w, p)
	if err != nil || r.Status != objectediting.StatusUnknown {
		t.Fatalf("status: %#v %v", r, err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "before" {
		t.Fatal("cancelled operation wrote document")
	}
}

func TestObjectEditingMultipleInstancesSerializeCAS(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	second, err := NewObjectEditingFiles(s.receiptRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	in := objectEditingInput(t, s, w, p, "first")
	other := in
	other.Content = "second"
	other.OperationID = "operation-0002"
	var wg sync.WaitGroup
	statuses := make(chan string, 2)
	for i, store := range []*ObjectEditingFiles{s, second} {
		request := in
		if i == 1 {
			request = other
		}
		wg.Add(1)
		go func(store *ObjectEditingFiles, request objectediting.CommitInput) {
			defer wg.Done()
			r, _ := store.Commit(context.Background(), request)
			statuses <- r.Status
		}(store, request)
	}
	wg.Wait()
	close(statuses)
	counts := map[string]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[objectediting.StatusCommitted] != 1 || counts[objectediting.StatusConflict] != 1 {
		t.Fatalf("statuses: %#v", counts)
	}
}

func TestObjectEditingRejectsUnsafePathsAndProtectedMetadata(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	outside := filepath.Join(filepath.Dir(w), "outside.txt")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(w, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(w, "private")
	if err := os.Mkdir(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(protected, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewObjectEditingFiles(s.receiptRoot, []string{protected})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("protected hard link", func(t *testing.T) {
		alias := filepath.Join(w, "alias.txt")
		if err := os.Link(secret, alias); err != nil {
			t.Fatal(err)
		}
		doc, err := s.Read(context.Background(), w, alias)
		if !errors.Is(err, objectediting.ErrForbidden) || doc.Content != "" {
			t.Fatalf("protected hard link: content bytes=%d err=%v", len(doc.Content), err)
		}
	})
	for _, dir := range []string{".git", ".analytix"} {
		if err := os.Mkdir(filepath.Join(w, dir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(w, dir, "private"), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{".git", ".analytix"} {
		if _, err := s.Read(context.Background(), filepath.Join(w, dir), "private"); !errors.Is(err, objectediting.ErrForbidden) {
			t.Fatalf("metadata workspace bypassed protection: %v", err)
		}
	}
	for _, target := range []string{outside, "../outside.txt", link, secret, filepath.Join(w, ".git/private"), filepath.Join(w, ".analytix/private"), w} {
		if _, err := s.Read(context.Background(), w, target); !errors.Is(err, objectediting.ErrForbidden) {
			t.Fatalf("unsafe target rejected incorrectly: %v", err)
		}
	}
	if _, err := s.Read(context.Background(), w, p); err != nil {
		t.Fatal(err)
	}
	parentLink := filepath.Join(w, "parent-link")
	if err := os.Symlink(protected, parentLink); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(context.Background(), w, filepath.Join(parentLink, "secret.txt")); !errors.Is(err, objectediting.ErrForbidden) {
		t.Fatalf("parent symlink: %v", err)
	}
}

func TestAtomicTextDefaultPolicyPreservesHardLinkSupport(t *testing.T) {
	_, w, p := objectEditingFixture(t, []byte("before"))
	alias := filepath.Join(w, "alias.txt")
	if err := os.Link(p, alias); err != nil {
		t.Fatal(err)
	}
	state, err := inspectAtomicTextTargetBounded(alias, false, objectediting.MaxTextBytes)
	if err != nil || !state.Exists || string(state.Content) != "before" {
		t.Fatalf("ordinary hard link read: exists=%v content bytes=%d err=%v", state.Exists, len(state.Content), err)
	}
	if err := atomicReplaceText(atomicTextReplaceRequest{Path: p, Content: []byte("after"), ExpectedExists: true, ExpectedHash: digestAtomicText(state.Content)}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{p: "after", alias: "before"} {
		if content, err := os.ReadFile(path); err != nil || string(content) != want {
			t.Fatalf("ordinary replacement content bytes=%d err=%v", len(content), err)
		}
	}
}

func TestObjectEditingCommitRejectsHardLinkCreatedAfterInspection(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	protected := filepath.Join(filepath.Dir(w), "protected")
	if err := os.Mkdir(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := NewObjectEditingFiles(s.receiptRoot, []string{protected})
	if err != nil {
		t.Fatal(err)
	}
	input := objectEditingInput(t, s, w, p, "after")
	s.replaceDocument = func(request atomicTextReplaceRequest) error {
		if err := os.Link(p, filepath.Join(protected, "secret.txt")); err != nil {
			t.Fatal(err)
		}
		return atomicReplaceText(request)
	}
	if _, err := s.Commit(context.Background(), input); !errors.Is(err, objectediting.ErrForbidden) {
		t.Fatalf("hard link added before replacement: %v", err)
	}
	if content, err := os.ReadFile(p); err != nil || string(content) != "before" {
		t.Fatalf("target changed: content bytes=%d err=%v", len(content), err)
	}
}

func TestObjectEditingRejectsBinaryMalformedAndOversizedText(t *testing.T) {
	for name, raw := range map[string][]byte{"nul": {0, 1, 2, 3}, "invalid-utf8": {0xff, 0x00}, "odd-utf16": {0xff, 0xfe, 0x41}, "unpaired-surrogate": {0xff, 0xfe, 0x00, 0xd8}, "pdf": []byte("%PDF-1.4"), "too-large": bytes.Repeat([]byte("x"), objectediting.MaxTextBytes+1)} {
		t.Run(name, func(t *testing.T) {
			s, w, p := objectEditingFixture(t, raw)
			_, err := s.Read(context.Background(), w, p)
			want := objectediting.ErrNotText
			if name == "too-large" {
				want = objectediting.ErrTooLarge
			}
			if !errors.Is(err, want) {
				t.Fatalf("read: %v", err)
			}
			input := objectediting.CommitInput{Workspace: w, Path: p, BaseRevision: digestAtomicText(raw), Content: "replacement", ObjectIdentity: digestAtomicText([]byte("synthetic identity")), OperationID: "operation-invalid-file"}
			if _, err := s.Commit(context.Background(), input); !errors.Is(err, want) {
				t.Fatalf("commit invalid file: %v", err)
			}
			got, _ := os.ReadFile(p)
			entries, _ := os.ReadDir(s.receiptRoot)
			if !bytes.Equal(got, raw) || len(entries) != 0 {
				t.Fatal("rejected file was modified or journaled")
			}
		})
	}
	s, w, p := objectEditingFixture(t, filetoolsapp.EncodeTextBytes("before", filetoolsapp.TextEncodingUTF16LE))
	in := objectEditingInput(t, s, w, p, strings.Repeat("x", objectediting.MaxTextBytes/2+1))
	if _, err := s.Commit(context.Background(), in); !errors.Is(err, objectediting.ErrTooLarge) {
		t.Fatalf("encoded byte size: %v", err)
	}
	in.Content = "bad\x00text"
	if _, err := s.Commit(context.Background(), in); !errors.Is(err, objectediting.ErrNotText) {
		t.Fatalf("input text: %v", err)
	}
	expanded := filetoolsapp.EncodeTextBytes(strings.Repeat("中", objectediting.MaxTextBytes/3+1), filetoolsapp.TextEncodingUTF16LE)
	s, w, p = objectEditingFixture(t, expanded)
	if _, err := s.Read(context.Background(), w, p); !errors.Is(err, objectediting.ErrTooLarge) {
		t.Fatalf("decoded text size: %v", err)
	}
}

func TestObjectEditingFilesystemAliasesShareReceiptBinding(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	sensitive, known := mutationPathCaseSensitive(w)
	if !known || sensitive {
		t.Skip("requires a case-insensitive filesystem")
	}
	in := objectEditingInput(t, s, w, p, "after")
	r, err := s.Commit(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	aliasWorkspace := filepath.Join(filepath.Dir(w), "WORKSPACE")
	aliasPath := filepath.Join(aliasWorkspace, "REPORT.TXT")
	doc, err := s.Read(context.Background(), aliasWorkspace, aliasPath)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := s.Read(context.Background(), w, p)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Workspace != canonical.Workspace || doc.IdentityPath != canonical.IdentityPath {
		t.Fatal("filesystem aliases have distinct identities")
	}
	replayed, err := s.Status(context.Background(), in.ObjectIdentity, in.OperationID, aliasWorkspace, aliasPath)
	if err != nil || replayed != r {
		t.Fatalf("alias receipt: %#v %v", replayed, err)
	}
}

func TestObjectEditingLastMomentExternalDriftIsNotOverwritten(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	in := objectEditingInput(t, s, w, p, "after")
	s.replaceDocument = func(req atomicTextReplaceRequest) error {
		if err := os.WriteFile(p, []byte("external drift"), 0o640); err != nil {
			return err
		}
		return atomicReplaceText(req)
	}
	r, err := s.Commit(context.Background(), in)
	if !errors.Is(err, objectediting.ErrConflict) || r.Status != objectediting.StatusConflict {
		t.Fatalf("drift: %#v %v", r, err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "external drift" {
		t.Fatal("external drift overwritten")
	}
}

func TestObjectEditingJournalContainsNoDocumentOrPathAndRejectsTampering(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("SYNTHETIC_PRIVATE_ORIGINAL"))
	in := objectEditingInput(t, s, w, p, "SYNTHETIC_PRIVATE_EDITED")
	if _, err := s.Commit(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.receiptRoot)
	if err != nil || len(entries) != 1 {
		t.Fatalf("journal inventory: %v %v", entries, err)
	}
	path := filepath.Join(s.receiptRoot, entries[0].Name())
	raw, _ := os.ReadFile(path)
	for _, forbidden := range []string{"SYNTHETIC_PRIVATE", w, p, "report.txt"} {
		if bytes.Contains(raw, []byte(forbidden)) || strings.Contains(entries[0].Name(), forbidden) {
			t.Fatal("journal contains plaintext")
		}
	}
	for name, tampered := range map[string][]byte{
		"trailing object": append(append([]byte(nil), raw...), []byte("\n{}")...),
		"duplicate key":   bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		"whitespace":      append([]byte(" "), raw...),
		"escaped key":     bytes.Replace(raw, []byte(`"version"`), []byte(`"\u0076ersion"`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, tampered, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Status(context.Background(), in.ObjectIdentity, in.OperationID, w, p); !errors.Is(err, objectediting.ErrPersistence) {
				t.Fatalf("tampered receipt: %v", err)
			}
		})
	}
}

func TestObjectEditingReceiptRootMustStayPrivateAndIdentical(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	root := s.receiptRoot
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewObjectEditingFiles(root, nil); !errors.Is(err, objectediting.ErrForbidden) {
		t.Fatalf("public root: %v", err)
	}
	if _, err := s.Read(context.Background(), w, p); !errors.Is(err, objectediting.ErrForbidden) {
		t.Fatalf("changed permissions: %v", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(context.Background(), w, p); !errors.Is(err, objectediting.ErrForbidden) {
		t.Fatalf("replaced root: %v", err)
	}
	missing := filepath.Join(filepath.Dir(root), "missing", "child")
	if _, err := NewObjectEditingFiles(missing, nil); err == nil {
		t.Fatal("created missing root")
	}
	if _, err := os.Lstat(filepath.Dir(missing)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("constructor created parents")
	}
	link := root + "-alias"
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewObjectEditingFiles(link, nil); !errors.Is(err, objectediting.ErrForbidden) {
		t.Fatalf("symlink root: %v", err)
	}
}

func TestObjectEditingRejectsOperationTokensAndCancelledLock(t *testing.T) {
	s, w, p := objectEditingFixture(t, []byte("before"))
	in := objectEditingInput(t, s, w, p, "after")
	for _, id := range []string{"../escape", "short", "operation with spaces", strings.Repeat("x", 129)} {
		in.OperationID = id
		if _, err := s.Commit(context.Background(), in); !errors.Is(err, objectediting.ErrInvalidInput) {
			t.Fatalf("token: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Read(ctx, w, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}
