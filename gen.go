// Copyright (c) David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

type pseudoResponseWriter struct {
	outputFn string
	file     *os.File
	hdr      http.Header
}

func (w *pseudoResponseWriter) Header() http.Header {
	return w.hdr
}

func (w *pseudoResponseWriter) Write(buf []byte) (int, error) {
	if w.file == nil {
		var err error
		w.file, err = os.OpenFile(w.outputFn, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			log.Print(err)
		}
	}
	w.file.Write(buf)
	return 200, nil
}

func (w *pseudoResponseWriter) WriteHeader(statusCode int) {
	// Do nothing
}

func (h *Htex) GenerateStaticContent(outputDir string) {
	mkDirs := func(fullFn, outputFn string) {
		// Print generated file
		fmt.Println(fullFn, "->", outputFn)
		os.MkdirAll(filepath.Dir(outputFn), os.ModePerm)
	}

	h.ScanFiles(
		// Dynamic content
		func(fullFn, query string) {
			outputFn := filepath.Join(outputDir, query, "index.html")
			mkDirs(fullFn, outputFn)

			// Emulate a GET request to the .htex file to generate its content.
			w := &pseudoResponseWriter{outputFn, nil, http.Header{}}
			r := &http.Request{Method: "GET"}
			r.URL = &url.URL{}

			hf, err := h.parseHtexFile(w, r, fullFn)
			if err != nil {
				log.Print(err)
			} else {
				h.writeHtexFile(w, r, hf, hf.layout, nil)
			}
		},
		// Static content
		func(fullFn, fn string) {
			outputFn := filepath.Join(outputDir, fn)
			mkDirs(fullFn, outputFn)

			content, err := os.ReadFile(fullFn)
			if err != nil {
				log.Print(err)
			} else {
				os.WriteFile(outputFn, content, 0666)
			}
		})
}
