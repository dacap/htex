// Copyright (c) David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"bufio"
	"strings"
	"testing"
)

type LexerTest struct {
	text  string
	kinds []Tok
	texts []string
}

func testLexer(l *Lexer, t *testing.T, tests []LexerTest) {
	for _, test := range tests {
		s := bufio.NewScanner(strings.NewReader(test.text))
		result, err := l.lexScanner("test.htex", s)
		if err != nil {
			t.Error(err)
		}

		// Check tokens
		if len(result.tokens) != len(test.kinds) {
			t.Errorf("parsing '%s' generated %d tokens (expected '%d')\n", test.text, len(result.tokens), len(test.kinds))
			t.Errorf("result tokens (%d):\n", len(result.tokens))
			for i := 0; i < len(result.tokens); i++ {
				t.Errorf(" [%d] = %v\n", i, result.tokens[i])
			}
			t.Errorf("expected tokens (%d):\n", len(test.kinds))
			for i := 0; i < len(test.kinds); i++ {
				t.Errorf(" [%d] = %v\n", i, test.kinds[i])
			}
			return
		}
		for i := 0; i < len(test.kinds); i++ {
			if test.kinds[i] != result.tokens[i].kind {
				t.Errorf("parsing '%s' token %d is %v (expected %v)\n", test.text, i, result.tokens[i].kind, test.kinds[i])
			}
			if test.texts[i] != result.tokens[i].text {
				t.Errorf("parsing '%s' token %d is %v (expected %v)\n", test.text, i, result.tokens[i].text, test.texts[i])
			}
		}
	}
}

func TestLexerBasic(t *testing.T) {
	tests := []LexerTest{
		{
			"",
			[]Tok{},
			[]string{},
		},
		{
			"a",
			[]Tok{TokText},
			[]string{"a"},
		},
	}
	l := NewLexer()
	testLexer(l, t, tests)
}

func TestLexerSkipComments(t *testing.T) {
	tests := []LexerTest{
		{
			"a<!-->b",
			[]Tok{TokText, TokText},
			[]string{"a", "b"},
		},
		{
			"a<!--",
			[]Tok{TokText},
			[]string{"a"},
		},
		{
			"a<!--->b",
			[]Tok{TokText, TokText},
			[]string{"a", "b"},
		},
		{
			"a<!---->b",
			[]Tok{TokText, TokText},
			[]string{"a", "b"},
		},
		{
			"a<!-- c -->b",
			[]Tok{TokText, TokText},
			[]string{"a", "b"},
		},
		{
			"abc<!-- c -->",
			[]Tok{TokText},
			[]string{"abc"},
		},
		{
			"abcd<!-- c --",
			[]Tok{TokText},
			[]string{"abcd"},
		},
	}
	l := NewLexer()
	testLexer(l, t, tests)
}

func TestLexerKeepComments(t *testing.T) {
	tests := []LexerTest{
		{
			"a<!-->b",
			[]Tok{TokText},
			[]string{"a<!-->b"},
		},
		{
			"a<!-- c -->b",
			[]Tok{TokText},
			[]string{"a<!-- c -->b"},
		},
		{
			"abc<!-- c -->",
			[]Tok{TokText},
			[]string{"abc<!-- c -->"},
		},
		{
			"abcd<!-- c --",
			[]Tok{TokText},
			[]string{"abcd<!-- c --"},
		},
	}
	l := NewLexer()
	l.KeepComments = true
	testLexer(l, t, tests)
}

func TestLexerDocType(t *testing.T) {
	tests := []LexerTest{
		{
			"<!doctype html>",
			[]Tok{TokText},
			[]string{"<!doctype html>"},
		},
		{
			"a<!DOCTYPE html>b<!DocType html>c",
			[]Tok{TokText},
			[]string{"a<!DOCTYPE html>b<!DocType html>c"},
		},
		{
			"a<!doctype html",
			[]Tok{TokText},
			[]string{"a<!doctype html"},
		},
	}
	l := NewLexer()
	testLexer(l, t, tests)
}

func TestLexerElem(t *testing.T) {
	tests := []LexerTest{
		{
			"a<!b>c",
			[]Tok{TokText, TokElemBegin, TokElemEnd, TokText},
			[]string{"a", "<!b", ">", "c"},
		},
		{
			"a<!b x>d<!e y>f",
			[]Tok{TokText, TokElemBegin, TokText, TokElemEnd, TokText, TokElemBegin, TokText, TokElemEnd, TokText},
			[]string{"a", "<!b", "x", ">", "d", "<!e", "y", ">", "f"},
		},
		{
			"<!a><!b><!c>",
			[]Tok{TokElemBegin, TokElemEnd, TokElemBegin, TokElemEnd, TokElemBegin, TokElemEnd},
			[]string{"<!a", ">", "<!b", ">", "<!c", ">"},
		},
	}
	l := NewLexer()
	testLexer(l, t, tests)
}
