// Copyright 2025 David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"bufio"
	"bytes"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type memoryResponseWriter struct {
	buf bytes.Buffer
	hdr http.Header
}

func (w *memoryResponseWriter) Header() http.Header {
	return w.hdr
}

func (w *memoryResponseWriter) Write(buf []byte) (int, error) {
	w.buf.Write(buf)
	return 200, nil
}

func (w *memoryResponseWriter) WriteHeader(statusCode int) {
	// Do nothing
}

type ParseTest struct {
	reqStr   string
	text     string
	expected string
	elems    []ElemKind
}

func testParsing(h *Htex, t *testing.T, tests []ParseTest) {
	for _, test := range tests {
		w := &memoryResponseWriter{}

		method, urlPath, _ := strings.Cut(test.reqStr, " ")
		r := &http.Request{Method: method}
		r.URL, _ = url.ParseRequestURI(urlPath)

		s := bufio.NewScanner(strings.NewReader(test.text))
		hf, err := h.parseHtexScanner(w, r, "test.htex", s)
		if err != nil {
			t.Error(err)
		}

		// Check tokens
		if len(hf.elems) != len(test.elems) {
			t.Errorf("parsing '%s' generated %d elements (expected '%d')\n", test.text, len(hf.elems), len(test.elems))
			t.Errorf("result elements (%d):\n", len(hf.elems))
			for i := 0; i < len(hf.elems); i++ {
				t.Errorf(" [%d] = %v\n", i, hf.elems[i])
			}
			t.Errorf("expected elements (%d):\n", len(test.elems))
			for i := 0; i < len(test.elems); i++ {
				t.Errorf(" [%d] = %v\n", i, test.elems[i])
			}
			return
		}
		for i := 0; i < len(test.elems); i++ {
			if test.elems[i] != hf.elems[i].kind {
				t.Errorf("parsing '%s' token %d is %v (expected %v)\n", test.text, i, hf.elems[i].kind, test.elems[i])
			}
		}

		h.writeHtexFile(w, r, hf, nil, nil)
		result := w.buf.String()
		if result != test.expected {
			t.Errorf("parsing '%s' => '%s' (expected '%s')\n", test.text, result, test.expected)
		}
	}
}

func TestBasic(t *testing.T) {
	tests := []ParseTest{
		{
			"GET /",
			"",
			"",
			[]ElemKind{},
		},
		{
			"GET /",
			"a",
			"a",
			[]ElemKind{ElemText},
		},
	}
	h := NewHtex(".", false)
	testParsing(h, t, tests)
}

func TestSkipComments(t *testing.T) {
	tests := []ParseTest{
		{
			"GET /",
			"a<!-->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"GET /",
			"a<!--",
			"a",
			[]ElemKind{ElemText},
		},
		{
			"GET /",
			"a<!--->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"GET /",
			"a<!---->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"GET /",
			"a<!-- c -->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"GET /",
			"abc<!-- c -->",
			"abc",
			[]ElemKind{ElemText},
		},
		{
			"GET /",
			"abcd<!-- c --",
			"abcd",
			[]ElemKind{ElemText},
		},
	}
	h := NewHtex(".", false)
	testParsing(h, t, tests)
}

func TestKeepComments(t *testing.T) {
	tests := []ParseTest{
		{
			"GET /",
			"a<!-->b",
			"a<!-->b",
			[]ElemKind{ElemText},
		},
		{
			"GET /",
			"a<!-- c -->b",
			"a<!-- c -->b",
			[]ElemKind{ElemText},
		},
		{
			"GET /",
			"abc<!-- c -->",
			"abc<!-- c -->",
			[]ElemKind{ElemText},
		},
		{
			"GET /",
			"abcd<!-- c --",
			"abcd<!-- c --",
			[]ElemKind{ElemText},
		},
	}
	h := NewHtex(".", false)
	h.KeepComments = true
	testParsing(h, t, tests)
}

func TestDocType(t *testing.T) {
	tests := []ParseTest{
		{
			"GET /",
			"<!doctype html>",
			"<!doctype html>",
			[]ElemKind{ElemText},
		},
		{
			"GET /",
			"a<!DOCTYPE html>b<!DocType html>c",
			"a<!DOCTYPE html>b<!DocType html>c",
			[]ElemKind{ElemText},
		},
		{
			"GET /",
			"a<!doctype html",
			"a<!doctype html",
			[]ElemKind{ElemText},
		},
	}
	h := NewHtex(".", false)
	testParsing(h, t, tests)
}

func TestData(t *testing.T) {
	tests := []ParseTest{
		{
			"GET /",
			"a<!data x>b",
			"ab",
			[]ElemKind{ElemText, ElemData, ElemText},
		},
		{
			"GET /",
			"a<!data x>b<!data y>c",
			"abc",
			[]ElemKind{ElemText, ElemData, ElemText, ElemData, ElemText},
		},
	}
	h := NewHtex(".", false)
	testParsing(h, t, tests)
}

func TestMethodGet(t *testing.T) {
	tests := []ParseTest{
		{
			"GET /",
			"a<!method>b",
			"a",
			[]ElemKind{ElemText, ElemMethod, ElemText},
		},
		{
			"GET /",
			"a<!method any>b",
			"ab",
			[]ElemKind{ElemText, ElemMethod, ElemText},
		},
		{
			"GET /",
			"a<!method get>b<!method post>c",
			"ab",
			[]ElemKind{ElemText, ElemMethod, ElemText, ElemMethod, ElemText},
		},
		{
			"GET /",
			"a<!method get id>id",
			"a",
			[]ElemKind{ElemText, ElemMethod, ElemText},
		},
		{
			"GET /?id=42",
			"a,<!method get id>id=<!query id>",
			"a,id=42",
			[]ElemKind{ElemText, ElemMethod, ElemText, ElemQuery},
		},
		{
			"GET /?user=david&pass=abc",
			"<!query user>,<!query pass>",
			"david,abc",
			[]ElemKind{ElemQuery, ElemText, ElemQuery},
		},
	}
	h := NewHtex(".", false)
	testParsing(h, t, tests)
}
