// Copyright 2025 David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

type ElemKind int

const (
	ElemNone ElemKind = iota
	ElemText
	ElemContent
	ElemMethod
	ElemData
	ElemIncludeRaw
	ElemIncludeEscaped
)

type Elem struct {
	kind ElemKind
	text string
}

type HtexFile struct {
	fn     string
	elems  []Elem
	layout *HtexFile
}

func splitHtexTokens() func([]byte, bool) (int, []byte, error) {
	insideHtexElem := false
	closingHtexElem := false
	return func(data []byte, atEOF bool) (int, []byte, error) {
		if closingHtexElem {
			closingHtexElem = false
			return 1, data[0:1], nil
		}
		for i := 0; i < len(data); i++ {
			if insideHtexElem {
				if data[i] == ' ' || data[i] == '\r' || data[i] == '\n' {
					for ; i < len(data) &&
						(data[i] == ' ' ||
							data[i] == '\r' ||
							data[i] == '\n'); i++ {
					}
					return i, data[:i], nil
				} else if data[i] == '>' {
					insideHtexElem = false
					closingHtexElem = true
					return i, data[:i], nil
				}
			}
			if data[i] == '<' && data[i+1] == '!' &&
				!bytes.Equal(data[i+2:i+9], []byte("DOCTYPE")) &&
				!bytes.Equal(data[i+2:i+9], []byte("doctype")) {
				insideHtexElem = true
				if i > 0 {
					return i, data[:i], nil
				}
				var j int
				k := len(data)
				for j = 0; j < len(data); j++ {
					if data[j] == ' ' {
						k = j + 1
						break
					} else if data[j] == '>' {
						k = j
						break
					}
				}
				return k, data[i:j], nil
			}
		}
		if !atEOF {
			return 0, nil, nil
		}
		return 0, data, bufio.ErrFinalToken
	}
}

type Htex struct {
	localRoot   string
	verbose     bool
	HttpHandler http.Handler
}

// relativeTo is a path to the current local filename that is being
// processed (so relative URLs will be relative to the directory of
// this file)
func (h *Htex) solveUrlPathToLocalPath(relativeTo string, urlPath string) string {
	if urlPath[0] == '/' {
		return path.Join(h.localRoot, urlPath)
	} else {
		return path.Join(path.Dir(relativeTo), urlPath)
	}
}

func (h *Htex) parseHtexFile(w http.ResponseWriter, r *http.Request, fn string) (*HtexFile, error) {
	if h.verbose {
		log.Println(" -> parse file", fn)
	}

	file, err := os.Open(fn)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer file.Close()

	htexFile := &HtexFile{fn: fn}
	insideHtexElem := false
	var tok string
	scanner := bufio.NewScanner(file)
	scanner.Split(splitHtexTokens())
	for scanner.Scan() {
		elem := Elem{ElemNone, ""}
		tok = scanner.Text()
		if len(tok) > 2 && tok[0] == '<' && tok[1] == '!' {
			if strings.HasPrefix(tok, "<!DOCTYPE") ||
				strings.HasPrefix(tok, "<!doctype") {
				elem = Elem{ElemText, tok}
			} else {
				insideHtexElem = true
				if strings.HasPrefix(tok, "<!layout") {
					scanner.Scan()
					layoutFn := h.solveUrlPathToLocalPath(fn, scanner.Text())
					layout, err := h.parseHtexFile(w, r, layoutFn)
					if layout != nil {
						htexFile.layout = layout
					} else if err != nil {
						http.Error(w, "500 internal error", http.StatusInternalServerError)
						return nil, err
					}
				} else if tok == "<!content" {
					elem = Elem{ElemContent, tok}
				} else if tok == "<!data" {
					scanner.Scan()
					paramName := scanner.Text()
					elem = Elem{ElemData, paramName}
				} else if tok == "<!method" {
					scanner.Scan()
					methodName := scanner.Text()
					elem = Elem{ElemMethod, strings.ToLower(methodName)}
				} else if tok == "<!include-raw" {
					scanner.Scan()
					includeFn := scanner.Text()
					elem = Elem{ElemIncludeRaw, includeFn}
				} else if tok == "<!include-escaped" {
					scanner.Scan()
					includeFn := scanner.Text()
					elem = Elem{ElemIncludeEscaped, includeFn}
				} else {
					log.Println("invalid htex element", tok)
				}
			}
		} else if insideHtexElem {
			if tok == ">" {
				insideHtexElem = false
			}
		} else {
			elem = Elem{ElemText, tok}
		}
		if elem.kind != ElemNone {
			htexFile.elems = append(htexFile.elems, elem)
		}
	}
	return htexFile, nil
}

func (h *Htex) writeHtexFile(w http.ResponseWriter, r *http.Request, htexFile *HtexFile, layout *HtexFile, content func(http.ResponseWriter, *http.Request)) {
	if layout != nil {
		h.writeHtexFile(w, r, layout, layout.layout,
			func(w http.ResponseWriter, r *http.Request) {
				h.writeHtexFile(w, r, htexFile, nil, content)
			})
		return
	}

	skipUntilNewMethod := false
	methodName := strings.ToLower(r.Method)
	for _, elem := range htexFile.elems {
		if elem.kind == ElemMethod {
			if elem.text == methodName || elem.text == "any" {
				skipUntilNewMethod = false
			} else {
				skipUntilNewMethod = true
				continue
			}
		} else if skipUntilNewMethod {
			continue
		} else if elem.kind == ElemContent {
			if content != nil {
				content(w, r)
			} else {
				// <!content> is used without parent file, this can
				// happen if we access the layout directly from the
				// URL. This is an accepted behavior, and we replace
				// <!content> element with nothing.
			}
		} else if elem.kind == ElemData {
			if r.Form.Has(elem.text) {
				w.Write([]byte(r.Form[elem.text][0]))
			}
		} else if elem.kind == ElemIncludeRaw || elem.kind == ElemIncludeEscaped {
			fn := h.solveUrlPathToLocalPath(htexFile.fn, elem.text)
			content, err := os.ReadFile(fn)
			if elem.kind == ElemIncludeEscaped {
				content = []byte(html.EscapeString(string(content)))
			}
			if err != nil {
				log.Println(err)
			} else {
				w.Write(content)
			}
		} else if elem.kind == ElemText {
			w.Write([]byte(elem.text))
		}
	}
}

func (h *Htex) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	verbose := h.verbose
	url := path.Clean(r.URL.String())
	if verbose {
		log.Println(r.RemoteAddr, r.Method, r.URL, url)
	}

	fn := path.Join(h.localRoot, url)
	base := path.Base(fn)

	if base == "." {
		fn = path.Join(path.Dir(fn), "index")
	}

	// Ignore requests to access ".htex" files as static content
	ext := path.Ext(fn)
	if ext == ".htex" {
		http.NotFound(w, r)
		return
	}

	// Ignore all requests to hidden folders/files (except
	// "/.well-known" which is used to verify
	// domains/certificates).
	if strings.Contains(url, "/.") &&
		!strings.HasPrefix(url, "/.well-known") {
		if verbose {
			log.Println(" -> ignore hidden dir", fn)
		}
		http.NotFound(w, r)
		return
	}

	s, _ := os.Stat(fn)

	// Static files
	if s != nil && s.Mode().IsRegular() {
		if verbose {
			log.Println(" -> static file", fn)
		}
		http.ServeFile(w, r, fn)
		return
	}

	// Directory files
	if s != nil && s.Mode().IsDir() {
		fn = fn + "/index"
	}

	fn = fn + ".htex"
	s, _ = os.Stat(fn)
	if s != nil && s.Mode().IsRegular() {
		hdr := w.Header()
		hdr.Set("Content-Type", "text/html; charset=utf-8")
		if h.verbose {
			log.Println(" -> dynamic file", fn)
		}
		htexFile, _ := h.parseHtexFile(w, r, fn)
		if htexFile != nil {
			r.ParseForm()
			h.writeHtexFile(w, r, htexFile, htexFile.layout, nil)
		}
		return
	}

	http.NotFound(w, r)
}

func (h *Htex) RunWebServer(port int, fullchain string, privkey string) {
	s, err := os.Stat(h.localRoot)
	if err != nil || s == nil || !s.Mode().IsDir() {
		log.Fatalln("cannot open directory:", h.localRoot)
	}

	if fullchain != "" && privkey != "" {
		// Start HTTPS server
		if port == 0 {
			port = 443
		}
		fmt.Printf("htex server at https://localhost:%d for %s\n", port, h.localRoot)
		log.Fatal(http.ListenAndServeTLS(
			fmt.Sprint(":", port), fullchain, privkey, h.HttpHandler))
	} else {
		// Start HTTP server
		if port == 0 {
			port = 80
		}
		fmt.Printf("htex server at http://localhost:%d for %s\n", port, h.localRoot)
		log.Fatal(http.ListenAndServe(fmt.Sprint(":", port), h.HttpHandler))
	}
}

func NewHtex(localRoot string, verbose bool) *Htex {
	h := &Htex{localRoot, verbose, nil}
	if verbose {
		h.HttpHandler = &LogHtexHandler{handler: h}
	} else {
		h.HttpHandler = h
	}
	return h
}
