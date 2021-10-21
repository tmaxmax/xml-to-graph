package main

import (
	"os"

	"gihtub.com/tmaxmax/xml-to-graph/internal/cli"
)

func main() {
	c := cli.New(os.Args[1:])
	os.Exit(c.Run())
}
