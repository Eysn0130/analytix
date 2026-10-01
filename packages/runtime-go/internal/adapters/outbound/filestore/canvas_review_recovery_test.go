//go:build darwin || linux

package filestore

import (
	"context"
	"errors"
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
	identity "analytix.local/runtime-go/internal/domain/identity"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// Recreate every in-memory owner over the same real private filestore. This is
// deterministic restart evidence, not a claim about installed app recovery.
func canvasReviewCore(t *testing.T, files *ObjectEditingFiles, authority *officeTestIdentity, loseReply bool) (*canvasapp.Service, *canvasReceiptBoundary, *canvasRevocableProjector) {
	t.Helper()
	projector := &canvasRevocableProjector{identity: authority}
	registry := managedediting.New(workspacemutation.NewCoordinator(), managededitingfiles.New())
	objects := &canvasReceiptBoundary{Service: objectapp.New(authority, files), loseReply: loseReply}
	core := canvasapp.New(authority, map[string]canvasapp.Objects{"canvas": objects})
	if err := core.BindHost(projector, registry.WithCapture); err != nil {
		t.Fatal(err)
	}
	return core, objects, projector
}
func canvasReviewOpen(t *testing.T, core *canvasapp.Service, p identity.PrincipalV1, workspace, path string) canvasapp.Document {
	t.Helper()
	doc, err := core.Open(context.Background(), p, "thread_main", workspace, path, "canvas")
	if err != nil {
		t.Fatal("open", err)
	}
	return doc
}
func canvasReviewProposal(t *testing.T, core *canvasapp.Service, p identity.PrincipalV1, doc canvasapp.Document) canvasapp.Proposal {
	t.Helper()
	label := "Reviewed display"
	v, err := core.ProposeScene(context.Background(), p, doc.SessionID, doc.ThreadID, doc.Revision, []string{"n1"}, []canvas.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
	if err != nil {
		t.Fatal("propose", err)
	}
	return v
}
func canvasReviewPrincipal(t *testing.T) *officeTestIdentity {
	t.Helper()
	p, err := identity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	return &officeTestIdentity{principal: p}
}

func TestCanvasReviewRestartReconcilesUnknownSaveWithoutReplay(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "unaccepted", true: "lost-save-reply"}[accepted], func(t *testing.T) {
			ctx := context.Background()
			before, _ := canvasEditingBytes(t, "canvas")
			files, workspace, path := canvasEditingFixture(t, "canvas", before)
			authority := canvasReviewPrincipal(t)
			core, objects, _ := canvasReviewCore(t, files, authority, true)
			doc := canvasReviewOpen(t, core, authority.principal, workspace, path)
			proposal := canvasReviewProposal(t, core, authority.principal, doc)
			writes := 0
			files.replaceDocument = func(r atomicTextReplaceRequest) error { writes++; return atomicReplaceText(r) }
			if accepted {
				r, e := core.Apply(ctx, authority.principal, doc.SessionID, doc.ThreadID, proposal.ID)
				if e != nil || r.Status != editing.StatusUnknown || writes != 1 || objects.committed.Status != editing.StatusCommitted {
					t.Fatal("lost reply", r, e, writes)
				}
			} else {
				officeEditingAssertBytes(t, path, before)
			}
			// Do not call Close: drop the full Core/object/Registry owners like a restart.
			reopenedFiles, e := NewCanvasObjectEditingFiles(files.receiptRoot, nil, "canvas")
			if e != nil {
				t.Fatal(e)
			}
			reopenedFiles.replaceDocument = func(r atomicTextReplaceRequest) error { writes++; return atomicReplaceText(r) }
			fresh, _, _ := canvasReviewCore(t, reopenedFiles, authority, false)
			next := canvasReviewOpen(t, fresh, authority.principal, workspace, path)
			reviews, e := fresh.Proposals(ctx, authority.principal, next.SessionID, next.ThreadID)
			if e != nil || len(reviews) != 1 || reviews[0].ID == proposal.ID {
				t.Fatal("new proposal identity", reviews, e)
			}
			if accepted {
				if reviews[0].Status != "applied" {
					t.Fatal("saved intent falsely restored as unapplied", reviews)
				}
				r, e := fresh.ProposalStatus(ctx, authority.principal, next.SessionID, next.ThreadID, reviews[0].ID)
				if e != nil || r != objects.committed || writes != 1 {
					t.Fatal("query changed original write", r, e, writes)
				}
				recovery, e := fresh.Recovery(ctx, authority.principal, next.SessionID, next.ThreadID)
				if e != nil || recovery.Current == nil || !recovery.Current.CanUndo {
					t.Fatal("restart undo", recovery, e)
				}
				undone, e := fresh.RecoverOperation(ctx, authority.principal, next.SessionID, next.ThreadID, "undo", recovery.Current.ChangeID, next.Revision)
				if e != nil || undone.Status != editing.StatusCommitted || writes != 2 {
					t.Fatal("explicit undo", undone, e, writes)
				}
				officeEditingAssertBytes(t, path, before)
			} else {
				if reviews[0].Status != "proposed" || writes != 0 {
					t.Fatal("auto apply on restart", reviews, writes)
				}
				r, e := fresh.Apply(ctx, authority.principal, next.SessionID, next.ThreadID, reviews[0].ID)
				if e != nil || r.Status != editing.StatusCommitted || writes != 1 {
					t.Fatal("fresh explicit acceptance", r, e, writes)
				}
			}
		})
	}
}

func TestCanvasReviewStatusRechecksAuthorityAfterStorage(t *testing.T) {
	for _, transition := range []string{"authorized", "identity", "scope", "cancel"} {
		t.Run(transition, func(t *testing.T) {
			ctx := context.Background()
			before, _ := canvasEditingBytes(t, "canvas")
			files, workspace, path := canvasEditingFixture(t, "canvas", before)
			authority := canvasReviewPrincipal(t)
			principal := authority.principal
			core, objects, projector := canvasReviewCore(t, files, authority, true)
			doc := canvasReviewOpen(t, core, principal, workspace, path)
			proposal := canvasReviewProposal(t, core, principal, doc)
			writes := 0
			files.replaceDocument = func(r atomicTextReplaceRequest) error { writes++; return atomicReplaceText(r) }
			if _, e := core.Apply(ctx, principal, doc.SessionID, doc.ThreadID, proposal.ID); e != nil {
				t.Fatal(e)
			}
			query, cancel := context.WithCancel(ctx)
			defer cancel()
			objects.afterStatus = func() {
				switch transition {
				case "identity":
					authority.principal = identity.PrincipalV1{}
				case "scope":
					projector.revoked = true
				case "cancel":
					cancel()
				}
			}
			receipt, e := core.ProposalStatus(query, principal, doc.SessionID, doc.ThreadID, proposal.ID)
			if writes != 1 || objects.statusCalls != 1 || objects.queriedID != objects.committed.OperationID {
				t.Fatal("status not original read only", writes, objects.statusCalls)
			}
			if transition == "authorized" {
				if e != nil || receipt != objects.committed {
					t.Fatal(receipt, e)
				}
			} else {
				expected := editing.Receipt{OperationID: objects.committed.OperationID, Status: editing.StatusUnknown}
				if !errors.Is(e, canvasapp.ErrUnavailable) || receipt != expected {
					t.Fatal("receipt revealed after revocation", receipt, e)
				}
			}
		})
	}
}

func TestCanvasReviewPersistenceFailureDoesNotExposeOrApplyProposal(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-write", true: "lost-ack"}[lost], func(t *testing.T) {
			before, _ := canvasEditingBytes(t, "canvas")
			files, workspace, path := canvasEditingFixture(t, "canvas", before)
			authority := canvasReviewPrincipal(t)
			core, _, _ := canvasReviewCore(t, files, authority, false)
			doc := canvasReviewOpen(t, core, authority.principal, workspace, path)
			files.replaceJournal = func(r atomicTextReplaceRequest) error {
				if strings.HasPrefix(filepath.Base(r.Path), "canvas-review-") {
					if lost {
						if e := atomicReplaceText(r); e != nil {
							return e
						}
					}
					return errors.New("synthetic durable-review acknowledgement failure")
				}
				return atomicReplaceText(r)
			}
			label := "Reviewed display"
			result, e := core.ProposeScene(context.Background(), authority.principal, doc.SessionID, doc.ThreadID, doc.Revision, []string{"n1"}, []canvas.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
			if e == nil || result.ID != "" {
				t.Fatal("unacknowledged intent exposed", result, e)
			}
			officeEditingAssertBytes(t, path, before)
			freshFiles, e := NewCanvasObjectEditingFiles(files.receiptRoot, nil, "canvas")
			if e != nil {
				t.Fatal(e)
			}
			fresh, _, _ := canvasReviewCore(t, freshFiles, authority, false)
			opened := canvasReviewOpen(t, fresh, authority.principal, workspace, path)
			restored, e := fresh.Proposals(context.Background(), authority.principal, opened.SessionID, opened.ThreadID)
			expected := 0
			if lost {
				expected = 1
			}
			if e != nil || len(restored) != expected {
				t.Fatal("restart did not reflect actual durable bytes", restored, e)
			}
			officeEditingAssertBytes(t, path, before)
		})
	}
}

func TestCanvasReviewPredecessorCASAndPrivateFileRejection(t *testing.T) {
	for _, attack := range []string{"changed-predecessor", "tamper", "symlink", "hardlink", "world-readable"} {
		t.Run(attack, func(t *testing.T) {
			ctx := context.Background()
			before, _ := canvasEditingBytes(t, "canvas")
			files, workspace, path := canvasEditingFixture(t, "canvas", before)
			authority := canvasReviewPrincipal(t)
			core, _, _ := canvasReviewCore(t, files, authority, false)
			doc := canvasReviewOpen(t, core, authority.principal, workspace, path)
			proposal := canvasReviewProposal(t, core, authority.principal, doc)
			records, e := filepath.Glob(filepath.Join(files.receiptRoot, "canvas-review-*.json"))
			if e != nil || len(records) != 1 {
				t.Fatal(records, e)
			}
			record := records[0]
			switch attack {
			case "changed-predecessor":
				// Another valid owner rejects the durable intent, without altering formal bytes.
				other, _, _ := canvasReviewCore(t, files, authority, false)
				d := canvasReviewOpen(t, other, authority.principal, workspace, path)
				ps, e := other.Proposals(ctx, authority.principal, d.SessionID, d.ThreadID)
				if e != nil || len(ps) != 1 {
					t.Fatal(ps, e)
				}
				if e = other.Reject(ctx, authority.principal, d.SessionID, d.ThreadID, ps[0].ID); e != nil {
					t.Fatal(e)
				}
			case "tamper":
				body, e := os.ReadFile(record)
				if e != nil {
					t.Fatal(e)
				}
				body = []byte(strings.Replace(string(body), "Reviewed display", "Injected display", -1))
				if e = os.WriteFile(record, body, 0600); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				moved := record + ".preserved"
				if e = os.Rename(record, moved); e != nil {
					t.Fatal(e)
				}
				if e = os.Symlink(moved, record); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e = os.Link(record, record+".alias"); e != nil {
					t.Fatal(e)
				}
			case "world-readable":
				if e = os.Chmod(record, 0644); e != nil {
					t.Fatal(e)
				}
			}
			if r, e := core.Apply(ctx, authority.principal, doc.SessionID, doc.ThreadID, proposal.ID); e == nil || r.Status == editing.StatusCommitted {
				t.Fatal("invalid successor accepted", r, e)
			}
			officeEditingAssertBytes(t, path, before)
		})
	}
}
