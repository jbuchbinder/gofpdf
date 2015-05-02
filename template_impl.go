package gofpdf

//
//  GoFPDI
//
//    Copyright 2015 Marcus Downing
//
//  FPDI - Version 1.5.2
//
//    Copyright 2004-2014 Setasign - Jan Slabon
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

import (
// "bytes"
)

// newTpl creates a template, copying graphics settings from a template if one is given
func newTpl(corner PointType, size SizeType, unitStr, fontDirStr string, fn func(*Tpl), copyFrom *Fpdf) Template {
	orientationStr := "p"
	if size.Wd > size.Ht {
		orientationStr = "l"
	}
	sizeStr := ""

	fpdf := fpdfNew(orientationStr, unitStr, sizeStr, fontDirStr, size)
	tpl := Tpl{*fpdf}
	if copyFrom != nil {
		tpl.loadParamsFromFpdf(copyFrom)
	}
	tpl.Fpdf.SetAutoPageBreak(false, 0)
	tpl.Fpdf.AddPage()
	fn(&tpl)
	bytes := tpl.Fpdf.pages[tpl.Fpdf.page].Bytes()

	id := generateTemplateID()
	template := Fpdf_Tpl{id, corner, size, bytes}
	return &template
}

// Fpdf_Tpl is a concrete implementation of the Template interface.
type Fpdf_Tpl struct {
	id     int64
	corner PointType
	size   SizeType
	bytes  []byte
}

func (t *Fpdf_Tpl) ID() int64 {
	return t.id
}

func (t *Fpdf_Tpl) Size() (corner PointType, size SizeType) {
	return t.corner, t.size
}

func (t *Fpdf_Tpl) Bytes() []byte {
	return t.bytes
}

// Tpl is an Fpdf used for writing a template.
// It has most of the facilities of an Fpdf,but cannot add more pages.
// Tpl is used directly only during the limited time a template is writable.
type Tpl struct {
	Fpdf
}

func (t *Tpl) loadParamsFromFpdf(f *Fpdf) {
	t.Fpdf.compress = false

	t.Fpdf.k = f.k
	t.Fpdf.x = f.x
	t.Fpdf.y = f.y
	t.Fpdf.lineWidth = f.lineWidth
	t.Fpdf.capStyle = f.capStyle
	t.Fpdf.joinStyle = f.joinStyle

	t.Fpdf.color.draw = f.color.draw
	t.Fpdf.color.fill = f.color.fill
	t.Fpdf.color.text = f.color.text

	t.Fpdf.currentFont = f.currentFont
	t.Fpdf.fontFamily = f.fontFamily
	t.Fpdf.fontSize = f.fontSize
	t.Fpdf.fontSizePt = f.fontSizePt
	t.Fpdf.fontStyle = f.fontStyle
	t.Fpdf.ws = f.ws
}

// Things you can't do to a template

func (t *Tpl) AddPage() {
}

func (t *Tpl) AddPageFormat(orientationStr string, size SizeType) {
}

func (t *Tpl) SetAutoPageBreak(auto bool, margin float64) {
}
