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
	if !ValidImageAnnotationWrite("", source, []ImageAnnotationRegion{}) || ValidImageAnnotationWrite("", source, nil) {
		t.Fatal("clear contract")
	}
	regions := []ImageAnnotationRegion{{RegionID: strings.Repeat("a", 48), Region: ImageRegion{Width: 1, Height: 1}, Note: "note"}}
	if !ValidImageAnnotationWrite("", source, regions) {
		t.Fatal("annotation rectangle")
	}
	regions[0].Region.X = math.MaxInt
	if ValidImageAnnotationWrite("", source, regions) {
		t.Fatal("overflow rectangle")
	}
}

func TestImageAnnotationCollectionBudgetAndIdentity(t *testing.T) {
	source := strings.Repeat("b", 64)
	regions := []ImageAnnotationRegion{
		{RegionID: strings.Repeat("a", 48), Region: ImageRegion{Width: 1, Height: 1}, Note: strings.Repeat("😀", 1024)},
		{RegionID: strings.Repeat("b", 48), Region: ImageRegion{Width: 1, Height: 1}, Note: strings.Repeat("中", 2048)},
	}
	if !ValidImageAnnotationWrite("", source, regions) {
		t.Fatal("exact shared UTF16 boundary")
	}
	for _, mode := range []string{"total note", "duplicate", "short id", "uppercase id", "invalid utf8", "byte budget", "too many"} {
		t.Run(mode, func(t *testing.T) {
			bad := append([]ImageAnnotationRegion{}, regions...)
			switch mode {
			case "total note":
				bad[1].Note += "a"
			case "duplicate":
				bad[1].RegionID = bad[0].RegionID
			case "short id":
				bad[0].RegionID = strings.Repeat("a", 32)
			case "uppercase id":
				bad[0].RegionID = strings.Repeat("A", 48)
			case "invalid utf8":
				bad[0].Note = string([]byte{0xff})
			case "byte budget":
				bad[0].Note = strings.Repeat("😀", MaxAnnotationNoteBytes/4+1)
			case "too many":
				bad = nil
				for i := 0; i < 9; i++ {
					bad = append(bad, ImageAnnotationRegion{RegionID: strings.Repeat(string(rune('a'+i%6)), 47) + string(rune('0'+i)), Region: ImageRegion{Width: 1, Height: 1}})
				}
			}
			if ValidImageAnnotationWrite("", source, bad) {
				t.Fatal("invalid collection accepted")
			}
		})
	}
	regions = nil
	for i := 0; i < 8; i++ {
		regions = append(regions, ImageAnnotationRegion{RegionID: strings.Repeat("a", 47) + string(rune('0'+i)), Region: ImageRegion{Width: 1, Height: 1}})
	}
	if !ValidImageAnnotationWrite("", source, regions) {
		t.Fatal("eight regions rejected")
	}
}
