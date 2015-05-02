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
	"fmt"
)

const (
// Name prefix of templates used in Resources dictionary. Have to begin with a /.
// TEMPLATE_PREFIX          = "/TPL"
// TEMPLATE_COMPRESS_FILTER = "/Filter /FlateDecode "
)

// CreateTemplate defines a new template using the current page size.
func (f *Fpdf) CreateTemplate(fn func(*Tpl)) Template {
	return newTpl(PointType{0, 0}, f.curPageSize, f.unitStr, f.fontDirStr, fn, f)
}

// CreateTemplateCustom starts a template, using the given bounds.
func (f *Fpdf) CreateTemplateCustom(corner PointType, size SizeType, fn func(*Tpl)) Template {
	return newTpl(corner, size, f.unitStr, f.fontDirStr, fn, f)
}

// CreateTemplate creates a template not attached to any document
func CreateTemplate(corner PointType, size SizeType, unitStr, fontDirStr string, fn func(*Tpl)) Template {
	return newTpl(corner, size, unitStr, fontDirStr, fn, nil)
}

// UseTemplate adds a template to the current page or another template,
// using the size and position at which it was originally written.
func (f *Fpdf) UseTemplate(t Template) {
	corner, size := t.Size()
	f.UseTemplateScaled(t, corner, size)
}

// UseTemplateScaled adds a template to the current page or another template,
// using the given page coordinates.
func (f *Fpdf) UseTemplateScaled(t Template, corner PointType, size SizeType) {
	// You have to add at least a page first
	if f.page <= 0 {
		fmt.Println("No page!")
		return
	}

	// make a note of the fact that we actually use this template
	f.templates[t.ID()] = t

	// template data
	_, templateSize := t.Size()
	scaleX := size.Wd / templateSize.Wd
	scaleY := size.Ht / templateSize.Ht
	tx := corner.X * f.k
	ty := corner.Y * f.k

	// fmt.Printf("UseTemplateScaled: Writing to buffer (state = %d)\n", f.state)
	// fmt.Printf("q %.4F 0 0 %.4F %.4F %.4F cm\n", scaleX, scaleY, tx, ty)
	f.outf("q %.4F 0 0 %.4F %.4F %.4F cm", scaleX, scaleY, tx, ty) // Translate
	// fmt.Printf("%s%d Do Q\n", TEMPLATE_PREFIX, t.ID())
	f.outf("/TPL%d Do Q", t.ID())

	// fmt.Println(string(f.pages[f.page].Bytes()[:]))
}

var nextTemplateIDChannel chan int64 = func() chan int64 {
	ch := make(chan int64)
	go func() {
		var nextId int64 = 1
		for {
			ch <- nextId
			nextId++
		}
	}()
	return ch
}()

// generateTemplateID gives the next template ID. These numbers are global so that they can never clash.
func generateTemplateID() int64 {
	return <-nextTemplateIDChannel
}

// Template is an object that can be written to, then used and re-used any number of times within a document.
type Template interface {
	ID() int64
	Size() (corner PointType, size SizeType)
	Bytes() []byte
}

func (f *Fpdf) putTemplates() {
	filter := ""
	if f.compress {
		filter = "/Filter /FlateDecode "
	}

	var t Template
	for _, t = range f.templates {
		corner, size := t.Size()

		f.newobj()
		f.templateObjects[t.ID()] = f.n
		f.outf("<<%s/Type /XObject", filter)
		f.out("/Subtype /Form")
		f.out("/Formtype 1")
		f.outf("/BBox [%.2F %.2F %.2F %.2F]", corner.X*f.k, corner.Y*f.k, (corner.X+size.Wd)*f.k, (corner.Y+size.Ht)*f.k)
		if corner.X != 0 || corner.Y != 0 {
			f.outf("/Matrix [1 0 0 1 %.5F %.5F]", -corner.X*f.k*2, corner.Y*f.k*2)
		}

		// Resources
		f.out("/Resources ")
		f.out("<</ProcSet [/PDF /Text /ImageB /ImageC /ImageI]")

		// if ...

		f.out(">>")
		var buffer []byte = t.Bytes()
		// fmt.Println("Put template bytes", string(buffer[:]))
		if f.compress {
			buffer = sliceCompress(buffer)
		}
		f.outf("/Length %d >>", len(buffer))
		f.putstream(buffer)
		f.out("endobj")
	}
}
