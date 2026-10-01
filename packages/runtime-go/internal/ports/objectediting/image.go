package objectediting

import (
	"context"
	"regexp"
)

const MaxImageRegions = 8

var imageRegionID = regexp.MustCompile(`^[a-f0-9]{48}$`)

func ValidImageRegionID(id string) bool { return imageRegionID.MatchString(id) }

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
type ImageAnnotationRegion struct {
	RegionID string      `json:"regionId"`
	Region   ImageRegion `json:"region"`
	Note     string      `json:"note"`
}

type ImageAnnotation struct {
	ObjectID           string                  `json:"objectId"`
	ThreadID           string                  `json:"threadId"`
	AnnotationRevision string                  `json:"annotationRevision"`
	SourceRevision     string                  `json:"sourceRevision"`
	Width              int                     `json:"width"`
	Height             int                     `json:"height"`
	Regions            []ImageAnnotationRegion `json:"regions"`
	UpdatedAt          string                  `json:"updatedAt"`
	Current            bool                    `json:"current"`
}
type ImageAnnotationWriteInput struct {
	AnnotationDraftTarget
	ExpectedAnnotationRevision, SourceRevision string
	Regions                                    []ImageAnnotationRegion
}
type ImageFiles interface {
	ReadImage(context.Context, string, string) (ImageDocument, error)
	ReadImageAnnotation(context.Context, AnnotationDraftTarget) (ImageAnnotation, error)
	WriteImageAnnotation(context.Context, ImageAnnotationWriteInput) (ImageAnnotation, error)
}

func ValidImageAnnotationWrite(expected, source string, regions []ImageAnnotationRegion) bool {
	if !ValidAnnotationWrite(expected, "", source) || regions == nil || len(regions) > MaxImageRegions {
		return false
	}
	ids := make(map[string]bool, len(regions))
	units, bytes := 0, 0
	for _, item := range regions {
		if !ValidImageRegionID(item.RegionID) || ids[item.RegionID] || !ValidAnnotationWrite(expected, item.Note, source) {
			return false
		}
		ids[item.RegionID] = true
		r := item.Region
		if r.X < 0 || r.Y < 0 || r.Width <= 0 || r.Height <= 0 || r.X >= MaxImageDimension || r.Y >= MaxImageDimension || r.Width > MaxImageDimension-r.X || r.Height > MaxImageDimension-r.Y {
			return false
		}
		bytes += len(item.Note)
		for _, r := range item.Note {
			units++
			if r > 0xffff {
				units++
			}
		}
		if units > MaxAnnotationNoteUTF16 || bytes > MaxAnnotationNoteBytes {
			return false
		}
	}
	return true
}
