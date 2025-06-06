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
	text     string
	expected string
	elems    []ElemKind
}

func testParsing(h *Htex, t *testing.T, tests []ParseTest) {
	r := &http.Request{Method: "GET"}
	r.URL = &url.URL{}

	for _, test := range tests {
		w := &memoryResponseWriter{}
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
			"",
			"",
			[]ElemKind{},
		},
		{
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
			"a<!-->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"a<!--",
			"a",
			[]ElemKind{ElemText},
		},
		{
			"a<!--->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"a<!---->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"a<!-- c -->b",
			"ab",
			[]ElemKind{ElemText, ElemText},
		},
		{
			"abc<!-- c -->",
			"abc",
			[]ElemKind{ElemText},
		},
		{
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
			"a<!-->b",
			"a<!-->b",
			[]ElemKind{ElemText},
		},
		{
			"a<!-- c -->b",
			"a<!-- c -->b",
			[]ElemKind{ElemText},
		},
		{
			"abc<!-- c -->",
			"abc<!-- c -->",
			[]ElemKind{ElemText},
		},
		{
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
			"<!doctype html>",
			"<!doctype html>",
			[]ElemKind{ElemText},
		},
		{
			"a<!DOCTYPE html>b<!DocType html>c",
			"a<!DOCTYPE html>b<!DocType html>c",
			[]ElemKind{ElemText},
		},
		{
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
			"a<!data x>b",
			"ab",
			[]ElemKind{ElemText, ElemData, ElemText},
		},
		{
			"a<!data x>b<!data y>c",
			"abc",
			[]ElemKind{ElemText, ElemData, ElemText, ElemData, ElemText},
		},
	}
	h := NewHtex(".", false)
	testParsing(h, t, tests)
}
