// Copyright (c) David Capello. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.txt file.

package htex

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type CLI struct {
	ExeName   string
	EnableGen bool
	flag      *flag.FlagSet
	defUsage  func()
}

func NewCLI(exeName string) CLI {
	cli := CLI{}
	cli.ExeName = exeName
	cli.EnableGen = true
	return cli
}

func baseUsage(c *CLI) {
	c.defUsage()

	out := c.flag.Output()
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  ", c.ExeName, "server")
	if c.EnableGen {
		fmt.Fprintln(out, "  ", c.ExeName, "gen")
	}
	fmt.Fprintln(out, "  ", c.ExeName, "help")
}

func (c *CLI) invalidArgExit(arg string) {
	fmt.Fprintln(c.flag.Output(), "invalid argument", arg)
	c.flag.Usage()
	os.Exit(1)
}

func (c *CLI) Run(args []string) {
	c.flag = flag.NewFlagSet(c.ExeName, flag.ExitOnError)

	var verbose bool
	c.flag.BoolVar(&verbose, "verbose", false, "verbose output")

	var fullchain, privkey, root, output string
	var port int
	server := flag.NewFlagSet(c.ExeName+" server", flag.ExitOnError)
	server.IntVar(&port, "port", 0, "port to listen (80 or 443 by default)")
	server.StringVar(&fullchain, "fullchain", "", "TLS certificate")
	server.StringVar(&privkey, "privkey", "", "private key for the TLS certificate")
	server.StringVar(&root, "root", "", "root directory to serve content ('public' by default)")

	gen := flag.NewFlagSet(c.ExeName+" gen", flag.ExitOnError)
	gen.StringVar(&root, "root", "", "source directory to scan")
	gen.StringVar(&output, "output", "", "output of the generation")

	flag.NewFlagSet("help", flag.ExitOnError)

	c.defUsage = c.flag.Usage
	c.flag.Usage = func() { baseUsage(c) }
	c.flag.Parse(args)

	if c.flag.NArg() == 0 {
		c.flag.Usage()
		return
	}

	cmd := c.flag.Args()[0]
	switch cmd {
	case "server":
		server.Parse(c.flag.Args()[1:])
		if root != "" {
			root, _ = filepath.Abs(root)
		} else {
			root, _ = filepath.Abs("public")
		}
		h := NewHtex(root, verbose)
		h.RunWebServer(port, fullchain, privkey)
	case "gen":
		if !c.EnableGen {
			c.invalidArgExit(cmd)
		}
		gen.Parse(c.flag.Args()[1:])
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
		h := NewHtex(root, verbose)
		h.GenerateStaticContent(output)
	case "help":
		if c.flag.NArg() >= 2 {
			cmd := c.flag.Args()[1]
			switch cmd {
			case "server":
				server.Usage()
			case "gen":
				if c.EnableGen {
					gen.Usage()
				}
			default:
				c.invalidArgExit(cmd)
			}
		} else {
			c.flag.Usage()
		}
	default:
		c.invalidArgExit(cmd)
	}
}
