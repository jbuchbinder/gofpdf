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
	"fmt"
	"os"
	"strings"
	// "regexp"
	"bytes"
	"strconv"
	// "bufio"
	"errors"
	"github.com/jung-kurt/gofpdf"
	"math"
	"compress/zlib"
	"compress/lzw"
	"encoding/ascii85"
	"io"
	"regexp"
	"encoding/hex"
)

const (
	defaultPdfVersion = "1.3"
)

// PDFParser is a high-level parser for PDF elements
// See fpdf_pdf_parser.php
type PDFParser struct {
	reader          *PDFTokenReader // the underlying token reader
	pageNumber      int             // the current page number
	lastUsedPageBox string          // the most recently used page box
	pages           []PDFPage       // already loaded pages

	xref struct {
						maxObject    int                 // the highest xref object number
						xrefLocation int64               // the location of the xref table
						xref         map[ObjectRef]int64 // all the xref offsets
						trailer Dictionary
		 }
	currentObject ObjectDeclaration
	currentDictionary Dictionary
	root Dictionary
}

// OpenPDFParser opens an existing PDF file and readies it
func OpenPDFParser(file *os.File) (*PDFParser, error) {
	// fmt.Println("Opening PDF file:", filename)
	reader, err := NewTokenReader(file)
	if err != nil {
		return nil, err
	}

	parser := new(PDFParser)
	parser.reader = reader
	parser.pageNumber = 0
	parser.lastUsedPageBox = DefaultBox

	// read xref data
	offset, err := parser.reader.findXrefTable()
	if err != nil {
		return nil, err
	}

	err = parser.readXrefTable(offset)
	if err != nil {
		return nil, err
	}

	err = parser.readRoot()
	if err != nil {
		return nil, err
	}

	// check for encryption
	if parser.getEncryption() {
		return nil, errors.New("File is encrypted!")
	}

	getPagesObj, err := parser.getPagesObj()
	if err != nil {
		return nil, err
	}

	err = parser.readPages(getPagesObj)
	if err != nil {
		return nil, err
	}

	return parser, nil
}

func (parser *PDFParser) setPageNumber(pageNumber int) {
	parser.pageNumber = pageNumber
}

// Close releases references and closes the file handle of the parser
func (parser *PDFParser) Close() {
	parser.reader.Close()
}

// PDFPage is a page extracted from an existing PDF document
type PDFPage struct {
	Dictionary
	Number int
}

// GetPageBoxes gets the all the bounding boxes for a given page
//
// pageNumber is 1-indexed
// k is a scaling factor from user space units to points
func (parser *PDFParser) GetPageBoxes(pageNumber int, k float64) PageBoxes {
	boxes := make(map[string]*PageBox, 5)
	if pageNumber < 0 || (pageNumber - 1) >= len(parser.pages) {
		return PageBoxes{boxes, DefaultBox}
	}

	page := parser.pages[pageNumber - 1]
	if box := parser.getPageBox(page.Dictionary, MediaBox, k); box != nil {
		boxes[MediaBox] = box
	}
	if box := parser.getPageBox(page.Dictionary, CropBox, k); box != nil {
		boxes[CropBox] = box
	}
	if box := parser.getPageBox(page.Dictionary, BleedBox, k); box != nil {
		boxes[BleedBox] = box
	}
	if box := parser.getPageBox(page.Dictionary, TrimBox, k); box != nil {
		boxes[TrimBox] = box
	}
	if box := parser.getPageBox(page.Dictionary, ArtBox, k); box != nil {
		boxes[ArtBox] = box
	}
	return PageBoxes{boxes, DefaultBox}
}

// getPageBox reads a bounding box from a page.
//
// page is a /Page dictionary.
//
// k is a scaling factor from user space units to points.
func (parser *PDFParser) getPageBox(pageObj Dictionary, boxIndex string, k float64) *PageBox {
	page := pageObj

	var box Value

	// Do we have this box in our page?
	if boxRef, ok := page[boxIndex]; ok {

		// If box is a reference, resolve it.
		if boxRef.Type() == typeObjRef {
			box = parser.resolveObject(boxRef);
			if box == nil {
				return nil
			}
		}
		if boxRef.Type() == typeArray {
			box = boxRef
		}
	}

	if box != nil {
		if box.Type() == typeArray {

			boxDetails := box.(Array)
			x := float64(boxDetails[0].(Real)) / k
			y := float64(boxDetails[1].(Real)) / k
			w := math.Abs(float64(boxDetails[0].(Real)) - float64(boxDetails[2].(Real))) / k
			h := math.Abs(float64(boxDetails[1].(Real)) - float64(boxDetails[3].(Real))) / k
			llx := math.Min(float64(boxDetails[0].(Real)), float64(boxDetails[2].(Real))) / k
			lly := math.Min(float64(boxDetails[1].(Real)), float64(boxDetails[3].(Real))) / k
			urx := math.Max(float64(boxDetails[0].(Real)), float64(boxDetails[2].(Real))) / k
			ury := math.Max(float64(boxDetails[1].(Real)), float64(boxDetails[3].(Real))) / k

			return &PageBox{
				gofpdf.PointType{
					x,
					y,
				},
				gofpdf.SizeType{
					w,
					h,
				},
				gofpdf.PointType{
					llx,
					lly,
				},
				gofpdf.PointType{
					urx,
					ury,
				},
			}
		}
	} else {
		// Box not found, take it from the parent.
		if parentPageRef, ok := page["/Parent"]; ok {
			parentPageObj := parser.resolveObject(parentPageRef)
			return parser.getPageBox(parentPageObj.Values[0].(Dictionary), boxIndex, k)
		}
	}

	return nil
}

func (parser *PDFParser) checkXrefTableOffset(offset int64) (int64, error) {
	// if the file is corrupt, it may not line up correctly
	// token := parser.reader.ReadToken()
	// if !bytes.Equal(token, Token("xref")) {
	// 	// bad PDF file! no cookie for you
	// 	// look to see if we can find the xref table nearby
	// 	fmt.Println("Corrupt PDF. Scanning for xref table")
	// 	parser.reader.Seek(-20, 1)
	// 	parser.reader.SkipToToken(Token("xref"))
	// 	token = parser.reader.ReadToken()
	// 	if !bytes.Equal(token, Token("xref")) {
	// 		return errors.New("Corrupt PDF: Could not find xref table")
	// 	}
	// }

	return offset, nil
}

func (parser *PDFParser) readXrefTable(offset int64) error {

	// first read in the Xref table data and the trailer dictionary
	if _, err := parser.reader.Seek(offset, 0); err != nil {
		return err
	}

	lines, ok := parser.reader.ReadLinesToToken(Token("trailer"))
	if !ok {
		return errors.New("Cannot read end of xref table")
	}

	// read the lines, store the xref table data
	start := 1
	if parser.xref.xrefLocation == 0 {
		parser.xref.maxObject = 0
		parser.xref.xrefLocation = offset
		parser.xref.xref = make(map[ObjectRef]int64, len(lines))
	}
	for _, lineBytes := range lines {
		// fmt.Println("Xref table line:", lineBytes)
		line := strings.TrimSpace(string(lineBytes))
		// fmt.Println("Reading xref table line:", line)
		if line != "" {
			if line == "xref" {
				continue
			}
			pieces := strings.Split(line, " ")
			switch len(pieces) {
			case 0:
				continue
			case 2:
				start, _ = strconv.Atoi(pieces[0])
				end, _ := strconv.Atoi(pieces[1])
				if end > parser.xref.maxObject {
					parser.xref.maxObject = end
				}
			case 3:
				// if _, ok := parser.xref.xref[start]; !ok {
				// 	parser.xref.xref[start] = make(map[int]int, len(lines))
				// }
				xr, _ := strconv.ParseInt(pieces[0], 10, 64)
				gen, _ := strconv.Atoi(pieces[1])

				ref := ObjectRef{start, gen}
				if _, ok := parser.xref.xref[ref]; !ok {
					if pieces[2] == "n" {
						parser.xref.xref[ref] = xr
					} else {
						// xref[ref] = nil // ???
					}
				}
				start++
			default:
				return errors.New("Unexpected data in xref table: '" + line + "'")
			}
		}
	}

	// first read in the Xref table data and the trailer dictionary
	if _, err := parser.reader.Seek(offset, 0); err != nil {
		return err
	}

	// Find the trailer token.
	ok = parser.reader.SkipToToken(Token("trailer"))
	if !ok {
		return errors.New("Cannot skip to trailer")
	}

	// Start reading of trailer token.
	parser.reader.ReadToken()

	// Read trailer into dictionary.
	trailer := parser.readValue(nil)
	parser.xref.trailer = trailer.(Dictionary)

	return nil
}

// readRoot reads the object reference for the root.
func (parser *PDFParser) readRoot() (error) {
	if rootRef, ok := parser.xref.trailer["/Root"]; ok {
		if rootRef.Type() != typeObjRef {
			return errors.New("Wrong Type of Root-Element! Must be an indirect reference")
		}

		root := parser.resolveObject(rootRef);
		if root == nil {
			return errors.New("Could not find reference to root")
		}
		parser.root = root.Values[0].(Dictionary)
		return nil
	} else {
		return errors.New("Could not find root in trailer")
	}
}

// getPagesObj gets the pages object from the root element.
func (parser *PDFParser) getPagesObj() (Dictionary, error) {
	if pagesRef, ok := parser.root["/Pages"]; ok {
		if pagesRef.Type() != typeObjRef {
			return nil, errors.New("Wrong Type of Pages-Element! Must be an indirect reference")
		}

		pages := parser.resolveObject(pagesRef);
		if pages == nil {
			return nil, errors.New("Could not find reference to pages")
		}
		return pages.Values[0].(Dictionary), nil
	} else {
		return nil, errors.New("Could not find /Pages in /Root-Dictionary")
	}
}

// readPages parses the PDF Page Object into PDFPages
func (parser *PDFParser) readPages(pages Dictionary) (error) {
	var kids Array
	if kidsRef, ok := pages["/Kids"]; ok {
		if kidsRef.Type() != typeArray {
			return errors.New("Wrong Type of Kids-Element! Must be an array")
		}

		kids = kidsRef.(Array)
		if kids == nil {
			return errors.New("Could not find reference to kids")
		}
	} else {
		return errors.New("Cannot find /Kids in current /Page-Dictionary")
	}

	for k, val := range kids {
		pageObj := parser.resolveObject(val);
		if pageObj == nil {
			return errors.New(fmt.Sprintf("Could not find reference to page %i", (k + 1)))
		}

		page := PDFPage{
			pageObj.Values[0].(Dictionary),
			(k + 1),
		}
		parser.pages = append(parser.pages, page)
	}

	return nil
}

// getEncryption checks if the pdf has encryption.
func (parser *PDFParser) getEncryption() bool {
	if _, ok := parser.xref.trailer["/Encrypt"]; ok {
		return true
	}
	return false
}

// readValue reads the next value from the PDF
func (parser *PDFParser) readValue(token Token) Value {
	if token == nil {
		token = parser.reader.ReadToken()
	}

	str := token.String()
	switch str {
	case "<":
		// This is a hex value
		// Read the value, then the terminator
		bytes, _ := parser.reader.ReadBytesToToken(Token(">"))
		//fmt.Println("Read hex:", bytes)
		return Hex(bytes)

	case "<<":
		// This is a dictionary.
		// Recurse into this function until we reach
		// the end of the dictionary.
		result := make(map[string]Value, 32)

		validToken := true

		// Skip one line for dictionary.
		for validToken {
			key := parser.reader.ReadToken()
			if (key.Equals(Token(">>"))) {
				validToken = false
				break;
			}

			if key == nil {
				return nil // ?
			}

			value := parser.readValue(nil)
			if value == nil {
				return nil // ?
			}

			// Catch missing value
			if value.Type() == typeToken && value.Equals(Token(">>")) {
				result[key.String()] = Null(struct{}{})
				break
			}

			result[key.String()] = value

			// This is needed to get the length of stream.
			// @todo: is there a better way? We can't use currentObject as
			// @todo the values haven't been added to the currentObject yet.
			parser.currentDictionary = Dictionary(result)
		}

		return Dictionary(result)

	case "[":
		// This is an array.
		// Recurse into this function until we reach
		// the end of the array.
		result := make([]Value, 0, 32)
		for {
			// We peek here, as the token could be the value.
			token := parser.reader.ReadToken()
			if token.Equals(Token("]")) {
				break;
			}

			value := parser.readValue(token)
			result = append(result, value)
		}
		return Array(result)

	case "(":
		// This is a string
		openBrackets := 1
		buf := bytes.NewBuffer([]byte{})
		for openBrackets > 0 {
			b, ok := parser.reader.ReadByte()
			if !ok {
				break
			}
			switch b {
			case 0x28: // (
				openBrackets++
			case 0x29: // )
				openBrackets++
			case 0x5C: // \
				b, ok = parser.reader.ReadByte()
				if !ok {
					break
				}
			}
			buf.WriteByte(b)
		}
		return String(buf.Bytes())

	case "stream":
		// ensure line breaks in front of the stream
		peek := parser.reader.Peek(32)
		for _, c := range peek {
			if !isPdfWhitespace(c) {
				break
			}
			parser.reader.ReadByte()
		}

		lengthObj := parser.currentDictionary["/Length"]
		if lengthObj.Type() == typeObjRef {
			lengthObj = parser.resolveObject(lengthObj)
		}

		length := int(lengthObj.(Real))
		stream, _ := parser.reader.ReadBytes(length)

		if endstream := parser.reader.ReadToken(); endstream.Equals(Token("endstream")) {
			// We don't throw an error here because the next
			// round trip will start at a new offset
		}

		return Stream(stream)
	}

	if number, err := strconv.Atoi(str); err == nil {
		// A numeric token. Make sure that
		// it is not part of something else.
		if moreTokens := parser.reader.PeekTokens(2); len(moreTokens) == 2 {
			if number2, err := strconv.Atoi(string(moreTokens[0])); err == nil {
				// Two numeric tokens in a row.
				// In this case, we're probably in
				// front of either an object reference
				// or an object specification.
				// Determine the case and return the data
				switch string(moreTokens[1]) {
				case "obj":
					parser.reader.ReadTokens(2)
					return ObjectRef{number, number2}
				case "R":
					parser.reader.ReadTokens(2)
					return ObjectRef{number, number2}
				}
			}
		}

		if real, err := strconv.ParseFloat(str, 64); err == nil {
			return Real(real)
		}

		return Numeric(number)
	}

	if real, err := strconv.ParseFloat(str, 64); err == nil {
		return Real(real)
	}

	if str == "true" {
		return Boolean(true)
	}
	if str == "false" {
		return Boolean(false)
	}
	if str == "null" {
		return Null(struct{}{})
	}
	// Just a token. Return it.
	return token
}

func (parser *PDFParser) resolveObject(spec Value) *ObjectDeclaration {
	// Exit if we get invalid data
	if spec == nil {
		return nil
	}

	if objRef, ok := spec.(ObjectRef); ok {

		// This is a reference, resolve it
		if offset, ok := parser.xref.xref[objRef]; ok {
			originalOffset, _ := parser.reader.Seek(0, 1)
			parser.reader.Seek(offset, 0)
			header := parser.readValue(nil)

			// Check to see if we got the correct object.
			if header != objRef {

				// Reset seeker, we want to find our object.
				parser.reader.Seek(0, 0)
				toSearchFor := Token(fmt.Sprintf("%d %d obj", objRef.Obj, objRef.Gen))
				if parser.reader.SkipToToken(toSearchFor) {
					parser.reader.SkipBytes(len(toSearchFor))
				} else {
					// Unable to find object

					// Reset to the original position
					parser.reader.Seek(originalOffset, 0)
					return nil
				}
			}

			// If we're being asked to store all the information
			// about the object, we add the object ID and generation
			// number for later use
			result := ObjectDeclaration{header.(ObjectRef).Obj, header.(ObjectRef).Gen, make([]Value, 0, 2)}
			parser.currentObject = result

			// Now simply read the object data until
			// we encounter an end-of-object marker
			for {
				value := parser.readValue(nil)
				if value == nil || len(result.Values) > 1 { // ???
					// in this case the parser couldn't find an "endobj" so we break here
					break
				}

				if value.Type() == typeToken && value.Equals(Token("endobj")) {
					break
				}

				result.Values = append(result.Values, value)
			}

			// Reset to the original position
			parser.reader.Seek(originalOffset, 0)

			return &result

		} else {
			// Unable to find object
			return nil
		}
	}

	if obj, ok := spec.(*ObjectDeclaration); ok {
		return obj
	}
	// Er, it's a what now?
	return nil
}

// getPageRotation reads the page rotation for a specific page.
func (parser *PDFParser) getPageRotation() (error, Value) {

	for i, page := range parser.pages {
		if i == (parser.pageNumber - 1) {
			return parser._getPageRotation(page.Dictionary)

		}
	}

	return errors.New(fmt.Sprintf("Page %s does not exists.", parser.pageNumber - 1)), nil
}

// _getPageRotation reads the page rotation for a specific page.
func (parser *PDFParser) _getPageRotation(pageObj Value) (error, Value) {
	page := pageObj.(Dictionary)

	if rotation, ok := page["/Rotate"]; ok {
		if rotation.Type() == typeObject {
			return nil, rotation.(Object).Value
		}
		return nil, rotation
	}

	if parentObj, ok := page["/Parent"]; ok {
		parent := parser.resolveObject(parentObj)
		err, parentRotation := parser._getPageRotation(parent.Values[0])
		if err != nil {
			return err, nil
		}

		if parentRotation == nil {
			return nil, nil
		}

		if parentRotation.Type() == typeObject {
			return nil, parentRotation.(Object).Value
		}
		return nil, parentRotation
	}
	return nil, nil
}

// getPageResources reads the page resources for a specific page.
func (parser *PDFParser) getPageResources() (error, []Value) {

	for i, page := range parser.pages {
		if i == (parser.pageNumber - 1) {
			return parser._getPageResources(page.Dictionary)

		}
	}

	return errors.New(fmt.Sprintf("Page %s does not exists.", parser.pageNumber - 1)), nil
}

// _getPageResources reads the page resources for a specific page.
func (parser *PDFParser) _getPageResources(pageObj Value) (error, []Value) {
	page := pageObj.(Dictionary)

	if resources, ok := page["/Resources"]; ok {
		resources := parser.resolveObject(resources)
		return nil, resources.Values
	}

	if parentObj, ok := page["/Parent"]; ok {
		parent := parser.resolveObject(parentObj)
		err, parentResources := parser._getPageResources(parent.Values[0])
		if err != nil {
			return err, nil
		}
		if parentResources == nil {
			return nil, nil
		}
		return nil, parentResources
	}
	return nil, nil
}

// getContent reads the page resources for a specific page.
func (parser *PDFParser) getContent() (error, []byte) {

	var returnBytes []byte

	for i, page := range parser.pages {
		if i == (parser.pageNumber - 1) {

			pageDitct := page.Dictionary

			if contentRef, ok := pageDitct["/Contents"]; ok {
				var contents [][]Value
				parser._getPageContent(contentRef, &contents)
				for _, content := range contents {
					newBytes := parser._unFilterStream(content)
					if newBytes != nil {
						returnBytes = append(returnBytes, newBytes...)
					}
				}
			}
			return nil, returnBytes
		}
	}

	return errors.New(fmt.Sprintf("Page %s does not exists.", parser.pageNumber - 1)), nil
}

// _getPageContent reads the page resources for a specific page.
func (parser *PDFParser) _getPageContent(contentRef Value, resultContent *[][]Value) {
	if contentRef.Type() == typeObjRef {
		content := parser.resolveObject(contentRef)
		if content.Values[0].Type() == typeArray {
			parser._getPageContent(content.Values[0], resultContent)
		} else {
			*resultContent = append(*resultContent, content.Values)
		}
	} else if contentRef.Type() == typeArray {
		for _, pageContent := range contentRef.(Array) {
			parser._getPageContent(pageContent, resultContent)
		}
	}
}

func (parser *PDFParser) _unFilterStream(content []Value) []byte {
	var useFilters []string

	if filter, ok := content[0].(Dictionary)["/Filter"]; ok {
		if filter.Type() == typeObjRef {
			tmpFilter := parser.resolveObject(filter)
			filter = tmpFilter.Values[0]
		}

		if filter.Type() == typeToken {
			useFilters = append(useFilters, filter.(Token).String())
		} else if filter.Type() == typeArray {
			for _, tmpFilter := range filter.(Array) {
				useFilters = append(useFilters, tmpFilter.(Token).String())
			}
		}
	}

	stream := content[1].(Stream)

	for _, filter := range useFilters {
		switch (filter) {
		case "/Fl":
		case "/FlateDecode":
			var out bytes.Buffer
			zlibReader, _ := zlib.NewReader(stream.GetReader())
			defer zlibReader.Close()

			io.Copy(&out, zlibReader)

			return out.Bytes()
			break
		case "/LZWDecode":
			var out bytes.Buffer
			lzwReader := lzw.NewReader(stream.GetReader(), lzw.MSB, 8)
			defer lzwReader.Close()

			io.Copy(&out, lzwReader)

			return out.Bytes()
			break
		case "/ASCII85Decode":
			var out bytes.Buffer
			ascii85Reader := ascii85.NewDecoder(stream.GetReader())

			io.Copy(&out, ascii85Reader)

			return out.Bytes()
			break
		case "ASCIIHexDecode":
			hexbytes := []byte(stream)
			hexstring := string(hexbytes)
			re := regexp.MustCompile("[^0-9A-Fa-f]")
			hexstring = re.ReplaceAllString(hexstring, "")
			hexstring = strings.TrimRight(hexstring, ">")
			if (len(hexstring) % 2) == 1 {
				hexstring += "0";
			}

			out, err := hex.DecodeString(hexstring)
			if err != nil {
				return nil
			}
			return out
			break

		}
	}

	return nil
}
