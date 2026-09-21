package objectediting

import "context"

const MaxImageBytes = 12 << 20
const MaxImageDimension = 8192
const MaxImagePixels = 16 << 20

// ImageRegion uses source natural pixels, with an exclusive lower/right edge.
type ImageRegion struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func ValidImageRegion(r ImageRegion, width, height int) bool {
	return width > 0 && height > 0 && width <= MaxImageDimension && height <= MaxImageDimension && int64(width)*int64(height) <= MaxImagePixels && r.X >= 0 && r.Y >= 0 && r.Width > 0 && r.Height > 0 && r.X < width && r.Y < height && r.Width <= width-r.X && r.Height <= height-r.Y
}

type ImageDocument struct {
	Workspace, IdentityPath, Path, Revision, MIMEType string
	Width, Height                                     int
	Bytes                                             []byte
}
type ImageAnnotation struct {
	ObjectID           string       `json:"objectId"`
	ThreadID           string       `json:"threadId"`
	AnnotationRevision string       `json:"annotationRevision"`
	SourceRevision     string       `json:"sourceRevision"`
	Width              int          `json:"width"`
	Height             int          `json:"height"`
	Region             *ImageRegion `json:"region"`
	Note               string       `json:"note"`
	UpdatedAt          string       `json:"updatedAt"`
	Current            bool         `json:"current"`
}
type ImageAnnotationWriteInput struct {
	AnnotationDraftTarget
	ExpectedAnnotationRevision, SourceRevision, Note string
	Region                                           *ImageRegion
}
type ImageFiles interface {
	ReadImage(context.Context, string, string) (ImageDocument, error)
	ReadImageAnnotation(context.Context, AnnotationDraftTarget) (ImageAnnotation, error)
	WriteImageAnnotation(context.Context, ImageAnnotationWriteInput) (ImageAnnotation, error)
}

func ValidImageAnnotationWrite(expected, source, note string, region *ImageRegion) bool {
	return ValidAnnotationWrite(expected, note, source) && (region != nil && region.X >= 0 && region.Y >= 0 && region.Width > 0 && region.Height > 0 && region.X < MaxImageDimension && region.Y < MaxImageDimension && region.Width <= MaxImageDimension-region.X && region.Height <= MaxImageDimension-region.Y || region == nil && note == "")
}
