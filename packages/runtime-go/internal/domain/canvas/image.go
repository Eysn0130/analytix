package canvas

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"strconv"
)

const MaxImageBytes = 32 << 20
const MaxImagePixels = 16 << 20
const MaxImageDimension = 8192

type Region struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}
type Mark struct {
	ID          string `json:"id"`
	Shape       string `json:"shape"`
	X1          int    `json:"x1"`
	Y1          int    `json:"y1"`
	X2          int    `json:"x2"`
	Y2          int    `json:"y2"`
	Stroke      string `json:"stroke"`
	StrokeWidth int    `json:"strokeWidth"`
}
type ImageOperation struct {
	Kind    string  `json:"kind"`
	Region  *Region `json:"region,omitempty"`
	Degrees int     `json:"degrees,omitempty"`
	Mark    *Mark   `json:"mark,omitempty"`
}
type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type ImageChange struct {
	Operation ImageOperation `json:"operation"`
	Before    Dimensions     `json:"before"`
	After     Dimensions     `json:"after"`
}
type ImageResult struct {
	PNG          []byte        `json:"-"`
	MIME         string        `json:"mime"`
	SourceDigest string        `json:"sourceDigest"`
	Digest       string        `json:"digest"`
	Width        int           `json:"width"`
	Height       int           `json:"height"`
	ByteLength   int           `json:"byteLength"`
	Diff         []ImageChange `json:"diff"`
}

func validRegion(r Region) bool {
	return r.X >= 0 && r.Y >= 0 && r.X < MaxImageDimension && r.Y < MaxImageDimension && r.Width > 0 && r.Height > 0 && r.Width <= MaxImageDimension && r.Height <= MaxImageDimension
}
func validMark(m Mark) bool {
	return idPattern.MatchString(m.ID) && (m.Shape == "rectangle" || m.Shape == "line") && m.X1 >= 0 && m.Y1 >= 0 && m.X2 >= 0 && m.Y2 >= 0 && m.X1 < MaxImageDimension && m.Y1 < MaxImageDimension && m.X2 < MaxImageDimension && m.Y2 < MaxImageDimension && colorPattern.MatchString(m.Stroke) && m.StrokeWidth >= 1 && m.StrokeWidth <= 16 && (m.Shape != "rectangle" || (m.X1 < m.X2 && m.Y1 < m.Y2))
}
func ParseImageOperations(raw []byte) ([]ImageOperation, error) {
	var operations []ImageOperation
	if decode(raw, &operations, 1<<20) != nil || len(operations) < 1 || len(operations) > MaxOperations {
		return nil, ErrInvalid
	}
	var objects []map[string]json.RawMessage
	json.Unmarshal(raw, &objects)
	seen := map[string]bool{}
	for i, op := range operations {
		if len(objects[i]) != 2 {
			return nil, ErrInvalid
		}
		switch op.Kind {
		case "crop":
			if op.Region == nil || !validRegion(*op.Region) || op.Degrees != 0 || op.Mark != nil {
				return nil, ErrInvalid
			}
		case "rotate":
			if (op.Degrees != 90 && op.Degrees != 180 && op.Degrees != 270) || op.Region != nil || op.Mark != nil {
				return nil, ErrInvalid
			}
		case "mark":
			if op.Mark == nil || !validMark(*op.Mark) || op.Region != nil || op.Degrees != 0 || seen[op.Mark.ID] {
				return nil, ErrInvalid
			}
			seen[op.Mark.ID] = true
		default:
			return nil, ErrInvalid
		}
	}
	return operations, nil
}

// We do not silently ignore EXIF orientation. A missing tag means orientation 1;
// any explicit other orientation, duplicate tag or malformed TIFF is rejected.
func exifOrientationOne(tiff []byte) bool {
	if len(tiff) < 8 {
		return false
	}
	var order binary.ByteOrder
	if string(tiff[:2]) == "II" {
		order = binary.LittleEndian
	} else if string(tiff[:2]) == "MM" {
		order = binary.BigEndian
	} else {
		return false
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return false
	}
	offset := uint64(order.Uint32(tiff[4:8]))
	if offset < 8 || offset+2 > uint64(len(tiff)) {
		return false
	}
	count := uint64(order.Uint16(tiff[offset : offset+2]))
	if count > 1024 || offset+2+12*count+4 > uint64(len(tiff)) {
		return false
	}
	seen := false
	for i := uint64(0); i < count; i++ {
		entry := tiff[offset+2+12*i : offset+2+12*(i+1)]
		if order.Uint16(entry[:2]) == 0x112 {
			if seen || order.Uint16(entry[2:4]) != 3 || order.Uint32(entry[4:8]) != 1 || order.Uint16(entry[8:10]) != 1 {
				return false
			}
			seen = true
		}
	}
	return true
}
func orientationSupported(raw []byte) bool {
	if len(raw) >= 8 && bytes.Equal(raw[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		seen := false
		for offset := 8; offset < len(raw); {
			if len(raw)-offset < 12 {
				return false
			}
			size := uint64(binary.BigEndian.Uint32(raw[offset : offset+4]))
			if size > uint64(len(raw)-offset-12) {
				return false
			}
			kind := string(raw[offset+4 : offset+8])
			end := offset + 8 + int(size)
			if kind == "eXIf" {
				if seen || !exifOrientationOne(raw[offset+8:end]) {
					return false
				}
				seen = true
			}
			offset = end + 4
			if kind == "IEND" {
				return offset == len(raw)
			}
		}
		return false
	}
	if len(raw) < 2 || raw[0] != 0xff || raw[1] != 0xd8 {
		return false
	}
	seen := false
	for offset := 2; offset < len(raw); {
		if raw[offset] != 0xff {
			return false
		}
		for offset < len(raw) && raw[offset] == 0xff {
			offset++
		}
		if offset == len(raw) {
			return false
		}
		marker := raw[offset]
		offset++
		if marker == 0xda {
			return true
		}
		if marker == 0xd9 {
			return false
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		if offset+2 > len(raw) {
			return false
		}
		size := int(binary.BigEndian.Uint16(raw[offset : offset+2]))
		if size < 2 || size > len(raw)-offset {
			return false
		}
		segment := raw[offset+2 : offset+size]
		if marker == 0xe1 && len(segment) >= 6 && string(segment[:6]) == "Exif\x00\x00" {
			if seen || !exifOrientationOne(segment[6:]) {
				return false
			}
			seen = true
		}
		offset += size
	}
	return false
}
func imageDimensionsOK(w, h int) bool {
	return w > 0 && h > 0 && w <= MaxImageDimension && h <= MaxImageDimension && int64(w)*int64(h) <= MaxImagePixels
}
func decodeImage(raw []byte) (*image.NRGBA, error) {
	if len(raw) == 0 || len(raw) > MaxImageBytes || !orientationSupported(raw) {
		return nil, ErrInvalid
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg") || !imageDimensionsOK(config.Width, config.Height) {
		return nil, ErrInvalid
	}
	decoded, actual, err := image.Decode(bytes.NewReader(raw))
	if err != nil || actual != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return nil, ErrInvalid
	}
	result := image.NewNRGBA(image.Rect(0, 0, config.Width, config.Height))
	draw.Draw(result, result.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	return result, nil
}
func dimensions(i *image.NRGBA) Dimensions { return Dimensions{i.Bounds().Dx(), i.Bounds().Dy()} }
func rotateImage(src *image.NRGBA, degrees int) *image.NRGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	nw, nh := w, h
	if degrees != 180 {
		nw, nh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			nx, ny := w-1-x, h-1-y
			if degrees == 90 {
				nx, ny = h-1-y, x
			} else if degrees == 270 {
				nx, ny = y, w-1-x
			}
			dst.SetNRGBA(nx, ny, src.NRGBAAt(x, y))
		}
	}
	return dst
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func strokeLine(dst *image.NRGBA, x1, y1, x2, y2, width int, c color.NRGBA) {
	// Parallel integer Bresenham strokes keep work proportional to line length.
	horizontal := absInt(x2-x1) >= absInt(y2-y1)
	for offset := -(width / 2); offset < width-width/2; offset++ {
		x, y, tx, ty := x1, y1, x2, y2
		if horizontal {
			y += offset
			ty += offset
		} else {
			x += offset
			tx += offset
		}
		dx, dy := absInt(tx-x), -absInt(ty-y)
		sx, sy := 1, 1
		if x > tx {
			sx = -1
		}
		if y > ty {
			sy = -1
		}
		err := dx + dy
		for {
			dst.SetNRGBA(x, y, c)
			if x == tx && y == ty {
				break
			}
			twice := 2 * err
			if twice >= dy {
				err += dy
				x += sx
			}
			if twice <= dx {
				err += dx
				y += sy
			}
		}
	}
}
func drawMark(dst *image.NRGBA, m Mark) {
	rgb, _ := strconv.ParseUint(m.Stroke[1:], 16, 32)
	c := color.NRGBA{uint8(rgb >> 16), uint8(rgb >> 8), uint8(rgb), 255}
	if m.Shape == "line" {
		strokeLine(dst, m.X1, m.Y1, m.X2, m.Y2, m.StrokeWidth, c)
		return
	}
	strokeLine(dst, m.X1, m.Y1, m.X2, m.Y1, m.StrokeWidth, c)
	strokeLine(dst, m.X2, m.Y1, m.X2, m.Y2, m.StrokeWidth, c)
	strokeLine(dst, m.X2, m.Y2, m.X1, m.Y2, m.StrokeWidth, c)
	strokeLine(dst, m.X1, m.Y2, m.X1, m.Y1, m.StrokeWidth, c)
}

// TransformImage decodes source-owned bytes and applies operations in order.
// Coordinates always refer to the current natural pixels, after earlier edits.
// Original retention, authorization, CAS and undo belong to the Core caller.
func TransformImage(raw []byte, operations []ImageOperation) (ImageResult, error) {
	if len(operations) < 1 || len(operations) > MaxOperations {
		return ImageResult{}, ErrInvalid
	}
	body, err := json.Marshal(operations)
	if err != nil {
		return ImageResult{}, ErrInvalid
	}
	ops, err := ParseImageOperations(body)
	if err != nil {
		return ImageResult{}, err
	}
	current, err := decodeImage(raw)
	if err != nil {
		return ImageResult{}, err
	}
	diff := make([]ImageChange, 0, len(ops))
	// Bound cumulative decode/transform work as well as any one allocation.
	var pixelWork int64
	for _, op := range ops {
		before := dimensions(current)
		work := int64(before.Width) * int64(before.Height)
		if op.Kind == "crop" {
			work = int64(op.Region.Width) * int64(op.Region.Height)
		}
		if op.Kind == "mark" {
			m := op.Mark
			work = int64(absInt(m.X2-m.X1)+absInt(m.Y2-m.Y1)+1) * int64(m.StrokeWidth)
			if m.Shape == "rectangle" {
				work *= 4
			}
		}
		pixelWork += work
		if pixelWork > 4*MaxImagePixels {
			return ImageResult{}, ErrInvalid
		}
		switch op.Kind {
		case "crop":
			r := *op.Region
			if r.X+r.Width > before.Width || r.Y+r.Height > before.Height {
				return ImageResult{}, ErrInvalid
			}
			next := image.NewNRGBA(image.Rect(0, 0, r.Width, r.Height))
			draw.Draw(next, next.Bounds(), current, image.Pt(r.X, r.Y), draw.Src)
			current = next
		case "rotate":
			current = rotateImage(current, op.Degrees)
		case "mark":
			m := *op.Mark
			if m.X1 >= before.Width || m.X2 >= before.Width || m.Y1 >= before.Height || m.Y2 >= before.Height {
				return ImageResult{}, ErrInvalid
			}
			drawMark(current, m)
		}
		diff = append(diff, ImageChange{op, before, dimensions(current)})
	}
	var encoded boundedPNG
	if png.Encode(&encoded, current) != nil {
		return ImageResult{}, ErrInvalid
	}
	pngBytes := encoded.Bytes()
	size := dimensions(current)
	return ImageResult{PNG: pngBytes, MIME: "image/png", SourceDigest: privateBytesDigest(raw), Digest: privateBytesDigest(pngBytes), Width: size.Width, Height: size.Height, ByteLength: len(pngBytes), Diff: diff}, nil
}

// Enforce the encoded-byte cap while writing, not after growing an unbounded buffer.
type boundedPNG struct{ bytes.Buffer }

func (b *boundedPNG) Write(p []byte) (int, error) {
	if len(p) > MaxImageBytes-b.Len() {
		return 0, ErrInvalid
	}
	return b.Buffer.Write(p)
}
