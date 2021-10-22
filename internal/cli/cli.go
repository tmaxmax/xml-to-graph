package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

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

	usageFlagGlob = `A pattern that is used to match the files that will be converted. CLI arguments
have priority over this flag.`

	cliDescription = `
██╗░░██╗███╗░░░███╗██╗░░░░░░░░░░░████████╗░█████╗░░░░░░░░██████╗░██████╗░░█████╗░██████╗░██╗░░██╗
╚██╗██╔╝████╗░████║██║░░░░░░░░░░░╚══██╔══╝██╔══██╗░░░░░░██╔════╝░██╔══██╗██╔══██╗██╔══██╗██║░░██║
░╚███╔╝░██╔████╔██║██║░░░░░█████╗░░░██║░░░██║░░██║█████╗██║░░██╗░██████╔╝███████║██████╔╝███████║
░██╔██╗░██║╚██╔╝██║██║░░░░░╚════╝░░░██║░░░██║░░██║╚════╝██║░░╚██╗██╔══██╗██╔══██║██╔═══╝░██╔══██║
██╔╝╚██╗██║░╚═╝░██║███████╗░░░░░░░░░██║░░░╚█████╔╝░░░░░░╚██████╔╝██║░░██║██║░░██║██║░░░░░██║░░██║
╚═╝░░╚═╝╚═╝░░░░░╚═╝╚══════╝░░░░░░░░░╚═╝░░░░╚════╝░░░░░░░░╚═════╝░╚═╝░░╚═╝╚═╝░░╚═╝╚═╝░░░░░╚═╝░░╚═╝

xml-to-graph is a tool that converts XML files to input files for graph
exercises. Pass the paths you want to convert as arguments to the command:

	xml-to-graph path/to/file.xml path/to/another.xml

and the converted files 'file.in' and 'another.in' will be saved by default to the
location where xml-to-graph is called from. Customize the save location, output, and
more using the command's flags.

`
)

type CLI struct {
	outputDir string
	filepaths []string
	printer   *graph.Printer
	ch        chan string
	progress  chan struct{}
	gr        *errgroup.Group
	ctx       context.Context
	ps        *http.Server
	brp       sync.Pool
	verbose   bool
}

func New(args []string) *CLI {
	f := flag.NewFlagSet(cliName, flag.ExitOnError)
	formatString := f.String("format", "%n %m\n%M\n", usageFlagFormat)
	outputDir := f.String("output-dir", ".", usageFlagOutputDir)
	globPattern := f.String("glob", "", usageFlagGlob)
	profilerAddr := f.String("profiler", "", "The address for the pprof server (leave empty for disabling the profiler)")
	verboseOuput := f.Bool("verbose", false, "Show various information and progress")
	usage := f.Usage
	f.Usage = func() {
		fmt.Fprint(os.Stderr, cliDescription)
		usage()
	}
	f.Parse(args)

	fmtStr := *formatString
	if fmtStr == "" {
		fmtStr = f.Lookup("format").DefValue
	}

	p, err := graph.ParsePrinter(fmtStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n\n%s\n", err, usageFlagFormat)
		os.Exit(1)
	}

	filepaths := f.Args()
	if len(filepaths) == 0 && *globPattern != "" {
		ps, err := filepath.Glob(*globPattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "glob pattern invalid: %v\n", err)
			os.Exit(1)
		}

		filepaths = ps
	}

	c := &CLI{
		outputDir: *outputDir,
		filepaths: filepaths,
		printer:   p,
		ch:        make(chan string),
		progress:  make(chan struct{}),
		brp: sync.Pool{
			New: func() interface{} {
				return bufio.NewReader(nil)
			},
		},
		verbose: *verboseOuput,
	}

	if c.outputDir == "" {
		c.outputDir = f.Lookup("output-dir").DefValue
	}

	if err := os.MkdirAll(c.outputDir, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(2)
	}

	if *profilerAddr != "" {
		c.ps = &http.Server{Addr: *profilerAddr}
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)
	}

	return c
}

func (c *CLI) printf(format string, args ...interface{}) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

func (c *CLI) Run() int {
	l := len(c.filepaths)
	if l == 0 {
		c.printf("No files to process, exiting...\n")
		return 0
	}

	workers := runtime.GOMAXPROCS(-1)
	if l < workers {
		workers = l
	}

	c.printf("Starting file conversion...\nParallelism: %d workers\n", workers)
	abs, err := filepath.Abs(c.outputDir)
	if err == nil {
		c.printf("Output directory: %s\n", abs)
	}

	sctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gr, ctx := errgroup.WithContext(sctx)
	c.gr = gr
	c.ctx = ctx
	pserr := make(chan error, 1)

	if c.ps != nil {
		c.printf("Profiler server listening at %s\n", c.ps.Addr)
		shouldShutdown := true

		go func() {
			if err := c.ps.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				shouldShutdown = false
				fmt.Fprintf(os.Stderr, "Failed to run pprof server: %v\n", c.ps.Addr)
				cancel()
			}
		}()

		go func() {
			<-c.ctx.Done()
			if shouldShutdown {
				pserr <- c.ps.Shutdown(context.Background())
			} else {
				close(pserr)
			}
		}()

		defer func() {
			if err, ok := <-pserr; err != nil {
				fmt.Fprintf(os.Stderr, "Failed to shut down profiler server: %v\n", err)
			} else if ok {
				c.printf("Profiler server was successfully shut down!\n")
			}
		}()
	}

	for i := 0; i < workers; i++ {
		c.gr.Go(c.worker)
	}

	if c.verbose {
		c.gr.Go(c.outputProgress)
	}

	start := time.Now()
	c.gr.Go(c.sendPaths)

	waitErr := make(chan error)
	go func() { waitErr <- c.gr.Wait() }()

	select {
	case <-ctx.Done():
	case <-sctx.Done():
	}

	duration := time.Now().Sub(start)

	if err = <-waitErr; sctx.Err() != nil {
		c.printf("\nConversion stopped forcefully, exiting after %v...\n", duration)
		return 0
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "\nFailed to process files: %v\n", err)
		return 2
	} else {
		c.printf("\nAll files were successfully converted! Done in %v\n", duration)
		return 0
	}
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
	l := len(c.filepaths)

	printProgress := func() {
		progress := float64(done) / float64(len(c.filepaths))
		hashes := int(float64(barSize) * progress)
		dashes := barSize - hashes
		barStr := strings.Repeat("#", hashes) + strings.Repeat("-", dashes)
		fmt.Fprintf(os.Stderr, "Progress: [%s] %d/%d %.2f%%\r", barStr, done, l, progress*100)
	}

	printProgress()

	for {
		select {
		case <-c.progress:
			done++
			printProgress()
			if done == l {
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

	br := c.brp.Get().(*bufio.Reader)
	defer c.brp.Put(br)
	br.Reset(input)

	g, err := graph.FromXMLNoStd(br)
	if err != nil {
		return err
	}

	_, err = c.printer.Print(output, &g)
	if err != nil {
		return err
	}

	if c.verbose {
		select {
		case <-c.ctx.Done():
		case c.progress <- struct{}{}:
		}
	}

	return nil
}
