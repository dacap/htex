// Copyright (c) David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (h *Htex) ScanFiles(dynamicQuery, staticFile func(fullFn, query string)) {
	filepath.Walk(h.localRoot, func(fullFn string, info os.FileInfo, err error) error {
		fn := filepath.ToSlash(fullFn[len(h.localRoot):])

		// Skip hidden files folders
		if info.IsDir() {
			if strings.HasPrefix(fn, "/.") &&
				!strings.HasPrefix(fn, "/.well-known") {
				return filepath.SkipDir
			}
			return nil
		}

		var query string
		ext := path.Ext(fn)
		if ext == ".htex" {
			// Convert the filename into a URL pattern
			query = fn[:len(fn)-len(ext)]
			queryLen := len(query)
			if queryLen >= 6 && query[queryLen-6:] == "/index" {
				query = query[0 : queryLen-5]
			}
			dynamicQuery(fullFn, query)
		} else {
			staticFile(fullFn, fn)
		}
		return nil
	})
}
