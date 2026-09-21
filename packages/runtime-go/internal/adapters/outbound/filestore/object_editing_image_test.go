//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func imageFixtureBytes(t *testing.T, kind string) []byte {
	t.Helper()
	var body bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	img.Pix[0] = 123
	var err error
	if kind == "jpeg" {
		err = jpeg.Encode(&body, img, nil)
	} else {
		err = png.Encode(&body, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
func imageFileFixture(t *testing.T) (*ObjectEditingFiles, editing.AnnotationDraftTarget, []byte) {
	t.Helper()
	root := t.TempDir()
	workspace := workspacetest.New(t)
	path := filepath.Join(workspace, "image.png")
	raw := imageFixtureBytes(t, "png")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	files, err := NewObjectEditingFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return files, editing.AnnotationDraftTarget{Workspace: workspace, Path: path, ObjectIdentity: strings.Repeat("a", 64), ThreadID: "primary"}, raw
}
func imageChunk(kind string, body []byte) []byte {
	raw := make([]byte, len(body)+12)
	binary.BigEndian.PutUint32(raw, uint32(len(body)))
	copy(raw[4:], kind)
	copy(raw[8:], body)
	binary.BigEndian.PutUint32(raw[len(raw)-4:], crc32.ChecksumIEEE(raw[4:len(raw)-4]))
	return raw
}
func headerImage(width, height uint32) []byte {
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, width)
	binary.BigEndian.PutUint32(header[4:], height)
	header[8] = 8
	header[9] = 6
	out := append([]byte("\x89PNG\r\n\x1a\n"), imageChunk("IHDR", header)...)
	return append(out, imageChunk("IEND", nil)...)
}
func orientJPEG(raw []byte, value uint16) []byte {
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint16(tiff[18:], value)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(payload)+2))
	segment = append(segment, payload...)
	return append(append(append([]byte{}, raw[:2]...), segment...), raw[2:]...)
}
func TestImageSnapshotBoundsOrientationAnimationAndPath(t *testing.T) {
	for _, kind := range []string{"png", "jpeg"} {
		t.Run(kind, func(t *testing.T) {
			f, target, _ := imageFileFixture(t)
			raw := imageFixtureBytes(t, kind)
			path := target.Path
			if kind == "jpeg" {
				path = strings.TrimSuffix(path, ".png") + ".jpg"
				raw = orientJPEG(raw, 1)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := f.ReadImage(context.Background(), target.Workspace, path)
			if err != nil || got.Width != 8 || got.Height != 6 || got.Revision != digestAtomicText(raw) || !bytes.Equal(got.Bytes, raw) {
				t.Fatalf("snapshot mismatch: %v", err)
			}
		})
	}
	for _, mode := range []string{"oversize", "dimension", "pixels", "orientation", "animation", "wrong encoding", "symlink", "hardlink", "parent replacement", "protected"} {
		t.Run(mode, func(t *testing.T) {
			f, target, raw := imageFileFixture(t)
			path := target.Path
			switch mode {
			case "oversize":
				raw = bytes.Repeat([]byte("x"), editing.MaxImageBytes+1)
			case "dimension":
				raw = headerImage(8193, 1)
			case "pixels":
				raw = headerImage(8192, 2049)
			case "orientation":
				path = strings.TrimSuffix(path, ".png") + ".jpg"
				raw = orientJPEG(imageFixtureBytes(t, "jpeg"), 6)
			case "animation":
				raw = append(append(append([]byte{}, raw[:33]...), imageChunk("acTL", make([]byte, 8))...), raw[33:]...)
			case "wrong encoding":
				raw = imageFixtureBytes(t, "jpeg")
			case "symlink":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".link"); err != nil {
					t.Fatal(err)
				}
			case "parent replacement":
				original := target.Workspace + "-original"
				if err := os.Rename(target.Workspace, original); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Remove(target.Workspace); _ = os.Rename(original, target.Workspace) })
				if err := os.Symlink(original, target.Workspace); err != nil {
					t.Fatal(err)
				}
			case "protected":
				var err error
				f, err = NewObjectEditingFiles(f.receiptRoot, []string{target.Workspace})
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode != "symlink" && mode != "hardlink" && mode != "parent replacement" && mode != "protected" {
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := f.ReadImage(context.Background(), target.Workspace, path); err == nil || len(got.Bytes) != 0 {
				t.Fatal("unsafe snapshot admitted")
			}
		})
	}
}
func TestImageAnnotationCASLostAcknowledgementRestartAndStaleSource(t *testing.T) {
	f, target, raw := imageFileFixture(t)
	ctx := context.Background()
	initial, err := f.ReadImageAnnotation(ctx, target)
	if err != nil || initial.Current || initial.AnnotationRevision != "" || initial.Region != nil {
		t.Fatal("initial annotation", err)
	}
	in := editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, SourceRevision: digestAtomicText(raw), Region: &editing.ImageRegion{X: 1, Y: 1, Width: 3, Height: 2}, Note: "private-user@example.invalid"}
	writes := 0
	f.replaceJournal = func(request atomicTextReplaceRequest) error {
		writes++
		if err := atomicReplaceText(request); err != nil {
			return err
		}
		return errors.New("lost ack")
	}
	if _, err := f.WriteImageAnnotation(ctx, in); !errors.Is(err, editing.ErrPersistence) {
		t.Fatal("lost ack claimed saved", err)
	}
	saved, err := f.WriteImageAnnotation(ctx, in)
	if err != nil || writes != 1 || !saved.Current || saved.Note != in.Note {
		t.Fatal("idempotent retry", err, writes)
	}
	restarted, err := NewObjectEditingFiles(f.receiptRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restarted.ReadImageAnnotation(ctx, target)
	if err != nil || !reflect.DeepEqual(saved, restored) {
		t.Fatal("restart lost typed geometry", err)
	}
	stale := in
	stale.Note = "replacement"
	if _, err := restarted.WriteImageAnnotation(ctx, stale); !errors.Is(err, editing.ErrConflict) {
		t.Fatal("CAS bypass", err)
	}
	invalid := in
	invalid.ExpectedAnnotationRevision = saved.AnnotationRevision
	invalid.Region = &editing.ImageRegion{X: 7, Y: 0, Width: 2, Height: 1}
	if _, err := restarted.WriteImageAnnotation(ctx, invalid); !errors.Is(err, editing.ErrInvalidInput) {
		t.Fatal("out of image region", err)
	}
	if err := os.WriteFile(target.Path, imageFixtureBytes(t, "jpeg"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ReadImageAnnotation(ctx, target); err == nil {
		t.Fatal("invalid source reinterpreted")
	}
	// Use another valid exact source with the same dimensions.
	var body bytes.Buffer
	_ = png.Encode(&body, image.NewNRGBA(image.Rect(0, 0, 8, 6)))
	replacement := body.Bytes()
	if err := os.WriteFile(target.Path, replacement, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err = restarted.ReadImageAnnotation(ctx, target)
	if err != nil || restored.Current || restored.SourceRevision != saved.SourceRevision || !reflect.DeepEqual(restored.Region, saved.Region) {
		t.Fatal("stale source was remapped", err)
	}
	stale.ExpectedAnnotationRevision = saved.AnnotationRevision
	if _, err := restarted.WriteImageAnnotation(ctx, stale); !errors.Is(err, editing.ErrConflict) {
		t.Fatal("stale source write", err)
	}
	clear := editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, ExpectedAnnotationRevision: saved.AnnotationRevision, SourceRevision: digestAtomicText(replacement)}
	cleared, err := restarted.WriteImageAnnotation(ctx, clear)
	if err != nil || !cleared.Current || cleared.Region != nil || cleared.Note != "" {
		t.Fatal("clear", err)
	}
	if _, err := restarted.WriteImageAnnotation(ctx, in); err == nil {
		t.Fatal("old note resurrected")
	}
	after, _ := os.ReadFile(target.Path)
	if !bytes.Equal(after, replacement) {
		t.Fatal("annotation modified image")
	}
}

func TestImageAnnotationPrivateRecordTamperAndThreadIsolation(t *testing.T) {
	for _, mode := range []string{"unknown", "geometry", "mode", "symlink", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			f, target, raw := imageFileFixture(t)
			ctx := context.Background()
			input := editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, SourceRevision: digestAtomicText(raw), Region: &editing.ImageRegion{Width: 3, Height: 2}, Note: "private note"}
			saved, err := f.WriteImageAnnotation(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			other := target
			other.ThreadID = "another-thread"
			if got, err := f.ReadImageAnnotation(ctx, other); err != nil || got.Note != "" || got.AnnotationRevision != "" {
				t.Fatal("cross thread annotation", err)
			}
			path := f.annotationPath(target.ObjectIdentity, target.ThreadID)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "unknown":
				body = append(body[:len(body)-1], []byte(`,"extra":"untrusted"}`)...)
				err = os.WriteFile(path, body, 0600)
			case "geometry":
				body = bytes.Replace(body, []byte(`"width":3`), []byte(`"width":9999`), 1)
				err = os.WriteFile(path, body, 0600)
			case "mode":
				err = os.Chmod(path, 0644)
			case "symlink":
				err = os.Rename(path, path+".original")
				if err == nil {
					err = os.Symlink(path+".original", path)
				}
			case "hardlink":
				err = os.Link(path, path+".link")
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, err := f.ReadImageAnnotation(ctx, target); err == nil || got.Note != "" || got.Region != nil {
				t.Fatal("unsafe private record projected", err)
			}
			input.ExpectedAnnotationRevision = saved.AnnotationRevision
			input.Note = "replacement"
			if _, err := f.WriteImageAnnotation(ctx, input); err == nil {
				t.Fatal("unsafe private record overwritten")
			}
		})
	}
}
