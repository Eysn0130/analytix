package toolcatalog

import "encoding/json"

// GenerationToolParameters advertises only kinds whose plugin and writer are
// currently available. Core still applies the stricter discriminated contract.
func GenerationToolParameters(kinds []string) json.RawMessage {
	if len(kinds) == 0 {
		kinds = []string{"docx"}
	}
	object := func(properties map[string]any, required ...string) map[string]any {
		value := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
		if len(required) > 0 {
			value["required"] = required
		}
		return value
	}
	array := func(items any, max int) map[string]any {
		return map[string]any{"type": "array", "items": items, "maxItems": max}
	}
	text := func(max int) map[string]any { return map[string]any{"type": "string", "maxLength": max} }
	enum := func(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	number := func(min, max float64) map[string]any {
		return map[string]any{"type": "number", "minimum": min, "maximum": max}
	}
	id := map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9_-]{0,63}$"}
	color := map[string]any{"type": "string", "pattern": "^[A-Fa-f0-9]{6}$"}
	boolean := map[string]any{"type": "boolean"}
	image := object(map[string]any{"id": id, "type": enum("png", "jpg", "gif", "bmp"), "dataBase64": text(1048576)}, "id", "type", "dataBase64")
	style := object(map[string]any{"bold": boolean, "fontColor": color, "fill": color, "align": enum("left", "center", "right"), "wrap": boolean, "numberFormat": text(255)})
	cell := object(map[string]any{"address": text(16), "type": enum("text", "number", "boolean", "formula"), "value": map[string]any{"anyOf": []any{text(32767), map[string]any{"type": "number"}, boolean}}, "formula": text(1024), "style": style}, "address", "type")
	chartSeries := object(map[string]any{"name": map[string]any{"type": "string", "maxLength": 16, "description": "Single A1 cell reference on this sheet, pointing to an existing non-empty text label."}, "categories": text(40), "values": text(40)}, "name", "categories", "values")
	chart := object(map[string]any{"id": id, "type": enum("bar", "line", "pie"), "title": text(255), "anchor": text(16), "series": array(chartSeries, 20)}, "id", "type", "anchor", "series")
	sheet := object(map[string]any{"id": id, "name": text(31), "cells": array(cell, 10000), "columns": array(map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": 255}, 1024), "frozenRows": map[string]any{"type": "integer", "minimum": 0, "maximum": 99999}, "charts": array(chart, 100)}, "id", "name", "cells")
	workbook := object(map[string]any{"sheets": array(sheet, 20)}, "sheets")
	presentationSeries := object(map[string]any{"name": text(6000), "values": array(map[string]any{"type": "number"}, 100)}, "name", "values")
	presentationObject := object(map[string]any{
		"id": id, "kind": enum("text", "shape", "chart", "image"), "x": number(0, 13.333333), "y": number(0, 7.5), "w": number(0, 13.333333), "h": number(0, 7.5),
		"text": text(6000), "fontSize": number(10, 60), "bold": boolean, "color": color, "align": enum("left", "center", "right"),
		"shape": enum("rect", "ellipse", "line"), "fill": color, "lineColor": color, "lineWidth": number(0, 20),
		"chartType": enum("bar", "line", "pie"), "title": text(256), "categories": array(text(6000), 100), "series": array(presentationSeries, 8), "imageId": id,
	}, "id", "kind", "x", "y", "w", "h")
	slide := object(map[string]any{"id": id, "background": color, "objects": array(presentationObject, 100)}, "id", "objects")
	presentation := object(map[string]any{"slides": array(slide, 50)}, "slides")
	properties := map[string]any{"path": text(4096), "kind": enum(kinds...), "title": text(256)}
	for _, kind := range kinds {
		switch kind {
		case "docx":
			properties["markdown"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 1048576}
			properties["images"] = array(image, 24)
		case "xlsx":
			properties["workbook"] = workbook
		case "pptx":
			properties["presentation"] = presentation
			properties["images"] = array(image, 24)
		}
	}
	body, _ := json.Marshal(object(properties, "path", "kind"))
	return body
}
