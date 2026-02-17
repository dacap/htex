// Copyright (c) David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package main

import (
	"os"
	"github.com/dacap/htex"
)

func main() {
	cli := htex.NewCLI(os.Args[0])
	cli.Run(os.Args[1:])
}
