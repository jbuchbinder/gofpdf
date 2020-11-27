module github.com/jbuchbinder/gofpdf

go 1.15

replace (
	github.com/jbuchbinder/gofpdf => ./
	gofpdf => ./
)

require (
	github.com/boombuler/barcode v1.0.0
	github.com/foobaz/lossypng v0.0.0-20200814224715-48fa8819852a
	github.com/phpdave11/gofpdi v1.0.13
	github.com/ruudk/golang-pdf417 v0.0.0-20181029194003-1af4ab5afa58
	golang.org/x/image v0.0.0-20190910094157-69e4b8554b2a
	rsc.io/pdf v0.1.1
)
