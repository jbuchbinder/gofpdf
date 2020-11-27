package gofpdi_test

import (
	"fmt"
	"time"

	"github.com/jbuchbinder/gofpdf"
	"github.com/jbuchbinder/gofpdf/contrib/read/gofpdi"
	"github.com/jbuchbinder/gofpdf/internal/example"
)

// ExampleRead tests the ability to read an existing PDF file
// and use a page of it as a template in another file
func ExampleRead() {
	filename := example.Filename("Fpdf_AddPage")

	// force the test to fail after 10 seconds
	go func() {
		time.Sleep(10000 * time.Millisecond)
		panic("Time out")
	}()

	reader, err := gofpdi.Open(filename)
	if err != nil {
		fmt.Println(err)
		return
	}

	// page
	template := reader.Page(1)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.UseTemplate(template)
	fileStr := example.Filename("contrib_read_Read")
	err = pdf.OutputFileAndClose(fileStr)
	example.Summary(err, fileStr)

	// Output:
	// Successfully generated ../../../pdf/contrib_read_Read.pdf
}
