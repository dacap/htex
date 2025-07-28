// Copyright 2025 David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dacap/htex"
)

var defUsage func()

func baseUsage() {
	defUsage()
	fmt.Println("commands:")
	fmt.Println("  htex server")
	fmt.Println("  htex gen")
	fmt.Println("  htex help")
}

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "verbose", false, "verbose output")

	var fullchain, privkey, root, output string
	var port int
	server := flag.NewFlagSet("server", flag.ExitOnError)
	server.IntVar(&port, "port", 0, "port to listen (80 or 443 by default)")
	server.StringVar(&fullchain, "fullchain", "", "TLS certificate")
	server.StringVar(&privkey, "privkey", "", "private key for the TLS certificate")
	server.StringVar(&root, "root", "", "root directory to serve content ('public' by default)")

	gen := flag.NewFlagSet("gen", flag.ExitOnError)
	gen.StringVar(&root, "root", "", "source directory to scan")
	gen.StringVar(&output, "output", "", "output of the generation")

	flag.NewFlagSet("help", flag.ExitOnError)

	defUsage = flag.Usage
	flag.Usage = baseUsage
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		return
	}

	cmd := flag.Args()[0]
	switch cmd {
	case "server":
		server.Parse(flag.Args()[1:])
		if root != "" {
			root, _ = filepath.Abs(root)
		} else {
			root, _ = filepath.Abs("public")
		}
		h := htex.NewHtex(root, verbose)
		h.RunWebServer(port, fullchain, privkey)
	case "gen":
		gen.Parse(flag.Args()[1:])
		if root != "" {
			root, _ = filepath.Abs(root)
		} else {
			root, _ = filepath.Abs("public")
		}
		if output != "" {
			output, _ = filepath.Abs(output)
		} else {
			output, _ = filepath.Abs("output")
		}
		h := htex.NewHtex(root, verbose)
		h.GenerateStaticContent(output)
	case "help":
		if flag.NArg() >= 2 {
			cmd := flag.Args()[1]
			switch cmd {
			case "server":
				server.Usage()
			case "gen":
				gen.Usage()
			}
		} else {
			flag.Usage()
		}
	default:
		fmt.Println("invalid argument", cmd)
		os.Exit(1)
	}
}
