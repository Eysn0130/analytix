package objectediting

import (
	"math"
	"strings"
	"testing"
)

func TestImageRegionBoundsAndClear(t *testing.T) {
	if !ValidImageRegion(ImageRegion{X: 1, Y: 2, Width: 7, Height: 4}, 8, 6) {
		t.Fatal("exclusive exact edge")
	}
	for _, region := range []ImageRegion{{X: -1, Width: 1, Height: 1}, {Width: 0, Height: 1}, {Width: 9, Height: 1}, {X: 7, Width: 2, Height: 1}, {X: math.MaxInt, Width: 1, Height: 1}, {Width: math.MaxInt, Height: 1}} {
		if ValidImageRegion(region, 8, 6) {
			t.Fatal("out of bounds region")
		}
	}
	for _, dims := range [][2]int{{0, 1}, {8193, 1}, {8192, 2049}, {math.MaxInt, math.MaxInt}} {
		if ValidImageRegion(ImageRegion{Width: 1, Height: 1}, dims[0], dims[1]) {
			t.Fatal("invalid dimensions")
		}
	}
	source := strings.Repeat("b", 64)
	if !ValidImageAnnotationWrite("", source, "", nil) || ValidImageAnnotationWrite("", source, "orphan", nil) {
		t.Fatal("clear contract")
	}
	if !ValidImageAnnotationWrite("", source, "note", &ImageRegion{Width: 1, Height: 1}) || ValidImageAnnotationWrite("", source, "note", &ImageRegion{X: math.MaxInt, Width: 1, Height: 1}) {
		t.Fatal("annotation rectangle")
	}
}
