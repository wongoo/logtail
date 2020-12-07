// Command makeStatic reads a set of files and writes a Go source file to "static.go"
// that declares a map of string constants containing contents of the input files.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"os"
	"unicode/utf8"
)

var files = []string{
	"static/index.html",
}

func main() {
	if err := makeStatic(); err != nil {
		log.Fatal(err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func makeStatic() error {
	f, err := os.Create("static.go")
	if err != nil {
		return err
	}
	defer f.Close()
	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "%v\n\n%v\n\npackage logtail\n\n", license, warning)
	fmt.Fprintf(buf, "var indexHTMLContent=[]byte(")
	for _, fn := range files {
		b, err := ioutil.ReadFile(fn)
		if err != nil {
			return err
		}
		if utf8.Valid(b) {
			fmt.Fprintf(buf, "`%s`", sanitize(b))
		} else {
			fmt.Fprintf(buf, "%q", b)
		}
	}
	fmt.Fprintf(buf, ")")

	fmtBuf, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return ioutil.WriteFile("static.go", fmtBuf, 0666)
}

// sanitize prepares a valid UTF-8 string as a raw string constant.
func sanitize(b []byte) []byte {
	// Replace ` with `+"`"+`
	b = bytes.Replace(b, []byte("`"), []byte("`+\"`\"+`"), -1)

	// Replace BOM with `+"\xEF\xBB\xBF"+`
	// (A BOM is valid UTF-8 but not permitted in Go source files.
	// I wouldn't bother handling this, but for some insane reason
	// jquery.js has a BOM somewhere in the middle.)
	return bytes.Replace(b, []byte("\xEF\xBB\xBF"), []byte("`+\"\\xEF\\xBB\\xBF\"+`"), -1)
}

const warning = `// Code generated by "makeStatic"; DO NOT EDIT.`

var license = `// Copyright 2020 wongoo. All rights reserved.`
