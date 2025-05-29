// Copyright 2025 David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package main

import (
	"log"
	"net/http"
	"time"
)

type logResponseWriter struct {
	w    http.ResponseWriter
	code int
}

func (w *logResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *logResponseWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

func (w *logResponseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
	w.w.WriteHeader(statusCode)
}

type LogHtexHandler struct {
	handler HtexHandler
	w       logResponseWriter
}

func (h *LogHtexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h.w = logResponseWriter{w: w, code: 200}
	h.handler.ServeHTTP(&h.w, r)
	log.Println(" -> response code", h.w.code, "time", time.Since(start))
}
