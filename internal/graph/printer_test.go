package graph_test

import (
	"strings"
	"testing"

	"gihtub.com/tmaxmax/xml-to-graph/internal/graph"
)

func TestParsePrinter(t *testing.T) {
	type testCase struct {
		format string
		hasErr bool
		output string
		graph  graph.Graph
	}

	tests := []testCase{
		{format: "", hasErr: true},
		{format: "%", hasErr: true},
		{format: "%.5", hasErr: true},
		{format: "%5z", hasErr: true},
		{format: "%.4AN", hasErr: true},
		{format: "%x", hasErr: true},
		{
			format: "Nodes: %n\n%N\n\nEdges: %m\n%2RM\n\nCosts: %w\n\nAdjacency matrix:\n%a\n",
			graph: graph.Graph{
				Nodes: []graph.Node{
					{ID: 1, Cost: 1.5},
					{ID: 2, Cost: 0.2},
					{ID: 3, Cost: 4},
				},
				Edges: []graph.Edge{
					{Src: 1, Dst: 2, Cost: 0.3},
					{Src: 1, Dst: 3, Cost: 0.1},
					{Src: 2, Dst: 3, Cost: 1.4},
				},
			},
			output: `Nodes: 3
1
2
3

Edges: 3
1 2 1
1 3 0
2 3 3

Costs: 1.5 0.2 4

Adjacency matrix:
0 1 1
1 0 1
1 1 0
`,
		},
	}

	for _, test := range tests {
		t.Run(test.format, func(t *testing.T) {
			p, err := graph.ParsePrinter(test.format)
			if test.hasErr != (err != nil) {
				t.Fatalf("Error expected: %t, error received: %v", test.hasErr, err)
			}
			if test.hasErr {
				t.Log(err)
				return
			}

			sb := strings.Builder{}
			_, _ = p.Print(&sb, &test.graph)

			if sb.String() != test.output {
				t.Fatalf("Invalid output:\nexpected:%q\nreceived%q\n", test.output, sb.String())
			}
		})
	}
}
