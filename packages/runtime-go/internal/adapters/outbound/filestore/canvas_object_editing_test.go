//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	canvasdomain "analytix.local/runtime-go/internal/domain/canvas"
	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

func canvasEditingBytes(t *testing.T, kind string) ([]byte, []byte) {
	t.Helper()
	if kind == "png" {
		pixels := image.NewNRGBA(image.Rect(0, 0, 8, 6))
		pixels.SetNRGBA(2, 1, color.NRGBA{R: 123, A: 255})
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, pixels); err != nil {
			t.Fatal(err)
		}
		changed, err := canvasdomain.TransformImage(encoded.Bytes(), []canvasdomain.ImageOperation{{Kind: "crop", Region: &canvasdomain.Region{X: 1, Y: 1, Width: 4, Height: 3}}})
		if err != nil {
			t.Fatal(err)
		}
		return encoded.Bytes(), changed.PNG
	}
	before := []byte(`{"schemaVersion":1,"facts":{"nodes":[{"id":"n1","label":"保留事实","attributes":{"amount":"100.00"},"sources":[],"assumption":true}],"edges":[]},"presentation":{"nodes":[{"id":"n1","layout":{"x":0,"y":0,"width":120,"height":60},"displayLabel":"原始标签","style":{"fill":"#ffffff","stroke":"#000000","strokeWidth":1,"dash":"solid","shape":"rounded"}}],"edges":[]}}`)
	scene, err := canvasdomain.ParseScene(before)
	if err != nil {
		t.Fatal(err)
	}
	label := "修改展示"
	changed, err := canvasdomain.Apply(scene, []canvasdomain.Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(changed.Scene)
	if err != nil {
		t.Fatal(err)
	}
	return before, after
}

func canvasEditingFixture(t *testing.T, kind string, before []byte) (*ObjectEditingFiles, string, string) {
	t.Helper()
	text, workspace, previous := objectEditingFixture(t, before)
	path := filepath.Join(workspace, "object."+kind)
	if err := os.Rename(previous, path); err != nil {
		t.Fatal(err)
	}
	store, err := NewCanvasObjectEditingFiles(text.receiptRoot, nil, kind)
	if err != nil {
		t.Fatal(err)
	}
	return store, workspace, path
}

func TestCanvasObjectsUseNativeSaveRestartAndUndo(t *testing.T) {
	for _, kind := range []string{"canvas", "png"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			before, after := canvasEditingBytes(t, kind)
			store, workspace, path := canvasEditingFixture(t, kind, before)
			input := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString(after))
			draft := editing.NativeChangeDraft{ChangeID: strings.Repeat("c", 64), ThreadID: "canvas-thread", ProposalID: strings.Repeat("a", 48), BaseRevision: input.BaseRevision, BeforeText: "Original", AfterText: "Reviewed local change"}
			prepared, err := store.PrepareNativeChange(ctx, editing.NativeChangeInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, Draft: draft})
			if err != nil {
				t.Fatal("prepare", err)
			}
			officeEditingAssertBytes(t, path, before)
			input.OperationID = prepared.SaveOperationID
			receipt, err := store.CommitNativeChange(ctx, editing.NativeCommitInput{CommitInput: input, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID})
			if err != nil || receipt.Status != editing.StatusCommitted {
				t.Fatal("commit", receipt, err)
			}
			officeEditingAssertBytes(t, path, after)
			restarted, err := NewCanvasObjectEditingFiles(store.receiptRoot, nil, kind)
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := restarted.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, draft.ThreadID)
			if err != nil || recovery.Current == nil || !recovery.Current.CanUndo {
				t.Fatal("recovery", recovery, err)
			}
			hidden, err := restarted.NativeRecovery(ctx, input.ObjectIdentity, workspace, path, "another-thread")
			if err != nil || hidden.Current != nil || hidden.Pending != nil {
				t.Fatal("thread isolation", err)
			}
			undo := editing.NativeUndoInput{Workspace: workspace, Path: path, ObjectIdentity: input.ObjectIdentity, ThreadID: draft.ThreadID, ChangeID: draft.ChangeID, BaseRevision: receipt.Revision}
			result, err := restarted.UndoNativeChange(ctx, undo)
			if err != nil || result.Status != editing.StatusCommitted {
				t.Fatal("undo", result, err)
			}
			officeEditingAssertBytes(t, path, before)
			if _, err = restarted.UndoNativeChange(ctx, undo); err != nil {
				t.Fatal("undo replay", err)
			}
			officeEditingAssertBytes(t, path, before)
		})
	}
}

func TestCanvasObjectsRejectMalformedCandidatesAndPathAliases(t *testing.T) {
	for _, kind := range []string{"canvas", "png"} {
		t.Run(kind, func(t *testing.T) {
			before, _ := canvasEditingBytes(t, kind)
			store, workspace, path := canvasEditingFixture(t, kind, before)
			if _, err := NewOfficeObjectEditingFiles(store.receiptRoot, nil, kind); !errors.Is(err, editing.ErrInvalidInput) {
				t.Fatal("Office accepted Canvas kind", err)
			}
			input := objectEditingInput(t, store, workspace, path, base64.StdEncoding.EncodeToString([]byte(`{"schemaVersion":1}`)))
			if _, err := store.Commit(context.Background(), input); !errors.Is(err, editing.ErrNotText) {
				t.Fatal("invalid payload accepted", err)
			}
			officeEditingAssertBytes(t, path, before)
			alias := filepath.Join(workspace, "alias."+kind)
			if err := os.Link(path, alias); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Read(context.Background(), workspace, path); err == nil {
				t.Fatal("hardlinked original admitted")
			}
			if _, err := store.Read(context.Background(), workspace, alias); err == nil {
				t.Fatal("hardlinked alias admitted")
			}
			officeEditingAssertBytes(t, path, before)
		})
	}
}
