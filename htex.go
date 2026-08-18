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
	ElemIf
	ElemElseIf
	ElemElse
	ElemEnd
)

type Elem struct {
	kind    ElemKind
	text    string
	values  *url.Values
	jump    int
	jumpEnd int
}

func newElem(kind ElemKind, text string) Elem {
	return Elem{kind, text, nil, 0, 0}
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

	// Auxiliary structure to keep track of the current level of
	// <!if><!elseif><!else><!end> elements to update their
	// jump/jumpEnd fields.
	type Ifs struct {
		idxs []int
	}

	ti := newTokensIter(tokens)
	var ifs []Ifs

	parsePath := func() string {
		var result string
		for ti.token.kind != TokElemEnd {
			result += ti.token.text
			if ti.token.separated {
				result += " "
			}
			ti.advance()
		}
		return result
	}

	hf := &HtexFile{fn: fn}
	lastMethod := -1
	for ti.advance() {
		elem := newElem(ElemNone, "")

		switch ti.token.kind {
		case TokText:
			elem = newElem(ElemText, ti.token.text)
			break
		case TokElemBegin:
			t := strings.ToLower(ti.token.text[2:])

			if t == "layout" {
				ti.advance()
				layoutFn := parsePath()
				layoutFn = h.solveUrlPathToLocalPath(fn, layoutFn)
				elem = newElem(ElemLayout, layoutFn)
			} else if t == "content" {
				elem = newElem(ElemContent, "")
			} else if t == "get" {
				err := ti.expectTok(TokText)
				if err != nil {
					return hf, err
				}

				varName := ti.token.text
				elem = newElem(ElemGet, varName)
			} else if t == "set" {
				err := ti.expectTok(TokText)
				if err != nil {
					return hf, err
				}

				varName := ti.token.text
				var values *url.Values = nil
				ti.advance()
				for ti.token.kind == TokText {
					value := ti.token.text
					if values == nil {
						values = &url.Values{}
					}
					values.Add(varName, value)
					ti.advance()
				}
				elem = newElem(ElemSet, varName)
				elem.values = values
			} else if t == "url" {
				elem = newElem(ElemUrl, "")
			} else if t == "data" {
				err := ti.expectTok(TokText)
				if err != nil {
					return hf, err
				}

				paramName := ti.token.text
				elem = newElem(ElemData, paramName)
			} else if t == "query" {
				var key string
				if ti.nextTok() == TokText {
					ti.advance()
					key = ti.token.text
				}
				elem = newElem(ElemQuery, key)
			} else if t == "exec" {
				ti.advance()
				command := parsePath()
				elem = newElem(ElemExec, command)
			} else if t == "method" {
				var methodName string
				var values *url.Values = nil
				if ti.nextTok() == TokText {
					ti.advance()
					methodName = strings.ToLower(ti.token.text)
					ti.advance()
					for ti.token.kind == TokText {
						nameAndValue := ti.token.text
						name, value, _ := strings.Cut(nameAndValue, "=")
						if values == nil {
							values = &url.Values{}
						}
						values.Add(name, value)
						ti.advance()
					}
				}
				elem = newElem(ElemMethod, methodName)
				elem.values = values
				if lastMethod >= 0 {
					hf.elems[lastMethod].jump = len(hf.elems)
				}
				lastMethod = len(hf.elems)
			} else if t == "include-raw" {
				ti.advance()
				includeFn := parsePath()
				elem = newElem(ElemIncludeRaw, includeFn)
			} else if t == "include-escaped" {
				ti.advance()
				includeFn := parsePath()
				elem = newElem(ElemIncludeEscaped, includeFn)
			} else if t == "include-markdown" {
				ti.advance()
				includeFn := parsePath()
				elem = newElem(ElemIncludeMarkdown, includeFn)
			} else if t == "if" {
				ti.advance()
				elem = newElem(ElemIf, ti.token.text)
				ifs = append(ifs, Ifs{[]int{len(hf.elems)}})
			} else if t == "elseif" {
				n := len(ifs)
				if n == 0 {
					return nil, fmt.Errorf("unexpected element <!elseif> without <!if>")
				}
				hf.elems[ifs[n-1].idxs[len(ifs[n-1].idxs)-1]].jump = len(hf.elems)
				ifs[n-1].idxs = append(ifs[n-1].idxs, len(hf.elems))

				ti.advance()
				elem = newElem(ElemElseIf, ti.token.text)
			} else if t == "else" {
				n := len(ifs)
				if n == 0 {
					return nil, fmt.Errorf("unexpected element <!else> without <!if>")
				}
				hf.elems[ifs[n-1].idxs[len(ifs[n-1].idxs)-1]].jump = len(hf.elems)
				ifs[n-1].idxs = append(ifs[n-1].idxs, len(hf.elems))

				elem = newElem(ElemElse, "")
			} else if t == "end" {
				n := len(ifs)
				if n == 0 {
					return nil, fmt.Errorf("unexpected element <!end> without <!if>")
				}
				endIdx := len(hf.elems)
				for j := 0; j < len(ifs[n-1].idxs); j++ {
					hf.elems[ifs[n-1].idxs[j]].jumpEnd = endIdx
				}
				ifs = ifs[:n-1]

				elem = newElem(ElemEnd, "")
			} else {
				log.Println("invalid htex element", t)
			}

			for ti.token.kind != TokElemEnd {
				ti.advance()
			}
		}

		if elem.kind != ElemNone {
			hf.elems = append(hf.elems, elem)
		}
	}
	if lastMethod >= 0 {
		hf.elems[lastMethod].jump = len(hf.elems)
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

	var insideIf []bool
	vars := make(map[string]string)
	n := len(hf.elems)
	for i := 0; i < n; i++ {
		elem := hf.elems[i]

		if elem.kind == ElemMethod {
			if ((elem.text == methodName) && (elem.values == nil || matchQuery(elem.values, &query))) ||
				elem.text == "any" {
				// Do nothing
			} else {
				i = elem.jump - 1
			}
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
		} else if elem.kind == ElemIf {
			// TODO evaluate any kind of expression from "elem"
			cond := (elem.text == "true")
			insideIf = append(insideIf, cond)

			if cond {
				// Continue with elements inside <!if>...<!end>
			} else if elem.jump > 0 {
				i = elem.jump - 1
			} else if elem.jumpEnd > 0 {
				i = elem.jumpEnd - 1
			}
		} else if elem.kind == ElemElseIf {
			if len(insideIf) == 0 {
				log.Println("unexpected <!elseif> without <!if>")
				return
			}
			if insideIf[len(insideIf)-1] {
				// Go to <!end> as we already entered in the first <!if>
				i = elem.jumpEnd - 1
			} else {
				// TODO evaluate any kind of expression from "elem"
				if elem.text == "true" {
					insideIf[len(insideIf)-1] = true
				} else if elem.jump > 0 {
					i = elem.jump - 1
				} else if elem.jumpEnd > 0 {
					i = elem.jumpEnd - 1
				}
			}
		} else if elem.kind == ElemElse {
			if len(insideIf) == 0 {
				log.Print("unexpected <!else> without <!if>")
				return
			}
			if insideIf[len(insideIf)-1] {
				// Go to <!end> as we already entered in the first <!if>
				i = elem.jumpEnd - 1
			} else {
				// Enter in the <!else>...<!end>
			}
		} else if elem.kind == ElemEnd {
			insideIf = insideIf[:len(insideIf)-1]
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
