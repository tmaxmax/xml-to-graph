package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"gihtub.com/tmaxmax/xml-to-graph/internal/graph"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	inputFile := flag.String("i", "", "The file to parse the graph data from. Defaults to stdin.")
	outputFile := flag.String("o", "", "The file to write the output to. Defaults to stdout.")

	flag.Parse()

	var input io.Reader = os.Stdin
	var output io.Writer = os.Stdout

	if *inputFile != "" {
		f, err := os.Open(*inputFile)
		if err != nil {
			log.Printf("Failed to open file %q: %v", *inputFile, err)
			return 1
		}
		defer f.Close()

		input = f
	}

	if *outputFile != "" {
		f, err := os.Create(*outputFile)
		if err != nil {
			log.Printf("Failed to open output file: %q: %v\n", *outputFile, err)
			return 1
		}
		defer f.Close()

		output = f
	}

	var g graph.Graph
	if err := xml.NewDecoder(input).Decode(&g); err != nil {
		log.Printf("Failed to parse XML input: %v\n", err)
		return 1
	}

	_, err := fmt.Fprintf(output, "%d\n%d\n", len(g.Nodes), len(g.Edges))
	if err != nil {
		log.Printf("Failed to write output: %v\n", err)
		return 1
	}
	for _, e := range g.Edges {
		_, err := fmt.Fprintf(output, "%d %d\n", e.Src, e.Dst)
		if err != nil {
			log.Printf("Failed to write output: %v\n", err)
			return 1
		}
	}

	return 0
}
