//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
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
	"time"

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
	if err != nil || initial.Current || initial.AnnotationRevision != "" || initial.Regions == nil || len(initial.Regions) != 0 {
		t.Fatal("initial annotation", err)
	}
	in := editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, SourceRevision: digestAtomicText(raw), Regions: []editing.ImageAnnotationRegion{{RegionID: strings.Repeat("a", 48), Region: editing.ImageRegion{X: 1, Y: 1, Width: 3, Height: 2}, Note: "private-user@example.invalid"}, {RegionID: strings.Repeat("b", 48), Region: editing.ImageRegion{X: 5, Y: 1, Width: 2, Height: 2}, Note: "second region"}}}
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
	if err != nil || writes != 1 || !saved.Current || !reflect.DeepEqual(saved.Regions, in.Regions) {
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
	stale.Regions = append([]editing.ImageAnnotationRegion{}, in.Regions...)
	stale.Regions[1].Note = "replacement"
	if _, err := restarted.WriteImageAnnotation(ctx, stale); !errors.Is(err, editing.ErrConflict) {
		t.Fatal("CAS bypass", err)
	}
	invalid := in
	invalid.ExpectedAnnotationRevision = saved.AnnotationRevision
	invalid.Regions = append([]editing.ImageAnnotationRegion{}, in.Regions...)
	invalid.Regions[1].Region = editing.ImageRegion{X: 7, Y: 0, Width: 2, Height: 1}
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
	if err != nil || restored.Current || restored.SourceRevision != saved.SourceRevision || !reflect.DeepEqual(restored.Regions, saved.Regions) {
		t.Fatal("stale source was remapped", err)
	}
	stale.ExpectedAnnotationRevision = saved.AnnotationRevision
	if _, err := restarted.WriteImageAnnotation(ctx, stale); !errors.Is(err, editing.ErrConflict) {
		t.Fatal("stale source write", err)
	}
	clear := editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, ExpectedAnnotationRevision: saved.AnnotationRevision, SourceRevision: digestAtomicText(replacement), Regions: []editing.ImageAnnotationRegion{}}
	cleared, err := restarted.WriteImageAnnotation(ctx, clear)
	if err != nil || !cleared.Current || cleared.Regions == nil || len(cleared.Regions) != 0 || cleared.AnnotationRevision == "" {
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
			input := editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, SourceRevision: digestAtomicText(raw), Regions: []editing.ImageAnnotationRegion{{RegionID: strings.Repeat("a", 48), Region: editing.ImageRegion{Width: 3, Height: 2}, Note: "private note"}}}
			saved, err := f.WriteImageAnnotation(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			other := target
			other.ThreadID = "another-thread"
			if got, err := f.ReadImageAnnotation(ctx, other); err != nil || len(got.Regions) != 0 || got.AnnotationRevision != "" {
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
			if got, err := f.ReadImageAnnotation(ctx, target); err == nil || len(got.Regions) != 0 {
				t.Fatal("unsafe private record projected", err)
			}
			input.ExpectedAnnotationRevision = saved.AnnotationRevision
			input.Regions[0].Note = "replacement"
			if _, err := f.WriteImageAnnotation(ctx, input); err == nil {
				t.Fatal("unsafe private record overwritten")
			}
		})
	}
}

func TestImageAnnotationV2ReadMigrationKeepsExactCAS(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "region", true: "empty"}[empty], func(t *testing.T) {
			f, target, raw := imageFileFixture(t)
			resolved, err := f.target(target.Workspace, target.Path)
			if err != nil {
				t.Fatal(err)
			}
			legacy := annotationRecord{Version: 2, Kind: "image-region", ObjectIdentity: target.ObjectIdentity, PathBinding: resolved.binding, ThreadID: target.ThreadID, SourceRevision: digestAtomicText(raw), Width: 8, Height: 6, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			if !empty {
				legacy.Region = &editing.ImageRegion{X: 1, Y: 1, Width: 3, Height: 2}
				legacy.Note = "legacy annotation"
			}
			body, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			path := f.annotationPath(target.ObjectIdentity, target.ThreadID)
			if err := f.writeObjectPrivate(path, body, "", editing.MaxAnnotationRecordBytes); err != nil {
				t.Fatal(err)
			}
			oldHash := digestAtomicText(body)
			migrated, err := f.ReadImageAnnotation(context.Background(), target)
			if err != nil || migrated.AnnotationRevision != oldHash || migrated.Regions == nil || !migrated.Current {
				t.Fatal("migration", migrated, err)
			}
			if !empty && (len(migrated.Regions) != 1 || migrated.Regions[0].RegionID != strings.Repeat("0", 48) || migrated.Regions[0].Note != legacy.Note || migrated.Regions[0].Region != *legacy.Region) {
				t.Fatal("legacy selection changed")
			}
			if empty && len(migrated.Regions) != 0 {
				t.Fatal("empty v2 resurrected region")
			}
			again, err := f.ReadImageAnnotation(context.Background(), target)
			if err != nil || !reflect.DeepEqual(again, migrated) {
				t.Fatal("unstable migration", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(after, body) {
				t.Fatal("migration wrote on read")
			}
			write := editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, SourceRevision: migrated.SourceRevision, Regions: migrated.Regions}
			if _, err := f.WriteImageAnnotation(context.Background(), write); !errors.Is(err, editing.ErrConflict) {
				t.Fatal("v2 predecessor used as new-write CAS", err)
			}
			write.ExpectedAnnotationRevision = oldHash
			saved, err := f.WriteImageAnnotation(context.Background(), write)
			if err != nil || saved.AnnotationRevision == oldHash || !reflect.DeepEqual(saved.Regions, migrated.Regions) {
				t.Fatal("explicit v3 save", err)
			}
			v3, _ := os.ReadFile(path)
			var record imageAnnotationRecordV3
			if json.Unmarshal(v3, &record) != nil || record.Version != 3 || record.ExpectedDraftRevision != oldHash || record.Regions == nil {
				t.Fatal("v3 persistence contract")
			}
			replay, err := f.WriteImageAnnotation(context.Background(), write)
			if err != nil || !reflect.DeepEqual(replay, saved) {
				t.Fatal("v3 lost-ack replay", err)
			}
		})
	}
}

func TestImageAnnotationVersionsRejectUnknownFields(t *testing.T) {
	for _, version := range []int{2, 3} {
		t.Run(map[int]string{2: "v2", 3: "v3"}[version], func(t *testing.T) {
			f, target, raw := imageFileFixture(t)
			saved, err := f.WriteImageAnnotation(context.Background(), editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, SourceRevision: digestAtomicText(raw), Regions: []editing.ImageAnnotationRegion{}})
			if err != nil {
				t.Fatal(err)
			}
			path := f.annotationPath(target.ObjectIdentity, target.ThreadID)
			body, _ := os.ReadFile(path)
			var record map[string]any
			if json.Unmarshal(body, &record) != nil {
				t.Fatal("decode")
			}
			if version == 2 {
				record["version"] = 2
				delete(record, "regions")
				record["note"] = ""
			}
			record["unexpected"] = true
			body, _ = json.Marshal(record)
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			if result, err := f.ReadImageAnnotation(context.Background(), target); err == nil || len(result.Regions) != 0 {
				t.Fatal("unknown disk field accepted")
			}
			if _, err := f.WriteImageAnnotation(context.Background(), editing.ImageAnnotationWriteInput{AnnotationDraftTarget: target, ExpectedAnnotationRevision: saved.AnnotationRevision, SourceRevision: digestAtomicText(raw), Regions: []editing.ImageAnnotationRegion{}}); err == nil {
				t.Fatal("unknown record overwritten")
			}
		})
	}
}
