// Copyright 2025 David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var localRoot string
var verbose bool

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

func SplitHtexTokens() func([]byte, bool) (int, []byte, error) {
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
				(!bytes.Equal(data[i+2:i+9], []byte("DOCTYPE")) ||
					!bytes.Equal(data[i+2:i+9], []byte("doctype"))) {
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

// relativeTo is a path to the current local filename that is being
// processed (so relative URLs will be relative to the directory of
// this file)
func SolveUrlPathToLocalPath(relativeTo string, urlPath string) string {
	if urlPath[0] == '/' {
		return path.Join(localRoot, urlPath)
	} else {
		return path.Join(path.Dir(relativeTo), urlPath)
	}
}

func ParseHtexFile(r *http.Request, fn string) *HtexFile {
	if verbose {
		log.Println(" -> parse file", fn)
	}

	file, err := os.Open(fn)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer file.Close()

	htexFile := &HtexFile{fn: fn}
	insideHtexElem := false
	var tok string
	scanner := bufio.NewScanner(file)
	scanner.Split(SplitHtexTokens())
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
					layoutFn := SolveUrlPathToLocalPath(fn, scanner.Text())
					layout := ParseHtexFile(r, layoutFn)
					if layout != nil {
						htexFile.layout = layout
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
	return htexFile
}

func WriteHtexFile(w http.ResponseWriter, r *http.Request, htexFile *HtexFile, layout *HtexFile, content func(http.ResponseWriter, *http.Request)) {
	if layout != nil {
		WriteHtexFile(w, r, layout, layout.layout,
			func(w http.ResponseWriter, r *http.Request) { WriteHtexFile(w, r, htexFile, nil, content) })
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
			fn := SolveUrlPathToLocalPath(htexFile.fn, elem.text)
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

type HtexHandler struct {
}

func (hh *HtexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	url := path.Clean(r.URL.String())
	if verbose {
		log.Println(r.RemoteAddr, r.Method, r.URL, url)
	}

	fn := path.Join(localRoot, url)
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
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		if verbose {
			log.Println(" -> dynamic file", fn)
		}
		htexFile := ParseHtexFile(r, fn)
		if htexFile != nil {
			r.ParseForm()
			WriteHtexFile(w, r, htexFile, htexFile.layout, nil)
		}
		return
	}

	http.NotFound(w, r)
}

func main() {
	var fullchain, privkey string
	var port int
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.StringVar(&fullchain, "fullchain", "", "TLS certificate")
	flag.StringVar(&privkey, "privkey", "", "private key for the TLS certificate")
	flag.IntVar(&port, "port", 0, "port to listen (80 or 443 by default)")
	flag.Parse()
	n := len(flag.Args())
	if n > 0 {
		localRoot, _ = filepath.Abs(flag.Args()[0])
	} else {
		localRoot, _ = filepath.Abs("public")
	}

	s, err := os.Stat(localRoot)
	if err != nil || s == nil || !s.Mode().IsDir() {
		log.Fatalln("cannot open directory:", localRoot)
	}

	var handler http.Handler
	if verbose {
		handler = &LogHtexHandler{}
	} else {
		handler = &HtexHandler{}
	}

	if fullchain != "" && privkey != "" {
		// Start HTTPS server
		if port == 0 {
			port = 443
		}
		log.Printf("htex server running https://localhost:%d\n -> local dir %s\n", port, localRoot)
		log.Fatal(http.ListenAndServeTLS(
			fmt.Sprint(":", port), fullchain, privkey, handler))
	} else {
		// Start HTTP server
		if port == 0 {
			port = 80
		}
		log.Printf("htex server running https://localhost:%d\n -> local dir %s\n", port, localRoot)
		log.Fatal(http.ListenAndServe(fmt.Sprint(":", port), handler))
	}
}
