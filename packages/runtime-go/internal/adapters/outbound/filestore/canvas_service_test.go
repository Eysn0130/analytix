//go:build darwin || linux

package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/adapters/outbound/managededitingfiles"
	canvasapp "analytix.local/runtime-go/internal/app/canvasediting"
	"analytix.local/runtime-go/internal/app/managedediting"
	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	"analytix.local/runtime-go/internal/app/workspacemutation"
	canvas "analytix.local/runtime-go/internal/domain/canvas"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type canvasRevocableProjector struct {
	identity *officeTestIdentity
	revoked  bool
}

func (p *canvasRevocableProjector) ValidateCurrent(ctx context.Context, in objectapp.ScopeAuthority) error {
	if p.revoked {
		return objectapp.ErrProjection
	}
	return (officeRecoveryTestProjector{p.identity}).ValidateCurrent(ctx, in)
}
func (p *canvasRevocableProjector) AuthorizeAndProject(ctx context.Context, in objectapp.ProjectionInput) ([]objectapp.ProtectedRange, error) {
	return nil, p.ValidateCurrent(ctx, in.ScopeAuthority)
}

type canvasRecoveryRevoker struct {
	*objectapp.Service
	after func()
}

func (o *canvasRecoveryRevoker) NativeRecovery(ctx context.Context, id, thread string) (fileport.NativeRecovery, error) {
	r, e := o.Service.NativeRecovery(ctx, id, thread)
	if o.after != nil {
		o.after()
	}
	return r, e
}
func (o *canvasRecoveryRevoker) CancelNativeChange(ctx context.Context, id, thread, change, base string) (fileport.NativeRecovery, error) {
	r, e := o.Service.CancelNativeChange(ctx, id, thread, change, base)
	if o.after != nil {
		o.after()
	}
	return r, e
}

func TestCanvasServiceFailedOpenAndBrokenCloseReleaseCapacity(t *testing.T) {
	ctx := context.Background()
	before, _ := canvasEditingBytes(t, "canvas")
	files, workspace, path := canvasEditingFixture(t, "canvas", before)
	principal, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	authority := &officeTestIdentity{principal: principal}
	service := canvasapp.New(authority, map[string]canvasapp.Objects{"canvas": objectapp.New(authority, files)})
	if err := service.BindHost(officeRecoveryTestProjector{authority}, func(_ context.Context, _, _ string, validate func() error) (func(), error) {
		return func() {}, validate()
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 65; i++ {
		p := filepath.Join(workspace, fmt.Sprintf("rejected-%d.canvas", i))
		if err := os.WriteFile(p, before, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Open(ctx, principal, "wrong-thread", workspace, p, "canvas"); err == nil {
			t.Fatal("wrong-thread admitted")
		}
	}
	doc, err := service.Open(ctx, principal, "thread_main", workspace, path, "canvas")
	if err != nil {
		t.Fatal("rejected opens leaked sessions", err)
	}
	if err = os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = service.Close(ctx, principal, doc.SessionID, doc.ThreadID); err != nil {
		t.Fatal("broken file could not close", err)
	}
	if _, err = service.Read(ctx, principal, doc.SessionID, doc.ThreadID); err == nil {
		t.Fatal("closed session remained")
	}
}

func TestCanvasServiceRevalidatesRecoveryBoundaries(t *testing.T) {
	for _, operation := range []string{"undo", "recovery", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			before, _ := canvasEditingBytes(t, "canvas")
			files, workspace, path := canvasEditingFixture(t, "canvas", before)
			principal, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
			authority := &officeTestIdentity{principal: principal}
			projector := &canvasRevocableProjector{identity: authority}
			objects := &canvasRecoveryRevoker{Service: objectapp.New(authority, files)}
			service := canvasapp.New(authority, map[string]canvasapp.Objects{"canvas": objects})
			revokeAfterCapture := false
			if err := service.BindHost(projector, func(_ context.Context, _, _ string, validate func() error) (func(), error) {
				err := validate()
				if revokeAfterCapture {
					projector.revoked = true
				}
				return func() {}, err
			}); err != nil {
				t.Fatal(err)
			}
			doc, err := service.Open(ctx, principal, "thread_main", workspace, path, "canvas")
			if err != nil {
				t.Fatal(err)
			}
			label := "Saved"
			proposal, err := service.ProposeScene(ctx, principal, doc.SessionID, doc.ThreadID, doc.Revision, []string{"n1"}, []canvas.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := service.Apply(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID)
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := service.Recovery(ctx, principal, doc.SessionID, doc.ThreadID)
			if err != nil || recovery.Current == nil {
				t.Fatal(err)
			}
			if operation == "undo" {
				revokeAfterCapture = true
				if _, err = service.RecoverOperation(ctx, principal, doc.SessionID, doc.ThreadID, "undo", recovery.Current.ChangeID, receipt.Revision); err == nil {
					t.Fatal("revoked undo succeeded")
				}
				officeEditingAssertBytes(t, path, after)
			} else {
				objects.after = func() { projector.revoked = true }
				var result fileport.NativeRecovery
				if operation == "recovery" {
					result, err = service.Recovery(ctx, principal, doc.SessionID, doc.ThreadID)
				} else {
					opened, openErr := objects.Open(ctx, workspace, path)
					if openErr != nil {
						t.Fatal(openErr)
					}
					next := fileport.NativeChangeDraft{ChangeID: strings.Repeat("b", 64), ThreadID: doc.ThreadID, ProposalID: strings.Repeat("b", 48), BaseRevision: receipt.Revision, BeforeText: "Prior saved version", AfterText: "Uncommitted change"}
					if _, prepareErr := objects.PrepareNativeChange(ctx, opened.SessionID, next); prepareErr != nil {
						t.Fatal(prepareErr)
					}
					result, err = service.Cancel(ctx, principal, doc.SessionID, doc.ThreadID, next.ChangeID, receipt.Revision)
				}
				if err == nil || result.Current != nil || result.Pending != nil {
					t.Fatal("revoked recovery result escaped", result, err)
				}
			}
		})
	}
}

func TestCanvasServiceReviewApplyRestartUndo(t *testing.T) {
	for _, kind := range []string{"canvas", "png"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			before, _ := canvasEditingBytes(t, kind)
			files, workspace, path := canvasEditingFixture(t, kind, before)
			principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
			if err != nil {
				t.Fatal(err)
			}
			authority := &officeTestIdentity{principal: principal}
			registry := managedediting.New(workspacemutation.NewCoordinator(), managededitingfiles.New())
			writes := 0
			bind := func() *canvasapp.Service {
				files.replaceDocument = func(request atomicTextReplaceRequest) error {
					if !registry.HasCaptures() {
						t.Fatal("mutation lacked managed capture")
					}
					writes++
					return atomicReplaceText(request)
				}
				service := canvasapp.New(authority, map[string]canvasapp.Objects{kind: objectapp.New(authority, files)})
				// Use the production registry: a path in its session-ID argument
				// must fail rather than being accepted by a permissive test callback.
				err := service.BindHost(officeRecoveryTestProjector{authority}, registry.WithCapture)
				if err != nil {
					t.Fatal(err)
				}
				return service
			}
			service := bind()
			if _, err := service.Open(ctx, principal, "wrong-thread", workspace, path, kind); err == nil {
				t.Fatal("wrong thread opened")
			}
			doc, err := service.Open(ctx, principal, "thread_main", workspace, path, kind)
			if err != nil {
				t.Fatal(err)
			}
			var proposal canvasapp.Proposal
			if kind == "canvas" {
				label := "展示修改"
				if _, err := service.ProposeScene(ctx, principal, doc.SessionID, doc.ThreadID, doc.Revision, []string{"unknown"}, []canvas.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}}); err == nil {
					t.Fatal("selection escape")
				}
				proposal, err = service.ProposeScene(ctx, principal, doc.SessionID, doc.ThreadID, doc.Revision, []string{"n1"}, []canvas.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
			} else {
				proposal, err = service.ProposeImage(ctx, principal, doc.SessionID, doc.ThreadID, doc.Revision, []canvas.ImageOperation{{Kind: "crop", Region: &canvas.Region{X: 1, Y: 1, Width: 4, Height: 3}}})
			}
			if err != nil {
				t.Fatal("proposal", err)
			}
			if writes != 0 {
				t.Fatal("proposal wrote original")
			}
			officeEditingAssertBytes(t, path, before)
			// Mutating the returned view cannot mutate Core's retained review.
			if kind == "canvas" {
				proposal.SceneDiff[0].FactLabel = "tampered"
			}
			read, err := service.Proposal(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID)
			if err != nil || kind == "canvas" && read.SceneDiff[0].FactLabel == "tampered" {
				t.Fatal("view aliased Core", err)
			}
			receipt, err := service.Apply(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID)
			if err != nil || receipt.Status != fileport.StatusCommitted || writes != 1 || registry.HasCaptures() {
				t.Fatal("apply", receipt, err, writes, registry.HasCaptures())
			}
			replay, err := service.Apply(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID)
			if err != nil || replay != receipt || writes != 1 {
				t.Fatal("duplicate apply", err, writes)
			}
			files, err = NewCanvasObjectEditingFiles(files.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			service = bind()
			doc, err = service.Open(ctx, principal, "thread_main", workspace, path, kind)
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := service.Recovery(ctx, principal, doc.SessionID, doc.ThreadID)
			if err != nil || recovery.Current == nil || !recovery.Current.CanUndo {
				t.Fatal("restart recovery", recovery, err)
			}
			undo, err := service.RecoverOperation(ctx, principal, doc.SessionID, doc.ThreadID, "undo", recovery.Current.ChangeID, doc.Revision)
			if err != nil || undo.Status != fileport.StatusCommitted || writes != 2 || registry.HasCaptures() {
				t.Fatal("undo", undo, err)
			}
			officeEditingAssertBytes(t, path, before)
			if err = service.Close(ctx, principal, doc.SessionID, doc.ThreadID); err != nil {
				t.Fatal("close", err)
			}
		})
	}
}

func TestCanvasServicePreservesExternalChangeAndRevokedIdentity(t *testing.T) {
	ctx := context.Background()
	before, after := canvasEditingBytes(t, "canvas")
	files, workspace, path := canvasEditingFixture(t, "canvas", before)
	principal, _ := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	authority := &officeTestIdentity{principal: principal}
	service := canvasapp.New(authority, map[string]canvasapp.Objects{"canvas": objectapp.New(authority, files)})
	if _, err := service.Open(ctx, principal, "thread_main", workspace, path, "canvas"); err == nil {
		t.Fatal("unbound host accepted")
	}
	if err := service.BindHost(officeRecoveryTestProjector{authority}, func(_ context.Context, _, _ string, validate func() error) (func(), error) {
		return func() {}, validate()
	}); err != nil {
		t.Fatal(err)
	}
	doc, err := service.Open(ctx, principal, "thread_main", workspace, path, "canvas")
	if err != nil {
		t.Fatal(err)
	}
	label := "提案"
	proposal, err := service.ProposeScene(ctx, principal, doc.SessionID, doc.ThreadID, doc.Revision, []string{"n1"}, []canvas.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, after, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Apply(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID); !errors.Is(err, canvasapp.ErrStale) {
		t.Fatal("external change not rejected", err)
	}
	officeEditingAssertBytes(t, path, after)
	authority.principal, _ = identitydomain.NewPrincipalV1(strings.Repeat("b", 64), "other", "other")
	if _, err = service.Read(ctx, principal, doc.SessionID, doc.ThreadID); err == nil {
		t.Fatal("revoked principal read")
	}
	officeEditingAssertBytes(t, path, after)
}
