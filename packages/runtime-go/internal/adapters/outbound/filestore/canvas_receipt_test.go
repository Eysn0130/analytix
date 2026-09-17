//go:build darwin || linux

package filestore

import (
	"context"
	"errors"
	"os"
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

// All storage, CAS, receipt and capture work is real. The wrapper models a lost
// reply and an authority transition after storage has returned its receipt.
type canvasReceiptBoundary struct {
	*objectapp.Service
	loseReply   bool
	afterStatus func()
	committed   fileport.Receipt
	statusCalls int
	queriedID   string
}

func (o *canvasReceiptBoundary) CommitNativeChange(ctx context.Context, session, thread, change, operation, base, content string) (fileport.Receipt, error) {
	r, err := o.Service.CommitNativeChange(ctx, session, thread, change, operation, base, content)
	o.committed = r
	if err == nil && o.loseReply {
		return fileport.Receipt{OperationID: r.OperationID, Status: fileport.StatusUnknown}, nil
	}
	return r, err
}

func (o *canvasReceiptBoundary) Status(ctx context.Context, session, operation string) (fileport.Receipt, error) {
	o.statusCalls++
	o.queriedID = operation
	r, err := o.Service.Status(ctx, session, operation)
	if o.afterStatus != nil {
		o.afterStatus()
	}
	return r, err
}

func TestCanvasReplayReceiptRevalidatesAuthority(t *testing.T) {
	for _, state := range []string{"applied", "pending"} {
		for _, transition := range []string{"authorized", "scope-revoked", "identity-revoked", "cancelled"} {
			t.Run(state+"/"+transition, func(t *testing.T) {
				ctx := context.Background()
				before, _ := canvasEditingBytes(t, "canvas")
				files, workspace, path := canvasEditingFixture(t, "canvas", before)
				principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
				if err != nil {
					t.Fatal(err)
				}
				authority := &officeTestIdentity{principal: principal}
				projector := &canvasRevocableProjector{identity: authority}
				registry := managedediting.New(workspacemutation.NewCoordinator(), managededitingfiles.New())
				writes := 0
				files.replaceDocument = func(request atomicTextReplaceRequest) error {
					if !registry.HasCaptures() {
						t.Fatal("write without managed capture")
					}
					writes++
					return atomicReplaceText(request)
				}
				objects := &canvasReceiptBoundary{Service: objectapp.New(authority, files), loseReply: state == "pending"}
				service := canvasapp.New(authority, map[string]canvasapp.Objects{"canvas": objects})
				if err = service.BindHost(projector, registry.WithCapture); err != nil {
					t.Fatal(err)
				}
				doc, err := service.Open(ctx, principal, "thread_main", workspace, path, "canvas")
				if err != nil {
					t.Fatal(err)
				}
				label := "Reviewed display"
				proposal, err := service.ProposeScene(ctx, principal, doc.SessionID, doc.ThreadID, doc.Revision, []string{"n1"}, []canvas.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
				if err != nil {
					t.Fatal(err)
				}
				first, err := service.Apply(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID)
				if err != nil || writes != 1 || objects.committed.Status != fileport.StatusCommitted {
					t.Fatal("initial save", first, err, writes)
				}
				if (first.Status == fileport.StatusUnknown) != objects.loseReply {
					t.Fatal("lost-reply setup", first)
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				queryCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				objects.afterStatus = func() {
					switch transition {
					case "scope-revoked":
						projector.revoked = true
					case "identity-revoked":
						authority.principal = identitydomain.PrincipalV1{}
					case "cancelled":
						cancel()
					}
				}
				replay, err := service.Apply(queryCtx, principal, doc.SessionID, doc.ThreadID, proposal.ID)
				if objects.statusCalls != 1 || objects.queriedID != objects.committed.OperationID || writes != 1 || registry.HasCaptures() {
					t.Fatal("replay did not query only the original operation", objects.statusCalls, objects.queriedID, writes)
				}
				if transition == "authorized" {
					if err != nil || replay != objects.committed {
						t.Fatal("authorized receipt unavailable", replay, err)
					}
				} else {
					expected := fileport.Receipt{OperationID: objects.committed.OperationID, Status: fileport.StatusUnknown}
					if !errors.Is(err, canvasapp.ErrUnavailable) || replay != expected {
						t.Fatal("receipt escaped after authority transition", replay, err)
					}
				}
				officeEditingAssertBytes(t, path, after)
				// Fresh authorization can resolve the same operation, never issue a
				// second write. This does not restore permission to edit the file.
				projector.revoked, authority.principal, objects.afterStatus = false, principal, nil
				resolved, err := service.Apply(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID)
				if err != nil || resolved != objects.committed || objects.statusCalls != 2 || writes != 1 {
					t.Fatal("receipt recovery reapplied or changed the operation", resolved, err, writes)
				}
				officeEditingAssertBytes(t, path, after)
			})
		}
	}
}
