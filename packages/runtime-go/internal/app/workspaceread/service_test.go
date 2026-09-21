package workspaceread

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	fileport "analytix.local/runtime-go/internal/ports/workspaceread"
)

type testAuthority struct {
	scope   Scope
	err     error
	calls   int
	current func()
}

func (a *testAuthority) Current(context.Context, string) (Scope, error) {
	a.calls++
	if a.current != nil {
		a.current()
	}
	return a.scope, a.err
}

type testFiles struct {
	root            fileport.Root
	inspects, scans int
	inspect         func()
	scan            func(context.Context, fileport.Root, bool, time.Duration, func() error) ([]fileport.File, error)
}

func (f *testFiles) InspectRoot(context.Context, string) (fileport.Root, error) {
	f.inspects++
	if f.inspect != nil {
		f.inspect()
	}
	return f.root, nil
}
func (f *testFiles) Scan(ctx context.Context, root fileport.Root, pdf bool, budget time.Duration, check func() error) ([]fileport.File, error) {
	f.scans++
	return f.scan(ctx, root, pdf, budget, check)
}

func testService(t *testing.T) (*Service, *testAuthority, *testFiles) {
	t.Helper()
	a := &testAuthority{scope: Scope{ThreadID: "thread-1", Workspace: "/workspace", Binding: "current-principal-policy"}}
	f := &testFiles{root: fileport.Root{Workspace: "/workspace", Identity: "device:inode", Policy: "protected-policy"}}
	s := &Service{Files: f}
	if err := s.BindAuthority(a); err != nil {
		t.Fatal(err)
	}
	return s, a, f
}

func testAuthorize(t *testing.T, s *Service) Snapshot {
	t.Helper()
	snapshot, err := s.Read(context.Background(), Request{Action: "authorize", ThreadID: "thread-1"})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestWorkspaceReadPreservesThreadAliasWhileBindingCanonicalRoot(t *testing.T) {
	s, a, f := testService(t)
	a.scope.Workspace = "/tmp/fixture"
	f.root.Workspace = "/private/tmp/fixture"
	snapshot := testAuthorize(t, s)
	if snapshot.Workspace != a.scope.Workspace {
		t.Fatal("lost trusted thread spelling")
	}
	f.scan = func(_ context.Context, root fileport.Root, _ bool, _ time.Duration, check func() error) ([]fileport.File, error) {
		if root.Workspace != f.root.Workspace {
			t.Fatal("scan did not use canonical root")
		}
		return nil, check()
	}
	if _, err := s.Read(context.Background(), Request{Action: "scan", ThreadID: a.scope.ThreadID, Binding: snapshot.Binding}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceReadServiceAuthorizeValidateScan(t *testing.T) {
	s, a, f := testService(t)
	f.inspect = func() {
		if a.calls == 0 {
			t.Fatal("file IO before authority")
		}
	}
	authorized := testAuthorize(t, s)
	if len(authorized.Binding) != 64 || authorized.ThreadID != a.scope.ThreadID || authorized.Workspace != f.root.Workspace || authorized.Files != nil {
		t.Fatalf("unexpected authorize: %+v", authorized)
	}
	validated, err := s.Read(context.Background(), Request{Action: "validate", ThreadID: "thread-1", Binding: authorized.Binding})
	if err != nil || !reflect.DeepEqual(validated, authorized) || f.scans != 0 {
		t.Fatal("validate did not preserve binding")
	}
	for _, pdf := range []bool{false, true} {
		want := []fileport.File{{Path: "document.md", Kind: "text", Revision: strings.Repeat("a", 64), Content: []byte("content")}}
		f.scan = func(ctx context.Context, root fileport.Root, includePDF bool, budget time.Duration, check func() error) ([]fileport.File, error) {
			wantBudget := 250 * time.Millisecond
			if pdf {
				wantBudget = 2500 * time.Millisecond
			}
			if root != f.root || includePDF != pdf || budget != wantBudget {
				t.Fatal("wrong scan scope or limits")
			}
			if err := check(); err != nil {
				t.Fatal(err)
			}
			return want, nil
		}
		got, err := s.Read(context.Background(), Request{Action: "scan", ThreadID: "thread-1", Binding: authorized.Binding, IncludePDF: pdf})
		if err != nil || !reflect.DeepEqual(got.Files, want) || got.Binding != authorized.Binding {
			t.Fatal("scan did not return bound files")
		}
	}
}

func TestWorkspaceReadServiceRejectsBeforeIO(t *testing.T) {
	for _, request := range []Request{
		{Action: "unknown", ThreadID: "thread-1"}, {Action: "authorize", ThreadID: "../thread"},
		{Action: "authorize", ThreadID: "thread-1", Binding: "supplied"}, {Action: "authorize", ThreadID: "thread-1", IncludePDF: true},
		{Action: "validate", ThreadID: "thread-1", Binding: strings.Repeat("g", 64)}, {Action: "scan", ThreadID: "thread-1", Binding: strings.Repeat("A", 64)},
		{Action: "scan", ThreadID: "thread-1", Binding: " " + strings.Repeat("a", 64)},
	} {
		s, a, f := testService(t)
		got, err := s.Read(context.Background(), request)
		if err == nil || !reflect.DeepEqual(got, Snapshot{}) || a.calls != 0 || f.inspects != 0 || f.scans != 0 {
			t.Fatalf("invalid request performed effects: %+v", request)
		}
	}
	s, a, f := testService(t)
	a.err = fileport.ErrUnavailable
	if _, err := s.Read(context.Background(), Request{Action: "authorize", ThreadID: "thread-1"}); err == nil || f.inspects != 0 {
		t.Fatal("unauthorized request performed file IO")
	}
	a.err = nil
	a.scope.ThreadID = "another-thread"
	if _, err := s.Read(context.Background(), Request{Action: "authorize", ThreadID: "thread-1"}); err == nil || f.inspects != 0 {
		t.Fatal("cross-thread authority performed file IO")
	}
	if s.BindAuthority(a) == nil {
		t.Fatal("authority rebound")
	}
}

func TestWorkspaceReadServiceInvalidatesChangedBindings(t *testing.T) {
	for _, change := range []string{"principal", "workspace", "identity", "policy"} {
		t.Run(change, func(t *testing.T) {
			s, a, f := testService(t)
			authorized := testAuthorize(t, s)
			switch change {
			case "principal":
				a.scope.Binding = "revoked"
			case "workspace":
				a.scope.Workspace = "/elsewhere"
			case "identity":
				f.root.Identity = "replacement"
			case "policy":
				f.root.Policy = "new-policy"
			}
			for _, action := range []string{"validate", "scan"} {
				got, err := s.Read(context.Background(), Request{Action: action, ThreadID: "thread-1", Binding: authorized.Binding})
				if err == nil || !reflect.DeepEqual(got, Snapshot{}) || f.scans != 0 {
					t.Fatal("stale binding admitted")
				}
			}
		})
	}
}

func TestWorkspaceReadServiceRechecksDuringAndAfterScan(t *testing.T) {
	for _, change := range []string{"callback-revocation", "after-scan-revocation", "after-scan-root", "scan-error", "cancel"} {
		t.Run(change, func(t *testing.T) {
			s, a, f := testService(t)
			authorized := testAuthorize(t, s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.scan = func(_ context.Context, _ fileport.Root, _ bool, _ time.Duration, check func() error) ([]fileport.File, error) {
				switch change {
				case "callback-revocation":
					a.err = fileport.ErrUnavailable
					before := f.inspects
					if check() == nil || f.inspects != before {
						t.Fatal("revocation callback did file IO or accepted authority")
					}
				case "after-scan-revocation":
					a.scope.Binding = "revoked"
				case "after-scan-root":
					f.root.Identity = "replacement"
				case "cancel":
					cancel()
				case "scan-error":
					return []fileport.File{{Content: []byte("partial")}}, fileport.ErrUnavailable
				}
				return []fileport.File{{Content: []byte("must not leak")}}, nil
			}
			got, err := s.Read(ctx, Request{Action: "scan", ThreadID: "thread-1", Binding: authorized.Binding})
			if err == nil || !reflect.DeepEqual(got, Snapshot{}) {
				t.Fatal("invalid scan returned partial data")
			}
		})
	}
}

func TestWorkspaceReadServiceRechecksBeforeScan(t *testing.T) {
	s, a, f := testService(t)
	authorized := testAuthorize(t, s)
	revokeAt := a.calls + 2
	a.current = func() {
		if a.calls == revokeAt {
			a.err = fileport.ErrUnavailable
		}
	}
	before := f.inspects
	got, err := s.Read(context.Background(), Request{Action: "scan", ThreadID: "thread-1", Binding: authorized.Binding})
	if err == nil || !reflect.DeepEqual(got, Snapshot{}) || f.scans != 0 || f.inspects != before+1 {
		t.Fatal("authority changed before scan was admitted or performed subsequent file IO")
	}
}
