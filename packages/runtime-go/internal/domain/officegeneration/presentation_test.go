package officegeneration

import (
	"strings"
	"testing"
)

const validPresentationFixture = `{"slides":[{"id":"slide-1","objects":[{"id":"title-1","kind":"text","x":1,"y":1,"w":10,"h":1,"text":"汇报 < 与 >"}]}]}`

func TestPresentationContractAcceptsStructuredObjectsAndRejectsInvalidInputs(t *testing.T) {
	p, err := ParsePresentation([]byte(validPresentationFixture))
	if err != nil || len(p.Slides) != 1 {
		t.Fatal(err)
	}
	for name, input := range map[string]string{
		"missing coordinate":    strings.Replace(validPresentationFixture, `"x":1,`, "", 1),
		"duplicate coordinate":  strings.Replace(validPresentationFixture, `"x":1,`, `"x":1,"x":2,`, 1),
		"null optional":         strings.Replace(validPresentationFixture, `"text":`, `"bold":null,"text":`, 1),
		"wrong kind fields":     strings.Replace(validPresentationFixture, `"text":`, `"shape":"rect","text":`, 1),
		"off slide":             strings.Replace(validPresentationFixture, `"w":10`, `"w":13`, 1),
		"unknown network input": strings.Replace(validPresentationFixture, `"text":`, `"url":"https://example.invalid","text":`, 1),
		"invalid ID":            strings.Replace(validPresentationFixture, `title-1`, `bad\"id`, 1),
		"invalid XML":           strings.Replace(validPresentationFixture, `汇报 < 与 >`, `bad\u0001text`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePresentation([]byte(input)); err == nil {
				t.Fatal("accepted invalid presentation")
			}
		})
	}
}

func TestPresentationImageInventoryAndChartSeriesAreBounded(t *testing.T) {
	p, err := ParsePresentation([]byte(`{"slides":[{"id":"s","objects":[{"id":"image","kind":"image","x":0,"y":0,"w":1,"h":1,"imageId":"asset"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Validate(map[string]bool{}, "") == nil {
		t.Fatal("unresolved image accepted")
	}
	if err = p.Validate(map[string]bool{"asset": true}, ""); err != nil {
		t.Fatal(err)
	}
	valid := `{"slides":[{"id":"s","objects":[{"id":"chart","kind":"chart","x":0,"y":0,"w":5,"h":4,"chartType":"bar","categories":["A","B"],"series":[{"name":"金额","values":[1,2]}]}]}]}`
	if _, err = ParsePresentation([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	if _, err = ParsePresentation([]byte(strings.Replace(valid, `[1,2]`, `[1]`, 1))); err == nil {
		t.Fatal("mismatched series accepted")
	}
}

func TestPresentationRejectsNonCanonicalFieldSpelling(t *testing.T) {
	base := `{"slides":[{"id":"s","objects":[{"id":"t","kind":"text","x":0,"y":0,"w":1,"h":1,"text":"hi"}]}]}`
	for _, pair := range [][2]string{{`"text"`, `"Text"`}, {`"slides"`, `"Slides"`}, {`"id":"s"`, `"id":"s","Background":"FFFFFF"`}, {`"text":"hi"`, `"text":"hi","FontSize":24`}} {
		if _, err := ParsePresentation([]byte(strings.Replace(base, pair[0], pair[1], 1))); err == nil {
			t.Fatal("noncanonical field admitted", pair[1])
		}
	}
}
