package graph_test

import (
	"bufio"
	"encoding/xml"
	"reflect"
	"testing"

	"gihtub.com/tmaxmax/xml-to-graph/internal/graph"
)

var expected = graph.Graph{
	Nodes: []graph.Node{
		{ID: 1, Cost: 30.0},
		{ID: 2, Cost: 20.0},
		{ID: 3, Cost: 10.0},
	},
	Edges: []graph.Edge{
		{Src: 2, Dst: 1, Cost: 50.0},
		{Src: 3, Dst: 2, Cost: 100.0, Directed: true},
	},
}

func TestFromXML(t *testing.T) {
	f, _ := benchFiles.Open("testfile.xml")
	g, err := graph.FromXML(f)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(g, expected) {
		t.Fatalf("Invalid output:\nexpected %+v\nreceived %+v", expected, g)
	}
}

func TestFromXMLNoStd(t *testing.T) {
	f, _ := benchFiles.Open("testfile.xml")
	g, err := graph.FromXMLNoStd(bufio.NewReader(f))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(g, expected) {
		t.Fatalf("Invalid output:\nexpected %+v\nreceived %+v", expected, g)
	}
}

func BenchmarkGraphUnmarshalXML_reflect(b *testing.B) {
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		f, _ := benchFiles.Open("benchfile.xml")
		var g graph.Graph
		if err := xml.NewDecoder(f).Decode(&g); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraphUnmarshalXML_rawToken(b *testing.B) {
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		f, _ := benchFiles.Open("benchfile.xml")
		if _, err := graph.FromXML(f); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraphUnmarshalXML_noStd(b *testing.B) {
	br := bufio.NewReader(nil)

	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		f, _ := benchFiles.Open("benchfile.xml")
		br.Reset(f)
		if _, err := graph.FromXMLNoStd(br); err != nil {
			b.Fatal(err)
		}
	}
}
