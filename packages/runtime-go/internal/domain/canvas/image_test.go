package canvas

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func imageFixture(t *testing.T, format string) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			im.SetNRGBA(x, y, color.NRGBA{uint8(20 + x*50), uint8(30 + y*80), uint8(x + y*3), 255})
		}
	}
	var b bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&b, im, &jpeg.Options{Quality: 100})
	} else {
		err = png.Encode(&b, im)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func resultPixels(t *testing.T, r ImageResult) image.Image {
	t.Helper()
	im, err := png.Decode(bytes.NewReader(r.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if r.Digest != privateBytesDigest(r.PNG) || r.ByteLength != len(r.PNG) || im.Bounds().Dx() != r.Width || im.Bounds().Dy() != r.Height {
		t.Fatal("incorrect image result metadata")
	}
	return im
}
func TestImageCropRotateProducesExactNaturalPixels(t *testing.T) {
	raw := imageFixture(t, "png")
	copyRaw := append([]byte{}, raw...)
	original, _ := png.Decode(bytes.NewReader(raw))
	ops := []ImageOperation{{Kind: "crop", Region: &Region{1, 0, 2, 2}}, {Kind: "rotate", Degrees: 90}}
	result, err := TransformImage(raw, ops)
	if err != nil {
		t.Fatal(err)
	}
	pixels := resultPixels(t, result)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if pixels.At(x, y) != original.At(y+1, 1-x) {
				t.Fatalf("incorrect clockwise pixel %d,%d", x, y)
			}
		}
	}
	if result.SourceDigest != privateBytesDigest(raw) || !bytes.Equal(raw, copyRaw) || len(result.Diff) != 2 || result.Diff[0].Before != (Dimensions{3, 2}) || result.Diff[1].After != (Dimensions{2, 2}) {
		t.Fatal("source or diff changed")
	}
	for _, degrees := range []int{90, 180, 270} {
		r, err := TransformImage(raw, []ImageOperation{{Kind: "rotate", Degrees: degrees}, {Kind: "rotate", Degrees: 360 - degrees}})
		if err != nil {
			t.Fatal(err)
		}
		p := resultPixels(t, r)
		for y := 0; y < 2; y++ {
			for x := 0; x < 3; x++ {
				if p.At(x, y) != original.At(x, y) {
					t.Fatal("rotation inverse changed pixels")
				}
			}
		}
	}
}
func TestImageMarksAndJPEG(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		raw := imageFixture(t, format)
		mark := Mark{"mark-1", "line", 0, 0, 2, 0, "#ff0000", 1}
		r, err := TransformImage(raw, []ImageOperation{{Kind: "mark", Mark: &mark}})
		if err != nil {
			t.Fatal(err)
		}
		p := resultPixels(t, r)
		for x := 0; x < 3; x++ {
			if color.NRGBAModel.Convert(p.At(x, 0)) != (color.NRGBA{255, 0, 0, 255}) {
				t.Fatal("mark missing")
			}
		}
		if r.Diff[0].Operation.Mark.ID != "mark-1" {
			t.Fatal("mark ID lost")
		}
	}
	mark := Mark{"rectangle-1", "rectangle", 0, 0, 2, 1, "#00ff00", 1}
	r, err := TransformImage(imageFixture(t, "png"), []ImageOperation{{Kind: "mark", Mark: &mark}})
	if err != nil {
		t.Fatal(err)
	}
	p := resultPixels(t, r)
	if color.NRGBAModel.Convert(p.At(2, 1)) != (color.NRGBA{0, 255, 0, 255}) {
		t.Fatal("rectangle border missing")
	}
}
func jpegWithOrientation(raw []byte, orientation uint16) []byte {
	tiff := make([]byte, 26)
	copy(tiff, "II")
	binary.LittleEndian.PutUint16(tiff[2:], 42)
	binary.LittleEndian.PutUint32(tiff[4:], 8)
	binary.LittleEndian.PutUint16(tiff[8:], 1)
	binary.LittleEndian.PutUint16(tiff[10:], 0x112)
	binary.LittleEndian.PutUint16(tiff[12:], 3)
	binary.LittleEndian.PutUint32(tiff[14:], 1)
	binary.LittleEndian.PutUint16(tiff[18:], orientation)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xff, 0xe1, 0, byte(len(payload) + 2)}
	segment = append(segment, payload...)
	out := append([]byte{}, raw[:2]...)
	out = append(out, segment...)
	return append(out, raw[2:]...)
}
func TestEXIFOrientationIsExplicitlyRejectedRatherThanIgnored(t *testing.T) {
	source := imageFixture(t, "jpeg")
	op := []ImageOperation{{Kind: "rotate", Degrees: 90}}
	if _, err := TransformImage(jpegWithOrientation(source, 1), op); err != nil {
		t.Fatal("orientation 1 rejected", err)
	}
	for _, orientation := range []uint16{0, 2, 3, 4, 5, 6, 7, 8, 9} {
		if _, err := TransformImage(jpegWithOrientation(source, orientation), op); err == nil {
			t.Fatalf("ignored EXIF orientation %d", orientation)
		}
	}
	if _, err := TransformImage(jpegWithOrientation(jpegWithOrientation(source, 1), 1), op); err == nil {
		t.Fatal("accepted duplicate EXIF")
	}
	malformed := jpegWithOrientation(source, 1)
	malformed[12] = 'X'
	if _, err := TransformImage(malformed, op); err == nil {
		t.Fatal("accepted malformed TIFF")
	}
}
func pngHeader(width, height uint32) []byte {
	chunk := func(kind string, data []byte) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint32(b, uint32(len(data)))
		copy(b[4:], kind)
		b = append(b, data...)
		checksum := make([]byte, 4)
		binary.BigEndian.PutUint32(checksum, crc32.ChecksumIEEE(b[4:]))
		return append(b, checksum...)
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr, width)
	binary.BigEndian.PutUint32(ihdr[4:], height)
	ihdr[8], ihdr[9] = 8, 6
	raw := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	raw = append(raw, chunk("IHDR", ihdr)...)
	return append(raw, chunk("IEND", nil)...)
}
func TestImageBoundsAndMalformedOperationsFail(t *testing.T) {
	op := []ImageOperation{{Kind: "rotate", Degrees: 90}}
	for _, raw := range [][]byte{nil, []byte("GIF89a"), imageFixture(t, "png")[:20], pngHeader(8192, 8192), pngHeader(8193, 1)} {
		if _, err := TransformImage(raw, op); err == nil {
			t.Fatal("accepted invalid or oversized image")
		}
	}
	bad := []string{`[]`, `[{"kind":"rotate","degrees":45}]`, `[{"kind":"crop","region":{"y":0,"width":1,"height":1}}]`, `[{"kind":"crop","region":{"x":0.5,"y":0,"width":1,"height":1}}]`, `[{"kind":"rotate","degrees":90,"sourcePath":"/tmp/a"}]`, `[{"kind":"rotate","degrees":90,"degrees":180}]`}
	for _, raw := range bad {
		if _, err := ParseImageOperations([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := TransformImage(imageFixture(t, "png"), []ImageOperation{{Kind: "crop", Region: &Region{2, 1, 2, 1}}}); err == nil {
		t.Fatal("accepted out-of-bounds crop")
	}
	mark := Mark{"m1", "line", 0, 0, 3, 0, "#ff0000", 1}
	if _, err := TransformImage(imageFixture(t, "png"), []ImageOperation{{Kind: "mark", Mark: &mark}}); err == nil {
		t.Fatal("accepted out-of-bounds mark")
	}
	mark.X2 = 2
	if _, err := TransformImage(imageFixture(t, "png"), []ImageOperation{{Kind: "mark", Mark: &mark}, {Kind: "mark", Mark: &mark}}); err == nil {
		t.Fatal("accepted duplicate mark ID")
	}
}
