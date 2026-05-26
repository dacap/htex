// Copyright (c) David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"bufio"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
	ElemLayout
	ElemData
	ElemQuery
	ElemExec
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
	fn    string
	elems []Elem
}

type LayoutResolver func(string) *bufio.Scanner

type Htex struct {
	localRoot      string
	verbose        bool
	KeepComments   bool
	HttpHandler    http.Handler
	LayoutResolver LayoutResolver
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

func (h *Htex) parseHtexLayoutFile(w http.ResponseWriter, r *http.Request, fn string) (*HtexFile, error) {
	if h.verbose {
		log.Println(" -> parse layout file", fn)
	}
	var scanner *bufio.Scanner = nil
	if h.LayoutResolver != nil {
		scanner = h.LayoutResolver(fn)
	}
	if scanner == nil {
		file, err := os.Open(fn)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		defer file.Close()
		scanner = bufio.NewScanner(file)
	}
	return h.parseHtexScanner(w, r, fn, scanner)
}

func (h *Htex) parseHtexScanner(w http.ResponseWriter, r *http.Request, fn string, scanner *bufio.Scanner) (*HtexFile, error) {
	lexer := NewLexer()
	lexer.KeepComments = h.KeepComments
	tokens, err := lexer.lexScanner(fn, scanner)
	if err != nil {
		return nil, err
	}

	i := -1
	n := len(tokens.tokens)
	var token Token

	nextTok := func() Tok {
		if i+1 < n {
			return tokens.tokens[i+1].kind
		}
		return TokEof
	}

	advance := func() {
		i++
		if i < n {
			token = tokens.tokens[i]
		} else {
			token = Token{TokEof, "", false}
		}
	}

	expectTok := func(expected Tok) error {
		if nextTok() == expected {
			advance()
			return nil
		}
		return fmt.Errorf("expected token %v not found, %v found", expected, nextTok())
	}

	parsePath := func() string {
		var result string
		for token.kind != TokElemEnd {
			result += token.text
			if token.separated {
				result += " "
			}
			advance()
		}
		return result
	}

	hf := &HtexFile{fn: fn}
	advance()
	for i < n {
		elem := Elem{ElemNone, "", nil}

		switch token.kind {
		case TokText:
			elem = Elem{ElemText, token.text, nil}
			break
		case TokElemBegin:
			t := strings.ToLower(token.text[2:])

			if t == "layout" {
				advance()
				layoutFn := parsePath()
				layoutFn = h.solveUrlPathToLocalPath(fn, layoutFn)
				elem = Elem{ElemLayout, layoutFn, nil}
			} else if t == "content" {
				elem = Elem{ElemContent, "", nil}
			} else if t == "get" {
				err := expectTok(TokText)
				if err != nil {
					return hf, err
				}

				varName := token.text
				elem = Elem{ElemGet, varName, nil}
			} else if t == "set" {
				err := expectTok(TokText)
				if err != nil {
					return hf, err
				}

				varName := token.text
				var values *url.Values = nil
				advance()
				for token.kind == TokText {
					value := token.text
					if values == nil {
						values = &url.Values{}
					}
					values.Add(varName, value)
					advance()
				}
				elem = Elem{ElemSet, varName, values}
			} else if t == "url" {
				elem = Elem{ElemUrl, "", nil}
			} else if t == "data" {
				err := expectTok(TokText)
				if err != nil {
					return hf, err
				}

				paramName := token.text
				elem = Elem{ElemData, paramName, nil}
			} else if t == "query" {
				var key string
				if nextTok() == TokText {
					advance()
					key = token.text
				}
				elem = Elem{ElemQuery, key, nil}
			} else if t == "exec" {
				advance()
				command := parsePath()
				elem = Elem{ElemExec, command, nil}
			} else if t == "method" {
				var methodName string
				var values *url.Values = nil
				if nextTok() == TokText {
					advance()
					methodName = strings.ToLower(token.text)
					advance()
					for token.kind == TokText {
						nameAndValue := token.text
						name, value, _ := strings.Cut(nameAndValue, "=")
						if values == nil {
							values = &url.Values{}
						}
						values.Add(name, value)
						advance()
					}
				}
				elem = Elem{ElemMethod, methodName, values}
			} else if t == "include-raw" {
				advance()
				includeFn := parsePath()
				elem = Elem{ElemIncludeRaw, includeFn, nil}
			} else if t == "include-escaped" {
				advance()
				includeFn := parsePath()
				elem = Elem{ElemIncludeEscaped, includeFn, nil}
			} else if t == "include-markdown" {
				advance()
				includeFn := parsePath()
				elem = Elem{ElemIncludeMarkdown, includeFn, nil}
			} else {
				log.Println("invalid htex element", t)
			}

			for token.kind != TokElemEnd {
				advance()
			}
		}

		if elem.kind != ElemNone {
			hf.elems = append(hf.elems, elem)
		}

		advance()
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

func (h *Htex) writeHtexFile0(w http.ResponseWriter, r *http.Request, hf *HtexFile, content func(http.ResponseWriter, *http.Request), searchLayout bool) {
	methodName := strings.ToLower(r.Method)
	query := r.URL.Query()

	// Find the layout that matches the HTTP method/query the most
	var layout *HtexFile = nil
	skipUntilNewMethod := false
	if searchLayout {
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
			} else if elem.kind == ElemLayout {
				layoutFn := elem.text
				var err error
				layout, err = h.parseHtexLayoutFile(w, r, layoutFn)
				if err != nil {
					log.Println("layout not found:", hf.fn)
					http.Error(w, "500 internal error", http.StatusInternalServerError)
					return
				}
			} else {
				// TODO what to do with ElemGet/ElemSet?
			}
		}
	}

	if layout != nil {
		h.writeHtexFile(w, r, layout,
			func(w http.ResponseWriter, r *http.Request) {
				h.writeHtexFile0(w, r, hf, content, false)
			})
		return
	}

	vars := make(map[string]string)
	skipUntilNewMethod = false
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
		} else if elem.kind == ElemExec {
			args := strings.Fields(elem.text)
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = h.localRoot
			out, err := cmd.Output()
			if err != nil {
				log.Print(err)
			} else {
				w.Write([]byte(html.EscapeString(string(out))))
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

func (h *Htex) writeHtexFile(w http.ResponseWriter, r *http.Request, hf *HtexFile, content func(http.ResponseWriter, *http.Request)) {
	h.writeHtexFile0(w, r, hf, content, true)
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
			h.writeHtexFile(w, r, hf, nil)
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
			h.writeHtexFile(w, r, hf, nil)
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
		localRoot:      localRoot,
		verbose:        verbose,
		KeepComments:   false,
		HttpHandler:    nil,
		LayoutResolver: nil,
	}
	if verbose {
		h.HttpHandler = &LogHtexHandler{handler: h}
	} else {
		h.HttpHandler = h
	}
	return h
}
