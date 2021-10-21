package cli

import (
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gihtub.com/tmaxmax/xml-to-graph/internal/graph"
)

const (
	cliName = "xml-to-graph"

	usageFlagFormat = `A C-like format string that describes how the graphs should be written.
Your shell is responsible for handling escape sequences such as \n.

Available verbs:
 - %: print a literal percent sign
 - n: print the number of nodes in the graph
 - m: print the number of edges in the graph
 - a: print the adjacency matrix of the graph
 - {cost function}w: print the cost/weight of each node
 - {cost function}N: print the nodes in the graph, optionally together with their costs
 - {cost function}M: print the edges in the graph, optionally together with their costs

A cost function describes how the cost of a node/edge should be printed.
It is defined by a ratio and a rounding function. The cost function is applied
as following: the initial cost is multiplied with the ratio, then it is rounded
using the provided function. When a verb requires a cost function,
such as "%w", but none is provided, the default cost function is used:
the ratio is 1 and no rounding is applied. The cost function looks like this:
 {ratio}{rounding mode}
A ratio is a floating-point number in non-scientific notation. The following are valid
ratios:
 - .23: ratio of 0.23
 - 56: ratio of 56.0
 - 3.42: ratio of 3.42
The rounding mode names the rounding function to be used. It is optional in the
cost function. The following rounding modes are allowed:
 - X (default): no rounding
 - F: floor
 - R: round to nearest integer
 - C: ceil
Valid cost functions are:
 - .5: ratio 0.5, no rounding function
 - 3.6R: ratio 3.6, rounding to nearest integer

Format string examples:
 - "%n %m\n%.1Fw\n%M" - Prints the number of nodes and edges, on another line the costs
   of each node, floored and with a ratio of 0.1, and on the following lines all the
   edges without their costs.
 - "%n\n%a\n%.1FM" - Prints the number of nodes, the graph's adjacency matrix on the
   next line, and on the following lines each edge of the graph together with their
   costs, floored and with a ratio of 0.1`
)

type CLI struct {
	filepaths []string
	printer   *graph.Printer
}

func New(args []string) *CLI {
	f := flag.NewFlagSet(cliName, flag.ExitOnError)
	formatString := f.String("format", "%n %m\n%M\n", usageFlagFormat)
	f.Parse(args)

	p, err := graph.ParsePrinter(*formatString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n\n%s", err, usageFlagFormat)
		os.Exit(1)
	}

	return &CLI{filepaths: f.Args(), printer: p}
}

func (c *CLI) Run() int {
	for _, p := range c.filepaths {
		if err := c.processFile(p); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to process file %q: %v\n", p, err)
			return 2
		}
	}

	return 0
}

func (c *CLI) processFile(path string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()

	outputPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".in"
	log.Println(outputPath)
	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	var g graph.Graph
	if err := xml.NewDecoder(input).Decode(&g); err != nil {
		return err
	}

	_, err = c.printer.Print(output, &g)
	return err
}
