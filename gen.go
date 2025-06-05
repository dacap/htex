// Copyright 2025 David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	filepath.Walk(h.localRoot, func(fullFn string, info os.FileInfo, err error) error {
		fn := filepath.ToSlash(fullFn[len(h.localRoot):])

		// Skip hidden files folders
		if info.IsDir() {
			if strings.HasPrefix(fn, "/.") &&
				!strings.HasPrefix(fn, "/.well-known") {
				return filepath.SkipDir
			}
			return nil
		}

		var query string
		ext := path.Ext(fn)
		var outputFn string
		if ext == ".htex" {
			// Convert the filename into a URL pattern
			query = "/" + fn[:len(fn)-len(ext)]
			queryLen := len(query)
			if queryLen >= 6 && query[queryLen-6:] == "/index" {
				query = query[0 : queryLen-5]
			}
			outputFn = filepath.Join(outputDir, query, "index.html")
		} else {
			outputFn = filepath.Join(outputDir, fn)
		}

		// Print generated file
		fmt.Println(fullFn, "->", outputFn)

		os.MkdirAll(filepath.Dir(outputFn), os.ModePerm)

		if ext == ".htex" {
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
		} else {
			content, err := os.ReadFile(fullFn)
			if err != nil {
				log.Print(err)
			} else {
				os.WriteFile(outputFn, content, 0666)
			}
		}
		return nil
	})
}
