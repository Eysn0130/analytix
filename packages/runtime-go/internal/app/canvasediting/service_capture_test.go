package canvasediting

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/managededitingfiles"
	"analytix.local/runtime-go/internal/app/managedediting"
	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	"analytix.local/runtime-go/internal/app/workspacemutation"
	canvas "analytix.local/runtime-go/internal/domain/canvas"
	identity "analytix.local/runtime-go/internal/domain/identity"
	files "analytix.local/runtime-go/internal/ports/objectediting"
)

// This is a real managed-capture boundary test with a persistence spy. The
// filestore journey separately exercises actual CAS, restart and undo bytes.
type captureObjects struct {
	Objects
	opened objectapp.Opened
	write  func() (files.Receipt, error)
}

func (o *captureObjects) Open(context.Context, string, string) (objectapp.Opened, error) {
	return o.opened, nil
}
func (o *captureObjects) PrepareNativeChange(context.Context, string, files.NativeChangeDraft) (files.NativeChangeStatus, error) {
	return files.NativeChangeStatus{SaveOperationID: "save-1"}, nil
}
func (o *captureObjects) CommitNativeChange(context.Context, string, string, string, string, string, string) (files.Receipt, error) {
	return o.write()
}
func (o *captureObjects) UndoNativeChange(context.Context, string, string, string, string) (files.Receipt, error) {
	return o.write()
}
func (o *captureObjects) ResumeNativeChange(context.Context, string, string, string, string) (files.Receipt, error) {
	return o.write()
}

type captureAuthority struct {
	principal identity.PrincipalV1
	revoked   bool
}

func (a *captureAuthority) ResolveCurrent(context.Context) (identity.PrincipalV1, error) {
	return a.principal, nil
}
func (a *captureAuthority) ValidateCurrent(_ context.Context, p identity.PrincipalV1) error {
	if a.revoked || !identity.SamePrincipalV1(a.principal, p) {
		return ErrUnavailable
	}
	return nil
}

type captureProjector struct{ authority *captureAuthority }

func (p captureProjector) ValidateCurrent(ctx context.Context, in objectapp.ScopeAuthority) error {
	return p.authority.ValidateCurrent(ctx, in.Principal)
}
func (p captureProjector) AuthorizeAndProject(ctx context.Context, in objectapp.ProjectionInput) ([]objectapp.ProtectedRange, error) {
	return nil, p.ValidateCurrent(ctx, in.ScopeAuthority)
}

func TestCanvasMutationManagedCapture(t *testing.T) {
	for _, action := range []string{"apply", "undo", "resume"} {
		for _, condition := range []string{"authorized", "wrong-thread", "stale-revision", "revoked", "hardlink", "write-error"} {
			t.Run(action+"/"+condition, func(t *testing.T) {
				ctx := context.Background()
				workspace, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(workspace, "scene.canvas")
				raw := []byte(`{"schemaVersion":1,"facts":{"nodes":[{"id":"n1","label":"Original fact","attributes":{},"sources":[],"assumption":true}],"edges":[]},"presentation":{"nodes":[{"id":"n1","layout":{"x":0,"y":0,"width":120,"height":60},"displayLabel":"Original view","style":{"fill":"#ffffff","stroke":"#000000","strokeWidth":1,"dash":"solid","shape":"rounded"}}],"edges":[]}}`)
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				principal, err := identity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
				if err != nil {
					t.Fatal(err)
				}
				authority := &captureAuthority{principal: principal}
				coordinator := workspacemutation.NewCoordinator()
				registry := managedediting.New(coordinator, managededitingfiles.New())
				calls := 0
				writeFailure := errors.New("synthetic persistence failure")
				objects := &captureObjects{opened: objectapp.Opened{SessionID: "object-session", ObjectID: strings.Repeat("b", 64), Path: path, Revision: digest(raw), Content: base64.StdEncoding.EncodeToString(raw)}}
				objects.write = func() (files.Receipt, error) {
					calls++
					if !registry.HasCaptures() {
						t.Fatal("write without capture")
					}
					unlock, err := coordinator.Acquire(ctx)
					if err != nil {
						t.Fatal(err)
					}
					blocked := registry.CheckMutation(path)
					unlock()
					if !errors.Is(blocked, managedediting.ErrMutationBlocked) {
						t.Fatal("wrong captured object", blocked)
					}
					if condition == "write-error" {
						return files.Receipt{}, writeFailure
					}
					return files.Receipt{OperationID: "save-1", Status: files.StatusCommitted}, nil
				}
				service := New(authority, map[string]Objects{"canvas": objects})
				if err = service.BindHost(captureProjector{authority}, registry.WithCapture); err != nil {
					t.Fatal(err)
				}
				doc, err := service.Open(ctx, principal, "thread_main", workspace, path, "canvas")
				if err != nil {
					t.Fatal(err)
				}
				label := "New view"
				proposal, err := service.ProposeScene(ctx, principal, doc.SessionID, doc.ThreadID, doc.Revision, []string{"n1"}, []canvas.Operation{{Kind: "set-display-label", Target: "node", ID: "n1", DisplayLabel: &label}})
				if err != nil {
					t.Fatal(err)
				}
				if calls != 0 || registry.HasCaptures() {
					t.Fatal("proposal performed a write or retained a write lease")
				}
				thread := doc.ThreadID
				switch condition {
				case "wrong-thread":
					thread = "thread_other"
				case "stale-revision":
					changed := append(append([]byte{}, raw...), '\n')
					objects.opened.Content = base64.StdEncoding.EncodeToString(changed)
					objects.opened.Revision = digest(changed)
				case "revoked":
					authority.revoked = true
				case "hardlink":
					if err = os.Link(path, filepath.Join(workspace, "alias.canvas")); err != nil {
						t.Fatal(err)
					}
				}
				var receipt files.Receipt
				if action == "apply" {
					receipt, err = service.Apply(ctx, principal, doc.SessionID, thread, proposal.ID)
				} else {
					receipt, err = service.RecoverOperation(ctx, principal, doc.SessionID, thread, action, "change-1", doc.Revision)
				}
				if registry.HasCaptures() {
					t.Fatal("capture leaked after mutation returned")
				}
				switch condition {
				case "authorized":
					if err != nil || calls != 1 || receipt.Status != files.StatusCommitted {
						t.Fatal("authorized mutation blocked", receipt, err, calls)
					}
				case "stale-revision":
					if !errors.Is(err, ErrStale) || calls != 0 {
						t.Fatal("stale revision reached persistence", err, calls)
					}
				case "write-error":
					if !errors.Is(err, writeFailure) || calls != 1 {
						t.Fatal("write failure lost", err, calls)
					}
				default:
					if err == nil || calls != 0 {
						t.Fatal("invalid mutation reached persistence", err, calls)
					}
				}
			})
		}
	}
}
