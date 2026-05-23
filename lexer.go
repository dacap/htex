// Copyright (c) David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"bufio"
	"bytes"
	"log"
	"os"
	"strings"
)

type Tok int

const (
	TokEof Tok = iota
	TokNone
	TokText      // Regular HTML text to print
	TokElemBegin // <!element ...
	TokElemEnd   // ... >
)

type Token struct {
	kind Tok
	text string
}

type Tokens struct {
	tokens []Token
}

type Lexer struct {
	KeepComments bool
}

func splitTokens(l *Lexer) func([]byte, bool) (int, []byte, error) {
	insideElem := false
	insideComment := false
	closingElem := false
	return func(data []byte, atEOF bool) (int, []byte, error) {
		if closingElem {
			closingElem = false
			return 1, data[0:1], nil
		}
		for i := 0; i < len(data); i++ {
			if insideElem {
				if data[i] == ' ' || data[i] == '\r' || data[i] == '\n' {
					var j = i
					for ; i < len(data) &&
						(data[i] == ' ' ||
							data[i] == '\r' ||
							data[i] == '\n'); i++ {
					}
					return i, data[:j], nil
				} else if data[i] == '>' {
					insideElem = false
					closingElem = true
					return i, data[:i], nil
				}
			} else if insideComment {
				// Here l.KeepComments is always false, as if we keep
				// comments it will be part of a TokText, not a
				// separated token.
				if !l.KeepComments {
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
					// TokText token.
					if l.KeepComments {
						for ; i+3 < len(data) &&
							(data[i] != '-' || data[i+1] != '-' || data[i+2] != '>'); i++ {
						}
						i += 2
					} else {
						insideComment = true
						return i, data[:i], nil
					}
				} else {
					// <!htex-element...
					insideElem = true
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
							closingElem = true
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

func (l *Lexer) lexFile(fn string) (*Tokens, error) {
	file, err := os.Open(fn)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	return l.lexScanner(fn, scanner)
}

func (l *Lexer) lexScanner(fn string, scanner *bufio.Scanner) (*Tokens, error) {
	tokens := &Tokens{}

	insideElem := false
	var T string
	scanner.Split(splitTokens(l))

	nextToken := true
	for true {
		if nextToken {
			if !scanner.Scan() {
				break
			}
		} else {
			nextToken = true
		}

		token := Token{TokNone, ""}

		T = scanner.Text()
		if len(T) > 2 && T[0] == '<' && T[1] == '!' {
			t := strings.ToLower(T)
			if strings.HasPrefix(t, "<!doctype") {
				token = Token{TokText, T}
			} else if strings.HasPrefix(t, "<!--") {
				if l.KeepComments {
					token = Token{TokText, T}
				} else {
					// Ignore the whole comment token (which includes "<!-- ... -->")
				}
			} else {
				insideElem = true

				token := Token{TokElemBegin, T}
				tokens.tokens = append(tokens.tokens, token)

				for scanner.Scan() {
					if scanner.Text() == ">" {
						nextToken = false
						break
					}

					text := scanner.Text()
					token := Token{TokText, text}
					tokens.tokens = append(tokens.tokens, token)
				}
			}
		} else if insideElem {
			if T == ">" {
				insideElem = false
				token = Token{TokElemEnd, T}
			} else {
				panic("token '>' not found")
			}
		} else if T != "" {
			token = Token{TokText, T}
		}
		if token.kind != TokNone {
			tokens.tokens = append(tokens.tokens, token)
		}
	}
	return tokens, nil
}

func NewLexer() *Lexer {
	l := &Lexer{
		KeepComments: false,
	}
	return l
}
