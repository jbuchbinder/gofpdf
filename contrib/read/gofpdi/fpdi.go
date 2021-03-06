package gofpdi

/*
 * Copyright (c) 2015 Kurt Jung (Gmail: kurt.w.jung),
 *   Marcus Downing, Jan Slabon (Setasign)
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/jbuchbinder/gofpdf"
)

// Fpdi represents a PDF file parser which can load templates to use in other documents
type Fpdi struct {
	numPages        int        // the number of pages in the PDF cocument
	lastUsedPageBox string     // the most recently used value of boxNames
	parser          *PDFParser // the actual document reader
	pdfVersion      string     // the PDF version
	k               float64    // default scale factor (number of points in user unit)
}

// Open makes an existing PDF file usable for templates
func Open(file *os.File) (*Fpdi, error) {
	parser, err := OpenPDFParser(file)
	if err != nil {
		return nil, err
	}

	td := new(Fpdi)
	td.parser = parser
	td.pdfVersion = td.parser.reader.pdfVersion
	td.numPages = len(parser.pages)
	td.k = 72.0 / 25.4

	return td, nil
}

// Open makes an existing PDF file usable for templates
func OpenFromFileName(filename string) (*Fpdi, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	parser, err := OpenPDFParser(file)
	if err != nil {
		return nil, err
	}

	td := new(Fpdi)
	td.parser = parser
	td.pdfVersion = td.parser.reader.pdfVersion
	td.numPages = len(td.parser.pages)

	td.k = 72.0 / 25.4

	return td, nil
}

// CountPages returns the number of pages in this source document
func (td *Fpdi) CountPages() int {
	return td.numPages
}

// Page imports a single page of the source document using default settings
func (td *Fpdi) Page(pageNumber int) gofpdf.Template {
	return td.ImportPage(pageNumber, DefaultBox, false)
}

// ImportPage imports a single page of the source document to use as a template in another document
func (td *Fpdi) ImportPage(pageNumber int, boxName string, groupXObject bool) gofpdf.Template {
	if boxName == "" {
		boxName = DefaultBox
	}

	boxName = "/" + strings.TrimLeft(boxName, "/")

	td.parser.setPageNumber(pageNumber)

	t := new(TemplatePage)
	t.id = gofpdf.GenerateTemplateID()

	pageBoxes := td.parser.GetPageBoxes(pageNumber, td.k)

	pageBox := pageBoxes.get(boxName)
	td.lastUsedPageBox = pageBoxes.lastUsedPageBox

	t.box = pageBox
	t.parser = td.parser

	err, resources := td.parser.getPageResources()

	if err == nil && resources != nil {
		t.resources = resources
	}
	err, content := td.parser.getContent()
	if err == nil && content != nil {
		t.buffer = content
	}
	t.groupXObject = groupXObject
	t.x = 0
	t.y = 0

	// groupXObject only works => 1.4
	if t.groupXObject {
		i, _ := strconv.ParseFloat(td.pdfVersion, 64)
		i2 := 1.4
		td.pdfVersion = strconv.FormatFloat(math.Max(i, i2), 'f', 12, 64)
	}

	err, _ = td.parser.getPageRotation()

	return t
}

// GetLastUsedPageBox returns the last used page boundary box.
func (td *Fpdi) GetLastUsedPageBox() string {
	return td.lastUsedPageBox
}

// Close releases references and closes the file handle of the parser
func (td *Fpdi) Close() {
	td.parser.Close()
}
