// Copyright (c) David Capello. All rights reserved.
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
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gomarkdown/markdown"
	mhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

type ElemKind int

const (
	ElemNone ElemKind = iota
	ElemText
	ElemContent
	ElemGet // <!get varname>
	ElemSet // <!set varname value>
	ElemUrl
	ElemMethod
	ElemData
	ElemQuery
	ElemIncludeRaw
	ElemIncludeEscaped
	ElemIncludeMarkdown
)

type Elem struct {
	kind   ElemKind
	text   string
	values *url.Values
}

type HtexFile struct {
	fn     string
	elems  []Elem
	layout *HtexFile
}

type Htex struct {
	localRoot    string
	verbose      bool
	KeepComments bool
	HttpHandler  http.Handler
}

func splitHtexTokens(h *Htex) func([]byte, bool) (int, []byte, error) {
	insideHtexElem := false
	insideComment := false
	closingHtexElem := false
	return func(data []byte, atEOF bool) (int, []byte, error) {
		if closingHtexElem {
			closingHtexElem = false
			return 1, data[0:1], nil
		}
		for i := 0; i < len(data); i++ {
			if insideHtexElem {
				if data[i] == ' ' || data[i] == '\r' || data[i] == '\n' {
					var j = i
					for ; i < len(data) &&
						(data[i] == ' ' ||
							data[i] == '\r' ||
							data[i] == '\n'); i++ {
					}
					return i, data[:j], nil
				} else if data[i] == '>' {
					insideHtexElem = false
					closingHtexElem = true
					return i, data[:i], nil
				}
			} else if insideComment {
				// Here h.KeepComments is always false, as if we keep
				// comments it will be part of a ElemText, not a
				// separated token.
				if !h.KeepComments {
					insideComment = false
					for ; i+3 < len(data) &&
						(data[i] != '-' || data[i+1] != '-' || data[i+2] != '>'); i++ {
					}
					i += 3
					return i, data[:i], nil
				}
			}
			if i+2 < len(data) && data[i] == '<' && data[i+1] == '!' &&
				!bytes.EqualFold(data[i+2:i+9], []byte("doctype")) {

				// Starting HTML comment "<!--"...
				if data[i+2] == '-' && data[i+3] == '-' {
					// If we're going to keep comments, we just pass
					// the whole comment and make it part of the next
					// ElemText token.
					if h.KeepComments {
						for ; i+3 < len(data) &&
							(data[i] != '-' || data[i+1] != '-' || data[i+2] != '>'); i++ {
						}
						i += 2
					} else {
						insideComment = true
						return i, data[:i], nil
					}
				} else {
					// <!htex-tag...
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
							closingHtexElem = true
							k = j
							break
						}
					}
					return k, data[i:j], nil
				}
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
func (h *Htex) solveUrlPathToLocalPath(relativeTo string, urlPath string) string {
	if urlPath[0] == '/' {
		return filepath.Join(h.localRoot, urlPath)
	} else {
		return filepath.Join(filepath.Dir(relativeTo), urlPath)
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

	scanner := bufio.NewScanner(file)
	return h.parseHtexScanner(w, r, fn, scanner)
}

func (h *Htex) parseHtexScanner(w http.ResponseWriter, r *http.Request, fn string, scanner *bufio.Scanner) (*HtexFile, error) {
	hf := &HtexFile{fn: fn}
	insideHtexElem := false
	var tok string
	scanner.Split(splitHtexTokens(h))

	nextToken := true
	for true {
		if nextToken {
			if !scanner.Scan() {
				break
			}
		} else {
			nextToken = true
		}

		elem := Elem{ElemNone, "", nil}
		tok = scanner.Text()
		if len(tok) > 2 && tok[0] == '<' && tok[1] == '!' {
			lowerTok := strings.ToLower(tok)
			if strings.HasPrefix(lowerTok, "<!doctype") ||
				(h.KeepComments && strings.HasPrefix(tok, "<!--")) {
				elem = Elem{ElemText, tok, nil}
			} else {
				insideHtexElem = true
				if strings.HasPrefix(lowerTok, "<!layout") {
					if !scanner.Scan() {
						break
					}
					if scanner.Text() == ">" {
						nextToken = false
						continue
					}

					layoutFn := h.solveUrlPathToLocalPath(fn, scanner.Text())
					layout, err := h.parseHtexFile(w, r, layoutFn)
					if layout != nil {
						hf.layout = layout
					} else if err != nil {
						http.Error(w, "500 internal error", http.StatusInternalServerError)
						return nil, err
					}
				} else if lowerTok == "<!content" {
					elem = Elem{ElemContent, "", nil}
				} else if lowerTok == "<!get" {
					if !scanner.Scan() {
						break
					}
					varName := scanner.Text()
					elem = Elem{ElemGet, varName, nil}
				} else if lowerTok == "<!set" {
					if !scanner.Scan() {
						break
					}
					varName := scanner.Text()
					var values *url.Values = nil
					for scanner.Scan() {
						value := scanner.Text()
						if value == ">" {
							nextToken = false
							break
						}
						if values == nil {
							values = &url.Values{}
						}
						values.Add(varName, value)
					}
					elem = Elem{ElemSet, varName, values}
				} else if lowerTok == "<!url" {
					elem = Elem{ElemUrl, "", nil}
				} else if lowerTok == "<!data" {
					if !scanner.Scan() {
						break
					}
					if scanner.Text() == ">" {
						nextToken = false
						continue
					}
					paramName := scanner.Text()
					elem = Elem{ElemData, paramName, nil}
				} else if lowerTok == "<!query" {
					if !scanner.Scan() {
						break
					}
					var key string
					if scanner.Text() == ">" {
						nextToken = false
					} else {
						key = scanner.Text()
					}
					elem = Elem{ElemQuery, key, nil}
				} else if lowerTok == "<!method" {
					var methodName string
					var values *url.Values = nil

					if !scanner.Scan() {
						// At least '>' was expected
						break
					}

					if scanner.Text() == ">" {
						nextToken = false
					} else {
						methodName = scanner.Text()

						for scanner.Scan() {
							nameAndValue := scanner.Text()
							if nameAndValue == ">" {
								nextToken = false
								break
							}
							name, value, _ := strings.Cut(nameAndValue, "=")
							if values == nil {
								values = &url.Values{}
							}
							values.Add(name, value)
						}
					}
					elem = Elem{ElemMethod, strings.ToLower(methodName), values}
				} else if lowerTok == "<!include-raw" {
					if !scanner.Scan() {
						break
					}
					if scanner.Text() == ">" {
						nextToken = false
						continue
					}
					includeFn := scanner.Text()
					elem = Elem{ElemIncludeRaw, includeFn, nil}
				} else if lowerTok == "<!include-escaped" {
					scanner.Scan()
					includeFn := scanner.Text()
					elem = Elem{ElemIncludeEscaped, includeFn, nil}
				} else if lowerTok == "<!include-markdown" {
					scanner.Scan()
					includeFn := scanner.Text()
					elem = Elem{ElemIncludeMarkdown, includeFn, nil}
				} else if strings.HasPrefix(tok, "<!--") {
					// Ignore the whole comment token (which includes "<!-- ... -->")
					insideHtexElem = false
				} else {
					log.Println("invalid htex element", tok)
				}
			}
		} else if insideHtexElem {
			if tok == ">" {
				insideHtexElem = false
			}
		} else if tok != "" {
			elem = Elem{ElemText, tok, nil}
		}
		if elem.kind != ElemNone {
			hf.elems = append(hf.elems, elem)
		}
	}
	return hf, nil
}

func matchQuery(a *url.Values, b *url.Values) bool {
	for k, v := range *a {
		if !b.Has(k) {
			return false
		}
		if len(v) != 0 && v[0] != "" {
			u := b.Get(k)
			if u != v[0] {
				return false
			}
		}
	}
	return true
}

func markdownToHtml(md []byte) []byte {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)
	htmlFlags := mhtml.CommonFlags | mhtml.HrefTargetBlank
	opts := mhtml.RendererOptions{Flags: htmlFlags}
	renderer := mhtml.NewRenderer(opts)
	return markdown.Render(doc, renderer)
}

func (h *Htex) writeHtexFile(w http.ResponseWriter, r *http.Request, hf *HtexFile, layout *HtexFile, content func(http.ResponseWriter, *http.Request)) {
	if layout != nil {
		h.writeHtexFile(w, r, layout, layout.layout,
			func(w http.ResponseWriter, r *http.Request) {
				h.writeHtexFile(w, r, hf, nil, content)
			})
		return
	}

	query := r.URL.Query()
	vars := make(map[string]string)

	skipUntilNewMethod := false
	methodName := strings.ToLower(r.Method)
	for _, elem := range hf.elems {
		if elem.kind == ElemMethod {
			if ((elem.text == methodName) && (elem.values == nil || matchQuery(elem.values, &query))) ||
				elem.text == "any" {
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
		} else if elem.kind == ElemGet {
			value, exist := vars[elem.text]
			if exist {
				w.Write([]byte(value))
			}
		} else if elem.kind == ElemSet {
			if elem.values != nil {
				vars[elem.text] = (*elem.values)[elem.text][0]
			} else {
				delete(vars, elem.text)
			}
		} else if elem.kind == ElemUrl {
			w.Write([]byte(path.Clean(r.URL.Path)))
		} else if elem.kind == ElemData {
			if r.Form.Has(elem.text) {
				w.Write([]byte(r.Form[elem.text][0]))
			}
		} else if elem.kind == ElemQuery {
			if len(elem.text) > 0 {
				if query.Has(elem.text) {
					w.Write([]byte(query.Get(elem.text)))
				}
			} else {
				w.Write([]byte(r.URL.RawQuery))
			}
		} else if elem.kind == ElemIncludeRaw ||
			elem.kind == ElemIncludeEscaped ||
			elem.kind == ElemIncludeMarkdown {

			fn := h.solveUrlPathToLocalPath(hf.fn, elem.text)
			content, err := os.ReadFile(fn)
			if elem.kind == ElemIncludeEscaped {
				content = []byte(html.EscapeString(string(content)))
			} else if elem.kind == ElemIncludeMarkdown {
				content = markdownToHtml(content)
			}

			if err != nil {
				log.Print(err)
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
	url := path.Clean(r.URL.Path)
	if verbose {
		log.Println(r.RemoteAddr, r.Method, r.URL)
	}

	fn := path.Join(h.localRoot, url)
	base := path.Base(fn)

	if base == "." {
		fn = filepath.Join(filepath.Dir(fn), "index")
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

	// Dynamic content from .htex file
	s, _ = os.Stat(fn + ".htex")
	if s != nil && s.Mode().IsRegular() {
		fn = fn + ".htex"
		hdr := w.Header()
		hdr.Set("Content-Type", "text/html; charset=utf-8")
		if h.verbose {
			log.Println(" -> dynamic file", fn)
		}
		hf, _ := h.parseHtexFile(w, r, fn)
		if hf != nil {
			r.ParseForm()
			h.writeHtexFile(w, r, hf, hf.layout, nil)
		}
		return
	}

	// Wildcard handler from "_.htex" file
	fnDir, _ := filepath.Split(fn)
	wildcardFn := filepath.Join(fnDir, "_.htex")
	s, _ = os.Stat(wildcardFn)
	if s != nil && s.Mode().IsRegular() {
		fn = wildcardFn
		hdr := w.Header()
		hdr.Set("Content-Type", "text/html; charset=utf-8")
		if h.verbose {
			log.Println(" -> dynamic file", fn)
		}
		hf, _ := h.parseHtexFile(w, r, fn)
		if hf != nil {
			r.ParseForm()
			h.writeHtexFile(w, r, hf, hf.layout, nil)
		}
		return
	}

	// Static content from .html file. Generally this is only for
	// the index.html when we access / or other URL path without
	// index.html and there is no index.htex first. Any other
	// static .html file is served with the first http.ServeFile()
	s, _ = os.Stat(fn + ".html")
	if s != nil && s.Mode().IsRegular() {
		fn = fn + ".html"
		hdr := w.Header()
		hdr.Set("Content-Type", "text/html; charset=utf-8")
		if h.verbose {
			log.Println(" -> static file", fn)
		}
		http.ServeFile(w, r, fn)
		return
	}

	// 404
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
	h := &Htex{
		localRoot:    localRoot,
		verbose:      verbose,
		KeepComments: false,
		HttpHandler:  nil,
	}
	if verbose {
		h.HttpHandler = &LogHtexHandler{handler: h}
	} else {
		h.HttpHandler = h
	}
	return h
}
