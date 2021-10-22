package cli

import (
	"context"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gihtub.com/tmaxmax/xml-to-graph/internal/graph"
	"golang.org/x/sync/errgroup"
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
   costs, floored and with a ratio of 0.1
`

	usageFlagOutputDir = `The directory to output the converted files to.`
)

type CLI struct {
	outputDir string
	filepaths []string
	printer   *graph.Printer
	ch        chan string
	progress  chan struct{}
	gr        *errgroup.Group
	ctx       context.Context
}

func New(args []string) *CLI {
	f := flag.NewFlagSet(cliName, flag.ExitOnError)
	formatString := f.String("format", "%n %m\n%M\n", usageFlagFormat)
	outputDir := f.String("output-dir", ".", usageFlagOutputDir)
	f.Parse(args)

	fmtStr := *formatString
	if fmtStr == "" {
		fmtStr = f.Lookup("format").DefValue
	}

	p, err := graph.ParsePrinter(fmtStr)
	if err != nil {
		fatalf("%v\n\n%s", err, usageFlagFormat)
	}

	gr, ctx := errgroup.WithContext(context.Background())
	c := &CLI{
		outputDir: *outputDir,
		filepaths: f.Args(),
		printer:   p,
		ch:        make(chan string),
		progress:  make(chan struct{}),
		gr:        gr,
		ctx:       ctx,
	}

	if c.outputDir == "" {
		c.outputDir = f.Lookup("output-dir").DefValue
	}

	return c
}

func (c *CLI) Run() int {
	l := len(c.filepaths)
	if l == 0 {
		printf("No files to process, exiting...\n")
		return 0
	}

	workers := runtime.GOMAXPROCS(-1)
	if l < workers {
		workers = l
	}

	printf("Starting file conversion...\n")
	printf("Parallelism: %d workers\n", workers)
	abs, err := filepath.Abs(c.outputDir)
	if err == nil {
		printf("Output directory: %s\n", abs)
	}

	for i := 0; i < workers; i++ {
		c.gr.Go(c.worker)
	}

	c.gr.Go(c.outputProgress)
	c.gr.Go(c.sendPaths)

	if err := c.gr.Wait(); err != nil {
		printf("\nFailed to process files: %v\n", err)
		return 2
	}

	printf("\n")

	return 0
}

func (c *CLI) sendPaths() error {
	defer close(c.ch)

	for _, p := range c.filepaths {
		select {
		case c.ch <- p:
		case <-c.ctx.Done():
			return nil
		}
	}

	return nil
}

func (c *CLI) worker() error {
	for {
		select {
		case p, ok := <-c.ch:
			if !ok {
				return nil
			}
			if err := c.processFile(p); err != nil {
				return err
			}
		case <-c.ctx.Done():
			return nil
		}
	}
}

func (c *CLI) outputProgress() error {
	const barSize = 40

	var done int

	printProgress := func() {
		l := len(c.filepaths)
		p := float64(done) / float64(l)
		hashes := int(float64(barSize) * p)
		dashes := barSize - hashes
		barStr := strings.Repeat("#", hashes) + strings.Repeat("-", dashes)
		printf("Progress: [%s] %d/%d %d%%\r\r", barStr, done, l, int(p*100+0.5))
	}

	printProgress()

	for {
		select {
		case <-c.progress:
			done++
			printProgress()
			if done == len(c.filepaths) {
				return nil
			}
		case <-c.ctx.Done():
			return nil
		}
	}
}

func (c *CLI) processFile(path string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()

	outputPath := filepath.Join(c.outputDir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".in")
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
	if err != nil {
		return err
	}

	select {
	case <-c.ctx.Done():
	case c.progress <- struct{}{}:
	}

	return nil
}
